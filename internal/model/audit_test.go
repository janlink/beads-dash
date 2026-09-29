package model_test

import (
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
)

func TestChangeSummary(t *testing.T) {
	a := model.Issue{Status: "open", Priority: 2, Title: "t"}
	if got := model.ChangeSummary(&a, &a); len(got) != 0 {
		t.Fatalf("same issue: %v", got)
	}
	b := a
	b.Status, b.Priority, b.Assignee, b.Description = "closed", 1, "ann", "new"
	got := strings.Join(model.ChangeSummary(&a, &b), "; ")
	want := "status open → closed; priority P2 → P1; assignee none → ann; description edited"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
