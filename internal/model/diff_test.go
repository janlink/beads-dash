package model_test

import (
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

var t1 = t0.Add(30 * time.Second)

func snap(at time.Time, ready []string, blocked map[string][]string, issues ...model.Issue) *model.Snapshot {
	return model.NewSnapshot(issues, model.Readiness{Ready: ready, Blocked: blocked}, at)
}

func with(is model.Issue, opts ...func(*model.Issue)) model.Issue {
	for _, o := range opts {
		o(&is)
	}
	return is
}

func assignee(a string) func(*model.Issue) { return func(is *model.Issue) { is.Assignee = a } }
func status(s string) func(*model.Issue)   { return func(is *model.Issue) { is.Status = s } }
func priority(p int) func(*model.Issue)    { return func(is *model.Issue) { is.Priority = p } }
func title(s string) func(*model.Issue)    { return func(is *model.Issue) { is.Title = s } }
func labels(l ...string) func(*model.Issue) {
	return func(is *model.Issue) { is.Labels = l }
}
func comments(n int) func(*model.Issue) { return func(is *model.Issue) { is.CommentCount = n } }

func kindsOf(evs []model.Event) []model.Kind {
	out := make([]model.Kind, len(evs))
	for i, e := range evs {
		out[i] = e.Kind
	}
	return out
}

func wantKinds(t *testing.T, evs []model.Event, want ...model.Kind) {
	t.Helper()
	got := kindsOf(evs)
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
}

func TestDiffFirstSnapshotHasNoEvents(t *testing.T) {
	if evs := model.Diff(nil, snap(t0, nil, nil, issue("a", "open")), model.Statuses{}); evs != nil {
		t.Errorf("events = %v", evs)
	}
	if evs := model.Diff(snap(t0, nil, nil), nil, model.Statuses{}); evs != nil {
		t.Errorf("events = %v", evs)
	}
}

func TestDiffNoChangeNoEvents(t *testing.T) {
	a := snap(t0, []string{"a"}, nil, issue("a", "open"))
	b := snap(t1, []string{"a"}, nil, issue("a", "open"))
	if evs := model.Diff(a, b, model.Statuses{}); len(evs) != 0 {
		t.Errorf("events = %v", evs)
	}
}

func TestDiffCreatedTakesCreatorAsActor(t *testing.T) {
	prev := snap(t0, nil, nil)
	cur := snap(t1, nil, nil, with(issue("a", "open"), func(is *model.Issue) { is.CreatedBy = "ann" }))
	evs := model.Diff(prev, cur, model.Statuses{})
	wantKinds(t, evs, model.KindCreated)
	if e := evs[0]; e.IssueID != "a" || e.Title != "t a" || e.Actor != "ann" || !e.Time.Equal(t1) || e.Prefill {
		t.Errorf("event = %+v", e)
	}
}

func TestDiffDeletedKeepsTitle(t *testing.T) {
	prev := snap(t0, nil, nil, issue("a", "open"), issue("b", "open"))
	cur := snap(t1, nil, nil, issue("b", "open"))
	evs := model.Diff(prev, cur, model.Statuses{})
	wantKinds(t, evs, model.KindDeleted)
	if evs[0].IssueID != "a" || evs[0].Title != "t a" || evs[0].Actor != "" {
		t.Errorf("event = %+v", evs[0])
	}
}

func TestDiffClaimedVersusUnassigned(t *testing.T) {
	st := model.Statuses{}
	open := snap(t0, nil, nil, issue("a", "open"))
	claimed := snap(t1, nil, nil, with(issue("a", "in_progress"), assignee("ann")))
	evs := model.Diff(open, claimed, st)
	wantKinds(t, evs, model.KindClaimed)
	if evs[0].Detail != "ann" {
		t.Errorf("detail = %q", evs[0].Detail)
	}

	reassigned := snap(t1, nil, nil, with(issue("a", "in_progress"), assignee("bob")))
	wantKinds(t, model.Diff(claimed, reassigned, st), model.KindClaimed)

	released := snap(t1, nil, nil, with(issue("a", "in_progress"), assignee("")))
	wantKinds(t, model.Diff(claimed, released, st), model.KindUnassigned)

	releasedOpen := snap(t1, nil, nil, with(issue("a", "open"), assignee("")))
	wantKinds(t, model.Diff(claimed, releasedOpen, st), model.KindStatusChanged, model.KindUnassigned)

	assignedOnly := snap(t1, nil, nil, with(issue("a", "open"), assignee("ann")))
	wantKinds(t, model.Diff(open, assignedOnly, st))

	startLater := snap(t1, nil, nil, with(issue("a", "in_progress"), assignee("ann")))
	wantKinds(t, model.Diff(assignedOnly, startLater, st), model.KindClaimed)

	startUnassigned := snap(t1, nil, nil, issue("a", "in_progress"))
	evs = model.Diff(open, startUnassigned, st)
	wantKinds(t, evs, model.KindStatusChanged)
	if evs[0].Detail != "open→in_progress" {
		t.Errorf("detail = %q", evs[0].Detail)
	}
}

func TestDiffClosedReopened(t *testing.T) {
	st := model.Statuses{}
	open := snap(t0, []string{"a"}, nil, with(issue("a", "in_progress"), assignee("ann")))
	closed := snap(t1, nil, nil, with(issue("a", "closed"), assignee("ann")))
	wantKinds(t, model.Diff(open, closed, st), model.KindClosed)
	wantKinds(t, model.Diff(closed, open, st), model.KindReopened, model.KindBecameReady)

	custom := model.NewStatuses([]model.StatusInfo{
		{Name: "open", Category: model.CategoryActive},
		{Name: "closed", Category: model.CategoryDone},
		{Name: "shipped", Category: model.CategoryDone},
	})
	toShipped := snap(t1, nil, nil, issue("a", "shipped"))
	wantKinds(t, model.Diff(snap(t0, nil, nil, issue("a", "open")), toShipped, custom), model.KindClosed)
	wantKinds(t, model.Diff(toShipped, snap(t1, nil, nil, issue("a", "closed")), custom), model.KindStatusChanged)
}

func TestDiffStatusChangedToFrozen(t *testing.T) {
	prev := snap(t0, nil, nil, issue("a", "open"))
	cur := snap(t1, nil, nil, with(issue("a", "deferred")))
	evs := model.Diff(prev, cur, model.Statuses{})
	wantKinds(t, evs, model.KindStatusChanged)
	if evs[0].Detail != "open→deferred" {
		t.Errorf("detail = %q", evs[0].Detail)
	}
}

func TestDiffBlockedAndUnblocked(t *testing.T) {
	st := model.Statuses{}
	blocked := snap(t1, []string{"b"}, map[string][]string{"a": {"b"}}, issue("a", "open"), issue("b", "open"))
	free := snap(t0, []string{"a", "b"}, nil, issue("a", "open"), issue("b", "open"))
	wantKinds(t, model.Diff(free, blocked, st), model.KindBecameBlocked)
	wantKinds(t, model.Diff(blocked, free, st), model.KindUnblocked, model.KindBecameReady)

	rawBlocked := snap(t1, []string{"b"}, nil, with(issue("a", "blocked")), issue("b", "open"))
	wantKinds(t, model.Diff(free, rawBlocked, st), model.KindBecameBlocked)
	wantKinds(t, model.Diff(rawBlocked, free, st), model.KindUnblocked, model.KindBecameReady)
}

func TestDiffBecameReadyViaDerivedEffect(t *testing.T) {
	st := model.Statuses{}
	blocker := issue("blocker", "open")
	dependent := with(issue("dep", "open"), dep("blocker", "blocks"))
	before := snap(t0, []string{"blocker"}, map[string][]string{"dep": {"blocker"}}, blocker, dependent)
	closedBlocker := with(blocker, status("closed"))
	after := snap(t1, []string{"dep"}, nil, closedBlocker, dependent)
	evs := model.Diff(before, after, st)
	wantKinds(t, evs, model.KindClosed, model.KindUnblocked, model.KindBecameReady)
	for _, e := range evs {
		if e.IssueID == "dep" && e.Actor != "" {
			t.Errorf("derived event %v has actor %q", e.Kind, e.Actor)
		}
	}
}

func TestDiffClosedIssueGetsNoBlockedOrReadyEvents(t *testing.T) {
	prev := snap(t0, []string{"a"}, nil, issue("a", "open"))
	cur := snap(t1, nil, map[string][]string{"a": {"x"}}, issue("a", "closed"), issue("x", "open"))
	evs := model.Diff(prev, cur, model.Statuses{})
	wantKinds(t, evs, model.KindClosed, model.KindCreated)
}

func TestDiffPriorityChanged(t *testing.T) {
	evs := model.Diff(snap(t0, nil, nil, issue("a", "open")), snap(t1, nil, nil, with(issue("a", "open"), priority(1))), model.Statuses{})
	wantKinds(t, evs, model.KindPriorityChanged)
	if evs[0].Detail != "2→1" {
		t.Errorf("detail = %q", evs[0].Detail)
	}
}

func TestDiffEditedCoalescesFields(t *testing.T) {
	prev := snap(t0, nil, nil, with(issue("a", "open"), labels("x", "y")))
	cur := snap(t1, nil, nil, with(issue("a", "open"), title("new"), labels("y", "z"), func(is *model.Issue) {
		is.Description = "d"
		is.IssueType = "bug"
	}))
	evs := model.Diff(prev, cur, model.Statuses{})
	wantKinds(t, evs, model.KindEdited)
	if evs[0].Detail != "title, description, labels, type" || evs[0].Title != "new" {
		t.Errorf("event = %+v", evs[0])
	}

	reordered := snap(t1, nil, nil, with(issue("a", "open"), labels("y", "x")))
	wantKinds(t, model.Diff(prev, reordered, model.Statuses{}))
}

func TestDiffCommented(t *testing.T) {
	prev := snap(t0, nil, nil, with(issue("a", "open"), comments(1)))
	one := model.Diff(prev, snap(t1, nil, nil, with(issue("a", "open"), comments(2))), model.Statuses{})
	wantKinds(t, one, model.KindCommented)
	if one[0].Detail != "" {
		t.Errorf("detail = %q", one[0].Detail)
	}
	three := model.Diff(prev, snap(t1, nil, nil, with(issue("a", "open"), comments(4))), model.Statuses{})
	if three[0].Detail != "+3" {
		t.Errorf("detail = %q", three[0].Detail)
	}
	wantKinds(t, model.Diff(snap(t0, nil, nil, with(issue("a", "open"), comments(3))), snap(t1, nil, nil, issue("a", "open")), model.Statuses{}))
}

func TestDiffIgnoresEdgesAndFingerprintOnlyChanges(t *testing.T) {
	prev := snap(t0, []string{"a", "b"}, nil, issue("a", "open"), issue("b", "open"))
	cur := snap(t1, []string{"a", "b"}, nil, with(issue("a", "open"), dep("b", "related")), with(issue("b", "open"), func(is *model.Issue) {
		is.UpdatedAt = is.UpdatedAt.Add(time.Hour)
		is.Owner = "someone"
	}))
	if prev.Fingerprint() == cur.Fingerprint() {
		t.Fatal("test needs differing fingerprints")
	}
	if evs := model.Diff(prev, cur, model.Statuses{}); len(evs) != 0 {
		t.Errorf("events = %v", evs)
	}
}

func TestDiffOrdersByIssueID(t *testing.T) {
	prev := snap(t0, nil, nil, issue("b", "open"), issue("d", "open"))
	cur := snap(t1, nil, nil, with(issue("b", "open"), priority(1)), issue("a", "open"), with(issue("c", "open")))
	evs := model.Diff(prev, cur, model.Statuses{})
	var ids []string
	for _, e := range evs {
		ids = append(ids, e.IssueID+":"+string(e.Kind))
	}
	want := []string{"a:created", "b:priority", "c:created", "d:deleted"}
	if len(ids) != len(want) {
		t.Fatalf("events = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("events = %v, want %v", ids, want)
		}
	}
}
