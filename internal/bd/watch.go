package bd

import (
	"context"
	"errors"
)

func isCanceled(err error) bool { return errors.Is(err, context.Canceled) }

// vanishLimit is how many consecutive refreshes must report a missing
// workspace before it counts as gone.
const vanishLimit = 3

// WorkspaceWatch applies the vanishing-workspace rule: a workspace that
// disappears at runtime is a transient failure until three consecutive
// refreshes report no_beads_directory, then a [ClassNotWorkspace] failure.
type WorkspaceWatch struct {
	streak int
}

// Observe feeds the outcome of one refresh (nil on success) and returns the
// error to surface: err itself, or a [ClassNotWorkspace] error on the third
// consecutive missing-workspace failure.
func (w *WorkspaceWatch) Observe(err error) error {
	if isCanceled(err) {
		return err
	}
	if err == nil || CodeOf(err) != CodeNoBeadsDirectory {
		w.streak = 0
		return err
	}
	w.streak++
	if w.streak < vanishLimit {
		return err
	}
	var be *Error
	if errors.As(err, &be) {
		out := *be
		out.Class = ClassNotWorkspace
		return &out
	}
	return &Error{Class: ClassNotWorkspace, Code: CodeNoBeadsDirectory, Err: err}
}
