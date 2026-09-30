package look

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
)

func testLook(t theme.Tier) Look {
	return New(theme.NewPalette(theme.Default(), true, theme.DepthNone), theme.GlyphsFor(t))
}

func TestRuleFillsExactlyW(t *testing.T) {
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierSafe, theme.TierASCII} {
		l := testLook(tier)
		for w := 1; w <= 40; w++ {
			row := l.Rule(w, l.Words(Word(theme.Strong, "Now")), l.Words(Word(theme.Dim, "live")))
			if got := ansi.StringWidth(row); got != w {
				t.Fatalf("tier %v w=%d: %d cells: %q", tier, w, got, row)
			}
		}
	}
}

func TestRuleSetsWordsIntoLine(t *testing.T) {
	l := testLook(theme.TierFancy)
	got := l.Rule(24, l.Words(Word(theme.Strong, "Now")), l.Words(Word(theme.Dim, "live")))
	if want := "─ Now ─────────── live ─"; ansi.Strip(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRuleJunctions(t *testing.T) {
	tests := []struct {
		tier        theme.Tier
		down, up, h string
	}{
		{theme.TierFancy, "┬", "┴", "─"},
		{theme.TierSafe, "┬", "┴", "─"},
		{theme.TierASCII, "+", "+", "-"},
	}
	for _, tt := range tests {
		l := testLook(tt.tier)
		if got := l.Rule(8, []Seg{l.Tee(true)}, nil); ansi.Strip(got) != tt.down+strings.Repeat(tt.h, 7) {
			t.Errorf("tier %v down: %q", tt.tier, got)
		}
		if got := l.Rule(8, []Seg{l.Tee(false)}, nil); ansi.Strip(got) != tt.up+strings.Repeat(tt.h, 7) {
			t.Errorf("tier %v up: %q", tt.tier, got)
		}
	}
}

func TestRuleASCIIUsesOnlyASCII(t *testing.T) {
	l := testLook(theme.TierASCII)
	row := l.Rule(30, append([]Seg{l.Tee(true)}, l.Words(Word(theme.Strong, "Now"))...), l.Words(Word(theme.Dim, "live")))
	for _, r := range row {
		if r > 0x7f {
			t.Fatalf("non-ASCII %q in %q", r, row)
		}
	}
}

func TestRuleNeverOverflows(t *testing.T) {
	l := testLook(theme.TierFancy)
	left := l.Words(Word(theme.Strong, "a very long heading indeed"))
	right := l.Words(Word(theme.Dim, "right"))
	for w := 1; w < 40; w++ {
		if got := ansi.StringWidth(l.Rule(w, left, right)); got != w {
			t.Errorf("w=%d: %d cells", w, got)
		}
	}
	if got := l.Rule(12, left, right); strings.Contains(got, "right") {
		t.Errorf("right kept although it did not fit: %q", got)
	}
	if got := l.Rule(0, left, right); got != "" {
		t.Errorf("zero width: %q", got)
	}
}

func TestRuleAmbiguousWideGlyph(t *testing.T) {
	l := testLook(theme.TierFancy)
	l.Glyphs.Rule = "世"
	for w := 2; w <= 20; w++ {
		row := l.Rule(w, []Seg{{theme.Strong, "ab"}}, nil)
		if got := ansi.StringWidth(row); got != w {
			t.Fatalf("w=%d: %d cells: %q", w, got, row)
		}
	}
	if got := l.Rule(7, []Seg{{theme.Strong, "ab"}}, nil); ansi.Strip(got) != "ab世世 " {
		t.Errorf("odd remainder is not left blank: %q", got)
	}
}

func TestFitIDKeepsTail(t *testing.T) {
	l := testLook(theme.TierFancy)
	tests := []struct {
		id   string
		w    int
		want string
	}{
		{"beads-dash-www.32", 17, "beads-dash-www.32"},
		{"beads-dash-www.32", 16, "…ads-dash-www.32"},
		{"beads-dash-www.32", 7, "…www.32"},
		{"beads-dash-www.3", 8, "…h-www.3"},
		{"bd-1", 8, "bd-1    "},
	}
	for _, tt := range tests {
		got := l.FitID(tt.id, tt.w)
		if ansi.StringWidth(got) != tt.w {
			t.Errorf("FitID(%q, %d) = %q, %d cells", tt.id, tt.w, got, ansi.StringWidth(got))
		}
		if got != tt.want {
			t.Errorf("FitID(%q, %d) = %q, want %q", tt.id, tt.w, got, tt.want)
		}
	}
}

func TestFitIDASCIIEllipsis(t *testing.T) {
	l := testLook(theme.TierASCII)
	if got := l.TruncID("beads-dash-www.32", 10); got != "...-www.32" {
		t.Errorf("got %q", got)
	}
	if got := l.TruncID("beads-dash-www.32", 2); ansi.StringWidth(got) > 2 {
		t.Errorf("%q overflows", got)
	}
}

func TestTruncIDDistinguishesSiblings(t *testing.T) {
	l := testLook(theme.TierFancy)
	a, b := l.TruncID("beads-dash-www.31", 10), l.TruncID("beads-dash-www.32", 10)
	if a == b {
		t.Errorf("%q == %q", a, b)
	}
}
