package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/proc"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/form"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// formHooks is what a form dialog leaves to the dialog that embeds it.
type formHooks interface {
	// dirty reports unsaved edits; closing asks first.
	dirty() bool
	// submit validates and starts the write with start; nil when a field is
	// invalid.
	submit() tea.Cmd
	// finish reads the outcome of the write and reports whether the dialog
	// closes. A dialog that stays open shows the error with fail.
	finish(res writeResult) bool
	// pick opens the picker for a Links field.
	pick(f *form.Field) tea.Cmd
	// edited runs after the value of f changed.
	edited(f *form.Field)
	// extra handles the actions the base does not know.
	extra(act keys.Action) tea.Cmd
	// banner is shown above the fields.
	banner(l look.Look, w int) []string
	// aside is appended to the field counter in the frame's title row.
	aside() string
	// conflicted reports whether the conflict keys apply.
	conflicted() bool
}

type spinMsg struct {
	owner Dialog
}

const spinEvery = 100 * time.Millisecond

// formDialog is the frame, key handling, write lock and error row that the
// form dialogs share. The dialog that embeds it supplies formHooks.
type formDialog struct {
	a     *App
	h     formHooks
	self  Dialog
	title string
	f     *form.Form

	busy bool
	spin int
	// errShort is the error row; errFull the whole text, shown while errOpen.
	errShort string
	errFull  []string
	errOpen  bool
	scroll   int
}

func (d *formDialog) Context() keys.Context { return keys.Form }

// Dirty reports unsaved edits.
func (d *formDialog) Dirty() bool { return d.h.dirty() }

func (d *formDialog) Type(m tea.KeyPressMsg) tea.Cmd {
	if d.busy {
		return nil
	}
	ev, cmd := d.f.Key(m)
	switch ev {
	case form.Submit:
		return d.submit()
	case form.Pick:
		return d.h.pick(d.f.Focused())
	case form.Edited:
		d.touched()
	case form.Nothing, form.Moved:
	}
	return cmd
}

func (d *formDialog) Paste(s string) tea.Cmd {
	if d.busy {
		return nil
	}
	ev, cmd := d.f.Paste(s)
	if ev == form.Edited {
		d.touched()
	}
	return cmd
}

func (d *formDialog) touched() {
	f := d.f.Focused()
	f.Err = ""
	d.h.edited(f)
}

func (d *formDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	if d.busy {
		return nil, false
	}
	switch act { //nolint:exhaustive // only the actions of the form context arrive
	case keys.Close:
		if d.h.dirty() {
			d.a.pushDialog(d.a.newConfirm(confirmOpts{
				Title: "Discard changes?",
				Lines: []string{"Your edits have not been saved."},
				Yes:   func(a *App) tea.Cmd { a.dropDialog(d.self); return nil },
			}))
			return nil, false
		}
		return nil, true
	case keys.Next:
		d.f.Move(1)
	case keys.Prev:
		d.f.Move(-1)
	case keys.Apply:
		return d.submit(), false
	case keys.Editor:
		return d.editor(), false
	case keys.FormError:
		d.errOpen = !d.errOpen && len(d.errFull) > 0
	case keys.KeepMine, keys.TakeTheirs, keys.Reload:
		if d.h.conflicted() {
			return d.h.extra(act), false
		}
	default:
		return d.h.extra(act), false
	}
	return nil, false
}

func (d *formDialog) editor() tea.Cmd {
	cur := d.f.Focused()
	if cur.Kind != form.Area {
		if d.f.Focused().Kind == form.Text || cur.Kind == form.Tokens {
			d.f.Key(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})
		}
		return nil
	}
	cmd, ok := form.Edit(d.a.o.Getenv, d.self, cur.Key, cur.Value())
	if !ok {
		d.a.hint = "no $VISUAL or $EDITOR set"
		return nil
	}
	return cmd
}

func (d *formDialog) Update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case form.EditorDone:
		if m.Owner != d.self {
			return nil
		}
		if m.Err != nil {
			d.fail(m.Err)
			return nil
		}
		if f := d.f.Field(m.Key); f != nil {
			f.Set(m.Text)
			f.Err = ""
			d.h.edited(f)
		}
	case spinMsg:
		if m.owner == d.self && d.busy {
			d.spin++
			return d.tick()
		}
	}
	return nil
}

func (d *formDialog) tick() tea.Cmd {
	return tea.Tick(spinEvery, func(time.Time) tea.Msg { return spinMsg{owner: d.self} })
}

func (d *formDialog) submit() tea.Cmd {
	if d.busy {
		return nil
	}
	d.errShort, d.errFull, d.errOpen = "", nil, false
	return d.h.submit()
}

// start locks the form and sends op; the outcome goes to finish.
func (d *formDialog) start(op writeOp) tea.Cmd {
	d.busy, d.spin = true, 0
	op.done = func(a *App, res writeResult) tea.Cmd {
		d.busy = false
		if d.h.finish(res) {
			a.dropDialog(d.self)
		}
		return nil
	}
	return tea.Batch(d.a.write(op), d.tick())
}

// fail shows err in the error row; the values stay.
func (d *formDialog) fail(err error) {
	d.errShort = writeSummary(err, 1)
	d.errFull = writeDetail(err)
	d.errOpen = false
}

var (
	spinFancy = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinASCII = []string{"|", "/", "-", `\`}
)

func (d *formDialog) spinner(l look.Look) string {
	frames := spinFancy
	if l.Glyphs.Tier == theme.TierASCII {
		frames = spinASCII
	}
	return frames[d.spin%len(frames)]
}

// layout sizes the form's multi-line fields for the screen.
func (d *formDialog) layout(l look.Look, cols, rows int) {
	if cols > 0 {
		d.f.Layout(l, d.innerWidth(cols, rows))
	}
}

func (d *formDialog) innerWidth(cols, rows int) int {
	if dialog.FullScreen(cols, rows) {
		return cols - 4
	}
	return min(dialog.MaxWidth, cols-4) - 4
}

func (d *formDialog) Frame(l look.Look, cols, rows int) dialog.Frame {
	iw := d.innerWidth(cols, rows)
	content := d.h.banner(l, iw)
	fields, from, to := d.f.View(l, iw)
	off := len(content)
	content = append(content, fields...)
	foot := d.errorRows(l, iw)
	avail := max(dialog.Page(cols, rows, 1<<20)-len(foot), 1)
	body := content
	if len(content) > avail {
		d.scroll = form.Scroll(d.scroll, from+off, to+off, avail, len(content))
		if d.scroll < len(content)-avail {
			body = append([]string(nil), content[d.scroll:d.scroll+avail-1]...)
			body = append(body, l.Paint(theme.Faint, fmt.Sprintf("%s %d more", l.Glyphs.Ellipsis, len(content)-d.scroll-avail+1)))
		} else {
			body = content[d.scroll:]
		}
	} else {
		d.scroll = 0
	}
	return dialog.Frame{
		Title: d.title, Aside: d.aside(l), Hints: d.hints(), Body: append(body, foot...),
	}
}

func (d *formDialog) aside(l look.Look) string {
	at, of := d.f.Index()
	parts := []string{fmt.Sprintf("field %d/%d", at, of)}
	if s := d.h.aside(); s != "" {
		parts = append(parts, s)
	}
	text := strings.Join(parts, " · ")
	if d.busy {
		return d.spinner(l) + " writing · " + text
	}
	return text
}

func (d *formDialog) hints() []keys.Hint {
	var out []keys.Hint
	for _, h := range d.a.hintsFor(keys.Form) {
		switch h.Key {
		case "Ctrl+E":
			if d.f.Focused().Kind != form.Area || proc.EditorArgv(d.a.o.Getenv) == nil {
				continue
			}
		case "Ctrl+G":
			if len(d.errFull) == 0 {
				continue
			}
		case "Ctrl+O", "Ctrl+T", "Ctrl+R":
			if !d.h.conflicted() {
				continue
			}
		}
		out = append(out, h)
	}
	return out
}

const errRowsMax = 8

func (d *formDialog) errorRows(l look.Look, w int) []string {
	if d.errShort == "" {
		return nil
	}
	if d.errOpen {
		var rows []string
		for i, line := range d.errFull {
			for _, part := range wrapText(line, w-2) {
				if i == 0 && len(rows) == 0 {
					part = "! " + part
				} else {
					part = "  " + part
				}
				rows = append(rows, l.Paint(theme.Error, part))
			}
		}
		foot := "Ctrl+G shortens"
		if extra := len(rows) - errRowsMax; extra > 0 {
			foot = fmt.Sprintf("+%d more · %s", extra, foot)
		}
		return append(rows[:min(len(rows), errRowsMax)], l.Paint(theme.Faint, foot))
	}
	text := "! " + d.errShort
	more := ""
	if len(d.errFull) > 1 || ansi.StringWidth(text) > w {
		more = "  Ctrl+G more"
	}
	room := max(w-ansi.StringWidth(more), 8)
	return []string{l.Paint(theme.Error, ansi.Truncate(text, room, l.Glyphs.Ellipsis)) + l.Paint(theme.Faint, more)}
}

// wrapText breaks s into lines of at most w cells at spaces.
func wrapText(s string, w int) []string {
	if w < 8 {
		return []string{s}
	}
	var out []string
	line := ""
	flush := func() {
		if line != "" {
			out = append(out, line)
		}
		line = ""
	}
	for _, word := range strings.Fields(s) {
		for ansi.StringWidth(word) > w {
			flush()
			head := ansi.Truncate(word, w, "")
			out = append(out, head)
			word = strings.TrimPrefix(word, head)
		}
		switch {
		case line == "":
			line = word
		case ansi.StringWidth(line)+1+ansi.StringWidth(word) <= w:
			line += " " + word
		default:
			flush()
			line = word
		}
	}
	flush()
	if len(out) == 0 {
		return []string{""}
	}
	return out
}
