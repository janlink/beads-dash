package model_test

import (
	"testing"

	"github.com/janlink/beads-dash/internal/model"
)

func rec(seq int64, op, id, actor string, is *model.Issue) model.JournalRecord {
	return model.JournalRecord{Seq: seq, Time: t0.Add(secs(seq)), Op: op, IssueID: id, Actor: actor, Issue: is}
}

func TestJournalTrackerClassifiesRecords(t *testing.T) {
	tr := model.NewJournalTracker(model.Statuses{})
	base := issue("a", "open")
	claimed := with(base, status("in_progress"), assignee("ann"))
	edited := with(claimed, title("renamed"), priority(1))

	steps := []struct {
		r    model.JournalRecord
		want []model.Kind
	}{
		{rec(1, model.OpCreate, "a", "ann", &base), []model.Kind{model.KindCreated}},
		{rec(2, model.OpUpdate, "a", "ann", &claimed), []model.Kind{model.KindClaimed}},
		{rec(3, model.OpUpdate, "a", "bob", &edited), []model.Kind{model.KindPriorityChanged, model.KindEdited}},
		{rec(4, model.OpComment, "a", "bob", &edited), []model.Kind{model.KindCommented}},
		{rec(5, model.OpClose, "a", "ann", &edited), []model.Kind{model.KindClosed}},
		{rec(6, model.OpDelete, "a", "ann", nil), []model.Kind{model.KindDeleted}},
	}
	for _, s := range steps {
		evs := tr.Apply(s.r)
		wantKinds(t, evs, s.want...)
		for _, e := range evs {
			if !e.Prefill || e.Actor != s.r.Actor || e.Seq != s.r.Seq || !e.Time.Equal(s.r.Time) {
				t.Errorf("seq %d: event %+v", s.r.Seq, e)
			}
		}
	}
}

func TestJournalTrackerDeleteKeepsPriorTitle(t *testing.T) {
	tr := model.NewJournalTracker(model.Statuses{})
	base := issue("a", "open")
	tr.Apply(rec(1, model.OpCreate, "a", "ann", &base))
	evs := tr.Apply(rec(2, model.OpDelete, "a", "ann", nil))
	if len(evs) != 1 || evs[0].Title != "t a" {
		t.Errorf("events = %+v", evs)
	}
}

func TestJournalTrackerActorlessRecordsUpdateStateSilently(t *testing.T) {
	tr := model.NewJournalTracker(model.Statuses{})
	base := issue("a", "open")
	closed := with(base, status("closed"))
	tr.Apply(rec(1, model.OpCreate, "a", "ann", &base))
	wantKinds(t, tr.Apply(rec(2, model.OpUpdate, "a", "", &closed)))
	reopened := with(base, status("open"))
	wantKinds(t, tr.Apply(rec(3, model.OpUpdate, "a", "ann", &reopened)), model.KindReopened)
}

func TestJournalTrackerUpdateWithoutHistoryYieldsNothing(t *testing.T) {
	tr := model.NewJournalTracker(model.Statuses{})
	is := issue("a", "in_progress")
	wantKinds(t, tr.Apply(rec(9, model.OpUpdate, "a", "ann", &is)))
	wantKinds(t, tr.Apply(rec(10, model.OpUpdate, "a", "ann", nil)))
	wantKinds(t, tr.Apply(rec(11, "future_op", "a", "ann", &is)))
}

func TestJournalTrackerBlockedFlag(t *testing.T) {
	tr := model.NewJournalTracker(model.Statuses{})
	is := issue("a", "open")
	tr.Apply(rec(1, model.OpCreate, "a", "ann", &is))
	r := rec(2, model.OpDepAdd, "a", "ann", &is)
	r.Blocked = true
	wantKinds(t, tr.Apply(r), model.KindBecameBlocked)
	r = rec(3, model.OpDepRemove, "a", "ann", &is)
	wantKinds(t, tr.Apply(r), model.KindUnblocked)
}

func TestAttributeActors(t *testing.T) {
	is := issue("a", "open")
	records := []model.JournalRecord{
		rec(1, model.OpUpdate, "a", "old", &is),
		rec(2, model.OpUpdate, "a", "ann", &is),
		rec(3, model.OpUpdate, "a", "", &is),
		rec(4, model.OpClose, "b", "bob", &is),
		rec(5, model.OpDepAdd, "c", "cat", &is),
		rec(6, model.OpComment, "d", "dan", &is),
		rec(7, model.OpCreate, "e", "eve", &is),
		rec(8, model.OpDelete, "f", "fay", nil),
	}
	events := []model.Event{
		{Kind: model.KindPriorityChanged, IssueID: "a"},
		{Kind: model.KindClosed, IssueID: "b"},
		{Kind: model.KindBecameBlocked, IssueID: "c"},
		{Kind: model.KindBecameReady, IssueID: "b"},
		{Kind: model.KindCommented, IssueID: "d"},
		{Kind: model.KindCreated, IssueID: "e", Actor: "creator"},
		{Kind: model.KindDeleted, IssueID: "f"},
		{Kind: model.KindEdited, IssueID: "g"},
		{Kind: model.KindClosed, IssueID: "d"},
		{Kind: model.KindEdited, IssueID: "a", Prefill: true, Actor: "keep"},
		{Kind: model.Kind("weird"), IssueID: "a"},
	}
	model.AttributeActors(events, records)
	want := []string{"ann", "bob", "cat", "", "dan", "eve", "fay", "", "", "keep", ""}
	for i, w := range want {
		if events[i].Actor != w {
			t.Errorf("event %d (%s %s): actor %q, want %q", i, events[i].Kind, events[i].IssueID, events[i].Actor, w)
		}
	}
	if events[0].Seq != 2 || !events[0].Time.Equal(records[1].Time) {
		t.Errorf("event 0 = %+v", events[0])
	}
}

func TestUpdateToClosedIsAttributedAndTracked(t *testing.T) {
	open := issue("a", "open")
	closed := with(open, status("closed"))
	tr := model.NewJournalTracker(model.Statuses{})
	tr.Apply(rec(1, model.OpCreate, "a", "ann", &open))
	evs := tr.Apply(rec(2, model.OpUpdate, "a", "bob", &closed))
	wantKinds(t, evs, model.KindClosed)
	if evs[0].Actor != "bob" {
		t.Errorf("event = %+v", evs[0])
	}

	live := []model.Event{{Kind: model.KindClosed, IssueID: "a"}}
	model.AttributeActors(live, []model.JournalRecord{rec(2, model.OpUpdate, "a", "bob", &closed)})
	if live[0].Actor != "bob" {
		t.Errorf("closed via update op not attributed: %+v", live[0])
	}
}

func TestJournalTrackerSetStatuses(t *testing.T) {
	open := issue("a", "open")
	review := with(open, status("review"))
	tr := model.NewJournalTracker(model.Statuses{})
	tr.Apply(rec(1, model.OpCreate, "a", "ann", &open))
	before := kindsOf(tr.Apply(rec(2, model.OpUpdate, "a", "ann", &review)))
	tr.Apply(rec(3, model.OpUpdate, "a", "ann", &open))
	tr.SetStatuses(model.BuiltinStatuses())
	after := kindsOf(tr.Apply(rec(4, model.OpUpdate, "a", "ann", &review)))
	if len(before) == 0 || len(after) == 0 {
		t.Errorf("before %v after %v", before, after)
	}
}
