package keys

// Default returns bdash's key map. Bindings marked Later belong to features
// that are not built yet; they reserve their keys so that conflicts
// show up early.
func Default() *Map {
	var m Map
	m.by[Always] = []Binding{
		{Keys: []string{"ctrl+c"}, Action: QuitForce, Desc: "quit"},
	}
	m.by[Global] = []Binding{
		{Keys: []string{"?"}, Action: OpenHelp, Desc: "help", Hint: 1},
		{Keys: []string{"1", "2", "3", "4", "5", "6"}, Action: SwitchView, Label: "1-6", Desc: "switch view", Hint: 4},
		{Keys: []string{"t"}, Action: OpenAppearance, Desc: "appearance", Hint: 5},
		{Keys: []string{"r"}, Action: Refresh, Desc: "refresh", Hint: 6},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "close, then clear marks and scope"},
		{Keys: []string{"backspace", "ctrl+o"}, Action: Back, Label: "Backspace", Desc: "back to the previous issue"},
		{Keys: []string{"!"}, Action: OpenDetails, Desc: "error details"},
		{Keys: []string{":"}, Action: "bar.command", Desc: "command bar", Later: true},
		{Keys: []string{"/"}, Action: "bar.search", Desc: "search", Later: true},
		{Keys: []string{"f"}, Action: "bar.filter", Desc: "filter", Later: true},
		{Keys: []string{"N"}, Action: "notifications", Desc: "notifications", Later: true},
		{Keys: []string{"ctrl+p"}, Action: "picker", Desc: "jump to issue", Later: true},
		{Keys: []string{"tab"}, Action: FocusNext, Label: "Tab", Desc: "focus list or detail"},
		{Keys: []string{"D"}, Action: DetailToggle, Desc: "show or hide the detail panel"},
	}
	m.by[View] = []Binding{
		{Keys: []string{"q"}, Action: Quit, Desc: "quit", Hint: 2},
		{Keys: []string{"j", "down"}, Action: NavDown, Label: "j/Down", Desc: "move down", Hint: 3, HintKey: "j/k", HintDesc: "move"},
		{Keys: []string{"k", "up"}, Action: NavUp, Label: "k/Up", Desc: "move up"},
		{Keys: []string{"g g", "home"}, Action: NavFirst, Label: "gg", Desc: "first"},
		{Keys: []string{"G", "end"}, Action: NavLast, Label: "G", Desc: "last"},
		{Keys: []string{"ctrl+d"}, Action: NavHalfDown, Label: "Ctrl+D", Desc: "half page down"},
		{Keys: []string{"ctrl+u"}, Action: NavHalfUp, Label: "Ctrl+U", Desc: "half page up"},
		{Keys: []string{"pgdown"}, Action: NavPageDown, Label: "PgDn", Desc: "page down"},
		{Keys: []string{"pgup"}, Action: NavPageUp, Label: "PgUp", Desc: "page up"},
		{Keys: []string{"space"}, Action: Mark, Label: "Space", Desc: "mark", Hint: 3},
		{Keys: []string{"h", "left"}, Action: Left, Label: "h/Left", Desc: "column, collapse"},
		{Keys: []string{"l", "right"}, Action: Right, Label: "l/Right", Desc: "column, expand"},
		{Keys: []string{"z h"}, Action: ScrollLeft, Label: "zh", Desc: "scroll left"},
		{Keys: []string{"z l"}, Action: ScrollRight, Label: "zl", Desc: "scroll right"},
		{Keys: []string{"z H"}, Action: ScrollHalfLeft, Label: "zH", Desc: "scroll half a screen left"},
		{Keys: []string{"z L"}, Action: ScrollHalfRight, Label: "zL", Desc: "scroll half a screen right"},
		{Keys: []string{"enter"}, Action: Open, Label: "Enter", Desc: "open the detail, fold row", Hint: 4, HintDesc: "open"},
		{Keys: []string{"e"}, Action: "edit", Desc: "edit", Later: true},
		{Keys: []string{"n"}, Action: "new", Desc: "new issue", Later: true},
		{Keys: []string{"s"}, Action: "change.status", Desc: "status", Later: true},
		{Keys: []string{"p"}, Action: "change.priority", Desc: "priority", Later: true},
		{Keys: []string{"a"}, Action: "change.assignee", Desc: "assignee", Later: true},
		{Keys: []string{"#"}, Action: "change.labels", Desc: "labels", Later: true},
		{Keys: []string{"c"}, Action: "close.reopen", Desc: "close or reopen", Later: true},
		{Keys: []string{"y"}, Action: "copy.id", Desc: "copy ID", Later: true},
		{Keys: []string{"x"}, Action: "export", Desc: "export", Later: true},
		{Keys: []string{"<"}, Action: "move.left", Desc: "move card left", Later: true},
		{Keys: []string{">"}, Action: "move.right", Desc: "move card right", Later: true},
		{Keys: []string{"o"}, Action: "toggle.closed", Desc: "toggle closed", Later: true},
		{Keys: []string{"+"}, Action: "widen", Desc: "more", Later: true},
		{Keys: []string{"-"}, Action: "narrow", Desc: "less", Later: true},
	}
	m.by[Memories] = []Binding{
		{Keys: []string{"n"}, Action: "memory.new", Desc: "new memory", Later: true},
		{Keys: []string{"e"}, Action: "memory.edit", Desc: "edit memory", Later: true},
		{Keys: []string{"d"}, Action: "memory.forget", Desc: "forget memory", Later: true},
	}
	m.by[Tree] = []Binding{
		{Keys: []string{"z M"}, Action: FoldAll, Label: "zM", Desc: "fold all"},
		{Keys: []string{"z R"}, Action: UnfoldAll, Label: "zR", Desc: "unfold all"},
	}
	m.by[Overview] = []Binding{
		{Keys: []string{"enter"}, Action: Open, Label: "Enter", Desc: "open the detail, or the view that owns the row", Hint: 4, HintDesc: "open"},
	}
	m.by[Graph] = []Binding{
		{Keys: []string{"enter"}, Action: Open, Label: "Enter", Desc: "focus the graph on the issue; on the focus, open the detail", Hint: 4, HintDesc: "focus"},
		{Keys: []string{"z M"}, Action: FoldAll, Label: "zM", Desc: "fold all"},
		{Keys: []string{"z R"}, Action: UnfoldAll, Label: "zR", Desc: "unfold all"},
		{Keys: []string{"i"}, Action: ToggleIsolated, Desc: "show or hide issues without dependencies"},
		{Keys: []string{"+"}, Action: DepthMore, Desc: "deeper focus graph"},
		{Keys: []string{"-"}, Action: DepthLess, Desc: "shallower focus graph"},
	}
	m.by[Panel] = []Binding{
		{Keys: []string{"j", "down"}, Action: NavDown, Label: "j/Down", Desc: "scroll down", Hint: 3, HintKey: "j/k", HintDesc: "scroll"},
		{Keys: []string{"k", "up"}, Action: NavUp, Label: "k/Up", Desc: "scroll up"},
		{Keys: []string{"g g", "home"}, Action: NavFirst, Label: "gg", Desc: "top"},
		{Keys: []string{"G", "end"}, Action: NavLast, Label: "G", Desc: "bottom"},
		{Keys: []string{"ctrl+d"}, Action: NavHalfDown, Label: "Ctrl+D", Desc: "half page down"},
		{Keys: []string{"ctrl+u"}, Action: NavHalfUp, Label: "Ctrl+U", Desc: "half page up"},
		{Keys: []string{"pgdown"}, Action: NavPageDown, Label: "PgDn", Desc: "page down"},
		{Keys: []string{"pgup"}, Action: NavPageUp, Label: "PgUp", Desc: "page up"},
		{Keys: []string{"]"}, Action: SectionNext, Desc: "next section", Hint: 4, HintKey: "]/[", HintDesc: "section"},
		{Keys: []string{"["}, Action: SectionPrev, Desc: "previous section"},
		{Keys: []string{"h", "left"}, Action: Left, Label: "h/Left", Desc: "close section"},
		{Keys: []string{"l", "right"}, Action: Right, Label: "l/Right", Desc: "open section, show all lines"},
		{Keys: []string{"enter"}, Action: Jump, Label: "Enter", Desc: "jump to the issue on the row; open or close a section", Hint: 5, HintDesc: "jump"},
		{Keys: []string{"o"}, Action: SectionsAll, Desc: "open or close all sections"},
		{Keys: []string{"+"}, Action: DepthMore, Desc: "deeper focus graph"},
		{Keys: []string{"-"}, Action: DepthLess, Desc: "shallower focus graph"},
		{Keys: []string{"m"}, Action: Markdown, Desc: "markdown or source", Hint: 6},
		{Keys: []string{"y"}, Action: "copy.id", Desc: "copy ID", Later: true},
	}
	m.by[Bar] = []Binding{
		{Keys: []string{"enter"}, Action: "bar.accept", Label: "Enter", Desc: "accept", Later: true},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "close", Later: true},
		{Keys: []string{"up", "ctrl+p"}, Action: "bar.up", Label: "Up", Desc: "previous", Later: true},
		{Keys: []string{"down", "ctrl+n"}, Action: "bar.down", Label: "Down", Desc: "next", Later: true},
		{Keys: []string{"tab"}, Action: "bar.complete", Label: "Tab", Desc: "complete", Later: true},
	}
	m.by[Form] = []Binding{
		{Keys: []string{"tab"}, Action: Next, Label: "Tab", Desc: "next field", Later: true},
		{Keys: []string{"shift+tab"}, Action: Prev, Label: "Shift+Tab", Desc: "previous field", Later: true},
		{Keys: []string{"ctrl+s"}, Action: Apply, Label: "Ctrl+S", Desc: "submit", Later: true},
		{Keys: []string{"ctrl+e"}, Action: "editor", Label: "Ctrl+E", Desc: "editor", Later: true},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "close", Later: true},
	}
	m.by[Help] = []Binding{
		{Keys: []string{"esc", "?"}, Action: Close, Label: "Esc", Desc: "close", Hint: 1},
		{Keys: []string{"j", "down"}, Action: NavDown, Label: "j/Down", Desc: "scroll down", Hint: 2, HintKey: "j/k", HintDesc: "scroll"},
		{Keys: []string{"k", "up"}, Action: NavUp, Label: "k/Up", Desc: "scroll up"},
		{Keys: []string{"pgdown"}, Action: NavPageDown, Label: "PgDn", Desc: "page down"},
		{Keys: []string{"pgup"}, Action: NavPageUp, Label: "PgUp", Desc: "page up"},
		{Keys: []string{"/"}, Action: HelpFilter, Desc: "filter", Hint: 3},
	}
	m.by[Appearance] = []Binding{
		{Keys: []string{"enter"}, Action: Apply, Label: "Enter", Desc: "apply", Hint: 1},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "revert", Hint: 2},
		{Keys: []string{"down", "tab", "j"}, Action: NavDown, Label: "Down/Tab/j", Desc: "next row", Hint: 3, HintKey: "Up/Dn", HintDesc: "row"},
		{Keys: []string{"up", "shift+tab", "k"}, Action: NavUp, Label: "Up/Shift+Tab/k", Desc: "previous row"},
		{Keys: []string{"right", "l"}, Action: Next, Label: "Right/l", Desc: "next value", Hint: 4, HintKey: "Left/Right", HintDesc: "value"},
		{Keys: []string{"left", "h"}, Action: Prev, Label: "Left/h", Desc: "previous value"},
	}
	m.by[Details] = []Binding{
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "close", Hint: 1},
		{Keys: []string{"j", "down"}, Action: NavDown, Label: "j/Down", Desc: "scroll down", Hint: 4, HintKey: "j/k", HintDesc: "scroll"},
		{Keys: []string{"k", "up"}, Action: NavUp, Label: "k/Up", Desc: "scroll up"},
		{Keys: []string{"pgdown"}, Action: NavPageDown, Label: "PgDn", Desc: "page down"},
		{Keys: []string{"pgup"}, Action: NavPageUp, Label: "PgUp", Desc: "page up"},
		{Keys: []string{"r"}, Action: Retry, Desc: "retry now", Hint: 2},
		{Keys: []string{"y"}, Action: Copy, Desc: "copy error", Hint: 3},
	}
	m.by[Startup] = []Binding{
		{Keys: []string{"q"}, Action: Quit, Desc: "quit", Hint: 1},
		{Keys: []string{"r"}, Action: Retry, Desc: "recheck now", Hint: 2},
		{Keys: []string{"down", "j"}, Action: PickDn, Label: "Down/j", Desc: "next fix", Hint: 3, HintKey: "Up/Dn", HintDesc: "pick"},
		{Keys: []string{"up", "k"}, Action: PickUp, Label: "Up/k", Desc: "previous fix"},
		{Keys: []string{"y"}, Action: Copy, Desc: "copy fix", Hint: 4},
	}
	m.by[TooSmall] = []Binding{
		{Keys: []string{"q"}, Action: Quit, Desc: "quit", Hint: 1},
	}
	return &m
}
