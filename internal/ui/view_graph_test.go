package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/ui/state"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func graphApp(t testing.TB, f flavour, cols, rows int, docked bool) *App {
	t.Helper()
	snap, _ := uitest.Graph()
	return loaded(t, f, cols, rows, snap, withView("graph", docked))
}

func TestGraphGoldens(t *testing.T) {
	for _, s := range goldenSizes {
		t.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(t *testing.T) {
			testgolden.Equal(t, screen(graphApp(t, plain, s[0], s[1], false)))
		})
	}
	t.Run("truecolor", func(t *testing.T) {
		testgolden.Equal(t, graphApp(t, truecolor, 100, 30, false).View().Content)
	})
	t.Run("focus", func(t *testing.T) {
		a := graphApp(t, plain, 100, 30, false)
		a.sess.SetCurrent("gr-b")
		press(a, "enter")
		testgolden.Equal(t, screen(a))
	})
}

func TestGraphFoldFocusAndEsc(t *testing.T) {
	a := graphApp(t, plain, 100, 30, false)
	g := a.view().(*Graph)
	a.sess.SetCurrent("gr-a")
	if !strings.Contains(screen(a), "gr-d") {
		t.Fatal("d hidden before folding")
	}
	press(a, "h")
	if strings.Contains(screen(a), "gr-d") || !strings.Contains(screen(a), "gr-a") {
		t.Fatalf("h folds the node:\n%s", screen(a))
	}
	press(a, "l", "l")
	if a.sess.Current() != "gr-b" {
		t.Fatalf("l unfolds then enters, at %q", a.sess.Current())
	}
	press(a, "enter")
	if g.focusID() != "gr-b" {
		t.Fatalf("enter focuses, got %q", g.focusID())
	}
	out := screen(a)
	if !strings.Contains(out, "waits on") || !strings.Contains(out, "holds up") {
		t.Fatalf("focus outlines missing:\n%s", out)
	}
	press(a, "esc")
	if g.focusID() != "" {
		t.Fatal("esc leaves the focus")
	}
	press(a, "z", "M")
	if strings.Contains(screen(a), "gr-b") {
		t.Fatal("zM folds every node")
	}
}

func TestGraphIsolatedToggle(t *testing.T) {
	a := graphApp(t, plain, 120, 30, false)
	if strings.Contains(screen(a), "gr-lone1") || !strings.Contains(screen(a), "2 isolated hidden") {
		t.Fatalf("isolated issues are hidden and counted:\n%s", screen(a))
	}
	press(a, "i")
	if !strings.Contains(screen(a), "gr-lone1") {
		t.Fatal("i shows the isolated section")
	}
}

func TestGraphHorizontalScroll(t *testing.T) {
	a := graphApp(t, plain, 100, 30, false)
	g := a.view().(*Graph)
	press(a, "z", "l")
	if g.xoff != graphStep {
		t.Fatalf("zl scrolls by %d, got %d", graphStep, g.xoff)
	}
	press(a, "z", "h", "z", "h")
	if g.xoff != 0 {
		t.Fatalf("zh stops at 0, got %d", g.xoff)
	}
}

// graphRenderTime is the fastest of runs renders of a graph view over n
// issues.
func graphRenderTime(t *testing.T, n, runs int) time.Duration {
	t.Helper()
	snap := bigGraph(n)
	a := loaded(t, plain, 200, 50, snap, withView("graph", false))
	g := a.view().(*Graph)
	a.sess.SetCurrent(g.rows[len(g.rows)/2].ID)
	press(a, "j")
	a.View()
	best := time.Duration(1<<63 - 1)
	for range runs {
		start := time.Now()
		a.View()
		best = min(best, time.Since(start))
	}
	return best
}

func TestGraphRenderScalesLinearly(t *testing.T) {
	if raceEnabled || testing.Short() {
		t.Skip("timing ratios need an uninstrumented run")
	}
	small := graphRenderTime(t, 5000, 5)
	large := graphRenderTime(t, 20000, 5)
	ratio := float64(large) / float64(max(small, time.Microsecond))
	t.Logf("5k: %v, 20k: %v, ratio %.1f", small, large, ratio)
	if ratio >= 6 {
		t.Errorf("20k issues render %.1fx slower than 5k, want under 6x for 4x the data", ratio)
	}
}

func TestGraphRenderBudget(t *testing.T) {
	if os.Getenv("BDASH_PERF") != "1" || raceEnabled || testing.Short() {
		t.Skip("set BDASH_PERF=1 on an unloaded machine to check the wall-clock budget")
	}
	if per := graphRenderTime(t, 5000, 5); per > 8*time.Millisecond {
		t.Errorf("render takes %v, budget 8ms", per)
	}
}

// bigGraph is n issues in chains that end in cycles, with wide fans and
// shared blockers, so the outline holds long paths, references and cycle rows.
func bigGraph(n int) *model.Snapshot {
	issues := make([]model.Issue, n)
	for i := range issues {
		is := model.Issue{ID: fmt.Sprintf("bg-%05d", i), Title: fmt.Sprintf("Graph issue %d with a moderately long descriptive title", i), Status: "open", IssueType: "task", Priority: i % 5, UpdatedAt: uitest.T0}
		dep := func(to int) {
			is.Dependencies = append(is.Dependencies, model.Edge{From: is.ID, To: fmt.Sprintf("bg-%05d", to), Type: "blocks"})
		}
		switch {
		case i%500 == 0:
			dep((i + n - 1) % n)
		case i > 0:
			dep(i - 1)
			if i%7 == 0 && i > 20 {
				dep(i - 20)
			}
		}
		issues[i] = is
	}
	return model.NewSnapshot(issues, model.Readiness{}, uitest.T0)
}

func focusApp(t testing.TB, f flavour, cols, rows int, id string) *App {
	t.Helper()
	snap, _ := uitest.Graph()
	a := loaded(t, f, cols, rows, snap, withView("tree", true))
	a.sess.SetCurrent(id)
	press(a, "tab")
	return a
}

func TestFocusGraphGoldens(t *testing.T) {
	for _, s := range goldenSizes {
		t.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(t *testing.T) {
			testgolden.Equal(t, screen(focusApp(t, plain, s[0], s[1], "gr-b")))
		})
	}
	t.Run("truecolor", func(t *testing.T) {
		testgolden.Equal(t, focusApp(t, truecolor, 120, 40, "gr-b").View().Content)
	})
	t.Run("cycle", func(t *testing.T) {
		testgolden.Equal(t, screen(focusApp(t, plain, 120, 40, "gr-z")))
	})
}

func TestFocusGraphDepthKeysAndJump(t *testing.T) {
	a := focusApp(t, plain, 120, 40, "gr-w")
	has := func(id string) bool {
		_ = screen(a)
		return strings.Contains(ansi.Strip(strings.Join(a.panelLines(a.frame()), "\n")), id+" ")
	}
	if !has("gr-x") {
		t.Fatalf("the panel starts at depth 3:\n%s", screen(a))
	}
	press(a, "-")
	if !has("gr-y") || has("gr-x") {
		t.Fatalf("- shows one level less:\n%s", screen(a))
	}
	press(a, "-")
	if has("gr-y") {
		t.Fatalf("- shows fewer levels:\n%s", screen(a))
	}
	press(a, "+", "j", "enter")
	if a.sess.Current() == "gr-w" {
		t.Fatalf("enter on a graph row jumps to it, still at %q", a.sess.Current())
	}
}

func TestGraphBackRestoresPreviousFocus(t *testing.T) {
	a := graphApp(t, plain, 100, 30, false)
	g := a.view().(*Graph)
	a.sess.SetCurrent("gr-a")
	press(a, "enter")
	if g.focusID() != "gr-a" {
		t.Fatalf("focus %q", g.focusID())
	}
	press(a, "j", "enter")
	second := g.focusID()
	if second == "gr-a" || second == "" {
		t.Fatalf("second focus %q", second)
	}
	press(a, "backspace")
	if g.focusID() != "gr-a" {
		t.Fatalf("back restores the focus: %q", g.focusID())
	}
	press(a, "backspace")
	if g.focusID() != "" || a.sess.Current() != "gr-a" {
		t.Fatalf("back leaves the focus: %q at %q", g.focusID(), a.sess.Current())
	}
}

func TestGraphEscLeavesFocusOnlyAfterMarks(t *testing.T) {
	a := graphApp(t, plain, 100, 30, false)
	g := a.view().(*Graph)
	a.sess.SetCurrent("gr-a")
	press(a, "enter", "space")
	if len(a.sess.MarkedIDs()) == 0 {
		t.Fatal("space marks")
	}
	press(a, "esc")
	if len(a.sess.MarkedIDs()) != 0 || g.focusID() != "gr-a" {
		t.Fatalf("first esc clears marks, focus %q", g.focusID())
	}
	press(a, "esc")
	if g.focusID() != "" {
		t.Fatal("second esc leaves the focus")
	}
}

func TestGraphOpenOnFocusedIssueShowsDetail(t *testing.T) {
	a := graphApp(t, plain, 100, 30, false)
	g := a.view().(*Graph)
	a.sess.SetCurrent("gr-b")
	press(a, "enter")
	if r := g.rows[g.byID["gr-b"]]; r.Kind != model.OutSelf {
		t.Fatalf("focused issue maps to %v", r.Kind)
	}
	press(a, "enter")
	if g.focusID() != "gr-b" || !a.sess.Has(state.LayerDetail) {
		t.Fatalf("enter on the focused issue opens the detail, focus %q", g.focusID())
	}
}
