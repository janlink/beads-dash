package refresh

import (
	"errors"
	"io"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
)

func rec(op, id, actor string, at time.Time, is *model.Issue) bd.Event {
	return bd.Event{Op: op, IssueID: id, Actor: actor, Time: at, Issue: is}
}

func ptr(is model.Issue) *model.Issue { return &is }

func newEventsRig(t *testing.T, o rigOpts) *rig {
	t.Helper()
	o.journal = true
	return newRig(t, o)
}

func TestEventsModeStartsFollowerAfterFirstSnapshot(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	if got := r.fake.FollowStarts(); !reflect.DeepEqual(got, []int64{0}) {
		t.Fatalf("follow starts = %v", got)
	}
	st := r.eng.Status()
	if st.Mode != Events || !st.Following || st.Fallback {
		t.Fatalf("status = %+v", st)
	}
	calls := r.fake.Calls()
	if i := slices.Index(calls, "EventsFollow"); i < slices.Index(calls, "Ready") {
		t.Errorf("follower started before the first snapshot: %v", calls)
	}
	if n := len(r.updates()[0].Events); n != 0 {
		t.Errorf("events mode prefill comes from the journal, got %d timestamp events", n)
	}
}

func TestJournalPolling1_2_2HasNoFollower(t *testing.T) {
	r := newRig(t, rigOpts{})
	if r.count("EventsFollow") != 0 || r.eng.Status().Mode != Polling {
		t.Error("no journal, no follower")
	}
}

func TestJournalHistoryBecomesPrefillWithActors(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	old := t0.Add(-time.Hour)
	created := issue("f-1", "One", "open")
	claimed := created
	claimed.Status, claimed.Assignee = "in_progress", "alice"
	r.fake.Emit(
		rec("create", "f-1", "bob", old, &created),
		rec("update", "f-1", "alice", old.Add(time.Minute), &claimed),
	)
	r.eng.barrier()
	if len(r.updates()) != 1 {
		t.Fatal("prefill must be batched, not published per record")
	}
	r.step(300 * time.Millisecond)
	u := r.last()
	if len(u.Events) != 2 || u.Changed || len(u.Highlights) != 0 || len(u.Notification.Events) != 0 {
		t.Fatalf("update = %+v", u)
	}
	for _, e := range u.Events {
		if !e.Prefill {
			t.Errorf("history event not marked prefill: %+v", e)
		}
	}
	if u.Events[0].Kind != model.KindCreated || u.Events[0].Actor != "bob" ||
		u.Events[1].Kind != model.KindClaimed || u.Events[1].Actor != "alice" {
		t.Errorf("events = %+v", u.Events)
	}
	if got := r.eng.Events(); len(got) != 2 || got[0].Kind != model.KindClaimed {
		t.Errorf("ring = %+v", got)
	}
	m := r.mark()
	r.step(time.Second)
	if r.count("List") != 1 || len(r.since(m)) != 0 {
		t.Errorf("history records must not trigger a refresh: %v", r.since(m))
	}
}

func TestLiveRecordTriggersDebouncedRefreshWithActor(t *testing.T) {
	r := newEventsRig(t, rigOpts{notify: true, kinds: model.KindNames()})
	setIssue(r, "f-1", func(is *model.Issue) { is.Status, is.Assignee = "in_progress", "carol" })
	after := ptr(r.fake.Issues()[0])
	r.fake.Emit(rec("update", "f-1", "carol", t0.Add(50*time.Millisecond), after))
	r.eng.barrier()
	m := r.mark()

	r.step(200 * time.Millisecond)
	if len(r.since(m)) != 0 {
		t.Fatal("refreshed before the debounce elapsed")
	}
	r.step(100 * time.Millisecond)
	if got := r.since(m); !reflect.DeepEqual(got, []string{"VCStatus", "List", "Ready"}) {
		t.Fatalf("calls = %v", got)
	}
	u := r.last()
	if len(u.Events) != 1 {
		t.Fatalf("events = %+v", u.Events)
	}
	e := u.Events[0]
	if e.Kind != model.KindClaimed || e.Actor != "carol" || e.Prefill || e.Seq == 0 {
		t.Errorf("event = %+v", e)
	}
	if !reflect.DeepEqual(u.Highlights, []string{"f-1"}) {
		t.Errorf("highlights = %v", u.Highlights)
	}
}

func TestRecordBurstMakesOneRefresh(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	setIssue(r, "f-1", func(is *model.Issue) { is.Priority = 0 })
	setIssue(r, "f-2", func(is *model.Issue) { is.Priority = 1 })
	for i, id := range []string{"f-1", "f-2", "f-1"} {
		r.fake.Emit(rec("update", id, "dave", t0.Add(time.Duration(i+1)*10*time.Millisecond), ptr(issue(id, "x", "open"))))
	}
	r.eng.barrier()
	r.step(300 * time.Millisecond)
	if r.count("List") != 2 {
		t.Errorf("%d full refreshes for one burst", r.count("List")-1)
	}
	u := r.last()
	if len(u.Events) != 2 {
		t.Fatalf("events = %+v", u.Events)
	}
	for _, e := range u.Events {
		if e.Kind != model.KindPriorityChanged || e.Actor != "dave" {
			t.Errorf("event = %+v", e)
		}
	}
}

func TestRecordDuringRefreshKeepsItsAttribution(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	setIssue(r, "f-1", func(is *model.Issue) { is.Priority = 0 })
	r.fake.Emit(rec("update", "f-1", "erin", t0.Add(10*time.Millisecond), ptr(issue("f-1", "x", "open"))))
	r.eng.barrier()
	r.step(300 * time.Millisecond)
	later := t0.Add(400 * time.Millisecond)
	r.fake.Emit(rec("update", "f-2", "frank", later, ptr(issue("f-2", "x", "open"))))
	setIssue(r, "f-2", func(is *model.Issue) { is.Priority = 4 })
	r.eng.barrier()
	r.step(300 * time.Millisecond)
	u := r.last()
	if len(u.Events) != 1 || u.Events[0].IssueID != "f-2" || u.Events[0].Actor != "frank" {
		t.Errorf("update = %+v", u.Events)
	}
}

func TestDerivedEffectGetsNoActor(t *testing.T) {
	r := newEventsRig(t, rigOpts{notify: true, kinds: model.KindNames()})
	blocker := issue("f-1", "Blocker", "open")
	blocked := issue("f-2", "Blocked", "open")
	blocked.Dependencies = []model.Edge{{From: "f-2", To: "f-1", Type: "blocks"}}
	r.fake.SetIssues(blocker, blocked)
	r.fake.SetReadiness([]string{"f-1"}, map[string][]string{"f-2": {"f-1"}})
	r.refresh()

	setStatus(r, "f-1", "closed")
	r.fake.SetReadiness([]string{"f-2"}, nil)
	closed := ptr(r.fake.Issues()[0])
	r.fake.Emit(
		rec("close", "f-1", "gina", t0.Add(time.Second), closed),
		rec("update", "f-2", "", t0.Add(time.Second), ptr(issue("f-2", "Blocked", "open"))),
	)
	r.eng.barrier()
	r.step(300 * time.Millisecond)
	byKind := map[model.Kind]model.Event{}
	for _, e := range r.last().Events {
		byKind[e.Kind] = e
	}
	if e := byKind[model.KindClosed]; e.Actor != "gina" || e.IssueID != "f-1" {
		t.Errorf("closed = %+v", e)
	}
	if e := byKind[model.KindBecameReady]; e.Actor != "" || e.IssueID != "f-2" {
		t.Errorf("became ready = %+v", e)
	}
	if n := r.last().Notification; len(n.Events) != 3 {
		t.Errorf("a derived effect must notify: %+v", n)
	}
}

func TestEventsModeGateAndSafetyCadence(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	m := r.mark()
	r.step(9 * time.Second)
	if len(r.since(m)) != 0 {
		t.Fatalf("gate before 10 s: %v", r.since(m))
	}
	r.step(time.Second)
	if got := r.since(m); !reflect.DeepEqual(got, []string{"VCStatus"}) {
		t.Fatalf("gate at 10 s: %v", got)
	}
	for range 10 {
		r.step(10 * time.Second)
	}
	if r.count("List") != 1 {
		t.Fatal("safety poll before 120 s")
	}
	r.step(10 * time.Second)
	if r.count("List") != 2 {
		t.Error("no safety poll at 120 s")
	}
}

func TestEventsModeGateSeesUnjournaledChange(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	setStatus(r, "f-1", "closed")
	r.fake.SetVCStatus(bd.VCStatus{Branch: "main", Commit: "pulled"})
	r.step(10 * time.Second)
	u := r.last()
	if !reflect.DeepEqual(kinds(u.Events), []model.Kind{model.KindClosed}) || u.Events[0].Actor != "" {
		t.Errorf("events = %+v", u.Events)
	}
}

func TestFollowerRestartsFromLastSeqWithBackoff(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	old := t0.Add(-time.Hour)
	r.fake.Emit(rec("create", "f-1", "bob", old, ptr(issue("f-1", "One", "open"))))
	r.eng.barrier()

	boom := errors.New("pipe closed")
	r.fake.KillFollowers(boom)
	r.eng.barrier()
	if r.eng.Status().Following {
		t.Fatal("follower should be gone")
	}
	if r.fake.OpenFollowers() != 0 {
		t.Fatal("dead stream must be closed")
	}
	lists := r.count("List")
	r.step(1900 * time.Millisecond)
	if r.count("EventsFollow") != 1 {
		t.Fatal("restarted before 2 s")
	}
	r.step(100 * time.Millisecond)
	if got := r.fake.FollowStarts(); !reflect.DeepEqual(got, []int64{0, 1}) {
		t.Fatalf("follow starts = %v", got)
	}
	if r.count("List") != lists+1 {
		t.Error("a restart must come with a full refresh")
	}
	if !r.eng.Status().Following {
		t.Error("not following after restart")
	}

	r.fake.KillFollowers(boom)
	r.eng.barrier()
	r.step(3900 * time.Millisecond)
	if r.count("EventsFollow") != 2 {
		t.Fatal("second restart before 4 s")
	}
	r.step(100 * time.Millisecond)
	if r.count("EventsFollow") != 3 {
		t.Error("no second restart at 4 s")
	}
}

func TestDeliveredRecordsResetTheDeathCount(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	boom := errors.New("died")
	for i := range 5 {
		r.fake.Emit(rec("update", "f-1", "x", t0.Add(-time.Hour), ptr(issue("f-1", "One", "open"))))
		r.fake.KillFollowers(boom)
		r.eng.barrier()
		r.step(2 * time.Second)
		if st := r.eng.Status(); st.Fallback || !st.Following {
			t.Fatalf("round %d: %+v", i, st)
		}
	}
}

func TestLongLivedFollowerResetsTheDeathCount(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	boom := errors.New("died")
	for range 4 {
		r.step(time.Minute)
		r.fake.KillFollowers(boom)
		r.eng.barrier()
		r.step(2 * time.Second)
		if r.eng.Status().Fallback {
			t.Fatal("stable followers must not count towards the fallback")
		}
	}
}

func TestThreeQuickDeathsFallBackToPolling(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	boom := errors.New("died")
	r.fake.KillFollowers(boom)
	r.eng.barrier()
	r.step(2 * time.Second)
	r.fake.KillFollowers(boom)
	r.eng.barrier()
	r.step(4 * time.Second)
	r.fake.KillFollowers(boom)
	r.eng.barrier()
	st := r.eng.Status()
	if !st.Fallback || st.Following || st.Mode != Events {
		t.Fatalf("status = %+v", st)
	}
	starts := r.count("EventsFollow")
	m := r.mark()
	r.step(2 * time.Second)
	if got := r.since(m); len(got) == 0 || got[len(got)-1] == "EventsFollow" {
		t.Errorf("after fallback the 2 s polling gate applies: %v", got)
	}
	r.step(time.Hour)
	if r.count("EventsFollow") != starts {
		t.Error("follower restarted after the fallback")
	}
	setStatus(r, "f-1", "closed")
	r.fake.SetVCStatus(bd.VCStatus{Branch: "main", Commit: "c9"})
	r.step(2 * time.Second)
	if len(r.last().Events) != 1 {
		t.Error("polling fallback missed a change")
	}
}

func TestCleanExitCountsAsDeath(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	r.fake.FailWith("EventsFollow", &bd.Error{Class: bd.ClassTransient, Command: "events tail"})
	r.fake.KillFollowers(io.EOF)
	r.eng.barrier()
	r.step(2 * time.Second)
	r.step(4 * time.Second)
	if st := r.eng.Status(); !st.Fallback {
		t.Errorf("journal that cannot be followed must fall back: %+v", st)
	}
}

func TestTruncatedJournalResumesAtTheFloorAtOnce(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	lists := r.count("List")
	r.fake.Truncate(50, 90)
	r.eng.barrier()
	if got := r.fake.FollowStarts(); !reflect.DeepEqual(got, []int64{0, 49}) {
		t.Fatalf("follow starts = %v", got)
	}
	if r.count("List") != lists+1 {
		t.Error("a truncated journal needs a full refresh")
	}
	if st := r.eng.Status(); !st.Following || st.Fallback {
		t.Errorf("status = %+v", st)
	}
}

func TestSeqResetRestartsFollowerAtZero(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	r.fake.Emit(rec("update", "f-1", "x", t0.Add(-time.Hour), ptr(issue("f-1", "One", "open"))))
	r.eng.barrier()
	change := func(i int) {
		setIssue(r, "f-1", func(is *model.Issue) { is.Priority = i })
		r.fake.SetVCStatus(bd.VCStatus{Branch: "main", Commit: "s" + string(rune('0'+i))})
	}
	change(0)
	r.step(10 * time.Second)
	if r.count("EventsFollow") != 1 {
		t.Fatal("one silent change is not a reset")
	}
	change(1)
	r.step(10 * time.Second)
	if got := r.fake.FollowStarts(); !reflect.DeepEqual(got, []int64{0, 0}) {
		t.Fatalf("follow starts = %v", got)
	}
	if !r.eng.Status().Following {
		t.Error("follower not running after the reset restart")
	}
}

func TestAnnouncedChangesDoNotLookLikeASeqReset(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	for i := range 4 {
		setIssue(r, "f-1", func(is *model.Issue) { is.Priority = i })
		r.fake.Emit(rec("update", "f-1", "x", r.clk.Now().Add(time.Millisecond), ptr(issue("f-1", "One", "open"))))
		r.eng.barrier()
		r.step(time.Second)
	}
	if r.count("EventsFollow") != 1 {
		t.Errorf("follow starts = %v", r.fake.FollowStarts())
	}
}

func TestReplayReachingTheOldSeqDisablesFurtherResets(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	r.fake.Emit(rec("update", "f-1", "x", t0.Add(-time.Hour), ptr(issue("f-1", "One", "open"))))
	r.eng.barrier()
	silent := func(c string, i int) {
		setIssue(r, "f-1", func(is *model.Issue) { is.Priority = i })
		r.fake.SetVCStatus(bd.VCStatus{Branch: "main", Commit: c})
		r.step(10 * time.Second)
	}
	silent("a1", 0)
	silent("a2", 1)
	if r.count("EventsFollow") != 2 {
		t.Fatalf("follow starts = %v", r.fake.FollowStarts())
	}
	r.fake.DrainFollowers()
	r.eng.barrier()
	for i := range 4 {
		silent("b"+string(rune('0'+i)), 10+i)
	}
	if r.count("EventsFollow") != 2 {
		t.Errorf("resets went on after the replay proved the journal intact: %v", r.fake.FollowStarts())
	}
}

func TestRecordArrivalClearsSeqResetSuspicion(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	r.fake.Emit(rec("update", "f-1", "x", t0.Add(-time.Hour), ptr(issue("f-1", "One", "open"))))
	r.eng.barrier()
	setIssue(r, "f-1", func(is *model.Issue) { is.Priority = 0 })
	r.fake.SetVCStatus(bd.VCStatus{Branch: "main", Commit: "z1"})
	r.step(10 * time.Second)
	r.fake.Emit(rec("update", "f-2", "y", r.clk.Now().Add(time.Millisecond), ptr(issue("f-2", "Two", "open"))))
	r.eng.barrier()
	r.step(300 * time.Millisecond)
	setIssue(r, "f-1", func(is *model.Issue) { is.Priority = 1 })
	r.fake.SetVCStatus(bd.VCStatus{Branch: "main", Commit: "z2"})
	r.step(10 * time.Second)
	if r.count("EventsFollow") != 1 {
		t.Errorf("suspicion survived a delivered record: %v", r.fake.FollowStarts())
	}
}

func TestJournalCatchUpOf100kRecordsStaysBounded(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	const total = 100_000
	recs := make([]bd.Event, total)
	for i := range recs {
		id := "h-" + string(rune('a'+i%26)) + string(rune('a'+i/26%26))
		op := "update"
		if i < 1000 {
			op = "create"
		}
		recs[i] = rec(op, id, "hist", t0.Add(-48*time.Hour+time.Duration(i)*time.Second), ptr(issue(id, "H", "open")))
	}
	began := time.Now()
	r.fake.Emit(recs...)
	r.eng.barrier()
	r.step(300 * time.Millisecond)
	took := time.Since(began)
	if took > 30*time.Second {
		t.Errorf("catch-up of %d records took %v", total, took)
	}
	if n := len(r.eng.Events()); n > model.RingCapacity {
		t.Errorf("ring holds %d", n)
	}
	for _, u := range r.updates() {
		if len(u.Events) > model.RingCapacity {
			t.Fatalf("one update carried %d events", len(u.Events))
		}
	}
	if len(r.eng.Events()) == 0 {
		t.Error("no prefill")
	}
	if got := r.count("List"); got != 1 {
		t.Errorf("history triggered %d refreshes", got-1)
	}
}

func TestSeqResetDoesNotDuplicateHistory(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	r.fake.Emit(rec("create", "f-1", "x", t0.Add(-time.Hour), ptr(issue("f-1", "One", "open"))))
	r.eng.barrier()
	r.step(300 * time.Millisecond)
	before := len(r.eng.Events())
	for i := range 2 {
		setIssue(r, "f-1", func(is *model.Issue) { is.Priority = i })
		r.fake.SetVCStatus(bd.VCStatus{Branch: "main", Commit: "d" + string(rune('0'+i))})
		r.step(10 * time.Second)
	}
	r.fake.Emit(rec("create", "f-1", "x", t0.Add(-time.Hour), ptr(issue("f-1", "One", "open"))))
	r.eng.barrier()
	r.step(300 * time.Millisecond)
	prefill := 0
	for _, e := range r.eng.Events() {
		if e.Prefill {
			prefill++
		}
	}
	if prefill != before {
		t.Errorf("%d prefill events, was %d", prefill, before)
	}
}

func TestOwnWriteInEventsModeKeepsJournalActor(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	setStatus(r, "f-1", "closed")
	r.fake.Emit(rec("close", "f-1", "me-from-journal", t0.Add(time.Millisecond), ptr(r.fake.Issues()[0])))
	r.write("f-1")
	e := r.last().Events[0]
	if !e.Own || e.Actor != "me-from-journal" {
		t.Errorf("event = %+v", e)
	}
}

func TestStopClosesFollower(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	r.eng.Stop()
	if r.fake.OpenFollowers() != 0 {
		t.Error("follower outlived the engine")
	}
	r.eng.Stop()
}

func TestPausedEventsModeDefersRefreshUntilFocus(t *testing.T) {
	r := newEventsRig(t, rigOpts{})
	r.focus(false)
	setIssue(r, "f-1", func(is *model.Issue) { is.Priority = 0 })
	r.fake.Emit(rec("update", "f-1", "zed", t0.Add(time.Millisecond), ptr(issue("f-1", "One", "open"))))
	r.eng.barrier()
	m := r.mark()
	r.step(time.Hour)
	if len(r.since(m)) != 0 {
		t.Fatalf("paused engine refreshed: %v", r.since(m))
	}
	r.focus(true)
	u := r.last()
	if len(u.Events) != 1 || u.Events[0].Actor != "zed" {
		t.Errorf("events = %+v", u.Events)
	}
}

func TestUnfocusedWithNotificationsStillRefreshesOnRecords(t *testing.T) {
	r := newEventsRig(t, rigOpts{notify: true})
	r.focus(false)
	setIssue(r, "f-1", func(is *model.Issue) { is.Priority = 0 })
	r.fake.Emit(rec("update", "f-1", "zed", t0.Add(time.Millisecond), ptr(issue("f-1", "One", "open"))))
	r.eng.barrier()
	r.step(300 * time.Millisecond)
	if len(r.last().Events) != 1 {
		t.Error("the journal keeps notifications live while blurred")
	}
	m := r.mark()
	r.step(40 * time.Second)
	if len(r.since(m)) != 0 {
		t.Errorf("gate ran before 5x10 s: %v", r.since(m))
	}
	r.step(10 * time.Second)
	if got := r.since(m); !reflect.DeepEqual(got, []string{"VCStatus"}) {
		t.Errorf("blurred gate: %v", got)
	}
}
