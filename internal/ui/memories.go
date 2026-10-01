package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
	"github.com/janlink/beads-dash/internal/ui/state"
)

const (
	memSlot   = 4
	memPrefix = "mem:"
	// memSideCols is the width from which the preview sits beside the list;
	// below it, Enter opens the preview full screen.
	memSideCols   = 72
	issueOnlyHint = "issue action: switch to 1-4"
)

type memReadMsg struct {
	seq  int
	list []model.Memory
	err  error
}

// memState is the Memories view's data. Memories are not part of the
// snapshot: they are read lazily while the view is visible.
type memState struct {
	list    []model.Memory
	loaded  bool
	tried   bool
	reading bool
	want    bool
	seq     int
	err     error
	// commit and polled are what the engine reported when the last read
	// began; a different value asks for another read.
	commit string
	polled time.Time

	cursor string
	marks  map[string]bool
	query  model.MemoryQuery
	// gone counts the keys the last read found forgotten by someone else;
	// ownGone are the keys this session forgot.
	gone    int
	ownGone map[string]bool
	// follow is the key the cursor jumps to once a read lists it.
	follow string

	focus  bool
	source bool
	scroll int
	top    int

	md      map[memMDKey][]string
	rowKeys []string
	// listPage and prevPage are the rows of the list and of the preview body
	// from the last draw; prevX and prevY are where the preview starts in the
	// body.
	listPage, prevPage int
	prevX, prevY       int
}

type memMDKey struct {
	key, content string
	w            int
	source       bool
	look         int
	query        string
}

func newMemState() *memState {
	return &memState{marks: map[string]bool{}, ownGone: map[string]bool{}, md: map[memMDKey][]string{}}
}

// inMemories reports whether the Memories view is the current one.
func (a *App) inMemories() bool {
	if a.slot != memSlot || a.mem == nil {
		return false
	}
	_, ok := a.views[memSlot].(*memoriesView)
	return ok
}

// shown lists the memories the query lets through, in key order.
func (m *memState) shown() []model.Memory {
	if !m.query.Active() {
		return m.list
	}
	return m.query.Filter(m.list)
}

func (m *memState) get(key string) (model.Memory, bool) {
	for _, x := range m.list {
		if x.Key == key {
			return x, true
		}
	}
	return model.Memory{}, false
}

func memKeys(ms []model.Memory) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Key
	}
	return out
}

// settle keeps the cursor on a key of the shown list after the list or the
// query changed: it stays, else moves to the nearest survivor of old order.
func (m *memState) settle(oldShown []string) {
	shown := memKeys(m.shown())
	if slices.Contains(shown, m.cursor) {
		return
	}
	m.scroll = 0
	if next := state.NearestSurvivor(oldShown, m.cursor, func(k string) bool { return slices.Contains(shown, k) }); next != "" {
		m.cursor = next
		return
	}
	m.cursor = ""
	if len(shown) > 0 {
		m.cursor = shown[0]
	}
}

// memSync starts a read when the view is visible and the engine saw a change
// since the last one, or one was asked for.
func (a *App) memSync() tea.Cmd {
	m := a.mem
	if m == nil {
		return nil
	}
	if !a.inMemories() {
		m.polled = a.status.LastSuccess
		return nil
	}
	if a.eng == nil || a.report != nil || m.reading {
		return nil
	}
	st := a.status
	due := m.want || !m.tried ||
		(st.Commit != "" && st.Commit != m.commit) ||
		(!st.LastSuccess.IsZero() && !st.LastSuccess.Equal(m.polled))
	if !due {
		return nil
	}
	return a.memRead()
}

func (a *App) memRead() tea.Cmd {
	m := a.mem
	m.reading, m.tried, m.want = true, true, false
	m.commit, m.polled = a.status.Commit, a.status.LastSuccess
	m.seq++
	seq, eng := m.seq, a.eng
	return func() tea.Msg {
		var list []model.Memory
		err := eng.Do(context.Background(), func(ctx context.Context, c bd.Client) error {
			var err error
			list, err = c.Memories(ctx)
			return err
		})
		return memReadMsg{seq: seq, list: list, err: err}
	}
}

func (a *App) onMemRead(msg memReadMsg) tea.Cmd {
	m := a.mem
	if msg.seq != m.seq {
		return nil
	}
	m.reading = false
	if msg.err != nil {
		m.err = msg.err
		return nil
	}
	m.err = nil
	oldShown := memKeys(m.shown())
	old := m.list
	list := slices.Clone(msg.list)
	model.SortMemories(list)
	m.list = list
	var cmd tea.Cmd
	if m.loaded {
		ch := model.DiffMemories(old, list)
		gone := 0
		for _, k := range ch.Removed {
			if !m.ownGone[k] {
				gone++
			}
		}
		m.gone = gone
		if len(ch.Changed) > 0 {
			ids := make([]string, len(ch.Changed))
			for i, k := range ch.Changed {
				ids[i] = memPrefix + k
			}
			a.hl.Trigger(a.now(), ids, nil)
			cmd = a.hlSchedule()
		}
	}
	clear(m.ownGone)
	m.loaded = true
	for k := range m.marks {
		if _, ok := m.get(k); !ok {
			delete(m.marks, k)
		}
	}
	if m.follow != "" {
		if _, ok := m.get(m.follow); ok {
			m.cursor, m.follow = m.follow, ""
		}
	}
	m.settle(oldShown)
	a.notifyMemoryDialogs()
	return cmd
}

// memoryWatcher is implemented by dialogs that react to a fresh read of the
// memories, such as the memory dialog watching the key it edits.
type memoryWatcher interface{ Memories() }

func (a *App) notifyMemoryDialogs() {
	for _, d := range slices.Clone(a.dialogs) {
		if w, ok := d.(memoryWatcher); ok {
			w.Memories()
		}
	}
}

func (a *App) memChanged(key string) bool { return a.hl.Live(memPrefix+key, a.now()) }

// memChangedCount is how many memory highlights are live.
func (a *App) memChangedCount() int {
	n := 0
	for _, id := range a.hl.IDs(a.now()) {
		if strings.HasPrefix(id, memPrefix) {
			n++
		}
	}
	return n
}

func (m *memState) setQuery(text string) {
	old := memKeys(m.shown())
	m.query = model.ParseMemoryQuery(text)
	m.settle(old)
	m.top = 0
}

func (m *memState) current() (model.Memory, bool) {
	if m.cursor == "" {
		return model.Memory{}, false
	}
	return m.get(m.cursor)
}

func (a *App) memNavigate(act keys.Action) {
	m := a.mem
	shown := m.shown()
	rows := make([]listRow, len(shown))
	for i := range rows {
		rows[i] = listRow{key: shown[i].Key, sel: true}
	}
	from := slices.Index(memKeys(shown), m.cursor)
	to := moveTo(rows, from, act, max(m.listPage, 1))
	if to < 0 || to >= len(shown) || shown[to].Key == m.cursor {
		return
	}
	m.cursor = shown[to].Key
	m.scroll = 0
}

func (a *App) previewShown() bool { return a.cols >= memSideCols }

// memAct runs the actions of the Memories view; it reports whether it took
// the action.
func (a *App) memAct(act keys.Action) (tea.Cmd, bool) {
	m := a.mem
	switch act { //nolint:exhaustive // the action set is open: the shell handles the rest
	case keys.NavDown, keys.NavUp, keys.NavFirst, keys.NavLast, keys.NavHalfDown, keys.NavHalfUp, keys.NavPageDown, keys.NavPageUp:
		if m.focus {
			m.scroll = memScrolled(m.scroll, act, m.previewPage())
		} else {
			a.memNavigate(act)
		}
	case keys.Mark:
		if k := m.cursor; k != "" {
			if m.marks[k] {
				delete(m.marks, k)
			} else {
				m.marks[k] = true
			}
		}
	case keys.Open:
		if a.previewShown() {
			a.hint = issueOnlyHint
			return nil, true
		}
		if mem, ok := m.current(); ok {
			a.pushDialog(&memPreviewDialog{a: a, key: mem.Key})
		}
	case keys.FocusNext:
		if a.previewShown() {
			if _, ok := m.current(); ok {
				m.focus = !m.focus
				m.scroll = 0
			}
		}
	case keys.Markdown:
		m.source = !m.source
	case keys.OpenFilter:
		a.hint = "filters do not apply to memories"
	case keys.MemoryNew:
		return a.openMemory("", false), true
	case keys.MemoryEdit:
		if mem, ok := m.current(); ok {
			return a.openMemory(mem.Key, true), true
		}
		a.hint = "no memory to edit"
	case keys.MemoryForget:
		return a.openForget(), true
	case keys.MemoryCopy:
		return a.copyMemory(), true
	case keys.Close:
		if _, layered := a.sess.Top(); layered {
			return nil, false
		}
		switch {
		case m.focus:
			m.focus = false
		case len(m.marks) > 0:
			clear(m.marks)
		case m.query.Active():
			m.setQuery("")
		}
	case keys.New, keys.Edit, keys.ChangeStatus, keys.ChangePriority, keys.ChangeAssignee, keys.ChangeLabels,
		keys.CloseReopen, keys.MoveLeft, keys.MoveRight, keys.DetailToggle, keys.Export:
		a.hint = issueOnlyHint
	case keys.Left, keys.Right, keys.ScrollLeft, keys.ScrollRight, keys.ScrollHalfLeft, keys.ScrollHalfRight:
	default:
		return nil, false
	}
	return nil, true
}

func memScrolled(at int, act keys.Action, page int) int {
	switch act { //nolint:exhaustive // only the scrolling actions matter here
	case keys.NavDown:
		at++
	case keys.NavUp:
		at--
	case keys.NavFirst:
		at = 0
	case keys.NavLast:
		at = 1 << 30
	case keys.NavHalfDown:
		at += max(page/2, 1)
	case keys.NavHalfUp:
		at -= max(page/2, 1)
	case keys.NavPageDown:
		at += max(page, 1)
	case keys.NavPageUp:
		at -= max(page, 1)
	}
	return max(at, 0)
}

// previewPage is the height of the docked preview's body from the last draw.
func (m *memState) previewPage() int { return max(m.prevPage, 1) }

// copyMemory copies the content of the current memory.
func (a *App) copyMemory() tea.Cmd {
	mem, ok := a.mem.current()
	if !ok {
		a.hint = "no memory to copy"
		return nil
	}
	return a.copy("memory "+mem.Key, mem.Content)
}

func (a *App) copyMemoryByKey(key string) tea.Cmd {
	mem, ok := a.mem.get(key)
	if !ok {
		a.hint = "no memory to copy"
		return nil
	}
	return a.copy("memory "+key, mem.Content)
}

// memRefresh asks for a re-read after a write or a retry.
func (a *App) memRefresh() {
	if a.mem != nil {
		a.mem.want = true
	}
}

// memChips are the footer chips of the Memories view.
func (a *App) memChips() []ruleItem {
	m := a.mem
	var items []ruleItem
	chip := func(prio int, role theme.Role, text string) {
		items = append(items, ruleItem{seg: look.Word(role, text), prio: prio})
	}
	g := a.look.Glyphs
	if n := len(m.marks); n > 0 {
		chip(0, theme.Primary, fmt.Sprintf("%s %d marked", g.Mark, n))
	}
	if n := a.memChangedCount(); n > 0 {
		chip(2, theme.Changed, fmt.Sprintf("%s %d changed", g.Change, n))
	}
	if m.gone > 0 {
		chip(3, theme.Warning, fmt.Sprintf("%d forgotten elsewhere", m.gone))
	}
	if m.loaded && m.err != nil {
		chip(3, theme.Warning, g.Stale+" stale")
	}
	if a.status.Slow {
		chip(3, theme.Warning, "slow")
	}
	return items
}

// memClick moves the cursor to the memory under the pointer, or focuses the
// preview when the click lands on it.
func (a *App) memClick(x, y int) {
	m := a.mem
	if x >= m.prevX && y >= m.prevY && a.previewShown() {
		if _, ok := m.current(); ok {
			m.focus = true
		}
		return
	}
	m.focus = false
	if y >= 0 && y < len(m.rowKeys) && m.rowKeys[y] != "" {
		m.cursor = m.rowKeys[y]
		m.scroll = 0
	}
}

// memWheel scrolls the preview when the pointer is over it, else moves the
// cursor.
func (a *App) memWheel(x, y, n int) {
	m := a.mem
	if x >= m.prevX && y >= m.prevY && a.previewShown() {
		m.scroll = max(m.scroll+n, 0)
		return
	}
	act := keys.NavDown
	if n < 0 {
		act = keys.NavUp
	}
	for range max(n, -n) {
		a.memNavigate(act)
	}
}
