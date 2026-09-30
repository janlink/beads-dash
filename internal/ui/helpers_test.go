package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/appearance"
	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/term"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

type flavour struct{ depth, glyphs string }

var (
	plain     = flavour{"none", "ascii"}
	truecolor = flavour{"truecolor", "fancy"}
)

func resolved(f flavour) config.Resolved {
	s := config.Defaults()
	s.Background, s.Glyphs, s.Color, s.Ambiguous = "dark", f.glyphs, f.depth, "narrow"
	s.DetailDocked = false
	return config.Resolved{Settings: s}
}

func testOptions(f flavour, mod func(*Options)) Options {
	r := resolved(f)
	o := Options{
		Settings: r,
		Appearance: appearance.Resolve(appearance.Input{
			Settings:    r.Settings,
			Getenv:      func(string) string { return "" },
			DetectDepth: func() theme.Depth { return theme.DepthTrueColor },
			Probe:       func(term.Options) term.Result { return term.Result{Dark: true} },
		}),
		Client:  bd.NewFake(),
		Journal: answeredJournal{},
		Now:     func() time.Time { return uitest.T0 },
	}
	if mod != nil {
		mod(&o)
	}
	return o
}

func workspace() bd.Session {
	return bd.Session{
		Version:   bd.VersionInfo{Raw: "1.2.2"},
		Workspace: bd.Workspace{Path: "/work/demo/.beads", Prefix: "ws"},
		Statuses:  model.BuiltinStatuses(),
	}
}

func liveStatus() refresh.Status {
	return refresh.Status{Loaded: true, LastSuccess: uitest.T0, Focused: true}
}

// loaded returns an app past startup, showing snap at the given size.
func loaded(t testing.TB, f flavour, cols, rows int, snap *model.Snapshot, mod func(*Options)) *App {
	t.Helper()
	a := New(testOptions(f, mod))
	a.syncMD = true
	send(a, tea.WindowSizeMsg{Width: cols, Height: rows})
	send(a, sessionMsg{sess: workspace()})
	send(a, updateMsg{refresh.Update{Snapshot: snap, Status: liveStatus(), Session: workspace()}})
	return a
}

func sample(t testing.TB, f flavour, cols, rows int) *App {
	snap, _ := uitest.Sample()
	return loaded(t, f, cols, rows, snap, nil)
}

func send(a *App, msgs ...tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	for _, m := range msgs {
		_, cmd = a.Update(m)
	}
	return cmd
}

func press(a *App, keys ...string) {
	for _, k := range keys {
		send(a, keyMsg(k))
	}
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	}
	if len(k) == len("ctrl+")+1 && strings.HasPrefix(k, "ctrl+") {
		return tea.KeyPressMsg{Code: rune(k[len("ctrl+")]), Mod: tea.ModCtrl}
	}
	r := []rune(k)
	return tea.KeyPressMsg{Code: r[0], Text: k}
}

func screen(a *App) string { return ansi.Strip(a.View().Content) }

func lines(a *App) []string { return strings.Split(screen(a), "\n") }

func isOpen[T Dialog](a *App) bool {
	_, ok := a.topDialog().(T)
	return ok
}

// typeText presses each character of s as a key, then lets the search
// debounce elapse.
func typeText(a *App, s string) {
	for _, r := range s {
		if r == ' ' {
			press(a, "space")
			continue
		}
		press(a, string(r))
	}
	settle(a)
}

// settle lets the search debounce elapse.
func settle(a *App) {
	if b := a.bar; b != nil && b.dirty {
		send(a, searchTickMsg{b.gen})
	}
}

// answeredJournal is a journal store whose viewer has declined already, so
// no test meets the opt-in dialog by accident.
type answeredJournal struct{}

func (answeredJournal) Journal(string) config.JournalAnswer { return config.JournalDeclined }

func (answeredJournal) SetJournal(string, config.JournalAnswer) error { return nil }

// chipText is the plain text of the chips the footer rule carries.
func chipText(a *App) string {
	var parts []string
	for _, it := range a.chipItems() {
		for _, s := range it.seg {
			parts = append(parts, s.Text)
		}
	}
	return strings.Join(parts, "  ")
}
