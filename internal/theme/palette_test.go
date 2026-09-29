package theme

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var sgrRE = regexp.MustCompile("\x1b\\[([0-9;]*)m")

func sgrCodes(s string) []int {
	var out []int
	for _, m := range sgrRE.FindAllStringSubmatch(s, -1) {
		for _, f := range strings.Split(m[1], ";") {
			n, _ := strconv.Atoi(f)
			out = append(out, n)
		}
	}
	return out
}

func hasColourCode(s string) bool {
	for _, n := range sgrCodes(s) {
		if (n >= 30 && n <= 38) || (n >= 40 && n <= 48) || (n >= 90 && n <= 97) || (n >= 100 && n <= 107) {
			return true
		}
	}
	return false
}

func TestNoneDepthEmitsNoColour(t *testing.T) {
	for _, th := range themes {
		p := NewPalette(th, true, DepthNone)
		for _, r := range Roles() {
			out := p.Style(r).Render("x") + p.SelectedStyle(r).Render("x")
			if hasColourCode(out) {
				t.Errorf("%s/%s emits colour at none: %q", th.Name, r, out)
			}
		}
	}
}

func TestSelectionInverseAtLowDepth(t *testing.T) {
	th := Default()
	for _, depth := range []Depth{Depth16, DepthNone} {
		p := NewPalette(th, true, depth)
		out := p.SelectedStyle(Text).Render("x")
		if !slices.Contains(sgrCodes(out), 7) {
			t.Errorf("%s: selected style is not inverse: %q", depth, out)
		}
	}
}

func TestSelectionBackgroundAtColourDepth(t *testing.T) {
	th := Default()
	for _, depth := range []Depth{DepthTrueColor, Depth256} {
		p := NewPalette(th, true, depth)
		out := p.SelectedStyle(Dim).Render("x")
		if !strings.Contains(out, "48;2;49;50;68") {
			t.Errorf("%s: no selection background in %q", depth, out)
		}
		if slices.Contains(sgrCodes(out), 7) {
			t.Errorf("%s: selection must not be inverse: %q", depth, out)
		}
	}
}

func TestSelectedLaddersLiftedOneRung(t *testing.T) {
	th := Default()
	p := NewPalette(th, true, DepthTrueColor)
	got := p.SelectedStyle(Dim).Render("x")
	want := p.Style(Text).Background(p.Style(Selection).GetBackground()).Render("x")
	if got != want {
		t.Errorf("dim on selection = %q, want text on selection %q", got, want)
	}
}

func TestANSI16UsesIndexColours(t *testing.T) {
	p := NewPalette(Default(), true, Depth16)
	out := p.Style(StatusBlocked).Render("x")
	if !slices.Contains(sgrCodes(out), 31) {
		t.Errorf("blocked at 16 colours should be red: %q", out)
	}
	if strings.Contains(out, "38;2") {
		t.Errorf("16 colours must not emit truecolor: %q", out)
	}
}

func TestLookup(t *testing.T) {
	if th, ok := Lookup("Ocean"); !ok || th.Name != "ocean" {
		t.Errorf("Lookup(Ocean) = %v %v", th.Name, ok)
	}
	if _, ok := Lookup("nope"); ok {
		t.Error("Lookup(nope) succeeded")
	}
	if got := Names(); len(got) != 5 || got[0] != DefaultName {
		t.Errorf("Names() = %v", got)
	}
}

func TestParseDepthAndBackground(t *testing.T) {
	for _, s := range []string{"auto", "truecolor", "256", "16", "none"} {
		d, ok := ParseDepth(s)
		if !ok || d.String() != s {
			t.Errorf("ParseDepth(%q) = %v %v", s, d, ok)
		}
	}
	if _, ok := ParseDepth("8"); ok {
		t.Error("ParseDepth(8) succeeded")
	}
	if !BackgroundAuto.IsDark(true) || BackgroundAuto.IsDark(false) || !BackgroundDark.IsDark(false) || BackgroundLight.IsDark(true) {
		t.Error("Background.IsDark wrong")
	}
	if _, ok := ParseBackground("x"); ok {
		t.Error("ParseBackground(x) succeeded")
	}
}

func TestDetectDepthLeavesNoColorToCaller(t *testing.T) {
	env := []string{"TTY_FORCE=1", "TERM=xterm-256color", "COLORTERM=truecolor"}
	var sink strings.Builder
	if got := DetectDepth(&sink, env); got != DepthTrueColor {
		t.Fatalf("baseline = %s, want truecolor", got)
	}
	if got := DetectDepth(&sink, append(env, "NO_COLOR=1")); got != DepthTrueColor {
		t.Errorf("NO_COLOR must not change detection, got %s", got)
	}
}

func TestRoleMapping(t *testing.T) {
	if StatusRole(5) != StatusOther || StatusRole(-1) != StatusOpen || StatusRole(9) != StatusOther {
		t.Error("StatusRole clamp")
	}
	if PriorityRole(2) != Priority2 || PriorityRole(9) != Priority4 {
		t.Error("PriorityRole")
	}
	if TypeRole("bug") != TypeBug || TypeRole("spike") != TypeOther {
		t.Error("TypeRole")
	}
}
