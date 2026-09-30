package export

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/testgolden"
)

var (
	zone = time.FixedZone("", 2*3600)
	t0   = time.Date(2026, 9, 29, 7, 41, 0, 0, time.UTC)
)

func raw(is model.Issue, extra string) json.RawMessage {
	m := map[string]any{"id": is.ID, "title": is.Title, "status": is.Status, "priority": is.Priority, "issue_type": is.IssueType, "comment_count": is.CommentCount}
	b, _ := json.Marshal(m)
	if extra != "" {
		b = append(b[:len(b)-1], []byte(","+extra+"}")...)
	}
	return b
}

func fixture() (*model.Snapshot, model.Statuses) {
	issues := []model.Issue{
		{ID: "r1-w24", Title: "Epic A", Status: "open", IssueType: "epic", Priority: 1, Description: "The epic body.\r\n\r\nSecond paragraph.\n\n", CreatedAt: t0, UpdatedAt: t0},
		{
			ID: "r1-w24.1", Title: "Child task A1", Status: "in_progress", IssueType: "task", Priority: 0, Assignee: "alice",
			Labels: []string{"area:core", "ui"}, Parent: "r1-w24", CommentCount: 2,
			Description: "Do the thing.", Design: "Use a table.", AcceptanceCriteria: "- it works", Notes: "Watch the | pipe.",
			CreatedAt: t0, UpdatedAt: t0.Add(time.Minute), StartedAt: t0.Add(time.Minute),
			Dependencies: []model.Edge{{From: "r1-w24.1", To: "r1-w24", Type: "parent-child"}, {From: "r1-w24.1", To: "external:jira-1", Type: "blocks"}},
		},
		{ID: "r1-w24.1.1", Title: "Grandchild of A1", Status: "open", IssueType: "task", Priority: 2, Parent: "r1-w24.1", CreatedAt: t0, UpdatedAt: t0},
		{
			ID: "r1-w24.3", Title: "Feature | blocked by A1", Status: "open", IssueType: "feature", Priority: 2, CreatedAt: t0, UpdatedAt: t0,
			Dependencies: []model.Edge{{From: "r1-w24.3", To: "r1-w24.1", Type: "blocks"}, {From: "r1-w24.3", To: "r1-w24", Type: "related"}},
		},
		{ID: "r1-w24.4", Title: "Parked", Status: "deferred", IssueType: "chore", Priority: 3, CreatedAt: t0, UpdatedAt: t0},
		{
			ID: "r1-w24.5", Title: "Done", Status: "closed", IssueType: "bug", Priority: 4, CreatedAt: t0, UpdatedAt: t0,
			ClosedAt: t0.Add(2 * time.Hour), CloseReason: "fixed upstream",
		},
	}
	for i := range issues {
		extra := ""
		if issues[i].ID == "r1-w24.1" {
			extra = `"future_field":{"kept":true}`
		}
		issues[i].Raw = raw(issues[i], extra)
	}
	ready := model.Readiness{Ready: []string{"r1-w24.1"}, Blocked: map[string][]string{"r1-w24.3": {"r1-w24.1"}}}
	return model.NewSnapshot(issues, ready, t0), model.BuiltinStatuses()
}

func comments() map[string][]bd.Comment {
	return map[string][]bd.Comment{"r1-w24.1": {
		{ID: "c2", IssueID: "r1-w24.1", Author: "bob", Text: "Second **comment**\r\n", CreatedAt: t0.Add(2 * time.Minute)},
		{ID: "c1", IssueID: "r1-w24.1", Author: "janlink", Text: "First comment by default actor", CreatedAt: t0.Add(time.Minute)},
	}}
}

func input(ids []string, set Set, withComments bool) Input {
	snap, st := fixture()
	return Input{
		Snap: snap, Statuses: st, IDs: ids, Set: set, WithComments: withComments, Comments: comments(),
		Prefix: "r1", Query: "status:open", Now: t0.Add(24 * time.Hour), Loc: zone, Version: "v0.1.0",
	}
}

var allIDs = []string{"r1-w24", "r1-w24.1", "r1-w24.1.1", "r1-w24.3", "r1-w24.4", "r1-w24.5"}

func TestGoldens(t *testing.T) {
	cases := []struct {
		name string
		in   Input
	}{
		{"single", input([]string{"r1-w24.1"}, Current, false)},
		{"single_comments", input([]string{"r1-w24.1"}, Current, true)},
		{"set", input(allIDs, Scope, false)},
		{"set_comments", input(allIDs[:3], Marked, true)},
	}
	for _, c := range cases {
		for _, f := range Formats {
			t.Run(c.name+"/"+strings.ToLower(f.String()), func(t *testing.T) {
				out, err := Render(f, c.in)
				if err != nil {
					t.Fatal(err)
				}
				testgolden.Equal(t, string(out))
			})
		}
	}
}

func TestOverCap(t *testing.T) {
	snap, st := fixture()
	big := *mustIssue(t, snap, "r1-w24")
	big.Description = strings.Repeat("0123456789abcdef\n", 60000)
	snap2 := model.NewSnapshot([]model.Issue{big}, model.Readiness{}, t0)
	out, err := Render(Markdown, Input{Snap: snap2, Statuses: st, IDs: []string{"r1-w24"}, Loc: zone})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) <= clipboard.MaxBytes {
		t.Fatalf("fixture is only %d bytes", len(out))
	}
	testgolden.Equal(t, OverCap(len(out))+"\n"+OverCap(clipboard.MaxBytes)+"|"+OverCap(clipboard.MaxBytes+1)+"\n")
}

func mustIssue(t *testing.T, s *model.Snapshot, id string) *model.Issue {
	t.Helper()
	is, ok := s.Issue(id)
	if !ok {
		t.Fatalf("no issue %s", id)
	}
	return is
}

func TestJSONWithoutRawIsAnError(t *testing.T) {
	snap := model.NewSnapshot([]model.Issue{{ID: "r1-x", Title: "No raw", Status: "open", CreatedAt: t0, UpdatedAt: t0}}, model.Readiness{}, t0)
	_, err := Render(JSON, Input{Snap: snap, Statuses: model.BuiltinStatuses(), IDs: []string{"r1-x"}, Set: Marked, Loc: zone, Now: t0})
	if err == nil || !strings.Contains(err.Error(), "r1-x") {
		t.Errorf("err = %v", err)
	}
}

func TestJSONIsBdsRawArrayWithCommentsKey(t *testing.T) {
	out, err := Render(JSON, input([]string{"r1-w24.1", "r1-w24"}, Marked, true))
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]json.RawMessage
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || string(got[0]["id"]) != `"r1-w24.1"` || string(got[1]["id"]) != `"r1-w24"` {
		t.Fatalf("order or ids wrong: %s", out)
	}
	var kept bytes.Buffer
	_ = json.Compact(&kept, got[0]["future_field"])
	if kept.String() != `{"kept":true}` {
		t.Errorf("unknown field lost: %s", got[0]["future_field"])
	}
	if _, ok := got[1]["comments"]; ok {
		t.Error("an issue without comments got a comments key")
	}
	var cs []wireComment
	if err := json.Unmarshal(got[0]["comments"], &cs); err != nil || len(cs) != 2 || cs[0].Author != "janlink" {
		t.Errorf("comments = %s (%v)", got[0]["comments"], err)
	}
	for _, derived := range []string{"presentation", "ready", "blocked", "progress"} {
		if _, ok := got[0][derived]; ok {
			t.Errorf("derived field %q in JSON", derived)
		}
	}
	if !strings.HasSuffix(string(out), "]\n") || strings.Contains(string(out), "\r") {
		t.Errorf("framing: %q", out[len(out)-4:])
	}
}

func TestSingleJSONIsStillAnArray(t *testing.T) {
	out, err := Render(JSON, input([]string{"r1-w24"}, Current, false))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "[\n  {") {
		t.Errorf("not an array: %s", out)
	}
}

func TestFormatsEndWithOneNewline(t *testing.T) {
	for _, f := range Formats {
		out, err := Render(f, input(allIDs, Scope, true))
		if err != nil {
			t.Fatal(err)
		}
		s := string(out)
		if !strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\n\n") || strings.Contains(s, "\r") || strings.HasPrefix(s, "\ufeff") {
			t.Errorf("%s framing wrong", f)
		}
	}
}

func TestVanishedIDsAreSkipped(t *testing.T) {
	out, err := Render(Markdown, input([]string{"r1-w24", "gone"}, Marked, false))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "gone") || strings.Contains(string(out), "2 issues") {
		t.Errorf("vanished issue listed:\n%s", out)
	}
}

func TestName(t *testing.T) {
	now := time.Date(2026, 9, 29, 14, 12, 0, 0, time.UTC)
	for _, c := range []struct {
		f    Format
		set  Set
		ids  []string
		want string
	}{
		{Markdown, Current, []string{"r1-w24.1"}, "r1-w24.1.md"},
		{JSON, Marked, []string{"r1-a"}, "r1-a.json"},
		{Markdown, Scope, []string{"a", "b"}, "r1-scope-20260929-1412.md"},
		{Text, Marked, []string{"a", "b"}, "r1-marked-20260929-1412.txt"},
	} {
		if got := Name(c.f, c.set, c.ids, "r1", now); got != c.want {
			t.Errorf("Name = %q, want %q", got, c.want)
		}
	}
}

func TestSize(t *testing.T) {
	for n, want := range map[int]string{0: "0 B", 1023: "1023 B", 1024: "1 KB", 1229: "1.2 KB", 4096: "4 KB", 38 * 1024: "38 KB", 1300000: "1.2 MB"} {
		if got := Size(n); got != want {
			t.Errorf("Size(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestResolvePath(t *testing.T) {
	vol := ""
	if runtime.GOOS == "windows" {
		vol = "C:"
	}
	at := func(p string) string { return vol + filepath.FromSlash(p) }
	for _, c := range []struct{ p, want string }{
		{"a.md", at("/work/a.md")},
		{"~/x/a.md", at("/home/u/x/a.md")},
		{"/abs/a.md", at("/abs/a.md")},
		{"sub/../a.md", at("/work/a.md")},
	} {
		if got := ResolvePath(c.p, at("/work"), at("/home/u")); got != c.want {
			t.Errorf("ResolvePath(%q) = %q, want %q", c.p, got, c.want)
		}
	}
}

func TestResolvePathWindowsForms(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("volume and backslash forms only exist on Windows")
	}
	for _, c := range []struct{ p, want string }{
		{`D:\x\a.md`, `D:\x\a.md`},
		{`D:/x/a.md`, `D:\x\a.md`},
		{`\\srv\share\a.md`, `\\srv\share\a.md`},
		{`\abs\a.md`, `C:\abs\a.md`},
		{`~\x\a.md`, `C:\home\u\x\a.md`},
		{`sub\..\a.md`, `C:\work\a.md`},
	} {
		if got := ResolvePath(c.p, `C:\work`, `C:\home\u`); got != c.want {
			t.Errorf("ResolvePath(%q) = %q, want %q", c.p, got, c.want)
		}
	}
}

func TestWriteFileIsAtomicAndKeepsNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.md")
	if err := WriteFile(path, []byte("one\n"), false); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("two\n"), true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "two\n" {
		t.Errorf("content %q", b)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("left %d entries, want only the export", len(entries))
	}
	if err := WriteFile(filepath.Join(dir, "missing", "x.md"), nil, false); err == nil {
		t.Error("a missing directory was created or ignored")
	}
}

func TestWriteFileWithoutOverwriteNeverReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.md")
	if err := os.WriteFile(path, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("theirs"), false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("err = %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "mine" {
		t.Errorf("content %q", b)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("left %d entries", len(entries))
	}
}

func TestWriteFileOverwriteKeepsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.md")
	if err := os.WriteFile(path, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("new"), true); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if runtime.GOOS == "windows" {
		if st.Mode().Perm()&0o200 == 0 {
			t.Errorf("mode %v, want writable", st.Mode().Perm())
		}
		return
	}
	if st.Mode().Perm() != 0o640 {
		t.Errorf("mode %v, want 0640", st.Mode().Perm())
	}
}

func TestWriteFileOnADirectoryFails(t *testing.T) {
	dir := t.TempDir()
	for _, overwrite := range []bool{false, true} {
		err := WriteFile(dir, []byte("x"), overwrite)
		if err == nil || !strings.Contains(err.Error(), "is a directory") {
			t.Errorf("overwrite=%v: err = %v", overwrite, err)
		}
	}
}

func TestFreeName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "r1-w24.md")
	for _, name := range []string{"r1-w24.md", "r1-w24-1.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := FreeName(path)
	if err != nil || filepath.Base(got) != "r1-w24-2.md" {
		t.Errorf("FreeName = %q, %v", got, err)
	}
}

func TestParseFormat(t *testing.T) {
	for in, want := range map[string]Format{"md": Markdown, "Markdown": Markdown, "json": JSON, "text": Text, "txt": Text} {
		if got, ok := ParseFormat(in); !ok || got != want {
			t.Errorf("ParseFormat(%q) = %v, %v", in, got, ok)
		}
	}
	if _, ok := ParseFormat("yaml"); ok {
		t.Error("yaml accepted")
	}
}
