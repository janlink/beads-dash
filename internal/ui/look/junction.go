package look

import (
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
)

// IsRuleAt reports whether cell x of line holds the rule glyph.
func (l Look) IsRuleAt(line string, x int) bool {
	return x >= 0 && ansi.Strip(ansi.Cut(line, x, x+1)) == l.Glyphs.Rule
}

// Junction sets glyph into cell x of a rule line, painted as the rule. The
// line is left alone where cell x or a neighbour holds a word, so that the
// junction never touches one.
func (l Look) Junction(line string, x int, glyph string) string {
	w := ansi.StringWidth(line)
	for _, c := range []int{x - 1, x, x + 1} {
		if c >= 0 && c < w && !l.IsRuleAt(line, c) {
			return line
		}
	}
	if x < 0 || x >= w {
		return line
	}
	return ansi.Cut(line, 0, x) + l.Paint(theme.Rule, glyph) + ansi.Cut(line, x+1, ansi.StringWidth(line))
}

// Divider is the cell of a vertical border between a block on the left and
// one on the right on the same row: the plain border, or the junction with
// the rules that run into it.
func (l Look) Divider(left, right string) string {
	g := l.Glyphs
	lr := l.IsRuleAt(left, ansi.StringWidth(left)-1)
	rr := l.IsRuleAt(right, 0)
	switch {
	case lr && rr:
		return l.Paint(theme.Rule, g.RuleCross)
	case lr:
		return l.Paint(theme.Rule, g.RuleLeft)
	case rr:
		return l.Paint(theme.Rule, g.RuleRight)
	}
	return l.Paint(theme.Rule, firstCell(g.Vertical))
}

func firstCell(s string) string { return ansi.Cut(s, 0, 1) }
