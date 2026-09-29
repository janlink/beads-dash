package dialog_test

import (
	"testing"

	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/keys"
)

func open() *dialog.Appearance {
	return dialog.NewAppearance(dialog.Choices{Theme: "default", Background: "auto", Glyphs: "auto"}, nil)
}

func TestAppearanceCyclesValuesAndPreviews(t *testing.T) {
	a := open()
	if out := a.Handle(keys.Next); out != dialog.Preview || a.Choices().Theme != "ocean" {
		t.Errorf("Next = %v theme %q", out, a.Choices().Theme)
	}
	if out := a.Handle(keys.Prev); out != dialog.Preview || a.Choices().Theme != "default" {
		t.Errorf("Prev = %v theme %q", out, a.Choices().Theme)
	}
	a.Handle(keys.Prev)
	if a.Choices().Theme != "monochrome" {
		t.Errorf("theme wraps to %q, want monochrome", a.Choices().Theme)
	}
}

func TestAppearanceRowsAndBackgroundRow(t *testing.T) {
	a := open()
	a.Handle(keys.NavDown)
	a.Handle(keys.Next)
	if got := a.Choices().Background; got != "dark" {
		t.Errorf("background = %q, want dark (auto|dark|light order)", got)
	}
	a.Handle(keys.NavDown)
	a.Handle(keys.Next)
	if got := a.Choices().Glyphs; got != "fancy" {
		t.Errorf("glyphs = %q", got)
	}
	a.Handle(keys.NavDown)
	a.Handle(keys.Next)
	if a.Choices().Theme != "ocean" {
		t.Error("rows do not wrap")
	}
}

func TestAppearanceApplyAndCancel(t *testing.T) {
	a := open()
	a.Handle(keys.Next)
	got := a.Choices().Changed(a.Original())
	if len(got) != 1 || got[config.KeyTheme] != "ocean" {
		t.Errorf("changed = %v", got)
	}
	if a.Handle(keys.Apply) != dialog.Apply || a.Handle(keys.Close) != dialog.Cancel {
		t.Error("Enter and Esc outcomes")
	}
	if a.Original().Theme != "default" {
		t.Error("original choices changed")
	}
}
