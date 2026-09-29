package detail

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
	"github.com/janlink/beads-dash/internal/ui/rows"
)

// Section is one accordion section of the panel.
type Section int

// The sections in display order.
const (
	Description Section = iota
	Design
	Acceptance
	Notes
	Dependencies
	Children
	Details

	sectionCount
)

var sectionTitles = [sectionCount]string{
	Description: "Description", Design: "Design", Acceptance: "Acceptance",
	Notes: "Notes", Dependencies: "Dependencies", Children: "Children", Details: "Details",
}

// Title is the heading of the section.
func (s Section) Title() string { return sectionTitles[s] }

const (
	proseCap     = 12
	readingWidth = 100
	refCap       = 5
	childCap     = 30
	barW         = 8
	maxCachedMD  = 256
)

// Input is everything one frame of the panel is drawn from.
type Input struct {
	Look look.Look
	// Gen changes whenever Look does, which drops the cached markdown.
	Gen      int
	Snap     *model.Snapshot
	Statuses model.Statuses
	Rows     *rows.Renderer
	// ID is the issue shown; empty shows a hint instead.
	ID  string
	Now time.Time
	// Events are the events behind the issue's live change highlight, oldest
	// first; none when it is not highlighted.
	Events  []model.Event
	Frame   Frame
	W, H    int
	Focused bool
}

// mdKey identifies one rendered prose section: the issue as of its last
// update, the section, the width and the look it was drawn for.
type mdKey struct {
	gen, w  int
	id      string
	updated int64
	sec     Section
}

// maxMarkdownBytes bounds what goes into the markdown renderer; the rest of a
// longer text is cut off.
const maxMarkdownBytes = 16 << 10

// Job is one markdown rendering the panel wants. Run is pure and safe to call
// off the UI goroutine.
type Job struct {
	key  mdKey
	text string
	look look.Look
}

// Result is a finished Job.
type Result struct {
	key   mdKey
	lines []string
}

// Run renders the markdown.
func (j Job) Run() Result {
	return Result{j.key, renderMarkdown(j.look, j.text, j.key.w)}
}

// Panel is the state of the detail panel that outlives issues: which
// sections are open, the reading mode, the scroll position. It is not safe
// for concurrent use.
type Panel struct {
	open     [sectionCount]bool
	cursor   Section
	scroll   int
	source   bool
	expanded [sectionCount]bool
	forID    string

	starts    [sectionCount]int
	focusable [sectionCount]bool
	capped    [sectionCount]bool
	total     int
	viewRows  int

	// row is the target the cursor stands on in the cursor's section, -1 on the
	// section heading.
	row        int
	targets    [sectionCount][]target
	closedOpen bool
	reveal     bool

	gen      int
	md       map[mdKey][]string
	inflight map[mdKey]bool
}

// New returns a panel with Description, Dependencies and Children open.
func New() *Panel {
	p := &Panel{row: -1, md: map[mdKey][]string{}, inflight: map[mdKey]bool{}}
	p.open[Description], p.open[Dependencies], p.open[Children] = true, true, true
	for i := range p.starts {
		p.starts[i] = -1
	}
	return p
}

// Open reports whether a section is open.
func (p *Panel) Open(s Section) bool { return p.open[s] }

// Cursor is the section the section keys act on.
func (p *Panel) Cursor() Section { return p.cursor }

// Source reports whether prose shows as source instead of markdown.
func (p *Panel) Source() bool { return p.source }

// Scroll is the first content line shown.
func (p *Panel) Scroll() int { return p.scroll }

// ToggleSource switches between markdown and source.
func (p *Panel) ToggleSource() { p.source = !p.source }

// ScrollBy moves the content by n lines; the next frame clamps it.
func (p *Panel) ScrollBy(n int) { p.scroll = max(p.scroll+n, 0) }

// ScrollTo moves the content to line n.
func (p *Panel) ScrollTo(n int) { p.scroll = max(n, 0) }

// Page is the number of content lines visible in the last frame.
func (p *Panel) Page() int { return max(p.viewRows, 1) }

// End scrolls to the last line.
func (p *Panel) End() { p.scroll = max(p.total-p.viewRows, 0) }

// Next moves the cursor to the next section and scrolls it to the top.
func (p *Panel) Next() { p.step(1) }

// Prev moves the cursor to the previous section and scrolls it to the top.
func (p *Panel) Prev() { p.step(-1) }

func (p *Panel) step(d int) {
	for s := p.cursor + Section(d); s >= 0 && s < sectionCount; s += Section(d) {
		if p.focusable[s] {
			p.cursor, p.row = s, -1
			p.scroll = max(p.starts[s], 0)
			return
		}
	}
}

// Move steps the cursor through the issue rows of an open section, from its
// heading to the first row and back; it reports false at either end, where
// the caller scrolls instead.
func (p *Panel) Move(d int) bool {
	n := len(p.targets[p.cursor])
	switch {
	case d > 0 && p.row+1 < n:
		p.row++
	case d < 0 && p.row >= 0:
		p.row--
	default:
		return false
	}
	p.reveal = true
	return true
}

// Row is the issue the cursor stands on, if it stands on an issue row.
func (p *Panel) Row() (id string, ok bool) {
	t := p.targets[p.cursor]
	if p.row < 0 || p.row >= len(t) || t[p.row].fold {
		return "", false
	}
	return t[p.row].id, true
}

// OnFold reports whether the cursor stands on the closed-children row.
func (p *Panel) OnFold() bool {
	t := p.targets[p.cursor]
	return p.row >= 0 && p.row < len(t) && t[p.row].fold
}

// foldRow is the index of the closed-children row among the children rows.
func (p *Panel) foldRow() int {
	for i, t := range p.targets[Children] {
		if t.fold {
			return i
		}
	}
	return -1
}

// Enter acts on the cursor: on the closed-children row it opens or closes
// it, on a section heading it toggles the section; an issue row is left to
// the caller, see Row.
func (p *Panel) Enter() {
	switch {
	case p.OnFold():
		p.closedOpen = !p.closedOpen
	case p.row < 0:
		p.Toggle()
	}
}

// Expand opens the cursor's section, or lifts the line cap of an open one.
func (p *Panel) Expand() {
	if p.OnFold() {
		p.closedOpen = true
		return
	}
	switch {
	case !p.open[p.cursor]:
		p.open[p.cursor] = true
	case p.capped[p.cursor]:
		p.expanded[p.cursor] = true
	}
}

// Collapse closes the closed children when the cursor is on or among them,
// else the cursor's section.
func (p *Panel) Collapse() {
	if p.cursor == Children && p.closedOpen && p.row >= 0 {
		p.closedOpen = false
		p.row = p.foldRow()
		p.reveal = true
		return
	}
	p.open[p.cursor] = false
	p.row = -1
}

// Toggle opens a closed section, lifts the cap of a capped one and closes
// the rest.
func (p *Panel) Toggle() {
	if p.open[p.cursor] && (!p.capped[p.cursor] || p.expanded[p.cursor]) {
		p.open[p.cursor] = false
		return
	}
	p.Expand()
}

// ToggleAll closes every section, or opens them all when none is open.
func (p *Panel) ToggleAll() {
	anyOpen := false
	for s := range sectionCount {
		anyOpen = anyOpen || p.open[s]
	}
	for s := range sectionCount {
		p.open[s] = !anyOpen
	}
}

// Render draws the panel as exactly in.H lines of in.W cells.
func (p *Panel) Render(in Input) []string {
	if in.Gen != p.gen {
		p.gen = in.Gen
		clear(p.md)
		clear(p.inflight)
	}
	if in.ID != p.forID {
		p.forID = in.ID
		p.expanded = [sectionCount]bool{}
		p.scroll, p.cursor, p.row, p.closedOpen = 0, Description, -1, false
	}
	l := in.Look
	g := l.Glyphs
	innerW, innerH := in.W, in.H
	var top []string
	edge := ""
	switch in.Frame {
	case Bottom:
		role, label := theme.Dim, " Detail "
		if in.Focused {
			role, label = theme.Primary, " Detail (focused) "
		}
		rule := strings.TrimRight(g.Rule, " ")
		lead := strings.Repeat(rule, 2)
		fill := max(in.W-ansi.StringWidth(lead)-ansi.StringWidth(label), 0)
		top = []string{l.Paint(theme.Rule, lead) + l.Paint(role, label) + l.Paint(theme.Rule, strings.Repeat(rule, fill))}
		innerH--
	case Side:
		role := theme.Rule
		if in.Focused {
			role = theme.Primary
		}
		edge = l.Paint(role, g.Vertical)
		innerW -= ansi.StringWidth(g.Vertical)
	case Hidden, Overlay:
	}

	var lines []string
	is, ok := model.Issue{}, false
	if in.Snap != nil && in.ID != "" {
		var ip *model.Issue
		if ip, ok = in.Snap.Issue(in.ID); ok {
			is = *ip
		}
	}
	if !ok {
		lines = append([]string{"", " " + l.Paint(theme.Dim, "No issue selected.")}, make([]string, max(innerH-2, 0))...)
		lines = lines[:max(innerH, 0)]
	} else {
		lines = p.compose(in, &is, innerW, innerH)
	}
	out := make([]string, 0, in.H)
	out = append(out, top...)
	for _, s := range lines {
		out = append(out, edge+l.Fit(s, innerW))
	}
	for len(out) < in.H {
		out = append(out, edge+l.Fit("", innerW))
	}
	return out[:in.H]
}

// compose lays out header and sections for an innerW x innerH area and
// returns the visible lines.
func (p *Panel) compose(in Input, is *model.Issue, w, h int) []string {
	l := in.Look
	head := []string{p.headLine(in, is), " " + l.Paint(theme.Strong, ansi.Truncate(oneLine(is.Title), max(w-1, 0), l.Glyphs.Ellipsis))}
	var content []string
	p.starts = [sectionCount]int{-1, -1, -1, -1, -1, -1, -1}
	p.focusable = [sectionCount]bool{}
	p.capped = [sectionCount]bool{}
	p.targets = [sectionCount][]target{}
	if len(in.Events) > 0 {
		content = append(content, p.changedLine(in))
	}
	bw := max(w-3, 1)
	pw := proseWidth(w)
	for s := range sectionCount {
		b := p.section(in, is, s, bw, pw)
		if b.omit {
			continue
		}
		if !b.none {
			p.starts[s], p.focusable[s] = len(content), true
		}
		p.targets[s] = nil
		if p.open[s] {
			p.targets[s] = b.targets
		}
		content = append(content, p.sectionHead(in, s, b))
		if b.none || !p.open[s] {
			continue
		}
		for i, line := range b.body {
			prefix := "   "
			if in.Focused && s == p.cursor && p.row >= 0 && p.row < len(b.targets) && b.targets[p.row].line == i {
				prefix = l.Paint(theme.Primary, l.Glyphs.Band) + "  "
			}
			content = append(content, prefix+line)
		}
	}
	if !p.focusable[p.cursor] {
		for s := range sectionCount {
			if p.focusable[s] {
				p.cursor = s
				break
			}
		}
	}
	room := max(h-len(head), 0)
	p.row = min(p.row, len(p.targets[p.cursor])-1)
	if p.reveal {
		p.reveal = false
		line := max(p.starts[p.cursor], 0)
		if p.row >= 0 {
			line += 1 + p.targets[p.cursor][p.row].line
		}
		p.scroll = min(max(p.scroll, line-room+1), line)
	}
	p.total, p.viewRows = len(content), room
	p.scroll = min(max(p.scroll, 0), max(len(content)-room, 0))
	end := min(p.scroll+room, len(content))
	return append(head, content[p.scroll:end]...)
}

type block struct {
	omit    bool
	none    bool
	summary string
	body    []string
	targets []target
}

// target is a body line the cursor can stand on: a line naming another issue,
// or the closed-children row.
type target struct {
	id   string
	fold bool
	line int
}

func (p *Panel) sectionHead(in Input, s Section, b block) string {
	l := in.Look
	g := l.Glyphs
	title := sectionTitles[s]
	if b.none {
		return " " + l.Paint(theme.Faint, g.Bullet+" ") + l.Paint(theme.Dim, title) + "  " + l.Paint(theme.Faint, "none")
	}
	cur, role := " ", theme.Primary
	if in.Focused && s == p.cursor {
		cur, role = l.Paint(theme.Primary, g.Band), theme.Strong
	}
	mark := g.FoldClosed
	if p.open[s] {
		mark = g.FoldOpen
	}
	line := cur + l.Paint(theme.Dim, mark+" ") + l.Paint(role, title)
	if b.summary != "" {
		line += "  " + l.Paint(theme.Dim, b.summary)
	}
	return line
}

func (p *Panel) section(in Input, is *model.Issue, s Section, bw, pw int) block {
	switch s {
	case Description:
		return p.prose(in, is, s, is.Description, pw, false)
	case Design:
		return p.prose(in, is, s, is.Design, pw, true)
	case Acceptance:
		return p.prose(in, is, s, is.AcceptanceCriteria, pw, true)
	case Notes:
		return p.prose(in, is, s, is.Notes, pw, true)
	case Dependencies:
		return p.dependencies(in, is, bw)
	case Children:
		return p.children(in, is, bw)
	case Details, sectionCount:
	}
	return p.details(in, is, bw)
}

func (p *Panel) prose(in Input, is *model.Issue, s Section, text string, w int, omitEmpty bool) block {
	text = cleanProse(text)
	if text == "" {
		return block{omit: omitEmpty, none: true}
	}
	l := in.Look
	lines := p.markdown(in, mdKey{p.gen, w, is.ID, is.UpdatedAt.UnixNano(), s}, text)
	b := block{}
	first, _, _ := strings.Cut(text, "\n")
	n := strings.Count(text, "\n") + 1
	b.summary = fmt.Sprintf("%d lines · %s", n, strings.TrimSpace(oneLine(first)))
	if n == 1 {
		b.summary = strings.TrimSpace(oneLine(first))
	}
	if !p.open[s] {
		return b
	}
	b.summary = ""
	if s == Description {
		b.summary = "markdown"
		if p.source {
			b.summary = "source"
		}
	}
	if s == Description && len(lines) > proseCap {
		p.capped[s] = true
		if !p.expanded[s] {
			more := len(lines) - proseCap
			lines = append(lines[:proseCap:proseCap], l.Paint(theme.Primary, l.Glyphs.FoldClosed+" "+fmt.Sprintf("%d more lines", more)))
		}
	}
	b.body = lines
	return b
}

// cleanProse normalises line ends and cuts the text down to what the
// markdown renderer takes.
func cleanProse(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if len(text) > maxMarkdownBytes {
		cut := maxMarkdownBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "…"
	}
	return text
}

// proseWidth is the width prose wraps at inside an area w cells wide.
func proseWidth(w int) int { return min(max(w-3, 1), readingWidth) }

// innerWidth is the width of the panel's content area.
func innerWidth(in Input) int {
	if in.Frame == Side {
		return in.W - ansi.StringWidth(in.Look.Glyphs.Vertical)
	}
	return in.W
}

// markdown returns the rendered text, or its source wrapped at the same width
// until the renderer's result arrives; in source mode it is always the
// source.
func (p *Panel) markdown(in Input, k mdKey, text string) []string {
	l := in.Look
	if !p.source {
		if lines, ok := p.md[k]; ok {
			return lines
		}
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		for _, x := range strings.Split(ansi.Wrap(oneLine(line), k.w, ""), "\n") {
			out = append(out, l.Paint(theme.Dim, x))
		}
	}
	return out
}

// Plan lists the markdown renderings the panel needs for what it shows next
// and has not got yet; each is listed once until its result is applied.
func (p *Panel) Plan(in Input) []Job {
	if in.Gen != p.gen {
		p.gen = in.Gen
		clear(p.md)
		clear(p.inflight)
	}
	if p.source || in.Snap == nil || in.ID == "" {
		return nil
	}
	is, ok := in.Snap.Issue(in.ID)
	if !ok {
		return nil
	}
	w := proseWidth(innerWidth(in))
	var jobs []Job
	for s, text := range map[Section]string{Description: is.Description, Design: is.Design, Acceptance: is.AcceptanceCriteria, Notes: is.Notes} {
		text = cleanProse(text)
		k := mdKey{p.gen, w, is.ID, is.UpdatedAt.UnixNano(), s}
		if text == "" || p.inflight[k] {
			continue
		}
		if _, done := p.md[k]; done {
			continue
		}
		p.inflight[k] = true
		jobs = append(jobs, Job{k, text, in.Look})
	}
	return jobs
}

// Apply stores a finished rendering; one for a look that has since changed is
// dropped.
func (p *Panel) Apply(r Result) {
	delete(p.inflight, r.key)
	if r.key.gen != p.gen {
		return
	}
	if len(p.md) >= maxCachedMD {
		clear(p.md)
	}
	p.md[r.key] = r.lines
}

func renderMarkdown(l look.Look, text string, w int) []string {
	r, err := glamour.NewTermRenderer(glamour.WithStyles(theme.GlamourStyle(l.Palette, l.Glyphs)), glamour.WithWordWrap(w))
	if err != nil {
		return plain(l, text, w)
	}
	out, err := r.Render(text)
	if err != nil {
		return plain(l, text, w)
	}
	lines := strings.Split(strings.Trim(out, "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for i, line := range lines {
		keep := ansi.StringWidth(strings.TrimRight(ansi.Strip(line), " "))
		lines[i] = ansi.Truncate(line, keep, "")
	}
	return lines
}

func plain(l look.Look, text string, w int) []string {
	var out []string
	for _, x := range strings.Split(ansi.Wrap(text, w, ""), "\n") {
		out = append(out, l.Paint(theme.Text, x))
	}
	return out
}

func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || (r >= 0x7f && r <= 0x9f) {
			return ' '
		}
		return r
	}, s)
}
