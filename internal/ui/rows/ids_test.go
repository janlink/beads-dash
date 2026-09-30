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

func TestLongIDsKeepTheirTail(t *testing.T) {
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierASCII} {
		r, ids := longIDs(t, tier)
		ellipsis := theme.GlyphsFor(tier).Ellipsis
		draw := map[string]func(id string) string{
			"standard": func(id string) string { return r.Line(100, rows.Row{ID: id}, "v", r.Standard(id)) },
			"kanban":   func(id string) string { return r.Line(100, rows.Row{ID: id}, "v", r.Kanban(id)) },
			"ready": func(id string) string {
				return r.Line(140, rows.Row{ID: id}, "v", r.Ready(rows.ReadyRow{ID: id, Cols: r.ReadyColumns(140), Now: uitest.T0}))
			},
			"tree": func(id string) string {
				return r.Line(100, rows.Row{ID: id}, "v", r.Tree(model.TreeRow{Kind: model.TreeIssue, ID: id}))
			},
		}
		for name, fn := range draw {
			for _, id := range ids {
				got := ansi.Strip(fn(id))
				tail := id[strings.LastIndex(id, "www"):]
				if !strings.Contains(got, ellipsis) || !strings.Contains(got, tail) {
					t.Errorf("%s %v %s: %q keeps neither the ellipsis nor %q", name, tier, id, got, tail)
				}
				if strings.Contains(got, "beads-dash-www") || strings.Contains(got, "www."+ellipsis) {
					t.Errorf("%s %v %s: the id is cut at the end: %q", name, tier, id, got)
				}
			}
			a, b := ansi.Strip(fn(ids[0])), ansi.Strip(fn(ids[1]))
			if !strings.Contains(a, "www.31") || !strings.Contains(b, "www.32") {
				t.Errorf("%s %v: siblings are not told apart: %q / %q", name, tier, a, b)
			}
		}
	}
}

func TestFeedLongIDKeepsTail(t *testing.T) {
	r, ids := longIDs(t, theme.TierFancy)
	e := model.Event{IssueID: ids[1], Time: uitest.T0.Add(-time.Minute), Kind: model.KindCreated}
	got := ansi.Strip(r.Line(120, rows.Row{}, "feed", r.Feed(rows.FeedLine{Event: e, Now: uitest.T0})))
	if !strings.Contains(got, "…") || !strings.Contains(got, "www.32") || strings.Contains(got, "www.…") {
		t.Errorf("feed row %q", got)
	}
}
