//go:build unix

package bd

import (
	"context"
	"errors"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestEventsFollowNeedsAStreamer(t *testing.T) {
	c := NewExec(ExecOptions{Runner: replay{}})
	if _, err := c.EventsFollow(context.Background(), 0); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("err = %v", err)
	}
}

func TestEventsFollowMissingBinary(t *testing.T) {
	c := NewExec(ExecOptions{Bin: "/nonexistent/bd"})
	_, err := c.EventsFollow(context.Background(), 0)
	if !IsClass(err, ClassBdMissing) {
		t.Errorf("err = %v", err)
	}
}

func followScript(t *testing.T, body string) *ExecClient {
	t.Helper()
	return NewExec(ExecOptions{Bin: script(t, body)})
}

func TestEventsFollowStreamsRecordsAndArgv(t *testing.T) {
	c := followScript(t, `
echo "args: $*" >&2
printf '%s\n' '{"seq":8,"ts":"2026-09-29T10:00:00Z","op":"create","issue_id":"a","actor":"ann","issue":{"id":"a","title":"A"}}'
printf '%s\n' '' 'garbage line'
printf '%s\n' '{"seq":9,"ts":"2026-09-29T10:00:01Z","op":"close","issue_id":"a","actor":"ann","issue":{"id":"a","title":"A","status":"closed"}}'
`)
	s, err := c.EventsFollow(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, seq := range []int64{8, 9} {
		ev, err := s.Next()
		if err != nil || ev.Seq != seq {
			t.Fatalf("Next = %+v, %v; want seq %d", ev, err, seq)
		}
	}
	if _, err := s.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("clean exit: err = %v, want EOF", err)
	}
	if _, err := s.Next(); !errors.Is(err, io.EOF) {
		t.Errorf("Next after the end: err = %v", err)
	}
}

func TestEventsFollowPassesArgv(t *testing.T) {
	c := followScript(t, `for a in "$@"; do printf '[%s]' "$a"; done >&2; exit 1`)
	s, err := c.EventsFollow(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.Next()
	var be *Error
	if !errors.As(err, &be) || be.ExitCode != 1 || be.Command != "events tail" {
		t.Fatalf("err = %v", err)
	}
	if want := "[events][tail][--since][7][--follow][--json]"; be.Stderr != want {
		t.Errorf("argv = %q, want %q", be.Stderr, want)
	}
	neg, err := c.EventsFollow(context.Background(), -5)
	if err != nil {
		t.Fatalf("negative since: %v", err)
	}
	_ = neg.Close()
}

func TestEventsFollowTruncatedMidStream(t *testing.T) {
	c := followScript(t, `
printf '%s\n' '{"seq":3,"ts":"2026-09-29T10:00:00Z","op":"update","issue_id":"a","issue":{"id":"a"}}'
printf '%s\n' '{"code":"events_journal_truncated","since":3,"floor":10,"head":20}'
sleep 30
`)
	s, err := c.EventsFollow(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if ev, err := s.Next(); err != nil || ev.Seq != 3 {
		t.Fatalf("Next = %+v, %v", ev, err)
	}
	_, err = s.Next()
	var tr *JournalTruncatedError
	if !errors.As(err, &tr) || tr.Floor != 10 || tr.Head != 20 || tr.Since != 3 {
		t.Errorf("err = %v", err)
	}
}

func TestEventsFollowTruncatedAtStartPrettyEnvelope(t *testing.T) {
	c := followScript(t, `
printf '{\n  "data": {\n    "code": "events_journal_truncated",\n    "floor": 6,\n    "head": 12,\n    "since": 2\n  },\n  "schema_version": 1\n}\n'
exit 1
`)
	s, err := c.EventsFollow(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.Next()
	var tr *JournalTruncatedError
	if !errors.As(err, &tr) || tr.Floor != 6 || tr.Since != 2 {
		t.Errorf("err = %v", err)
	}
}

func TestEventsFollowFailureCarriesStderr(t *testing.T) {
	c := followScript(t, `echo "Error: boom" >&2; exit 3`)
	s, err := c.EventsFollow(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.Next()
	var be *Error
	if !errors.As(err, &be) || be.ExitCode != 3 || be.Class != ClassTransient || !strings.Contains(be.Stderr, "boom") {
		t.Errorf("err = %v", err)
	}
}

func TestEventsFollowCloseKillsProcessGroup(t *testing.T) {
	pidFile := t.TempDir() + "/pid"
	c := followScript(t, `
printf '%s\n' '{"seq":1,"ts":"2026-09-29T10:00:00Z","op":"create","issue_id":"a","issue":{"id":"a"}}'
sleep 300 &
echo $! > `+pidFile+`
wait
`)
	s, err := c.EventsFollow(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Next(); err != nil {
		t.Fatal(err)
	}
	var pid int
	waitFor(t, "child pid file", func() bool { p, ok := readPID(pidFile); pid = p; return ok })

	done := make(chan error, 1)
	go func() { _, err := s.Next(); done <- err }()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Error("Next after Close returned no error")
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close = %v", err)
	}
	waitFor(t, "grandchild to die", func() bool { return syscall.Kill(pid, 0) != nil })
}

func TestEventsFollowContextCancel(t *testing.T) {
	c := followScript(t, `sleep 300`)
	ctx, cancel := context.WithCancel(context.Background())
	s, err := c.EventsFollow(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	done := make(chan error, 1)
	go func() { _, err := s.Next(); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Next did not return after cancel")
	}
}
