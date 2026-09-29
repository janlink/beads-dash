package bd

import (
	"context"
	"errors"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

// FetchSnapshot reads bd list, then bd ready --explain only if list
// succeeded, and decodes and derives the snapshot. At most one bd call is in
// flight, so a serialized runner can own it. It blocks for the duration of
// both calls, so run it from a command, never from the UI goroutine. The
// calls are not atomic: one snapshot can mix states and the next refresh
// corrects it. A failure is returned as bd reported it; run
// [ClassifyReadFailure] on it to learn whether the directory is no
// workspace.
func FetchSnapshot(ctx context.Context, c Client, now func() time.Time) (*model.Snapshot, error) {
	issues, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	ready, err := c.Ready(ctx)
	if err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return model.NewSnapshot(issues, ready, now()), nil
}

// ClassifyReadFailure refines a failed list or ready. bd reports a missing
// workspace on those commands as plain text only, so this asks c.Where once
// (one more bd call, run it in the same serialized slot as the read) and
// returns a copy of err with Code set to [CodeNoBeadsDirectory] when the
// directory is no workspace. Any other error is returned unchanged.
func ClassifyReadFailure(ctx context.Context, c Client, err error) error {
	var be *Error
	if !errors.As(err, &be) || be.Class != ClassTransient || be.Code != "" || be.ExitCode == 0 ||
		(be.Command != "list" && be.Command != "ready") {
		return err
	}
	if _, werr := c.Where(ctx); !IsClass(werr, ClassNotWorkspace) {
		return err
	}
	refined := *be
	refined.Code = CodeNoBeadsDirectory
	return &refined
}
