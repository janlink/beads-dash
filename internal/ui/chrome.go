package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/detail"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/screens"
	"github.com/janlink/beads-dash/internal/ui/state"
)

type span struct{ from, to int }

// ruleLead is where the first word of a rule starts: the line glyph and a pad.
const ruleLead = 2

// tabSegs lays out the footer's view tabs as one rule entry, a rule segment
// between the tabs, and returns the column span of each tab counted from the
// left edge of the rule. Without named, the tabs are their numbers only.
func (a *App) tabSegs(named bool) ([]look.Seg, [6]span) {
	var spans [6]span
	var segs []look.Seg
	sep := look.Seg{Role: theme.Rule, Text: " " + a.look.Glyphs.Rule + " "}
	sepW := look.SegWidth([]look.Seg{sep})
	x := ruleLead
	for i, name := range ViewNames {
		label := strconv.Itoa(i + 1)
		if named {
			label += " " + name
		}
		var tab []look.Seg
		switch {
		case i == a.slot && a.look.Palette.Depth() == theme.DepthNone:
			tab = look.Word(theme.Strong, "["+label+"]")
		case i == a.slot:
			tab = look.Word(theme.Primary, label)
		default:
			tab = look.Word(theme.Faint, label)
		}
		if i > 0 {
			segs = append(segs, sep)
			x += sepW
		}
		w := look.SegWidth(tab)
		spans[i] = span{x, x + w}
		segs = append(segs, tab...)
		x += w
	}
	return segs, spans
}

// ruleItem is one word of a chrome rule, in display order. A rule keeps the
// words that fit, lowest prio first; flex gives a word that may shrink to the
// room left for its text, or nil when nothing fits.
type ruleItem struct {
	seg   []look.Seg
	right bool
	prio  int
	flex  func(room int) []look.Seg
	// must marks a word the rule is not worth drawing without.
	must bool
}

// fitRule keeps the items that fit in w cells with at least one cell of line
// left and returns the left and right words set into the rule; ok is false
// when a must word did not fit.
func fitRule(l look.Look, w int, items []ruleItem) (left, right []look.Seg, ok bool) {
	left, right, ok, _ = fitRuleAll(l, w, items)
	return left, right, ok
}

// fitRuleAll is fitRule that also reports whether every item kept its full
// text.
func fitRuleAll(l look.Look, w int, items []ruleItem) (left, right []look.Seg, ok, whole bool) {
	kept := make([][]look.Seg, len(items))
	build := func() ([]look.Seg, []look.Seg) {
		var le, ri [][]look.Seg
		for i, it := range items {
			switch {
			case kept[i] == nil:
			case it.right:
				ri = append(ri, kept[i])
			default:
				le = append(le, kept[i])
			}
		}
		return l.Words(le...), l.Words(ri...)
	}
	width := func() int {
		le, ri := build()
		return look.SegWidth(le) + look.SegWidth(ri)
	}
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(x, y int) int { return items[x].prio - items[y].prio })
	for _, i := range order {
		it := items[i]
		if it.seg != nil {
			if kept[i] = it.seg; width() < w {
				continue
			}
			kept[i] = nil
		}
		if it.flex == nil {
			continue
		}
		if room := w - width() - look.WordCost - 1; room > 0 {
			if kept[i] = it.flex(room); kept[i] != nil && width() >= w {
				kept[i] = nil
			}
		}
	}
	ok, whole = true, true
	for i, it := range items {
		ok = ok && (!it.must || kept[i] != nil)
		whole = whole && kept[i] != nil && (it.seg == nil || look.SegWidth(kept[i]) == look.SegWidth(it.seg))
	}
	left, right = build()
	return left, right, ok, whole
}

// panelSide is the width of a detail panel docked at the right, 0 when there
// is none.
func (a *App) panelSide() int {
	if d := a.frame(); d.Frame == detail.Side {
		return d.W
	}
	return 0
}

// headerWords are the header rule's words for w cells: the view name and the
// workspace on the left; the scope label, the bd version warning, the issue
// count with the cursor position and the live marker on the right. When the
// row runs short the workspace goes first, then the scope label shrinks and
// goes, then the bd warning and the count, then the live marker loses its
// word.
func (a *App) headerWords(w int) (left, right []look.Seg) {
	words := []bool{true, false}
	if BreakpointOf(a.cols) == Narrow {
		words = words[1:]
	}
	for _, word := range words {
		items := []ruleItem{{seg: look.Word(theme.Strong, ViewNames[a.slot]), must: true}}
		if ws := a.workspaceName(); ws != "" {
			items = append(items, ruleItem{seg: look.Word(theme.Dim, ws), prio: 5})
		}
		items = append(items, ruleItem{right: true, prio: 4, flex: func(room int) []look.Seg {
			if s := a.view().Scope(room); s != "" {
				return look.Word(theme.Dim, s)
			}
			return nil
		}})
		if a.bds.Untested {
			items = append(items, ruleItem{right: true, prio: 3, seg: look.Word(theme.Warning, "bd "+a.bds.Version.Parsed.String()+" untested")})
		}
		if stats := a.statsWord(); stats != "" {
			items = append(items, ruleItem{right: true, prio: 1, seg: look.Word(theme.Dim, stats)})
		}
		items = append(items, ruleItem{right: true, seg: a.liveSegs(word), must: true})
		var ok bool
		if left, right, ok = fitRule(a.look, w, items); ok {
			break
		}
	}
	return left, right
}

// statsWord is the issue count and, when the view has a cursor position, the
// position: "44 issues · 3/44". The scope label carries the count while a
// scope is active.
func (a *App) statsWord() string {
	if a.snap == nil || a.inMemories() || a.slot == overviewSlot {
		return ""
	}
	pos := a.position()
	if a.sess.ScopeActive {
		return pos
	}
	n := a.snap.Len()
	word := fmt.Sprintf("%d issues", n)
	if n == 1 {
		word = "1 issue"
	}
	if pos != "" {
		word += " · " + pos
	}
	return word
}

// footerLayout is the footer rule's words for w cells and the column span of
// each view tab: the tabs on the left and the chips on the right. The tab
// names shrink to numbers before any chip or note is cut; then the note
// shortens or leaves first, the other chips by their rank.
func (a *App) footerLayout(w int) (left, right []look.Seg, spans [6]span) {
	for _, named := range []bool{true, false} {
		var tabs []look.Seg
		tabs, spans = a.tabSegs(named)
		items := []ruleItem{{seg: tabs, must: true}}
		for _, it := range a.chipItems() {
			it.right = true
			items = append(items, it)
		}
		var whole bool
		if left, right, _, whole = fitRuleAll(a.look, w, items); whole {
			break
		}
	}
	return left, right, spans
}

func (a *App) header() string {
	l := a.look
	pw := a.panelSide()
	left, right := a.headerWords(a.cols - pw)
	row := l.Rule(a.cols-pw, left, right)
	if pw > 0 {
		id := l.TruncID(a.sess.Current(), max(pw-1-look.WordCost-1, 1))
		row += l.Rule(pw, append([]look.Seg{l.Tee(true)}, l.Words(look.Word(theme.Strong, id))...), nil)
	}
	return row
}

// liveSegs is the live marker: the snapshot's state and age.
func (a *App) liveSegs(word bool) []look.Seg {
	g := a.look.Glyphs
	switch {
	case a.startErr == nil && !a.status.Loaded && a.snap == nil:
		return look.Word(theme.Dim, g.Bullet+" connecting")
	case a.status.Stale || a.status.Err != nil:
		age := a.now().Sub(a.status.LastSuccess)
		return []look.Seg{{Role: theme.Warning, Text: g.Stale + " stale "}, {Role: theme.Dim, Text: ageText(age)}}
	}
	sep := " "
	if word {
		sep = " live "
	}
	return []look.Seg{{Role: theme.Success, Text: g.Live + sep}, {Role: theme.Dim, Text: a.status.LastSuccess.Format("15:04:05")}}
}

func ageText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", max(int(d/time.Second), 0))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return fmt.Sprintf("%dh", int(d/time.Hour))
}

// chipItems are the counters and health chips the footer rule carries.
func (a *App) chipItems() []ruleItem {
	if a.inMemories() {
		return a.memChips()
	}
	l := a.look
	var items []ruleItem
	chip := func(prio int, role theme.Role, text string) {
		items = append(items, ruleItem{seg: look.Word(role, text), prio: prio})
	}
	if n := a.sess.MarkCount(); n > 0 {
		s := fmt.Sprintf("%s %d marked", l.Glyphs.Mark, n)
		if h := a.sess.HiddenMarks(a.visible); h > 0 {
			s += fmt.Sprintf(" (%d hidden)", h)
		}
		chip(0, theme.Primary, s)
	}
	ids := slices.DeleteFunc(a.hl.IDs(a.now()), func(id string) bool { return strings.HasPrefix(id, memPrefix) })
	if len(ids) > 0 {
		hidden := 0
		for _, id := range ids {
			if !a.visible(id) {
				hidden++
			}
		}
		s := fmt.Sprintf("%s %d changed", l.Glyphs.Change, len(ids))
		if hidden > 0 {
			s += fmt.Sprintf(" (%d hidden)", hidden)
		}
		chip(2, theme.Changed, s)
	}
	if a.status.Slow {
		chip(3, theme.Warning, "slow")
	}
	if a.status.GCHint {
		chip(3, theme.Warning, "gc")
	}
	if a.journalLimited() {
		chip(4, theme.Dim, "limited: no actors")
	}
	if a.status.Fallback {
		chip(4, theme.Warning, "journal-limited")
	}
	if n, ok := a.view().(Noter); ok && a.snap != nil {
		if note := n.Note(); note != "" {
			items = append(items, ruleItem{seg: look.Word(theme.Warning, note), prio: 5, flex: func(room int) []look.Seg {
				if room < minNoteCells {
					return nil
				}
				return look.Word(theme.Warning, rows.MidCut(note, room, l.Glyphs.Ellipsis))
			}})
		}
	}
	return items
}

// minNoteCells is the least a shortened footer note keeps.
const minNoteCells = 12

// position is the current issue's place among the issues the view shows, as
// "3/47"; empty when there is none.
func (a *App) position() string {
	if a.snap == nil || a.slot == overviewSlot || a.inMemories() {
		return ""
	}
	ids := a.view().Visible(a.env())
	if i := slices.Index(ids, a.sess.Current()); i >= 0 {
		return fmt.Sprintf("%d/%d", i+1, len(ids))
	}
	return ""
}

// footer is the two rows under the body: a rule with the view tabs and the
// counters and chips, then the hints or the notice. Beside a side panel the
// rule closes the panel's border and carries the panel's keys.
func (a *App) footer() []string {
	l := a.look
	pw := a.panelSide()
	left, right, _ := a.footerLayout(a.cols - pw)
	rule := l.Rule(a.cols-pw, left, right)
	if pw > 0 {
		keyWord := a.panelKeySegs(pw - 1 - look.WordCost - 1)
		rule += l.Rule(pw, append([]look.Seg{l.Tee(false)}, l.Words(keyWord)...), nil)
	}
	return []string{rule, a.hintsRow()}
}

// panelKeySegs are the keys of the docked panel as one rule word of at most w
// cells: the panel's own keys while it has the keys, else the list's keys that
// act on the panel's issue.
func (a *App) panelKeySegs(w int) []look.Seg {
	hs := []keys.Hint{
		a.keyHint(keys.View, keys.Edit, "edit"),
		a.keyHint(keys.View, keys.Export, "export"),
		a.keyHint(keys.Global, keys.FocusNext, "focus"),
	}
	if a.sess.Has(state.LayerDetailFocus) {
		hs = []keys.Hint{
			a.keyHint(keys.Panel, keys.SectionNext, "section"),
			a.keyHint(keys.Panel, keys.Markdown, "markdown"),
			a.keyHint(keys.Panel, keys.Export, "export"),
			a.keyHint(keys.Panel, keys.Close, "back"),
		}
	}
	var segs []look.Seg
	for i, h := range dropToFit(hs, w, nil) {
		if i > 0 {
			segs = append(segs, look.Seg{Role: theme.Rule, Text: "  "})
		}
		segs = append(segs, look.Seg{Role: theme.Strong, Text: h.Key}, look.Seg{Role: theme.Faint, Text: " " + h.Desc})
	}
	return segs
}

// keyHint is the hint for the key bound to act in c, worded desc; it is empty
// when c has no such key.
func (a *App) keyHint(c keys.Context, act keys.Action, desc string) keys.Hint {
	for _, lc := range keys.Layered(c) {
		for _, b := range a.km.Active(lc) {
			if b.Action != act {
				continue
			}
			key := b.Text()
			if b.HintKey != "" {
				key = b.HintKey
			}
			return keys.Hint{Key: key, Desc: desc}
		}
	}
	return keys.Hint{}
}

// hintGap is the cells between two hints.
const hintGap = 2

// legendMinCols is the terminal width from which the hints row shows the
// status legend.
const legendMinCols = 120

// listHints are the hints of a view that has the keys and the order the hints
// leave in when the row runs short, help last. ok is false while another
// layer has the keys.
func (a *App) listHints() (hs []keys.Hint, drop []string, ok bool) {
	c := a.context()
	switch c { //nolint:exhaustive // only the view contexts show the list hints
	case keys.View, keys.Tree, keys.Overview, keys.Graph:
		details := "details"
		if c == keys.Graph {
			details = "focus"
		}
		open := a.keyHint(c, keys.Open, details)
		if open.Key != "" {
			open.Key = a.look.Glyphs.Enter
		}
		hs = []keys.Hint{
			a.keyHint(c, keys.OpenSearch, "search"),
			a.keyHint(c, keys.OpenFilter, "filter"),
			open,
			a.keyHint(c, keys.OpenCommand, "cmd"),
			a.keyHint(c, keys.OpenHelp, "help"),
		}
	case keys.Panel:
		hs = []keys.Hint{
			a.keyHint(c, keys.NavDown, "scroll"),
			a.keyHint(c, keys.SectionNext, "section"),
			a.keyHint(c, keys.Jump, "jump"),
			a.keyHint(c, keys.OpenCommand, "cmd"),
			a.keyHint(c, keys.OpenHelp, "help"),
		}
		if hs[2].Key != "" {
			hs[2].Key = a.look.Glyphs.Enter
		}
		if a.panelSide() > 0 {
			hs = slices.DeleteFunc(hs, func(h keys.Hint) bool { return h.Desc == "section" })
		}
	case keys.Memories:
		hs = []keys.Hint{
			a.keyHint(c, keys.OpenSearch, "search"),
			a.keyHint(c, keys.OpenFilter, "filter"),
			a.keyHint(c, keys.NavDown, "move"),
			a.keyHint(c, keys.MemoryNew, "new"),
			a.keyHint(c, keys.MemoryEdit, "edit"),
			a.keyHint(c, keys.MemoryForget, "forget"),
			a.keyHint(c, keys.OpenCommand, "cmd"),
			a.keyHint(c, keys.OpenHelp, "help"),
		}
	default:
		return nil, nil, false
	}
	hs = slices.DeleteFunc(hs, func(h keys.Hint) bool { return h.Key == "" })
	return hs, []string{"cmd", "details", "focus", "jump", "section", "scroll", "edit", "forget", "new", "move", "filter", "search", "help"}, true
}

func (a *App) hintsRow() string {
	l := a.look
	if n, ok := a.noticeRow(); ok {
		return n
	}
	if a.hint != "" {
		return l.Fit(" "+l.Paint(theme.Warning, a.hint), a.cols)
	}
	hs, drop, listed := a.listHints()
	if !listed {
		hs = a.hintsFor(a.context())
		if h, ok := a.topDialog().(hinter); ok {
			hs = h.hints()
		}
	}
	if a.escClears() {
		hs = append([]keys.Hint{{Key: "Esc", Desc: "clear"}}, hs...)
	}
	if a.emptyShown() {
		hs = append(slices.Clone(a.emptyWorkspace().Hints), hs...)
	}
	inner := a.cols - 2
	legend := a.legendSegs()
	if listed && a.cols >= legendMinCols && !a.inMemories() && !a.emptyShown() && hintsWidth(hs)+hintGap+look.SegWidth(legend) <= inner {
		pad := inner - hintsWidth(hs) - look.SegWidth(legend)
		return l.Fit(" "+a.hintsText(hs)+strings.Repeat(" ", pad)+a.paintSegs(legend), a.cols)
	}
	return l.Fit(" "+a.hintsText(dropToFit(hs, inner, drop)), a.cols)
}

// legendSegs is the status legend: each status's glyph in its colour and its
// name.
func (a *App) legendSegs() []look.Seg {
	entries := []struct {
		status int
		name   string
	}{{0, "open"}, {1, "in progress"}, {2, "blocked"}, {4, "closed"}, {3, "frozen"}}
	var segs []look.Seg
	for i, e := range entries {
		if i > 0 {
			segs = append(segs, look.Seg{Role: theme.Faint, Text: strings.Repeat(" ", hintGap)})
		}
		glyph := strings.TrimSpace(a.look.Glyphs.Status[e.status])
		segs = append(segs, look.Seg{Role: theme.StatusRole(e.status), Text: glyph + " "}, look.Seg{Role: theme.Faint, Text: e.name})
	}
	return segs
}

func (a *App) paintSegs(segs []look.Seg) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(a.look.Paint(s.Role, s.Text))
	}
	return b.String()
}

// emptyShown reports whether the empty-workspace block is on screen.
func (a *App) emptyShown() bool {
	return a.snap != nil && a.snap.Len() == 0 && a.report == nil && len(a.dialogs) == 0
}

func hintWidth(h keys.Hint) int {
	w := ansi.StringWidth(h.Key)
	if h.Desc != "" {
		w += 1 + ansi.StringWidth(h.Desc)
	}
	return w
}

func hintsWidth(hs []keys.Hint) int {
	w := 0
	for i, h := range hs {
		if i > 0 {
			w += hintGap
		}
		w += hintWidth(h)
	}
	return w
}

// dropToFit removes hints until the rest fit in w cells. Hints leave in the
// order of their words in drop, then from the end.
func dropToFit(hs []keys.Hint, w int, drop []string) []keys.Hint {
	hs = slices.Clone(hs)
	for len(hs) > 0 && hintsWidth(hs) > w {
		i := len(hs) - 1
		for _, word := range drop {
			if j := slices.IndexFunc(hs, func(h keys.Hint) bool { return h.Desc == word }); j >= 0 {
				i = j
				break
			}
		}
		hs = slices.Delete(hs, i, i+1)
	}
	return hs
}

// hintsText paints hints with the key strong and the word faint.
func (a *App) hintsText(hs []keys.Hint) string {
	l := a.look
	cells := make([]string, len(hs))
	for i, h := range hs {
		cells[i] = l.Paint(theme.Strong, h.Key)
		if h.Desc != "" {
			cells[i] += " " + l.Paint(theme.Faint, h.Desc)
		}
	}
	return strings.Join(cells, strings.Repeat(" ", hintGap))
}

func (a *App) noticeRow() (string, bool) {
	n, ok := a.notice()
	if !ok {
		return "", false
	}
	return screens.NoticeRow(a.look, n, a.cols), true
}

// escClears reports whether Esc, with no layer above the view, clears the
// scope or the marks.
func (a *App) escClears() bool {
	if a.report != nil || len(a.dialogs) > 0 || a.bar != nil {
		return false
	}
	if _, layered := a.sess.Top(); layered {
		return false
	}
	if a.inMemories() {
		return a.mem.focus || len(a.mem.marks) > 0 || a.mem.query.Active()
	}
	return a.sess.ScopeActive || a.sess.MarkCount() > 0
}
