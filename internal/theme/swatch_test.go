package theme_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/theme"
)

func swatch(name string, p theme.Palette, g theme.Glyphs) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s\n", name, map[bool]string{true: "dark", false: "light"}[p.Dark()], p.Depth())
	for _, r := range theme.Roles() {
		sample := "Sample text"
		switch {
		case r >= theme.StatusOpen && r <= theme.StatusOther:
			sample = g.Status[r-theme.StatusOpen] + " status"
		case r == theme.Changed:
			sample = g.Change + " changed"
		}
		fmt.Fprintf(&b, "%-20s %s\n", r, p.Style(r).Render(sample))
	}
	b.WriteString("selected row: ")
	for _, r := range theme.Ladder[:4] {
		b.WriteString(p.SelectedStyle(r).Render(" " + r.String() + " "))
	}
	b.WriteString(p.SelectedStyle(theme.Changed).Render(g.Change))
	b.WriteString("\n")
	return b.String()
}

func TestSwatchTrueColor(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th, _ := theme.Lookup(name)
			g := theme.GlyphsFor(theme.TierFancy)
			out := swatch(name, theme.NewPalette(th, true, theme.DepthTrueColor), g) + "\n" +
				swatch(name, theme.NewPalette(th, false, theme.DepthTrueColor), g)
			testgolden.Equal(t, out)
		})
	}
}

func TestSwatchNone(t *testing.T) {
	for _, name := range theme.Names() {
		t.Run(name, func(t *testing.T) {
			th, _ := theme.Lookup(name)
			testgolden.Equal(t, swatch(name, theme.NewPalette(th, true, theme.DepthNone), theme.GlyphsFor(theme.TierASCII)))
		})
	}
}
