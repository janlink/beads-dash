package ui

import (
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

// hintsFor is the hint list of a context; the generated hints and the dialog
// frame hints are the same list.
func (a *App) hintsFor(c keys.Context) []keys.Hint { return a.km.Hints(c) }

func (a *App) errorReport() screens.Report {
	return screens.Diagnose(a.reportInput(a.detailError(), false, false))
}

// detailError is the failure the details dialog explains: the engine's, the
// startup one, or the failed read of the memories.
func (a *App) detailError() error {
	switch {
	case a.status.Err != nil:
		return a.status.Err
	case a.startErr != nil:
		return a.startErr
	case a.inMemories() && a.mem.err != nil:
		return a.mem.err
	}
	return nil
}

func (a *App) reportInput(err error, first, vanished bool) screens.Input {
	return screens.Input{
		Err: err, Session: a.bds, FirstSnapshot: first, Vanished: vanished,
		BdPath: a.o.BdPath, Dir: a.o.Dir, BeadsDir: a.o.BeadsDir,
		BdashViaBrew: a.o.BdashViaBrew, BdashVersion: a.o.BdashVersion,
	}
}
