package ui

import (
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/testbd"
)

func (r *writeRig) create(title string) string {
	r.t.Helper()
	r.key("n")
	r.text(title)
	r.key("ctrl+s")
	id := r.a.sess.Current()
	if is, ok := r.a.snap.Issue(id); !ok || is.Title != title {
		r.t.Fatalf("creating %q left %q current:\n%s", title, id, screen(r.a))
	}
	return id
}

func TestIntegrationWriteRoundTrip(t *testing.T) {
	testbd.EachVersion(t, func(t *testing.T, w testbd.Workspace) {
		r := newLiveRig(t, w, 120, 40, "kanban")
		blocked := r.create("Alpha")
		blocker := r.create("Beta")

		r.a.sess.SetCurrent(blocked)
		r.line("dep add " + blocker)
		if !r.a.snap.IsBlocked(blocked) {
			t.Fatalf("%s is not blocked after :dep add", blocked)
		}

		r.key("e")
		r.text(" edited")
		r.key("ctrl+s")
		if got := r.issue(blocked).Title; got != "Alpha edited" {
			t.Fatalf("title %q", got)
		}

		r.key("c", "enter")
		if !isOpen[*closeDialog](r.a) {
			t.Fatalf("closing a blocked issue was not refused:\n%s", screen(r.a))
		}
		if out := screen(r.a); !strings.Contains(out, blocker) {
			t.Errorf("blocker not listed:\n%s", out)
		}
		if r.issue(blocked).Status == "closed" {
			t.Fatal("the guard was bypassed")
		}
		r.key("enter")
		if r.a.sess.Current() != blocker {
			t.Fatalf("enter jumped to %q, want %s", r.a.sess.Current(), blocker)
		}

		r.key("c")
		r.text("done")
		r.key("enter")
		if r.issue(blocker).Status != "closed" {
			t.Fatalf("blocker status %q", r.issue(blocker).Status)
		}

		r.a.sess.SetCurrent(blocked)
		r.key("c", "enter")
		if r.issue(blocked).Status != "closed" {
			t.Fatalf("status %q after the blocker closed:\n%s", r.issue(blocked).Status, screen(r.a))
		}

		r.a.sess.SetCurrent(blocked)
		r.key("c", "enter")
		if r.issue(blocked).Status != "open" {
			t.Errorf("status %q after reopen", r.issue(blocked).Status)
		}
	})
}

func TestIntegrationBulkPriorityOnThreeMarks(t *testing.T) {
	testbd.EachVersion(t, func(t *testing.T, w testbd.Workspace) {
		r := newLiveRig(t, w, 120, 40, "tree")
		ids := []string{r.create("One"), r.create("Two"), r.create("Three")}
		for _, id := range ids {
			r.a.sess.SetCurrent(id)
			r.key("space")
		}
		if n := len(r.a.sess.MarkedIDs()); n != 3 {
			t.Fatalf("%d marks", n)
		}
		r.key("p", "0")
		for _, id := range ids {
			if p := r.issue(id).Priority; p != 0 {
				t.Errorf("%s priority %d", id, p)
			}
		}
		if n := len(r.a.sess.MarkedIDs()); n != 0 {
			t.Errorf("%d marks left", n)
		}
	})
}
