//go:build unix

package bd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func script(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fakebd")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExecRunnerEnvironmentAndDir(t *testing.T) {
	bin := script(t, `printf '%s|%s|%s|%s|%s|%s' "$BD_JSON_ENVELOPE" "${BEADS_MAX_ROWS-unset}" "$BEADS_DIR" "$EXTRA" "$BD_DISABLE_METRICS" "$(pwd)"`)
	dir := t.TempDir()
	t.Setenv("BEADS_MAX_ROWS", "5")
	t.Setenv("BD_JSON_ENVELOPE", "0")
	t.Setenv("BD_DISABLE_METRICS", "0")
	t.Setenv("BEADS_DIR", "/from/process")
	res, err := ExecRunner{Bin: bin, Dir: dir, Env: []string{"EXTRA=x"}}.Run(context.Background(), nil)
	if err != nil || res.ExitCode != 0 {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	resolved, _ := filepath.EvalSymlinks(dir)
	if got, want := string(res.Stdout), "1|unset|/from/process|x|1|"+resolved; got != want {
		t.Errorf("env = %q, want %q", got, want)
	}

	res, err = ExecRunner{Bin: bin, Env: []string{"BEADS_DIR=/explicit"}}.Run(context.Background(), nil)
	if err != nil || !strings.Contains(string(res.Stdout), "|/explicit|") {
		t.Errorf("explicit BEADS_DIR: %q, %v", res.Stdout, err)
	}
}

func TestExecRunnerPassesArgvWithoutShell(t *testing.T) {
	bin := script(t, `for a in "$@"; do printf '[%s]' "$a"; done`)
	res, err := ExecRunner{Bin: bin}.Run(context.Background(), []string{"a b", "$HOME", ";rm", "*"})
	if err != nil || string(res.Stdout) != "[a b][$HOME][;rm][*]" {
		t.Errorf("stdout = %q, %v", res.Stdout, err)
	}
}

func TestExecRunnerExitCodeIsData(t *testing.T) {
	bin := script(t, `echo out; echo err >&2; exit 3`)
	res, err := ExecRunner{Bin: bin}.Run(context.Background(), nil)
	if err != nil || res.ExitCode != 3 || strings.TrimSpace(string(res.Stdout)) != "out" || strings.TrimSpace(string(res.Stderr)) != "err" {
		t.Errorf("Run = %+v, %v", res, err)
	}
}

func TestExecRunnerMissingBinary(t *testing.T) {
	_, err := ExecRunner{Bin: filepath.Join(t.TempDir(), "nope")}.Run(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	c := NewExec(ExecOptions{Bin: filepath.Join(t.TempDir(), "nope")})
	if _, err := c.Where(context.Background()); !IsClass(err, ClassBdMissing) {
		t.Errorf("Where = %v", err)
	}
	c = NewExec(ExecOptions{Bin: "definitely-not-a-bd-binary-xyz"})
	if _, err := c.Where(context.Background()); !IsClass(err, ClassBdMissing) {
		t.Errorf("Where via PATH = %v", err)
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func readPID(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	return pid, err == nil && pid > 0
}

func TestExecRunnerTimeoutKillsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild")
	bin := script(t, `sleep 60 &
echo $! > "`+pidFile+`.tmp"
mv "`+pidFile+`.tmp" "`+pidFile+`"
wait`)
	c := NewExec(ExecOptions{Bin: bin, Timeouts: Timeouts{Probe: 2 * time.Second, Read: 2 * time.Second, Write: time.Second}})
	_, err := c.List(context.Background())
	if !IsClass(err, ClassTimeout) {
		t.Fatalf("err = %v, want timeout", err)
	}
	pid, ok := readPID(pidFile)
	if !ok {
		t.Fatal("the script never recorded its grandchild")
	}
	waitFor(t, "the grandchild to die", func() bool { return syscall.Kill(pid, 0) != nil })
}

func TestExecRunnerCancel(t *testing.T) {
	started := filepath.Join(t.TempDir(), "started")
	bin := script(t, `touch "`+started+`"
sleep 60`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			if _, err := os.Stat(started); err == nil {
				cancel()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	done := make(chan error, 1)
	go func() {
		_, err := NewExec(ExecOptions{Bin: bin}).List(ctx)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancel did not kill bd")
	}
}

func TestProbeHonoursTimeouts(t *testing.T) {
	bin := script(t, `sleep 60`)
	_, err := Probe(context.Background(), bin, Timeouts{Probe: 300 * time.Millisecond})
	if !IsClass(err, ClassTimeout) {
		t.Errorf("Probe = %v, want timeout", err)
	}
}
