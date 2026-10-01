// Package rows draws issue rows: the shared gutter and per-snapshot cached row
// bodies. Views supply which rows are visible; the highlight and mark state
// composes per frame outside the cache.
package rows

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// GutterWidth is the cells the gutter takes: the current band and one cell
// shared by the mark and the change marker.
const GutterWidth = 2

const (
	maxFactWidth = 14
	minTitle     = 20
	// maxTypeW caps the type column however long a custom type is.
	maxTypeW = 10
	// minIDCol is the narrowest an ID column is squeezed to, unless every ID is
	// shorter.
	minIDCol = 8
)

// Row is one row to draw and its per-frame state. The gutter's priority bar
// follows the issue ID names; a Closed row draws the closed bar instead.
type Row struct {
	ID      string
	Current bool
	Marked  bool
	Changed bool
	Closed  bool
}

// Body draws the part of a row right of the gutter, exactly w cells wide; sel
// asks for the current-row styling.
type Body func(w int, sel bool) string

type cacheKey struct {
	view, id string
	w        int
}

// maxCached bounds the body cache across resizes.
const maxCached = 1 << 14

// Renderer draws rows for one snapshot. It is not safe for concurrent use.
type Renderer struct {
	look     look.Look
	snap     *model.Snapshot
	statuses model.Statuses
	idW      int
	typeW    int
	stem     int
	need     int
	label    int
	bars     bool
	cache    map[cacheKey]string
	terms    []needle
	gutter   [barStates][2][3]string
}

// barStates counts the bar variants of a gutter: none, one per priority and
// closed.
const barStates = 7

const barClosed = 6

// The states of the gutter's shared cell.
const (
	slotNone = iota
	slotChange
	slotMark
)

// New returns a renderer drawing with l.
func New(l look.Look) *Renderer {
	r := &Renderer{cache: map[cacheKey]string{}}
	r.SetLook(l)
	return r
}

// SetLook switches palette or glyphs, which invalidates every cached row.
func (r *Renderer) SetLook(l look.Look) {
	r.look = l
	clear(r.cache)
	g := l.Glyphs
	r.bars = g.Tier != theme.TierASCII && barsDistinct(l)
	cell := func(on bool, glyph string, role theme.Role, sel bool) string {
		text := " "
		if on {
			text = glyph
		}
		switch {
		case sel && on:
			return l.PaintSel(role, text)
		case sel:
			return l.PaintSel(theme.Text, text)
		case on:
			return l.Paint(role, text)
		}
		return text
	}
	for bar := range barStates {
		for cur := range 2 {
			band, role := cur == 1, theme.Primary
			if bar > 0 && r.bars {
				band, role = true, barRole(bar)
			}
			r.gutter[bar][cur][slotNone] = cell(band, g.Band, role, cur == 1) +
				cell(false, "", theme.Text, cur == 1)
			r.gutter[bar][cur][slotChange] = cell(band, g.Band, role, cur == 1) +
				cell(true, g.Change, theme.Changed, cur == 1)
			r.gutter[bar][cur][slotMark] = cell(band, g.Band, role, cur == 1) +
				cell(true, g.Mark, theme.Strong, cur == 1)
		}
	}
}

// barsDistinct reports whether the palette tells priorities apart by colour;
// a colourless one would draw every bar the same.
func barsDistinct(l look.Look) bool {
	if l.Palette.Depth() == theme.DepthNone {
		return false
	}
	seen := map[string]bool{}
	for p := range 5 {
		seen[l.Paint(theme.PriorityRole(p), "x")] = true
	}
	seen[l.Paint(theme.Rule, "x")] = true
	return len(seen) >= 5
}

func barRole(bar int) theme.Role {
	return theme.GutterRole(bar-1, bar == barClosed)
}

// SetTreeRows sets the rows of the tree being drawn; the ID column is measured
// over them so it does not change as the view scrolls.
func (r *Renderer) SetTreeRows(tree []model.TreeRow) {
	stem, need, labelW := 0, 0, 0
	for _, row := range tree {
		label := ansi.StringWidth(row.ID)
		if row.Kind == model.TreeClosedFold {
			label = ansi.StringWidth(r.closedText(row))
		}
		labelW = max(labelW, label)
		stem = max(stem, stemWidth(row.Depth))
		need = max(need, stemWidth(row.Depth)+label)
	}
	if stem != r.stem || need != r.need || labelW != r.label {
		r.stem, r.need, r.label = stem, need, labelW
		clear(r.cache)
	}
}

// Bind sets the snapshot and status table rows are read from. A new snapshot
// invalidates the cached rows.
func (r *Renderer) Bind(snap *model.Snapshot, st model.Statuses) {
	if snap == r.snap {
		return
	}
	r.snap, r.statuses = snap, st
	clear(r.cache)
	r.idW, r.typeW = 0, 0
	if snap != nil {
		for _, id := range snap.IDs() {
			r.idW = max(r.idW, ansi.StringWidth(id))
			if is, ok := snap.Issue(id); ok {
				r.typeW = max(r.typeW, min(ansi.StringWidth(oneLine(is.IssueType)), maxTypeW))
			}
		}
	}
}

// idCol is the width of the ID column when avail cells are left for it: the
// longest ID if that fits, else the room, but never less than minIDCol.
func (r *Renderer) idCol(avail int) int { return fitCol(r.idW, avail) }

// fitCol is need cells when avail leaves that much, else the room, but never
// less than minIDCol.
func fitCol(need, avail int) int { return max(min(need, avail), min(need, minIDCol)) }

// glyphW is the width of the glyph column that heads a row: the status glyph,
// or the fold caret of a parent.
func (r *Renderer) glyphW() int { return ansi.StringWidth(r.look.Glyphs.Status[0]) }

// glyphCell draws s padded to the glyph column and the gap after it, in one
// role.
func (r *Renderer) glyphCell(paint func(theme.Role, string) string, role theme.Role, s string) string {
	return paint(role, r.look.Fit(s, r.glyphW())+" ")
}

// barState picks the gutter's bar variant of a row.
func (r *Renderer) barState(row Row) int {
	if !r.bars {
		return 0
	}
	if row.Closed {
		return barClosed
	}
	if row.ID == "" || r.snap == nil {
		return 0
	}
	is, ok := r.snap.Issue(row.ID)
	if !ok {
		return 0
	}
	if r.snap.Present(row.ID, r.statuses).Status == model.Closed {
		return barClosed
	}
	return min(max(is.Priority, 0), 4) + 1
}

// Gutter is the two gutter cells of a row: the priority bar (or the current
// band where there are no colours) and the cell for the mark, which wins over
// the change marker.
func (r *Renderer) Gutter(row Row) string {
	cur, slot := 0, slotNone
	if row.Current {
		cur = 1
	}
	switch {
	case row.Marked:
		slot = slotMark
	case row.Changed:
		slot = slotChange
	}
	return r.gutter[r.barState(row)][cur][slot]
}

// Line draws one row exactly w cells wide: the gutter composed for this
// frame, then the body that view's provider draws. Bodies of rows that are not
// current are cached per (view, id, w) until the snapshot or look changes.
func (r *Renderer) Line(w int, row Row, view string, body Body) string {
	if w <= GutterWidth {
		return strings.Repeat(" ", max(w, 0))
	}
	bw := w - GutterWidth
	if row.Current {
		return r.Gutter(row) + body(bw, true)
	}
	k := cacheKey{view, row.ID, bw}
	s, ok := r.cache[k]
	if !ok {
		if len(r.cache) >= maxCached {
			clear(r.cache)
		}
		s = body(bw, false)
		r.cache[k] = s
	}
	return r.Gutter(row) + s
}

// Standard is the one-line body of an issue: status, priority, ID, title and
// assignee.
func (r *Renderer) Standard(id string) Body {
	return func(w int, sel bool) string { return r.render(w, id, sel, true) }
}

// Unassigned is the standard row without the assignee, for lists grouped by
// assignee.
func (r *Renderer) Unassigned(id string) Body {
	return func(w int, sel bool) string { return r.render(w, id, sel, false) }
}

func (r *Renderer) render(w int, id string, sel, withAssignee bool) string {
	paint := r.look.Paint
	if sel {
		paint = r.look.PaintSel
	}
	is, ok := r.snap.Issue(id)
	if !ok {
		return paint(theme.Faint, r.look.Fit(id, w))
	}
	g := r.look.Glyphs
	pres := r.snap.Present(id, r.statuses)
	idx := look.StatusIndex(int(pres.Status))
	statusRole := theme.StatusRole(idx)
	if pres.BlockedMarker {
		statusRole = theme.StatusBlocked
	}

	prio := "P" + string(rune('0'+min(max(is.Priority, 0), 9)))
	rest := r.glyphW() + 1 + 1 + 2 + 1
	factW := 0
	if withAssignee {
		factW = maxFactWidth + 1
	}
	typeW := r.typeCol(w - rest - factW - min(r.idW, minIDCol) - minTitle)
	idW := r.idCol(w - rest - factW - typeW - minTitle)
	idText := r.look.FitID(id, idW)
	if withAssignee && w-rest-idW-typeW-factW < minTitle {
		factW = 0
	}
	titleW := max(w-rest-idW-typeW-factW, 0)
	dim := pres.Status == model.Closed
	titleRole := theme.Text
	if dim {
		titleRole = theme.Dim
	}
	var b strings.Builder
	sp := paint(theme.Text, " ")
	b.WriteString(r.glyphCell(paint, statusRole, g.Status[idx]))
	b.WriteString(r.hl(paint, theme.Dim, idText))
	b.WriteString(sp)
	b.WriteString(r.typeCell(paint, is, typeW, dim))
	b.WriteString(r.hl(paint, titleRole, r.look.Fit(oneLine(is.Title), titleW)))
	b.WriteString(sp)
	b.WriteString(paint(theme.PriorityRole(is.Priority), prio))
	if factW > 0 {
		b.WriteString(sp)
		b.WriteString(paint(theme.Dim, r.look.Fit(ansi.Truncate(oneLine(is.Assignee), maxFactWidth, g.Ellipsis), maxFactWidth)))
	}
	return b.String()
}

// typeCol is the width of the type column, including its separator, when room
// cells are left for it and the title; it is zero when the column would not
// leave the title its minimum.
func (r *Renderer) typeCol(room int) int {
	if r.typeW == 0 || room < r.typeW+1 {
		return 0
	}
	return r.typeW + 1
}

// typeCell draws the type word padded to the column and its separator; it is
// empty when the column is not shown.
func (r *Renderer) typeCell(paint func(theme.Role, string) string, is *model.Issue, col int, dim bool) string {
	if col == 0 {
		return ""
	}
	role := theme.TypeRole(is.IssueType)
	if dim {
		role = theme.Dim
	}
	return paint(role, r.look.Fit(oneLine(is.IssueType), col-1)) + paint(theme.Text, " ")
}

// statusText names the status of an issue on a row's right side; it is empty
// for an open issue, whose glyph says enough.
func statusText(pres model.Presentation, is *model.Issue) string {
	switch pres.Status {
	case model.Open:
		return ""
	case model.InProgress:
		return "in progress"
	case model.Blocked:
		return "blocked"
	case model.Closed:
		return "closed"
	case model.Frozen, model.Other:
	}
	return strings.ReplaceAll(oneLine(is.Status), "_", " ")
}

// headerCell is one label of a column header: where its column starts, how
// wide it is (0 for as far as there is room) and its text.
type headerCell struct {
	at, width int
	text      string
}

// headerLine draws a dim header of w cells from cells in increasing order of
// position; a label is cut to its column.
func (r *Renderer) headerLine(w int, cells []headerCell) string {
	var b strings.Builder
	x := 0
	for _, c := range cells {
		if c.at < x || c.at >= w {
			continue
		}
		b.WriteString(strings.Repeat(" ", c.at-x))
		cw := c.width
		if cw == 0 {
			cw = w - c.at
		}
		text := ansi.Truncate(c.text, cw, "")
		b.WriteString(r.look.Paint(theme.Faint, text))
		x = c.at + ansi.StringWidth(text)
	}
	return r.look.Fit(b.String(), w)
}

// oneLine replaces C0 and C1 control characters so a title cannot break the layout.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || (r >= 0x7f && r <= 0x9f) {
			return ' '
		}
		return r
	}, s)
}
