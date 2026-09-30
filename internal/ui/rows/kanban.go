package rows

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
)

const (
	kanbanFactW = 16
	kanbanBarW  = 6
	// kanbanIDCol is the card width from which the ID is shown.
	kanbanIDCol = 44
	// minKanbanTitle cells stay for the title before the fact is dropped.
	minKanbanTitle = 10
)

// BlockedMark is the glyph of a blocked fact, one cell wide.
func BlockedMark(g theme.Glyphs) string {
	if g.Tier == theme.TierASCII {
		return "!"
	}
	return "⊘"
}

// Kanban draws a card: priority, the ID when the card is wide enough, the
// title and one fact on the right. The fact is the first that applies: the
// raw status of an Other issue, the blocker count, progress,
// assignee, labels.
func (r *Renderer) Kanban(id string) Body {
	return func(w int, sel bool) string { return r.card(id, w, sel) }
}

func (r *Renderer) card(id string, w int, sel bool) string {
	paint := r.look.Paint
	if sel {
		paint = r.look.PaintSel
	}
	is, ok := r.snap.Issue(id)
	if !ok {
		return paint(theme.Faint, r.look.Fit(id, w))
	}
	role, fact := r.cardFact(is)
	fact = ansi.Truncate(fact, kanbanFactW, r.look.Glyphs.Ellipsis)
	fixed := 1 + 2 + 1
	withID := w+GutterWidth >= kanbanIDCol
	if withID {
		fixed += r.idW + 1
	}
	factW := ansi.StringWidth(fact)
	if factW > 0 && w-fixed-factW-1 < minKanbanTitle {
		fact, factW = "", 0
	}
	titleW := max(w-fixed, 0)
	if factW > 0 {
		titleW -= factW + 1
	}
	var b strings.Builder
	sp := paint(theme.Text, " ")
	b.WriteString(sp)
	b.WriteString(paint(theme.PriorityRole(is.Priority), "P"+strconv.Itoa(min(max(is.Priority, 0), 9))))
	b.WriteString(sp)
	if withID {
		b.WriteString(r.hl(paint, theme.Dim, r.look.FitID(id, r.idW)))
		b.WriteString(sp)
	}
	b.WriteString(r.hl(paint, theme.Text, r.look.Fit(oneLine(is.Title), titleW)))
	if factW > 0 {
		b.WriteString(sp)
		b.WriteString(paint(role, fact))
	}
	return r.look.Fit(b.String(), w)
}

func (r *Renderer) cardFact(is *model.Issue) (theme.Role, string) {
	g := r.look.Glyphs
	pres := r.snap.Present(is.ID, r.statuses)
	blockers := len(r.snap.BlockedBy(is.ID))
	switch {
	case pres.Status == model.Other:
		return theme.Warning, oneLine(is.Status)
	case blockers > 0 && pres.Status == model.InProgress:
		return theme.Error, BlockedMark(g) + " blocked " + strconv.Itoa(blockers)
	case blockers > 0:
		return theme.Error, BlockedMark(g) + " " + strconv.Itoa(blockers)
	}
	if p, ok := r.snap.Progress(is.ID, r.statuses); ok {
		full := p.Direct.Closed * kanbanBarW / p.Direct.Total
		bar := strings.Repeat(g.BarFull, full) + strings.Repeat(g.BarEmpty, kanbanBarW-full)
		return theme.Dim, bar + " " + strconv.Itoa(p.Direct.Closed) + "/" + strconv.Itoa(p.Direct.Total)
	}
	switch {
	case is.Assignee != "":
		return theme.Text, "@" + MidCut(oneLine(is.Assignee), kanbanFactW-1, g.Ellipsis)
	case len(is.Labels) > 0:
		s := "#" + oneLine(is.Labels[0])
		if n := len(is.Labels) - 1; n > 0 {
			s += " +" + strconv.Itoa(n)
		}
		return theme.Dim, s
	}
	return theme.Dim, ""
}

// IDWidth is the width the ID column of the snapshot's rows takes.
func (r *Renderer) IDWidth() int { return r.idW }
