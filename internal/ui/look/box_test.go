package look

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
)

func TestFrameFillsExactlyWByH(t *testing.T) {
	body := []string{"a line far too long to fit into a narrow panel", "short"}
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierSafe, theme.TierASCII} {
		l := testLook(tier)
		for w := 4; w <= 40; w++ {
			for h := 2; h <= 5; h++ {
				p := Panel{Title: "Needs attention", Aside: Word(theme.Dim, "+12"), Bottom: Word(theme.Dim, "e edit  x export")}
				out := l.Frame(p, w, h, body)
				if len(out) != h {
					t.Fatalf("tier %v %dx%d: %d rows", tier, w, h, len(out))
				}
				for i, row := range out {
					if got := ansi.StringWidth(row); got != w {
						t.Fatalf("tier %v %dx%d row %d: %d cells: %q", tier, w, h, i, got, ansi.Strip(row))
					}
				}
			}
		}
	}
}

func TestFrameSetsTitleAsideAndBottomIntoTheBorder(t *testing.T) {
	l := testLook(theme.TierFancy)
	p := Panel{Title: "Now", Aside: Word(theme.Dim, "+3"), Bottom: Word(theme.Dim, "Tab focus")}
	got := ansi.Strip(strings.Join(l.Frame(p, 24, 3, []string{"body"}), "\n"))
	want := "╭─ Now ─────────── +3 ─╮\n" +
		"│body                  │\n" +
		"╰────────── Tab focus ─╯"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestFrameDropsTheAsideBeforeTheTitle(t *testing.T) {
	l := testLook(theme.TierFancy)
	top := ansi.Strip(l.Frame(Panel{Title: "Needs attention", Aside: Word(theme.Dim, "+12")}, 22, 2, nil)[0])
	if strings.Contains(top, "+12") || !strings.Contains(top, "Needs attention") {
		t.Errorf("top border %q", top)
	}
}

func TestFrameASCIIUsesOnlyASCII(t *testing.T) {
	l := testLook(theme.TierASCII)
	for _, row := range l.Frame(Panel{Title: "Now", Focused: true}, 20, 3, []string{"x"}) {
		for _, r := range ansi.Strip(row) {
			if r > 0x7f {
				t.Fatalf("non-ASCII %q in %q", r, row)
			}
		}
	}
}
