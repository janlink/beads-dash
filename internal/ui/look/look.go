// Package look bundles a palette and glyph set with the styles derived from
// them, so drawing code does not rebuild a style per cell.
package look

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
)

// Look is what a frame is drawn with.
type Look struct {
	Palette theme.Palette
	Glyphs  theme.Glyphs

	styles   []lipgloss.Style
	selected []lipgloss.Style
}

// New resolves every role's style once.
func New(p theme.Palette, g theme.Glyphs) Look {
	roles := theme.Roles()
	l := Look{Palette: p, Glyphs: g, styles: make([]lipgloss.Style, len(roles)), selected: make([]lipgloss.Style, len(roles))}
	for _, r := range roles {
		l.styles[r] = p.Style(r)
		l.selected[r] = p.SelectedStyle(r)
	}
	return l
}

// Style is the style of a role.
func (l Look) Style(r theme.Role) lipgloss.Style { return l.styles[r] }

// Selected is the style of a role on the selected row.
func (l Look) Selected(r theme.Role) lipgloss.Style { return l.selected[r] }

// Paint renders s in a role; an empty string stays empty.
func (l Look) Paint(r theme.Role, s string) string {
	if s == "" {
		return ""
	}
	return l.styles[r].Render(s)
}

// PaintSel renders s in a role on the selected row.
func (l Look) PaintSel(r theme.Role, s string) string {
	if s == "" {
		return ""
	}
	return l.selected[r].Render(s)
}

// Fit truncates s to w cells with the glyph set's ellipsis and pads it with
// spaces to exactly w.
func (l Look) Fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, l.Glyphs.Ellipsis)
	}
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// StatusIndex maps a presentation status to the index the theme and glyph
// tables use (Open first, Other last).
func StatusIndex(p int) int { return (p + 5) % 6 }
