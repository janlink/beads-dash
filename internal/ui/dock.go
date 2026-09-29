package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/ui/detail"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/state"
)

// dock is where the docked panel sits; nothing docks without issues.
func (a *App) dock() detail.Dock {
	if a.snap == nil || a.snap.Len() == 0 {
		return detail.Dock{}
	}
	return detail.Place(a.cols, a.bodyHeight(), a.docked)
}

// frame is the panel as drawn now: docked, an overlay while the detail layer
// is open, or nothing.
func (a *App) frame() detail.Dock {
	if d := a.dock(); d.Frame != detail.Hidden {
		return d
	}
	if a.snap != nil && a.snap.Len() > 0 && a.sess.Has(state.LayerDetail) {
		return detail.Dock{Frame: detail.Overlay, W: a.cols, H: a.bodyHeight()}
	}
	return detail.Dock{}
}

// listSize is the room the view gets next to the panel.
func (a *App) listSize() (w, h int) {
	body := a.bodyHeight()
	switch d := a.frame(); d.Frame {
	case detail.Side:
		return a.cols - d.W, body
	case detail.Bottom:
		return a.cols, body - d.H
	case detail.Hidden, detail.Overlay:
	}
	return a.cols, body
}

// syncLayers keeps the layer stack in line with where the panel sits: a
// docked panel is entered through detail focus, an overlay is the detail
// layer.
func (a *App) syncLayers() {
	if a.dock().Frame != detail.Hidden {
		a.sess.Replace(state.LayerDetail, state.LayerDetailFocus)
		return
	}
	a.sess.Replace(state.LayerDetailFocus, state.LayerDetail)
}

// openDetail moves into the current issue's detail; it reports whether
// there was one to open.
func (a *App) openDetail() bool {
	if a.sess.Current() == "" || a.snap == nil {
		return false
	}
	if a.dock().Frame != detail.Hidden {
		if !a.sess.Has(state.LayerDetailFocus) {
			a.sess.Push(state.LayerDetailFocus)
		}
		return true
	}
	if !a.sess.Has(state.LayerDetail) {
		a.sess.Push(state.LayerDetail)
	}
	return true
}

// focusNext moves the keys between the list and a docked panel.
func (a *App) focusNext() {
	if a.dock().Frame == detail.Hidden || a.sess.Current() == "" {
		return
	}
	if a.sess.Has(state.LayerDetailFocus) {
		a.sess.Remove(state.LayerDetailFocus)
		return
	}
	a.sess.Push(state.LayerDetailFocus)
}

// toggleDocked shows or hides the docked panel and remembers the choice.
func (a *App) toggleDocked() tea.Cmd {
	if a.dock().Frame != detail.Hidden {
		a.sess.Remove(state.LayerDetailFocus)
	}
	a.docked = !a.docked
	store := a.o.Store
	if store == nil {
		return nil
	}
	docked := a.docked
	return func() tea.Msg { return savedMsg{store.Set(config.KeyDetailDocked, docked)} }
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
		return nil
	}
	jobs := a.panel.Plan(a.panelInput(d))
	if len(jobs) == 0 {
		return nil
	}
	if a.syncMD {
		for _, j := range jobs {
			a.panel.Apply(j.Run())
		}
		return nil
	}
	cmds := make([]tea.Cmd, len(jobs))
	for i, j := range jobs {
		cmds[i] = func() tea.Msg { return mdMsg{j.Run()} }
	}
	return tea.Batch(cmds...)
}

// overPanel reports whether body cell (x, y) is inside the panel.
func (a *App) overPanel(x, y int) bool {
	switch d := a.frame(); d.Frame {
	case detail.Overlay:
		return true
	case detail.Side:
		return x >= a.cols-d.W
	case detail.Bottom:
		return y >= a.bodyHeight()-d.H
	case detail.Hidden:
	}
	return false
}

func (a *App) clickBody(x, y int) {
	if a.overPanel(x, y) {
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
