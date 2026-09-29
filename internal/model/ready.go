package model

import "sort"

// BlockedRow is a blocked issue with the first blocker bd names.
type BlockedRow struct {
	ID    string
	First string
}

// ReadyList is the Ready view's content: bd's verdict, sorted and grouped.
type ReadyList struct {
	// Unassigned and Assigned hold the ready issues by assignee.
	Unassigned []string
	Assigned   []string
	Blocked    []BlockedRow
	// Containers counts the ready or blocked issues left out because they
	// have children.
	Containers int
}

// Len is the number of issues listed, blocked ones included.
func (r ReadyList) Len() int { return len(r.Unassigned) + len(r.Assigned) + len(r.Blocked) }

// BuildReady groups bd's ready and blocked verdict. Only the scope's facets
// apply, not its status visibility. Issues that are in progress, closed or
// frozen never appear, and containers stay out unless the scope names their
// type.
func BuildReady(snap *Snapshot, st Statuses, sc Scope, m *Matches) ReadyList {
	var out ReadyList
	keep := func(id string) bool {
		if !m.Facets(id) {
			return false
		}
		switch snap.Present(id, st).Status {
		case InProgress, Closed, Frozen:
			return false
		case Other, Open, Blocked:
		}
		is, _ := snap.Issue(id)
		if snap.IsContainer(id) && !sc.NamesType(is.IssueType) {
			out.Containers++
			return false
		}
		return true
	}
	for _, id := range snap.ReadyIDs() {
		if !keep(id) {
			continue
		}
		if is, _ := snap.Issue(id); is.Assignee == "" {
			out.Unassigned = append(out.Unassigned, id)
		} else {
			out.Assigned = append(out.Assigned, id)
		}
	}
	var blocked []string
	for _, id := range snap.BlockedIDs() {
		if keep(id) {
			blocked = append(blocked, id)
		}
	}
	byPriority := func(ids []string) {
		sort.SliceStable(ids, func(i, j int) bool {
			a, _ := snap.Issue(ids[i])
			b, _ := snap.Issue(ids[j])
			return siblingLess(a, b, false, false)
		})
	}
	byPriority(out.Unassigned)
	byPriority(out.Assigned)
	byPriority(blocked)
	for _, id := range blocked {
		out.Blocked = append(out.Blocked, BlockedRow{ID: id, First: snap.FirstBlocker(id)})
	}
	return out
}
