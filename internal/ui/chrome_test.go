package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/ui/detail"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

var chromeSizes = [][2]int{{60, 16}, {80, 24}, {120, 40}, {200, 50}}

func (f flavour) rule() string {
	if f.glyphs == "ascii" {
		return "-"
	}
	return "─"
}

func cellsOf(s string) []string {
	var out []string
	for _, r := range ansi.Strip(s) {
		out = append(out, string(r))
	}
	return out
}

func TestChromeRowsFillTheScreen(t *testing.T) {
	for _, f := range []flavour{plain, truecolor} {
		for _, s := range chromeSizes {
			t.Run(fmt.Sprintf("%s/%dx%d", f.glyphs, s[0], s[1]), func(t *testing.T) {
				for _, view := range []string{"tree", "kanban", "ready"} {
					a := viewApp(t, f, s[0], s[1], view, true)
					ls := strings.Split(ansi.Strip(a.View().Content), "\n")
					if len(ls) != s[1] {
						t.Fatalf("%s: %d rows, want %d", view, len(ls), s[1])
					}
					for i, l := range ls {
						if w := ansi.StringWidth(l); w != s[0] {
							t.Fatalf("%s: row %d is %d cells, want %d: %q", view, i, w, s[0], l)
						}
					}
					if rule := ls[s[1]-2]; !strings.HasPrefix(rule, f.rule()) || strings.Contains(rule, "help") {
						t.Errorf("%s: footer rule %q", view, rule)
					}
					if head := ls[0]; !strings.HasPrefix(head, f.rule()+" ") {
						t.Errorf("%s: header %q is not a rule", view, head)
					}
				}
			})
		}
	}
}

func TestChromeJunctionsUnderSidePanel(t *testing.T) {
	for _, tc := range []struct {
		f         flavour
		down, up  string
		vertical  string
		ruleGlyph string
	}{
		{truecolor, "┬", "┴", "│", "─"},
		{plain, "+", "+", "|", "-"},
	} {
		t.Run(tc.f.glyphs, func(t *testing.T) {
			a := viewApp(t, tc.f, 200, 50, "tree", true)
			press(a, "enter")
			d := a.dock()
			if d.Frame != detail.Side {
				t.Fatalf("frame %v, want side", d.Frame)
			}
			ls := strings.Split(ansi.Strip(a.View().Content), "\n")
			col := 200 - d.W
			header, footer := cellsOf(ls[0]), cellsOf(ls[48])
			if header[col] != tc.down {
				t.Errorf("%s header junction %q at %d, want %q: %q", tc.f.glyphs, header[col], col, tc.down, ls[0])
			}
			if footer[col] != tc.up {
				t.Errorf("%s footer junction %q at %d, want %q: %q", tc.f.glyphs, footer[col], col, tc.up, ls[48])
			}
			if edge := cellsOf(ls[1])[col]; edge != tc.vertical {
				t.Errorf("%s panel edge %q at %d, want %q", tc.f.glyphs, edge, col, tc.vertical)
			}
			if strings.Count(ls[0], tc.down) != 1 && tc.f.glyphs != "ascii" {
				t.Errorf("header forks %d times: %q", strings.Count(ls[0], tc.down), ls[0])
			}
			if !strings.Contains(ls[0], "live 12:00:00") || !strings.Contains(ls[0], "Tree") || !strings.Contains(ls[48], "2 Tree") {
				t.Errorf("tabs or live marker lost beside the panel: %q %q", ls[0], ls[48])
			}
			testgolden.Equal(t, strings.Join(ls[:2], "\n")+"\n...\n"+strings.Join(ls[48:], "\n"))
		})
	}
}

func TestChromeNoJunctionWithoutSidePanel(t *testing.T) {
	for _, s := range [][2]int{{60, 16}, {80, 24}, {120, 40}, {199, 50}} {
		a := viewApp(t, truecolor, s[0], s[1], "tree", true)
		ls := strings.Split(ansi.Strip(a.View().Content), "\n")
		for _, row := range []string{ls[0], ls[s[1]-2]} {
			if strings.ContainsAny(row, "┬┴") {
				t.Errorf("%dx%d: junction without a side panel: %q", s[0], s[1], row)
			}
		}
	}
}

func TestChromeHeaderDropsPartsInOrder(t *testing.T) {
	a := viewApp(t, plain, 120, 40, "tree", false)
	full := cellsOf(lines(a)[0])
	if !strings.Contains(strings.Join(full, ""), "live") {
		t.Fatal("wide header lacks the live marker")
	}
	for cols := 60; cols <= 130; cols++ {
		a := viewApp(t, plain, cols, 24, "tree", false)
		head := lines(a)[0]
		if w := ansi.StringWidth(head); w != cols {
			t.Fatalf("%d cols: header is %d cells: %q", cols, w, head)
		}
		if !strings.Contains(head, "Tree") || !strings.Contains(head, "12:00:00") {
			t.Fatalf("%d cols: view name or live marker dropped: %q", cols, head)
		}
	}
}

func TestChromeBarsKeepTheRowCount(t *testing.T) {
	for _, s := range chromeSizes {
		for _, key := range []string{"/", "f", ":"} {
			t.Run(fmt.Sprintf("%dx%d/%s", s[0], s[1], key), func(t *testing.T) {
				a := viewApp(t, plain, s[0], s[1], "tree", true)
				press(a, key)
				ls := lines(a)
				if len(ls) != s[1] {
					t.Fatalf("%d rows, want %d", len(ls), s[1])
				}
				if a.bodyHeight() < 1 {
					t.Fatalf("body height %d", a.bodyHeight())
				}
			})
		}
	}
}

func TestMinimumSizeKeepsABody(t *testing.T) {
	a := viewApp(t, plain, MinCols, MinRows, "tree", false)
	if h := a.bodyHeight(); h != MinRows-chromeRows {
		t.Errorf("body height %d at the minimum size, want %d", h, MinRows-chromeRows)
	}
	for _, key := range []string{"/", "f", ":"} {
		a := viewApp(t, plain, MinCols, MinRows, "tree", false)
		press(a, key)
		if a.bodyHeight() < 1 || len(lines(a)) != MinRows {
			t.Errorf("bar %q: body %d, %d rows", key, a.bodyHeight(), len(lines(a)))
		}
	}
	small := viewApp(t, plain, MinCols, MinRows-1, "tree", false)
	if !strings.Contains(screen(small), "too small") {
		t.Errorf("below the minimum: %q", screen(small))
	}
}

func TestNoticeReplacesTheHintsRow(t *testing.T) {
	a := viewApp(t, plain, 100, 30, "tree", false)
	before := a.bodyHeight()
	a.notices = append(a.notices, screens.Notice{Text: "hello toast"})
	ls := lines(a)
	if !strings.Contains(ls[29], "hello toast") || strings.Contains(ls[29], "help") {
		t.Errorf("notice row %q", ls[29])
	}
	if !strings.HasPrefix(ls[28], "- ") || !strings.Contains(ls[28], "2 Tree") {
		t.Errorf("rule above the notice: %q", ls[28])
	}
	if a.bodyHeight() != before {
		t.Errorf("a notice changed the body height %d -> %d", before, a.bodyHeight())
	}
}

func TestFooterHintsRow(t *testing.T) {
	for _, tc := range []struct {
		f    flavour
		want string
	}{
		{truecolor, " / search  f filter  ↵ details  : cmd  ? help"},
		{plain, " / search  f filter  Enter details  : cmd  ? help"},
	} {
		ls := lines(viewApp(t, tc.f, 100, 30, "tree", false))
		if row := ls[29]; !strings.HasPrefix(row, tc.want+" ") || strings.Contains(row, "frozen") {
			t.Errorf("%s hints row %q", tc.f.glyphs, row)
		}
	}
}

func TestFooterLegendFromWideTerminals(t *testing.T) {
	a := viewApp(t, truecolor, 120, 30, "tree", false)
	row := lines(a)[29]
	if !strings.HasSuffix(strings.TrimSpace(row), "○ open  ◐ in progress  ● blocked  ✓ closed  ▫ frozen") {
		t.Errorf("legend missing: %q", row)
	}
	if !strings.HasPrefix(row, " / search  f filter  ↵ details  : cmd  ? help") {
		t.Errorf("hints lost beside the legend: %q", row)
	}
	if row := lines(viewApp(t, truecolor, 119, 30, "tree", false))[29]; strings.Contains(row, "frozen") {
		t.Errorf("legend below 120 columns: %q", row)
	}
	if row := lines(viewApp(t, truecolor, 120, 30, "memories", false))[29]; strings.Contains(row, "frozen") {
		t.Errorf("legend in Memories: %q", row)
	}
}

func TestFooterHintsDropInOrder(t *testing.T) {
	var prev []string
	for cols := 60; cols >= 12; cols-- {
		hs, drop, _ := viewApp(t, plain, 100, 30, "tree", false).listHints()
		var words []string
		for _, h := range dropToFit(hs, cols-2, drop) {
			words = append(words, h.Desc)
		}
		if prev != nil && len(words) > len(prev) {
			t.Fatalf("%d cols kept more hints than wider: %v vs %v", cols, words, prev)
		}
		prev = words
	}
	hs, drop, _ := viewApp(t, plain, 100, 30, "tree", false).listHints()
	var kept []string
	for _, h := range dropToFit(hs, 20, drop) {
		kept = append(kept, h.Desc)
	}
	if got := strings.Join(kept, ","); got != "search,help" {
		t.Errorf("kept %q, want search,help", got)
	}
}

func TestFooterHintsKeepMemoriesOwnKeys(t *testing.T) {
	row := lines(viewApp(t, plain, 100, 30, "memories", false))[29]
	for _, want := range []string{"/ search", "f filter", "n new", "e edit", "d forget", ": cmd", "? help"} {
		if !strings.Contains(row, want) {
			t.Errorf("Memories hints lack %q: %q", want, row)
		}
	}
	if strings.Contains(row, "details") {
		t.Errorf("Memories hints offer details: %q", row)
	}
}

func TestHelpListsTheHintsTheFooterLeftOut(t *testing.T) {
	a := viewApp(t, plain, 120, 50, "tree", false)
	press(a, "?")
	out := screen(a)
	for _, want := range []string{"refresh", "appearance", "switch view", "mark"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
}

func TestFooterTabsShrinkBeforeTheNoteLeaves(t *testing.T) {
	var a *App
	var sawNumbers bool
	for cols := 100; cols >= 60; cols-- {
		a = viewApp(t, plain, cols, 30, "tree", false)
		a.sess.ToggleMark("ws-4k2")
		rule := lines(a)[28]
		named := strings.Contains(rule, "Kanban")
		marked := strings.Contains(rule, "marked")
		if !named && !sawNumbers {
			sawNumbers = true
			if !marked {
				t.Errorf("%d cols: tabs shrank but the chip left: %q", cols, rule)
			}
		}
		if named && !marked {
			t.Errorf("%d cols: chip left while the tabs were named: %q", cols, rule)
		}
	}
	if !sawNumbers {
		t.Error("tabs never shrank to numbers")
	}
}

func TestFooterTabClicksFollowTheRule(t *testing.T) {
	for _, cols := range []int{60, 100} {
		for i := range ViewNames {
			a := viewApp(t, plain, cols, 30, "tree", false)
			_, _, spans := a.footerLayout(cols)
			send(a, tea.MouseClickMsg{Button: tea.MouseLeft, X: spans[i].from, Y: 1 + a.bodyHeight()})
			if a.slot != i {
				t.Errorf("%d cols: click on tab %d landed on slot %d", cols, i+1, a.slot)
			}
		}
	}
}

func TestSidePanelFooterCarriesThePanelKeys(t *testing.T) {
	a := viewApp(t, truecolor, 200, 30, "tree", true)
	if rule := lines(a)[28]; !strings.Contains(rule, "┴─ e edit  x export  Tab focus ─") {
		t.Errorf("list-focused panel keys: %q", rule)
	}
	press(a, "enter")
	if rule := lines(a)[28]; !strings.Contains(rule, "┴─ ]/[ section  m markdown  x export  Esc back ─") {
		t.Errorf("panel-focused panel keys: %q", rule)
	}
}

func TestBottomPanelIsSeparatedFromTheFooter(t *testing.T) {
	a := viewApp(t, plain, 120, 40, "tree", true)
	press(a, "enter")
	if a.dock().Frame != detail.Bottom {
		t.Fatalf("frame %v, want bottom", a.dock().Frame)
	}
	ls := lines(a)
	if strings.TrimSpace(ls[len(ls)-3]) != "" {
		t.Errorf("no blank row above the footer rule: %q", ls[len(ls)-3])
	}
	if !strings.HasPrefix(ls[len(ls)-2], "- ") {
		t.Errorf("footer rule %q", ls[len(ls)-2])
	}
}

func TestSidePanelFocusedHintsDoNotRepeatTheRuleKeys(t *testing.T) {
	a := viewApp(t, truecolor, 200, 30, "tree", true)
	press(a, "enter")
	ls := lines(a)
	if !strings.Contains(ls[28], "]/[ section") || strings.Contains(ls[29], "section") {
		t.Errorf("rule %q hints %q", ls[28], ls[29])
	}
}

func TestBottomGapRowIsNotThePanel(t *testing.T) {
	a := viewApp(t, plain, 120, 40, "tree", true)
	body := a.bodyHeight()
	if !a.overPanel(0, body-bottomGap-1) || a.overPanel(0, body-bottomGap) {
		t.Errorf("panel hit-test at the gap row: last %v gap %v", a.overPanel(0, body-bottomGap-1), a.overPanel(0, body-bottomGap))
	}
}
