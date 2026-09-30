package ui

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/ui/detail"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

// pump runs the commands an app returns and feeds their messages back.
type pump struct {
	t    testing.TB
	a    *App
	msgs chan tea.Msg
}

func newPump(t testing.TB, a *App) *pump { return &pump{t: t, a: a, msgs: make(chan tea.Msg, 64)} }

func (p *pump) spawn(c tea.Cmd) {
	if c != nil {
		go func() { p.msgs <- c() }()
	}
}

func (p *pump) send(m tea.Msg) { p.spawn(send(p.a, m)) }

func (p *pump) poke() { p.send(tea.WindowSizeMsg{Width: 120, Height: 60}) }

func (p *pump) until(done func() bool) {
	p.t.Helper()
	timeout := time.After(10 * time.Second)
	for !done() {
		select {
		case m := <-p.msgs:
			if b, ok := m.(tea.BatchMsg); ok {
				for _, c := range b {
					p.spawn(c)
				}
				continue
			}
			p.send(m)
		case <-timeout:
			p.t.Fatalf("state not reached; screen:\n%s", screen(p.a))
		}
	}
}

func count(calls []string, method string) int {
	return len(slices.DeleteFunc(slices.Clone(calls), func(c string) bool { return c != method }))
}

type auditEnv struct {
	*pump
	fake *bd.Fake
}

// auditFixture is the tree workspace with comment counts on ws-9qe and ws-4k2,
// shown with the panel docked at the bottom.
func auditFixture(t *testing.T) auditEnv {
	t.Helper()
	fake := treeFake(t)
	issues := fake.Issues()
	for i := range issues {
		switch issues[i].ID {
		case "ws-9qe":
			issues[i].CommentCount = 2
		case "ws-4k2":
			issues[i].CommentCount = 1
		}
	}
	fake.SetIssues(issues...)
	fake.SetComments("ws-9qe",
		bd.Comment{Author: "ann", Text: "first thoughts", CreatedAt: uitest.T0.Add(-5 * time.Hour)},
		bd.Comment{Author: "bob", Text: "second thoughts", CreatedAt: uitest.T0.Add(-2 * time.Hour)})
	fake.SetComments("ws-4k2", bd.Comment{Author: "cy", Text: "epic remark", CreatedAt: uitest.T0.Add(-time.Hour)})
	a := New(testOptions(plain, func(o *Options) {
		withView("tree", true)(o)
		o.Client = fake
	}))
	send(a, tea.WindowSizeMsg{Width: 120, Height: 60})
	p := newPump(t, a)
	p.spawn(a.Init())
	p.until(func() bool { return a.snap != nil && a.eng != nil })
	t.Cleanup(func() {
		a.eng.Stop()
		a.cancel()
	})
	a.sess.SetCurrent("ws-9qe")
	return auditEnv{p, fake}
}

func (e auditEnv) openAudit() {
	e.a.View()
	for e.a.panel.Cursor() != detail.Audit {
		e.a.panel.Next()
	}
	e.a.panel.Expand()
	e.poke()
}

func TestAuditCommentsLoadLazilyNewestFirst(t *testing.T) {
	e := auditFixture(t)
	e.poke()
	e.a.View()
	if n := count(e.fake.Calls(), "Comments"); n != 0 {
		t.Fatalf("a closed section fetched %d times", n)
	}
	e.openAudit()
	e.until(func() bool { return strings.Contains(screen(e.a), "second thoughts") })
	out := screen(e.a)
	if strings.Index(out, "second thoughts") > strings.Index(out, "first thoughts") {
		t.Errorf("comments are not newest first:\n%s", out)
	}
	e.poke()
	e.a.View()
	if n := count(e.fake.Calls(), "Comments"); n != 1 {
		t.Errorf("Comments called %d times, want 1", n)
	}
}

func TestAuditSkipsIssueWithoutComments(t *testing.T) {
	e := auditFixture(t)
	e.a.sess.SetCurrent("ws-7mt")
	e.openAudit()
	if n := count(e.fake.Calls(), "Comments"); n != 0 {
		t.Fatalf("fetched comments of an issue without any: %d", n)
	}
	if !strings.Contains(screen(e.a), "no comments") {
		t.Errorf("no empty note:\n%s", screen(e.a))
	}
}

func TestAuditHistoryFetchedOnExpandAndReduced(t *testing.T) {
	e := auditFixture(t)
	is := func(status string, prio int, title string) model.Issue {
		return model.Issue{ID: "ws-9qe", Title: title, Status: status, Priority: prio}
	}
	at := func(h int) time.Time { return uitest.T0.Add(-time.Duration(h) * time.Hour) }
	e.fake.SetHistory("ws-9qe",
		bd.HistoryEntry{Committer: "root", CommitDate: at(1), Issue: is("in_progress", 0, "Payment")},
		bd.HistoryEntry{Committer: "root", CommitDate: at(2), Issue: is("open", 0, "Payment")},
		bd.HistoryEntry{Committer: "root", CommitDate: at(3), Issue: is("open", 0, "Payment")},
		bd.HistoryEntry{Committer: "root", CommitDate: at(4), Issue: is("open", 2, "Payment")})
	e.openAudit()
	e.until(func() bool { return strings.Contains(screen(e.a), "second thoughts") })
	if n := count(e.fake.Calls(), "History"); n != 0 {
		t.Fatalf("history fetched before it was expanded: %d", n)
	}
	e.a.panel.Move(1)
	e.a.panel.Enter()
	e.poke()
	e.until(func() bool { return strings.Contains(screen(e.a), "priority P2 → P0") })
	out := screen(e.a)
	for _, want := range []string{"no author recorded", "status open → in_progress", "created", "Recorded changes (3)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if n := count(e.fake.Calls(), "History"); n != 1 {
		t.Errorf("History called %d times", n)
	}
}

func TestAuditCacheHoldsUntilUpdatedAtChanges(t *testing.T) {
	e := auditFixture(t)
	e.openAudit()
	e.until(func() bool { return strings.Contains(screen(e.a), "second thoughts") })

	e.a.sess.SetCurrent("ws-4k2")
	e.poke()
	e.until(func() bool { return strings.Contains(screen(e.a), "epic remark") })
	e.a.sess.SetCurrent("ws-9qe")
	e.poke()
	e.a.View()
	if !strings.Contains(screen(e.a), "second thoughts") {
		t.Fatal("cached comments were not shown again")
	}
	if n := count(e.fake.Calls(), "Comments"); n != 2 {
		t.Fatalf("going back refetched: %d calls", n)
	}

	issues := e.fake.Issues()
	for i := range issues {
		if issues[i].ID == "ws-9qe" {
			issues[i].CommentCount = 3
			issues[i].UpdatedAt = time.Now()
		}
	}
	e.fake.SetIssues(issues...)
	e.fake.SetComments("ws-9qe", bd.Comment{Author: "dee", Text: "late news", CreatedAt: time.Now()})
	before := e.a.snap
	e.a.eng.Refresh()
	e.until(func() bool { return e.a.snap != before })
	e.until(func() bool { return strings.Contains(screen(e.a), "late news") })
	if n := count(e.fake.Calls(), "Comments"); n != 3 {
		t.Errorf("an updated issue fetched %d times in all, want 3", n)
	}
}

func TestAuditRefetchesWhenOnlyCommentCountChanges(t *testing.T) {
	e := auditFixture(t)
	e.openAudit()
	e.until(func() bool { return strings.Contains(screen(e.a), "second thoughts") })

	issues := e.fake.Issues()
	for i := range issues {
		if issues[i].ID == "ws-9qe" {
			issues[i].CommentCount = 3
		}
	}
	e.fake.SetIssues(issues...)
	e.fake.SetComments("ws-9qe", bd.Comment{Author: "dee", Text: "late news", CreatedAt: time.Now()})
	before := e.a.snap
	e.a.eng.Refresh()
	e.until(func() bool { return e.a.snap != before })
	e.until(func() bool { return strings.Contains(screen(e.a), "late news") })
}

func TestAuditSupersededFetchIsCancelledAndDropped(t *testing.T) {
	e := auditFixture(t)
	var mu sync.Mutex
	first := true
	started, release := make(chan struct{}), make(chan struct{})
	e.fake.SetHook(func(method string) {
		if method != "Comments" {
			return
		}
		mu.Lock()
		block := first
		first = false
		mu.Unlock()
		if block {
			close(started)
			<-release
		}
	})
	e.openAudit()
	<-started

	e.a.sess.SetCurrent("ws-4k2")
	e.poke()
	if len(e.a.fetching) != 1 {
		t.Fatalf("superseded fetch not cancelled: %d running", len(e.a.fetching))
	}
	close(release)
	e.until(func() bool { return strings.Contains(screen(e.a), "epic remark") })
	if out := screen(e.a); strings.Contains(out, "failed") || strings.Contains(out, "canceled") {
		t.Errorf("the cancelled read surfaced:\n%s", out)
	}

	e.a.sess.SetCurrent("ws-9qe")
	e.poke()
	e.until(func() bool { return strings.Contains(screen(e.a), "second thoughts") })
	if n := count(e.fake.Calls(), "Comments"); n != 3 {
		t.Errorf("Comments called %d times, want 3", n)
	}
}

var errBoom = errors.New("boom")

func TestAuditErrorIsInlineAndRetried(t *testing.T) {
	e := auditFixture(t)
	e.fake.FailWith("Comments", errBoom)
	e.openAudit()
	e.until(func() bool { return strings.Contains(screen(e.a), "comments failed: boom") })
	e.fake.FailWith("Comments", nil)
	e.send(keyMsg("r"))
	e.poke()
	e.until(func() bool { return strings.Contains(screen(e.a), "second thoughts") })
}
