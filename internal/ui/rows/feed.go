package rows

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
)

const (
	feedAgeW   = 4
	feedKindW  = 16
	minFeedTit = 12
	// feedLabelCol is the line width from which the kind is spelled out
	// beside its glyph.
	feedLabelCol = 64
)

// FeedLine is the input of one activity-feed line.
type FeedLine struct {
	Event model.Event
	Now   time.Time
}

// KindGlyph is the glyph and role of an event kind. Kinds that name a status
// borrow its glyph, padded to the width of the status glyphs.
func KindGlyph(g theme.Glyphs, k model.Kind) (string, theme.Role) {
	ascii := g.Tier == theme.TierASCII
	pick := func(fancy, plain string) string {
		if ascii {
			return plain
		}
		return fancy
	}
	var s string
	role := theme.Dim
	switch k {
	case model.KindClaimed:
		s, role = g.Status[1], theme.StatusInProgress
	case model.KindClosed:
		s, role = g.Status[4], theme.StatusClosed
	case model.KindBecameBlocked:
		s, role = g.Status[2], theme.StatusBlocked
	case model.KindUnblocked, model.KindReopened:
		s, role = g.Status[0], theme.StatusOpen
	case model.KindBecameReady:
		s, role = pick("▶", ">"), theme.Success
	case model.KindCreated:
		s, role = "+", theme.Strong
	case model.KindUnassigned:
		s = "-"
	case model.KindStatusChanged:
		s = g.Arrow
	case model.KindPriorityChanged:
		s = pick("↕", "^")
	case model.KindDeleted:
		s = pick("×", "x")
	case model.KindEdited:
		s = "~"
	case model.KindCommented:
		s = "#"
	}
	if s == "" {
		s = "?"
	}
	w := ansi.StringWidth(g.Status[0])
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s, role
}

// Feed draws an activity-feed line: age, kind, ID, title, then the detail
// and the actor when they fit. Live lines are drawn per frame, not cached:
// the same issue can appear many times.
func (r *Renderer) Feed(l FeedLine) Body {
	return func(w int, sel bool) string { return r.feed(l, w, sel) }
}

func (r *Renderer) feed(l FeedLine, w int, sel bool) string {
	paint := r.look.Paint
	if sel {
		paint = r.look.PaintSel
	}
	g := r.look.Glyphs
	e := l.Event
	dim := e.Prefill
	role := func(rl theme.Role) theme.Role {
		if dim {
			return theme.Dim
		}
		return rl
	}
	title := e.Title
	if is, ok := r.snap.Issue(e.IssueID); ok {
		title = is.Title
	}
	glyph, glyphRole := KindGlyph(g, e.Kind)
	age := Age(l.Now, e.Time)
	labelled := w+GutterWidth >= feedLabelCol
	fixed := feedAgeW + 1 + ansi.StringWidth(glyph) + 1 + r.idW + 1
	if labelled {
		fixed += feedKindW + 1
	}
	var detail, actor string
	if e.Detail != "" {
		detail = " " + e.Detail
	}
	if e.Actor != "" && e.Actor != e.Detail {
		actor = " · " + e.Actor
	}
	tail := ""
	for _, c := range []string{detail + actor, actor} {
		if w-fixed-ansi.StringWidth(c) >= minFeedTit {
			tail = c
			break
		}
	}
	titleW := max(w-fixed-ansi.StringWidth(tail), 0)

	var b strings.Builder
	sp := paint(theme.Text, " ")
	b.WriteString(paint(theme.Faint, strings.Repeat(" ", max(feedAgeW-len(age), 0))+age))
	b.WriteString(sp)
	b.WriteString(paint(role(glyphRole), glyph))
	b.WriteString(sp)
	if labelled {
		b.WriteString(paint(role(theme.Text), r.look.Fit(e.Kind.Label(), feedKindW)))
		b.WriteString(sp)
	}
	b.WriteString(paint(theme.Dim, r.look.FitID(e.IssueID, r.idW)))
	b.WriteString(sp)
	b.WriteString(paint(role(theme.Text), r.look.Fit(oneLine(title), titleW)))
	if tail != "" {
		b.WriteString(paint(theme.Dim, oneLine(tail)))
	}
	return r.look.Fit(b.String(), w)
}
