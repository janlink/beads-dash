package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/detail"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

type span struct{ from, to int }

// ruleLead is where the first word of a rule starts: the line glyph and a pad.
const ruleLead = 2

// tabSegs lays out the header's view tabs as one rule entry and returns the
// column span of each tab, counted from the left edge of the rule. With
// current set, only the current view's name is shown.
func (a *App) tabSegs(current bool) ([]look.Seg, [6]span) {
	var spans [6]span
	if current {
		return look.Word(theme.Strong, ViewNames[a.slot]), spans
	}
	var segs []look.Seg
	x := ruleLead
	for i, name := range ViewNames {
		var tab []look.Seg
		switch {
		case i == a.slot && a.look.Palette.Depth() == theme.DepthNone:
			tab = look.Word(theme.Strong, fmt.Sprintf("[%d %s]", i+1, name))
		case i == a.slot:
			tab = []look.Seg{{Role: theme.Strong, Text: fmt.Sprintf("%d", i+1)}, {Role: theme.Primary, Text: " " + name}}
		case a.views[i] == nil:
			tab = look.Word(theme.Faint, fmt.Sprintf("%d %s", i+1, name))
		default:
			tab = look.Word(theme.Dim, fmt.Sprintf("%d %s", i+1, name))
		}
		if i > 0 {
			segs = append(segs, look.Seg{Role: theme.Rule, Text: "  "})
			x += 2
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
	for i, it := range items {
		if it.must && kept[i] == nil {
			left, right = build()
			return left, right, false
		}
	}
	left, right = build()
	return left, right, true
}

// panelSide is the width of a detail panel docked at the right, 0 when there
// is none.
func (a *App) panelSide() int {
	if d := a.dock(); d.Frame == detail.Side {
		return d.W
	}
	return 0
}

// headerWords are the header rule's words for w cells: the tabs on the left;
// the scope label, the bd version warning and the live marker on the right.
// When the row runs short the scope label shrinks and goes first, then the bd
// warning, then the live marker loses its word, then the tabs shrink to the
// current view's name.
func (a *App) headerWords(w int) (left, right []look.Seg) {
	narrow := BreakpointOf(a.cols) == Narrow
	type rung struct{ tabs, live bool }
	rungs := []rung{{true, true}, {true, false}, {false, true}, {false, false}}
	if narrow {
		rungs = rungs[3:]
	}
	var ok bool
	for _, r := range rungs {
		tabs, _ := a.tabSegs(!r.tabs)
		items := []ruleItem{{seg: tabs, must: true}}
		items = append(items, ruleItem{right: true, prio: 4, flex: func(room int) []look.Seg {
			if s := a.view().Scope(room); s != "" {
				return look.Word(theme.Dim, s)
			}
			return nil
		}})
		if a.bds.Untested {
			items = append(items, ruleItem{right: true, prio: 3, seg: look.Word(theme.Warning, "bd "+a.bds.Version.Parsed.String()+" untested")})
		}
		items = append(items, ruleItem{right: true, seg: a.liveSegs(r.live), must: true})
		if left, right, ok = fitRule(a.look, w, items); ok {
			break
		}
	}
	return left, right
}

// tabsCollapsed reports whether the header shows only the current view's name.
func (a *App) tabsCollapsed() bool {
	if BreakpointOf(a.cols) == Narrow {
		return true
	}
	full, _ := a.tabSegs(false)
	left, _ := a.headerWords(a.cols - a.panelSide())
	return look.SegWidth(left) < look.SegWidth(a.look.Words(full))
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
			items = append(items, ruleItem{seg: look.Word(theme.Dim, note), prio: 5, flex: func(room int) []look.Seg {
				if room < minNoteCells {
					return nil
				}
				return look.Word(theme.Dim, rows.MidCut(note, room, l.Glyphs.Ellipsis))
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

// footer is the two rows under the body: a rule with the counters, chips and
// position, then the hints or the notice.
func (a *App) footer() []string {
	l := a.look
	items := a.chipItems()
	if pos := a.position(); pos != "" {
		items = append(items, ruleItem{seg: look.Word(theme.Dim, pos), right: true, prio: 1})
	}
	pw := a.panelSide()
	left, right, _ := fitRule(l, a.cols-pw, items)
	rule := l.Rule(a.cols-pw, left, right)
	if pw > 0 {
		rule += l.Rule(pw, []look.Seg{l.Tee(false)}, nil)
	}
	return []string{rule, a.hintsRow()}
}

func (a *App) hintsRow() string {
	l := a.look
	if n, ok := a.noticeRow(); ok {
		return n
	}
	if a.hint != "" {
		return l.Fit(" "+l.Paint(theme.Warning, a.hint), a.cols)
	}
	hs := a.hintsFor(a.context())
	if h, ok := a.topDialog().(hinter); ok {
		hs = h.hints()
	}
	if a.escClears() {
		hs = append([]keys.Hint{{Key: "Esc", Desc: "clear"}}, hs...)
	}
	if a.emptyShown() {
		hs = append(slices.Clone(a.emptyWorkspace().Hints), hs...)
	}
	return l.Fit(" "+fitHints(hs, a.cols-2, l), a.cols)
}

// emptyShown reports whether the empty-workspace block is on screen.
func (a *App) emptyShown() bool {
	return a.snap != nil && a.snap.Len() == 0 && a.report == nil && len(a.dialogs) == 0
}

// fitHints joins as many hints as fit in w cells, dropping from the end.
func fitHints(hs []keys.Hint, w int, l look.Look) string {
	var b strings.Builder
	used := 0
	for _, h := range hs {
		cell := l.Paint(theme.Strong, h.Key)
		cw := ansi.StringWidth(h.Key)
		if h.Desc != "" {
			cell += " " + l.Paint(theme.Dim, h.Desc)
			cw += 1 + ansi.StringWidth(h.Desc)
		}
		if used > 0 {
			cw += 2
		}
		if used+cw > w {
			break
		}
		if used > 0 {
			b.WriteString("  ")
		}
		b.WriteString(cell)
		used += cw
	}
	return b.String()
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
