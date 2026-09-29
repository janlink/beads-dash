package state_test

import (
	"slices"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/ui/state"
)

var t0 = time.Date(2026, 9, 29, 12, 0, 0, 300*int(time.Millisecond), time.UTC)

func TestHighlightLivesForVisibleTime(t *testing.T) {
	h := state.NewHighlights(10 * time.Second)
	h.Trigger(t0, []string{"a"}, nil)
	if !h.Live("a", t0.Add(9*time.Second)) {
		t.Error("highlight gone after 9 s")
	}
	if !h.Live("a", t0.Add(10*time.Second+600*time.Millisecond)) {
		t.Error("highlight ended before its 1 s bucket did")
	}
	if h.Live("a", t0.Add(10*time.Second+800*time.Millisecond)) {
		t.Error("highlight outlived its bucket")
	}
}

func TestHighlightBucketsExpireTogether(t *testing.T) {
	h := state.NewHighlights(10 * time.Second)
	h.Trigger(t0, []string{"a"}, nil)
	h.Trigger(t0.Add(200*time.Millisecond), []string{"b"}, nil)
	d, ok := h.Next(t0.Add(time.Second))
	if !ok {
		t.Fatal("no tick scheduled")
	}
	at := t0.Add(time.Second).Add(d)
	gone := h.Expire(at)
	if !slices.Equal(gone, []string{"a", "b"}) {
		t.Errorf("expired %v at bucket end %v, want both", gone, at)
	}
	if _, ok := h.Next(at); ok {
		t.Error("tick still scheduled with nothing left")
	}
}

func TestHighlightPausesOnBlurAndDialog(t *testing.T) {
	h := state.NewHighlights(10 * time.Second)
	h.Trigger(t0, []string{"a"}, nil)
	h.SetPaused(t0.Add(4*time.Second), true)
	if _, ok := h.Next(t0.Add(5 * time.Second)); ok {
		t.Error("tick scheduled while paused")
	}
	if !h.Live("a", t0.Add(time.Hour)) {
		t.Error("paused highlight expired")
	}
	h.SetPaused(t0.Add(time.Hour), false)
	resumed := t0.Add(time.Hour)
	if !h.Live("a", resumed.Add(5*time.Second)) {
		t.Error("6 s should remain after 4 s of visible time")
	}
	if h.Live("a", resumed.Add(7*time.Second)) {
		t.Error("highlight outlived its remaining time after resume")
	}
}

func TestHighlightTriggeredWhilePausedKeepsFullDuration(t *testing.T) {
	h := state.NewHighlights(10 * time.Second)
	h.SetPaused(t0, true)
	h.Trigger(t0.Add(time.Minute), []string{"a"}, nil)
	h.SetPaused(t0.Add(2*time.Minute), false)
	if !h.Live("a", t0.Add(2*time.Minute+9*time.Second)) {
		t.Error("change that landed while blurred lost time")
	}
}

func TestHighlightRestartKeepsEvents(t *testing.T) {
	h := state.NewHighlights(10 * time.Second)
	e1 := model.Event{IssueID: "a", Kind: model.KindEdited}
	e2 := model.Event{IssueID: "a", Kind: model.KindCommented}
	h.Trigger(t0, []string{"a"}, []model.Event{e1})
	h.Trigger(t0.Add(8*time.Second), []string{"a"}, []model.Event{e2, {IssueID: "b"}, {IssueID: "a", Prefill: true}})
	if !h.Live("a", t0.Add(17*time.Second)) {
		t.Error("second event did not restart the clock")
	}
	if got := h.Events("a"); len(got) != 2 || got[0].Kind != model.KindEdited || got[1].Kind != model.KindCommented {
		t.Errorf("events = %+v", got)
	}
}

func TestHighlightOffAndPrune(t *testing.T) {
	h := state.NewHighlights(0)
	h.Trigger(t0, []string{"a"}, nil)
	if h.Live("a", t0) || len(h.IDs(t0)) != 0 {
		t.Error("highlight_seconds = 0 must switch highlights off")
	}
	h = state.NewHighlights(5 * time.Second)
	h.Trigger(t0, []string{"a", "b"}, nil)
	h.Prune(set("a"))
	if got := h.IDs(t0); !slices.Equal(got, []string{"a"}) {
		t.Errorf("IDs = %v", got)
	}
}
