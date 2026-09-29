// Package uitest holds fixtures shared by the UI packages' tests.
package uitest

import (
	"fmt"
	"strings"
	"time"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// T0 is the fixed "now" of golden tests.
var T0 = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)

// Look builds a look for a colour depth and glyph tier on the default theme.
func Look(depth theme.Depth, tier theme.Tier, dark bool) look.Look {
	return look.New(theme.NewPalette(theme.Default(), dark, depth), theme.GlyphsFor(tier))
}

// Sample is a small workspace covering every presentation status.
func Sample() (*model.Snapshot, model.Statuses) {
	at := func(d time.Duration) time.Time { return T0.Add(-d) }
	issues := []model.Issue{
		{ID: "ws-4k2", Title: "Checkout: guest orders", Status: "in_progress", IssueType: "epic", Priority: 1, Assignee: "alice", CreatedAt: at(72 * time.Hour), UpdatedAt: at(time.Hour)},
		{ID: "ws-4k2.1", Title: "Address form validation", Status: "open", IssueType: "task", Priority: 1, Parent: "ws-4k2", CreatedAt: at(72 * time.Hour), UpdatedAt: at(2 * time.Hour)},
		{ID: "ws-4k2.2", Title: "Guest order confirmation mail", Status: "open", IssueType: "task", Priority: 2, Parent: "ws-4k2", CreatedAt: at(48 * time.Hour), UpdatedAt: at(3 * time.Hour)},
		{ID: "ws-9qe", Title: "Payment provider timeout retries", Status: "open", IssueType: "bug", Priority: 0, CreatedAt: at(5 * time.Hour), UpdatedAt: at(5 * time.Hour)},
		{ID: "ws-7mt", Title: "Cart badge shows stale count", Status: "in_progress", IssueType: "bug", Priority: 2, Assignee: "bob", CreatedAt: at(24 * time.Hour), UpdatedAt: at(4 * time.Hour)},
		{ID: "ws-2hz", Title: "Split pricing service", Status: "open", IssueType: "feature", Priority: 2, CreatedAt: at(144 * time.Hour), UpdatedAt: at(6 * time.Hour)},
		{ID: "ws-8np", Title: "Search: typo tolerance", Status: "deferred", IssueType: "feature", Priority: 3, CreatedAt: at(288 * time.Hour), UpdatedAt: at(12 * time.Hour)},
		{ID: "ws-5ca", Title: "Update analytics consent banner", Status: "closed", IssueType: "chore", Priority: 3, CreatedAt: at(96 * time.Hour), UpdatedAt: at(8 * time.Hour), ClosedAt: at(8 * time.Hour)},
		{ID: "ws-1zz", Title: "Mystery status issue", Status: "weird", IssueType: "task", Priority: 4, CreatedAt: at(96 * time.Hour), UpdatedAt: at(8 * time.Hour)},
	}
	ready := model.Readiness{
		Ready:   []string{"ws-4k2.1", "ws-9qe", "ws-7mt", "ws-5ca"},
		Blocked: map[string][]string{"ws-4k2.2": {"ws-4k2.1"}, "ws-2hz": {"ws-9qe"}},
		Reason: map[string]string{
			"ws-4k2.1": "no blocking dependencies", "ws-4k2.5": "no blocking dependencies", "ws-4k2.5.1": "no blocking dependencies",
			"ws-9qe": "no blocking dependencies", "ws-7mt": "no blocking dependencies", "ws-4k2": "no blocking dependencies",
			"ws-lost": "1 blocker(s) resolved",
		},
	}
	return model.NewSnapshot(issues, ready, T0), model.BuiltinStatuses()
}

// Big builds a snapshot of n issues for benchmarks.
func Big(n int) (*model.Snapshot, model.Statuses) {
	statuses := []string{"open", "in_progress", "closed", "deferred", "blocked"}
	issues := make([]model.Issue, n)
	for i := range issues {
		issues[i] = model.Issue{
			ID:        fmt.Sprintf("big-%05d", i),
			Title:     fmt.Sprintf("Issue number %d with a moderately long descriptive title", i),
			Status:    statuses[i%len(statuses)],
			IssueType: "task",
			Priority:  i % 5,
			Assignee:  [...]string{"", "alice", "bob"}[i%3],
			CreatedAt: T0.Add(-time.Duration(i) * time.Minute),
			UpdatedAt: T0.Add(-time.Duration(i) * time.Second),
		}
	}
	return model.NewSnapshot(issues, model.Readiness{}, T0), model.BuiltinStatuses()
}

// Deep builds a hierarchical snapshot of about n issues for budgets: epics
// with children and grandchildren, readiness with reasons and blockers, and
// markdown descriptions, one of about 5 KB on every fifth issue.
func Deep(n int) (*model.Snapshot, model.Statuses) {
	para := "Retries **fail** after the provider times out; see `client.go` and [the notes](https://example.com/notes).\n\n- back off between tries\n- add jitter\n\n> keep the budget in mind\n\n"
	long := strings.Repeat(para, 5000/len(para)+1)
	var issues []model.Issue
	var ready model.Readiness
	ready.Blocked, ready.Reason = map[string][]string{}, map[string]string{}
	add := func(id, parent string, i int) {
		is := model.Issue{
			ID: id, Title: fmt.Sprintf("Issue %s with a moderately long descriptive title", id), Parent: parent,
			Status: [...]string{"open", "in_progress", "closed", "open", "deferred"}[i%5], IssueType: "task", Priority: i % 5,
			Assignee:  [...]string{"", "alice", "bob"}[i%3],
			CreatedAt: T0.Add(-time.Duration(i) * time.Minute), UpdatedAt: T0.Add(-time.Duration(i) * time.Second),
		}
		switch {
		case i%5 == 0:
			is.Description = long
		case i%3 == 0:
			is.Description = para
		}
		if is.Status == "closed" {
			is.ClosedAt = is.UpdatedAt
		}
		issues = append(issues, is)
		switch {
		case is.Status == "closed" || is.Status == "deferred":
		case i%7 == 0 && len(issues) > 1:
			ready.Blocked[id] = []string{issues[len(issues)-2].ID}
		default:
			ready.Ready = append(ready.Ready, id)
			ready.Reason[id] = "no blocking dependencies"
		}
	}
	for e := 0; len(issues) < n; e++ {
		epic := fmt.Sprintf("deep-%04d", e)
		add(epic, "", len(issues))
		for c := 0; c < 8 && len(issues) < n; c++ {
			child := fmt.Sprintf("%s.%d", epic, c)
			add(child, epic, len(issues))
			for g := 0; g < 4 && len(issues) < n; g++ {
				add(fmt.Sprintf("%s.%d", child, g), child, len(issues))
			}
		}
	}
	return model.NewSnapshot(issues, ready, T0), model.BuiltinStatuses()
}

// Tree is a workspace for the tree and ready views: an epic with open,
// blocked and closed children and a deeper level, a fully closed epic, an
// orphan, a blocked chain and a description to draw.
func Tree() (*model.Snapshot, model.Statuses) {
	at := func(d time.Duration) time.Time { return T0.Add(-d) }
	h := time.Hour
	issues := []model.Issue{
		{ID: "ws-4k2", Title: "Checkout: guest orders", Status: "in_progress", IssueType: "epic", Priority: 1, Assignee: "alice", CreatedAt: at(72 * h), UpdatedAt: at(h)},
		{ID: "ws-4k2.1", Title: "Address form validation", Status: "open", IssueType: "task", Priority: 1, Parent: "ws-4k2", CreatedAt: at(72 * h), UpdatedAt: at(2 * h)},
		{ID: "ws-4k2.2", Title: "Guest order confirmation mail", Status: "open", IssueType: "task", Priority: 2, Parent: "ws-4k2", CreatedAt: at(48 * h), UpdatedAt: at(3 * h), Dependencies: []model.Edge{{From: "ws-4k2.2", To: "ws-4k2.1", Type: "blocks"}}},
		{ID: "ws-4k2.3", Title: "Order summary page", Status: "closed", IssueType: "task", Priority: 2, Parent: "ws-4k2", CreatedAt: at(80 * h), UpdatedAt: at(9 * h), ClosedAt: at(9 * h)},
		{ID: "ws-4k2.4", Title: "Guest cart merge", Status: "closed", IssueType: "task", Priority: 2, Parent: "ws-4k2", CreatedAt: at(81 * h), UpdatedAt: at(10 * h), ClosedAt: at(10 * h)},
		{ID: "ws-4k2.5", Title: "Shipping options", Status: "open", IssueType: "feature", Priority: 3, Parent: "ws-4k2", CreatedAt: at(60 * h), UpdatedAt: at(4 * h)},
		{ID: "ws-4k2.5.1", Title: "Pickup point lookup", Status: "open", IssueType: "task", Priority: 3, Assignee: "carol", Parent: "ws-4k2.5", CreatedAt: at(59 * h), UpdatedAt: at(4 * h)},
		{
			ID: "ws-9qe", Title: "Payment provider timeout retries", Status: "open", IssueType: "bug", Priority: 0, CreatedAt: at(5 * h), UpdatedAt: at(5 * h),
			Description: "Retries **fail** after the provider times out.\n\n- back off between tries\n- add jitter\n",
		},
		{ID: "ws-7mt", Title: "Cart badge shows stale count", Status: "in_progress", IssueType: "bug", Priority: 2, Assignee: "bob", CreatedAt: at(24 * h), UpdatedAt: at(4 * h)},
		{ID: "ws-2hz", Title: "Split pricing service", Status: "open", IssueType: "feature", Priority: 2, CreatedAt: at(144 * h), UpdatedAt: at(6 * h), Dependencies: []model.Edge{{From: "ws-2hz", To: "ws-9qe", Type: "blocks"}}},
		{ID: "ws-8np", Title: "Search: typo tolerance", Status: "deferred", IssueType: "feature", Priority: 3, CreatedAt: at(288 * h), UpdatedAt: at(12 * h)},
		{ID: "ws-5ca", Title: "Update analytics consent banner", Status: "closed", IssueType: "chore", Priority: 3, CreatedAt: at(96 * h), UpdatedAt: at(8 * h), ClosedAt: at(8 * h)},
		{ID: "ws-old", Title: "Legacy checkout removal", Status: "closed", IssueType: "epic", Priority: 2, CreatedAt: at(400 * h), UpdatedAt: at(300 * h), ClosedAt: at(300 * h)},
		{ID: "ws-old.1", Title: "Delete legacy routes", Status: "closed", IssueType: "task", Priority: 2, Parent: "ws-old", CreatedAt: at(399 * h), UpdatedAt: at(300 * h), ClosedAt: at(300 * h)},
		{ID: "ws-lost", Title: "Follow-up from a deleted epic", Status: "open", IssueType: "task", Priority: 2, Assignee: "alice", Parent: "ws-gone", CreatedAt: at(30 * h), UpdatedAt: at(30 * h)},
	}
	ready := model.Readiness{
		Ready:   []string{"ws-4k2.1", "ws-4k2.5", "ws-4k2.5.1", "ws-9qe", "ws-7mt", "ws-lost", "ws-4k2"},
		Blocked: map[string][]string{"ws-4k2.2": {"ws-4k2.1"}, "ws-2hz": {"ws-9qe"}},
		Reason: map[string]string{
			"ws-4k2.1": "no blocking dependencies", "ws-4k2.5": "no blocking dependencies", "ws-4k2.5.1": "no blocking dependencies",
			"ws-9qe": "no blocking dependencies", "ws-7mt": "no blocking dependencies", "ws-4k2": "no blocking dependencies",
			"ws-lost": "1 blocker(s) resolved",
		},
	}
	return model.NewSnapshot(issues, ready, T0), model.BuiltinStatuses()
}

// Graph is a workspace for the dependency graph: a diamond, a chain, an epic
// whose entry child holds back a sibling, a dependency cycle, a closed
// blocker, issues without edges and a long title.
func Graph() (*model.Snapshot, model.Statuses) {
	h := time.Hour
	mk := func(id, title, status string, parent string, blocks ...string) model.Issue {
		is := model.Issue{ID: id, Title: title, Status: status, IssueType: "task", Priority: 2, Parent: parent, CreatedAt: T0.Add(-48 * h), UpdatedAt: T0.Add(-h)}
		if status == "closed" {
			is.ClosedAt = is.UpdatedAt
		}
		for _, to := range blocks {
			is.Dependencies = append(is.Dependencies, model.Edge{From: id, To: to, Type: "blocks"})
		}
		return is
	}
	issues := []model.Issue{
		mk("gr-a", "Extract pricing client", "in_progress", ""),
		mk("gr-b", "Migrate cart totals", "open", "", "gr-a"),
		mk("gr-c", "Migrate invoice totals with a rather long title that needs cutting somewhere", "open", "", "gr-a"),
		mk("gr-d", "Remove legacy pricing", "open", "", "gr-b", "gr-c"),
		mk("gr-e", "Checkout epic", "open", ""),
		mk("gr-e1", "Address form", "open", "gr-e"),
		mk("gr-e2", "Confirmation mail", "open", "gr-e", "gr-e1"),
		mk("gr-x", "Cache warmup", "open", "", "gr-z"),
		mk("gr-y", "Cache eviction", "open", "", "gr-x"),
		mk("gr-z", "Cache sizing", "open", "", "gr-y"),
		mk("gr-w", "Cache metrics", "open", "", "gr-z"),
		mk("gr-k", "Closed prerequisite", "closed", ""),
		mk("gr-l", "Depends on closed", "open", "", "gr-k"),
		mk("gr-lone1", "Standalone chore", "open", ""),
		mk("gr-lone2", "Another standalone chore", "deferred", ""),
	}
	return model.NewSnapshot(issues, model.Readiness{}, T0), model.BuiltinStatuses()
}
