package model_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

func closedAgo(d time.Duration) func(*model.Issue) {
	return func(is *model.Issue) { is.ClosedAt = t0.Add(-d) }
}

func overviewOf(query string, events ...model.Event) model.Overview {
	snap := readyFixture()
	st := model.BuiltinStatuses()
	sc := model.ParseScope(query, false)
	return model.BuildOverview(snap, st, sc, sc.Apply(snap, st), events, t0)
}

func TestOverviewNeedsAttentionIsReadyHead(t *testing.T) {
	o := overviewOf("")
	snap := readyFixture()
	st := model.BuiltinStatuses()
	sc := model.ParseScope("", false)
	list := model.BuildReady(snap, st, sc, sc.Apply(snap, st))
	want := append(append([]string(nil), list.Unassigned...), list.Assigned...)[:3]
	if !reflect.DeepEqual(o.Attention.Ready, want) {
		t.Errorf("Ready = %v, want the first three of the Ready view %v", o.Attention.Ready, want)
	}
	a := o.Attention
	if a.ReadyTotal != 4 || a.ReadyUnassigned != 2 || a.AssignedNotStarted != 2 || a.BlockedTotal != 2 {
		t.Errorf("attention counts = %+v", a)
	}
	if want := []string{"wait-a", "wait-b"}; !reflect.DeepEqual(a.Blocked, want) {
		t.Errorf("Blocked = %v, want %v", a.Blocked, want)
	}
}

func TestOverviewSparklineBuckets(t *testing.T) {
	day := 24 * time.Hour
	snap := model.NewSnapshot([]model.Issue{
		issue("a", "closed", closedAgo(time.Minute)),
		issue("b", "closed", closedAgo(23*time.Hour)),
		issue("c", "closed", closedAgo(day)),
		issue("d", "closed", closedAgo(13*day+23*time.Hour)),
		issue("e", "closed", closedAgo(14*day)),
		issue("f", "closed", closedAgo(-time.Hour)),
		issue("g", "closed"),
		issue("h", "open", closedAgo(time.Hour)),
	}, model.Readiness{}, t0)
	st := model.BuiltinStatuses()
	sc := model.ParseScope("", true)
	o := model.BuildOverview(snap, st, sc, sc.Apply(snap, st), nil, t0)
	var want [model.SparkDays]int
	want[13], want[12], want[0] = 3, 1, 1
	if o.Closed14 != want {
		t.Errorf("Closed14 = %v, want %v", o.Closed14, want)
	}
	if o.Counts[model.Closed] != 7 || o.Total != 8 || o.ClosedPercent() != 87 {
		t.Errorf("closed %d of %d (%d%%)", o.Counts[model.Closed], o.Total, o.ClosedPercent())
	}
}

func TestOverviewCountersActiveAndDeps(t *testing.T) {
	snap := model.NewSnapshot([]model.Issue{
		issue("p", "in_progress", func(is *model.Issue) { is.Assignee = "zed" }),
		issue("q", "in_progress", prio(0), func(is *model.Issue) { is.Assignee = "amy" }),
		issue("r", "in_progress", func(is *model.Issue) { is.Assignee = "amy" }),
		issue("s", "in_progress"),
		issue("t", "open", dep("p", "blocks"), dep("q", "related"), parent("p")),
	}, model.Readiness{Blocked: map[string][]string{"t": {"p"}}}, t0)
	st := model.BuiltinStatuses()
	sc := model.ParseScope("", false)
	o := model.BuildOverview(snap, st, sc, sc.Apply(snap, st), nil, t0)
	want := []model.Assignee{{Who: "amy", IDs: []string{"q", "r"}}, {Who: "zed", IDs: []string{"p"}}}
	if !reflect.DeepEqual(o.Active, want) {
		t.Errorf("Active = %v, want %v", o.Active, want)
	}
	if o.Deps != 1 || o.Counts[model.InProgress] != 4 || o.Counts[model.Blocked] != 1 {
		t.Errorf("deps %d counts %v", o.Deps, o.Counts)
	}
}

func TestOverviewAssignedNotStartedCountsBlockedToo(t *testing.T) {
	assign := func(is *model.Issue) { is.Assignee = "amy" }
	snap := model.NewSnapshot([]model.Issue{
		issue("a", "open", assign),
		issue("b", "open", assign),
		issue("c", "open"),
		issue("d", "in_progress", assign),
	}, model.Readiness{Ready: []string{"a", "c"}, Blocked: map[string][]string{"b": {"c"}}}, t0)
	st := model.BuiltinStatuses()
	sc := model.ParseScope("", false)
	o := model.BuildOverview(snap, st, sc, sc.Apply(snap, st), nil, t0)
	if o.Attention.AssignedNotStarted != 2 {
		t.Errorf("assigned not started = %d, want 2", o.Attention.AssignedNotStarted)
	}
}

func TestOverviewFollowsScope(t *testing.T) {
	evs := []model.Event{
		{Kind: model.KindClaimed, IssueID: "mine"},
		{Kind: model.KindClosed, IssueID: "done"},
		{Kind: model.KindDeleted, IssueID: "gone"},
		{Kind: model.KindCreated, IssueID: "late"},
	}
	o := overviewOf("", evs...)
	if len(o.Feed) != 4 || o.Feed[0].IssueID != "mine" || o.Feed[1].IssueID != "done" || o.Feed[2].IssueID != "gone" {
		t.Errorf("feed = %v: a closed and a deleted issue stay in without a scope", o.Feed)
	}
	o = overviewOf("assignee:bob", evs...)
	if len(o.Feed) != 1 || o.Feed[0].IssueID != "mine" || o.Total != 2 {
		t.Errorf("scoped feed = %v total %d", o.Feed, o.Total)
	}
}

func TestBucketOf(t *testing.T) {
	now := t0
	tests := []struct {
		ago  time.Duration
		want model.FeedBucket
	}{
		{time.Minute, model.BucketJustNow},
		{30 * time.Minute, model.BucketLastHour},
		{5 * time.Hour, model.BucketToday},
		{30 * time.Hour, model.BucketYesterday},
		{72 * time.Hour, model.BucketEarlier},
	}
	for _, tt := range tests {
		if got := model.BucketOf(now, now.Add(-tt.ago)); got != tt.want {
			t.Errorf("%v ago: %v, want %v", tt.ago, got.Label(), tt.want.Label())
		}
	}
}
