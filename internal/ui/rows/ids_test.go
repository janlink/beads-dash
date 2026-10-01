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

func longIDs(t *testing.T, tier theme.Tier) (*rows.Renderer, []string) {
	t.Helper()
	at := uitest.T0
	var issues []model.Issue
	ids := []string{"beads-dash-www.31", "beads-dash-www.32"}
	for _, id := range ids {
		issues = append(issues, model.Issue{ID: id, Title: "Some title", Status: "open", IssueType: "task", Priority: 2, CreatedAt: at, UpdatedAt: at})
	}
	snap := model.NewSnapshot(issues, model.Readiness{Ready: ids}, at)
	r := rows.New(uitest.Look(theme.DepthNone, tier, true))
	r.Bind(snap, model.BuiltinStatuses())
	return r, ids
}

type idDraw func(id string) string

func idDraws(r *rows.Renderer, w int) map[string]idDraw {
	return map[string]idDraw{
		"standard": func(id string) string { return r.Line(w, rows.Row{ID: id}, "standard", r.Standard(id)) },
		"kanban":   func(id string) string { return r.Line(w, rows.Row{ID: id}, "kanban", r.Kanban(id)) },
		"ready": func(id string) string {
			return r.Line(w, rows.Row{ID: id}, "ready", r.Ready(rows.ReadyRow{ID: id, Cols: r.ReadyColumns(w), Now: uitest.T0}))
		},
		"tree": func(id string) string {
			row := model.TreeRow{Kind: model.TreeIssue, ID: id}
			r.SetTreeRows([]model.TreeRow{row})
			return r.Line(w, rows.Row{ID: id}, "tree", r.Tree(row))
		},
		"feed": func(id string) string {
			e := model.Event{IssueID: id, Time: uitest.T0.Add(-time.Minute), Kind: model.KindCreated}
			return r.Line(w, rows.Row{ID: id}, "feed", r.Feed(rows.FeedLine{Event: e, Now: uitest.T0}))
		},
		"outline": func(id string) string {
			all := []model.OutlineRow{{Kind: model.OutNode, ID: id}, {Kind: model.OutRef, ID: id}}
			return r.Outline(all, 0, w, rows.OutlineStyle{}) + r.Outline(all, 1, w, rows.OutlineStyle{})
		},
	}
}

func TestWideRowsShowFullIDs(t *testing.T) {
	r, ids := longIDs(t, theme.TierFancy)
	for name, fn := range idDraws(r, 200) {
		for _, id := range ids {
			got := ansi.Strip(fn(id))
			if !strings.Contains(got, id) || strings.Contains(got, "…") {
				t.Errorf("%s: %q lacks the full %s", name, got, id)
			}
		}
	}
}

func TestNarrowRowsCutIDsAndKeepTitle(t *testing.T) {
	r, ids := longIDs(t, theme.TierFancy)
	for name, fn := range idDraws(r, 60) {
		if name != "standard" {
			fn = idDraws(r, 44)[name]
		}
		if name == "feed" || name == "outline" {
			continue
		}
		for _, id := range ids {
			got := ansi.Strip(fn(id))
			if strings.Contains(got, id) || !strings.Contains(got, "…") || !strings.Contains(got, "www."+id[len(id)-2:]) {
				t.Errorf("%s: %q does not front-cut %s", name, got, id)
			}
			if name != "feed" && !strings.Contains(got, "Some title") {
				t.Errorf("%s: %q lost the title", name, got)
			}
		}
	}
}

func TestRowsWithoutFactsCutIDsWhenTitleRunsOut(t *testing.T) {
	r, ids := longIDs(t, theme.TierFancy)
	for _, name := range []string{"feed", "outline"} {
		got := ansi.Strip(idDraws(r, 36)[name](ids[1]))
		if strings.Contains(got, ids[1]) || !strings.Contains(got, "…") || !strings.Contains(got, "www.32") || !strings.Contains(got, "Some title") {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestLongIDsKeepTheirTail(t *testing.T) {
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierASCII} {
		r, ids := longIDs(t, tier)
		ellipsis := theme.GlyphsFor(tier).Ellipsis
		draw := map[string]idDraw{}
		for _, name := range []string{"standard", "kanban", "ready", "tree"} {
			draw[name] = idDraws(r, 44)[name]
		}
		for name, fn := range draw {
			for _, id := range ids {
				got := ansi.Strip(fn(id))
				tail := id[len(id)-5:]
				if !strings.Contains(got, ellipsis) || !strings.Contains(got, tail) {
					t.Errorf("%s %v %s: %q keeps neither the ellipsis nor %q", name, tier, id, got, tail)
				}
				if strings.Contains(got, "beads-dash-www") || strings.Contains(got, "www."+ellipsis) {
					t.Errorf("%s %v %s: the id is cut at the end: %q", name, tier, id, got)
				}
			}
			a, b := ansi.Strip(fn(ids[0])), ansi.Strip(fn(ids[1]))
			if !strings.Contains(a, "ww.31") || !strings.Contains(b, "ww.32") {
				t.Errorf("%s %v: siblings are not told apart: %q / %q", name, tier, a, b)
			}
		}
	}
}

func TestFeedLongIDKeepsTail(t *testing.T) {
	r, ids := longIDs(t, theme.TierFancy)
	e := model.Event{IssueID: ids[1], Time: uitest.T0.Add(-time.Minute), Kind: model.KindCreated}
	got := ansi.Strip(r.Line(34, rows.Row{}, "feed", r.Feed(rows.FeedLine{Event: e, Now: uitest.T0})))
	if !strings.Contains(got, "…") || !strings.Contains(got, "www.32") || strings.Contains(got, "www.…") {
		t.Errorf("feed row %q", got)
	}
}
