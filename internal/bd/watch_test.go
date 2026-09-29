package bd

import (
	"context"
	"errors"
	"testing"
)

func noWS() error { return &Error{Class: ClassTransient, Command: "list", Code: CodeNoBeadsDirectory} }

func TestWorkspaceWatchEscalatesOnThirdConsecutive(t *testing.T) {
	var w WorkspaceWatch
	for i := 1; i <= 2; i++ {
		if err := w.Observe(noWS()); !IsClass(err, ClassTransient) {
			t.Fatalf("observation %d = %v, want transient", i, err)
		}
	}
	err := w.Observe(noWS())
	if !IsClass(err, ClassNotWorkspace) || CodeOf(err) != CodeNoBeadsDirectory {
		t.Fatalf("third observation = %v", err)
	}
	if err := w.Observe(noWS()); !IsClass(err, ClassNotWorkspace) {
		t.Errorf("stays not-a-workspace while it keeps failing: %v", err)
	}
}

func TestWorkspaceWatchResets(t *testing.T) {
	var w WorkspaceWatch
	_ = w.Observe(noWS())
	_ = w.Observe(noWS())
	if err := w.Observe(nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Observe(noWS()); !IsClass(err, ClassTransient) {
		t.Errorf("after a success the streak restarts: %v", err)
	}
	_ = w.Observe(noWS())
	other := &Error{Class: ClassTimeout}
	if got := w.Observe(other); !errors.Is(got, other) {
		t.Error("other errors pass through")
	}
	if err := w.Observe(noWS()); !IsClass(err, ClassTransient) {
		t.Errorf("another error resets the streak: %v", err)
	}
}

func TestWorkspaceWatchIgnoresCancellation(t *testing.T) {
	var w WorkspaceWatch
	_ = w.Observe(noWS())
	_ = w.Observe(noWS())
	if err := w.Observe(context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := w.Observe(noWS()); !IsClass(err, ClassNotWorkspace) {
		t.Errorf("a cancelled refresh is no observation: %v", err)
	}
}

func TestWorkspaceWatchWrapsUnclassifiedError(t *testing.T) {
	var w WorkspaceWatch
	wrapped := errors.New("wrapped")
	bare := &Error{Code: CodeNoBeadsDirectory, Err: wrapped}
	_ = w.Observe(bare)
	_ = w.Observe(bare)
	if err := w.Observe(bare); !IsClass(err, ClassNotWorkspace) || !errors.Is(err, wrapped) {
		t.Errorf("err = %v", err)
	}
}
