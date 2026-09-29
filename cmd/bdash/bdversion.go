package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/cli"
	"github.com/janlink/beads-dash/internal/config"
)

// bdVersionLine builds the --version line about the bd binary bdash would
// use: BDASH_BD when set, else bd on PATH. probe and lookPath are the
// process edges.
func bdVersionLine(
	ctx context.Context,
	getenv func(string) string,
	lookPath func(string) (string, error),
	probe func(ctx context.Context, bin string, t bd.Timeouts) (bd.VersionInfo, error),
) string {
	want, origin := getenv(config.EnvBd), config.EnvBd
	if want == "" {
		want, origin = "bd", "PATH"
	}
	path, err := lookPath(want)
	if err != nil {
		return fmt.Sprintf("bd: not found (%s)", origin)
	}
	info, err := probe(ctx, path, bd.DefaultTimeouts().Scaled(timeoutScale(getenv)))
	switch {
	case err == nil && info.Support == bd.Supported:
		return fmt.Sprintf("bd %s at %s: supported", info.Parsed, path)
	case err == nil:
		return fmt.Sprintf("bd %s at %s: %s (%s)", info.Parsed, path, info.Support, info.Reason)
	case info.Support == bd.Unsupported || bd.IsClass(err, bd.ClassUnsupported):
		return fmt.Sprintf("bd at %s: unsupported (%s)", path, info.Reason)
	}
	var be *bd.Error
	if errors.As(err, &be) && be.Message != "" {
		return fmt.Sprintf("bd at %s: version unknown (%s)", path, be.Message)
	}
	return fmt.Sprintf("bd at %s: version unknown (%v)", path, err)
}

func systemBdVersion(getenv func(string) string) cli.BdVersionFunc {
	return func(ctx context.Context) string {
		return bdVersionLine(ctx, getenv, exec.LookPath, bd.Probe)
	}
}
