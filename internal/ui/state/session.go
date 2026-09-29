// Package state is the UI session state shared by all views: the current
// issue, marked issues, the back stack, the layer stack and the change
// highlights. Everything is keyed by issue ID, never by row, so it survives
// view switches, refreshes and resizes. It does no I/O.
package state

import "slices"

// Layer is one thing that can sit above the base view, in the order it can
// nest.
type Layer int

const (
	// LayerDetail is a detail panel (or overlay) showing the current issue.
	LayerDetail Layer = iota
	// LayerDetailFocus is keyboard focus inside the detail panel.
	LayerDetailFocus
	// LayerBar is a docked search, filter or command bar.
	LayerBar
	// LayerDialog is an overlay dialog.
	LayerDialog
)

// Esc is what one Esc press did.
type Esc int

const (
	// EscNothing means there was nothing left to close or clear.
	EscNothing Esc = iota
	// EscLayer means the top layer was closed.
	EscLayer
	// EscMarks means the marks were cleared.
	EscMarks
	// EscScope means the active search or filter should be cleared.
	EscScope
)

// Session is the ID-keyed state of one run.
type Session struct {
	current string
	marks   map[string]struct{}
	back    []string
	layers  []Layer
	// ScopeActive is set by views while a search or filter narrows the list.
	ScopeActive bool
}

// New returns an empty session.
func New() *Session { return &Session{marks: map[string]struct{}{}} }

// Current is the current issue's ID, empty when there is none.
func (s *Session) Current() string { return s.current }

// SetCurrent moves the current issue without touching the back stack, as
// cursor movement does.
func (s *Session) SetCurrent(id string) { s.current = id }

// Jump moves the current issue to id and remembers where it came from, so
// Back can return.
func (s *Session) Jump(id string) {
	if id == s.current {
		return
	}
	if s.current != "" && (len(s.back) == 0 || s.back[len(s.back)-1] != s.current) {
		s.back = append(s.back, s.current)
	}
	s.current = id
}

// Back returns to the most recent jump origin that alive still accepts, and
// reports whether there was one.
func (s *Session) Back(alive func(id string) bool) bool {
	for len(s.back) > 0 {
		id := s.back[len(s.back)-1]
		s.back = s.back[:len(s.back)-1]
		if alive == nil || alive(id) {
			s.current = id
			return true
		}
	}
	return false
}

// NearestSurvivor picks where the current issue goes when it is no longer
// shown: the closest issue in the old display order that alive accepts,
// looking below before above at the same distance. It returns "" when nothing
// survives or cur was not in the old order.
func NearestSurvivor(oldOrder []string, cur string, alive func(id string) bool) string {
	at := slices.Index(oldOrder, cur)
	if at < 0 {
		return ""
	}
	if alive(cur) {
		return cur
	}
	for d := 1; at-d >= 0 || at+d < len(oldOrder); d++ {
		if i := at + d; i < len(oldOrder) && alive(oldOrder[i]) {
			return oldOrder[i]
		}
		if i := at - d; i >= 0 && alive(oldOrder[i]) {
			return oldOrder[i]
		}
	}
	return ""
}

// ToggleMark flips the mark on id and reports whether it is marked now.
func (s *Session) ToggleMark(id string) bool {
	if _, ok := s.marks[id]; ok {
		delete(s.marks, id)
		return false
	}
	s.marks[id] = struct{}{}
	return true
}

// Marked reports whether id is marked.
func (s *Session) Marked(id string) bool {
	_, ok := s.marks[id]
	return ok
}

// MarkCount is the number of marked issues, hidden ones included.
func (s *Session) MarkCount() int { return len(s.marks) }

// MarkedIDs lists the marked IDs sorted.
func (s *Session) MarkedIDs() []string {
	out := make([]string, 0, len(s.marks))
	for id := range s.marks {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// HiddenMarks counts the marked issues visible does not accept.
func (s *Session) HiddenMarks(visible func(id string) bool) int {
	n := 0
	for id := range s.marks {
		if !visible(id) {
			n++
		}
	}
	return n
}

// ClearMarks drops every mark.
func (s *Session) ClearMarks() { clear(s.marks) }

// Prune forgets marks and back-stack entries of issues that no longer exist.
func (s *Session) Prune(exists func(id string) bool) {
	for id := range s.marks {
		if !exists(id) {
			delete(s.marks, id)
		}
	}
	s.back = slices.DeleteFunc(s.back, func(id string) bool { return !exists(id) })
}

// Push opens a layer on top.
func (s *Session) Push(l Layer) { s.layers = append(s.layers, l) }

// Pop closes the top layer, if any.
func (s *Session) Pop() {
	if len(s.layers) > 0 {
		s.layers = s.layers[:len(s.layers)-1]
	}
}

// Top is the topmost layer; ok is false at base level.
func (s *Session) Top() (Layer, bool) {
	if len(s.layers) == 0 {
		return 0, false
	}
	return s.layers[len(s.layers)-1], true
}

// Has reports whether l is somewhere on the stack.
func (s *Session) Has(l Layer) bool { return slices.Contains(s.layers, l) }

// Remove takes the topmost layer of kind l off the stack wherever it sits.
func (s *Session) Remove(l Layer) {
	for i := len(s.layers) - 1; i >= 0; i-- {
		if s.layers[i] == l {
			s.layers = slices.Delete(s.layers, i, i+1)
			return
		}
	}
}

// Replace turns the topmost layer of kind from into to, keeping its place in
// the stack, and reports whether there was one.
func (s *Session) Replace(from, to Layer) bool {
	for i := len(s.layers) - 1; i >= 0; i-- {
		if s.layers[i] == from {
			s.layers[i] = to
			return true
		}
	}
	return false
}

// Esc applies one Esc press: it closes the top layer (dialog, docked bar,
// detail focus, detail panel, in that order), then clears the marks, then
// asks for the active search or filter to be cleared.
func (s *Session) Esc() Esc {
	switch {
	case len(s.layers) > 0:
		s.Pop()
		return EscLayer
	case len(s.marks) > 0:
		s.ClearMarks()
		return EscMarks
	case s.ScopeActive:
		return EscScope
	}
	return EscNothing
}
