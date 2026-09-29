package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

const (
	graphSlot     = 5
	graphStep     = 4
	graphMaxDepth = model.FocusMaxDepth
)

// Graph is the dependency graph view: one outline per connected group, or,
// after focusing an issue, what that issue waits on and holds up. Issues the
// scope does not match are drawn dimmed, never removed.
type Graph struct {
	folds map[string]bool
	iso   bool
	focus []string
	depth int
	ver   int

	built struct {
		snap  *model.Snapshot
		iso   bool
		focus string
		depth int
		ver   int
		ok    bool
	}
	rows     []model.OutlineRow
	isolated int
	groups   int
	vis      []int
	sel      []listRow
	keys     []string
	byID     map[string]int
	maxDepth int

	win     window
	lastCur string
	lastPos int
	// pos is the row the cursor sits on; it counts only while its issue is
	// the current one.
	pos  int
	xoff int
	h, w int

	info scopeInfo
}

// NewGraph returns the dependency graph view.
func NewGraph() *Graph {
	return &Graph{folds: map[string]bool{}, depth: model.FocusDepthDefault, pos: -1, lastPos: -1}
}

// Name implements View.
func (*Graph) Name() string { return "graph" }

// Context implements View.
func (*Graph) Context() keys.Context { return keys.Graph }

// Scope implements View.
func (v *Graph) Scope(w int) string {
	if w < 8 {
		return ""
	}
	var parts []string
	if v.info.active {
		parts = append(parts, v.info.marker+" "+v.info.query+" · others dimmed")
	}
	if len(v.focus) > 0 {
		parts = append(parts, "focus "+v.focus[len(v.focus)-1])
	} else if !v.iso && v.isolated > 0 {
		parts = append(parts, fmt.Sprintf("%d isolated hidden", v.isolated))
	}
	if v.xoff > 0 {
		parts = append(parts, fmt.Sprintf("col +%d", v.xoff))
	}
	return rows.MidCut(strings.Join(parts, " · "), w, v.info.ellipsis)
}

func (v *Graph) focusID() string {
	if len(v.focus) == 0 {
		return ""
	}
	return v.focus[len(v.focus)-1]
}

func (v *Graph) ensure(env Env) {
	if env.Snap == nil {
		v.built.ok = false
		v.rows, v.vis, v.sel, v.keys, v.byID = nil, nil, nil, nil, nil
		return
	}
	focus := v.focusID()
	b := &v.built
	if b.ok && b.snap == env.Snap && b.iso == v.iso && b.focus == focus && b.depth == v.depth && b.ver == v.ver {
		return
	}
	if b.snap != env.Snap {
		for id := range v.folds {
			if _, ok := env.Snap.Issue(id); !ok {
				delete(v.folds, id)
			}
		}
	}
	structural := !b.ok || b.snap != env.Snap || b.iso != v.iso || b.focus != focus || b.depth != v.depth
	b.snap, b.iso, b.focus, b.depth, b.ver, b.ok = env.Snap, v.iso, focus, v.depth, v.ver, true
	if structural {
		if focus == "" {
			o := model.BuildOutline(env.Snap, v.iso)
			v.rows, v.isolated, v.groups = o.Rows, len(o.Isolated), o.Groups
		} else {
			v.rows = model.BuildFocus(env.Snap, env.Statuses, focus, model.FocusOptions{Depth: v.depth})
			v.isolated, v.groups = 0, 0
		}
		v.byID = make(map[string]int, len(v.rows))
		v.maxDepth = 0
		for i, r := range v.rows {
			v.maxDepth = max(v.maxDepth, r.Depth)
			if r.Kind.Selectable() {
				if _, ok := v.byID[r.ID]; !ok {
					v.byID[r.ID] = i
				}
			}
		}
		if focus != "" {
			for i, r := range v.rows {
				if r.Kind == model.OutSelf {
					v.byID[focus] = i
				}
			}
		}
		v.pos = -1
	}
	v.layout()
}

func (v *Graph) folded(i int) bool {
	r := v.rows[i]
	return r.Kind == model.OutNode && r.End > i+1 && v.folds[r.ID]
}

// layout lists the rows a fold does not hide.
func (v *Graph) layout() {
	v.vis, v.sel, v.keys = v.vis[:0], v.sel[:0], v.keys[:0]
	for i := 0; i < len(v.rows); i++ {
		r := v.rows[i]
		v.vis = append(v.vis, i)
		v.keys = append(v.keys, strconv.Itoa(i))
		v.sel = append(v.sel, listRow{v.keys[len(v.keys)-1], r.Kind.Selectable()})
		if v.folded(i) {
			i = r.End - 1
		}
	}
}

func (v *Graph) visAt(row int) (int, bool) {
	k := sort.SearchInts(v.vis, row)
	return k, k < len(v.vis) && v.vis[k] == row
}

// cursor is the index into vis of the row the cursor is on: the row last
// moved to, else the current issue's first row, else the nearest visible
// ancestor of that row.
func (v *Graph) cursor(env Env) int {
	row := -1
	if v.pos >= 0 && v.pos < len(v.rows) && v.rows[v.pos].ID == env.Current {
		row = v.pos
	} else if r, ok := v.byID[env.Current]; ok {
		row = r
	}
	for row >= 0 {
		if k, ok := v.visAt(row); ok {
			return k
		}
		row = v.rows[row].Parent
	}
	return -1
}

// Sync implements Syncer: it opens the folds above the current issue when
// something outside the view moved it into one, and drops a focus whose issue
// left the snapshot.
func (v *Graph) Sync(env Env) {
	v.info = scopeInfoFor(env, 0, false)
	if env.Snap != nil && len(v.focus) > 0 {
		for len(v.focus) > 0 {
			if _, ok := env.Snap.Issue(v.focusID()); ok {
				break
			}
			v.focus = v.focus[:len(v.focus)-1]
		}
	}
	v.ensure(env)
	if env.Current == v.lastCur {
		return
	}
	v.lastCur = env.Current
	row, ok := v.byID[env.Current]
	if !ok {
		return
	}
	if _, shown := v.visAt(row); shown {
		return
	}
	for p := v.rows[row].Parent; p >= 0; p = v.rows[p].Parent {
		delete(v.folds, v.rows[p].ID)
	}
	v.ver++
	v.ensure(env)
}

// Visible implements View.
func (v *Graph) Visible(env Env) []string {
	v.ensure(env)
	var out []string
	seen := map[string]bool{}
	for _, i := range v.vis {
		r := v.rows[i]
		if r.Kind.Selectable() && !seen[r.ID] {
			seen[r.ID] = true
			out = append(out, r.ID)
		}
	}
	return out
}

// Has implements View.
func (v *Graph) Has(env Env, id string) bool {
	v.ensure(env)
	_, ok := v.byID[id]
	return ok
}

func (v *Graph) move(env Env, k int) {
	row := v.vis[k]
	v.pos, v.lastCur = row, v.rows[row].ID
	env.Act.SetCurrent(v.rows[row].ID)
}

func (v *Graph) setFocus(env Env, id string) {
	if id == "" || id == v.focusID() {
		return
	}
	prev := v.focusID()
	v.focus = append(v.focus, id)
	env.Act.JumpFocus(graphSlot, prev, id)
	v.refocus(env)
}

// Close implements Closer: Esc steps back out of a focus. It comes after the
// layers and the marks in the Esc cascade and ahead of the search scope.
func (v *Graph) Close(env Env) bool {
	if len(v.focus) == 0 {
		return false
	}
	v.focus = v.focus[:len(v.focus)-1]
	v.refocus(env)
	return true
}

// Restore implements Restorer: Back returns to the focus the jump left.
func (v *Graph) Restore(env Env, focus string) {
	v.focus = v.focus[:0]
	if focus != "" {
		v.focus = append(v.focus, focus)
	}
	v.refocus(env)
}

// refocus starts afresh after the focus changed: folds belong to one mode.
func (v *Graph) refocus(env Env) {
	clear(v.folds)
	v.xoff = 0
	v.ensure(env)
}

func (v *Graph) foldAll(env Env, folded bool) {
	clear(v.folds)
	if folded {
		for i, r := range v.rows {
			if r.Kind == model.OutNode && r.End > i+1 {
				v.folds[r.ID] = true
			}
		}
	}
	v.ver++
	v.ensure(env)
	if k := v.cursor(env); k >= 0 {
		v.move(env, k)
	}
}

func (v *Graph) setFold(env Env, id string, folded bool) {
	if folded {
		v.folds[id] = true
	} else {
		delete(v.folds, id)
	}
	v.ver++
	v.ensure(env)
}

// Handle implements View.
func (v *Graph) Handle(a keys.Action, env Env) (tea.Cmd, bool) {
	v.ensure(env)
	switch a { //nolint:exhaustive // the view acts on its own subset
	case keys.ToggleIsolated:
		if len(v.focus) == 0 {
			v.iso = !v.iso
			v.ensure(env)
		}
		return nil, true
	case keys.DepthMore, keys.DepthLess:
		d := v.depth + 1
		if a == keys.DepthLess {
			d = v.depth - 1
		}
		v.depth = min(max(d, 1), graphMaxDepth)
		v.ensure(env)
		return nil, true
	case keys.ScrollLeft, keys.ScrollRight, keys.ScrollHalfLeft, keys.ScrollHalfRight:
		step := graphStep
		if a == keys.ScrollHalfLeft || a == keys.ScrollHalfRight {
			step = max(v.w/2, 1)
		}
		if a == keys.ScrollLeft || a == keys.ScrollHalfLeft {
			step = -step
		}
		v.xoff = min(max(v.xoff+step, 0), v.maxDepth*v.guide())
		return nil, true
	}
	if len(v.rows) == 0 {
		return nil, false
	}
	k := v.cursor(env)
	switch {
	case isNav(a):
		if to := moveTo(v.sel, k, a, max(v.h, 1)); to >= 0 {
			v.move(env, to)
		}
		return nil, true
	case a == keys.FoldAll || a == keys.UnfoldAll:
		v.foldAll(env, a == keys.FoldAll)
		return nil, true
	}
	if k < 0 {
		switch a { //nolint:exhaustive // only these need a row
		case keys.Open:
			if _, ok := env.Snap.Issue(env.Current); ok && len(v.focus) == 0 {
				v.setFocus(env, env.Current)
				return nil, true
			}
		case keys.Left, keys.Right:
			return nil, true
		}
		return nil, false
	}
	ri := v.vis[k]
	r := v.rows[ri]
	switch a { //nolint:exhaustive // the view acts on its own subset
	case keys.Left:
		if r.Kind == model.OutNode && r.End > ri+1 && !v.folds[r.ID] {
			v.setFold(env, r.ID, true)
			return nil, true
		}
		if r.Parent >= 0 && v.rows[r.Parent].Kind.Selectable() {
			if pk, ok := v.visAt(r.Parent); ok {
				v.move(env, pk)
			}
		}
		return nil, true
	case keys.Right:
		switch {
		case v.folded(ri):
			v.setFold(env, r.ID, false)
		case r.Kind == model.OutNode && r.End > ri+1 && k+1 < len(v.vis):
			v.move(env, k+1)
		}
		return nil, true
	case keys.Open:
		if !r.Kind.Selectable() || r.ID == v.focusID() {
			return nil, false
		}
		v.setFocus(env, r.ID)
		return nil, true
	}
	return nil, false
}

func (v *Graph) guide() int {
	if rows.Narrow(v.w) {
		return 2
	}
	return 3
}

func (v *Graph) dimmed(env Env, id string) bool {
	return env.Scope.Active() && env.Matches != nil && !env.Matches.Facets(id)
}

// follow scrolls sideways when the cursor row's label would be out of view.
func (v *Graph) follow(cur, w int) {
	if cur == v.lastPos {
		return
	}
	v.lastPos = cur
	if cur < 0 {
		return
	}
	start := v.rows[v.vis[cur]].Depth * v.guide()
	switch {
	case start < v.xoff:
		v.xoff = max(start-v.guide(), 0)
	case start+min(minGraphLabel, w) > v.xoff+w:
		v.xoff = start + min(minGraphLabel, w) - w
	}
}

const minGraphLabel = 24

// Render implements View.
func (v *Graph) Render(env Env, w, h int) []string {
	v.h, v.w = h, w-rows.GutterWidth
	v.ensure(env)
	if len(v.rows) == 0 && v.isolated > 0 {
		e := screens.EmptyGraph
		e.Body = fmt.Sprintf("Only isolated issues here. i shows %d hidden.", v.isolated)
		return screens.RenderEmpty(env.Look, e, w, h)
	}
	if len(v.rows) == 0 || len(v.vis) == 0 {
		return screens.RenderEmpty(env.Look, screens.EmptyGraph, w, h)
	}
	cur := v.cursor(env)
	v.follow(cur, v.w)
	curKey := ""
	if cur >= 0 {
		curKey = v.keys[cur]
	}
	first := v.win.layout(v.keys, curKey, h)
	style := rows.OutlineStyle{Narrow: v.guide() == 2, XOff: v.xoff}
	out := make([]string, h)
	for i := range out {
		k := first + i
		if k >= len(v.vis) {
			out[i] = env.Look.Fit("", w)
			continue
		}
		ri := v.vis[k]
		r := v.rows[ri]
		g := rows.Row{Current: k == cur}
		st := style
		st.Sel = k == cur
		if r.Kind.Selectable() {
			g.ID, g.Marked, g.Changed = r.ID, env.Marked(r.ID), env.Changed(r.ID)
			st.Dim = v.dimmed(env, r.ID)
		}
		if v.folded(ri) {
			st.Hidden = r.End - ri - 1
		}
		out[i] = env.Rows.Gutter(g) + env.Rows.Outline(v.rows, ri, v.w, st)
	}
	return out
}

// Scroll implements View.
func (v *Graph) Scroll(n int) { v.win.scroll(n) }

// At implements View. Clicking a row puts the cursor on it.
func (v *Graph) At(y int) (string, bool) {
	key, ok := v.win.at(y)
	if !ok {
		return "", false
	}
	ri, err := strconv.Atoi(key)
	if err != nil || ri >= len(v.rows) || !v.rows[ri].Kind.Selectable() {
		return "", false
	}
	v.pos = ri
	return v.rows[ri].ID, true
}
