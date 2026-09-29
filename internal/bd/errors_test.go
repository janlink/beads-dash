package bd

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type runnerFunc func(ctx context.Context, argv []string) (Result, error)

func (f runnerFunc) Run(ctx context.Context, argv []string) (Result, error) { return f(ctx, argv) }

func fixed(res Result, err error) Runner {
	return runnerFunc(func(context.Context, []string) (Result, error) { return res, err })
}

func TestClassifyRunFailures(t *testing.T) {
	deadline, cancelDeadline := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancelDeadline()
	<-deadline.Done()

	tests := []struct {
		name string
		run  Runner
		ctx  context.Context
		want Class
	}{
		{"not on PATH", fixed(Result{}, &exec.Error{Name: "bd", Err: exec.ErrNotFound}), context.Background(), ClassBdMissing},
		{"no such file", fixed(Result{}, &fs.PathError{Op: "fork/exec", Path: "/x/bd", Err: fs.ErrNotExist}), context.Background(), ClassBdMissing},
		{"other exec failure", fixed(Result{}, errors.New("permission denied")), context.Background(), ClassTransient},
		{"deadline in runner", fixed(Result{}, context.DeadlineExceeded), context.Background(), ClassTimeout},
		{"parent deadline", runnerFunc(func(ctx context.Context, _ []string) (Result, error) { return Result{}, ctx.Err() }), deadline, ClassTimeout},
		{"exit 1 plain text", fixed(Result{ExitCode: 1, Stderr: []byte("Error: boom")}, nil), context.Background(), ClassTransient},
		{"exit 2", fixed(Result{ExitCode: 2, Stderr: []byte("max rows")}, nil), context.Background(), ClassTransient},
		{"skew on stderr", fixed(Result{ExitCode: 1, Stderr: []byte(`{"error":"schema version mismatch","schema_skew":{"delta":1},"schema_version":1}`)}, nil), context.Background(), ClassUnsupported},
		{"schema_version 2", fixed(Result{Stdout: []byte(`{"schema_version":2,"data":[]}`)}, nil), context.Background(), ClassUnsupported},
		{"empty output", fixed(Result{}, nil), context.Background(), ClassTransient},
		{"garbage output", fixed(Result{Stdout: []byte("not json")}, nil), context.Background(), ClassTransient},
		{"wrong shape", fixed(Result{Stdout: []byte(`{"schema_version":1,"data":{"x":1}}`)}, nil), context.Background(), ClassTransient},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewExec(ExecOptions{Runner: tc.run})
			_, err := c.List(tc.ctx)
			got, ok := ClassOf(err)
			if !ok || got != tc.want {
				t.Errorf("class = %v (%v), want %v; err = %v", got, ok, tc.want, err)
			}
			var be *Error
			if errors.As(err, &be) && be.Command != "list" {
				t.Errorf("Command = %q, want list", be.Command)
			}
		})
	}
}

func TestClassifyByCodeNotText(t *testing.T) {
	text := Result{ExitCode: 1, Stderr: []byte("Error: no beads database found")}
	c := NewExec(ExecOptions{Runner: replay{"list": text, "where": {Stdout: []byte(`{"schema_version":1,"data":{"path":"/w"}}`)}}})
	_, err := c.List(context.Background())
	if CodeOf(err) != "" {
		t.Errorf("error text must not set a code: %v", err)
	}
	if !IsClass(err, ClassTransient) {
		t.Errorf("class = %v", err)
	}
}

func TestWhereNotWorkspaceNeedsJSONKey(t *testing.T) {
	c := NewExec(ExecOptions{Runner: replay{"where": {ExitCode: 1, Stderr: []byte("no_beads_directory in prose")}}})
	if _, err := c.Where(context.Background()); !IsClass(err, ClassTransient) {
		t.Errorf("where without the JSON key = %v, want transient", err)
	}
	env := `{"schema_version":1,"data":{"error":"no_beads_directory"}}`
	c = NewExec(ExecOptions{Runner: replay{"where": {Stdout: []byte(env), ExitCode: 1}}})
	if _, err := c.Where(context.Background()); !IsClass(err, ClassNotWorkspace) {
		t.Errorf("enveloped where error = %v, want not a workspace", err)
	}
}

func TestCanceledContextIsNotClassified(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := NewExec(ExecOptions{Runner: runnerFunc(func(ctx context.Context, _ []string) (Result, error) {
		return Result{}, ctx.Err()
	})})
	_, err := c.List(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, ok := ClassOf(err); ok {
		t.Error("a cancelled call has no class")
	}
}

func TestCallTimeoutComesFromKind(t *testing.T) {
	var got []time.Duration
	r := runnerFunc(func(ctx context.Context, _ []string) (Result, error) {
		d, _ := ctx.Deadline()
		got = append(got, time.Until(d))
		return Result{Stdout: []byte(`{"schema_version":1,"data":{}}`)}, nil
	})
	c := NewExec(ExecOptions{Runner: r, Timeouts: Timeouts{Probe: time.Minute, Read: time.Hour, Write: 24 * time.Hour}})
	_, _ = c.Where(context.Background())
	_, _ = c.Statuses(context.Background())
	if len(got) != 2 || got[0] > time.Minute || got[1] <= time.Minute || got[1] > time.Hour {
		t.Errorf("deadlines = %v", got)
	}
}

func TestTimeoutsScaled(t *testing.T) {
	d := DefaultTimeouts()
	if d.Probe != 5*time.Second || d.Read != 30*time.Second {
		t.Fatalf("defaults = %+v", d)
	}
	s := d.Scaled(2)
	if s.Probe != 10*time.Second || s.Read != time.Minute || s.Write != time.Minute {
		t.Errorf("scaled = %+v", s)
	}
	if d.Scaled(0) != d || d.Scaled(-1) != d {
		t.Error("non-positive scale must change nothing")
	}
	if d.of(callKind(99)) != d.Read {
		t.Error("unknown kind falls back to Read")
	}
}

func TestRejectsFlagLikeArguments(t *testing.T) {
	c := NewExec(ExecOptions{Runner: fixed(Result{}, errors.New("must not run"))})
	ctx := context.Background()
	if _, err := c.Comments(ctx, "--all"); err == nil || strings.Contains(err.Error(), "must not run") {
		t.Errorf("Comments with flag-like ID: %v", err)
	}
	if _, err := c.History(ctx, ""); err == nil || strings.Contains(err.Error(), "must not run") {
		t.Errorf("History with empty ID: %v", err)
	}
	if _, err := c.ConfigGet(ctx, "-x"); err == nil || strings.Contains(err.Error(), "must not run") {
		t.Errorf("ConfigGet with flag-like key: %v", err)
	}
}

func TestErrorMessageAndUnwrap(t *testing.T) {
	inner := errors.New("inner")
	e := &Error{Class: ClassTimeout, Command: "list", ExitCode: 3, Code: "c", Message: "m", Err: inner}
	msg := e.Error()
	for _, part := range []string{"bd list", "timeout", "(c)", "exit 3", ": m", "inner"} {
		if !strings.Contains(msg, part) {
			t.Errorf("%q lacks %q", msg, part)
		}
	}
	if !errors.Is(e, inner) {
		t.Error("Unwrap broken")
	}
	if got := (&Error{}).Error(); got != "bd: transient" {
		t.Errorf("zero Error = %q", got)
	}
	for c := ClassTransient; c <= ClassPartialWrite; c++ {
		if strings.HasPrefix(c.String(), "class(") {
			t.Errorf("class %d has no name", c)
		}
	}
	if Class(99).String() != "class(99)" {
		t.Error("unknown class name")
	}
	if CodeOf(errors.New("x")) != "" || IsClass(errors.New("x"), ClassTransient) {
		t.Error("plain errors carry no class")
	}
}

func TestCommandName(t *testing.T) {
	for argv, want := range map[string]string{
		"list --all": "list", "vc status --json": "vc status", "config get k": "config get",
		"comments t-1 --json": "comments", "dep --help": "dep", "": "",
	} {
		var a []string
		if argv != "" {
			a = strings.Fields(argv)
		}
		if got := commandName(a); got != want {
			t.Errorf("commandName(%q) = %q, want %q", argv, got, want)
		}
	}
}

func TestStderrIsClipped(t *testing.T) {
	c := NewExec(ExecOptions{Runner: fixed(Result{ExitCode: 1, Stderr: []byte(strings.Repeat("x", 10000))}, nil)})
	_, err := c.List(context.Background())
	var be *Error
	if !errors.As(err, &be) || len(be.Stderr) != stderrKeep {
		t.Errorf("stderr kept = %d bytes", len(be.Stderr))
	}
}

func TestWritesAndEventsNotImplemented(t *testing.T) {
	c := NewExec(ExecOptions{Runner: fixed(Result{}, errors.New("must not run"))})
	ctx := context.Background()
	if _, err := c.Create(ctx, CreateSpec{}); !errors.Is(err, ErrNotImplemented) {
		t.Error("Create")
	}
	if !errors.Is(c.Update(ctx, "x", UpdateSpec{}), ErrNotImplemented) {
		t.Error("Update")
	}
	if _, err := c.Close(ctx, nil, ""); !errors.Is(err, ErrNotImplemented) {
		t.Error("Close")
	}
	if _, err := c.Reopen(ctx, nil); !errors.Is(err, ErrNotImplemented) {
		t.Error("Reopen")
	}
	for name, err := range map[string]error{
		"DepAdd": c.DepAdd(ctx, "", "", ""), "DepRemove": c.DepRemove(ctx, "", ""),
		"Comment": c.Comment(ctx, "", ""), "Remember": c.Remember(ctx, "", ""),
		"Forget": c.Forget(ctx, ""), "ConfigSet": c.ConfigSet(ctx, "", ""),
	} {
		if !errors.Is(err, ErrNotImplemented) {
			t.Errorf("%s = %v", name, err)
		}
	}
	if _, err := c.EventsFollow(ctx, 0); !errors.Is(err, ErrNotImplemented) {
		t.Error("EventsFollow")
	}
}

func TestUnwrapTolerance(t *testing.T) {
	for name, body := range map[string]string{
		"bare array":        `[{"id":"a"}]`,
		"envelope":          `{"schema_version":1,"data":[{"id":"a"}]}`,
		"legacy object":     `{"path":"/w","schema_version":1}`,
		"no schema version": `{"data":[]}`,
	} {
		if _, err := unwrap("x", []byte(body)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := unwrap("x", []byte(`{"path":"/w","schema_version":7}`)); !IsClass(err, ClassUnsupported) {
		t.Error("schema_version 7 must stop")
	}
}

func TestParseFailureShapes(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		code string
		skew bool
		ok   bool
	}{
		{"flat", []string{`{"error":"no_beads_directory","schema_version":1}`}, "no_beads_directory", false, true},
		{"enveloped", []string{`{"data":{"error":"x","message":"m"},"schema_version":1}`}, "x", false, true},
		{"second stream", []string{"", `{"error":"y"}`}, "y", false, true},
		{"skew only", []string{`{"schema_skew":{"delta":2}}`}, "", true, true},
		{"plain text", []string{"Error: no beads database found"}, "", false, false},
		{"array", []string{`[1]`}, "", false, false},
		{"invalid json", []string{`{oops`}, "", false, false},
		{"no error key", []string{`{"data":{"x":1}}`}, "", false, false},
	}
	for _, tc := range tests {
		streams := make([][]byte, len(tc.in))
		for i, s := range tc.in {
			streams[i] = []byte(s)
		}
		f, ok := parseFailure(streams...)
		if ok != tc.ok || f.Code != tc.code || f.Skew != tc.skew {
			t.Errorf("%s: got %+v, %v", tc.name, f, ok)
		}
	}
}
