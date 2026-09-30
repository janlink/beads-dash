package bd

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestFixtureMemoriesParse(t *testing.T) {
	forEachM8(t, func(t *testing.T, m m8) {
		c := m.client("memories-empty")
		if got, err := c.Memories(context.Background()); err != nil || len(got) != 0 {
			t.Errorf("empty store = %v, %v", got, err)
		}
		got, err := m.client("memories-odd").Memories(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		var keys []string
		for _, mem := range got {
			keys = append(keys, mem.Key)
		}
		want := []string{"-dash", "Weird.Key", "first-key", "schema_version", "some-content-words-here-for-the-slug", "spaced key", "weird.key"}
		if !reflect.DeepEqual(keys, want) {
			t.Errorf("keys = %q, want byte-wise %q", keys, want)
		}
		for _, mem := range got {
			if mem.Key == "spaced key" && mem.Content != "line1\nline2" {
				t.Errorf("multi-line content = %q", mem.Content)
			}
		}
	})
}

func TestFixtureRemember(t *testing.T) {
	forEachM8(t, func(t *testing.T, m m8) {
		ctx := context.Background()
		for name, key := range map[string]string{
			"remember-ok":               "first-key",
			"remember-overwrite":        "first-key",
			"remember-recalled-shape":   "some-content-words-here-for-the-slug",
			"remember-leading-dash":     "-dash",
			"remember-schema-version":   "schema_version",
			"remember-spaced-key":       "spaced key",
			"remember-mixed-case-upper": "Weird.Key",
			"remember-mixed-case-lower": "weird.key",
		} {
			if err := m.client(name).Remember(ctx, key, "x"); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
		if err := m.client("remember-ok").Remember(ctx, "other-key", "x"); err == nil {
			t.Error("an answer for another key passed")
		}
		be := wantErr(t, m.client("remember-empty-content").Remember(ctx, "k", "x"), ClassRejected)
		if !strings.Contains(be.Message, "empty") {
			t.Errorf("message = %q", be.Message)
		}
	})
}

func TestFixtureForget(t *testing.T) {
	forEachM8(t, func(t *testing.T, m m8) {
		ctx := context.Background()
		if err := m.client("forget-ok").Forget(ctx, "first-key"); err != nil {
			t.Errorf("forget-ok: %v", err)
		}
		if err := m.client("forget-leading-dash").Forget(ctx, "-dash"); err != nil {
			t.Errorf("forget-leading-dash: %v", err)
		}
		for _, name := range []string{"forget-missing", "forget-again"} {
			if err := m.client(name).Forget(ctx, "first-key"); !errors.Is(err, ErrMemoryGone) {
				t.Errorf("%s = %v, want ErrMemoryGone", name, err)
			}
		}
		if err := m.client("forget-ok").Forget(ctx, "other"); err == nil || errors.Is(err, ErrMemoryGone) {
			t.Errorf("an answer for another key = %v", err)
		}
	})
}

func TestMemoryArgvAlwaysHasKeyAndDoubleDash(t *testing.T) {
	var got [][]string
	c := NewExec(ExecOptions{Runner: runnerFunc(func(_ context.Context, argv []string) (Result, error) {
		got = append(got, argv)
		if argv[0] == "remember" {
			return Result{Stdout: []byte(`{"data":{"action":"remembered","key":"-k","value":"-v"},"schema_version":1}`)}, nil
		}
		return Result{Stdout: []byte(`{"data":{"deleted":"true","key":"-k"},"schema_version":1}`)}, nil
	})})
	ctx := context.Background()
	if err := c.Remember(ctx, "-k", "-v"); err != nil {
		t.Fatal(err)
	}
	if err := c.Forget(ctx, "-k"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"remember", "--json", "--key=-k", "--", "-v"}, {"forget", "--json", "--", "-k"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

func TestMemoryValidationRunsNothing(t *testing.T) {
	c := NewExec(ExecOptions{Runner: fixed(Result{}, errors.New("must not run"))})
	ctx := context.Background()
	for name, err := range map[string]error{
		"empty key":     c.Remember(ctx, "  ", "x"),
		"newline key":   c.Remember(ctx, "a\nb", "x"),
		"empty content": c.Remember(ctx, "k", " \n"),
		"forget empty":  c.Forget(ctx, ""),
	} {
		_ = wantErr(t, err, ClassRejected)
		_ = name
	}
}

func TestFixtureConfigSetJournal(t *testing.T) {
	ok := Result{Stdout: []byte(`{"data":{"key":"events-journal","location":"config.yaml","value":"true"},"schema_version":1}`)}
	if err := NewExec(ExecOptions{Runner: fixed(ok, nil)}).ConfigSet(context.Background(), "events-journal", "true"); err != nil {
		t.Errorf("ConfigSet = %v", err)
	}
	if err := NewExec(ExecOptions{Runner: fixed(ok, nil)}).ConfigSet(context.Background(), "events-journal", "false"); err == nil {
		t.Error("an answer with another value passed")
	}
}

func TestFakeMemoryWrites(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	before, _ := f.VCStatus(ctx)
	if err := f.Remember(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	if after, _ := f.VCStatus(ctx); after.Commit == before.Commit {
		t.Error("a memory write did not move the commit hash")
	}
	if err := f.Forget(ctx, "nope"); !errors.Is(err, ErrMemoryGone) {
		t.Errorf("Forget missing = %v", err)
	}
	_ = wantErr(t, f.Remember(ctx, "k", ""), ClassRejected)
	if err := f.Forget(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	var methods []string
	for _, w := range f.Writes() {
		methods = append(methods, w.Method+":"+w.Key)
	}
	if want := []string{"Remember:k", "Forget:nope", "Remember:k", "Forget:k"}; !reflect.DeepEqual(methods, want) {
		t.Errorf("writes = %q", methods)
	}
}
