package model_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

func TestPrefillFromTimestamps(t *testing.T) {
	at := func(m int) time.Time { return t0.Add(time.Duration(m) * time.Minute) }
	a := with(issue("a", "closed"), assignee("ann"), func(is *model.Issue) {
		is.CreatedAt, is.CreatedBy, is.StartedAt, is.ClosedAt = at(1), "cat", at(3), at(5)
	})
	b := with(issue("b", "in_progress"), func(is *model.Issue) { is.CreatedAt, is.StartedAt = at(2), at(4) })
	c := with(issue("c", "open"), func(is *model.Issue) { is.CreatedAt = time.Time{} })
	evs := model.Prefill(snap(t1, nil, nil, a, b, c))
	var got []string
	for _, e := range evs {
		if !e.Prefill {
			t.Errorf("%v is not marked prefill", e)
		}
		got = append(got, fmt.Sprintf("%s:%s:%s", e.IssueID, e.Kind, e.Actor))
	}
	want := []string{"a:created:cat", "b:created:", "a:claimed:", "b:status:", "a:closed:"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	if evs[2].Detail != "ann" {
		t.Errorf("claimed detail = %q", evs[2].Detail)
	}
	if model.Prefill(nil) != nil {
		t.Error("nil snapshot must prefill nothing")
	}
}

func TestPrefillKeepsNewestFiveHundred(t *testing.T) {
	var issues []model.Issue
	for i := 0; i < model.RingCapacity+10; i++ {
		i := i
		issues = append(issues, with(issue(fmt.Sprintf("i%04d", i), "open"), func(is *model.Issue) {
			is.CreatedAt = t0.Add(time.Duration(i) * time.Second)
		}))
	}
	evs := model.Prefill(snap(t1, nil, nil, issues...))
	if len(evs) != model.RingCapacity {
		t.Fatalf("len = %d", len(evs))
	}
	if evs[0].IssueID != "i0010" || evs[len(evs)-1].IssueID != fmt.Sprintf("i%04d", model.RingCapacity+9) {
		t.Errorf("window = %s .. %s", evs[0].IssueID, evs[len(evs)-1].IssueID)
	}
}
