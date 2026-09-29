package refresh

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
)

var t0 = time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC)

type rig struct {
	t    *testing.T
	fake *bd.Fake
	clk  *FakeClock
	eng  *Engine
	ctx  context.Context

	mu  sync.Mutex
	ups []Update
}

type rigOpts struct {
	journal bool
	notify  bool
	kinds   []string
	tun     func(*Tunables)
	client  func(*bd.Fake) bd.Client
	noStart bool
}

func issue(id, title, status string) model.Issue {
	return model.Issue{
		ID: id, Title: title, Status: status, IssueType: "task", Priority: 2,
		CreatedAt: t0.Add(-48 * time.Hour), UpdatedAt: t0.Add(-48 * time.Hour),
	}
}

func newRig(t *testing.T, o rigOpts) *rig {
	t.Helper()
	r := &rig{t: t, fake: bd.NewFake(), clk: NewFakeClock(t0), ctx: context.Background()}
	r.fake.SetIssues(issue("f-1", "One", "open"), issue("f-2", "Two", "open"))
	r.fake.SetReadiness([]string{"f-1", "f-2"}, nil)
	if o.journal {
		r.fake.SetConfig("events-journal", "true")
	}
	sess, err := bd.OpenSession(r.ctx, r.fake)
	if err != nil {
		t.Fatal(err)
	}
	tun := DefaultTunables()
	if o.tun != nil {
		o.tun(&tun)
	}
	var client bd.Client = r.fake
	if o.client != nil {
		client = o.client(r.fake)
	}
	kinds := model.KindSetOf("closed", "blocked", "ready")
	if o.kinds != nil {
		kinds = model.KindSetOf(o.kinds...)
	}
	r.eng = New(Options{
		Client: client, Session: sess, Clock: r.clk, Tunables: &tun,
		Actor: "me", Notify: o.notify, Kinds: kinds,
	})
	r.eng.tap = r.collect
	if !o.noStart {
		r.start()
	}
	return r
}

func (r *rig) start() {
	r.eng.Start(r.ctx)
	r.t.Cleanup(r.eng.Stop)
	r.eng.barrier()
}

func (r *rig) collect(u Update) {
	r.mu.Lock()
	r.ups = append(r.ups, u)
	r.mu.Unlock()
}

// step advances the clock and lets the engine act on what came due.
func (r *rig) step(d time.Duration) {
	r.t.Helper()
	r.clk.Advance(d)
	r.eng.barrier()
}

func (r *rig) updates() []Update {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Update(nil), r.ups...)
}

func (r *rig) last() Update {
	r.t.Helper()
	ups := r.updates()
	if len(ups) == 0 {
		r.t.Fatal("no update published")
	}
	return ups[len(ups)-1]
}

// callsSince returns the client calls after the mark, filtered to the names.
func (r *rig) count(method string) int {
	n := 0
	for _, c := range r.fake.Calls() {
		if c == method {
			n++
		}
	}
	return n
}

func (r *rig) mark() int { return len(r.fake.Calls()) }

func (r *rig) since(mark int) []string { return r.fake.Calls()[mark:] }

func transient(cmd string) error {
	return &bd.Error{Class: bd.ClassTransient, Command: cmd, ExitCode: 1, Message: "boom"}
}

func setStatus(r *rig, id, status string) {
	r.t.Helper()
	issues := r.fake.Issues()
	for i := range issues {
		if issues[i].ID == id {
			issues[i].Status = status
			issues[i].UpdatedAt = r.clk.Now()
		}
	}
	r.fake.SetIssues(issues...)
}

func setIssue(r *rig, id string, f func(*model.Issue)) {
	r.t.Helper()
	issues := r.fake.Issues()
	for i := range issues {
		if issues[i].ID == id {
			f(&issues[i])
			issues[i].UpdatedAt = r.clk.Now()
		}
	}
	r.fake.SetIssues(issues...)
}

func kinds(evs []model.Event) []model.Kind {
	var out []model.Kind
	for _, e := range evs {
		out = append(out, e.Kind)
	}
	return out
}

func (r *rig) refresh() {
	r.t.Helper()
	r.eng.Refresh()
	r.eng.barrier()
}

func (r *rig) focus(b bool) {
	r.t.Helper()
	r.eng.SetFocus(b)
	r.eng.barrier()
}

func (r *rig) setNotify(on bool, k model.KindSet) {
	r.t.Helper()
	r.eng.SetNotify(on, k)
	r.eng.barrier()
}

func (r *rig) write(ids ...string) {
	r.t.Helper()
	err := r.eng.Write(r.ctx, ids, func(context.Context, bd.Client) error { return nil })
	if err != nil {
		r.t.Fatal(err)
	}
}
