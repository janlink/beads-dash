package ui

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func TestKanbanGoldens(t *testing.T) {
	sizes := append([][2]int{{79, 24}, {199, 50}}, goldenSizes...)
	for _, s := range sizes {
		t.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(t *testing.T) {
			testgolden.Equal(t, screen(viewApp(t, plain, s[0], s[1], "kanban", false)))
		})
	}
	t.Run("truecolor", func(t *testing.T) {
		testgolden.Equal(t, viewApp(t, truecolor, 120, 40, "kanban", false).View().Content)
	})
}

func kanbanColumnOf(k *Kanban, id string) int {
	for i, c := range k.cols {
		if slices.Contains(c.IDs, id) {
			return i
		}
	}
	return -1
}

func TestKanbanColumnsStepWithHAndL(t *testing.T) {
	a := viewApp(t, plain, 200, 50, "kanban", false)
	_ = screen(a)
	k := a.view().(*Kanban)
	start := kanbanColumnOf(k, a.sess.Current())
	if start < 0 {
		t.Fatalf("current %q is on no column", a.sess.Current())
	}
	press(a, "l")
	_ = screen(a)
	if got := kanbanColumnOf(k, a.sess.Current()); got <= start {
		t.Errorf("l moved from column %d to %d", start, got)
	}
	press(a, "h")
	_ = screen(a)
	if got := kanbanColumnOf(k, a.sess.Current()); got != start {
		t.Errorf("h returned to column %d, want %d", got, start)
	}
}

func TestKanbanClickSelectsCard(t *testing.T) {
	a := viewApp(t, plain, 200, 50, "kanban", false)
	_ = screen(a)
	k := a.view().(*Kanban)
	if len(k.hits) == 0 {
		t.Fatal("no card was drawn")
	}
	h := k.hits[len(k.hits)-1]
	id, ok := k.AtXY(h.x0, h.y)
	if !ok || id != h.id {
		t.Errorf("AtXY(%d,%d) = %q, %v; want %q", h.x0, h.y, id, ok, h.id)
	}
}

func TestKanbanViewStaysWithinBudgetWithFiveThousandIssues(t *testing.T) {
	if raceEnabled || testing.Short() {
		t.Skip("timing is only meaningful without the race detector")
	}
	snap, _ := uitest.Deep(5000)
	a := loaded(t, truecolor, 200, 50, snap, withView("kanban", false))
	_ = a.View()
	const runs = 200
	var worst, total time.Duration
	for i := range runs {
		key := "j"
		if i%40 >= 30 {
			key = "k"
		}
		start := time.Now()
		press(a, key)
		_ = a.View()
		d := time.Since(start)
		total += d
		worst = max(worst, d)
	}
	if per := total / runs; per > 8*time.Millisecond {
		t.Errorf("a step took %v on average, budget 8ms (worst %v)", per, worst)
	}
	t.Logf("average %v, worst %v", total/runs, worst)
}
