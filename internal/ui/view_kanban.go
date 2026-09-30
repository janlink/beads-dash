package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

const (
	kanbanMinCol = 30
	kanbanMaxCol = 64
	// kanbanGroupedBelow is the width under which the columns become one
	// grouped list.
	kanbanGroupedBelow = 80
)

type kanbanSpot struct{ col, row int }

type kanbanSpan struct{ x0, x1, col int }

type kanbanHit struct {
	x0, x1, y int
	col       int
	id        string
}

// Kanban is the board: one column per presentation status, paged, with one
// line per card. Below 80 cells the columns become sections of one list.
type Kanban struct {
	built struct {
		snap  *model.Snapshot
		scope string
		ok    bool
	}
	cols []model.KanbanColumn
	spot map[string]kanbanSpot
	flat []string
	// memo is the card each column last had the cursor on.
	memo map[model.PresentationStatus]string
	// col is the column the cursor rests on while it belongs to colOwner:
	// an empty column holds no card to tell it by.
	col      int
	colOwner string
	start    int

	win     window
	grouped bool
	w, h    int
	lastCur string
	hits    []kanbanHit
	spans   []kanbanSpan
	label   scopeInfo
}

// NewKanban returns the Kanban view.
func NewKanban() *Kanban { return &Kanban{} }

// Name implements View.
func (*Kanban) Name() string { return "kanban" }

// Scope implements View.
func (v *Kanban) Scope(w int) string { return v.label.fit(w) }

// Context implements View.
func (*Kanban) Context() keys.Context { return keys.View }

func (v *Kanban) ensure(env Env) {
	if env.Snap == nil || env.Matches == nil {
		v.built.ok = false
		v.cols, v.spot, v.flat, v.memo = nil, nil, nil, nil
		return
	}
	scope := env.Scope.Key()
	if v.built.ok && v.built.snap == env.Snap && v.built.scope == scope {
		return
	}
	v.built.snap, v.built.scope, v.built.ok = env.Snap, scope, true
	v.cols = model.BuildKanban(env.Snap, env.Statuses, env.Scope, env.Matches)
	v.spot = make(map[string]kanbanSpot, env.Matches.Len())
	v.flat = v.flat[:0]
	for c, col := range v.cols {
		for r, id := range col.IDs {
			v.spot[id] = kanbanSpot{c, r}
			v.flat = append(v.flat, id)
		}
	}
	v.col = min(v.col, max(len(v.cols)-1, 0))
	kept := map[model.PresentationStatus]string{}
	for _, col := range v.cols {
		id := v.memo[col.Status]
		if s, ok := v.spot[id]; ok && v.cols[s.col].Status == col.Status {
			kept[col.Status] = id
		}
	}
	v.memo = kept
}

func (v *Kanban) has(id string) bool {
	_, ok := v.spot[id]
	return ok
}

// place is where the cursor rests: the column and the row in it, -1 when the
// column holds no current card.
func (v *Kanban) place(env Env) (col, row int) {
	if len(v.cols) == 0 {
		return -1, -1
	}
	if v.colOwner == env.Current {
		col = min(v.col, len(v.cols)-1)
		return col, slices.Index(v.cols[col].IDs, env.Current)
	}
	if s, ok := v.spot[env.Current]; ok {
		return s.col, s.row
	}
	return min(v.col, len(v.cols)-1), -1
}

// Sync implements Syncer: it remembers the card of the current column.
func (v *Kanban) Sync(env Env) {
	v.ensure(env)
	v.label = scopeInfo{}
	if env.Matches != nil {
		v.label = scopeInfoFor(env, len(v.flat), true)
	}
	if s, ok := v.spot[env.Current]; ok {
		v.memo[v.cols[s.col].Status] = env.Current
	}
}

// Visible implements View: the cards column by column.
func (v *Kanban) Visible(env Env) []string {
	v.ensure(env)
	return v.flat
}

// Has implements View.
func (v *Kanban) Has(env Env, id string) bool {
	v.ensure(env)
	return v.has(id)
}

func (v *Kanban) moveCard(env Env, col, row int) {
	id := v.cols[col].IDs[row]
	v.col, v.colOwner = col, id
	v.memo[v.cols[col].Status] = id
	env.Act.SetCurrent(id)
}

func (v *Kanban) moveColumn(env Env, to int) {
	if to < 0 || to >= len(v.cols) {
		return
	}
	col := v.cols[to]
	if len(col.IDs) == 0 {
		v.col, v.colOwner = to, env.Current
		return
	}
	row := slices.Index(col.IDs, v.memo[col.Status])
	v.moveCard(env, to, max(row, 0))
}

// Handle implements View.
func (v *Kanban) Handle(a keys.Action, env Env) (tea.Cmd, bool) {
	v.ensure(env)
	if len(v.cols) == 0 {
		return nil, false
	}
	col, row := v.place(env)
	switch {
	case isNav(a):
		v.navigate(env, a, col, row)
		return nil, true
	case a == keys.Left || a == keys.Right:
		v.step(env, col, a == keys.Right)
		return nil, true
	}
	return nil, false
}

func (v *Kanban) navigate(env Env, a keys.Action, col, row int) {
	if v.grouped {
		lr := make([]listRow, len(v.flat))
		for i, id := range v.flat {
			lr[i] = listRow{id, true}
		}
		if to := moveTo(lr, slices.Index(v.flat, env.Current), a, max(v.h, 1)); to >= 0 {
			s := v.spot[v.flat[to]]
			v.moveCard(env, s.col, s.row)
		}
		return
	}
	ids := v.cols[col].IDs
	lr := make([]listRow, len(ids))
	for i, id := range ids {
		lr[i] = listRow{id, true}
	}
	if to := moveTo(lr, row, a, max(v.h-2, 1)); to >= 0 {
		v.moveCard(env, col, to)
	}
}

// step moves to the neighbouring column; the grouped list skips empty ones.
func (v *Kanban) step(env Env, col int, right bool) {
	d := -1
	if right {
		d = 1
	}
	for to := col + d; to >= 0 && to < len(v.cols); to += d {
		if !v.grouped || len(v.cols[to].IDs) > 0 {
			v.moveColumn(env, to)
			return
		}
	}
}

// Render implements View.
func (v *Kanban) Render(env Env, w, h int) []string {
	v.w, v.h, v.lastCur = w, h, env.Current
	v.hits, v.spans = v.hits[:0], v.spans[:0]
	v.ensure(env)
	if env.Matches == nil {
		return screens.RenderEmpty(env.Look, screens.EmptyReady, w, h)
	}
	if len(v.flat) == 0 {
		e := screens.Empty{Title: "No issues to show.", Body: "Every issue is closed and closed issues are hidden."}
		if env.Scope.Active() {
			e = screens.EmptyScopeMatch(env.Scope.Query(), env.Matches.HiddenClosed)
		}
		return screens.RenderEmpty(env.Look, e, w, h)
	}
	v.grouped = w < kanbanGroupedBelow
	if v.grouped {
		return v.renderGrouped(env, w, h)
	}
	return v.renderColumns(env, w, h)
}

func (v *Kanban) heading(env Env, c model.KanbanColumn, w int, current bool, extra string) string {
	l := env.Look
	name := c.Status.String()
	role := theme.Strong
	if current {
		role = theme.Primary
		if l.Palette.Depth() == theme.DepthNone {
			name = "[" + name + "]"
		}
	}
	rule := l.Glyphs.Rule
	count := " " + itoa(len(c.IDs)) + " "
	hint, hintW := "", 0
	if extra != "" {
		hintW = ansi.StringWidth(extra) + 1
		hint = l.Paint(theme.Dim, " "+extra)
	}
	room := max(w-2-ansi.StringWidth(count)-hintW, 1)
	name = ansi.Truncate(name, room, l.Glyphs.Ellipsis)
	head := l.Paint(theme.Rule, rule+" ") + l.Paint(role, name) + l.Paint(theme.Dim, count)
	rest := max(w-ansi.StringWidth(head)-hintW, 0)
	return l.Fit(head+l.Paint(theme.Rule, strings.Repeat(rule, rest))+hint, w)
}

func (v *Kanban) card(env Env, id string, w int) string {
	return env.Rows.Line(w, rows.Row{ID: id, Current: id == env.Current, Marked: env.Marked(id), Changed: env.Changed(id)}, v.Name(), env.Rows.Kanban(id))
}

func (v *Kanban) renderGrouped(env Env, w, h int) []string {
	type gr struct {
		key string
		col int
		id  string
	}
	var order []gr
	var keysList []string
	for c, col := range v.cols {
		k := "\x00" + col.Status.String()
		order = append(order, gr{k, c, ""})
		keysList = append(keysList, k)
		for _, id := range col.IDs {
			order = append(order, gr{id, c, id})
			keysList = append(keysList, id)
		}
	}
	cur := env.Current
	if _, ok := v.spot[cur]; !ok {
		col, _ := v.place(env)
		if col >= 0 {
			cur = "\x00" + v.cols[col].Status.String()
		}
	}
	first := v.win.layout(keysList, cur, h)
	col, _ := v.place(env)
	out := make([]string, h)
	for i := range out {
		if first+i >= len(order) {
			out[i] = env.Look.Fit("", w)
			continue
		}
		g := order[first+i]
		if g.id == "" {
			out[i] = v.heading(env, v.cols[g.col], w, g.col == col, "")
			continue
		}
		out[i] = v.card(env, g.id, w)
		v.hits = append(v.hits, kanbanHit{0, w - 1, i, g.col, g.id})
	}
	return out
}

func (v *Kanban) renderColumns(env Env, w, h int) []string {
	l := env.Look
	n := len(v.cols)
	k := max(min(n, (w+1)/(kanbanMinCol+1)), 1)
	colW := min(kanbanMaxCol, (w+1)/k-1)
	cur, curRow := v.place(env)
	v.start = min(max(v.start, cur-k+1), max(cur, 0))
	v.start = min(max(v.start, 0), n-k)
	per := max(h-2, 1)
	blocks := make([][]string, 0, k)
	x := 0
	for c := v.start; c < v.start+k; c++ {
		col := v.cols[c]
		extra := ""
		switch {
		case c == v.start && v.start > 0:
			extra = kanbanHint(l.Glyphs, v.start, true)
		case c == v.start+k-1 && c < n-1:
			extra = kanbanHint(l.Glyphs, n-1-c, false)
		}
		lines := make([]string, 0, h)
		lines = append(lines, v.heading(env, col, colW, c == cur, extra))
		ref := slices.Index(col.IDs, v.memo[col.Status])
		if c == cur && curRow >= 0 {
			ref = curRow
		}
		page := max(ref, 0) / per
		if len(col.IDs) == 0 {
			lines = append(lines, l.Fit(l.Paint(theme.Dim, "  (empty)"), colW))
		}
		for i := page * per; i < min(len(col.IDs), (page+1)*per); i++ {
			id := col.IDs[i]
			v.hits = append(v.hits, kanbanHit{x, x + colW - 1, len(lines), c, id})
			lines = append(lines, v.card(env, id, colW))
		}
		for len(lines) < h-1 {
			lines = append(lines, l.Fit("", colW))
		}
		lines = append(lines, l.Fit("  "+v.dots(env, (len(col.IDs)+per-1)/per, page), colW))
		blocks = append(blocks, lines)
		v.spans = append(v.spans, kanbanSpan{x, x + colW - 1, c})
		x += colW + 1
	}
	out := make([]string, h)
	for y := range out {
		parts := make([]string, len(blocks))
		for i, b := range blocks {
			parts[i] = b[y]
		}
		out[y] = l.Fit(strings.Join(parts, " "), w)
	}
	return out
}

// kanbanHint says how many columns lie off screen on one side.
func kanbanHint(g theme.Glyphs, n int, left bool) string {
	ascii := g.Tier == theme.TierASCII
	switch {
	case left && ascii:
		return "<" + itoa(n)
	case left:
		return "‹" + itoa(n)
	case ascii:
		return itoa(n) + ">"
	}
	return itoa(n) + "›"
}

// dots is the page indicator, empty for a single page.
func (v *Kanban) dots(env Env, pages, page int) string {
	if pages < 2 {
		return ""
	}
	g := env.Look.Glyphs
	var b strings.Builder
	for p := range pages {
		if p == page {
			b.WriteString(env.Look.Paint(theme.Primary, g.Live))
		} else {
			b.WriteString(env.Look.Paint(theme.Dim, g.Stale))
		}
	}
	return b.String()
}

// Scroll implements View: the grouped list scrolls; columns page.
func (v *Kanban) Scroll(n int) {
	if v.grouped {
		v.win.scroll(n)
	}
}

// At implements View.
func (v *Kanban) At(y int) (string, bool) {
	if v.grouped {
		k, ok := v.win.at(y)
		if _, isCard := v.spot[k]; !ok || !isCard {
			return "", false
		}
		return k, true
	}
	return v.AtXY(0, y)
}

// AtXY implements Pointer: a click on a card selects it, one on a column
// puts the cursor there.
func (v *Kanban) AtXY(x, y int) (string, bool) {
	for _, h := range v.hits {
		if y == h.y && x >= h.x0 && x <= h.x1 {
			s := v.spot[h.id]
			v.col, v.colOwner = h.col, h.id
			if s.col < len(v.cols) {
				v.memo[v.cols[s.col].Status] = h.id
			}
			return h.id, true
		}
	}
	for _, sp := range v.spans {
		if x >= sp.x0 && x <= sp.x1 {
			v.col, v.colOwner = sp.col, v.lastCur
		}
	}
	return "", false
}
