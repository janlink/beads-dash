package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

type readyKind uint8

const (
	readyGroup readyKind = iota
	readyIssue
	readyBlockedHead
	readyBlockedIssue
)

const readyBlockedKey = "\x00blocked"

type readyRow struct {
	kind   readyKind
	key    string
	id     string
	title  string
	count  int
	reason string
}

// Ready is bd's verdict on what can be worked on now: unassigned first, then
// assigned but not started, with a collapsed section of the blocked issues
// that names the blocker.
type Ready struct {
	blockedOpen bool

	built struct {
		snap  *model.Snapshot
		scope string
		open  bool
		ok    bool
	}
	list  model.ReadyList
	rows  []readyRow
	keys  []string
	sel   []listRow
	ids   []string
	index map[string]int
	all   map[string]bool

	win           window
	lastCur       string
	pos, posOwner string
	h             int
	label         scopeInfo
	note          string
}

// NewReady returns the Ready view.
func NewReady() *Ready { return &Ready{} }

// Name implements View.
func (*Ready) Name() string { return "ready" }

// Scope implements View.
func (v *Ready) Scope(w int) string { return v.label.fit(w) }

// Note implements Noter: the footer says how many containers were left out.
func (v *Ready) Note() string { return v.note }

// Context implements View.
func (*Ready) Context() keys.Context { return keys.View }

func (v *Ready) ensure(env Env) {
	if env.Snap == nil || env.Matches == nil {
		v.built.ok = false
		v.rows, v.keys, v.sel, v.ids, v.index, v.all = nil, nil, nil, nil, nil, nil
		v.list = model.ReadyList{}
		return
	}
	scope := env.Scope.Key()
	if v.built.ok && v.built.snap == env.Snap && v.built.scope == scope && v.built.open == v.blockedOpen {
		return
	}
	if !v.built.ok || v.built.snap != env.Snap || v.built.scope != scope {
		v.list = model.BuildReady(env.Snap, env.Statuses, env.Scope, env.Matches)
	}
	v.built.snap, v.built.scope, v.built.open, v.built.ok = env.Snap, scope, v.blockedOpen, true
	v.rows = v.rows[:0]
	v.all = map[string]bool{}
	add := func(r readyRow) { v.rows = append(v.rows, r) }
	group := func(title string, ids []string) {
		if len(ids) == 0 {
			return
		}
		add(readyRow{kind: readyGroup, key: "\x00g:" + title, title: title, count: len(ids)})
		for _, id := range ids {
			v.all[id] = true
			add(readyRow{kind: readyIssue, key: id, id: id, reason: env.Snap.ReadyReason(id)})
		}
	}
	group("Unassigned", v.list.Unassigned)
	group("Assigned, not started", v.list.Assigned)
	if n := len(v.list.Blocked); n > 0 {
		add(readyRow{kind: readyBlockedHead, key: readyBlockedKey, title: "Blocked", count: n})
		for _, b := range v.list.Blocked {
			v.all[b.ID] = true
			if !v.blockedOpen {
				continue
			}
			r := readyRow{kind: readyBlockedIssue, key: b.ID, id: b.ID}
			if b.First != "" {
				r.reason = waitsOn(env, b.First)
			}
			add(r)
		}
	}
	v.keys, v.sel, v.ids = v.keys[:0], v.sel[:0], v.ids[:0]
	v.index = make(map[string]int, len(v.rows))
	for i, r := range v.rows {
		v.keys = append(v.keys, r.key)
		v.sel = append(v.sel, listRow{r.key, r.kind != readyGroup})
		v.index[r.key] = i
		if r.id != "" {
			v.ids = append(v.ids, r.id)
		}
	}
}

func waitsOn(env Env, id string) string {
	if env.Look.Glyphs.Tier == theme.TierASCII {
		return "waits on " + id
	}
	return "⧗ waits on " + id
}

// Sync implements Syncer: it opens the blocked section when the current issue
// was moved into it from outside the view, and refreshes the scope label and
// the footer note.
func (v *Ready) Sync(env Env) {
	v.ensure(env)
	v.label, v.note = scopeInfo{}, ""
	if env.Matches != nil {
		v.label = scopeInfoFor(env, v.list.Len(), false)
		switch n := v.list.Containers; n {
		case 0:
		case 1:
			v.note = "1 container hidden"
		default:
			v.note = fmt.Sprintf("%d containers hidden", n)
		}
	}
	if env.Current == v.lastCur {
		return
	}
	v.lastCur = env.Current
	if v.blockedOpen || env.Current == "" || env.Snap == nil {
		return
	}
	for _, b := range v.list.Blocked {
		if b.ID == env.Current {
			v.blockedOpen = true
			v.ensure(env)
			return
		}
	}
}

func (v *Ready) cursor(env Env) (string, bool) {
	if v.pos != "" && v.posOwner == env.Current {
		if _, ok := v.index[v.pos]; ok {
			return v.pos, true
		}
	}
	if _, ok := v.index[env.Current]; ok && env.Current != "" {
		return env.Current, true
	}
	return "", false
}

// Visible implements View.
func (v *Ready) Visible(env Env) []string {
	v.ensure(env)
	return v.ids
}

// Has implements View: blocked issues count although the section is closed.
func (v *Ready) Has(env Env, id string) bool {
	v.ensure(env)
	return v.all[id]
}

func (v *Ready) move(env Env, i int) {
	r := v.rows[i]
	owner := r.id
	if r.kind == readyBlockedHead {
		owner = env.Current
	}
	v.pos, v.posOwner, v.lastCur = r.key, owner, owner
	if r.id != "" {
		env.Act.SetCurrent(r.id)
	}
}

// Handle implements View.
func (v *Ready) Handle(a keys.Action, env Env) (tea.Cmd, bool) {
	v.ensure(env)
	if len(v.rows) == 0 {
		return nil, false
	}
	i := -1
	if k, ok := v.cursor(env); ok {
		i = v.index[k]
	}
	if isNav(a) {
		if to := moveTo(v.sel, i, a, max(v.h, 1)); to >= 0 {
			v.move(env, to)
		}
		return nil, true
	}
	if i < 0 {
		return nil, false
	}
	r := v.rows[i]
	setOpen := func(open bool) {
		v.blockedOpen = open
		v.ensure(env)
		if j, ok := v.index[readyBlockedKey]; ok {
			v.move(env, j)
		}
	}
	switch a { //nolint:exhaustive // the view acts on its own subset
	case keys.Open:
		if r.kind == readyBlockedHead {
			setOpen(!v.blockedOpen)
			return nil, true
		}
	case keys.Right:
		if r.kind == readyBlockedHead {
			if v.blockedOpen && i+1 < len(v.rows) {
				v.move(env, i+1)
			} else {
				setOpen(true)
			}
		}
		return nil, true
	case keys.Left:
		switch r.kind {
		case readyBlockedHead:
			setOpen(false)
		case readyBlockedIssue:
			if j, ok := v.index[readyBlockedKey]; ok {
				v.move(env, j)
			}
		case readyGroup, readyIssue:
		}
		return nil, true
	}
	return nil, false
}

// Render implements View.
func (v *Ready) Render(env Env, w, h int) []string {
	v.h = h
	v.ensure(env)
	if env.Matches == nil {
		return screens.RenderEmpty(env.Look, screens.EmptyReady, w, h)
	}
	if len(v.rows) == 0 {
		e := screens.EmptyReady
		if env.Scope.Active() {
			e = screens.EmptyScopeMatch(env.Scope.Query(), env.Matches.HiddenClosed)
		}
		return screens.RenderEmpty(env.Look, e, w, h)
	}
	cur, _ := v.cursor(env)
	first := v.win.layout(v.keys, cur, h)
	cols := env.Rows.ReadyColumns(w)
	l := env.Look
	out := make([]string, h)
	for i := range out {
		idx := first + i
		if idx >= len(v.rows) {
			out[i] = l.Fit("", w)
			continue
		}
		r := v.rows[idx]
		isCur := r.key == cur
		switch r.kind {
		case readyGroup, readyBlockedHead:
			out[i] = v.head(env, r, w, isCur)
		case readyIssue, readyBlockedIssue:
			row := rows.Row{ID: r.id, Current: isCur, Marked: env.Marked(r.id), Changed: env.Changed(r.id)}
			body := env.Rows.Ready(rows.ReadyRow{ID: r.id, Cols: cols, Now: env.Now, Reason: r.reason, Pinned: r.kind == readyBlockedIssue})
			out[i] = env.Rows.Line(w, row, v.Name(), body)
		}
	}
	return out
}

func (v *Ready) head(env Env, r readyRow, w int, cur bool) string {
	l := env.Look
	g := l.Glyphs
	paint := l.Paint
	if cur {
		paint = l.PaintSel
	}
	text := fmt.Sprintf("%s (%d)", r.title, r.count)
	role := theme.Strong
	if r.kind == readyBlockedHead {
		fold := g.FoldClosed
		if v.blockedOpen {
			fold = g.FoldOpen
		}
		text = fold + " " + text
		role = theme.Dim
	}
	gutter := env.Rows.Gutter(rows.Row{Current: cur})
	return gutter + l.Fit(paint(role, text), w-rows.GutterWidth)
}

// Scroll implements View.
func (v *Ready) Scroll(n int) { v.win.scroll(n) }

// At implements View.
func (v *Ready) At(y int) (string, bool) {
	k, ok := v.win.at(y)
	if !ok {
		return "", false
	}
	i, ok := v.index[k]
	if !ok || v.rows[i].kind == readyGroup {
		return "", false
	}
	r := v.rows[i]
	if r.kind == readyBlockedHead {
		return "", false
	}
	v.pos, v.posOwner = k, r.id
	return r.id, true
}

// IssueViews returns the views that list issues, keyed by the slot they take.
func IssueViews() map[int]View {
	return map[int]View{1: NewOverview(), 2: NewTree(), 3: NewKanban(), 4: NewReady(), 6: NewGraph()}
}
