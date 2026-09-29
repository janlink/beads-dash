package model_test

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

func issue(id, status string, opts ...func(*model.Issue)) model.Issue {
	is := model.Issue{
		ID: id, Title: "t " + id, Status: status, IssueType: "task", Priority: 2,
		UpdatedAt: time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC), Raw: json.RawMessage(`{"id":"` + id + `"}`),
	}
	for _, o := range opts {
		o(&is)
	}
	return is
}

func parent(p string) func(*model.Issue) {
	return func(is *model.Issue) {
		is.Parent = p
		is.Dependencies = append(is.Dependencies, model.Edge{From: is.ID, To: p, Type: model.EdgeParentChild})
	}
}

func dep(to, typ string) func(*model.Issue) {
	return func(is *model.Issue) {
		is.Dependencies = append(is.Dependencies, model.Edge{From: is.ID, To: to, Type: typ})
	}
}

var t0 = time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)

func TestSnapshotEdgeInversion(t *testing.T) {
	s := model.NewSnapshot([]model.Issue{
		issue("e", "open"),
		issue("e.2", "open", parent("e")),
		issue("e.1", "open", parent("e")),
		issue("e.1.1", "open", parent("e.1")),
		issue("x", "open", dep("e.1", "blocks"), dep("external:other:cap", "blocks"), dep("gone", "blocks")),
		issue("y", "open", dep("e.1", "related")),
		issue("orphan", "open", parent("missing")),
		issue("onlyfield", "open", func(is *model.Issue) { is.Parent = "e" }),
		issue("self", "open", parent("self")),
	}, model.Readiness{}, t0)

	if got, want := s.Children("e"), []string{"e.1", "e.2", "onlyfield"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Children(e) = %v, want %v", got, want)
	}
	if got, want := s.Children("e.1"), []string{"e.1.1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Children(e.1) = %v, want %v", got, want)
	}
	if len(s.Children("missing")) != 0 || len(s.Children("self")) != 0 {
		t.Error("children of a missing parent or self must be dropped")
	}
	wantDeps := []model.Edge{{From: "x", To: "e.1", Type: "blocks"}, {From: "y", To: "e.1", Type: "related"}}
	if got := s.Dependents("e.1"); !reflect.DeepEqual(got, wantDeps) {
		t.Errorf("Dependents(e.1) = %v, want %v", got, wantDeps)
	}
	if len(s.Dependents("e")) != 0 {
		t.Error("parent-child edges are children, not dependents")
	}
	if !s.IsContainer("e") || s.IsContainer("e.1.1") || s.IsContainer("nope") {
		t.Error("IsContainer wrong")
	}
	if len(s.Dependents("gone")) != 0 {
		t.Error("edges to missing issues must not be inverted")
	}
}

func TestSnapshotEdgeFromDefaultsToIssue(t *testing.T) {
	a := issue("a", "open")
	a.Dependencies = []model.Edge{{To: "b", Type: "blocks"}}
	s := model.NewSnapshot([]model.Issue{a, issue("b", "open")}, model.Readiness{}, t0)
	if got := s.Dependents("b"); len(got) != 1 || got[0].From != "a" {
		t.Errorf("Dependents(b) = %v", got)
	}
}

func TestSnapshotReadinessDropsMissing(t *testing.T) {
	s := model.NewSnapshot(
		[]model.Issue{issue("a", "open"), issue("b", "open"), issue("c", "in_progress")},
		model.Readiness{
			Ready:   []string{"b", "ghost", "a", "b"},
			Blocked: map[string][]string{"c": {"b", "a"}, "ghost": {"a"}},
		}, t0)
	if got, want := s.ReadyIDs(), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ReadyIDs = %v, want %v", got, want)
	}
	if !s.IsReady("a") || s.IsReady("c") || s.IsReady("ghost") {
		t.Error("IsReady wrong")
	}
	if got, want := s.BlockedIDs(), []string{"c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BlockedIDs = %v, want %v", got, want)
	}
	if got, want := s.BlockedBy("c"), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("BlockedBy(c) = %v, want %v", got, want)
	}
	if !s.IsBlocked("c") || s.IsBlocked("a") || s.IsBlocked("ghost") {
		t.Error("IsBlocked wrong")
	}
}

func TestSnapshotAccessors(t *testing.T) {
	a := issue("a", "open")
	s := model.NewSnapshot([]model.Issue{issue("b", "open"), a}, model.Readiness{}, t0)
	if s.Len() != 2 || !reflect.DeepEqual(s.IDs(), []string{"a", "b"}) {
		t.Errorf("IDs = %v", s.IDs())
	}
	if !s.FetchedAt().Equal(t0) {
		t.Errorf("FetchedAt = %v", s.FetchedAt())
	}
	got, ok := s.Issue("a")
	if !ok || string(got.Raw) != `{"id":"a"}` {
		t.Errorf("Issue(a) = %+v, %v", got, ok)
	}
	if _, ok := s.Issue("zz"); ok {
		t.Error("Issue(zz) found")
	}
	dup := model.NewSnapshot([]model.Issue{issue("a", "open"), issue("a", "closed")}, model.Readiness{}, t0)
	if got, _ := dup.Issue("a"); got.Status != "closed" || dup.Len() != 1 {
		t.Errorf("duplicate ID must keep the last: %+v", got)
	}
}

func TestSnapshotPresent(t *testing.T) {
	s := model.NewSnapshot(
		[]model.Issue{issue("a", "open"), issue("b", "in_progress"), issue("c", "closed")},
		model.Readiness{Blocked: map[string][]string{"a": {"c"}, "b": {"c"}}}, t0)
	st := model.BuiltinStatuses()
	if got := s.Present("a", st); got.Status != model.Blocked {
		t.Errorf("a = %+v", got)
	}
	if got := s.Present("b", st); got != (model.Presentation{Status: model.InProgress, BlockedMarker: true}) {
		t.Errorf("b = %+v", got)
	}
	if got := s.Present("c", st); got.Status != model.Closed {
		t.Errorf("c = %+v", got)
	}
	if got := s.Present("ghost", st); got.Status != model.Other {
		t.Errorf("ghost = %+v", got)
	}
}

func TestProgress(t *testing.T) {
	s := model.NewSnapshot([]model.Issue{
		issue("e", "open"),
		issue("e.1", "closed", parent("e")),
		issue("e.2", "open", parent("e")),
		issue("e.3", "deferred", parent("e")),
		issue("e.4", "parked", parent("e")),
		issue("e.5", "shipped", parent("e")),
		issue("e.2.1", "closed", parent("e.2")),
		issue("e.2.2", "closed", parent("e.2")),
		issue("e.2.2.1", "open", parent("e.2.2")),
		issue("leaf", "open"),
	}, model.Readiness{}, t0)
	st := model.NewStatuses(append(model.BuiltinStatuses().All(),
		model.StatusInfo{Name: "shipped", Category: model.CategoryDone}))

	p, ok := s.Progress("e", st)
	if !ok {
		t.Fatal("epic has progress")
	}
	if want := (model.Count{Closed: 2, Total: 5}); p.Direct != want {
		t.Errorf("Direct = %+v, want %+v", p.Direct, want)
	}
	if want := (model.Count{Closed: 4, Total: 8}); p.Descendants != want {
		t.Errorf("Descendants = %+v, want %+v", p.Descendants, want)
	}
	p, ok = s.Progress("e.2", st)
	if !ok || p.Direct != (model.Count{Closed: 2, Total: 2}) || p.Descendants != (model.Count{Closed: 2, Total: 3}) {
		t.Errorf("e.2 = %+v, %v", p, ok)
	}
	if _, ok := s.Progress("leaf", st); ok {
		t.Error("issue without children must show no progress")
	}
	if _, ok := s.Progress("ghost", st); ok {
		t.Error("unknown issue must show no progress")
	}
}

func TestProgressSurvivesParentCycle(t *testing.T) {
	a := issue("a", "open", parent("b"))
	b := issue("b", "open", parent("a"))
	s := model.NewSnapshot([]model.Issue{a, b}, model.Readiness{}, t0)
	p, ok := s.Progress("a", model.BuiltinStatuses())
	if !ok || p.Direct.Total != 1 || p.Descendants.Total != 1 {
		t.Errorf("cycle progress = %+v, %v", p, ok)
	}
}

func fixtureIssues() []model.Issue {
	return []model.Issue{
		issue("e", "open"),
		issue("a", "in_progress", parent("e"), func(is *model.Issue) { is.Labels = []string{"x", "y"} }),
		issue("b", "open", parent("e"), dep("a", "blocks"), dep("c", "related")),
		issue("c", "closed"),
		issue("d", "open"),
	}
}

func fixtureReadiness() model.Readiness {
	return model.Readiness{Ready: []string{"e", "d"}, Blocked: map[string][]string{"b": {"a", "c"}}}
}

func TestFingerprintStableUnderShuffle(t *testing.T) {
	base := model.NewSnapshot(fixtureIssues(), fixtureReadiness(), t0).Fingerprint()
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		issues := fixtureIssues()
		rng.Shuffle(len(issues), func(i, j int) { issues[i], issues[j] = issues[j], issues[i] })
		for k := range issues {
			deps := issues[k].Dependencies
			rng.Shuffle(len(deps), func(i, j int) { deps[i], deps[j] = deps[j], deps[i] })
			lbl := issues[k].Labels
			rng.Shuffle(len(lbl), func(i, j int) { lbl[i], lbl[j] = lbl[j], lbl[i] })
		}
		r := fixtureReadiness()
		rng.Shuffle(len(r.Ready), func(i, j int) { r.Ready[i], r.Ready[j] = r.Ready[j], r.Ready[i] })
		by := r.Blocked["b"]
		rng.Shuffle(len(by), func(i, j int) { by[i], by[j] = by[j], by[i] })
		if got := model.NewSnapshot(issues, r, t0.Add(time.Duration(i)*time.Second)).Fingerprint(); got != base {
			t.Fatalf("fingerprint changed under shuffle %d: %s != %s", i, got, base)
		}
	}
}

func TestFingerprintCoversRawButNotItsVolatileParts(t *testing.T) {
	raw := func(s string) []model.Issue {
		is := fixtureIssues()
		is[0].Raw = json.RawMessage(s)
		return is
	}
	fp := func(is []model.Issue, at time.Time) string {
		return model.NewSnapshot(is, fixtureReadiness(), at).Fingerprint()
	}
	base := fp(raw(`{"a":1,"labels":["x","y"],"dependency_count":1,"dependencies":[{"issue_id":"a","depends_on_id":"b","type":"blocks","created_at":"t1"},{"issue_id":"a","depends_on_id":"c","type":"related","created_at":"t1"}]}`), t0)
	same := fp(raw(`{"labels":["y","x"],"dependency_count":7,"a":1,"dependencies":[{"depends_on_id":"c","type":"related","issue_id":"a","created_at":"t2"},{"type":"blocks","issue_id":"a","depends_on_id":"b","created_at":"t9"}]}`), t0.Add(time.Hour))
	if base != same {
		t.Error("key order, edge timestamps, derived counts and fetch time must not affect the fingerprint")
	}
	if base == fp(raw(`{"a":2,"labels":["x","y"],"dependencies":[]}`), t0) {
		t.Error("an undecoded field change must change the fingerprint")
	}
}

func TestSnapshotIgnoresChildMissingFromList(t *testing.T) {
	issues := []model.Issue{
		{ID: "p", Status: "open"},
		{ID: "ghost-child", Status: "open", Dependencies: nil},
	}
	issues[0].Dependencies = []model.Edge{{From: "missing", To: "p", Type: model.EdgeParentChild}}
	s := model.NewSnapshot(issues, model.Readiness{}, t0)
	if s.IsContainer("p") {
		t.Error("an edge from an unlisted issue must not make a container")
	}
	if _, ok := s.Progress("p", model.Statuses{}); ok {
		t.Error("Progress must not report for a missing child")
	}
}

func TestFingerprintDetectsChanges(t *testing.T) {
	base := model.NewSnapshot(fixtureIssues(), fixtureReadiness(), t0).Fingerprint()
	mutations := map[string]func([]model.Issue, *model.Readiness){
		"status":   func(is []model.Issue, _ *model.Readiness) { is[4].Status = "closed" },
		"title":    func(is []model.Issue, _ *model.Readiness) { is[4].Title = "renamed" },
		"priority": func(is []model.Issue, _ *model.Readiness) { is[4].Priority = 0 },
		"labels":   func(is []model.Issue, _ *model.Readiness) { is[1].Labels = []string{"x"} },
		"updated":  func(is []model.Issue, _ *model.Readiness) { is[4].UpdatedAt = is[4].UpdatedAt.Add(time.Second) },
		"comments": func(is []model.Issue, _ *model.Readiness) { is[4].CommentCount = 3 },
		"edge": func(is []model.Issue, _ *model.Readiness) {
			is[4].Dependencies = []model.Edge{{From: "d", To: "a", Type: "blocks"}}
		},
		"edge type": func(is []model.Issue, _ *model.Readiness) { is[2].Dependencies[0].Type = "related" },
		"added":     func(is []model.Issue, _ *model.Readiness) { is[4].ID = "d2" },
		"ready":     func(_ []model.Issue, r *model.Readiness) { r.Ready = r.Ready[:1] },
		"blocked":   func(_ []model.Issue, r *model.Readiness) { r.Blocked["d"] = []string{"a"} },
		"blocker":   func(_ []model.Issue, r *model.Readiness) { r.Blocked["b"] = []string{"a"} },
	}
	for name, mutate := range mutations {
		issues := fixtureIssues()
		r := fixtureReadiness()
		mutate(issues, &r)
		if got := model.NewSnapshot(issues, r, t0).Fingerprint(); got == base {
			t.Errorf("%s change left the fingerprint unchanged", name)
		}
	}
}
