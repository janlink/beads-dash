package ui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/form"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// refusal is one reason bd gave for not closing an issue.
type refusal struct {
	issue, by, why string
}

// closeDialog closes or reopens issues with an optional reason. bd's guards
// stay in charge: when they refuse, the dialog lists what stands in the way
// and Enter jumps to it.
type closeDialog struct {
	formDialog
	ids    []string
	marked bool
	reopen bool
	// stuck are the issues bd refused, and text what it said.
	stuck []string
	text  []string
	sel   int
}

// closesTo reports whether setting the status ends the issue, which only bd
// close does; update -s would leave it without a closed_at.
func (a *App) closesTo(status string) bool {
	return status == "closed" || a.bds.Statuses.Category(status) == model.CategoryDone
}

func (a *App) isClosed(id string) bool {
	return a.snap != nil && a.snap.Present(id, a.bds.Statuses).Status == model.Closed
}

// openClose opens the close or reopen dialog for the marked issues or the
// current one: reopen when they are all closed, else close.
func (a *App) openClose() tea.Cmd {
	ids, marked := a.targets()
	if len(ids) == 0 || a.snap == nil {
		a.hint = "no issue to close"
		return nil
	}
	a.pushDialog(a.newCloseDialog(ids, marked))
	return nil
}

func (a *App) newCloseDialog(ids []string, marked bool) *closeDialog {
	var open []string
	for _, id := range ids {
		if !a.isClosed(id) {
			open = append(open, id)
		}
	}
	d := &closeDialog{marked: marked, reopen: len(open) == 0, ids: open}
	if d.reopen {
		d.ids = slices.Clone(ids)
	}
	verb := "Close"
	if d.reopen {
		verb = "Reopen"
	}
	title := verb + " " + issueWord(len(d.ids))
	if len(d.ids) == 1 {
		title = verb + " " + d.ids[0]
	}
	reason := form.NewText("reason", "Reason", "")
	reason.Placeholder = "optional"
	d.f = form.New(reason)
	d.formDialog = formDialog{a: a, h: d, self: d, title: title, f: d.f}
	return d
}

func (d *closeDialog) dirty() bool { return d.f.Changes() > 0 }

func (d *closeDialog) aside() string { return "" }

func (d *closeDialog) conflicted() bool { return false }

func (d *closeDialog) edited(*form.Field) {}

func (d *closeDialog) pick(*form.Field) tea.Cmd { return nil }

func (d *closeDialog) extra(keys.Action) tea.Cmd { return nil }

func (d *closeDialog) banner(l look.Look, w int) []string {
	var out []string
	for _, id := range d.ids[:min(len(d.ids), 5)] {
		out = append(out, l.Paint(theme.Dim, l.Fit(id+" "+d.title1(id), w)))
	}
	if len(d.ids) > 5 {
		out = append(out, l.Paint(theme.Faint, fmt.Sprintf("and %d more", len(d.ids)-5)))
	}
	return append(out, "")
}

func (d *closeDialog) title1(id string) string {
	if is, ok := d.a.snap.Issue(id); ok {
		return oneLineText(is.Title)
	}
	return ""
}

func (d *closeDialog) submit() tea.Cmd {
	reason := d.f.Field("reason").Value()
	ids := slices.Clone(d.ids)
	reopen := d.reopen
	return d.start(writeOp{
		ids: ids,
		run: func(ctx context.Context, c bd.Client) (string, error) {
			var err error
			if reopen {
				_, err = c.Reopen(ctx, ids, reason)
			} else {
				_, err = c.Close(ctx, ids, reason)
			}
			return "", err
		},
	})
}

func (d *closeDialog) finish(res writeResult) bool {
	a := d.a
	if d.marked {
		a.settleMarks(d.ids, res.err)
	}
	if res.err == nil {
		verb := "closed"
		if d.reopen {
			verb = "reopened"
		}
		a.toast(verb + " " + issueWord(len(d.ids)))
		return true
	}
	d.fail(res.err)
	if d.reopen {
		return false
	}
	d.stuck, d.text = d.stuckOn(res.err), writeDetail(res.err)
	if len(d.stuck) > 0 {
		d.errShort, d.errFull = "", nil
		d.sel = 0
	}
	return false
}

// stuckOn names the issues bd refused: those it listed as failed, or every
// target of a refusal that named none.
func (d *closeDialog) stuckOn(err error) []string {
	var be *bd.Error
	if !errors.As(err, &be) || (be.Class != bd.ClassRejected && be.Class != bd.ClassPartialWrite) {
		return nil
	}
	var out []string
	for _, f := range be.Failed {
		out = append(out, f.ID)
	}
	if len(out) == 0 && be.Class == bd.ClassRejected {
		out = slices.Clone(d.ids)
	}
	return out
}

// rows lists what stands in the way of each refused issue, from the snapshot
// as it is now.
func (d *closeDialog) rows() []refusal {
	var out []refusal
	for _, id := range d.stuck {
		snap := d.a.snap
		if snap == nil {
			continue
		}
		for _, b := range snap.BlockedBy(id) {
			out = append(out, refusal{issue: id, by: b, why: "blocked by"})
		}
		for _, c := range snap.Children(id) {
			if !d.a.isClosed(c) {
				out = append(out, refusal{issue: id, by: c, why: "open child"})
			}
		}
	}
	return out
}

func (d *closeDialog) Context() keys.Context {
	if len(d.stuck) > 0 {
		return keys.Refused
	}
	return keys.Form
}

func (d *closeDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	if len(d.stuck) == 0 {
		return d.formDialog.Handle(act)
	}
	rows := d.rows()
	switch act { //nolint:exhaustive // only the actions of the refused context arrive
	case keys.Close:
		return nil, true
	case keys.NavDown:
		d.sel = min(d.sel+1, max(len(rows)-1, 0))
	case keys.NavUp:
		d.sel = max(d.sel-1, 0)
	case keys.Open:
		if d.sel < len(rows) {
			target := rows[d.sel].by
			d.a.dropDialog(d)
			d.a.jumpTo(target)
		}
	}
	return nil, false
}

func (d *closeDialog) Type(m tea.KeyPressMsg) tea.Cmd {
	if len(d.stuck) > 0 {
		return nil
	}
	return d.formDialog.Type(m)
}

func (d *closeDialog) Frame(l look.Look, cols, rows int) dialog.Frame {
	if len(d.stuck) == 0 {
		return d.formDialog.Frame(l, cols, rows)
	}
	iw := d.innerWidth(cols, rows)
	var body []string
	for _, line := range d.text[:min(len(d.text), 3)] {
		for _, part := range wrapText(line, iw) {
			body = append(body, l.Paint(theme.Error, part))
		}
	}
	if extra := len(d.text) - 3; extra > 0 {
		body = append(body, l.Paint(theme.Faint, fmt.Sprintf("+%d more", extra)))
	}
	list := d.rows()
	if len(list) == 0 {
		body = append(body, "", l.Paint(theme.Dim, "Nothing to jump to."))
	} else {
		body = append(body, "", l.Paint(theme.Primary, "Stands in the way"))
	}
	for i, r := range list {
		title := ""
		if is, ok := d.a.snap.Issue(r.by); ok {
			title = oneLineText(is.Title)
		}
		line := fmt.Sprintf("  %s %s %s %s", r.issue, r.why, r.by, title)
		line = l.Fit(line, iw)
		if i == d.sel {
			body = append(body, l.PaintSel(theme.Text, line))
		} else {
			body = append(body, l.Paint(theme.Text, line))
		}
	}
	aside := ""
	if len(list) > 0 {
		aside = fmt.Sprintf("%d/%d", d.sel+1, len(list))
	}
	return dialog.Frame{Title: strings.Replace(d.title, "Close", "Cannot close", 1), Aside: aside, Hints: d.a.hintsFor(keys.Refused), Body: body}
}

func (d *closeDialog) hints() []keys.Hint {
	if len(d.stuck) > 0 {
		return d.a.hintsFor(keys.Refused)
	}
	return d.formDialog.hints()
}
