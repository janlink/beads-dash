package ui

import (
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

type ovRegion int

// The focusable regions in Tab order.
const (
	regFeed ovRegion = iota
	regAttention
	regActive
	regionCount
)

// Slots of the views an Overview row can jump to.
const (
	slotKanban = 3
	slotReady  = 4
)

// ovItem is one cursor stop of a region. view is the slot Enter jumps to;
// zero opens the detail.
type ovItem struct {
	id   string
	view int
}

// ovBox is where a region was drawn in the last frame.
type ovBox struct {
	x, y, w, h int
	// at maps a body line to the item it shows, -1 for none.
	at []int
}

// Overview is the dashboard: the activity feed beside a sidebar of counters,
// Needs attention and the active assignees. Below 80 cells the sidebar
// collapses to a three-line strip.
type Overview struct {
	built struct {
		snap  *model.Snapshot
		scope string
		feed  int
		first model.Event
		min   time.Time
		ok    bool
	}
	data  model.Overview
	items [regionCount][]ovItem
	all   map[string]bool
	ids   []string

	focus ovRegion
	cur   [regionCount]int
	// off is the first shown body line of each region; keep is the line the
	// last frame kept in view, so scrolling is not undone by the next frame.
	off  [regionCount]int
	keep [regionCount]int
	// held is set by scrolling: the next frame leaves the offset alone.
	held  [regionCount]bool
	boxes [regionCount]ovBox
	// drawn is set once a frame was rendered, which tells the regions shown.
	drawn bool
	// roomy is set while the regions are laid out in three columns, which
	// changes their reading order.
	roomy bool
	// exited is set when Tab left the last region for the detail panel: the
	// next Tab from the list starts over at the first region.
	exited bool
	// info is the scope label; unscoped is the label without a scope.
	info     scopeInfo
	unscoped string
}

// NewOverview returns the Overview.
func NewOverview() *Overview { return &Overview{} }

// Name implements View.
func (*Overview) Name() string { return "overview" }

// Scope implements View.
func (v *Overview) Scope(w int) string {
	if v.info.active {
		return v.info.fit(w)
	}
	if ansi.StringWidth(v.unscoped) > w {
		return ""
	}
	return v.unscoped
}

// Context implements View.
func (*Overview) Context() keys.Context { return keys.Overview }

func (v *Overview) ensure(env Env) {
	if env.Snap == nil || env.Matches == nil {
		v.built.ok = false
		v.data, v.all, v.ids = model.Overview{}, nil, nil
		v.items = [regionCount][]ovItem{}
		return
	}
	scope := env.Scope.Key()
	minute := env.Now.Truncate(time.Minute)
	var first model.Event
	if len(env.Feed) > 0 {
		first = env.Feed[0]
	}
	b := &v.built
	if b.ok && b.snap == env.Snap && b.scope == scope && b.feed == len(env.Feed) && b.first == first && b.min.Equal(minute) {
		return
	}
	b.snap, b.scope, b.feed, b.first, b.min, b.ok = env.Snap, scope, len(env.Feed), first, minute, true
	v.data = model.BuildOverview(env.Snap, env.Statuses, env.Scope, env.Matches, env.Feed, env.Now)
	v.items = [regionCount][]ovItem{}
	for _, e := range v.data.Feed {
		v.items[regFeed] = append(v.items[regFeed], ovItem{e.IssueID, 0})
	}
	a := v.data.Attention
	for _, id := range append(slices.Clone(a.Ready), a.Blocked...) {
		v.items[regAttention] = append(v.items[regAttention], ovItem{id, slotReady})
	}
	for _, g := range v.data.Active {
		for _, id := range g.IDs {
			v.items[regActive] = append(v.items[regActive], ovItem{id, slotKanban})
		}
	}
	v.all = map[string]bool{}
	v.ids = v.ids[:0]
	for _, r := range v.items {
		for _, it := range r {
			if !v.all[it.id] {
				v.all[it.id] = true
				v.ids = append(v.ids, it.id)
			}
		}
	}
}

// cursor is the index of the current issue in a region: the remembered stop
// when it still shows the current issue, else its first appearance; -1 when
// the region does not list it.
func (v *Overview) cursor(r ovRegion, current string) int {
	items := v.items[r]
	if i := v.cur[r]; i < len(items) && items[i].id == current {
		return i
	}
	return slices.IndexFunc(items, func(it ovItem) bool { return it.id == current })
}

// active is the region that has the keys: the focused one, or the next with
// something to point at while it is empty.
func (v *Overview) active() ovRegion {
	if len(v.items[v.focus]) > 0 {
		return v.focus
	}
	return v.nextRegion(v.focus)
}

// Sync implements Syncer: focus follows the current issue to a region that
// lists it.
func (v *Overview) Sync(env Env) {
	v.ensure(env)
	v.info, v.unscoped = scopeInfo{}, ""
	if env.Matches != nil {
		v.info = scopeInfoFor(env, v.data.Total, false)
		v.unscoped = fmt.Sprintf("%d issues · all", v.data.Total)
	}
	if v.cursor(v.active(), env.Current) >= 0 {
		return
	}
	for r := range regionCount {
		if v.cursor(r, env.Current) >= 0 {
			v.focus = r
			return
		}
	}
}

// order lists the regions the way they read on screen.
func (v *Overview) order() [regionCount]ovRegion {
	if v.roomy {
		return [regionCount]ovRegion{regFeed, regActive, regAttention}
	}
	return [regionCount]ovRegion{regFeed, regAttention, regActive}
}

// usable reports whether r has something to point at and was drawn.
func (v *Overview) usable(r ovRegion) bool {
	return len(v.items[r]) > 0 && (!v.drawn || v.boxes[r].w > 0)
}

// nextRegion is the usable region after r in reading order, wrapping; r
// itself when there is none.
func (v *Overview) nextRegion(r ovRegion) ovRegion {
	next, ok := v.after(r)
	if !ok {
		next = v.first(r)
	}
	return next
}

// after is the usable region after r in reading order, without wrapping.
func (v *Overview) after(r ovRegion) (ovRegion, bool) {
	ord := v.order()
	at := slices.Index(ord[:], r)
	for _, n := range ord[at+1:] {
		if v.usable(n) {
			return n, true
		}
	}
	return r, false
}

// first is the first usable region in reading order; fallback when none is.
func (v *Overview) first(fallback ovRegion) ovRegion {
	for _, n := range v.order() {
		if v.usable(n) {
			return n
		}
	}
	return fallback
}

// Visible implements View: the feed's issues, then Needs attention's, then
// the active assignees'.
func (v *Overview) Visible(env Env) []string {
	v.ensure(env)
	return v.ids
}

// Has implements View.
func (v *Overview) Has(env Env, id string) bool {
	v.ensure(env)
	return v.all[id]
}

func (v *Overview) move(env Env, r ovRegion, i int) {
	v.focus, v.cur[r] = r, i
	env.Act.SetCurrent(v.items[r][i].id)
}

// Handle implements View.
func (v *Overview) Handle(a keys.Action, env Env) (tea.Cmd, bool) {
	v.ensure(env)
	focus := v.active()
	items := v.items[focus]
	restart := v.exited
	v.exited = false
	switch {
	case a == keys.FocusNext:
		next, ok := v.after(focus)
		if !ok {
			if env.Docked && !restart {
				v.exited = true
				return nil, false
			}
			next = v.first(focus)
		}
		if next == focus {
			return nil, false
		}
		i := v.cursor(next, env.Current)
		v.move(env, next, max(i, 0))
		return nil, true
	case isNav(a):
		if len(items) == 0 {
			return nil, true
		}
		lr := make([]listRow, len(items))
		for i := range lr {
			lr[i].sel = true
		}
		page := max(v.boxes[focus].h-1, 1)
		if to := moveTo(lr, v.cursor(focus, env.Current), a, page); to >= 0 {
			v.move(env, focus, to)
		}
		return nil, true
	case a == keys.Open:
		i := v.cursor(focus, env.Current)
		if i < 0 || items[i].view == 0 {
			return nil, false
		}
		env.Act.Show(items[i].view, items[i].id)
		return nil, true
	}
	return nil, false
}

// Scroll implements View.
func (v *Overview) Scroll(n int) {
	r := v.active()
	v.off[r] = max(v.off[r]+n, 0)
	v.held[r] = true
}

// At implements View; clicks go through AtXY.
func (*Overview) At(int) (string, bool) { return "", false }

// AtXY implements Pointer: a click focuses the region and selects the line.
func (v *Overview) AtXY(x, y int) (string, bool) {
	for r, b := range v.boxes {
		if b.w == 0 || x < b.x || x >= b.x+b.w || y <= b.y || y >= b.y+b.h {
			continue
		}
		line := v.off[r] + y - b.y - 1
		if line < 0 || line >= len(b.at) || b.at[line] < 0 || b.at[line] >= len(v.items[r]) {
			return "", false
		}
		v.focus, v.cur[r] = ovRegion(r), b.at[line]
		return v.items[r][b.at[line]].id, true
	}
	return "", false
}

func (v *Overview) empty(env Env, w, h int) []string {
	e := screens.Empty{Title: "Nothing to show yet.", Body: "The workspace has no issues that pass the scope."}
	if env.Scope.Active() {
		e = screens.EmptyScopeMatch(env.Scope.Query(), env.Matches.HiddenClosed)
	}
	return screens.RenderEmpty(env.Look, e, w, h)
}
