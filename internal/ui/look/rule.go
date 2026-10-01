package look

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
)

// Seg is a run of text set into a rule, painted in its role.
type Seg struct {
	Role theme.Role
	Text string
}

// SegWidth is the cells segs take.
func SegWidth(segs []Seg) int {
	n := 0
	for _, s := range segs {
		n += ansi.StringWidth(s.Text)
	}
	return n
}

// Word is one entry for Words: a single run.
func Word(role theme.Role, text string) []Seg { return []Seg{{role, text}} }

// Words sets entries into a rule, one cell of line around each:
// "─ first ─ second ─". An entry may hold several runs.
func (l Look) Words(entries ...[]Seg) []Seg {
	rule := l.Glyphs.Rule
	out := []Seg{{theme.Rule, rule}}
	for _, e := range entries {
		out = append(out, Seg{theme.Rule, " "})
		out = append(out, e...)
		out = append(out, Seg{theme.Rule, " " + rule})
	}
	return out
}

// WordCost is the cells Words spends around an entry of w cells.
const WordCost = 3

// Rule draws left, then line, then right across exactly w cells. The line
// takes whole glyphs, so a rule glyph wider than one cell leaves an odd
// remainder blank. When the words do not fit, right goes first and left is
// cut at w.
func (l Look) Rule(w int, left, right []Seg) string {
	if w <= 0 {
		return ""
	}
	if SegWidth(left)+SegWidth(right) > w {
		right = nil
	}
	if lw := SegWidth(left); lw > w {
		left = clip(left, w, l.Glyphs.Ellipsis)
	}
	fill := w - SegWidth(left) - SegWidth(right)
	unit := max(ansi.StringWidth(l.Glyphs.Rule), 1)
	n := fill / unit
	var b strings.Builder
	l.write(&b, left)
	b.WriteString(l.Paint(theme.Rule, strings.Repeat(l.Glyphs.Rule, n)+strings.Repeat(" ", fill-n*unit)))
	l.write(&b, right)
	return b.String()
}

func (l Look) write(b *strings.Builder, segs []Seg) {
	for _, s := range segs {
		b.WriteString(l.Paint(s.Role, s.Text))
	}
}

func clip(segs []Seg, w int, ellipsis string) []Seg {
	var out []Seg
	for _, s := range segs {
		sw := ansi.StringWidth(s.Text)
		if sw <= w {
			out = append(out, s)
			w -= sw
			continue
		}
		out = append(out, Seg{s.Role, ansi.Truncate(s.Text, w, ellipsis)})
		break
	}
	return out
}

// FitID shortens an issue ID to w cells from the front, keeping the tail that
// tells IDs of one prefix apart ("…www.32"), and pads it to exactly w.
func (l Look) FitID(id string, w int) string {
	if w <= 0 {
		return ""
	}
	s := l.TruncID(id, w)
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// TruncID is FitID without the padding.
func (l Look) TruncID(id string, w int) string {
	total := ansi.StringWidth(id)
	if total <= w {
		return id
	}
	ew := ansi.StringWidth(l.Glyphs.Ellipsis)
	if w <= ew {
		return ansi.Truncate(l.Glyphs.Ellipsis, w, "")
	}
	return l.Glyphs.Ellipsis + ansi.TruncateLeft(id, total-(w-ew), "")
}
