package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/state"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

type ping struct{}

// stubView is a placeholder that can narrow what it shows and record what the
// shell offers it.
type stubView struct {
	placeholder
	name    string
	only    []string
	handled map[keys.Action]bool
	onKey   func(keys.Action, Env)
	got     []keys.Action
	msgs    int
}

func (v *stubView) Name() string { return v.name }

func (v *stubView) Visible(env Env) []string {
	if v.only == nil {
		return v.placeholder.Visible(env)
	}
	return v.only
}

func (v *stubView) Has(env Env, id string) bool {
	for _, s := range v.Visible(env) {
		if s == id {
			return true
		}
	}
	return false
}

func (v *stubView) Handle(a keys.Action, env Env) (tea.Cmd, bool) {
	v.got = append(v.got, a)
	if v.onKey != nil {
		v.onKey(a, env)
	}
	return nil, v.handled[a]
}

func (v *stubView) Update(tea.Msg) tea.Cmd { v.msgs++; return nil }

func TestCtrlCQuitsWhileFilteringHelp(t *testing.T) {
	a := sample(t, plain, 100, 30)
	press(a, "?", "/", "a")
	press(a, "ctrl+c")
	if !a.quitting {
		t.Error("ctrl+c must quit with the help filter active")
	}
}

func TestQuitOnlyAtBaseLevel(t *testing.T) {
	a := sample(t, plain, 100, 30)
	a.sess.Push(state.LayerDetailFocus)
	press(a, "q")
	if a.quitting {
		t.Error("q must not quit from panel focus")
	}
	press(a, "esc", "q")
	if !a.quitting {
		t.Error("q quits once the panel is closed")
	}
}

func TestViewSwitchKeepsCursorPerSlot(t *testing.T) {
	ids := snapOf(t).IDs()
	one := &stubView{name: "one"}
	two := &stubView{name: "two", only: ids[3:6]}
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.Views = map[int]View{1: one, 2: two} })
	a.sess.SetCurrent(ids[1])
	press(a, "2")
	if a.sess.Current() != ids[3] {
		t.Fatalf("first visit: current %q, want the view's first row", a.sess.Current())
	}
	press(a, "j")
	want := ids[4]
	if a.sess.Current() != want {
		t.Fatalf("current %q", a.sess.Current())
	}
	press(a, "1")
	if a.sess.Current() != want {
		t.Errorf("a cursor visible in both views must stay: %q", a.sess.Current())
	}
	a.sess.SetCurrent(ids[0])
	press(a, "2")
	if a.sess.Current() != want {
		t.Errorf("returning to a view restores its last position: %q, want %q", a.sess.Current(), want)
	}
}

func TestViewHandleResultDecidesShellFallback(t *testing.T) {
	ids := snapOf(t).IDs()
	v := &stubView{name: "one", handled: map[keys.Action]bool{keys.NavDown: true}}
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.Views = map[int]View{1: v} })
	a.sess.SetCurrent(ids[0])
	press(a, "j")
	if a.sess.Current() != ids[0] {
		t.Errorf("a handled action must not move the cursor: %q", a.sess.Current())
	}
	press(a, "k")
	if len(v.got) != 2 || a.sess.Current() != ids[0] {
		t.Errorf("declined k at the top: got %v current %q", v.got, a.sess.Current())
	}
	press(a, "G")
	if a.sess.Current() != ids[len(ids)-1] {
		t.Errorf("a declined G must fall back to the shell: %q", a.sess.Current())
	}
}

func TestViewsReceiveMessagesAndSessionActions(t *testing.T) {
	ids := snapOf(t).IDs()
	km := keys.Default()
	keys.Add(km, keys.View, keys.Binding{Keys: []string{"J"}, Action: "test.jump", Desc: "jump"})
	v := &stubView{name: "one", handled: map[keys.Action]bool{"test.jump": true}}
	v.onKey = func(act keys.Action, env Env) {
		if act == "test.jump" {
			env.Act.Jump(ids[5])
			env.Act.OpenLayer(state.LayerDialog)
			env.Act.OpenLayer(state.LayerDetail)
		}
	}
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.Keys = km; o.Views = map[int]View{1: v} })
	a.sess.SetCurrent(ids[1])
	press(a, "J")
	if a.sess.Current() != ids[5] || !a.sess.Has(state.LayerDetail) || a.sess.Has(state.LayerDialog) {
		t.Fatalf("current %q detail %v dialog %v", a.sess.Current(), a.sess.Has(state.LayerDetail), a.sess.Has(state.LayerDialog))
	}
	press(a, "backspace")
	if a.sess.Current() != ids[1] {
		t.Errorf("Back after a jump: %q", a.sess.Current())
	}
	send(a, ping{})
	if v.msgs != 1 {
		t.Errorf("view saw %d messages, want 1", v.msgs)
	}
}

func TestDialogsStack(t *testing.T) {
	a := loaded(t, plain, 100, 30, snapOf(t), nil)
	press(a, "t")
	a.pushDialog(&helpDialog{a: a, under: keys.View})
	if len(a.dialogs) != 2 || a.context() != keys.Help {
		t.Fatalf("dialogs %d context %v", len(a.dialogs), a.context())
	}
	press(a, "esc")
	if len(a.dialogs) != 1 || a.context() != keys.Appearance || !a.sess.Has(state.LayerDialog) {
		t.Errorf("Esc must close only the top dialog: %d %v", len(a.dialogs), a.context())
	}
	press(a, "esc")
	if len(a.dialogs) != 0 || a.sess.Has(state.LayerDialog) {
		t.Error("all dialogs closed")
	}
}

func TestFatalReportCancelsOpenDialogs(t *testing.T) {
	a := sample(t, plain, 100, 30)
	before := a.app.Theme.Name
	press(a, "t", "right")
	if a.app.Theme.Name == before {
		t.Fatal("no preview")
	}
	gone := refresh.Status{Loaded: true, Stale: true, LastSuccess: uitest.T0, Failures: 3, Err: &bd.Error{Class: bd.ClassNotWorkspace, Command: "list"}}
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Status: gone}})
	if len(a.dialogs) != 0 || a.app.Theme.Name != before {
		t.Errorf("dialogs %d theme %q, want the cancel path to revert to %q", len(a.dialogs), a.app.Theme.Name, before)
	}
}

func TestRecheckCostsOneBdCall(t *testing.T) {
	fake := bd.NewFake()
	fake.FailWith("Version", &bd.Error{Class: bd.ClassBdMissing, Command: "version", Message: "not found"})
	a := New(testOptions(plain, func(o *Options) { o.Client = fake }))
	send(a, tea.WindowSizeMsg{Width: 80, Height: 24}, a.Init()())
	base := len(fake.Calls())
	msg := a.recheck()()
	if n := len(fake.Calls()) - base; n != 1 {
		t.Errorf("a failing recheck made %d bd calls, want 1", n)
	}
	send(a, msg)
	fake.FailWith("Version", nil)
	msg = a.recheck()()
	if m, ok := msg.(sessionMsg); !ok || m.err != nil {
		t.Errorf("recheck after the fix: %+v", msg)
	}
}

func TestFirstSnapshotFailureShowsCountdown(t *testing.T) {
	b := New(testOptions(plain, nil))
	send(b, tea.WindowSizeMsg{Width: 80, Height: 24}, sessionMsg{sess: workspace()})
	st := refresh.Status{Err: &bd.Error{Class: bd.ClassTransient, Command: "list", Message: "busy"}, Failures: 1, NextRetry: uitest.T0.Add(4 * time.Second)}
	send(b, updateMsg{refresh.Update{Status: st, Session: workspace()}})
	if !strings.Contains(screen(b), "rechecking in 4s") {
		t.Errorf("no countdown:\n%s", screen(b))
	}
}

func TestNoticeDismissedOnlyWhenShown(t *testing.T) {
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.Warnings = []string{"config: bad key"} })
	fail := liveStatus()
	fail.Stale, fail.Err = true, errors.New("boom")
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Status: fail}})
	press(a, "j")
	if len(a.notices) != 1 {
		t.Error("a hidden notice was dismissed by a key")
	}
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Status: liveStatus()}})
	send(a, tea.WindowSizeMsg{Width: 40, Height: 10})
	press(a, "j")
	if len(a.notices) != 1 {
		t.Error("the notice was dismissed on the too-small screen")
	}
	send(a, tea.WindowSizeMsg{Width: 100, Height: 30})
	press(a, "j")
	if len(a.notices) != 0 {
		t.Error("a shown notice must be dismissed by the next key")
	}
}

func TestDetailsScrollKeysAreClamped(t *testing.T) {
	a := sample(t, plain, 100, 30)
	fail := liveStatus()
	fail.Stale, fail.Err = true, errors.New(strings.Repeat("line\n", 60))
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Status: fail}})
	press(a, "!")
	d, ok := a.topDialog().(*detailsDialog)
	if !ok {
		t.Fatal("details did not open")
	}
	screen(a)
	press(a, "j")
	if d.at != 1 {
		t.Errorf("j scrolled to %d", d.at)
	}
	for range 200 {
		press(a, "j")
	}
	screen(a)
	last := d.at
	if want := dialog.MaxScroll(100, 30, len(d.Frame(a.look, 100, 30).Body)); last != want {
		t.Errorf("scroll %d, want it clamped to %d", last, want)
	}
	press(a, "k")
	if d.at != last-1 {
		t.Errorf("k after the end moved to %d, want %d", d.at, last-1)
	}
	press(a, "pgup")
	if d.at >= last-1 {
		t.Error("PgUp did not scroll up")
	}
}

func TestSyncPauseDoesNotRearmTheTick(t *testing.T) {
	a := loaded(t, plain, 100, 30, snapOf(t), nil)
	ids := snapOf(t).IDs()
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Highlights: ids[:2], Status: liveStatus()}})
	gen := a.hlGen
	press(a, "j", "k", "space")
	if a.hlGen != gen {
		t.Errorf("hlGen %d -> %d: keys re-armed an unchanged deadline", gen, a.hlGen)
	}
	send(a, tea.BlurMsg{})
	if a.hlGen == gen {
		t.Error("blur must cancel the armed tick")
	}
}
