package refresh

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
)

// ErrNotRunning is returned by [Engine.Do] and [Engine.Write] when the engine
// is not running: before Start, or after Stop or the end of its context.
var ErrNotRunning = errors.New("refresh: engine is not running")

// Options configures [New].
type Options struct {
	// Client is the bd boundary. The engine owns it: every call it makes and
	// every call made through [Engine.Do] runs one at a time.
	Client bd.Client
	// Session comes from [bd.OpenSession]; EventsJournal selects the mode.
	Session bd.Session
	// Clock defaults to the wall clock.
	Clock Clock
	// Tunables defaults to [DefaultTunables].
	Tunables *Tunables
	// Actor is bd's actor name, credited with the effects of bdash's own
	// writes when no journal record names one.
	Actor string
	// Notify says whether notifications are on and for which kinds.
	Notify bool
	Kinds  model.KindSet
}

// Engine keeps a snapshot current and turns changes into activity events. One
// goroutine owns all state; bd is never called from two places at once.
type Engine struct {
	client bd.Client
	clk    Clock
	tun    Tunables
	actor  string

	fev chan followMsg

	lmu    sync.Mutex
	state  lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	wg     sync.WaitGroup

	qmu   sync.Mutex
	queue []func()
	wake  chan struct{}

	omu     sync.Mutex
	pending *Update
	owake   chan struct{}
	out     chan Update
	tap     func(Update)

	mu     sync.Mutex
	status Status
	ring   model.Ring

	sess      bd.Session
	snap      *model.Snapshot
	published *model.Snapshot
	watch     bd.WorkspaceWatch
	notify    bool
	kinds     model.KindSet
	focused   bool
	mode      Mode

	lastFullAt   time.Time
	lastGateAt   time.Time
	lastSnapDur  time.Duration
	lastGateDur  time.Duration
	lastSuccess  time.Time
	failures     int
	retryAt      time.Time
	lastErr      error
	slow         bool
	gcHint       bool
	samples      []time.Duration
	firstSample  time.Duration
	hash         string
	hashKnown    bool
	unreliable   bool
	plainPolls   int
	gateErrs     int
	hashSuspect  int
	debounceAt   time.Time
	prefillAt    time.Time
	prefillNew   []model.Event
	own          map[string]struct{}
	refreshingAt time.Time

	fw *follower
	fl followState
}

// New builds an engine; call [Engine.Start] to run it.
func New(o Options) *Engine {
	e := &Engine{
		client:  o.Client,
		clk:     o.Clock,
		actor:   o.Actor,
		wake:    make(chan struct{}, 1),
		owake:   make(chan struct{}, 1),
		out:     make(chan Update),
		fev:     make(chan followMsg),
		done:    make(chan struct{}),
		sess:    o.Session,
		notify:  o.Notify,
		kinds:   o.Kinds,
		focused: true,
		own:     map[string]struct{}{},
	}
	if e.clk == nil {
		e.clk = RealClock{}
	}
	if o.Tunables != nil {
		e.tun = *o.Tunables
	} else {
		e.tun = DefaultTunables()
	}
	if o.Session.EventsJournal {
		e.mode = Events
	}
	return e
}

type lifecycle int

const (
	idle lifecycle = iota
	running
	stopped
)

// Start runs the engine until ctx ends or [Engine.Stop] is called. The first
// refresh begins at once. A second call does nothing.
func (e *Engine) Start(ctx context.Context) {
	e.lmu.Lock()
	defer e.lmu.Unlock()
	if e.state != idle {
		return
	}
	e.state = running
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.wg.Add(1)
	go e.pump()
	go e.run()
}

// Stop cancels in-flight bd work, ends the engine and waits until every
// goroutine of it has exited. It is safe to call more than once and before
// Start.
func (e *Engine) Stop() {
	e.lmu.Lock()
	prev := e.state
	e.state = stopped
	cancel := e.cancel
	e.lmu.Unlock()
	if prev == idle {
		close(e.done)
		close(e.out)
		return
	}
	cancel()
	<-e.done
	e.wg.Wait()
}

func (e *Engine) isRunning() bool {
	e.lmu.Lock()
	defer e.lmu.Unlock()
	return e.state == running
}

// Updates is the stream of published [Update]s; it is closed when the engine
// stops. The engine never blocks on it: while the reader is busy, updates
// merge into one that carries the latest snapshot and status and every event
// since (the newest [model.RingCapacity] of them).
func (e *Engine) Updates() <-chan Update { return e.out }

// Events returns the activity feed, newest first.
func (e *Engine) Events() []model.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ring.Newest()
}

// Status returns the current health signals.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := e.status
	s.RefreshingSince = e.refreshingAt
	return s
}

// post queues work for the engine goroutine without blocking or doing any
// I/O on the caller's.
func (e *Engine) post(f func()) {
	e.qmu.Lock()
	e.queue = append(e.queue, f)
	e.qmu.Unlock()
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) takeQueue() []func() {
	e.qmu.Lock()
	defer e.qmu.Unlock()
	q := e.queue
	e.queue = nil
	return q
}

// Refresh posts a full refresh that bypasses the gate and any back-off and
// reloads the status and type tables. It returns at once.
func (e *Engine) Refresh() {
	e.post(func() {
		if e.sess.RefreshMeta(e.ctx, e.client) == nil && e.fl.tracker != nil {
			e.fl.tracker.SetStatuses(e.sess.Statuses)
		}
		e.refresh(e.ctx, trigManual)
	})
}

// SetFocus posts terminal focus. Blurring pauses or slows polling; regaining
// focus refreshes at once.
func (e *Engine) SetFocus(focused bool) {
	e.post(func() {
		if e.focused == focused {
			return
		}
		e.focused = focused
		if focused {
			e.refresh(e.ctx, trigFocus)
			return
		}
		e.publishStatusIfChanged()
	})
}

// SetNotify posts whether notifications are on and for which kinds.
func (e *Engine) SetNotify(on bool, kinds model.KindSet) {
	e.post(func() {
		e.notify, e.kinds = on, kinds
		e.publishStatusIfChanged()
	})
}

// Do runs fn in the engine's serialized bd queue, so it never overlaps a
// refresh, a write or another Do, and waits for it. It is for lazy reads. It
// blocks, so call it from a tea.Cmd, never from Update or View. fn's context
// ends when ctx ends or the engine stops.
func (e *Engine) Do(ctx context.Context, fn func(context.Context, bd.Client) error) error {
	return e.exec(ctx, nil, false, fn)
}

type ownKey struct{}

// Own marks ids as the caller's own writes from inside a function passed to
// [Engine.Write], for issues that only exist once the write has run, such as
// a created one. Outside a write it does nothing.
func Own(ctx context.Context, ids ...string) {
	if own, ok := ctx.Value(ownKey{}).(func(...string)); ok {
		own(ids...)
	}
}

// Write runs a bd write in the queue: it marks ids as own writes, runs fn, and
// refreshes at once, all in one queued step so no other refresh can slip in
// between. The events those issues show are credited to the actor, highlighted
// and never notified. The refresh happens even when fn fails, because bd's
// exit code can lie about partial results. Blocking, like [Engine.Do].
func (e *Engine) Write(ctx context.Context, ids []string, fn func(context.Context, bd.Client) error) error {
	return e.exec(ctx, ids, true, fn)
}

func (e *Engine) exec(ctx context.Context, ids []string, write bool, fn func(context.Context, bd.Client) error) error {
	if !e.isRunning() {
		return ErrNotRunning
	}
	res := make(chan error, 1)
	e.post(func() {
		wctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer context.AfterFunc(e.ctx, cancel)()
		if err := wctx.Err(); err != nil {
			res <- err
			return
		}
		if write {
			own := func(ids ...string) {
				for _, id := range ids {
					if id != "" {
						e.own[id] = struct{}{}
					}
				}
			}
			own(ids...)
			wctx = context.WithValue(wctx, ownKey{}, own)
		}
		err := fn(wctx, e.client)
		if write {
			e.refresh(e.ctx, trigWrite)
		}
		res <- err
	})
	select {
	case err := <-res:
		return err
	case <-e.done:
		return ErrNotRunning
	case <-ctx.Done():
		return ctx.Err()
	}
}

// barrier returns once the engine has run everything posted before it and
// everything due on the clock. Tests use it.
func (e *Engine) barrier() {
	fin := make(chan struct{})
	e.post(func() {
		e.settle()
		close(fin)
	})
	select {
	case <-fin:
	case <-e.done:
	}
}

func (e *Engine) run() {
	ctx := e.ctx
	defer close(e.done)
	defer e.stopFollower()
	timer := e.clk.NewTimer(0)
	defer timer.Stop()
	e.refresh(ctx, trigStart)
	for {
		e.arm(timer)
		select {
		case <-ctx.Done():
			return
		case <-e.wake:
			for _, f := range e.takeQueue() {
				f()
			}
		case m := <-e.fev:
			e.onFollow(m)
		case <-timer.C():
			e.settle()
		}
	}
}

func (e *Engine) arm(t Timer) {
	at, ok := e.nextDue()
	if !ok {
		t.Stop()
		return
	}
	t.Reset(at.Sub(e.clk.Now()))
}

// settle ticks until nothing is due, with a bound against a clock that
// cannot move.
func (e *Engine) settle() {
	for range 16 {
		if !e.tick(e.ctx) {
			return
		}
	}
}

// pump hands merged updates to the reader of [Engine.Updates].
func (e *Engine) pump() {
	defer e.wg.Done()
	defer close(e.out)
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-e.owake:
		}
		for {
			e.omu.Lock()
			u := e.pending
			e.pending = nil
			e.omu.Unlock()
			if u == nil {
				break
			}
			select {
			case e.out <- *u:
			case <-e.ctx.Done():
				return
			}
		}
	}
}

func (e *Engine) enqueue(u Update) {
	if e.tap != nil {
		e.tap(u)
	}
	e.omu.Lock()
	if e.pending == nil {
		e.pending = &u
	} else {
		e.pending.merge(u)
	}
	e.omu.Unlock()
	select {
	case e.owake <- struct{}{}:
	default:
	}
}

func (p *Update) merge(u Update) {
	p.Snapshot, p.Status, p.Session = u.Snapshot, u.Status, u.Session
	p.Changed = p.Changed || u.Changed
	p.Events = append(p.Events, u.Events...)
	if n := len(p.Events) - model.RingCapacity; n > 0 {
		p.Events = slices.Clone(p.Events[n:])
	}
	for _, id := range u.Highlights {
		if !slices.Contains(p.Highlights, id) {
			p.Highlights = append(p.Highlights, id)
		}
	}
	p.Notification.Events = append(p.Notification.Events, u.Notification.Events...)
	p.Notification.Summary = p.Notification.Summary || u.Notification.Summary ||
		len(p.Notification.Events) > model.SummaryThreshold
}
