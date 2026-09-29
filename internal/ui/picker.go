package ui

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/dialog"
	"github.com/janlink/beads-dash/internal/ui/input"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

const pickerRows = 14

// pickerOpts configures an issue picker.
type pickerOpts struct {
	Title string
	// Multi lets Tab mark several issues; Done then gets every marked one.
	Multi bool
	// Valid greys out and refuses an issue that cannot be chosen, and says
	// why. Nil accepts every issue.
	Valid func(id string) (ok bool, why string)
	// Done receives the chosen IDs and reports whether the picker closes.
	Done func(a *App, ids []string) (tea.Cmd, bool)
}

type pickItem struct {
	id, title, text string
	closed          bool
	status          int
}

type pickerSource []pickItem

func (s pickerSource) String(i int) string { return s[i].text }
func (s pickerSource) Len() int            { return len(s) }

// picker is the issue picker dialog: a text field over the issues fuzzy
// matched by ID and title.
type picker struct {
	a          *App
	o          pickerOpts
	in         *input.Field
	items      pickerSource
	list       pickerSource
	showClosed bool
	res        []fuzzy.Match
	sel        int
	marked     []string
	note       string
}

func (a *App) newPicker(o pickerOpts) *picker {
	p := &picker{a: a, o: o, in: input.New("")}
	if a.snap != nil {
		for _, id := range a.snap.IDs() {
			is, _ := a.snap.Issue(id)
			title := oneLineText(is.Title)
			st := a.snap.Present(id, a.bds.Statuses).Status
			p.items = append(p.items, pickItem{
				id: id, title: title, text: id + " " + title,
				closed: st == model.Closed, status: look.StatusIndex(int(st)),
			})
		}
	}
	p.refilter()
	return p
}

func (a *App) openJumpPicker() {
	if a.snap == nil || a.snap.Len() == 0 {
		a.hint = "no issues to jump to"
		return
	}
	a.pushDialog(a.newPicker(pickerOpts{
		Title: "Jump to issue",
		Done: func(a *App, ids []string) (tea.Cmd, bool) {
			a.jumpTo(ids[0])
			return nil, true
		},
	}))
}

func oneLineText(s string) string { return strings.Join(strings.Fields(s), " ") }

// refilter recomputes the candidates, best match first; with an empty field
// every issue in snapshot order.
func (p *picker) refilter() {
	src := p.items
	if !p.showClosed {
		src = slices.DeleteFunc(slices.Clone(src), func(i pickItem) bool { return i.closed })
	}
	p.list = src
	if q := strings.TrimSpace(p.in.Text()); q != "" {
		p.res = fuzzy.FindFrom(q, src)
	} else {
		p.res = make([]fuzzy.Match, len(src))
		for i := range src {
			p.res[i] = fuzzy.Match{Index: i}
		}
	}
	p.sel = 0
	p.note = ""
}

func (*picker) Context() keys.Context { return keys.Picker }

func (p *picker) Type(m tea.KeyPressMsg) {
	if p.in.Update(m) {
		p.refilter()
	}
}

func (p *picker) Paste(text string) {
	if p.in.Paste(text) {
		p.refilter()
	}
}

func (p *picker) Update(tea.Msg) tea.Cmd { return nil }

func (p *picker) valid(id string) (bool, string) {
	if p.o.Valid == nil {
		return true, ""
	}
	return p.o.Valid(id)
}

func (p *picker) Handle(act keys.Action) (tea.Cmd, bool) {
	page := pickerRows - 1
	switch act { //nolint:exhaustive // the picker context binds only these
	case keys.Close:
		return nil, true
	case keys.NavDown:
		p.move(1)
	case keys.NavUp:
		p.move(-1)
	case keys.NavPageDown:
		p.move(page)
	case keys.NavPageUp:
		p.move(-page)
	case keys.PickerClosed:
		p.showClosed = !p.showClosed
		p.refilter()
	case keys.PickerMark:
		p.mark()
	case keys.Apply:
		return p.choose()
	}
	return nil, false
}

func (p *picker) move(n int) {
	if len(p.res) == 0 {
		return
	}
	p.sel = min(max(p.sel+n, 0), len(p.res)-1)
	p.note = ""
}

func (p *picker) selected() (string, bool) {
	if p.sel < 0 || p.sel >= len(p.res) {
		return "", false
	}
	return p.list[p.res[p.sel].Index].id, true
}

func (p *picker) mark() {
	id, ok := p.selected()
	if !ok || !p.o.Multi {
		return
	}
	if ok, why := p.valid(id); !ok {
		p.note = why
		return
	}
	if i := slices.Index(p.marked, id); i >= 0 {
		p.marked = slices.Delete(p.marked, i, i+1)
	} else {
		p.marked = append(p.marked, id)
	}
	p.move(1)
}

func (p *picker) choose() (tea.Cmd, bool) {
	ids := slices.Clone(p.marked)
	if len(ids) == 0 {
		id, ok := p.selected()
		if !ok {
			return nil, false
		}
		if ok, why := p.valid(id); !ok {
			p.note = why
			return nil, false
		}
		ids = []string{id}
	}
	return p.o.Done(p.a, ids)
}

func (p *picker) Frame(l look.Look, w, h int) dialog.Frame {
	capRows := pickerRows
	if dialog.FullScreen(w, h) {
		capRows = h - 2
	}
	capRows = max(capRows, 2)
	iw := w - 4
	if !dialog.FullScreen(w, h) {
		iw = min(dialog.MaxWidth, w-4) - 4
	}
	body := make([]string, 0, capRows)
	prompt := l.Paint(theme.Primary, ">") + " "
	body = append(body, prompt+p.in.View(l, theme.Strong, max(iw-2, 1), true))
	room := capRows - 1
	first := 0
	if p.sel >= room {
		first = p.sel - room + 1
	}
	for i := first; i < first+room && i < len(p.res); i++ {
		body = append(body, p.row(l, i, iw))
	}
	if len(p.res) == 0 {
		body = append(body, l.Paint(theme.Dim, "No issue matches."))
	}
	for len(body) < capRows {
		body = append(body, "")
	}
	aside := fmt.Sprintf("%d/%d", len(p.res), len(p.items))
	switch {
	case p.note != "":
		aside = p.note
	case p.showClosed:
		aside += " · closed shown"
	default:
		aside += " · closed hidden"
	}
	if n := len(p.marked); n > 0 {
		aside += fmt.Sprintf(" · %d marked", n)
	}
	return dialog.Frame{Title: p.o.Title, Aside: aside, Hints: p.hints(), Body: body}
}

func (p *picker) hints() []keys.Hint {
	hs := p.a.hintsFor(keys.Picker)
	if p.o.Multi {
		return hs
	}
	return slices.DeleteFunc(slices.Clone(hs), func(h keys.Hint) bool { return h.Key == "Tab" })
}

func (p *picker) row(l look.Look, i, w int) string {
	it := p.list[p.res[i].Index]
	ok, why := p.valid(it.id)
	cursor := i == p.sel
	paint := l.Paint
	if cursor {
		paint = l.PaintSel
	}
	mark := "  "
	if slices.Contains(p.marked, it.id) {
		mark = l.Glyphs.Mark + " "
	}
	role, idRole, statusRole := theme.Text, theme.Dim, theme.StatusRole(it.status)
	switch {
	case !ok:
		role, idRole, statusRole = theme.Faint, theme.Faint, theme.Faint
	case it.closed:
		role = theme.Dim
	}
	glyph := l.Glyphs.Status[it.status]
	idCells := p.idCells()
	hit := runeHits(it.text, p.res[i].MatchedIndexes)
	suffix := ""
	if !ok && why != "" {
		suffix = " " + oneLineText(why)
	}
	titleW := max(w-2-ansi.StringWidth(glyph)-1-idCells-1-ansi.StringWidth(suffix), 0)
	title := ansi.Truncate(it.title, titleW, l.Glyphs.Ellipsis)
	var b strings.Builder
	b.WriteString(paint(theme.Primary, mark))
	b.WriteString(paint(statusRole, glyph))
	b.WriteString(paint(theme.Text, " "))
	b.WriteString(paintMatch(paint, idRole, ansi.Truncate(it.id, idCells, l.Glyphs.Ellipsis), 0, hit))
	b.WriteString(paint(theme.Text, strings.Repeat(" ", max(idCells-ansi.StringWidth(it.id), 0)+1)))
	b.WriteString(paintMatch(paint, role, title, utf8.RuneCountInString(it.id)+1, hit))
	if suffix != "" {
		b.WriteString(paint(theme.Text, strings.Repeat(" ", max(titleW-ansi.StringWidth(title), 0))))
		b.WriteString(paint(theme.Dim, suffix))
	}
	return l.Fit(b.String(), w)
}

// runeHits turns the byte offsets fuzzy reports for text into rune positions.
func runeHits(text string, byteIdx []int) map[int]bool {
	hit := make(map[int]bool, len(byteIdx))
	for _, x := range byteIdx {
		if x >= 0 && x <= len(text) {
			hit[utf8.RuneCountInString(text[:x])] = true
		}
	}
	return hit
}

func (p *picker) idCells() int {
	n := 0
	for _, it := range p.list {
		n = max(n, ansi.StringWidth(it.id))
	}
	return min(n, 16)
}

// paintMatch paints the runes of s whose position in "id title" is in hit in
// the match role; off is where s starts in that text.
func paintMatch(paint func(theme.Role, string) string, role theme.Role, s string, off int, hit map[int]bool) string {
	var b strings.Builder
	i := off
	for _, r := range s {
		if hit[i] {
			b.WriteString(paint(theme.Match, string(r)))
		} else {
			b.WriteString(paint(role, string(r)))
		}
		i++
	}
	return b.String()
}
