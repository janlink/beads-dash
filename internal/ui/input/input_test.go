package input

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func key(s string) tea.KeyPressMsg {
	switch s {
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "delete":
		return tea.KeyPressMsg{Code: tea.KeyDelete}
	case "ctrl+u", "ctrl+k", "ctrl+w", "ctrl+a", "ctrl+e":
		return tea.KeyPressMsg{Code: rune(s[len(s)-1]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func feed(f *Field, keys ...string) {
	for _, k := range keys {
		f.Update(key(k))
	}
}

func TestEditing(t *testing.T) {
	f := New("")
	feed(f, "a", "b", "c")
	if f.Text() != "abc" || f.Pos() != 3 {
		t.Fatalf("%q at %d", f.Text(), f.Pos())
	}
	feed(f, "left", "X")
	if f.Text() != "abXc" || f.Pos() != 3 {
		t.Fatalf("insert mid: %q at %d", f.Text(), f.Pos())
	}
	feed(f, "backspace")
	if f.Text() != "abc" || f.Pos() != 2 {
		t.Fatalf("backspace: %q at %d", f.Text(), f.Pos())
	}
	feed(f, "delete")
	if f.Text() != "ab" {
		t.Fatalf("delete: %q", f.Text())
	}
	feed(f, "home", "backspace")
	if f.Text() != "ab" || f.Pos() != 0 {
		t.Fatalf("backspace at start: %q at %d", f.Text(), f.Pos())
	}
	feed(f, "end")
	if f.Pos() != 2 {
		t.Fatalf("end: %d", f.Pos())
	}
}

func TestKillKeys(t *testing.T) {
	f := New("one two three")
	feed(f, "ctrl+w")
	if f.Text() != "one two " {
		t.Fatalf("ctrl+w: %q", f.Text())
	}
	feed(f, "ctrl+a", "right", "right", "right", "right", "ctrl+k")
	if f.Text() != "one " {
		t.Fatalf("ctrl+k: %q", f.Text())
	}
	feed(f, "ctrl+u")
	if f.Text() != "" || f.Pos() != 0 {
		t.Fatalf("ctrl+u: %q", f.Text())
	}
}

func TestUpdateReportsChanges(t *testing.T) {
	f := New("ab")
	if f.Update(key("left")) || f.Update(key("home")) {
		t.Error("cursor keys do not change the text")
	}
	if !f.Update(key("x")) {
		t.Error("typing changes the text")
	}
	if f.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt}) {
		t.Error("alt combinations are not text")
	}
}

func TestSetFlattensControlCharacters(t *testing.T) {
	f := New("a\nb\tc")
	if f.Text() != "a b c" {
		t.Errorf("%q", f.Text())
	}
	f.Update(tea.KeyPressMsg{Text: "x\ny"})
	if f.Text() != "a b cxy" {
		t.Errorf("paste: %q", f.Text())
	}
}

func TestViewIsExactlyWideAndKeepsTheCursorVisible(t *testing.T) {
	l := uitest.Look(theme.DepthNone, theme.TierASCII, true)
	f := New("the quick brown fox jumps")
	for _, w := range []int{1, 5, 12, 40} {
		got := ansi.Strip(f.View(l, theme.Text, w, true))
		if ansi.StringWidth(got) != w {
			t.Errorf("width %d: %q is %d wide", w, got, ansi.StringWidth(got))
		}
	}
	if got := ansi.Strip(f.View(l, theme.Text, 8, true)); got != "x jumps " {
		t.Errorf("cursor at the end scrolls: %q", got)
	}
	feed(f, "home")
	if got := ansi.Strip(f.View(l, theme.Text, 8, true)); got != "the quic" {
		t.Errorf("cursor at the start: %q", got)
	}
}
