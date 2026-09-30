// Package host describes the machine and terminal bdash runs on, as far as
// the clipboard and notification routes care.
package host

import "strings"

// Env is the detected environment.
type Env struct {
	// GOOS is the operating system: linux, darwin or windows.
	GOOS string
	// WSL is set on Linux running under Windows Subsystem for Linux.
	WSL bool
	// SSH is set inside an SSH session, where the machine's own clipboard and
	// notification daemon are not the viewer's.
	SSH bool
	// Tmux, Zellij and Screen are set inside those multiplexers. TmuxPane is
	// $TMUX_PANE, the pane bdash runs in.
	Tmux, Zellij, Screen bool
	TmuxPane             string
	// Wayland and X11 are set when the session has a display of that kind.
	Wayland, X11 bool
	// Term is $TERM and TermProgram is $TERM_PROGRAM; Kitty is set when
	// kitty's window variable is present.
	Term, TermProgram string
	Kitty             bool
}

// Detect reads the environment from getenv. readFile reads a file, for the
// kernel release that gives WSL away; it may be nil.
func Detect(goos string, getenv func(string) string, readFile func(string) ([]byte, error)) Env {
	set := func(names ...string) bool {
		for _, n := range names {
			if getenv(n) != "" {
				return true
			}
		}
		return false
	}
	e := Env{
		GOOS:        goos,
		SSH:         set("SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"),
		Tmux:        set("TMUX"),
		Zellij:      set("ZELLIJ"),
		Screen:      set("STY"),
		TmuxPane:    getenv("TMUX_PANE"),
		Wayland:     set("WAYLAND_DISPLAY"),
		X11:         set("DISPLAY"),
		Term:        getenv("TERM"),
		TermProgram: getenv("TERM_PROGRAM"),
		Kitty:       set("KITTY_WINDOW_ID"),
	}
	if goos == "linux" {
		e.WSL = set("WSL_DISTRO_NAME", "WSL_INTEROP")
		if !e.WSL && readFile != nil {
			if b, err := readFile("/proc/sys/kernel/osrelease"); err == nil {
				e.WSL = strings.Contains(strings.ToLower(string(b)), "microsoft")
			}
		}
	}
	return e
}
