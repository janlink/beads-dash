package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/clipboard"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/notify"
)

// titleGuard separates a window title change and an OSC 52 write in both
// directions: Windows Terminal freezes for seconds when both reach it in the
// same frame.
const titleGuard = 100 * time.Millisecond

type (
	titleHeldMsg struct{}
	copiedMsg    struct {
		what string
		res  clipboard.Result
	}
	notifiedMsg struct {
		msgs []notify.Message
		d    notify.Delivery
	}
)

// copy puts text on the clipboard: OSC 52 through the terminal and, when a
// Clipboard is wired, the helper routes. what names the text in the footer.
func (a *App) copy(what, text string) tea.Cmd {
	if err := clipboard.Check(text); err != nil {
		a.warn(what + ": " + err.Error())
		return nil
	}
	osc := a.afterTitle(tea.SetClipboard(text))
	a.copyAt = a.now().Add(a.titleWait())
	if a.o.Clipboard == nil {
		msg, warn := clipboard.Result{}.Message(what)
		a.note(msg, warn)
		return osc
	}
	board := a.o.Clipboard
	return tea.Batch(osc, func() tea.Msg {
		return copiedMsg{what, board.Write(context.Background(), text)}
	})
}

func (a *App) note(text string, warn bool) {
	if warn {
		a.warn(text)
		return
	}
	a.toast(text)
}

// titleWait is how much longer an OSC 52 write has to wait for the guard
// time after the last title change.
func (a *App) titleWait() time.Duration {
	if a.titleAt.IsZero() {
		return 0
	}
	return max(titleGuard-a.now().Sub(a.titleAt), 0)
}

// afterTitle holds cmd back until the guard time has passed since the window
// title last changed.
func (a *App) afterTitle(cmd tea.Cmd) tea.Cmd {
	wait := a.titleWait()
	if wait <= 0 {
		return cmd
	}
	return tea.Tick(wait, func(time.Time) tea.Msg { return cmd() })
}

func (a *App) onCopied(m copiedMsg) {
	msg, warn := m.res.Message(m.what)
	a.note(msg, warn)
}

// windowTitle is the title the window should show.
func (a *App) windowTitle() string {
	t := "bdash"
	if n := a.workspaceName(); n != "" {
		t += " · " + n
	}
	return t
}

// syncTitle moves the shown title to the wanted one, holding the old title
// while the guard time after a copy runs.
func (a *App) syncTitle() tea.Cmd {
	want := a.windowTitle()
	if want == a.title {
		return nil
	}
	if a.title != "" && !a.copyAt.IsZero() {
		if wait := a.copyAt.Add(titleGuard).Sub(a.now()); wait > 0 {
			if a.titleHeld {
				return nil
			}
			a.titleHeld = true
			return tea.Tick(wait, func(time.Time) tea.Msg { return titleHeldMsg{} })
		}
	}
	a.title, a.titleAt = want, a.now()
	return nil
}

func (a *App) copyCurrentID() tea.Cmd {
	id := a.sess.Current()
	if id == "" {
		a.hint = "no issue to copy"
		return nil
	}
	return a.copy(id, id)
}

// notifyKinds is the kind set notifications are on for.
func (a *App) notifyKinds() model.KindSet { return model.KindSetOf(a.notifyNames...) }

func (a *App) pushNotify() {
	if a.eng != nil {
		a.eng.SetNotify(a.notifyOn, a.notifyKinds())
	}
}

// deliver hands the notification of one refresh to the notifier.
func (a *App) deliver(n model.Notification) tea.Cmd {
	if !a.notifyOn {
		return nil
	}
	msgs := notify.Compose(n)
	if len(msgs) == 0 {
		return nil
	}
	nf := a.o.Notifier
	return func() tea.Msg {
		d := notify.Delivery{Route: notify.RouteBell, Bell: true}
		if nf != nil {
			d = nf.Send(context.Background(), msgs)
		}
		return notifiedMsg{msgs, d}
	}
}

func (a *App) onNotified(m notifiedMsg) tea.Cmd {
	var cmds []tea.Cmd
	if m.d.Raw != "" {
		cmds = append(cmds, tea.Raw(m.d.Raw))
	}
	if m.d.Bell {
		cmds = append(cmds, tea.Raw("\a"))
		for _, msg := range m.msgs {
			a.toast(msg.Line())
		}
	}
	switch {
	case a.notifyWarned:
	case m.d.Hint != "":
		a.notifyWarned = true
		a.warn(m.d.Hint)
	case m.d.Err != nil:
		a.notifyWarned = true
		a.warn("desktop notification failed, using the bell: " + shortLine(m.d.Err.Error()))
	}
	return tea.Batch(cmds...)
}

func (a *App) setNotify(on bool, names []string) tea.Cmd {
	a.notifyOn, a.notifyNames = on, names
	a.pushNotify()
	switch {
	case !on && a.notifyMethod != notify.MethodOff:
		a.notifyPrev, a.notifyMethod = a.notifyMethod, notify.MethodOff
	case on && a.notifyMethod == notify.MethodOff:
		a.notifyMethod = a.notifyPrev
	}
	store := a.o.Store
	if store == nil {
		return nil
	}
	method := a.notifyMethod
	return func() tea.Msg {
		if err := store.Set("notify.method", method); err != nil {
			return savedMsg{err}
		}
		return savedMsg{store.Set("notify.kinds", names)}
	}
}

// notifyRestore is method unless it is off, which has nothing to restore.
func notifyRestore(method string) string {
	if method == notify.MethodOff {
		return ""
	}
	return method
}
