package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
	"github.com/janlink/beads-dash/internal/ui/screens"
)

type span struct{ from, to int }

// tabs lays out the header's view tabs from the left edge and returns the
// text with the column span of each tab.
func (a *App) tabs() (string, [6]span) {
	var spans [6]span
	if BreakpointOf(a.cols) == Narrow {
		return " " + a.look.Paint(theme.Strong, ViewNames[a.slot]), spans
	}
	var b strings.Builder
	b.WriteString(" ")
	x := 1
	for i, name := range ViewNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		role := theme.Dim
		switch {
		case i == a.slot:
			role = theme.Primary
			label = a.look.Paint(theme.Strong, fmt.Sprintf("%d", i+1)) + " " + a.look.Paint(role, name)
			if a.look.Palette.Depth() == theme.DepthNone {
				label = fmt.Sprintf("[%d %s]", i+1, name)
			}
		case a.views[i] == nil:
			role = theme.Faint
			label = a.look.Paint(role, label)
		default:
			label = a.look.Paint(role, label)
		}
		w := ansi.StringWidth(label)
		spans[i] = span{x, x + w}
		b.WriteString(label)
		b.WriteString("  ")
		x += w + 2
	}
	return strings.TrimRight(b.String(), " "), spans
}

func (a *App) header() string {
	l := a.look
	left, _ := a.tabs()
	var right []string
	if a.bds.Untested {
		right = append(right, l.Paint(theme.Warning, "bd "+a.bds.Version.Parsed.String()+" untested"))
	}
	right = append(right, a.liveMarker())
	r := strings.Join(right, "  ") + " "
	rw := ansi.StringWidth(r)
	lw := max(a.cols-rw, 0)
	if s := a.view().Scope(lw - ansi.StringWidth(left) - 3); s != "" {
		left += "  " + l.Paint(theme.Dim, s)
	}
	return l.Fit(left, lw) + r
}

func (a *App) liveMarker() string {
	l := a.look
	g := l.Glyphs
	switch {
	case a.startErr == nil && !a.status.Loaded && a.snap == nil:
		return l.Paint(theme.Dim, g.Bullet+" connecting")
	case a.status.Stale || a.status.Err != nil:
		age := a.now().Sub(a.status.LastSuccess)
		return l.Paint(theme.Warning, g.Stale+" stale ") + l.Paint(theme.Dim, ageText(age))
	}
	word := " live "
	if BreakpointOf(a.cols) == Narrow {
		word = " "
	}
	return l.Paint(theme.Success, g.Live+word) + l.Paint(theme.Dim, a.status.LastSuccess.Format("15:04:05"))
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

// chips are the counters and health chips at the right of the footer.
func (a *App) chips() string {
	l := a.look
	var parts []string
	if n := a.sess.MarkCount(); n > 0 {
		s := fmt.Sprintf("%s %d marked", l.Glyphs.Mark, n)
		if h := a.sess.HiddenMarks(a.visible); h > 0 {
			s += fmt.Sprintf(" (%d hidden)", h)
		}
		parts = append(parts, l.Paint(theme.Primary, s))
	}
	if ids := a.hl.IDs(a.now()); len(ids) > 0 {
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
		parts = append(parts, l.Paint(theme.Changed, s))
	}
	if n, ok := a.view().(Noter); ok && a.snap != nil {
		if s := n.Note(); s != "" {
			parts = append(parts, l.Paint(theme.Dim, s))
		}
	}
	if a.status.Slow {
		parts = append(parts, l.Paint(theme.Warning, "slow"))
	}
	if a.status.GCHint {
		parts = append(parts, l.Paint(theme.Warning, "gc"))
	}
	if a.status.Fallback {
		parts = append(parts, l.Paint(theme.Warning, "journal-limited"))
	}
	return strings.Join(parts, "  ")
}

func (a *App) footer() string {
	l := a.look
	right := a.chips()
	rw := ansi.StringWidth(right)
	if rw > 0 {
		rw += 2
	}
	room := max(a.cols-rw-1, 0)
	var left string
	if a.hint != "" {
		left = " " + l.Paint(theme.Warning, a.hint)
	} else {
		hs := a.hintsFor(a.context())
		if a.emptyShown() {
			hs = append(slices.Clone(a.emptyWorkspace().Hints), hs...)
		}
		left = " " + fitHints(hs, room-1, l)
	}
	if rw == 0 {
		return l.Fit(left, a.cols)
	}
	return l.Fit(left, a.cols-rw) + right + "  "
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
