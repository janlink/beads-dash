package ui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/detail"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

const (
	memListMaxCols = 40
	memKeyMaxCols  = 40
)

// memoriesView is view slot 5. The memories are not issues, so the shell
// pre-empts the actions that need an issue and the view keeps its own state
// in the app's memState.
type memoriesView struct{ a *App }

func (*memoriesView) Name() string { return "memories" }

func (v *memoriesView) Context() keys.Context {
	m := v.a.mem
	if m != nil && m.focus && v.a.previewShown() {
		if _, ok := m.current(); ok {
			return keys.MemoryPreview
		}
	}
	return keys.Memories
}

func (*memoriesView) Visible(Env) []string { return nil }

func (*memoriesView) Has(Env, string) bool { return false }

func (*memoriesView) Scroll(int) {}

func (*memoriesView) At(int) (string, bool) { return "", false }

func (v *memoriesView) Handle(act keys.Action, _ Env) (tea.Cmd, bool) {
	if isNav(act) {
		v.a.memNavigate(act)
		return nil, true
	}
	return nil, false
}

func (v *memoriesView) Scope(w int) string {
	a, m := v.a, v.a.mem
	if !m.loaded || w < 8 {
		return ""
	}
	marker := "⌕"
	if a.look.Glyphs.Tier == theme.TierASCII {
		marker = "/"
	}
	total := len(m.list)
	prime := model.CompactCount(model.PrimeChars(m.list))
	count := fmt.Sprintf("%d", total)
	if m.query.Active() {
		count = fmt.Sprintf("%d/%d", len(m.shown()), total)
	}
	for _, tail := range []string{" · " + prime + " chars in bd prime", " · " + prime, ""} {
		head := count
		if m.query.Active() {
			head = marker + " " + m.query.Text() + " · " + count
		}
		s := head + tail
		if ansi.StringWidth(s) <= w {
			return s
		}
	}
	if m.query.Active() {
		return rows.MidCut(marker+" "+m.query.Text()+" · "+count, w, a.look.Glyphs.Ellipsis)
	}
	return count
}

// memLayout is how the body divides between the list and the preview.
type memLayout struct {
	listW, listH   int
	prevW, prevH   int
	side, previews bool
}

func memLayoutFor(w, h int) memLayout {
	if w >= memSideCols {
		lw := min(w/2, memListMaxCols)
		return memLayout{listW: lw, listH: h, prevW: w - lw, prevH: h, side: true, previews: true}
	}
	return memLayout{listW: w, listH: h}
}

func (v *memoriesView) Render(env Env, w, h int) []string {
	m, l := v.a.mem, env.Look
	m.rowKeys = make([]string, h)
	lay := memLayoutFor(w, h)
	m.prevX, m.prevY = lay.origin(w, h)
	switch {
	case !m.loaded && m.err != nil:
		return screens.RenderEmpty(l, screens.Empty{
			Title: "The memories could not be read.", Body: firstLine(m.err),
			Hints: []keys.Hint{{Key: "r", Desc: "retry"}, {Key: "!", Desc: "details"}},
		}, w, h)
	case !m.loaded:
		return screens.RenderEmpty(l, screens.Empty{Title: "Reading the memories"}, w, h)
	case len(m.list) == 0:
		return screens.RenderEmpty(l, screens.EmptyMemories, w, h)
	}
	shown := m.shown()
	if len(shown) == 0 {
		return screens.RenderEmpty(l, screens.Empty{
			Title: fmt.Sprintf("No memory matches %q · Esc clear", m.query.Text()),
		}, w, h)
	}
	if env.Framed {
		return v.panels(env, shown, lay, w, h)
	}
	list := v.listLines(env, shown, lay.listW, lay.listH)
	if !lay.previews {
		return list
	}
	cur, _ := m.current()
	prev := v.previewLines(env, cur, lay)
	if lay.side {
		out := make([]string, h)
		for i := range out {
			out[i] = list[i] + prev[i]
		}
		return out
	}
	return append(list, prev...)
}

// origin is where the preview starts in the body; a layout without one puts
// it past the corner.
func (lay memLayout) origin(w, h int) (x, y int) {
	switch {
	case !lay.previews:
		return w, h
	case lay.side:
		return lay.listW, 0
	}
	return 0, lay.listH
}

func (v *memoriesView) listLines(env Env, shown []model.Memory, w, h int) []string {
	a, m, l := v.a, v.a.mem, env.Look
	m.listPage = h
	ci := slices.Index(memKeys(shown), m.cursor)
	if ci >= 0 {
		if ci < m.top {
			m.top = ci
		}
		if ci >= m.top+h {
			m.top = ci - h + 1
		}
	}
	m.top = min(max(m.top, 0), max(len(shown)-h, 0))
	kw := 0
	for _, x := range shown {
		kw = max(kw, ansi.StringWidth(x.Key))
	}
	kw = min(kw, memKeyMaxCols, max((w-rows.GutterWidth)/2, 8))
	terms := m.query.Terms()
	out := make([]string, 0, h)
	for i := m.top; i < len(shown) && len(out) < h; i++ {
		x := shown[i]
		row := rows.Row{ID: x.Key, Current: x.Key == m.cursor, Marked: m.marks[x.Key], Changed: a.memChanged(x.Key)}
		body := v.rowBody(l, x, kw, w-rows.GutterWidth, row.Current, terms)
		m.rowKeys[len(out)] = x.Key
		out = append(out, env.Rows.Gutter(row)+body)
	}
	for len(out) < h {
		out = append(out, l.Fit("", w))
	}
	return out
}

func (v *memoriesView) rowBody(l look.Look, x model.Memory, kw, w int, sel bool, terms []model.MatchTerm) string {
	paint := l.Paint
	if sel {
		paint = l.PaintSel
	}
	key := ansi.Truncate(oneLineText(x.Key), kw, l.Glyphs.Ellipsis)
	pad := strings.Repeat(" ", max(kw-ansi.StringWidth(key), 0))
	room := max(w-kw-2, 0)
	snippet := ansi.Truncate(model.FirstLine(x.Content), room, l.Glyphs.Ellipsis)
	line := paintMatches(paint, theme.Strong, key, terms) + paint(theme.Text, pad+"  ") +
		paintMatches(paint, theme.Dim, snippet, terms)
	return l.Fit(line, w)
}

// paintMatches paints s in role and the occurrences of the terms in the match
// role; cell widths do not change.
func paintMatches(paint func(theme.Role, string) string, role theme.Role, s string, terms []model.MatchTerm) string {
	if len(terms) == 0 || s == "" {
		return paint(role, s)
	}
	runes := []rune(s)
	lower := make([]rune, len(runes))
	for i, r := range runes {
		lower[i] = unicode.ToLower(r)
	}
	marked := make([]bool, len(runes))
	hit := false
	for _, t := range terms {
		needle := []rune(t.Text)
		hay := runes
		if !t.Exact {
			needle = []rune(strings.ToLower(t.Text))
			hay = lower
		}
		n := len(needle)
		for i := 0; n > 0 && i+n <= len(hay); {
			if slices.Equal(hay[i:i+n], needle) {
				for j := i; j < i+n; j++ {
					marked[j] = true
				}
				hit = true
				i += n
				continue
			}
			i++
		}
	}
	if !hit {
		return paint(role, s)
	}
	var b strings.Builder
	for i := 0; i < len(runes); {
		j := i
		for j < len(runes) && marked[j] == marked[i] {
			j++
		}
		r := role
		if marked[i] {
			r = theme.Match
		}
		b.WriteString(paint(r, string(runes[i:j])))
		i = j
	}
	return b.String()
}

// previewLines draws the preview for lay: a header with the key and size,
// then the content, scrolled.
func (v *memoriesView) previewLines(env Env, cur model.Memory, lay memLayout) []string {
	a, m, l := v.a, v.a.mem, env.Look
	w, h := lay.prevW, lay.prevH
	lead, inner := "", w
	var out []string
	if lay.side {
		lead = l.Paint(theme.Border, l.Glyphs.Vertical) + " "
		inner = w - 2
	} else {
		out = append(out, l.Rule(w, nil, nil))
	}
	out = append(out, lead+l.Fit(a.memHeader(cur, inner, m.focus), inner))
	for _, s := range v.previewPage(env, cur, inner, max(h-len(out), 1)) {
		out = append(out, lead+l.Fit(s, inner))
	}
	return out[:h]
}

// previewPage is the page of the preview body that is scrolled into view, w
// cells wide and page rows tall; its last row says how much more there is.
func (v *memoriesView) previewPage(env Env, cur model.Memory, w, page int) []string {
	a, m, l := v.a, v.a.mem, env.Look
	body := a.memBody(cur, w)
	m.prevPage = page
	m.scroll = min(m.scroll, max(len(body)-page, 0))
	shown := body[min(m.scroll, len(body)):min(m.scroll+page, len(body))]
	more := len(body) - m.scroll - page
	out := make([]string, page)
	for i := range out {
		if i < len(shown) {
			out[i] = shown[i]
		}
		if i == page-1 && more > 0 {
			out[i] = l.Paint(theme.Faint, fmt.Sprintf("%s %d more", l.Glyphs.Ellipsis, more+1))
		}
	}
	return out
}

// panels draws the list and the preview as framed panels, a blank column
// apart; the one with the keys is focused.
func (v *memoriesView) panels(env Env, shown []model.Memory, lay memLayout, w, h int) []string {
	m, l := v.a.mem, env.Look
	listW := w
	if lay.previews {
		listW = lay.listW
	}
	body := max(h-2, 1)
	list := v.listLines(env, shown, max(look.PanelInner(listW)-1, 1), body)
	for i := range list {
		list[i] += " "
	}
	m.rowKeys = append([]string{""}, m.rowKeys[:h-1]...)
	lp := look.Panel{Title: "Memories", Aside: look.Word(theme.Dim, itoa(len(shown))), Focused: env.Focused && !m.focus}
	out := l.Frame(lp, listW, h, list)
	if !lay.previews {
		return out
	}
	cur, _ := m.current()
	pw := w - listW - 1
	inner := max(look.PanelInner(pw)-2, 1)
	page := v.previewPage(env, cur, inner, body)
	for i, s := range page {
		page[i] = " " + l.Fit(s, inner) + " "
	}
	lines, chars := model.MemorySize(cur.Content)
	size := fmt.Sprintf("%d %s · %d chars", lines, plural(lines, "line", "lines"), chars)
	pp := look.Panel{Title: oneLineText(cur.Key), Aside: look.Word(theme.Dim, size), Focused: env.Focused && m.focus}
	prev := l.Frame(pp, pw, h, page)
	for i := range out {
		out[i] += " " + prev[i]
	}
	return out
}

// memHeader is the preview's title row: the key and the size of the content.
func (a *App) memHeader(mem model.Memory, w int, focused bool) string {
	l := a.look
	lines, chars := model.MemorySize(mem.Content)
	size := fmt.Sprintf(" · %d %s · %d chars", lines, plural(lines, "line", "lines"), chars)
	keyRole := theme.Strong
	if focused {
		keyRole = theme.Primary
	}
	key := ansi.Truncate(oneLineText(mem.Key), max(w-ansi.StringWidth(size), 4), l.Glyphs.Ellipsis)
	return paintMatches(l.Paint, keyRole, key, a.mem.query.Terms()) + l.Paint(theme.Dim, size)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// memBody is the content of mem wrapped at w cells: rendered as markdown, or
// as written while the source is asked for or a search is on, so the hits show.
func (a *App) memBody(mem model.Memory, w int) []string {
	m := a.mem
	source := m.source || m.query.Active()
	k := memMDKey{key: mem.Key, content: mem.Content, w: w, source: source, look: a.lookGen, query: m.query.Text()}
	if lines, ok := m.md[k]; ok {
		return lines
	}
	var lines []string
	if source {
		lines = memSource(a.look, mem.Content, w, m.query.Terms())
	} else {
		lines = detail.Markdown(a.look, mem.Content, w)
	}
	if len(m.md) >= 64 {
		clear(m.md)
	}
	m.md[k] = lines
	return lines
}

func memSource(l look.Look, content string, w int, terms []model.MatchTerm) []string {
	text := strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
	if text == "" {
		return nil
	}
	var out []string
	for _, x := range strings.Split(ansi.Wrap(text, max(w, 1), ""), "\n") {
		out = append(out, paintMatches(l.Paint, theme.Text, x, terms))
	}
	return out
}
