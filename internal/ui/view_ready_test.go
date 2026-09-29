package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/testgolden"
)

func TestReadyGoldens(t *testing.T) {
	for _, s := range goldenSizes {
		t.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(t *testing.T) {
			testgolden.Equal(t, screen(viewApp(t, plain, s[0], s[1], "ready", false)))
		})
	}
	t.Run("blocked open", func(t *testing.T) {
		a := viewApp(t, plain, 120, 30, "ready", false)
		a.sess.SetCurrent("ws-2hz")
		testgolden.Equal(t, screen(a))
	})
	t.Run("truecolor", func(t *testing.T) {
		testgolden.Equal(t, viewApp(t, truecolor, 120, 30, "ready", false).View().Content)
	})
}

func TestReadyGroupsContainersAndBlocked(t *testing.T) {
	a := viewApp(t, plain, 120, 30, "ready", false)
	out := screen(a)
	for _, want := range []string{"Unassigned (", "Assigned, not started (", "Blocked (2)", "containers hidden"} {
		if want == "containers hidden" {
			want = "1 container hidden"
		}
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ws-4k2 ") && strings.Contains(out, "Checkout: guest orders") {
		t.Errorf("a container is listed:\n%s", out)
	}
	if strings.Contains(out, "waits on") {
		t.Error("blocked rows show while the section is collapsed")
	}
	if strings.Contains(out, "ws-7mt") {
		t.Error("an in-progress issue is listed as ready")
	}

	ready := a.view().(*Ready)
	for range 20 {
		if k, ok := ready.cursor(a.env()); ok && ready.rows[ready.index[k]].kind == readyBlockedHead {
			break
		}
		press(a, "j")
	}
	press(a, "enter")
	out = screen(a)
	if !strings.Contains(out, "waits on ws-9qe") || !strings.Contains(out, "waits on ws-4k2.1") {
		t.Errorf("open blocked section lacks the blockers:\n%s", out)
	}
	press(a, "h")
	if strings.Contains(screen(a), "waits on") {
		t.Error("h closes the blocked section")
	}
}

func TestReadyScopeAppliesFacetsNotStatusVisibility(t *testing.T) {
	a := viewApp(t, plain, 120, 30, "ready", false)
	a.setScope(model.ParseScope("type:bug", false))
	out := screen(a)
	if !strings.Contains(out, "ws-9qe") || strings.Contains(out, "ws-4k2.1") {
		t.Errorf("type facet must narrow the list:\n%s", out)
	}
	a.setScope(model.ParseScope("zzzz", false))
	if !strings.Contains(screen(a), `No issue matches "zzzz".`) {
		t.Error("empty scope state missing")
	}
}
