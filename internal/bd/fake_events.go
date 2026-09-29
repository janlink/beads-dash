package bd

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"time"
)

// journalState is the fake's events journal and its open follow streams,
// guarded by its own lock so client hooks may call the Fake freely.
type journalState struct {
	mu      sync.Mutex
	cond    *sync.Cond
	records []Event
	streams []*fakeStream
	starts  []int64
}

func (j *journalState) init() {
	if j.cond == nil {
		j.cond = sync.NewCond(&j.mu)
	}
}

// waitGuard bounds how long the fake's synchronisation helpers wait for the
// code under test, so a broken test fails instead of hanging.
const waitGuard = 10 * time.Second

// waitLocked waits on the journal cond until done reports true; j.mu must be
// held. It panics after [waitGuard].
func (j *journalState) waitLocked(what string, done func() bool) {
	expired := false
	timer := time.AfterFunc(waitGuard, func() {
		j.mu.Lock()
		expired = true
		j.cond.Broadcast()
		j.mu.Unlock()
	})
	defer timer.Stop()
	for !done() {
		if expired {
			panic("bd.Fake: timed out waiting for " + what)
		}
		j.cond.Wait()
	}
}

// AppendEvents adds records to the fake's journal without waiting for any
// follower. A zero Seq gets the next number.
func (f *Fake) AppendEvents(evs ...Event) {
	j := &f.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	j.init()
	for _, e := range evs {
		if e.Seq == 0 {
			e.Seq = 1
			if n := len(j.records); n > 0 {
				e.Seq = j.records[n-1].Seq + 1
			}
		}
		j.records = append(j.records, e)
	}
	j.cond.Broadcast()
}

// Emit appends records and waits until every open follow stream has handed
// them to its reader and asked for the next one, so the consumer has
// received them when Emit returns.
func (f *Fake) Emit(evs ...Event) {
	f.AppendEvents(evs...)
	f.DrainFollowers()
}

// DrainFollowers waits until every open follow stream has delivered all
// records and is waiting for more.
func (f *Fake) DrainFollowers() {
	j := &f.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	j.init()
	j.waitLocked("follow streams to drain", func() bool {
		for _, s := range j.streams {
			if !s.closed && (!s.waiting || s.pos < len(j.after(s.since))) {
				return false
			}
		}
		return true
	})
}

// KillFollowers makes every open follow stream fail with err after the
// records already delivered, and waits until each has been closed by its
// consumer.
func (f *Fake) KillFollowers(err error) {
	j := &f.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	j.init()
	open := slices.Clone(j.streams)
	for _, s := range open {
		if !s.closed {
			s.fail = err
		}
	}
	j.cond.Broadcast()
	j.waitLocked("follow streams to close", func() bool {
		for _, s := range open {
			if !s.closed {
				return false
			}
		}
		return true
	})
}

// Truncate ends every open follow stream the way bd does when the journal
// was pruned past the follower.
func (f *Fake) Truncate(floor, head int64) {
	f.KillFollowers(&JournalTruncatedError{Floor: floor, Head: head})
}

// OpenFollowers is the number of follow streams not yet closed.
func (f *Fake) OpenFollowers() int {
	j := &f.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	n := 0
	for _, s := range j.streams {
		if !s.closed {
			n++
		}
	}
	return n
}

// FollowStarts lists the since value of every EventsFollow call so far.
func (f *Fake) FollowStarts() []int64 {
	j := &f.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	return slices.Clone(j.starts)
}

func (j *journalState) after(since int64) []Event {
	i, _ := slices.BinarySearchFunc(j.records, since, func(e Event, s int64) int {
		switch {
		case e.Seq <= s:
			return -1
		default:
			return 1
		}
	})
	return j.records[i:]
}

// EventsFollow implements [Client]: a stream of the fake's journal records
// after since, then whatever is appended while it is open.
func (f *Fake) EventsFollow(ctx context.Context, since int64) (EventStream, error) {
	if err := f.enter(ctx, "EventsFollow"); err != nil {
		return nil, err
	}
	j := &f.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	j.init()
	s := &fakeStream{j: j, since: since}
	j.streams = append(j.streams, s)
	j.starts = append(j.starts, since)
	context.AfterFunc(ctx, func() { _ = s.Close() })
	return s, nil
}

type fakeStream struct {
	j       *journalState
	since   int64
	pos     int
	waiting bool
	closed  bool
	fail    error
}

func (s *fakeStream) Next() (Event, error) {
	j := s.j
	j.mu.Lock()
	defer j.mu.Unlock()
	for {
		if s.closed {
			return Event{}, io.EOF
		}
		if recs := j.after(s.since); s.pos < len(recs) {
			e := recs[s.pos]
			s.pos++
			s.waiting = false
			j.cond.Broadcast()
			return e, nil
		}
		if s.fail != nil {
			err := s.fail
			s.fail = nil
			var trunc *JournalTruncatedError
			if errors.As(err, &trunc) && trunc.Since == 0 {
				t := *trunc
				t.Since = s.since
				err = &t
			}
			s.waiting = false
			j.cond.Broadcast()
			return Event{}, err
		}
		s.waiting = true
		j.cond.Broadcast()
		j.cond.Wait()
	}
}

func (s *fakeStream) Close() error {
	s.j.mu.Lock()
	defer s.j.mu.Unlock()
	s.closed = true
	s.j.cond.Broadcast()
	return nil
}
