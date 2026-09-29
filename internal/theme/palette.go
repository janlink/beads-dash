package theme

import (
	"slices"
	"strconv"

	"charm.land/lipgloss/v2"
)

// Palette resolves a theme for one variant and colour depth into styles.
type Palette struct {
	theme Theme
	dark  bool
	depth Depth
}

// NewPalette resolves t. depth must be concrete; DepthAuto is treated as
// truecolor.
func NewPalette(t Theme, dark bool, depth Depth) Palette {
	if depth == DepthAuto {
		depth = DepthTrueColor
	}
	return Palette{theme: t, dark: dark, depth: depth}
}

func (p Palette) Theme() Theme { return p.theme }
func (p Palette) Dark() bool   { return p.dark }
func (p Palette) Depth() Depth { return p.depth }

// hasHex reports whether the palette draws with the hex tables.
func (p Palette) hasHex() bool { return p.depth == DepthTrueColor || p.depth == Depth256 }

// inverseSelection reports whether selection is drawn inverse.
func (p Palette) inverseSelection() bool { return !p.hasHex() }

// Hex returns the role's hex colour for this palette's variant.
func (p Palette) Hex(r Role) string { return p.theme.Hex(p.dark, r) }

// parts resolves a role to the colour strings and attributes it draws with at
// this palette's depth. fg and bg are empty when the depth has no colour for it.
func (p Palette) parts(r Role) (fg, bg string, a Attr) {
	if !p.hasHex() {
		a = p.theme.ANSI16[r]
		if p.depth == Depth16 && a.FG != DefaultFG {
			fg = strconv.Itoa(a.FG)
		}
		a.FG = DefaultFG
		return fg, "", a
	}
	a = hexAttrs[r]
	if r == Selection || r == Surface {
		return "", p.Hex(r), a
	}
	return p.Hex(r), "", a
}

// Style returns the style for a role.
func (p Palette) Style(r Role) lipgloss.Style {
	fg, bg, a := p.parts(r)
	s := lipgloss.NewStyle()
	if fg != "" {
		s = s.Foreground(lipgloss.Color(fg))
	}
	if bg != "" {
		s = s.Background(lipgloss.Color(bg))
	}
	return s.Bold(a.Bold).Faint(a.Faint).Underline(a.Underline).Reverse(a.Reverse)
}

// SelectedStyle returns the style for a role drawn on the selected row: at
// truecolor/256 the selection background with the text ladder lifted one rung,
// at 16/none the role's style inverse.
func (p Palette) SelectedStyle(r Role) lipgloss.Style {
	if p.inverseSelection() {
		return p.Style(r).Reverse(true)
	}
	return p.Style(lift(r)).Background(lipgloss.Color(p.Hex(Selection)))
}

// lift moves a text ladder role one rung more prominent.
func lift(r Role) Role {
	if i := slices.Index(Ladder, r); i > 0 {
		return Ladder[i-1]
	}
	return r
}

var hexAttrs = func() (a [roleCount]Attr) {
	for _, r := range []Role{Strong, Primary, Error, StatusBlocked, Priority0, Priority1, Changed} {
		a[r].Bold = true
	}
	a[Match].Underline = true
	return a
}()
