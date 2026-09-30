package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// confirmOpts configures a confirmation dialog.
type confirmOpts struct {
	Title string
	Lines []string
	// Yes runs when the user presses y; nothing else confirms.
	Yes func(a *App) tea.Cmd
}

// confirmDialog asks before something destructive. The default answer is no.
type confirmDialog struct {
	a *App
	o confirmOpts
}

func (a *App) newConfirm(o confirmOpts) *confirmDialog { return &confirmDialog{a: a, o: o} }

func (*confirmDialog) Context() keys.Context { return keys.Confirm }

func (d *confirmDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	if act == keys.Apply {
		return d.o.Yes(d.a), true
	}
	return nil, true
}

func (*confirmDialog) Update(tea.Msg) tea.Cmd { return nil }

func (d *confirmDialog) Frame(l look.Look, _, _ int) dialog.Frame {
	body := make([]string, 0, len(d.o.Lines)+2)
	for _, line := range d.o.Lines {
		body = append(body, l.Paint(theme.Text, line))
	}
	body = append(body, "", l.Paint(theme.Dim, "Press y to confirm. Anything else keeps things as they are."))
	return dialog.Frame{Title: d.o.Title, Hints: d.a.hintsFor(keys.Confirm), Body: body}
}
