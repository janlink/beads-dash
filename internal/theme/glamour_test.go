package theme

import (
	"strings"
	"testing"

	"charm.land/glamour/v2"
)

const sampleMarkdown = "# Title\n\nSome **bold** and `code` with a [link](https://example.com).\n\n- item one\n- [x] done\n\n> quoted\n\n```go\nfmt.Println(\"hi\")\n```\n"

func renderMarkdown(t *testing.T, p Palette, g Glyphs) string {
	t.Helper()
	r, err := glamour.NewTermRenderer(glamour.WithStyles(GlamourStyle(p, g)), glamour.WithWordWrap(60))
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render(sampleMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGlamourStyleTrueColorUsesRoleColours(t *testing.T) {
	th := Default()
	for _, dark := range []bool{true, false} {
		p := NewPalette(th, dark, DepthTrueColor)
		out := renderMarkdown(t, p, GlyphsFor(TierFancy))
		if !hasColourCode(out) {
			t.Errorf("dark=%v: no colour in %q", dark, out)
		}
	}
}

func TestGlamourStyleDiffersByVariant(t *testing.T) {
	g := GlyphsFor(TierFancy)
	dark := renderMarkdown(t, NewPalette(Default(), true, DepthTrueColor), g)
	light := renderMarkdown(t, NewPalette(Default(), false, DepthTrueColor), g)
	if dark == light {
		t.Error("dark and light render identically")
	}
}

func TestGlamourStyleDiffersByTheme(t *testing.T) {
	g := GlyphsFor(TierFancy)
	ocean, _ := Lookup("ocean")
	a := renderMarkdown(t, NewPalette(Default(), true, DepthTrueColor), g)
	b := renderMarkdown(t, NewPalette(ocean, true, DepthTrueColor), g)
	if a == b {
		t.Error("themes render identically")
	}
}

func TestGlamourStyleNoneHasNoColour(t *testing.T) {
	out := renderMarkdown(t, NewPalette(Default(), true, DepthNone), GlyphsFor(TierASCII))
	if hasColourCode(out) {
		t.Errorf("colour at none: %q", out)
	}
	if !strings.Contains(out, "[x]") || !strings.Contains(out, "* item one") {
		t.Errorf("ascii glyphs missing: %q", out)
	}
}

func TestGlamourStyle16UsesIndexColours(t *testing.T) {
	out := renderMarkdown(t, NewPalette(Default(), true, Depth16), GlyphsFor(TierFancy))
	if strings.Contains(out, "38;2;") || strings.Contains(out, "38;5;") {
		t.Errorf("16 colours must use basic codes: %q", out)
	}
	if !hasColourCode(out) {
		t.Errorf("no colour at 16: %q", out)
	}
}
