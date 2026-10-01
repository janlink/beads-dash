package rows

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
)

const (
	readyWhoW    = 12
	readyAgeW    = 4
	readyReasonW = 22
	minReadyCols = 20
	// readyIDRef is the ID width the columns are chosen for; a wider ID takes
	// the room left over.
	readyIDRef = 16
)

// ReadyCols says which optional columns of a Ready row are shown. The
// required ones are glyph, ID, priority and title.
type ReadyCols struct {
	Type, Assignee, Age, Reason bool
}

// ReadyColumns picks the columns for a row w cells wide, gutter included. As
// the width shrinks they go in this order: reason below 120, age below 100,
// assignee below 80. A column also goes when it would leave the title less
// than 20 cells, in the same order, the type last.
func (r *Renderer) ReadyColumns(w int) ReadyCols {
	c := ReadyCols{Type: r.typeW > 0, Assignee: w >= 80, Age: w >= 100, Reason: w >= 120}
	idW := min(r.idW, readyIDRef)
	room := func() int { return w - GutterWidth - r.readyFixed(c, idW) }
	if room() < minReadyCols {
		c.Reason = false
	}
	if room() < minReadyCols {
		c.Age = false
	}
	if room() < minReadyCols {
		c.Assignee = false
	}
	if room() < minReadyCols {
		c.Type = false
	}
	return c
}

// readyFixed is the width of the row besides title and ID column, with the ID
// column counted at idW.
func (r *Renderer) readyFixed(c ReadyCols, idW int) int {
	sw := ansi.StringWidth(r.look.Glyphs.Status[0])
	n := 1 + sw + 1 + idW + 1 + 1 + 2
	if c.Type {
		n += r.typeW + 1
	}
	if c.Assignee {
		n += readyWhoW + 1
	}
	if c.Age {
		n += readyAgeW + 1
	}
	if c.Reason {
		n += readyReasonW + 1
	}
	return n
}

// ReadyRow is the per-row input of a Ready row.
type ReadyRow struct {
	ID   string
	Cols ReadyCols
	Now  time.Time
	// Reason is the dimmed text of the last column; Pinned shows it at every
	// width, for rows that name their blocker.
	Reason string
	Pinned bool
}

// Ready draws a row of the Ready view: glyph, ID, type, title, priority,
// assignee, age and reason.
func (r *Renderer) Ready(row ReadyRow) Body {
	return func(w int, sel bool) string { return r.ready(row, w, sel) }
}

func (r *Renderer) ready(row ReadyRow, w int, sel bool) string {
	paint := r.look.Paint
	if sel {
		paint = r.look.PaintSel
	}
	is, ok := r.snap.Issue(row.ID)
	if !ok {
		return paint(theme.Faint, r.look.Fit(row.ID, w))
	}
	g := r.look.Glyphs
	pres := r.snap.Present(row.ID, r.statuses)
	idx := look.StatusIndex(int(pres.Status))
	statusRole := theme.StatusRole(idx)
	if pres.BlockedMarker {
		statusRole = theme.StatusBlocked
	}
	cols := row.Cols
	fixed := r.readyFixed(cols, 0)
	idW := r.idCol(w - fixed - minReadyCols)
	fixed += idW
	reason := ansi.Truncate(row.Reason, readyReasonW, g.Ellipsis)
	titleW := max(w-fixed, 0)
	inline := ""
	if !cols.Reason && row.Pinned && reason != "" && titleW-1-ansi.StringWidth(reason) >= minReadyCols {
		inline = reason
		titleW -= ansi.StringWidth(reason) + 1
	}
	dim := pres.Status == model.Closed
	titleRole := theme.Text
	if dim {
		titleRole = theme.Dim
	}
	typeCol := 0
	if cols.Type {
		typeCol = r.typeW + 1
	}

	var b strings.Builder
	sp := paint(theme.Text, " ")
	b.WriteString(sp)
	b.WriteString(paint(statusRole, g.Status[idx]+" "))
	b.WriteString(r.hl(paint, theme.Dim, r.look.FitID(is.ID, idW)))
	b.WriteString(sp)
	b.WriteString(r.typeCell(paint, is, typeCol, dim))
	b.WriteString(r.hl(paint, titleRole, r.look.Fit(oneLine(is.Title), titleW)))
	if inline != "" {
		b.WriteString(sp)
		b.WriteString(paint(theme.Dim, inline))
	}
	b.WriteString(sp)
	b.WriteString(paint(theme.PriorityRole(is.Priority), "P"+strconv.Itoa(min(max(is.Priority, 0), 9))))
	if cols.Assignee {
		b.WriteString(sp)
		b.WriteString(paint(theme.Dim, r.look.Fit(ansi.Truncate(oneLine(is.Assignee), readyWhoW, g.Ellipsis), readyWhoW)))
	}
	if cols.Age {
		age := Age(row.Now, is.CreatedAt)
		b.WriteString(sp)
		b.WriteString(paint(theme.Faint, strings.Repeat(" ", max(readyAgeW-len(age), 0))+age))
	}
	if cols.Reason {
		b.WriteString(sp)
		b.WriteString(paint(theme.Dim, r.look.Fit(reason, readyReasonW)))
	}
	return r.look.Fit(b.String(), w)
}

// ReadyHeader is the dim column header line of the Ready view, w cells wide
// including the gutter.
func (r *Renderer) ReadyHeader(w int, cols ReadyCols) string {
	bw := w - GutterWidth
	if bw <= 0 {
		return strings.Repeat(" ", max(w, 0))
	}
	sw := ansi.StringWidth(r.look.Glyphs.Status[0])
	fixed := r.readyFixed(cols, 0)
	idW := r.idCol(bw - fixed - minReadyCols)
	at := 1 + sw + 1
	cells := []headerCell{{at, idW, "ID"}}
	at += idW + 1
	if cols.Type {
		cells = append(cells, headerCell{at, r.typeW, "TYPE"})
		at += r.typeW + 1
	}
	cells = append(cells, headerCell{at, 0, "TITLE"})
	at = bw - 2
	if cols.Reason {
		at -= readyReasonW + 1
	}
	if cols.Age {
		at -= readyAgeW + 1
	}
	if cols.Assignee {
		at -= readyWhoW + 1
	}
	cells = append(cells, headerCell{at, 2, "PR"})
	at += 3
	if cols.Assignee {
		cells = append(cells, headerCell{at, readyWhoW, "ASSIGNEE"})
		at += readyWhoW + 1
	}
	if cols.Age {
		cells = append(cells, headerCell{at, readyAgeW, "AGE"})
		at += readyAgeW + 1
	}
	if cols.Reason {
		cells = append(cells, headerCell{at, readyReasonW, "REASON"})
	}
	return strings.Repeat(" ", GutterWidth) + r.headerLine(bw, cells)
}
