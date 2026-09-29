package model

import "sort"

// KanbanColumn is one column of the board: the issues of one presentation
// status in card order.
type KanbanColumn struct {
	Status PresentationStatus
	IDs    []string
}

// KanbanOrder is the column order: the flow left to right, Closed last so it
// is the first to drop off screen.
var KanbanOrder = [6]PresentationStatus{Open, InProgress, Blocked, Frozen, Other, Closed}

// BuildKanban sorts the issues the scope shows into columns by presentation
// status; cards follow the sibling order. A scope with a status facet keeps
// only the columns that have cards; otherwise the Closed column is left out
// while the status visibility hides closed issues and none is shown.
func BuildKanban(snap *Snapshot, st Statuses, sc Scope, m *Matches) []KanbanColumn {
	var by [6][]string
	for _, id := range m.IDs() {
		p := snap.Present(id, st).Status
		by[p] = append(by[p], id)
	}
	out := make([]KanbanColumn, 0, len(KanbanOrder))
	for _, p := range KanbanOrder {
		ids := by[p]
		if len(ids) == 0 && (sc.NamesStatus() || p == Closed && !sc.ShowClosed()) {
			continue
		}
		sort.SliceStable(ids, func(i, j int) bool {
			a, _ := snap.Issue(ids[i])
			b, _ := snap.Issue(ids[j])
			return siblingLess(a, b, false, false)
		})
		out = append(out, KanbanColumn{Status: p, IDs: ids})
	}
	return out
}
