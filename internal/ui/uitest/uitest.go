// Package uitest holds fixtures shared by the UI packages' tests.
package uitest

import (
	"fmt"
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
