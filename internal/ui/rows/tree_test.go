package rows_test

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func treeRenderer(tier theme.Tier) (*rows.Renderer, []model.TreeRow) {
	snap, st := uitest.Tree()
	r := rows.New(uitest.Look(theme.DepthNone, tier, true))
	r.Bind(snap, st)
	return r, model.BuildTree(snap, st, model.ParseScope("", false).Apply(snap, st), &model.Folds{})
}

func TestTreeRowsHaveExactWidth(t *testing.T) {
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierASCII} {
		r, tree := treeRenderer(tier)
		for _, w := range []int{30, 57, 60, 80, 120} {
			for _, row := range tree {
				for _, sel := range []bool{false, true} {
					if got := ansi.StringWidth(r.Tree(row)(w, sel)); got != w {
						t.Errorf("tier %v w %d %s: %d cells", tier, w, row.ID, got)
					}
				}
			}
		}
	}
}

func TestTreeRowFacts(t *testing.T) {
	r, tree := treeRenderer(theme.TierASCII)
	find := func(id string, kind model.TreeRowKind) model.TreeRow {
		for _, row := range tree {
			if row.ID == id && row.Kind == kind {
				return row
			}
		}
		t.Fatalf("no row %s", id)
		return model.TreeRow{}
	}
	text := func(row model.TreeRow, w int) string { return ansi.Strip(r.Tree(row)(w, false)) }

	if s := text(find("ws-4k2", model.TreeIssue), 100); !strings.Contains(s, "2/5") || !strings.Contains(s, "#") {
		t.Errorf("parent lacks progress: %q", s)
	}
	if s := text(find("ws-4k2", model.TreeIssue), 60); !strings.Contains(s, "2/5") || strings.Contains(s, "#") {
		t.Errorf("narrow parent keeps the number only: %q", s)
	}
	if s := text(find("ws-4k2.5.1", model.TreeIssue), 100); !strings.Contains(s, "P3 carol") {
		t.Errorf("leaf lacks priority and assignee: %q", s)
	}
	if s := text(find("ws-4k2.5.1", model.TreeIssue), 60); strings.Contains(s, "carol") || !strings.Contains(s, "P3") {
		t.Errorf("narrow leaf drops the assignee: %q", s)
	}
	if s := text(find("ws-lost", model.TreeIssue), 100); !strings.Contains(s, "^ ws-gone") {
		t.Errorf("orphan lacks its marker: %q", s)
	}
	if s := text(find("ws-4k2", model.TreeClosedFold), 100); !strings.Contains(s, "2 closed") {
		t.Errorf("closed row: %q", s)
	}
}

func TestDeepRowsCapTheirGuidesAndKeepTheTitle(t *testing.T) {
	snap, st := uitest.Tree()
	r := rows.New(uitest.Look(theme.DepthNone, theme.TierASCII, true))
	r.Bind(snap, st)
	row := model.TreeRow{ID: "ws-4k2.5.1", Depth: 30, Last: true, More: 1<<30 - 1}
	s := ansi.Strip(r.Tree(row)(60, false))
	if !strings.Contains(s, "…") && !strings.Contains(s, "...") {
		t.Errorf("no depth marker: %q", s)
	}
	if !strings.Contains(s, "Pickup point lookup") {
		t.Errorf("title lost: %q", s)
	}
}

func TestReadyColumnsDropInOrder(t *testing.T) {
	r, _ := treeRenderer(theme.TierASCII)
	for _, tc := range []struct {
		w    int
		want rows.ReadyCols
	}{
		{200, rows.ReadyCols{Type: true, Assignee: true, Age: true, Reason: true}},
		{120, rows.ReadyCols{Type: true, Assignee: true, Age: true, Reason: true}},
		{119, rows.ReadyCols{Type: true, Assignee: true, Age: true}},
		{99, rows.ReadyCols{Type: true, Assignee: true}},
		{89, rows.ReadyCols{Assignee: true}},
		{79, rows.ReadyCols{}},
		{60, rows.ReadyCols{}},
	} {
		if got := r.ReadyColumns(tc.w); got != tc.want {
			t.Errorf("w %d: %+v, want %+v", tc.w, got, tc.want)
		}
	}
}

func TestReadyRowsHaveExactWidthAndTheirColumns(t *testing.T) {
	r, _ := treeRenderer(theme.TierASCII)
	now := uitest.T0
	for _, w := range []int{60, 80, 100, 120, 200} {
		cols := r.ReadyColumns(w)
		for _, pinned := range []bool{false, true} {
			s := r.Ready(rows.ReadyRow{ID: "ws-lost", Cols: cols, Now: now, Reason: "waits on ws-1", Pinned: pinned})(w-rows.GutterWidth, false)
			if got := ansi.StringWidth(s); got != w-rows.GutterWidth {
				t.Errorf("w %d: %d cells", w, got)
			}
		}
	}
	s := ansi.Strip(r.Ready(rows.ReadyRow{ID: "ws-lost", Cols: r.ReadyColumns(200), Now: now, Reason: "no blockers"})(197, false))
	for _, want := range []string{"ws-lost", "P2", "task", "alice", "1d", "no blockers"} {
		if !strings.Contains(s, want) {
			t.Errorf("row lacks %q: %q", want, s)
		}
	}
}

func TestAge(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "now"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{2 * 24 * time.Hour, "2d"},
		{15 * 24 * time.Hour, "2w"},
		{100 * 24 * time.Hour, "3mo"},
		{800 * 24 * time.Hour, "2y"},
	} {
		if got := rows.Age(now, now.Add(-tc.ago)); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.ago, got, tc.want)
		}
	}
	if rows.Age(now, time.Time{}) != "" || rows.Age(now, now.Add(time.Hour)) != "now" {
		t.Error("zero and future times")
	}
}
