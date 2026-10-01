package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/ui/detail"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/state"
)

// frame is the panel as drawn now: beside the view or over it while it is
// shown, otherwise nothing; nothing shows without issues.
func (a *App) frame() detail.Dock {
	if !a.shown || a.snap == nil || a.snap.Len() == 0 || a.inMemories() {
		return detail.Dock{}
	}
	_, board := a.view().(*Kanban)
	return detail.Place(a.cols, a.bodyHeight(), board)
}

// listSize is the room the view gets next to the panel.
func (a *App) listSize() (w, h int) {
	if d := a.frame(); d.Frame == detail.Side {
		return a.cols - d.W, a.bodyHeight()
	}
	return a.cols, a.bodyHeight()
}

// syncLayers keeps the layer stack in line with where the panel sits: a side
// panel is entered through detail focus, an overlay is the detail layer and
// lies under any bar or dialog opened over it.
func (a *App) syncLayers() {
	if !a.shown && a.sess.Has(state.LayerDetail) {
		a.shown = true
	}
	switch a.frame().Frame {
	case detail.Side:
		a.sess.Replace(state.LayerDetail, state.LayerDetailFocus)
	case detail.Overlay:
		if !a.sess.Replace(state.LayerDetailFocus, state.LayerDetail) && !a.sess.Has(state.LayerDetail) {
			a.sess.PushUnder(state.LayerDetail)
		}
	case detail.Hidden:
		a.sess.Remove(state.LayerDetailFocus)
		a.sess.Remove(state.LayerDetail)
	}
}

// toggleDetail shows or hides the panel for the current issue.
func (a *App) toggleDetail() {
	if !a.shown && (a.sess.Current() == "" || a.snap == nil) {
		return
	}
	a.shown = !a.shown
	if !a.shown {
		a.sess.Remove(state.LayerDetailFocus)
		a.sess.Remove(state.LayerDetail)
	}
}

// focusNext moves the keys between the list and a side panel.
func (a *App) focusNext() {
	if a.frame().Frame != detail.Side || a.sess.Current() == "" {
		return
	}
	if a.sess.Has(state.LayerDetailFocus) {
		a.sess.Remove(state.LayerDetailFocus)
		return
	}
	a.sess.Push(state.LayerDetailFocus)
}

func (a *App) panelAct(act keys.Action) {
	p := a.panel
	switch act { //nolint:exhaustive // the panel binds only these
	case keys.NavDown:
		if !p.Move(1) {
			p.ScrollBy(1)
		}
	case keys.NavUp:
		if !p.Move(-1) {
			p.ScrollBy(-1)
		}
	case keys.NavHalfDown:
		p.ScrollBy(max(p.Page()/2, 1))
	case keys.NavHalfUp:
		p.ScrollBy(-max(p.Page()/2, 1))
	case keys.NavPageDown:
		p.ScrollBy(p.Page())
	case keys.NavPageUp:
		p.ScrollBy(-p.Page())
	case keys.NavFirst:
		p.ScrollTo(0)
	case keys.NavLast:
		p.End()
	case keys.SectionNext:
		p.Next()
	case keys.SectionPrev:
		p.Prev()
	case keys.Left:
		p.Collapse()
	case keys.Right:
		p.Expand()
	case keys.Jump:
		if id, ok := p.Row(); ok {
			a.sess.Jump(id)
			return
		}
		p.Enter()
	case keys.SectionsAll:
		p.ToggleAll()
	case keys.Markdown:
		p.ToggleSource()
	case keys.DepthMore:
		p.DepthMore()
	case keys.DepthLess:
		p.DepthLess()
	}
}

func (a *App) panelInput(d detail.Dock) detail.Input {
	cur := a.sess.Current()
	now := a.now()
	in := detail.Input{
		Look: a.look, Gen: a.lookGen, Snap: a.snap, Statuses: a.bds.Statuses, Rows: a.rend,
		ID: cur, Now: now, Frame: d.Frame, W: d.W, H: d.H,
		Focused: d.Frame == detail.Overlay || a.sess.Has(state.LayerDetailFocus),
	}
	if a.hl.Live(cur, now) {
		in.Events = a.hl.Events(cur)
	}
	return in
}

func (a *App) panelLines(d detail.Dock) []string {
	return a.panel.Render(a.panelInput(d))
}

// mdMsg carries a finished markdown rendering to the panel.
type mdMsg struct{ res detail.Result }

// renderJobs starts the markdown renderings the visible panel still needs off
// the update loop.
func (a *App) renderJobs() tea.Cmd {
	d := a.frame()
	if d.Frame == detail.Hidden {
		for _, f := range a.panel.CancelAudit() {
			a.endFetch(f.Seq)
		}
		return nil
	}
	in := a.panelInput(d)
	jobs := a.panel.Plan(in)
	if a.syncMD {
		for _, j := range jobs {
			a.panel.Apply(j.Run())
		}
		return a.auditJobs(in)
	}
	cmds := make([]tea.Cmd, 0, len(jobs)+1)
	for _, j := range jobs {
		cmds = append(cmds, func() tea.Msg { return mdMsg{j.Run()} })
	}
	cmds = append(cmds, a.auditJobs(in))
	return tea.Batch(cmds...)
}

// overPanel reports whether body column x is inside the panel.
func (a *App) overPanel(x int) bool {
	switch d := a.frame(); d.Frame {
	case detail.Overlay:
		return true
	case detail.Side:
		return x >= a.cols-d.W
	case detail.Hidden:
	}
	return false
}

func (a *App) clickBody(x, y int) {
	if a.inMemories() {
		a.memClick(x, y)
		return
	}
	if a.overPanel(x) {
		if a.frame().Frame != detail.Overlay && !a.sess.Has(state.LayerDetailFocus) && a.sess.Current() != "" {
			a.sess.Push(state.LayerDetailFocus)
		}
		return
	}
	if a.sess.Has(state.LayerDetailFocus) {
		a.sess.Remove(state.LayerDetailFocus)
	}
	var id string
	var ok bool
	if p, isPointer := a.view().(Pointer); isPointer {
		id, ok = p.AtXY(x, y)
	} else {
		id, ok = a.view().At(y)
	}
	if ok {
		a.sess.SetCurrent(id)
	}
}

// viewFocusNext offers Tab to the view while the list has the keys; it
// reports whether the view used it.
func (a *App) viewFocusNext() bool {
	if a.baseContext() != a.view().Context() {
		return false
	}
	_, ok := a.view().Handle(keys.FocusNext, a.env())
	return ok
}
