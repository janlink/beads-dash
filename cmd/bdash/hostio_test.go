package main

import (
	"testing"

	"github.com/janlink/beads-dash/internal/proc"
	"github.com/janlink/beads-dash/internal/ui"
)

func TestWireHostFillsClipboardAndNotifier(t *testing.T) {
	var o ui.Options
	wireHost(&o, func(string) string { return "" }, "off", proc.NewFake())
	if o.Clipboard == nil || o.Notifier == nil {
		t.Errorf("Clipboard %v, Notifier %v", o.Clipboard, o.Notifier)
	}
}
