package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/janlink/beads-dash/internal/appearance"
	"github.com/janlink/beads-dash/internal/cli"
	"github.com/janlink/beads-dash/internal/config"
)

// Session is everything the running program needs from startup.
type Session struct {
	Getenv     func(string) string
	Options    cli.Options
	Settings   config.Resolved
	Appearance appearance.Appearance
	Paths      config.Paths
	Store      *config.Store
	State      *config.State
	// Warnings are shown once in the footer.
	Warnings []string
}

// deps are the process edges of run, replaceable in tests.
type deps struct {
	getenv    func(string) string
	paths     func() (config.Paths, error)
	build     cli.BuildInfo
	bdVersion cli.BdVersionFunc
	start     func(Session) error
}

// run executes bdash and returns the process exit status.
func run(args []string, stdout, stderr io.Writer, d deps) int {
	opts, err := cli.Parse(args)
	if err != nil {
		fmt.Fprintln(stderr, "bdash:", err)
		fmt.Fprintln(stderr, cli.UsageHint)
		return cli.ExitUsage
	}

	paths, pathsErr := d.paths()
	switch {
	case opts.Help:
		io.WriteString(stdout, cli.HelpText(paths, pathsErr)) //nolint:errcheck // nothing to do when stdout is gone
		return 0
	case opts.Version:
		ctx, cancel := context.WithTimeout(context.Background(), versionTimeout(d.getenv))
		defer cancel()
		io.WriteString(stdout, cli.VersionText(ctx, d.build, d.bdVersion)) //nolint:errcheck // nothing to do when stdout is gone
		return 0
	}

	s := Session{Options: opts, Paths: paths, Getenv: d.getenv}
	var loaded config.Loaded
	if pathsErr != nil {
		loaded = config.Loaded{Settings: config.Defaults(), Set: map[string]bool{}, Broken: true}
		s.Warnings = append(s.Warnings, fmt.Sprintf("%v; choices last for this session only", pathsErr))
	} else {
		loaded = config.Load(paths.ConfigFile)
	}
	s.State = config.NewState(paths)
	s.Store = config.NewStore(paths.ConfigFile, loaded.Broken)
	s.Settings = config.Resolve(loaded, opts.Flags(), d.getenv)
	s.Warnings = append(s.Warnings, s.Settings.Warnings...)

	if err := d.start(s); err != nil {
		fmt.Fprintln(stderr, "bdash:", err)
		return 1
	}
	return 0
}

func systemDeps() deps {
	return deps{
		getenv:    os.Getenv,
		paths:     config.SystemPaths,
		build:     cli.NewBuildInfo(version, commit),
		bdVersion: nil,
		start:     start,
	}
}

// versionTimeout bounds the bd call behind --version: 5 s scaled by
// BDASH_TIMEOUT_SCALE.
func versionTimeout(getenv func(string) string) time.Duration {
	scale := 1.0
	if v, err := strconv.ParseFloat(getenv(config.EnvTimeoutScale), 64); err == nil && v > 0 {
		scale = v
	}
	return time.Duration(float64(5*time.Second) * scale)
}
