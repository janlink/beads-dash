package ui

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/detail"
	"github.com/janlink/beads-dash/internal/ui/state"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

// overviewFeed is the prefill of the tree workspace plus a few live events,
// one with the actor a journal supplies.
func overviewFeed() []model.Event {
	snap, _ := uitest.Tree()
	ago := func(d time.Duration) time.Time { return uitest.T0.Add(-d) }
	return append(model.Prefill(snap),
		model.Event{Kind: model.KindClaimed, IssueID: "ws-7mt", Title: "Cart badge shows stale count", Time: ago(40 * time.Second), Actor: "bob", Detail: "bob"},
		model.Event{Kind: model.KindPriorityChanged, IssueID: "ws-9qe", Title: "Payment provider timeout retries", Time: ago(3 * time.Minute), Detail: "2→0"},
		model.Event{Kind: model.KindBecameBlocked, IssueID: "ws-2hz", Title: "Split pricing service", Time: ago(20 * time.Minute), Detail: "by ws-9qe"},
	)
}

func overviewApp(t testing.TB, f flavour, cols, rows int) *App {
	t.Helper()
	a := viewApp(t, f, cols, rows, "overview", false)
	send(a, updateMsg{refresh.Update{Events: overviewFeed(), Status: liveStatus(), Session: workspace()}})
	return a
}

func TestOverviewGoldens(t *testing.T) {
	sizes := append([][2]int{{79, 24}, {199, 50}}, goldenSizes...)
	for _, s := range sizes {
		t.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(t *testing.T) {
			testgolden.Equal(t, screen(overviewApp(t, plain, s[0], s[1])))
		})
	}
	t.Run("truecolor", func(t *testing.T) {
		testgolden.Equal(t, overviewApp(t, truecolor, 120, 40).View().Content)
	})
}

func TestOverviewFeedEnterOpensDetailAndEscReturns(t *testing.T) {
	a := overviewApp(t, plain, 120, 40)
	_ = screen(a)
	feed := a.env().Feed
	if len(feed) == 0 {
		t.Fatal("no feed")
	}
	a.sess.SetCurrent(feed[0].IssueID)
	_ = screen(a)
	press(a, "enter")
	if a.frame().Frame != detail.Side {
		t.Fatalf("Enter on a feed line does not open the detail:\n%s", screen(a))
	}
	if out := screen(a); !strings.Contains(out, feed[0].IssueID) {
		t.Errorf("detail does not show %s:\n%s", feed[0].IssueID, out)
	}
	press(a, "esc")
	_ = screen(a)
	if a.frame().Frame != detail.Hidden {
		t.Errorf("Esc leaves the detail open:\n%s", screen(a))
	}
	if a.slot != 0 || !strings.Contains(screen(a), "Activity") {
		t.Errorf("not back on the Overview:\n%s", screen(a))
	}
}

func TestOverviewAttentionRowJumpsToReady(t *testing.T) {
	a := overviewApp(t, plain, 120, 40)
	_ = screen(a)
	ov := a.view().(*Overview)
	press(a, "tab")
	_ = screen(a)
	if ov.focus != regAttention {
		t.Fatalf("Tab focuses region %v", ov.focus)
	}
	press(a, "j")
	_ = screen(a)
	want := ov.items[regAttention][1].id
	if got := a.sess.Current(); got != want {
		t.Fatalf("j moved current to %q, want %q", got, want)
	}
	press(a, "enter")
	if a.slot != slotReady-1 || a.sess.Current() != want {
		t.Errorf("slot %d current %q, want Ready and %q", a.slot, a.sess.Current(), want)
	}
}

func TestOverviewActiveRowJumpsToKanban(t *testing.T) {
	a := overviewApp(t, plain, 120, 40)
	_ = screen(a)
	ov := a.view().(*Overview)
	press(a, "tab", "tab")
	_ = screen(a)
	if ov.focus != regActive {
		t.Fatalf("focus %v, want Active", ov.focus)
	}
	want := ov.items[regActive][0].id
	press(a, "enter")
	if a.slot != slotKanban-1 || a.sess.Current() != want {
		t.Errorf("slot %d current %q, want Kanban and %q", a.slot, a.sess.Current(), want)
	}
}

func TestOverviewTabCyclesRegions(t *testing.T) {
	a := overviewApp(t, plain, 200, 50)
	_ = screen(a)
	ov := a.view().(*Overview)
	seen := map[ovRegion]bool{ov.active(): true}
	for range 3 {
		press(a, "tab")
		_ = screen(a)
		seen[ov.active()] = true
	}
	if len(seen) < 3 {
		t.Errorf("Tab visited %d regions, want 3", len(seen))
	}
}

func TestTeatestOverviewFeedFlow(t *testing.T) {
	fake := fakeWorkspace(t)
	a := New(testOptions(plain, func(o *Options) {
		o.Client, o.Now = fake, time.Now
		o.NewEngine = func(s bd.Session) Engine {
			return refresh.New(refresh.Options{Client: fake, Session: s})
		}
		withView("overview", false)(o)
	}))
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(120, 40))
	waitFor := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(10*time.Second))
	}
	waitFor("Activity")
	tm.Send(keyMsg("enter"))
	waitFor("Dependencies")
	tm.Send(keyMsg("esc"))
	waitFor("Needs attention")
	tm.Send(keyMsg("q"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(10*time.Second)).(*App)
	if !ok {
		t.Fatal("final model is not *ui.App")
	}
	if final.slot != 0 || final.sess.Has(state.LayerDetail) {
		t.Errorf("slot %d, detail open %v", final.slot, final.sess.Has(state.LayerDetail))
	}
}

func TestTeatestOverviewAttentionRowFlow(t *testing.T) {
	fake := fakeWorkspace(t)
	a := New(testOptions(plain, func(o *Options) {
		o.Client, o.Now = fake, time.Now
		o.NewEngine = func(s bd.Session) Engine {
			return refresh.New(refresh.Options{Client: fake, Session: s})
		}
		withView("overview", false)(o)
	}))
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(120, 40))
	waitFor := func(text string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(text))
		}, teatest.WithDuration(10*time.Second))
	}
	waitFor("Needs attention")
	tm.Send(keyMsg("tab"))
	tm.Send(keyMsg("enter"))
	waitFor("Unassigned")
	tm.Send(keyMsg("backspace"))
	waitFor("Needs attention")
	tm.Send(keyMsg("q"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(10*time.Second)).(*App)
	if !ok {
		t.Fatal("final model is not *ui.App")
	}
	if final.slot != 0 {
		t.Errorf("slot %d after Backspace, want the Overview", final.slot)
	}
}

func TestOverviewBackspaceRestoresViewAndIssue(t *testing.T) {
	a := overviewApp(t, plain, 120, 40)
	_ = screen(a)
	press(a, "tab")
	_ = screen(a)
	want := a.sess.Current()
	press(a, "enter")
	if a.slot != slotReady-1 {
		t.Fatalf("slot %d, want Ready", a.slot)
	}
	press(a, "backspace")
	if a.slot != 0 || a.sess.Current() != want {
		t.Errorf("slot %d current %q, want the Overview and %q", a.slot, a.sess.Current(), want)
	}
}

func TestOverviewTabFollowsReadingOrderAndLeavesForTheDock(t *testing.T) {
	a := viewApp(t, plain, 420, 50, "overview", true)
	send(a, updateMsg{refresh.Update{Events: overviewFeed(), Status: liveStatus(), Session: workspace()}})
	_ = screen(a)
	ov := a.view().(*Overview)
	got := []ovRegion{ov.active()}
	for range 2 {
		press(a, "tab")
		_ = screen(a)
		got = append(got, ov.active())
	}
	if want := []ovRegion{regFeed, regActive, regAttention}; !slices.Equal(got, want) {
		t.Fatalf("regions %v, want %v", got, want)
	}
	press(a, "tab")
	if !a.sess.Has(state.LayerDetailFocus) {
		t.Fatal("Tab after the last region does not focus the docked detail")
	}
	press(a, "tab")
	_ = screen(a)
	if a.sess.Has(state.LayerDetailFocus) {
		t.Fatal("Tab in the detail does not return to the list")
	}
	press(a, "tab")
	_ = screen(a)
	if ov.active() != regFeed {
		t.Errorf("Tab after coming back starts at region %v, want the feed", ov.active())
	}
}

func TestOverviewEmptyFeedNamesTheAction(t *testing.T) {
	a := viewApp(t, plain, 120, 40, "overview", false)
	if out := screen(a); !strings.Contains(out, "No recent activity.") || !strings.Contains(out, "refresh") {
		t.Errorf("unscoped empty feed:\n%s", out)
	}
	a.setScope(model.ParseScope("assignee:bob", false))
	if out := screen(a); !strings.Contains(out, "No recent activity in this scope.") || !strings.Contains(out, "Esc") {
		t.Errorf("scoped empty feed:\n%s", out)
	}
}

func TestOverviewActiveRowsDropTheAssigneeAndFeedTheEchoedActor(t *testing.T) {
	out := screen(overviewApp(t, plain, 120, 40))
	if strings.Contains(out, "P1 ws-4k2     Checkout: guest orders  alice") || strings.Contains(out, "bob · bob") {
		t.Errorf("repeated assignee or actor:\n%s", out)
	}
	if !strings.Contains(out, "bob") {
		t.Errorf("claimed feed line lost its actor:\n%s", out)
	}
}

func TestOverviewWheelScrollSticks(t *testing.T) {
	a := overviewApp(t, plain, 120, 16)
	_ = screen(a)
	ov := a.view().(*Overview)
	ov.Scroll(3)
	_ = screen(a)
	first := ov.off[regFeed]
	_ = screen(a)
	if first == 0 || ov.off[regFeed] != first {
		t.Errorf("offset %d then %d: the scroll did not stick", first, ov.off[regFeed])
	}
}

func TestOverviewClickIgnoresStaleBoxes(t *testing.T) {
	a := overviewApp(t, plain, 120, 40)
	_ = screen(a)
	ov := a.view().(*Overview)
	ov.items[regFeed] = ov.items[regFeed][:1]
	for y := range 40 {
		ov.AtXY(5, y)
	}
}

func TestSparklineIsOneCellPerDay(t *testing.T) {
	var counts [model.SparkDays]int
	counts[model.SparkDays-1] = 1
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierASCII} {
		got := sparkline(theme.Glyphs{Tier: tier}, counts[:])
		if w := ansi.StringWidth(got); w != model.SparkDays {
			t.Errorf("tier %v: %q is %d cells wide, want %d", tier, got, w, model.SparkDays)
		}
		if strings.ContainsRune(got, '�') {
			t.Errorf("tier %v: %q holds a replacement character", tier, got)
		}
	}
}

func TestOverviewNowCardHoldsOnlyTheSparkline(t *testing.T) {
	out := screen(viewApp(t, plain, 140, 38, "overview", false))
	if strings.ContainsRune(out, '�') {
		t.Error("overview holds a replacement character")
	}
}
