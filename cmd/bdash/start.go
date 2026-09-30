package main

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/janlink/beads-dash/internal/appearance"
	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/cli"
	"github.com/janlink/beads-dash/internal/refresh"
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
	_, err = tea.NewProgram(ui.New(uiOptions(s)), opts...).Run()
	return err
}

// uiOptions wires the session into the shell: the bd client, the actor and
// the facts the failure screens quote.
func uiOptions(s Session) ui.Options {
	getenv := s.Getenv
	bin := s.Settings.BdBinary
	dir := s.Options.Path
	if dir == "" {
		dir, _ = os.Getwd()
	}
	client := bd.NewExec(bd.ExecOptions{
		Bin:      bin,
		Dir:      s.Options.Path,
		Timeouts: bd.DefaultTimeouts().Scaled(s.Settings.TimeoutScale),
	})
	tunables := refresh.TunablesFrom(s.Settings.Settings.Refresh)
	o := ui.Options{
		Client:       client,
		Tunables:     &tunables,
		Actor:        resolveActor(getenv, gitUserName),
		Settings:     s.Settings,
		Views:        ui.IssueViews(),
		Appearance:   s.Appearance,
		Getenv:       getenv,
		Warnings:     s.Warnings,
		NoMouse:      s.Options.NoMouse,
		Dir:          dir,
		BdPath:       lookBd(bin),
		BeadsDir:     getenv("BEADS_DIR"),
		BdashVersion: cli.NewBuildInfo(version, commit).Version,
		BdashViaBrew: installedViaBrew(),
	}
	if s.Store != nil {
		o.Store = s.Store
	}
	if s.State != nil {
		o.History = s.State
		o.Journal = s.State
	}
	return o
}

func installedViaBrew() bool {
	exe, _ := os.Executable()
	return strings.Contains(exe, "/Cellar/") || strings.Contains(exe, "/homebrew/")
}

func lookBd(bin string) string {
	if bin == "" {
		bin = "bd"
	}
	p, err := exec.LookPath(bin)
	if err != nil {
		return ""
	}
	return p
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
