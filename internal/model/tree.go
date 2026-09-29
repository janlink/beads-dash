package model

import (
	"sort"
	"strings"
)

// TreeRowKind tells an issue row from the fold row of closed children.
type TreeRowKind uint8

const (
	// TreeIssue is a row for one issue.
	TreeIssue TreeRowKind = iota
	// TreeClosedFold is the "N closed" row under a parent; ID is the parent.
	TreeClosedFold
)

// TreeRow is one row of the tree, in display order.
type TreeRow struct {
	Kind TreeRowKind
	// ID is the issue of the row; for a fold row, its parent.
	ID    string
	Depth int
	// More has bit i set when the ancestor at depth i has later siblings, so
	// its guide continues through this row. Depths past 63 are not tracked.
	More uint64
	// Last is set on the last sibling shown under its parent.
	Last bool
	// Context marks an issue shown only because a descendant is in scope.
	Context bool
	// Orphan is the missing parent of a root whose parent is not in the
	// snapshot.
	Orphan string
	// Foldable marks a row with children below it; Folded says they are
	// hidden.
	Foldable bool
	Folded   bool
	// Closed is the number of closed children a fold row stands for.
	Closed int
}

// Folds is the per-session fold state, keyed by issue ID. An entry decides
// a row; without one the default applies: only fully closed subtrees and the
// closed-children rows start folded.
type Folds struct{ set map[string]bool }

// ClosedFoldKey is the fold key of the closed-children row under parent.
func ClosedFoldKey(parent string) string { return closedFoldPrefix + parent }

const closedFoldPrefix = "\x00closed:"

// Set folds or unfolds one row.
func (f *Folds) Set(key string, folded bool) {
	if f.set == nil {
		f.set = map[string]bool{}
	}
	f.set[key] = folded
}

// Get returns the explicit state of a row.
func (f *Folds) Get(key string) (folded, ok bool) {
	folded, ok = f.set[key]
	return
}

// SetAll folds or unfolds every parent of snap, closed-children rows
// included.
func (f *Folds) SetAll(snap *Snapshot, folded bool) {
	for _, id := range snap.IDs() {
		if snap.IsContainer(id) {
			f.Set(id, folded)
			f.Set(ClosedFoldKey(id), folded)
		}
	}
}

// Prune forgets entries of issues that no longer exist.
func (f *Folds) Prune(exists func(id string) bool) {
	for k := range f.set {
		id, _ := strings.CutPrefix(k, closedFoldPrefix)
		if !exists(id) {
			delete(f.set, k)
		}
	}
}

// siblingLess orders siblings: not closed before closed, then priority, then
// oldest first.
func siblingLess(a, b *Issue, aClosed, bClosed bool) bool {
	if aClosed != bClosed {
		return bClosed
	}
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ID < b.ID
}

type treeBuilder struct {
	snap     *Snapshot
	m        *Matches
	folds    *Folds
	parent   map[string]string
	kids     map[string][]string
	closed   map[string]bool
	shown    map[string]bool
	subClose map[string]bool
	rows     []TreeRow
}

// BuildTree lays the issues out as a tree. Roots and siblings follow the
// sibling order. An issue is shown when the scope shows it or a descendant;
// closed children the scope hides only by status visibility stand behind one
// closed-children row per parent. A nil m shows everything.
func BuildTree(snap *Snapshot, st Statuses, m *Matches, folds *Folds) []TreeRow {
	if folds == nil {
		folds = &Folds{}
	}
	if m == nil {
		m = Scope{showClosed: true}.Apply(snap, st)
	}
	b := &treeBuilder{
		snap: snap, m: m, folds: folds,
		parent: map[string]string{}, kids: map[string][]string{},
		closed: make(map[string]bool, snap.Len()), shown: make(map[string]bool, snap.Len()),
		subClose: make(map[string]bool, snap.Len()),
	}
	orphans := b.link(st)
	roots := b.kids[""]
	b.sortIDs(roots)
	for _, id := range roots {
		b.markShown(id)
	}
	var visible []string
	for _, id := range roots {
		if b.shown[id] {
			visible = append(visible, id)
		}
	}
	for i, id := range visible {
		b.emit(id, 0, 0, i == len(visible)-1, orphans[id], false)
	}
	return b.rows
}

// link derives each issue's tree parent, breaks parent cycles and groups
// children. It returns the missing parent of every orphan root.
func (b *treeBuilder) link(st Statuses) map[string]string {
	orphans := map[string]string{}
	for _, id := range b.snap.IDs() {
		is, _ := b.snap.Issue(id)
		b.closed[id] = b.snap.Present(id, st).Status == Closed
		want := is.Parent
		if want == "" {
			for _, e := range is.Dependencies {
				if e.Type == EdgeParentChild && e.To != "" {
					want = e.To
					break
				}
			}
		}
		if want == "" || want == id {
			continue
		}
		if _, ok := b.snap.Issue(want); ok {
			b.parent[id] = want
		} else {
			orphans[id] = want
		}
	}
	state := map[string]uint8{}
	for _, id := range b.snap.IDs() {
		var path []string
		cur := id
		for cur != "" && state[cur] == 0 {
			state[cur] = 1
			path = append(path, cur)
			cur = b.parent[cur]
			if cur != "" && state[cur] == 1 {
				delete(b.parent, cur)
				break
			}
		}
		for _, p := range path {
			state[p] = 2
		}
	}
	for _, id := range b.snap.IDs() {
		p := b.parent[id]
		b.kids[p] = append(b.kids[p], id)
	}
	for p, ids := range b.kids {
		if p != "" {
			b.sortIDs(ids)
		}
	}
	return orphans
}

func (b *treeBuilder) sortIDs(ids []string) {
	sort.SliceStable(ids, func(i, j int) bool {
		x, _ := b.snap.Issue(ids[i])
		y, _ := b.snap.Issue(ids[j])
		return siblingLess(x, y, b.closed[ids[i]], b.closed[ids[j]])
	})
}

// markShown decides bottom-up which issues are on screen and, for the
// default folds, which subtrees are fully closed.
func (b *treeBuilder) markShown(id string) {
	show := b.m.Has(id)
	allClosed := b.closed[id]
	for _, c := range b.kids[id] {
		b.markShown(c)
		show = show || b.shown[c]
		allClosed = allClosed && b.subClose[c]
	}
	b.shown[id] = show
	b.subClose[id] = allClosed
}

func (b *treeBuilder) folded(key string, def bool) bool {
	if v, ok := b.folds.Get(key); ok {
		return v
	}
	return def
}

// emit appends the row of id and its children. revealed marks the closed
// children an opened closed-children row lists: they are not context.
func (b *treeBuilder) emit(id string, depth int, more uint64, last bool, orphan string, revealed bool) {
	var shown, hidden []string
	for _, c := range b.kids[id] {
		switch {
		case b.shown[c]:
			shown = append(shown, c)
		case b.closed[c] && b.m.Facets(c):
			hidden = append(hidden, c)
		}
	}
	container := len(shown)+len(hidden) > 0
	folded := container && b.folded(id, b.subClose[id] && len(b.kids[id]) > 0)
	b.rows = append(b.rows, TreeRow{
		Kind: TreeIssue, ID: id, Depth: depth, More: more, Last: last,
		Context: !revealed && !b.m.Has(id), Orphan: orphan, Foldable: container, Folded: folded,
	})
	if folded {
		return
	}
	childMore := more
	if !last && depth < 64 {
		childMore |= 1 << uint(depth)
	}
	for i, c := range shown {
		b.emit(c, depth+1, childMore, i == len(shown)-1 && len(hidden) == 0, "", false)
	}
	if len(hidden) == 0 {
		return
	}
	foldedClosed := b.folded(ClosedFoldKey(id), true)
	b.rows = append(b.rows, TreeRow{
		Kind: TreeClosedFold, ID: id, Depth: depth + 1, More: childMore, Last: true,
		Foldable: true, Folded: foldedClosed, Closed: len(hidden),
	})
	if foldedClosed {
		return
	}
	for i, c := range hidden {
		b.emit(c, depth+2, childMore, i == len(hidden)-1, "", true)
	}
}

// TreeParents maps every issue that has a parent in snap to it, using the
// tree's own rule: the parent field, else a parent-child edge, with parent
// cycles cut.
func TreeParents(snap *Snapshot, st Statuses) map[string]string {
	b := &treeBuilder{
		snap: snap, parent: map[string]string{}, kids: map[string][]string{},
		closed: make(map[string]bool, snap.Len()),
	}
	b.link(st)
	return b.parent
}

// SortedChildren lists the direct children of id in the tree's sibling
// order, closed ones apart from the rest.
func SortedChildren(snap *Snapshot, st Statuses, id string) (live, closed []string) {
	kids := snap.Children(id)
	live = make([]string, 0, len(kids))
	for _, c := range kids {
		if snap.Present(c, st).Status == Closed {
			closed = append(closed, c)
		} else {
			live = append(live, c)
		}
	}
	order := func(ids []string) {
		sort.SliceStable(ids, func(i, j int) bool {
			a, _ := snap.Issue(ids[i])
			b, _ := snap.Issue(ids[j])
			return siblingLess(a, b, false, false)
		})
	}
	order(live)
	order(closed)
	return live, closed
}
