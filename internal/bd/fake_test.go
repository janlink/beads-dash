package bd

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

func fakeIssue(id, status string) model.Issue {
	return model.Issue{ID: id, Title: "t " + id, Status: status, IssueType: "task", Priority: 2}
}

var allClasses = []Class{
	ClassBdMissing, ClassUnsupported, ClassNotWorkspace, ClassTimeout,
	ClassTransient, ClassRejected, ClassPartialWrite,
}

func TestFakeCoversEveryErrorClass(t *testing.T) {
	ctx := context.Background()
	reads := map[string]func(*Fake) error{
		"Version":   func(f *Fake) error { _, err := f.Version(ctx); return err },
		"Where":     func(f *Fake) error { _, err := f.Where(ctx); return err },
		"List":      func(f *Fake) error { _, err := f.List(ctx); return err },
		"Ready":     func(f *Fake) error { _, err := f.Ready(ctx); return err },
		"Statuses":  func(f *Fake) error { _, err := f.Statuses(ctx); return err },
		"Types":     func(f *Fake) error { _, err := f.Types(ctx); return err },
		"VCStatus":  func(f *Fake) error { _, err := f.VCStatus(ctx); return err },
		"Comments":  func(f *Fake) error { _, err := f.Comments(ctx, "a"); return err },
		"History":   func(f *Fake) error { _, err := f.History(ctx, "a"); return err },
		"Memories":  func(f *Fake) error { _, err := f.Memories(ctx); return err },
		"ConfigGet": func(f *Fake) error { _, err := f.ConfigGet(ctx, "k"); return err },
		"EventsFollow": func(f *Fake) error {
			_, err := f.EventsFollow(ctx, 0)
			return err
		},
		"Create": func(f *Fake) error { _, err := f.Create(ctx, CreateSpec{Title: "x"}); return err },
		"Update": func(f *Fake) error { return f.Update(ctx, []string{"a"}, UpdateSpec{Claim: true}) },
		"Close":  func(f *Fake) error { _, err := f.Close(ctx, []string{"a"}, ""); return err },
		"Reopen": func(f *Fake) error {
			_, _ = f.Close(ctx, []string{"a"}, "")
			_, err := f.Reopen(ctx, []string{"a"}, "")
			return err
		},
		"DepAdd": func(f *Fake) error { return f.DepAdd(ctx, "a", "b", "") },
		"DepRemove": func(f *Fake) error {
			return f.DepRemove(ctx, "a", "b")
		},
		"Comment":  func(f *Fake) error { return f.Comment(ctx, "a", "x") },
		"Remember": func(f *Fake) error { return f.Remember(ctx, "k", "v") },
		"Forget": func(f *Fake) error {
			f.SetMemory("k", "v")
			return f.Forget(ctx, "k")
		},
		"ConfigSet": func(f *Fake) error { return f.ConfigSet(ctx, "k", "v") },
	}
	for method, call := range reads {
		for _, class := range allClasses {
			f := NewFake()
			f.SetIssues(fakeIssue("a", "open"), fakeIssue("b", "open"))
			f.FailWith(method, &Error{Class: class, Command: method})
			err := call(f)
			if !IsClass(err, class) {
				t.Errorf("%s with class %v: got %v", method, class, err)
			}
			f.FailWith(method, nil)
			if method == "EventsFollow" {
				continue
			}
			if err := call(f); err != nil {
				t.Errorf("%s after clearing the failure: %v", method, err)
			}
		}
	}
}

func TestFakeHonoursCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewFake().List(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestFakeNeverDerivesReadiness(t *testing.T) {
	f := NewFake()
	blocker := fakeIssue("b", "open")
	a := fakeIssue("a", "open")
	a.Dependencies = []model.Edge{{From: "a", To: "b", Type: "blocks"}}
	f.SetIssues(a, blocker)
	ctx := context.Background()
	r, _ := f.Ready(ctx)
	if len(r.Ready) != 0 || len(r.Blocked) != 0 {
		t.Fatalf("fake derived readiness: %+v", r)
	}
	f.SetReadiness([]string{"b"}, map[string][]string{"a": {"b"}})
	if _, err := f.Close(ctx, []string{"b"}, "done"); err != nil {
		t.Fatal(err)
	}
	r, _ = f.Ready(ctx)
	if !reflect.DeepEqual(r.Ready, []string{"b"}) || !reflect.DeepEqual(r.Blocked["a"], []string{"b"}) {
		t.Errorf("a write changed readiness: %+v", r)
	}
	r.Ready[0] = "mutated"
	if again, _ := f.Ready(ctx); again.Ready[0] != "b" {
		t.Error("Ready must return copies")
	}
}

func TestFakeReadsAndSetters(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	f.SetWorkspace(Workspace{Path: "/w", Prefix: "p"})
	f.SetStatuses(model.NewStatuses([]model.StatusInfo{{Name: "x", Category: model.CategoryActive}}))
	f.SetTypes([]TypeInfo{{Name: "only"}})
	f.SetVCStatus(VCStatus{Branch: "b", Commit: "c1"})
	f.SetMemory("k2", "v2")
	f.SetMemory("k1", "v1")
	f.SetConfig("events-journal", "true")
	t0 := time.Unix(100, 0)
	f.SetComments("a", Comment{ID: "2", CreatedAt: t0.Add(time.Second)}, Comment{ID: "1", CreatedAt: t0})
	f.SetHistory("a", HistoryEntry{CommitHash: "h"})
	f.SetIssues(fakeIssue("a", "open"))

	if w, _ := f.Where(ctx); w.Prefix != "p" {
		t.Errorf("Where = %+v", w)
	}
	if s, _ := f.Statuses(ctx); s.Category("x") != model.CategoryActive {
		t.Error("Statuses")
	}
	if ty, _ := f.Types(ctx); len(ty) != 1 {
		t.Error("Types")
	}
	if vc, _ := f.VCStatus(ctx); vc.Commit != "c1" {
		t.Error("VCStatus")
	}
	if m, _ := f.Memories(ctx); len(m) != 2 || m[0].Key != "k1" {
		t.Errorf("Memories = %+v", m)
	}
	if v, _ := f.ConfigGet(ctx, "events-journal"); v.Value != "true" {
		t.Error("ConfigGet")
	}
	if v, _ := f.ConfigGet(ctx, "unset"); v.Value != "" || v.Key != "unset" {
		t.Errorf("ConfigGet unset = %+v", v)
	}
	if c, _ := f.Comments(ctx, "a"); len(c) != 2 || c[0].ID != "1" {
		t.Errorf("Comments = %+v", c)
	}
	if h, _ := f.History(ctx, "a"); len(h) != 1 {
		t.Error("History")
	}
	issues, _ := f.List(ctx)
	if len(issues) != 1 || len(issues[0].Raw) == 0 {
		t.Errorf("List = %+v; Raw must be synthesised", issues)
	}
	f.SetVersion("1.2.2")
	if v, err := f.Version(ctx); err != nil || v.caps.eventsJournal {
		t.Errorf("Version = %+v, %v", v, err)
	}
	f.SetVersion("1.2.1")
	if v, err := f.Version(ctx); !IsClass(err, ClassUnsupported) || v.Raw != "1.2.1" {
		t.Errorf("Version 1.2.1 = %+v, %v", v, err)
	}
	if got := f.Calls(); len(got) == 0 || got[0] != "Where" {
		t.Errorf("Calls = %v", got)
	}
}

func TestFakeWrites(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	f.SetIssues(fakeIssue("f-1", "open"), fakeIssue("f-2", "open"))

	p := 0
	id, err := f.Create(ctx, CreateSpec{Title: "child", Parent: "f-1", Priority: &p, Labels: []string{"x"}, Type: "bug"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Create(ctx, CreateSpec{}); !IsClass(err, ClassRejected) {
		t.Errorf("create without title = %v", err)
	}
	title, status, assignee := "renamed", "in_progress", "alice"
	if err := f.Update(ctx, []string{id}, UpdateSpec{Title: &title, Status: &status, Assignee: &assignee, AddLabels: []string{"y", "x"}, RemoveLabels: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.Update(ctx, []string{"nope"}, UpdateSpec{Claim: true}); !IsClass(err, ClassRejected) {
		t.Errorf("update of unknown issue = %v", err)
	}
	issues, _ := f.List(ctx)
	var got model.Issue
	for _, is := range issues {
		if is.ID == id {
			got = is
		}
	}
	if got.Title != "renamed" || got.Status != "in_progress" || got.Assignee != "alice" || got.Priority != 0 ||
		got.IssueType != "bug" || !reflect.DeepEqual(got.Labels, []string{"y"}) || len(got.Dependencies) != 1 {
		t.Errorf("created+updated issue = %+v", got)
	}

	done, err := f.Close(ctx, []string{id, "ghost"}, "because")
	var be *Error
	if !errors.As(err, &be) || be.Class != ClassPartialWrite || !reflect.DeepEqual(done, []string{id}) ||
		!reflect.DeepEqual(be.Applied, []string{id}) || len(be.Failed) != 1 || be.Failed[0].ID != "ghost" {
		t.Errorf("Close = %v, %v; want a partial write with ghost failed", done, err)
	}
	if done, err := f.Close(ctx, []string{"f-1"}, "because"); err != nil || len(done) != 1 {
		t.Errorf("Close of the parent after its child = %v, %v", done, err)
	}
	if _, err := f.Close(ctx, []string{"ghost"}, ""); !IsClass(err, ClassRejected) {
		t.Errorf("Close of only unknown IDs = %v", err)
	}
	if done, err := f.Reopen(ctx, []string{"f-1"}, ""); err != nil || len(done) != 1 {
		t.Errorf("Reopen = %v, %v", done, err)
	}

	if err := f.DepAdd(ctx, "f-2", "f-1", ""); err != nil {
		t.Fatal(err)
	}
	if err := f.DepAdd(ctx, "ghost", "f-1", ""); err == nil {
		t.Error("DepAdd from unknown issue")
	}
	if err := f.DepRemove(ctx, "f-2", "f-1"); err != nil {
		t.Fatal(err)
	}
	if err := f.DepRemove(ctx, "ghost", "f-1"); err == nil {
		t.Error("DepRemove from unknown issue")
	}
	if err := f.Comment(ctx, "f-2", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := f.Comment(ctx, "ghost", "hello"); err == nil {
		t.Error("Comment on unknown issue")
	}
	if c, _ := f.Comments(ctx, "f-2"); len(c) != 1 || c[0].Text != "hello" {
		t.Errorf("Comments = %+v", c)
	}
	if err := f.Remember(ctx, "k", "v"); err != nil {
		t.Fatal(err)
	}
	if m, _ := f.Memories(ctx); len(m) != 1 {
		t.Error("Remember")
	}
	if err := f.Forget(ctx, "k"); err != nil {
		t.Fatal(err)
	}
	if m, _ := f.Memories(ctx); len(m) != 0 {
		t.Error("Forget")
	}
	if err := f.ConfigSet(ctx, "events-journal", "true"); err != nil {
		t.Fatal(err)
	}
	if v, _ := f.ConfigGet(ctx, "events-journal"); v.Value != "true" {
		t.Error("ConfigSet")
	}
	if len(f.Issues()) != 3 {
		t.Errorf("Issues() = %d", len(f.Issues()))
	}
}

func TestFakeDeferSetsAndClearsTheDeferral(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	id, err := f.Create(ctx, CreateSpec{Title: "Later", Defer: "2030-01-15"})
	if err != nil {
		t.Fatal(err)
	}
	get := func() model.Issue {
		t.Helper()
		for _, is := range f.Issues() {
			if is.ID == id {
				return is
			}
		}
		t.Fatalf("no issue %s", id)
		return model.Issue{}
	}
	if is := get(); is.Status != "deferred" || is.DeferUntil.Format("2006-01-02") != "2030-01-15" {
		t.Fatalf("created %q until %v", is.Status, is.DeferUntil)
	}
	none := ""
	if err := f.Update(ctx, []string{id}, UpdateSpec{Defer: &none}); err != nil {
		t.Fatal(err)
	}
	if is := get(); is.Status != "open" || !is.DeferUntil.IsZero() {
		t.Errorf("cleared: %q until %v", is.Status, is.DeferUntil)
	}
	date := "2031-02-03"
	if err := f.Update(ctx, []string{id}, UpdateSpec{Defer: &date}); err != nil {
		t.Fatal(err)
	}
	if is := get(); is.Status != "deferred" || is.DeferUntil.Format("2006-01-02") != date {
		t.Errorf("set: %q until %v", is.Status, is.DeferUntil)
	}
}
