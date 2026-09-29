package refresh

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
)

// gauge wraps a client and records how many calls overlap.
type gauge struct {
	bd.Client
	in, max atomic.Int32
}

func (g *gauge) enter() func() {
	n := g.in.Add(1)
	for {
		m := g.max.Load()
		if n <= m || g.max.CompareAndSwap(m, n) {
			break
		}
	}
	return func() { g.in.Add(-1) }
}

func (g *gauge) List(ctx context.Context) ([]model.Issue, error) {
	defer g.enter()()
	return g.Client.List(ctx)
}

func (g *gauge) Ready(ctx context.Context) (model.Readiness, error) {
	defer g.enter()()
	return g.Client.Ready(ctx)
}

func (g *gauge) VCStatus(ctx context.Context) (bd.VCStatus, error) {
	defer g.enter()()
	return g.Client.VCStatus(ctx)
}

func (g *gauge) Where(ctx context.Context) (bd.Workspace, error) {
	defer g.enter()()
	return g.Client.Where(ctx)
}

func (g *gauge) Statuses(ctx context.Context) (model.Statuses, error) {
	defer g.enter()()
	return g.Client.Statuses(ctx)
}

func (g *gauge) Types(ctx context.Context) ([]bd.TypeInfo, error) {
	defer g.enter()()
	return g.Client.Types(ctx)
}

func (g *gauge) Comments(ctx context.Context, id string) ([]bd.Comment, error) {
	defer g.enter()()
	return g.Client.Comments(ctx, id)
}

func (g *gauge) Close(ctx context.Context, ids []string, reason string) ([]string, error) {
	defer g.enter()()
	return g.Client.Close(ctx, ids, reason)
}

func TestNeverMoreThanOneBdCallInFlight(t *testing.T) {
	var g *gauge
	r := newRig(t, rigOpts{client: func(f *bd.Fake) bd.Client {
		g = &gauge{Client: f}
		return g
	}})
	var wg sync.WaitGroup
	run := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}
	for range 8 {
		run(func() {
			_ = r.eng.Do(r.ctx, func(ctx context.Context, c bd.Client) error {
				_, err := c.Comments(ctx, "f-1")
				return err
			})
		})
		run(func() { r.refresh() })
		run(func() { r.write("f-1") })
		run(func() { r.focus(true) })
		run(func() { r.clk.Advance(3 * time.Second); r.eng.barrier() })
	}
	wg.Wait()
	if got := g.max.Load(); got != 1 {
		t.Errorf("max concurrent bd calls = %d", got)
	}
	if r.count("Comments") != 8 {
		t.Errorf("%d Do calls ran", r.count("Comments"))
	}
}

func TestDoRunsInTheQueueAndReturnsItsError(t *testing.T) {
	r := newRig(t, rigOpts{})
	boom := errors.New("boom")
	if err := r.eng.Do(r.ctx, func(context.Context, bd.Client) error { return boom }); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
	ran := false
	ctx, cancel := context.WithCancel(r.ctx)
	cancel()
	err := r.eng.Do(ctx, func(context.Context, bd.Client) error { ran = true; return nil })
	if !errors.Is(err, context.Canceled) || ran {
		t.Errorf("canceled Do: ran=%v err=%v", ran, err)
	}
}

func TestCallsBeforeStartAndAfterStopReturn(t *testing.T) {
	noop := func(context.Context, bd.Client) error { return nil }
	r := newRig(t, rigOpts{noStart: true})
	r.eng.Refresh()
	r.eng.SetFocus(false)
	r.eng.SetNotify(true, nil)
	if err := r.eng.Do(r.ctx, noop); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Do before Start = %v", err)
	}
	r.eng.Start(r.ctx)
	r.eng.Start(r.ctx)
	r.eng.barrier()
	if r.eng.Status().Focused {
		t.Error("calls posted before Start were lost")
	}
	r.eng.Stop()
	if err := r.eng.Do(r.ctx, noop); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Do after Stop = %v", err)
	}
	if err := r.eng.Write(r.ctx, []string{"f-1"}, noop); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Write after Stop = %v", err)
	}
	r.eng.Refresh()
	r.eng.SetFocus(true)
	r.eng.barrier()
	if _, open := <-r.eng.Updates(); open {
		t.Error("Updates must close on Stop")
	}
}

func TestStopBeforeStart(t *testing.T) {
	r := newRig(t, rigOpts{noStart: true})
	r.eng.Stop()
	r.eng.Start(r.ctx)
	if _, open := <-r.eng.Updates(); open {
		t.Error("stopped engine must not start")
	}
}

func TestStopCancelsInFlightWork(t *testing.T) {
	r := newRig(t, rigOpts{})
	inside := make(chan struct{})
	res := make(chan error, 1)
	go func() {
		res <- r.eng.Do(r.ctx, func(ctx context.Context, _ bd.Client) error {
			close(inside)
			<-ctx.Done()
			return ctx.Err()
		})
	}()
	<-inside
	r.eng.Stop()
	if err := <-res; !errors.Is(err, context.Canceled) {
		t.Errorf("Do = %v", err)
	}
}

func TestCallerContextDoesNotLeakIntoTheEngine(t *testing.T) {
	r := newRig(t, rigOpts{journal: true})
	ctx, cancel := context.WithCancel(r.ctx)
	_ = r.eng.Do(ctx, func(context.Context, bd.Client) error { return nil })
	cancel()
	r.fake.KillFollowers(errors.New("died"))
	r.eng.barrier()
	r.step(2 * time.Second)
	if !r.eng.Status().Following || r.fake.OpenFollowers() != 1 {
		t.Error("follower restarted under a caller context must survive its cancellation")
	}
	r.fake.DrainFollowers()
}

func TestWriteMarksRunsAndRefreshesInOneStep(t *testing.T) {
	r := newRig(t, rigOpts{notify: true, kinds: model.KindNames()})
	err := r.eng.Write(r.ctx, []string{"f-1"}, func(ctx context.Context, c bd.Client) error {
		_, err := c.Close(ctx, []string{"f-1"}, "done")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	u := r.last()
	if len(u.Events) != 1 || u.Events[0].Kind != model.KindClosed || !u.Events[0].Own || u.Events[0].Actor != "me" {
		t.Errorf("events = %+v", u.Events)
	}
	if len(u.Notification.Events) != 0 {
		t.Errorf("own write notified: %+v", u.Notification)
	}
	boom := errors.New("partial")
	setStatus(r, "f-2", "closed")
	err = r.eng.Write(r.ctx, []string{"f-2"}, func(context.Context, bd.Client) error { return boom })
	if !errors.Is(err, boom) {
		t.Errorf("Write = %v", err)
	}
	if e := r.last().Events; len(e) != 1 || !e[0].Own {
		t.Errorf("a failed write must still refresh: %+v", e)
	}
}

func TestUpdatesCoalesceWithoutLosingEvents(t *testing.T) {
	r := newRig(t, rigOpts{notify: true, kinds: model.KindNames()})
	for _, id := range []string{"f-1", "f-2"} {
		setStatus(r, id, "closed")
		r.refresh()
	}
	closed := map[string]int{}
	highlighted := map[string]bool{}
	notified := 0
	var final Update
	guard := time.After(10 * time.Second)
	for len(closed) < 2 {
		select {
		case u := <-r.eng.Updates():
			final = u
			for _, e := range u.Events {
				if e.Kind == model.KindClosed && !e.Prefill {
					closed[e.IssueID]++
				}
			}
			for _, id := range u.Highlights {
				highlighted[id] = true
			}
			notified += len(u.Notification.Events)
		case <-guard:
			t.Fatalf("saw only %v", closed)
		}
	}
	if closed["f-1"] != 1 || closed["f-2"] != 1 || len(highlighted) != 2 || notified != 2 {
		t.Errorf("closed %v highlighted %v notified %d", closed, highlighted, notified)
	}
	if final.Snapshot == nil || !final.Status.Loaded {
		t.Errorf("final = %+v", final)
	}
}

func TestMergedUpdateKeepsLatestStatusAndCapsEvents(t *testing.T) {
	a := Update{Changed: true, Highlights: []string{"x"}, Status: Status{Failures: 1}}
	for i := range 400 {
		a.Events = append(a.Events, model.Event{Seq: int64(i)})
	}
	b := Update{Highlights: []string{"x", "y"}, Status: Status{Failures: 2}}
	for i := range 400 {
		b.Events = append(b.Events, model.Event{Seq: int64(1000 + i)})
	}
	b.Notification = model.Notification{Events: make([]model.Event, 4)}
	a.merge(b)
	if a.Status.Failures != 2 || !a.Changed || len(a.Events) != model.RingCapacity ||
		a.Events[len(a.Events)-1].Seq != 1399 || len(a.Highlights) != 2 || !a.Notification.Summary {
		t.Errorf("merged = %d events, %+v", len(a.Events), a)
	}
}

func TestFollowerStartFailureCountsAsDeath(t *testing.T) {
	r := newRig(t, rigOpts{journal: true, noStart: true})
	r.fake.FailWith("EventsFollow", transient("events tail"))
	r.start()
	if st := r.eng.Status(); st.Following || st.Fallback {
		t.Fatalf("status = %+v", st)
	}
	r.step(2 * time.Second)
	r.step(4 * time.Second)
	if st := r.eng.Status(); !st.Fallback {
		t.Errorf("status = %+v", st)
	}
	if !r.eng.Status().Loaded {
		t.Error("snapshot must load despite the follower")
	}
}
