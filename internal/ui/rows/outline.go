package rows

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
)

const (
	guideWide   = 3
	guideNarrow = 2
	// minLabel cells stay for the label of an outline row however deep it
	// sits; deeper rows scroll sideways.
	minLabel = 24
)

// OutlineStyle is the per-frame state of an outline row.
type OutlineStyle struct {
	// Narrow draws the guides two cells a level instead of three.
	Narrow bool
	// Sel asks for the current-row styling.
	Sel bool
	// Dim draws an issue outside the scope fainter.
	Dim bool
	// Hidden is the number of rows folded below the row, 0 when it is open.
	Hidden int
	// XOff is the number of cells the outline is scrolled to the left.
	XOff int
}

// GuideCells is the width of one guide level.
func (s OutlineStyle) GuideCells() int {
	if s.Narrow {
		return guideNarrow
	}
	return guideWide
}

// RefGlyph marks a later sighting of an issue drawn elsewhere.
func RefGlyph(g theme.Glyphs) string {
	if g.Tier == theme.TierASCII {
		return "~"
	}
	if g.Tier == theme.TierSafe {
		return "→"
	}
	return "↗"
}

// CycleGlyph marks an issue that lies on a dependency cycle.
func CycleGlyph(g theme.Glyphs) string {
	if g.Tier == theme.TierASCII {
		return "@"
	}
	if g.Tier == theme.TierSafe {
		return "∞"
	}
	return "⟲"
}

// WaitsGlyph heads what an issue waits on.
func WaitsGlyph(g theme.Glyphs) string {
	if g.Tier == theme.TierASCII {
		return "<"
	}
	return "▲"
}

// HoldsGlyph heads what an issue holds up.
func HoldsGlyph(g theme.Glyphs) string {
	if g.Tier == theme.TierASCII {
		return ">"
	}
	return "▼"
}

// FoldedGlyph precedes the count of rows a fold hides.
func FoldedGlyph(g theme.Glyphs) string {
	if g.Tier == theme.TierASCII {
		return "+"
	}
	return "▸"
}

// Outline draws row i of an outline as exactly w cells, scrolled by st.XOff:
// the guides, then the label. Only the guide levels and label cells inside
// the window are painted, so deep rows cost no more than shallow ones.
func (r *Renderer) Outline(all []model.OutlineRow, i, w int, st OutlineStyle) string {
	if w <= 0 {
		return ""
	}
	row := all[i]
	paint := r.look.Paint
	if st.Sel {
		paint = r.look.PaintSel
	}
	cw := st.GuideCells()
	d := row.Depth
	lo := min(st.XOff/cw+1, d+1)
	hi := min(d, (st.XOff+w-1)/cw+1)
	c0 := (lo - 1) * cw

	var b strings.Builder
	if lo <= hi {
		b.WriteString(r.guideLevels(all, i, lo, hi, cw, paint, st.Dim))
	}
	labelW := max(w-d*cw, minLabel)
	if labelStart := d * cw; labelStart < st.XOff+w {
		b.WriteString(r.outlineLabel(row, labelW, paint, st))
	}
	s := ansi.Cut(b.String(), st.XOff-c0, st.XOff-c0+w)
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += paint(theme.Text, strings.Repeat(" ", pad))
	}
	return s
}

// guideLevels draws the guide levels lo..hi of row i. The level of the row
// itself is its connector; every level left of it is a vertical line while
// the ancestor there has later siblings.
func (r *Renderer) guideLevels(all []model.OutlineRow, i, lo, hi, cw int, paint func(theme.Role, string) string, dim bool) string {
	g := r.look.Glyphs
	row := all[i]
	vert := make([]bool, row.Depth+1)
	anc := row.Parent
	for level := row.Depth - 1; level >= 1 && anc >= 0; level-- {
		vert[level] = !all[anc].Last
		anc = all[anc].Parent
	}
	cell := func(s string) string {
		if cw == guideWide {
			return s + " "
		}
		return s
	}
	line := theme.Rule
	if row.Dim || dim {
		line = theme.Faint
	}
	var b strings.Builder
	for level := lo; level <= hi; level++ {
		switch {
		case level == row.Depth && row.Last:
			b.WriteString(paint(line, cell(g.LastBranch)))
		case level == row.Depth:
			b.WriteString(paint(line, cell(g.Branch)))
		case vert[level]:
			b.WriteString(paint(theme.Rule, cell(g.Vertical)))
		default:
			b.WriteString(paint(theme.Text, strings.Repeat(" ", cw)))
		}
	}
	return b.String()
}

func (r *Renderer) outlineLabel(row model.OutlineRow, w int, paint func(theme.Role, string) string, st OutlineStyle) string {
	g := r.look.Glyphs
	text := func(role theme.Role, s string) string {
		if st.Dim {
			role = theme.Faint
		}
		return paint(role, s)
	}
	switch row.Kind {
	case model.OutHeader:
		return paint(theme.Dim, r.look.Fit(fmt.Sprintf("%s%s %d issues", g.Rule, g.Rule, row.Count), w))
	case model.OutIsolated:
		return paint(theme.Dim, r.look.Fit(fmt.Sprintf("%s%s without dependencies (%d)", g.Rule, g.Rule, row.Count), w))
	case model.OutWaits:
		return paint(theme.Dim, r.look.Fit(fmt.Sprintf("%s waits on (%d · %d open)", WaitsGlyph(g), row.Count, row.Aux), w))
	case model.OutHolds:
		return paint(theme.Dim, r.look.Fit(fmt.Sprintf("%s holds up (%d)", HoldsGlyph(g), row.Count), w))
	case model.OutKids:
		return paint(theme.Dim, r.look.Fit(fmt.Sprintf("children (%d)", row.Count), w))
	case model.OutMore:
		return paint(theme.Faint, r.look.Fit(fmt.Sprintf("%s %d more", g.Ellipsis, row.Count), w))
	case model.OutCycle:
		return text(theme.Dim, r.look.Fit(fmt.Sprintf("%s %s (cycle)", CycleGlyph(g), row.ID), w))
	case model.OutRef:
		return r.refLabel(row, w, text)
	case model.OutNode, model.OutParent, model.OutSelf, model.OutKid:
	}
	return r.issueLabel(row, w, text, st)
}

func (r *Renderer) refLabel(row model.OutlineRow, w int, text func(theme.Role, string) string) string {
	g := r.look.Glyphs
	head := RefGlyph(g) + " " + ansi.Truncate(row.ID, maxIDWidth, g.Ellipsis)
	title := ""
	if is, ok := r.snap.Issue(row.ID); ok {
		title = oneLine(is.Title)
	}
	room := w - ansi.StringWidth(head) - 1
	if room <= 0 || title == "" {
		return text(theme.Faint, r.look.Fit(head, w))
	}
	return text(theme.Faint, head+" ") + text(theme.Faint, r.look.Fit(title, room))
}

func (r *Renderer) issueLabel(row model.OutlineRow, w int, text func(theme.Role, string) string, st OutlineStyle) string {
	g := r.look.Glyphs
	is, ok := r.snap.Issue(row.ID)
	if !ok {
		return text(theme.Faint, r.look.Fit(row.ID, w))
	}
	pres := r.snap.Present(row.ID, r.statuses)
	idx := look.StatusIndex(int(pres.Status))
	statusRole := theme.StatusRole(idx)
	if pres.BlockedMarker {
		statusRole = theme.StatusBlocked
	}
	titleRole, idRole := theme.Text, theme.Dim
	if pres.Status == model.Closed {
		titleRole = theme.Dim
	}
	if row.Kind == model.OutSelf {
		idRole = theme.Strong
	}
	var head strings.Builder
	if row.Kind == model.OutParent {
		head.WriteString(text(theme.Dim, UpGlyph(g)+" "))
	}
	head.WriteString(text(statusRole, g.Status[idx]))
	head.WriteString(text(theme.Text, " "))
	id := ansi.Truncate(is.ID, maxIDWidth, g.Ellipsis)
	head.WriteString(text(idRole, id))
	headW := ansi.StringWidth(head.String())
	if row.Member {
		head.WriteString(text(theme.Changed, " "+CycleGlyph(g)))
		headW += 1 + ansi.StringWidth(CycleGlyph(g))
	}
	suffix := ""
	if st.Hidden > 0 {
		suffix = fmt.Sprintf(" %s%d", FoldedGlyph(g), st.Hidden)
	}
	titleW := w - headW - 1 - ansi.StringWidth(suffix)
	if titleW <= 0 {
		return head.String() + text(theme.Text, strings.Repeat(" ", max(w-headW, 0)))
	}
	title := r.look.Fit(oneLine(is.Title), titleW)
	if suffix != "" {
		return head.String() + text(theme.Text, " ") + text(titleRole, title) + text(theme.Dim, suffix)
	}
	return head.String() + text(theme.Text, " ") + text(titleRole, title)
}
