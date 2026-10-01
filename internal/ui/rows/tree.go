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
	// minTreeTitle cells stay for the title however deep the row sits.
	minTreeTitle = 20
	// treeIDRef is the ID width the type column is budgeted for; a wider ID
	// takes the room left over.
	treeIDRef = 16
	// stemShare is the largest part of a row, in percent, the stems may take.
	stemShare = 40
)

// UpGlyph marks a root whose parent is not in the snapshot.
func UpGlyph(g theme.Glyphs) string {
	if g.Tier == theme.TierASCII {
		return "^"
	}
	return "↑"
}

// Tree draws a row of the tree: the glyph column, the ID column, type and
// title, then the orphan marker and the right-hand block (progress for
// parents; priority, status text and assignee for leaves). The glyph column
// holds the fold caret of a parent and the status glyph of a leaf; the ID
// column holds the stems and the ID. A closed-children row shows the count in
// the ID column instead.
// Rows at depth 0 carry no stems, which is how views list children.
func (r *Renderer) Tree(row model.TreeRow) Body {
	return func(w int, sel bool) string { return r.tree(row, w, false, sel) }
}

// Flat is Tree for a short list of children whose ID column is not widened
// for the depth of the tree view.
func (r *Renderer) Flat(row model.TreeRow) Body {
	return func(w int, sel bool) string { return r.tree(row, w, true, sel) }
}

// TreeHeader is the dim column header line of the tree view, w cells wide
// including the gutter.
func (r *Renderer) TreeHeader(w int) string {
	bw := w - GutterWidth
	if bw <= 0 {
		return strings.Repeat(" ", max(w, 0))
	}
	plan := r.treePlan(bw, false)
	lead := r.glyphW() + 1
	cells := []headerCell{{lead, plan.id, "ID"}}
	titleAt := lead + plan.id + 1
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
	id     int
	stem   int
	typ    int
	fact   int
	status bool
	who    bool
}

const (
	treeStatusW = 11
)

// stemWidth is the cells the stems of a row at depth take: two a level and the
// gap before the ID.
func stemWidth(depth int) int {
	if depth == 0 {
		return 0
	}
	return treeIndent*depth + 1
}

// treePlan picks the widest right-hand block that leaves the title its
// minimum: priority, status text and assignee, then priority and status text,
// then priority alone. Narrow rows start at priority alone. When even that
// leaves no room for the title, the type column goes.
func (r *Renderer) treePlan(w int, flat bool) treeLayout {
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
	stem, need, label := min(r.stem, w*stemShare/100), r.need, r.label
	if flat {
		stem, need, label = 0, r.idW, r.idW
	}
	need = min(need, stem+label)
	idRef := min(need, treeIDRef+stem)
	for _, withType := range []bool{true, false} {
		for _, m := range modes {
			pick = m
			rest := r.glyphW() + 1 + 1 + 1 + m.fact
			if withType {
				pick.typ = r.typeCol(w - rest - idRef - minTreeTitle)
			} else {
				pick.typ = 0
			}
			pick.id = fitCol(need, w-rest-pick.typ-minTreeTitle)
			pick.stem = min(max(pick.id-label, min(stem, treeIndent+1+ansi.StringWidth(r.look.Glyphs.Ellipsis))), stem)
			if w-rest-pick.id-pick.typ >= minTreeTitle {
				return pick
			}
		}
	}
	return pick
}

func (r *Renderer) tree(row model.TreeRow, w int, flat, sel bool) string {
	paint := r.look.Paint
	if sel {
		paint = r.look.PaintSel
	}
	g := r.look.Glyphs
	plan := r.treePlan(w, flat)

	caret := ""
	if row.Foldable {
		caret = g.FoldOpen
		if row.Folded {
			caret = g.FoldClosed
		}
	}
	if row.Kind == model.TreeClosedFold {
		text := r.closedText(row)
		lead, _ := r.stems(row, plan, paint)
		return r.look.Fit(r.glyphCell(paint, theme.Dim, caret)+lead+paint(theme.Dim, text), w)
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
	textRole, idRole, caretRole := theme.Text, theme.Dim, theme.Dim
	dim := false
	switch {
	case row.Context:
		textRole, idRole, caretRole = theme.Faint, theme.Faint, theme.Faint
		statusRole = theme.Faint
		dim = true
	case pres.Status == model.Closed:
		textRole = theme.Dim
		dim = true
	}

	var b strings.Builder
	if row.Foldable {
		b.WriteString(r.glyphCell(paint, caretRole, caret))
	} else {
		b.WriteString(r.glyphCell(paint, statusRole, g.Status[idx]))
	}
	lead, lw := r.stems(row, plan, paint)
	b.WriteString(lead)
	b.WriteString(r.hl(paint, idRole, r.look.FitID(is.ID, plan.id-lw)))
	fixed := r.glyphW() + 1 + plan.id + 1 + plan.typ
	orphan := ""
	room := w - fixed - 1 - plan.fact
	if row.Orphan != "" {
		if o := UpGlyph(g) + " " + r.look.TruncID(row.Orphan, r.idW); room-ansi.StringWidth(o)-1 >= minTreeTitle {
			orphan = o
		}
	}
	titleW := max(room, 0)
	if orphan != "" {
		titleW -= ansi.StringWidth(orphan) + 1
	}

	b.WriteString(paint(theme.Text, " "))
	b.WriteString(r.typeCell(paint, is, plan.typ, dim))
	b.WriteString(r.hl(paint, textRole, r.look.Fit(oneLine(is.Title), titleW)))
	if orphan != "" {
		b.WriteString(paint(theme.Text, " "))
		b.WriteString(paint(theme.Faint, orphan))
	}
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(r.treeFact(paint, row.ID, is, pres, plan, statusRole, r.parentGlyph(pres, idx)))
	return r.look.Fit(b.String(), w)
}

// parentGlyph is the status glyph a parent shows before its progress count,
// where the caret has replaced it in its own column: always when colour cannot
// carry the status, and for a blocked parent whatever the colour.
func (r *Renderer) parentGlyph(pres model.Presentation, idx int) string {
	g := r.look.Glyphs
	switch {
	case pres.BlockedMarker:
		return g.Status[look.StatusIndex(int(model.Blocked))]
	case pres.Status == model.Blocked, r.look.Palette.Depth() == theme.DepthNone:
		return g.Status[idx]
	}
	return ""
}

// treeFact draws the right-hand block: progress for a parent; priority, status
// text and assignee for a leaf. The parts keep their columns from row to row.
func (r *Renderer) treeFact(paint func(theme.Role, string) string, id string, is *model.Issue, pres model.Presentation, plan treeLayout, progressRole theme.Role, parentGlyph string) string {
	g := r.look.Glyphs
	var fact string
	used := 0
	add := func(role theme.Role, s string) {
		fact += paint(role, s)
		used += ansi.StringWidth(s)
	}
	if prog, container := r.snap.Progress(id, r.statuses); container {
		if parentGlyph != "" {
			add(progressRole, parentGlyph+" ")
		}
		add(progressRole, fmt.Sprintf("%d/%d", prog.Direct.Closed, prog.Direct.Total))
		if used+1+treeBarW <= plan.fact && prog.Descendants.Total > 0 {
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

// closedText is the label of a closed-children row.
func (r *Renderer) closedText(row model.TreeRow) string {
	return fmt.Sprintf("%s %d closed", r.look.Glyphs.Status[look.StatusIndex(int(model.Closed))], row.Closed)
}

// stems draws the branch of a row: a vertical line for each ancestor that has
// later siblings, then the row's own connector and a gap, two cells a level.
// Stems wider than the frame's budget lose their outer levels, marked by an
// ellipsis, so every row of a frame loses the same cells whatever its ID. It
// returns the drawing and its width.
func (r *Renderer) stems(row model.TreeRow, plan treeLayout, paint func(theme.Role, string) string) (string, int) {
	d := row.Depth
	if d == 0 {
		return "", 0
	}
	g := r.look.Glyphs
	type seg struct {
		text string
		role theme.Role
	}
	segs := make([]seg, 0, d+2)
	drop := max(stemWidth(d)-plan.stem, 0)
	if drop > 0 {
		ew := ansi.StringWidth(g.Ellipsis)
		segs = append(segs, seg{g.Ellipsis, theme.Faint})
		drop += ew
	}
	for j := 1; j < d; j++ {
		if j < 64 && row.More&(1<<uint(j)) != 0 {
			segs = append(segs, seg{r.look.Fit(g.Vertical, treeIndent), theme.Rule})
		} else {
			segs = append(segs, seg{strings.Repeat(" ", treeIndent), theme.Text})
		}
	}
	join := g.Branch
	if row.Last {
		join = g.LastBranch
	}
	segs = append(segs, seg{r.look.Fit(join, treeIndent), theme.Rule}, seg{" ", theme.Text})
	var b strings.Builder
	w := 0
	for i, sg := range segs {
		if i == 0 && drop > 0 {
			b.WriteString(paint(sg.role, sg.text))
			w += ansi.StringWidth(sg.text)
			continue
		}
		sw := ansi.StringWidth(sg.text)
		if drop >= sw {
			drop -= sw
			continue
		}
		text := ansi.TruncateLeft(sg.text, drop, "")
		drop = 0
		b.WriteString(paint(sg.role, text))
		w += ansi.StringWidth(text)
	}
	return b.String(), w
}
