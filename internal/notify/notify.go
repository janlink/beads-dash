package notify

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/host"
	"github.com/janlink/beads-dash/internal/proc"
)

// Methods are the values of the notify.method setting.
const (
	MethodAuto     = "auto"
	MethodDesktop  = "desktop"
	MethodTerminal = "terminal"
	MethodBell     = "bell"
	MethodOff      = "off"
)

// Routes name how a delivery went, for messages and tests.
const (
	RouteNone     = "none"
	RouteBell     = "bell"
	RouteTerminal = "terminal"
)

// Delivery is what [Notifier.Send] did. The shell finishes the job for the
// routes that go through the terminal.
type Delivery struct {
	// Route is the backend name, RouteTerminal, RouteBell or RouteNone.
	Route string
	// Raw is a terminal sequence for the shell to write.
	Raw string
	// Bell asks for the terminal bell and the messages in the footer, which
	// is where notifications end up when no other route works.
	Bell bool
	// Err is why a desktop route was given up on, when one was tried.
	Err error
	// Hint explains that the desktop method has no backend on this host; the
	// shell shows it once.
	Hint string
}

// Notifier delivers notifications by the configured method.
type Notifier struct {
	env    host.Env
	run    proc.Runner
	method string

	desktop desktopBackend

	// sendMu serializes sends, so a burst of refreshes cannot start several
	// helpers at once.
	sendMu sync.Mutex
	// locked is the failure that switched the desktop route off for the
	// session.
	locked error

	mu     sync.Mutex
	passOK *bool
}

// New returns the notifier for env and the notify.method setting.
func New(env host.Env, run proc.Runner, method string) *Notifier {
	return &Notifier{env: env, run: run, method: method, desktop: selectDesktop(env, run.LookPath)}
}

// Backend names the desktop backend, "" when there is none.
func (n *Notifier) Backend() string { return n.desktop.name }

const sendTimeout = 10 * time.Second

// Send delivers msgs. It blocks while a desktop helper runs, so call it
// from a command.
func (n *Notifier) Send(ctx context.Context, msgs []Message) Delivery {
	if len(msgs) == 0 || n.method == MethodOff {
		return Delivery{Route: RouteNone}
	}
	n.sendMu.Lock()
	defer n.sendMu.Unlock()
	last := n.locked
	hint := ""
	for _, route := range n.order() {
		switch route {
		case RouteTerminal:
			if seq, ok := n.terminal(ctx, msgs); ok {
				return Delivery{Route: RouteTerminal, Raw: seq}
			}
		case MethodDesktop:
			if n.desktop.name == "" {
				if n.method == MethodDesktop && n.noDesktopHost() {
					hint = desktopUnavailable
				}
				continue
			}
			if n.locked != nil {
				continue
			}
			err := n.sendDesktop(ctx, msgs)
			if err == nil {
				return Delivery{Route: n.desktop.name}
			}
			last = fmt.Errorf("%s: %w", n.desktop.name, err)
			if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
				n.locked = last
			}
		}
	}
	return Delivery{Route: RouteBell, Bell: true, Err: last, Hint: hint}
}

// order lists the routes to try, best first. Over SSH the machine's own
// notification service is not in front of the viewer, so only the terminal
// can reach them.
func (n *Notifier) order() []string {
	switch n.method {
	case MethodBell:
		return nil
	case MethodDesktop:
		return []string{MethodDesktop}
	case MethodTerminal:
		return []string{RouteTerminal, MethodDesktop}
	}
	if n.env.SSH {
		return []string{RouteTerminal}
	}
	return []string{MethodDesktop}
}

func (n *Notifier) sendDesktop(ctx context.Context, msgs []Message) error {
	for _, m := range msgs {
		sctx, cancel := context.WithTimeout(ctx, sendTimeout)
		_, err := n.run.Run(sctx, n.desktop.argv(m), nil)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

// screenChunk is the longest string sequence GNU screen takes.
const screenChunk = 768

// terminal builds the escape sequences for msgs. Inside tmux they need
// tmux's passthrough, which is off unless the user allowed it.
func (n *Notifier) terminal(ctx context.Context, msgs []Message) (string, bool) {
	if n.env.Tmux && !n.passthrough(ctx) {
		return "", false
	}
	var b strings.Builder
	for _, m := range msgs {
		seq := n.sequence(m)
		switch {
		case n.env.Tmux:
			seq = ansi.TmuxPassthrough(seq)
		case n.env.Screen:
			seq = ansi.ScreenPassthrough(seq, screenChunk)
		}
		b.WriteString(seq)
	}
	return b.String(), true
}

func (n *Notifier) passthrough(ctx context.Context) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.passOK != nil {
		return *n.passOK
	}
	ok := false
	if p, err := n.run.LookPath("tmux"); err == nil {
		sctx, cancel := context.WithTimeout(ctx, sendTimeout)
		argv := []string{p, "show", "-Apv"}
		if n.env.TmuxPane != "" {
			argv = append(argv, "-t", n.env.TmuxPane)
		}
		out, err := n.run.Run(sctx, append(argv, "allow-passthrough"), nil)
		cancel()
		v := strings.TrimSpace(string(out))
		ok = err == nil && (v == "on" || v == "all")
	}
	n.passOK = &ok
	return ok
}

// sequence is the notification escape for the terminal: OSC 99 for kitty,
// OSC 9 for iTerm2 and OSC 777 for everything else that has one (WezTerm,
// Ghostty, foot).
func (n *Notifier) sequence(m Message) string {
	title, body := clean(m.Title), clean(m.Body)
	switch {
	case n.env.Kitty || n.env.Term == "xterm-kitty":
		return ansi.DesktopNotification(title, "i=1", "d=0", "p=title") +
			ansi.DesktopNotification(body, "i=1", "d=1", "p=body")
	case n.env.TermProgram == "iTerm.app":
		return ansi.Notify(strings.TrimSuffix(title+": "+body, ": "))
	}
	return "\x1b]777;notify;" + title + ";" + body + "\x07"
}

// clean drops what would end or corrupt an escape sequence.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == ';':
			return ','
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0):
			return ' '
		}
		return r
	}, s)
}

const desktopUnavailable = "desktop notifications are not available on Windows; use terminal or bell"

func (n *Notifier) noDesktopHost() bool { return n.env.GOOS == "windows" || n.env.WSL }
