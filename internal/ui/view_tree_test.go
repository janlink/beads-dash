package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/ui/state"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

var goldenSizes = [][2]int{{60, 16}, {60, 24}, {80, 24}, {120, 40}, {200, 50}}

// withView starts the app on one of the issue views; docked turns the panel
// setting on.
func withView(view string, docked bool) func(*Options) {
	return func(o *Options) {
		o.Views = IssueViews()
		o.Settings.Settings.View = view
		o.Settings.Settings.DetailDocked = docked
	}
}

func viewApp(t testing.TB, f flavour, cols, rows int, view string, docked bool) *App {
	t.Helper()
	snap, _ := uitest.Tree()
	return loaded(t, f, cols, rows, snap, withView(view, docked))
}

func TestTreeGoldens(t *testing.T) {
	for _, s := range goldenSizes {
		t.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(t *testing.T) {
			testgolden.Equal(t, screen(viewApp(t, plain, s[0], s[1], "tree", false)))
		})
	}
	t.Run("truecolor", func(t *testing.T) {
		testgolden.Equal(t, viewApp(t, truecolor, 100, 30, "tree", false).View().Content)
	})
}

func TestTreeFoldingAndCursor(t *testing.T) {
	a := viewApp(t, plain, 100, 30, "tree", false)
	tree := a.view().(*Tree)
	a.sess.SetCurrent("ws-4k2")
	row := func() model.TreeRow { return tree.rows[tree.cursorIndex(a.env())] }

	press(a, "h")
	if a.sess.Current() != "ws-4k2" || !row().Folded {
		t.Fatalf("h on an open parent folds it: current %q folded %v", a.sess.Current(), row().Folded)
	}
	press(a, "l")
	if row().Folded {
		t.Fatal("l on a folded parent unfolds it")
	}
	press(a, "l")
	if a.sess.Current() != "ws-4k2.1" {
		t.Errorf("l on an open parent enters it, got %q", a.sess.Current())
	}
	press(a, "h")
	if a.sess.Current() != "ws-4k2" {
		t.Errorf("h on a leaf goes to its parent, got %q", a.sess.Current())
	}

	press(a, "z", "M")
	for _, r := range tree.rows {
		if r.Depth > 0 {
			t.Fatalf("zM left %q unfolded", r.ID)
		}
	}
	a.sess.SetCurrent("ws-4k2")
	press(a, "z", "R")
	deep := 0
	for _, r := range tree.rows {
		deep = max(deep, r.Depth)
	}
	if deep < 2 {
		t.Errorf("zR unfolded to depth %d", deep)
	}
}

func TestTreeFoldAllMovesCurrentToItsVisibleAncestor(t *testing.T) {
	a := viewApp(t, plain, 100, 30, "tree", false)
	a.sess.SetCurrent("ws-4k2.5.1")
	press(a, "z", "M")
	if got := a.sess.Current(); got != "ws-4k2" {
		t.Errorf("current %q after zM, want the root", got)
	}
}

func TestTreeClosedRowFoldsAndKeepsParentCurrent(t *testing.T) {
	a := viewApp(t, plain, 100, 30, "tree", false)
	tree := a.view().(*Tree)
	a.sess.SetCurrent("ws-4k2.5.1")
	press(a, "j")
	i := tree.cursorIndex(a.env())
	if i < 0 || tree.rows[i].Kind != model.TreeClosedFold || a.sess.Current() != "ws-4k2" {
		t.Fatalf("cursor row %d, current %q: want the closed-children row of ws-4k2", i, a.sess.Current())
	}
	before := len(tree.rows)
	press(a, "enter")
	if len(tree.rows) != before+2 {
		t.Errorf("Enter on the closed row unfolds it: %d -> %d rows", before, len(tree.rows))
	}
	if a.sess.Has(state.LayerDetail) || a.sess.Has(state.LayerDetailFocus) {
		t.Error("Enter on a fold row must not open the detail")
	}
	press(a, "enter")
	if len(tree.rows) != before {
		t.Errorf("second Enter folds again: %d rows", len(tree.rows))
	}
}

func TestTreeRevealsAnIssueMovedIntoAFoldedSubtree(t *testing.T) {
	a := viewApp(t, plain, 100, 30, "tree", false)
	tree := a.view().(*Tree)
	press(a, "z", "M")
	target := ""
	for id, p := range tree.parents {
		if p != "" && tree.present[id] {
			target = id
			break
		}
	}
	if target == "" {
		t.Fatal("no nested issue in the fixture")
	}
	a.sess.Jump(target)
	send(a, tea.WindowSizeMsg{Width: 100, Height: 30})
	if _, ok := tree.index[target]; !ok {
		t.Errorf("%q is not shown after a jump to it", target)
	}
}

func TestTreeScopeKeepsAncestorsAndEmptyState(t *testing.T) {
	a := viewApp(t, plain, 100, 30, "tree", false)
	a.setScope(model.ParseScope("mail", false))
	out := screen(a)
	if !strings.Contains(out, "Guest order confirmation mail") || !strings.Contains(out, "Checkout") {
		t.Errorf("scope must keep the match and its parent:\n%s", out)
	}
	if strings.Contains(out, "Payment provider") {
		t.Errorf("scope shows a non-match:\n%s", out)
	}
	a.setScope(model.ParseScope("zzzzz", false))
	if out := screen(a); !strings.Contains(out, `No issue matches "zzzzz".`) {
		t.Errorf("empty scope state missing:\n%s", out)
	}
	press(a, "esc")
	if a.scope.Active() || !strings.Contains(screen(a), "Payment provider") {
		t.Error("Esc must clear the scope")
	}
}

func TestTreeLeftOnAnOpenClosedRowCollapsesIt(t *testing.T) {
	a := viewApp(t, plain, 100, 30, "tree", false)
	tree := a.view().(*Tree)
	a.sess.SetCurrent("ws-4k2.5.1")
	press(a, "j")
	before := len(tree.rows)
	press(a, "enter")
	if len(tree.rows) != before+2 {
		t.Fatalf("Enter opens the closed row: %d -> %d rows", before, len(tree.rows))
	}
	press(a, "h")
	if len(tree.rows) != before {
		t.Errorf("h on the open row leaves %d rows, want %d", len(tree.rows), before)
	}
	if i := tree.cursorIndex(a.env()); i < 0 || tree.rows[i].Kind != model.TreeClosedFold {
		t.Errorf("the cursor left the closed row")
	}
}

func TestTreeChangedWalkSurvivesCycles(t *testing.T) {
	issues := []model.Issue{
		{ID: "c-1", Title: "a", Status: "open", Parent: "c-2", CreatedAt: uitest.T0},
		{ID: "c-2", Title: "b", Status: "open", Parent: "c-1", CreatedAt: uitest.T0},
		{ID: "c-3", Title: "root", Status: "open", CreatedAt: uitest.T0},
	}
	snap := model.NewSnapshot(issues, model.Readiness{}, uitest.T0)
	a := loaded(t, plain, 100, 30, snap, withView("tree", false))
	env := a.env()
	env.Changed = func(string) bool { return false }
	v := a.view().(*Tree)
	if v.changed(env, model.TreeRow{Kind: model.TreeIssue, ID: "c-1", Folded: true}) {
		t.Error("nothing changed")
	}
	env.Changed = func(id string) bool { return id == "c-2" }
	if !v.changed(env, model.TreeRow{Kind: model.TreeIssue, ID: "c-1", Folded: true}) {
		t.Error("a change below a folded row must show")
	}
}

func TestHeaderShowsTheScopeLabelOnlyWhileActive(t *testing.T) {
	a := viewApp(t, plain, 100, 30, "tree", false)
	send(a, tea.WindowSizeMsg{Width: 140, Height: 30})
	if h := lines(a)[0]; strings.Contains(h, " · ") {
		t.Errorf("no scope, no label: %q", h)
	}
	a.setScope(model.ParseScope("mail", false))
	send(a, tea.WindowSizeMsg{Width: 140, Height: 30})
	h := lines(a)[0]
	if !strings.Contains(h, "/ mail · 1/") || !strings.Contains(h, "closed hidden") {
		t.Errorf("header lacks the scope label: %q", h)
	}
	send(a, tea.WindowSizeMsg{Width: 60, Height: 30})
	if w := len([]rune(lines(a)[0])); w != 60 {
		t.Errorf("header is %d cells wide, want 60", w)
	}
	a.setScope(model.ParseScope("", false))
	send(a, tea.WindowSizeMsg{Width: 140, Height: 30})
	if h := lines(a)[0]; strings.Contains(h, "closed hidden") {
		t.Errorf("label survives clearing the scope: %q", h)
	}
}
