package refresh

import (
	"sort"
	"sync"
	"time"
)

// Clock is the engine's time source. Tests replace it with a [FakeClock].
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
}

// Timer is a resettable one-shot timer. A Reset or Stop leaves no stale tick
// in C.
type Timer interface {
	C() <-chan time.Time
	Reset(d time.Duration)
	Stop()
}

// RealClock is the wall clock.
type RealClock struct{}

// Now implements [Clock].
func (RealClock) Now() time.Time { return time.Now() }

// NewTimer implements [Clock]; the timer starts stopped.
func (RealClock) NewTimer(d time.Duration) Timer {
	t := time.NewTimer(d)
	if !t.Stop() {
		<-t.C
	}
	return &realTimer{t: t}
}

type realTimer struct{ t *time.Timer }

func (r *realTimer) C() <-chan time.Time { return r.t.C }

func (r *realTimer) Reset(d time.Duration) {
	r.Stop()
	r.t.Reset(d)
}

func (r *realTimer) Stop() {
	if !r.t.Stop() {
		select {
		case <-r.t.C:
		default:
		}
	}
}

// FakeClock is a manually advanced clock. Advance fires due timers
// synchronously, so a channel receive after Advance never races the clock.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

// NewFakeClock starts at the given time.
func NewFakeClock(start time.Time) *FakeClock { return &FakeClock{now: start} }

// Now implements [Clock].
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// NewTimer implements [Clock]; the timer starts stopped.
func (c *FakeClock) NewTimer(time.Duration) Timer {
	t := &fakeTimer{c: c, ch: make(chan time.Time, 1)}
	c.mu.Lock()
	c.timers = append(c.timers, t)
	c.mu.Unlock()
	return t
}

// Advance moves time forward and fires every timer that became due, in
// deadline order, each seeing the time of its own deadline.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	target := c.now.Add(d)
	for {
		var due []*fakeTimer
		for _, t := range c.timers {
			if t.armed && !t.at.After(target) {
				due = append(due, t)
			}
		}
		if len(due) == 0 {
			break
		}
		sort.SliceStable(due, func(i, j int) bool { return due[i].at.Before(due[j].at) })
		t := due[0]
		if t.at.After(c.now) {
			c.now = t.at
		}
		t.fire(c.now)
	}
	c.now = target
}

type fakeTimer struct {
	c     *FakeClock
	ch    chan time.Time
	at    time.Time
	armed bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) fire(now time.Time) {
	t.armed = false
	select {
	case t.ch <- now:
	default:
	}
}

func (t *fakeTimer) Reset(d time.Duration) {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	t.drain()
	t.at = t.c.now.Add(d)
	t.armed = true
	if d <= 0 {
		t.fire(t.c.now)
	}
}

func (t *fakeTimer) Stop() {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	t.armed = false
	t.drain()
}

func (t *fakeTimer) drain() {
	select {
	case <-t.ch:
	default:
	}
}
