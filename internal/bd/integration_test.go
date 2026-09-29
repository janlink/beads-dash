package bd

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/testbd"
)

const seeded = "seeded"

func TestMain(m *testing.M) {
	os.Exit(testbd.Run(m, testbd.Options{Recipes: map[string]testbd.Recipe{seeded: seedWorkspace}}))
}

// seedWorkspace builds an epic that bd calls blocked (by another epic), so
// its children are blocked by parent propagation, plus a free task, a closed
// and a deferred one, a custom status, comments and a memory.
func seedWorkspace(e testbd.Env) error {
	if _, err := e.Bd("config", "set", "status.custom", "review:wip"); err != nil {
		return err
	}
	create := func(title string, args ...string) (string, error) {
		out, err := e.Bd(append([]string{"create", "--silent", "--title=" + title}, args...)...)
		return strings.TrimSpace(out), err
	}
	id := map[string]string{}
	steps := []struct {
		title string
		args  []string
	}{
		{"Epic A", []string{"--type=epic"}},
		{"Blocker epic", []string{"--type=epic"}},
		{"Free task", []string{"--type=task"}},
		{"Closed task", []string{"--type=task"}},
		{"Deferred task", []string{"--type=task"}},
		{"Review task", []string{"--type=task"}},
	}
	for _, s := range steps {
		out, err := create(s.title, s.args...)
		if err != nil {
			return err
		}
		id[s.title] = out
	}
	for _, s := range []struct{ title, parent string }{
		{"Open child", "Epic A"}, {"WIP child", "Epic A"},
	} {
		out, err := create(s.title, "--type=task", "--parent="+id[s.parent])
		if err != nil {
			return err
		}
		id[s.title] = out
	}
	cmds := [][]string{
		{"dep", "add", id["Epic A"], id["Blocker epic"]},
		{"update", id["WIP child"], "--status", "in_progress"},
		{"update", id["Deferred task"], "--status", "deferred"},
		{"update", id["Review task"], "--status", "review"},
		{"close", id["Closed task"], "--reason=done"},
		{"comment", id["WIP child"], "first comment"},
		{"comment", id["WIP child"], "second comment"},
		{"remember", "integration memory", "--key", "note-1"},
	}
	for _, c := range cmds {
		if _, err := e.Bd(c...); err != nil {
			return err
		}
	}
	return nil
}

func forEachVersion(t *testing.T, f func(t *testing.T, w testbd.Workspace, c *ExecClient)) {
	t.Helper()
	for _, v := range testbd.Versions {
		t.Run("bd-"+v, func(t *testing.T) {
			w := testbd.NewNamed(t, v, seeded)
			f(t, w, NewExec(ExecOptions{Bin: w.Bin, Dir: w.Dir}))
		})
	}
}

func titleIndex(s *model.Snapshot) map[string]string {
	out := map[string]string{}
	for _, id := range s.IDs() {
		is, _ := s.Issue(id)
		out[is.Title] = id
	}
	return out
}

func TestIntegrationSnapshotAndSession(t *testing.T) {
	forEachVersion(t, func(t *testing.T, w testbd.Workspace, c *ExecClient) {
		ctx := context.Background()
		sess, err := OpenSession(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if sess.Version.Raw != w.Version || sess.Version.Support != Supported {
			t.Errorf("version = %+v", sess.Version)
		}
		if sess.Workspace.Prefix != "t" || sess.Workspace.Path == "" {
			t.Errorf("workspace = %+v", sess.Workspace)
		}
		if sess.Statuses.Category("review") != model.CategoryWIP || sess.Statuses.Category("closed") != model.CategoryDone {
			t.Errorf("statuses = %+v", sess.Statuses.All())
		}
		if sess.EventsJournal {
			t.Error("journal is off in a fresh workspace")
		}

		snap, err := FetchSnapshot(ctx, c, nil)
		if err != nil {
			t.Fatal(err)
		}
		id := titleIndex(snap)
		if snap.Len() != 8 {
			t.Fatalf("snapshot has %d issues, want 8", snap.Len())
		}
		for _, title := range []string{"Open child", "WIP child"} {
			if !snap.IsBlocked(id[title]) || snap.IsReady(id[title]) {
				t.Errorf("%s: bd's verdict is blocked by the parent's dependency, got blocked=%v ready=%v",
					title, snap.IsBlocked(id[title]), snap.IsReady(id[title]))
			}
		}
		if !snap.IsReady(id["Free task"]) || snap.IsBlocked(id["Free task"]) {
			t.Error("free task must be ready")
		}
		st := sess.Statuses
		want := map[string]model.Presentation{
			"Open child":    {Status: model.Blocked},
			"WIP child":     {Status: model.InProgress, BlockedMarker: true},
			"Free task":     {Status: model.Open},
			"Closed task":   {Status: model.Closed},
			"Deferred task": {Status: model.Frozen},
			"Review task":   {Status: model.InProgress},
		}
		for title, p := range want {
			if got := snap.Present(id[title], st); got != p {
				t.Errorf("%s presents as %+v, want %+v", title, got, p)
			}
		}
		if !snap.IsContainer(id["Epic A"]) || snap.IsContainer(id["Free task"]) {
			t.Error("containers")
		}
		p, ok := snap.Progress(id["Epic A"], st)
		if !ok || p.Direct != (model.Count{Closed: 0, Total: 2}) {
			t.Errorf("progress = %+v, %v", p, ok)
		}
		dependents := snap.Dependents(id["Blocker epic"])
		if len(dependents) != 1 || dependents[0].From != id["Epic A"] || dependents[0].Type != "blocks" {
			t.Errorf("dependents of the blocker = %+v", dependents)
		}
		again, err := FetchSnapshot(ctx, c, nil)
		if err != nil || again.Fingerprint() != snap.Fingerprint() {
			t.Errorf("fingerprint not stable across polls: %v", err)
		}
		for _, id := range snap.IDs() {
			is, _ := snap.Issue(id)
			if !strings.Contains(string(is.Raw), `"id"`) {
				t.Errorf("%s: Raw = %s", id, is.Raw)
			}
		}
	})
}

func TestIntegrationReads(t *testing.T) {
	forEachVersion(t, func(t *testing.T, w testbd.Workspace, c *ExecClient) {
		ctx := context.Background()
		snap, err := FetchSnapshot(ctx, c, nil)
		if err != nil {
			t.Fatal(err)
		}
		wip := titleIndex(snap)["WIP child"]
		comments, err := c.Comments(ctx, wip)
		if err != nil || len(comments) != 2 {
			t.Fatalf("comments = %+v, %v", comments, err)
		}
		if comments[0].Text != "first comment" || comments[1].Text != "second comment" || comments[0].Author == "" {
			t.Errorf("comments = %+v", comments)
		}
		hist, err := c.History(ctx, wip)
		if err != nil || len(hist) == 0 || hist[0].Issue.ID != wip || hist[0].CommitHash == "" {
			t.Errorf("history = %+v, %v", hist, err)
		}
		mem, err := c.Memories(ctx)
		if err != nil || len(mem) != 1 || mem[0].Key != "note-1" || mem[0].Content != "integration memory" {
			t.Errorf("memories = %+v, %v", mem, err)
		}
		vc, err := c.VCStatus(ctx)
		if err != nil || vc.Commit == "" || vc.Branch == "" {
			t.Errorf("vc status = %+v, %v", vc, err)
		}
		ty, err := c.Types(ctx)
		if err != nil || len(ty) < 9 {
			t.Errorf("types = %+v, %v", ty, err)
		}
		v, err := c.ConfigGet(ctx, "events-journal")
		if err != nil || (v.Value != "" && v.Value != "false") {
			t.Errorf("events-journal = %+v, %v", v, err)
		}
		on, err := journalEnabled(ctx, c, capabilitiesFor(mustVersion(t, w.Version)))
		if err != nil || on {
			t.Errorf("JournalEnabled = %v, %v", on, err)
		}
	})
}

func mustVersion(t *testing.T, s string) Version {
	t.Helper()
	v, err := ParseVersion(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestIntegrationBeadsDir(t *testing.T) {
	forEachVersion(t, func(t *testing.T, w testbd.Workspace, c *ExecClient) {
		ctx := context.Background()
		ws, err := c.Where(ctx)
		if err != nil {
			t.Fatal(err)
		}
		elsewhere := t.TempDir()
		lost := NewExec(ExecOptions{Bin: w.Bin, Dir: elsewhere})
		if _, err := lost.Where(ctx); !IsClass(err, ClassNotWorkspace) {
			t.Fatalf("outside a workspace: %v", err)
		}

		via := NewExec(ExecOptions{Bin: w.Bin, Dir: elsewhere, Env: []string{"BEADS_DIR=" + ws.Path}})
		got, err := via.Where(ctx)
		if err != nil || got.Path != ws.Path {
			t.Fatalf("Where via BEADS_DIR = %+v, %v", got, err)
		}
		snap, err := FetchSnapshot(ctx, via, nil)
		if err != nil || snap.Len() != 8 {
			t.Errorf("snapshot via BEADS_DIR: %v, %v", snap, err)
		}
	})
}

func TestIntegrationErrorClasses(t *testing.T) {
	forEachVersion(t, func(t *testing.T, w testbd.Workspace, c *ExecClient) {
		ctx := context.Background()
		lost := NewExec(ExecOptions{Bin: w.Bin, Dir: t.TempDir()})

		if _, err := lost.Where(ctx); !IsClass(err, ClassNotWorkspace) {
			t.Errorf("Where = %v", err)
		}
		_, err := lost.List(ctx)
		err = ClassifyReadFailure(ctx, lost, err)
		if !IsClass(err, ClassTransient) || CodeOf(err) != CodeNoBeadsDirectory {
			t.Errorf("List = %v", err)
		}
		var watch WorkspaceWatch
		var last error
		for i := 0; i < 3; i++ {
			_, err := FetchSnapshot(ctx, lost, nil)
			last = watch.Observe(ClassifyReadFailure(ctx, lost, err))
		}
		if !IsClass(last, ClassNotWorkspace) {
			t.Errorf("after three failed polls: %v", last)
		}

		if _, err := c.Comments(ctx, "t-doesnotexist"); !IsClass(err, ClassTransient) {
			t.Errorf("comments of a missing issue = %v", err)
		}
		missing := NewExec(ExecOptions{Bin: w.Bin + "-missing", Dir: w.Dir})
		if _, err := missing.Version(ctx); !IsClass(err, ClassBdMissing) {
			t.Errorf("missing binary = %v", err)
		}
		info, err := Probe(ctx, w.Bin, Timeouts{})
		if err != nil || info.Raw != w.Version || info.Support != Supported {
			t.Errorf("Probe = %+v, %v", info, err)
		}
	})
}
