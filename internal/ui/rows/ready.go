package rows

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
)

const (
	readyTypeW   = 8
	readyWhoW    = 12
	readyAgeW    = 4
	readyReasonW = 22
	minReadyCols = 20
)

// ReadyCols says which optional columns of a Ready row are shown. The
// required ones are glyph, ID, priority and title.
type ReadyCols struct {
	Type, Assignee, Age, Reason bool
}

// ReadyColumns picks the columns for a row w cells wide, gutter included. As
// the width shrinks they go in this order: reason below 120, age below 100,
// type below 90, assignee below 80. A column also goes when it would leave
// the title less than 20 cells, in the same order.
func (r *Renderer) ReadyColumns(w int) ReadyCols {
	c := ReadyCols{Type: w >= 90, Assignee: w >= 80, Age: w >= 100, Reason: w >= 120}
	room := func() int { return w - GutterWidth - r.readyFixed(c) }
	if room() < minReadyCols {
		c.Reason = false
	}
	if room() < minReadyCols {
		c.Age = false
	}
	if room() < minReadyCols {
		c.Type = false
	}
	if room() < minReadyCols {
		c.Assignee = false
	}
	return c
}

func (r *Renderer) readyFixed(c ReadyCols) int {
	sw := ansi.StringWidth(r.look.Glyphs.Status[0])
	n := 1 + sw + 1 + r.idW + 1 + 2 + 1
	if c.Type {
		n += readyTypeW + 1
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

// Ready draws a row of the Ready view: glyph, ID, priority, type, title,
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
	fixed := r.readyFixed(cols)
	reason := ansi.Truncate(row.Reason, readyReasonW, g.Ellipsis)
	switch {
	case cols.Reason:
	case row.Pinned && reason != "" && w-fixed-1-ansi.StringWidth(reason) >= minReadyCols:
		fixed += ansi.StringWidth(reason) + 1
	default:
		reason = ""
	}
	titleW := max(w-fixed, 0)

	var b strings.Builder
	sp := paint(theme.Text, " ")
	b.WriteString(sp)
	b.WriteString(paint(statusRole, g.Status[idx]))
	b.WriteString(sp)
	b.WriteString(r.hl(paint, theme.Dim, r.look.Fit(is.ID, r.idW)))
	b.WriteString(sp)
	b.WriteString(paint(theme.PriorityRole(is.Priority), "P"+strconv.Itoa(min(max(is.Priority, 0), 9))))
	b.WriteString(sp)
	if cols.Type {
		b.WriteString(paint(theme.TypeRole(is.IssueType), r.look.Fit(oneLine(is.IssueType), readyTypeW)))
		b.WriteString(sp)
	}
	b.WriteString(r.hl(paint, theme.Text, r.look.Fit(oneLine(is.Title), titleW)))
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
	} else if reason != "" {
		b.WriteString(sp)
		b.WriteString(paint(theme.Dim, reason))
	}
	return r.look.Fit(b.String(), w)
}
