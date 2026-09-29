package rows_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func line(r *rows.Renderer, w int, row rows.Row) string {
	return r.Line(w, row, "test", r.Standard(row.ID))
}

func newRenderer(depth theme.Depth, tier theme.Tier) (*rows.Renderer, []string) {
	snap, st := uitest.Sample()
	r := rows.New(uitest.Look(depth, tier, true))
	r.Bind(snap, st)
	return r, snap.IDs()
}

func TestLinesHaveExactWidth(t *testing.T) {
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierSafe, theme.TierASCII} {
		r, ids := newRenderer(theme.DepthTrueColor, tier)
		for _, w := range []int{20, 40, 57, 80, 200} {
			for _, id := range ids {
				for _, cur := range []bool{false, true} {
					if got := ansi.StringWidth(line(r, w, rows.Row{ID: id, Current: cur, Marked: cur, Changed: !cur})); got != w {
						t.Errorf("tier %v width %d id %s cur %v: line is %d cells", tier, w, id, cur, got)
					}
				}
			}
		}
	}
}

func TestChangeSlotNeverShiftsText(t *testing.T) {
	r, _ := newRenderer(theme.DepthNone, theme.TierFancy)
	plain := ansi.Strip(line(r, 80, rows.Row{ID: "ws-9qe"}))
	changed := ansi.Strip(line(r, 80, rows.Row{ID: "ws-9qe", Changed: true}))
	marked := ansi.Strip(line(r, 80, rows.Row{ID: "ws-9qe", Marked: true, Changed: true}))
	body := func(s string) string { return ansi.Cut(s, rows.GutterWidth, 80) }
	if body(plain) != body(changed) || body(plain) != body(marked) {
		t.Errorf("body moved:\n%q\n%q\n%q", plain, changed, marked)
	}
	if g := theme.GlyphsFor(theme.TierFancy); !strings.HasPrefix(changed, "  "+g.Change) {
		t.Errorf("gutter = %q", changed)
	}
}

func TestGutterComposesAllThreeCells(t *testing.T) {
	r, _ := newRenderer(theme.DepthNone, theme.TierASCII)
	got := ansi.Strip(r.Gutter(rows.Row{ID: "x", Current: true, Marked: true, Changed: true}))
	if got != "|#+" {
		t.Errorf("gutter = %q, want |#+", got)
	}
	if got := ansi.Strip(r.Gutter(rows.Row{ID: "x"})); got != "   " {
		t.Errorf("empty gutter = %q", got)
	}
}

func TestSelectedRowIsInverseWithoutColour(t *testing.T) {
	r, _ := newRenderer(theme.DepthNone, theme.TierASCII)
	line := line(r, 60, rows.Row{ID: "ws-9qe", Current: true, Changed: true})
	if !strings.Contains(line, "\x1b[7m") && !strings.Contains(line, ";7m") && !strings.Contains(line, "\x1b[1;7m") {
		t.Errorf("selected row has no reverse attribute: %q", line)
	}
	if strings.Contains(line, "38;") || strings.Contains(line, "48;") {
		t.Errorf("colour codes at depth none: %q", line)
	}
}

func TestChangeGlyphIsBoldInChangedRole(t *testing.T) {
	r, _ := newRenderer(theme.DepthTrueColor, theme.TierFancy)
	cell := r.Gutter(rows.Row{ID: "x", Changed: true})
	if !strings.Contains(cell, "\x1b[1;") && !strings.Contains(cell, "\x1b[1m") {
		t.Errorf("change glyph is not bold: %q", cell)
	}
}

func TestRowBodiesAreCachedPerSnapshot(t *testing.T) {
	snap, st := uitest.Big(500)
	r := rows.New(uitest.Look(theme.DepthTrueColor, theme.TierFancy, true))
	r.Bind(snap, st)
	id := snap.IDs()[3]
	a := line(r, 80, rows.Row{ID: id})
	if b := line(r, 80, rows.Row{ID: id, Changed: true}); ansi.Cut(ansi.Strip(a), rows.GutterWidth, 80) != ansi.Cut(ansi.Strip(b), rows.GutterWidth, 80) {
		t.Error("highlight state changed the cached body")
	}
	if n := testing.AllocsPerRun(50, func() { line(r, 80, rows.Row{ID: id}) }); n > 3 {
		t.Errorf("cached row allocates %v times", n)
	}
}

func TestSampleRowsGolden(t *testing.T) {
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierASCII} {
		t.Run(tier.String(), func(t *testing.T) {
			r, ids := newRenderer(theme.DepthNone, tier)
			var lines []string
			for i, id := range ids {
				lines = append(lines, ansi.Strip(line(r, 72, rows.Row{ID: id, Current: i == 1, Marked: i == 2 || i == 1, Changed: i == 3 || i == 1})))
			}
			testgolden.Equal(t, strings.Join(lines, "\n")+"\n")
		})
	}
}

func BenchmarkFrameOfRows(b *testing.B) {
	snap, st := uitest.Big(5000)
	r := rows.New(uitest.Look(theme.DepthTrueColor, theme.TierFancy, true))
	r.Bind(snap, st)
	ids := snap.IDs()[:46]
	b.ReportAllocs()
	for b.Loop() {
		for i, id := range ids {
			_ = line(r, 200, rows.Row{ID: id, Current: i == 5, Marked: i%7 == 0, Changed: i%9 == 0})
		}
	}
}

func TestBodyCacheIsKeyedByViewIDAndWidth(t *testing.T) {
	r, ids := newRenderer(theme.DepthNone, theme.TierASCII)
	calls := 0
	body := func(w int, _ bool) string {
		calls++
		return strings.Repeat("x", w)
	}
	draw := func(view string, w int, cur bool) {
		r.Line(w, rows.Row{ID: ids[0], Current: cur}, view, body)
	}
	draw("a", 40, false)
	draw("a", 40, false)
	if calls != 1 {
		t.Fatalf("same view, id and width drew %d times", calls)
	}
	draw("b", 40, false)
	draw("a", 30, false)
	if calls != 3 {
		t.Errorf("view and width must each miss the cache: %d calls", calls)
	}
	draw("a", 40, true)
	draw("a", 40, true)
	if calls != 5 {
		t.Errorf("the current row is drawn fresh every frame: %d calls", calls)
	}
}
