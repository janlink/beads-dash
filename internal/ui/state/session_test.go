package state_test

import (
	"slices"
	"testing"

	"github.com/janlink/beads-dash/internal/ui/state"
)

func set(ids ...string) func(string) bool {
	return func(id string) bool { return slices.Contains(ids, id) }
}

func TestNearestSurvivor(t *testing.T) {
	old := []string{"a", "b", "c", "d", "e"}
	tests := []struct {
		name  string
		cur   string
		alive []string
		want  string
	}{
		{"still there", "c", []string{"a", "b", "c"}, "c"},
		{"next below wins at equal distance", "c", []string{"b", "d"}, "d"},
		{"above when below is gone", "c", []string{"a", "b"}, "b"},
		{"farther below beats nearer none", "b", []string{"e"}, "e"},
		{"first row deleted", "a", []string{"b", "c"}, "b"},
		{"last row deleted", "e", []string{"a", "d"}, "d"},
		{"nothing survives", "c", nil, ""},
		{"unknown current", "x", []string{"a"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := state.NearestSurvivor(old, tt.cur, set(tt.alive...)); got != tt.want {
				t.Errorf("NearestSurvivor = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMarksSurviveHidden(t *testing.T) {
	s := state.New()
	for _, id := range []string{"a", "b", "c"} {
		s.ToggleMark(id)
	}
	visible := set("a", "b")
	if got := s.HiddenMarks(visible); got != 1 {
		t.Errorf("hidden = %d, want 1", got)
	}
	if s.MarkCount() != 3 {
		t.Errorf("marks = %d, want 3 (hidden marks count)", s.MarkCount())
	}
	s.Prune(set("a", "c"))
	if got := s.MarkedIDs(); !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("marks after prune = %v", got)
	}
	if !s.Marked("c") {
		t.Error("hidden but existing mark was dropped")
	}
	if s.ToggleMark("a") || s.Marked("a") {
		t.Error("second toggle did not unmark")
	}
}

func TestEscCascade(t *testing.T) {
	s := state.New()
	s.Push(state.LayerDetail)
	s.Push(state.LayerDetailFocus)
	s.Push(state.LayerBar)
	s.Push(state.LayerDialog)
	s.ToggleMark("a")
	s.ScopeActive = true

	var got []state.Layer
	for range 4 {
		top, _ := s.Top()
		got = append(got, top)
		if e := s.Esc(); e != state.EscLayer {
			t.Fatalf("Esc = %v, want a layer", e)
		}
	}
	want := []state.Layer{state.LayerDialog, state.LayerBar, state.LayerDetailFocus, state.LayerDetail}
	if !slices.Equal(got, want) {
		t.Errorf("closed %v, want %v", got, want)
	}
	if e := s.Esc(); e != state.EscMarks || s.MarkCount() != 0 {
		t.Errorf("Esc = %v, marks %d; want marks cleared", e, s.MarkCount())
	}
	if e := s.Esc(); e != state.EscScope {
		t.Errorf("Esc = %v, want scope clear", e)
	}
	s.ScopeActive = false
	if e := s.Esc(); e != state.EscNothing {
		t.Errorf("Esc = %v, want nothing", e)
	}
}

func TestRemoveClosesTheTopmostOfAKind(t *testing.T) {
	s := state.New()
	s.Push(state.LayerDialog)
	s.Push(state.LayerBar)
	s.Push(state.LayerDialog)
	s.Remove(state.LayerDialog)
	if top, _ := s.Top(); top != state.LayerBar {
		t.Errorf("top = %v, want the bar", top)
	}
	if !s.Has(state.LayerDialog) {
		t.Error("the lower dialog was removed too")
	}
}

func TestBackStack(t *testing.T) {
	s := state.New()
	s.SetCurrent("a")
	s.Jump("b")
	s.Jump("c")
	s.Jump("c")
	if _, ok := s.Back(set("a", "b", "c")); !ok || s.Current() != "b" {
		t.Errorf("back to %q", s.Current())
	}
	if _, ok := s.Back(set("c")); ok || s.Current() != "b" {
		t.Errorf("back with only c alive moved to %q", s.Current())
	}
	if _, ok := s.Back(set()); ok {
		t.Error("Back succeeded with an empty stack")
	}
}

func TestPruneDropsDeletedBackEntries(t *testing.T) {
	s := state.New()
	s.SetCurrent("a")
	s.Jump("b")
	s.Jump("c")
	s.Prune(set("b", "c"))
	if _, ok := s.Back(nil); !ok || s.Current() != "b" {
		t.Errorf("back = %q, want b", s.Current())
	}
	if _, ok := s.Back(nil); ok {
		t.Error("deleted origin a stayed on the stack")
	}
}

func TestReplaceKeepsTheLayerInPlace(t *testing.T) {
	s := state.New()
	s.Push(state.LayerDetail)
	s.Push(state.LayerDialog)
	if !s.Replace(state.LayerDetail, state.LayerDetailFocus) {
		t.Fatal("Replace found nothing")
	}
	if top, _ := s.Top(); top != state.LayerDialog {
		t.Errorf("top = %v, want the dialog", top)
	}
	s.Pop()
	if top, _ := s.Top(); top != state.LayerDetailFocus {
		t.Errorf("below the dialog: %v", top)
	}
	if s.Replace(state.LayerBar, state.LayerDialog) {
		t.Error("replaced a layer that is not there")
	}
}

func TestJumpFromRemembersTheViewEvenForTheSameIssue(t *testing.T) {
	s := state.New()
	s.SetCurrent("a")
	s.JumpFrom(0, "a")
	s.JumpFrom(3, "b")
	if slot, ok := s.Back(nil); !ok || slot != 3 || s.Current() != "a" {
		t.Errorf("first back: slot %d current %q", slot, s.Current())
	}
	if slot, ok := s.Back(nil); !ok || slot != 0 || s.Current() != "a" {
		t.Errorf("second back: slot %d current %q", slot, s.Current())
	}
}
