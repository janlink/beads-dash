package ui

import (
	"context"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/ui/command"
)

func texts(args []command.Word) []string {
	out := make([]string, len(args))
	for i, w := range args {
		out[i] = w.Text
	}
	return out
}

func (a *App) assigneeArgs(prev []string, prefix string) []string {
	if len(prev) > 0 {
		return nil
	}
	assignees, _ := a.issueSuggestions()
	return filterPrefix(append([]string{"me", "-"}, assignees...), prefix)
}

func (a *App) labelArgs(_ []string, prefix string) []string {
	sign, word := "", prefix
	if strings.HasPrefix(prefix, "+") || strings.HasPrefix(prefix, "-") {
		sign, word = prefix[:1], prefix[1:]
	}
	_, labels := a.issueSuggestions()
	var out []string
	for _, l := range filterPrefix(labels, word) {
		out = append(out, sign+l)
	}
	return out
}

func (a *App) idArgs(prefix string) []string { return a.idsWith(prefix, maxIDCandidate) }

func (a *App) registerWrites() {
	a.register(command.Spec{
		Name: "status", Aliases: []string{"s"}, Usage: "<status>", Summary: "set the status", Min: 1, Max: 1,
		Help: "Sets the status of the marked issues, else the current one. closed opens the close dialog.",
		Args: func(prev []string, prefix string) []string {
			if len(prev) > 0 {
				return nil
			}
			return filterPrefix(a.statusNames(), prefix)
		},
	}, (*App).cmdStatus)
	a.register(command.Spec{
		Name: "priority", Aliases: []string{"p"}, Usage: "<0-4>", Summary: "set the priority", Min: 1, Max: 1,
		Help: "Sets the priority of the marked issues, else the current one.",
		Args: choose([]string{"0", "1", "2", "3", "4"}),
	}, (*App).cmdPriority)
	a.register(command.Spec{
		Name: "assign", Usage: "<name|me|->", Summary: "set the assignee", Min: 1, Max: 1,
		Help: "Assigns the marked issues, else the current one. me is you; - unassigns.",
		Args: a.assigneeArgs,
	}, (*App).cmdAssign)
	a.register(command.Spec{
		Name: "label", Usage: "+x -y ...", Summary: "add and remove labels", Min: 1, Max: -1,
		Help: "+x adds a label and -x removes one, on the marked issues, else the current one. A bare word adds.",
		Args: a.labelArgs,
	}, (*App).cmdLabel)
	a.register(command.Spec{
		Name: "close", Usage: "[reason]", Summary: "close with a reason", Max: -1,
		Help: "Closes the marked issues, else the current one. Without a reason the close dialog opens. bd's guards still apply.",
	}, (*App).cmdClose)
	a.register(command.Spec{
		Name: "reopen", Usage: "[reason]", Summary: "reopen with a reason", Max: -1,
		Help: "Reopens the marked issues, else the current one. Without a reason the dialog opens.",
	}, (*App).cmdReopen)
	a.register(command.Spec{
		Name: "claim", Summary: "claim the issue",
		Help: "Assigns the marked issues, else the current one, to you and sets them in progress. Fails for an issue someone else holds.",
	}, (*App).cmdClaim)
	a.register(command.Spec{
		Name: "dep", Usage: "add|rm <id>", Summary: "add or remove a blocker", Min: 2, Max: 2,
		Help: "Makes the current issue blocked by another one, or removes that. Acts on the current issue only.",
		Args: func(prev []string, prefix string) []string {
			switch len(prev) {
			case 0:
				return filterPrefix([]string{"add", "rm"}, prefix)
			case 1:
				return a.idArgs(prefix)
			}
			return nil
		},
	}, (*App).cmdDep)
	a.register(command.Spec{
		Name: "parent", Usage: "<id|->", Summary: "set the parent", Min: 1, Max: 1,
		Help: "Moves the current issue under another one; - removes the parent. Acts on the current issue only.",
		Args: func(prev []string, prefix string) []string {
			if len(prev) > 0 {
				return nil
			}
			return a.idArgs(prefix)
		},
	}, (*App).cmdParent)
	a.register(command.Spec{
		Name: "new", Usage: "[title]", Summary: "new issue", Max: -1,
		Help: "Opens the issue form for a new issue, with the title filled in.",
	}, (*App).cmdNew)
	a.register(command.Spec{
		Name: "edit", Summary: "edit the current issue",
		Help: "Opens the issue form for the current issue.",
	}, (*App).cmdEdit)
}

func (a *App) needTargets() ([]string, bool, error) {
	ids, marked := a.targets()
	if len(ids) == 0 || a.snap == nil {
		return nil, false, &command.Error{Msg: "no issue to change"}
	}
	return ids, marked, nil
}

func (a *App) cmdStatus(args []command.Word) (tea.Cmd, error) {
	ids, marked, err := a.needTargets()
	if err != nil {
		return nil, err
	}
	name := strings.ToLower(args[0].Text)
	if !slices.Contains(a.statusNames(), name) {
		return nil, argError(args[0], "no status %q; try %s", args[0].Text, strings.Join(a.statusNames(), ", "))
	}
	if a.closesTo(name) {
		a.pushDialog(a.newCloseDialog(ids, marked))
		return nil, nil
	}
	op := a.statusChange(ids, name)
	return a.change(ids, marked, op.did, op.run), nil
}

func (a *App) cmdPriority(args []command.Word) (tea.Cmd, error) {
	ids, marked, err := a.needTargets()
	if err != nil {
		return nil, err
	}
	p := priorityOf(priorityBase + strings.TrimPrefix(strings.ToUpper(args[0].Text), priorityBase))
	if p == nil {
		return nil, argError(args[0], "priority is 0 to 4")
	}
	op := priorityChange(*p)
	return a.change(ids, marked, op.did, op.run), nil
}

func (a *App) cmdAssign(args []command.Word) (tea.Cmd, error) {
	ids, marked, err := a.needTargets()
	if err != nil {
		return nil, err
	}
	op, err := a.assigneeChange(args[0].Text)
	if err != nil {
		return nil, argError(args[0], "%s", err)
	}
	return a.change(ids, marked, op.did, op.run), nil
}

func (a *App) cmdLabel(args []command.Word) (tea.Cmd, error) {
	ids, marked, err := a.needTargets()
	if err != nil {
		return nil, err
	}
	add, remove := labelWords(nil, texts(args), false)
	if len(add) == 0 && len(remove) == 0 {
		return nil, argError(args[0], "give a label as +x or -x")
	}
	op := labelChange(add, remove)
	return a.change(ids, marked, op.did, op.run), nil
}

func (a *App) cmdClose(args []command.Word) (tea.Cmd, error) {
	return a.closeCommand(args, false)
}

func (a *App) cmdReopen(args []command.Word) (tea.Cmd, error) {
	return a.closeCommand(args, true)
}

// closeCommand opens the close or reopen dialog; with a reason it submits at
// once, so a refusal still lists what stands in the way.
func (a *App) closeCommand(args []command.Word, reopen bool) (tea.Cmd, error) {
	ids, marked, err := a.needTargets()
	if err != nil {
		return nil, err
	}
	var open []string
	for _, id := range ids {
		if a.isClosed(id) == reopen {
			open = append(open, id)
		}
	}
	if len(open) == 0 {
		if reopen {
			return nil, &command.Error{Msg: "nothing to reopen: no issue is closed"}
		}
		return nil, &command.Error{Msg: "nothing to close: every issue is closed already"}
	}
	d := a.newCloseDialog(open, marked)
	if d.reopen != reopen {
		return nil, &command.Error{Msg: "nothing to do"}
	}
	a.pushDialog(d)
	if len(args) == 0 {
		return nil, nil
	}
	d.f.Field("reason").Set(strings.Join(texts(args), " "))
	return d.formDialog.submit(), nil
}

func (a *App) cmdClaim([]command.Word) (tea.Cmd, error) {
	ids, marked, err := a.needTargets()
	if err != nil {
		return nil, err
	}
	op := claimChange()
	return a.change(ids, marked, op.did, op.run), nil
}

// single runs one write that is not a change of a set of issues.
func (a *App) single(ids []string, did string, run func(ctx context.Context, c bd.Client) error) tea.Cmd {
	return a.write(writeOp{
		ids: ids,
		run: func(ctx context.Context, c bd.Client) (string, error) { return "", run(ctx, c) },
		done: func(a *App, res writeResult) tea.Cmd {
			if res.err != nil {
				a.warn(writeSummary(res.err, len(ids)))
				return nil
			}
			a.toast(did)
			return nil
		},
	})
}

func (a *App) cmdDep(args []command.Word) (tea.Cmd, error) {
	cur := a.sess.Current()
	if cur == "" || a.snap == nil {
		return nil, &command.Error{Msg: "no current issue"}
	}
	verb := strings.ToLower(args[0].Text)
	if verb != "add" && verb != "rm" {
		return nil, argError(args[0], "use add or rm")
	}
	other, ok := a.resolveID(args[1].Text)
	if !ok {
		return nil, argError(args[1], "no issue %q", args[1].Text)
	}
	ids := []string{cur, other}
	if verb == "rm" {
		return a.single(ids, cur+" no longer blocked by "+other, func(ctx context.Context, c bd.Client) error {
			return c.DepRemove(ctx, cur, other)
		}), nil
	}
	switch {
	case other == cur:
		return nil, argError(args[1], "an issue cannot block itself")
	case a.dependsOn(other, cur):
		return nil, argError(args[1], "%s already depends on %s: that would make a cycle", other, cur)
	}
	return a.single(ids, cur+" blocked by "+other, func(ctx context.Context, c bd.Client) error {
		return c.DepAdd(ctx, cur, other, depBlocks)
	}), nil
}

func (a *App) cmdParent(args []command.Word) (tea.Cmd, error) {
	cur := a.sess.Current()
	if cur == "" || a.snap == nil {
		return nil, &command.Error{Msg: "no current issue"}
	}
	parent := ""
	if args[0].Text != "-" {
		id, ok := a.resolveID(args[0].Text)
		if !ok {
			return nil, argError(args[0], "no issue %q", args[0].Text)
		}
		if id == cur || a.isDescendant(id, cur) {
			return nil, argError(args[0], "%s is the issue or below it", id)
		}
		parent = id
	}
	did := cur + " has no parent"
	if parent != "" {
		did = cur + " under " + parent
	}
	return a.single([]string{cur, parent}, did, func(ctx context.Context, c bd.Client) error {
		return c.Update(ctx, []string{cur}, bd.UpdateSpec{Parent: &parent})
	}), nil
}

func (a *App) cmdNew(args []command.Word) (tea.Cmd, error) {
	return a.openNew(strings.Join(texts(args), " ")), nil
}

func (a *App) cmdEdit([]command.Word) (tea.Cmd, error) {
	if a.sess.Current() == "" {
		return nil, &command.Error{Msg: "no current issue"}
	}
	return a.openEdit(), nil
}
