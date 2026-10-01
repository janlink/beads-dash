package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

// Tree is the parent/child tree view. Every row, the closed-children rows
// included, is a place for the cursor; the current issue of a closed-children
// row is its parent.
type Tree struct {
	folds model.Folds
	ver   int

	built struct {
		snap  *model.Snapshot
		scope string
		ver   int
		ok    bool
	}
	rows    []model.TreeRow
	keys    []string
	sel     []listRow
	ids     []string
	index   map[string]int
	parents map[string]string
	present map[string]bool
	gen     int

	win     window
	lastCur string
	// pos is the row the cursor sits on when it is a closed-children row;
	// it counts only while its owner is the current issue.
	pos, posOwner string
	h             int
	hdr           int
	label         scopeInfo
}

// NewTree returns the tree view.
func NewTree() *Tree { return &Tree{} }

// Name implements View.
func (*Tree) Name() string { return "tree" }

// Scope implements View.
func (v *Tree) Scope(w int) string { return v.label.fit(w) }

// Context implements View.
func (*Tree) Context() keys.Context { return keys.Tree }

func (v *Tree) rowView() string { return "tree#" + itoa(v.gen) }

func (v *Tree) ensure(env Env) {
	if env.Snap == nil || env.Matches == nil {
		v.built.ok = false
		v.rows, v.keys, v.sel, v.ids, v.index = nil, nil, nil, nil, nil
		return
	}
	scope := env.Scope.Key()
	if v.built.ok && v.built.snap == env.Snap && v.built.scope == scope && v.built.ver == v.ver {
		return
	}
	if v.built.snap != env.Snap {
		v.folds.Prune(func(id string) bool { _, ok := env.Snap.Issue(id); return ok })
	}
	v.built.snap, v.built.scope, v.built.ver, v.built.ok = env.Snap, scope, v.ver, true
	v.gen++
	v.rows = model.BuildTree(env.Snap, env.Statuses, env.Matches, &v.folds)
	v.parents = model.TreeParents(env.Snap, env.Statuses)
	v.keys = v.keys[:0]
	v.sel = v.sel[:0]
	v.ids = v.ids[:0]
	v.index = make(map[string]int, len(v.rows))
	for i, r := range v.rows {
		k := r.ID
		if r.Kind == model.TreeClosedFold {
			k = model.ClosedFoldKey(r.ID)
		} else {
			v.ids = append(v.ids, r.ID)
		}
		v.keys = append(v.keys, k)
		v.sel = append(v.sel, listRow{k, true})
		v.index[k] = i
	}
	env.Rows.SetTreeRows(v.rows)
	v.present = make(map[string]bool, env.Matches.Len())
	for _, id := range env.Matches.IDs() {
		for c := id; c != "" && !v.present[c]; c = v.parents[c] {
			v.present[c] = true
		}
	}
}

// Sync implements Syncer: it reveals the current issue when something outside
// the view moved it under a folded row.
func (v *Tree) Sync(env Env) {
	v.ensure(env)
	v.label = scopeInfo{}
	if env.Matches != nil {
		v.label = scopeInfoFor(env, env.Matches.Len(), true)
	}
	if env.Current == v.lastCur {
		return
	}
	v.lastCur = env.Current
	cur := env.Current
	if cur == "" || env.Snap == nil {
		return
	}
	if _, ok := v.index[cur]; ok || (!v.present[cur] && !env.Matches.Facets(cur)) {
		return
	}
	for c := cur; ; {
		p, ok := v.parents[c]
		if !ok {
			break
		}
		v.folds.Set(p, false)
		if env.Snap.Present(c, env.Statuses).Status == model.Closed {
			v.folds.Set(model.ClosedFoldKey(p), false)
		}
		c = p
	}
	v.ver++
	v.ensure(env)
}

// cursor is the key of the row the cursor is on: the current issue's row, or
// the nearest ancestor that is shown.
func (v *Tree) cursor(env Env) (string, bool) {
	if v.pos != "" && v.posOwner == env.Current {
		if _, ok := v.index[v.pos]; ok {
			return v.pos, true
		}
	}
	for c := env.Current; c != ""; c = v.parents[c] {
		if _, ok := v.index[c]; ok {
			return c, true
		}
	}
	return "", false
}

func (v *Tree) cursorIndex(env Env) int {
	if k, ok := v.cursor(env); ok {
		return v.index[k]
	}
	return -1
}

// Visible implements View.
func (v *Tree) Visible(env Env) []string {
	v.ensure(env)
	return v.ids
}

// Has implements View: an issue belongs to the tree when the scope shows it or
// a descendant, folded away or not.
func (v *Tree) Has(env Env, id string) bool {
	v.ensure(env)
	if _, ok := v.index[id]; ok {
		return true
	}
	return v.present[id]
}

func (v *Tree) move(env Env, i int) {
	r := v.rows[i]
	v.pos, v.posOwner, v.lastCur = v.keys[i], r.ID, r.ID
	env.Act.SetCurrent(r.ID)
}

func (v *Tree) setFold(env Env, key string, folded bool) {
	v.folds.Set(key, folded)
	v.ver++
	v.ensure(env)
}

func (v *Tree) foldAll(env Env, folded bool) {
	if env.Snap == nil {
		return
	}
	v.folds.SetAll(env.Snap, folded)
	v.ver++
	v.ensure(env)
	if k, ok := v.cursor(env); ok {
		v.move(env, v.index[k])
	}
}

// Handle implements View.
func (v *Tree) Handle(a keys.Action, env Env) (tea.Cmd, bool) {
	v.ensure(env)
	if len(v.rows) == 0 {
		return nil, false
	}
	i := v.cursorIndex(env)
	switch {
	case isNav(a):
		if to := moveTo(v.sel, i, a, max(v.h, 1)); to >= 0 {
			v.move(env, to)
		}
		return nil, true
	case a == keys.FoldAll || a == keys.UnfoldAll:
		v.foldAll(env, a == keys.FoldAll)
		return nil, true
	}
	if i < 0 {
		return nil, a == keys.Left || a == keys.Right
	}
	r := v.rows[i]
	key := v.keys[i]
	switch a { //nolint:exhaustive // the view acts on its own subset
	case keys.Left:
		if r.Foldable && !r.Folded {
			v.setFold(env, key, true)
			if r.Kind == model.TreeClosedFold {
				v.pos = key
			}
			return nil, true
		}
		target := v.parents[r.ID]
		if r.Kind == model.TreeClosedFold {
			target = r.ID
		}
		if j, ok := v.index[target]; ok && target != "" {
			v.move(env, j)
		}
		return nil, true
	case keys.Right:
		switch {
		case r.Foldable && r.Folded:
			v.setFold(env, key, false)
		case r.Foldable && i+1 < len(v.rows):
			v.move(env, i+1)
		}
		return nil, true
	case keys.Open:
		if r.Kind == model.TreeClosedFold {
			v.setFold(env, key, !r.Folded)
			return nil, true
		}
	}
	return nil, false
}

// Render implements View.
func (v *Tree) Render(env Env, w, h int) []string {
	v.hdr = headerRows(h)
	v.h = h - v.hdr
	v.ensure(env)
	if len(v.rows) == 0 {
		hidden := 0
		if env.Matches != nil {
			hidden = env.Matches.HiddenClosed
		}
		return screens.RenderEmpty(env.Look, screens.EmptyScopeMatch(env.Scope.Query(), hidden), w, h)
	}
	cur, _ := v.cursor(env)
	first := v.win.layout(v.keys, cur, v.h)
	name := v.rowView()
	out := make([]string, h)
	if v.hdr > 0 {
		out[0] = env.Rows.TreeHeader(w)
	}
	for i := v.hdr; i < h; i++ {
		idx := first + i - v.hdr
		if idx >= len(v.rows) {
			out[i] = env.Look.Fit("", w)
			continue
		}
		r := v.rows[idx]
		row := rows.Row{ID: r.ID, Current: v.keys[idx] == cur, Changed: v.changed(env, r)}
		view := name
		if r.Kind == model.TreeClosedFold {
			view += "#f"
			row.Closed = true
		} else {
			row.Marked = env.Marked(r.ID)
		}
		out[i] = env.Rows.Line(w, row, view, env.Rows.Tree(r))
	}
	return out
}

// changed marks a row whose issue changed; a folded parent and a
// closed-children row carry the marks of what they hide.
func (v *Tree) changed(env Env, r model.TreeRow) bool {
	switch {
	case r.Kind == model.TreeClosedFold:
		for _, c := range env.Snap.Children(r.ID) {
			if env.Snap.Present(c, env.Statuses).Status == model.Closed && env.Changed(c) {
				return true
			}
		}
		return false
	case env.Changed(r.ID):
		return true
	case r.Folded:
		seen := map[string]bool{r.ID: true}
		stack := append([]string(nil), env.Snap.Children(r.ID)...)
		for len(stack) > 0 {
			id := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[id] {
				continue
			}
			seen[id] = true
			if env.Changed(id) {
				return true
			}
			stack = append(stack, env.Snap.Children(id)...)
		}
	}
	return false
}

// Scroll implements View.
func (v *Tree) Scroll(n int) { v.win.scroll(n) }

// At implements View. Clicking a closed-children row puts the cursor on it.
func (v *Tree) At(y int) (string, bool) {
	k, ok := v.win.at(y - v.hdr)
	if !ok {
		return "", false
	}
	i, ok := v.index[k]
	if !ok {
		return "", false
	}
	v.pos, v.posOwner = k, v.rows[i].ID
	return v.rows[i].ID, true
}
