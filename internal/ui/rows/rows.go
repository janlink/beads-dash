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

// GutterWidth is the cells the gutter takes: current band, mark, change.
const GutterWidth = 3

const (
	maxIDWidth   = 16
	maxFactWidth = 14
	minTitle     = 20
)

// Row is one row to draw and its per-frame state.
type Row struct {
	ID      string
	Current bool
	Marked  bool
	Changed bool
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
	cache    map[cacheKey]string
	gutter   [2][2][2]string
}

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
	for cur := range 2 {
		for mark := range 2 {
			for change := range 2 {
				r.gutter[cur][mark][change] = cell(cur == 1, g.Band, theme.Primary, cur == 1) +
					cell(mark == 1, g.Mark, theme.Strong, cur == 1) +
					cell(change == 1, g.Change, theme.Changed, cur == 1)
			}
		}
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
	r.idW = 0
	if snap != nil {
		for _, id := range snap.IDs() {
			r.idW = max(r.idW, ansi.StringWidth(id))
		}
	}
	r.idW = min(r.idW, maxIDWidth)
}

// Gutter is the three gutter cells of a row.
func (r *Renderer) Gutter(row Row) string {
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	return r.gutter[b(row.Current)][b(row.Marked)][b(row.Changed)]
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
	return func(w int, sel bool) string { return r.render(w, id, sel) }
}

func (r *Renderer) render(w int, id string, sel bool) string {
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
	idText := r.look.Fit(id, r.idW)
	fact := ansi.Truncate(oneLine(is.Assignee), maxFactWidth, g.Ellipsis)

	fixed := 1 + ansi.StringWidth(g.Status[idx]) + 1 + 2 + 1 + r.idW + 1
	factW := 0
	if fact != "" && w-fixed-1-ansi.StringWidth(fact) >= minTitle {
		factW = ansi.StringWidth(fact)
	} else {
		fact = ""
	}
	titleW := w - fixed
	if factW > 0 {
		titleW -= factW + 1
	}
	titleRole := theme.Text
	if pres.Status == model.Closed {
		titleRole = theme.Dim
	}
	var b strings.Builder
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(paint(statusRole, g.Status[idx]))
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(paint(theme.PriorityRole(is.Priority), prio))
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(paint(theme.Dim, idText))
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(paint(titleRole, r.look.Fit(oneLine(is.Title), max(titleW, 0))))
	if factW > 0 {
		b.WriteString(paint(theme.Text, " "))
		b.WriteString(paint(theme.Dim, fact))
	}
	return b.String()
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
