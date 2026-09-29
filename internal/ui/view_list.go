package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/rows"
)

// listRow is one row of a list view; sel says whether the cursor can rest on
// it. Headers are rows the cursor skips.
type listRow struct {
	key string
	sel bool
}

// moveTo returns the index the cursor lands on after a navigation action,
// starting from index from (negative when the cursor is on no row). Landing
// on a row the cursor cannot rest on moves on in the direction of travel,
// then back.
func moveTo(rows []listRow, from int, act keys.Action, page int) int {
	n := len(rows)
	if n == 0 {
		return -1
	}
	dir, to := 1, from
	switch act { //nolint:exhaustive // only the navigation actions move the cursor
	case keys.NavDown:
		to = from + 1
	case keys.NavUp:
		to, dir = from-1, -1
	case keys.NavFirst:
		to = 0
	case keys.NavLast:
		to, dir = n-1, -1
	case keys.NavHalfDown:
		to = from + max(page/2, 1)
	case keys.NavHalfUp:
		to, dir = from-max(page/2, 1), -1
	case keys.NavPageDown:
		to = from + max(page, 1)
	case keys.NavPageUp:
		to, dir = from-max(page, 1), -1
	}
	if from < 0 {
		to, dir = 0, 1
	}
	to = min(max(to, 0), n-1)
	for _, d := range []int{dir, -dir} {
		for i := to; i >= 0 && i < n; i += d {
			if rows[i].sel {
				return i
			}
		}
	}
	return from
}

// scopeInfo is what the header says about the scope: the query, how many of
// the issues the view lists, and the closed issues it hides.
type scopeInfo struct {
	active       bool
	query        string
	marker       string
	ellipsis     string
	shown, total int
	closedHidden bool
	plusClosed   int
	unknown      []string
}

// scopeInfoFor describes the scope for a view that lists shown issues.
// hidesClosed says whether the view applies the status visibility.
func scopeInfoFor(env Env, shown int, hidesClosed bool) scopeInfo {
	g := env.Look.Glyphs
	marker := "⌕"
	if g.Tier == theme.TierASCII {
		marker = "/"
	}
	si := scopeInfo{
		active: env.Scope.Active(), query: env.Scope.Query(), marker: marker, ellipsis: g.Ellipsis,
		shown: shown, unknown: env.Scope.Unknown(),
	}
	if env.Snap != nil {
		si.total = env.Snap.Len()
	}
	if hidesClosed && !env.Scope.ShowClosed() {
		si.closedHidden = true
		if env.Matches != nil {
			si.plusClosed = env.Matches.HiddenClosed
		}
	}
	return si
}

// fit is the label in at most w cells; the query is cut out in the middle
// when it is longer, the counts and notes after it stay whole. Nothing when no
// scope is active or w is too small to say anything.
func (si scopeInfo) fit(w int) string {
	if !si.active || w < 8 {
		return ""
	}
	parts := []string{fmt.Sprintf("%d/%d", si.shown, si.total)}
	if si.closedHidden {
		parts = append(parts, "closed hidden")
	}
	if si.plusClosed > 0 {
		parts = append(parts, fmt.Sprintf("+%d closed", si.plusClosed))
	}
	if len(si.unknown) > 0 {
		parts = append(parts, "unknown: "+strings.Join(si.unknown, ", "))
	}
	head := si.marker + " "
	tail := " · " + strings.Join(parts, " · ")
	if room := w - ansi.StringWidth(head) - ansi.StringWidth(tail); room >= minQueryCells {
		return head + rows.MidCut(si.query, room, si.ellipsis) + tail
	}
	return rows.MidCut(head+si.query+tail, w, si.ellipsis)
}

const minQueryCells = 6

func itoa(n int) string { return strconv.Itoa(n) }
