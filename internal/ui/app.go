package ui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/appearance"
	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/notify"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/ui/command"
	"github.com/janlink/beads-dash/internal/ui/detail"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/screens"
	"github.com/janlink/beads-dash/internal/ui/state"
)

type (
	sessionMsg struct {
		sess bd.Session
		err  error
	}
	engineMsg struct {
		eng    Engine
		cancel context.CancelFunc
		sess   bd.Session
	}
	updateMsg     struct{ u refresh.Update }
	closedMsg     struct{}
	recheckMsg    struct{ gen int }
	clockMsg      struct{}
	hlTickMsg     struct{ gen int }
	searchTickMsg struct{ gen int }
	savedMsg      struct{ err error }
)

const (
	wheelStep     = 3
	vanishedAfter = 3
)

// App is the root model. Update never blocks: everything that touches bd,
// the config file or the clipboard is a returned command, except the engine's
// focus and refresh posts, which only enqueue a request.
type App struct {
	o  Options
	km *keys.Map
	mx *keys.Matcher

	app     appearance.Appearance
	look    look.Look
	rend    *rows.Renderer
	choices dialog.Choices

	cols, rows int
	focused    bool

	sess    *state.Session
	acts    sessionActions
	hl      *state.Highlights
	hlGen   int
	hlDue   time.Time
	views   [6]View
	slot    int
	slotCur [6]string

	scope model.Scope
	away  away
	mset  *model.Matches
	msnap *model.Snapshot
	mkey  string
	// expComments caches comment threads read for exports.
	expComments map[string]commentEntry
	panel       *detail.Panel
	docked      bool
	// syncMD renders markdown inside Update; tests use it to see the final page.
	syncMD bool
	// fetching holds the cancel of each audit read still running, by its seq.
	fetching map[int]context.CancelFunc
	lookGen  int

	bds        bd.Session
	startErr   error
	checking   bool
	checkGen   int
	startFails int
	nextCheck  time.Time
	report     *screens.Report
	vanished   bool
	pick       int
	eng        Engine
	cancel     context.CancelFunc
	snap       *model.Snapshot
	feed       model.Ring
	feedNewest []model.Event
	status     refresh.Status
	ticking    bool
	notices    []screens.Notice
	hint       string
	dialogs    []Dialog
	quitting   bool

	bar         *bar
	searchHist  *recall
	commandHist *recall
	cmds        *command.Table
	run         map[string]commandFunc

	mem                                         *memState
	journalAsked, journalDeclined, journalLater bool
	localJournal                                *memJournal

	title        string
	titleAt      time.Time
	copyAt       time.Time
	titleHeld    bool
	notifyOn     bool
	notifyNames  []string
	notifyMethod string
	// notifyPrev is the method to restore when notifications go back on.
	notifyPrev   string
	notifyWarned bool

	writeSeq int
	pending  map[int]writeOp
	// pendingCurrent is an issue the current one moves to as soon as a
	// snapshot holds it.
	pendingCurrent string
}

// New returns the root model.
func New(o Options) *App {
	o.fill()
	a := &App{
		o: o, km: o.Keys, mx: keys.NewMatcher(o.Keys),
		app: o.Appearance, cols: 80, rows: 24, focused: true,
		sess:         state.New(),
		mem:          newMemState(),
		notifyOn:     o.Settings.Settings.NotifyMethod != "off",
		notifyNames:  slices.Clone(o.Settings.Settings.NotifyKinds),
		notifyMethod: o.Settings.Settings.NotifyMethod,
		notifyPrev:   cmp.Or(notifyRestore(o.Settings.Settings.NotifyMethod), notify.MethodAuto),
		hl:           state.NewHighlights(time.Duration(o.Settings.Settings.HighlightSeconds) * time.Second),
		choices: dialog.Choices{
			Theme: o.Settings.Settings.Theme, Background: o.Settings.Settings.Background, Glyphs: o.Settings.Settings.Glyphs,
		},
	}
	var saved []string
	if o.History != nil {
		saved = o.History.History()
	}
	search, typed := splitHistory(saved)
	a.searchHist, a.commandHist = newRecall(search), newRecall(typed)
	a.cmds, a.run = command.NewTable(), map[string]commandFunc{}
	a.registerBuiltins()
	a.acts = sessionActions{s: a.sess, show: a.show}
	a.panel = detail.New()
	a.docked = o.Settings.Settings.DetailDocked
	a.setScope(model.ParseScope("", o.Settings.Settings.ShowClosed))
	a.setLook()
	a.rend = rows.New(a.look)
	for n, v := range o.Views {
		if n >= 1 && n <= 6 {
			a.views[n-1] = v
		}
	}
	if !hasView(a.views) {
		a.views[0] = &placeholder{}
	} else if a.views[memSlot] == nil {
		a.views[memSlot] = &memoriesView{a: a}
	}
	a.slot = a.startSlot()
	for _, w := range o.Warnings {
		a.notices = append(a.notices, screens.Notice{Text: w, Warn: true})
	}
	if o.Appearance.Notice != "" {
		a.notices = append(a.notices, screens.Notice{Text: o.Appearance.Notice})
	}
	return a
}

func hasView(vs [6]View) bool {
	for _, v := range vs {
		if v != nil {
			return true
		}
	}
	return false
}

func (a *App) startSlot() int {
	name := strings.ToLower(a.o.Settings.Settings.View)
	for i, n := range ViewNames {
		if strings.ToLower(n) == name && a.views[i] != nil {
			return i
		}
	}
	for i, v := range a.views {
		if v != nil {
			return i
		}
	}
	return 0
}

// matches is the scope applied to the snapshot, recomputed only when either
// changes.
func (a *App) matches() *model.Matches {
	if a.snap == nil {
		return nil
	}
	if k := a.scope.Key(); a.mset == nil || a.msnap != a.snap || a.mkey != k {
		a.mset, a.msnap, a.mkey = a.scope.Apply(a.snap, a.bds.Statuses), a.snap, k
	}
	return a.mset
}

func (a *App) setLook() {
	a.lookGen++
	a.look = look.New(a.app.Palette, a.app.Glyphs)
	a.relayout()
	if a.rend != nil {
		a.rend.SetLook(a.look)
	}
}

func (a *App) now() time.Time { return a.o.Now() }

func (a *App) view() View { return a.views[a.slot] }

func (a *App) mouseOn() bool { return a.o.Settings.Settings.Mouse && !a.o.NoMouse }

// Init implements tea.Model.
func (a *App) Init() tea.Cmd { return a.openSession() }

func (a *App) openSession() tea.Cmd {
	c := a.o.Client
	a.checking = true
	return func() tea.Msg {
		s, err := bd.OpenSession(context.Background(), c)
		return sessionMsg{s, err}
	}
}

// recheck costs one bd call while the failure persists: it repeats only the
// step that failed and opens the whole session once that step passes.
func (a *App) recheck() tea.Cmd {
	c, prev, failed := a.o.Client, a.bds, a.startErr
	a.checking = true
	return func() tea.Msg {
		ctx := context.Background()
		switch {
		case prev.Version.Parsed == (bd.Version{}) || bd.IsClass(failed, bd.ClassUnsupported):
			v, err := c.Version(ctx)
			if err != nil {
				prev.Version, prev.Untested = v, v.Support == bd.Untested
				return sessionMsg{prev, err}
			}
		case prev.Workspace.Path == "":
			if _, err := c.Where(ctx); err != nil {
				return sessionMsg{prev, err}
			}
		default:
			if _, err := c.Statuses(ctx); err != nil {
				return sessionMsg{prev, err}
			}
		}
		s, err := bd.OpenSession(ctx, c)
		return sessionMsg{s, err}
	}
}

func (a *App) startEngine(s bd.Session) tea.Cmd {
	newEngine := a.o.NewEngine
	return func() tea.Msg {
		e := newEngine(s)
		ctx, cancel := context.WithCancel(context.Background())
		e.Start(ctx)
		return engineMsg{e, cancel, s}
	}
}

func waitUpdate(e Engine) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-e.Updates()
		if !ok {
			return closedMsg{}
		}
		return updateMsg{u}
	}
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := a.update(msg)
	a.syncLayers()
	if s, ok := a.view().(Syncer); ok && a.snap != nil {
		s.Sync(a.env())
	}
	cmd = tea.Batch(cmd, a.renderJobs(), a.memSync(), a.syncTitle())
	if a.needsClock() && !a.ticking && !a.quitting {
		a.ticking = true
		cmd = tea.Batch(cmd, tea.Tick(time.Second, func(time.Time) tea.Msg { return clockMsg{} }))
	}
	return a, cmd
}

func (a *App) needsClock() bool {
	return a.startErr != nil || a.status.Stale || a.status.Err != nil
}

func (a *App) update(msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.cols, a.rows = m.Width, m.Height
		a.relayout()
	case tea.KeyPressMsg:
		return a.key(m)
	case tea.PasteMsg:
		return a.paste(m.Content)
	case searchTickMsg:
		if b := a.bar; b != nil && m.gen == b.gen {
			a.flushSearch()
		}
	case tea.MouseClickMsg:
		a.click(tea.Mouse(m))
	case tea.MouseWheelMsg:
		a.wheel(tea.Mouse(m))
	case tea.FocusMsg:
		return a.setFocus(true)
	case tea.BlurMsg:
		return a.setFocus(false)
	case sessionMsg:
		return a.onSession(m)
	case engineMsg:
		return a.onEngine(m)
	case updateMsg:
		return tea.Batch(a.apply(m.u), waitUpdate(a.eng))
	case mdMsg:
		a.panel.Apply(m.res)
	case auditMsg:
		a.endFetch(m.res.Seq)
		a.panel.ApplyAudit(m.res)
	case writeDoneMsg:
		return a.onWrite(m)
	case exportMsg:
		return m.apply(a)
	case memReadMsg:
		return a.onMemRead(m)
	case closedMsg:
	case recheckMsg:
		if m.gen == a.checkGen && a.startErr != nil && !a.checking {
			return a.recheck()
		}
	case titleHeldMsg:
		a.titleHeld = false
	case clockMsg:
		a.ticking = false
	case hlTickMsg:
		if m.gen == a.hlGen {
			a.hlDue = time.Time{}
			a.hl.Expire(a.now())
			return a.hlSchedule()
		}
	case copiedMsg:
		a.onCopied(m)
	case notifiedMsg:
		return a.onNotified(m)
	case savedMsg:
		if m.err != nil {
			a.notices = append(a.notices, screens.Notice{Text: fmt.Sprintf("could not save the choice: %v; it lasts for this session only", m.err), Warn: true})
		}
	default:
		return a.forward(msg)
	}
	return nil
}

func (a *App) onSession(m sessionMsg) tea.Cmd {
	a.checking = false
	a.bds = m.sess
	if m.err == nil {
		a.startErr, a.report, a.startFails = nil, nil, 0
		if m.sess.Untested {
			a.notices = append(a.notices, screens.Notice{
				Text: fmt.Sprintf("bd %s is newer than the tested %d.%d line; bdash may misread it", m.sess.Version.Parsed, bd.TestedCeiling.Major, bd.TestedCeiling.Minor),
				Warn: true,
			})
		}
		return a.startEngine(m.sess)
	}
	a.startErr = m.err
	a.startFails++
	a.computeReport()
	back := min(a.o.RecheckMin<<(a.startFails-1), a.o.RecheckMax)
	a.nextCheck = a.now().Add(back)
	a.checkGen++
	gen := a.checkGen
	return tea.Tick(back, func(time.Time) tea.Msg { return recheckMsg{gen} })
}

func (a *App) onEngine(m engineMsg) tea.Cmd {
	a.eng, a.cancel = m.eng, m.cancel
	if !a.focused {
		m.eng.SetFocus(false)
	}
	m.eng.SetNotify(a.notifyOn, a.notifyKinds())
	return waitUpdate(m.eng)
}

func (a *App) setFocus(on bool) tea.Cmd {
	a.focused = on
	if a.eng != nil {
		a.eng.SetFocus(on)
	}
	return a.syncPause()
}

// forward hands a message nobody above consumed to the appearance, the views
// and the open dialogs.
func (a *App) forward(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	next, cmd := a.app.Update(msg)
	if next.Dark != a.app.Dark || next.Palette != a.app.Palette {
		a.app = next
		a.setLook()
	}
	cmds = append(cmds, cmd)
	for _, v := range a.views {
		if u, ok := v.(Updater); ok {
			cmds = append(cmds, u.Update(msg))
		}
	}
	for _, d := range a.dialogs {
		cmds = append(cmds, d.Update(msg))
	}
	return tea.Batch(cmds...)
}

// syncPause pauses the highlight clock on blur and while a dialog is open.
func (a *App) syncPause() tea.Cmd {
	a.hl.SetPaused(a.now(), !a.focused || len(a.dialogs) > 0)
	return a.hlSchedule()
}

// hlSchedule arms the tick for the earliest highlight deadline; it does
// nothing while a tick for that deadline is already armed.
func (a *App) hlSchedule() tea.Cmd {
	now := a.now()
	d, ok := a.hl.Next(now)
	if !ok {
		if !a.hlDue.IsZero() {
			a.hlGen++
			a.hlDue = time.Time{}
		}
		return nil
	}
	due := now.Add(d)
	if due.Equal(a.hlDue) {
		return nil
	}
	a.hlGen++
	a.hlDue = due
	gen := a.hlGen
	return tea.Tick(d, func(time.Time) tea.Msg { return hlTickMsg{gen} })
}

func (a *App) apply(u refresh.Update) tea.Cmd {
	a.status = u.Status
	if u.Session.Workspace.Path != "" {
		a.bds = u.Session
	}
	if a.status.Err == nil {
		a.vanished = false
	}
	var cmds []tea.Cmd
	if u.Snapshot != nil && u.Snapshot != a.snap && !a.vanished {
		old := slices.Clone(a.view().Visible(a.env()))
		a.snap = u.Snapshot
		a.rend.Bind(a.snap, a.bds.Statuses)
		exists := func(id string) bool {
			_, ok := a.snap.Issue(id)
			return ok || strings.HasPrefix(id, memPrefix)
		}
		a.sess.Prune(exists)
		a.hl.Prune(exists)
		a.keepCurrent(old)
		if a.pendingCurrent != "" {
			a.focusIssue(a.pendingCurrent)
		}
		a.clampCursors()
		a.notifyDialogs()
	}
	if len(u.Events) > 0 {
		a.feed.Merge(u.Events...)
		a.feedNewest = a.feed.Newest()
	}
	if len(u.Highlights) > 0 {
		a.hl.Trigger(a.now(), u.Highlights, u.Events)
		cmds = append(cmds, a.hlSchedule())
	}
	a.computeReport()
	if a.report != nil {
		cmds = append(cmds, a.cancelDialogs())
	}
	a.maybeAskJournal()
	cmds = append(cmds, a.deliver(u.Notification))
	return tea.Batch(cmds...)
}

// keepCurrent moves the current issue to its nearest neighbour when the view
// no longer shows it.
func (a *App) keepCurrent(old []string) {
	cur := a.sess.Current()
	if cur != "" {
		if _, ok := a.snap.Issue(cur); ok {
			if slices.Contains(old, cur) {
				a.reconcile(old)
			}
			return
		}
	}
	a.reconcile(old)
	if a.sess.Current() == "" {
		if vis := a.view().Visible(a.env()); len(vis) > 0 {
			a.sess.SetCurrent(vis[0])
		}
	}
}

func (a *App) computeReport() {
	if a.snap != nil && bd.IsClass(a.status.Err, bd.ClassNotWorkspace) && a.status.Failures >= vanishedAfter {
		a.vanished = true
		a.snap = nil
		a.rend.Bind(nil, a.bds.Statuses)
	}
	var r screens.Report
	switch {
	case a.startErr != nil:
		r = screens.Diagnose(a.reportInput(a.startErr, false, false))
	case a.vanished:
		r = screens.Diagnose(a.reportInput(a.status.Err, false, true))
	case a.snap == nil && a.status.Err != nil:
		r = screens.Diagnose(a.reportInput(a.status.Err, true, false))
	default:
		a.report = nil
		return
	}
	if a.report == nil || a.report.Kind != r.Kind {
		a.pick = 0
	}
	a.report = &r
}

func (a *App) env() Env {
	now := a.now()
	return Env{
		Snap: a.snap, Statuses: a.bds.Statuses, Look: a.look, Rows: a.rend, Current: a.sess.Current(),
		Marked:  a.sess.Marked,
		Changed: func(id string) bool { return a.hl.Live(id, now) },
		Act:     a.acts,
		Scope:   a.scope, Matches: a.matches(), Now: now, Cols: a.cols, Feed: a.feedNewest,
		Docked: a.dock().Frame != detail.Hidden && a.sess.Current() != "",
	}
}

func (a *App) visible(id string) bool { return a.view().Has(a.env(), id) }

func (a *App) tooSmall() bool { return a.cols < MinCols || a.rows < MinRows }

func (a *App) context() keys.Context {
	switch {
	case a.tooSmall():
		return keys.TooSmall
	case a.report != nil:
		return keys.Startup
	}
	if d := a.topDialog(); d != nil {
		return d.Context()
	}
	return a.baseContext()
}

// baseContext is the key context below the dialogs.
func (a *App) baseContext() keys.Context {
	if top, ok := a.sess.Top(); ok {
		switch top {
		case state.LayerDetailFocus, state.LayerDetail:
			return keys.Panel
		case state.LayerBar:
			return a.bar.kind.context()
		case state.LayerDialog:
		}
	}
	return a.view().Context()
}

func (a *App) topDialog() Dialog {
	if n := len(a.dialogs); n > 0 {
		return a.dialogs[n-1]
	}
	return nil
}

// chromeRows are the header rule and the footer's two rows.
const chromeRows = 3

// bodyHeight is the rows left for the body between the header and the footer.
func (a *App) bodyHeight() int {
	return max(a.rows-chromeRows-a.barHeight(), 1)
}

func (a *App) refreshNotice() (screens.Notice, bool) {
	if a.snap == nil || a.report != nil || a.status.Err == nil {
		return screens.Notice{}, false
	}
	text := "refresh failed: " + firstLine(a.status.Err)
	if d := a.status.NextRetry.Sub(a.now()); d > 0 {
		text += fmt.Sprintf(" - retrying in %ds", int((d+time.Second-1)/time.Second))
	}
	return screens.Notice{Text: text, Warn: true, Keys: []keys.Hint{{Key: "r", Desc: "retry"}, {Key: "!", Desc: "details"}}}, true
}

// queueShown reports whether the first queued notice is on screen.
func (a *App) queueShown() bool {
	if len(a.notices) == 0 || a.report != nil || a.tooSmall() {
		return false
	}
	if _, ok := a.refreshNotice(); ok {
		return false
	}
	return len(a.dialogs) == 0 || !dialog.FullScreen(a.cols, a.rows)
}

func (a *App) notice() (screens.Notice, bool) {
	if n, ok := a.refreshNotice(); ok {
		return n, true
	}
	if a.queueShown() {
		return a.notices[0], true
	}
	return screens.Notice{}, false
}

func firstLine(err error) string {
	var be *bd.Error
	msg := err.Error()
	if errors.As(err, &be) && be.Message != "" {
		msg = be.Message
	}
	line, _, _ := strings.Cut(strings.TrimSpace(msg), "\n")
	return line
}

func (a *App) key(m tea.KeyPressMsg) tea.Cmd {
	a.hint = ""
	if a.queueShown() {
		a.notices = a.notices[1:]
	}
	k := m.String()
	if act, ok := a.always(k); ok {
		return a.act(act, k)
	}
	if d, ok := a.topDialog().(rawKeys); ok && d.RawKeys() && a.report == nil && !a.tooSmall() {
		return a.topDialog().Update(m)
	}
	b, res := a.mx.Feed(a.context(), k)
	switch res {
	case keys.NoMatch:
		return a.typeKey(m)
	case keys.Pending:
		return nil
	case keys.Matched:
	}
	return a.act(b.Action, k)
}

// always resolves the keys that work in every state, text input included.
func (a *App) always(k string) (keys.Action, bool) {
	for _, b := range a.km.Active(keys.Always) {
		if slices.Contains(b.Keys, k) {
			return b.Action, true
		}
	}
	return "", false
}

func (a *App) act(act keys.Action, key string) tea.Cmd {
	switch act { //nolint:exhaustive // the action set is open: views add their own
	case keys.QuitForce, keys.Quit:
		if a.dirtyDialog() && !isConfirm(a.topDialog()) {
			a.pushDialog(a.newConfirm(confirmOpts{
				Title: "Discard changes and quit?",
				Lines: []string{"An open form has unsaved changes."},
				Yes:   func(a *App) tea.Cmd { return a.quit() },
			}))
			return nil
		}
		return a.quit()
	}
	if a.report != nil {
		return a.reportAct(act)
	}
	if d := a.topDialog(); d != nil {
		cmd, closed := d.Handle(act)
		if closed {
			a.popDialog()
		}
		return tea.Batch(cmd, a.syncPause())
	}
	if a.barOpen() {
		if cmd, ok := a.barAct(act); ok {
			return tea.Batch(cmd, a.syncPause())
		}
	}
	if a.inMemories() {
		if cmd, ok := a.memAct(act); ok {
			return tea.Batch(cmd, a.syncPause())
		}
	}
	switch act { //nolint:exhaustive // the action set is open: views add their own
	case keys.Retry, keys.Refresh:
		return a.retry()
	case keys.OpenSearch:
		a.openBar(barSearch)
	case keys.OpenFilter:
		a.openBar(barFilter)
	case keys.OpenCommand:
		a.openBar(barCommand)
	case keys.OpenPicker:
		a.openJumpPicker()
	case keys.OpenHelp:
		a.pushDialog(&helpDialog{a: a, under: a.baseContext()})
	case keys.OpenAppearance:
		a.pushDialog(a.newAppearanceDialog())
	case keys.Notifications:
		a.pushDialog(a.newNotifyDialog())
	case keys.CopyID:
		return a.copyCurrentID()
	case keys.Export:
		return a.openExport()
	case keys.OpenDetails:
		if a.detailError() == nil {
			a.hint = "no error to show"
			return nil
		}
		a.pushDialog(&detailsDialog{a: a})
	case keys.SwitchView:
		a.switchView(key)
	case keys.Back:
		if o, ok := a.sess.BackTo(a.visible); ok && o.Slot != state.NoView {
			if r, restores := a.views[o.Slot].(Restorer); restores && o.Focused {
				r.Restore(a.env(), o.Focus)
			}
			a.switchTo(o.Slot, "")
		}
	case keys.Close:
		if _, layered := a.sess.Top(); !layered && len(a.sess.MarkedIDs()) == 0 {
			if c, ok := a.view().(Closer); ok && c.Close(a.env()) {
				return nil
			}
		}
		barOpen := a.barOpen()
		if a.sess.Esc() == state.EscScope {
			a.applyScope(a.clearedScope())
		}
		if barOpen {
			return tea.Batch(a.closeBar(), a.syncPause())
		}
	case keys.Open:
		if cmd, ok := a.view().Handle(act, a.env()); ok || !a.openDetail() {
			return cmd
		}
	case keys.FocusNext:
		if !a.viewFocusNext() {
			a.focusNext()
		}
	case keys.DetailToggle:
		return a.toggleDocked()
	case keys.Mark:
		if id := a.sess.Current(); id != "" {
			a.sess.ToggleMark(id)
		}
	case keys.New:
		return a.openNew("")
	case keys.Edit:
		return a.openEdit()
	case keys.ChangeStatus:
		return a.openQuick(quickStatus)
	case keys.ChangePriority:
		return a.openQuick(quickPriority)
	case keys.ChangeAssignee:
		return a.openQuick(quickAssignee)
	case keys.ChangeLabels:
		return a.openQuick(quickLabels)
	case keys.CloseReopen:
		return a.openClose()
	case keys.MoveLeft:
		return a.moveCards(false)
	case keys.MoveRight:
		return a.moveCards(true)
	default:
		if a.baseContext() == keys.Panel {
			a.panelAct(act)
			return nil
		}
		return a.viewAct(act)
	}
	return a.syncPause()
}

// viewAct offers an action to the view; cursor movement the view declines
// falls back to the shell's own navigation of the base list.
func (a *App) viewAct(act keys.Action) tea.Cmd {
	cmd, ok := a.view().Handle(act, a.env())
	if !ok && isNav(act) && a.baseContext() == a.view().Context() {
		a.navigate(act)
	}
	return cmd
}

func (a *App) reportAct(act keys.Action) tea.Cmd {
	switch act { //nolint:exhaustive // the startup context binds only these
	case keys.Retry, keys.Refresh:
		return a.retry()
	case keys.Copy:
		if a.report == nil {
			return nil
		}
		text := strings.Join(a.report.Raw, "\n")
		if len(a.report.Fixes) > 0 {
			text = a.report.Fixes[min(a.pick, len(a.report.Fixes)-1)].Cmd
		}
		return a.copy("fix", text)
	case keys.PickDn:
		a.pick++
	case keys.PickUp:
		a.pick--
	}
	n := 0
	if a.report != nil {
		n = len(a.report.Fixes)
	}
	a.pick = min(max(a.pick, 0), max(n-1, 0))
	return nil
}

func (a *App) pushDialog(d Dialog) {
	a.dialogs = append(a.dialogs, d)
	a.sess.Push(state.LayerDialog)
	a.relayout()
}

// relayout hands the screen size and look to the dialogs that size their
// content ahead of drawing.
func (a *App) relayout() {
	for _, d := range a.dialogs {
		if l, ok := d.(interface{ layout(look.Look, int, int) }); ok {
			l.layout(a.look, a.cols, a.rows)
		}
	}
}

func (a *App) popDialog() {
	if n := len(a.dialogs); n > 0 {
		a.dialogs = a.dialogs[:n-1]
		a.sess.Remove(state.LayerDialog)
	}
}

// cancelDialogs closes every dialog through its cancel path, top first.
func (a *App) cancelDialogs() tea.Cmd {
	var cmds []tea.Cmd
	for len(a.dialogs) > 0 {
		cmd, _ := a.topDialog().Handle(keys.Close)
		cmds = append(cmds, cmd)
		a.popDialog()
	}
	cmds = append(cmds, a.syncPause())
	return tea.Batch(cmds...)
}

func (a *App) preview(c dialog.Choices) {
	a.app = a.app.Preview(a.o.Getenv, c.Theme, c.Background, c.Glyphs)
	a.setLook()
}

func (a *App) persist(changes map[string]string) tea.Cmd {
	store := a.o.Store
	if store == nil || len(changes) == 0 {
		return nil
	}
	names := make([]string, 0, len(changes))
	for k := range changes {
		names = append(names, k)
	}
	sort.Strings(names)
	return func() tea.Msg {
		for _, k := range names {
			if err := store.Set(k, changes[k]); err != nil {
				return savedMsg{err}
			}
		}
		return savedMsg{}
	}
}

func (a *App) switchView(key string) { a.switchTo(int(key[0]-'1'), "") }

// show opens view number n (1-6) on issue id; the back key returns to the
// view and issue it left.
func (a *App) show(n int, id string) {
	if n < 1 || n > 6 || a.views[n-1] == nil {
		return
	}
	a.switchTo(n-1, id)
}

// switchTo makes slot n the current view. With an id, that issue becomes
// current and the view and issue left go on the back stack, even when the
// issue stays the same.
func (a *App) switchTo(n int, id string) {
	if n < 0 || n > 5 {
		return
	}
	if a.views[n] == nil {
		a.hint = fmt.Sprintf("%s is not available yet", ViewNames[n])
		return
	}
	if n == a.slot && id == "" {
		return
	}
	cur := a.sess.Current()
	a.slotCur[a.slot] = cur
	if id != "" {
		a.sess.JumpFrom(a.slot, id)
		cur = id
	}
	a.slot = n
	env := a.env()
	v := a.views[n]
	if cur != "" && v.Has(env, cur) {
		return
	}
	if last := a.slotCur[n]; last != "" && v.Has(env, last) {
		a.sess.SetCurrent(last)
		return
	}
	if vis := v.Visible(env); len(vis) > 0 {
		a.sess.SetCurrent(vis[0])
	}
}

func isNav(act keys.Action) bool {
	switch act { //nolint:exhaustive // only the navigation actions matter here
	case keys.NavDown, keys.NavUp, keys.NavFirst, keys.NavLast, keys.NavHalfDown, keys.NavHalfUp, keys.NavPageDown, keys.NavPageUp:
		return true
	}
	return false
}

func (a *App) navigate(act keys.Action) {
	env := a.env()
	order := a.view().Visible(env)
	if len(order) == 0 {
		return
	}
	i := slices.Index(order, env.Current)
	_, h := a.listSize()
	to := i
	switch act { //nolint:exhaustive // the action set is open: views add their own
	case keys.NavDown:
		to = i + 1
	case keys.NavUp:
		to = i - 1
	case keys.NavFirst:
		to = 0
	case keys.NavLast:
		to = len(order) - 1
	case keys.NavHalfDown:
		to = i + max(h/2, 1)
	case keys.NavHalfUp:
		to = i - max(h/2, 1)
	case keys.NavPageDown:
		to = i + h
	case keys.NavPageUp:
		to = i - h
	}
	if i < 0 {
		to = 0
	}
	a.sess.SetCurrent(order[min(max(to, 0), len(order)-1)])
}

func (a *App) retry() tea.Cmd {
	if a.startErr != nil {
		if a.checking {
			return nil
		}
		a.startFails = 0
		return a.recheck()
	}
	a.memRefresh()
	a.panel.RetryAudit()
	if a.eng != nil {
		a.eng.Refresh()
	}
	return nil
}

func (a *App) dirtyDialog() bool {
	for _, d := range a.dialogs {
		if x, ok := d.(interface{ Dirty() bool }); ok && x.Dirty() {
			return true
		}
	}
	return false
}

func isConfirm(d Dialog) bool {
	_, ok := d.(*confirmDialog)
	return ok
}

func (a *App) quit() tea.Cmd {
	a.quitting = true
	e, cancel := a.eng, a.cancel
	stop := func() tea.Msg {
		if e != nil {
			e.Stop()
			cancel()
		}
		return nil
	}
	return tea.Sequence(stop, tea.Quit)
}

func (a *App) click(m tea.Mouse) {
	if !a.mouseOn() || m.Button != tea.MouseLeft || len(a.dialogs) > 0 || a.report != nil || a.tooSmall() {
		return
	}
	switch {
	case m.Y == 1+a.bodyHeight():
		_, _, spans := a.footerLayout(a.cols - a.panelSide())
		for i, s := range spans {
			if m.X >= s.from && m.X < s.to {
				a.switchView(fmt.Sprintf("%d", i+1))
			}
		}
	case m.Y >= 1 && m.Y-1 < a.bodyHeight():
		a.clickBody(m.X, m.Y-1)
	}
}

func (a *App) wheel(m tea.Mouse) {
	if !a.mouseOn() || len(a.dialogs) > 0 || a.report != nil || a.tooSmall() {
		return
	}
	n := 0
	switch m.Button { //nolint:exhaustive // only the wheel is handled
	case tea.MouseWheelUp:
		n = -wheelStep
	case tea.MouseWheelDown:
		n = wheelStep
	default:
		return
	}
	if a.inMemories() {
		a.memWheel(m.X, m.Y-1, n)
		return
	}
	if a.overPanel(m.X, m.Y-1) {
		a.panel.ScrollBy(n)
		return
	}
	a.view().Scroll(n)
}

func (a *App) workspaceName() string {
	p := a.bds.Workspace.Path
	if p == "" {
		return ""
	}
	if filepath.Base(p) == ".beads" { // lintcheck:allow workspace directory name
		p = filepath.Dir(p)
	}
	return filepath.Base(p)
}

// View implements tea.Model.
func (a *App) View() tea.View {
	v := tea.NewView(a.render())
	v.AltScreen = true
	v.ReportFocus = true
	if a.mouseOn() {
		v.MouseMode = tea.MouseModeCellMotion
	}
	v.WindowTitle = a.title
	if v.WindowTitle == "" {
		v.WindowTitle = a.windowTitle()
	}
	return v
}

func (a *App) render() string {
	l := a.look
	switch {
	case a.tooSmall():
		return strings.Join(screens.TooSmall(l, a.cols, a.rows, MinCols, MinRows), "\n")
	case a.report != nil:
		retry := time.Duration(0)
		switch {
		case a.startErr != nil:
			retry = max(a.nextCheck.Sub(a.now()), 0)
		case !a.status.NextRetry.IsZero():
			retry = max(a.status.NextRetry.Sub(a.now()), 0)
		}
		return strings.Join(screens.Render(screens.View{
			Look: l, Hints: a.hintsFor(keys.Startup), Sel: a.pick, RetryIn: retry, Cols: a.cols, Rows: a.rows,
		}, *a.report), "\n")
	}
	body := a.body(a.bodyHeight())
	lines := make([]string, 0, a.rows)
	lines = append(lines, a.header())
	lines = append(lines, body...)
	lines = append(lines, a.footer()...)
	if a.bar != nil {
		lines = append(lines, a.barLines()...)
	}
	page := strings.Join(lines, "\n")
	for _, d := range a.dialogs {
		page = dialog.Overlay(l, a.cols, a.rows, page, d.Frame(l, a.cols, a.rows))
	}
	return page
}

func (a *App) body(h int) []string {
	if a.inMemories() {
		return a.view().Render(a.env(), a.cols, h)
	}
	switch {
	case a.snap == nil:
		return screens.RenderEmpty(a.look, screens.Empty{Title: "Reading the workspace"}, a.cols, h)
	case a.snap.Len() == 0:
		return screens.RenderEmpty(a.look, a.emptyWorkspace(), a.cols, h)
	}
	d := a.frame()
	env := a.env()
	switch d.Frame {
	case detail.Overlay:
		return a.panelLines(d)
	case detail.Bottom:
		return append(a.view().Render(env, a.cols, h-d.H), a.panelLines(d)...)
	case detail.Side:
		list := a.view().Render(env, a.cols-d.W, h)
		side := a.panelLines(d)
		for i := range list {
			list[i] += side[i]
		}
		return list
	case detail.Hidden:
	}
	return a.view().Render(env, a.cols, h)
}

func (a *App) emptyWorkspace() screens.Empty {
	newKey := false
	for _, b := range a.km.Active(keys.View) {
		newKey = newKey || slices.Contains(b.Keys, "n")
	}
	return screens.EmptyWorkspace(a.workspaceName(), newKey)
}
