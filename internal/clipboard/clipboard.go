// Package clipboard copies text out of bdash. Bubble Tea's OSC 52 write is
// always sent by the shell; this package adds the routes that need a helper
// program: the machine's own clipboard when it is local, and tmux's buffer.
package clipboard

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/janlink/beads-dash/internal/host"
	"github.com/janlink/beads-dash/internal/proc"
)

// MaxBytes is the most text any route is given. tmux drops escape sequences
// past 1 MiB, which is about 780 KB of text once base64 has inflated it.
const MaxBytes = 700 * 1024

// ErrTooLarge and ErrEmpty are what [Check] rejects text with.
var (
	ErrTooLarge = errors.New("too large to copy")
	ErrEmpty    = errors.New("nothing to copy")
)

// Check reports whether text may be copied on every route.
func Check(text string) error {
	switch {
	case text == "":
		return ErrEmpty
	case len(text) > MaxBytes:
		return fmt.Errorf("%w: %d KB is over the %d KB limit", ErrTooLarge, (len(text)+1023)/1024, MaxBytes/1024)
	}
	return nil
}

// OSC52 names the route the shell sends itself, for messages.
const OSC52 = "OSC 52"

// Route is one helper program that takes the text on its standard input.
type Route struct {
	// Name is what messages call the route.
	Name string
	// Argv is the command; Argv[0] is the path found by the lookup.
	Argv []string
	// Native is set for the machine's own clipboard, unset for tmux.
	Native bool
	utf16  bool
}

// Payload is the bytes the route reads: UTF-16LE for clip.exe, which mangles
// UTF-8, and UTF-8 for everything else.
func (r Route) Payload(text string) []byte {
	if r.utf16 {
		return host.UTF16LE(text)
	}
	return []byte(text)
}

// windowsClip is where WSL finds clip.exe when Windows' directories are not
// on its PATH.
const windowsClip = "/mnt/c/Windows/System32/clip.exe"

// Select picks the helper routes for env: at most one native writer, and
// tmux's buffer inside tmux. Over SSH the machine's clipboard is not the
// viewer's, so there is no native route.
func Select(env host.Env, look func(string) (string, error)) []Route {
	var out []Route
	if !env.SSH {
		if r, ok := native(env, look); ok {
			out = append(out, r)
		}
	}
	if env.Tmux {
		if p, err := look("tmux"); err == nil {
			out = append(out, Route{Name: "tmux buffer", Argv: []string{p, "load-buffer", "-w", "-"}})
		}
	}
	return out
}

func native(env host.Env, look func(string) (string, error)) (Route, bool) {
	route := func(name string, utf16 bool, args ...string) (Route, bool) {
		p, err := look(name)
		if err != nil {
			return Route{}, false
		}
		return Route{Name: name, Argv: append([]string{p}, args...), Native: true, utf16: utf16}, true
	}
	switch env.GOOS {
	case "darwin":
		return route("pbcopy", false)
	case "windows":
		return route("clip.exe", true)
	}
	if env.WSL {
		if r, ok := route("clip.exe", true); ok {
			return r, true
		}
		if _, err := look(windowsClip); err == nil {
			return Route{Name: "clip.exe", Argv: []string{windowsClip}, Native: true, utf16: true}, true
		}
	}
	if env.Wayland {
		if r, ok := route("wl-copy", false); ok {
			return r, true
		}
	}
	if env.X11 {
		if r, ok := route("xclip", false, "-selection", "clipboard"); ok {
			return r, true
		}
		if r, ok := route("xsel", false, "--clipboard", "--input"); ok {
			return r, true
		}
	}
	return Route{}, false
}

// Board writes text through the helper routes.
type Board struct {
	routes []Route
	run    proc.Runner
}

// New returns the board for env, running helpers through run.
func New(env host.Env, run proc.Runner) *Board {
	return &Board{routes: Select(env, run.LookPath), run: run}
}

// Routes lists the routes the board writes to.
func (b *Board) Routes() []Route { return b.routes }

// writeTimeout bounds one helper.
const writeTimeout = 3 * time.Second

// Failure is a route that did not take the text.
type Failure struct {
	Route string
	Err   error
}

// Result is what the routes did.
type Result struct {
	// Copied names the routes that took the text.
	Copied   []string
	Failures []Failure
}

// Write gives text to every route. It blocks, so call it from a command.
func (b *Board) Write(ctx context.Context, text string) Result {
	var res Result
	if err := Check(text); err != nil {
		res.Failures = append(res.Failures, Failure{Err: err})
		return res
	}
	for _, r := range b.routes {
		rctx, cancel := context.WithTimeout(ctx, writeTimeout)
		_, err := b.run.Run(rctx, r.Argv, r.Payload(text))
		cancel()
		if err != nil {
			res.Failures = append(res.Failures, Failure{Route: r.Name, Err: err})
			continue
		}
		res.Copied = append(res.Copied, r.Name)
	}
	return res
}

// Message describes the outcome for the footer: what was copied, the routes
// that took it and the OSC 52 write, which nothing acknowledges and so is
// only ever "sent". Warn is set when a route failed.
func (r Result) Message(what string) (msg string, warn bool) {
	var parts []string
	for _, c := range r.Copied {
		parts = append(parts, "copied to "+c)
	}
	parts = append(parts, "sent via "+OSC52)
	for _, f := range r.Failures {
		warn = true
		parts = append(parts, fmt.Sprintf("%s failed: %s", f.Route, shortErr(f.Err)))
	}
	return what + ": " + strings.Join(parts, "; "), warn
}

func shortErr(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	return line
}
