package dialog_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func backdrop(cols, rows int) string {
	line := strings.Repeat("x", cols)
	out := make([]string, rows)
	for i := range out {
		out[i] = line
	}
	return strings.Join(out, "\n")
}

func TestBoxIsExactlyTheRequestedSize(t *testing.T) {
	for _, tier := range []theme.Tier{theme.TierFancy, theme.TierASCII} {
		l := uitest.Look(theme.DepthTrueColor, tier, true)
		f := dialog.Frame{Title: "Title", Aside: "2 changed", Hints: []keys.Hint{{Key: "Enter", Desc: "apply"}}, Body: []string{"one", "two"}}
		for _, size := range [][2]int{{60, 16}, {76, 6}, {30, 5}} {
			box := dialog.Box(l, f, size[0], size[1])
			if len(box) != size[1] {
				t.Errorf("tier %v: %d lines, want %d", tier, len(box), size[1])
			}
			for _, b := range box {
				if w := ansi.StringWidth(b); w != size[0] {
					t.Errorf("tier %v: line %q is %d cells, want %d", tier, ansi.Strip(b), w, size[0])
				}
			}
		}
	}
}

func TestHintsDropFromTheEndThenAsideShortens(t *testing.T) {
	l := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	f := dialog.Frame{Title: "Appearance", Aside: "3 changed", Body: []string{"x"}, Hints: []keys.Hint{
		{Key: "Enter", Desc: "apply"}, {Key: "Esc", Desc: "revert"}, {Key: "Tab", Desc: "row"},
	}}
	wide := ansi.Strip(dialog.Box(l, f, 60, 3)[2])
	if !strings.Contains(wide, "Enter apply") || !strings.Contains(wide, "Tab row") {
		t.Errorf("wide bottom border = %q", wide)
	}
	narrow := ansi.Strip(dialog.Box(l, f, 26, 3)[2])
	if !strings.Contains(narrow, "Enter apply") || strings.Contains(narrow, "Tab row") {
		t.Errorf("narrow bottom border = %q", narrow)
	}
	if top := ansi.Strip(dialog.Box(l, f, 20, 3)[0]); strings.Contains(top, "changed") {
		t.Errorf("aside kept in a tight frame: %q", top)
	}
}

func TestBodyScrollsInsteadOfClipping(t *testing.T) {
	l := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	var body []string
	for i := range 20 {
		body = append(body, string(rune('a'+i)))
	}
	box := dialog.Box(l, dialog.Frame{Title: "T", Body: body}, 20, 7)
	got := ansi.Strip(strings.Join(box, "\n"))
	if !strings.Contains(got, "a ") || !strings.Contains(got, "16 more") {
		t.Errorf("box:\n%s", got)
	}
	box = dialog.Box(l, dialog.Frame{Title: "T", Body: body, Scroll: 100}, 20, 7)
	if got := ansi.Strip(strings.Join(box, "\n")); !strings.Contains(got, "t ") || strings.Contains(got, " more") {
		t.Errorf("scrolled to the end:\n%s", got)
	}
}

func TestOverlayCentresAndDims(t *testing.T) {
	l := uitest.Look(theme.DepthTrueColor, theme.TierASCII, true)
	f := dialog.Frame{Title: "T", Body: []string{"hi"}}
	out := dialog.Overlay(l, 100, 30, backdrop(100, 30), f)
	lines := strings.Split(out, "\n")
	if len(lines) != 30 {
		t.Fatalf("%d lines", len(lines))
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w != 100 {
			t.Errorf("line %d is %d cells", i, w)
		}
	}
	if !strings.Contains(lines[0], "\x1b[") {
		t.Error("backdrop not dimmed")
	}
	if !strings.Contains(ansi.Strip(lines[13]), "+- T ") {
		t.Error("dialog not vertically centred")
	}

	none := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	if out := dialog.Overlay(none, 100, 30, backdrop(100, 30), f); strings.Split(out, "\n")[0] != strings.Repeat("x", 100) {
		t.Error("backdrop touched at depth none")
	}
	keep := dialog.Frame{Title: "T", Body: []string{"hi"}, KeepBackdrop: true}
	if out := dialog.Overlay(l, 100, 30, backdrop(100, 30), keep); strings.Split(out, "\n")[0] != strings.Repeat("x", 100) {
		t.Error("KeepBackdrop dimmed the backdrop")
	}
}

func TestFullScreenBelow80x24(t *testing.T) {
	l := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	for _, size := range [][2]int{{79, 30}, {100, 23}, {60, 16}} {
		if !dialog.FullScreen(size[0], size[1]) {
			t.Errorf("%v is not full-screen", size)
		}
		out := dialog.Overlay(l, size[0], size[1], backdrop(size[0], size[1]), dialog.Frame{Title: "T", Body: []string{"hi"}})
		lines := strings.Split(out, "\n")
		if len(lines) != size[1] || ansi.StringWidth(lines[0]) != size[0] || strings.Contains(out, "xxx") {
			t.Errorf("%v: %d lines, backdrop visible: %v", size, len(lines), strings.Contains(out, "xxx"))
		}
	}
	if dialog.FullScreen(80, 24) {
		t.Error("80×24 must be an overlay")
	}
	if w, h := dialog.Size(200, 50, 5); w != 80 || h != 7 {
		t.Errorf("Size(200,50,5) = %d×%d", w, h)
	}
	if w, _ := dialog.Size(82, 30, 5); w != 78 {
		t.Errorf("width at 82 cols = %d, want cols-4", w)
	}
}
