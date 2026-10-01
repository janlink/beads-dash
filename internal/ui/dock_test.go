package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/ui/detail"
	"github.com/janlink/beads-dash/internal/ui/state"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func TestDetailPlacementGoldens(t *testing.T) {
	for _, s := range goldenSizes {
		t.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(t *testing.T) {
			a := viewApp(t, plain, s[0], s[1], "tree", false)
			press(a, "enter")
			testgolden.Equal(t, screen(a))
		})
	}
	t.Run("docked not focused 120x40", func(t *testing.T) {
		testgolden.Equal(t, screen(viewApp(t, plain, 120, 40, "ready", true)))
	})
	t.Run("truecolor 120x40", func(t *testing.T) {
		a := viewApp(t, truecolor, 120, 40, "tree", true)
		press(a, "tab")
		testgolden.Equal(t, a.View().Content)
	})
}

func TestDetailPlacementEdges(t *testing.T) {
	for _, tc := range []struct {
		cols, rows int
		want       detail.Frame
	}{
		{106, 30, detail.Overlay},
		{107, 30, detail.Side},
		{200, 50, detail.Side},
		{200, 16, detail.Side},
		{80, 24, detail.Overlay},
	} {
		t.Run(fmt.Sprintf("%dx%d", tc.cols, tc.rows), func(t *testing.T) {
			a := viewApp(t, plain, tc.cols, tc.rows, "tree", false)
			press(a, "enter")
			if got := a.frame().Frame; got != tc.want {
				t.Fatalf("frame %v, want %v", got, tc.want)
			}
			if tc.cols == 106 || tc.cols == 107 || tc.rows == 16 {
				testgolden.Equal(t, screen(a))
			}
			for i, l := range lines(a) {
				if w := len([]rune(l)); w != tc.cols {
					t.Fatalf("line %d is %d cells wide, want %d", i, w, tc.cols)
				}
			}
		})
	}
}

func TestEnterTogglesDetailAndEscClosesIt(t *testing.T) {
	a := viewApp(t, plain, 60, 24, "tree", false)
	press(a, "enter")
	if !a.sess.Has(state.LayerDetail) || a.baseContext().String() != "panel" {
		t.Fatalf("Enter opens the overlay: layers %v context %v", a.sess.Has(state.LayerDetail), a.baseContext())
	}
	press(a, "esc")
	if a.shown || a.sess.Has(state.LayerDetail) {
		t.Error("Esc closes the overlay")
	}

	a = viewApp(t, plain, 120, 40, "tree", false)
	press(a, "enter")
	if a.frame().Frame != detail.Side || a.sess.Has(state.LayerDetailFocus) {
		t.Fatal("Enter shows the side panel and leaves the keys in the list")
	}
	press(a, "tab")
	if !a.sess.Has(state.LayerDetailFocus) {
		t.Error("Tab focuses the panel")
	}
	press(a, "esc")
	if a.sess.Has(state.LayerDetailFocus) || a.frame().Frame != detail.Side {
		t.Error("Esc leaves the panel focus and keeps the panel")
	}
	press(a, "esc")
	if a.frame().Frame != detail.Hidden {
		t.Error("Esc in the list closes the side panel")
	}
	press(a, "enter", "enter")
	if a.frame().Frame != detail.Hidden {
		t.Error("Enter twice hides the side panel again")
	}
}

func TestResizeMovesTheDetailLayerBetweenOverlayAndSide(t *testing.T) {
	a := viewApp(t, plain, 60, 24, "tree", true)
	if !a.sess.Has(state.LayerDetail) {
		t.Fatal("a shown panel on a narrow terminal is an overlay")
	}
	send(a, tea.WindowSizeMsg{Width: 120, Height: 40})
	if !a.sess.Has(state.LayerDetailFocus) || a.sess.Has(state.LayerDetail) {
		t.Error("a wide terminal docks the overlay and focuses it")
	}
	send(a, tea.WindowSizeMsg{Width: 60, Height: 24})
	if !a.sess.Has(state.LayerDetail) || a.sess.Has(state.LayerDetailFocus) {
		t.Error("a narrow terminal turns the focus back into the overlay")
	}
}

func TestOverlayLiesUnderAnOpenBar(t *testing.T) {
	a := viewApp(t, plain, 60, 24, "tree", false)
	press(a, "/")
	press(a, "D")
	if top, _ := a.sess.Top(); top != state.LayerBar {
		t.Fatalf("top layer %v, want the bar", top)
	}
}

func TestPanelKeysAndShownSetting(t *testing.T) {
	a := viewApp(t, plain, 120, 40, "tree", true)
	a.sess.SetCurrent("ws-4k2")
	press(a, "tab")
	_ = screen(a)
	press(a, "]")
	if a.panel.Cursor() == detail.Description {
		t.Error("] moves to the next section")
	}
	press(a, "o")
	if a.panel.Open(detail.Dependencies) {
		t.Error("o with sections open closes them all")
	}
	press(a, "m")
	if !a.panel.Source() {
		t.Error("m shows the source")
	}

	press(a, "D")
	if a.shown || a.sess.Has(state.LayerDetailFocus) || a.frame().Frame != detail.Hidden {
		t.Errorf("D hides the panel: shown %v frame %v", a.shown, a.frame().Frame)
	}
}

func TestMouseFocusesAndScrollsThePanel(t *testing.T) {
	a := viewApp(t, plain, 120, 40, "tree", true)
	a.o.Settings.Settings.Mouse = true
	send(a, tea.MouseClickMsg{Button: tea.MouseLeft, X: 100, Y: 10})
	if !a.sess.Has(state.LayerDetailFocus) {
		t.Error("a click in the panel focuses it")
	}
	send(a, tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 3})
	if a.sess.Has(state.LayerDetailFocus) {
		t.Error("a click in the list returns the focus")
	}
}

func TestCurrentFollowsTreeAndReadyAndHighlightsShowInBoth(t *testing.T) {
	snap, _ := uitest.Tree()
	now := uitest.T0
	a := loaded(t, plain, 120, 30, snap, func(o *Options) {
		withView("tree", false)(o)
		o.Now = func() time.Time { return now }
	})
	a.sess.SetCurrent("ws-4k2.1")
	press(a, "4")
	if a.sess.Current() != "ws-4k2.1" {
		t.Fatalf("Ready dropped the current issue: %q", a.sess.Current())
	}
	send(a, updateMsg{refresh.Update{Snapshot: snap, Highlights: []string{"ws-4k2.1"}, Status: liveStatus()}})
	if !strings.Contains(screen(a), "changed") {
		t.Error("no changed chip")
	}
	marked := func() bool {
		for _, l := range lines(a) {
			if strings.Contains(l, "ws-4k2.1") {
				return strings.Contains(l[:3], "+")
			}
		}
		return false
	}
	if !marked() {
		t.Errorf("Ready row lacks the change mark:\n%s", screen(a))
	}
	press(a, "2")
	if a.sess.Current() != "ws-4k2.1" || !marked() {
		t.Errorf("Tree row lacks the change mark or lost current:\n%s", screen(a))
	}
	press(a, "4")
	a.sess.SetCurrent("ws-2hz")
	press(a, "2")
	if a.sess.Current() != "ws-2hz" {
		t.Errorf("Tree must keep a blocked issue current, got %q", a.sess.Current())
	}
}

func TestStepsStayUnderBudgetWithFiveThousandIssues(t *testing.T) {
	if raceEnabled || testing.Short() {
		t.Skip("timing is only meaningful without the race detector")
	}
	snap, _ := uitest.Deep(5000)
	for _, view := range []string{"tree", "ready"} {
		a := loaded(t, truecolor, 200, 50, snap, withView(view, true))
		a.syncMD = false
		press(a, "z", "R")
		a.sess.SetCurrent("deep-0000")
		if a.frame().Frame != detail.Side {
			t.Fatalf("%s: frame %v, want the side panel", view, a.frame().Frame)
		}
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
			t.Errorf("%s: a step took %v on average, budget 8ms (worst %v)", view, per, worst)
		}
		t.Logf("%s: average %v, worst %v", view, total/runs, worst)
	}
}

func TestEnterOnAPanelRowJumpsAndBackReturns(t *testing.T) {
	a := viewApp(t, plain, 120, 40, "tree", true)
	a.sess.SetCurrent("ws-4k2")
	press(a, "tab")
	_ = screen(a)
	for range 8 {
		if a.panel.Cursor() == detail.Children {
			break
		}
		press(a, "]")
		_ = screen(a)
	}
	if a.panel.Cursor() != detail.Children {
		t.Fatalf("cursor %v", a.panel.Cursor())
	}
	press(a, "j")
	_ = screen(a)
	id, ok := a.panel.Row()
	if !ok {
		t.Fatal("j does not move onto a child row")
	}
	press(a, "enter")
	if a.sess.Current() != id {
		t.Fatalf("Enter jumped to %q, want %q", a.sess.Current(), id)
	}
	if !strings.Contains(screen(a), id) {
		t.Errorf("the panel does not show the issue jumped to:\n%s", screen(a))
	}
	press(a, "backspace")
	if a.sess.Current() != "ws-4k2" {
		t.Errorf("back returned to %q", a.sess.Current())
	}
}

func TestDetailToggleIsListedInHelp(t *testing.T) {
	a := viewApp(t, plain, 120, 100, "tree", true)
	press(a, "?")
	if out := screen(a); !strings.Contains(out, "show or hide the detail") {
		t.Errorf("help lacks the D binding:\n%s", out)
	}
}

func TestMarkdownRendersOffTheUpdateLoop(t *testing.T) {
	a := viewApp(t, plain, 120, 40, "tree", true)
	a.syncMD = false
	a.panel = detail.New()
	a.sess.SetCurrent("ws-9qe")
	cmd := send(a, tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd == nil {
		t.Fatal("no render command for the docked panel")
	}
	if out := screen(a); !strings.Contains(out, "Retries **fail**") {
		t.Fatalf("until the renderer answers the source is shown:\n%s", out)
	}
	var feed func(c tea.Cmd)
	feed = func(c tea.Cmd) {
		switch m := c().(type) {
		case tea.BatchMsg:
			for _, c := range m {
				feed(c)
			}
		case mdMsg:
			send(a, m)
		}
	}
	feed(cmd)
	if out := screen(a); !strings.Contains(out, "Retries fail after") || strings.Contains(out, "**fail**") {
		t.Errorf("the rendered markdown did not arrive:\n%s", out)
	}
}
