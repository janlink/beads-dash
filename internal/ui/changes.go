package ui

import (
	"context"
	"errors"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
)

// change is one write over several issues in one bd call, with the words
// the notice uses.
type changeOp struct {
	did string
	run func(ctx context.Context, c bd.Client, ids []string) error
}

func (a *App) statusChange(ids []string, status string) changeOp {
	var closed []string
	for _, id := range ids {
		if a.isClosed(id) {
			closed = append(closed, id)
		}
	}
	return changeOp{did: "status " + status, run: func(ctx context.Context, c bd.Client, ids []string) error {
		if len(closed) > 0 {
			if _, err := c.Reopen(ctx, closed, ""); err != nil {
				return err
			}
		}
		upd := slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return status == "open" && slices.Contains(closed, id) })
		if len(upd) == 0 {
			return nil
		}
		return c.Update(ctx, upd, bd.UpdateSpec{Status: &status})
	}}
}

func priorityChange(p int) changeOp {
	return changeOp{did: "priority " + priorityText(p), run: func(ctx context.Context, c bd.Client, ids []string) error {
		return c.Update(ctx, ids, bd.UpdateSpec{Priority: &p})
	}}
}

func (a *App) assigneeChange(v string) (changeOp, error) {
	who := v
	switch v {
	case "me":
		who = a.actor()
		if who == "" {
			return changeOp{}, errors.New(errNoActor)
		}
	case "-", "unassigned", "none":
		who = ""
	}
	did := "assigned to " + who
	if who == "" {
		did = "unassigned"
	}
	return changeOp{did: did, run: func(ctx context.Context, c bd.Client, ids []string) error {
		return c.Update(ctx, ids, bd.UpdateSpec{Assignee: &who})
	}}, nil
}

func labelChange(add, remove []string) changeOp {
	var parts []string
	for _, l := range add {
		parts = append(parts, "+"+l)
	}
	for _, l := range remove {
		parts = append(parts, "-"+l)
	}
	return changeOp{did: "labels " + strings.Join(parts, " "), run: func(ctx context.Context, c bd.Client, ids []string) error {
		return c.Update(ctx, ids, bd.UpdateSpec{AddLabels: add, RemoveLabels: remove})
	}}
}

func claimChange() changeOp {
	return changeOp{did: "claimed", run: func(ctx context.Context, c bd.Client, ids []string) error {
		return c.Update(ctx, ids, bd.UpdateSpec{Claim: true})
	}}
}

// labelWords reads label words: +x adds, -x removes, and a bare word adds. In
// set mode bare words are the whole set, measured against orig, and no words
// at all clear it; words that are all +x or -x stay explicit.
func labelWords(orig, words []string, set bool) (add, remove []string) {
	var bare []string
	for _, w := range words {
		switch {
		case strings.HasPrefix(w, "+") && len(w) > 1:
			add = append(add, w[1:])
		case strings.HasPrefix(w, "-") && len(w) > 1:
			remove = append(remove, w[1:])
		default:
			bare = append(bare, w)
		}
	}
	if !set {
		return append(add, bare...), remove
	}
	if len(words) > 0 && len(bare) == 0 {
		return add, remove
	}
	for _, l := range bare {
		if !slices.Contains(orig, l) {
			add = append(add, l)
		}
	}
	for _, l := range orig {
		if !slices.Contains(bare, l) {
			remove = append(remove, l)
		}
	}
	return add, remove
}

// moveTo is where a Kanban card goes.
type moveKind int

const (
	moveNone moveKind = iota
	moveStart
	moveStop
	moveClose
	moveReopen
)

func moveOf(p model.PresentationStatus, forward bool) moveKind {
	switch {
	case forward && p == model.Open:
		return moveStart
	case forward && p == model.InProgress:
		return moveClose
	case !forward && p == model.InProgress:
		return moveStop
	case !forward && p == model.Closed:
		return moveReopen
	}
	return moveNone
}

// moveCards moves the marked cards or the current one along Open, In progress
// and Closed. Cards in any other column stay.
func (a *App) moveCards(forward bool) tea.Cmd {
	if _, ok := a.view().(*Kanban); !ok {
		a.hint = "cards move in the Kanban view"
		return nil
	}
	ids, marked := a.targets()
	if len(ids) == 0 || a.snap == nil {
		return nil
	}
	groups := map[moveKind][]string{}
	skipped := 0
	for _, id := range ids {
		k := moveOf(a.snap.Present(id, a.bds.Statuses).Status, forward)
		if k == moveNone {
			skipped++
			continue
		}
		groups[k] = append(groups[k], id)
	}
	if len(groups) == 0 {
		a.hint = "only Open, In progress and Closed cards move"
		return nil
	}
	if skipped > 0 {
		a.hint = issueWord(skipped) + " stayed: only Open, In progress and Closed cards move"
	}
	var cmds []tea.Cmd
	if ids := groups[moveStart]; len(ids) > 0 {
		s := "in_progress"
		cmds = append(cmds, a.change(ids, marked, "status "+s, func(ctx context.Context, c bd.Client, ids []string) error {
			return c.Update(ctx, ids, bd.UpdateSpec{Status: &s})
		}))
	}
	if ids := groups[moveStop]; len(ids) > 0 {
		s := "open"
		cmds = append(cmds, a.change(ids, marked, "status "+s, func(ctx context.Context, c bd.Client, ids []string) error {
			return c.Update(ctx, ids, bd.UpdateSpec{Status: &s})
		}))
	}
	if ids := groups[moveReopen]; len(ids) > 0 {
		cmds = append(cmds, a.change(ids, marked, "reopened", func(ctx context.Context, c bd.Client, ids []string) error {
			_, err := c.Reopen(ctx, ids, "")
			return err
		}))
	}
	if ids := groups[moveClose]; len(ids) > 0 {
		a.pushDialog(a.newCloseDialog(ids, marked))
	}
	return tea.Batch(cmds...)
}
