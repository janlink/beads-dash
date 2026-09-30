package clipboard_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/host"
	"github.com/janlink/beads-dash/internal/proc"
)

func TestSmokeRoutesOfTheRunningHost(t *testing.T) {
	env := host.Detect(runtime.GOOS, os.Getenv, os.ReadFile)
	routes := clipboard.Select(env, proc.System{}.LookPath)
	switch runtime.GOOS {
	case "windows":
		if len(routes) == 0 || routes[0].Name != "clip.exe" {
			t.Fatalf("routes = %+v, want clip.exe first", routes)
		}
		if got := routes[0].Payload("ü"); len(got) != 2 || got[0] != 0xfc || got[1] != 0 {
			t.Errorf("clip.exe payload = % x, want UTF-16LE", got)
		}
	case "darwin":
		if len(routes) == 0 || routes[0].Name != "pbcopy" {
			t.Fatalf("routes = %+v, want pbcopy first", routes)
		}
	}
	t.Logf("routes on %s: %d", runtime.GOOS, len(routes))
}
