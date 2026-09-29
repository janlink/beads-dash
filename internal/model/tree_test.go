package model_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

func prio(p int) func(*model.Issue) { return func(is *model.Issue) { is.Priority = p } }

func created(h int) func(*model.Issue) {
	return func(is *model.Issue) { is.CreatedAt = t0.Add(time.Duration(h) * time.Hour) }
}

// dump renders rows as "id" indented by depth; fold rows read "+N", context
// rows are in parentheses, orphans carry ^parent and folded parents a ~.
func dump(rows []model.TreeRow) string {
	var out []string
	for _, r := range rows {
		s := strings.Repeat(" ", r.Depth)
		switch {
		case r.Kind == model.TreeClosedFold:
			s += fmt.Sprintf("+%d", r.Closed)
			if r.Folded {
				s += "~"
			}
		case r.Context:
			s += "(" + r.ID + ")"
		default:
			s += r.ID
		}
		if r.Orphan != "" {
			s += " ^" + r.Orphan
		}
		if r.Kind == model.TreeIssue && r.Folded {
			s += " ~"
		}
		out = append(out, s)
	}
	return strings.Join(out, "\n")
}

func build(t *testing.T, issues []model.Issue, query string, showClosed bool, folds *model.Folds) []model.TreeRow {
	t.Helper()
	snap := model.NewSnapshot(issues, model.Readiness{}, t0)
	st := model.BuiltinStatuses()
	return model.BuildTree(snap, st, model.ParseScope(query, showClosed).Apply(snap, st), folds)
}

func wantTree(t *testing.T, rows []model.TreeRow, want ...string) {
	t.Helper()
	if got, w := dump(rows), strings.Join(want, "\n"); got != w {
		t.Errorf("tree:\n%s\nwant:\n%s", got, w)
	}
}

func TestTreeSiblingOrder(t *testing.T) {
	rows := build(t, []model.Issue{
		issue("late", "open", prio(2), created(5)),
		issue("early", "open", prio(2), created(1)),
		issue("urgent", "open", prio(0), created(9)),
		issue("done", "closed", prio(0), created(0)),
		issue("tie-b", "open", prio(3), created(2)),
		issue("tie-a", "open", prio(3), created(2)),
	}, "", true, nil)
	wantTree(t, rows, "urgent", "early", "late", "tie-a", "tie-b", "done")
}

func TestTreeNestsChildrenAndFlagsGuides(t *testing.T) {
	rows := build(t, []model.Issue{
		issue("e", "open"),
		issue("e.1", "open", parent("e"), prio(1)),
		issue("e.1.1", "open", parent("e.1")),
		issue("e.2", "open", parent("e"), prio(2)),
		issue("z", "open", prio(4)),
	}, "", false, nil)
	wantTree(t, rows, "e", " e.1", "  e.1.1", " e.2", "z")
	last := map[string]bool{}
	more := map[string]uint64{}
	for _, r := range rows {
		last[r.ID], more[r.ID] = r.Last, r.More
	}
	if last["e"] || !last["z"] || last["e.1"] || !last["e.2"] || !last["e.1.1"] {
		t.Errorf("Last flags: %v", last)
	}
	if more["e.1.1"] != 0b11 || more["e.1"] != 0b1 || more["e.2"] != 0b1 || more["z"] != 0 {
		t.Errorf("More masks: %v", more)
	}
	if !rows[0].Foldable || rows[0].Folded || rows[4].Foldable {
		t.Errorf("foldable: %+v", rows)
	}
}

func TestTreeOrphansBecomeRootsWithMarker(t *testing.T) {
	rows := build(t, []model.Issue{
		issue("a", "open"),
		issue("kid", "open", parent("gone")),
		issue("edge", "open", dep("gone2", model.EdgeParentChild)),
	}, "", false, nil)
	wantTree(t, rows, "a", "edge ^gone2", "kid ^gone")
}

func TestTreeParentCycleStillShowsEveryIssue(t *testing.T) {
	rows := build(t, []model.Issue{
		issue("a", "open", parent("b")), issue("b", "open", parent("a")), issue("s", "open", parent("s")),
	}, "", false, nil)
	if len(rows) != 3 {
		t.Errorf("rows:\n%s", dump(rows))
	}
}

func TestTreeStartsWithFullyClosedSubtreesFolded(t *testing.T) {
	issues := []model.Issue{
		issue("done", "closed"),
		issue("done.1", "closed", parent("done")),
		issue("done.1.1", "closed", parent("done.1")),
		issue("live", "open", prio(1)),
		issue("live.1", "closed", parent("live")),
		issue("mixed", "closed", prio(1)),
		issue("mixed.1", "open", parent("mixed")),
	}
	rows := build(t, issues, "", true, nil)
	wantTree(t, rows, "live", " live.1", "mixed", " mixed.1", "done ~")

	var f model.Folds
	f.Set("done", false)
	f.Set("mixed", true)
	rows = build(t, issues, "", true, &f)
	wantTree(t, rows, "live", " live.1", "mixed ~", "done", " done.1 ~")
}

func TestTreeFoldAllAndUnfoldAll(t *testing.T) {
	issues := []model.Issue{
		issue("e", "open"), issue("e.1", "open", parent("e")), issue("e.1.1", "open", parent("e.1")),
		issue("f", "open", prio(3)), issue("f.1", "closed", parent("f")),
	}
	snap := model.NewSnapshot(issues, model.Readiness{}, t0)
	st := model.BuiltinStatuses()
	m := model.ParseScope("", false).Apply(snap, st)
	var f model.Folds
	f.SetAll(snap, true)
	wantTree(t, model.BuildTree(snap, st, m, &f), "e ~", "f ~")
	f.SetAll(snap, false)
	wantTree(t, model.BuildTree(snap, st, m, &f), "e", " e.1", "  e.1.1", "f", " +1", "  f.1")
}

func TestTreeFoldsPrune(t *testing.T) {
	var f model.Folds
	f.Set("keep", true)
	f.Set("gone", true)
	f.Set(model.ClosedFoldKey("gone"), true)
	f.Set(model.ClosedFoldKey("keep"), false)
	f.Prune(func(id string) bool { return id == "keep" })
	if v, ok := f.Get("keep"); !ok || !v {
		t.Error("keep was pruned")
	}
	if _, ok := f.Get("gone"); ok {
		t.Error("gone stayed")
	}
	if _, ok := f.Get(model.ClosedFoldKey("gone")); ok {
		t.Error("closed fold of gone stayed")
	}
	if v, ok := f.Get(model.ClosedFoldKey("keep")); !ok || v {
		t.Error("closed fold of keep lost")
	}
}

func TestTreeClosedChildrenFoldIntoOneRow(t *testing.T) {
	issues := []model.Issue{
		issue("e", "open"),
		issue("e.1", "open", parent("e"), prio(1)),
		issue("e.2", "closed", parent("e")),
		issue("e.3", "closed", parent("e"), prio(3)),
		issue("e.3.1", "closed", parent("e.3")),
	}
	wantTree(t, build(t, issues, "", false, nil), "e", " e.1", " +2~")

	var f model.Folds
	f.Set(model.ClosedFoldKey("e"), false)
	wantTree(t, build(t, issues, "", false, &f), "e", " e.1", " +2", "  e.2", "  e.3 ~")

	wantTree(t, build(t, issues, "", true, nil), "e", " e.1", " e.2", " e.3 ~")
	wantTree(t, build(t, issues, "status:closed status:open", false, nil), "e", " e.1", " e.2", " e.3 ~")
}

func TestTreeScopeKeepsAncestorsDimmed(t *testing.T) {
	issues := []model.Issue{
		issue("e", "open", func(is *model.Issue) { is.Title = "Epic" }),
		issue("e.1", "open", parent("e"), func(is *model.Issue) { is.Title = "Needle" }),
		issue("e.1.1", "open", parent("e.1"), func(is *model.Issue) { is.Title = "Leaf" }),
		issue("e.2", "open", parent("e"), prio(3)),
		issue("other", "open", prio(3)),
	}
	wantTree(t, build(t, issues, "leaf", false, nil), "(e)", " (e.1)", "  e.1.1")
	wantTree(t, build(t, issues, "needle", false, nil), "(e)", " e.1")
}

func TestTreeScopeFoldsOnlyClosedChildrenThatMatch(t *testing.T) {
	issues := []model.Issue{
		issue("e", "open", func(is *model.Issue) { is.Labels = []string{"ui"} }),
		issue("e.1", "closed", parent("e"), func(is *model.Issue) { is.Labels = []string{"ui"} }),
		issue("e.2", "closed", parent("e")),
	}
	wantTree(t, build(t, issues, "label:ui", false, nil), "e", " +1~")
}

func TestTreeHiddenClosedContextWithOpenDescendantIsNotFolded(t *testing.T) {
	issues := []model.Issue{
		issue("e", "open"),
		issue("e.1", "closed", parent("e")),
		issue("e.1.1", "open", parent("e.1")),
	}
	wantTree(t, build(t, issues, "", false, nil), "e", " (e.1)", "  e.1.1")
}

func TestTreeNilScopeShowsAll(t *testing.T) {
	snap := model.NewSnapshot([]model.Issue{issue("a", "closed"), issue("b", "open")}, model.Readiness{}, t0)
	wantTree(t, model.BuildTree(snap, model.BuiltinStatuses(), nil, nil), "b", "a")
}

func TestTreeParentsAndSortedChildren(t *testing.T) {
	st := model.BuiltinStatuses()
	snap := model.NewSnapshot([]model.Issue{
		issue("e", "open", prio(1)),
		issue("e.1", "open", parent("e"), prio(2), created(1)),
		issue("e.2", "open", parent("e"), prio(1), created(2)),
		issue("e.3", "closed", parent("e"), created(0)),
		issue("e.4", "closed", parent("e"), created(-1)),
	}, model.Readiness{}, t0)
	p := model.TreeParents(snap, st)
	if p["e.1"] != "e" || p["e"] != "" || len(p) != 4 {
		t.Errorf("parents = %v", p)
	}
	live, closed := model.SortedChildren(snap, st, "e")
	if strings.Join(live, " ") != "e.2 e.1" || strings.Join(closed, " ") != "e.4 e.3" {
		t.Errorf("live %v closed %v", live, closed)
	}
}
