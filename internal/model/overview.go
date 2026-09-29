package model

import (
	"sort"
	"time"
)

// SparkDays is the number of days the closed-per-day sparkline covers.
const SparkDays = 14

// AttentionTop is how many Ready and Blocked issues Needs attention lists.
const AttentionTop = 3

// FeedBucket groups feed events by age.
type FeedBucket uint8

// The buckets, newest first.
const (
	BucketJustNow FeedBucket = iota
	BucketLastHour
	BucketToday
	BucketYesterday
	BucketEarlier
)

var bucketLabels = [...]string{"just now", "last hour", "today", "yesterday", "earlier"}

// Label is the heading of the bucket.
func (b FeedBucket) Label() string { return bucketLabels[b] }

// BucketOf classifies the age of t as of now.
func BucketOf(now, t time.Time) FeedBucket {
	d := now.Sub(t)
	switch {
	case d < 5*time.Minute:
		return BucketJustNow
	case d < time.Hour:
		return BucketLastHour
	case d < 24*time.Hour:
		return BucketToday
	case d < 48*time.Hour:
		return BucketYesterday
	}
	return BucketEarlier
}

// Attention is the Needs attention box: the head of the Ready list, the
// head of the blocked list and the assigned-but-not-started count.
type Attention struct {
	Ready, Blocked           []string
	ReadyTotal, BlockedTotal int
	// ReadyUnassigned is how many of the ready issues have no assignee.
	ReadyUnassigned int
	// AssignedNotStarted is the number of open issues that have an assignee,
	// blocked ones included.
	AssignedNotStarted int
}

// Assignee is a person with work in progress.
type Assignee struct {
	Who string
	IDs []string
}

// Overview is everything the Overview draws besides the feed's layout.
type Overview struct {
	// Total counts the issues that pass the scope's facets; Counts splits
	// them by presentation status.
	Total  int
	Counts [6]int
	Ready  int
	// Deps counts the blocking dependency edges of those issues.
	Deps int
	// Closed14 is the closed issues per day over the last [SparkDays] days,
	// oldest first; the last cell is the 24 hours up to now.
	Closed14  [SparkDays]int
	Attention Attention
	// Active lists the assignees with issues in progress, the busiest first.
	Active []Assignee
	// Feed is the events of the scope's issues, newest first.
	Feed []Event
}

// ClosedPercent is the share of closed issues, rounded down.
func (o Overview) ClosedPercent() int {
	if o.Total == 0 {
		return 0
	}
	return o.Counts[Closed] * 100 / o.Total
}

// BuildOverview aggregates the workspace for the Overview. Like the Ready
// view it applies only the scope's facets, not its status visibility, so a
// just-closed issue still counts and shows in the feed. events is the feed
// ring, newest first.
func BuildOverview(snap *Snapshot, st Statuses, sc Scope, m *Matches, events []Event, now time.Time) Overview {
	var o Overview
	who := map[string][]string{}
	for _, id := range snap.IDs() {
		if !m.Facets(id) {
			continue
		}
		is, _ := snap.Issue(id)
		p := snap.Present(id, st).Status
		o.Total++
		o.Counts[p]++
		if is.Assignee != "" && (p == Open || p == Blocked) {
			o.Attention.AssignedNotStarted++
		}
		for _, e := range is.Dependencies {
			if BlockingEdge(e.Type) {
				o.Deps++
			}
		}
		switch p {
		case InProgress:
			if is.Assignee != "" {
				who[is.Assignee] = append(who[is.Assignee], id)
			}
		case Closed:
			if !is.ClosedAt.IsZero() {
				day := int(max(now.Sub(is.ClosedAt), 0) / (24 * time.Hour))
				if day < SparkDays {
					o.Closed14[SparkDays-1-day]++
				}
			}
		case Open, Blocked, Frozen, Other:
		}
	}
	list := BuildReady(snap, st, sc, m)
	ready := append(append([]string(nil), list.Unassigned...), list.Assigned...)
	o.Ready = len(ready)
	o.Attention.Ready = head(ready, AttentionTop)
	o.Attention.ReadyTotal = len(ready)
	o.Attention.ReadyUnassigned = len(list.Unassigned)
	o.Attention.BlockedTotal = len(list.Blocked)
	for _, b := range list.Blocked[:min(len(list.Blocked), AttentionTop)] {
		o.Attention.Blocked = append(o.Attention.Blocked, b.ID)
	}
	for name, ids := range who {
		sort.SliceStable(ids, func(i, j int) bool {
			a, _ := snap.Issue(ids[i])
			b, _ := snap.Issue(ids[j])
			return siblingLess(a, b, false, false)
		})
		o.Active = append(o.Active, Assignee{Who: name, IDs: ids})
	}
	sort.Slice(o.Active, func(i, j int) bool {
		a, b := o.Active[i], o.Active[j]
		if len(a.IDs) != len(b.IDs) {
			return len(a.IDs) > len(b.IDs)
		}
		return a.Who < b.Who
	})
	for _, e := range events {
		if _, known := snap.Issue(e.IssueID); !known && !sc.Active() || m.Facets(e.IssueID) {
			o.Feed = append(o.Feed, e)
		}
	}
	return o
}

func head(ids []string, n int) []string { return ids[:min(len(ids), n)] }
