package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sahilm/fuzzy"

	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/ui/keys"
)

func openPicker(t testing.TB, a *App) *picker {
	t.Helper()
	press(a, "ctrl+p")
	p, ok := a.topDialog().(*picker)
	if !ok {
		t.Fatalf("Ctrl+P opened %T", a.topDialog())
	}
	return p
}

func TestPickerJumpsAndPushesTheBackStack(t *testing.T) {
	a := treeApp(t, 100, 30)
	a.sess.SetCurrent("ws-7mt")
	p := openPicker(t, a)
	if a.context() != keys.Picker {
		t.Fatalf("context %v", a.context())
	}
	typeText(a, "pymt")
	if len(p.res) == 0 || p.list[p.res[0].Index].id != "ws-9qe" {
		t.Fatalf("fuzzy match on the title failed: %+v", p.res)
	}
	press(a, "enter")
	if len(a.dialogs) != 0 || a.sess.Current() != "ws-9qe" {
		t.Fatalf("dialogs %d current %q", len(a.dialogs), a.sess.Current())
	}
	press(a, "backspace")
	if a.sess.Current() != "ws-7mt" {
		t.Errorf("back returns to the issue the jump left: %q", a.sess.Current())
	}
}

func TestPickerMatchesTheID(t *testing.T) {
	a := treeApp(t, 100, 30)
	p := openPicker(t, a)
	typeText(a, "4k2.5")
	if got := p.list[p.res[0].Index].id; !strings.HasPrefix(got, "ws-4k2.5") {
		t.Errorf("best match %q", got)
	}
}

func TestPickerHidesClosedUntilToggled(t *testing.T) {
	a := treeApp(t, 100, 30)
	p := openPicker(t, a)
	typeText(a, "analytics")
	if len(p.res) != 0 {
		t.Fatalf("closed issue offered: %d", len(p.res))
	}
	press(a, "ctrl+t")
	if len(p.res) != 1 || p.list[p.res[0].Index].id != "ws-5ca" {
		t.Fatalf("closed issue missing after the toggle: %+v", p.res)
	}
	if !strings.Contains(screen(a), "closed shown") {
		t.Errorf("aside does not say closed are shown:\n%s", screen(a))
	}
}

func TestPickerEscClosesWithoutJumping(t *testing.T) {
	a := treeApp(t, 100, 30)
	a.sess.SetCurrent("ws-7mt")
	openPicker(t, a)
	typeText(a, "pay")
	press(a, "esc")
	if len(a.dialogs) != 0 || a.sess.Current() != "ws-7mt" {
		t.Errorf("dialogs %d current %q", len(a.dialogs), a.sess.Current())
	}
}

func TestPickerJumpOutsideTheScopeClearsIt(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, "/")
	typeText(a, "guest")
	press(a, "enter")
	openPicker(t, a)
	typeText(a, "9qe")
	press(a, "enter")
	if a.scope.Active() || a.sess.Current() != "ws-9qe" {
		t.Errorf("query %q current %q", a.scope.Query(), a.sess.Current())
	}
}

func TestPickerMultiSelectAndInvalidCandidates(t *testing.T) {
	a := treeApp(t, 100, 30)
	var got []string
	p := a.newPicker(pickerOpts{
		Title: "Pick",
		Multi: true,
		Valid: func(id string) (bool, string) {
			if id == "ws-9qe" {
				return false, "already a dependency"
			}
			return true, ""
		},
		Done: func(_ *App, ids []string) (tea.Cmd, bool) {
			got = ids
			return nil, true
		},
	})
	a.pushDialog(p)
	typeText(a, "ws-")
	pick := func(id string) {
		t.Helper()
		for i, m := range p.res {
			if p.list[m.Index].id == id {
				p.sel = i
				return
			}
		}
		t.Fatalf("%s not offered", id)
	}
	pick("ws-9qe")
	var row string
	for _, ln := range strings.Split(screen(a), "\n") {
		if strings.Contains(ln, "ws-9qe") {
			row = ln
		}
	}
	if i, j := strings.Index(row, "ws-9qe"), strings.Index(row, "already a dependency"); i < 0 || j < i {
		t.Errorf("an invalid candidate carries its reason after the ID: %q", row)
	}
	press(a, "tab")
	if len(p.marked) != 0 || p.note != "already a dependency" {
		t.Fatalf("an invalid candidate must not be marked: %v %q", p.marked, p.note)
	}
	press(a, "enter")
	if got != nil || len(a.dialogs) != 1 {
		t.Fatalf("Enter on an invalid candidate must refuse: %v", got)
	}
	if !strings.Contains(screen(a), "already a dependency") {
		t.Errorf("reason not shown:\n%s", screen(a))
	}
	pick("ws-7mt")
	press(a, "tab")
	pick("ws-2hz")
	press(a, "tab")
	if !slices.Equal(p.marked, []string{"ws-7mt", "ws-2hz"}) {
		t.Fatalf("marked %v", p.marked)
	}
	press(a, "enter")
	if !slices.Equal(got, []string{"ws-7mt", "ws-2hz"}) || len(a.dialogs) != 0 {
		t.Errorf("Done got %v, dialogs %d", got, len(a.dialogs))
	}
}

func TestPickerSingleModeHasNoMarkHint(t *testing.T) {
	a := treeApp(t, 100, 30)
	p := openPicker(t, a)
	for _, h := range p.hints() {
		if h.Key == "Tab" {
			t.Errorf("single-select picker hints %v", p.hints())
		}
	}
}

func TestPickerGoldens(t *testing.T) {
	for _, s := range [][2]int{{60, 16}, {80, 24}} {
		size := fmt.Sprintf("%dx%d", s[0], s[1])
		t.Run("empty "+size, func(t *testing.T) {
			a := treeApp(t, s[0], s[1])
			a.sess.SetCurrent("ws-4k2")
			openPicker(t, a)
			testgolden.Equal(t, screen(a))
		})
		t.Run("query "+size, func(t *testing.T) {
			a := treeApp(t, s[0], s[1])
			openPicker(t, a)
			typeText(a, "gst")
			press(a, "down")
			testgolden.Equal(t, screen(a))
		})
		t.Run("multi "+size, func(t *testing.T) {
			a := treeApp(t, s[0], s[1])
			a.pushDialog(a.newPicker(pickerOpts{
				Title: "Add dependency", Multi: true,
				Valid: func(id string) (bool, string) { return id != "ws-9qe", "already a dependency" },
				Done:  func(*App, []string) (tea.Cmd, bool) { return nil, true },
			}))
			press(a, "tab", "tab")
			testgolden.Equal(t, screen(a))
		})
	}
}

func TestRuneHitsConvertsFuzzyByteOffsets(t *testing.T) {
	src := pickerSource{{text: "ws-1 Größe ändern"}}
	res := fuzzy.FindFrom("änd", src)
	if len(res) != 1 {
		t.Fatalf("no match: %v", res)
	}
	hit := runeHits(src[0].text, res[0].MatchedIndexes)
	runes := []rune(src[0].text)
	var got []rune
	for i, r := range runes {
		if hit[i] {
			got = append(got, r)
		}
	}
	if string(got) != "änd" {
		t.Errorf("highlighted %q, want %q", string(got), "änd")
	}
}

func TestTailHitsFollowTheFrontCut(t *testing.T) {
	id := "beads-dash-www.32"
	hit := map[int]bool{0: true, 15: true, 16: true}
	got := tailHits(id, "…www.32", "…", hit)
	if len(got) != 2 || !got[5] || !got[6] {
		t.Errorf("hits %v, want the last two runes of the tail", got)
	}
	if same := tailHits(id, id, "…", hit); len(same) != 3 {
		t.Errorf("an uncut id keeps its hits: %v", same)
	}
}
