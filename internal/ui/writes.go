package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/ui/screens"
	"github.com/janlink/beads-dash/internal/ui/state"
)

// writeResult is what a bd write answered: the error, and the ID of an issue
// it created.
type writeResult struct {
	err     error
	created string
}

type writeDoneMsg struct {
	seq int
	res writeResult
}

// writeOp is one bd write and what happens with its outcome.
type writeOp struct {
	// ids are the issues the write touches; the engine credits their events
	// to the user.
	ids []string
	run func(ctx context.Context, c bd.Client) (created string, err error)
	// done runs on the UI goroutine once the engine has refreshed.
	done func(a *App, res writeResult) tea.Cmd
}

// write sends op through the engine's write queue. The outcome comes back as
// a writeDoneMsg and reaches op.done; the engine refreshes after every write,
// failed ones included.
func (a *App) write(op writeOp) tea.Cmd {
	a.writeSeq++
	seq := a.writeSeq
	if a.pending == nil {
		a.pending = map[int]writeOp{}
	}
	a.pending[seq] = op
	eng := a.eng
	ids := slices.DeleteFunc(slices.Clone(op.ids), func(id string) bool { return id == "" })
	return func() tea.Msg {
		if eng == nil {
			return writeDoneMsg{seq: seq, res: writeResult{err: errNoEngine}}
		}
		var created string
		err := eng.Write(context.Background(), ids, func(ctx context.Context, c bd.Client) error {
			var err error
			created, err = op.run(ctx, c)
			return err
		})
		return writeDoneMsg{seq: seq, res: writeResult{err: err, created: created}}
	}
}

func (a *App) onWrite(m writeDoneMsg) tea.Cmd {
	op, ok := a.pending[m.seq]
	if !ok {
		return nil
	}
	delete(a.pending, m.seq)
	var cmd tea.Cmd
	if op.done != nil {
		cmd = op.done(a, m.res)
	}
	return tea.Batch(cmd, a.syncPause())
}

// toast shows a plain notice; warn one in the warning colour.
func (a *App) toast(text string) { a.notices = append(a.notices, screens.Notice{Text: text}) }

func (a *App) warn(text string) {
	a.notices = append(a.notices, screens.Notice{Text: text, Warn: true})
}

// focusIssue makes id the current issue without touching the back stack. An
// issue the snapshot does not hold yet becomes current when it arrives.
func (a *App) focusIssue(id string) {
	if a.snap == nil {
		a.pendingCurrent = id
		return
	}
	if _, ok := a.snap.Issue(id); !ok {
		a.pendingCurrent = id
		return
	}
	a.pendingCurrent = ""
	if !a.visible(id) && a.scope.Active() {
		a.applyScope(a.clearedScope())
	}
	a.sess.SetCurrent(id)
}

// targets are the issues a change acts on: the marked ones, else the current
// one. marked reports which of the two it is.
func (a *App) targets() (ids []string, marked bool) {
	if ids = a.sess.MarkedIDs(); len(ids) > 0 {
		return ids, true
	}
	if cur := a.sess.Current(); cur != "" {
		return []string{cur}, false
	}
	return nil, false
}

// issueWord is "1 issue" or "3 issues".
func issueWord(n int) string {
	if n == 1 {
		return "1 issue"
	}
	return fmt.Sprintf("%d issues", n)
}

// settleMarks drops the marks of the issues a write changed: all of ids on
// success, only the applied ones on a partial write, so what stays marked is
// what still needs doing.
func (a *App) settleMarks(ids []string, err error) {
	if err == nil {
		a.sess.Unmark(ids...)
		return
	}
	var be *bd.Error
	if errors.As(err, &be) && be.Class == bd.ClassPartialWrite {
		a.sess.Unmark(be.Applied...)
	}
}

// change runs one bd write over ids. Marks stay on whatever the write left
// undone, so the change can be tried again.
func (a *App) change(ids []string, marked bool, did string, run func(ctx context.Context, c bd.Client, ids []string) error) tea.Cmd {
	if len(ids) == 0 {
		a.hint = "no issue to change"
		return nil
	}
	ids = slices.Clone(ids)
	return a.write(writeOp{
		ids: ids,
		run: func(ctx context.Context, c bd.Client) (string, error) { return "", run(ctx, c, ids) },
		done: func(a *App, res writeResult) tea.Cmd {
			if marked {
				a.settleMarks(ids, res.err)
			}
			if res.err != nil {
				a.warn(writeSummary(res.err, len(ids)))
				return nil
			}
			a.toast(fmt.Sprintf("%s (%s)", did, issueWord(len(ids))))
			return nil
		},
	})
}

// writeSummary is the one-line account of a failed write of n issues.
func writeSummary(err error, n int) string {
	var be *bd.Error
	if !errors.As(err, &be) {
		return "write failed: " + shortLine(err.Error())
	}
	if be.Class == bd.ClassPartialWrite && len(be.Applied) > 0 && len(be.Failed) > 0 {
		text := fmt.Sprintf("%d of %d changed", len(be.Applied), n)
		if len(be.Failed) > 0 {
			text += "; " + failureLine(be.Failed[0])
		}
		return text
	}
	if len(be.Failed) > 0 {
		return failureLine(be.Failed[0])
	}
	if be.Message != "" {
		return shortLine(be.Message)
	}
	return "write failed: " + shortLine(be.Error())
}

func failureLine(f bd.WriteFailure) string { return f.ID + ": " + shortLine(f.Message) }

// writeDetail is the full text of a write error, one entry per line.
func writeDetail(err error) []string {
	var be *bd.Error
	if !errors.As(err, &be) {
		return splitLines(err.Error())
	}
	var out []string
	if be.Message != "" {
		out = append(out, splitLines(be.Message)...)
	}
	for _, f := range be.Failed {
		out = append(out, splitLines(f.ID+": "+f.Message)...)
	}
	if len(be.Applied) > 0 {
		out = append(out, "changed: "+strings.Join(be.Applied, ", "))
	}
	if len(out) == 0 {
		out = splitLines(be.Error())
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func shortLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

const errNoActor = "bdash does not know who you are; set BEADS_ACTOR"

// actor is who "me" is: the actor resolved at start-up in bd's order.
func (a *App) actor() string { return a.o.Actor }

// dropDialog closes d wherever it sits on the dialog stack.
func (a *App) dropDialog(d Dialog) {
	if i := slices.Index(a.dialogs, d); i >= 0 {
		a.dialogs = slices.Delete(a.dialogs, i, i+1)
		a.sess.Remove(state.LayerDialog)
	}
}
