package host_test

import (
	"errors"
	"testing"

	"github.com/janlink/beads-dash/internal/host"
)

func env(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }

func release(s string) func(string) ([]byte, error) {
	return func(string) ([]byte, error) { return []byte(s), nil }
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		file func(string) ([]byte, error)
		want host.Env
	}{
		{"wsl by variable", "linux", map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}, nil, host.Env{GOOS: "linux", WSL: true}},
		{"wsl by kernel", "linux", nil, release("6.6.87.2-microsoft-standard-WSL2\n"), host.Env{GOOS: "linux", WSL: true}},
		{"plain linux", "linux", nil, release("6.8.0-generic"), host.Env{GOOS: "linux"}},
		{"unreadable kernel release", "linux", nil, func(string) ([]byte, error) { return nil, errors.New("no") }, host.Env{GOOS: "linux"}},
		{"wsl variable is ignored off linux", "windows", map[string]string{"WSL_DISTRO_NAME": "x"}, nil, host.Env{GOOS: "windows"}},
		{"ssh", "darwin", map[string]string{"SSH_CONNECTION": "1 2 3 4"}, nil, host.Env{GOOS: "darwin", SSH: true}},
		{"tmux and display", "linux", map[string]string{"TMUX": "/tmp/t,1,0", "WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, nil, host.Env{GOOS: "linux", Tmux: true, Wayland: true, X11: true}},
		{"terminal", "linux", map[string]string{"TERM": "xterm-kitty", "TERM_PROGRAM": "x", "KITTY_WINDOW_ID": "1", "ZELLIJ": "0"}, nil, host.Env{GOOS: "linux", Term: "xterm-kitty", TermProgram: "x", Kitty: true, Zellij: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := host.Detect(tc.goos, env(tc.env), tc.file); got != tc.want {
				t.Errorf("Detect = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestUTF16LE(t *testing.T) {
	got := host.UTF16LE("a✅🚫")
	want := []byte{'a', 0, 0x05, 0x27, 0x3d, 0xd8, 0xab, 0xde}
	if string(got) != string(want) {
		t.Errorf("UTF16LE = % x, want % x", got, want)
	}
}
