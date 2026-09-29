package model_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

func secs(n int64) time.Duration { return time.Duration(n) * time.Second }

func TestHighlightIDs(t *testing.T) {
	evs := []model.Event{
		{Kind: model.KindEdited, IssueID: "b"},
		{Kind: model.KindPriorityChanged, IssueID: "a"},
		{Kind: model.KindCommented, IssueID: "b"},
		{Kind: model.KindDeleted, IssueID: "gone"},
		{Kind: model.KindEdited, IssueID: "gone"},
		{Kind: model.KindCreated, IssueID: "old", Prefill: true},
		{Kind: model.KindClosed, IssueID: "own", Own: true},
	}
	got := model.HighlightIDs(evs)
	if want := []string{"b", "a", "own"}; !reflect.DeepEqual(got, want) {
		t.Errorf("HighlightIDs = %v, want %v", got, want)
	}
	if got := model.HighlightIDs(nil); got != nil {
		t.Errorf("HighlightIDs(nil) = %v", got)
	}
}

func TestNotifyFiltersKindsPrefillAndOwn(t *testing.T) {
	kinds := model.KindSetOf("closed", "blocked", "ready")
	evs := []model.Event{
		{Kind: model.KindClosed, IssueID: "a"},
		{Kind: model.KindEdited, IssueID: "b"},
		{Kind: model.KindBecameReady, IssueID: "c"},
		{Kind: model.KindClosed, IssueID: "d", Prefill: true},
		{Kind: model.KindClosed, IssueID: "e", Own: true},
	}
	n := model.Notify(evs, kinds)
	if len(n.Events) != 2 || n.Events[0].IssueID != "a" || n.Events[1].IssueID != "c" || n.Summary {
		t.Errorf("Notify = %+v", n)
	}
}

func TestNotifySummaryAboveThreshold(t *testing.T) {
	kinds := model.KindSetOf("closed")
	var evs []model.Event
	for i := 0; i < model.SummaryThreshold; i++ {
		evs = append(evs, model.Event{Kind: model.KindClosed, IssueID: "x"})
	}
	if n := model.Notify(evs, kinds); n.Summary || len(n.Events) != 3 {
		t.Errorf("3 events: %+v", n)
	}
	evs = append(evs, model.Event{Kind: model.KindClosed, IssueID: "y"})
	if n := model.Notify(evs, kinds); !n.Summary || len(n.Events) != 4 {
		t.Errorf("4 events: %+v", n)
	}
	if n := model.Notify(evs, model.KindSet{}); len(n.Events) != 0 || n.Summary {
		t.Errorf("empty set: %+v", n)
	}
}
