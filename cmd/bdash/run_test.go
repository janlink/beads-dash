package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/cli"
	"github.com/janlink/beads-dash/internal/config"
)

type harness struct {
	stdout, stderr bytes.Buffer
	started        *Session
	startErr       error
	env            map[string]string
	paths          config.Paths
	pathsErr       error
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	return &harness{
		env: map[string]string{},
		paths: config.Paths{
			ConfigDir: dir, StateDir: dir,
			ConfigFile:     filepath.Join(dir, "config.toml"),
			HistoryFile:    filepath.Join(dir, "history"),
			WorkspacesFile: filepath.Join(dir, "workspaces.json"),
		},
	}
}

func (h *harness) run(args ...string) int {
	return run(args, &h.stdout, &h.stderr, deps{
		getenv: func(k string) string { return h.env[k] },
		paths:  func() (config.Paths, error) { return h.paths, h.pathsErr },
		build:  cli.BuildInfo{Version: "0.1.0", Commit: "abcdef0123", GoVersion: "go1.26"},
		bdVersion: func(context.Context) string {
			return "bd 1.2.2 at /usr/bin/bd: supported"
		},
		start: func(s Session) error {
			h.started = &s
			return h.startErr
		},
	})
}

func TestRunVersion(t *testing.T) {
	h := newHarness(t)
	for _, arg := range []string{"--version", "-v"} {
		h.stdout.Reset()
		if code := h.run(arg); code != 0 {
			t.Fatalf("%s exit %d", arg, code)
		}
		want := "bdash 0.1.0 (commit abcdef0, go1.26)\nbd 1.2.2 at /usr/bin/bd: supported\n"
		if h.stdout.String() != want {
			t.Errorf("%s printed %q, want %q", arg, h.stdout.String(), want)
		}
	}
	if h.started != nil {
		t.Error("--version started the program")
	}
}

func TestRunHelpListsResolvedPaths(t *testing.T) {
	h := newHarness(t)
	if code := h.run("--help"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out := h.stdout.String(); !strings.Contains(out, h.paths.ConfigFile) || !strings.Contains(out, "BDASH_GLYPHS") {
		t.Errorf("help = %q", out)
	}
	if h.started != nil {
		t.Error("--help started the program")
	}
}

func TestRunUsageErrorExitsTwo(t *testing.T) {
	for _, args := range [][]string{{"a", "b"}, {"--nope"}, {"--view", "bad"}} {
		h := newHarness(t)
		if code := h.run(args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
		if !strings.Contains(h.stderr.String(), "Try 'bdash --help'") || h.stdout.Len() != 0 || h.started != nil {
			t.Errorf("%v: stderr %q stdout %q started %v", args, h.stderr.String(), h.stdout.String(), h.started != nil)
		}
	}
}

func TestRunStartsWithPrecedenceApplied(t *testing.T) {
	h := newHarness(t)
	if err := os.WriteFile(h.paths.ConfigFile, []byte("view = \"tree\"\nglyphs = \"safe\"\nmystery = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.env["BDASH_GLYPHS"] = "ascii"
	if code := h.run("--view", "kanban", "/work/space"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, h.stderr.String())
	}
	s := h.started
	if s == nil {
		t.Fatal("program not started")
	}
	if s.Options.Path != "/work/space" || s.Settings.Settings.View != "kanban" || s.Settings.Settings.Glyphs != "ascii" {
		t.Errorf("session = %+v", s.Settings.Settings)
	}
	if got := s.Settings.OverrideNote(config.KeyGlyphs); got != "overridden by BDASH_GLYPHS" {
		t.Errorf("note = %q", got)
	}
	if len(s.Warnings) != 1 || !strings.Contains(s.Warnings[0], "mystery") {
		t.Errorf("warnings = %v", s.Warnings)
	}
	if s.Store == nil || s.State == nil {
		t.Error("store and state must be wired")
	}
}

func TestRunBrokenConfigDoesNotWrite(t *testing.T) {
	h := newHarness(t)
	const broken = "view = = x\n"
	if err := os.WriteFile(h.paths.ConfigFile, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := h.run(); code != 0 {
		t.Fatalf("a broken config must not stop startup, exit %d", code)
	}
	if err := h.started.Store.Set(config.KeyTheme, "ocean"); !errors.Is(err, config.ErrReadOnly) {
		t.Errorf("Set err = %v", err)
	}
	if b, _ := os.ReadFile(h.paths.ConfigFile); string(b) != broken {
		t.Errorf("config rewritten: %q", b)
	}
	if len(h.started.Warnings) != 1 {
		t.Errorf("warnings = %v", h.started.Warnings)
	}
}

func TestRunWithoutHomeStillStarts(t *testing.T) {
	h := newHarness(t)
	h.paths, h.pathsErr = config.Paths{}, config.ErrNoConfigDir
	if code := h.run(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	s := h.started
	if s.State == nil || len(s.Warnings) != 1 {
		t.Errorf("state %v warnings %v", s.State, s.Warnings)
	}
	if err := s.State.AppendHistory("x"); err != nil || len(s.State.History()) != 0 {
		t.Errorf("state without paths must be a no-op: %v", err)
	}
	if err := s.Store.Set(config.KeyTheme, "ocean"); err == nil {
		t.Error("store must be read-only without a config directory")
	}
	h = newHarness(t)
	h.paths, h.pathsErr = config.Paths{}, config.ErrNoConfigDir
	if code := h.run("--help"); code != 0 || !strings.Contains(h.stdout.String(), "unavailable") {
		t.Errorf("help without home: exit %d %q", code, h.stdout.String())
	}
}

func TestRunStartFailureExitsOne(t *testing.T) {
	h := newHarness(t)
	h.startErr = errors.New("boom")
	if code := h.run(); code != 1 || !strings.Contains(h.stderr.String(), "boom") {
		t.Errorf("exit %d stderr %q", code, h.stderr.String())
	}
}

func TestRunNilBdVersionPrintsBdashOnly(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"-v"}, &out, &errOut, deps{
		getenv: func(string) string { return "" },
		paths:  func() (config.Paths, error) { return config.Paths{}, nil },
		build:  cli.BuildInfo{Version: "dev", GoVersion: "go1.26"},
	})
	if code != 0 || out.String() != "bdash dev (go1.26)\n" {
		t.Errorf("exit %d out %q", code, out.String())
	}
}

func TestBdVersionLine(t *testing.T) {
	found := func(string) (string, error) { return "/opt/bd", nil }
	probeOf := func(raw string) func(context.Context, string, bd.Timeouts) (bd.VersionInfo, error) {
		return func(_ context.Context, _ string, _ bd.Timeouts) (bd.VersionInfo, error) { return bd.CheckVersion(raw) }
	}
	tests := []struct {
		name   string
		env    map[string]string
		look   func(string) (string, error)
		probe  func(context.Context, string, bd.Timeouts) (bd.VersionInfo, error)
		want   string
		wantIn string
	}{
		{name: "supported", look: found, probe: probeOf("1.3.0"), want: "bd 1.3.0 at /opt/bd: supported"},
		{name: "untested", look: found, probe: probeOf("1.9.0"), wantIn: "bd 1.9.0 at /opt/bd: untested ("},
		{name: "unsupported", look: found, probe: probeOf("1.2.1"), wantIn: "bd at /opt/bd: unsupported"},
		{
			name: "not on PATH", look: func(string) (string, error) { return "", errors.New("nope") },
			probe: probeOf("1.3.0"), want: "bd: not found (PATH)",
		},
		{
			name: "BDASH_BD missing", env: map[string]string{"BDASH_BD": "/x/bd"},
			look:  func(string) (string, error) { return "", errors.New("nope") },
			probe: probeOf("1.3.0"), want: "bd: not found (BDASH_BD)",
		},
		{
			name: "probe fails", look: found,
			probe: func(context.Context, string, bd.Timeouts) (bd.VersionInfo, error) {
				return bd.VersionInfo{}, &bd.Error{Class: bd.ClassTimeout, Message: "no answer in time"}
			},
			want: "bd at /opt/bd: version unknown (no answer in time)",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(k string) string { return tc.env[k] }
			got := bdVersionLine(context.Background(), getenv, tc.look, tc.probe)
			if tc.want != "" && got != tc.want || tc.wantIn != "" && !strings.Contains(got, tc.wantIn) {
				t.Errorf("line = %q, want %q%s", got, tc.want, tc.wantIn)
			}
		})
	}
}

func TestBdVersionLineResolvesBinaryAndScalesTimeout(t *testing.T) {
	var looked, probed string
	var timeouts bd.Timeouts
	getenv := func(k string) string {
		return map[string]string{"BDASH_BD": "/custom/bd", "BDASH_TIMEOUT_SCALE": "2"}[k]
	}
	bdVersionLine(context.Background(), getenv,
		func(n string) (string, error) { looked = n; return n, nil },
		func(_ context.Context, bin string, t bd.Timeouts) (bd.VersionInfo, error) {
			probed, timeouts = bin, t
			return bd.CheckVersion("1.3.0")
		})
	if looked != "/custom/bd" || probed != "/custom/bd" || timeouts.Probe != 10*time.Second {
		t.Errorf("looked %q, probed %q, timeouts %+v", looked, probed, timeouts)
	}
}
