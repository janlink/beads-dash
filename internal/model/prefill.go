package model

import (
	"sort"
	"time"
)

// Prefill rebuilds activity from what one snapshot already tells: each
// issue's created_at, started_at and closed_at, newest [RingCapacity] first
// taken, returned oldest first. Events are marked Prefill. Only a created
// event knows its actor (created_by); a started issue counts as claimed when
// it has an assignee and as a status change otherwise.
func Prefill(s *Snapshot) []Event {
	if s == nil {
		return nil
	}
	var out []Event
	add := func(t time.Time, e Event) {
		if t.IsZero() {
			return
		}
		e.Time, e.Prefill = t, true
		out = append(out, e)
	}
	for _, id := range s.IDs() {
		is, _ := s.Issue(id)
		add(is.CreatedAt, Event{Kind: KindCreated, IssueID: id, Title: is.Title, Actor: is.CreatedBy})
		if is.Assignee != "" {
			add(is.StartedAt, Event{Kind: KindClaimed, IssueID: id, Title: is.Title, Detail: is.Assignee})
		} else {
			add(is.StartedAt, Event{Kind: KindStatusChanged, IssueID: id, Title: is.Title, Detail: "→ in progress"})
		}
		add(is.ClosedAt, Event{Kind: KindClosed, IssueID: id, Title: is.Title})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	if extra := len(out) - RingCapacity; extra > 0 {
		out = out[extra:]
	}
	return out
}
