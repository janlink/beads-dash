package model

import (
	"sort"
)

// OutlineKind says what a row of an outline is.
type OutlineKind uint8

// The row kinds. Only the kinds that name an issue can hold the cursor.
const (
	// OutHeader opens one connected group of the dependency graph; Count is
	// its number of issues.
	OutHeader OutlineKind = iota
	// OutIsolated opens the section of issues without any edge; Count is
	// their number.
	OutIsolated
	// OutNode is an issue drawn in full at its first sighting.
	OutNode
	// OutRef is a later sighting of an issue that is drawn elsewhere.
	OutRef
	// OutCycle is an edge back to an issue on the path above it.
	OutCycle
	// OutMore stands for Count branches that were cut off.
	OutMore
	// OutParent is the parent line of a focus graph.
	OutParent
	// OutWaits heads the outline of what the focused issue waits on: Count
	// direct blockers, Aux of them not closed.
	OutWaits
	// OutHolds heads the outline of what the focused issue holds up.
	OutHolds
	// OutSelf is the focused issue between the two outlines.
	OutSelf
	// OutKids heads the list of the focused issue's children.
	OutKids
	// OutKid is one child of the focused issue.
	OutKid
)

// Selectable reports whether the cursor can rest on rows of the kind.
func (k OutlineKind) Selectable() bool {
	switch k {
	case OutNode, OutRef, OutCycle, OutParent, OutSelf, OutKid:
		return true
	case OutHeader, OutIsolated, OutMore, OutWaits, OutHolds, OutKids:
	}
	return false
}

// OutlineRow is one row of a dependency outline. Rows are in display order;
// the connectors of a row follow from Depth, Last and the chain of Parent
// rows, so no row carries text.
type OutlineRow struct {
	Kind OutlineKind
	ID   string
	// Parent is the row of the issue this one hangs under, -1 at the top.
	Parent int
	// Depth is the number of connector levels left of the row; 0 has none.
	Depth int
	// Last marks the last row among its siblings.
	Last bool
	// Dim marks a parent-child edge, which is drawn fainter than a blocks edge.
	Dim bool
	// Member marks an issue that lies on a dependency cycle.
	Member bool
	// End is the row after the last row below this node; End == index+1 for
	// a node without children.
	End        int
	Count, Aux int
}

// Outline is the dependency graph as one outline per connected group.
type Outline struct {
	Rows []OutlineRow
	// Isolated are the issues without any edge, sorted by ID; they are in
	// Rows only when the outline was asked to show them.
	Isolated []string
	// Groups is the number of connected groups with at least two issues.
	Groups int
}

type link struct {
	to   int
	dim  bool
	back bool
}

type depGraph struct {
	ids    []string
	out    [][]link
	in     [][]link
	backIn []int
	cyc    []bool
}

func newDepGraph(snap *Snapshot) *depGraph {
	ids := snap.IDs()
	n := len(ids)
	idx := make(map[string]int, n)
	for i, id := range ids {
		idx[id] = i
	}
	g := &depGraph{ids: ids, out: make([][]link, n), in: make([][]link, n), backIn: make([]int, n), cyc: make([]bool, n)}
	add := func(a, b int, dim bool) {
		g.out[a] = append(g.out[a], link{to: b, dim: dim})
		g.in[b] = append(g.in[b], link{to: a, dim: dim})
	}
	for i, id := range ids {
		last := -1
		for _, e := range snap.Dependents(id) {
			j, ok := idx[e.From]
			if !ok || j == i || !BlockingEdge(e.Type) || j == last {
				continue
			}
			last = j
			add(i, j, false)
		}
	}
	blocked := make([]bool, n)
	for i := range ids {
		blocked[i] = len(g.in[i]) > 0
	}
	for i, id := range ids {
		for _, c := range snap.Children(id) {
			if j := idx[c]; !blocked[j] {
				add(i, j, true)
			}
		}
	}
	for i := range ids {
		sort.SliceStable(g.out[i], func(a, b int) bool { return g.out[i][a].to < g.out[i][b].to })
		sort.SliceStable(g.in[i], func(a, b int) bool { return g.in[i][a].to < g.in[i][b].to })
	}
	g.markBackEdges()
	g.markCycles()
	return g
}

// markBackEdges flags the edges that close a cycle in depth-first order, so
// that the rest of the graph is acyclic and every issue is reachable from an
// issue without blockers.
func (g *depGraph) markBackEdges() {
	state := make([]uint8, len(g.ids))
	var visit func(u int)
	visit = func(u int) {
		state[u] = 1
		for k := range g.out[u] {
			l := &g.out[u][k]
			switch state[l.to] {
			case 0:
				visit(l.to)
			case 1:
				l.back = true
				g.backIn[l.to]++
			}
		}
		state[u] = 2
	}
	for i := range g.ids {
		if state[i] == 0 {
			visit(i)
		}
	}
}

// markCycles flags every issue in a strongly connected group of blocks edges.
func (g *depGraph) markCycles() {
	n := len(g.ids)
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = -1
	}
	var stack []int
	next := 0
	var strong func(v int)
	strong = func(v int) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for _, l := range g.out[v] {
			if l.dim {
				continue
			}
			switch {
			case index[l.to] < 0:
				strong(l.to)
				low[v] = min(low[v], low[l.to])
			case onStack[l.to]:
				low[v] = min(low[v], index[l.to])
			}
		}
		if low[v] != index[v] {
			return
		}
		start := len(stack) - 1
		for stack[start] != v {
			start--
		}
		members := stack[start:]
		stack = stack[:start]
		for _, w := range members {
			onStack[w] = false
			g.cyc[w] = len(members) > 1
		}
	}
	for i := range g.ids {
		if index[i] < 0 {
			strong(i)
		}
	}
}

// groups splits the issues that have edges into weakly connected groups,
// biggest first, and lists the issues without edges. Members are in ID order.
func (g *depGraph) groups() (groups [][]int, isolated []int) {
	n := len(g.ids)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for a := range g.out {
		for _, l := range g.out[a] {
			if ra, rb := find(a), find(l.to); ra != rb {
				parent[max(ra, rb)] = min(ra, rb)
			}
		}
	}
	at := map[int]int{}
	for i := range n {
		if len(g.out[i]) == 0 && len(g.in[i]) == 0 {
			isolated = append(isolated, i)
			continue
		}
		r := find(i)
		gi, ok := at[r]
		if !ok {
			gi = len(groups)
			at[r] = gi
			groups = append(groups, nil)
		}
		groups[gi] = append(groups[gi], i)
	}
	sort.SliceStable(groups, func(a, b int) bool { return len(groups[a]) > len(groups[b]) })
	return groups, isolated
}

type emitter struct {
	g      *depGraph
	rows   []OutlineRow
	seen   []bool
	onPath []bool
}

func (e *emitter) node(i, parent, depth int, last, dim bool) {
	row := len(e.rows)
	e.rows = append(e.rows, OutlineRow{Kind: OutNode, ID: e.g.ids[i], Parent: parent, Depth: depth, Last: last, Dim: dim, Member: e.g.cyc[i]})
	e.seen[i], e.onPath[i] = true, true
	kids := e.g.out[i]
	for k, l := range kids {
		isLast := k == len(kids)-1
		switch {
		case e.onPath[l.to]:
			e.rows = append(e.rows, OutlineRow{Kind: OutCycle, ID: e.g.ids[l.to], Parent: row, Depth: depth + 1, Last: isLast, Dim: l.dim, Member: !l.dim && e.g.cyc[l.to], End: len(e.rows) + 1})
		case e.seen[l.to]:
			e.rows = append(e.rows, OutlineRow{Kind: OutRef, ID: e.g.ids[l.to], Parent: row, Depth: depth + 1, Last: isLast, Dim: l.dim, End: len(e.rows) + 1})
		default:
			e.node(l.to, row, depth+1, isLast, l.dim)
		}
	}
	e.onPath[i] = false
	e.rows[row].End = len(e.rows)
}

// BuildOutline draws the blocks edges of the whole snapshot as one outline per
// connected group, biggest first. Roots are issues nobody blocks; an issue is
// drawn in full at its first sighting in depth-first order and later edges to
// it become references, edges back into the path above become cycle rows.
// Parent-child edges are added only to entry children, the children no
// blocks edge holds back; the others are reached through their blockers.
// With showIsolated the issues without any edge follow as a last section.
func BuildOutline(snap *Snapshot, showIsolated bool) *Outline {
	g := newDepGraph(snap)
	groups, isolated := g.groups()
	e := &emitter{g: g, seen: make([]bool, len(g.ids)), onPath: make([]bool, len(g.ids))}
	out := &Outline{Groups: len(groups)}
	for _, grp := range groups {
		e.rows = append(e.rows, OutlineRow{Kind: OutHeader, Parent: -1, Count: len(grp)})
		for _, i := range grp {
			if len(g.in[i])-g.backIn[i] == 0 && !e.seen[i] {
				e.node(i, -1, 0, false, false)
			}
		}
	}
	for _, i := range isolated {
		out.Isolated = append(out.Isolated, g.ids[i])
	}
	if showIsolated && len(isolated) > 0 {
		e.rows = append(e.rows, OutlineRow{Kind: OutIsolated, Parent: -1, Count: len(isolated)})
		for _, i := range isolated {
			e.rows = append(e.rows, OutlineRow{Kind: OutNode, ID: g.ids[i], Parent: -1, End: len(e.rows) + 1})
		}
	}
	out.Rows = e.rows
	return out
}

const (
	// FocusFan is the most branches drawn under one issue of a focus graph.
	FocusFan = 20
	// FocusDepthDefault is the depth of the focus graph in the graph view.
	FocusDepthDefault = 3
	// FocusMaxDepth is the deepest level a focus graph is drawn to.
	FocusMaxDepth = 8
)

type focus struct {
	snap   *Snapshot
	rows   []OutlineRow
	seen   map[string]bool
	onPath map[string]bool
}

func (f *focus) node(id string, parent, depth int, last bool, kind OutlineKind, member bool) int {
	f.rows = append(f.rows, OutlineRow{Kind: kind, ID: id, Parent: parent, Depth: depth, Last: last, Member: member})
	return len(f.rows) - 1
}

func (f *focus) walk(id string, parent, depth, budget int, next func(string) []string) {
	kids := next(id)
	if len(kids) == 0 {
		return
	}
	if budget == 0 {
		f.cut(kids, parent, depth)
		return
	}
	shown, rest := kids, 0
	if len(kids) > FocusFan {
		shown, rest = kids[:FocusFan], len(kids)-FocusFan
	}
	for i, k := range shown {
		last := i == len(shown)-1 && rest == 0
		switch {
		case f.onPath[k]:
			f.node(k, parent, depth, last, OutCycle, true)
		case f.seen[k]:
			f.node(k, parent, depth, last, OutRef, false)
		default:
			f.seen[k] = true
			row := f.node(k, parent, depth, last, OutNode, f.snap.OnCycle(k))
			f.onPath[k] = true
			f.walk(k, row, depth+1, budget-1, next)
			delete(f.onPath, k)
			f.rows[row].End = len(f.rows)
		}
	}
	if rest > 0 {
		f.rows = append(f.rows, OutlineRow{Kind: OutMore, Parent: parent, Depth: depth, Last: true, Count: rest})
	}
}

// cut ends a branch at the depth limit: edges back into the path stay visible
// as cycle rows, the rest is counted.
func (f *focus) cut(kids []string, parent, depth int) {
	var back []string
	for _, k := range kids {
		if f.onPath[k] {
			back = append(back, k)
		}
	}
	more := len(kids) - len(back)
	for i, k := range back {
		f.node(k, parent, depth, i == len(back)-1 && more == 0, OutCycle, true)
	}
	if more > 0 {
		f.rows = append(f.rows, OutlineRow{Kind: OutMore, Parent: parent, Depth: depth, Last: true, Count: more})
	}
}

// Blockers lists what an issue waits on: its blocks edges and the blockers bd
// names for it, sorted by ID, present in the snapshot only.
func Blockers(snap *Snapshot, id string) []string {
	is, ok := snap.Issue(id)
	if !ok {
		return nil
	}
	var out []string
	add := func(to string) {
		if _, ok := snap.Issue(to); !ok || to == id {
			return
		}
		for _, x := range out {
			if x == to {
				return
			}
		}
		out = append(out, to)
	}
	for _, e := range is.Dependencies {
		if e.To != "" && (e.From == "" || e.From == id) && BlockingEdge(e.Type) {
			add(e.To)
		}
	}
	for _, b := range snap.BlockedBy(id) {
		add(b)
	}
	sort.Strings(out)
	return out
}

// Holding lists the issues that wait on id through a blocks edge, sorted by ID.
func Holding(snap *Snapshot, id string) []string {
	var out []string
	for _, e := range snap.Dependents(id) {
		if BlockingEdge(e.Type) && e.From != id && (len(out) == 0 || out[len(out)-1] != e.From) {
			out = append(out, e.From)
		}
	}
	return out
}

// FocusParent is the parent of an issue when the snapshot has it.
func FocusParent(snap *Snapshot, id string) string {
	is, ok := snap.Issue(id)
	if !ok {
		return ""
	}
	parent := is.Parent
	if parent == "" {
		for _, e := range is.Dependencies {
			if e.Type == EdgeParentChild && e.To != "" && (e.From == "" || e.From == id) {
				parent = e.To
				break
			}
		}
	}
	if _, ok := snap.Issue(parent); !ok {
		return ""
	}
	return parent
}

// FocusOptions tune BuildFocus.
type FocusOptions struct {
	// Depth is how many levels each outline shows; deeper branches end in a
	// row counting what was cut off.
	Depth int
	// Children lists the children of the focused issue after the outlines.
	Children bool
}

// BuildFocus draws one issue with what it waits on above and what it holds
// up below, each as an outline of blocks edges: the parent line, the waits-on
// outline, the issue, the holds-up outline and, on request, its children.
// Empty parts are left out. The rows are empty when id is not in the snapshot.
func BuildFocus(snap *Snapshot, st Statuses, id string, o FocusOptions) []OutlineRow {
	if _, ok := snap.Issue(id); !ok {
		return nil
	}
	depth := max(o.Depth, 1)
	f := &focus{snap: snap}
	if p := FocusParent(snap, id); p != "" {
		f.rows = append(f.rows, OutlineRow{Kind: OutParent, ID: p, Parent: -1})
	}
	waits, holds := Blockers(snap, id), Holding(snap, id)
	blockers := func(x string) []string { return Blockers(snap, x) }
	holding := func(x string) []string { return Holding(snap, x) }
	if len(waits) > 0 {
		open := 0
		for _, w := range waits {
			if snap.Present(w, st).Status != Closed {
				open++
			}
		}
		f.rows = append(f.rows, OutlineRow{Kind: OutWaits, Parent: -1, Count: len(waits), Aux: open})
		f.seen, f.onPath = map[string]bool{id: true}, map[string]bool{id: true}
		f.walk(id, -1, 1, depth, blockers)
	}
	f.rows = append(f.rows, OutlineRow{Kind: OutSelf, ID: id, Parent: -1, Member: snap.OnCycle(id)})
	if len(holds) > 0 {
		f.rows = append(f.rows, OutlineRow{Kind: OutHolds, Parent: -1, Count: len(holds)})
		f.seen, f.onPath = map[string]bool{id: true}, map[string]bool{id: true}
		f.walk(id, -1, 1, depth, holding)
	}
	if kids := snap.Children(id); o.Children && len(kids) > 0 {
		f.rows = append(f.rows, OutlineRow{Kind: OutKids, Parent: -1, Count: len(kids)})
		for _, k := range kids {
			f.rows = append(f.rows, OutlineRow{Kind: OutKid, ID: k, Parent: -1})
		}
	}
	return f.rows
}
