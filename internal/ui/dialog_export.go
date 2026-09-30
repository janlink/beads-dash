package ui

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/export"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/form"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

const (
	fExpSet      = "set"
	fExpFormat   = "format"
	fExpComments = "comments"
	fExpTarget   = "target"
	fExpPath     = "path"

	targetClipboard = "Clipboard"
	targetFile      = "File"

	// commentFetchCost is the time one comment read is assumed to take.
	commentFetchCost = 500 * time.Millisecond
	previewRows      = 8
)

// commentAutoFetch is the most comment reads that start without a submit.
var commentAutoFetch = 20

// exportDialog collects what to export and where it goes. The document is
// built off the UI goroutine and rebuilt whenever a choice changes.
type exportDialog struct {
	formDialog
	sets  exportSets
	order []export.Set
	// gen numbers the builds; results of older ones are dropped.
	gen   int
	stale bool
	built []byte
	// preview holds the first lines of built, cut once per build.
	preview []string
	// builtGen is the gen the document in built belongs to.
	builtGen int
	building bool
	buildErr error
	// gone counts the issues that left the snapshot since the dialog opened.
	gone int

	commentsTouched, pathTouched bool
	missing                      []string
	fetched, fetchTotal          int
	fetchCancel                  context.CancelFunc
	fetchErr                     string
	submitWanted                 bool
	capNote                      string
}

func (a *App) newExportDialog(sets exportSets) *exportDialog {
	d := &exportDialog{sets: sets, stale: true}
	var labels []string
	for _, s := range []export.Set{export.Current, export.Marked, export.Scope} {
		d.order = append(d.order, s)
		labels = append(labels, d.label(s))
	}
	def := sets.defaultSet()
	set := form.NewChoice(fExpSet, "Set", labels, d.label(def))
	set.Faint = func(v string) bool {
		i := slices.Index(labels, v)
		return i >= 0 && len(d.sets[d.order[i]].ids) == 0
	}
	format := form.NewChoice(fExpFormat, "Format", []string{"Markdown", "JSON", "Text"}, "Markdown")
	comments := form.NewCheck(fExpComments, "Comments", def == export.Current)
	target := form.NewChoice(fExpTarget, "Target", []string{targetClipboard, targetFile}, targetClipboard)
	path := form.NewText(fExpPath, "Path", "")
	path.Skip = true
	d.f = form.New(set, format, comments, target, path)
	d.formDialog = formDialog{a: a, h: d, self: d, title: "Export", f: d.f}
	d.refreshPath()
	return d
}

func (d *exportDialog) label(s export.Set) string {
	set := d.sets[s]
	switch s {
	case export.Current:
		if len(set.ids) == 0 {
			return "Current (none)"
		}
		return "Current"
	case export.Marked:
		if set.hidden > 0 {
			return fmt.Sprintf("Marked (%d, %d hidden)", len(set.ids), set.hidden)
		}
		return fmt.Sprintf("Marked (%d)", len(set.ids))
	case export.Scope:
	}
	return fmt.Sprintf("Scope (%d)", len(set.ids))
}

func (d *exportDialog) set() export.Set {
	f := d.f.Field(fExpSet)
	if i := slices.Index(f.Options, f.Value()); i >= 0 {
		return d.order[i]
	}
	return d.order[0]
}

func (d *exportDialog) format() export.Format {
	f, _ := export.ParseFormat(strings.ToLower(d.f.Field(fExpFormat).Value()))
	return f
}

func (d *exportDialog) withComments() bool { return d.f.Field(fExpComments).On() }

func (d *exportDialog) toFile() bool { return d.f.Field(fExpTarget).Value() == targetFile }

// ids are the issues of the chosen set that the snapshot still holds.
func (d *exportDialog) ids() []string {
	all := d.sets[d.set()].ids
	if d.a.snap == nil {
		return nil
	}
	return slices.DeleteFunc(slices.Clone(all), func(id string) bool {
		_, ok := d.a.snap.Issue(id)
		return !ok
	})
}

func (d *exportDialog) refreshPath() {
	if d.pathTouched {
		return
	}
	ids := d.ids()
	name := export.Name(d.format(), d.set(), ids, d.a.bds.Workspace.Prefix, d.a.now())
	f := d.f.Field(fExpPath)
	f.Set(d.a.exportPath(name))
	f.Orig = f.Value()
}

func (d *exportDialog) dirty() bool { return false }

func (d *exportDialog) finish(writeResult) bool { return false }

func (d *exportDialog) conflicted() bool { return false }

func (d *exportDialog) pick(*form.Field) tea.Cmd { return nil }

func (d *exportDialog) extra(keys.Action) tea.Cmd { return nil }

func (d *exportDialog) edited(f *form.Field) {
	switch f.Key {
	case fExpSet:
		if !d.commentsTouched {
			d.f.Field(fExpComments).Set(map[bool]string{true: "on", false: "off"}[d.set() == export.Current])
		}
		d.refreshPath()
		d.stale = true
	case fExpFormat:
		d.refreshPath()
		d.stale = true
	case fExpComments:
		d.commentsTouched = true
		d.stale = true
	case fExpTarget:
		d.syncTarget()
	case fExpPath:
		d.pathTouched = true
	}
}

// syncTarget shows the path only for a file and keeps an over-size document
// off the clipboard. It reports whether it moved the target to File.
func (d *exportDialog) syncTarget() bool {
	flipped := d.capNote != "" && !d.toFile()
	if flipped {
		d.f.Field(fExpTarget).Set(targetFile)
	}
	d.f.Field(fExpPath).Skip = !d.toFile()
	return flipped
}

// Snapshot rebuilds from the new data on the next flush.
func (d *exportDialog) Snapshot() {
	d.stale = true
}

func (d *exportDialog) RawKeys() bool { return false }

// flush starts whatever the last change needs: comment reads, then the build.
func (d *exportDialog) flush() tea.Cmd {
	if !d.stale {
		return nil
	}
	d.stale = false
	d.gen++
	d.cancel()
	d.built, d.preview, d.buildErr, d.fetchErr = nil, nil, nil, ""
	d.gone = len(d.sets[d.set()].ids) - len(d.ids())
	d.missing = nil
	if d.withComments() {
		for _, id := range d.ids() {
			is, _ := d.a.snap.Issue(id)
			if is.CommentCount == 0 {
				continue
			}
			if _, ok := d.a.freshComments(id); !ok {
				d.missing = append(d.missing, id)
			}
		}
	}
	d.fetched, d.fetchTotal = 0, len(d.missing)
	if len(d.missing) > commentAutoFetch && !d.submitWanted {
		return nil
	}
	return d.next()
}

// next reads the following comment thread, or builds once all are in.
func (d *exportDialog) next() tea.Cmd {
	if d.fetched < d.fetchTotal {
		ctx, cancel := context.WithCancel(context.Background())
		d.fetchCancel = cancel
		id, gen, a, eng := d.missing[d.fetched], d.gen, d.a, d.a.eng
		is, _ := a.snap.Issue(id)
		updated, count := is.UpdatedAt.UnixNano(), is.CommentCount
		return func() tea.Msg {
			list, err := fetchComments(ctx, eng, id)
			return exportFetchedMsg{d: d, gen: gen, id: id, updated: updated, count: count, list: list, err: err}
		}
	}
	return d.build()
}

func (d *exportDialog) build() tea.Cmd {
	d.building = true
	a := d.a
	in := a.exportInput(d.set(), d.ids(), d.withComments())
	if in.WithComments {
		in.Comments = map[string][]bd.Comment{}
		for _, id := range in.IDs {
			if list, ok := a.freshComments(id); ok {
				in.Comments[id] = list
			}
		}
	}
	f, gen := d.format(), d.gen
	return func() tea.Msg {
		out, err := export.Render(f, in)
		return exportBuiltMsg{d: d, gen: gen, out: out, err: err}
	}
}

func (d *exportDialog) cancel() {
	if d.fetchCancel != nil {
		d.fetchCancel()
		d.fetchCancel = nil
	}
}

type (
	exportFetchedMsg struct {
		d       *exportDialog
		gen     int
		id      string
		updated int64
		count   int
		list    []bd.Comment
		err     error
	}
	exportBuiltMsg struct {
		d   *exportDialog
		gen int
		out []byte
		err error
	}
)

func (m exportFetchedMsg) apply(a *App) tea.Cmd {
	d := m.d
	if !a.hasDialog(d) || m.gen != d.gen {
		return nil
	}
	if m.err != nil {
		d.fetchErr = fmt.Sprintf("comments for %s failed: %s", m.id, shortLine(m.err.Error()))
		d.submitWanted, d.busy = false, false
		return nil
	}
	a.storeComments(m.id, m.updated, m.count, m.list)
	d.fetched++
	return d.next()
}

func (m exportBuiltMsg) apply(a *App) tea.Cmd {
	d := m.d
	if !a.hasDialog(d) || m.gen != d.gen {
		return nil
	}
	d.building = false
	d.built, d.buildErr, d.builtGen = m.out, m.err, m.gen
	if m.err != nil {
		d.submitWanted, d.busy = false, false
		d.fail(m.err)
		return nil
	}
	d.capNote = export.OverCap(len(m.out))
	d.preview = previewOf(m.out)
	if d.syncTarget() && d.submitWanted {
		d.submitWanted, d.busy = false, false
		a.hint = "too large for the clipboard: pick a path and export again"
		return nil
	}
	if d.submitWanted {
		d.submitWanted = false
		return d.deliver()
	}
	return nil
}

// skipComments drops the comments after a failed read and rebuilds.
func (d *exportDialog) skipComments() tea.Cmd {
	d.f.Field(fExpComments).Set("off")
	d.commentsTouched = true
	d.stale = true
	return d.flush()
}

func (d *exportDialog) ready() bool {
	return !d.stale && !d.building && d.built != nil && d.builtGen == d.gen
}

func (d *exportDialog) submit() tea.Cmd {
	if d.fetchErr != "" {
		return d.skipComments()
	}
	if len(d.ids()) == 0 {
		d.f.Field(fExpSet).Err = "No issues in this set"
		return nil
	}
	if d.toFile() {
		f := d.f.Field(fExpPath)
		if strings.TrimSpace(f.Value()) == "" {
			f.Err = "Path is required"
			return nil
		}
	}
	d.busy, d.spin = true, 0
	if d.ready() {
		return tea.Batch(d.deliver(), d.tick())
	}
	d.submitWanted = true
	cmd := d.flush()
	if cmd == nil && len(d.missing) > 0 && d.fetched == 0 && !d.building {
		cmd = d.next()
	}
	return tea.Batch(cmd, d.tick())
}

func (d *exportDialog) deliver() tea.Cmd {
	a := d.a
	ids := d.ids()
	what := fmt.Sprintf("%s as %s", issueWord(len(ids)), d.format())
	if len(ids) == 1 {
		what = fmt.Sprintf("%s as %s", ids[0], d.format())
	}
	if !d.toFile() {
		d.busy = false
		if err := clipboard.Check(string(d.built)); err != nil {
			d.fail(err)
			return nil
		}
		cmd := a.copy(fmt.Sprintf("%s (%s)", what, export.Size(len(d.built))), string(d.built))
		a.dropDialog(d.self)
		return cmd
	}
	path := export.ResolvePath(d.f.Field(fExpPath).Value(), a.o.StartDir, a.home())
	return exportFile(exportJob{data: d.built, what: what, path: path, owner: d}, modeCheck)
}

func (d *exportDialog) banner(l look.Look, w int) []string {
	var out []string
	if d.gone > 0 {
		out = append(out, l.Paint(theme.Warning, l.Fit(fmt.Sprintf("! %s no longer exists", issueWord(d.gone)), w)))
	}
	if d.capNote != "" {
		out = append(out, l.Paint(theme.Warning, l.Fit("! "+d.capNote, w)))
	}
	if d.fetchErr != "" {
		out = append(out, l.Paint(theme.Error, l.Fit("! "+d.fetchErr+", export without?", w)))
	}
	if len(out) > 0 {
		out = append(out, "")
	}
	return out
}

func (d *exportDialog) aside() string {
	a := d.a
	if a.status.Stale || a.status.Err != nil {
		return "from snapshot " + ageText(a.now().Sub(a.status.LastSuccess)) + " old"
	}
	return ""
}

// status is the line under the fields.
func (d *exportDialog) status() string {
	n := len(d.ids())
	head := fmt.Sprintf("%s · %s", issueWord(n), d.format())
	switch {
	case d.fetchErr != "":
		return head
	case d.fetched < d.fetchTotal && (d.fetchCancel != nil || d.submitWanted):
		left := time.Duration(d.fetchTotal-d.fetched) * commentFetchCost
		return fmt.Sprintf("%s · comments %d/%d · ~%d s left", head, d.fetched, d.fetchTotal, int((left+time.Second-1)/time.Second))
	case len(d.missing) > commentAutoFetch && !d.submitWanted && d.fetched == 0:
		est := time.Duration(len(d.missing)) * commentFetchCost
		return fmt.Sprintf("%s · %d comment reads on export, ~%d s", head, len(d.missing), int(est/time.Second))
	case d.ready():
		return fmt.Sprintf("%s · %s", head, export.Size(len(d.built)))
	}
	return head + " · building"
}

func (d *exportDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	cmd, closed := d.formDialog.Handle(act)
	if closed {
		d.cancel()
		return cmd, true
	}
	return tea.Batch(cmd, d.flush()), false
}

func (d *exportDialog) Type(m tea.KeyPressMsg) tea.Cmd {
	cmd := d.formDialog.Type(m)
	return tea.Batch(cmd, d.flush())
}

func (d *exportDialog) Paste(s string) tea.Cmd {
	cmd := d.formDialog.Paste(s)
	return tea.Batch(cmd, d.flush())
}

func (d *exportDialog) Update(msg tea.Msg) tea.Cmd {
	return tea.Batch(d.formDialog.Update(msg), d.flush())
}

// fail shows err and ends the wait.
func (d *exportDialog) fail(err error) {
	d.busy = false
	d.formDialog.fail(err)
}

func (d *exportDialog) Frame(l look.Look, cols, rows int) dialog.Frame {
	fr := d.formDialog.Frame(l, cols, rows)
	iw := d.innerWidth(cols, rows)
	fr.Hints = renameSave(fr.Hints)
	body := fr.Body
	page := dialog.Page(cols, rows, 1<<20)
	body = append(body, l.Paint(theme.Dim, l.Fit("  "+d.status(), iw)))
	room := min(page-len(body), previewRows)
	if d.ready() && room > 0 {
		for _, line := range d.preview[:min(len(d.preview), room)] {
			body = append(body, l.Paint(theme.Faint, l.Fit("  "+line, iw)))
		}
	}
	fr.Body = body
	return fr
}

// exportOverwriteDialog asks before a file is replaced.
type exportOverwriteDialog struct {
	a   *App
	job exportJob
}

func (*exportOverwriteDialog) Context() keys.Context { return keys.Overwrite }

func (d *exportOverwriteDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	var mode exportFileMode
	switch act { //nolint:exhaustive // only the actions of the overwrite context arrive
	case keys.DoOverwrite:
		mode = modeOverwrite
	case keys.KeepBoth:
		mode = modeKeepBoth
	case keys.Close:
		return nil, true
	default:
		return nil, false
	}
	if o := d.job.owner; o != nil {
		o.busy = true
	}
	return exportFile(d.job, mode), true
}

func (*exportOverwriteDialog) Update(tea.Msg) tea.Cmd { return nil }

func (d *exportOverwriteDialog) Frame(l look.Look, cols, rows int) dialog.Frame {
	iw := cols - 4
	if !dialog.FullScreen(cols, rows) {
		iw = min(dialog.MaxWidth, cols-4) - 4
	}
	body := []string{
		l.Paint(theme.Text, l.Fit("File exists: "+d.job.path, iw)),
		"",
		l.Paint(theme.Dim, l.Fit("Overwrite replaces it. Keep both saves next to it under a numbered name.", iw)),
	}
	return dialog.Frame{Title: "Overwrite file?", Hints: d.a.hintsFor(keys.Overwrite), Body: body}
}

// previewOf cuts the first previewRows lines of a document, keeping their
// indentation.
func previewOf(doc []byte) []string {
	var out []string
	for len(out) < previewRows && len(doc) > 0 {
		line := doc
		if i := bytes.IndexByte(doc, '\n'); i >= 0 {
			line, doc = doc[:i], doc[i+1:]
		} else {
			doc = nil
		}
		out = append(out, strings.ReplaceAll(strings.TrimRight(string(line), "\r"), "\t", "  "))
	}
	return out
}

func renameSave(hs []keys.Hint) []keys.Hint {
	out := slices.Clone(hs)
	for i, h := range out {
		if h.Key == "Ctrl+S" {
			out[i].Desc = "export"
		}
	}
	return out
}

// hints are the footer hints under the dialog.
func (d *exportDialog) hints() []keys.Hint { return renameSave(d.formDialog.hints()) }
