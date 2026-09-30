package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/export"
	"github.com/janlink/beads-dash/internal/testgolden"
)

func markSome(r *writeRig, ids ...string) {
	for _, id := range ids {
		r.a.sess.SetCurrent(id)
		r.key("space")
	}
}

func (r *writeRig) export() *exportDialog {
	r.t.Helper()
	d, ok := r.a.topDialog().(*exportDialog)
	if !ok {
		r.t.Fatalf("no export dialog open:\n%s", screen(r.a))
	}
	return d
}

// exportBusy reports whether the open export dialog waits for a comment read
// or a build.
func (r *writeRig) exportBusy() bool {
	d, ok := r.a.topDialog().(*exportDialog)
	return ok && (d.building || d.fetchCancel != nil || d.busy)
}

func (r *writeRig) waitReady() {
	r.t.Helper()
	d := r.export()
	for i := 0; i < 20 && !d.ready(); i++ {
		r.run(d.flush())
		time.Sleep(5 * time.Millisecond)
	}
	if !d.ready() {
		r.t.Fatalf("export never became ready:\n%s", screen(r.a))
	}
}

func withRaw(r *writeRig) {
	for _, id := range r.a.snap.IDs() {
		is, _ := r.a.snap.Issue(id)
		is.Raw, _ = json.Marshal(map[string]any{"id": is.ID, "title": is.Title, "status": is.Status})
	}
}

func withComments(r *writeRig, id string, n int) {
	is, _ := r.a.snap.Issue(id)
	is.CommentCount = n
	var cs []bd.Comment
	for i := range n {
		cs = append(cs, bd.Comment{ID: fmt.Sprint(i + 1), IssueID: id, Author: "ana", Text: fmt.Sprintf("remark %d", i+1), CreatedAt: time.Date(2026, 9, 28, 10, i, 0, 0, time.UTC)})
	}
	r.fake.SetComments(id, cs...)
}

func TestExportDialogGoldens(t *testing.T) {
	scenes := []struct {
		name string
		set  func(*writeRig)
	}{
		{"marked_clipboard", func(r *writeRig) {
			markSome(r, "ws-9qe", "ws-2hz")
			r.key("x")
			r.waitReady()
		}},
		{"current_with_comments", func(r *writeRig) {
			withComments(r, "ws-9qe", 2)
			r.a.sess.SetCurrent("ws-9qe")
			r.key("x")
			r.waitReady()
		}},
		{"file_json", func(r *writeRig) {
			withRaw(r)
			markSome(r, "ws-9qe", "ws-2hz", "ws-7mt")
			r.key("x")
			r.key("down", "right")
			r.key("down", "down", "right")
			r.waitReady()
		}},
		{"over_cap", func(r *writeRig) {
			is, _ := r.a.snap.Issue("ws-9qe")
			is.Description = strings.Repeat("a long line of the description\n", 30000)
			r.a.sess.SetCurrent("ws-9qe")
			r.key("x")
			r.waitReady()
		}},
	}
	for _, s := range scenes {
		for _, sz := range formSizes {
			t.Run(fmt.Sprintf("%s/%dx%d", s.name, sz[0], sz[1]), func(t *testing.T) {
				r := newRig(t, sz[0], sz[1])
				s.set(r)
				testgolden.Equal(t, screen(r.a))
			})
		}
	}
}

func exportBoardRig(t *testing.T) (*writeRig, *fakeBoard) {
	t.Helper()
	board := &fakeBoard{res: clipboard.Result{Copied: []string{"wl-copy"}}}
	r := newRig(t, 100, 30)
	r.a.o.Clipboard = board
	return r, board
}

func TestExportDefaultsToMarkedElseCurrent(t *testing.T) {
	r, _ := exportBoardRig(t)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("x")
	d := r.export()
	if d.set() != 0 || len(d.ids()) != 1 {
		t.Errorf("set %v ids %v, want the current issue", d.set(), d.ids())
	}
	if !d.withComments() {
		t.Error("comments are off for a single issue")
	}
	r.key("esc")
	markSome(r, "ws-9qe", "ws-2hz")
	r.key("x")
	d = r.export()
	if d.set() != 1 || len(d.ids()) != 2 || d.withComments() {
		t.Errorf("set %v ids %v comments %v, want the marked pair without comments", d.set(), d.ids(), d.withComments())
	}
}

func TestExportToClipboardCopiesAndCloses(t *testing.T) {
	r, board := exportBoardRig(t)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("x")
	r.waitReady()
	r.key("ctrl+s")
	if len(r.a.dialogs) != 0 {
		t.Fatalf("dialog still open:\n%s", screen(r.a))
	}
	got := board.got()
	if len(got) != 1 || !strings.HasPrefix(got[0], "# ws-9qe") && !strings.Contains(got[0], "ws-9qe") {
		t.Fatalf("board got %q", got)
	}
	if n := lastNotice(r.a); !strings.Contains(n, "ws-9qe as Markdown") {
		t.Errorf("notice = %q", n)
	}
}

func TestExportToFileWritesAndAsksBeforeReplacing(t *testing.T) {
	r, _ := exportBoardRig(t)
	dir := t.TempDir()
	r.a.o.StartDir = dir
	r.a.sess.SetCurrent("ws-9qe")
	r.key("x")
	d := r.export()
	d.f.FocusKey(fExpTarget)
	r.key("right")
	r.waitReady()
	path := filepath.Join(dir, "ws-9qe.md")
	if got := d.f.Field(fExpPath).Value(); got != "ws-9qe.md" {
		t.Fatalf("default path %q", got)
	}
	r.key("ctrl+s")
	first, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(first), "ws-9qe") {
		t.Fatalf("file: %v %q", err, first)
	}
	if !strings.Contains(lastNotice(r.a), "Saved ws-9qe as Markdown") || len(r.a.dialogs) != 0 {
		t.Errorf("notice %q dialogs %d", lastNotice(r.a), len(r.a.dialogs))
	}

	if err := os.WriteFile(path, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	r.key("x")
	r.export().f.FocusKey(fExpTarget)
	r.key("right")
	r.waitReady()
	r.key("ctrl+s")
	if _, ok := r.a.topDialog().(*exportOverwriteDialog); !ok {
		t.Fatalf("no overwrite prompt:\n%s", screen(r.a))
	}
	if b, _ := os.ReadFile(path); string(b) != "mine" {
		t.Fatalf("file replaced before the answer: %q", b)
	}
	r.key("b")
	if b, _ := os.ReadFile(filepath.Join(dir, "ws-9qe-1.md")); len(b) == 0 {
		t.Error("keep both wrote nothing")
	}
	if b, _ := os.ReadFile(path); string(b) != "mine" {
		t.Errorf("keep both replaced the file: %q", b)
	}

	r.key("x")
	r.export().f.FocusKey(fExpTarget)
	r.key("right")
	r.waitReady()
	r.key("ctrl+s", "o")
	if b, _ := os.ReadFile(path); string(b) == "mine" {
		t.Error("overwrite kept the old file")
	}
}

func TestExportFetchesCommentsOncePerChange(t *testing.T) {
	r, board := exportBoardRig(t)
	withComments(r, "ws-9qe", 2)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("x")
	r.waitReady()
	d := r.export()
	if !strings.Contains(string(d.built), "remark 2") {
		t.Fatalf("comments missing:\n%s", d.built)
	}
	if _, ok := r.a.freshComments("ws-9qe"); !ok {
		t.Error("thread not cached")
	}
	d.f.FocusKey(fExpComments)
	r.key("space")
	r.waitReady()
	if strings.Contains(string(d.built), "remark") {
		t.Error("comments still in the export after unticking")
	}
	r.key("space")
	r.waitReady()
	if !strings.Contains(string(d.built), "remark 1") {
		t.Error("comments lost after ticking again")
	}
	r.key("ctrl+s")
	if got := board.got(); len(got) != 1 || !strings.Contains(got[0], "remark 2") {
		t.Errorf("board got %q", got)
	}
}

func TestExportCommentFailureOffersExportWithout(t *testing.T) {
	r, board := exportBoardRig(t)
	withComments(r, "ws-9qe", 1)
	r.fake.FailWith("Comments", &bd.Error{Message: "bd is busy"})
	r.a.sess.SetCurrent("ws-9qe")
	r.key("x")
	d := r.export()
	r.run(d.flush())
	if d.fetchErr == "" || !strings.Contains(screen(r.a), "export without?") {
		t.Fatalf("no failure shown:\n%s", screen(r.a))
	}
	r.key("ctrl+s")
	r.waitReady()
	r.key("ctrl+s")
	if got := board.got(); len(got) != 1 || strings.Contains(got[0], "remark") {
		t.Errorf("board got %q", got)
	}
}

func TestExportManyCommentReadsWaitForSubmit(t *testing.T) {
	r, board := exportBoardRig(t)
	defer func(n int) { commentAutoFetch = n }(commentAutoFetch)
	commentAutoFetch = 3
	for _, id := range r.a.snap.IDs() {
		withComments(r, id, 1)
	}
	r.key("x")
	d := r.export()
	d.f.FocusKey(fExpSet)
	for d.set() != 2 {
		r.key("right")
	}
	d.f.FocusKey(fExpComments)
	d.f.Field(fExpComments).Set("on")
	d.commentsTouched, d.stale = true, true
	r.run(d.flush())
	if len(d.missing) <= commentAutoFetch {
		t.Fatalf("only %d comment threads to read", len(d.missing))
	}
	if d.fetched != 0 || !strings.Contains(d.status(), "comment reads on export") {
		t.Fatalf("fetch started on its own: %q", d.status())
	}
	r.key("ctrl+s")
	if len(board.got()) != 1 {
		t.Errorf("board got %d texts", len(board.got()))
	}
}

func TestExportCommand(t *testing.T) {
	r, board := exportBoardRig(t)
	dir := t.TempDir()
	r.a.o.StartDir = dir
	withRaw(r)
	markSome(r, "ws-9qe", "ws-2hz")
	r.line("export json")
	got := board.got()
	if len(got) != 1 || !strings.HasPrefix(got[0], "[") {
		t.Fatalf("board got %q", got)
	}
	r.line("export md out.md")
	b, err := os.ReadFile(filepath.Join(dir, "out.md"))
	if err != nil || !strings.Contains(string(b), "ws-2hz") {
		t.Fatalf("file: %v %q", err, b)
	}
	if !strings.Contains(lastNotice(r.a), "2 issues as Markdown") {
		t.Errorf("notice %q", lastNotice(r.a))
	}
	r.line("export md out.md")
	if _, ok := r.a.topDialog().(*exportOverwriteDialog); !ok {
		t.Errorf("no overwrite prompt for :export onto an existing file")
	}
}

func TestExportCommandRejectsUnknownFormat(t *testing.T) {
	r, _ := exportBoardRig(t)
	r.a.sess.SetCurrent("ws-9qe")
	r.line("export pdf")
	if n := lastNotice(r.a); n != "" && strings.Contains(n, "Saved") {
		t.Errorf("notice %q", n)
	}
	if len(r.a.dialogs) != 0 {
		t.Error("dialog opened")
	}
}

func TestExportOverTheCapForcesAFile(t *testing.T) {
	r, board := exportBoardRig(t)
	is, _ := r.a.snap.Issue("ws-9qe")
	is.Description = strings.Repeat("a long line of the description\n", 30000)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("x")
	r.waitReady()
	d := r.export()
	if !d.toFile() || !strings.Contains(screen(r.a), "over the 700 KB clipboard limit") {
		t.Fatalf("clipboard still offered:\n%s", screen(r.a))
	}
	d.f.FocusKey(fExpTarget)
	r.key("left")
	if !d.toFile() || len(board.got()) != 0 {
		t.Error("the clipboard target came back")
	}
}

func TestExportStaleSnapshotNote(t *testing.T) {
	r, _ := exportBoardRig(t)
	r.a.sess.SetCurrent("ws-9qe")
	r.a.status.Stale = true
	r.a.status.LastSuccess = r.a.now().Add(-5 * time.Minute)
	r.key("x")
	if !strings.Contains(screen(r.a), "from snapshot 5m old") {
		t.Errorf("no stale note:\n%s", screen(r.a))
	}
}

func TestExportWithNothingToExportSaysSo(t *testing.T) {
	r, _ := exportBoardRig(t)
	r.a.sess.SetCurrent("")
	r.key("x")
	if len(r.a.dialogs) != 0 && len(r.export().ids()) == 0 {
		t.Error("an empty export dialog opened")
	}
}

func TestExportEmptySetBlocksSubmit(t *testing.T) {
	r, board := exportBoardRig(t)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("x")
	r.waitReady()
	d := r.export()
	r.key("right")
	if d.set() != export.Marked || len(d.ids()) != 0 {
		t.Fatalf("set %v ids %v", d.set(), d.ids())
	}
	r.key("ctrl+s")
	if len(board.got()) != 0 || !r.a.hasDialog(d) {
		t.Fatalf("an empty set was exported:\n%s", screen(r.a))
	}
	if !strings.Contains(screen(r.a), "No issues in this set") {
		t.Errorf("no reason shown:\n%s", screen(r.a))
	}
}

func TestExportCapFlipDropsThePendingSubmit(t *testing.T) {
	r, board := exportBoardRig(t)
	is, _ := r.a.snap.Issue("ws-9qe")
	is.Description = strings.Repeat("a long line of the description\n", 30000)
	r.a.sess.SetCurrent("ws-9qe")
	r.key("x")
	r.waitReady()
	d := r.export()
	d.f.Field(fExpTarget).Set(targetClipboard)
	d.submitWanted, d.busy, d.stale = true, true, true
	r.run(d.flush())
	r.waitReady()
	if d.submitWanted || d.busy || len(board.got()) != 0 {
		t.Errorf("pending submit survived: wanted %v busy %v board %v", d.submitWanted, d.busy, board.got())
	}
	if !strings.Contains(r.a.hint, "too large") {
		t.Errorf("hint = %q", r.a.hint)
	}
}

func TestExportInMemoriesViewIsDisabledWithAHint(t *testing.T) {
	r, _ := exportBoardRig(t)
	r.key("5")
	r.key("x")
	if len(r.a.dialogs) != 0 || r.a.hint != issueOnlyHint {
		t.Errorf("dialogs %d hint %q", len(r.a.dialogs), r.a.hint)
	}
}

func TestExportOrderIsTheUnfoldedOutline(t *testing.T) {
	r, _ := exportBoardRig(t)
	r.key("2")
	r.key("z", "M")
	folded := len(r.a.view().Visible(r.a.env()))
	order := r.a.outlineOrder()
	if len(order) <= folded {
		t.Errorf("outline has %d rows, folded tree %d", len(order), folded)
	}
	r.key("1")
	if got := r.a.exportOrder(); !slices.Equal(got, order) {
		t.Errorf("overview order %v, want %v", got, order)
	}
}
