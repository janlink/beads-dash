package model_test

import (
	"reflect"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
)

func readyFixture() *model.Snapshot {
	who := func(a string) func(*model.Issue) { return func(is *model.Issue) { is.Assignee = a } }
	typ := func(t string) func(*model.Issue) { return func(is *model.Issue) { is.IssueType = t } }
	return model.NewSnapshot([]model.Issue{
		issue("late", "open", prio(2), created(9)),
		issue("early", "open", prio(2), created(1)),
		issue("urgent", "open", prio(0), created(8), who("alice")),
		issue("mine", "open", prio(1), created(2), who("bob")),
		issue("wip", "in_progress", prio(0), who("bob")),
		issue("done", "closed"),
		issue("frozen", "deferred"),
		issue("epic", "open", prio(0), typ("epic")),
		issue("epic.1", "open", parent("epic"), prio(3)),
		issue("wait-b", "open", prio(3), created(2)),
		issue("wait-a", "open", prio(1), created(3)),
		issue("blocker", "open", prio(4)),
		issue("wait-epic", "open", typ("epic")),
		issue("wait-epic.1", "open", parent("wait-epic"), prio(4)),
		issue("wait-wip", "in_progress"),
	}, model.Readiness{
		Ready: []string{"late", "early", "urgent", "mine", "wip", "done", "frozen", "epic", "gone"},
		Blocked: map[string][]string{
			"wait-b": {"blocker", "zzz"}, "wait-a": {"zzz", "blocker"}, "wait-epic": {"blocker"},
			"wait-wip": {"blocker"}, "done": {"blocker"},
		},
	}, t0)
}

func readyOf(query string) model.ReadyList {
	snap := readyFixture()
	st := model.BuiltinStatuses()
	sc := model.ParseScope(query, false)
	return model.BuildReady(snap, st, sc, sc.Apply(snap, st))
}

func TestReadyGroupsAndSorts(t *testing.T) {
	r := readyOf("")
	if want := []string{"early", "late"}; !reflect.DeepEqual(r.Unassigned, want) {
		t.Errorf("Unassigned = %v, want %v", r.Unassigned, want)
	}
	if want := []string{"urgent", "mine"}; !reflect.DeepEqual(r.Assigned, want) {
		t.Errorf("Assigned = %v, want %v", r.Assigned, want)
	}
	want := []model.BlockedRow{{ID: "wait-a", First: "zzz"}, {ID: "wait-b", First: "blocker"}}
	if !reflect.DeepEqual(r.Blocked, want) {
		t.Errorf("Blocked = %v, want %v", r.Blocked, want)
	}
	if r.Len() != 6 {
		t.Errorf("Len = %d", r.Len())
	}
}

func TestReadyExcludesContainersAndCountsThem(t *testing.T) {
	r := readyOf("")
	if r.Containers != 2 {
		t.Errorf("Containers = %d, want 2 (one ready, one blocked)", r.Containers)
	}
	for _, id := range append(append([]string{}, r.Unassigned...), r.Assigned...) {
		if id == "epic" {
			t.Error("epic is a container")
		}
	}
}

func TestReadyTypeFacetBringsContainersBack(t *testing.T) {
	r := readyOf("type:epic")
	if want := []string{"epic"}; !reflect.DeepEqual(r.Unassigned, want) || len(r.Assigned) != 0 {
		t.Errorf("type:epic: %v %v", r.Unassigned, r.Assigned)
	}
	if len(r.Blocked) != 1 || r.Blocked[0].ID != "wait-epic" || r.Containers != 0 {
		t.Errorf("blocked %v containers %d", r.Blocked, r.Containers)
	}
}

func TestReadyFacetsApplyButStatusVisibilityDoesNot(t *testing.T) {
	r := readyOf("assignee:bob")
	if want := []string{"mine"}; !reflect.DeepEqual(r.Assigned, want) || len(r.Unassigned) != 0 || len(r.Blocked) != 0 {
		t.Errorf("assignee:bob: %+v", r)
	}
	if r := readyOf("status:frozen"); r.Len() != 0 {
		t.Errorf("frozen issues never appear: %+v", r)
	}
	// closed hidden by default must not remove ready issues
	if r := readyOf(""); len(r.Unassigned) == 0 {
		t.Error("default scope emptied the list")
	}
}

func TestReadyEmptyVerdict(t *testing.T) {
	snap := model.NewSnapshot([]model.Issue{issue("a", "open")}, model.Readiness{}, t0)
	st := model.BuiltinStatuses()
	sc := model.ParseScope("", false)
	if r := model.BuildReady(snap, st, sc, sc.Apply(snap, st)); r.Len() != 0 || r.Containers != 0 {
		t.Errorf("%+v", r)
	}
}

func TestBlockedRowNamesTheBlockerBdListsFirst(t *testing.T) {
	snap := model.NewSnapshot(
		[]model.Issue{{ID: "a", Status: "open"}, {ID: "b", Status: "open"}},
		model.Readiness{Blocked: map[string][]string{"a": {"external:proj:cap", "b"}}, Reason: map[string]string{"b": "no blocking dependencies"}},
		t0,
	)
	if got := snap.FirstBlocker("a"); got != "external:proj:cap" {
		t.Errorf("first blocker %q, want bd's first", got)
	}
	if got := snap.BlockedBy("a"); got[0] != "b" {
		t.Errorf("the sorted set stays sorted: %v", got)
	}
	l := model.BuildReady(snap, model.BuiltinStatuses(), model.ParseScope("", false), model.ParseScope("", false).Apply(snap, model.BuiltinStatuses()))
	if len(l.Blocked) != 1 || l.Blocked[0].First != "external:proj:cap" {
		t.Errorf("blocked rows %+v", l.Blocked)
	}
	if snap.ReadyReason("b") != "" {
		t.Error("a reason for an issue bd did not list as ready is dropped")
	}
}
