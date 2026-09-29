// Package dialog draws overlay dialogs: the shared frame with its title, aside
// and generated key hints, the dimmed backdrop, and the Appearance dialog.
package dialog

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

const (
	// MaxWidth is the widest a dialog gets.
	MaxWidth = 80
	// FullScreenCols and FullScreenRows are the terminal size below which a
	// dialog fills the screen.
	FullScreenCols = 80
	FullScreenRows = 24
)

// Frame is what a dialog shows.
type Frame struct {
	Title string
	// Aside sits right of the title, e.g. "field 3/8 · 2 changes".
	Aside string
	// Hints come from the dialog's key map, most important first.
	Hints []keys.Hint
	// Body holds styled lines; lines wider than the frame are cut and the
	// body scrolls when it is taller.
	Body   []string
	Scroll int
	// KeepBackdrop leaves the view behind undimmed, for dialogs that preview
	// changes on it.
	KeepBackdrop bool
}

// FullScreen reports whether a dialog fills a terminal of this size.
func FullScreen(cols, rows int) bool { return cols < FullScreenCols || rows < FullScreenRows }

// Size is the outer size of a dialog whose body has n lines.
func Size(cols, rows, n int) (w, h int) {
	if FullScreen(cols, rows) {
		return cols, rows
	}
	return min(MaxWidth, cols-4), min(n+2, rows-4)
}

// Page is how many body lines a dialog shows at once, for paging keys.
func Page(cols, rows, n int) int {
	_, h := Size(cols, rows, n)
	return max(h-2, 1)
}

// MaxScroll is the largest Scroll that still shows something for a body of n
// lines.
func MaxScroll(cols, rows, n int) int { return max(n-Page(cols, rows, n), 0) }

type border struct{ tl, tr, bl, br, h, v string }

func borderFor(l look.Look) border {
	if l.Glyphs.Tier == theme.TierASCII {
		return border{"+", "+", "+", "+", "-", "|"}
	}
	return border{"╭", "╮", "╰", "╯", "─", "│"}
}

// Box draws the framed dialog exactly w by h cells.
func Box(l look.Look, f Frame, w, h int) []string {
	if w < 8 || h < 3 {
		return nil
	}
	bd := borderFor(l)
	iw, ch := w-4, h-2
	line := func(content string) string {
		return l.Paint(theme.Border, bd.v) + " " + l.Fit(content, iw) + " " + l.Paint(theme.Border, bd.v)
	}

	out := make([]string, 0, h)
	out = append(out, topBorder(l, bd, f, w))
	body, more := window(f.Body, f.Scroll, ch)
	for _, b := range body {
		out = append(out, line(b))
	}
	if more > 0 {
		out = append(out, line(l.Paint(theme.Faint, fmt.Sprintf("%s %d more", l.Glyphs.Ellipsis, more))))
	}
	for len(out) < h-1 {
		out = append(out, line(""))
	}
	return append(out, bottomBorder(l, bd, f.Hints, w))
}

// window returns the lines to show in ch rows and how many are cut below.
// While something is cut below, the last row goes to the "more" marker.
func window(body []string, scroll, ch int) (shown []string, more int) {
	if len(body) <= ch {
		return body, 0
	}
	last := len(body) - ch
	scroll = min(max(scroll, 0), last)
	if scroll == last {
		return body[scroll:], 0
	}
	room := ch - 1
	return body[scroll : scroll+room], len(body) - scroll - room
}

func topBorder(l look.Look, bd border, f Frame, w int) string {
	title := " " + f.Title + " "
	aside := ""
	if f.Aside != "" {
		aside = " " + f.Aside + " "
	}
	room := w - 4
	if ansi.StringWidth(title)+ansi.StringWidth(aside)+1 > room {
		aside = ""
	}
	title = ansi.Truncate(title, room, l.Glyphs.Ellipsis)
	fill := max(room-ansi.StringWidth(title)-ansi.StringWidth(aside), 0)
	return l.Paint(theme.Border, bd.tl+bd.h) + l.Paint(theme.Strong, title) +
		l.Paint(theme.Border, strings.Repeat(bd.h, fill)) + l.Paint(theme.Dim, aside) +
		l.Paint(theme.Border, bd.h+bd.tr)
}

func bottomBorder(l look.Look, bd border, hints []keys.Hint, w int) string {
	room := w - 4
	var text string
	used := 0
	for _, h := range hints {
		part := l.Paint(theme.Primary, h.Key) + " " + l.Paint(theme.Dim, h.Desc)
		pw := ansi.StringWidth(h.Key) + 1 + ansi.StringWidth(h.Desc)
		sep, sw := "", 0
		if text != "" {
			sep, sw = l.Paint(theme.Faint, "  "), 2
		}
		if used+sw+pw+2 > room {
			break
		}
		text += sep + part
		used += sw + pw
	}
	if text != "" {
		text = " " + text + " "
		used += 2
	}
	fill := max(room-used, 0)
	return l.Paint(theme.Border, bd.bl+strings.Repeat(bd.h, fill+1)) + text + l.Paint(theme.Border, bd.h+bd.br)
}

// Overlay draws f centred over backdrop (the view, cols by rows), dimming the
// backdrop except at colour depth none or when the frame asks to keep it. At
// sizes below 80×24 the dialog is the whole screen.
func Overlay(l look.Look, cols, rows int, backdrop string, f Frame) string {
	w, h := Size(cols, rows, len(f.Body))
	box := Box(l, f, w, h)
	if FullScreen(cols, rows) {
		return strings.Join(box, "\n")
	}
	lines := strings.Split(backdrop, "\n")
	for len(lines) < rows {
		lines = append(lines, "")
	}
	dim := !f.KeepBackdrop && l.Palette.Depth() != theme.DepthNone
	x, y := (cols-w)/2, (rows-h)/2
	out := make([]string, rows)
	for i := range out {
		bl := ansi.Truncate(lines[i], cols, "")
		if pad := cols - ansi.StringWidth(bl); pad > 0 {
			bl += strings.Repeat(" ", pad)
		}
		if dim {
			bl = l.Paint(theme.Faint, ansi.Strip(bl))
		}
		if i < y || i >= y+len(box) {
			out[i] = bl
			continue
		}
		out[i] = ansi.Truncate(bl, x, "") + reset(bl) + box[i-y] + ansi.Cut(bl, x+w, cols)
	}
	return strings.Join(out, "\n")
}

func reset(s string) string {
	if strings.Contains(s, "\x1b") {
		return "\x1b[m"
	}
	return ""
}
