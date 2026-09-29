package model

import (
	"encoding/json"
	"time"
)

// EdgeParentChild is the dependency type that makes From a child of To.
const EdgeParentChild = "parent-child"

// Edge is one dependency record: From depends on To. Type is bd's open
// string (blocks, parent-child, related, ...). Edge timestamps are never
// kept because bd labels local wall time as UTC.
type Edge struct {
	From string
	To   string
	Type string
}

// Issue is one issue as bd lists it. Status and IssueType are raw strings;
// custom and orphaned values pass through untouched. Raw is bd's own JSON
// for the issue, kept for export.
type Issue struct {
	ID                 string
	Title              string
	Description        string
	Design             string
	AcceptanceCriteria string
	Notes              string
	Status             string
	IssueType          string
	Priority           int
	Assignee           string
	Owner              string
	CreatedBy          string
	Parent             string
	Labels             []string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	StartedAt          time.Time
	ClosedAt           time.Time
	DueAt              time.Time
	CloseReason        string
	ExternalRef        string
	EstimatedMinutes   int
	CommentCount       int
	// Dependencies are the outgoing edges bd lists on this issue, parent-child
	// included.
	Dependencies []Edge
	Raw          json.RawMessage
}

// Readiness is bd's own verdict from ready --explain: the ready IDs with the
// reason bd gives for each and, per blocked ID, the IDs that block it in bd's
// order. bdash never derives any of it.
type Readiness struct {
	Ready   []string
	Reason  map[string]string
	Blocked map[string][]string
}

// BlockingEdge reports whether a dependency type holds its source back until
// the target is done.
func BlockingEdge(edgeType string) bool {
	switch edgeType {
	case "blocks", "conditional-blocks", "waits-for":
		return true
	}
	return false
}
