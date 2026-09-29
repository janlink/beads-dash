package term_test

import (
	"testing"

	uv "github.com/charmbracelet/ultraviolet"

	"github.com/janlink/beads-dash/internal/term"
)

func TestIsColorSchemeEvent(t *testing.T) {
	if !term.IsColorSchemeEvent(uv.DarkColorSchemeEvent{}) || !term.IsColorSchemeEvent(uv.LightColorSchemeEvent{}) {
		t.Error("scheme events not recognised")
	}
	if term.IsColorSchemeEvent(uv.KeyPressEvent{}) || term.IsColorSchemeEvent("x") {
		t.Error("other messages recognised as scheme events")
	}
}
