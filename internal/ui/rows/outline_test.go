package rows_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func TestOutlineLinesHaveExactWidthAtEveryOffset(t *testing.T) {
	snap, st := uitest.Graph()
	out := model.BuildOutline(snap, true)
	focus := model.BuildFocus(snap, st, "gr-z", model.FocusOptions{Depth: 3, Children: true})
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierSafe, theme.TierASCII} {
		r := rows.New(uitest.Look(theme.DepthTrueColor, tier, true))
		r.Bind(snap, st)
		for _, all := range [][]model.OutlineRow{out.Rows, focus} {
			for i := range all {
				for _, w := range []int{1, 10, 40, 100} {
					for _, x := range []int{0, 2, 5, 40} {
						for _, sel := range []bool{false, true} {
							s := r.Outline(all, i, w, rows.OutlineStyle{Narrow: w < 40, Sel: sel, XOff: x, Dim: i%2 == 0})
							if got := ansi.StringWidth(s); got != w {
								t.Fatalf("tier %v row %d w %d x %d: %d cells", tier, i, w, x, got)
							}
						}
					}
				}
			}
		}
	}
}

func TestOutlineGuidesAndFold(t *testing.T) {
	snap, st := uitest.Graph()
	r := rows.New(uitest.Look(theme.DepthNone, theme.TierASCII, true))
	r.Bind(snap, st)
	o := model.BuildOutline(snap, false)
	var d int
	for i, row := range o.Rows {
		if row.ID == "gr-d" && row.Kind == model.OutNode {
			d = i
		}
	}
	wide := ansi.Strip(r.Outline(o.Rows, d, 60, rows.OutlineStyle{}))
	narrow := ansi.Strip(r.Outline(o.Rows, d, 60, rows.OutlineStyle{Narrow: true}))
	if !strings.HasPrefix(wide, "|  `- ") || !strings.HasPrefix(narrow, "| `-") {
		t.Errorf("guides: wide %q narrow %q", wide, narrow)
	}
	folded := ansi.Strip(r.Outline(o.Rows, 1, 60, rows.OutlineStyle{Hidden: 3}))
	if !strings.Contains(folded, "+3") {
		t.Errorf("no fold count in %q", folded)
	}
	scrolled := ansi.Strip(r.Outline(o.Rows, d, 60, rows.OutlineStyle{XOff: 3}))
	if !strings.HasPrefix(scrolled, "`- ") || strings.Contains(scrolled, "|") {
		t.Errorf("scrolled row %q", scrolled)
	}
}
