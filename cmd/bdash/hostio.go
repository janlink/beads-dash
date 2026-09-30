package main

import (
	"os"
	"runtime"

	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/host"
	"github.com/janlink/beads-dash/internal/notify"
	"github.com/janlink/beads-dash/internal/proc"
	"github.com/janlink/beads-dash/internal/ui"
)

// wireHost connects the clipboard and the notifications to the machine bdash
// runs on. The notifier is built with a method that delivers even when the
// setting says off, so switching notifications on in the session works.
func wireHost(o *ui.Options, getenv func(string) string, method string, run proc.Runner) {
	env := host.Detect(runtime.GOOS, getenv, os.ReadFile)
	if method == notify.MethodOff {
		method = notify.MethodAuto
	}
	o.Clipboard = clipboard.New(env, run)
	o.Notifier = notify.New(env, run, method)
}
