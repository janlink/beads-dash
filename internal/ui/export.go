package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/export"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/ui/command"
)

// exportMsg is an export result that reaches the app from a command.
type exportMsg interface{ apply(a *App) tea.Cmd }

// exportSet is the issues one export set held when the dialog opened.
type exportSet struct {
	ids []string
	// hidden counts the marked issues the source view does not show.
	hidden int
}

// exportSets holds the three sets, indexed by export.Set.
type exportSets [3]exportSet

// exportOrder is the cursor order of the source view; Overview, Memories and
// Graph have no issue order of their own and use the outline order.
func (a *App) exportOrder() []string {
	switch a.slot {
	case overviewSlot, memSlot, graphSlot:
		return a.outlineOrder()
	}
	return a.view().Visible(a.env())
}

// outlineOrder lists the issues depth-first as the Tree shows them with
// every row unfolded.
func (a *App) outlineOrder() []string {
	var open model.Folds
	open.SetAll(a.snap, false)
	var ids []string
	for _, r := range model.BuildTree(a.snap, a.bds.Statuses, a.matches(), &open) {
		if r.Kind != model.TreeClosedFold {
			ids = append(ids, r.ID)
		}
	}
	return ids
}

// collectExportSets freezes the ID lists of the three sets in the order of
// the source view.
func (a *App) collectExportSets() exportSets {
	var sets exportSets
	if a.snap == nil {
		return sets
	}
	order := a.exportOrder()
	rank := make(map[string]int, len(order))
	for i, id := range order {
		rank[id] = i
	}
	sorted := func(ids []string) []string {
		out := slices.DeleteFunc(slices.Clone(ids), func(id string) bool {
			_, ok := a.snap.Issue(id)
			return !ok
		})
		slices.SortStableFunc(out, func(x, y string) int {
			rx, okx := rank[x]
			ry, oky := rank[y]
			switch {
			case okx && oky:
				return rx - ry
			case okx:
				return -1
			case oky:
				return 1
			}
			return strings.Compare(x, y)
		})
		return out
	}
	if cur := a.sess.Current(); cur != "" {
		sets[export.Current].ids = sorted([]string{cur})
	}
	marked := sorted(a.sess.MarkedIDs())
	sets[export.Marked].ids = marked
	for _, id := range marked {
		if !a.visible(id) {
			sets[export.Marked].hidden++
		}
	}
	if m := a.matches(); m != nil {
		sets[export.Scope].ids = sorted(m.IDs())
	}
	return sets
}

// defaultSet is Marked when any issue is marked, else Current; a
// workspace view with neither falls back to the Scope.
func (s exportSets) defaultSet() export.Set {
	switch {
	case len(s[export.Marked].ids) > 0:
		return export.Marked
	case len(s[export.Current].ids) > 0:
		return export.Current
	}
	return export.Scope
}

func (a *App) home() string {
	if h := a.o.Getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

// exportPath is the path prefilled in the dialog: the name under export.dir
// when that is set, else the bare name; both resolve against the start
// directory when the export is written.
func (a *App) exportPath(name string) string {
	if d := a.o.Settings.Settings.ExportDir; d != "" {
		return filepath.Join(d, name)
	}
	return name
}

func (a *App) exportInput(set export.Set, ids []string, withComments bool) export.Input {
	in := export.Input{
		Snap: a.snap, Statuses: a.bds.Statuses, IDs: slices.Clone(ids), Set: set, WithComments: withComments,
		Prefix: a.bds.Workspace.Prefix, Query: a.scope.Query(), Now: a.now(), Loc: a.now().Location(), Version: a.o.BdashVersion,
	}
	return in
}

func (a *App) openExport() tea.Cmd {
	if a.snap == nil {
		a.hint = "nothing loaded to export"
		return nil
	}
	sets := a.collectExportSets()
	if len(sets[export.Current].ids)+len(sets[export.Marked].ids)+len(sets[export.Scope].ids) == 0 {
		a.hint = "no issues to export"
		return nil
	}
	d := a.newExportDialog(sets)
	a.pushDialog(d)
	return d.flush()
}

// exportJob is a finished export on its way to a file.
type exportJob struct {
	data []byte
	what string
	path string
	// owner is the dialog that started the job; nil for the command bar.
	owner *exportDialog
}

type exportFileMode int

const (
	modeCheck exportFileMode = iota
	modeOverwrite
	modeKeepBoth
)

type (
	exportExistsMsg struct{ job exportJob }
	exportWroteMsg  struct {
		job  exportJob
		path string
		err  error
	}
	exportDirectMsg struct {
		f     export.Format
		out   []byte
		err   error
		path  string
		count int
		id    string
	}
)

// exportFile writes the job off the UI goroutine. The first try reports an
// existing target instead of replacing it.
func exportFile(j exportJob, mode exportFileMode) tea.Cmd {
	return func() tea.Msg {
		path := j.path
		switch mode {
		case modeCheck:
			taken, err := export.Exists(path)
			if err != nil {
				return exportWroteMsg{job: j, path: path, err: err}
			}
			if taken {
				return exportExistsMsg{j}
			}
		case modeKeepBoth:
			free, err := export.FreeName(path)
			if err != nil {
				return exportWroteMsg{job: j, path: path, err: err}
			}
			path = free
		case modeOverwrite:
		}
		return exportWroteMsg{job: j, path: path, err: export.WriteFile(path, j.data, mode == modeOverwrite)}
	}
}

func (a *App) hasDialog(d Dialog) bool { return slices.Contains(a.dialogs, d) }

func (m exportExistsMsg) apply(a *App) tea.Cmd {
	if o := m.job.owner; o != nil {
		if !a.hasDialog(o) {
			return nil
		}
		o.busy = false
	}
	a.pushDialog(&exportOverwriteDialog{a: a, job: m.job})
	return nil
}

func (m exportWroteMsg) apply(a *App) tea.Cmd {
	o := m.job.owner
	open := o != nil && a.hasDialog(o)
	if o != nil {
		o.busy = false
	}
	if m.err != nil {
		switch {
		case open:
			o.fail(m.err)
		default:
			a.warn("export failed: " + shortLine(m.err.Error()))
		}
		return nil
	}
	a.toast(fmt.Sprintf("Saved %s %s %s (%s)", m.job.what, a.look.Glyphs.Arrow, m.path, export.Size(len(m.job.data))))
	if open {
		a.dropDialog(o)
	}
	return nil
}

func (m exportDirectMsg) apply(a *App) tea.Cmd {
	if m.err != nil {
		a.warn("export failed: " + shortLine(m.err.Error()))
		return nil
	}
	what := fmt.Sprintf("%s as %s (%s)", m.describe(), m.f, export.Size(len(m.out)))
	if m.path == "" {
		if err := clipboard.Check(string(m.out)); err != nil {
			a.warn(fmt.Sprintf("%s: %s; give :export %s a path to save it", what, err, strings.ToLower(m.f.String())))
			return nil
		}
		return a.copy(what, string(m.out))
	}
	job := exportJob{data: m.out, what: fmt.Sprintf("%s as %s", m.describe(), m.f), path: m.path}
	return exportFile(job, modeCheck)
}

func (m exportDirectMsg) describe() string {
	if m.count == 1 {
		return m.id
	}
	return issueWord(m.count)
}

func (a *App) registerExport() {
	a.register(command.Spec{
		Name: "export", Usage: "<md|json|text> [path]", Summary: "export the marked or current issues", Min: 1, Max: 2,
		Help: "Exports the marked issues, else the current one, without the dialog and without comments. Without a path the text goes to the clipboard; with one, to a file, asking before an existing file is replaced.",
		Args: func(prev []string, prefix string) []string {
			if len(prev) > 0 {
				return nil
			}
			return filterPrefix([]string{"md", "json", "text"}, prefix)
		},
	}, (*App).cmdExport)
}

func (a *App) cmdExport(args []command.Word) (tea.Cmd, error) {
	f, ok := export.ParseFormat(args[0].Text)
	if !ok {
		return nil, argError(args[0], "want md, json or text")
	}
	if a.snap == nil {
		return nil, errors.New("nothing loaded to export")
	}
	sets := a.collectExportSets()
	set := sets.defaultSet()
	if set == export.Scope {
		return nil, errors.New("no marked or current issue to export; use x for the whole scope")
	}
	ids := sets[set].ids
	path := ""
	if len(args) > 1 {
		path = export.ResolvePath(args[1].Text, a.o.StartDir, a.home())
	}
	in := a.exportInput(set, ids, false)
	count := len(ids)
	first := ""
	if count > 0 {
		first = ids[0]
	}
	return func() tea.Msg {
		out, err := export.Render(f, in)
		return exportDirectMsg{f: f, out: out, err: err, path: path, count: count, id: first}
	}, nil
}

// commentEntry is an issue's comments as fetched, valid while the issue's
// updated_at and comment count stay the same.
type commentEntry struct {
	updated int64
	count   int
	list    []bd.Comment
}

func (a *App) freshComments(id string) ([]bd.Comment, bool) {
	is, ok := a.snap.Issue(id)
	if !ok {
		return nil, false
	}
	e, ok := a.expComments[id]
	if !ok || e.updated != is.UpdatedAt.UnixNano() || e.count != is.CommentCount {
		return nil, false
	}
	return e.list, true
}

func (a *App) storeComments(id string, updated int64, count int, list []bd.Comment) {
	if a.expComments == nil {
		a.expComments = map[string]commentEntry{}
	}
	a.expComments[id] = commentEntry{updated: updated, count: count, list: list}
}

// fetchComments reads one issue's comments in the engine's serialized queue.
// eng is read on the UI goroutine by the caller.
func fetchComments(ctx context.Context, eng Engine, id string) (list []bd.Comment, err error) {
	if eng == nil {
		return nil, errNoEngine
	}
	err = eng.Do(ctx, func(ctx context.Context, c bd.Client) error {
		var err error
		list, err = c.Comments(ctx, id)
		return err
	})
	return list, err
}
