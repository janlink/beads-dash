package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/cli"
	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

// bdCheck judges the bd binary bdash would use: BDASH_BD when set, else bd on
// PATH. probe and lookPath are the process edges. It returns the one-line
// verdict and, when bd is unusable, the input for the startup report.
func bdCheck(
	ctx context.Context,
	getenv func(string) string,
	lookPath func(string) (string, error),
	probe func(ctx context.Context, bin string, t bd.Timeouts) (bd.VersionInfo, error),
) (string, *screens.Input) {
	want, origin := getenv(config.EnvBd), config.EnvBd
	if want == "" {
		want, origin = "bd", "PATH"
	}
	path, err := lookPath(want)
	if err != nil {
		line := fmt.Sprintf("bd: not found (%s)", origin)
		return line, &screens.Input{Err: &bd.Error{Class: bd.ClassBdMissing, Command: want, Message: line}}
	}
	info, err := probe(ctx, path, bd.DefaultTimeouts().Scaled(timeoutScale(getenv)))
	in := &screens.Input{Err: err, Session: bd.Session{Version: info}, BdPath: path}
	switch {
	case err == nil && info.Support == bd.Supported:
		return fmt.Sprintf("bd %s at %s: supported", info.Parsed, path), nil
	case err == nil:
		return fmt.Sprintf("bd %s at %s: %s (%s)", info.Parsed, path, info.Support, info.Reason), nil
	case info.Support == bd.Unsupported || bd.IsClass(err, bd.ClassUnsupported):
		return fmt.Sprintf("bd at %s: unsupported (%s)", path, info.Reason), in
	}
	var be *bd.Error
	if errors.As(err, &be) && be.Message != "" {
		return fmt.Sprintf("bd at %s: version unknown (%s)", path, be.Message), in
	}
	return fmt.Sprintf("bd at %s: version unknown (%v)", path, err), in
}

func bdVersionLine(
	ctx context.Context,
	getenv func(string) string,
	lookPath func(string) (string, error),
	probe func(ctx context.Context, bin string, t bd.Timeouts) (bd.VersionInfo, error),
) string {
	line, _ := bdCheck(ctx, getenv, lookPath, probe)
	return line
}

// bdVersionText is the --version text about bd: the verdict line and, when bd
// is unusable, the startup checklist the failure screen shows.
func bdVersionText(
	ctx context.Context,
	getenv func(string) string,
	lookPath func(string) (string, error),
	probe func(ctx context.Context, bin string, t bd.Timeouts) (bd.VersionInfo, error),
) string {
	line, in := bdCheck(ctx, getenv, lookPath, probe)
	if in == nil {
		return line
	}
	in.Dir, _ = os.Getwd()
	in.BeadsDir = getenv("BEADS_DIR")
	in.BdashVersion = cli.NewBuildInfo(version, commit).Version
	in.BdashViaBrew = installedViaBrew()
	return line + "\n\n" + strings.TrimRight(screens.Text(screens.Diagnose(*in)), "\n")
}

func systemBdVersion(getenv func(string) string) cli.BdVersionFunc {
	return func(ctx context.Context) string {
		return bdVersionText(ctx, getenv, exec.LookPath, bd.Probe)
	}
}
