package notify_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/janlink/beads-dash/internal/host"
	"github.com/janlink/beads-dash/internal/notify"
	"github.com/janlink/beads-dash/internal/proc"
)

func TestSmokeBackendOfTheRunningHost(t *testing.T) {
	env := host.Detect(runtime.GOOS, os.Getenv, os.ReadFile)
	n := notify.New(env, proc.System{}, notify.MethodAuto)
	switch {
	case runtime.GOOS == "windows" || env.WSL:
		if n.Backend() != "" {
			t.Errorf("backend = %q, want none", n.Backend())
		}
	case runtime.GOOS == "darwin":
		if n.Backend() != "osascript" {
			t.Errorf("backend = %q, want osascript", n.Backend())
		}
	}
	t.Logf("backend on %s: %q", runtime.GOOS, n.Backend())
}
