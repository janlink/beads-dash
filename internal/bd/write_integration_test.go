package bd

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/testbd"
)

func issueByID(t *testing.T, c *ExecClient, id string) model.Issue {
	t.Helper()
	all, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range all {
		if is.ID == id {
			return is
		}
	}
	t.Fatalf("issue %s not listed", id)
	return model.Issue{}
}

func TestIntegrationWriteRoundTrip(t *testing.T) {
	forEachVersion(t, func(t *testing.T, _ testbd.Workspace, c *ExecClient) {
		ctx := context.Background()
		p := 1
		id, err := c.Create(ctx, CreateSpec{
			Title: "Round trip", Type: "bug", Priority: &p, Labels: []string{"a", "b"},
			Description: "first **line**\n\n- second", Notes: "n",
		})
		if err != nil {
			t.Fatal(err)
		}
		got := issueByID(t, c, id)
		if got.Title != "Round trip" || got.IssueType != "bug" || got.Priority != 1 || got.Description != "first **line**\n\n- second" ||
			!reflect.DeepEqual(got.Labels, []string{"a", "b"}) {
			t.Fatalf("created = %+v", got)
		}

		title, p3 := "Renamed", 3
		if err := c.Update(ctx, []string{id}, UpdateSpec{Title: &title, Priority: &p3, AddLabels: []string{"c"}, RemoveLabels: []string{"a"}}); err != nil {
			t.Fatal(err)
		}
		got = issueByID(t, c, id)
		if got.Title != "Renamed" || got.Priority != 3 || !reflect.DeepEqual(got.Labels, []string{"b", "c"}) {
			t.Fatalf("updated = %+v", got)
		}
		none := ""
		if err := c.Update(ctx, []string{id}, UpdateSpec{Notes: &none}); err != nil {
			t.Fatal(err)
		}
		if n := issueByID(t, c, id).Notes; n != "" {
			t.Errorf("notes after clearing = %q", n)
		}

		blocker, err := c.Create(ctx, CreateSpec{Title: "Blocker"})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.DepAdd(ctx, id, blocker, ""); err != nil {
			t.Fatal(err)
		}
		if err := c.DepAdd(ctx, blocker, id, ""); !IsClass(err, ClassRejected) {
			t.Errorf("dependency cycle = %v", err)
		}
		_, err = c.Close(ctx, []string{id}, "too early")
		var be *Error
		if !errors.As(err, &be) || be.Class != ClassRejected || !strings.Contains(be.Message, "block") {
			t.Fatalf("close under an open blocker = %v", err)
		}
		if s := issueByID(t, c, id).Status; s != "open" {
			t.Errorf("status after refused close = %q", s)
		}

		// Batch with one refused issue: bd exits 0 for some versions; either way
		// the refused ID must surface as a partial write.
		free, err := c.Create(ctx, CreateSpec{Title: "Free"})
		if err != nil {
			t.Fatal(err)
		}
		done, err := c.Close(ctx, []string{free, id}, "batch")
		if !errors.As(err, &be) || be.Class != ClassPartialWrite || !reflect.DeepEqual(done, []string{free}) ||
			len(be.Failed) != 1 || be.Failed[0].ID != id {
			t.Fatalf("batch close = %v, %v", done, err)
		}

		if _, err := c.Close(ctx, []string{blocker}, "done"); err != nil {
			t.Fatal(err)
		}
		if done, err := c.Close(ctx, []string{id}, "all done"); err != nil || !slices.Equal(done, []string{id}) {
			t.Fatalf("close = %v, %v", done, err)
		}
		if got := issueByID(t, c, id); got.Status != "closed" || got.CloseReason != "all done" {
			t.Errorf("closed = %+v", got)
		}
		if done, err := c.Reopen(ctx, []string{id}, "not done"); err != nil || !slices.Equal(done, []string{id}) {
			t.Fatalf("reopen = %v, %v", done, err)
		}
		if got := issueByID(t, c, id); got.Status != "open" {
			t.Errorf("reopened = %+v", got)
		}
		if _, err := c.Reopen(ctx, []string{id}, ""); !IsClass(err, ClassRejected) {
			t.Errorf("reopen of an open issue = %v", err)
		}
		if err := c.DepRemove(ctx, id, blocker); err != nil {
			t.Fatal(err)
		}
	})
}

func TestIntegrationBulkAndPartialUpdate(t *testing.T) {
	forEachVersion(t, func(t *testing.T, _ testbd.Workspace, c *ExecClient) {
		ctx := context.Background()
		var ids []string
		for _, title := range []string{"One", "Two", "Three"} {
			id, err := c.Create(ctx, CreateSpec{Title: title})
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		p0 := 0
		if err := c.Update(ctx, ids, UpdateSpec{Priority: &p0}); err != nil {
			t.Fatal(err)
		}
		for _, id := range ids {
			if got := issueByID(t, c, id).Priority; got != 0 {
				t.Errorf("%s priority = %d", id, got)
			}
		}

		p4 := 4
		err := c.Update(ctx, []string{ids[0], "t-zzz", ids[1]}, UpdateSpec{Priority: &p4})
		var be *Error
		if !errors.As(err, &be) || be.Class != ClassPartialWrite || len(be.Failed) != 1 || be.Failed[0].ID != "t-zzz" ||
			!reflect.DeepEqual(be.Applied, []string{ids[0], ids[1]}) {
			t.Fatalf("partial update = %v", err)
		}
		if got := issueByID(t, c, ids[1]).Priority; got != 4 {
			t.Errorf("the issues bd could resolve must change; priority = %d", got)
		}

		if err := c.Update(ctx, ids[:1], UpdateSpec{Claim: true}); err != nil {
			t.Fatal(err)
		}
		if got := issueByID(t, c, ids[0]); got.Status != "in_progress" || got.Assignee == "" {
			t.Errorf("claimed = %+v", got)
		}
		unassign := ""
		if err := c.Update(ctx, ids[:1], UpdateSpec{Assignee: &unassign}); err != nil {
			t.Fatal(err)
		}
		if got := issueByID(t, c, ids[0]).Assignee; got != "" {
			t.Errorf("assignee after unassign = %q", got)
		}

		if _, err := c.Create(ctx, CreateSpec{Title: "Bad", Priority: &[]int{9}[0]}); !IsClass(err, ClassRejected) {
			t.Errorf("invalid priority = %v", err)
		}
	})
}

func TestIntegrationCreateChildInheritsLabels(t *testing.T) {
	forEachVersion(t, func(t *testing.T, _ testbd.Workspace, c *ExecClient) {
		ctx := context.Background()
		parent, err := c.Create(ctx, CreateSpec{Title: "Parent", Type: "epic", Labels: []string{"team"}})
		if err != nil {
			t.Fatal(err)
		}
		inherit, err := c.Create(ctx, CreateSpec{Title: "Inherits", Parent: parent})
		if err != nil {
			t.Fatal(err)
		}
		plain, err := c.Create(ctx, CreateSpec{Title: "Plain", Parent: parent, NoInheritLabels: true})
		if err != nil {
			t.Fatal(err)
		}
		if got := issueByID(t, c, inherit).Labels; !slices.Contains(got, "team") {
			t.Errorf("child labels = %v, want team inherited", got)
		}
		if got := issueByID(t, c, plain).Labels; slices.Contains(got, "team") {
			t.Errorf("child labels = %v, want none inherited", got)
		}
	})
}

func TestIntegrationLongTextsRoundTrip(t *testing.T) {
	forEachVersion(t, func(t *testing.T, _ testbd.Workspace, c *ExecClient) {
		ctx := context.Background()
		long := func(head string) string {
			return head + "\n\n" + strings.Repeat("- \"quoted\" $HOME `tick` line with ünïcode\n", 400) + "tail"
		}
		desc, design := long("--starts with dashes"), long("design")
		id, err := c.Create(ctx, CreateSpec{Title: "Long", Description: desc, Design: design, Notes: "short"})
		if err != nil {
			t.Fatal(err)
		}
		got := issueByID(t, c, id)
		if got.Description != desc || got.Design != design {
			t.Fatalf("created: description ok %v, design ok %v (len %d, %d)", got.Description == desc, got.Design == design, len(got.Description), len(got.Design))
		}

		desc2, design2 := long("second"), long("design two")
		if err := c.Update(ctx, []string{id}, UpdateSpec{Description: &desc2, Design: &design2}); err != nil {
			t.Fatal(err)
		}
		got = issueByID(t, c, id)
		if got.Description != desc2 || got.Design != design2 {
			t.Errorf("updated: description ok %v, design ok %v", got.Description == desc2, got.Design == design2)
		}
	})
}
