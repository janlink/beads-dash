package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/command"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// Dialog is an overlay on the shell. Dialogs stack; the top one owns the keys
// and the highlight clock is paused while any is open.
type Dialog interface {
	// Context is the key context of the dialog.
	Context() keys.Context
	// Handle acts on a key-map action and reports whether the dialog closed.
	// keys.Close is the cancel path: it is also how the shell closes a dialog
	// that a fatal error covers.
	Handle(a keys.Action) (cmd tea.Cmd, closed bool)
	// Update receives every message that is not a key press or is a key press
	// the dialog asked for with RawKeys.
	Update(msg tea.Msg) tea.Cmd
	// Frame draws the dialog for a terminal of w by h cells.
	Frame(l look.Look, w, h int) dialog.Frame
}

// rawKeys is implemented by dialogs that take text input: while it reports
// true the shell hands them key presses unresolved.
type rawKeys interface{ RawKeys() bool }

// scroller is the scroll state of a dialog body.
type scroller struct {
	at, page int
}

func (s *scroller) handle(act keys.Action) {
	page := max(s.page, 1)
	switch act { //nolint:exhaustive // only the scrolling actions matter here
	case keys.NavDown:
		s.at++
	case keys.NavUp:
		s.at--
	case keys.NavPageDown:
		s.at += page
	case keys.NavPageUp:
		s.at -= page
	}
	s.at = max(s.at, 0)
}

func (s *scroller) clamp(w, h, n int) {
	s.page = dialog.Page(w, h, n)
	s.at = min(max(s.at, 0), dialog.MaxScroll(w, h, n))
}

type helpDialog struct {
	a         *App
	under     keys.Context
	filter    string
	filtering bool
	// cmds are the commands listed after the keys; only drops the keys.
	cmds []command.Spec
	only bool
	scroller
}

func (*helpDialog) Context() keys.Context { return keys.Help }

func (d *helpDialog) RawKeys() bool { return d.filtering }

func (d *helpDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	switch act { //nolint:exhaustive // only the actions of the help context arrive
	case keys.Close:
		return nil, true
	case keys.HelpFilter:
		d.filtering = true
	default:
		d.handle(act)
	}
	return nil, false
}

func (d *helpDialog) Update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch s := k.String(); s {
	case "esc":
		d.filter, d.filtering = "", false
	case "enter":
		d.filtering = false
	case "backspace":
		if r := []rune(d.filter); len(r) > 0 {
			d.filter = string(r[:len(r)-1])
		}
	case "space":
		d.filter += " "
	default:
		if r := []rune(s); len(r) == 1 {
			d.filter += s
		}
	}
	d.at = 0
	return nil
}

func (d *helpDialog) Frame(l look.Look, w, h int) dialog.Frame {
	var body []string
	shown := 0
	if d.filtering || d.filter != "" {
		cursor := ""
		if d.filtering {
			cursor = "_"
		}
		body = append(body, l.Paint(theme.Primary, "/ ")+l.Paint(theme.Strong, d.filter+cursor), "")
	}
	needle := strings.ToLower(d.filter)
	var secs []keys.Section
	if !d.only {
		secs = d.a.km.Sections(d.under)
	}
	for _, sec := range secs {
		var lines []string
		keyW := 0
		for _, b := range sec.Bindings {
			keyW = max(keyW, ansi.StringWidth(b.Text()))
		}
		for _, b := range sec.Bindings {
			if needle != "" && !strings.Contains(strings.ToLower(b.Text()+" "+b.Desc), needle) {
				continue
			}
			pad := strings.Repeat(" ", keyW-ansi.StringWidth(b.Text()))
			lines = append(lines, "  "+l.Paint(theme.Strong, b.Text())+pad+"  "+l.Paint(theme.Text, b.Desc))
			shown++
		}
		if len(lines) == 0 {
			continue
		}
		body = append(body, l.Paint(theme.Primary, sec.Title))
		body = append(body, lines...)
		body = append(body, "")
	}
	if len(d.cmds) > 0 {
		body, shown = d.commandLines(l, needle, body, shown)
	}
	if shown == 0 {
		body = append(body, l.Paint(theme.Dim, "No key matches."))
	}
	hints := d.a.hintsFor(keys.Help)
	if d.filtering {
		hints = []keys.Hint{{Key: "Enter", Desc: "done"}, {Key: "Esc", Desc: "clear"}}
	}
	d.clamp(w, h, len(body))
	title := "Keys"
	if d.only {
		title = "Command"
	}
	return dialog.Frame{Title: title, Aside: fmt.Sprintf("%d shown", shown), Hints: hints, Body: body, Scroll: d.at}
}

type detailsDialog struct {
	a *App
	scroller
}

func (*detailsDialog) Context() keys.Context { return keys.Details }

func (d *detailsDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	switch act { //nolint:exhaustive // only the actions of the details context arrive
	case keys.Close:
		return nil, true
	case keys.Retry:
		return d.a.retry(), false
	case keys.Copy:
		r := d.a.errorReport()
		return d.a.copy("error details", strings.Join(r.Raw, "\n")), false
	default:
		d.handle(act)
	}
	return nil, false
}

func (*detailsDialog) Update(tea.Msg) tea.Cmd { return nil }

func (d *detailsDialog) Frame(l look.Look, w, h int) dialog.Frame {
	r := d.a.errorReport()
	var body []string
	body = append(body, l.Paint(theme.Strong, r.Title), "")
	if r.Ran != "" {
		body = append(body, l.Paint(theme.Primary, "bd said")+l.Paint(theme.Faint, "  "+r.Ran))
	}
	for _, line := range r.Raw {
		body = append(body, "  "+l.Paint(theme.Text, line))
	}
	if len(r.Fixes) > 0 {
		body = append(body, "", l.Paint(theme.Primary, "What to do"))
		for _, f := range r.Fixes {
			body = append(body, "  "+l.Paint(theme.Primary, "$ "+f.Cmd)+"  "+l.Paint(theme.Dim, "# "+f.Why))
		}
	}
	d.clamp(w, h, len(body))
	return dialog.Frame{Title: "Error details", Hints: d.a.hintsFor(keys.Details), Body: body, Scroll: d.at}
}

type appearanceDialog struct {
	a *App
	m *dialog.Appearance
}

func (a *App) newAppearanceDialog() *appearanceDialog {
	return &appearanceDialog{a: a, m: dialog.NewAppearance(a.choices, map[string]string{
		config.KeyTheme:      a.o.Settings.OverrideNote(config.KeyTheme),
		config.KeyBackground: a.o.Settings.OverrideNote(config.KeyBackground),
		config.KeyGlyphs:     a.o.Settings.OverrideNote(config.KeyGlyphs),
	})}
}

func (*appearanceDialog) Context() keys.Context { return keys.Appearance }

func (d *appearanceDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	a := d.a
	switch d.m.Handle(act) {
	case dialog.Preview:
		a.preview(d.m.Choices())
	case dialog.Apply:
		c := d.m.Choices()
		a.choices = c
		return a.persist(c.Changed(d.m.Original())), true
	case dialog.Cancel:
		a.preview(d.m.Original())
		return nil, true
	case dialog.Nothing:
	}
	return nil, false
}

func (*appearanceDialog) Update(tea.Msg) tea.Cmd { return nil }

func (d *appearanceDialog) Frame(l look.Look, _, _ int) dialog.Frame {
	return d.m.Frame(l, d.a.hintsFor(keys.Appearance))
}

func (d *helpDialog) commandLines(l look.Look, needle string, body []string, shown int) ([]string, int) {
	lineW := 0
	for _, s := range d.cmds {
		lineW = max(lineW, ansi.StringWidth(s.Line()))
	}
	var lines []string
	for _, s := range d.cmds {
		if needle != "" && !strings.Contains(strings.ToLower(s.Line()+" "+s.Summary), needle) {
			continue
		}
		pad := strings.Repeat(" ", lineW-ansi.StringWidth(s.Line()))
		lines = append(lines, "  "+l.Paint(theme.Strong, s.Line())+pad+"  "+l.Paint(theme.Text, s.Summary))
		if d.only {
			if len(s.Aliases) > 0 {
				lines = append(lines, "", "  "+l.Paint(theme.Dim, "also: :"+strings.Join(s.Aliases, ", :")))
			}
			if s.Help != "" {
				lines = append(lines, "", "  "+l.Paint(theme.Dim, s.Help))
			}
		}
		shown++
	}
	if len(lines) == 0 {
		return body, shown
	}
	body = append(body, l.Paint(theme.Primary, "Commands"))
	body = append(body, lines...)
	return append(body, ""), shown
}

// hinter is implemented by dialogs whose footer hints depend on their state.
type hinter interface{ hints() []keys.Hint }

// snapshotWatcher is implemented by dialogs that react to a new snapshot,
// such as a form watching the issue it edits.
type snapshotWatcher interface{ Snapshot() }

// notifyDialogs tells the open dialogs about a new snapshot.
func (a *App) notifyDialogs() {
	for _, d := range slices.Clone(a.dialogs) {
		if w, ok := d.(snapshotWatcher); ok {
			w.Snapshot()
		}
	}
}
