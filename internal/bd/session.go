package bd

import (
	"context"

	"github.com/janlink/beads-dash/internal/model"
)

// Session is what bdash learns once per run and on manual refresh, outside
// the snapshot: the bd build, the workspace, and status and type metadata.
type Session struct {
	Version   VersionInfo
	Workspace Workspace
	Statuses  model.Statuses
	Types     []TypeInfo
	// Untested is set when bd is newer than the newest tested line.
	Untested bool
	// EventsJournal reports whether the workspace's events journal is on;
	// always false for a bd without one.
	EventsJournal bool
}

// OpenSession runs the startup probes in order: version (refusing an
// unsupported bd), where (refusing a directory that is no workspace), then
// statuses, types and the journal state. A failure of the journal probe is
// not fatal; the journal counts as off.
func OpenSession(ctx context.Context, c Client) (Session, error) {
	var s Session
	v, err := c.Version(ctx)
	s.Version = v
	s.Untested = v.Support == Untested
	if err != nil {
		return s, err
	}
	if s.Workspace, err = c.Where(ctx); err != nil {
		return s, err
	}
	if err := s.RefreshMeta(ctx, c); err != nil {
		return s, err
	}
	if on, err := journalEnabled(ctx, c, v.caps); err == nil {
		s.EventsJournal = on
	}
	return s, nil
}

// RefreshMeta reloads the status and type tables in place.
func (s *Session) RefreshMeta(ctx context.Context, c Client) error {
	st, err := c.Statuses(ctx)
	if err != nil {
		return err
	}
	ty, err := c.Types(ctx)
	if err != nil {
		return err
	}
	s.Statuses, s.Types = st, ty
	return nil
}
