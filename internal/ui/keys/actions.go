package keys

// Actions of the shell. Views add their own.
const (
	QuitForce      Action = "quit.force"
	Quit           Action = "quit"
	OpenHelp       Action = "help"
	OpenAppearance Action = "appearance"
	Refresh        Action = "refresh"
	SwitchView     Action = "view.switch"
	Back           Action = "back"
	Close          Action = "close"
	OpenDetails    Action = "details"

	NavDown     Action = "nav.down"
	NavUp       Action = "nav.up"
	NavFirst    Action = "nav.first"
	NavLast     Action = "nav.last"
	NavHalfDown Action = "nav.halfdown"
	NavHalfUp   Action = "nav.halfup"
	NavPageDown Action = "nav.pagedown"
	NavPageUp   Action = "nav.pageup"
	Mark        Action = "mark"

	HelpFilter Action = "help.filter"

	Open         Action = "open"
	Left         Action = "left"
	Right        Action = "right"
	FoldAll      Action = "fold.all"
	UnfoldAll    Action = "unfold.all"
	FocusNext    Action = "focus.next"
	DetailToggle Action = "detail.toggle"
	SectionNext  Action = "section.next"
	SectionPrev  Action = "section.prev"
	Jump         Action = "jump"
	SectionsAll  Action = "sections.all"
	Markdown     Action = "markdown"

	ScrollLeft      Action = "scroll.left"
	ScrollRight     Action = "scroll.right"
	ScrollHalfLeft  Action = "scroll.halfleft"
	ScrollHalfRight Action = "scroll.halfright"
	DepthMore       Action = "depth.more"
	DepthLess       Action = "depth.less"
	ToggleIsolated  Action = "graph.isolated"

	Prev  Action = "prev"
	Next  Action = "next"
	Apply Action = "apply"

	Retry  Action = "retry"
	Copy   Action = "copy"
	PickUp Action = "pick.up"
	PickDn Action = "pick.down"
)
