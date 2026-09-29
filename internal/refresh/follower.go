package refresh

import (
	"context"
	"errors"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
)

const liveCap = 1000

type followMsg struct {
	gen int
	rec *bd.Event
	err error
}

type follower struct {
	gen     int
	stream  bd.EventStream
	stop    chan struct{}
	started time.Time
	records int
}

// followState is the journal-follow bookkeeping owned by the engine goroutine.
type followState struct {
	gen       int
	lastSeq   int64
	deaths    int
	restartAt time.Time
	resumeAt  int64
	fallback  bool
	tracker   *model.JournalTracker
	// boundary separates journal history from live records: with history set,
	// records older than it become prefill; without, they only feed the
	// tracker.
	boundary time.Time
	history  bool
	// live are the records seen since the last refresh, for actor attribution.
	live       []model.JournalRecord
	recsSince  int
	suspicious int
	// resetProbe is the last seq seen before a seq-reset restart: replaying
	// up to it proves the journal did not reset, and resetDisabled then ends
	// the restarts, since the unannounced changes have another cause.
	resetProbe    int64
	resetDisabled bool
}

func (f *followState) active() bool { return f.tracker != nil }

// startFollower opens the journal stream from the resume point. The first
// start and a seq-reset restart read from 0.
func (e *Engine) startFollower(ctx context.Context) {
	e.fl.restartAt = time.Time{}
	if e.fw != nil || e.fl.fallback || e.mode != Events {
		return
	}
	if e.fl.resumeAt == 0 || e.fl.tracker == nil {
		e.fl.tracker = model.NewJournalTracker(e.sess.Statuses)
	}
	stream, err := e.client.EventsFollow(ctx, e.fl.resumeAt)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		e.followerDied(err, nil)
		return
	}
	e.fl.gen++
	f := &follower{gen: e.fl.gen, stream: stream, stop: make(chan struct{}), started: e.clk.Now()}
	e.fw = f
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.read(f)
	}()
}

func (e *Engine) read(f *follower) {
	defer f.stream.Close()
	for {
		ev, err := f.stream.Next()
		m := followMsg{gen: f.gen}
		if err != nil {
			m.err = err
		} else {
			m.rec = &ev
		}
		select {
		case e.fev <- m:
		case <-f.stop:
			return
		}
		if err != nil {
			return
		}
	}
}

func (e *Engine) stopFollower() {
	if e.fw == nil {
		return
	}
	close(e.fw.stop)
	_ = e.fw.stream.Close()
	e.fw = nil
}

func (e *Engine) onFollow(m followMsg) {
	f := e.fw
	if f == nil || m.gen != f.gen {
		return
	}
	if m.err != nil {
		e.followerDied(m.err, f)
		e.publishStatusIfChanged()
		return
	}
	f.records++
	rec := m.rec.Record()
	e.fl.lastSeq = rec.Seq
	e.fl.resumeAt = rec.Seq
	e.fl.deaths = 0
	e.fl.suspicious = 0
	if e.fl.resetProbe > 0 && rec.Seq >= e.fl.resetProbe {
		e.fl.resetProbe = 0
		e.fl.resetDisabled = true
	}
	evs := e.fl.tracker.Apply(rec)
	if !rec.Time.IsZero() && rec.Time.Before(e.fl.boundary) {
		if e.fl.history {
			e.bufferPrefill(evs)
		}
		return
	}
	e.fl.live = append(e.fl.live, rec)
	if n := len(e.fl.live) - liveCap; n > 0 {
		e.fl.live = e.fl.live[n:]
	}
	e.fl.recsSince++
	if e.debounceAt.IsZero() {
		e.debounceAt = e.clk.Now().Add(e.tun.Debounce)
	}
}

// followerDied handles the end of a follower: a truncated journal resumes at
// its floor at once, anything else after the back-off from the last seq, and
// the third death in a row gives up on events.
func (e *Engine) followerDied(err error, f *follower) {
	now := e.clk.Now()
	e.stopFollower()
	if f != nil && now.Sub(f.started) >= e.tun.FollowerStable {
		e.fl.deaths = 0
	}
	e.fl.deaths++
	if e.fl.deaths >= e.tun.FollowerDeaths {
		e.fl.fallback = true
		e.fl.restartAt = time.Time{}
		e.fl.live, e.fl.recsSince, e.fl.tracker = nil, 0, nil
		e.debounceAt = now
		return
	}
	var tr *bd.JournalTruncatedError
	switch {
	case errors.As(err, &tr):
		e.fl.resumeAt = max(tr.Floor-1, 0)
		e.fl.restartAt = now
	default:
		e.fl.resumeAt = e.fl.lastSeq
		e.fl.restartAt = now.Add(e.tun.backoff(e.fl.deaths))
	}
	e.debounceAt = e.fl.restartAt
}

// checkSeqReset restarts the follower at seq 0 when periodic refreshes keep
// finding changes that no journal record announced: a reset journal looks
// like a follower that has caught up.
func (e *Engine) checkSeqReset(ctx context.Context, trig trigger, changed, sawRecords bool) {
	if !trig.periodic() || e.fw == nil || e.fl.resetDisabled || e.fl.lastSeq == 0 {
		return
	}
	if !changed || sawRecords {
		e.fl.suspicious = 0
		return
	}
	e.fl.suspicious++
	if e.fl.suspicious < e.tun.SeqResetRefreshes {
		return
	}
	e.fl.suspicious = 0
	e.stopFollower()
	e.fl.resetProbe = e.fl.lastSeq
	e.fl.resumeAt, e.fl.lastSeq = 0, 0
	e.fl.history = false
	e.fl.boundary = e.clk.Now()
	e.fl.live, e.fl.recsSince = nil, 0
	e.startFollower(ctx)
}
