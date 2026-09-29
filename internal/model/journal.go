package model

import "time"

// Journal operations, as bd writes them.
const (
	OpCreate    = "create"
	OpUpdate    = "update"
	OpClose     = "close"
	OpDelete    = "delete"
	OpDepAdd    = "dep_add"
	OpDepRemove = "dep_remove"
	OpComment   = "comment"
)

// JournalRecord is one entry of bd's events journal: a committed mutation
// with the issue as it was afterwards. The journal carries no ready verdict,
// so records alone never say that an issue became ready.
type JournalRecord struct {
	Seq     int64
	Time    time.Time
	Op      string
	IssueID string
	// Actor is empty for derived maintenance, which has no one behind it.
	Actor string
	// Issue is the issue after the mutation; nil for a delete or when the
	// record carried none.
	Issue   *Issue
	Blocked bool
}

// JournalTracker turns journal records into activity events by remembering
// the last state seen of each issue. It builds the feed of a workspace whose
// history predates bdash. Records without an actor create no event of their
// own but still update the remembered state. Not safe for concurrent use.
type JournalTracker struct {
	st    Statuses
	state map[string]trackedIssue
}

type trackedIssue struct {
	issue   Issue
	blocked bool
}

// NewJournalTracker starts a tracker that judges statuses with st.
func NewJournalTracker(st Statuses) *JournalTracker {
	return &JournalTracker{st: st, state: map[string]trackedIssue{}}
}

// SetStatuses replaces the status table the tracker judges with, after the
// workspace's custom statuses changed.
func (t *JournalTracker) SetStatuses(st Statuses) { t.st = st }

// Apply takes the next record in sequence order and returns the events it
// stands for, all marked Prefill. An update to an issue the tracker has not
// seen before has nothing to compare with and yields nothing.
func (t *JournalTracker) Apply(r JournalRecord) []Event {
	prev, seen := t.state[r.IssueID]
	if r.Op == OpDelete {
		delete(t.state, r.IssueID)
	} else if r.Issue != nil {
		t.state[r.IssueID] = trackedIssue{issue: *r.Issue, blocked: r.Blocked}
	}
	if r.Actor == "" {
		return nil
	}
	base := Event{IssueID: r.IssueID, Time: r.Time, Actor: r.Actor, Prefill: true, Seq: r.Seq}
	one := func(k Kind, title string) []Event {
		base.Kind, base.Title = k, title
		return []Event{base}
	}
	title := ""
	if r.Issue != nil {
		title = r.Issue.Title
	}
	switch r.Op {
	case OpCreate:
		return one(KindCreated, title)
	case OpClose:
		return one(KindClosed, title)
	case OpDelete:
		return one(KindDeleted, prev.issue.Title)
	case OpComment:
		return one(KindCommented, title)
	case OpUpdate, OpDepAdd, OpDepRemove:
		if !seen || r.Issue == nil {
			return nil
		}
		evs := diffIssue(&prev.issue, r.Issue, verdict{blocked: prev.blocked}, verdict{blocked: r.Blocked}, t.st, r.Time)
		for i := range evs {
			evs[i].Actor, evs[i].Prefill, evs[i].Seq = r.Actor, true, r.Seq
		}
		return evs
	}
	return nil
}

// AttributeActors fills in who did what for live events from the journal
// records that arrived since the previous refresh: each event takes the
// actor and time of the newest record on its issue that could have caused
// it. Events without such a record, such as derived effects on other issues,
// keep no actor.
func AttributeActors(events []Event, records []JournalRecord) {
	for i := range events {
		e := &events[i]
		if e.Prefill {
			continue
		}
		for j := len(records) - 1; j >= 0; j-- {
			r := records[j]
			if r.IssueID != e.IssueID || r.Actor == "" || !causes(r.Op, e.Kind) {
				continue
			}
			e.Actor, e.Time, e.Seq = r.Actor, r.Time, r.Seq
			break
		}
	}
}

func causes(op string, k Kind) bool {
	switch k {
	case KindCreated:
		return op == OpCreate
	case KindClosed:
		return op == OpClose || op == OpUpdate
	case KindDeleted:
		return op == OpDelete
	case KindCommented:
		return op == OpComment
	case KindBecameBlocked, KindUnblocked, KindBecameReady:
		return op == OpUpdate || op == OpDepAdd || op == OpDepRemove
	case KindClaimed, KindUnassigned, KindReopened, KindStatusChanged, KindPriorityChanged, KindEdited:
		return op == OpUpdate
	}
	return false
}
