package ui

import (
	"bytes"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/screens"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

type memStore struct {
	mu   sync.Mutex
	sets [][2]string
	err  error
}

func (m *memStore) Set(name string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sets = append(m.sets, [2]string{name, fmt.Sprint(value)})
	return m.err
}

func TestAppearanceDialogGoldens(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {80, 24}} {
		a := sample(t, plain, size[0], size[1])
		press(a, "t")
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			testgolden.Equal(t, screen(a))
		})
	}
}

func TestAppearancePreviewRevertAndPersist(t *testing.T) {
	store := &memStore{}
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.Store = store })
	press(a, "t", "right")
	if a.app.Theme.Name == "default" {
		t.Fatal("no live preview of the theme")
	}
	press(a, "esc")
	if a.app.Theme.Name != "default" || len(a.dialogs) != 0 || len(store.sets) != 0 {
		t.Errorf("Esc must revert: theme %q dialogs %d sets %v", a.app.Theme.Name, len(a.dialogs), store.sets)
	}

	press(a, "t", "right")
	chosen := a.app.Theme.Name
	cmd := send(a, keyMsg("enter"))
	if a.app.Theme.Name != chosen || len(a.dialogs) != 0 {
		t.Errorf("Enter keeps the choice: %q", a.app.Theme.Name)
	}
	runAll(cmd)
	if len(store.sets) != 1 || store.sets[0] != [2]string{config.KeyTheme, chosen} {
		t.Errorf("persisted %v", store.sets)
	}
}

func runAll(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch m := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range m {
			runAll(c)
		}
	case tea.Msg:
		_ = m
	}
}

func TestAppearanceSaveFailureNotice(t *testing.T) {
	store := &memStore{err: errors.New("read-only")}
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.Store = store })
	press(a, "t", "right")
	cmd := a.act(keys.Apply, "enter")
	if cmd == nil {
		t.Fatal("applying a changed choice must persist it")
	}
	msg := cmd()
	if len(store.sets) == 0 {
		t.Fatal("the store was not written")
	}
	send(a, msg)
	if !strings.Contains(screen(a), "could not save the choice: read-only") {
		t.Errorf("no notice:\n%s", screen(a))
	}
}

func TestAppearanceOverrideNote(t *testing.T) {
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) {
		o.Settings.Origins = map[string]config.Origin{config.KeyGlyphs: {Kind: config.OriginEnv, Name: "BDASH_GLYPHS"}}
	})
	press(a, "t")
	if !strings.Contains(screen(a), "overridden by BDASH_GLYPHS") {
		t.Errorf("no override note:\n%s", screen(a))
	}
}

func TestStartupFailureScreenAndRecheck(t *testing.T) {
	fake := bd.NewFake()
	fake.FailWith("Version", &bd.Error{Class: bd.ClassBdMissing, Command: "version", Message: "not found"})
	a := New(testOptions(plain, func(o *Options) { o.Client = fake }))
	cmd := a.Init()
	msg := cmd()
	cmd = send(a, tea.WindowSizeMsg{Width: 80, Height: 24}, msg)
	if cmd == nil || !strings.Contains(screen(a), "bd is not installed") {
		t.Fatalf("no failure screen:\n%s", screen(a))
	}
	if !strings.Contains(screen(a), "rechecking in 2s") {
		t.Errorf("no countdown:\n%s", screen(a))
	}
	press(a, "1", "j", "t")
	if len(a.dialogs) != 0 || a.slot != 0 {
		t.Error("shell keys leaked into the startup screen")
	}

	fake.FailWith("Version", nil)
	fake.SetVersion("1.2.3")
	fake.SetWorkspace(bd.Workspace{Path: "/work/demo/.beads"})
	press(a, "r")
	if !a.checking {
		t.Fatal("r must recheck now")
	}
}

func TestStartupRecheckBackoff(t *testing.T) {
	a := New(testOptions(plain, nil))
	e := &bd.Error{Class: bd.ClassBdMissing}
	for i, want := range []time.Duration{2, 4, 8, 16, 30, 30} {
		send(a, sessionMsg{err: e})
		if got := a.nextCheck.Sub(uitest.T0); got != want*time.Second {
			t.Errorf("failure %d: %v, want %vs", i+1, got, want)
		}
	}
}

func TestFirstSnapshotFailedAndVanishedWorkspace(t *testing.T) {
	a := New(testOptions(plain, nil))
	send(a, tea.WindowSizeMsg{Width: 80, Height: 24}, sessionMsg{sess: workspace()})
	send(a, updateMsg{refresh.Update{Status: refresh.Status{Err: &bd.Error{Class: bd.ClassTransient, Command: "list"}, Failures: 1}, Session: workspace()}})
	if a.report == nil || !strings.Contains(screen(a), "couldn't read this workspace") {
		t.Fatalf("no first-snapshot screen:\n%s", screen(a))
	}
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Status: liveStatus()}})
	if a.report != nil {
		t.Error("screen stays after a snapshot arrived")
	}

	gone := refresh.Status{Loaded: true, Stale: true, LastSuccess: uitest.T0, Err: &bd.Error{Class: bd.ClassNotWorkspace, Command: "list"}, Failures: 3}
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Status: gone}})
	if a.report == nil || a.report.Kind != screens.Vanished || !strings.Contains(screen(a), "workspace is gone") {
		t.Errorf("no vanished screen:\n%s", screen(a))
	}
	if a.snap != nil {
		t.Error("the snapshot survived the vanished workspace")
	}
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Status: gone}})
	if a.report == nil || a.snap != nil {
		t.Error("a repeated failing update must neither clear the screen nor bring the snapshot back")
	}
	send(a, updateMsg{refresh.Update{Snapshot: snapOf(t), Status: liveStatus()}})
	if a.report != nil || a.snap == nil {
		t.Error("a successful refresh must bring the workspace back")
	}
}

func TestUntestedNoticeShownOnce(t *testing.T) {
	a := New(testOptions(plain, nil))
	ws := workspace()
	ws.Untested = true
	ws.Version = bd.VersionInfo{Raw: "1.4.0", Parsed: bd.Version{Major: 1, Minor: 4}}
	send(a, tea.WindowSizeMsg{Width: 120, Height: 30}, sessionMsg{sess: ws},
		updateMsg{refresh.Update{Snapshot: snapOf(t), Status: liveStatus(), Session: ws}})
	if !strings.Contains(screen(a), "newer than the tested 1.3 line") || !strings.Contains(lines(a)[0], "bd 1.4.0 untested") {
		t.Errorf("notice or chip missing:\n%s", screen(a))
	}
	press(a, "j")
	if strings.Contains(screen(a), "newer than the tested") {
		t.Error("notice not one-time")
	}
}

func TestSchemeEventsReachAppearance(t *testing.T) {
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) {
		o.Appearance.TrackScheme = true
		o.Appearance.Background = theme.BackgroundAuto
		o.Appearance.ProbedDark, o.Appearance.Dark = true, true
	})
	if cmd := send(a, tea.KeyPressMsg{}); cmd != nil {
		t.Error("unrelated messages produce no command")
	}
	send(a, tea.BackgroundColorMsg{Color: color.White})
	if a.app.Dark {
		t.Error("a light background report must switch the palette to light")
	}
	send(a, tea.BackgroundColorMsg{Color: color.Black})
	if !a.app.Dark {
		t.Error("a dark background report must switch the palette back")
	}
}

func fakeWorkspace(t testing.TB) *bd.Fake {
	t.Helper()
	fake := bd.NewFake()
	fake.SetVersion("1.2.3")
	fake.SetWorkspace(bd.Workspace{Path: "/work/demo/.beads", Prefix: "ws"})
	snap, _ := uitest.Sample()
	var issues []model.Issue
	for _, id := range snap.IDs() {
		is, _ := snap.Issue(id)
		issues = append(issues, *is)
	}
	fake.SetIssues(issues...)
	fake.SetReadiness([]string{"ws-4k2.1", "ws-9qe"}, nil)
	return fake
}

func TestTeatestFlow(t *testing.T) {
	var eng atomic.Pointer[refresh.Engine]
	fake := fakeWorkspace(t)
	a := New(testOptions(plain, func(o *Options) {
		o.Client, o.Now = fake, time.Now
		o.NewEngine = func(s bd.Session) Engine {
			e := refresh.New(refresh.Options{Client: fake, Session: s})
			eng.Store(e)
			return e
		}
	}))
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(100, 30))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Checkout: guest orders"))
	}, teatest.WithDuration(10*time.Second))

	waitFocus := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for eng.Load().Status().Focused != want {
			if time.Now().After(deadline) {
				t.Fatalf("engine focus never became %v", want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	tm.Send(tea.BlurMsg{})
	waitFocus(false)
	tm.Send(tea.FocusMsg{})
	waitFocus(true)
	tm.Send(keyMsg("j"))
	tm.Send(keyMsg("q"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(10*time.Second)).(*App)
	if !ok {
		t.Fatal("final model is not *ui.App")
	}
	if !final.quitting || !final.focused || final.snap == nil {
		t.Errorf("quitting %v focused %v snapshot %v", final.quitting, final.focused, final.snap)
	}
}

func BenchmarkView200x50Cold(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		snap, _ := uitest.Big(5000)
		a := loaded(b, truecolor, 200, 50, snap, nil)
		press(a, "space", "j", "space")
		b.StartTimer()
		a.View()
	}
}

func BenchmarkView200x50(b *testing.B) {
	snap, _ := uitest.Big(5000)
	a := loaded(b, truecolor, 200, 50, snap, nil)
	press(a, "space", "j", "space")
	send(a, updateMsg{refresh.Update{Snapshot: snap, Highlights: snap.IDs()[:20], Status: liveStatus()}})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.View()
	}
}

func TestViewStaysWithinBudget(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("timing")
	}
	res := testing.Benchmark(BenchmarkView200x50)
	if per := time.Duration(res.NsPerOp()); per > 8*time.Millisecond {
		t.Errorf("warm View at 200x50 took %v, budget 8ms", per)
	}
	var took []time.Duration
	for range 10 {
		snap, _ := uitest.Big(5000)
		a := loaded(t, truecolor, 200, 50, snap, nil)
		press(a, "space", "j", "space")
		start := time.Now()
		a.View()
		took = append(took, time.Since(start))
	}
	slices.Sort(took)
	if median := took[len(took)/2]; median > 8*time.Millisecond {
		t.Errorf("cold View at 200x50 took %v (median), budget 8ms", median)
	}
}
