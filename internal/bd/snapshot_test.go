package bd

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

func TestFetchSnapshotBuildsFromFake(t *testing.T) {
	f := NewFake()
	parent := fakeIssue("e", "open")
	child := fakeIssue("e.1", "in_progress")
	child.Parent = "e"
	f.SetIssues(child, parent)
	f.SetReadiness([]string{"e", "ghost"}, map[string][]string{"e.1": {"e"}, "ghost": {"e"}})
	snap, err := FetchSnapshot(context.Background(), f, testNow)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Len() != 2 || !snap.FetchedAt().Equal(testNow()) {
		t.Errorf("snapshot = %v at %v", snap.IDs(), snap.FetchedAt())
	}
	if !snap.IsReady("e") || !snap.IsBlocked("e.1") || snap.IsBlocked("ghost") || !snap.IsContainer("e") {
		t.Error("readiness not applied")
	}
	if snap.Fingerprint() == "" {
		t.Error("no fingerprint")
	}
	if snap2, _ := FetchSnapshot(context.Background(), f, nil); snap2.Fingerprint() != snap.Fingerprint() {
		t.Error("same state must give the same fingerprint")
	}
}

func TestFetchSnapshotErrors(t *testing.T) {
	ctx := context.Background()
	for _, class := range allClasses {
		for _, method := range []string{"List", "Ready"} {
			f := NewFake()
			f.FailWith(method, &Error{Class: class, Command: method})
			snap, err := FetchSnapshot(ctx, f, nil)
			if snap != nil || !IsClass(err, class) {
				t.Errorf("%s failing with %v: %v, %v", method, class, snap, err)
			}
		}
	}
	f := NewFake()
	f.FailWith("List", &Error{Class: ClassTransient})
	f.FailWith("Ready", &Error{Class: ClassTimeout})
	if _, err := FetchSnapshot(ctx, f, nil); !IsClass(err, ClassTransient) && !IsClass(err, ClassTimeout) {
		t.Errorf("both failing = %v", err)
	}
}

type flightClient struct {
	*Fake
	inFlight, peak atomic.Int32
	calls          []string
}

func (f *flightClient) enter(name string) func() {
	f.calls = append(f.calls, name)
	n := f.inFlight.Add(1)
	for {
		p := f.peak.Load()
		if n <= p || f.peak.CompareAndSwap(p, n) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond)
	return func() { f.inFlight.Add(-1) }
}

func (f *flightClient) List(ctx context.Context) ([]model.Issue, error) {
	defer f.enter("List")()
	return f.Fake.List(ctx)
}

func (f *flightClient) Ready(ctx context.Context) (model.Readiness, error) {
	defer f.enter("Ready")()
	return f.Fake.Ready(ctx)
}

func TestFetchSnapshotHasOneCallInFlight(t *testing.T) {
	f := &flightClient{Fake: NewFake()}
	if _, err := FetchSnapshot(context.Background(), f, nil); err != nil {
		t.Fatal(err)
	}
	if f.peak.Load() != 1 || len(f.calls) != 2 || f.calls[0] != "List" || f.calls[1] != "Ready" {
		t.Errorf("peak in flight = %d, calls = %v; want List then Ready, one at a time", f.peak.Load(), f.calls)
	}
}

func TestFetchSnapshotSkipsReadyAfterListFailure(t *testing.T) {
	f := &flightClient{Fake: NewFake()}
	f.FailWith("List", &Error{Class: ClassTransient, Message: "list"})
	f.FailWith("Ready", &Error{Class: ClassTimeout})
	if _, err := FetchSnapshot(context.Background(), f, nil); !IsClass(err, ClassTransient) {
		t.Fatalf("err = %v, want the list failure", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("calls = %v, ready must not run after a list failure", f.calls)
	}
}

func TestFetchSnapshotHonoursCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := NewFake()
	f.FailWith("List", context.Canceled)
	cancel()
	if _, err := FetchSnapshot(ctx, f, nil); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestClassifyReadFailure(t *testing.T) {
	ctx := context.Background()
	plain := &Error{Class: ClassTransient, Command: "list", ExitCode: 1}
	notWS := func() *Fake {
		f := NewFake()
		f.FailWith("Where", &Error{Class: ClassNotWorkspace, Command: "where", Code: CodeNoBeadsDirectory})
		return f
	}
	if got := ClassifyReadFailure(ctx, notWS(), plain); CodeOf(got) != CodeNoBeadsDirectory || !IsClass(got, ClassTransient) {
		t.Errorf("no-workspace list = %v", got)
	}
	if plain.Code != "" {
		t.Error("the input error must not be mutated")
	}
	for name, err := range map[string]error{
		"ready in a workspace": plain,
		"timeout":              &Error{Class: ClassTimeout, Command: "list", ExitCode: 1},
		"already coded":        &Error{Class: ClassTransient, Command: "list", ExitCode: 1, Code: "x"},
		"other command":        &Error{Class: ClassTransient, Command: "comments", ExitCode: 1},
		"no exit code":         &Error{Class: ClassTransient, Command: "list"},
		"cancel":               context.Canceled,
	} {
		f := NewFake()
		if got := ClassifyReadFailure(ctx, f, err); !errors.Is(got, err) {
			t.Errorf("%s changed: %v", name, got)
		}
		if name != "ready in a workspace" && len(f.Calls()) != 0 {
			t.Errorf("%s asked bd: %v", name, f.Calls())
		}
	}
}
