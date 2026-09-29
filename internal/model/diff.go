package model

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Diff turns the change from prev to cur into activity events, one per
// change and issue, ordered by issue ID. Time is when cur was fetched. The
// only actor it knows is the creator of a created issue; who did the rest is
// for the journal or the caller to say. A nil prev, as at the first
// snapshot, yields nothing.
//
// Dependency edges produce no events of their own: their effect shows as
// became blocked, unblocked or became ready. Title, description, label and
// type changes coalesce into one edited event. An issue that is closed after
// the change gets no blocked or ready events.
func Diff(prev, cur *Snapshot, st Statuses) []Event {
	if prev == nil || cur == nil {
		return nil
	}
	ids := unionIDs(prev.IDs(), cur.IDs())
	var out []Event
	for _, id := range ids {
		a, inPrev := prev.Issue(id)
		b, inCur := cur.Issue(id)
		switch {
		case inPrev && !inCur:
			out = append(out, Event{Kind: KindDeleted, IssueID: id, Title: a.Title, Time: cur.FetchedAt()})
		case !inPrev && inCur:
			out = append(out, Event{Kind: KindCreated, IssueID: id, Title: b.Title, Time: cur.FetchedAt(), Actor: b.CreatedBy})
		default:
			out = append(out, diffIssue(a, b, verdictOf(prev, a), verdictOf(cur, b), st, cur.FetchedAt())...)
		}
	}
	return out
}

func unionIDs(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case j >= len(b) || (i < len(a) && a[i] < b[j]):
			out = append(out, a[i])
			i++
		case i >= len(a) || b[j] < a[i]:
			out = append(out, b[j])
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	return out
}

// verdict is bd's answer about one issue at one point in time.
type verdict struct {
	blocked, ready bool
}

func verdictOf(s *Snapshot, is *Issue) verdict {
	return verdict{blocked: s.IsBlocked(is.ID), ready: s.IsReady(is.ID)}
}

func (v verdict) blockedWith(is *Issue) bool { return v.blocked || is.Status == statusBlocked }

func diffIssue(a, b *Issue, va, vb verdict, st Statuses, at time.Time) []Event {
	var out []Event
	emit := func(k Kind, detail string) {
		out = append(out, Event{Kind: k, IssueID: b.ID, Title: b.Title, Time: at, Detail: detail})
	}

	catA, catB := st.Category(a.Status), st.Category(b.Status)
	statusChanged := a.Status != b.Status
	closedNow := catB == CategoryDone
	pa := Present(a.Status, va.blocked, st)
	pb := Present(b.Status, vb.blocked, st)

	assigneeChanged := a.Assignee != b.Assignee
	startedWork := statusChanged && pb.Status == InProgress && pa.Status != InProgress && catA != CategoryDone
	claimed := b.Assignee != "" && pb.Status == InProgress && (assigneeChanged || startedWork)

	switch {
	case statusChanged && catA == CategoryDone && catB != CategoryDone:
		emit(KindReopened, "")
	case statusChanged && catA != CategoryDone && closedNow:
		emit(KindClosed, "")
	case statusChanged && !claimed && a.Status != statusBlocked && b.Status != statusBlocked:
		emit(KindStatusChanged, a.Status+"→"+b.Status)
	}
	if claimed {
		emit(KindClaimed, b.Assignee)
	}
	if assigneeChanged && a.Assignee != "" && b.Assignee == "" {
		emit(KindUnassigned, a.Assignee)
	}

	if !closedNow {
		blockedA, blockedB := va.blockedWith(a), vb.blockedWith(b)
		switch {
		case !blockedA && blockedB:
			emit(KindBecameBlocked, "")
		case blockedA && !blockedB:
			emit(KindUnblocked, "")
		}
		if !va.ready && vb.ready {
			emit(KindBecameReady, "")
		}
	}

	if a.Priority != b.Priority {
		emit(KindPriorityChanged, strconv.Itoa(a.Priority)+"→"+strconv.Itoa(b.Priority))
	}
	if fields := editedFields(a, b); len(fields) > 0 {
		emit(KindEdited, strings.Join(fields, ", "))
	}
	if n := b.CommentCount - a.CommentCount; n > 0 {
		detail := ""
		if n > 1 {
			detail = fmt.Sprintf("+%d", n)
		}
		emit(KindCommented, detail)
	}
	return out
}

func editedFields(a, b *Issue) []string {
	var f []string
	if a.Title != b.Title {
		f = append(f, "title")
	}
	if a.Description != b.Description || a.Design != b.Design ||
		a.AcceptanceCriteria != b.AcceptanceCriteria || a.Notes != b.Notes {
		f = append(f, "description")
	}
	if !sameLabels(a.Labels, b.Labels) {
		f = append(f, "labels")
	}
	if a.IssueType != b.IssueType {
		f = append(f, "type")
	}
	return f
}

func sameLabels(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := slices.Clone(a), slices.Clone(b)
	sort.Strings(x)
	sort.Strings(y)
	return slices.Equal(x, y)
}
