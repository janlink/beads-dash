package cli_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/cli"
	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/testgolden"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want cli.Options
	}{
		{"none", nil, cli.Options{}},
		{"path", []string{"/w"}, cli.Options{Path: "/w"}},
		{"view", []string{"--view", "tree"}, cli.Options{View: "tree"}},
		{"view equals", []string{"--view=graph"}, cli.Options{View: "graph"}},
		{"single dash", []string{"-view", "ready"}, cli.Options{View: "ready"}},
		{"no-mouse", []string{"--no-mouse"}, cli.Options{NoMouse: true}},
		{"version long", []string{"--version"}, cli.Options{Version: true}},
		{"version short", []string{"-v"}, cli.Options{Version: true}},
		{"help long", []string{"--help"}, cli.Options{Help: true}},
		{"help short", []string{"-h"}, cli.Options{Help: true}},
		{"flags after path", []string{"/w", "--view", "kanban", "--no-mouse"}, cli.Options{Path: "/w", View: "kanban", NoMouse: true}},
		{"flags around path", []string{"--no-mouse", "/w", "--view=tree"}, cli.Options{Path: "/w", View: "tree", NoMouse: true}},
		{"terminator", []string{"--", "--odd-dir"}, cli.Options{Path: "--odd-dir"}},
		{"flag then terminator", []string{"--no-mouse", "--", "-x"}, cli.Options{Path: "-x", NoMouse: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cli.Parse(tt.args)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestParseUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"two paths", []string{"a", "b"}, "too many arguments"},
		{"two paths around flag", []string{"a", "--no-mouse", "b"}, "too many arguments"},
		{"unknown flag", []string{"--theme", "x"}, "not defined"},
		{"bad view", []string{"--view", "table"}, "invalid --view"},
		{"view without value", []string{"--view"}, "needs an argument"},
		{"dropped flag", []string{"--poll", "5"}, "not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cli.Parse(tt.args)
			var usage *cli.UsageError
			if !errors.As(err, &usage) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want usage error containing %q", err, tt.want)
			}
		})
	}
}

func TestOptionsFlags(t *testing.T) {
	got := cli.Options{View: "tree", NoMouse: true, Path: "/w"}.Flags()
	if want := (config.Flags{View: "tree", NoMouse: true}); got != want {
		t.Errorf("got %+v", got)
	}
}

func TestVersionText(t *testing.T) {
	b := cli.BuildInfo{Version: "0.1.0", Commit: "abcdef0123456", GoVersion: "go1.26"}
	if got, want := cli.VersionText(context.Background(), b, nil), "bdash 0.1.0 (commit abcdef0, go1.26)\n"; got != want {
		t.Errorf("without bd: %q, want %q", got, want)
	}
	bd := func(context.Context) string { return "bd 1.2.2 at /usr/bin/bd: supported" }
	if got, want := cli.VersionText(context.Background(), b, bd), "bdash 0.1.0 (commit abcdef0, go1.26)\nbd 1.2.2 at /usr/bin/bd: supported\n"; got != want {
		t.Errorf("with bd: %q, want %q", got, want)
	}
	empty := func(context.Context) string { return "" }
	if got := cli.VersionText(context.Background(), b, empty); strings.Count(got, "\n") != 1 {
		t.Errorf("empty bd line must be omitted: %q", got)
	}
	noCommit := cli.BuildInfo{Version: "dev", GoVersion: "go1.26"}
	if got, want := noCommit.String(), "bdash dev (go1.26)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNewBuildInfoDefaults(t *testing.T) {
	b := cli.NewBuildInfo("", "")
	if b.Version != "dev" || !strings.HasPrefix(b.GoVersion, "go") {
		t.Errorf("got %+v", b)
	}
	if b := cli.NewBuildInfo("1.0.0", "abc"); b.Version != "1.0.0" || b.Commit != "abc" {
		t.Errorf("got %+v", b)
	}
}

func TestHelpTextGolden(t *testing.T) {
	p, err := config.ResolvePaths(func(string) string { return "" }, "linux", "/home/jan")
	if err != nil {
		t.Fatal(err)
	}
	testgolden.Equal(t, cli.HelpText(p, nil))
}

func TestHelpTextWithoutPaths(t *testing.T) {
	got := cli.HelpText(config.Paths{}, config.ErrNoConfigDir)
	if !strings.Contains(got, "unavailable") || !strings.Contains(got, "BDASH_GLYPHS") {
		t.Errorf("got %q", got)
	}
}

func TestHelpTextListsEveryEnvVarAndFlag(t *testing.T) {
	got := cli.HelpText(config.Paths{}, nil)
	for _, want := range []string{"BDASH_CONFIG_DIR", "BDASH_GLYPHS", "BDASH_COLOR", "NO_COLOR", "BDASH_BD", "BDASH_TIMEOUT_SCALE", "BEADS_DIR", "--view", "--no-mouse", "--version", "--help"} {
		if !strings.Contains(got, want) {
			t.Errorf("help lacks %s", want)
		}
	}
}
