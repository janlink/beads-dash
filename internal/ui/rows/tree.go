package rows

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// NarrowWidth is the row width, gutter included, below which facts shrink to
// the bare minimum: progress without its bar, priority without the assignee.
const NarrowWidth = 80

// Narrow reports whether a row body of width w is drawn in the narrow form.
func Narrow(w int) bool { return w+GutterWidth < NarrowWidth }

const (
	treeIndent = 2
	treeBarW   = 6
	treeWho    = 10
	// guideShare is the largest part of a row, in percent, the guides may take.
	guideShare = 40
	// minTitle cells stay for the title however deep the row sits.
	minTreeTitle = 20
	// treeIDRef is the ID width guides are budgeted for; a wider ID takes the
	// room left over.
	treeIDRef = 16
)

// UpGlyph marks a root whose parent is not in the snapshot.
func UpGlyph(g theme.Glyphs) string {
	if g.Tier == theme.TierASCII {
		return "^"
	}
	return "↑"
}

// Tree draws a row of the tree: guides, fold marker, status glyph, ID, type
// and title, then the orphan marker and the right-hand block (progress for
// parents; priority, status text and assignee for leaves). A closed-children
// row shows the count instead.
// Rows at depth 0 carry no guides, which is how views list children.
func (r *Renderer) Tree(row model.TreeRow) Body {
	return func(w int, sel bool) string { return r.tree(row, w, r.depth, sel) }
}

// Flat is Tree for a short list of children that does not pad its guides to
// the depth of the tree view.
func (r *Renderer) Flat(row model.TreeRow) Body {
	return func(w int, sel bool) string { return r.tree(row, w, 0, sel) }
}

// TreeHeader is the dim column header line of the tree view, w cells wide
// including the gutter.
func (r *Renderer) TreeHeader(w int) string {
	bw := w - GutterWidth
	if bw <= 0 {
		return strings.Repeat(" ", max(w, 0))
	}
	plan := r.treePlan(bw, r.depth)
	sw := ansi.StringWidth(r.look.Glyphs.Status[0])
	lead := 1 + 2 + sw + 1
	cells := []headerCell{{lead, plan.id, "ID"}}
	titleAt := 1 + plan.guide + 2 + sw + 1 + plan.id + 1
	if plan.typ > 0 {
		cells = append(cells, headerCell{titleAt, plan.typ - 1, "TYPE"})
		titleAt += plan.typ
	}
	cells = append(cells, headerCell{titleAt, 0, "TITLE"})
	factAt := bw - plan.fact
	cells = append(cells, headerCell{factAt, 2, "PR"})
	if plan.status {
		cells = append(cells, headerCell{factAt + 3, treeStatusW, "STATUS"})
	}
	if plan.who {
		cells = append(cells, headerCell{factAt + 3 + treeStatusW + 1, treeWho, "WHO"})
	}
	return strings.Repeat(" ", GutterWidth) + r.headerLine(bw, cells)
}

// treeLayout is the column plan of tree rows for one body width; every row
// of a frame shares it so the columns line up.
type treeLayout struct {
	guide  int
	id     int
	typ    int
	fact   int
	status bool
	who    bool
}

const (
	treeStatusW = 11
	treeBarMin  = 12
)

// treePlan picks the widest right-hand block that leaves the title its
// minimum: priority, status text and assignee, then priority and status text,
// then priority alone. Narrow rows start at priority alone. When even that
// leaves no room for the title, the type column goes.
func (r *Renderer) treePlan(w, depth int) treeLayout {
	prio := 2
	modes := []treeLayout{
		{fact: prio + 1 + treeStatusW + 1 + treeWho, status: true, who: true},
		{fact: prio + 1 + treeStatusW, status: true},
		{fact: prio + 3},
	}
	if Narrow(w) {
		modes = modes[2:]
	}
	var pick treeLayout
	for _, withType := range []bool{true, false} {
		for _, m := range modes {
			pick = m
			idRef := min(r.idW, treeIDRef)
			pick.guide = max(min(2*depth, w*guideShare/100), 0)
			pick.guide = min(pick.guide, max(w-idRef-minTreeTitle-m.fact-8, treeIndent))
			if depth == 0 {
				pick.guide = 0
			}
			rest := 1 + pick.guide + 2 + ansi.StringWidth(r.look.Glyphs.Status[0]) + 1 + 1 + m.fact
			if withType {
				pick.typ = r.typeCol(w - rest - idRef - minTreeTitle)
			} else {
				pick.typ = 0
			}
			pick.id = r.idCol(w - rest - pick.typ - minTreeTitle)
			if w-rest-pick.id-pick.typ >= minTreeTitle {
				return pick
			}
		}
	}
	return pick
}

func (r *Renderer) tree(row model.TreeRow, w, depth int, sel bool) string {
	paint := r.look.Paint
	if sel {
		paint = r.look.PaintSel
	}
	g := r.look.Glyphs

	fold := "  "
	if row.Foldable {
		mark := g.FoldOpen
		if row.Folded {
			mark = g.FoldClosed
		}
		fold = mark + " "
	}
	if row.Kind == model.TreeClosedFold {
		guides, _ := r.guides(row, w*guideShare/100, paint)
		text := fmt.Sprintf("%s %d closed", g.Status[look.StatusIndex(int(model.Closed))], row.Closed)
		return r.look.Fit(" "+guides+paint(theme.Dim, fold+text), w)
	}

	is, ok := r.snap.Issue(row.ID)
	if !ok {
		return paint(theme.Faint, r.look.Fit(row.ID, w))
	}
	pres := r.snap.Present(row.ID, r.statuses)
	idx := look.StatusIndex(int(pres.Status))
	statusRole := theme.StatusRole(idx)
	if pres.BlockedMarker {
		statusRole = theme.StatusBlocked
	}
	textRole, idRole := theme.Text, theme.Dim
	dim := false
	switch {
	case row.Context:
		textRole, idRole = theme.Faint, theme.Faint
		statusRole = theme.Faint
		dim = true
	case pres.Status == model.Closed:
		textRole = theme.Dim
		dim = true
	}

	plan := r.treePlan(w, max(depth, row.Depth))
	guides, gw := r.guides(row, plan.guide, paint)
	guidePad := max(plan.guide-gw, 0)
	id := r.look.FitID(is.ID, plan.id)
	sw := ansi.StringWidth(g.Status[idx])
	fixed := 1 + max(gw, plan.guide) + 2 + sw + 1 + plan.id + 1 + plan.typ
	orphan := ""
	room := w - fixed - 1 - plan.fact
	if row.Orphan != "" {
		if o := UpGlyph(g) + " " + r.look.TruncID(row.Orphan, plan.id); room-ansi.StringWidth(o)-1 >= minTreeTitle {
			orphan = o
		}
	}
	titleW := max(room, 0)
	if orphan != "" {
		titleW -= ansi.StringWidth(orphan) + 1
	}

	var b strings.Builder
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(guides)
	b.WriteString(paint(theme.Dim, fold))
	b.WriteString(paint(statusRole, g.Status[idx]+" "))
	b.WriteString(r.hl(paint, idRole, id))
	b.WriteString(paint(theme.Text, strings.Repeat(" ", 1+guidePad)))
	b.WriteString(r.typeCell(paint, is, plan.typ, dim))
	b.WriteString(r.hl(paint, textRole, r.look.Fit(oneLine(is.Title), titleW)))
	if orphan != "" {
		b.WriteString(paint(theme.Text, " "))
		b.WriteString(paint(theme.Faint, orphan))
	}
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(r.treeFact(paint, row.ID, is, pres, plan))
	return r.look.Fit(b.String(), w)
}

// treeFact draws the right-hand block: progress for a parent; priority, status
// text and assignee for a leaf. The parts keep their columns from row to row.
func (r *Renderer) treeFact(paint func(theme.Role, string) string, id string, is *model.Issue, pres model.Presentation, plan treeLayout) string {
	g := r.look.Glyphs
	var fact string
	used := 0
	add := func(role theme.Role, s string) {
		fact += paint(role, s)
		used += ansi.StringWidth(s)
	}
	if prog, container := r.snap.Progress(id, r.statuses); container {
		add(theme.Text, fmt.Sprintf("%d/%d", prog.Direct.Closed, prog.Direct.Total))
		if plan.fact >= treeBarMin && prog.Descendants.Total > 0 {
			full := prog.Descendants.Closed * treeBarW / prog.Descendants.Total
			add(theme.Text, " ")
			add(theme.Success, strings.Repeat(strings.TrimRight(g.BarFull, " "), full))
			add(theme.Faint, strings.Repeat(strings.TrimRight(g.BarEmpty, " "), treeBarW-full))
		}
	} else {
		add(theme.PriorityRole(is.Priority), "P"+strconv.Itoa(min(max(is.Priority, 0), 9)))
		if plan.status {
			add(theme.Text, " ")
			role := theme.StatusRole(look.StatusIndex(int(pres.Status)))
			if pres.BlockedMarker {
				role = theme.StatusBlocked
			}
			if pres.Status == model.Closed {
				role = theme.Dim
			}
			text := r.look.Fit(ansi.Truncate(statusText(pres, is), treeStatusW, g.Ellipsis), treeStatusW)
			add(role, text)
		}
		if plan.who && is.Assignee != "" {
			add(theme.Text, " ")
			add(theme.Dim, ansi.Truncate(oneLine(is.Assignee), treeWho, g.Ellipsis))
		}
	}
	if pad := plan.fact - used; pad > 0 {
		fact += paint(theme.Text, strings.Repeat(" ", pad))
	}
	return fact
}

// guides draws the indent of a row: a vertical line for each ancestor that
// has later siblings and the branch to the row itself, two cells a level.
// When that would take more than capCells the outer levels collapse into a
// marker counting them.
func (r *Renderer) guides(row model.TreeRow, capCells int, paint func(theme.Role, string) string) (string, int) {
	d := row.Depth
	if d == 0 {
		return "", 0
	}
	g := r.look.Glyphs
	cells := make([]string, d)
	for j := 1; j < d; j++ {
		if j < 64 && row.More&(1<<uint(j)) != 0 {
			cells[j-1] = g.Vertical
		} else {
			cells[j-1] = strings.Repeat(" ", treeIndent)
		}
	}
	cells[d-1] = g.Branch
	if row.Last {
		cells[d-1] = g.LastBranch
	}
	marker := ""
	hidden := 0
	if treeIndent*d > capCells {
		hidden = d
		for range 3 {
			marker = g.Ellipsis + strconv.Itoa(hidden)
			keep := max((capCells-ansi.StringWidth(marker))/treeIndent, 1)
			hidden = max(d-keep, 0)
		}
		marker = g.Ellipsis + strconv.Itoa(hidden)
	}
	var b strings.Builder
	w := 0
	if hidden > 0 {
		b.WriteString(paint(theme.Faint, marker))
		w += ansi.StringWidth(marker)
	}
	for _, c := range cells[hidden:] {
		b.WriteString(paint(theme.Rule, c))
		w += ansi.StringWidth(c)
	}
	return b.String(), w
}
