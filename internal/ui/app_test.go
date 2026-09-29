package ui_test

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/ui"
)

func TestViewGolden(t *testing.T) {
	testgolden.Equal(t, ui.New().View().Content)
}

func TestQuits(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"q", tea.KeyPressMsg{Code: 'q', Text: "q"}},
		{"ctrl+c", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := teatest.NewTestModel(t, ui.New(), teatest.WithInitialTermSize(80, 24))
			teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
				return bytes.Contains(b, []byte("press q to quit"))
			}, teatest.WithDuration(5*time.Second))
			tm.Send(tt.key)
			if _, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(ui.App); !ok {
				t.Error("final model is not ui.App")
			}
		})
	}
}
