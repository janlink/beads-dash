package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/refresh"

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

func TestIntegrationMemoriesRememberRenameForget(t *testing.T) {
	testbd.EachVersion(t, func(t *testing.T, w testbd.Workspace) {
		r := newLiveRig(t, w, 120, 40, "tree")
		r.key("5")
		r.line("remember Prefer small commits")
		r.key("ctrl+s")
		if _, ok := r.a.mem.get("prefer-small-commits"); !ok {
			t.Fatalf("not remembered:\n%s", screen(r.a))
		}

		r.a.mem.cursor = "prefer-small-commits"
		r.key("e")
		r.a.topDialog().(*memoryDialog).f.Field(fMemKey).Set("small-commits")
		r.key("ctrl+s")
		if _, ok := r.a.mem.get("small-commits"); !ok {
			t.Fatalf("not renamed:\n%s", screen(r.a))
		}
		if _, ok := r.a.mem.get("prefer-small-commits"); ok {
			t.Error("old key survived the rename")
		}

		r.a.mem.cursor = "small-commits"
		r.key("d", "y")
		if len(r.a.mem.list) != 0 {
			t.Errorf("left %v", r.a.mem.list)
		}
		out, err := w.Bd("memories", "--json")
		if err != nil || strings.Contains(out, "small-commits") {
			t.Errorf("bd still holds it: %v %s", err, out)
		}
	})
}

func TestIntegrationJournalOptInTurnsEventsOn(t *testing.T) {
	w := testbd.New(t, "1.3.0")
	client := bd.NewExec(bd.ExecOptions{Bin: w.Bin, Dir: w.Dir})
	a := New(testOptions(plain, func(o *Options) {
		withView("tree", false)(o)
		o.Client, o.Journal, o.Now = client, &memJournal{}, time.Now
	}))
	send(a, tea.WindowSizeMsg{Width: 120, Height: 40})
	p := newPump(t, a)
	p.spawn(a.Init())
	p.until(func() bool { return a.snap != nil && a.eng != nil })
	t.Cleanup(func() {
		a.eng.Stop()
		a.cancel()
	})
	eng := a.eng.(*refresh.Engine)
	if eng.Status().Mode == refresh.Events {
		t.Fatal("events mode before the opt-in")
	}
	p.until(func() bool { return isOpen[*journalDialog](a) })
	p.send(keyMsg("y"))
	p.until(func() bool {
		st := eng.Status()
		return st.Mode == refresh.Events && st.Following
	})
	if out, err := w.Bd("config", "get", "events-journal"); err != nil || !strings.Contains(out, "true") {
		t.Errorf("config %q %v", out, err)
	}
	if _, err := w.Bd("--actor", "alice", "create", "--silent", "--title=After the opt-in"); err != nil {
		t.Fatal(err)
	}
	p.until(func() bool {
		for _, e := range a.feed.Newest() {
			if e.Actor == "alice" && !e.Prefill {
				return true
			}
		}
		return false
	})
}
