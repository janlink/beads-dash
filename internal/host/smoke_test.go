package host_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/janlink/beads-dash/internal/host"
)

func TestSmokeDetectsTheRunningHost(t *testing.T) {
	env := host.Detect(runtime.GOOS, os.Getenv, os.ReadFile)
	if env.GOOS != runtime.GOOS {
		t.Errorf("GOOS = %q", env.GOOS)
	}
	if runtime.GOOS == "windows" && (env.WSL || env.Wayland || env.X11) {
		t.Errorf("a native Windows host looks like %+v", env)
	}
}
