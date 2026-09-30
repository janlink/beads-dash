package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

var sizes = [][2]int{{60, 16}, {80, 24}, {120, 40}, {200, 50}}

func TestShellGoldens(t *testing.T) {
	for _, s := range sizes {
		t.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(t *testing.T) {
			testgolden.Equal(t, screen(sample(t, plain, s[0], s[1])))
		})
	}
}

func TestShellTruecolorGolden(t *testing.T) {
	a := sample(t, truecolor, 80, 24)
	press(a, "j", "space")
	testgolden.Equal(t, a.View().Content)
}

func TestShellFillsTheScreenExactly(t *testing.T) {
	for _, s := range sizes {
		a := sample(t, plain, s[0], s[1])
		ls := lines(a)
		if len(ls) != s[1] {
			t.Errorf("%dx%d: %d lines", s[0], s[1], len(ls))
		}
		for i, l := range ls {
			if w := len([]rune(l)); w != s[0] {
				t.Errorf("%dx%d line %d: width %d", s[0], s[1], i, w)
			}
		}
	}
}

func TestTooSmallGolden(t *testing.T) {
	a := sample(t, plain, 59, 15)
	testgolden.Equal(t, screen(a))
	if !strings.Contains(screen(a), "Terminal too small") {
		t.Error("no too-small screen")
	}
	send(a, tea.WindowSizeMsg{Width: 60, Height: 16})
	if strings.Contains(screen(a), "too small") {
		t.Error("60x16 is big enough")
	}
}

func TestTooSmallKeepsRefreshing(t *testing.T) {
	a := sample(t, plain, 40, 10)
	snap, _ := uitest.Sample()
	send(a, updateMsg{refresh.Update{Snapshot: snap, Status: refresh.Status{Loaded: true, Stale: true, LastSuccess: uitest.T0}, Session: workspace()}})
	if a.status.Stale != true {
		t.Error("updates are ignored while the screen is too small")
	}
	press(a, "j", "?")
	if len(a.dialogs) != 0 {
		t.Error("keys other than q reached the shell")
	}
}

func TestBreakpoints(t *testing.T) {
	for cols, want := range map[int]Breakpoint{60: Narrow, 79: Narrow, 80: Regular, 119: Regular, 120: Roomy, 199: Roomy, 200: Wide} {
		if got := BreakpointOf(cols); got != want {
			t.Errorf("%d: %v, want %v", cols, got, want)
		}
	}
}

func TestHeaderTabsFromRegularWidth(t *testing.T) {
	narrow := lines(sample(t, plain, 79, 24))[0]
	if strings.Contains(narrow, "2 Tree") || !strings.Contains(narrow, "Overview") {
		t.Errorf("narrow header %q", narrow)
	}
	regular := lines(sample(t, plain, 80, 24))[0]
	if !strings.Contains(regular, "1 Overview") || !strings.Contains(regular, "2 Tree") {
		t.Errorf("regular header %q", regular)
	}
}

func TestHeaderIsOneRuleAndFooterTwoRows(t *testing.T) {
	a := sample(t, plain, 80, 24)
	ls := lines(a)
	if !strings.Contains(ls[0], "12:00:00 -") || !strings.HasPrefix(ls[0], "- [1 Overview]") {
		t.Errorf("header %q", ls[0])
	}
	if !strings.HasPrefix(ls[22], "---") || strings.Contains(ls[22], "help") {
		t.Errorf("footer rule %q", ls[22])
	}
	if !strings.Contains(ls[23], "? help") {
		t.Errorf("footer hints %q", ls[23])
	}
}

func TestHelpAndFooterHintsGoldens(t *testing.T) {
	for _, s := range sizes {
		t.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(t *testing.T) {
			a := sample(t, plain, s[0], s[1])
			base := lines(a)
			out := base[0] + "\n" + base[len(base)-1] + "\n\n"
			press(a, "?")
			out += screen(a)
			testgolden.Equal(t, out)
		})
	}
}

func TestHelpFilter(t *testing.T) {
	a := sample(t, plain, 120, 40)
	press(a, "?", "/", "m", "a", "r", "k")
	s := screen(a)
	if !strings.Contains(s, "Space") || strings.Contains(s, "Backspace") {
		t.Errorf("filter did not narrow the list:\n%s", s)
	}
	press(a, "esc")
	if !isOpen[*helpDialog](a) || a.topDialog().(*helpDialog).filter != "" {
		t.Error("Esc in the filter must clear it and keep the overlay")
	}
	press(a, "esc")
	if len(a.dialogs) != 0 {
		t.Error("second Esc must close the overlay")
	}
}

func TestDialogsOwnTheirKeys(t *testing.T) {
	a := sample(t, plain, 100, 30)
	press(a, "?", "q", "j", "1", "t", "space")
	if !isOpen[*helpDialog](a) || a.quitting || a.sess.MarkCount() != 0 {
		t.Errorf("help overlay leaked keys: dialogs %d quitting %v marks %d", len(a.dialogs), a.quitting, a.sess.MarkCount())
	}
}

func TestQuitRules(t *testing.T) {
	a := sample(t, plain, 100, 30)
	press(a, "q")
	if !a.quitting {
		t.Error("q at base level must quit")
	}
	a = sample(t, plain, 100, 30)
	press(a, "t", "q")
	if a.quitting {
		t.Error("q inside a dialog must not quit")
	}
	press(a, "ctrl+c")
	if !a.quitting {
		t.Error("ctrl+c quits from anywhere")
	}
}

func TestViewSwitchingAndUnregisteredHint(t *testing.T) {
	a := sample(t, plain, 100, 30)
	press(a, "3")
	if a.slot != 0 || !strings.Contains(lines(a)[29], "Kanban is not available yet") {
		t.Errorf("slot %d footer %q", a.slot, lines(a)[29])
	}
	press(a, "j")
	if strings.Contains(lines(a)[29], "not available") {
		t.Error("hint outlives the next key")
	}
}

func TestViewsRegisterIntoTheShell(t *testing.T) {
	second := &placeholder{}
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.Views = map[int]View{1: &placeholder{}, 2: second} })
	press(a, "2")
	if a.slot != 1 || a.view() != View(second) {
		t.Errorf("slot %d", a.slot)
	}
	if !strings.Contains(lines(a)[0], "[2 Tree]") {
		t.Errorf("header %q", lines(a)[0])
	}
}

func snapOf(_ testing.TB) *model.Snapshot {
	s, _ := uitest.Sample()
	return s
}

func TestNavigationAndCurrent(t *testing.T) {
	a := sample(t, plain, 100, 30)
	ids := snapOf(t).IDs()
	press(a, "j", "j")
	if a.sess.Current() != ids[2] {
		t.Errorf("current %q after jj", a.sess.Current())
	}
	press(a, "G")
	if a.sess.Current() != ids[len(ids)-1] {
		t.Errorf("current %q after G", a.sess.Current())
	}
	press(a, "g", "g")
	if a.sess.Current() != ids[0] {
		t.Errorf("current %q after gg", a.sess.Current())
	}
}

func TestNumberKeysWithoutViewDoNotMoveCursor(t *testing.T) {
	a := sample(t, plain, 100, 30)
	cur := a.sess.Current()
	press(a, "4", "5", "6")
	if a.sess.Current() != cur {
		t.Error("cursor moved")
	}
}

func TestEscCascade(t *testing.T) {
	a := sample(t, plain, 100, 30)
	press(a, "space", "?")
	press(a, "esc")
	if len(a.dialogs) != 0 || a.sess.MarkCount() != 1 {
		t.Fatalf("first Esc must close the dialog only: dialogs %d marks %d", len(a.dialogs), a.sess.MarkCount())
	}
	press(a, "esc")
	if a.sess.MarkCount() != 0 {
		t.Error("second Esc must clear the marks")
	}
}

func TestMarksSurviveWhileHiddenAndDieWithTheIssue(t *testing.T) {
	a := sample(t, plain, 100, 30)
	press(a, "space")
	id := a.sess.Current()
	press(a, "j", "space")
	snap := snapOf(t)
	kept := snap.IDs()[1:]
	var issues []model.Issue
	for _, id := range kept {
		is, _ := snap.Issue(id)
		issues = append(issues, *is)
	}
	send(a, updateMsg{refresh.Update{Snapshot: model.NewSnapshot(issues, model.Readiness{}, uitest.T0), Changed: true, Status: liveStatus()}})
	if a.sess.Marked(id) || a.sess.MarkCount() != 1 {
		t.Errorf("marks after deleting %s: %v", id, a.sess.MarkedIDs())
	}
}

func TestCurrentMovesToNearestNeighbourWhenItVanishes(t *testing.T) {
	a := sample(t, plain, 100, 30)
	snap := snapOf(t)
	ids := snap.IDs()
	press(a, "j", "j", "j")
	gone := a.sess.Current()
	if gone != ids[3] {
		t.Fatalf("setup: %q", gone)
	}
	var issues []model.Issue
	for _, id := range ids {
		if id == gone {
			continue
		}
		is, _ := snap.Issue(id)
		issues = append(issues, *is)
	}
	send(a, updateMsg{refresh.Update{Snapshot: model.NewSnapshot(issues, model.Readiness{}, uitest.T0), Changed: true, Status: liveStatus()}})
	if a.sess.Current() != ids[4] {
		t.Errorf("current %q, want the neighbour below %q", a.sess.Current(), ids[4])
	}
}

func TestResizeKeepsCurrentMarksAndScrollAnchor(t *testing.T) {
	snap, _ := uitest.Big(200)
	a := loaded(t, plain, 100, 30, snap, nil)
	press(a, "space", "G")
	last := a.sess.Current()
	send(a, tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 5, Y: 5}, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	anchor := a.view().(*placeholder).win.anchor
	send(a, tea.WindowSizeMsg{Width: 80, Height: 24})
	if a.sess.Current() != last || !a.sess.Marked(snap.IDs()[0]) {
		t.Error("resize lost the current issue or a mark")
	}
	if got := a.view().(*placeholder).win.anchor; got != anchor {
		t.Errorf("anchor %q -> %q", anchor, got)
	}
}

func TestBackStack(t *testing.T) {
	a := sample(t, plain, 100, 30)
	ids := snapOf(t).IDs()
	press(a, "j")
	a.sess.Jump(ids[5])
	press(a, "backspace")
	if a.sess.Current() != ids[1] {
		t.Errorf("current %q after Backspace", a.sess.Current())
	}
}

func TestHighlightsPauseOnBlurAndDialogAndTickPerBucket(t *testing.T) {
	now := uitest.T0
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.Now = func() time.Time { return now } })
	ids := snapOf(t).IDs()
	cmd := send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Highlights: ids[:2], Status: liveStatus()}})
	if cmd == nil {
		t.Fatal("a highlight schedules a tick")
	}
	if !strings.Contains(screen(a), "2 changed") {
		t.Errorf("no changed counter:\n%s", lines(a)[29])
	}
	send(a, tea.BlurMsg{})
	now = now.Add(30 * time.Second)
	send(a, hlTickMsg{a.hlGen})
	if !strings.Contains(screen(a), "2 changed") {
		t.Error("highlights ran out while blurred")
	}
	send(a, tea.FocusMsg{})
	press(a, "?")
	now = now.Add(30 * time.Second)
	if !strings.Contains(screen(a), "2 changed") {
		t.Error("highlights ran out while a dialog was open")
	}
	press(a, "esc")
	now = now.Add(11 * time.Second)
	send(a, hlTickMsg{a.hlGen})
	if strings.Contains(screen(a), "changed") {
		t.Error("highlights outlived their duration")
	}
}

func TestChangedCounterCountsHidden(t *testing.T) {
	a := sample(t, plain, 100, 30)
	snap := snapOf(t)
	ids := []string{snap.IDs()[0], "ghost-1"}
	send(a, updateMsg{refresh.Update{Snapshot: snap, Highlights: ids, Status: liveStatus()}})
	if !strings.Contains(lines(a)[28], "2 changed (1 hidden)") {
		t.Errorf("footer %q", lines(a)[28])
	}
}

func TestMarksCounterShowsHidden(t *testing.T) {
	a := sample(t, plain, 100, 30)
	press(a, "space")
	a.sess.ToggleMark("elsewhere")
	if !strings.Contains(lines(a)[28], "2 marked (1 hidden)") {
		t.Errorf("footer %q", lines(a)[28])
	}
}

func TestHealthChips(t *testing.T) {
	a := sample(t, plain, 120, 30)
	snap := snapOf(t)
	st := liveStatus()
	st.Slow, st.GCHint, st.Fallback = true, true, true
	send(a, updateMsg{refresh.Update{Snapshot: snap, Status: st}})
	f := lines(a)[28]
	for _, want := range []string{"slow", "gc", "journal-limited"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer %q lacks %q", f, want)
		}
	}
}

func TestStaleAndUntestedHeader(t *testing.T) {
	a := sample(t, plain, 120, 30)
	ws := workspace()
	ws.Untested = true
	ws.Version = bd.VersionInfo{Raw: "1.4.0", Parsed: bd.MinSupported}
	st := liveStatus()
	st.Stale, st.Err = true, &bd.Error{Class: bd.ClassTransient, Command: "list", Message: "database busy"}
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Status: st, Session: ws}})
	h := lines(a)[0]
	if !strings.Contains(h, "stale") || !strings.Contains(h, "untested") {
		t.Errorf("header %q", h)
	}
	if n := lines(a)[29]; !strings.Contains(n, "refresh failed: database busy") {
		t.Errorf("notice row %q", n)
	}
	press(a, "!")
	if !isOpen[*detailsDialog](a) || !strings.Contains(screen(a), "Error details") {
		t.Error("! must open the details overlay")
	}
}

func TestExitedNoticeDismissedByNextKey(t *testing.T) {
	a := New(testOptions(plain, func(o *Options) { o.Warnings = []string{"config is broken"} }))
	send(a, tea.WindowSizeMsg{Width: 100, Height: 30}, sessionMsg{sess: workspace()},
		updateMsg{refresh.Update{Snapshot: snapOf(t), Status: liveStatus()}})
	if !strings.Contains(lines(a)[29], "config is broken") {
		t.Fatalf("notice row %q", lines(a)[29])
	}
	press(a, "j")
	if strings.Contains(screen(a), "config is broken") {
		t.Error("notice outlived the next key")
	}
}

func TestEmptyWorkspaceState(t *testing.T) {
	a := loaded(t, plain, 80, 24, model.NewSnapshot(nil, model.Readiness{}, uitest.T0), nil)
	testgolden.Equal(t, screen(a))
}

func TestWindowTitleAndTerminalModes(t *testing.T) {
	a := sample(t, plain, 80, 24)
	v := a.View()
	if v.WindowTitle != "bdash · demo" || !v.AltScreen || !v.ReportFocus || v.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("view = %q alt %v focus %v mouse %v", v.WindowTitle, v.AltScreen, v.ReportFocus, v.MouseMode)
	}
	off := loaded(t, plain, 80, 24, snapOf(t), func(o *Options) { o.NoMouse = true })
	if off.View().MouseMode != tea.MouseModeNone {
		t.Error("--no-mouse leaves the mouse on")
	}
}

func TestMouse(t *testing.T) {
	a := sample(t, plain, 100, 30)
	ids := snapOf(t).IDs()
	a.View()
	send(a, tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 4})
	if a.sess.Current() != ids[3] {
		t.Errorf("click selected %q", a.sess.Current())
	}
	send(a, tea.MouseClickMsg{Button: tea.MouseLeft, X: 17, Y: 0})
	if a.slot != 0 || a.hint == "" {
		t.Errorf("click on the Tree tab: slot %d hint %q", a.slot, a.hint)
	}
	off := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.NoMouse = true })
	off.View()
	send(off, tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 4})
	if off.sess.Current() != ids[0] {
		t.Error("click acted with the mouse off")
	}
}
