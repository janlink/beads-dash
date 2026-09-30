package theme

import (
	"fmt"
	"math"
	"strconv"
	"testing"
)

func luminance(t testing.TB, hex string) float64 {
	t.Helper()
	if len(hex) != 7 || hex[0] != '#' {
		t.Fatalf("bad hex %q", hex)
	}
	var c [3]float64
	for i := range c {
		v, err := strconv.ParseUint(hex[1+2*i:3+2*i], 16, 8)
		if err != nil {
			t.Fatalf("bad hex %q: %v", hex, err)
		}
		f := float64(v) / 255
		if f <= 0.03928 {
			c[i] = f / 12.92
		} else {
			c[i] = math.Pow((f+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
}

func contrast(t testing.TB, fg, bg string) float64 {
	a, b := luminance(t, fg), luminance(t, bg)
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

const (
	barText    = 4.5
	barGraphic = 3.0
)

// defaultSoft lists the default theme's roles that sit below the graphic bar
// on purpose: the lowest priority recedes. Every other role meets the bar.
var defaultSoft = map[string]map[Role]float64{
	"selection": {Priority4: 2.0},
}

func barOf(theme, on string, r Role, bar float64) float64 {
	if theme != DefaultName {
		return bar
	}
	if soft, ok := defaultSoft[on][r]; ok {
		return soft
	}
	return bar
}

func TestContrast(t *testing.T) {
	textRungs := map[Role]float64{Strong: barText, Text: barText, Dim: barText, Faint: barGraphic}
	for _, th := range themes {
		for _, dark := range []bool{true, false} {
			name := fmt.Sprintf("%s/%s", th.Name, map[bool]string{true: "dark", false: "light"}[dark])
			t.Run(name, func(t *testing.T) {
				surface := th.Hex(dark, Surface)
				selection := th.Hex(dark, Selection)
				check := func(what, fg, bg string, bar float64) {
					if got := contrast(t, fg, bg); got < bar {
						t.Errorf("%s: %s on %s = %.2f, want >= %.1f", what, fg, bg, got, bar)
					}
				}
				for r, bar := range textRungs {
					check(r.String()+" on surface", th.Hex(dark, r), surface, bar)
				}
				for _, r := range []Role{Strong, Text, Dim, Faint} {
					check("lifted "+r.String()+" on selection", th.Hex(dark, lift(r)), selection, barText)
				}
				pure := "#ffffff"
				if dark {
					pure = "#000000"
				}
				for r, bar := range textRungs {
					check(r.String()+" on "+pure, th.Hex(dark, r), pure, bar)
				}
				check("rule on surface", th.Hex(dark, Rule), surface, 1.3)
				for _, r := range Roles() {
					if (r >= StatusOpen && r <= TypeOther) || r == Primary || r == Success || r == Error || r == Warning || r == Match {
						check(r.String(), th.Hex(dark, r), surface, barGraphic)
					}
				}
				for _, r := range []Role{Priority0, Priority1, Priority2, Priority3, Priority4, StatusClosed, TypeChore, TypeOther, TypeBug, TypeFeature, TypeTask, TypeEpic, TypeDecision, Warning, Success, StatusOpen, StatusBlocked, StatusFrozen, StatusOther} {
					check(r.String()+" on selection", th.Hex(dark, r), selection, barOf(th.Name, "selection", r, barGraphic))
				}
				check("border on surface", th.Hex(dark, Border), surface, 2.0)
				check("changed on surface", th.Hex(dark, Changed), surface, barGraphic)
				check("changed on selection", th.Hex(dark, Changed), selection, barGraphic)
			})
		}
	}
}

func TestANSI16LadderNeverWhite(t *testing.T) {
	for _, th := range themes {
		for _, r := range Ladder {
			if fg := th.ANSI16[r].FG; fg == 7 || fg == 15 {
				t.Errorf("%s: ladder role %s uses white (%d)", th.Name, r, fg)
			}
		}
	}
}

func TestThemeTablesComplete(t *testing.T) {
	if len(themes) != 5 {
		t.Fatalf("want 5 themes, got %d", len(themes))
	}
	for _, th := range themes {
		for _, r := range Roles() {
			for _, dark := range []bool{true, false} {
				if h := th.Hex(dark, r); len(h) != 7 {
					t.Errorf("%s dark=%v: role %s has no hex (%q)", th.Name, dark, r, h)
				}
			}
		}
	}
}
