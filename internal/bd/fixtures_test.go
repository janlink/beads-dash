package bd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
)

const testdataDir = "../../testdata"

// replay is a Runner that answers from canned results keyed by bd
// subcommand (see commandName).
type replay map[string]Result

func (r replay) Run(ctx context.Context, argv []string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	res, ok := r[commandName(argv)]
	if !ok {
		return Result{ExitCode: 127, Stderr: []byte("replay: no fixture for " + strings.Join(argv, " "))}, nil
	}
	return res, nil
}

func fixture(t testing.TB, rel ...string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{testdataDir}, rel...)...))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixtureOptional(rel ...string) ([]byte, bool) {
	b, err := os.ReadFile(filepath.Join(append([]string{testdataDir}, rel...)...))
	return b, err == nil
}

func fixtureVersions(t testing.TB) []string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(testdataDir, "bd-*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no testdata/bd-* directories: %v", err)
	}
	out := make([]string, len(dirs))
	for i, d := range dirs {
		out[i] = strings.TrimPrefix(filepath.Base(d), "bd-")
	}
	return out
}

func rawObjects(t testing.TB, data []byte) []map[string]any {
	t.Helper()
	var env struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatal(err)
	}
	return env.Data
}

func stripRaw(is []model.Issue) []model.Issue {
	out := make([]model.Issue, len(is))
	for i, x := range is {
		x.Raw = nil
		out[i] = x
	}
	return out
}

func TestFixtureListAndReadiness(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		t.Run(v, func(t *testing.T) {
			dir := "bd-" + v
			env := fixture(t, dir, "recipe-run", "envelope", "list-all.json")
			c := NewExec(ExecOptions{Runner: replay{
				"list":  {Stdout: env},
				"ready": {Stdout: fixture(t, dir, "recipe-run", "envelope", "ready-explain.json")},
			}})
			issues, err := c.List(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			want := rawObjects(t, env)
			if len(issues) != len(want) || len(issues) == 0 {
				t.Fatalf("decoded %d issues, fixture has %d", len(issues), len(want))
			}
			byID := map[string]model.Issue{}
			for _, is := range issues {
				byID[is.ID] = is
				if !json.Valid(is.Raw) {
					t.Fatalf("%s: Raw is not valid JSON", is.ID)
				}
			}
			for _, w := range want {
				is, ok := byID[w["id"].(string)]
				if !ok {
					t.Fatalf("issue %v missing", w["id"])
				}
				if is.Title != w["title"] || is.Status != w["status"] || is.IssueType != w["issue_type"] {
					t.Errorf("%s decoded wrongly: %+v", is.ID, is)
				}
				if int(w["priority"].(float64)) != is.Priority {
					t.Errorf("%s priority = %d", is.ID, is.Priority)
				}
				if is.CreatedAt.IsZero() || is.UpdatedAt.IsZero() {
					t.Errorf("%s timestamps not parsed", is.ID)
				}
				deps, _ := w["dependencies"].([]any)
				if len(deps) != len(is.Dependencies) {
					t.Errorf("%s: %d edges, fixture has %d", is.ID, len(is.Dependencies), len(deps))
				}
				if p, _ := w["parent"].(string); p != is.Parent {
					t.Errorf("%s parent = %q, want %q", is.ID, is.Parent, p)
				}
			}

			r, err := c.Ready(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(r.Ready) == 0 || len(r.Blocked) == 0 {
				t.Fatalf("readiness = %+v", r)
			}
			snap := model.NewSnapshot(issues, r, testNow())
			if len(snap.ReadyIDs()) != len(r.Ready) || len(snap.BlockedIDs()) != len(r.Blocked) {
				t.Errorf("ready/blocked IDs missing from list were dropped: %d/%d vs %d/%d",
					len(snap.ReadyIDs()), len(snap.BlockedIDs()), len(r.Ready), len(r.Blocked))
			}
			for _, id := range snap.ReadyIDs() {
				if snap.IsBlocked(id) {
					t.Errorf("%s is both ready and blocked", id)
				}
			}
			assertParentPropagation(t, snap)
			for _, id := range snap.ReadyIDs() {
				if snap.ReadyReason(id) == "" {
					t.Errorf("%s: bd's reason was dropped", id)
				}
			}
		})
	}
}

// assertParentPropagation checks the recipe's blocked epic: bd, not bdash,
// marks its children blocked.
func assertParentPropagation(t *testing.T, snap *model.Snapshot) {
	t.Helper()
	var epic string
	for _, id := range snap.IDs() {
		is, _ := snap.Issue(id)
		if is.Title == "Epic A" {
			epic = id
		}
	}
	if epic == "" {
		t.Fatal("recipe epic not found")
	}
	kids := snap.Children(epic)
	if len(kids) < 3 {
		t.Fatalf("epic has children %v", kids)
	}
	for _, k := range kids {
		if !snap.IsBlocked(k) {
			t.Errorf("child %s of a blocked epic should be blocked per bd", k)
		}
	}
	st := model.BuiltinStatuses()
	for _, k := range kids {
		is, _ := snap.Issue(k)
		p := snap.Present(k, st)
		switch is.Status {
		case "in_progress":
			if p.Status != model.InProgress || !p.BlockedMarker {
				t.Errorf("in-progress blocked child = %+v", p)
			}
		case "open":
			if p.Status != model.Blocked {
				t.Errorf("open blocked child = %+v", p)
			}
		}
	}
	p, ok := snap.Progress(epic, st)
	if !ok || p.Direct.Total != len(kids) {
		t.Errorf("epic progress = %+v, %v", p, ok)
	}
}

func TestFixtureLegacyEqualsEnvelope(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		t.Run(v, func(t *testing.T) {
			dir := "bd-" + v
			env := NewExec(ExecOptions{Runner: replay{
				"list":     {Stdout: fixture(t, dir, "recipe-run", "envelope", "list-all.json")},
				"ready":    {Stdout: fixture(t, dir, "recipe-run", "envelope", "ready-explain.json")},
				"statuses": {Stdout: fixture(t, dir, "recipe-run", "envelope", "statuses.json")},
				"types":    {Stdout: fixture(t, dir, "recipe-run", "envelope", "types.json")},
				"where":    {Stdout: fixture(t, dir, "recipe-run", "envelope", "where.json")},
			}})
			legacy := NewExec(ExecOptions{Runner: replay{
				"list":     {Stdout: fixture(t, dir, "recipe-run", "list-all.json")},
				"ready":    {Stdout: fixture(t, dir, "recipe-run", "ready-explain.json")},
				"statuses": {Stdout: fixture(t, dir, "recipe-run", "statuses.json")},
				"types":    {Stdout: fixture(t, dir, "recipe-run", "types.json")},
				"where":    {Stdout: fixture(t, dir, "recipe-run", "where.json")},
			}})
			ctx := context.Background()
			a, err := env.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			b, err := legacy.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(stripRaw(a), stripRaw(b)) {
				t.Error("legacy and envelope list differ")
			}
			ra, _ := env.Ready(ctx)
			rb, err := legacy.Ready(ctx)
			if err != nil || !reflect.DeepEqual(ra, rb) {
				t.Errorf("legacy and envelope ready differ: %v", err)
			}
			sa, _ := env.Statuses(ctx)
			sb, err := legacy.Statuses(ctx)
			if err != nil || !reflect.DeepEqual(sa.All(), sb.All()) {
				t.Errorf("legacy and envelope statuses differ: %v", err)
			}
			ta, _ := env.Types(ctx)
			tb, err := legacy.Types(ctx)
			if err != nil || !reflect.DeepEqual(ta, tb) {
				t.Errorf("legacy and envelope types differ: %v", err)
			}
			wa, _ := env.Where(ctx)
			wb, err := legacy.Where(ctx)
			if err != nil || wa != wb || wa.Prefix == "" || wa.Path == "" {
				t.Errorf("legacy and envelope where differ: %+v %+v %v", wa, wb, err)
			}
		})
	}
}

func TestFixtureMetaAndVersion(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		t.Run(v, func(t *testing.T) {
			dir := "bd-" + v
			c := NewExec(ExecOptions{Runner: replay{
				"statuses": {Stdout: fixture(t, dir, "recipe-run", "envelope", "statuses.json")},
				"types":    {Stdout: fixture(t, dir, "recipe-run", "envelope", "types.json")},
				"version":  {Stdout: fixture(t, dir, "recipe-run", "envelope", "version.json")},
			}})
			ctx := context.Background()
			info, err := c.Version(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if info.Raw != v || info.Support != Supported {
				t.Errorf("version = %+v, want supported %s", info, v)
			}
			if info.caps.eventsJournal != (v != "1.2.2") {
				t.Errorf("EventsJournal = %v for %s", info.caps.eventsJournal, v)
			}
			st, err := c.Statuses(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for name, cat := range map[string]model.Category{
				"open": model.CategoryActive, "in_progress": model.CategoryWIP, "blocked": model.CategoryWIP,
				"deferred": model.CategoryFrozen, "closed": model.CategoryDone, "pinned": model.CategoryFrozen,
				"hooked": model.CategoryWIP, "review": model.CategoryWIP,
			} {
				if got := st.Category(name); got != cat {
					t.Errorf("Category(%s) = %q, want %q", name, got, cat)
				}
			}
			if st.Category("parked") != model.CategoryUnknown {
				t.Error("orphaned status parked must be unknown")
			}
			ty, err := c.Types(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var core, custom int
			for _, x := range ty {
				if x.Custom {
					custom++
				} else {
					core++
				}
			}
			if core != 9 || custom != 1 || ty[len(ty)-1].Name != "incident" {
				t.Errorf("types core=%d custom=%d: %+v", core, custom, ty)
			}
		})
	}
}

func TestFixtureAuditReads(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		t.Run(v, func(t *testing.T) {
			sets := [][]string{{"history-memories"}, {"extras", "envelope"}, {"m1"}}
			var ran int
			for _, set := range sets {
				comments, ok := fixtureOptional(append(append([]string{"bd-" + v}, set...), "comments.json")...)
				if !ok {
					continue
				}
				ran++
				c := NewExec(ExecOptions{Runner: replay{"comments": {Stdout: comments}}})
				got, err := c.Comments(context.Background(), "x-1")
				if err != nil {
					t.Fatalf("%v: %v", set, err)
				}
				for i := 1; i < len(got); i++ {
					if got[i].CreatedAt.Before(got[i-1].CreatedAt) {
						t.Errorf("%v: comments not ordered by created_at", set)
					}
				}
				if len(got) == 0 || got[0].Author == "" || got[0].Text == "" {
					t.Errorf("%v: comments = %+v", set, got)
				}
			}
			hist, ok := fixtureOptional("bd-"+v, "extras", "envelope", "history.json")
			if ok {
				ran++
				c := NewExec(ExecOptions{Runner: replay{"history": {Stdout: hist}}})
				got, err := c.History(context.Background(), "x-1")
				if err != nil || len(got) == 0 {
					t.Fatalf("history = %v, %v", got, err)
				}
				if got[0].CommitHash == "" || got[0].CommitDate.IsZero() || got[0].Issue.ID == "" {
					t.Errorf("history entry = %+v", got[0])
				}
			}
			if mem, ok := fixtureOptional("bd-"+v, "extras", "envelope", "memories.json"); ok {
				ran++
				c := NewExec(ExecOptions{Runner: replay{"memories": {Stdout: mem}}})
				got, err := c.Memories(context.Background())
				if err != nil || len(got) == 0 || got[0].Key == "" {
					t.Fatalf("memories = %v, %v", got, err)
				}
			}
			if vc, ok := fixtureOptional("bd-"+v, "extras", "envelope", "vc-status.json"); ok {
				ran++
				c := NewExec(ExecOptions{Runner: replay{"vc status": {Stdout: vc}}})
				got, err := c.VCStatus(context.Background())
				if err != nil || got.Branch == "" || got.Commit == "" {
					t.Fatalf("vc status = %+v, %v", got, err)
				}
			}
			if ran == 0 {
				t.Errorf("no audit fixtures found for bd %s", v)
			}
		})
	}
}

func TestFixtureHistoryMemoriesLegacyDir(t *testing.T) {
	c := NewExec(ExecOptions{Runner: replay{
		"history":   {Stdout: fixture(t, "bd-1.2.2", "history-memories", "history.json")},
		"memories":  {Stdout: fixture(t, "bd-1.2.2", "history-memories", "memories-envelope.json")},
		"vc status": {Stdout: fixture(t, "bd-1.2.2", "history-memories", "vc-status.json")},
	}})
	ctx := context.Background()
	h, err := c.History(ctx, "x-1")
	if err != nil || len(h) < 2 {
		t.Fatalf("history = %d entries, %v", len(h), err)
	}
	m, err := c.Memories(ctx)
	if err != nil || len(m) != 2 || m[0].Key >= m[1].Key {
		t.Fatalf("memories = %+v, %v", m, err)
	}
	vc, err := c.VCStatus(ctx)
	if err != nil || vc.Branch != "main" {
		t.Fatalf("vc = %+v, %v", vc, err)
	}
}

func TestFixtureCustomStatusCategories(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		t.Run(v, func(t *testing.T) {
			m1 := func(name string) []byte { return fixture(t, "bd-"+v, "m1", name) }
			c := NewExec(ExecOptions{Runner: replay{
				"statuses": {Stdout: m1("statuses-custom.json")},
				"list":     {Stdout: m1("list-custom-status.json")},
				"ready":    {Stdout: m1("ready-explain-custom.json")},
			}})
			ctx := context.Background()
			st, err := c.Statuses(ctx)
			if err != nil {
				t.Fatal(err)
			}
			issues, err := c.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			r, err := c.Ready(ctx)
			if err != nil {
				t.Fatal(err)
			}
			snap := model.NewSnapshot(issues, r, testNow())
			got := map[string]model.PresentationStatus{}
			for _, id := range snap.IDs() {
				is, _ := snap.Issue(id)
				got[is.Status] = snap.Present(id, st).Status
			}
			if got["plain"] != model.Other {
				t.Errorf("custom status without category = %v, want Other", got["plain"])
			}
			if got["review"] != model.InProgress {
				t.Errorf("custom wip status = %v, want In progress", got["review"])
			}
			if in, _ := st.Lookup("plain"); in.Category != model.CategoryUnspecified || !in.Custom {
				t.Errorf("plain = %+v", in)
			}
			if st.Category("cold") != model.CategoryFrozen {
				t.Error("cold must be frozen")
			}
		})
	}
}

func TestFixtureConfigGet(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		t.Run(v, func(t *testing.T) {
			off := "false"
			if v == "1.2.2" {
				off = ""
			}
			for name, want := range map[string]string{
				"config-get-journal-off": off, "config-get-journal-on": "true", "config-get-unset": "",
			} {
				c := NewExec(ExecOptions{Runner: replay{"config get": {Stdout: fixture(t, "bd-"+v, "m1", name+".json")}}})
				got, err := c.ConfigGet(context.Background(), "events-journal")
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if got.Value != want {
					t.Errorf("%s value = %q, want %q", name, got.Value, want)
				}
			}
		})
	}
}

func rcOf(t *testing.T, rel ...string) int {
	t.Helper()
	n, err := strconv.Atoi(strings.TrimSpace(string(fixture(t, rel...))))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestFixtureErrorClasses replays every error fixture and checks the class
// decided from exit code, JSON error key and command.
func TestFixtureErrorClasses(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		t.Run(v, func(t *testing.T) {
			dir := "bd-" + v
			ctx := context.Background()
			result := func(name string) Result {
				return Result{
					Stdout:   fixture(t, dir, "m1", name+".json"),
					Stderr:   fixture(t, dir, "m1", name+".stderr"),
					ExitCode: rcOf(t, dir, "m1", name+".rc"),
				}
			}

			c := NewExec(ExecOptions{Runner: replay{"where": result("notws-where")}})
			_, err := c.Where(ctx)
			if !IsClass(err, ClassNotWorkspace) || CodeOf(err) != CodeNoBeadsDirectory {
				t.Errorf("where outside a workspace: %v", err)
			}

			c = NewExec(ExecOptions{Runner: replay{
				"list": result("notws-list"), "ready": result("notws-ready"), "where": result("notws-where"),
			}})
			for name, call := range map[string]func() error{
				"list":  func() error { _, err := c.List(ctx); return err },
				"ready": func() error { _, err := c.Ready(ctx); return err },
			} {
				err := call()
				if CodeOf(err) != "" {
					t.Errorf("%s carries code %q before classification", name, CodeOf(err))
				}
				err = ClassifyReadFailure(ctx, c, err)
				if !IsClass(err, ClassTransient) || CodeOf(err) != CodeNoBeadsDirectory {
					t.Errorf("%s outside a workspace = %v, want transient with %s", name, err, CodeNoBeadsDirectory)
				}
			}

			c = NewExec(ExecOptions{Runner: replay{"comments": result("comments-missing-issue")}})
			if _, err := c.Comments(ctx, "m1-zzz"); !IsClass(err, ClassTransient) {
				t.Errorf("comments of a missing issue = %v, want transient", err)
			}
			c = NewExec(ExecOptions{Runner: replay{"history": result("history-missing-issue")}})
			if h, err := c.History(ctx, "m1-zzz"); err != nil || len(h) != 0 {
				t.Errorf("history of a missing issue = %v, %v; bd answers rc 0 with null", h, err)
			}

			if _, ok := fixtureOptional(dir, "m1", "skew-list.rc"); ok {
				c = NewExec(ExecOptions{Runner: replay{"list": result("skew-list")}})
				_, err := c.List(ctx)
				if !IsClass(err, ClassUnsupported) {
					t.Errorf("schema skew = %v, want unsupported", err)
				}
			}
		})
	}
}

func TestFixtureLegacyErrorFiles(t *testing.T) {
	for _, v := range fixtureVersions(t) {
		t.Run(v, func(t *testing.T) {
			dir := "bd-" + v
			where, ok := fixtureOptional(dir, "recipe-run", "errors", "where-not-workspace.stdout.json")
			if !ok {
				t.Skip("no legacy error fixtures")
			}
			c := NewExec(ExecOptions{Runner: replay{
				"where": {Stdout: where, ExitCode: 1},
				"list":  {Stderr: fixture(t, dir, "recipe-run", "errors", "list-not-workspace.stderr.txt"), ExitCode: 1},
			}})
			if _, err := c.Where(context.Background()); !IsClass(err, ClassNotWorkspace) {
				t.Errorf("legacy where error = %v", err)
			}
			_, err := c.List(context.Background())
			err = ClassifyReadFailure(context.Background(), c, err)
			if !IsClass(err, ClassTransient) || CodeOf(err) != CodeNoBeadsDirectory {
				t.Errorf("legacy list error = %v", err)
			}
		})
	}
}

func TestContractCorpus(t *testing.T) {
	var ran int
	for _, v := range fixtureVersions(t) {
		root := filepath.Join(testdataDir, "bd-"+v, "contract-corpus")
		if _, err := os.Stat(root); err != nil {
			continue
		}
		ran++
		t.Run(v, func(t *testing.T) {
			for _, mode := range []string{"envelope", "flat"} {
				list := fixture(t, "bd-"+v, "contract-corpus", mode, "list.json")
				data, err := unwrap("list", list)
				if err != nil {
					t.Fatalf("%s list: %v", mode, err)
				}
				issues, err := parseIssues(data)
				if err != nil || len(issues) != 3 {
					t.Fatalf("%s list = %d issues, %v", mode, len(issues), err)
				}
				ids := []string{}
				for _, is := range issues {
					ids = append(ids, is.ID)
					if is.Title == "" || is.Status == "" {
						t.Errorf("%s: %+v", mode, is)
					}
				}
				if !reflect.DeepEqual(ids, []string{"corpus-closed", "corpus-dep", "corpus-root"}) {
					t.Errorf("%s ids = %v", mode, ids)
				}
				root := issues[2]
				if len(root.Labels) != 1 || root.Priority != 0 {
					t.Errorf("%s corpus-root = %+v", mode, root)
				}
			}
			for _, name := range []string{"ready", "show", "dep_list", "count", "create_root", "update"} {
				for _, mode := range []string{"envelope", "flat"} {
					if _, err := unwrap(name, fixture(t, "bd-"+v, "contract-corpus", mode, name+".json")); err != nil {
						t.Errorf("%s/%s: %v", mode, name, err)
					}
				}
			}
			for _, mode := range []string{"envelope", "flat"} {
				ready, err := unwrap("ready", fixture(t, "bd-"+v, "contract-corpus", mode, "ready.json"))
				if err != nil {
					t.Fatalf("%s ready: %v", mode, err)
				}
				if _, err := parseReadiness(ready); err == nil {
					t.Errorf("%s ready.json is the plain ready shape without --explain; parseReadiness must refuse it", mode)
				}

				body := fixture(t, "bd-"+v, "contract-corpus", mode, "error.json")
				f, found := parseFailure(body)
				if !found || f.Code == "" {
					t.Fatalf("%s error blob = %+v, %v", mode, f, found)
				}
				res := Result{Stdout: body, ExitCode: 1}
				c := NewExec(ExecOptions{Runner: replay{"comments": res}})
				_, err = c.Comments(context.Background(), "corpus-zzz")
				if !IsClass(err, ClassTransient) || CodeOf(err) != f.Code {
					t.Errorf("%s read failure = %v, want transient with code %q", mode, err, f.Code)
				}
				if err := c.exitError(kindWrite, "close", res); !IsClass(err, ClassRejected) || CodeOf(err) != f.Code {
					t.Errorf("%s write failure = %v, want rejected with code %q", mode, err, f.Code)
				}
			}
		})
	}
	if ran == 0 {
		t.Error("no contract corpus vendored")
	}
}
