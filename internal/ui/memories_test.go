package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/testgolden"
)

func memRig(t testing.TB, cols, rows int) *writeRig {
	t.Helper()
	r := newRig(t, cols, rows)
	r.fake.SetMemory("alpha-key", "First memory\nwith a second line")
	r.fake.SetMemory("beta-key", "Second memory")
	r.key("5")
	return r
}

func memoryContent(r *writeRig, key string) (string, bool) {
	r.t.Helper()
	list, err := r.fake.Memories(context.Background())
	if err != nil {
		r.t.Fatal(err)
	}
	for _, m := range list {
		if m.Key == key {
			return m.Content, true
		}
	}
	return "", false
}

func TestMemoriesViewReadsAndListsByKey(t *testing.T) {
	r := memRig(t, 120, 30)
	s := screen(r.a)
	for _, want := range []string{"alpha-key", "beta-key", "First memory", "2 · "} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
	if strings.Index(s, "alpha-key") > strings.Index(s, "beta-key") {
		t.Error("keys not in byte order")
	}
}

func rowContaining(s, sub string) int {
	for i, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

func TestMemoriesViewLayoutsByWidth(t *testing.T) {
	r := memRig(t, 60, 30)
	if rowContaining(screen(r.a), "2 lines") >= 0 {
		t.Error("width 60 shows a preview")
	}
	for _, w := range []int{72, 120} {
		s := screen(memRig(t, w, 30).a)
		if at, list := rowContaining(s, "2 lines"), rowContaining(s, "alpha-key  "); at != list && at != rowContaining(s, "Memories -") {
			t.Errorf("width %d: preview header on line %d, not beside the list (%d)", w, at, list)
		}
	}
}

func TestMemoriesAreNotReadWhileHidden(t *testing.T) {
	r := newRig(t, 120, 30)
	r.fake.SetMemory("alpha-key", "x")
	r.a.memRefresh()
	r.key("j", "k")
	if n := count(r.fake.Calls(), "Memories"); n != 0 {
		t.Fatalf("read %d times while hidden", n)
	}
	r.key("5")
	if n := count(r.fake.Calls(), "Memories"); n != 1 {
		t.Errorf("read %d times on showing", n)
	}
}

func TestMemoriesSearchFilters(t *testing.T) {
	r := memRig(t, 120, 30)
	r.key("/")
	r.text("beta")
	r.key("enter")
	s := screen(r.a)
	if strings.Contains(s, "alpha-key") || !strings.Contains(s, "beta-key") {
		t.Errorf("search did not filter:\n%s", s)
	}
}

func TestRememberCommandStoresWithSlugKey(t *testing.T) {
	r := memRig(t, 120, 30)
	r.line("remember Use tabs in Makefiles")
	if !isOpen[*memoryDialog](r.a) {
		t.Fatal("dialog not open")
	}
	r.key("ctrl+s")
	if got, ok := memoryContent(r, "use-tabs-in-makefiles"); !ok || got != "Use tabs in Makefiles" {
		t.Errorf("stored %q %v", got, ok)
	}
	if isOpen[*memoryDialog](r.a) {
		t.Error("dialog still open")
	}
}

func TestRenameRemembersThenForgets(t *testing.T) {
	r := memRig(t, 120, 30)
	r.key("e")
	if !isOpen[*memoryDialog](r.a) {
		t.Fatal("no dialog")
	}
	d := r.a.topDialog().(*memoryDialog)
	d.f.Field(fMemKey).Set("gamma-key")
	r.key("ctrl+s")
	if _, ok := memoryContent(r, "gamma-key"); !ok {
		t.Error("new key missing")
	}
	if _, ok := memoryContent(r, "alpha-key"); ok {
		t.Error("old key still there")
	}
	var methods []string
	for _, w := range r.fake.Writes() {
		methods = append(methods, w.Method)
	}
	if got := strings.Join(methods, ","); got != "Remember,Forget" {
		t.Errorf("writes %s", got)
	}
}

func TestRenameFailureKeepsBothAndSaysSo(t *testing.T) {
	r := memRig(t, 120, 30)
	r.fake.FailWith("Forget", errors.New("boom"))
	r.key("e")
	r.a.topDialog().(*memoryDialog).f.Field(fMemKey).Set("gamma-key")
	r.key("ctrl+s")
	if !isOpen[*memoryDialog](r.a) {
		t.Fatal("dialog closed")
	}
	if !strings.Contains(screen(r.a), "both keys now exist") {
		t.Errorf("no message:\n%s", screen(r.a))
	}
	for _, k := range []string{"alpha-key", "gamma-key"} {
		if _, ok := memoryContent(r, k); !ok {
			t.Errorf("%s missing", k)
		}
	}
	if r.a.topDialog().(*memoryDialog).dirty() {
		t.Error("the fields still count as edited after the partial rename")
	}
}

func TestEditDetectsChangeSinceOpened(t *testing.T) {
	r := memRig(t, 120, 30)
	r.key("e")
	r.fake.SetMemory("alpha-key", "Someone else wrote this")
	d := r.a.topDialog().(*memoryDialog)
	d.f.Field(fMemContent).Set("My edit")
	r.key("ctrl+s")
	if !isOpen[*memoryDialog](r.a) {
		t.Fatal("dialog closed")
	}
	if got, _ := memoryContent(r, "alpha-key"); got != "Someone else wrote this" {
		t.Errorf("overwritten: %q", got)
	}
	if !strings.Contains(screen(r.a), "Changed since opened") {
		t.Errorf("no banner:\n%s", screen(r.a))
	}
}

func TestRememberOnExistingKeyAsksToOverwrite(t *testing.T) {
	r := memRig(t, 120, 30)
	r.key("n")
	d := r.a.topDialog().(*memoryDialog)
	d.f.Field(fMemKey).Set("beta-key")
	d.f.Field(fMemContent).Set("Replacement")
	r.key("ctrl+s")
	if !isOpen[*memOverwriteDialog](r.a) {
		t.Fatalf("no prompt:\n%s", screen(r.a))
	}
	r.key("b")
	if got, _ := memoryContent(r, "beta-key-2"); got != "Replacement" {
		t.Errorf("keep both stored %q", got)
	}
	if got, _ := memoryContent(r, "beta-key"); got != "Second memory" {
		t.Errorf("original changed: %q", got)
	}
}

func TestOverwriteReplacesTheMemory(t *testing.T) {
	r := memRig(t, 120, 30)
	r.key("n")
	d := r.a.topDialog().(*memoryDialog)
	d.f.Field(fMemKey).Set("beta-key")
	d.f.Field(fMemContent).Set("Replacement")
	r.key("ctrl+s", "o")
	if got, _ := memoryContent(r, "beta-key"); got != "Replacement" {
		t.Errorf("stored %q", got)
	}
}

func TestForgetMarkedNeedsConfirmation(t *testing.T) {
	r := memRig(t, 120, 30)
	r.key("space", "j", "space", "d")
	if !isOpen[*confirmDialog](r.a) {
		t.Fatalf("no confirm:\n%s", screen(r.a))
	}
	if !strings.Contains(screen(r.a), "Forget 2 memories?") {
		t.Error("title")
	}
	r.key("y")
	list, _ := r.fake.Memories(context.Background())
	if len(list) != 0 {
		t.Errorf("left %v", list)
	}
}

func TestForgetCommandCompletesKeys(t *testing.T) {
	r := memRig(t, 120, 30)
	c := r.a.cmds.Complete("forget al", nil)
	if len(c.Cands) != 1 || c.Cands[0] != "alpha-key" {
		t.Errorf("completion %+v", c)
	}
}

func journalVersion(t testing.TB) bd.VersionInfo {
	t.Helper()
	v, err := bd.CheckVersion("1.3.0")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestEmptyMemoriesShowsHints(t *testing.T) {
	r := newRig(t, 120, 30)
	r.key("5")
	if s := screen(r.a); !strings.Contains(s, "No memories yet.") || !strings.Contains(s, "bd remember") {
		t.Errorf("empty state:\n%s", s)
	}
}

func TestJournalOptInEnablesEvents(t *testing.T) {
	r := newRig(t, 120, 30)
	r.fake.SetVersion("1.3.0")
	r.a.o.Journal = &memJournal{}
	r.a.bds.Version = journalVersion(t)
	r.a.maybeAskJournal()
	if !isOpen[*journalDialog](r.a) {
		t.Fatal("no dialog")
	}
	r.key("y")
	if r.eng.events != 1 {
		t.Errorf("events enabled %d times", r.eng.events)
	}
	if got := r.a.o.Journal.Journal(r.a.bds.Workspace.Path); got != config.JournalUnasked {
		t.Errorf("stored %q", got)
	}
}

func TestJournalDeclinedShowsHint(t *testing.T) {
	r := newRig(t, 120, 30)
	r.a.o.Journal = &memJournal{}
	r.a.bds.Version = journalVersion(t)
	r.a.maybeAskJournal()
	r.key("N")
	if !strings.Contains(chipText(r.a), "limited: no actors") {
		t.Errorf("chips %q", chipText(r.a))
	}
	if r.a.o.Journal.Journal(r.a.bds.Workspace.Path) != config.JournalDeclined {
		t.Error("not stored")
	}
}

func goldenMemRig(t testing.TB, cols, rows int) *writeRig {
	t.Helper()
	r := newRig(t, cols, rows)
	r.fake.SetMemory("dolt-push-needs-remote", "Run bd dolt push only after the remote is set.\n\n- check `bd dolt remote`\n- never force")
	r.fake.SetMemory("golden-update", "Regenerate goldens with `just golden-update` and review the diff.")
	r.fake.SetMemory("no-tabs", "Go files use tabs; everything else uses two spaces.")
	r.fake.SetMemory("test-pump", "Engine-driven tests share one persistent pump.")
	r.key("5")
	return r
}

func TestMemoriesViewGoldens(t *testing.T) {
	for _, s := range [][2]int{{60, 16}, {60, 20}, {90, 24}, {120, 30}} {
		t.Run(fmt.Sprintf("%dx%d", s[0], s[1]), func(t *testing.T) {
			testgolden.Equal(t, screen(goldenMemRig(t, s[0], s[1]).a))
		})
	}
	t.Run("search", func(t *testing.T) {
		r := goldenMemRig(t, 120, 30)
		r.key("/")
		r.text("golden")
		r.key("enter")
		testgolden.Equal(t, screen(r.a))
	})
}

func TestMemoryDialogGoldens(t *testing.T) {
	for _, s := range [][2]int{{60, 16}, {80, 24}} {
		t.Run(fmt.Sprintf("new-%dx%d", s[0], s[1]), func(t *testing.T) {
			r := goldenMemRig(t, s[0], s[1])
			r.line("remember Prefer small commits")
			testgolden.Equal(t, screen(r.a))
		})
		t.Run(fmt.Sprintf("edit-%dx%d", s[0], s[1]), func(t *testing.T) {
			r := goldenMemRig(t, s[0], s[1])
			r.key("j", "e")
			testgolden.Equal(t, screen(r.a))
		})
	}
}

func refreshMemories(r *writeRig) {
	r.t.Helper()
	r.a.memRefresh()
	r.run(send(r.a, tea.WindowSizeMsg{Width: r.a.cols, Height: r.a.rows}))
}

func TestBannerAppearsWhenARefreshFindsTheMemoryChanged(t *testing.T) {
	r := memRig(t, 120, 30)
	r.key("e")
	r.fake.SetMemory("alpha-key", "Someone else wrote this")
	refreshMemories(r)
	if !strings.Contains(screen(r.a), "Changed since opened") {
		t.Errorf("no banner:\n%s", screen(r.a))
	}
	r.fake.SetMemory("alpha-key", "First memory\nwith a second line")
	refreshMemories(r)
	if strings.Contains(screen(r.a), "Changed since opened") {
		t.Error("banner stays after the content is back")
	}
}

func TestGoneMemoryBannerAndUnchangedSaveRecreates(t *testing.T) {
	r := memRig(t, 120, 30)
	r.key("e")
	if err := r.fake.Forget(context.Background(), "alpha-key"); err != nil {
		t.Fatal(err)
	}
	refreshMemories(r)
	if !strings.Contains(screen(r.a), "Forgotten since opened") {
		t.Fatalf("no banner:\n%s", screen(r.a))
	}
	r.key("ctrl+s")
	if got, ok := memoryContent(r, "alpha-key"); !ok || got != "First memory\nwith a second line" {
		t.Errorf("not recreated: %q %v", got, ok)
	}
}

func TestForgottenElsewhereIsCounted(t *testing.T) {
	r := memRig(t, 120, 30)
	if err := r.fake.Forget(context.Background(), "beta-key"); err != nil {
		t.Fatal(err)
	}
	refreshMemories(r)
	if c := chipText(r.a); !strings.Contains(c, "1 forgotten elsewhere") {
		t.Errorf("chips %q", c)
	}
}

func TestForgetCountsGoneAsDone(t *testing.T) {
	r := memRig(t, 120, 30)
	r.a.mem.marks["alpha-key"], r.a.mem.marks["ghost"] = true, true
	r.run(r.a.forgetKeys([]string{"alpha-key", "ghost", "beta-key"}))
	list, _ := r.fake.Memories(context.Background())
	if len(list) != 0 || len(r.a.mem.marks) != 0 {
		t.Errorf("left %v, marks %v", list, r.a.mem.marks)
	}
	if !strings.Contains(screen(r.a), "2 memories, 1 gone already") && !noticeHas(r.a, "1 gone already") {
		t.Errorf("no note:\n%s", screen(r.a))
	}
}

func TestForgettingAGoneKeySaysSo(t *testing.T) {
	r := memRig(t, 120, 30)
	r.run(r.a.forgetKeys([]string{"ghost"}))
	if !noticeHas(r.a, "ghost was gone already") {
		t.Errorf("notices %v", r.a.notices)
	}
}

func TestBulkForgetStopsAtTheFirstErrorAndKeepsMarks(t *testing.T) {
	r := memRig(t, 120, 30)
	r.fake.FailWith("Forget", errors.New("boom"))
	r.a.mem.marks["alpha-key"], r.a.mem.marks["beta-key"] = true, true
	r.run(r.a.forgetKeys([]string{"alpha-key", "beta-key"}))
	if n := count(r.fake.Calls(), "Forget"); n != 1 {
		t.Errorf("%d forget calls", n)
	}
	if len(r.a.mem.marks) != 2 {
		t.Errorf("marks %v", r.a.mem.marks)
	}
	if !noticeHas(r.a, "stopped at alpha-key") {
		t.Errorf("notices %v", r.a.notices)
	}
}

func TestBulkForgetTakesMarksHiddenBySearch(t *testing.T) {
	r := memRig(t, 120, 30)
	r.key("space", "j", "space")
	r.key("/")
	r.text("beta")
	r.key("enter")
	r.key("d")
	if !isOpen[*confirmDialog](r.a) {
		t.Fatal("no confirm")
	}
	out := screen(r.a)
	if !strings.Contains(out, "Forget 2 memories?") || !strings.Contains(out, "1 of them are hidden by the search") {
		t.Errorf("confirm:\n%s", out)
	}
}

func TestMemoryReadErrorShowsInDetails(t *testing.T) {
	r := newRig(t, 120, 30)
	r.fake.FailWith("Memories", errors.New("dolt is down"))
	r.key("5")
	r.key("!")
	if !strings.Contains(screen(r.a), "dolt is down") {
		t.Errorf("details:\n%s", screen(r.a))
	}
}

func noticeHas(a *App, sub string) bool {
	for _, n := range a.notices {
		if strings.Contains(n.Text, sub) {
			return true
		}
	}
	return false
}

func journalRig(t testing.TB) *writeRig {
	t.Helper()
	r := newRig(t, 120, 30)
	r.a.o.Journal = &memJournal{}
	r.a.bds.Version = journalVersion(t)
	return r
}

func TestJournalQuestionWaitsForOpenDialogs(t *testing.T) {
	r := journalRig(t)
	r.key("?")
	r.a.maybeAskJournal()
	if isOpen[*journalDialog](r.a) || r.a.journalAsked {
		t.Fatal("asked over another dialog")
	}
	r.key("esc")
	r.a.maybeAskJournal()
	if !isOpen[*journalDialog](r.a) {
		t.Fatal("not asked once the dialog closed")
	}
}

func TestJournalNotNowKeepsTheChipAndStoresNothing(t *testing.T) {
	r := journalRig(t)
	r.a.maybeAskJournal()
	r.key("n")
	if !strings.Contains(chipText(r.a), "limited: no actors") {
		t.Errorf("chips %q", chipText(r.a))
	}
	if got := r.a.o.Journal.Journal(r.a.bds.Workspace.Path); got != config.JournalUnasked {
		t.Errorf("stored %q", got)
	}
}

func TestJournalNeverIsUndoneByTheCommand(t *testing.T) {
	r := journalRig(t)
	r.a.maybeAskJournal()
	r.key("N")
	r.line("journal")
	if !isOpen[*journalDialog](r.a) {
		t.Fatalf("no dialog:\n%s", screen(r.a))
	}
	r.key("y")
	if r.eng.events != 1 || r.a.journalDeclined {
		t.Errorf("events %d declined %v", r.eng.events, r.a.journalDeclined)
	}
	if got := r.a.o.Journal.Journal(r.a.bds.Workspace.Path); got != config.JournalUnasked {
		t.Errorf("stored %q after enabling", got)
	}
}

func TestJournalStoredEnabledWithJournalOffAsksAgain(t *testing.T) {
	r := journalRig(t)
	if err := r.a.o.Journal.SetJournal(r.a.bds.Workspace.Path, config.JournalEnabled); err != nil {
		t.Fatal(err)
	}
	r.a.maybeAskJournal()
	if !isOpen[*journalDialog](r.a) {
		t.Fatal("not asked again")
	}
}

func TestJournalDialogWarnsAboutTheSharedConfig(t *testing.T) {
	r := journalRig(t)
	r.a.maybeAskJournal()
	out := screen(r.a)
	if !strings.Contains(out, "beads config.yaml") || !strings.Contains(out, "every writer") {
		t.Errorf("dialog:\n%s", out)
	}
}
