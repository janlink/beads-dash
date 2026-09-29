package theme

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestGlyphSetsHaveNoDuplicates(t *testing.T) {
	for _, tier := range []Tier{TierFancy, TierSafe, TierASCII} {
		seen := map[string]string{}
		for _, s := range GlyphsFor(tier).ExclusiveSlots() {
			g := strings.TrimSpace(s.Glyph)
			if g == "" {
				t.Errorf("%s: slot %s is empty", tier, s.Name)
			}
			if prev, dup := seen[g]; dup {
				t.Errorf("%s: %q used by both %s and %s", tier, g, prev, s.Name)
			}
			seen[g] = s.Name
		}
	}
}

func TestChangeGlyphIsNotAStatusGlyph(t *testing.T) {
	want := map[Tier]string{TierFancy: "✱", TierSafe: "*", TierASCII: "+"}
	for tier, glyph := range want {
		g := GlyphsFor(tier)
		if g.Change != glyph {
			t.Errorf("%s change glyph = %q, want %q", tier, g.Change, glyph)
		}
		for _, s := range g.Status {
			if strings.TrimSpace(s) == glyph {
				t.Errorf("%s: status glyph %q equals the change glyph", tier, s)
			}
		}
	}
}

func TestSlotGroupsPaddedToWidest(t *testing.T) {
	for _, tier := range []Tier{TierFancy, TierSafe, TierASCII} {
		g := GlyphsFor(tier)
		groups := map[string][]string{
			"status": g.Status[:],
			"fold":   {g.FoldOpen, g.FoldClosed},
			"tree":   {g.Branch, g.LastBranch, g.Vertical},
			"bar":    {g.BarFull, g.BarEmpty},
			"check":  {g.Checked, g.Unchecked},
		}
		for name, members := range groups {
			w := ansi.StringWidth(members[0])
			for _, m := range members {
				if got := ansi.StringWidth(m); got != w {
					t.Errorf("%s/%s: %q has width %d, group width %d", tier, name, m, got, w)
				}
			}
		}
	}
	if got := GlyphsFor(TierASCII).Vertical; got != "| " {
		t.Errorf("ascii vertical = %q, want padded %q", got, "| ")
	}
}

func TestASCIITierHasNoAmbiguousGlyphs(t *testing.T) {
	if GlyphsFor(TierASCII).HasAmbiguous() {
		t.Error("ascii set contains ambiguous glyphs")
	}
	if !GlyphsFor(TierFancy).HasAmbiguous() {
		t.Error("fancy set should contain ambiguous glyphs")
	}
}

func TestNeutralGlyphsNotMarkedAmbiguous(t *testing.T) {
	for _, r := range "✓◊∙░▸▾▪▫❯✱↵" {
		if IsAmbiguous(r) {
			t.Errorf("%q is neutral width", r)
		}
	}
}

func TestResolveTier(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	tests := []struct {
		name string
		in   Tier
		env  map[string]string
		want Tier
	}{
		{"explicit wins over locale", TierSafe, map[string]string{"LANG": "C"}, TierSafe},
		{"no locale", TierAuto, nil, TierFancy},
		{"utf8", TierAuto, map[string]string{"LANG": "en_US.UTF-8"}, TierFancy},
		{"utf8 no dash", TierAuto, map[string]string{"LANG": "de_DE.utf8"}, TierFancy},
		{"modifier", TierAuto, map[string]string{"LANG": "de_DE.UTF-8@euro"}, TierFancy},
		{"latin1", TierAuto, map[string]string{"LANG": "en_US.ISO-8859-1"}, TierASCII},
		{"C locale", TierAuto, map[string]string{"LANG": "C"}, TierASCII},
		{"POSIX", TierAuto, map[string]string{"LC_ALL": "POSIX"}, TierASCII},
		{"LC_ALL beats LANG", TierAuto, map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "C"}, TierFancy},
		{"LC_CTYPE beats LANG", TierAuto, map[string]string{"LC_CTYPE": "en_US.ISO-8859-1", "LANG": "en_US.UTF-8"}, TierASCII},
		{"no charset", TierAuto, map[string]string{"LANG": "en_US"}, TierFancy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveTier(tt.in, env(tt.env)); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestFallbackForAmbiguous(t *testing.T) {
	if got, notice := FallbackForAmbiguous(TierFancy, true); got != TierASCII || notice == "" {
		t.Errorf("wide fancy: got %s %q", got, notice)
	}
	if got, notice := FallbackForAmbiguous(TierSafe, true); got != TierASCII || notice == "" {
		t.Errorf("wide safe: got %s %q", got, notice)
	}
	if got, notice := FallbackForAmbiguous(TierASCII, true); got != TierASCII || notice != "" {
		t.Errorf("wide ascii: got %s %q", got, notice)
	}
	if got, notice := FallbackForAmbiguous(TierFancy, false); got != TierFancy || notice != "" {
		t.Errorf("narrow: got %s %q", got, notice)
	}
}
