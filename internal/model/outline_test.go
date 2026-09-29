package model_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

func graphSnap(deps map[string][]string, parents map[string]string, ids ...string) *model.Snapshot {
	var issues []model.Issue
	for _, id := range ids {
		is := model.Issue{ID: id, Title: id, Status: "open", Parent: parents[id]}
		for _, to := range deps[id] {
			is.Dependencies = append(is.Dependencies, model.Edge{From: id, To: to, Type: "blocks"})
		}
		issues = append(issues, is)
	}
	return model.NewSnapshot(issues, model.Readiness{}, time.Time{})
}

func shape(rows []model.OutlineRow) string {
	var b strings.Builder
	for _, r := range rows {
		mark := [...]string{"H", "I", "N", "R", "C", "M", "P", "W", "O", "S", "K", "k"}[r.Kind]
		fmt.Fprintf(&b, "%s%s:%s", strings.Repeat(" ", r.Depth), mark, r.ID)
		if r.Kind == model.OutHeader || r.Kind == model.OutIsolated || r.Kind == model.OutMore {
			fmt.Fprintf(&b, "%d", r.Count)
		}
		if r.Dim {
			b.WriteString("~")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestOutlineDFSFirstSighting(t *testing.T) {
	// a blocks b and c; b blocks d; c blocks d.
	snap := graphSnap(map[string][]string{"b": {"a"}, "c": {"a"}, "d": {"b", "c"}}, nil, "a", "b", "c", "d")
	got := shape(model.BuildOutline(snap, false).Rows)
	want := "H:4\nN:a\n N:b\n  N:d\n N:c\n  R:d\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestOutlineComponentsBiggestFirstAndIsolated(t *testing.T) {
	snap := graphSnap(map[string][]string{"y": {"x"}, "c": {"b"}, "d": {"c"}}, nil, "b", "c", "d", "lone", "x", "y")
	o := model.BuildOutline(snap, false)
	if got, want := shape(o.Rows), "H:3\nN:b\n N:c\n  N:d\nH:2\nN:x\n N:y\n"; got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if o.Groups != 2 || len(o.Isolated) != 1 || o.Isolated[0] != "lone" {
		t.Fatalf("groups %d isolated %v", o.Groups, o.Isolated)
	}
	if got := shape(model.BuildOutline(snap, true).Rows); !strings.HasSuffix(got, "I:1\nN:lone\n") {
		t.Fatalf("isolated section missing:\n%s", got)
	}
}

func TestOutlineEntryChildrenOnly(t *testing.T) {
	// e has children k1 (free) and k2 (blocked by k1). Only k1 hangs under e.
	snap := graphSnap(map[string][]string{"k2": {"k1"}}, map[string]string{"k1": "e", "k2": "e"}, "e", "k1", "k2")
	got := shape(model.BuildOutline(snap, false).Rows)
	want := "H:3\nN:e\n N:k1~\n  N:k2\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestOutlineCycle(t *testing.T) {
	// a -> b -> c -> a in blocks direction, plus d hanging off c.
	snap := graphSnap(map[string][]string{"b": {"a"}, "c": {"b"}, "a": {"c"}, "d": {"c"}}, nil, "a", "b", "c", "d")
	o := model.BuildOutline(snap, false)
	got := shape(o.Rows)
	want := "H:4\nN:a\n N:b\n  N:c\n   C:a\n   N:d\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	member := map[string]bool{}
	for _, r := range o.Rows {
		if r.Kind == model.OutNode && r.Member {
			member[r.ID] = true
		}
	}
	if !member["a"] || !member["b"] || !member["c"] || member["d"] {
		t.Fatalf("members %v", member)
	}
}

func TestOutlineEndAndParent(t *testing.T) {
	snap := graphSnap(map[string][]string{"b": {"a"}, "c": {"b"}, "d": {"a"}}, nil, "a", "b", "c", "d")
	rows := model.BuildOutline(snap, false).Rows
	// rows: H a b c d
	if rows[1].End != 5 || rows[2].End != 4 || rows[3].End != 4 || rows[4].End != 5 {
		t.Fatalf("ends %v", rows)
	}
	if rows[3].Parent != 2 || rows[4].Parent != 1 || rows[1].Parent != -1 || !rows[4].Last || rows[2].Last {
		t.Fatalf("parents %v", rows)
	}
}

func TestBuildFocus(t *testing.T) {
	// x waits on w1,w2; w1 waits on w0; y and z wait on x; q waits on y.
	snap := graphSnap(map[string][]string{"x": {"w1", "w2"}, "w1": {"w0"}, "y": {"x"}, "z": {"x"}, "q": {"y"}},
		map[string]string{"x": "p"}, "p", "w0", "w1", "w2", "x", "y", "z", "q")
	rows := model.BuildFocus(snap, model.BuiltinStatuses(), "x", model.FocusOptions{Depth: 2})
	got := shape(rows)
	want := "P:p\nW:\n N:w1\n  N:w0\n N:w2\nS:x\nO:\n N:y\n  N:q\n N:z\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if rows[1].Count != 2 || rows[1].Aux != 2 {
		t.Fatalf("waits header %+v", rows[1])
	}
	rows = model.BuildFocus(snap, model.BuiltinStatuses(), "x", model.FocusOptions{Depth: 1})
	if got := shape(rows); !strings.Contains(got, " M:2\n") && !strings.Contains(got, " M:1\n") {
		t.Fatalf("no cut row:\n%s", got)
	}
	if model.BuildFocus(snap, model.BuiltinStatuses(), "nope", model.FocusOptions{Depth: 2}) != nil {
		t.Fatal("unknown id yields rows")
	}
}

func TestBuildFocusCycleAndFan(t *testing.T) {
	deps := map[string][]string{"a": {"b"}, "b": {"a"}}
	ids := []string{"a", "b", "hub"}
	for i := range 30 {
		id := fmt.Sprintf("s%02d", i)
		deps[id] = []string{"hub"}
		ids = append(ids, id)
	}
	snap := graphSnap(deps, nil, ids...)
	rows := model.BuildFocus(snap, model.BuiltinStatuses(), "a", model.FocusOptions{Depth: 3})
	if got := shape(rows); !strings.Contains(got, "C:a") {
		t.Fatalf("no cycle row:\n%s", got)
	}
	rows = model.BuildFocus(snap, model.BuiltinStatuses(), "hub", model.FocusOptions{Depth: 3})
	last := rows[len(rows)-1]
	if last.Kind != model.OutMore || last.Count != 10 {
		t.Fatalf("fan cap: %+v", last)
	}
}

func TestBuildFocusCutShowsCycleAndMembers(t *testing.T) {
	// c waits on b waits on a waits on c; d waits on c without being on the cycle.
	snap := graphSnap(map[string][]string{"c": {"b"}, "b": {"a"}, "a": {"c"}, "d": {"c"}}, nil, "a", "b", "c", "d")
	rows := model.BuildFocus(snap, model.BuiltinStatuses(), "c", model.FocusOptions{Depth: 2})
	var cut *model.OutlineRow
	for i := range rows {
		if rows[i].Kind == model.OutCycle {
			cut = &rows[i]
		}
	}
	if cut == nil || cut.ID != "c" || !cut.Member {
		t.Fatalf("depth cut should end in a cycle row back to c:\n%s", shape(rows))
	}
	for _, r := range rows {
		if r.Kind == model.OutNode && r.ID == "d" && r.Member {
			t.Fatal("d is not on the cycle")
		}
		if r.Kind == model.OutNode && r.ID == "b" && !r.Member {
			t.Fatal("b is on the cycle")
		}
	}
}
