package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/command"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/state"
)

// commandFunc runs a parsed command. A *command.Error it returns is shown
// under the bar at its position; any other error at the start of the line.
type commandFunc func(a *App, args []command.Word) (tea.Cmd, error)

// register adds a command to the command bar. Commands are registered once,
// at start.
func (a *App) register(s command.Spec, run commandFunc) {
	if err := a.cmds.Register(s); err != nil {
		panic(err)
	}
	a.run[strings.ToLower(s.Name)] = run
}

func argError(w command.Word, format string, args ...any) *command.Error {
	return &command.Error{Pos: w.Pos, Msg: fmt.Sprintf(format, args...)}
}

func (a *App) registerBuiltins() {
	names := make([]string, 0, 2*len(ViewNames))
	for i, n := range ViewNames {
		names = append(names, strings.ToLower(n), strconv.Itoa(i+1))
	}
	a.register(command.Spec{
		Name: "view", Usage: "<name|1-6>", Summary: "switch view", Min: 1, Max: 1,
		Help: "Switches to Overview, Tree, Kanban, Ready, Memories or Graph, by name or number.",
		Args: choose(names),
	}, (*App).cmdView)
	a.register(command.Spec{
		Name: "theme", Usage: "<name>", Summary: "set the colour theme", Min: 1, Max: 1,
		Help: "Applies a theme and saves it to the config file.",
		Args: choose(theme.Names()),
	}, (*App).cmdTheme)
	a.register(command.Spec{
		Name: "glyphs", Usage: "<auto|fancy|safe|ascii>", Summary: "set the glyph set", Min: 1, Max: 1,
		Help: "Applies a glyph set and saves it to the config file.",
		Args: choose(config.GlyphTiers),
	}, (*App).cmdGlyphs)
	a.register(command.Spec{
		Name: "clear", Summary: "clear the search and filter",
		Help: "Drops the search text and every filter, status filters included; only whether closed issues show by default stays.",
	}, (*App).cmdClear)
	a.register(command.Spec{
		Name: "refresh", Summary: "refresh from bd now",
	}, (*App).cmdRefresh)
	a.register(command.Spec{
		Name: "help", Usage: "[command]", Summary: "list the commands", Max: 1,
		Help: "Without an argument, lists every command; with one, explains it.",
		Args: func(prev []string, prefix string) []string {
			if len(prev) > 0 {
				return nil
			}
			var out []string
			for _, s := range a.cmds.Specs() {
				out = append(out, s.Name)
			}
			return filterPrefix(out, prefix)
		},
	}, (*App).cmdHelp)
	a.register(command.Spec{
		Name: "q", Aliases: []string{"quit"}, Summary: "quit bdash",
	}, (*App).cmdQuit)
	a.register(command.Spec{
		Name: "go", Usage: "<id>", Summary: "jump to an issue", Min: 1, Max: 1,
		Help: "Makes the issue current and pushes where you were on the back stack; Backspace returns. Typing just an ID does the same.",
		Args: func(prev []string, prefix string) []string {
			if len(prev) > 0 {
				return nil
			}
			return a.idsWith(prefix, maxIDCandidate)
		},
	}, (*App).cmdGo)
	a.registerWrites()
	a.registerMemories()
	a.registerJournal()
	a.registerClipboard()
}

// choose completes an argument from a fixed list.
func choose(values []string) func([]string, string) []string {
	return func(prev []string, prefix string) []string {
		if len(prev) != 0 {
			return nil
		}
		return filterPrefix(values, prefix)
	}
}

func filterPrefix(values []string, prefix string) []string {
	var out []string
	for _, v := range values {
		if strings.HasPrefix(strings.ToLower(v), strings.ToLower(prefix)) {
			out = append(out, v)
		}
	}
	return out
}

// runCommandLine runs what the command bar holds. A command that fails keeps
// the bar open with the error under it.
func (a *App) runCommandLine() tea.Cmd {
	b := a.bar
	text := strings.TrimSpace(b.in.Text())
	res := a.cmds.Parse(text)
	switch {
	case res.Err != nil:
		b.err = res.Err
		return nil
	case res.Call == nil && res.ID == "":
		return a.closeBar()
	}
	rec := b.rec
	a.bar = nil
	a.sess.Remove(state.LayerBar)
	var cmd tea.Cmd
	var err error
	if res.Call != nil {
		cmd, err = a.run[res.Call.Spec.Name](a, res.Call.Args)
	} else if id, ok := a.resolveID(res.ID); ok && a.jumpTo(id) {
		cmd = nil
	} else {
		err = argError(command.Word{Pos: res.IDPos}, "no command or issue %q", res.ID)
	}
	if err != nil {
		var ce *command.Error
		if !errors.As(err, &ce) {
			ce = &command.Error{Msg: err.Error()}
		}
		b.err = ce
		a.bar = b
		a.sess.Push(state.LayerBar)
		return cmd
	}
	return tea.Batch(cmd, a.remember(commandMark, text, rec))
}

func (a *App) cmdView(args []command.Word) (tea.Cmd, error) {
	name := strings.ToLower(args[0].Text)
	n := -1
	if i, err := strconv.Atoi(name); err == nil {
		n = i - 1
	}
	for i, v := range ViewNames {
		if strings.ToLower(v) == name {
			n = i
		}
	}
	if n < 0 || n >= len(ViewNames) {
		return nil, argError(args[0], "no view %q", args[0].Text)
	}
	if a.views[n] == nil {
		return nil, argError(args[0], "%s is not available yet", ViewNames[n])
	}
	a.switchTo(n, "")
	return nil, nil
}

func (a *App) cmdTheme(args []command.Word) (tea.Cmd, error) {
	t, ok := theme.Lookup(args[0].Text)
	if !ok {
		return nil, argError(args[0], "no theme %q; try %s", args[0].Text, strings.Join(theme.Names(), ", "))
	}
	c := a.choices
	c.Theme = t.Name
	return a.setChoices(c), nil
}

func (a *App) cmdGlyphs(args []command.Word) (tea.Cmd, error) {
	tier := strings.ToLower(args[0].Text)
	if !contains(config.GlyphTiers, tier) {
		return nil, argError(args[0], "no glyph set %q; try %s", args[0].Text, strings.Join(config.GlyphTiers, ", "))
	}
	c := a.choices
	c.Glyphs = tier
	return a.setChoices(c), nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func (a *App) cmdClear([]command.Word) (tea.Cmd, error) {
	a.applyScope(a.clearedScope())
	return nil, nil
}

func (a *App) cmdRefresh([]command.Word) (tea.Cmd, error) { return a.retry(), nil }

func (a *App) cmdQuit([]command.Word) (tea.Cmd, error) { return a.quit(), nil }

func (a *App) cmdGo(args []command.Word) (tea.Cmd, error) {
	id, ok := a.resolveID(args[0].Text)
	if !ok || !a.jumpTo(id) {
		return nil, argError(args[0], "no issue %q", args[0].Text)
	}
	return nil, nil
}

func (a *App) cmdHelp(args []command.Word) (tea.Cmd, error) {
	d := &helpDialog{a: a, under: a.underBar(), cmds: a.cmds.Specs()}
	if len(args) == 1 {
		s, ok := a.cmds.Lookup(strings.TrimPrefix(args[0].Text, ":"))
		if !ok {
			return nil, argError(args[0], "no command %q", args[0].Text)
		}
		d.cmds, d.only = []command.Spec{s}, true
	}
	a.pushDialog(d)
	return nil, nil
}

// setChoices previews the appearance choices and saves those that changed.
func (a *App) setChoices(c dialog.Choices) tea.Cmd {
	changed := c.Changed(a.choices)
	a.preview(c)
	a.choices = c
	return a.persist(changed)
}

// underBar is the key context that has the keys once the bar is gone.
func (a *App) underBar() keys.Context {
	if a.sess.Has(state.LayerDetailFocus) || a.sess.Has(state.LayerDetail) {
		return keys.Panel
	}
	return a.view().Context()
}
