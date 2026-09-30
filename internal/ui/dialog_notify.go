package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// notifyDialog switches notifications on or off and picks the kinds of change
// they announce. Row 0 is the switch, the rows after it are the kinds.
type notifyDialog struct {
	a     *App
	on    bool
	kinds map[string]bool
	row   int
	scroller
}

func (a *App) newNotifyDialog() *notifyDialog {
	d := &notifyDialog{a: a, on: a.notifyOn, kinds: map[string]bool{}}
	for _, n := range a.notifyNames {
		d.kinds[n] = true
	}
	return d
}

func (*notifyDialog) Context() keys.Context { return keys.Notify }

func (d *notifyDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	switch act { //nolint:exhaustive // only the actions of the notifications context arrive
	case keys.Close:
		return nil, true
	case keys.NavDown:
		d.row = min(d.row+1, len(model.KindNames()))
	case keys.NavUp:
		d.row = max(d.row-1, 0)
	case keys.Toggle:
		if d.row == 0 {
			d.on = !d.on
			break
		}
		name := model.KindNames()[d.row-1]
		d.kinds[name] = !d.kinds[name]
	case keys.Apply:
		return d.a.setNotify(d.on, d.chosen()), true
	}
	return nil, false
}

func (d *notifyDialog) chosen() []string {
	var out []string
	for _, n := range model.KindNames() {
		if d.kinds[n] {
			out = append(out, n)
		}
	}
	return out
}

func (*notifyDialog) Update(tea.Msg) tea.Cmd { return nil }

func (d *notifyDialog) Frame(l look.Look, w, h int) dialog.Frame {
	names := model.KindNames()
	line := func(i int, on bool, label string) string {
		cursor, box := "  ", "[ ] "
		if i == d.row {
			cursor = l.Paint(theme.Primary, "> ")
		}
		if on {
			box = "[x] "
		}
		style := theme.Text
		if i == d.row {
			style = theme.Strong
		}
		return cursor + l.Paint(style, box+label)
	}
	state := "off"
	if d.on {
		state = "on"
	}
	body := []string{line(0, d.on, "Notifications "+state), ""}
	body = append(body, l.Paint(theme.Primary, "Tell me when an issue"))
	first := len(body)
	for i, n := range names {
		body = append(body, line(i+1, d.kinds[n], model.Kind(n).Label()))
	}
	body = append(body, "", l.Paint(theme.Dim, "Your own changes never notify. Method: "+d.a.notifyMethod))
	at := 0
	if d.row > 0 {
		at = first + d.row - 1
	}
	d.clamp(w, h, len(body))
	if at < d.at {
		d.at = at
	}
	if at >= d.at+d.page {
		d.at = at - d.page + 1
	}
	d.at = min(max(d.at, 0), dialog.MaxScroll(w, h, len(body)))
	aside := fmt.Sprintf("%d kinds", len(d.chosen()))
	return dialog.Frame{Title: "Notifications", Aside: aside, Hints: d.a.hintsFor(keys.Notify), Body: body, Scroll: d.at}
}
