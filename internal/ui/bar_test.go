package ui

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/testgolden"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/state"
)

type memHistory struct{ lines []string }

func (m *memHistory) History() []string { return slices.Clone(m.lines) }

func (m *memHistory) AppendHistory(e string) error {
	m.lines = append(m.lines, e)
	return nil
}

func treeApp(t testing.TB, cols, rows int) *App {
	t.Helper()
	return viewApp(t, plain, cols, rows, "tree", false)
}

func runLine(a *App, line string) {
	press(a, ":")
	typeText(a, line)
	press(a, "enter")
}

func TestSearchNarrowsKeepsThenClears(t *testing.T) {
	a := treeApp(t, 100, 30)
	a.sess.SetCurrent("ws-7mt")
	press(a, "/")
	if !a.barOpen() || a.context() != keys.Bar {
		t.Fatalf("search bar not open: context %v", a.context())
	}
	typeText(a, "guest")
	vis := a.view().Visible(a.env())
	if !slices.Contains(vis, "ws-4k2.2") || slices.Contains(vis, "ws-9qe") {
		t.Fatalf("view did not narrow live: %v", vis)
	}
	if cur := a.sess.Current(); cur == "ws-7mt" || !a.view().Has(a.env(), cur) {
		t.Fatalf("current %q is outside the narrowed view", cur)
	}
	if out := screen(a); !strings.Contains(out, "guest") || !strings.Contains(out, "/") {
		t.Fatalf("bar not drawn:\n%s", out)
	}

	press(a, "esc")
	if a.bar != nil || a.scope.Query() != "guest" {
		t.Fatalf("Esc must close the bar and keep the search: bar %v query %q", a.bar, a.scope.Query())
	}
	press(a, "esc")
	if a.scope.Active() {
		t.Fatalf("second Esc must clear the search, query %q", a.scope.Query())
	}
	if a.sess.Current() != "ws-7mt" {
		t.Errorf("the issue that left the scope must return, current %q", a.sess.Current())
	}
}

func TestSearchEnterKeepsAndLettersGoToInput(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, "/")
	typeText(a, "qj fg")
	if a.quitting || a.scope.Query() != "qj fg" {
		t.Fatalf("letters must reach the input: quitting %v query %q", a.quitting, a.scope.Query())
	}
	press(a, "enter")
	if a.bar != nil || a.scope.Query() != "qj fg" {
		t.Errorf("Enter keeps the search: bar %v query %q", a.bar, a.scope.Query())
	}
}

func TestSearchArrowsAndCtrlNMoveTheCurrentIssue(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, "/")
	typeText(a, "a")
	vis := a.view().Visible(a.env())
	a.sess.SetCurrent(vis[0])
	press(a, "ctrl+n")
	if a.sess.Current() != vis[1] {
		t.Errorf("Ctrl+N: current %q, want %q", a.sess.Current(), vis[1])
	}
	press(a, "down")
	if a.sess.Current() != vis[2] {
		t.Errorf("Down: current %q, want %q", a.sess.Current(), vis[2])
	}
	press(a, "ctrl+p", "up")
	if a.sess.Current() != vis[0] {
		t.Errorf("Ctrl+P and Up: current %q, want %q", a.sess.Current(), vis[0])
	}
	if a.scope.Query() != "a" {
		t.Errorf("moving must not touch the text: %q", a.scope.Query())
	}
}

func TestSearchHistoryRecall(t *testing.T) {
	h := &memHistory{lines: []string{"/old one", ":view tree", "/status:open"}}
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.History = h; o.Views = IssueViews() })
	press(a, "/")
	press(a, "up")
	if a.bar.in.Text() != "status:open" || a.scope.Query() != "status:open" {
		t.Fatalf("Up on an empty bar recalls the newest search: %q", a.bar.in.Text())
	}
	press(a, "up")
	if a.bar.in.Text() != "old one" {
		t.Fatalf("second Up: %q", a.bar.in.Text())
	}
	press(a, "down", "down")
	if a.bar.in.Text() != "" {
		t.Fatalf("Down past the newest restores the draft: %q", a.bar.in.Text())
	}
	typeText(a, "zz")
	cmd := send(a, keyMsg("enter"))
	runAll(cmd)
	if got := h.lines[len(h.lines)-1]; got != "/zz" {
		t.Errorf("history file got %q", got)
	}
}

func TestSearchTabCompletesFacetsAndIDs(t *testing.T) {
	a := treeApp(t, 100, 30)
	tests := []struct{ typed, want string }{
		{"status:op", "status:open"},
		{"type:fea", "type:feature"},
		{"@al", "@alice"},
		{"assignee:bo", "assignee:bob"},
		{"pay ws-9", "pay ws-9qe"},
		{"-type:ch", "-type:chore"},
	}
	for _, tc := range tests {
		press(a, "/")
		press(a, "ctrl+u")
		settle(a)
		typeText(a, tc.typed)
		press(a, "tab")
		if got := a.bar.in.Text(); got != tc.want {
			t.Errorf("%q + Tab = %q, want %q", tc.typed, got, tc.want)
		}
		press(a, "esc")
		press(a, "esc")
	}
}

func TestMatchHighlightFollowsTheSearch(t *testing.T) {
	a := viewApp(t, truecolor, 100, 30, "tree", false)
	press(a, "/")
	typeText(a, "timeout")
	marked := func() bool {
		c := a.View().Content
		return strings.Contains(c, a.look.Paint(theme.Match, "timeout")) || strings.Contains(c, a.look.PaintSel(theme.Match, "timeout"))
	}
	if !marked() {
		t.Error("search term not highlighted in the title")
	}
	press(a, "ctrl+u")
	settle(a)
	if marked() {
		t.Error("highlight must go with the search text")
	}
}

func TestFilterBarTogglesFacetsAndKeepsFreeText(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, "/")
	typeText(a, "order")
	press(a, "enter")
	press(a, "f")
	if a.context() != keys.BarFilter {
		t.Fatalf("filter bar not open: %v", a.context())
	}
	press(a, "tab", "tab", "tab")
	if a.bar.col != int(model.GroupAssignee) {
		t.Fatalf("column %d", a.bar.col)
	}
	press(a, "down", "space")
	if got := a.scope.Query(); got != "order assignee:alice" {
		t.Fatalf("query %q", got)
	}
	press(a, "space")
	if got := a.scope.Query(); got != "order" {
		t.Fatalf("toggling back must restore the text: %q", got)
	}
	press(a, "shift+tab", "shift+tab", "shift+tab")
	press(a, "down", "down", "space")
	if q := a.scope.Query(); !strings.HasPrefix(q, "order status:") {
		t.Fatalf("status toggle: %q", q)
	}
	if !a.scope.Active() {
		t.Fatal("scope not active")
	}
}

func TestFilterBarFindNarrowsTheColumn(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, "f", "tab", "tab", "tab")
	typeText(a, "bo")
	opts := a.columnOpts(a.bar.col)
	if len(opts) != 1 || opts[0].value != "bob" {
		t.Fatalf("options %+v", opts)
	}
	press(a, "space")
	if a.scope.Query() != "assignee:bob" {
		t.Errorf("query %q", a.scope.Query())
	}
}

func TestFilterBarShowsAdvancedTokensAndKeepsThem(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, "/")
	typeText(a, "-type:bug parent:ws-4k2")
	press(a, "enter", "f")
	if out := screen(a); !strings.Contains(out, "advanced: -type:bug parent:ws-4k2") {
		t.Fatalf("advanced line missing:\n%s", out)
	}
	press(a, "tab", "tab", "tab", "down", "space")
	if got := a.scope.Query(); got != "-type:bug parent:ws-4k2 assignee:alice" {
		t.Errorf("query %q", got)
	}
}

func TestBarShrinksTheViewAndKeepsItLive(t *testing.T) {
	a := treeApp(t, 100, 30)
	before := a.bodyHeight()
	press(a, "f")
	if a.bodyHeight() >= before {
		t.Fatalf("body %d did not shrink from %d", a.bodyHeight(), before)
	}
	if n := len(lines(a)); n != 30 {
		t.Errorf("screen has %d lines", n)
	}
	press(a, "esc")
	if a.bodyHeight() != before {
		t.Errorf("body %d after closing, want %d", a.bodyHeight(), before)
	}
}

func TestCommandBarErrorsStayInPlace(t *testing.T) {
	a := treeApp(t, 100, 30)
	runLine(a, "kanban")
	if a.bar == nil || a.bar.err == nil {
		t.Fatal("a misparsed :kanban must leave the bar open with an error")
	}
	if a.bar.err.Pos != 0 || !strings.Contains(a.bar.err.Msg, `"kanban"`) {
		t.Errorf("error %+v", a.bar.err)
	}
	out := screen(a)
	if !strings.Contains(out, "^ no command or issue \"kanban\"") {
		t.Errorf("caret error not drawn:\n%s", out)
	}
	press(a, "ctrl+u")
	settle(a)
	typeText(a, "view")
	press(a, "enter")
	if a.bar == nil || a.bar.err == nil || !strings.Contains(a.bar.err.Msg, "needs an argument") {
		t.Fatalf("missing argument: %+v", a.bar)
	}
	press(a, "space")
	typeText(a, "nope")
	press(a, "enter")
	if a.bar.err == nil || a.bar.err.Pos != len(":view ")-1 {
		t.Errorf("error should point at the argument: %+v", a.bar.err)
	}
	press(a, "esc")
	if a.bar != nil {
		t.Error("Esc closes the command bar")
	}
}

func TestCommandView(t *testing.T) {
	a := treeApp(t, 100, 30)
	runLine(a, "view kanban")
	if a.slot != 2 || a.bar != nil {
		t.Errorf("slot %d bar %v", a.slot, a.bar)
	}
	runLine(a, "view 4")
	if a.slot != 3 {
		t.Errorf("slot %d", a.slot)
	}
	runLine(a, "view memories")
	if a.bar == nil || !strings.Contains(a.bar.err.Msg, "not available yet") {
		t.Errorf("unavailable view: %+v", a.bar)
	}
}

func TestCommandGoAndBareIDJumpAndBackReturns(t *testing.T) {
	a := treeApp(t, 100, 30)
	a.sess.SetCurrent("ws-7mt")
	runLine(a, "go ws-9qe")
	if a.sess.Current() != "ws-9qe" {
		t.Fatalf("current %q", a.sess.Current())
	}
	runLine(a, "2hz")
	if a.sess.Current() != "ws-2hz" {
		t.Fatalf("a bare ID resolves without the workspace prefix: %q", a.sess.Current())
	}
	press(a, "backspace")
	settle(a)
	if a.sess.Current() != "ws-9qe" {
		t.Errorf("back: %q", a.sess.Current())
	}
	press(a, "backspace")
	settle(a)
	if a.sess.Current() != "ws-7mt" {
		t.Errorf("back: %q", a.sess.Current())
	}
	runLine(a, "go ws-nope")
	if a.bar == nil || !strings.Contains(a.bar.err.Msg, "no issue") {
		t.Errorf("unknown issue: %+v", a.bar)
	}
}

func TestCommandGoClearsAScopeThatHidesTheIssue(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, "/")
	typeText(a, "guest")
	press(a, "enter")
	runLine(a, "go ws-9qe")
	if a.scope.Active() || a.sess.Current() != "ws-9qe" {
		t.Errorf("query %q current %q", a.scope.Query(), a.sess.Current())
	}
}

func TestCommandClearRefreshQuit(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, "/")
	typeText(a, "guest")
	press(a, "enter")
	runLine(a, "clear")
	if a.scope.Active() {
		t.Error(":clear left the scope")
	}
	runLine(a, "refresh")
	if a.bar != nil {
		t.Error(":refresh keeps the bar open")
	}
	runLine(a, "quit")
	if !a.quitting {
		t.Error(":quit did not quit")
	}
}

func TestCommandThemeAndGlyphsPreviewAndPersist(t *testing.T) {
	store := &memStore{}
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.Store = store; o.Views = IssueViews() })
	other := theme.Names()[1]
	press(a, ":")
	typeText(a, "theme "+other)
	cmd := send(a, keyMsg("enter"))
	runAll(cmd)
	if a.app.Theme.Name != other || a.choices.Theme != other {
		t.Errorf("theme %q choices %q", a.app.Theme.Name, a.choices.Theme)
	}
	if len(store.sets) != 1 || store.sets[0][0] != "theme" {
		t.Errorf("persisted %v", store.sets)
	}
	runLine(a, "glyphs safe")
	if a.choices.Glyphs != "safe" {
		t.Errorf("glyphs %q", a.choices.Glyphs)
	}
	runLine(a, "theme nonesuch")
	if a.bar == nil || a.bar.err.Pos != len(":theme ")-1 {
		t.Errorf("unknown theme: %+v", a.bar)
	}
}

func TestCommandHelp(t *testing.T) {
	a := treeApp(t, 120, 60)
	runLine(a, "help")
	if !isOpen[*helpDialog](a) {
		t.Fatal(":help opens no dialog")
	}
	if out := screen(a); !strings.Contains(out, ":theme <name>") || !strings.Contains(out, "Commands") {
		t.Errorf("command list missing:\n%s", out)
	}
	press(a, "esc")
	runLine(a, "help go")
	if out := screen(a); !strings.Contains(out, ":go <id>") || strings.Contains(out, ":theme") {
		t.Errorf("single command help wrong:\n%s", out)
	}
	press(a, "esc")
	runLine(a, "help nope")
	if a.bar == nil || a.bar.err == nil {
		t.Error(":help of an unknown command must be an error")
	}
}

func TestCommandTabCompletion(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, ":")
	typeText(a, "vi")
	press(a, "tab")
	if a.bar.in.Text() != "view " {
		t.Fatalf("verb completion: %q", a.bar.in.Text())
	}
	typeText(a, "gr")
	press(a, "tab")
	if a.bar.in.Text() != "view graph" {
		t.Fatalf("argument completion: %q", a.bar.in.Text())
	}
	press(a, "ctrl+u")
	settle(a)
	typeText(a, "g")
	press(a, "tab")
	first := a.bar.in.Text()
	press(a, "tab")
	second := a.bar.in.Text()
	if first == second || !slices.Contains([]string{"glyphs", "go"}, first) || !slices.Contains([]string{"glyphs", "go"}, second) {
		t.Errorf("Tab cycle: %q then %q", first, second)
	}
	press(a, "shift+tab")
	if a.bar.in.Text() != first {
		t.Errorf("Shift+Tab: %q, want %q", a.bar.in.Text(), first)
	}
	press(a, "ctrl+u")
	settle(a)
	typeText(a, "ws-9")
	press(a, "tab")
	if a.bar.in.Text() != "ws-9qe" {
		t.Errorf("ID completion: %q", a.bar.in.Text())
	}
	press(a, "ctrl+u")
	settle(a)
	typeText(a, "go ws-4k2.")
	press(a, "tab")
	if out := screen(a); !strings.Contains(out, "ws-4k2.1") {
		t.Errorf("candidates not listed:\n%s", out)
	}
}

func TestCommandHistoryPersistsAndRecalls(t *testing.T) {
	h := &memHistory{lines: []string{":view ready", "/foo"}}
	a := loaded(t, plain, 100, 30, snapOf(t), func(o *Options) { o.History = h; o.Views = IssueViews() })
	press(a, ":")
	press(a, "up")
	if a.bar.in.Text() != "view ready" {
		t.Fatalf("recalled %q", a.bar.in.Text())
	}
	cmd := send(a, keyMsg("enter"))
	runAll(cmd)
	if got := h.lines[len(h.lines)-1]; got != ":view ready" {
		t.Errorf("history %v", h.lines)
	}
	press(a, ":")
	typeText(a, "bogus words")
	press(a, "enter")
	if len(h.lines) != 3 {
		t.Errorf("a failed line must not be remembered: %v", h.lines)
	}
}

func TestFooterHintsEscClear(t *testing.T) {
	a := treeApp(t, 100, 30)
	if strings.Contains(lines(a)[29], "Esc clear") {
		t.Error("Esc clear without anything to clear")
	}
	press(a, "/")
	typeText(a, "guest")
	if strings.Contains(lines(a)[29-1], "Esc clear") {
		t.Error("the bar has its own hints")
	}
	press(a, "enter")
	if !strings.Contains(strings.Join(lines(a), "\n"), "Esc clear") {
		t.Errorf("no Esc clear hint with an active scope:\n%s", screen(a))
	}
}

func TestCurrentIssueLeavesTheScopeAndReturns(t *testing.T) {
	a := treeApp(t, 100, 30)
	a.sess.SetCurrent("ws-7mt")
	press(a, "/")
	typeText(a, "cart")
	if a.sess.Current() != "ws-7mt" {
		t.Fatalf("the current issue matches, it must stay: %q", a.sess.Current())
	}
	press(a, "ctrl+u")
	settle(a)
	typeText(a, "guest")
	moved := a.sess.Current()
	if moved == "ws-7mt" {
		t.Fatal("current did not leave")
	}
	press(a, "ctrl+u")
	settle(a)
	if a.sess.Current() != "ws-7mt" {
		t.Errorf("returns when the scope widens: %q", a.sess.Current())
	}

	typeText(a, "guest")
	press(a, "ctrl+n")
	chosen := a.sess.Current()
	press(a, "ctrl+u")
	settle(a)
	if a.sess.Current() != chosen {
		t.Errorf("a current issue the viewer moved must stay: %q, want %q", a.sess.Current(), chosen)
	}
}

type barState struct {
	query   string
	bar     bool
	visible []string
	current string
}

// probe records the state after every key and search tick the program handles.
type probe struct {
	*App
	mu  sync.Mutex
	log []barState
}

func (p *probe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	wasDirty := p.bar != nil && p.bar.dirty
	_, cmd := p.App.Update(msg)
	_, isKey := msg.(tea.KeyPressMsg)
	_, isTick := msg.(searchTickMsg)
	if (isKey || isTick && wasDirty && !p.bar.dirty) && p.snap != nil {
		p.mu.Lock()
		p.log = append(p.log, barState{
			query: p.scope.Query(), bar: p.bar != nil,
			visible: slices.Clone(p.view().Visible(p.env())), current: p.sess.Current(),
		})
		p.mu.Unlock()
	}
	return p, cmd
}

func TestTeatestSearchFlow(t *testing.T) {
	fake := fakeWorkspace(t)
	p := &probe{App: New(testOptions(plain, func(o *Options) {
		o.Client, o.Now = fake, time.Now
		o.Views = IssueViews()
		o.Settings.Settings.View = "tree"
	}))}
	tm := teatest.NewTestModel(t, p, teatest.WithInitialTermSize(100, 30))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Checkout: guest orders"))
	}, teatest.WithDuration(10*time.Second))

	for range 4 {
		tm.Send(keyMsg("j"))
	}
	tm.Send(keyMsg("/"))
	for _, r := range "guest" {
		tm.Send(keyMsg(string(r)))
	}
	time.Sleep(3 * searchDebounce)
	tm.Send(keyMsg("esc"))
	tm.Send(keyMsg("esc"))
	tm.Send(keyMsg("q"))
	tm.FinalModel(t, teatest.WithFinalTimeout(10*time.Second))

	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.log) != 14 {
		t.Fatalf("recorded %d steps", len(p.log))
	}
	typed, kept, cleared := p.log[10], p.log[11], p.log[12]
	if !typed.bar || typed.query != "guest" || !slices.Contains(typed.visible, "ws-4k2.2") || len(typed.visible) > 3 {
		t.Errorf("typing narrows the view live: %+v", typed)
	}
	if kept.bar || kept.query != "guest" || !slices.Equal(kept.visible, typed.visible) {
		t.Errorf("Esc closes the bar and keeps the search: %+v", kept)
	}
	if cleared.bar || cleared.query != "" || len(cleared.visible) <= 1 {
		t.Errorf("second Esc clears the search: %+v", cleared)
	}
	start := p.log[4].current
	if start == "" || typed.current == start {
		t.Errorf("current issue %q must leave to a survivor while the scope narrows from %q", typed.current, start)
	}
	if cleared.current != start {
		t.Errorf("current issue %q did not return to %q", cleared.current, start)
	}
	if !p.quitting || p.scope.Active() {
		t.Errorf("quitting %v scope %q", p.quitting, p.scope.Query())
	}
}

func TestBarGoldens(t *testing.T) {
	for _, s := range [][2]int{{60, 24}, {120, 40}} {
		size := fmt.Sprintf("%dx%d", s[0], s[1])
		t.Run("search "+size, func(t *testing.T) {
			a := treeApp(t, s[0], s[1])
			press(a, "/")
			typeText(a, "guest")
			testgolden.Equal(t, screen(a))
		})
		t.Run("filter "+size, func(t *testing.T) {
			a := treeApp(t, s[0], s[1])
			press(a, "/")
			typeText(a, "-type:chore order")
			press(a, "enter", "f", "tab", "tab", "tab", "down", "space")
			testgolden.Equal(t, screen(a))
		})
		t.Run("command hint "+size, func(t *testing.T) {
			a := treeApp(t, s[0], s[1])
			press(a, ":")
			typeText(a, "theme ")
			testgolden.Equal(t, screen(a))
		})
		t.Run("command error "+size, func(t *testing.T) {
			a := treeApp(t, s[0], s[1])
			runLine(a, "view nope")
			testgolden.Equal(t, screen(a))
		})
		t.Run("command candidates "+size, func(t *testing.T) {
			a := treeApp(t, s[0], s[1])
			press(a, ":")
			typeText(a, "g")
			press(a, "tab")
			testgolden.Equal(t, screen(a))
		})
	}
}

func TestHeaderScopeLabelIsMiddleTruncated(t *testing.T) {
	a := treeApp(t, 60, 24)
	press(a, "/")
	typeText(a, "status:open,in_progress type:bug,task,feature label:backend,frontend needle")
	press(a, "enter")
	head := lines(a)[0]
	if !strings.Contains(head, "...") || !strings.Contains(head, "status:") || !strings.Contains(head, "needle · 0/15 · closed hidden") {
		t.Errorf("label not cut out of the middle: %q", head)
	}
	testgolden.Equal(t, screen(a))
}

func TestBarKeysDoNotLeakToTheView(t *testing.T) {
	a := treeApp(t, 100, 30)
	a.sess.SetCurrent("ws-4k2")
	press(a, "/")
	typeText(a, "jkhl 1 3 t ?")
	if a.slot != 1 || isOpen[*helpDialog](a) || len(a.dialogs) != 0 {
		t.Errorf("keys reached the view: slot %d dialogs %d", a.slot, len(a.dialogs))
	}
	if a.sess.Current() == "" {
		t.Error("current lost")
	}
	if top, _ := a.sess.Top(); top != state.LayerBar {
		t.Errorf("top layer %v", top)
	}
}

func TestSearchIsDebounced(t *testing.T) {
	a := treeApp(t, 100, 30)
	full := len(a.view().Visible(a.env()))
	press(a, "/")
	for _, k := range []string{"g", "u", "e"} {
		press(a, k)
	}
	if a.scope.Query() != "" || len(a.view().Visible(a.env())) != full {
		t.Fatalf("scope applied before the pause: %q", a.scope.Query())
	}
	stale := a.bar.gen - 1
	send(a, searchTickMsg{stale})
	if a.scope.Query() != "" {
		t.Fatal("a stale tick applied the scope")
	}
	send(a, searchTickMsg{a.bar.gen})
	if a.scope.Query() != "gue" {
		t.Fatalf("scope %q after the pause", a.scope.Query())
	}
	press(a, "s", "t")
	press(a, "enter")
	if a.barOpen() || a.scope.Query() != "guest" {
		t.Errorf("Enter applies at once: open %v scope %q", a.barOpen(), a.scope.Query())
	}
}

func TestPasteGoesIntoTheBarWithoutNewlines(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, ":")
	send(a, tea.PasteMsg{Content: "go ws-9qe\n"})
	if got := a.bar.in.Text(); got != "go ws-9qe " {
		t.Errorf("bar holds %q", got)
	}
	press(a, "esc", "ctrl+p")
	send(a, tea.PasteMsg{Content: "timeout\r\nretries"})
	p := a.topDialog().(*picker)
	if got := p.in.Text(); got != "timeout retries" {
		t.Errorf("picker holds %q", got)
	}
}

func TestFilterBarShowsTheActiveColumnWithNeighboursWhenNarrow(t *testing.T) {
	a := treeApp(t, 60, 24)
	press(a, "f")
	out := screen(a)
	if !strings.Contains(out, "Status") || !strings.Contains(out, "Priority") || strings.Contains(out, "Labels") {
		t.Fatalf("first column shows itself and its right neighbours:\n%s", out)
	}
	press(a, "tab", "tab", "tab")
	out = screen(a)
	if strings.Contains(out, "Status") || !strings.Contains(out, "Assignee") || !strings.Contains(out, "Labels") {
		t.Errorf("window follows the active column:\n%s", out)
	}
	wide := treeApp(t, 120, 40)
	press(wide, "f")
	if out := screen(wide); !strings.Contains(out, "Status") || !strings.Contains(out, "Labels") {
		t.Errorf("wide bar shows every column:\n%s", out)
	}
}

func TestFilterCursorIsClampedWhenOptionsShrink(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, "f", "tab", "tab", "tab")
	for range 30 {
		press(a, "down")
	}
	a.bar.cur[a.bar.col] = 99
	press(a, "shift+tab")
	if n := len(a.columnOpts(int(model.GroupAssignee))); a.bar.cur[int(model.GroupAssignee)] >= max(n, 1) {
		t.Errorf("cursor %d beyond %d options", a.bar.cur[int(model.GroupAssignee)], n)
	}
}

func TestShiftTabAfterTheCommonPrefixStepGoesToTheLastCandidate(t *testing.T) {
	a := treeApp(t, 100, 30)
	press(a, ":")
	typeText(a, "ws-4")
	press(a, "tab")
	c := a.bar.comp
	if c == nil || c.at != -1 || len(c.cands) < 2 {
		t.Fatalf("no common-prefix step: %+v", c)
	}
	press(a, "shift+tab")
	if a.bar.comp.at != len(c.cands)-1 {
		t.Errorf("at %d, want %d", a.bar.comp.at, len(c.cands)-1)
	}
}

func TestHelpCompletionFollowsPrefixAndArgumentIndex(t *testing.T) {
	a := treeApp(t, 100, 30)
	if got := a.cmds.Complete("help cl", nil).Cands; !slices.Equal(got, []string{"clear"}) {
		t.Errorf("help cl completes to %q", got)
	}
	if got := a.cmds.Complete("help clear ", nil).Cands; len(got) != 0 {
		t.Errorf("a second argument completes to %q", got)
	}
}
