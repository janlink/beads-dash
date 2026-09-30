package ui

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/refresh"
)

func TestNewIssueCreatesAndFocusesIt(t *testing.T) {
	for _, snapFirst := range []bool{true, false} {
		r := newRig(t, 100, 30)
		if !snapFirst {
			r.eng.publish = nil
		}
		r.key("n")
		if !isOpen[*issueForm](r.a) {
			t.Fatal("n opens no form")
		}
		r.text("Brand new")
		r.key("ctrl+s")
		if !snapFirst {
			r.snapshot()
		}
		if isOpen[*issueForm](r.a) {
			t.Fatalf("form still open (snapshot first %v):\n%s", snapFirst, screen(r.a))
		}
		cur := r.a.sess.Current()
		if cur == "" || r.issue(cur).Title != "Brand new" {
			t.Errorf("current %q is not the new issue (snapshot first %v)", cur, snapFirst)
		}
	}
}

func lastWrite(t testing.TB, r *writeRig) bd.FakeWrite {
	t.Helper()
	w := r.fake.Writes()
	if len(w) == 0 {
		t.Fatal("no write reached bd")
	}
	return w[len(w)-1]
}

func TestEditSendsOnlyTheChangedFields(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e")
	if !isOpen[*issueForm](r.a) {
		t.Fatal("e opens no form")
	}
	r.text(" again")
	r.key("ctrl+s")
	w := lastWrite(t, r)
	if w.Method != "Update" || !slices.Equal(w.IDs, []string{"ws-9qe"}) {
		t.Fatalf("write %+v", w)
	}
	if w.Update.Title == nil || *w.Update.Title != "Payment provider timeout retries again" {
		t.Errorf("title %v", w.Update.Title)
	}
	u := w.Update
	u.Title = nil
	if !u.Empty() {
		t.Errorf("more than the title was sent: %+v", u)
	}
	if isOpen[*issueForm](r.a) {
		t.Error("the form stays open after a saved edit")
	}
}

func TestEditWithoutChangesWritesNothing(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e", "ctrl+s")
	if n := len(r.fake.Writes()); n != 0 {
		t.Errorf("%d writes for an unchanged form", n)
	}
}

func TestFailedWriteKeepsFormAndValues(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.fake.FailWith("Update", &bd.Error{Message: "database is locked"})
	r.key("e")
	r.text("!")
	r.key("ctrl+s")
	if !isOpen[*issueForm](r.a) {
		t.Fatal("a failed save closed the form")
	}
	out := screen(r.a)
	if !strings.Contains(out, "retries!") || !strings.Contains(out, "database is locked") {
		t.Errorf("values or error lost:\n%s", out)
	}
}

func TestBulkPriorityIsOneWriteAndClearsMarks(t *testing.T) {
	r := newRig(t, 100, 30)
	for _, id := range []string{"ws-9qe", "ws-2hz", "ws-7mt"} {
		r.a.sess.SetCurrent(id)
		r.key("space")
	}
	if n := len(r.a.sess.MarkedIDs()); n != 3 {
		t.Fatalf("%d marks", n)
	}
	r.key("p", "1")
	w := lastWrite(t, r)
	if len(r.fake.Writes()) != 1 || w.Method != "Update" || len(w.IDs) != 3 || w.Update.Priority == nil || *w.Update.Priority != 1 {
		t.Fatalf("writes %+v", r.fake.Writes())
	}
	if n := len(r.a.sess.MarkedIDs()); n != 0 {
		t.Errorf("%d marks left after success", n)
	}
	for _, id := range w.IDs {
		if r.issue(id).Priority != 1 {
			t.Errorf("%s priority %d", id, r.issue(id).Priority)
		}
	}
}

func TestFailedBulkWriteKeepsMarks(t *testing.T) {
	r := newRig(t, 100, 30)
	for _, id := range []string{"ws-9qe", "ws-2hz"} {
		r.a.sess.SetCurrent(id)
		r.key("space")
	}
	r.fake.FailWith("Update", errors.New("boom"))
	r.key("p", "1")
	if n := len(r.a.sess.MarkedIDs()); n != 2 {
		t.Errorf("%d marks after a failed write, want 2", n)
	}
}

func TestPartialBulkCloseKeepsMarksAndReportsRefusal(t *testing.T) {
	r := newRig(t, 100, 30)
	r.fake.Refuse("ws-2hz", "cannot close ws-2hz: stuck")
	for _, id := range []string{"ws-9qe", "ws-2hz"} {
		r.a.sess.SetCurrent(id)
		r.key("space")
	}
	r.key("c", "enter")
	if r.issue("ws-9qe").Status != "closed" || r.issue("ws-2hz").Status == "closed" {
		t.Fatalf("statuses %s %s", r.issue("ws-9qe").Status, r.issue("ws-2hz").Status)
	}
	if got := r.a.sess.MarkedIDs(); !slices.Equal(got, []string{"ws-2hz"}) {
		t.Errorf("marks after a partial write: %v, want only the refused one", got)
	}
	if !strings.Contains(screen(r.a), "ws-2hz") {
		t.Errorf("refusal not shown:\n%s", screen(r.a))
	}
}

func TestRefusedCloseListsBlockersAndEnterJumps(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-4k2")
	r.key("c", "enter")
	if !isOpen[*closeDialog](r.a) {
		t.Fatalf("no dialog after a refused close:\n%s", screen(r.a))
	}
	out := screen(r.a)
	if !strings.Contains(out, "ws-4k2.1") || !strings.Contains(out, "Cannot close") {
		t.Errorf("open children not listed:\n%s", out)
	}
	if r.issue("ws-4k2").Status == "closed" {
		t.Fatal("the guard was bypassed")
	}
	r.key("enter")
	if isOpen[*closeDialog](r.a) || !strings.HasPrefix(r.a.sess.Current(), "ws-4k2.") {
		t.Errorf("enter did not jump to a child: current %q", r.a.sess.Current())
	}
}

func TestRefusedCloseFromTheFormListsBlockersAndJumps(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-4k2")
	r.key("e")
	r.form().f.Field(fStatus).Set("closed")
	r.key("ctrl+s")
	if isOpen[*issueForm](r.a) || !isOpen[*closeDialog](r.a) {
		t.Fatalf("no refusal list after a refused close:\n%s", screen(r.a))
	}
	if out := screen(r.a); !strings.Contains(out, "ws-4k2.1") || !strings.Contains(out, "Cannot close") {
		t.Errorf("open children not listed:\n%s", out)
	}
	r.key("enter")
	if isOpen[*closeDialog](r.a) || !strings.HasPrefix(r.a.sess.Current(), "ws-4k2.") {
		t.Errorf("enter did not jump: current %q", r.a.sess.Current())
	}
}

func TestRefusedRowsFitTheDialog(t *testing.T) {
	r := newRig(t, 60, 24)
	r.a.sess.SetCurrent("ws-4k2")
	r.key("c", "enter")
	for _, line := range lines(r.a) {
		if w := ansi.StringWidth(line); w > 60 {
			t.Fatalf("line of width %d: %q", w, line)
		}
	}
}

func TestCloseThenReopen(t *testing.T) {
	r := newRigView(t, 120, 30, "kanban")
	r.a.sess.SetCurrent("ws-9qe")
	r.key("c")
	r.text("done")
	r.key("enter")
	if is := r.issue("ws-9qe"); is.Status != "closed" || is.CloseReason != "done" {
		t.Fatalf("after close: %s %q", is.Status, is.CloseReason)
	}
	r.a.sess.SetCurrent("ws-9qe")
	r.key("c", "enter")
	if r.issue("ws-9qe").Status != "open" {
		t.Errorf("after reopen: %s", r.issue("ws-9qe").Status)
	}
}

func TestKanbanMoves(t *testing.T) {
	r := newRigView(t, 120, 30, "kanban")
	r.a.sess.SetCurrent("ws-9qe")
	r.key(">")
	if r.issue("ws-9qe").Status != "in_progress" {
		t.Fatalf("> did not start: %s", r.issue("ws-9qe").Status)
	}
	r.key(">")
	if !isOpen[*closeDialog](r.a) {
		t.Fatal("moving into Closed opens no close dialog")
	}
	r.key("esc")
	r.key("<")
	if r.issue("ws-9qe").Status != "open" {
		t.Errorf("< did not stop: %s", r.issue("ws-9qe").Status)
	}
	r.a.sess.SetCurrent("ws-8np")
	before := len(r.fake.Writes())
	r.key(">")
	if len(r.fake.Writes()) != before || r.a.hint == "" {
		t.Errorf("a deferred card moved or gave no hint (%q)", r.a.hint)
	}
}

func TestKanbanMoveReopensFromClosed(t *testing.T) {
	r := newRigView(t, 120, 30, "kanban")
	r.a.sess.SetCurrent("ws-5ca")
	r.key("<")
	if r.issue("ws-5ca").Status != "open" {
		t.Errorf("status %s", r.issue("ws-5ca").Status)
	}
}

func TestKanbanMoveActsOnMarks(t *testing.T) {
	r := newRigView(t, 120, 30, "kanban")
	for _, id := range []string{"ws-9qe", "ws-4k2.1"} {
		r.a.sess.SetCurrent(id)
		r.key("space")
	}
	r.key(">")
	if w := lastWrite(t, r); len(w.IDs) != 2 || w.Update.Status == nil || *w.Update.Status != "in_progress" {
		t.Errorf("write %+v", w)
	}
}

func TestKanbanMoveAcrossGroupsKeepsMarksOfWhatIsStillToDo(t *testing.T) {
	r := newRigView(t, 120, 30, "kanban")
	for _, id := range []string{"ws-9qe", "ws-4k2"} {
		r.a.sess.SetCurrent(id)
		r.key("space")
	}
	r.key(">")
	if !isOpen[*closeDialog](r.a) {
		t.Fatalf("the in-progress card opened no close dialog:\n%s", screen(r.a))
	}
	if got := r.a.sess.MarkedIDs(); !slices.Equal(got, []string{"ws-4k2"}) {
		t.Fatalf("marks while the close dialog is open: %v", got)
	}
	r.fake.Refuse("ws-4k2", "cannot close ws-4k2: stuck")
	r.key("enter")
	if got := r.a.sess.MarkedIDs(); !slices.Equal(got, []string{"ws-4k2"}) {
		t.Errorf("marks after a refused close: %v", got)
	}
}

func TestWriteCommands(t *testing.T) {
	cases := []struct {
		line   string
		method string
		check  func(bd.FakeWrite) bool
	}{
		{"p 1", "Update", func(w bd.FakeWrite) bool { return *w.Update.Priority == 1 }},
		{"priority P3", "Update", func(w bd.FakeWrite) bool { return *w.Update.Priority == 3 }},
		{"s in_progress", "Update", func(w bd.FakeWrite) bool { return *w.Update.Status == "in_progress" }},
		{"assign me", "Update", func(w bd.FakeWrite) bool { return *w.Update.Assignee == "me" }},
		{"assign -", "Update", func(w bd.FakeWrite) bool { return *w.Update.Assignee == "" }},
		{"label +a -b", "Update", func(w bd.FakeWrite) bool {
			return slices.Equal(w.Update.AddLabels, []string{"a"}) && slices.Equal(w.Update.RemoveLabels, []string{"b"})
		}},
		{"claim", "Update", func(w bd.FakeWrite) bool { return w.Update.Claim }},
		{"close why not", "Close", func(w bd.FakeWrite) bool { return w.Reason == "why not" }},
		{"dep add ws-2hz", "DepAdd", func(w bd.FakeWrite) bool { return slices.Equal(w.IDs, []string{"ws-9qe", "ws-2hz"}) }},
		{"parent ws-4k2", "Update", func(w bd.FakeWrite) bool { return *w.Update.Parent == "ws-4k2" }},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			r := newRig(t, 100, 30)
			r.a.sess.SetCurrent("ws-9qe")
			r.line(c.line)
			w := lastWrite(t, r)
			if w.Method != c.method || !c.check(w) {
				t.Errorf("write %+v", w)
			}
		})
	}
}

func TestCommandsActOnMarks(t *testing.T) {
	r := newRig(t, 100, 30)
	for _, id := range []string{"ws-9qe", "ws-2hz"} {
		r.a.sess.SetCurrent(id)
		r.key("space")
	}
	r.line("p 4")
	if w := lastWrite(t, r); len(w.IDs) != 2 {
		t.Errorf("write %+v", w)
	}
}

func TestDepAndParentAreRejectedForBadTargets(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-4k2")
	for _, line := range []string{"dep add ws-4k2", "dep add nope", "parent ws-4k2.1", "p 9", "s nonsense"} {
		r.line(line)
		if a := r.a; a.bar == nil || a.bar.err == nil {
			t.Errorf(":%s was accepted", line)
		}
		r.key("esc")
		r.key("esc")
	}
	if n := len(r.fake.Writes()); n != 0 {
		t.Errorf("%d writes for bad commands", n)
	}
}

func TestNewCommandPrefillsTitle(t *testing.T) {
	r := newRig(t, 100, 30)
	r.line("new Hello there")
	if !isOpen[*issueForm](r.a) || !strings.Contains(screen(r.a), "Hello there") {
		t.Errorf("no prefilled form:\n%s", screen(r.a))
	}
}

func TestEditCommandOpensForm(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.line("edit")
	if !isOpen[*issueForm](r.a) {
		t.Error(":edit opens no form")
	}
}

func TestStatusClosedOpensCloseDialog(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.line("s closed")
	if !isOpen[*closeDialog](r.a) {
		t.Error("a closed status must go through the close dialog")
	}
	if len(r.fake.Writes()) != 0 {
		t.Error("closed was written as a plain status")
	}
}

func TestDoneCategoryStatusNeverGoesThroughUpdate(t *testing.T) {
	infos := append(model.BuiltinStatuses().All(), model.StatusInfo{Name: "archived", Category: model.CategoryDone, Custom: true})
	setup := func() *writeRig {
		r := newRig(t, 100, 30)
		r.a.bds.Statuses = model.NewStatuses(infos)
		r.a.sess.SetCurrent("ws-9qe")
		return r
	}
	r := setup()
	r.line("s archived")
	if !isOpen[*closeDialog](r.a) || len(r.fake.Writes()) != 0 {
		t.Errorf("command: close dialog %v, writes %v", isOpen[*closeDialog](r.a), r.fake.Writes())
	}

	r = setup()
	r.key("s")
	q, _ := r.a.topDialog().(*quickDialog)
	if q == nil {
		t.Fatalf("no quick dialog:\n%s", screen(r.a))
	}
	q.f.Focused().Set("archived")
	q.submit()
	if !isOpen[*closeDialog](r.a) || len(r.fake.Writes()) != 0 {
		t.Errorf("quick: close dialog %v, writes %v", isOpen[*closeDialog](r.a), r.fake.Writes())
	}

	r = setup()
	r.line("edit")
	fd := r.form()
	fd.f.Field(fStatus).Set("archived")
	df := fd.diff()
	if !df.closeIt || df.update.Status != nil {
		t.Errorf("form diff: closeIt %v, status %v", df.closeIt, df.update.Status)
	}
}

func TestConfirmOnlyYConfirms(t *testing.T) {
	for _, k := range []string{"enter", "n", "space", "esc", "x"} {
		a := sample(t, plain, 100, 30)
		yes := false
		a.pushDialog(a.newConfirm(confirmOpts{Title: "Sure?", Yes: func(*App) tea.Cmd { yes = true; return nil }}))
		press(a, k)
		if yes {
			t.Errorf("%q confirmed", k)
		}
		if isOpen[*confirmDialog](a) && k != "x" && k != "space" {
			t.Errorf("%q left the dialog open", k)
		}
	}
	a := sample(t, plain, 100, 30)
	yes := false
	a.pushDialog(a.newConfirm(confirmOpts{Title: "Sure?", Yes: func(*App) tea.Cmd { yes = true; return nil }}))
	press(a, "y")
	if !yes {
		t.Error("y did not confirm")
	}
}

func (r *writeRig) form() *issueForm {
	r.t.Helper()
	f, ok := r.a.topDialog().(*issueForm)
	if !ok {
		r.t.Fatalf("no issue form open:\n%s", screen(r.a))
	}
	return f
}

func (r *writeRig) external(id string, spec bd.UpdateSpec) {
	r.t.Helper()
	if err := r.fake.Update(context.Background(), []string{id}, spec); err != nil {
		r.t.Fatal(err)
	}
	r.snapshot()
}

func ptr[T any](v T) *T { return &v }

func TestConflictBannerAndDecisions(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e")
	r.text("!")
	r.external("ws-9qe", bd.UpdateSpec{Title: ptr("Retries, reworded"), Priority: ptr(3)})
	out := screen(r.a)
	if !strings.Contains(out, "Changed since opened") || !strings.Contains(out, "priority") {
		t.Fatalf("no banner:\n%s", out)
	}
	before := len(r.fake.Writes())
	r.key("ctrl+s")
	if len(r.fake.Writes()) != before {
		t.Fatal("submit went through with an undecided conflict")
	}
	if !isOpen[*issueForm](r.a) {
		t.Fatal("form closed")
	}
	r.key("ctrl+o", "ctrl+s")
	w := lastWrite(t, r)
	if w.Method != "Update" || w.Update.Title == nil || *w.Update.Title != "Payment provider timeout retries!" {
		t.Fatalf("keeping mine wrote %+v", w)
	}
	if w.Update.Priority != nil {
		t.Errorf("the untouched priority was sent: %d", *w.Update.Priority)
	}
}

func TestConflictTakeTheirsDropsMyEdit(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e")
	r.text("!")
	r.external("ws-9qe", bd.UpdateSpec{Title: ptr("Retries, reworded")})
	r.key("ctrl+t")
	if got := r.form().f.Field(fTitle).Value(); got != "Retries, reworded" {
		t.Errorf("title %q", got)
	}
	before := len(r.fake.Writes())
	r.key("ctrl+s")
	if len(r.fake.Writes()) != before {
		t.Error("taking theirs left a change to write")
	}
}

func TestForeignChangeToUntouchedFieldIsTakenSilently(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e")
	r.text("!")
	r.external("ws-9qe", bd.UpdateSpec{Priority: ptr(3)})
	if got := r.form().f.Field(fPriority).Value(); got != "P3" {
		t.Errorf("priority %q", got)
	}
	r.key("ctrl+s")
	if w := lastWrite(t, r); w.Update.Priority != nil {
		t.Errorf("priority sent: %+v", w.Update)
	}
}

func TestRetryAfterFailedSaveSaves(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.fake.FailWith("Update", &bd.Error{Message: "boom"})
	r.key("e")
	r.text("!")
	r.key("ctrl+s")
	r.fake.FailWith("Update", nil)
	r.key("ctrl+s")
	if isOpen[*issueForm](r.a) {
		t.Errorf("retry after a failure did not save:\n%s", screen(r.a))
	}
}

func TestOwnPartialWriteShowsNoConflictBanner(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e")
	f := r.form()
	f.f.Field(fTitle).Set("Renamed by me")
	f.f.Field(fBlockedBy).Set("ws-7mt")
	r.fake.FailWith("DepAdd", &bd.Error{Message: "boom"})
	r.key("ctrl+s")
	if !isOpen[*issueForm](r.a) {
		t.Fatalf("the form closed after a failed step:\n%s", screen(r.a))
	}
	if got := r.issue("ws-9qe").Title; got != "Renamed by me" {
		t.Fatalf("the update step did not apply: %q", got)
	}
	r.snapshot()
	if f.conflicted() || strings.Contains(screen(r.a), "changed underneath") {
		t.Errorf("own write shown as a conflict: %+v\n%s", f.conflicts, screen(r.a))
	}
}

func TestForeignWriteShowsConflictBanner(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e")
	f := r.form()
	f.f.Field(fTitle).Set("Renamed by me")
	r.external("ws-9qe", bd.UpdateSpec{Title: ptr("Renamed by them")})
	r.snapshot()
	if !f.conflicted() {
		t.Errorf("foreign write not shown:\n%s", screen(r.a))
	}
}

func TestCreateUnderParentCarriesLabelsUnlessRemoved(t *testing.T) {
	for _, drop := range []bool{false, true} {
		r := newRig(t, 100, 30)
		r.external("ws-4k2", bd.UpdateSpec{AddLabels: []string{"web", "urgent"}})
		r.key("n")
		f := r.form()
		f.setParent("ws-4k2")
		if got := f.f.Field(fLabels).List(); !slices.Equal(got, []string{"web", "urgent"}) {
			t.Fatalf("inherited labels %v", got)
		}
		if drop {
			f.f.Field(fLabels).Set("web")
		}
		f.f.Field(fTitle).Set("Child")
		r.key("ctrl+s")
		w := lastWrite(t, r)
		if w.Method != "Create" || w.Create.Parent != "ws-4k2" {
			t.Fatalf("write %+v", w)
		}
		if w.Create.NoInheritLabels != drop {
			t.Errorf("drop %v: NoInheritLabels %v", drop, w.Create.NoInheritLabels)
		}
	}
}

func TestRawBlockedStatusGetsAHint(t *testing.T) {
	r := newRig(t, 100, 30)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e")
	f := r.form()
	f.f.FocusKey(fStatus)
	f.f.Field(fStatus).Set("blocked")
	f.edited(f.f.Field(fStatus))
	if !strings.Contains(screen(r.a), "raw status") {
		t.Errorf("no raw-blocked hint:\n%s", screen(r.a))
	}
}

func TestDirtyFormAsksBeforeDiscarding(t *testing.T) {
	r := newRig(t, 100, 30)
	r.key("n")
	r.key("esc")
	if isOpen[*issueForm](r.a) {
		t.Fatal("a clean form should close at once")
	}
	r.key("n")
	r.text("x")
	r.key("esc")
	if !isOpen[*confirmDialog](r.a) {
		t.Fatal("no discard question for a dirty form")
	}
	r.key("n")
	if !isOpen[*issueForm](r.a) {
		t.Fatal("declining did not bring the form back")
	}
	r.key("esc", "y")
	if isOpen[*issueForm](r.a) || isOpen[*confirmDialog](r.a) {
		t.Error("y did not discard")
	}
}

func TestAdvancedRowOpensWhenAFieldIsSet(t *testing.T) {
	r := newRig(t, 100, 80)
	r.external("ws-9qe", bd.UpdateSpec{ExternalRef: ptr("gh-12")})
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e")
	if out := screen(r.a); !strings.Contains(out, "gh-12") {
		t.Errorf("advanced fields stay folded although one is set:\n%s", out)
	}
	r2 := newRig(t, 100, 80)
	r2.a.sess.SetCurrent("ws-2hz")
	r2.key("e")
	if out := screen(r2.a); !strings.Contains(out, "Advanced") || strings.Contains(out, "External") {
		t.Errorf("advanced fields not folded:\n%s", out)
	}
}

func TestTeatestCreateFlow(t *testing.T) {
	fake := fakeWorkspace(t)
	a := New(testOptions(plain, func(o *Options) {
		withView("tree", false)(o)
		o.Client, o.Now = fake, time.Now
		o.NewEngine = func(s bd.Session) Engine {
			return refresh.New(refresh.Options{Client: fake, Session: s})
		}
	}))
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(100, 30))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Checkout: guest orders"))
	}, teatest.WithDuration(10*time.Second))
	tm.Send(keyMsg("n"))
	for _, c := range "Fresh idea" {
		if c == ' ' {
			tm.Send(keyMsg("space"))
			continue
		}
		tm.Send(keyMsg(string(c)))
	}
	tm.Send(keyMsg("ctrl+s"))
	newRow := regexp.MustCompile(`\|[ +]+\S+ ws-\S+ Fresh idea`)
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return newRow.MatchString(ansi.Strip(string(b)))
	}, teatest.WithDuration(10*time.Second))
	tm.Send(keyMsg("q"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(10*time.Second)).(*App)
	if !ok {
		t.Fatal("final model is not *ui.App")
	}
	cur := final.sess.Current()
	is, found := final.snap.Issue(cur)
	if !found || is.Title != "Fresh idea" {
		t.Errorf("current issue %q is not the new one", cur)
	}
	if isOpen[*issueForm](final) {
		t.Error("the form is still open")
	}
}

func TestCtrlCWithDirtyFormAsksBeforeQuitting(t *testing.T) {
	r := newRig(t, 100, 30)
	r.line("new Draft")
	r.form()
	r.text("x")
	r.key("ctrl+c")
	if r.a.quitting || !isOpen[*confirmDialog](r.a) {
		t.Fatalf("quitting %v, confirm open %v:\n%s", r.a.quitting, isOpen[*confirmDialog](r.a), screen(r.a))
	}
	r.key("n")
	if r.a.quitting || !isOpen[*issueForm](r.a) {
		t.Fatalf("n quit or lost the form:\n%s", screen(r.a))
	}
	r.key("ctrl+c", "y")
	if !r.a.quitting {
		t.Error("y did not quit")
	}
}

func TestMeWithoutActorIsAnError(t *testing.T) {
	setup := func() *writeRig {
		r := newRig(t, 100, 30)
		r.a.o.Actor = ""
		r.a.sess.SetCurrent("ws-9qe")
		return r
	}
	r := setup()
	r.line("assign me")
	if len(r.fake.Writes()) != 0 || r.issue("ws-9qe").Assignee != "" {
		t.Errorf("assign me without an actor wrote: %v", r.fake.Writes())
	}

	r = setup()
	r.key("a")
	q, _ := r.a.topDialog().(*quickDialog)
	if q == nil {
		t.Fatalf("no quick dialog:\n%s", screen(r.a))
	}
	q.f.Focused().Set("me")
	q.submit()
	if len(r.fake.Writes()) != 0 || !strings.Contains(q.errShort, "who you are") {
		t.Errorf("quick: writes %v, error %q", r.fake.Writes(), q.errShort)
	}

	r = setup()
	r.key("e")
	f := r.form()
	f.f.Field(fAssignee).Set("me")
	r.key("ctrl+s")
	if len(r.fake.Writes()) != 0 || !isOpen[*issueForm](r.a) {
		t.Errorf("form: writes %v", r.fake.Writes())
	}
}

func TestExpandedErrorSaysWhenItIsCut(t *testing.T) {
	r := newRig(t, 100, 40)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e")
	f := r.form()
	f.errShort = "many"
	for i := range 20 {
		f.errFull = append(f.errFull, "line "+strings.Repeat("x", i+1))
	}
	f.errOpen = true
	rows := f.errorRows(r.a.look, 90)
	if last := ansi.Strip(rows[len(rows)-1]); !strings.Contains(last, "+12 more") {
		t.Errorf("no cut marker: %q", last)
	}
}

func TestEditFormLoadsTheDeferDate(t *testing.T) {
	r := newRig(t, 100, 30)
	r.external("ws-9qe", bd.UpdateSpec{Defer: ptr("2030-01-15")})
	r.snapshot()
	r.a.sess.SetCurrent("ws-9qe")
	r.key("e")
	f := r.form()
	if got := f.f.Field(fDefer).Value(); got != "2030-01-15" {
		t.Fatalf("defer field %q", got)
	}
	if f.f.Changes() != 0 {
		t.Errorf("a loaded defer date counts as a change")
	}
	f.f.Field(fDefer).Set("")
	r.key("ctrl+s")
	if w := lastWrite(t, r); w.Update.Defer == nil || *w.Update.Defer != "" {
		t.Errorf("clearing the date wrote %+v", w.Update)
	}
	if is := r.issue("ws-9qe"); !is.DeferUntil.IsZero() {
		t.Errorf("still deferred until %v", is.DeferUntil)
	}
}
