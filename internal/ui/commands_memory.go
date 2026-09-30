package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/ui/command"
)

func (a *App) registerMemories() {
	a.register(command.Spec{
		Name: "remember", Usage: "[text]", Summary: "store a memory", Max: -1,
		Help: "Opens the memory dialog, with the text as content when given.",
	}, (*App).cmdRemember)
	a.register(command.Spec{
		Name: "forget", Usage: "<key>", Summary: "forget a memory", Min: 1, Max: 1,
		Help: "Forgets the memory with that key after a confirmation.",
		Args: func(prev []string, prefix string) []string {
			if len(prev) > 0 || a.mem == nil {
				return nil
			}
			return filterPrefix(memKeys(a.mem.list), prefix)
		},
	}, (*App).cmdForget)
}

func (a *App) registerJournal() {
	a.register(command.Spec{
		Name: "journal", Summary: "ask about the events journal",
		Help: "Opens the question whether bdash may turn on bd's events journal, so changes name their actor.",
	}, (*App).cmdJournal)
}

func (a *App) cmdRemember(args []command.Word) (tea.Cmd, error) {
	d := a.newMemoryDialog("", strings.Join(texts(args), " "), false)
	a.pushDialog(d)
	return nil, nil
}

func (a *App) cmdForget(args []command.Word) (tea.Cmd, error) {
	key := args[0].Text
	if _, ok := a.mem.get(key); !ok {
		if !a.mem.loaded {
			return nil, &command.Error{Msg: "the memories are not read yet; open the Memories view first"}
		}
		return nil, &command.Error{Msg: "no memory with the key " + key}
	}
	a.confirmForget([]string{key})
	return nil, nil
}

func (a *App) registerClipboard() {
	a.register(command.Spec{
		Name: "copy", Summary: "copy the current ID",
		Help: "Copies the ID of the current issue; in Memories, the content of the current memory.",
	}, (*App).cmdCopy)
	a.register(command.Spec{
		Name: "notify", Usage: "[on|off]", Summary: "switch notifications", Max: 1,
		Help: "Switches notifications on or off and saves the choice; without an argument, opens the notifications dialog.",
		Args: choose([]string{"on", "off"}),
	}, (*App).cmdNotify)
}

func (a *App) cmdCopy([]command.Word) (tea.Cmd, error) {
	if a.inMemories() {
		return a.copyMemory(), nil
	}
	return a.copyCurrentID(), nil
}

func (a *App) cmdNotify(args []command.Word) (tea.Cmd, error) {
	if len(args) == 0 {
		a.pushDialog(a.newNotifyDialog())
		return nil, nil
	}
	switch args[0].Text {
	case "on", "off":
		on := args[0].Text == "on"
		a.toast("notifications " + args[0].Text)
		return a.setNotify(on, a.notifyNames), nil
	}
	return nil, argError(args[0], "want on or off")
}
