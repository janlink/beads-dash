package model_test

import (
	"reflect"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
)

func kanbanOf(query string, showClosed bool) []model.KanbanColumn {
	snap := readyFixture()
	st := model.BuiltinStatuses()
	sc := model.ParseScope(query, showClosed)
	return model.BuildKanban(snap, st, sc, sc.Apply(snap, st))
}

func TestKanbanColumnOrderAndCards(t *testing.T) {
	cols := kanbanOf("", true)
	var order []model.PresentationStatus
	for _, c := range cols {
		order = append(order, c.Status)
	}
	want := []model.PresentationStatus{model.Open, model.InProgress, model.Blocked, model.Frozen, model.Other, model.Closed}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("columns = %v, want %v", order, want)
	}
	if got, w := cols[1].IDs, []string{"wip", "wait-wip"}; !reflect.DeepEqual(got, w) {
		t.Errorf("in progress = %v, want %v", got, w)
	}
	if got, w := cols[2].IDs, []string{"wait-a", "wait-epic", "wait-b"}; !reflect.DeepEqual(got, w) {
		t.Errorf("blocked = %v, want %v", got, w)
	}
}

func TestKanbanHidesClosedColumn(t *testing.T) {
	for _, c := range kanbanOf("", false) {
		if c.Status == model.Closed {
			t.Fatal("closed column shown while closed issues are hidden")
		}
	}
	cols := kanbanOf("status:closed", false)
	if last := cols[len(cols)-1]; last.Status != model.Closed || len(last.IDs) != 1 {
		t.Errorf("status:closed keeps the closed column: %+v", last)
	}
}

func TestKanbanFollowsScope(t *testing.T) {
	for _, c := range kanbanOf("assignee:bob", false) {
		for _, id := range c.IDs {
			if id != "mine" && id != "wip" {
				t.Errorf("column %v holds %q outside the scope", c.Status, id)
			}
		}
	}
}

func TestKanbanStatusFacetKeepsOnlyMatchingColumns(t *testing.T) {
	for _, q := range []string{"status:open", "-status:open", "status:open status:blocked"} {
		for _, c := range kanbanOf(q, false) {
			if len(c.IDs) == 0 {
				t.Errorf("%q: empty column %v stays", q, c.Status)
			}
		}
	}
	for _, c := range kanbanOf("status:open", false) {
		if c.Status == model.Other || c.Status == model.Frozen {
			t.Errorf("status:open keeps the %v column", c.Status)
		}
	}
}
