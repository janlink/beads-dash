package refresh

import (
	"time"

	"github.com/janlink/beads-dash/internal/config"
)

// Tunables are the engine's timing knobs. The first six mirror the
// refresh.* settings; the rest are fixed by the design and only tests change
// them.
type Tunables struct {
	// GatePolling and GateEvents are the vc status gate intervals per mode.
	GatePolling time.Duration
	GateEvents  time.Duration
	// SafetyPolling and SafetyEvents are the floors of the full-poll safety
	// net per mode.
	SafetyPolling time.Duration
	SafetyEvents  time.Duration
	// PlainPoll is the full-poll interval when the vc hash is unreliable.
	PlainPoll time.Duration
	// UnfocusedFactor slows gate and safety poll while the terminal is blurred
	// and notifications are on.
	UnfocusedFactor int

	// Debounce batches journal records into one refresh.
	Debounce time.Duration
	// BackoffBase and BackoffMax bound the 2, 4, 8 ... retry schedule.
	BackoffBase time.Duration
	BackoffMax  time.Duration
	// SlowAfter is when a refresh counts as slow bd.
	SlowAfter time.Duration
	// SafetyCost scales the last snapshot duration into a lower bound of the
	// safety interval; SlowGate does the same for gate and plain polls.
	SafetyCost int
	SlowGate   int
	// GCHintFloor and GCHintRatio trigger the gc hint: the median vc status
	// duration reaching the floor, or the ratio times the first sample.
	GCHintFloor time.Duration
	GCHintRatio int
	// FollowerDeaths is how many consecutive follower deaths fall back to
	// polling; a follower alive for FollowerStable or that delivered a record
	// starts the count over.
	FollowerDeaths int
	FollowerStable time.Duration
	// HashReprobe is how many plain polls pass before vc status is asked
	// again to see whether the hash works after all.
	HashReprobe int
	// SeqResetRefreshes is how many consecutive periodic refreshes may find
	// changes without any journal record before the follower restarts at 0.
	SeqResetRefreshes int
}

// DefaultTunables returns the values of the design.
func DefaultTunables() Tunables {
	return TunablesFrom(config.Defaults().Refresh)
}

// TunablesFrom applies the refresh.* settings on top of the fixed values.
func TunablesFrom(r config.Refresh) Tunables {
	sec := func(n int) time.Duration { return time.Duration(n) * time.Second }
	return Tunables{
		GatePolling:       sec(r.GatePolling),
		GateEvents:        sec(r.GateEvents),
		SafetyPolling:     sec(r.SafetyPolling),
		SafetyEvents:      sec(r.SafetyEvents),
		PlainPoll:         sec(r.PlainPoll),
		UnfocusedFactor:   r.UnfocusedFactor,
		Debounce:          300 * time.Millisecond,
		BackoffBase:       2 * time.Second,
		BackoffMax:        60 * time.Second,
		SlowAfter:         3 * time.Second,
		SafetyCost:        10,
		SlowGate:          2,
		GCHintFloor:       500 * time.Millisecond,
		GCHintRatio:       5,
		FollowerDeaths:    3,
		FollowerStable:    60 * time.Second,
		HashReprobe:       12,
		SeqResetRefreshes: 2,
	}
}

func (t Tunables) backoff(failures int) time.Duration {
	d := t.BackoffBase
	for i := 1; i < failures && d < t.BackoffMax; i++ {
		d *= 2
	}
	return min(d, t.BackoffMax)
}
