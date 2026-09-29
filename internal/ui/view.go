package ui

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/state"
)

// ViewNames are the six view slots the number keys select, in order.
var ViewNames = [6]string{"Overview", "Tree", "Kanban", "Ready", "Memories", "Graph"}

// Actions is the part of the session a view may change.
type Actions interface {
	Current() string
	// SetCurrent moves the current issue as cursor movement does.
	SetCurrent(id string)
	// Jump moves the current issue and pushes the old one on the back stack.
	Jump(id string)
	// OpenLayer and CloseLayer manage the detail, detail focus and bar
	// layers; dialogs belong to the shell.
	OpenLayer(l state.Layer)
	CloseLayer(l state.Layer)
	HasLayer(l state.Layer) bool
	Marked(id string) bool
	ToggleMark(id string)
	MarkedIDs() []string
}

// Env is what a view reads to draw and gives it access to the session.
type Env struct {
	Snap     *model.Snapshot
	Statuses model.Statuses
	Look     look.Look
	Rows     *rows.Renderer
	Current  string
	Marked   func(id string) bool
	Changed  func(id string) bool
	Act      Actions
	// Scope and Matches are the active scope and its outcome on Snap;
	// Matches is nil while there is no snapshot.
	Scope   model.Scope
	Matches *model.Matches
	Now     time.Time
	// Cols is the width of the whole terminal, of which a view may get less.
	Cols int
}

// View is one of the numbered views.
type View interface {
	// Name keys the row cache; it is unique among the registered views.
	Name() string
	// Scope is the label the header shows beside the view name, in at most w
	// cells; empty when there is none or it does not fit.
	Scope(w int) string
	// Context is the key context of the view's base level.
	Context() keys.Context
	// Visible lists the issue IDs the view shows in cursor order.
	Visible(env Env) []string
	// Has reports whether the view shows id.
	Has(env Env, id string) bool
	// Handle acts on an action the shell does not own. It returns the command
	// to run and whether it acted.
	Handle(a keys.Action, env Env) (tea.Cmd, bool)
	// Render draws the body as exactly h lines of w cells.
	Render(env Env, w, h int) []string
	// Scroll moves the viewport by n lines without moving the current issue.
	Scroll(n int)
	// At is the issue drawn on body row y of the last Render.
	At(y int) (id string, ok bool)
}

// Noter is implemented by views that add a note to the footer chips.
type Noter interface {
	Note() string
}

// Syncer is implemented by views that keep state in line with the session
// after each update, such as revealing the current issue.
type Syncer interface {
	Sync(env Env)
}

// Updater is implemented by views that receive messages, such as lazy loads.
type Updater interface {
	Update(msg tea.Msg) tea.Cmd
}

// sessionActions gives views the session without the dialog layer.
type sessionActions struct{ s *state.Session }

func (a sessionActions) Current() string       { return a.s.Current() }
func (a sessionActions) SetCurrent(id string)  { a.s.SetCurrent(id) }
func (a sessionActions) Jump(id string)        { a.s.Jump(id) }
func (a sessionActions) Marked(id string) bool { return a.s.Marked(id) }
func (a sessionActions) ToggleMark(id string)  { a.s.ToggleMark(id) }
func (a sessionActions) MarkedIDs() []string   { return a.s.MarkedIDs() }

func (a sessionActions) OpenLayer(l state.Layer) {
	if l != state.LayerDialog && !a.s.Has(l) {
		a.s.Push(l)
	}
}

func (a sessionActions) CloseLayer(l state.Layer) {
	if l != state.LayerDialog {
		a.s.Remove(l)
	}
}

func (a sessionActions) HasLayer(l state.Layer) bool { return a.s.Has(l) }

// window keeps a list viewport stable across refreshes and resizes: the top
// row is remembered by ID, and the current issue is only pulled into view when
// it changes.
type window struct {
	anchor  string
	top     int
	last    string
	order   []string
	started bool
}

// layout returns the index of the first shown row for a viewport of h rows.
func (w *window) layout(order []string, current string, h int) int {
	w.order = order
	if i := slices.Index(order, w.anchor); w.anchor != "" && i >= 0 {
		w.top = i
	}
	if cur := slices.Index(order, current); cur >= 0 && (current != w.last || !w.started) {
		if cur < w.top {
			w.top = cur
		} else if cur >= w.top+h {
			w.top = cur - h + 1
		}
	}
	w.last, w.started = current, true
	w.top = min(max(w.top, 0), max(len(order)-h, 0))
	w.anchor = ""
	if w.top < len(order) {
		w.anchor = order[w.top]
	}
	return w.top
}

func (w *window) scroll(n int) {
	w.top = max(w.top+n, 0)
	if w.top < len(w.order) {
		w.anchor = w.order[w.top]
	}
}

func (w *window) at(y int) (string, bool) {
	i := w.top + y
	if y < 0 || i >= len(w.order) {
		return "", false
	}
	return w.order[i], true
}

// placeholder is the plain list of issues in snapshot order that fills the
// first slot when no view is registered.
type placeholder struct{ win window }

func (*placeholder) Name() string { return "list" }

func (*placeholder) Context() keys.Context { return keys.View }

func (*placeholder) Scope(int) string { return "" }

func (*placeholder) Visible(env Env) []string {
	if env.Snap == nil {
		return nil
	}
	return env.Snap.IDs()
}

func (*placeholder) Has(env Env, id string) bool {
	if env.Snap == nil {
		return false
	}
	_, ok := env.Snap.Issue(id)
	return ok
}

func (*placeholder) Handle(keys.Action, Env) (tea.Cmd, bool) { return nil, false }

func (p *placeholder) Render(env Env, w, h int) []string {
	order := p.Visible(env)
	first := p.win.layout(order, env.Current, h)
	out := make([]string, h)
	for i := range out {
		if first+i >= len(order) {
			out[i] = env.Look.Fit("", w)
			continue
		}
		id := order[first+i]
		out[i] = env.Rows.Line(w, rows.Row{ID: id, Current: id == env.Current, Marked: env.Marked(id), Changed: env.Changed(id)}, p.Name(), env.Rows.Standard(id))
	}
	return out
}

func (p *placeholder) Scroll(n int) { p.win.scroll(n) }

func (p *placeholder) At(y int) (string, bool) { return p.win.at(y) }
