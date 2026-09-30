package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/notify"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

type fakeBoard struct {
	mu    sync.Mutex
	texts []string
	res   clipboard.Result
}

func (f *fakeBoard) Write(_ context.Context, text string) clipboard.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.texts = append(f.texts, text)
	return f.res
}

func (f *fakeBoard) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.texts...)
}

type fakeNotifier struct {
	mu   sync.Mutex
	sent [][]notify.Message
	d    notify.Delivery
}

func (f *fakeNotifier) Send(_ context.Context, msgs []notify.Message) notify.Delivery {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msgs)
	return f.d
}

func (f *fakeNotifier) got() [][]notify.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]notify.Message(nil), f.sent...)
}

func lastNotice(a *App) string {
	if len(a.notices) == 0 {
		return ""
	}
	return a.notices[len(a.notices)-1].Text
}

func hasMsg(msgs []tea.Msg, typeName string) bool {
	for _, m := range msgs {
		if strings.Contains(fmt.Sprintf("%T", m), typeName) {
			return true
		}
	}
	return false
}

func copyRig(t testing.TB, board Clipboard) *App {
	t.Helper()
	snap, _ := uitest.Sample()
	a := loaded(t, plain, 100, 30, snap, func(o *Options) { o.Clipboard = board })
	a.sess.SetCurrent("ws-9qe")
	return a
}

func TestYCopiesTheCurrentID(t *testing.T) {
	board := &fakeBoard{res: clipboard.Result{Copied: []string{"wl-copy"}}}
	a := copyRig(t, board)
	cmd := send(a, keyMsg("y"))
	msgs := collect(cmd, time.Second)
	if !hasMsg(msgs, "lipboard") {
		t.Errorf("no OSC 52 message in %v", msgs)
	}
	for _, m := range msgs {
		send(a, m)
	}
	if got := board.got(); len(got) != 1 || got[0] != "ws-9qe" {
		t.Errorf("board got %v", got)
	}
	if got := lastNotice(a); got != "ws-9qe: copied to wl-copy; sent via OSC 52" {
		t.Errorf("notice = %q", got)
	}
}

func TestCopyWithoutBoardSaysSentOnly(t *testing.T) {
	a := copyRig(t, nil)
	send(a, keyMsg("y"))
	if got := lastNotice(a); got != "ws-9qe: sent via OSC 52" {
		t.Errorf("notice = %q", got)
	}
}

func TestCopyFailureIsAWarning(t *testing.T) {
	board := &fakeBoard{res: clipboard.Result{Failures: []clipboard.Failure{{Route: "xclip", Err: errors.New("no display")}}}}
	a := copyRig(t, board)
	for _, m := range collect(send(a, keyMsg("y")), time.Second) {
		send(a, m)
	}
	n := a.notices[len(a.notices)-1]
	if !n.Warn || !strings.Contains(n.Text, "xclip failed: no display") || !strings.Contains(n.Text, "sent via OSC 52") {
		t.Errorf("notice = %+v", n)
	}
}

func TestCopyCommand(t *testing.T) {
	board := &fakeBoard{}
	a := copyRig(t, board)
	r := &writeRig{t: t, a: a, eng: &syncEngine{}}
	r.line("copy")
	if got := board.got(); len(got) != 1 || got[0] != "ws-9qe" {
		t.Errorf("board got %v", got)
	}
}

func TestMemoriesCopyTheContent(t *testing.T) {
	board := &fakeBoard{}
	r := goldenMemRig(t, 120, 30)
	r.a.o.Clipboard = board
	r.key("y")
	r.line("copy")
	got := board.got()
	if len(got) != 2 || got[0] != got[1] || !strings.Contains(got[0], " ") || strings.HasPrefix(got[0], "ws-") {
		t.Errorf("board got %q", got)
	}
	mem, _ := r.a.mem.current()
	if got[0] != mem.Content {
		t.Errorf("copied %q, want the content %q", got[0], mem.Content)
	}
}

func TestCopyOverTheCapCopiesNothing(t *testing.T) {
	board := &fakeBoard{}
	a := copyRig(t, board)
	cmd := a.copy("memory big", strings.Repeat("x", clipboard.MaxBytes+1))
	if cmd != nil || len(board.got()) != 0 {
		t.Errorf("an oversized copy ran: %v", board.got())
	}
	n := a.notices[len(a.notices)-1]
	if !n.Warn || !strings.Contains(n.Text, "too large") {
		t.Errorf("notice = %+v", n)
	}
}

func TestCopyWaitsOutATitleChange(t *testing.T) {
	a := copyRig(t, nil)
	a.titleAt = a.now()
	start := time.Now()
	if msgs := collect(a.copy("x", "text"), 5*time.Second); !hasMsg(msgs, "lipboard") {
		t.Fatalf("copy never ran: %v", msgs)
	}
	if held := time.Since(start); held < titleGuard*9/10 {
		t.Errorf("copy ran after %v, in the frame of the title change", held)
	}
	a.titleAt = a.now().Add(-time.Second)
	if msgs := collect(a.copy("x", "text"), time.Second); !hasMsg(msgs, "lipboard") {
		t.Errorf("an old title change held the copy back")
	}
}

func TestTitleChangeIsTrackedInUpdate(t *testing.T) {
	a := copyRig(t, nil)
	if a.title == "" || a.titleAt.IsZero() {
		t.Fatalf("title not tracked after Update: %q", a.title)
	}
	first := a.titleAt
	a.View()
	a.syncTitle()
	if !a.titleAt.Equal(first) {
		t.Error("an unchanged title moved the mark")
	}
}

func TestTitleChangeWaitsOutACopy(t *testing.T) {
	a := copyRig(t, nil)
	old := a.title
	a.copyAt = a.now()
	a.bds.Workspace.Path = "/x/other"
	cmd := a.syncTitle()
	if a.title != old || cmd == nil {
		t.Fatalf("title changed within the guard of a copy: %q, cmd %v", a.title, cmd != nil)
	}
	if a.syncTitle() != nil {
		t.Error("a second tick was scheduled")
	}
	if got := a.View().WindowTitle; got != old {
		t.Errorf("view shows %q, want the held %q", got, old)
	}
	if !hasMsg(collect(cmd, 5*time.Second), "titleHeldMsg") {
		t.Fatal("no tick after the guard")
	}
	a.copyAt = a.now().Add(-time.Second)
	send(a, titleHeldMsg{})
	if a.title == old || !strings.Contains(a.title, "other") {
		t.Errorf("title after the guard = %q", a.title)
	}
}

func notification(kinds ...model.Kind) model.Notification {
	var n model.Notification
	for i, k := range kinds {
		n.Events = append(n.Events, model.Event{Kind: k, IssueID: fmt.Sprintf("ws-%d", i), Title: "T", Actor: "ann"})
	}
	n.Summary = len(kinds) > model.SummaryThreshold
	return n
}

func notifyRig(t testing.TB, nf Notifier, mod func(*Options)) (*App, *syncEngine) {
	t.Helper()
	snap, _ := uitest.Sample()
	a := loaded(t, plain, 100, 30, snap, func(o *Options) {
		o.Notifier = nf
		if mod != nil {
			mod(o)
		}
	})
	eng := &syncEngine{}
	a.eng = eng
	return a, eng
}

func TestNotificationIsDeliveredWhenOn(t *testing.T) {
	nf := &fakeNotifier{d: notify.Delivery{Route: "notify-send"}}
	a, _ := notifyRig(t, nf, nil)
	cmd := send(a, updateMsg{refresh.Update{Status: liveStatus(), Notification: notification(model.KindClosed)}})
	for _, m := range collect(cmd, time.Second) {
		send(a, m)
	}
	sent := nf.got()
	if len(sent) != 1 || len(sent[0]) != 1 || !strings.Contains(sent[0][0].Title, "ws-0") {
		t.Fatalf("sent = %+v", sent)
	}
}

func TestNotificationsOffDeliverNothing(t *testing.T) {
	nf := &fakeNotifier{}
	a, _ := notifyRig(t, nf, func(o *Options) { o.Settings.Settings.NotifyMethod = "off" })
	cmd := send(a, updateMsg{refresh.Update{Status: liveStatus(), Notification: notification(model.KindClosed)}})
	if msgs := collect(cmd, 50*time.Millisecond); hasMsg(msgs, "notifiedMsg") || len(nf.got()) != 0 {
		t.Errorf("delivered while off: %v", nf.got())
	}
}

func TestManyEventsBecomeOneSummary(t *testing.T) {
	nf := &fakeNotifier{d: notify.Delivery{Route: "notify-send"}}
	a, _ := notifyRig(t, nf, nil)
	n := notification(model.KindClosed, model.KindClosed, model.KindBecameBlocked, model.KindBecameReady)
	for _, m := range collect(send(a, updateMsg{refresh.Update{Status: liveStatus(), Notification: n}}), time.Second) {
		send(a, m)
	}
	if sent := nf.got(); len(sent) != 1 || len(sent[0]) != 1 || !strings.Contains(sent[0][0].Title, "4 changes") {
		t.Errorf("sent = %+v", sent)
	}
}

func TestBellDeliveryShowsTheFooter(t *testing.T) {
	nf := &fakeNotifier{d: notify.Delivery{Route: notify.RouteBell, Bell: true, Err: errors.New("notify-send: no daemon")}}
	a, _ := notifyRig(t, nf, nil)
	for _, m := range collect(send(a, updateMsg{refresh.Update{Status: liveStatus(), Notification: notification(model.KindClosed)}}), time.Second) {
		send(a, m)
	}
	var texts []string
	for _, n := range a.notices {
		texts = append(texts, n.Text)
	}
	all := strings.Join(texts, "|")
	if !strings.Contains(all, "bdash: ws-0 closed") || !strings.Contains(all, "no daemon") {
		t.Errorf("notices = %q", all)
	}
}

func TestNoNotifierRingsTheBell(t *testing.T) {
	a, _ := notifyRig(t, nil, nil)
	msgs := collect(send(a, updateMsg{refresh.Update{Status: liveStatus(), Notification: notification(model.KindClosed)}}), time.Second)
	var got *notifiedMsg
	for _, m := range msgs {
		if n, ok := m.(notifiedMsg); ok {
			got = &n
		}
	}
	if got == nil || !got.d.Bell {
		t.Fatalf("no bell delivery in %v", msgs)
	}
}

func TestTerminalDeliveryWritesRaw(t *testing.T) {
	a, _ := notifyRig(t, nil, nil)
	const seq = "\x1b]777;notify;a;b\x07"
	cmd := a.onNotified(notifiedMsg{d: notify.Delivery{Route: notify.RouteTerminal, Raw: seq}})
	var raws []tea.RawMsg
	for _, m := range collect(cmd, time.Second) {
		if r, ok := m.(tea.RawMsg); ok {
			raws = append(raws, r)
		}
	}
	if len(raws) != 1 || raws[0].Msg != seq {
		t.Errorf("raw writes = %#v, want the sequence once", raws)
	}
}

func TestWindowsDesktopHintShowsOnce(t *testing.T) {
	a, _ := notifyRig(t, nil, nil)
	d := notify.Delivery{Route: notify.RouteBell, Bell: true, Hint: "desktop notifications are not available on Windows; use terminal or bell"}
	a.onNotified(notifiedMsg{d: d})
	a.onNotified(notifiedMsg{d: d})
	n := 0
	for _, x := range a.notices {
		if strings.Contains(x.Text, "not available on Windows") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("hint shown %d times", n)
	}
}

func TestNotifyOnRestoresThePreviousMethod(t *testing.T) {
	store := &memStore{}
	a, _ := notifyRig(t, nil, func(o *Options) {
		o.Store = store
		o.Settings.Settings.NotifyMethod = notify.MethodBell
	})
	for _, on := range []bool{false, true} {
		for _, m := range collect(a.setNotify(on, []string{"closed"}), time.Second) {
			send(a, m)
		}
	}
	var methods []string
	for _, s := range store.sets {
		if s[0] == "notify.method" {
			methods = append(methods, s[1])
		}
	}
	if fmt.Sprint(methods) != "[off bell]" || a.notifyMethod != notify.MethodBell {
		t.Errorf("saved %v, session method %q", methods, a.notifyMethod)
	}
}

func TestNotifyDialogTogglesAndSaves(t *testing.T) {
	store := &memStore{}
	a, eng := notifyRig(t, nil, func(o *Options) { o.Store = store })
	press(a, "N")
	if !isOpen[*notifyDialog](a) {
		t.Fatal("N did not open the dialog")
	}
	press(a, "space")
	press(a, "down", "down", "down", "down", "space")
	cmd := send(a, keyMsg("enter"))
	for _, m := range collect(cmd, time.Second) {
		send(a, m)
	}
	if a.notifyOn {
		t.Error("notifications still on")
	}
	if len(eng.notify) == 0 || eng.notify[len(eng.notify)-1].on {
		t.Errorf("engine calls = %+v", eng.notify)
	}
	want := [][2]string{{"notify.method", "off"}, {"notify.kinds", "[blocked ready]"}}
	if len(store.sets) != 2 || store.sets[0] != want[0] || store.sets[1] != want[1] {
		t.Errorf("saved %v, want %v", store.sets, want)
	}
}

func TestNotifyDialogEscKeepsTheSettings(t *testing.T) {
	a, eng := notifyRig(t, nil, nil)
	press(a, "N", "space", "esc")
	if !a.notifyOn || len(eng.notify) != 0 {
		t.Errorf("cancel changed the state: %v %v", a.notifyOn, eng.notify)
	}
}

func TestNotifyCommand(t *testing.T) {
	store := &memStore{}
	a, eng := notifyRig(t, nil, func(o *Options) { o.Store = store })
	r := &writeRig{t: t, a: a, eng: eng}
	r.line("notify off")
	if a.notifyOn || eng.notify[len(eng.notify)-1].on {
		t.Error("notify off did not switch off")
	}
	r.line("notify on")
	if !a.notifyOn || !eng.notify[len(eng.notify)-1].on {
		t.Error("notify on did not switch on")
	}
	if last := store.sets[len(store.sets)-2]; last != [2]string{"notify.method", "auto"} {
		t.Errorf("saved %v", store.sets)
	}
	r.line("notify")
	if !isOpen[*notifyDialog](a) {
		t.Error("bare :notify did not open the dialog")
	}
}

func TestEngineStartCarriesTheNotifyState(t *testing.T) {
	a, eng := notifyRig(t, nil, func(o *Options) { o.Settings.Settings.NotifyKinds = []string{"closed"} })
	a.onEngine(engineMsg{eng: eng, cancel: func() {}})
	if len(eng.notify) != 1 || !eng.notify[0].on || !eng.notify[0].kinds.Has(model.KindClosed) || eng.notify[0].kinds.Has(model.KindBecameReady) {
		t.Errorf("engine calls = %+v", eng.notify)
	}
}

func TestNotifyDialogGoldens(t *testing.T) {
	for _, sz := range formSizes {
		t.Run(fmt.Sprintf("%dx%d", sz[0], sz[1]), func(t *testing.T) {
			a, _ := notifyRig(t, nil, nil)
			send(a, tea.WindowSizeMsg{Width: sz[0], Height: sz[1]})
			press(a, "N", "down", "down", "space")
			testgolden.Equal(t, screen(a))
		})
	}
	t.Run("off/80x24", func(t *testing.T) {
		a, _ := notifyRig(t, nil, nil)
		send(a, tea.WindowSizeMsg{Width: 80, Height: 24})
		press(a, "N", "space")
		testgolden.Equal(t, screen(a))
	})
}

func TestEngineDrivenNotificationsSkipOwnWrites(t *testing.T) {
	fake := treeFake(t)
	nf := &fakeNotifier{d: notify.Delivery{Route: "notify-send"}}
	a := New(testOptions(plain, func(o *Options) {
		withView("tree", false)(o)
		o.Client, o.Notifier, o.Actor = fake, nf, "me"
	}))
	send(a, tea.WindowSizeMsg{Width: 120, Height: 60})
	p := newPump(t, a)
	p.spawn(a.Init())
	p.until(func() bool { return a.snap != nil && a.eng != nil })
	t.Cleanup(func() {
		a.eng.Stop()
		a.cancel()
	})
	setClosed := func(id string) {
		issues := fake.Issues()
		for i := range issues {
			if issues[i].ID == id {
				issues[i].Status, issues[i].ClosedAt = "closed", uitest.T0
			}
		}
		fake.SetIssues(issues...)
	}

	err := a.eng.Write(context.Background(), []string{"ws-9qe"}, func(context.Context, bd.Client) error {
		setClosed("ws-9qe")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	setClosed("ws-4k2.5")
	a.eng.Refresh()
	p.until(func() bool { return len(nf.got()) > 0 })
	for _, msgs := range nf.got() {
		for _, m := range msgs {
			if strings.Contains(m.Title, "ws-9qe") {
				t.Errorf("own write was announced: %+v", m)
			}
		}
	}
	if !strings.Contains(nf.got()[0][0].Title, "ws-4k2.5") {
		t.Errorf("sent = %+v", nf.got())
	}
}
