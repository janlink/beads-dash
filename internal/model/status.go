package model

import "sort"

// Category is the class bd assigns to a status.
type Category string

const (
	CategoryActive Category = "active"
	CategoryWIP    Category = "wip"
	CategoryDone   Category = "done"
	CategoryFrozen Category = "frozen"
	// CategoryUnspecified is a custom status configured without a category.
	CategoryUnspecified Category = "unspecified"
	// CategoryUnknown is a status bd does not list, such as an orphaned one.
	CategoryUnknown Category = ""
)

// ParseCategory maps bd's category string, treating anything unrecognised as
// [CategoryUnknown].
func ParseCategory(s string) Category {
	switch c := Category(s); c {
	case CategoryActive, CategoryWIP, CategoryDone, CategoryFrozen, CategoryUnspecified:
		return c
	case CategoryUnknown:
	}
	return CategoryUnknown
}

// StatusInfo describes one status from bd statuses.
type StatusInfo struct {
	Name        string
	Category    Category
	Icon        string
	Description string
	Custom      bool
}

// Statuses is the status table of a workspace. The zero value holds bd's
// built-in statuses.
type Statuses struct {
	byName map[string]StatusInfo
	order  []string
}

const statusBlocked = "blocked"

var builtinStatuses = []StatusInfo{
	{Name: "open", Category: CategoryActive, Icon: "○", Description: "Available to work (default)"},
	{Name: "in_progress", Category: CategoryWIP, Icon: "◐", Description: "Actively being worked on"},
	{Name: statusBlocked, Category: CategoryWIP, Icon: "●", Description: "Blocked by a dependency"},
	{Name: "deferred", Category: CategoryFrozen, Icon: "❄", Description: "Deliberately put on ice for later"},
	{Name: "closed", Category: CategoryDone, Icon: "✓", Description: "Completed"},
	{Name: "pinned", Category: CategoryFrozen, Icon: "📌", Description: "Persistent, stays open indefinitely"},
	{Name: "hooked", Category: CategoryWIP, Icon: "◇", Description: "Attached to an agent's hook"},
}

// NewStatuses builds a table from bd's built-in and custom statuses, in the
// order given. A later entry replaces an earlier one of the same name.
func NewStatuses(infos []StatusInfo) Statuses {
	s := Statuses{byName: make(map[string]StatusInfo, len(infos))}
	for _, in := range infos {
		if _, dup := s.byName[in.Name]; !dup {
			s.order = append(s.order, in.Name)
		}
		s.byName[in.Name] = in
	}
	return s
}

// BuiltinStatuses is the table bd ships without custom statuses.
func BuiltinStatuses() Statuses { return NewStatuses(builtinStatuses) }

func (s Statuses) table() Statuses {
	if s.byName == nil {
		return BuiltinStatuses()
	}
	return s
}

// Lookup returns the info for a raw status name.
func (s Statuses) Lookup(name string) (StatusInfo, bool) {
	in, ok := s.table().byName[name]
	return in, ok
}

// Category returns the category of a raw status, [CategoryUnknown] when bd
// does not list it.
func (s Statuses) Category(name string) Category {
	in, _ := s.Lookup(name)
	return in.Category
}

// All lists the statuses in table order.
func (s Statuses) All() []StatusInfo {
	t := s.table()
	out := make([]StatusInfo, 0, len(t.order))
	for _, n := range t.order {
		out = append(out, t.byName[n])
	}
	return out
}

// Names lists the status names sorted alphabetically.
func (s Statuses) Names() []string {
	t := s.table()
	out := append([]string(nil), t.order...)
	sort.Strings(out)
	return out
}

// PresentationStatus is the status bdash shows.
type PresentationStatus uint8

const (
	Other PresentationStatus = iota
	Open
	InProgress
	Blocked
	Frozen
	Closed
)

func (p PresentationStatus) String() string {
	switch p {
	case Open:
		return "Open"
	case InProgress:
		return "In progress"
	case Blocked:
		return "Blocked"
	case Frozen:
		return "Frozen"
	case Closed:
		return "Closed"
	case Other:
	}
	return "Other"
}

// Presentation is a presentation status plus the marker for an in-progress
// issue that bd also calls blocked.
type Presentation struct {
	Status        PresentationStatus
	BlockedMarker bool
}

// Present maps a raw status and bd's blocked verdict to what bdash shows.
// Custom statuses follow their category; unlisted ones are Other.
func Present(rawStatus string, blocked bool, st Statuses) Presentation {
	if rawStatus == statusBlocked {
		return Presentation{Status: Blocked}
	}
	switch st.Category(rawStatus) {
	case CategoryActive:
		if blocked {
			return Presentation{Status: Blocked}
		}
		return Presentation{Status: Open}
	case CategoryWIP:
		return Presentation{Status: InProgress, BlockedMarker: blocked}
	case CategoryFrozen:
		return Presentation{Status: Frozen}
	case CategoryDone:
		return Presentation{Status: Closed}
	case CategoryUnspecified, CategoryUnknown:
	}
	return Presentation{Status: Other}
}
