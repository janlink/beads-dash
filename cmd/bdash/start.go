package main

import (
	"os"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/janlink/beads-dash/internal/appearance"
	"github.com/janlink/beads-dash/internal/term"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui"
)

// start resolves the appearance, probing the terminal before Bubble Tea owns
// it, and runs the program.
func start(s Session) (err error) {
	getenv := s.Getenv
	s.Appearance = appearance.Resolve(appearance.Input{
		Settings:    s.Settings.Settings,
		Getenv:      getenv,
		DetectDepth: func() theme.Depth { return theme.DetectDepth(os.Stdout, environFrom(getenv)) },
		Probe:       probeTTY,
	})

	if s.Appearance.TrackScheme {
		reset := term.EnableColorSchemeReporting(os.Stdout)
		defer reset()
	}

	opts := []tea.ProgramOption{tea.WithColorProfile(profileFor(s.Appearance.Depth))}
	_, err = tea.NewProgram(ui.New(), opts...).Run()
	return err
}

// probeTTY runs the probes on the real terminal. Windows consoles are not
// probed: the fallbacks (dark, narrow) apply and the settings can override
// them.
func probeTTY(o term.Options) term.Result {
	if runtime.GOOS == "windows" {
		return term.Result{Dark: true}
	}
	tty, err := term.Open()
	if err != nil {
		return term.Result{Dark: true}
	}
	defer tty.Close()
	res := term.Probe(tty, o)
	res.Interactive = true
	return res
}

var colorEnvKeys = []string{
	"TERM", "COLORTERM", "TERM_PROGRAM", "TMUX", "CLICOLOR", "CLICOLOR_FORCE", "TTY_FORCE", "WT_SESSION",
}

// environFrom builds the environment colorprofile inspects out of getenv.
func environFrom(getenv func(string) string) []string {
	var env []string
	for _, k := range colorEnvKeys {
		if v := getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func profileFor(d theme.Depth) colorprofile.Profile {
	switch d {
	case theme.DepthNone:
		return colorprofile.ASCII
	case theme.Depth16:
		return colorprofile.ANSI
	case theme.Depth256:
		return colorprofile.ANSI256
	case theme.DepthAuto, theme.DepthTrueColor:
		return colorprofile.TrueColor
	}
	return colorprofile.TrueColor
}
