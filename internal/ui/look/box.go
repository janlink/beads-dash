package look

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
)

// Panel is what a framed panel shows around its body.
type Panel struct {
	Title string
	// Aside sits right of the title, e.g. "+12" or "field 3/8".
	Aside []Seg
	// Bottom is set into the bottom border, e.g. the panel's key hints.
	Bottom []Seg
	// Focused panels have the keys: their border and title take the primary
	// colour.
	Focused bool
}

// PanelInner is the body width of a panel w cells wide.
func PanelInner(w int) int { return max(w-2, 0) }

type corners struct{ tl, tr, bl, br, h, v string }

func (l Look) corners() corners {
	if l.Glyphs.Tier == theme.TierASCII {
		return corners{"+", "+", "+", "+", "-", "|"}
	}
	return corners{"╭", "╮", "╰", "╯", "─", "│"}
}

// Frame draws p around body as exactly w by h cells: the title in the top
// border, body lines cut or padded to PanelInner(w) cells, blank lines below
// them. It returns nil when w or h leaves no room for a body.
func (l Look) Frame(p Panel, w, h int, body []string) []string {
	if w < 4 || h < 2 {
		return nil
	}
	c := l.corners()
	edge := theme.Border
	if p.Focused {
		edge = theme.Primary
	}
	iw := PanelInner(w)
	v := l.Paint(edge, c.v)
	out := make([]string, 0, h)
	out = append(out, l.frameTop(p, c, edge, w))
	for i := range h - 2 {
		s := ""
		if i < len(body) {
			s = body[i]
		}
		out = append(out, v+l.Fit(s, iw)+v)
	}
	return append(out, l.frameBottom(p.Bottom, c, edge, w))
}

func (l Look) frameTop(p Panel, c corners, edge theme.Role, w int) string {
	room := w - 4
	title := ""
	if p.Title != "" {
		title = " " + p.Title + " "
	}
	aside := p.Aside
	if aw := SegWidth(aside); aw > 0 && ansi.StringWidth(title)+aw+2+1 > room {
		aside = nil
	}
	title = ansi.Truncate(title, room, l.Glyphs.Ellipsis)
	tw := ansi.StringWidth(title)
	style := l.Style(theme.Strong)
	if p.Focused {
		style = l.Style(theme.Primary)
	}
	var b strings.Builder
	b.WriteString(l.Paint(edge, c.tl+c.h))
	if title != "" {
		b.WriteString(style.Bold(true).Render(title))
	}
	asideW := 0
	if SegWidth(aside) > 0 {
		asideW = SegWidth(aside) + 2
	}
	b.WriteString(l.Paint(edge, strings.Repeat(c.h, max(room-tw-asideW, 0))))
	if asideW > 0 {
		b.WriteString(" ")
		l.write(&b, aside)
		b.WriteString(" ")
	}
	b.WriteString(l.Paint(edge, c.h+c.tr))
	return b.String()
}

func (l Look) frameBottom(segs []Seg, c corners, edge theme.Role, w int) string {
	room := w - 4
	sw := SegWidth(segs)
	if sw > 0 && sw+2 > room {
		segs = clip(segs, max(room-2, 0), l.Glyphs.Ellipsis)
		sw = SegWidth(segs)
	}
	var b strings.Builder
	b.WriteString(l.Paint(edge, c.bl+c.h))
	if sw > 0 {
		b.WriteString(l.Paint(edge, strings.Repeat(c.h, max(room-sw-2, 0))))
		b.WriteString(" ")
		l.write(&b, segs)
		b.WriteString(" ")
	} else {
		b.WriteString(l.Paint(edge, strings.Repeat(c.h, room)))
	}
	b.WriteString(l.Paint(edge, c.h+c.br))
	return b.String()
}
