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
)

// UpGlyph marks a root whose parent is not in the snapshot.
func UpGlyph(g theme.Glyphs) string {
	if g.Tier == theme.TierASCII {
		return "^"
	}
	return "↑"
}

// Tree draws a row of the tree: guides, fold marker, status glyph, ID and
// title, then the orphan marker and the fact (progress for parents, priority
// and assignee for leaves). A closed-children row shows the count instead.
// Rows at depth 0 carry no guides, which is how views list children.
func (r *Renderer) Tree(row model.TreeRow) Body {
	return func(w int, sel bool) string { return r.tree(row, w, sel) }
}

func (r *Renderer) tree(row model.TreeRow, w int, sel bool) string {
	paint := r.look.Paint
	if sel {
		paint = r.look.PaintSel
	}
	g := r.look.Glyphs
	narrow := Narrow(w)

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
	switch {
	case row.Context:
		textRole, idRole = theme.Faint, theme.Faint
		statusRole = theme.Faint
	case pres.Status == model.Closed:
		textRole = theme.Dim
	}

	id := r.look.TruncID(is.ID, maxIDWidth)
	sw := ansi.StringWidth(g.Status[idx])

	prog, container := r.snap.Progress(row.ID, r.statuses)
	orphan := ""
	if row.Orphan != "" {
		orphan = UpGlyph(g) + " " + r.look.TruncID(row.Orphan, maxIDWidth)
	}
	who := ansi.Truncate(oneLine(is.Assignee), treeWho, g.Ellipsis)
	showBar, showWho, showOrphan := !narrow, !narrow && who != "", orphan != ""

	factWidth := func() int {
		switch {
		case container && showBar:
			return 13
		case container:
			return 5
		case showWho:
			return 13
		}
		return 2
	}
	fixed := 1 + 2 + sw + 1 + ansi.StringWidth(id) + 1
	guideCap := max(min(w*guideShare/100, w-fixed-minTreeTitle-5), treeIndent)
	guides, gw := r.guides(row, guideCap, paint)
	room := func() int {
		n := w - gw - fixed - 1 - factWidth()
		if showOrphan {
			n -= ansi.StringWidth(orphan) + 1
		}
		return n
	}
	if room() < minTreeTitle && showWho {
		showWho = false
	}
	if room() < minTreeTitle && showBar {
		showBar = false
	}
	if room() < minTreeTitle {
		showOrphan = false
	}

	var fact string
	if container {
		num := fmt.Sprintf("%d/%d", prog.Direct.Closed, prog.Direct.Total)
		fact = paint(theme.Text, num)
		if showBar && prog.Descendants.Total > 0 {
			full := prog.Descendants.Closed * treeBarW / prog.Descendants.Total
			fact += paint(theme.Text, " ") + paint(theme.Success, strings.Repeat(strings.TrimRight(g.BarFull, " "), full)) +
				paint(theme.Faint, strings.Repeat(strings.TrimRight(g.BarEmpty, " "), treeBarW-full))
		}
	} else {
		fact = paint(theme.PriorityRole(is.Priority), "P"+strconv.Itoa(min(max(is.Priority, 0), 9)))
		if showWho {
			fact += paint(theme.Text, " ") + paint(theme.Dim, r.look.Fit(who, treeWho))
		}
	}
	fw := factWidth()
	if container && !showBar {
		fw = 5
	}
	factPad := max(fw-ansi.StringWidth(fact), 0)

	titleW := max(w-gw-fixed-1-fw-boolInt(showOrphan)*(ansi.StringWidth(orphan)+1), 0)
	var b strings.Builder
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(guides)
	b.WriteString(paint(theme.Dim, fold))
	b.WriteString(paint(statusRole, g.Status[idx]))
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(r.hl(paint, idRole, id))
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(r.hl(paint, textRole, r.look.Fit(oneLine(is.Title), titleW)))
	if showOrphan {
		b.WriteString(paint(theme.Text, " "))
		b.WriteString(paint(theme.Faint, orphan))
	}
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(paint(theme.Text, strings.Repeat(" ", factPad)))
	b.WriteString(fact)
	return r.look.Fit(b.String(), w)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
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
