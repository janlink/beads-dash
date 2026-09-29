package refresh

import (
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
)

// Mode is how the engine learns about changes.
type Mode int

const (
	// Polling watches the vc status hash and polls in full periodically.
	Polling Mode = iota
	// Events follows the events journal and keeps the vc gate and a slow poll
	// as a safety net.
	Events
)

func (m Mode) String() string {
	if m == Events {
		return "events"
	}
	return "polling"
}

// Status is the health of the engine as the UI draws it. The engine only
// reports signals; presentation is up to the caller.
type Status struct {
	// Mode is the mode in force; Fallback is set when events mode gave up
	// after repeated follower deaths and the engine polls instead.
	Mode     Mode
	Fallback bool
	// Following is set while a journal follower is running.
	Following bool
	// Loaded is set once a snapshot exists.
	Loaded bool
	// Stale is set when a snapshot exists but the latest refresh failed. Its
	// age is now minus LastSuccess.
	Stale       bool
	LastSuccess time.Time
	// Err is the latest refresh failure, nil after a success; Failures counts
	// them in a row and NextRetry says when the engine tries again.
	Err       error
	Failures  int
	NextRetry time.Time
	// Slow is set when the latest refresh took at least SlowAfter;
	// LastRefresh is its duration. A refresh in flight shows as
	// RefreshingSince, so a caller can raise the signal while bd is stuck.
	Slow            bool
	LastRefresh     time.Duration
	RefreshingSince time.Time
	// GCHint is set, and stays set, once vc status is slow enough to suggest
	// bd gc --skip-decay.
	GCHint bool
	// HashUnreliable is set when the vc status hash does not track changes
	// and the engine polls in full at the plain interval.
	HashUnreliable bool
	// Focused and Paused: an unfocused terminal without notifications pauses
	// polling.
	Focused bool
	Paused  bool
}

func (s Status) same(o Status) bool {
	errText := func(e error) string {
		if e == nil {
			return ""
		}
		return e.Error()
	}
	a, b := s, o
	ea, eb := errText(a.Err), errText(b.Err)
	a.Err, b.Err = nil, nil
	return a == b && ea == eb
}

// Update is what the engine publishes: after every refresh that changed
// something, after a failed refresh, and when the activity feed grew.
type Update struct {
	// Snapshot is the current snapshot; nil until the first refresh succeeded.
	Snapshot *model.Snapshot
	// Changed is set when Snapshot differs from the one of the previous
	// update.
	Changed bool
	// Events are the activity events added since the previous update, oldest
	// first: live ones from this refresh, or prefill.
	Events []model.Event
	// Highlights is the change-highlight trigger set of this refresh.
	Highlights []string
	// Notification is the candidate stream of this refresh; empty when
	// notifications are off.
	Notification model.Notification
	Status       Status
	Session      bd.Session
}
