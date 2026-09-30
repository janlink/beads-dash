package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/command"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// JournalStore keeps the answer to the events-journal question per
// workspace.
type JournalStore interface {
	Journal(workspace string) config.JournalAnswer
	SetJournal(workspace string, a config.JournalAnswer) error
}

// journalDialog asks once per workspace whether bdash may turn on bd's events
// journal, which names the actor of each change.
type journalDialog struct{ a *App }

func (*journalDialog) Context() keys.Context { return keys.Journal }

func (d *journalDialog) Handle(act keys.Action) (tea.Cmd, bool) {
	a := d.a
	switch act { //nolint:exhaustive // only the actions of the journal context arrive
	case keys.JournalEnable:
		return a.enableJournal(), true
	case keys.JournalNever:
		a.storeJournal(config.JournalDeclined)
		a.journalDeclined = true
		a.toast("the question stays off; :journal asks again")
		return nil, true
	case keys.Close:
		a.journalLater = true
		return nil, true
	}
	return nil, false
}

func (*journalDialog) Update(tea.Msg) tea.Cmd { return nil }

func (d *journalDialog) Frame(l look.Look, _, _ int) dialog.Frame {
	body := []string{
		l.Paint(theme.Text, "bd can keep an events journal that says who changed what."),
		l.Paint(theme.Text, "Without it bdash cannot name actors."),
		"",
		l.Paint(theme.Warning, "Enabling writes events-journal to the beads config.yaml, which is"),
		l.Paint(theme.Warning, "usually tracked in git, and applies to every writer of this workspace."),
		l.Paint(theme.Dim, "Undo with bd config unset events-journal."),
	}
	return dialog.Frame{Title: "Enable the events journal?", Hints: d.a.hintsFor(keys.Journal), Body: body}
}

func (a *App) journalStore() JournalStore {
	if a.o.Journal != nil {
		return a.o.Journal
	}
	if a.localJournal == nil {
		a.localJournal = &memJournal{}
	}
	return a.localJournal
}

func (a *App) storeJournal(ans config.JournalAnswer) {
	if err := a.journalStore().SetJournal(a.bds.Workspace.Path, ans); err != nil {
		a.warn("could not store the answer: " + shortLine(err.Error()))
	}
}

func (a *App) enableJournal() tea.Cmd {
	return a.write(writeOp{
		run: func(ctx context.Context, c bd.Client) (string, error) {
			return "", c.ConfigSet(ctx, "events-journal", "true")
		},
		done: func(a *App, res writeResult) tea.Cmd {
			if res.err != nil {
				a.warn("events journal not enabled: " + writeSummary(res.err, 1))
				return nil
			}
			a.bds.EventsJournal = true
			if a.eng != nil {
				a.eng.EnableEvents()
			}
			a.journalDeclined, a.journalLater = false, false
			a.storeJournal(config.JournalUnasked)
			a.toast("events journal enabled")
			return nil
		},
	})
}

// maybeAskJournal opens the opt-in dialog after the first snapshot when bd
// has a journal that is off and the viewer has not declined. An answer of
// "enabled" in the store does not count while the journal is off.
func (a *App) maybeAskJournal() {
	if a.journalAsked || a.eng == nil || a.snap == nil || !a.journalOff() {
		return
	}
	if len(a.dialogs) > 0 || a.bar != nil || a.report != nil {
		return
	}
	a.journalAsked = true
	if a.journalStore().Journal(a.bds.Workspace.Path) == config.JournalDeclined {
		a.journalDeclined = true
		return
	}
	a.pushDialog(&journalDialog{a: a})
}

func (a *App) journalOff() bool { return a.bds.Version.HasJournal() && !a.bds.EventsJournal }

func (a *App) cmdJournal([]command.Word) (tea.Cmd, error) {
	switch {
	case !a.bds.Version.HasJournal():
		return nil, &command.Error{Msg: "this bd has no events journal"}
	case a.bds.EventsJournal:
		return nil, &command.Error{Msg: "the events journal is on already"}
	}
	a.pushDialog(&journalDialog{a: a})
	return nil, nil
}

// journalLimited reports whether the actors hint shows: the journal exists
// but is off.
func (a *App) journalLimited() bool {
	return a.snap != nil && a.journalOff() && (a.journalDeclined || a.journalLater)
}

// memJournal keeps the answers in memory when no store is wired.
type memJournal struct {
	m map[string]config.JournalAnswer
}

func (j *memJournal) Journal(ws string) config.JournalAnswer { return j.m[ws] }

func (j *memJournal) SetJournal(ws string, a config.JournalAnswer) error {
	if j.m == nil {
		j.m = map[string]config.JournalAnswer{}
	}
	j.m[ws] = a
	return nil
}
