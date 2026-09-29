package bd

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
)

func fixtureEvents(t *testing.T) []Event {
	t.Helper()
	sc := bufio.NewScanner(bytes.NewReader(fixture(t, "bd-1.3.0", "events", "tail-all.jsonl")))
	sc.Buffer(nil, 1<<20)
	var out []Event
	for sc.Scan() {
		ev, ok := parseEventRecord(sc.Bytes())
		if !ok {
			t.Fatalf("not a record: %s", sc.Text())
		}
		out = append(out, ev)
	}
	return out
}

func TestParseEventRecordsFromRealJournal(t *testing.T) {
	evs := fixtureEvents(t)
	ops := map[string]bool{}
	for i, e := range evs {
		ops[e.Op] = true
		if e.Seq != int64(i+1) || e.IssueID == "" || e.Time.IsZero() || len(e.Raw) == 0 {
			t.Errorf("record %d = %+v", i, e)
		}
		switch e.Op {
		case model.OpDelete:
			if e.Issue != nil {
				t.Errorf("delete carries an issue: %+v", e.Issue)
			}
		default:
			if e.Issue == nil || e.Issue.ID != e.IssueID || e.Issue.Title == "" {
				t.Errorf("seq %d op %s issue = %+v", e.Seq, e.Op, e.Issue)
			}
		}
	}
	for _, op := range []string{model.OpCreate, model.OpUpdate, model.OpClose, model.OpDelete, model.OpDepAdd, model.OpDepRemove, model.OpComment} {
		if !ops[op] {
			t.Errorf("fixture lacks op %s", op)
		}
	}
	var blocked, actorless int
	for _, e := range evs {
		if e.Blocked {
			blocked++
			if e.Op != model.OpDepAdd {
				t.Errorf("blocked flag on %s", e.Op)
			}
		}
		if e.Actor == "" {
			actorless++
		}
	}
	if blocked == 0 {
		t.Error("no record carries is_blocked")
	}
	if actorless == 0 {
		t.Error("fixture has no derived (actor-less) record")
	}
	rec := evs[0].Record()
	if rec.Seq != 1 || rec.Op != model.OpCreate || rec.Issue == nil || rec.Actor == "" {
		t.Errorf("Record() = %+v", rec)
	}
}

func TestParseEventRecordRejectsNonRecords(t *testing.T) {
	for _, line := range []string{
		`{"code":"events_journal_truncated","floor":6}`, `{"seq":0,"op":"create"}`, `{"seq":3}`,
		`not json`, `[]`,
	} {
		if _, ok := parseEventRecord([]byte(line)); ok {
			t.Errorf("accepted %q", line)
		}
	}
	ev, ok := parseEventRecord([]byte(`{"seq":5,"ts":"garbage","op":"update","issue_id":"x","issue":{"id":"x","is_blocked":true}}`))
	if !ok || !ev.Time.IsZero() || !ev.Blocked || ev.Issue == nil {
		t.Errorf("lenient parse = %+v, %v", ev, ok)
	}
	ev, ok = parseEventRecord([]byte(`{"seq":5,"ts":"2026-01-01T00:00:00Z","op":"delete","issue_id":"x","issue":null}`))
	if !ok || ev.Issue != nil {
		t.Errorf("delete = %+v, %v", ev, ok)
	}
}

func TestParseTruncatedShapes(t *testing.T) {
	pretty := fixture(t, "bd-1.3.0", "events", "truncated-follow.stdout")
	got, ok := parseTruncated(pretty, 99)
	if !ok || got.Since != 2 || got.Floor != 6 || got.Head != 12 {
		t.Errorf("envelope = %+v, %v", got, ok)
	}
	got, ok = parseTruncated([]byte(`{"code":"events_journal_truncated","since":1,"floor":4,"head":9}`), 0)
	if !ok || got.Since != 1 || got.Floor != 4 || got.Head != 9 {
		t.Errorf("flat = %+v, %v", got, ok)
	}
	got, ok = parseTruncated([]byte(`{"code":"events_journal_truncated","floor":4}`), 7)
	if !ok || got.Since != 7 {
		t.Errorf("since default = %+v, %v", got, ok)
	}
	for _, s := range []string{``, `text`, `{"code":"other"}`, `{"data":{"code":"other"}}`, `{"data":`} {
		if _, ok := parseTruncated([]byte(s), 0); ok {
			t.Errorf("accepted %q", s)
		}
	}
	if !strings.Contains((&JournalTruncatedError{Since: 2, Floor: 6, Head: 12}).Error(), "floor 6") {
		t.Error("error text lacks the floor")
	}
}

func TestStatusClosedThroughUpdateIsAnUpdateRecord(t *testing.T) {
	sc := bufio.NewScanner(bytes.NewReader(fixture(t, "bd-1.3.0", "events", "update-closed.jsonl")))
	tr := model.NewJournalTracker(model.BuiltinStatuses())
	var got []model.Event
	for sc.Scan() {
		ev, ok := parseEventRecord(sc.Bytes())
		if !ok {
			t.Fatalf("not a record: %s", sc.Text())
		}
		got = append(got, tr.Apply(ev.Record())...)
		if ev.Seq == 2 && ev.Op != model.OpUpdate {
			t.Errorf("op = %q, want update", ev.Op)
		}
	}
	if len(got) != 2 || got[1].Kind != model.KindClosed || got[1].Actor != "tester" {
		t.Errorf("events = %+v", got)
	}
}
