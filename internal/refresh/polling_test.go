package refresh

import (
	"reflect"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
)

func TestFirstRefreshPublishesSnapshotAndPrefillOnly(t *testing.T) {
	r := newRig(t, rigOpts{notify: true, kinds: model.KindNames()})
	ups := r.updates()
	if len(ups) != 1 {
		t.Fatalf("%d updates", len(ups))
	}
	u := ups[0]
	if u.Snapshot == nil || !u.Changed || u.Snapshot.Len() != 2 {
		t.Fatalf("update = %+v", u)
	}
	if len(u.Events) == 0 {
		t.Fatal("polling prefill missing")
	}
	for _, e := range u.Events {
		if !e.Prefill {
			t.Errorf("live event in first snapshot: %+v", e)
		}
	}
	if len(u.Highlights) != 0 || len(u.Notification.Events) != 0 {
		t.Errorf("first snapshot must not highlight or notify: %+v", u)
	}
	if st := u.Status; st.Mode != Polling || !st.Loaded || st.Stale || st.Following || !st.Focused {
		t.Errorf("status = %+v", st)
	}
	if got := r.fake.Calls(); !reflect.DeepEqual(got, []string{"Version", "Where", "Statuses", "Types", "ConfigGet", "VCStatus", "List", "Ready"}) {
		t.Errorf("calls = %v", got)
	}
	if n := len(r.eng.Events()); n != len(u.Events) {
		t.Errorf("ring holds %d, update %d", n, len(u.Events))
	}
}

func TestGateCadenceAndChangeDetection(t *testing.T) {
	r := newRig(t, rigOpts{})
	base := len(r.updates())

	m := r.mark()
	r.step(time.Second)
	if got := r.since(m); len(got) != 0 {
		t.Fatalf("gate ran after 1s: %v", got)
	}
	r.step(time.Second)
	if got := r.since(m); !reflect.DeepEqual(got, []string{"VCStatus"}) {
		t.Fatalf("gate at 2s: %v", got)
	}
	if len(r.updates()) != base {
		t.Error("an unchanged gate must not publish")
	}

	setStatus(r, "f-1", "closed")
	r.fake.SetVCStatus(bd.VCStatus{Branch: "main", Commit: "c1"})
	m = r.mark()
	r.step(2 * time.Second)
	if got := r.since(m); !reflect.DeepEqual(got, []string{"VCStatus", "List", "Ready"}) {
		t.Fatalf("changed gate: %v", got)
	}
	u := r.last()
	if !u.Changed || !reflect.DeepEqual(kinds(u.Events), []model.Kind{model.KindClosed}) {
		t.Fatalf("update = %+v", u)
	}
	if !reflect.DeepEqual(u.Highlights, []string{"f-1"}) {
		t.Errorf("highlights = %v", u.Highlights)
	}

	m = r.mark()
	r.step(2 * time.Second)
	if got := r.since(m); !reflect.DeepEqual(got, []string{"VCStatus"}) {
		t.Errorf("gate after refresh: %v", got)
	}
}

func TestSafetyPollWithoutHashChange(t *testing.T) {
	r := newRig(t, rigOpts{})
	m := r.mark()
	for range 14 {
		r.step(2 * time.Second)
	}
	if r.count("List") != 1 {
		t.Fatalf("polled in full before 30 s: %v", r.since(m))
	}
	r.step(2 * time.Second)
	if r.count("List") != 2 {
		t.Errorf("no safety poll at 30 s: %v", r.since(m))
	}
}

func TestSafetyIntervalStretchesWithSnapshotDuration(t *testing.T) {
	r := newRig(t, rigOpts{noStart: true})
	r.fake.SetHook(func(method string) {
		if method == "Ready" {
			r.clk.Advance(5 * time.Second)
		}
	})
	r.start()
	if got := r.eng.Status().LastRefresh; got != 5*time.Second {
		t.Fatalf("last refresh = %v", got)
	}
	r.fake.SetHook(nil)
	r.step(45 * time.Second)
	if r.count("List") != 1 {
		t.Fatal("safety poll before 10x the snapshot duration")
	}
	r.step(5 * time.Second)
	if r.count("List") != 2 {
		t.Error("no safety poll at 50 s")
	}
}

func TestSlowBdSignal(t *testing.T) {
	r := newRig(t, rigOpts{noStart: true})
	r.fake.SetHook(func(method string) {
		if method == "Ready" {
			r.clk.Advance(3 * time.Second)
		}
	})
	r.start()
	if !r.eng.Status().Slow || !r.last().Status.Slow {
		t.Fatal("3 s refresh must raise slow bd")
	}
	r.fake.SetHook(nil)
	r.step(50 * time.Second)
	if r.eng.Status().Slow {
		t.Error("slow bd must clear after a fast refresh")
	}
}

func TestRefreshInFlightIsVisible(t *testing.T) {
	r := newRig(t, rigOpts{noStart: true})
	var during time.Time
	r.fake.SetHook(func(method string) {
		if method == "List" {
			during = r.eng.Status().RefreshingSince
		}
	})
	r.start()
	if !during.Equal(t0) {
		t.Errorf("RefreshingSince during the call = %v", during)
	}
	if !r.eng.Status().RefreshingSince.IsZero() {
		t.Error("RefreshingSince must clear")
	}
}

func TestBackoffSchedule(t *testing.T) {
	r := newRig(t, rigOpts{noStart: true})
	r.fake.FailWith("List", transient("list"))
	r.start()
	st := r.eng.Status()
	if st.Loaded || st.Stale || st.Failures != 1 || st.Err == nil {
		t.Fatalf("failed first load: %+v", st)
	}
	want := []time.Duration{2, 4, 8, 16, 32, 60, 60}
	for i, w := range want {
		st = r.eng.Status()
		if got := st.NextRetry.Sub(r.clk.Now()); got != w*time.Second {
			t.Fatalf("failure %d: retry in %v, want %v", i+1, got, w*time.Second)
		}
		m := r.mark()
		r.step(w*time.Second - time.Millisecond)
		if len(r.since(m)) != 0 {
			t.Fatalf("retried early: %v", r.since(m))
		}
		r.step(time.Millisecond)
		if r.count("List") != i+2 {
			t.Fatalf("no retry after %v", w*time.Second)
		}
	}
	r.fake.FailWith("List", nil)
	r.step(60 * time.Second)
	st = r.eng.Status()
	if !st.Loaded || st.Stale || st.Err != nil || st.Failures != 0 || !st.NextRetry.IsZero() {
		t.Errorf("after recovery: %+v", st)
	}
	if u := r.last(); u.Snapshot == nil || !u.Changed {
		t.Errorf("recovery update: %+v", u)
	}
}

func TestStaleKeepsLastGoodSnapshot(t *testing.T) {
	r := newRig(t, rigOpts{})
	good := r.last().Snapshot
	r.fake.FailWith("Ready", transient("ready"))
	r.step(30 * time.Second)
	u := r.last()
	if !u.Status.Stale || u.Snapshot != good || u.Changed || u.Status.Err == nil {
		t.Fatalf("stale update: %+v", u)
	}
	if !u.Status.LastSuccess.Equal(t0) {
		t.Errorf("last success = %v", u.Status.LastSuccess)
	}
	r.step(2 * time.Second)
	if st := r.eng.Status(); st.Failures != 2 || !st.LastSuccess.Equal(t0) {
		t.Errorf("status = %+v", st)
	}
	r.fake.FailWith("Ready", nil)
	r.step(4 * time.Second)
	if st := r.eng.Status(); st.Stale || st.Err != nil {
		t.Errorf("stale did not clear: %+v", st)
	}
}

func TestBackoffSuspendsGateAndManualRefreshBypassesIt(t *testing.T) {
	r := newRig(t, rigOpts{})
	r.fake.FailWith("List", transient("list"))
	r.step(30 * time.Second)
	if r.eng.Status().Failures != 1 {
		t.Fatalf("status = %+v", r.eng.Status())
	}
	r.fake.FailWith("List", nil)
	m := r.mark()
	r.refresh()
	if got := r.since(m); len(got) < 3 || got[len(got)-2] != "List" {
		t.Fatalf("manual refresh calls: %v", got)
	}
	if st := r.eng.Status(); st.Stale || st.Failures != 0 {
		t.Errorf("manual refresh did not recover: %+v", st)
	}
}

func TestManualRefreshReloadsMetaAndBypassesGate(t *testing.T) {
	r := newRig(t, rigOpts{})
	m := r.mark()
	r.refresh()
	got := r.since(m)
	want := []string{"Statuses", "Types", "VCStatus", "List", "Ready"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestVanishedWorkspaceEscalatesAfterThreeFailures(t *testing.T) {
	r := newRig(t, rigOpts{})
	gone := &bd.Error{Class: bd.ClassTransient, Command: "list", ExitCode: 1, Code: bd.CodeNoBeadsDirectory}
	r.fake.FailWith("List", gone)
	for range 2 {
		r.refresh()
		if bd.IsClass(r.eng.Status().Err, bd.ClassNotWorkspace) {
			t.Fatal("escalated too early")
		}
	}
	r.refresh()
	if !bd.IsClass(r.eng.Status().Err, bd.ClassNotWorkspace) {
		t.Errorf("err = %v", r.eng.Status().Err)
	}
	r.fake.FailWith("List", nil)
	r.refresh()
	if r.eng.Status().Err != nil {
		t.Error("success must clear the error")
	}
}

func TestReadFailureIsClassifiedInTheSameSlot(t *testing.T) {
	r := newRig(t, rigOpts{})
	r.fake.FailWith("List", transient("list"))
	r.fake.FailWith("Where", &bd.Error{Class: bd.ClassNotWorkspace, Command: "where"})
	m := r.mark()
	r.refresh()
	got := r.since(m)
	want := []string{"Statuses", "Types", "VCStatus", "List", "Where"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	if bd.CodeOf(r.eng.Status().Err) != bd.CodeNoBeadsDirectory {
		t.Errorf("err = %v", r.eng.Status().Err)
	}
}

func TestOwnWritesAreMarkedAndNeverNotify(t *testing.T) {
	r := newRig(t, rigOpts{notify: true, kinds: model.KindNames()})
	setStatus(r, "f-1", "closed")
	setIssue(r, "f-2", func(is *model.Issue) { is.Priority = 0 })
	r.write("f-1")
	u := r.last()
	byIssue := map[string]model.Event{}
	for _, e := range u.Events {
		byIssue[e.IssueID] = e
	}
	own, other := byIssue["f-1"], byIssue["f-2"]
	if !own.Own || own.Actor != "me" || own.Kind != model.KindClosed {
		t.Errorf("own event = %+v", own)
	}
	if other.Own || other.Actor != "" {
		t.Errorf("other event = %+v", other)
	}
	if !reflect.DeepEqual(u.Highlights, []string{"f-1", "f-2"}) {
		t.Errorf("highlights = %v", u.Highlights)
	}
	if len(u.Notification.Events) != 1 || u.Notification.Events[0].IssueID != "f-2" {
		t.Errorf("notification = %+v", u.Notification)
	}

	setStatus(r, "f-2", "closed")
	r.refresh()
	if e := r.last().Events; len(e) != 1 || e[0].Own {
		t.Errorf("own marks must not outlive the refresh: %+v", e)
	}
}

func TestOwnMarksSurviveAFailedRefresh(t *testing.T) {
	r := newRig(t, rigOpts{})
	setStatus(r, "f-1", "closed")
	r.fake.FailWith("Ready", transient("ready"))
	r.write("f-1")
	r.fake.FailWith("Ready", nil)
	r.refresh()
	if e := r.last().Events; len(e) != 1 || !e[0].Own {
		t.Errorf("events = %+v", e)
	}
}

func TestNotificationsFollowSettingsAndSummarise(t *testing.T) {
	r := newRig(t, rigOpts{notify: true, kinds: []string{"closed"}})
	for _, id := range []string{"f-1", "f-2"} {
		setStatus(r, id, "closed")
	}
	r.refresh()
	if n := r.last().Notification; len(n.Events) != 2 || n.Summary {
		t.Errorf("notification = %+v", n)
	}

	r2 := newRig(t, rigOpts{notify: false, kinds: model.KindNames()})
	setStatus(r2, "f-1", "closed")
	r2.refresh()
	if n := r2.last().Notification; len(n.Events) != 0 {
		t.Errorf("notifications off: %+v", n)
	}
	r2.setNotify(true, model.KindSetOf("closed"))
	setStatus(r2, "f-2", "closed")
	r2.refresh()
	if n := r2.last().Notification; len(n.Events) != 1 {
		t.Errorf("after SetNotify: %+v", n)
	}
}

func TestFocusPausesPollingWithoutNotifications(t *testing.T) {
	r := newRig(t, rigOpts{})
	r.focus(false)
	st := r.eng.Status()
	if st.Focused || !st.Paused {
		t.Fatalf("status = %+v", st)
	}
	m := r.mark()
	r.step(10 * time.Minute)
	if got := r.since(m); len(got) != 0 {
		t.Fatalf("polled while paused: %v", got)
	}
	setStatus(r, "f-1", "closed")
	r.focus(true)
	got := r.since(m)
	if !reflect.DeepEqual(got, []string{"VCStatus", "List", "Ready"}) {
		t.Fatalf("focus regain: %v", got)
	}
	if u := r.last(); !u.Status.Focused || u.Status.Paused || len(u.Events) != 1 {
		t.Errorf("update = %+v", u)
	}
	m = r.mark()
	r.focus(true)
	if len(r.since(m)) != 0 {
		t.Error("focusing a focused terminal must do nothing")
	}
}

func TestFocusSlowsPollingWhenNotificationsAreOn(t *testing.T) {
	r := newRig(t, rigOpts{notify: true})
	r.focus(false)
	m := r.mark()
	r.step(8 * time.Second)
	if len(r.since(m)) != 0 {
		t.Fatalf("gate before 5x: %v", r.since(m))
	}
	r.step(2 * time.Second)
	if got := r.since(m); !reflect.DeepEqual(got, []string{"VCStatus"}) {
		t.Fatalf("gate at 10 s: %v", got)
	}
	for range 13 {
		r.step(10 * time.Second)
	}
	if r.count("List") != 1 {
		t.Fatal("safety poll before 150 s")
	}
	r.step(10 * time.Second)
	if r.count("List") != 2 {
		t.Error("no safety poll at 150 s")
	}
}

func TestManualRefreshWorksWhilePaused(t *testing.T) {
	r := newRig(t, rigOpts{})
	r.focus(false)
	m := r.mark()
	r.refresh()
	if len(r.since(m)) == 0 {
		t.Error("manual refresh ignored while paused")
	}
}

func TestGCHint(t *testing.T) {
	cost := func(d time.Duration) func(*rig) {
		return func(r *rig) {
			r.fake.SetHook(func(method string) {
				if method == "VCStatus" {
					r.clk.Advance(d)
				}
			})
		}
	}
	t.Run("floor", func(t *testing.T) {
		r := newRig(t, rigOpts{})
		cost(600 * time.Millisecond)(r)
		for range 6 {
			r.step(3 * time.Second)
		}
		if !r.eng.Status().GCHint {
			t.Error("median 0.6 s must raise the gc hint")
		}
	})
	t.Run("ratio", func(t *testing.T) {
		r := newRig(t, rigOpts{noStart: true})
		cost(20 * time.Millisecond)(r)
		r.start()
		cost(150 * time.Millisecond)(r)
		for range 8 {
			r.step(3 * time.Second)
		}
		if !r.eng.Status().GCHint {
			t.Error("5x the first sample must raise the gc hint")
		}
	})
	t.Run("fast stays quiet", func(t *testing.T) {
		r := newRig(t, rigOpts{})
		cost(100 * time.Millisecond)(r)
		for range 6 {
			r.step(3 * time.Second)
		}
		if r.eng.Status().GCHint {
			t.Error("fast vc status must not hint")
		}
	})
	t.Run("clears only with hysteresis", func(t *testing.T) {
		r := newRig(t, rigOpts{})
		cost(700 * time.Millisecond)(r)
		for range 8 {
			r.step(3 * time.Second)
		}
		if !r.eng.Status().GCHint {
			t.Fatal("hint not raised")
		}
		cost(300 * time.Millisecond)(r)
		for range 12 {
			r.step(3 * time.Second)
		}
		if !r.eng.Status().GCHint {
			t.Fatal("median between half and full threshold must keep the hint")
		}
		cost(0)(r)
		for range 12 {
			r.step(3 * time.Second)
		}
		if r.eng.Status().GCHint {
			t.Error("hint must clear once the median is below half the threshold")
		}
	})
}

func TestHashUnreliablePollsInFullAtThePlainInterval(t *testing.T) {
	t.Run("empty commit", func(t *testing.T) {
		r := newRig(t, rigOpts{})
		r.fake.SetVCStatus(bd.VCStatus{Branch: "main"})
		r.step(2 * time.Second)
		if !r.eng.Status().HashUnreliable {
			t.Fatal("an empty commit means the hash cannot be trusted")
		}
		m := r.mark()
		r.step(4 * time.Second)
		if len(r.since(m)) != 0 {
			t.Fatalf("no gate expected: %v", r.since(m))
		}
		r.step(time.Second)
		if got := r.since(m); !reflect.DeepEqual(got, []string{"List", "Ready"}) {
			t.Errorf("plain poll: %v", got)
		}
		m = r.mark()
		r.step(5 * time.Second)
		if len(r.since(m)) != 2 {
			t.Errorf("plain poll cadence: %v", r.since(m))
		}
	})
	t.Run("gate failing twice while refreshes work", func(t *testing.T) {
		r := newRig(t, rigOpts{})
		r.fake.FailWith("VCStatus", transient("vc status"))
		r.step(2 * time.Second)
		if r.eng.Status().HashUnreliable {
			t.Fatal("one failed gate is not enough")
		}
		r.step(2 * time.Second)
		if !r.eng.Status().HashUnreliable {
			t.Error("two failed gates with working refreshes")
		}
	})
	t.Run("safety polls keep finding changes under a still hash", func(t *testing.T) {
		r := newRig(t, rigOpts{})
		for range 2 {
			setIssue(r, "f-1", func(is *model.Issue) { is.Priority++ })
			r.step(30 * time.Second)
		}
		if !r.eng.Status().HashUnreliable {
			t.Error("hash never moved yet safety polls found changes twice")
		}
	})
	t.Run("a moving hash stays reliable", func(t *testing.T) {
		r := newRig(t, rigOpts{})
		for i := range 4 {
			setIssue(r, "f-1", func(is *model.Issue) { is.Priority++ })
			r.fake.SetVCStatus(bd.VCStatus{Branch: "main", Commit: "c" + string(rune('1'+i))})
			r.step(2 * time.Second)
		}
		if r.eng.Status().HashUnreliable {
			t.Error("hash tracked every change")
		}
	})
}

func TestHashReprobeRestoresTheGate(t *testing.T) {
	r := newRig(t, rigOpts{})
	r.fake.SetVCStatus(bd.VCStatus{Branch: "main"})
	r.step(2 * time.Second)
	if !r.eng.Status().HashUnreliable {
		t.Fatal("not unreliable")
	}
	r.fake.SetVCStatus(bd.VCStatus{Branch: "main", Commit: "ok"})
	for range 11 {
		r.step(5 * time.Second)
	}
	if !r.eng.Status().HashUnreliable {
		t.Fatal("re-probed too early")
	}
	r.step(5 * time.Second)
	if r.eng.Status().HashUnreliable {
		t.Fatal("a working vc status must restore the gate")
	}
	m := r.mark()
	r.step(2 * time.Second)
	if got := r.since(m); len(got) != 1 || got[0] != "VCStatus" {
		t.Errorf("gate after restore: %v", got)
	}
}
