package ui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/testbd"
	"github.com/janlink/beads-dash/internal/ui/uitest"
)

// syncEngine runs writes on the fake in the caller's goroutine and, like the
// real engine, publishes a fresh snapshot before Write returns.
type syncEngine struct {
	client bd.Client
	// publish sends the refreshed snapshot to the app; nil holds it back.
	publish func()
	// events counts EnableEvents calls.
	events int
	notify []notifyCall
}

func (e *syncEngine) Start(context.Context)          {}
func (e *syncEngine) Stop()                          {}
func (e *syncEngine) Updates() <-chan refresh.Update { return nil }
func (e *syncEngine) Refresh()                       {}
func (e *syncEngine) SetFocus(bool)                  {}
func (e *syncEngine) SetNotify(on bool, kinds model.KindSet) {
	e.notify = append(e.notify, notifyCall{on, kinds})
}
func (e *syncEngine) EnableEvents() { e.events++ }

func (e *syncEngine) Do(ctx context.Context, fn func(context.Context, bd.Client) error) error {
	return fn(ctx, e.client)
}

func (e *syncEngine) Write(ctx context.Context, _ []string, fn func(context.Context, bd.Client) error) error {
	err := fn(ctx, e.client)
	if e.publish != nil {
		e.publish()
	}
	return err
}

// writeRig is an app over a fake workspace whose writes run synchronously.
type writeRig struct {
	t      testing.TB
	a      *App
	client bd.Client
	// fake is set when the rig runs on the fake client.
	fake *bd.Fake
	eng  *syncEngine
}

func newRig(t testing.TB, cols, rows int) *writeRig { return newRigView(t, cols, rows, "tree") }

func newRigView(t testing.TB, cols, rows int, view string) *writeRig {
	t.Helper()
	fake := fakeWorkspace(t)
	fake.SetActor("me")
	snap, _ := uitest.Sample()
	r := &writeRig{t: t, client: fake, fake: fake, eng: &syncEngine{client: fake}}
	r.a = loaded(t, plain, cols, rows, snap, func(o *Options) {
		withView(view, false)(o)
		o.Client, o.Actor = fake, "me"
	})
	r.a.eng = r.eng
	r.eng.publish = r.snapshot
	return r
}

// newLiveRig is an app over a real bd workspace; it starts with a snapshot
// of what the workspace holds.
func newLiveRig(t testing.TB, w testbd.Workspace, cols, rows int, view string) *writeRig {
	t.Helper()
	client := bd.NewExec(bd.ExecOptions{Bin: w.Bin, Dir: w.Dir})
	r := &writeRig{t: t, client: client, eng: &syncEngine{client: client}}
	snap, err := bd.FetchSnapshot(context.Background(), client, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.a = loaded(t, plain, cols, rows, snap, func(o *Options) {
		withView(view, false)(o)
		o.Client, o.Actor = client, "tester"
	})
	r.a.eng = r.eng
	r.eng.publish = r.snapshot
	return r
}

func (r *writeRig) snapshot() {
	r.t.Helper()
	snap, err := bd.FetchSnapshot(context.Background(), r.client, func() time.Time { return uitest.T0 })
	if err != nil {
		r.t.Error(err)
		return
	}
	send(r.a, updateMsg{refresh.Update{Snapshot: snap, Status: liveStatus(), Session: workspace()}})
}

// collect runs cmd and returns the messages it produced; a command that has
// not answered within wait is a timer and is left out.
func collect(cmd tea.Cmd, wait time.Duration) []tea.Msg {
	if cmd == nil {
		return nil
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case m := <-done:
		if b, ok := m.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, c := range b {
				out = append(out, collect(c, wait)...)
			}
			return out
		}
		if m == nil {
			return nil
		}
		return []tea.Msg{m}
	case <-time.After(wait):
		return nil
	}
}

// run feeds cmd's messages back to the app until nothing is left.
func (r *writeRig) run(cmd tea.Cmd) {
	r.t.Helper()
	for depth := 0; cmd != nil && depth < 20; depth++ {
		var next []tea.Cmd
		wait := 250 * time.Millisecond
		if len(r.a.pending) > 0 || r.a.mem != nil && r.a.mem.reading || r.exportBusy() {
			wait = 30 * time.Second
		}
		for _, m := range collect(cmd, wait) {
			if _, ok := m.(spinMsg); ok {
				continue
			}
			next = append(next, send(r.a, m))
		}
		cmd = tea.Batch(next...)
	}
}

// key presses each key and runs the commands it returns.
func (r *writeRig) key(keys ...string) {
	r.t.Helper()
	for _, k := range keys {
		r.run(send(r.a, keyMsg(k)))
	}
}

func (r *writeRig) text(s string) {
	r.t.Helper()
	for _, c := range s {
		if c == ' ' {
			r.key("space")
			continue
		}
		r.key(string(c))
	}
}

func (r *writeRig) line(s string) {
	r.t.Helper()
	r.key(":")
	r.text(s)
	r.key("enter")
}

func (r *writeRig) issue(id string) model.Issue {
	r.t.Helper()
	is, ok := r.a.snap.Issue(id)
	if !ok {
		r.t.Fatalf("no issue %s", id)
	}
	return *is
}

type notifyCall struct {
	on    bool
	kinds model.KindSet
}
