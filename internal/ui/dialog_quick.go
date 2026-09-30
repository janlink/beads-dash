package ui

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/form"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

type quickKind int

const (
	quickStatus quickKind = iota
	quickPriority
	quickAssignee
	quickLabels
)

// quickDialog is the small popup for one field of the marked issues or the
// current one: status, priority, assignee or labels.
type quickDialog struct {
	formDialog
	kind   quickKind
	ids    []string
	marked bool
	// orig are the labels of a single target, to measure edits against.
	orig []string
	key  string
}

func (a *App) openQuick(kind quickKind) tea.Cmd {
	ids, marked := a.targets()
	if len(ids) == 0 || a.snap == nil {
		a.hint = "no issue to change"
		return nil
	}
	a.pushDialog(a.newQuick(kind, ids, marked))
	return nil
}

func (a *App) newQuick(kind quickKind, ids []string, marked bool) *quickDialog {
	d := &quickDialog{kind: kind, ids: ids, marked: marked}
	single, _ := a.snap.Issue(ids[0])
	if len(ids) != 1 {
		single = nil
	}
	subject := issueWord(len(ids))
	if single != nil {
		subject = single.ID
	}
	var f *form.Field
	var title string
	switch kind {
	case quickStatus:
		cur := "open"
		if single != nil {
			cur = single.Status
		}
		f, title, d.key = form.NewChoice("status", "Status", a.statusNames(), cur), "Status", "status"
	case quickPriority:
		cur := "P2"
		if single != nil {
			cur = priorityText(single.Priority)
		}
		f, title, d.key = form.NewChoice("priority", "Priority", priorityOptions, cur), "Priority", "priority"
	case quickAssignee:
		cur := ""
		if single != nil {
			cur = single.Assignee
		}
		assignees, _ := a.issueSuggestions()
		f = form.NewText("assignee", "Assignee", cur)
		f.Suggest = complete(append([]string{"me", "unassigned"}, assignees...))
		f.Placeholder = "name, me, or unassigned"
		title = "Assignee"
	case quickLabels:
		var words []string
		if single != nil {
			words = single.Labels
			d.orig = slices.Clone(words)
		}
		_, labels := a.issueSuggestions()
		f = form.NewTokens("labels", "Labels", words)
		f.Suggest = complete(labels)
		if single == nil {
			f.Placeholder = "+add -remove"
		}
		title = "Labels"
	}
	d.f = form.New(f)
	d.formDialog = formDialog{a: a, h: d, self: d, title: title + " · " + subject, f: d.f}
	return d
}

func (d *quickDialog) dirty() bool { return false }

func (d *quickDialog) aside() string { return "" }

func (d *quickDialog) conflicted() bool { return false }

func (d *quickDialog) edited(*form.Field) {}

func (d *quickDialog) pick(*form.Field) tea.Cmd { return nil }

func (d *quickDialog) extra(keys.Action) tea.Cmd { return nil }

func (d *quickDialog) banner(l look.Look, _ int) []string {
	var text string
	switch d.kind {
	case quickPriority:
		text = "Press 0-4 to set at once."
	case quickStatus:
		text = "Type the first letter; a unique one sets at once."
	case quickLabels:
		text = "Bare words are the whole set; +x adds, -x removes."
	case quickAssignee:
	}
	if text == "" {
		return nil
	}
	return []string{l.Paint(theme.Faint, text), ""}
}

// Type takes the digit of the priority and the unique letter of a status as
// the answer itself.
func (d *quickDialog) Type(m tea.KeyPressMsg) tea.Cmd {
	if d.busy {
		return nil
	}
	plain := m.Text != "" && m.Mod == 0
	f := d.f.Focused()
	switch {
	case plain && d.kind == quickPriority && m.Text >= "0" && m.Text <= "4":
		f.Set(priorityBase + m.Text)
		return d.formDialog.submit()
	case plain && d.kind == quickStatus:
		var match []string
		for _, o := range f.Options {
			if strings.HasPrefix(strings.ToLower(o), strings.ToLower(m.Text)) {
				match = append(match, o)
			}
		}
		if len(match) == 1 {
			f.Set(match[0])
			return d.formDialog.submit()
		}
	}
	return d.formDialog.Type(m)
}

func (d *quickDialog) op() (changeOp, bool) {
	a := d.a
	f := d.f.Focused()
	switch d.kind {
	case quickStatus:
		return a.statusChange(d.ids, f.Value()), true
	case quickPriority:
		p := priorityOf(f.Value())
		if p == nil {
			return changeOp{}, false
		}
		return priorityChange(*p), true
	case quickAssignee:
		op, err := a.assigneeChange(f.Value())
		if err != nil {
			d.errShort, d.errFull = err.Error(), nil
			return changeOp{}, false
		}
		return op, true
	case quickLabels:
		add, remove := labelWords(d.orig, strings.Fields(strings.ReplaceAll(f.Value(), ",", " ")), len(d.ids) == 1)
		if len(add) == 0 && len(remove) == 0 {
			return changeOp{}, false
		}
		return labelChange(add, remove), true
	}
	return changeOp{}, false
}

func (d *quickDialog) submit() tea.Cmd {
	if d.kind == quickStatus && d.a.closesTo(d.f.Focused().Value()) {
		ids, marked := d.ids, d.marked
		d.a.dropDialog(d)
		d.a.pushDialog(d.a.newCloseDialog(ids, marked))
		return nil
	}
	d.errShort, d.errFull = "", nil
	op, ok := d.op()
	if !ok {
		if d.errShort == "" {
			d.errShort = "Nothing to change"
		}
		return nil
	}
	ids := slices.Clone(d.ids)
	return d.start(writeOp{
		ids: ids,
		run: func(ctx context.Context, c bd.Client) (string, error) { return "", op.run(ctx, c, ids) },
	})
}

func (d *quickDialog) finish(res writeResult) bool {
	if d.marked {
		d.a.settleMarks(d.ids, res.err)
	}
	if res.err != nil {
		d.fail(res.err)
		d.errShort = writeSummary(res.err, len(d.ids))
		return false
	}
	op, _ := d.op()
	d.a.toast(op.did + " (" + issueWord(len(d.ids)) + ")")
	return true
}
