package model

import "fmt"

// ChangeSummary lists what differs between two versions of an issue, one short
// phrase per changed field. It is empty when nothing bdash shows changed.
func ChangeSummary(prev, next *Issue) []string {
	var out []string
	pair := func(name, a, b string) {
		if a != b {
			out = append(out, fmt.Sprintf("%s %s → %s", name, shown(a), shown(b)))
		}
	}
	pair("status", prev.Status, next.Status)
	if prev.Priority != next.Priority {
		out = append(out, fmt.Sprintf("priority P%d → P%d", prev.Priority, next.Priority))
	}
	pair("type", prev.IssueType, next.IssueType)
	pair("assignee", prev.Assignee, next.Assignee)
	pair("owner", prev.Owner, next.Owner)
	pair("ref", prev.ExternalRef, next.ExternalRef)
	edited := func(name, a, b string) {
		if a != b {
			out = append(out, name+" edited")
		}
	}
	edited("title", prev.Title, next.Title)
	edited("description", prev.Description, next.Description)
	edited("design", prev.Design, next.Design)
	edited("acceptance", prev.AcceptanceCriteria, next.AcceptanceCriteria)
	edited("notes", prev.Notes, next.Notes)
	if !prev.DueAt.Equal(next.DueAt) {
		out = append(out, "due date changed")
	}
	if prev.EstimatedMinutes != next.EstimatedMinutes {
		out = append(out, "estimate changed")
	}
	return out
}

func shown(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
