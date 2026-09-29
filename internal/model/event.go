package model

import (
	"slices"
	"sort"
	"time"
)

// Kind is the class of an activity event. The values are the names used in
// the notify.kinds setting.
type Kind string

const (
	KindCreated         Kind = "created"
	KindClaimed         Kind = "claimed"
	KindUnassigned      Kind = "unassigned"
	KindClosed          Kind = "closed"
	KindReopened        Kind = "reopened"
	KindStatusChanged   Kind = "status"
	KindBecameBlocked   Kind = "blocked"
	KindUnblocked       Kind = "unblocked"
	KindBecameReady     Kind = "ready"
	KindPriorityChanged Kind = "priority"
	KindDeleted         Kind = "deleted"
	KindEdited          Kind = "edited"
	KindCommented       Kind = "commented"
)

var kinds = []Kind{
	KindCreated, KindClaimed, KindUnassigned, KindClosed, KindReopened, KindStatusChanged,
	KindBecameBlocked, KindUnblocked, KindBecameReady, KindPriorityChanged, KindDeleted,
	KindEdited, KindCommented,
}

// Kinds lists every activity-event kind in the order the feed and the
// notification dialog show them.
func Kinds() []Kind { return slices.Clone(kinds) }

// KindNames lists the kinds' setting names in [Kinds] order.
func KindNames() []string {
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}

// ParseKind maps a setting name to its kind.
func ParseKind(s string) (Kind, bool) {
	k := Kind(s)
	return k, slices.Contains(kinds, k)
}

// Label is the wording the feed shows for the kind.
func (k Kind) Label() string {
	switch k {
	case KindCreated:
		return "created"
	case KindClaimed:
		return "claimed"
	case KindUnassigned:
		return "unassigned"
	case KindClosed:
		return "closed"
	case KindReopened:
		return "reopened"
	case KindStatusChanged:
		return "status changed"
	case KindBecameBlocked:
		return "became blocked"
	case KindUnblocked:
		return "unblocked"
	case KindBecameReady:
		return "became ready"
	case KindPriorityChanged:
		return "priority changed"
	case KindDeleted:
		return "deleted"
	case KindEdited:
		return "edited"
	case KindCommented:
		return "commented"
	}
	return string(k)
}

// KindSet is a set of kinds, such as the ones notifications are enabled for.
type KindSet map[Kind]struct{}

// KindSetOf builds a set from setting names; unknown names are dropped.
func KindSetOf(names ...string) KindSet {
	s := KindSet{}
	for _, n := range names {
		if k, ok := ParseKind(n); ok {
			s[k] = struct{}{}
		}
	}
	return s
}

// Has reports whether k is in the set.
func (s KindSet) Has(k Kind) bool {
	_, ok := s[k]
	return ok
}

// Event is one change bdash noticed: the issue, when, and who did it when
// known.
type Event struct {
	Kind    Kind
	IssueID string
	// Title is the issue's title at the time of the event.
	Title string
	Time  time.Time
	// Actor is empty when unknown: in polling mode everything except a
	// created event's creator, and journal-less derived effects.
	Actor string
	// Detail is short kind-specific text such as "2→1" for a priority change,
	// or the edited fields.
	Detail string
	// Prefill marks events rebuilt at startup rather than noticed live. They
	// never highlight or notify.
	Prefill bool
	// Own marks the effect of a write bdash made itself on the written issue.
	Own bool
	// Seq is the journal sequence of the record that supplied the actor, 0
	// when none did.
	Seq int64
}

// RingCapacity is how many events the activity feed keeps.
const RingCapacity = 500

// Ring keeps the newest [RingCapacity] events in chronological order. It is
// not safe for concurrent use.
type Ring struct {
	events []Event
}

// Len is the number of events held.
func (r *Ring) Len() int { return len(r.events) }

// Add appends live events, newer than everything held, dropping the oldest
// beyond the capacity.
func (r *Ring) Add(evs ...Event) {
	r.events = append(r.events, evs...)
	r.trim()
}

// Merge inserts events that may be older than some already held, such as
// prefill, by time; equal times keep the held ones first.
func (r *Ring) Merge(evs ...Event) {
	if len(evs) == 0 {
		return
	}
	r.events = append(r.events, evs...)
	sort.SliceStable(r.events, func(i, j int) bool { return r.events[i].Time.Before(r.events[j].Time) })
	r.trim()
}

func (r *Ring) trim() {
	if extra := len(r.events) - RingCapacity; extra > 0 {
		r.events = slices.Clone(r.events[extra:])
	}
}

// Newest returns the events newest first, as a copy.
func (r *Ring) Newest() []Event {
	out := make([]Event, len(r.events))
	for i, e := range r.events {
		out[len(out)-1-i] = e
	}
	return out
}
