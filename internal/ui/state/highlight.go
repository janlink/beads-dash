package state

import (
	"slices"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

// Bucket is the granularity highlights expire at: a burst of changes ends in
// one redraw.
const Bucket = time.Second

// Highlights is the change-highlight clock. Each highlight has a duration of
// visible time: it counts down only while the clock runs, so a blurred
// terminal or an open dialog stops it, and expiry falls on whole-second
// buckets.
type Highlights struct {
	duration time.Duration
	paused   bool
	items    map[string]*highlight
}

type highlight struct {
	remaining time.Duration
	since     time.Time
	events    []model.Event
}

// NewHighlights returns a clock; a duration of 0 turns highlights off.
func NewHighlights(d time.Duration) *Highlights {
	return &Highlights{duration: d, items: map[string]*highlight{}}
}

// Trigger starts or restarts the highlight of every id with its full
// duration. events are the ones that caused it and are kept for the detail.
func (h *Highlights) Trigger(now time.Time, ids []string, events []model.Event) {
	if h.duration <= 0 {
		return
	}
	for _, id := range ids {
		it := h.items[id]
		if it == nil {
			it = &highlight{}
			h.items[id] = it
		}
		it.remaining = h.duration
		it.since = time.Time{}
		if !h.paused {
			it.since = now
		}
		for _, e := range events {
			if e.IssueID == id && !e.Prefill {
				it.events = append(it.events, e)
			}
		}
	}
}

// SetPaused stops or resumes the clock; the remaining time of every running
// highlight is settled at the moment of the change.
func (h *Highlights) SetPaused(now time.Time, paused bool) {
	if paused == h.paused {
		return
	}
	h.paused = paused
	for _, it := range h.items {
		if paused {
			if !it.since.IsZero() {
				it.remaining = max(it.remaining-now.Sub(it.since), 0)
			}
			it.since = time.Time{}
		} else {
			it.since = now
		}
	}
}

func (h *Highlights) end(it *highlight, now time.Time) time.Time {
	if it.since.IsZero() {
		return now.Add(it.remaining)
	}
	return it.since.Add(it.remaining).Truncate(Bucket).Add(Bucket)
}

func (h *Highlights) live(it *highlight, now time.Time) bool {
	if it.since.IsZero() {
		return it.remaining > 0
	}
	return now.Before(h.end(it, now))
}

// Live reports whether id is highlighted at now.
func (h *Highlights) Live(id string, now time.Time) bool {
	it := h.items[id]
	return it != nil && h.live(it, now)
}

// IDs lists the live highlights sorted.
func (h *Highlights) IDs(now time.Time) []string {
	var out []string
	for id, it := range h.items {
		if h.live(it, now) {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

// Events returns the events behind id's highlight, oldest first.
func (h *Highlights) Events(id string) []model.Event {
	if it := h.items[id]; it != nil {
		return slices.Clone(it.events)
	}
	return nil
}

// Expire drops the highlights that have ended and returns their IDs.
func (h *Highlights) Expire(now time.Time) []string {
	var gone []string
	for id, it := range h.items {
		if !h.live(it, now) {
			delete(h.items, id)
			gone = append(gone, id)
		}
	}
	slices.Sort(gone)
	return gone
}

// Next is how long until the earliest highlight ends; ok is false when the
// clock is paused or nothing is highlighted, so no tick is needed.
func (h *Highlights) Next(now time.Time) (time.Duration, bool) {
	if h.paused {
		return 0, false
	}
	var best time.Time
	for _, it := range h.items {
		if it.since.IsZero() {
			continue
		}
		if e := h.end(it, now); best.IsZero() || e.Before(best) {
			best = e
		}
	}
	if best.IsZero() {
		return 0, false
	}
	return max(best.Sub(now), 0), true
}

// Prune drops highlights of issues that no longer exist.
func (h *Highlights) Prune(exists func(id string) bool) {
	for id := range h.items {
		if !exists(id) {
			delete(h.items, id)
		}
	}
}
