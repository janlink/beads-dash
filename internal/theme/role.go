// Package theme holds bdash's role tokens, themes, colour depth, glyph tiers
// and the Glamour styles derived from them. Views draw with roles only.
package theme

// Role names one semantic colour slot. Views never use raw colours.
type Role int

const (
	StatusOpen Role = iota
	StatusInProgress
	StatusBlocked
	StatusFrozen
	StatusClosed
	StatusOther

	Priority0
	Priority1
	Priority2
	Priority3
	Priority4

	TypeBug
	TypeFeature
	TypeTask
	TypeEpic
	TypeChore
	TypeDecision
	TypeOther

	Strong
	Text
	Dim
	Faint
	Rule

	Primary
	Border
	Surface
	Success
	Error
	Warning
	Selection
	Changed
	Match

	roleCount
)

var roleNames = [roleCount]string{
	StatusOpen:       "status.open",
	StatusInProgress: "status.in_progress",
	StatusBlocked:    "status.blocked",
	StatusFrozen:     "status.frozen",
	StatusClosed:     "status.closed",
	StatusOther:      "status.other",
	Priority0:        "priority.0",
	Priority1:        "priority.1",
	Priority2:        "priority.2",
	Priority3:        "priority.3",
	Priority4:        "priority.4",
	TypeBug:          "type.bug",
	TypeFeature:      "type.feature",
	TypeTask:         "type.task",
	TypeEpic:         "type.epic",
	TypeChore:        "type.chore",
	TypeDecision:     "type.decision",
	TypeOther:        "type.other",
	Strong:           "strong",
	Text:             "text",
	Dim:              "dim",
	Faint:            "faint",
	Rule:             "rule",
	Primary:          "primary",
	Border:           "border",
	Surface:          "surface",
	Success:          "success",
	Error:            "error",
	Warning:          "warning",
	Selection:        "selection",
	Changed:          "changed",
	Match:            "match",
}

func (r Role) String() string {
	if r < 0 || r >= roleCount {
		return "unknown"
	}
	return roleNames[r]
}

// Roles returns every role in declaration order.
func Roles() []Role {
	out := make([]Role, roleCount)
	for i := range out {
		out[i] = Role(i)
	}
	return out
}

// Ladder lists the text ladder from the most to the least prominent rung.
var Ladder = []Role{Strong, Text, Dim, Faint, Rule}

// StatusRole maps a presentation status index (Open, In progress, Blocked,
// Frozen, Closed, Other) to its role.
func StatusRole(i int) Role { return StatusOpen + Role(clamp(i, 0, 5)) }

// GutterRole is the role of the bar at a row's left edge: the issue's
// priority, or the rule grey once it is closed, so colour is spent on the
// status glyph alone there.
func GutterRole(priority int, closed bool) Role {
	if closed {
		return Rule
	}
	return PriorityRole(priority)
}

// PriorityRole maps a priority 0..4 to its role.
func PriorityRole(p int) Role { return Priority0 + Role(clamp(p, 0, 4)) }

// TypeRole maps a bd issue type to its role; unknown types are TypeOther.
func TypeRole(t string) Role {
	switch t {
	case "bug":
		return TypeBug
	case "feature":
		return TypeFeature
	case "task":
		return TypeTask
	case "epic":
		return TypeEpic
	case "chore":
		return TypeChore
	case "decision":
		return TypeDecision
	default:
		return TypeOther
	}
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }
