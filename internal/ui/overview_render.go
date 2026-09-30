package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
	"github.com/janlink/beads-dash/internal/ui/rows"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

const (
	ovWideFrom  = 200
	ovStripFrom = 80
	ovSideWide  = 40
	ovSideRoomy = 36
	ovSideBase  = 32
	minBoxH     = 3
	ovGap       = 2
)

// ovLine is one body line of a box and the item it shows, -1 for none.
type ovLine struct {
	s    string
	item int
}

func textLine(s string) ovLine { return ovLine{s, -1} }

// Render implements View.
func (v *Overview) Render(env Env, w, h int) []string {
	v.ensure(env)
	if env.Matches == nil || v.data.Total == 0 {
		return v.empty(env, w, h)
	}
	v.boxes = [regionCount]ovBox{}
	v.roomy = w >= ovWideFrom
	var out []string
	switch {
	case w >= ovWideFrom:
		out = v.wide(env, w, h)
	case w >= ovStripFrom:
		out = v.sidebar(env, w, h)
	default:
		out = v.strip(env, w, h)
	}
	v.drawn = true
	if v.boxes[v.active()].w == 0 {
		v.focus = regFeed
	}
	return out
}

func (v *Overview) wide(env Env, w, h int) []string {
	aw := w * 28 / 100
	fw := w - aw - ovSideWide - 2*ovGap
	feed := v.region(env, regFeed, "Activity", 0, 0, fw, h)
	active := v.region(env, regActive, "Active assignees", fw+ovGap, 0, aw, h)
	return hjoin(env.Look, h, ovGap, feed, active, v.side(env, fw+aw+2*ovGap, ovSideWide, h, false))
}

func (v *Overview) sidebar(env Env, w, h int) []string {
	sw := ovSideBase
	if w >= 120 {
		sw = ovSideRoomy
	}
	fw := w - sw - ovGap
	feed := v.region(env, regFeed, "Activity", 0, 0, fw, h)
	return hjoin(env.Look, h, ovGap, feed, v.side(env, fw+ovGap, sw, h, true))
}

// side stacks Now, Needs attention and, when asked, the active assignees.
func (v *Overview) side(env Env, x, w, h int, withActive bool) []string {
	now := v.nowLines(env, w)
	nh := min(len(now)+1, max(h-minBoxH, minBoxH))
	out := v.box(env, -1, "Now", now, w, nh, 0, 0)
	rest := h - nh
	if rest < minBoxH {
		return pad(env.Look, out, w, h)
	}
	attn := rest
	if withActive && rest >= 2*minBoxH {
		attn = rest * 55 / 100
	}
	out = append(out, v.region(env, regAttention, "Needs attention", x, nh, w, attn)...)
	if rest -= attn; withActive && rest >= minBoxH {
		out = append(out, v.region(env, regActive, "Active", x, nh+attn, w, rest)...)
	}
	return pad(env.Look, out, w, h)
}

func (v *Overview) strip(env Env, w, h int) []string {
	l := env.Look
	a := v.data.Attention
	head := []string{
		l.Fit(v.counters(env), w),
		l.Fit(l.Paint(theme.Success, sparkline(l.Glyphs, v.data.Closed14[:]))+l.Paint(theme.Dim, " closed/day · ")+
			l.Paint(theme.Success, readyGlyph(l.Glyphs)+" "+itoa(v.data.Ready)+" ready")+l.Paint(theme.Dim, " · ")+
			l.Paint(theme.StatusBlocked, l.Glyphs.Status[2]+" "+itoa(a.BlockedTotal)+" blocked"), w),
	}
	active := l.Paint(theme.Dim, "active: ")
	for i, g := range v.data.Active {
		if i > 0 {
			active += "  "
		}
		active += l.Paint(theme.Strong, g.Who) + l.Paint(theme.StatusInProgress, " "+l.Glyphs.Status[1]+itoa(len(g.IDs)))
	}
	if len(v.data.Active) == 0 {
		active += l.Paint(theme.Dim, "nobody")
	}
	head = append(head, l.Fit(active, w))
	return append(head, v.region(env, regFeed, "Activity", 0, len(head), w, h-len(head))...)
}

// region draws a focusable card and records where it went.
func (v *Overview) region(env Env, r ovRegion, title string, x, y, w, h int) []string {
	bh := h - 1
	var lines []ovLine
	switch r {
	case regFeed:
		lines = v.feedLines(env, w, bh)
	case regAttention:
		lines = v.attentionLines(env, w)
	case regActive, regionCount:
		lines = v.activeLines(env, w)
	}
	return v.box(env, r, title, lines, w, h, x, y)
}

func (v *Overview) focused(r ovRegion) bool { return v.active() == r }

// cursorRow is the index of the item the cursor is on when r has the keys.
func (v *Overview) cursorRow(env Env, r ovRegion) int {
	if !v.focused(r) {
		return -1
	}
	return v.cursor(r, env.Current)
}

func (v *Overview) feedLines(env Env, iw, bh int) []ovLine {
	if len(v.data.Feed) == 0 {
		var out []ovLine
		for _, s := range screens.RenderEmpty(env.Look, emptyFeed(env.Scope.Active()), iw, bh) {
			out = append(out, textLine(s))
		}
		return out
	}
	l := env.Look
	cur := v.cursorRow(env, regFeed)
	seen := map[string]bool{}
	var out []ovLine
	bucket := model.FeedBucket(255)
	for i, e := range v.data.Feed {
		if b := model.BucketOf(env.Now, e.Time); b != bucket {
			bucket = b
			out = append(out, textLine(l.Rule(iw, l.Words(look.Word(theme.Faint, b.Label())), nil)))
		}
		first := !seen[e.IssueID]
		seen[e.IssueID] = true
		row := rows.Row{Current: i == cur, Marked: first && env.Marked(e.IssueID), Changed: first && env.Changed(e.IssueID)}
		body := env.Rows.Feed(rows.FeedLine{Event: e, Now: env.Now})
		out = append(out, ovLine{env.Rows.Gutter(row) + body(iw-rows.GutterWidth, i == cur), i})
	}
	return out
}

func emptyFeed(scoped bool) screens.Empty {
	if scoped {
		return screens.Empty{
			Title: "No recent activity in this scope.",
			Body:  "No change to a matching issue has been seen yet.",
			Hints: []keys.Hint{{Key: "Esc", Desc: "clear scope"}},
		}
	}
	return screens.Empty{
		Title: "No recent activity.",
		Body:  "Changes show up here once bdash sees them.",
		Hints: []keys.Hint{{Key: "r", Desc: "refresh"}},
	}
}

func (v *Overview) issueLine(env Env, view string, id string, cur bool, iw, item int, body rows.Body) ovLine {
	row := rows.Row{ID: id, Current: cur, Marked: env.Marked(id), Changed: env.Changed(id)}
	return ovLine{env.Rows.Line(iw, row, view, body), item}
}

func (v *Overview) attentionLines(env Env, iw int) []ovLine {
	l := env.Look
	g := l.Glyphs
	a := v.data.Attention
	cur := v.cursorRow(env, regAttention)
	head := func(role theme.Role, glyph, text, note string) ovLine {
		s := l.Paint(role, glyph+" "+text)
		if note != "" {
			s += l.Paint(theme.Dim, " "+note)
		}
		return textLine(l.Fit(s, iw))
	}
	more := func(n int, where string) ovLine {
		return textLine(l.Fit(l.Paint(theme.Dim, "  "+g.Ellipsis+" "+itoa(n)+" more"+where), iw))
	}
	var out []ovLine
	out = append(out, head(theme.Success, readyGlyph(g), "Ready "+itoa(a.ReadyTotal), "("+itoa(a.ReadyUnassigned)+" unassigned)"))
	item := 0
	for _, id := range a.Ready {
		out = append(out, v.issueLine(env, "overview/attention", id, item == cur, iw, item, env.Rows.Standard(id)))
		item++
	}
	if n := a.ReadyTotal - len(a.Ready); n > 0 {
		out = append(out, more(n, " in "+itoa(slotReady)+" Ready"))
	}
	if a.AssignedNotStarted > 0 {
		out = append(out, textLine(l.Fit(l.Paint(theme.Dim, "  "+itoa(a.AssignedNotStarted)+" assigned, not started"), iw)))
	}
	out = append(out, head(theme.StatusBlocked, g.Status[2], "Blocked "+itoa(a.BlockedTotal), ""))
	for _, id := range a.Blocked {
		out = append(out, v.issueLine(env, "overview/attention", id, item == cur, iw, item, env.Rows.Standard(id)))
		item++
	}
	if n := a.BlockedTotal - len(a.Blocked); n > 0 {
		out = append(out, more(n, ""))
	}
	return out
}

func (v *Overview) activeLines(env Env, iw int) []ovLine {
	l := env.Look
	if len(v.data.Active) == 0 {
		return []ovLine{textLine(l.Fit(l.Paint(theme.Dim, "nobody has work in progress"), iw))}
	}
	cur := v.cursorRow(env, regActive)
	var out []ovLine
	item := 0
	for _, g := range v.data.Active {
		s := l.Paint(theme.Strong, g.Who) + l.Paint(theme.Dim, "  "+itoa(len(g.IDs))+" in progress")
		out = append(out, textLine(l.Fit(s, iw)))
		for _, id := range g.IDs {
			out = append(out, v.issueLine(env, "overview/active", id, item == cur, iw, item, env.Rows.Unassigned(id)))
			item++
		}
	}
	return out
}

func readyGlyph(g theme.Glyphs) string {
	if g.Tier == theme.TierASCII {
		return ">"
	}
	return "▶"
}

var presentationOrder = [...]model.PresentationStatus{model.Open, model.InProgress, model.Blocked, model.Frozen, model.Closed, model.Other}

func (v *Overview) nowLines(env Env, iw int) []ovLine {
	l := env.Look
	g := l.Glyphs
	o := v.data
	row := func(role theme.Role, glyph, name string, n int) ovLine {
		num := strings.Repeat(" ", max(4-len(itoa(n)), 0)) + itoa(n)
		label := l.Fit(glyph+" "+name, max(iw-4, 0))
		return textLine(l.Fit(l.Paint(role, label)+l.Paint(theme.Strong, num), iw))
	}
	var out []ovLine
	for _, p := range presentationOrder {
		if p == model.Other && o.Counts[p] == 0 {
			continue
		}
		idx := look.StatusIndex(int(p))
		out = append(out, row(theme.StatusRole(idx), g.Status[idx], p.String(), o.Counts[p]))
		if p == model.Open {
			out = append(out, row(theme.Success, readyGlyph(g), "Ready", o.Ready))
		}
	}
	out = append(out, textLine(""),
		textLine(l.Fit(l.Paint(theme.Dim, "closed/day, 14d"), iw)),
		textLine(l.Fit(l.Paint(theme.Success, sparkline(g, o.Closed14[:])), iw)),
		textLine(l.Fit(l.Paint(theme.Dim, itoa(o.ClosedPercent())+"% closed · "+itoa(o.Deps)+" deps"), iw)))
	return out
}

func (v *Overview) counters(env Env) string {
	l := env.Look
	g := l.Glyphs
	o := v.data
	var parts []string
	add := func(role theme.Role, glyph string, n int, name string) {
		parts = append(parts, l.Paint(role, glyph+itoa(n)+" "+name))
	}
	add(theme.StatusOpen, g.Status[0], o.Counts[model.Open], "open")
	add(theme.Success, readyGlyph(g), o.Ready, "ready")
	add(theme.StatusInProgress, g.Status[1], o.Counts[model.InProgress], "in prog")
	add(theme.StatusBlocked, g.Status[2], o.Counts[model.Blocked], "blocked")
	add(theme.StatusFrozen, g.Status[3], o.Counts[model.Frozen], "frozen")
	add(theme.StatusClosed, g.Status[4], o.Counts[model.Closed], "closed")
	if n := o.Counts[model.Other]; n > 0 {
		add(theme.StatusOther, g.Status[5], n, "other")
	}
	return strings.Join(parts, " ")
}

func sparkline(g theme.Glyphs, counts []int) string {
	ticks := []rune("▁▂▃▄▅▆▇█")
	if g.Tier == theme.TierASCII {
		ticks = []rune("._,-~=*#")
	}
	most := 1
	for _, n := range counts {
		most = max(most, n)
	}
	var b strings.Builder
	for _, n := range counts {
		b.WriteRune(ticks[n*(len(ticks)-1)/most])
	}
	return b.String()
}

// box draws a card of exactly w x h: a rule with the title, then the lines. A
// focusable region (r >= 0) keeps its cursor line in view and records where it
// was drawn.
func (v *Overview) box(env Env, r ovRegion, title string, lines []ovLine, w, h, x, y int) []string {
	l := env.Look
	g := l.Glyphs
	focused := r >= 0 && v.focused(r)
	titleRole := theme.Strong
	if focused {
		titleRole = theme.Primary
		title = g.FoldClosed + title
	}
	bh := h - 1
	off := 0
	if r >= 0 {
		off = v.scrollTo(env, r, lines, bh)
		v.boxes[r] = ovBox{x: x, y: y, w: w, h: h}
		at := make([]int, len(lines))
		for i, ln := range lines {
			at[i] = ln.item
		}
		v.boxes[r].at = at
	}
	var right []look.Seg
	if rest := len(lines) - off - bh; rest > 0 {
		right = l.Words(look.Word(theme.Dim, "+"+itoa(rest)))
	}
	title = ansi.Truncate(title, max(w-look.WordCost-1-look.SegWidth(right), 1), g.Ellipsis)
	out := []string{l.Rule(w, l.Words(look.Word(titleRole, title)), right)}
	for i := range bh {
		s := ""
		if off+i < len(lines) {
			s = lines[off+i].s
		}
		out = append(out, l.Fit(s, w))
	}
	return out
}

// scrollTo returns the first shown line of a region, keeping its cursor line
// in view when the cursor moved since the last frame.
func (v *Overview) scrollTo(env Env, r ovRegion, lines []ovLine, bh int) int {
	keep := -1
	if i := v.cursorRow(env, r); i >= 0 {
		keep = lineOf(lines, i)
	}
	off := v.off[r]
	if keep != v.keep[r] && keep >= 0 && !v.held[r] {
		if keep < off {
			off = keep
		} else if keep >= off+bh {
			off = keep - bh + 1
		}
	}
	off = min(max(off, 0), max(len(lines)-bh, 0))
	v.off[r], v.keep[r], v.held[r] = off, keep, false
	return off
}

// lineOf is the body line that shows item.
func lineOf(lines []ovLine, item int) int {
	for i, ln := range lines {
		if ln.item == item {
			return i
		}
	}
	return -1
}

// hjoin places blocks side by side, gap cells apart, as h lines.
func hjoin(l look.Look, h, gap int, blocks ...[]string) []string {
	out := make([]string, h)
	sep := strings.Repeat(" ", gap)
	for y := range out {
		parts := make([]string, len(blocks))
		for i, b := range blocks {
			if y < len(b) {
				parts[i] = b[y]
			} else {
				parts[i] = l.Fit("", blockWidth(b))
			}
		}
		out[y] = strings.Join(parts, sep)
	}
	return out
}

func blockWidth(b []string) int {
	if len(b) == 0 {
		return 0
	}
	return ansi.StringWidth(b[0])
}

// pad extends lines to h rows of w cells.
func pad(l look.Look, lines []string, w, h int) []string {
	for len(lines) < h {
		lines = append(lines, l.Fit("", w))
	}
	return lines[:h]
}
