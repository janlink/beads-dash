package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/command"
	"github.com/janlink/beads-dash/internal/ui/input"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/state"
)

type barKind uint8

const (
	barSearch barKind = iota
	barCommand
	barFilter
)

const (
	searchDebounce = 100 * time.Millisecond
	filterCols     = 5
	maxIDCandidate = 40
)

var filterNames = [filterCols]string{"Status", "Type", "Priority", "Assignee", "Labels"}

// bar is the docked search, filter or command bar. It sits under the footer
// while its layer is open.
type bar struct {
	kind barKind
	in   *input.Field
	rec  *recall
	err  *command.Error
	comp *completion

	// dirty is set while typed search text waits for its debounce tick; gen
	// tells the ticks apart.
	dirty bool
	gen   int

	col      int
	cur      [filterCols]int
	facets   model.Facets
	facetsOf *model.Snapshot
}

// completion is a Tab cycle in progress: text is what the field held after
// the last step, so any other edit ends the cycle.
type completion struct {
	start int
	cands []string
	at    int
	text  string
}

func (k barKind) context() keys.Context {
	switch k {
	case barCommand:
		return keys.BarCommand
	case barFilter:
		return keys.BarFilter
	case barSearch:
	}
	return keys.Bar
}

// barOpen reports whether a bar has the keys.
func (a *App) barOpen() bool {
	if a.bar == nil {
		return false
	}
	top, ok := a.sess.Top()
	return ok && top == state.LayerBar
}

func (a *App) openBar(k barKind) {
	if a.snap == nil && !a.inMemories() {
		a.hint = "nothing to search yet"
		return
	}
	b := &bar{kind: k, in: input.New("")}
	switch k {
	case barSearch:
		if a.inMemories() {
			b.in.Set(a.mem.query.Text())
		} else {
			b.in.Set(a.scope.Query())
		}
		b.rec = a.searchHist
	case barCommand:
		b.rec = a.commandHist
	case barFilter:
	}
	if b.rec != nil {
		b.rec.reset()
	}
	a.bar = b
	a.sess.Push(state.LayerBar)
}

// closeBar takes the bar down, keeping the scope it edited. A search worth
// remembering goes to the history.
func (a *App) closeBar() tea.Cmd {
	b := a.bar
	if b == nil {
		return nil
	}
	a.flushSearch()
	a.bar = nil
	a.sess.Remove(state.LayerBar)
	if b.kind == barSearch {
		return a.remember(searchMark, strings.TrimSpace(b.in.Text()), b.rec)
	}
	return nil
}

func (a *App) remember(mark, text string, r *recall) tea.Cmd {
	if text == "" {
		return nil
	}
	r.add(text)
	h := a.o.History
	if h == nil {
		return nil
	}
	return func() tea.Msg {
		_ = h.AppendHistory(mark + text)
		return nil
	}
}

func (a *App) barHeight() int {
	b := a.bar
	if b == nil {
		return 0
	}
	h := 1
	switch b.kind {
	case barCommand:
		h = 2
	case barFilter:
		h = 2 + a.filterRows()
		if len(model.SelectionOf(a.scope.Query()).Advanced) > 0 {
			h++
		}
	case barSearch:
	}
	return min(h, max(a.rows/2, 1))
}

func (a *App) filterRows() int { return min(max(a.rows/4, 3), 6) }

// barAct runs the actions of the bar contexts; it reports whether it took
// the action.
func (a *App) barAct(act keys.Action) (tea.Cmd, bool) {
	cmd, ok := a.barDo(act)
	a.clampCursors()
	return cmd, ok
}

// clampCursors keeps the option cursors of the filter bar inside their
// columns after the options changed.
func (a *App) clampCursors() {
	b := a.bar
	if b == nil || b.kind != barFilter {
		return
	}
	for c := range filterCols {
		b.cur[c] = min(b.cur[c], max(len(a.columnOpts(c))-1, 0))
	}
}

func (a *App) barDo(act keys.Action) (tea.Cmd, bool) {
	b := a.bar
	a.flushSearch()
	switch act { //nolint:exhaustive // the bar contexts bind only these
	case keys.BarAccept:
		return a.barAccept(), true
	case keys.BarIssueNext:
		a.stepIssue(keys.NavDown)
	case keys.BarIssuePrev:
		a.stepIssue(keys.NavUp)
	case keys.BarUp, keys.BarDown:
		a.barVertical(act)
	case keys.BarComplete:
		a.complete(false)
	case keys.BarCompleteBack:
		a.complete(true)
	case keys.BarToggle:
		a.toggleOption()
	case keys.BarColumnNext:
		b.col = (b.col + 1) % filterCols
	case keys.BarColumnPrev:
		b.col = (b.col + filterCols - 1) % filterCols
	default:
		return nil, false
	}
	return nil, true
}

func (a *App) barAccept() tea.Cmd {
	b := a.bar
	if b.kind == barCommand {
		return a.runCommandLine()
	}
	return a.closeBar()
}

// stepIssue moves the current issue behind the bar.
func (a *App) stepIssue(act keys.Action) {
	if _, ok := a.view().Handle(act, a.env()); !ok {
		a.navigate(act)
	}
}

func (a *App) barVertical(act keys.Action) {
	b := a.bar
	up := act == keys.BarUp
	switch b.kind {
	case barFilter:
		a.moveOption(up)
	case barCommand:
		a.recallStep(up)
	case barSearch:
		if b.rec.browsing() || (up && b.in.Text() == "") {
			a.recallStep(up)
			return
		}
		if up {
			a.stepIssue(keys.NavUp)
		} else {
			a.stepIssue(keys.NavDown)
		}
	}
}

func (a *App) recallStep(older bool) {
	b := a.bar
	var s string
	var ok bool
	if older {
		s, ok = b.rec.older(b.in.Text())
	} else {
		s, ok = b.rec.newer()
	}
	if !ok {
		return
	}
	b.in.Set(s)
	b.err, b.comp = nil, nil
	a.barEdited()
}

// typeKey hands a key no binding took to the text of the open bar or dialog.
func (a *App) typeKey(m tea.KeyPressMsg) tea.Cmd {
	if a.report != nil || a.tooSmall() {
		return nil
	}
	if d := a.topDialog(); d != nil {
		if t, ok := d.(typing); ok {
			return t.Type(m)
		}
		return nil
	}
	if !a.barOpen() {
		return nil
	}
	return a.barTyped(a.bar.in.Update(m))
}

// paste inserts pasted text into the open bar or dialog field.
func (a *App) paste(text string) tea.Cmd {
	if a.report != nil || a.tooSmall() {
		return nil
	}
	if d := a.topDialog(); d != nil {
		if t, ok := d.(pasting); ok {
			return t.Paste(text)
		}
		return nil
	}
	if !a.barOpen() {
		return nil
	}
	return a.barTyped(a.bar.in.Paste(text))
}

// barTyped reacts to an edit of the bar text: the search scope follows after
// a short pause, so a burst of keys narrows the view once.
func (a *App) barTyped(changed bool) tea.Cmd {
	if !changed {
		return nil
	}
	b := a.bar
	b.err, b.comp = nil, nil
	if b.rec != nil {
		b.rec.reset()
	}
	if b.kind == barFilter {
		b.cur = [filterCols]int{}
	}
	if b.kind != barSearch {
		return nil
	}
	b.dirty = true
	b.gen++
	gen := b.gen
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg { return searchTickMsg{gen} })
}

// typing is implemented by dialogs whose text field takes the keys the key
// map leaves over; pasting by those that take pasted text too.
type (
	typing  interface{ Type(tea.KeyPressMsg) tea.Cmd }
	pasting interface{ Paste(string) tea.Cmd }
)

// barEdited applies what the search bar text says to the scope.
func (a *App) barEdited() {
	if b := a.bar; b != nil && b.kind == barSearch {
		b.dirty = false
		if a.inMemories() {
			a.mem.setQuery(b.in.Text())
			return
		}
		a.applyScope(model.ParseScope(b.in.Text(), a.scope.ShowClosed()))
	}
}

// flushSearch applies typed search text that still waits for its tick.
func (a *App) flushSearch() {
	if b := a.bar; b != nil && b.dirty {
		a.barEdited()
	}
}

func (a *App) complete(back bool) {
	b := a.bar
	if b.kind == barFilter {
		return
	}
	text := b.in.Text()
	if c := b.comp; c != nil && c.text == text {
		n := len(c.cands)
		if back {
			if c.at < 0 {
				c.at = n
			}
			c.at = (c.at + n - 1) % n
		} else {
			c.at = (c.at + 1) % n
		}
		a.completeWith(c.start, c.cands[c.at])
		c.text = b.in.Text()
		return
	}
	comp := a.completions(text)
	switch len(comp.Cands) {
	case 0:
		b.comp = nil
	case 1:
		suffix := ""
		if b.kind == barCommand && a.takesArgs(comp.Cands[0]) && comp.Start == 0 {
			suffix = " "
		}
		a.completeWith(comp.Start, comp.Cands[0]+suffix)
		b.comp = nil
	default:
		runes := []rune(text)
		typed := string(runes[min(comp.Start, len(runes)):])
		c := &completion{start: comp.Start, cands: comp.Cands}
		if common := commonPrefix(comp.Cands); len([]rune(common)) > len([]rune(typed)) {
			c.at = -1
			a.completeWith(comp.Start, common)
		} else {
			if back {
				c.at = len(c.cands) - 1
			}
			a.completeWith(comp.Start, c.cands[c.at])
		}
		c.text = b.in.Text()
		b.comp = c
	}
}

func (a *App) takesArgs(verb string) bool {
	s, ok := a.cmds.Lookup(verb)
	return ok && s.Max != 0
}

func (a *App) completeWith(start int, cand string) {
	b := a.bar
	runes := []rune(b.in.Text())
	b.in.Set(string(runes[:min(start, len(runes))]) + cand)
	b.err = nil
	a.barEdited()
}

func commonPrefix(cands []string) string {
	p := []rune(cands[0])
	for _, c := range cands[1:] {
		i := 0
		for r := []rune(c); i < len(p) && i < len(r) && p[i] == r[i]; {
			i++
		}
		p = p[:i]
	}
	return string(p)
}

func (a *App) completions(text string) command.Completion {
	if a.bar.kind == barCommand {
		return a.cmds.Complete(text, func(prefix string) []string { return a.idsWith(prefix, maxIDCandidate) })
	}
	return a.searchCompletion(text)
}

// searchCompletion completes the last word of a search: a facet name, a
// facet value, or an issue ID.
func (a *App) searchCompletion(text string) command.Completion {
	if a.inMemories() {
		return command.Completion{}
	}
	i := strings.LastIndex(text, " ")
	start := utf8.RuneCountInString(text[:i+1])
	tok := text[i+1:]
	if strings.HasPrefix(tok, "-") {
		start++
		tok = tok[1:]
	}
	if name, ok := strings.CutPrefix(tok, "@"); ok {
		var out []string
		for _, v := range a.facetValues(model.GroupAssignee, name) {
			out = append(out, "@"+v)
		}
		return command.Completion{Start: start, Cands: out}
	}
	if key, val, ok := strings.Cut(tok, ":"); ok {
		g, known := facetGroup(key)
		if !known {
			return command.Completion{}
		}
		vstart := start + len([]rune(key)) + 1
		if i := strings.LastIndex(val, ","); i >= 0 {
			vstart += len([]rune(val[:i+1]))
			val = val[i+1:]
		}
		return command.Completion{Start: vstart, Cands: a.facetValues(g, val)}
	}
	if tok == "" {
		return command.Completion{}
	}
	var out []string
	for _, k := range []string{"status:", "type:", "priority:", "assignee:", "label:", "parent:"} {
		if strings.HasPrefix(k, strings.ToLower(tok)) {
			out = append(out, k)
		}
	}
	out = append(out, a.idsWith(tok, maxIDCandidate)...)
	return command.Completion{Start: start, Cands: out}
}

func facetGroup(key string) (model.FacetGroup, bool) {
	switch strings.ToLower(key) {
	case "status":
		return model.GroupStatus, true
	case "type":
		return model.GroupType, true
	case "priority":
		return model.GroupPriority, true
	case "assignee":
		return model.GroupAssignee, true
	case "label":
		return model.GroupLabel, true
	}
	return 0, false
}

// facetValues lists the values of group g that start with prefix and can be
// written in a facet token.
func (a *App) facetValues(g model.FacetGroup, prefix string) []string {
	var out []string
	for _, o := range a.filterOpts(g, "") {
		if o.value != "" && model.FacetValueOK(o.value) && !strings.ContainsAny(o.value, " \t") &&
			strings.HasPrefix(strings.ToLower(o.value), strings.ToLower(prefix)) {
			out = append(out, o.value)
		}
	}
	return out
}

// filterOpt is one checkbox of the filter bar.
type filterOpt struct {
	label, value string
	n            int
	on           bool
}

func (a *App) facetCounts() model.Facets {
	b := a.bar
	if b.facetsOf != a.snap {
		b.facetsOf = a.snap
		b.facets = model.Facets{}
		if a.snap != nil {
			b.facets = model.CountFacets(a.snap, a.bds.Statuses)
		}
	}
	return b.facets
}

// filterOpts lists the options of group g. find narrows the long lists to
// the labels that contain it.
func (a *App) filterOpts(g model.FacetGroup, find string) []filterOpt {
	f := a.facetCounts()
	sel := model.SelectionOf(a.scope.Query())
	var out []filterOpt
	switch g {
	case model.GroupStatus:
		shown := sel.StatusShown(a.scope.ShowClosed())
		for _, p := range model.StatusOrder {
			out = append(out, filterOpt{strings.ReplaceAll(model.StatusToken(p), "_", " "), model.StatusToken(p), f.Statuses[p], shown[p]})
		}
		return out
	case model.GroupType:
		for _, c := range f.Types {
			out = append(out, filterOpt{c.Name, c.Name, c.N, false})
		}
	case model.GroupPriority:
		for i, n := range f.Priorities {
			out = append(out, filterOpt{fmt.Sprintf("P%d", i), fmt.Sprint(i), n, false})
		}
	case model.GroupAssignee:
		out = append(out, filterOpt{"(none)", "none", f.Unassigned, false})
		for _, c := range f.Assignees {
			out = append(out, filterOpt{c.Name, c.Name, c.N, false})
		}
	case model.GroupLabel:
		for _, c := range f.Labels {
			out = append(out, filterOpt{c.Name, c.Name, c.N, false})
		}
	}
	for i := range out {
		out[i].on = sel.Has(g, out[i].value)
	}
	for _, v := range sel.Values(g) {
		if !slices.ContainsFunc(out, func(o filterOpt) bool { return strings.EqualFold(o.value, v) }) {
			out = append(out, filterOpt{v, v, 0, true})
		}
	}
	if find = strings.ToLower(strings.TrimSpace(find)); find != "" && g != model.GroupPriority {
		out = slices.DeleteFunc(out, func(o filterOpt) bool { return !strings.Contains(strings.ToLower(o.label), find) })
	}
	return out
}

func (a *App) columnOpts(col int) []filterOpt {
	return a.filterOpts(model.FacetGroup(col), a.bar.in.Text())
}

func (a *App) moveOption(up bool) {
	b := a.bar
	n := len(a.columnOpts(b.col))
	if n == 0 {
		return
	}
	d := 1
	if up {
		d = -1
	}
	b.cur[b.col] = min(max(b.cur[b.col]+d, 0), n-1)
}

func (a *App) toggleOption() {
	b := a.bar
	if b.kind != barFilter {
		return
	}
	opts := a.columnOpts(b.col)
	if len(opts) == 0 {
		return
	}
	o := opts[min(b.cur[b.col], len(opts)-1)]
	q := a.scope.Query()
	g := model.FacetGroup(b.col)
	if g == model.GroupStatus {
		p := model.StatusOrder[slices.IndexFunc(model.StatusOrder[:], func(p model.PresentationStatus) bool { return model.StatusToken(p) == o.value })]
		q = model.ToggleStatus(q, a.scope.ShowClosed(), p)
	} else {
		q = model.ToggleFacetValue(q, g, o.value)
	}
	a.applyScope(model.ParseScope(q, a.scope.ShowClosed()))
}

// barLines draws the open bar as exactly barHeight lines.
func (a *App) barLines() []string {
	b := a.bar
	l := a.look
	var out []string
	switch b.kind {
	case barSearch:
		out = []string{a.searchLine()}
	case barCommand:
		out = []string{a.commandLine(), a.commandHint()}
	case barFilter:
		out = a.filterLines()
	}
	h := a.barHeight()
	for len(out) < h {
		out = append(out, l.Fit("", a.cols))
	}
	return out[:h]
}

func (a *App) counts() string {
	if a.inMemories() {
		return fmt.Sprintf("%d/%d", len(a.mem.shown()), len(a.mem.list))
	}
	shown, total := 0, 0
	if m := a.matches(); m != nil {
		shown = m.Len()
	}
	if a.snap != nil {
		total = a.snap.Len()
	}
	s := fmt.Sprintf("%d/%d", shown, total)
	if m := a.matches(); m != nil && m.HiddenClosed > 0 {
		s += fmt.Sprintf(" · +%d closed", m.HiddenClosed)
	}
	return s
}

func (a *App) prompted(prompt string, role theme.Role, in *input.Field, right string) string {
	l := a.look
	left := " " + l.Paint(role, prompt) + " "
	rw := ansi.StringWidth(right)
	if rw > 0 {
		rw += 2
	}
	w := max(a.cols-ansi.StringWidth(left)-rw, 1)
	line := left + in.View(l, theme.Strong, w, true)
	if rw > 0 {
		line += l.Paint(theme.Dim, right) + "  "
	}
	return l.Fit(line, a.cols)
}

func (a *App) searchLine() string {
	g := a.look.Glyphs
	prompt := "⌕"
	if g.Tier == theme.TierASCII {
		prompt = "/"
	}
	return a.prompted(prompt, theme.Primary, a.bar.in, a.counts())
}

func (a *App) commandLine() string {
	return a.prompted(":", theme.Primary, a.bar.in, "")
}

func (a *App) commandHint() string {
	l := a.look
	b := a.bar
	switch {
	case b.err != nil:
		col := min(3+b.err.Pos, max(a.cols-2, 0))
		return l.Fit(strings.Repeat(" ", col)+l.Paint(theme.Error, "^ "+b.err.Msg), a.cols)
	case b.comp != nil && len(b.comp.cands) > 1:
		var parts []string
		for i, c := range b.comp.cands {
			if i == b.comp.at {
				parts = append(parts, l.PaintSel(theme.Strong, c))
			} else {
				parts = append(parts, l.Paint(theme.Dim, c))
			}
		}
		return l.Fit(" "+strings.Join(parts, "  "), a.cols)
	}
	return l.Fit(" "+l.Paint(theme.Dim, a.cmds.Hint(b.in.Text())), a.cols)
}

func (a *App) filterLines() []string {
	l := a.look
	b := a.bar
	sel := model.SelectionOf(a.scope.Query())
	var find string
	if b.in.Text() == "" {
		find = l.Paint(theme.Faint, "type to find an option")
	}
	right := a.counts() + " shown"
	head := " " + l.Paint(theme.Primary, "Filter") + "  "
	room := max(a.cols-ansi.StringWidth(head)-ansi.StringWidth(right)-3, 1)
	field := find
	if find == "" {
		field = b.in.View(l, theme.Strong, room, true)
	}
	top := l.Fit(head+l.Fit(field, room)+" "+l.Paint(theme.Dim, right), a.cols)

	orows := a.filterRows()
	shown := shownColumns(a.cols, b.col)
	widths := columnWidths(a.cols, shown)
	heads := make([]string, len(shown))
	cells := make([][]string, len(shown))
	for k, c := range shown {
		w := widths[k]
		opts := a.columnOpts(c)
		title := filterNames[c]
		if n := len(sel.Values(model.FacetGroup(c))); n > 0 && c != int(model.GroupStatus) {
			title += fmt.Sprintf(" (%d)", n)
		}
		role := theme.Dim
		if c == b.col {
			role = theme.Primary
		}
		heads[k] = l.Paint(role, l.Fit(title, w))
		first := 0
		if b.cur[c] >= orows {
			first = b.cur[c] - orows + 1
		}
		cells[k] = make([]string, orows)
		for i := range orows {
			cells[k][i] = a.optionCell(opts, first+i, w, c == b.col && first+i == b.cur[c])
		}
	}
	out := []string{top, l.Fit(strings.Join(heads, " "), a.cols)}
	for i := range orows {
		row := make([]string, len(shown))
		for k := range shown {
			row[k] = cells[k][i]
		}
		out = append(out, l.Fit(strings.Join(row, " "), a.cols))
	}
	if len(sel.Advanced) > 0 {
		out = append(out, l.Fit(" "+l.Paint(theme.Warning, "advanced: ")+l.Paint(theme.Dim, rows.MidCut(strings.Join(sel.Advanced, " "), max(a.cols-13, 4), l.Glyphs.Ellipsis)), a.cols))
	}
	return out
}

// narrowFilter is the width below which the filter bar shows the active
// column and its neighbours only.
const narrowFilter = 80

// shownColumns lists the filter columns drawn: all of them, or below
// narrowFilter the active one with its neighbours.
func shownColumns(cols, active int) []int {
	n := filterCols
	if cols < narrowFilter {
		n = 3
	}
	first := min(max(active-n/2, 0), filterCols-n)
	out := make([]int, n)
	for i := range out {
		out[i] = first + i
	}
	return out
}

// columnWidths splits the width among the shown filter columns, giving
// Status the most because its labels are the longest, with a cell between
// columns.
func columnWidths(cols int, shown []int) []int {
	weights := [filterCols]int{3, 2, 2, 2, 2}
	usable := max(cols-(len(shown)-1), len(shown))
	total := 0
	for _, c := range shown {
		total += weights[c]
	}
	out := make([]int, len(shown))
	used := 0
	for k, c := range shown {
		out[k] = usable * weights[c] / total
		used += out[k]
	}
	out[len(out)-1] += usable - used
	return out
}

func (a *App) optionCell(opts []filterOpt, i, w int, cursor bool) string {
	l := a.look
	if i >= len(opts) {
		return l.Fit("", w)
	}
	o := opts[i]
	box := "[ ] "
	if o.on {
		box = "[x] "
	}
	n := fmt.Sprint(o.n)
	text := l.Fit(box+o.label, max(w-ansi.StringWidth(n)-1, 0)) + " " + n
	text = l.Fit(text, w)
	role := theme.Dim
	switch {
	case o.on:
		role = theme.Strong
	case o.n == 0:
		role = theme.Faint
	}
	if cursor {
		return l.PaintSel(theme.Text, text)
	}
	return l.Paint(role, text)
}
