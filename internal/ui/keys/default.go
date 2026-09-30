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
		{Keys: []string{"/"}, Action: OpenSearch, Desc: "search", Hint: 5},
		{Keys: []string{"f"}, Action: OpenFilter, Desc: "filter", Hint: 7},
		{Keys: []string{":"}, Action: OpenCommand, Desc: "command bar", Hint: 7, HintDesc: "command"},
		{Keys: []string{"N"}, Action: Notifications, Desc: "notifications"},
		{Keys: []string{"ctrl+p"}, Action: OpenPicker, Label: "Ctrl+P", Desc: "jump to an issue", Hint: 7, HintDesc: "jump"},
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
		{Keys: []string{"e"}, Action: Edit, Desc: "edit"},
		{Keys: []string{"n"}, Action: New, Desc: "new issue"},
		{Keys: []string{"s"}, Action: ChangeStatus, Desc: "status"},
		{Keys: []string{"p"}, Action: ChangePriority, Desc: "priority"},
		{Keys: []string{"a"}, Action: ChangeAssignee, Desc: "assignee"},
		{Keys: []string{"#"}, Action: ChangeLabels, Desc: "labels"},
		{Keys: []string{"c"}, Action: CloseReopen, Desc: "close or reopen with a reason"},
		{Keys: []string{"y"}, Action: CopyID, Desc: "copy ID"},
		{Keys: []string{"x"}, Action: Export, Desc: "export"},
		{Keys: []string{"<"}, Action: MoveLeft, Desc: "Kanban: move the card back"},
		{Keys: []string{">"}, Action: MoveRight, Desc: "Kanban: move the card on"},
		{Keys: []string{"o"}, Action: "toggle.closed", Desc: "toggle closed", Later: true},
		{Keys: []string{"+"}, Action: "widen", Desc: "more", Later: true},
		{Keys: []string{"-"}, Action: "narrow", Desc: "less", Later: true},
	}
	m.by[Memories] = []Binding{
		{Keys: []string{"n"}, Action: MemoryNew, Desc: "new memory", Hint: 2, HintDesc: "new"},
		{Keys: []string{"e"}, Action: MemoryEdit, Desc: "edit memory", Hint: 4, HintDesc: "edit"},
		{Keys: []string{"d"}, Action: MemoryForget, Desc: "forget memory, or the marked ones", Hint: 5, HintDesc: "forget"},
		{Keys: []string{"y"}, Action: MemoryCopy, Desc: "copy the content", Hint: 6, HintDesc: "copy"},
		{Keys: []string{"enter"}, Action: Open, Label: "Enter", Desc: "open the full-screen preview below 80 columns"},
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
		{Keys: []string{"y"}, Action: CopyID, Desc: "copy ID"},
		{Keys: []string{"x"}, Action: Export, Desc: "export"},
	}
	m.by[MemoryPreview] = []Binding{
		{Keys: []string{"j", "down"}, Action: NavDown, Label: "j/Down", Desc: "scroll down", Hint: 3, HintKey: "j/k", HintDesc: "scroll"},
		{Keys: []string{"k", "up"}, Action: NavUp, Label: "k/Up", Desc: "scroll up"},
		{Keys: []string{"g g", "home"}, Action: NavFirst, Label: "gg", Desc: "top"},
		{Keys: []string{"G", "end"}, Action: NavLast, Label: "G", Desc: "bottom"},
		{Keys: []string{"ctrl+d"}, Action: NavHalfDown, Label: "Ctrl+D", Desc: "half page down"},
		{Keys: []string{"ctrl+u"}, Action: NavHalfUp, Label: "Ctrl+U", Desc: "half page up"},
		{Keys: []string{"pgdown"}, Action: NavPageDown, Label: "PgDn", Desc: "page down"},
		{Keys: []string{"pgup"}, Action: NavPageUp, Label: "PgUp", Desc: "page up"},
		{Keys: []string{"m"}, Action: Markdown, Desc: "markdown or source", Hint: 4},
		{Keys: []string{"y"}, Action: MemoryCopy, Desc: "copy the content", Hint: 5, HintDesc: "copy"},
	}
	m.by[Bar] = []Binding{
		{Keys: []string{"enter"}, Action: BarAccept, Label: "Enter", Desc: "keep the search and close", Hint: 1, HintDesc: "keep"},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "close, keeping the search", Hint: 2, HintDesc: "close"},
		{Keys: []string{"tab"}, Action: BarComplete, Label: "Tab", Desc: "complete a facet, value or ID", Hint: 3, HintDesc: "complete"},
		{Keys: []string{"shift+tab"}, Action: BarCompleteBack, Label: "Shift+Tab", Desc: "complete backwards"},
		{Keys: []string{"ctrl+n"}, Action: BarIssueNext, Label: "Ctrl+N", Desc: "next issue behind the bar", Hint: 4, HintKey: "Ctrl+N/P", HintDesc: "issue"},
		{Keys: []string{"ctrl+p"}, Action: BarIssuePrev, Label: "Ctrl+P", Desc: "previous issue behind the bar"},
		{Keys: []string{"down"}, Action: BarDown, Label: "Down", Desc: "next issue, or a newer search while browsing history", Hint: 5, HintKey: "Up/Dn", HintDesc: "issue, history"},
		{Keys: []string{"up"}, Action: BarUp, Label: "Up", Desc: "previous issue, or an older search on an empty bar"},
	}
	m.by[BarCommand] = []Binding{
		{Keys: []string{"enter"}, Action: BarAccept, Label: "Enter", Desc: "run the command", Hint: 1, HintDesc: "run"},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "close", Hint: 2},
		{Keys: []string{"tab"}, Action: BarComplete, Label: "Tab", Desc: "complete", Hint: 3},
		{Keys: []string{"shift+tab"}, Action: BarCompleteBack, Label: "Shift+Tab", Desc: "complete backwards"},
		{Keys: []string{"up"}, Action: BarUp, Label: "Up", Desc: "older command", Hint: 4, HintKey: "Up/Dn", HintDesc: "history"},
		{Keys: []string{"down"}, Action: BarDown, Label: "Down", Desc: "newer command"},
		{Keys: []string{"ctrl+n"}, Action: BarIssueNext, Label: "Ctrl+N", Desc: "next issue behind the bar"},
		{Keys: []string{"ctrl+p"}, Action: BarIssuePrev, Label: "Ctrl+P", Desc: "previous issue behind the bar"},
	}
	m.by[BarFilter] = []Binding{
		{Keys: []string{"enter"}, Action: BarAccept, Label: "Enter", Desc: "keep the filter and close", Hint: 1, HintDesc: "keep"},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "close, keeping the filter", Hint: 2, HintDesc: "close"},
		{Keys: []string{"space"}, Action: BarToggle, Label: "Space", Desc: "toggle the option", Hint: 3, HintDesc: "toggle"},
		{Keys: []string{"tab"}, Action: BarColumnNext, Label: "Tab", Desc: "next column", Hint: 4, HintKey: "Tab", HintDesc: "column"},
		{Keys: []string{"shift+tab"}, Action: BarColumnPrev, Label: "Shift+Tab", Desc: "previous column"},
		{Keys: []string{"down"}, Action: BarDown, Label: "Down", Desc: "next option", Hint: 5, HintKey: "Up/Dn", HintDesc: "option"},
		{Keys: []string{"up"}, Action: BarUp, Label: "Up", Desc: "previous option"},
		{Keys: []string{"ctrl+n"}, Action: BarIssueNext, Label: "Ctrl+N", Desc: "next issue behind the bar"},
		{Keys: []string{"ctrl+p"}, Action: BarIssuePrev, Label: "Ctrl+P", Desc: "previous issue behind the bar"},
	}
	m.by[Form] = []Binding{
		{Keys: []string{"ctrl+s"}, Action: Apply, Label: "Ctrl+S", Desc: "submit", Hint: 1, HintDesc: "save"},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "close", Hint: 2},
		{Keys: []string{"tab"}, Action: Next, Label: "Tab", Desc: "next field", Hint: 3, HintKey: "Tab", HintDesc: "field"},
		{Keys: []string{"shift+tab"}, Action: Prev, Label: "Shift+Tab", Desc: "previous field"},
		{Keys: []string{"ctrl+e"}, Action: Editor, Label: "Ctrl+E", Desc: "edit the text in $VISUAL or $EDITOR", Hint: 4, HintDesc: "editor"},
		{Keys: []string{"ctrl+g"}, Action: FormError, Label: "Ctrl+G", Desc: "show or shorten the full error", Hint: 5, HintDesc: "error"},
		{Keys: []string{"ctrl+o"}, Action: KeepMine, Label: "Ctrl+O", Desc: "conflict: keep my value", Hint: 6, HintDesc: "mine"},
		{Keys: []string{"ctrl+t"}, Action: TakeTheirs, Label: "Ctrl+T", Desc: "conflict: take the changed value", Hint: 7, HintDesc: "theirs"},
		{Keys: []string{"ctrl+r"}, Action: Reload, Label: "Ctrl+R", Desc: "conflict: discard my edits and reload", Hint: 8, HintDesc: "reload"},
	}
	m.by[Confirm] = []Binding{
		{Keys: []string{"y"}, Action: Apply, Desc: "yes", Hint: 1},
		{Keys: []string{"n", "esc", "enter"}, Action: Close, Label: "n/Esc", Desc: "no", Hint: 2, HintKey: "n", HintDesc: "no"},
	}
	m.by[Overwrite] = []Binding{
		{Keys: []string{"o"}, Action: DoOverwrite, Desc: "overwrite", Hint: 1},
		{Keys: []string{"b"}, Action: KeepBoth, Desc: "keep both", Hint: 2},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "back", Hint: 3},
	}
	m.by[Journal] = []Binding{
		{Keys: []string{"y"}, Action: JournalEnable, Desc: "enable", Hint: 1},
		{Keys: []string{"n", "esc"}, Action: Close, Label: "n/Esc", Desc: "not now", Hint: 2, HintKey: "n", HintDesc: "not now"},
		{Keys: []string{"N"}, Action: JournalNever, Desc: "never for this workspace", Hint: 3, HintDesc: "never"},
	}
	m.by[Refused] = []Binding{
		{Keys: []string{"enter"}, Action: Open, Label: "Enter", Desc: "jump to the selected blocker", Hint: 1, HintDesc: "jump"},
		{Keys: []string{"down", "j"}, Action: NavDown, Label: "Down/j", Desc: "next blocker", Hint: 3, HintKey: "Up/Dn", HintDesc: "blocker"},
		{Keys: []string{"up", "k"}, Action: NavUp, Label: "Up/k", Desc: "previous blocker"},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "close", Hint: 2},
	}
	m.by[Picker] = []Binding{
		{Keys: []string{"enter"}, Action: Apply, Label: "Enter", Desc: "pick", Hint: 1},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "close", Hint: 2},
		{Keys: []string{"down", "ctrl+n"}, Action: NavDown, Label: "Down/Ctrl+N", Desc: "next", Hint: 3, HintKey: "Up/Dn", HintDesc: "move"},
		{Keys: []string{"up", "ctrl+p"}, Action: NavUp, Label: "Up/Ctrl+P", Desc: "previous"},
		{Keys: []string{"pgdown"}, Action: NavPageDown, Label: "PgDn", Desc: "page down"},
		{Keys: []string{"pgup"}, Action: NavPageUp, Label: "PgUp", Desc: "page up"},
		{Keys: []string{"tab"}, Action: PickerMark, Label: "Tab", Desc: "mark for several", Hint: 4, HintDesc: "mark"},
		{Keys: []string{"ctrl+t"}, Action: PickerClosed, Label: "Ctrl+T", Desc: "show or hide closed issues", Hint: 5, HintDesc: "closed"},
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
	m.by[Notify] = []Binding{
		{Keys: []string{"enter"}, Action: Apply, Label: "Enter", Desc: "save", Hint: 1},
		{Keys: []string{"esc"}, Action: Close, Label: "Esc", Desc: "cancel", Hint: 2},
		{Keys: []string{"space"}, Action: Toggle, Label: "Space", Desc: "toggle", Hint: 3},
		{Keys: []string{"down", "tab", "j"}, Action: NavDown, Label: "Down/Tab/j", Desc: "next row", Hint: 4, HintKey: "Up/Dn", HintDesc: "row"},
		{Keys: []string{"up", "shift+tab", "k"}, Action: NavUp, Label: "Up/Shift+Tab/k", Desc: "previous row"},
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
