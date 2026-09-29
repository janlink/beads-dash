package refresh

import (
	"context"
	"slices"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
)

type trigger int

const (
	trigStart trigger = iota
	trigManual
	trigWrite
	trigFocus
	trigRetry
	trigDebounce
	trigSafety
	trigPlain
	trigGate
)

// periodic triggers are the ones that see changes nobody announced.
func (t trigger) periodic() bool { return t == trigSafety || t == trigPlain || t == trigGate }

const gcSamples = 9

func (e *Engine) paused() bool { return !e.focused && !e.notify }

func (e *Engine) focusFactor() time.Duration {
	if !e.focused && e.notify {
		return time.Duration(max(e.tun.UnfocusedFactor, 1))
	}
	return 1
}

func (e *Engine) eventsActive() bool { return e.mode == Events && !e.fl.fallback }

func (e *Engine) gateInterval() time.Duration {
	base := e.tun.GatePolling
	if e.eventsActive() {
		base = e.tun.GateEvents
	}
	return max(base, time.Duration(e.tun.SlowGate)*e.lastGateDur) * e.focusFactor()
}

func (e *Engine) safetyInterval() time.Duration {
	floor := e.tun.SafetyPolling
	if e.eventsActive() {
		floor = e.tun.SafetyEvents
	}
	return max(floor, time.Duration(e.tun.SafetyCost)*e.lastSnapDur) * e.focusFactor()
}

func (e *Engine) plainInterval() time.Duration {
	return max(e.tun.PlainPoll, time.Duration(e.tun.SlowGate)*e.lastSnapDur) * e.focusFactor()
}

// plainPolling is the fallback when the vc hash cannot tell changes: full
// polls at the plain interval and no gate.
func (e *Engine) plainPolling() bool { return e.unreliable && !e.eventsActive() }

func (e *Engine) gateDue() (time.Time, bool) {
	if e.unreliable {
		return time.Time{}, false
	}
	last := e.lastFullAt
	if e.lastGateAt.After(last) {
		last = e.lastGateAt
	}
	return last.Add(e.gateInterval()), true
}

func (e *Engine) safetyDue() (time.Time, bool) {
	if e.plainPolling() {
		return e.lastFullAt.Add(e.plainInterval()), true
	}
	return e.lastFullAt.Add(e.safetyInterval()), true
}

func earliest(ts ...time.Time) (time.Time, bool) {
	var out time.Time
	for _, t := range ts {
		if !t.IsZero() && (out.IsZero() || t.Before(out)) {
			out = t
		}
	}
	return out, !out.IsZero()
}

func (e *Engine) nextDue() (time.Time, bool) {
	restart := e.fl.restartAt
	if e.failures > 0 {
		return earliest(e.retryAt, restart, e.prefillAt)
	}
	if e.snap == nil {
		return earliest(restart, e.prefillAt)
	}
	if e.paused() {
		return earliest(restart, e.prefillAt)
	}
	gate, _ := e.gateDue()
	safety, _ := e.safetyDue()
	return earliest(restart, e.prefillAt, e.debounceAt, gate, safety)
}

func due(at, now time.Time) bool { return !at.IsZero() && !at.After(now) }

// tick does what has come due on the clock, at most one refresh or gate
// check per call, and reports whether it did anything.
func (e *Engine) tick(ctx context.Context) bool {
	now := e.clk.Now()
	did := false
	if due(e.fl.restartAt, now) {
		e.startFollower(ctx)
		e.publishStatusIfChanged()
		did = true
	}
	switch {
	case e.failures > 0:
		if due(e.retryAt, now) {
			e.refresh(ctx, trigRetry)
			did = true
		}
	case e.snap == nil || e.paused():
	default:
		gate, _ := e.gateDue()
		safety, _ := e.safetyDue()
		switch {
		case due(e.debounceAt, now):
			e.refresh(ctx, trigDebounce)
			did = true
		case due(safety, now):
			if e.plainPolling() {
				e.plainPolls++
				if e.plainPolls%max(e.tun.HashReprobe, 1) == 0 {
					e.reprobe(ctx)
				}
				e.refresh(ctx, trigPlain)
			} else {
				e.refresh(ctx, trigSafety)
			}
			did = true
		case due(gate, now):
			e.gateCheck(ctx)
			did = true
		}
	}
	if due(e.prefillAt, e.clk.Now()) {
		e.flushPrefill()
		did = true
	}
	return did
}

// reprobe asks vc status again while the hash counts as unreliable; a clean
// answer restores the gate.
func (e *Engine) reprobe(ctx context.Context) {
	vc, err := e.client.VCStatus(ctx)
	if err != nil || vc.Commit == "" {
		return
	}
	e.unreliable, e.gateErrs, e.hashSuspect = false, 0, 0
	e.hash, e.hashKnown = vc.Commit, true
}

func (e *Engine) flushPrefill() {
	evs := e.prefillNew
	if n := len(evs) - model.RingCapacity; n > 0 {
		evs = slices.Clone(evs[n:])
	}
	e.prefillNew, e.prefillAt = nil, time.Time{}
	e.mu.Lock()
	e.ring.Merge(evs...)
	e.mu.Unlock()
	e.emit(Update{Events: evs})
}

// gateCheck reads vc status and refreshes in full when the commit moved. A
// gate that cannot answer counts as a change; two such answers in a row while
// full refreshes work mark the hash unreliable.
func (e *Engine) gateCheck(ctx context.Context) {
	start := e.clk.Now()
	vc, err := e.client.VCStatus(ctx)
	if ctx.Err() != nil {
		return
	}
	e.lastGateAt = e.clk.Now()
	e.lastGateDur = e.lastGateAt.Sub(start)
	if err != nil {
		e.gateErrs++
		e.refresh(ctx, trigGate)
		return
	}
	e.gateErrs = 0
	e.sample(e.lastGateDur)
	if vc.Commit == "" {
		e.unreliable = true
		e.refresh(ctx, trigGate)
		return
	}
	moved := e.hashKnown && vc.Commit != e.hash
	e.hash, e.hashKnown = vc.Commit, true
	if moved {
		e.refresh(ctx, trigGate)
		return
	}
	e.publishStatusIfChanged()
}

func (e *Engine) sample(d time.Duration) {
	if len(e.samples) == 0 {
		e.firstSample = d
	}
	e.samples = append(e.samples, d)
	if len(e.samples) > gcSamples {
		e.samples = e.samples[1:]
	}
	if len(e.samples) < 3 {
		return
	}
	sorted := slices.Clone(e.samples)
	slices.Sort(sorted)
	median := sorted[len(sorted)/2]
	ratio := time.Duration(e.tun.GCHintRatio) * e.firstSample
	hot := median >= e.tun.GCHintFloor || (e.firstSample > 0 && median >= ratio)
	cool := median < e.tun.GCHintFloor/2 && (e.firstSample == 0 || median < ratio/2)
	switch {
	case hot:
		e.gcHint = true
	case cool:
		e.gcHint = false
	}
}

// readHash notes the vc commit before a full refresh so the gate does not
// fire again for a change the refresh is about to see. It reports whether the
// commit moved since the previous look.
func (e *Engine) readHash(ctx context.Context) (moved, ok bool) {
	if e.unreliable {
		return false, false
	}
	start := e.clk.Now()
	vc, err := e.client.VCStatus(ctx)
	if err != nil || vc.Commit == "" {
		return false, false
	}
	e.sample(e.clk.Now().Sub(start))
	moved = e.hashKnown && vc.Commit != e.hash
	e.hash, e.hashKnown = vc.Commit, true
	return moved, true
}

// refresh reads a full snapshot and publishes the outcome.
func (e *Engine) refresh(ctx context.Context, trig trigger) {
	if ctx.Err() != nil {
		return
	}
	start := e.clk.Now()
	e.mu.Lock()
	e.refreshingAt = start
	e.mu.Unlock()
	e.debounceAt = time.Time{}
	firstFetch := e.snap == nil

	moved, hashOK := false, false
	if trig == trigGate {
		hashOK = e.hashKnown
		moved = true
	} else {
		moved, hashOK = e.readHash(ctx)
	}
	snap, err := bd.FetchSnapshot(ctx, e.client, e.clk.Now)
	if err != nil {
		if ctx.Err() != nil {
			e.mu.Lock()
			e.refreshingAt = time.Time{}
			e.mu.Unlock()
			return
		}
		err = bd.ClassifyReadFailure(ctx, e.client, err)
	}
	err = e.watch.Observe(err)
	end := e.clk.Now()
	e.mu.Lock()
	e.refreshingAt = time.Time{}
	e.mu.Unlock()
	e.lastFullAt = end
	e.lastSnapDur = end.Sub(start)
	e.slow = e.lastSnapDur >= e.tun.SlowAfter

	if err != nil {
		e.failures++
		e.lastErr = err
		e.retryAt = end.Add(e.tun.backoff(e.failures))
		e.emit(Update{})
		return
	}
	e.failures, e.lastErr, e.retryAt = 0, nil, time.Time{}
	e.lastSuccess = end

	prev := e.snap
	changed := prev == nil || snap.Fingerprint() != prev.Fingerprint()
	var events []model.Event
	if changed {
		e.snap = snap
	}
	if prev != nil && changed {
		events = e.liveEvents(prev, snap, start)
	}
	e.trackReliability(trig, changed, moved, hashOK)
	sawRecords := e.fl.recsSince > 0
	e.fl.recsSince = 0
	clear(e.own)

	if firstFetch {
		e.fl.boundary, e.fl.history = start, true
		if e.eventsActive() {
			e.startFollower(ctx)
		} else {
			events = model.Prefill(snap)
			e.mu.Lock()
			e.ring.Merge(events...)
			e.mu.Unlock()
		}
	}
	e.checkSeqReset(ctx, trig, changed, sawRecords)

	u := Update{Events: events}
	if len(events) > 0 {
		u.Highlights = model.HighlightIDs(events)
		if e.notify {
			u.Notification = model.Notify(events, e.kinds)
		}
	}
	e.emit(u)
}

// liveEvents diffs two snapshots and credits the events to journal actors and
// to the viewer's own writes.
func (e *Engine) liveEvents(prev, cur *model.Snapshot, fetchStart time.Time) []model.Event {
	events := model.Diff(prev, cur, e.sess.Statuses)
	if e.fl.active() {
		model.AttributeActors(events, e.fl.live)
	}
	e.fl.live = slices.DeleteFunc(e.fl.live, func(r model.JournalRecord) bool {
		return r.Time.IsZero() || !r.Time.After(fetchStart)
	})
	for i := range events {
		ev := &events[i]
		if _, mine := e.own[ev.IssueID]; mine {
			ev.Own = true
			if ev.Actor == "" {
				ev.Actor = e.actor
			}
		}
	}
	e.mu.Lock()
	e.ring.Add(events...)
	e.mu.Unlock()
	return events
}

// bufferPrefill collects journal history, keeping only what the feed can
// hold; flushPrefill merges it once the burst pauses.
func (e *Engine) bufferPrefill(evs []model.Event) {
	if len(evs) == 0 {
		return
	}
	e.prefillNew = append(e.prefillNew, evs...)
	if n := len(e.prefillNew) - 2*model.RingCapacity; n > 0 {
		e.prefillNew = slices.Delete(e.prefillNew, 0, n+model.RingCapacity)
	}
	if e.prefillAt.IsZero() {
		e.prefillAt = e.clk.Now().Add(e.tun.Debounce)
	}
}

// trackReliability watches for a vc hash that misses changes: two safety
// polls in a row that found changes while the commit had not moved, or a gate
// that fails twice while refreshes succeed.
func (e *Engine) trackReliability(trig trigger, changed, moved, hashOK bool) {
	if e.gateErrs >= 2 {
		e.unreliable = true
	}
	if e.unreliable || !hashOK || trig != trigSafety {
		return
	}
	if changed && !moved {
		e.hashSuspect++
		if e.hashSuspect >= 2 {
			e.unreliable = true
		}
		return
	}
	e.hashSuspect = 0
}

func (e *Engine) statusNow() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	s := Status{
		Mode:           e.mode,
		Fallback:       e.fl.fallback,
		Following:      e.fw != nil,
		Loaded:         e.snap != nil,
		Stale:          e.snap != nil && e.failures > 0,
		LastSuccess:    e.lastSuccess,
		Err:            e.lastErr,
		Failures:       e.failures,
		NextRetry:      e.retryAt,
		Slow:           e.slow,
		LastRefresh:    e.lastSnapDur,
		GCHint:         e.gcHint,
		HashUnreliable: e.unreliable,
		Focused:        e.focused,
		Paused:         e.paused(),
	}
	return s
}

func (e *Engine) publishStatusIfChanged() {
	if !e.statusNow().same(e.lastStatus()) {
		e.emit(Update{})
	}
}

func (e *Engine) lastStatus() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

// emit completes u with the current state and publishes it unless it says
// nothing new.
func (e *Engine) emit(u Update) {
	st := e.statusNow()
	e.mu.Lock()
	prev := e.status
	e.status = st
	e.mu.Unlock()
	u.Changed = e.snap != e.published
	if !u.Changed && len(u.Events) == 0 && st.same(prev) {
		return
	}
	e.published = e.snap
	u.Snapshot, u.Status, u.Session = e.snap, st, e.sess
	e.enqueue(u)
}
