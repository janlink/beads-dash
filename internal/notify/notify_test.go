package notify_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/host"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/notify"
	"github.com/janlink/beads-dash/internal/proc"
)

func ev(kind model.Kind, id, title string) model.Event {
	return model.Event{Kind: kind, IssueID: id, Title: title}
}

func TestComposeOneMessagePerEventUpToTheThreshold(t *testing.T) {
	n := model.Notify([]model.Event{
		{Kind: model.KindClosed, IssueID: "ws-1", Title: "Fix login", Actor: "ann"},
		ev(model.KindBecameReady, "ws-2", "Ship it"),
	}, model.KindSetOf("closed", "ready"))
	got := notify.Compose(n)
	if len(got) != 2 {
		t.Fatalf("messages = %+v", got)
	}
	if got[0].Title != "bdash: ws-1 closed" || got[0].Body != "Fix login (ann)" {
		t.Errorf("first = %+v", got[0])
	}
	if got[1].Title != "bdash: ws-2 became ready" || got[1].Body != "Ship it" {
		t.Errorf("second = %+v", got[1])
	}
	if got := notify.Compose(model.Notification{}); got != nil {
		t.Errorf("Compose(empty) = %+v", got)
	}
}

func TestComposeSummarisesMoreThanThreeEvents(t *testing.T) {
	var evs []model.Event
	for _, k := range []model.Kind{model.KindClosed, model.KindClosed, model.KindBecameBlocked, model.KindBecameReady, model.KindBecameReady} {
		evs = append(evs, ev(k, "ws-x", "t"))
	}
	n := model.Notify(evs, model.KindSetOf("closed", "blocked", "ready"))
	got := notify.Compose(n)
	if len(got) != 1 {
		t.Fatalf("messages = %+v", got)
	}
	if got[0].Title != "bdash: 5 changes" || got[0].Body != "2 closed, 1 became blocked, 2 became ready" {
		t.Errorf("summary = %+v", got[0])
	}
	if got[0].Line() != "bdash: 5 changes: 2 closed, 1 became blocked, 2 became ready" {
		t.Errorf("line = %q", got[0].Line())
	}
}

func TestComposeKindFilterAndOwnWritesNeverReachTheMessages(t *testing.T) {
	evs := []model.Event{
		ev(model.KindEdited, "a", "x"),
		{Kind: model.KindClosed, IssueID: "b", Own: true},
		{Kind: model.KindClosed, IssueID: "c", Prefill: true},
		ev(model.KindClosed, "d", "kept"),
	}
	got := notify.Compose(model.Notify(evs, model.KindSetOf("closed")))
	if len(got) != 1 || got[0].Title != "bdash: d closed" {
		t.Errorf("messages = %+v", got)
	}
}

var one = []notify.Message{{Title: "bdash: ws-1 closed", Body: "Fix login"}}

func send(t *testing.T, env host.Env, method string, fake *proc.Fake, msgs []notify.Message) notify.Delivery {
	t.Helper()
	return notify.New(env, fake, method).Send(context.Background(), msgs)
}

func TestDesktopBackendSelection(t *testing.T) {
	tests := []struct {
		name     string
		env      host.Env
		programs []string
		want     string
	}{
		{"linux", host.Env{GOOS: "linux"}, []string{"notify-send"}, "notify-send"},
		{"linux without notify-send", host.Env{GOOS: "linux"}, nil, ""},
		{"macos", host.Env{GOOS: "darwin"}, []string{"osascript", "notify-send"}, "osascript"},
		{"windows has none", host.Env{GOOS: "windows"}, []string{"notify-send", "osascript"}, ""},
		{"wsl has none", host.Env{GOOS: "linux", WSL: true}, []string{"notify-send"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := notify.New(tc.env, proc.NewFake(tc.programs...), notify.MethodAuto)
			if n.Backend() != tc.want {
				t.Errorf("backend = %q, want %q", n.Backend(), tc.want)
			}
		})
	}
}

func TestNotifySendCommandHasNoShell(t *testing.T) {
	fake := proc.NewFake("notify-send")
	d := send(t, host.Env{GOOS: "linux"}, notify.MethodAuto, fake, one)
	if d.Route != "notify-send" || d.Bell || d.Raw != "" {
		t.Errorf("delivery = %+v", d)
	}
	calls := fake.Calls()
	if len(calls) != 1 || calls[0].Line() != "/fake/notify-send --app-name=bdash -- bdash: ws-1 closed Fix login" {
		t.Errorf("calls = %+v", calls)
	}
}

func TestOsascriptTakesTextAsArguments(t *testing.T) {
	fake := proc.NewFake("osascript")
	msg := []notify.Message{{Title: `t"; do shell script "x`, Body: `b\`}}
	d := send(t, host.Env{GOOS: "darwin"}, notify.MethodAuto, fake, msg)
	if d.Route != "osascript" {
		t.Fatalf("delivery = %+v", d)
	}
	argv := fake.Calls()[0].Argv
	if argv[len(argv)-2] != msg[0].Title || argv[len(argv)-1] != msg[0].Body {
		t.Errorf("argv = %q", argv)
	}
	for _, a := range argv[1 : len(argv)-2] {
		if strings.Contains(a, msg[0].Title) {
			t.Errorf("text spliced into the script: %q", a)
		}
	}
}

func TestDesktopFailureWithoutExitCodeIsReported(t *testing.T) {
	fake := proc.NewFake("notify-send")
	fake.FailWith("/fake/notify-send", errors.New("boom"))
	d := send(t, host.Env{GOOS: "linux"}, notify.MethodAuto, fake, one)
	if !d.Bell || d.Err == nil || !strings.Contains(d.Err.Error(), "notify-send: boom") {
		t.Errorf("delivery = %+v", d)
	}
}

func TestMethodOrder(t *testing.T) {
	linux := host.Env{GOOS: "linux"}
	tests := []struct {
		name      string
		env       host.Env
		method    string
		programs  []string
		route     string
		bell, raw bool
	}{
		{"off", linux, notify.MethodOff, []string{"notify-send"}, notify.RouteNone, false, false},
		{"bell", linux, notify.MethodBell, []string{"notify-send"}, notify.RouteBell, true, false},
		{"desktop", linux, notify.MethodDesktop, []string{"notify-send"}, "notify-send", false, false},
		{"desktop missing", linux, notify.MethodDesktop, nil, notify.RouteBell, true, false},
		{"auto local", linux, notify.MethodAuto, []string{"notify-send"}, "notify-send", false, false},
		{"auto over ssh uses the terminal", host.Env{GOOS: "linux", SSH: true}, notify.MethodAuto, []string{"notify-send"}, notify.RouteTerminal, false, true},
		{"terminal", linux, notify.MethodTerminal, []string{"notify-send"}, notify.RouteTerminal, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := send(t, tc.env, tc.method, proc.NewFake(tc.programs...), one)
			if d.Route != tc.route || d.Bell != tc.bell || (d.Raw != "") != tc.raw {
				t.Errorf("delivery = %+v", d)
			}
		})
	}
	if d := send(t, linux, notify.MethodAuto, proc.NewFake("notify-send"), nil); d.Route != notify.RouteNone {
		t.Errorf("no messages: %+v", d)
	}
}

func TestTerminalSequencePerTerminal(t *testing.T) {
	msg := []notify.Message{{Title: "a;b\x1b", Body: "c\nd"}}
	tests := []struct {
		name string
		env  host.Env
		want string
	}{
		{"default osc 777", host.Env{GOOS: "linux"}, "\x1b]777;notify;a,b ;c d\x07"},
		{"wezterm", host.Env{GOOS: "linux", TermProgram: "WezTerm"}, "\x1b]777;notify;a,b ;c d\x07"},
		{"iterm osc 9", host.Env{GOOS: "darwin", TermProgram: "iTerm.app"}, "\x1b]9;a,b : c d\x07"},
		{"kitty osc 99", host.Env{GOOS: "linux", Kitty: true}, "\x1b]99;i=1:d=0:p=title;a,b \x07\x1b]99;i=1:d=1:p=body;c d\x07"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := send(t, tc.env, notify.MethodTerminal, proc.NewFake(), msg)
			if d.Raw != tc.want {
				t.Errorf("raw = %q, want %q", d.Raw, tc.want)
			}
		})
	}
}

func TestTmuxWrapsOnlyWithAllowPassthrough(t *testing.T) {
	env := host.Env{GOOS: "linux", Tmux: true, TmuxPane: "%7"}
	for _, val := range []string{"on\n", "all\n"} {
		fake := proc.NewFake("tmux")
		fake.Output("/fake/tmux show -Apv -t %7 allow-passthrough", val)
		d := send(t, env, notify.MethodTerminal, fake, one)
		want := "\x1bPtmux;\x1b\x1b]777;notify;bdash: ws-1 closed;Fix login\x07\x1b\\"
		if d.Raw != want {
			t.Errorf("allow-passthrough %q: raw = %q, want %q", strings.TrimSpace(val), d.Raw, want)
		}
	}

	fake := proc.NewFake("tmux", "notify-send")
	fake.Output("/fake/tmux show -Apv -t %7 allow-passthrough", "off\n")
	d := send(t, env, notify.MethodTerminal, fake, one)
	if d.Raw != "" || d.Route != "notify-send" {
		t.Errorf("passthrough off should fall back to desktop: %+v", d)
	}

	d = send(t, host.Env{GOOS: "linux", Tmux: true, SSH: true}, notify.MethodAuto, proc.NewFake("tmux"), one)
	if d.Raw != "" || !d.Bell {
		t.Errorf("no passthrough and no desktop should bell: %+v", d)
	}
}

func TestPassthroughIsAskedOnce(t *testing.T) {
	fake := proc.NewFake("tmux")
	fake.Output("/fake/tmux show -Apv", "on\n")
	n := notify.New(host.Env{GOOS: "linux", Tmux: true}, fake, notify.MethodTerminal)
	n.Send(context.Background(), one)
	n.Send(context.Background(), one)
	if got := len(fake.Calls()); got != 1 {
		t.Errorf("tmux asked %d times", got)
	}
}

func TestZellijGetsRawSequences(t *testing.T) {
	d := send(t, host.Env{GOOS: "linux", Zellij: true}, notify.MethodTerminal, proc.NewFake(), one)
	if !strings.HasPrefix(d.Raw, "\x1b]777;") {
		t.Errorf("raw = %q", d.Raw)
	}
}

func TestScreenWrapsInChunks(t *testing.T) {
	long := []notify.Message{{Title: "bdash: ws-1 closed", Body: strings.Repeat("x", 1000)}}
	d := send(t, host.Env{GOOS: "linux", Screen: true}, notify.MethodTerminal, proc.NewFake(), long)
	if !strings.HasPrefix(d.Raw, "\x1bP") || !strings.HasSuffix(d.Raw, "\x1b\\") || strings.Count(d.Raw, "\x1b\\\x1bP") != 1 {
		t.Fatalf("raw = %q", d.Raw)
	}
	for _, chunk := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(d.Raw, "\x1bP"), "\x1b\\"), "\x1b\\\x1bP") {
		if len(chunk) > 768 {
			t.Errorf("chunk of %d bytes", len(chunk))
		}
	}
}

func TestDesktopFailureLocksTheDesktopForTheSession(t *testing.T) {
	fake := proc.NewFake("notify-send")
	fake.FailWith("/fake/notify-send", errors.New("boom"))
	n := notify.New(host.Env{GOOS: "linux"}, fake, notify.MethodAuto)
	first := n.Send(context.Background(), one)
	second := n.Send(context.Background(), one)
	if got := len(fake.Calls()); got != 1 {
		t.Errorf("notify-send ran %d times, want 1", got)
	}
	if !first.Bell || !second.Bell || second.Err == nil || !strings.Contains(second.Err.Error(), "notify-send: boom") {
		t.Errorf("deliveries = %+v, %+v", first, second)
	}
}

func TestTimeoutDoesNotLockTheDesktop(t *testing.T) {
	fake := proc.NewFake("notify-send")
	fake.FailWith("/fake/notify-send", context.DeadlineExceeded)
	n := notify.New(host.Env{GOOS: "linux"}, fake, notify.MethodAuto)
	n.Send(context.Background(), one)
	n.Send(context.Background(), one)
	if got := len(fake.Calls()); got != 2 {
		t.Errorf("notify-send ran %d times, want 2", got)
	}
}

func TestWindowsAndWSLHaveNoDesktopRoute(t *testing.T) {
	const hint = "desktop notifications are not available on Windows; use terminal or bell"
	for _, env := range []host.Env{{GOOS: "windows"}, {GOOS: "linux", WSL: true}} {
		fake := proc.NewFake("notify-send", "powershell.exe")
		auto := send(t, env, notify.MethodAuto, fake, one)
		if !auto.Bell || auto.Route != notify.RouteBell || auto.Hint != "" || auto.Err != nil {
			t.Errorf("%+v auto: %+v", env, auto)
		}
		desk := send(t, env, notify.MethodDesktop, fake, one)
		if !desk.Bell || desk.Hint != hint {
			t.Errorf("%+v desktop: %+v", env, desk)
		}
		term := send(t, env, notify.MethodTerminal, fake, one)
		if term.Route != notify.RouteTerminal || !strings.HasPrefix(term.Raw, "\x1b]777;") {
			t.Errorf("%+v terminal: %+v", env, term)
		}
		if got := len(fake.Calls()); got != 0 {
			t.Errorf("%+v started %d programs", env, got)
		}
	}
}

func TestNotifySendKeepsTextAfterTheOptionTerminator(t *testing.T) {
	fake := proc.NewFake("notify-send")
	msg := []notify.Message{{Title: "--hint=x", Body: "-u critical"}}
	send(t, host.Env{GOOS: "linux"}, notify.MethodAuto, fake, msg)
	argv := fake.Calls()[0].Argv
	if len(argv) != 5 || argv[2] != "--" || argv[3] != "--hint=x" || argv[4] != "-u critical" {
		t.Errorf("argv = %q", argv)
	}
}

type gate struct {
	*proc.Fake
	mu      sync.Mutex
	running int
	peak    int
}

func (g *gate) Run(ctx context.Context, argv []string, stdin []byte) ([]byte, error) {
	g.mu.Lock()
	g.running++
	g.peak = max(g.peak, g.running)
	g.mu.Unlock()
	time.Sleep(10 * time.Millisecond)
	g.mu.Lock()
	g.running--
	g.mu.Unlock()
	return g.Fake.Run(ctx, argv, stdin)
}

func TestSendsAreSerialized(t *testing.T) {
	g := &gate{Fake: proc.NewFake("notify-send")}
	n := notify.New(host.Env{GOOS: "linux"}, g, notify.MethodAuto)
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n.Send(context.Background(), one)
		}()
	}
	wg.Wait()
	if g.peak != 1 {
		t.Errorf("%d helpers ran at once", g.peak)
	}
}
