package form_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/form"
	"github.com/janlink/beads-dash/internal/ui/look"
)

func testLook() look.Look {
	return look.New(theme.NewPalette(theme.Default(), true, theme.DepthNone), theme.GlyphsFor(theme.TierASCII))
}

func press(f *form.Form, k string) form.Event {
	ev, _ := f.Key(key(k))
	return ev
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	r := []rune(s)
	return tea.KeyPressMsg{Code: r[0], Text: s}
}

func typeInto(f *form.Form, s string) {
	for _, r := range s {
		f.Key(key(string(r)))
	}
}

func sample() *form.Form {
	return form.New(
		form.NewText("title", "Title", ""),
		form.NewChoice("prio", "Priority", []string{"P0", "P1", "P2"}, "P2"),
		form.NewTokens("labels", "Labels", []string{"a"}),
		form.NewLinks("blocked", "Blocked by", nil),
		form.NewArea("desc", "Description", "one\ntwo", 3),
		form.NewFold(),
		advanced(form.NewText("due", "Due", "")),
	)
}

func advanced(f *form.Field) *form.Field { f.Advanced = true; return f }

func TestEnterWalksFieldsAndSubmitsOnTheLast(t *testing.T) {
	f := sample()
	typeInto(f, "hi")
	if got := f.Field("title").Value(); got != "hi" {
		t.Fatalf("title %q", got)
	}
	for _, want := range []form.Event{form.Moved, form.Moved, form.Moved, form.Pick} {
		if got := press(f, "enter"); got != want {
			t.Fatalf("enter = %v, want %v at %s", got, want, f.Focused().Key)
		}
		if want == form.Pick {
			break
		}
	}
}

func TestChoiceCyclesAndJumpsByLetter(t *testing.T) {
	f := sample()
	f.FocusKey("prio")
	f.Key(key("right"))
	if v := f.Field("prio").Value(); v != "P0" {
		t.Errorf("right from P2 wraps to %q", v)
	}
	f.Key(key("P"))
	if v := f.Field("prio").Value(); v != "P1" {
		t.Errorf("letter jumps to next match, got %q", v)
	}
}

func TestAdvancedOpensWhenAFieldIsSet(t *testing.T) {
	closed := sample()
	if closed.Open {
		t.Error("advanced open without a value")
	}
	set := advanced(form.NewText("due", "Due", "+1d"))
	f := form.New(form.NewText("title", "Title", ""), form.NewFold(), set)
	if !f.Open || f.AdvancedSet() != 1 {
		t.Errorf("open %v set %d", f.Open, f.AdvancedSet())
	}
}

func TestFoldRowTogglesAndCountsSet(t *testing.T) {
	f := sample()
	f.FocusKey("advanced")
	f.Layout(testLook(), 60)
	lines, _, _ := f.View(testLook(), 60)
	if !strings.Contains(strings.Join(lines, "\n"), "> Advanced") {
		t.Errorf("collapsed row missing:\n%s", strings.Join(lines, "\n"))
	}
	f.Key(key("enter"))
	if !f.Open {
		t.Fatal("Enter on the row must open it")
	}
	f.Move(1)
	typeInto(f, "+1d")
	f.FocusKey("advanced")
	f.Key(key("enter"))
	f.Layout(testLook(), 60)
	lines, _, _ = f.View(testLook(), 60)
	if !strings.Contains(strings.Join(lines, "\n"), "> Advanced (1 set)") {
		t.Errorf("no count:\n%s", strings.Join(lines, "\n"))
	}
	if f.Focused().Key != "advanced" {
		t.Errorf("focus %s", f.Focused().Key)
	}
}

func TestMoveWrapsOverShownFields(t *testing.T) {
	f := sample()
	f.Move(-1)
	if f.Focused().Key != "advanced" {
		t.Errorf("wrap back landed on %s", f.Focused().Key)
	}
	f.Move(1)
	if f.Focused().Key != "title" {
		t.Errorf("wrap forward landed on %s", f.Focused().Key)
	}
}

func TestTextareaTakesEnterAndArrows(t *testing.T) {
	f := sample()
	f.FocusKey("desc")
	f.Key(key("enter"))
	typeInto(f, "x")
	if v := f.Field("desc").Value(); !strings.Contains(v, "\n") || !strings.Contains(v, "x") {
		t.Errorf("value %q", v)
	}
	if got := press(f, "up"); got == form.Moved || f.Focused().Key != "desc" {
		t.Error("up inside a textarea must not leave it")
	}
}

func TestChangedTracksSetsForLists(t *testing.T) {
	f := form.NewTokens("l", "Labels", []string{"a", "b"})
	f.Set("b, a")
	if f.Changed() {
		t.Error("order alone is no change")
	}
	f.Set("a c")
	if !f.Changed() {
		t.Error("a different set is a change")
	}
}

func TestBulletMarksChangedFields(t *testing.T) {
	f := sample()
	f.Track = true
	typeInto(f, "x")
	f.Layout(testLook(), 60)
	lines, _, _ := f.View(testLook(), 60)
	if !strings.HasPrefix(ansi.Strip(lines[0]), "* Title") {
		t.Errorf("no bullet: %q", lines[0])
	}
}

func TestSuggestionsCompleteOnRight(t *testing.T) {
	x := form.NewTokens("l", "Labels", nil)
	x.Suggest = func(p string) []string {
		var out []string
		for _, s := range []string{"backend", "bug"} {
			if strings.HasPrefix(s, p) {
				out = append(out, s)
			}
		}
		return out
	}
	f := form.New(x)
	typeInto(f, "fix ba")
	if s := x.Suggestions(); len(s) != 1 || s[0] != "backend" {
		t.Fatalf("suggestions %v", s)
	}
	f.Key(key("right"))
	if v := x.Value(); v != "fix backend" {
		t.Errorf("completed to %q", v)
	}
}

func TestLinksBackspaceDropsTheLast(t *testing.T) {
	f := form.New(form.NewLinks("b", "Blocked by", []string{"x-1", "x-2"}))
	f.Key(key("backspace"))
	if got := f.Field("b").List(); len(got) != 1 || got[0] != "x-1" {
		t.Errorf("list %v", got)
	}
}

func TestScrollKeepsTheSpanInView(t *testing.T) {
	cases := []struct{ prev, from, to, page, total, want int }{
		{0, 0, 2, 10, 8, 0},
		{0, 12, 14, 10, 30, 5},
		{8, 3, 4, 10, 30, 3},
		{0, 28, 30, 10, 30, 20},
	}
	for _, c := range cases {
		if got := form.Scroll(c.prev, c.from, c.to, c.page, c.total); got != c.want {
			t.Errorf("Scroll%v = %d", c, got)
		}
	}
}

func TestEditWithoutAnEditorReportsFalse(t *testing.T) {
	if _, ok := form.Edit(func(string) string { return "" }, nil, "k", "x"); ok {
		t.Error("Edit without an editor must report false")
	}
}

func TestEditTouchesNoFileUntilTheCommandRuns(t *testing.T) {
	dir := t.TempDir()
	for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(k, dir)
	}
	get := func(k string) string {
		if k == "EDITOR" {
			return "true"
		}
		return ""
	}
	cmd, ok := form.Edit(get, nil, "k", "draft")
	if !ok || cmd == nil {
		t.Fatal("no command")
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "bdash-*.md")); len(files) != 0 {
		t.Fatalf("files before the command ran: %v", files)
	}
	cmd()
	files, _ := filepath.Glob(filepath.Join(dir, "bdash-*.md"))
	if len(files) != 1 {
		t.Fatalf("files after the command ran: %v", files)
	}
	if data, _ := os.ReadFile(files[0]); string(data) != "draft" {
		t.Errorf("file holds %q", data)
	}
}

func TestReadStripsOneTrailingNewline(t *testing.T) {
	for in, want := range map[string]string{"a": "a", "a\n": "a", "a\n\n": "a\n", "a\r\n": "a"} {
		path := t.TempDir() + "/e.md"
		if err := os.WriteFile(path, []byte(in), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := form.Read(nil, "k", path, nil); got.Text != want {
			t.Errorf("%q read as %q, want %q", in, got.Text, want)
		}
	}
}

func TestReadReturnsAndRemovesTheFile(t *testing.T) {
	path := t.TempDir() + "/e.md"
	if err := os.WriteFile(path, []byte("line one\r\nline two\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := form.Read("o", "desc", path, nil)
	if got.Text != "line one\nline two\n" || got.Key != "desc" || got.Err != nil {
		t.Errorf("read %+v", got)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("temporary file left behind")
	}
}

func TestCheckTogglesWithSpaceAndDrawsItsState(t *testing.T) {
	c := form.NewCheck("comments", "Comments", false)
	f := form.New(c, form.NewText("path", "Path", ""))
	if c.On() || c.Value() != "off" || c.Changed() {
		t.Fatalf("fresh check: on %v value %q changed %v", c.On(), c.Value(), c.Changed())
	}
	if ev := press(f, "space"); ev != form.Edited || !c.On() || !c.Changed() {
		t.Fatalf("space: event %v on %v changed %v", ev, c.On(), c.Changed())
	}
	lines, _, _ := f.View(testLook(), 40)
	if got := ansi.Strip(strings.Join(lines, "\n")); !strings.Contains(got, "[x]") {
		t.Errorf("ticked check draws as %q", got)
	}
	if press(f, "enter"); f.Focused().Key != "path" {
		t.Errorf("enter on a check left the focus on %q", f.Focused().Key)
	}
	c.Set("off")
	if c.On() {
		t.Error("Set(off) left the check on")
	}
}
