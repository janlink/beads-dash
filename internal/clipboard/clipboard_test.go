package clipboard_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/host"
	"github.com/janlink/beads-dash/internal/proc"
)

func names(rs []clipboard.Route) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

func TestRouteSelectionPerEnvironment(t *testing.T) {
	all := []string{"wl-copy", "xclip", "xsel", "pbcopy", "clip.exe", "tmux"}
	tests := []struct {
		name     string
		env      host.Env
		programs []string
		want     []string
	}{
		{"linux wayland", host.Env{GOOS: "linux", Wayland: true, X11: true}, all, []string{"wl-copy"}},
		{"linux wayland without wl-copy", host.Env{GOOS: "linux", Wayland: true, X11: true}, []string{"xclip"}, []string{"xclip"}},
		{"linux x11 prefers xclip", host.Env{GOOS: "linux", X11: true}, all, []string{"xclip"}},
		{"linux x11 falls back to xsel", host.Env{GOOS: "linux", X11: true}, []string{"xsel"}, []string{"xsel"}},
		{"linux headless", host.Env{GOOS: "linux"}, all, nil},
		{"wsl prefers clip.exe", host.Env{GOOS: "linux", WSL: true, Wayland: true}, all, []string{"clip.exe"}},
		{"wsl without clip.exe uses wslg", host.Env{GOOS: "linux", WSL: true, Wayland: true}, []string{"wl-copy"}, []string{"wl-copy"}},
		{"wsl finds clip.exe off the path", host.Env{GOOS: "linux", WSL: true}, []string{"/mnt/c/Windows/System32/clip.exe"}, []string{"clip.exe"}},
		{"ssh skips native", host.Env{GOOS: "linux", SSH: true, Wayland: true, X11: true}, all, nil},
		{"ssh wsl skips native", host.Env{GOOS: "linux", WSL: true, SSH: true}, all, nil},
		{"macos", host.Env{GOOS: "darwin"}, all, []string{"pbcopy"}},
		{"macos over ssh", host.Env{GOOS: "darwin", SSH: true}, all, nil},
		{"windows", host.Env{GOOS: "windows"}, all, []string{"clip.exe"}},
		{"tmux adds the buffer", host.Env{GOOS: "linux", Tmux: true, X11: true}, all, []string{"xclip", "tmux buffer"}},
		{"tmux over ssh keeps the buffer", host.Env{GOOS: "linux", Tmux: true, SSH: true}, all, []string{"tmux buffer"}},
		{"tmux without the program", host.Env{GOOS: "darwin", Tmux: true}, []string{"pbcopy"}, []string{"pbcopy"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := names(clipboard.Select(tc.env, proc.NewFake(tc.programs...).LookPath))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("routes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClipExeGetsUTF16LEAndOthersUTF8(t *testing.T) {
	fake := proc.NewFake("clip.exe", "tmux")
	b := clipboard.New(host.Env{GOOS: "windows", Tmux: true}, fake)
	text := "Ümlaut ✅ 🚫"
	res := b.Write(context.Background(), text)
	if len(res.Failures) != 0 || strings.Join(res.Copied, ",") != "clip.exe,tmux buffer" {
		t.Fatalf("result = %+v", res)
	}
	calls := fake.Calls()
	want := []byte{0xdc, 0x00, 'm', 0, 'l', 0, 'a', 0, 'u', 0, 't', 0, ' ', 0, 0x05, 0x27, ' ', 0, 0x3d, 0xd8, 0xab, 0xde}
	if !bytes.Equal(calls[0].Stdin, want) {
		t.Errorf("clip.exe stdin = % x, want % x", calls[0].Stdin, want)
	}
	if got := calls[1].Line(); got != "/fake/tmux load-buffer -w -" {
		t.Errorf("tmux command = %q", got)
	}
	if string(calls[1].Stdin) != text {
		t.Errorf("tmux stdin = %q", calls[1].Stdin)
	}
}

func TestCapAppliesBeforeAnyRoute(t *testing.T) {
	fake := proc.NewFake("pbcopy")
	b := clipboard.New(host.Env{GOOS: "darwin"}, fake)
	if err := clipboard.Check(strings.Repeat("x", clipboard.MaxBytes)); err != nil {
		t.Errorf("text at the cap rejected: %v", err)
	}
	big := strings.Repeat("x", clipboard.MaxBytes+1)
	if err := clipboard.Check(big); !errors.Is(err, clipboard.ErrTooLarge) {
		t.Errorf("Check(big) = %v", err)
	}
	res := b.Write(context.Background(), big)
	if len(res.Copied) != 0 || len(res.Failures) != 1 || len(fake.Calls()) != 0 {
		t.Errorf("over-cap text reached a route: %+v, %d calls", res, len(fake.Calls()))
	}
	if err := clipboard.Check(""); !errors.Is(err, clipboard.ErrEmpty) {
		t.Errorf("Check(empty) = %v", err)
	}
}

func TestMessageNamesRoutesAndSaysSentForOSC52(t *testing.T) {
	msg, warn := clipboard.Result{Copied: []string{"clip.exe"}}.Message("ws-1")
	if msg != "ws-1: copied to clip.exe; sent via OSC 52" || warn {
		t.Errorf("message = %q warn=%v", msg, warn)
	}
	msg, warn = clipboard.Result{}.Message("ws-1")
	if msg != "ws-1: sent via OSC 52" || warn {
		t.Errorf("message = %q warn=%v", msg, warn)
	}
	msg, warn = clipboard.Result{Failures: []clipboard.Failure{{Route: "xclip", Err: errors.New("exit status 1\nmore")}}}.Message("ws-1")
	if msg != "ws-1: sent via OSC 52; xclip failed: exit status 1" || !warn {
		t.Errorf("message = %q warn=%v", msg, warn)
	}
}

func TestFailedRouteDoesNotStopTheOthers(t *testing.T) {
	fake := proc.NewFake("xclip", "tmux")
	fake.FailWith("/fake/xclip", errors.New("no display"))
	b := clipboard.New(host.Env{GOOS: "linux", X11: true, Tmux: true}, fake)
	res := b.Write(context.Background(), "hi")
	if len(res.Copied) != 1 || res.Copied[0] != "tmux buffer" || len(res.Failures) != 1 || res.Failures[0].Route != "xclip" {
		t.Errorf("result = %+v", res)
	}
}
