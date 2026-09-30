package bd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var idPattern = regexp.MustCompile(`m8-[a-z0-9]+(\.[0-9]+)?`)

// m8 replays a captured write of the given bd version.
type m8 struct {
	t       testing.TB
	version string
}

func (m m8) result(name string) Result {
	m.t.Helper()
	dir := "bd-" + m.version
	return Result{
		Stdout:   fixture(m.t, dir, "m8", name+".json"),
		Stderr:   fixture(m.t, dir, "m8", name+".stderr"),
		ExitCode: rcOf(m.t, dir, "m8", name+".rc"),
	}
}

func (m m8) client(name string) *ExecClient {
	return NewExec(ExecOptions{Runner: fixed(m.result(name), nil)})
}

// returned lists the IDs of the issues the captured answer holds.
func (m m8) returned(name string) []string {
	m.t.Helper()
	r, err := readWrite("x", m.result(name))
	if err != nil {
		m.t.Fatal(err)
	}
	return r.ids
}

// mentioned is the first ID that the captured stderr names.
func (m m8) mentioned(name string) string {
	m.t.Helper()
	id := idPattern.FindString(string(m.result(name).Stderr))
	if id == "" {
		m.t.Fatalf("%s: stderr names no issue", name)
	}
	return id
}

func forEachM8(t *testing.T, fn func(t *testing.T, m m8)) {
	for _, v := range fixtureVersions(t) {
		if _, ok := fixtureOptional("bd-"+v, "m8", "create-ok.rc"); !ok {
			continue
		}
		t.Run(v, func(t *testing.T) { fn(t, m8{t, v}) })
	}
}

func wantErr(t *testing.T, err error, want Class) *Error {
	t.Helper()
	var be *Error
	if !errors.As(err, &be) || be.Class != want {
		t.Fatalf("err = %v, want class %v", err, want)
	}
	return be
}

func TestFixtureCreate(t *testing.T) {
	forEachM8(t, func(t *testing.T, m m8) {
		ctx := context.Background()
		id, err := m.client("create-ok").Create(ctx, CreateSpec{Title: "First"})
		if want := m.returned("create-ok"); err != nil || len(want) != 1 || id != want[0] {
			t.Errorf("Create = %q, %v; want %v", id, err, want)
		}
		if id, err := m.client("create-child").Create(ctx, CreateSpec{Title: "Child"}); err != nil || !strings.Contains(id, ".") {
			t.Errorf("Create child = %q, %v", id, err)
		}
		for name, want := range map[string]string{
			"create-empty-title":  "title required",
			"create-bad-priority": "invalid priority",
			"create-bad-type":     "invalid issue type",
		} {
			be := wantErr(t, func() error { _, err := m.client(name).Create(ctx, CreateSpec{Title: "x"}); return err }(), ClassRejected)
			if !strings.Contains(be.Message, want) {
				t.Errorf("%s: message %q lacks %q", name, be.Message, want)
			}
		}
	})
}

func TestFixtureCreateWithMissingDependency(t *testing.T) {
	forEachM8(t, func(t *testing.T, m m8) {
		id, err := m.client("create-deps-missing").Create(context.Background(), CreateSpec{Title: "Dep", Deps: []string{"m8-zzz"}})
		if m.version == "1.2.2" {
			// bd 1.2.2 creates the issue, exits 0 and only warns on stderr.
			be := wantErr(t, err, ClassPartialWrite)
			if id == "" || !reflect.DeepEqual(be.Applied, []string{id}) || !strings.Contains(be.Message, "failed to add dependency") {
				t.Errorf("id %q, %+v", id, be)
			}
			return
		}
		be := wantErr(t, err, ClassRejected)
		if id != "" || len(be.Applied) != 0 || !strings.Contains(be.Message, "m8-zzz") {
			t.Errorf("id %q, %+v", id, be)
		}
	})
}

func TestFixtureStateAfterFailedBatches(t *testing.T) {
	forEachM8(t, func(t *testing.T, m m8) {
		status := func(name string) map[string]string {
			var env struct {
				Data []struct{ Title, Status string } `json:"data"`
			}
			if err := json.Unmarshal(m.result("state-after-"+name).Stdout, &env); err != nil {
				t.Fatal(err)
			}
			out := map[string]string{}
			for _, is := range env.Data {
				out[is.Title] = is.Status
			}
			return out
		}
		// The guard refused Beta and closed Gamma: a refusal does not undo the rest.
		if s := status("close-blocked-batch"); s["Gamma"] != "closed" || s["Beta"] == "closed" {
			t.Errorf("after a guard refusal in a batch: %v", s)
		}
		// An unknown ID makes bd close nothing, whatever else the batch held.
		if s := status("close-missing-batch"); s["Delta"] != "open" {
			t.Errorf("after an unknown ID in a batch: %v", s)
		}
		if s := status("close-open-children"); s["Epic"] != "open" {
			t.Errorf("after closing an epic with open children: %v", s)
		}
	})
}

func TestFixtureUpdate(t *testing.T) {
	forEachM8(t, func(t *testing.T, m m8) {
		ctx := context.Background()
		spec := UpdateSpec{Claim: true}
		for _, name := range []string{"update-ok", "update-remove-label", "update-unassign", "update-clear-text", "update-cleared", "update-estimate", "update-due-clear", "update-claim"} {
			ids := m.returned(name)
			if len(ids) != 1 {
				t.Fatalf("%s returned %v", name, ids)
			}
			if err := m.client(name).Update(ctx, ids, spec); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}

		good := m.returned("update-partial")
		err := m.client("update-partial").Update(ctx, append(slices.Clone(good), "m8-zzz"), spec)
		be := wantErr(t, err, ClassPartialWrite)
		if !reflect.DeepEqual(be.Applied, good) || len(be.Failed) != 1 || be.Failed[0].ID != "m8-zzz" ||
			!strings.Contains(be.Failed[0].Message, "no issue found") {
			t.Errorf("partial update: %+v", be)
		}

		be = wantErr(t, m.client("update-missing").Update(ctx, []string{"m8-zzz"}, spec), ClassRejected)
		if len(be.Applied) != 0 || len(be.Failed) != 1 || be.Failed[0].ID != "m8-zzz" {
			t.Errorf("missing update: %+v", be)
		}

		held := m.mentioned("update-claim-conflict")
		be = wantErr(t, m.client("update-claim-conflict").Update(ctx, []string{held}, spec), ClassRejected)
		if !strings.Contains(be.Message, "already claimed") || len(be.Failed) != 1 || be.Failed[0].ID != held {
			t.Errorf("claim conflict: %+v", be)
		}

		for name, want := range map[string]string{"update-bad-priority": "invalid priority", "update-bad-status": "invalid status"} {
			be := wantErr(t, m.client(name).Update(ctx, []string{"m8-x"}, spec), ClassRejected)
			if !strings.Contains(be.Message, want) {
				t.Errorf("%s: %q", name, be.Message)
			}
		}
	})
}

func TestFixtureClose(t *testing.T) {
	forEachM8(t, func(t *testing.T, m m8) {
		ctx := context.Background()
		done, err := m.client("close-ok").Close(ctx, m.returned("close-ok"), "done it")
		if err != nil || !reflect.DeepEqual(done, m.returned("close-ok")) {
			t.Errorf("close-ok: %v, %v", done, err)
		}
		if _, err := m.client("close-again").Close(ctx, m.returned("close-again"), "x"); err != nil {
			t.Errorf("close-again: %v", err)
		}

		blocked := m.mentioned("close-blocked-single")
		_, err = m.client("close-blocked-single").Close(ctx, []string{blocked}, "nope")
		be := wantErr(t, err, ClassRejected)
		if !strings.Contains(be.Message, "blocked") || be.Failed[0].ID != blocked {
			t.Errorf("blocked single: %+v", be)
		}

		// bd exits 0 here: the close guard refused one issue of the batch.
		if rc := m.result("close-blocked-batch").ExitCode; rc != 0 {
			t.Fatalf("fixture exit code = %d, the point of the case is exit 0", rc)
		}
		closed := m.returned("close-blocked-batch")
		refused := m.mentioned("close-blocked-batch")
		done, err = m.client("close-blocked-batch").Close(ctx, append(slices.Clone(closed), refused), "batch")
		be = wantErr(t, err, ClassPartialWrite)
		if !reflect.DeepEqual(done, closed) || !reflect.DeepEqual(be.Applied, closed) ||
			len(be.Failed) != 1 || be.Failed[0].ID != refused || !strings.Contains(be.Failed[0].Message, "cannot close") {
			t.Errorf("guard refusal in a batch: done=%v %+v", done, be)
		}

		_, err = m.client("close-open-children").Close(ctx, []string{m.mentioned("close-open-children")}, "epic")
		be = wantErr(t, err, ClassRejected)
		if !strings.Contains(be.Message, "open child") {
			t.Errorf("open children: %+v", be)
		}

		_, err = m.client("close-missing-batch").Close(ctx, []string{"m8-x", "m8-zzz"}, "with missing")
		be = wantErr(t, err, ClassRejected)
		if be.ExitCode != 1 || len(be.Applied) != 0 || !strings.Contains(be.Message, "resolving ID") {
			t.Errorf("missing in a batch: %+v", be)
		}
	})
}

func TestFixtureReopen(t *testing.T) {
	forEachM8(t, func(t *testing.T, m m8) {
		ctx := context.Background()
		ids := m.returned("reopen-ok")
		if done, err := m.client("reopen-ok").Reopen(ctx, ids, "back"); err != nil || !reflect.DeepEqual(done, ids) {
			t.Errorf("reopen-ok: %v, %v", done, err)
		}
		open := m.mentioned("reopen-already-open")
		_, err := m.client("reopen-already-open").Reopen(ctx, []string{open}, "")
		if be := wantErr(t, err, ClassRejected); !strings.Contains(be.Message, "already open") {
			t.Errorf("already open: %+v", be)
		}
		_, err = m.client("reopen-missing-batch").Reopen(ctx, []string{open, "m8-zzz"}, "")
		if be := wantErr(t, err, ClassRejected); len(be.Failed) < 2 || be.Failed[1].ID != "m8-zzz" {
			t.Errorf("reopen with a missing ID: %+v", be)
		}
	})
}

func TestFixtureDep(t *testing.T) {
	forEachM8(t, func(t *testing.T, m m8) {
		ctx := context.Background()
		if err := m.client("dep-add").DepAdd(ctx, "m8-a", "m8-b", ""); err != nil {
			t.Errorf("dep-add: %v", err)
		}
		if err := m.client("dep-remove").DepRemove(ctx, "m8-a", "m8-b"); err != nil {
			t.Errorf("dep-remove: %v", err)
		}
		for name, want := range map[string]string{"dep-add-cycle": "cycle", "dep-add-missing": "no issue found"} {
			be := wantErr(t, m.client(name).DepAdd(ctx, "m8-a", "m8-b", ""), ClassRejected)
			if !strings.Contains(be.Message, want) {
				t.Errorf("%s: %q", name, be.Message)
			}
		}
		// A remove that finds no edge is still answered "removed"; a dependency
		// answer without the expected status is refused.
		c := NewExec(ExecOptions{Runner: fixed(Result{Stdout: []byte(`{"data":{"status":"added"}}`)}, nil)})
		_ = wantErr(t, c.DepRemove(ctx, "m8-a", "m8-b"), ClassRejected)
	})
}

func TestLegacyWriteShapes(t *testing.T) {
	legacy := func(name string) Result {
		return Result{
			Stdout: fixture(t, "bd-1.2.2", "writes", name+".out"),
			Stderr: fixture(t, "bd-1.2.2", "writes", name+".err"),
		}
	}
	c := NewExec(ExecOptions{Runner: fixed(legacy("close"), nil)})
	ids := payloadIDs(mustUnwrap(t, legacy("close").Stdout))
	if done, err := c.Close(context.Background(), ids, "done it"); err != nil || !reflect.DeepEqual(done, ids) {
		t.Errorf("legacy close = %v, %v", done, err)
	}
	res := legacy("err_close_blocked")
	res.ExitCode = 1
	c = NewExec(ExecOptions{Runner: fixed(res, nil)})
	_, err := c.Close(context.Background(), []string{"r3n50-00003"}, "")
	if be := wantErr(t, err, ClassRejected); !strings.Contains(be.Message, "blocked by open issues") {
		t.Errorf("legacy blocked close: %+v", be)
	}
}

func mustUnwrap(t *testing.T, stdout []byte) []byte {
	t.Helper()
	data, err := unwrap("x", stdout)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestWriteArgv(t *testing.T) {
	p, est := 1, 30
	title, empty := "-dash title", ""
	got, _, err := createArgv(CreateSpec{
		Title: "T", Type: "bug", Priority: &p, Labels: []string{"a", "b"}, NoInheritLabels: true,
		Parent: "x-1", Deps: []string{"x-2", "discovered-from:x-3"}, Estimate: &est, Due: "tomorrow",
	})
	want := []string{
		"create", "--json", "--title=T", "--type=bug", "--priority=1", "--parent=x-1", "--labels=a,b", "--no-inherit-labels",
		"--due=tomorrow", "--estimate=30", "--deps=x-2,discovered-from:x-3",
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("createArgv = %v, %v", got, err)
	}
	got, _, err = updateArgv([]string{"a", "b"}, UpdateSpec{
		Title: &title, Assignee: &empty, Priority: &p, AddLabels: []string{"x"}, RemoveLabels: []string{"y", "z"}, Claim: true,
	})
	want = []string{
		"update", "a", "b", "--json", "--title=-dash title", "--assignee=", "--priority=1",
		"--add-label=x", "--remove-label=y", "--remove-label=z", "--claim",
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("updateArgv = %v, %v", got, err)
	}
	for _, bad := range []func() error{
		func() error { _, _, err := createArgv(CreateSpec{Title: "t", Labels: []string{"a,b"}}); return err },
		func() error { _, _, err := updateArgv([]string{"-x"}, UpdateSpec{Claim: true}); return err },
		func() error { _, _, err := updateArgv(nil, UpdateSpec{Claim: true}); return err },
		func() error {
			_, _, err := updateArgv([]string{"a"}, UpdateSpec{RemoveLabels: []string{""}})
			return err
		},
	} {
		if bad() == nil {
			t.Error("invalid input passed")
		}
	}
}

func TestWriteRunFailuresKeepTheirClass(t *testing.T) {
	c := NewExec(ExecOptions{Runner: fixed(Result{}, context.DeadlineExceeded)})
	_, err := c.Close(context.Background(), []string{"a"}, "")
	if !IsClass(err, ClassTimeout) {
		t.Errorf("timeout = %v", err)
	}
	if err := c.Update(context.Background(), []string{"a"}, UpdateSpec{}); err != nil {
		t.Errorf("empty update must not run bd: %v", err)
	}
}

// inputRunner records the argv and stdin of a write and answers with res.
type inputRunner struct {
	res   Result
	argv  []string
	stdin []byte
	files map[string]string
}

func (r *inputRunner) Run(context.Context, []string) (Result, error) { return r.res, nil }

func (r *inputRunner) RunInput(_ context.Context, argv []string, stdin []byte) (Result, error) {
	r.argv, r.stdin = argv, stdin
	r.files = map[string]string{}
	for _, a := range argv {
		if path, ok := strings.CutPrefix(a, "--design-file="); ok {
			b, _ := os.ReadFile(path)
			r.files[path] = string(b)
		}
	}
	return r.res, nil
}

func TestLongTextsLeaveTheCommandLine(t *testing.T) {
	long := strings.Repeat("x", longText)
	run := &inputRunner{res: Result{Stdout: []byte(`{"data":[{"id":"a-1"}]}`)}}
	c := NewExec(ExecOptions{Runner: run})
	short := "short"
	if err := c.Update(context.Background(), []string{"a-1"}, UpdateSpec{Description: &long, Design: &long, Notes: &short}); err != nil {
		t.Fatal(err)
	}
	if string(run.stdin) != long || !slices.Contains(run.argv, "--body-file=-") || !slices.Contains(run.argv, "--notes=short") {
		t.Errorf("argv %v, stdin %d bytes", run.argv, len(run.stdin))
	}
	for _, a := range run.argv {
		if strings.Contains(a, long) {
			t.Error("a long text is on the command line")
		}
	}
	if len(run.files) != 1 {
		t.Fatalf("design file: %v", run.files)
	}
	for path, text := range run.files {
		if text != long {
			t.Errorf("design file holds %d bytes", len(text))
		}
		if _, err := os.Stat(path); err == nil {
			t.Error("the design file was not removed")
		}
	}
	tooLong := strings.Repeat("y", maxArg+1)
	if err := c.Update(context.Background(), []string{"a-1"}, UpdateSpec{Notes: &tooLong}); err == nil {
		t.Error("notes beyond the command line limit were passed on")
	}
}

func TestLongTextNeedsAnInputRunner(t *testing.T) {
	long := strings.Repeat("x", longText)
	c := NewExec(ExecOptions{Runner: fixed(Result{}, nil)})
	if err := c.Update(context.Background(), []string{"a-1"}, UpdateSpec{Description: &long}); err == nil {
		t.Error("stdin text without an input runner was sent")
	}
}

func TestReasonMatchesWholeIDs(t *testing.T) {
	r := writeReport{lines: []string{"cannot close x-10: blocked", "cannot close x-1: stuck"}}
	if got := r.reasonFor("x-1"); got != "cannot close x-1: stuck" {
		t.Errorf("reason for x-1 = %q", got)
	}
}
