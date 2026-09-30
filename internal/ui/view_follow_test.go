package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

func treeFake(t testing.TB) *bd.Fake {
	t.Helper()
	fake := bd.NewFake()
	fake.SetWorkspace(bd.Workspace{Path: "/work/demo/.beads", Prefix: "ws"})
	snap, _ := uitest.Tree()
	var issues []model.Issue
	for _, id := range snap.IDs() {
		is, _ := snap.Issue(id)
		issues = append(issues, *is)
	}
	fake.SetIssues(issues...)
	fake.SetReadiness([]string{"ws-4k2.1", "ws-4k2.5", "ws-9qe"}, map[string][]string{"ws-4k2.2": {"ws-4k2.1"}}, map[string]string{
		"ws-4k2.1": "no blocking dependencies", "ws-4k2.5": "no blocking dependencies", "ws-9qe": "no blocking dependencies",
	})
	return fake
}

func TestCurrentFollowsBetweenTreeAndReadyThroughTheEngine(t *testing.T) {
	fake := treeFake(t)
	a := New(testOptions(plain, func(o *Options) {
		withView("tree", false)(o)
		o.Client, o.Now = fake, time.Now
	}))
	send(a, tea.WindowSizeMsg{Width: 120, Height: 30})
	p := newPump(t, a)
	p.spawn(a.Init())
	p.until(func() bool { return a.snap != nil && a.eng != nil })
	defer func() {
		a.eng.Stop()
		a.cancel()
	}()

	a.sess.SetCurrent("ws-4k2.1")
	press(a, "4")
	if a.sess.Current() != "ws-4k2.1" || !a.view().Has(a.env(), "ws-4k2.1") {
		t.Fatalf("Ready dropped the current issue: %q", a.sess.Current())
	}
	press(a, "2")
	if a.sess.Current() != "ws-4k2.1" {
		t.Fatalf("Tree dropped the current issue: %q", a.sess.Current())
	}

	closed, _ := a.snap.Issue("ws-4k2.1")
	next := *closed
	next.Status, next.ClosedAt = "closed", time.Now()
	var issues []model.Issue
	for _, id := range a.snap.IDs() {
		is, _ := a.snap.Issue(id)
		if id == next.ID {
			is = &next
		}
		issues = append(issues, *is)
	}
	fake.SetIssues(issues...)
	fake.SetReadiness([]string{"ws-4k2.2", "ws-4k2.5", "ws-9qe"}, nil, map[string]string{
		"ws-4k2.2": "1 blocker(s) resolved", "ws-4k2.5": "no blocking dependencies", "ws-9qe": "no blocking dependencies",
	})
	before := a.snap
	a.eng.Refresh()
	p.until(func() bool { return a.snap != before })

	press(a, "4")
	cur := a.sess.Current()
	if cur == "" || !a.view().Has(a.env(), cur) {
		t.Errorf("Ready shows the current issue %q after the engine's update", cur)
	}
	press(a, "2")
	if a.sess.Current() != cur {
		t.Errorf("switching to the Tree moved current from %q to %q", cur, a.sess.Current())
	}
}
