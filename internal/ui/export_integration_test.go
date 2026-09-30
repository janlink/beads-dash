package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/testbd"
	"github.com/janlink/beads-dash/internal/testgolden"
)

var (
	exportTimes  = regexp.MustCompile(`\d{4}-\d\d-\d\d[T ]\d\d:\d\d(:\d\d(\.\d+)?)?( ?[+-]\d\d:\d\d|Z)?`)
	exportUUIDs  = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	exportAuthor = regexp.MustCompile(`(?m)^\*\*[^*]+\*\* ·`)
	exportUsers  = regexp.MustCompile(`("(?:author|created_by|owner|updated_by)": )"[^"]*"`)
)

func normalizeExport(s string) string {
	s = exportTimes.ReplaceAllString(s, "<time>")
	s = exportUUIDs.ReplaceAllString(s, "<uuid>")
	s = exportAuthor.ReplaceAllString(s, "**<user>** ·")
	return exportUsers.ReplaceAllString(s, `$1"<user>"`)
}

func seedExportIssues(t *testing.T, w testbd.Workspace) {
	t.Helper()
	steps := [][]string{
		{"create", "--silent", "--id=t-a1", "--title=Export epic", "--type=epic", "--priority=1", "--description=Everything the export shows."},
		{"create", "--silent", "--id=t-a2", "--title=Child with | pipe", "--type=bug", "--priority=0", "--labels=cli,ui", "--assignee=ana", "--description=First line.\n\nSecond paragraph."},
		{"dep", "add", "t-a2", "t-a1", "--type=parent-child"},
		{"create", "--silent", "--id=t-a3", "--title=Waits for the child", "--type=task", "--priority=2"},
		{"dep", "add", "t-a3", "t-a2"},
		{"comments", "add", "t-a2", "First remark"},
		{"comments", "add", "t-a2", "Second remark"},
	}
	for _, s := range steps {
		if _, err := w.Bd(s...); err != nil {
			t.Fatalf("bd %s: %v", strings.Join(s, " "), err)
		}
	}
}

func TestIntegrationExportSeededWorkspace(t *testing.T) {
	testbd.EachVersion(t, func(t *testing.T, w testbd.Workspace) {
		seedExportIssues(t, w)
		scenes := []struct {
			name string
			run  func(r *writeRig) *exportDialog
		}{
			{"current_markdown_comments", func(r *writeRig) *exportDialog {
				r.a.sess.SetCurrent("t-a2")
				r.key("x")
				return r.export()
			}},
			{"marked_json_comments", func(r *writeRig) *exportDialog {
				markSome(r, "t-a1", "t-a2", "t-a3")
				r.key("x")
				d := r.export()
				d.f.FocusKey(fExpFormat)
				r.key("right")
				d.f.FocusKey(fExpComments)
				r.key("space")
				return d
			}},
			{"marked_text", func(r *writeRig) *exportDialog {
				markSome(r, "t-a1", "t-a3")
				r.key("x")
				d := r.export()
				d.f.FocusKey(fExpFormat)
				r.key("right", "right")
				return d
			}},
		}
		for _, s := range scenes {
			t.Run(s.name, func(t *testing.T) {
				r := newLiveRig(t, w, 120, 40, "tree")
				board := &fakeBoard{res: clipboard.Result{Copied: []string{"wl-copy"}}}
				r.a.o.Clipboard = board
				r.a.bds.Workspace.Prefix = "t"
				s.run(r)
				r.waitReady()
				r.key("ctrl+s")
				got := board.got()
				if len(got) != 1 {
					t.Fatalf("board got %d texts:\n%s", len(got), screen(r.a))
				}
				testgolden.Equal(t, normalizeExport(got[0]))
			})
		}
	})
}
