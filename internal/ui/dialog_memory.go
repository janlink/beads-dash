package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/form"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

const (
	fMemKey     = "key"
	fMemContent = "content"
	contentRows = 6
)

// memConflict is what a re-read found different from the version a dialog
// opened on: theirs is the other content, gone says the key was forgotten.
type memConflict struct {
	theirs string
	gone   bool
}

func (*memConflict) Error() string { return "the memory changed since it was opened" }

// memExists says the target key already holds a memory.
type memExists struct{ key, content string }

func (e *memExists) Error() string { return "memory " + e.key + " exists" }

// memBoth says a rename stored the new key but could not forget the old one.
type memBoth struct {
	added, old string
	err        error
}

func (e *memBoth) Error() string {
	return fmt.Sprintf("both keys now exist: %s and %s; forgetting %s failed: %s", e.added, e.old, e.old, shortLine(e.err.Error()))
}

func (e *memBoth) Unwrap() error { return e.err }

// memoryDialog creates a memory or edits one; a changed key renames it.
type memoryDialog struct {
	formDialog
	edit bool
	// orig is the key being edited and base the content it had when the
	// dialog opened, or when the viewer last accepted a newer one.
	orig, base string
	// slugged is set while the key still follows the content.
	slugged  bool
	conflict *memConflict
}

func (a *App) newMemoryDialog(key, content string, edit bool) *memoryDialog {
	d := &memoryDialog{edit: edit, orig: key, base: content, slugged: !edit}
	title := "New memory"
	if edit {
		title = "Edit memory"
	}
	kf := form.NewText(fMemKey, "Key", key)
	kf.Required = true
	kf.Placeholder = "follows the content until you type here"
	cf := form.NewArea(fMemContent, "Content", content, contentRows)
	cf.Required = true
	d.f = form.New(kf, cf)
	d.f.Track = edit
	if d.slugged && content != "" {
		kf.Set(model.MemorySlug(content))
		kf.Rebase()
	}
	if !edit {
		d.f.FocusKey(fMemContent)
	}
	d.formDialog = formDialog{a: a, h: d, self: d, title: title, f: d.f}
	return d
}

// openMemory opens the memory dialog: for the memory key when editing, else
// empty.
func (a *App) openMemory(key string, edit bool) tea.Cmd {
	if edit {
		mem, ok := a.mem.get(key)
		if !ok {
			a.hint = "no memory to edit"
			return nil
		}
		a.pushDialog(a.newMemoryDialog(mem.Key, mem.Content, true))
		return nil
	}
	a.pushDialog(a.newMemoryDialog("", "", false))
	return nil
}

func (d *memoryDialog) dirty() bool {
	if d.edit {
		return d.f.Changes() > 0
	}
	return d.f.Field(fMemContent).Changed() || !d.slugged && d.f.Field(fMemKey).Changed()
}

func (d *memoryDialog) aside() string { return "" }

func (d *memoryDialog) conflicted() bool { return d.conflict != nil && !d.conflict.gone }

func (d *memoryDialog) pick(*form.Field) tea.Cmd { return nil }

func (d *memoryDialog) edited(f *form.Field) {
	switch f.Key {
	case fMemKey:
		d.slugged = false
	case fMemContent:
		if d.slugged {
			d.f.Field(fMemKey).Set(model.MemorySlug(f.Value()))
		}
	}
}

func (d *memoryDialog) extra(act keys.Action) tea.Cmd {
	c := d.conflict
	if c == nil || c.gone {
		return nil
	}
	switch act { //nolint:exhaustive // only the conflict actions are extra
	case keys.KeepMine:
		d.base, d.conflict = c.theirs, nil
	case keys.TakeTheirs, keys.Reload:
		f := d.f.Field(fMemContent)
		f.Set(c.theirs)
		f.Rebase()
		d.base, d.conflict = c.theirs, nil
	}
	return nil
}

func (d *memoryDialog) banner(l look.Look, w int) []string {
	c := d.conflict
	if c == nil {
		return nil
	}
	if c.gone {
		return []string{l.Paint(theme.Warning, "! Forgotten since opened. Saving stores it again."), ""}
	}
	out := []string{l.Paint(theme.Warning, "! Changed since opened. The other version:")}
	lines := splitLines(c.theirs)
	for _, line := range lines[:min(len(lines), 3)] {
		out = append(out, l.Paint(theme.Dim, "  "+l.Fit(line, w-2)))
	}
	if extra := len(lines) - 3; extra > 0 {
		out = append(out, l.Paint(theme.Faint, fmt.Sprintf("  %s %d more lines", l.Glyphs.Ellipsis, extra)))
	}
	return append(out, l.Paint(theme.Warning, "  Ctrl+O saves mine over it, Ctrl+R takes theirs."), "")
}

// Memories reacts to a fresh read: it raises the conflict banner when the
// memory being edited changed or is gone.
func (d *memoryDialog) Memories() {
	if !d.edit || d.busy {
		return
	}
	mem, ok := d.a.mem.get(d.orig)
	mine := d.f.Field(fMemContent).Value()
	switch {
	case !ok:
		d.conflict = &memConflict{gone: true}
	case mem.Content == d.base || mem.Content == mine:
		d.base = mem.Content
		d.conflict = nil
	default:
		d.conflict = &memConflict{theirs: mem.Content}
	}
}

func (d *memoryDialog) values() (key, content string) {
	return strings.TrimSpace(d.f.Field(fMemKey).Value()), d.f.Field(fMemContent).Value()
}

func (d *memoryDialog) validate() bool {
	key, content := d.values()
	kf, cf := d.f.Field(fMemKey), d.f.Field(fMemContent)
	kf.Err, cf.Err = "", ""
	bad := ""
	if err := bd.CheckMemoryKey(key); err != nil {
		kf.Err, bad = "Key: "+err.Error(), fMemKey
	}
	if strings.TrimSpace(content) == "" {
		cf.Err = "Content is required"
		if bad == "" {
			bad = fMemContent
		}
	}
	if bad != "" {
		d.f.FocusKey(bad)
		return false
	}
	return true
}

func (d *memoryDialog) submit() tea.Cmd { return d.save(false) }

// save validates and writes. overwrite says the viewer already agreed to
// replace a memory that holds the target key.
func (d *memoryDialog) save(overwrite bool) tea.Cmd {
	if !d.validate() {
		return nil
	}
	key, content := d.values()
	edit, orig, base := d.edit, d.orig, d.base
	if edit && key == orig && content == base && (d.conflict == nil || !d.conflict.gone) {
		d.a.dropDialog(d.self)
		return nil
	}
	return d.start(writeOp{
		run: func(ctx context.Context, c bd.Client) (string, error) {
			list, err := c.Memories(ctx)
			if err != nil {
				return "", err
			}
			if edit {
				if at, ok := findMemory(list, orig); ok && at.Content != base {
					return "", &memConflict{theirs: at.Content}
				}
			}
			if !overwrite && (!edit || key != orig) {
				if x, ok := findMemory(list, key); ok {
					return "", &memExists{key: key, content: x.Content}
				}
			}
			if err := c.Remember(ctx, key, content); err != nil {
				return "", err
			}
			if edit && key != orig {
				if err := c.Forget(ctx, orig); err != nil && !errors.Is(err, bd.ErrMemoryGone) {
					return key, &memBoth{added: key, old: orig, err: err}
				}
			}
			return key, nil
		},
	})
}

func findMemory(list []model.Memory, key string) (model.Memory, bool) {
	for _, m := range list {
		if m.Key == key {
			return m, true
		}
	}
	return model.Memory{}, false
}

func (d *memoryDialog) finish(res writeResult) bool {
	a := d.a
	a.memRefresh()
	key, content := d.values()
	var conflict *memConflict
	var exists *memExists
	var both *memBoth
	switch {
	case res.err == nil:
		switch {
		case !d.edit:
			a.toast("remembered " + key)
		case key != d.orig:
			a.toast(fmt.Sprintf("renamed %s %s %s", d.orig, a.look.Glyphs.Arrow, key))
			a.mem.ownGone[d.orig] = true
		default:
			a.toast("updated " + key)
		}
		a.mem.follow = key
		return true
	case errors.As(res.err, &conflict):
		d.conflict = conflict
		return false
	case errors.As(res.err, &exists):
		a.pushDialog(&memOverwriteDialog{a: a, from: d, key: exists.key, content: exists.content})
		return false
	case errors.As(res.err, &both):
		a.mem.follow = key
		d.orig, d.base, d.conflict = key, content, nil
		d.f.Field(fMemKey).Rebase()
		d.f.Field(fMemContent).Rebase()
		d.fail(res.err)
		a.warn(res.err.Error())
		return false
	}
	d.fail(res.err)
	return false
}

// memOverwriteDialog asks before a memory is replaced by another one that
// takes its key.
type memOverwriteDialog struct {
	a       *App
	from    *memoryDialog
	key     string
	content string
}

func (*memOverwriteDialog) Context() keys.Context { return keys.Overwrite }

func (d *memOverwriteDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	switch act { //nolint:exhaustive // only the actions of the overwrite context arrive
	case keys.DoOverwrite:
		return d.from.save(true), true
	case keys.KeepBoth:
		d.from.f.Field(fMemKey).Set(d.freeKey())
		d.from.slugged = false
		return d.from.save(false), true
	case keys.Close:
		return nil, true
	}
	return nil, false
}

// freeKey is the target key with a number added that no memory has.
func (d *memOverwriteDialog) freeKey() string {
	taken := memKeys(d.a.mem.list)
	for n := 2; ; n++ {
		if k := fmt.Sprintf("%s-%d", d.key, n); !slices.Contains(taken, k) {
			return k
		}
	}
}

func (*memOverwriteDialog) Update(tea.Msg) tea.Cmd { return nil }

func (d *memOverwriteDialog) Frame(l look.Look, cols, rows int) dialog.Frame {
	iw := cols - 4
	if !dialog.FullScreen(cols, rows) {
		iw = min(dialog.MaxWidth, cols-4) - 4
	}
	body := []string{l.Paint(theme.Text, l.Fit(fmt.Sprintf("Memory %q exists:", d.key), iw))}
	if line := model.FirstLine(d.content); line != "" {
		body = append(body, l.Paint(theme.Dim, l.Fit("  "+line, iw)))
	}
	body = append(body, "", l.Paint(theme.Dim, "Overwrite replaces it. Keep both stores yours under "+d.freeKey()+"."))
	return dialog.Frame{Title: "Overwrite memory?", Hints: d.a.hintsFor(keys.Overwrite), Body: body}
}

// memPreviewDialog shows one memory in full, as the layer of narrow
// terminals.
type memPreviewDialog struct {
	a      *App
	key    string
	scroll int
}

func (*memPreviewDialog) Context() keys.Context { return keys.MemoryPreview }

func (d *memPreviewDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	switch act { //nolint:exhaustive // only the actions of the preview context arrive
	case keys.Close:
		return nil, true
	case keys.Markdown:
		d.a.mem.source = !d.a.mem.source
	case keys.MemoryCopy:
		return d.a.copyKey(d.key), false
	case keys.NavDown, keys.NavUp, keys.NavFirst, keys.NavLast, keys.NavHalfDown, keys.NavHalfUp, keys.NavPageDown, keys.NavPageUp:
		d.scroll = memScrolled(d.scroll, act, d.a.previewDialogPage())
	}
	return nil, false
}

func (a *App) previewDialogPage() int { return dialog.Page(a.cols, a.rows, 1<<20) }

func (*memPreviewDialog) Update(tea.Msg) tea.Cmd { return nil }

func (d *memPreviewDialog) Frame(l look.Look, cols, rows int) dialog.Frame {
	a := d.a
	iw := cols - 4
	if !dialog.FullScreen(cols, rows) {
		iw = min(dialog.MaxWidth, cols-4) - 4
	}
	mem, ok := a.mem.get(d.key)
	if !ok {
		return dialog.Frame{Title: d.key, Hints: a.hintsFor(keys.MemoryPreview), Body: []string{l.Paint(theme.Warning, "This memory was forgotten.")}}
	}
	body := a.memBody(mem, iw)
	page := dialog.Page(cols, rows, len(body))
	d.scroll = min(d.scroll, max(len(body)-page, 0))
	lines, chars := model.MemorySize(mem.Content)
	return dialog.Frame{
		Title: mem.Key, Aside: fmt.Sprintf("%d %s · %d chars", lines, plural(lines, "line", "lines"), chars),
		Hints: a.hintsFor(keys.MemoryPreview), Body: body, Scroll: d.scroll,
	}
}

// copyKey hands key to the copy hook.
func (a *App) copyKey(key string) tea.Cmd {
	if a.o.CopyKey == nil {
		a.hint = "copying is not available yet"
		return nil
	}
	return a.o.CopyKey(key)
}

// openForget asks before every marked memory, else the current one, is
// forgotten. Marks hidden by the search count: the confirmation says how many.
func (a *App) openForget() tea.Cmd {
	targets := make([]string, 0, len(a.mem.marks))
	for k := range a.mem.marks {
		targets = append(targets, k)
	}
	slices.Sort(targets)
	if len(targets) == 0 {
		mem, ok := a.mem.current()
		if !ok {
			a.hint = "no memory to forget"
			return nil
		}
		targets = []string{mem.Key}
	}
	a.confirmForget(targets)
	return nil
}

func (a *App) confirmForget(targets []string) {
	title := "Forget memory?"
	var lines []string
	if len(targets) == 1 {
		lines = []string{targets[0]}
		if mem, ok := a.mem.get(targets[0]); ok {
			if first := model.FirstLine(mem.Content); first != "" {
				lines = append(lines, "  "+first)
			}
		}
	} else {
		title = fmt.Sprintf("Forget %d memories?", len(targets))
		lines = append(lines, targets[:min(len(targets), 8)]...)
		if extra := len(targets) - 8; extra > 0 {
			lines = append(lines, fmt.Sprintf("and %d more", extra))
		}
		shown := memKeys(a.mem.shown())
		hidden := 0
		for _, k := range targets {
			if !slices.Contains(shown, k) {
				hidden++
			}
		}
		if hidden > 0 {
			lines = append(lines, "", fmt.Sprintf("%d of them are hidden by the search.", hidden))
		}
	}
	a.pushDialog(a.newConfirm(confirmOpts{
		Title: title, Lines: lines,
		Yes: func(a *App) tea.Cmd { return a.forgetKeys(targets) },
	}))
}

// forgetOutcome is what forgetKeys did: the keys it forgot, the ones that
// were gone already, and the key that failed, if any.
type forgetOutcome struct {
	forgot, already []string
	failed          string
}

// forgetKeys forgets the keys one by one and stops at the first failure. A
// key that is gone already counts as done.
func (a *App) forgetKeys(targets []string) tea.Cmd {
	out := &forgetOutcome{}
	return a.write(writeOp{
		run: func(ctx context.Context, c bd.Client) (string, error) {
			for _, k := range targets {
				err := c.Forget(ctx, k)
				switch {
				case err == nil:
					out.forgot = append(out.forgot, k)
				case errors.Is(err, bd.ErrMemoryGone):
					out.already = append(out.already, k)
				default:
					out.failed = k
					return "", err
				}
			}
			return "", nil
		},
		done: func(a *App, res writeResult) tea.Cmd {
			m := a.mem
			for _, k := range out.forgot {
				m.ownGone[k] = true
			}
			a.memRefresh()
			if res.err != nil {
				a.warn(fmt.Sprintf("forgot %d of %d, stopped at %s: %s",
					len(out.forgot)+len(out.already), len(targets), out.failed, writeSummary(res.err, 1)))
				return nil
			}
			for _, k := range targets {
				delete(m.marks, k)
			}
			switch {
			case len(targets) == 1 && len(out.already) == 1:
				a.toast(targets[0] + " was gone already")
			case len(targets) == 1:
				a.toast("forgot " + targets[0])
			case len(out.already) > 0:
				a.toast(fmt.Sprintf("forgot %d memories, %d gone already", len(out.forgot), len(out.already)))
			default:
				a.toast(fmt.Sprintf("forgot %d memories", len(targets)))
			}
			return nil
		},
	})
}
