package screens

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// View is what a full-screen state needs beyond the report.
type View struct {
	Look  look.Look
	Hints []keys.Hint
	// Sel is the fix the copy key acts on.
	Sel int
	// RetryIn is the time until the next automatic recheck; zero hides it.
	RetryIn time.Duration
	Cols    int
	Rows    int
}

// SameReport points to the plain-text form of the checklist.
const SameReport = "same report: bdash -v"

// Render draws a report as exactly Rows lines of Cols cells.
func Render(v View, r Report) []string {
	room := max(v.Rows-2, 0)
	var out []string
	for _, drop := range [][3]bool{{false, false, false}, {false, false, true}, {true, false, true}, {true, true, true}} {
		out = build(v, r, drop[0], drop[1], drop[2])
		if len(out) <= room {
			break
		}
	}
	body := pageOf(v.Look, out[:min(len(out), room)], v.Cols, room)
	return append(body, pageOf(v.Look, []string{"", "  " + HintLine(v.Look, v.Hints)}, v.Cols, min(2, v.Rows))...)
}

// build lays the report out top to bottom; the drop flags leave out the
// sections that give way first when the screen is short.
func build(v View, r Report, noFacts, noWhat, noRaw bool) []string {
	l := v.Look
	cw := min(v.Cols-4, 96)
	const ind = "  "
	var out []string
	out = append(out, "")
	head := l.Paint(theme.Error, l.Glyphs.Status[2]+" ") + l.Paint(theme.Strong, r.Title)
	if v.RetryIn > 0 {
		head += l.Paint(theme.Dim, fmt.Sprintf("  rechecking in %ds", int((v.RetryIn+time.Second-1)/time.Second)))
	}
	out = append(out, ind+head, "")
	if !noWhat {
		out = append(out, wrap(l, theme.Text, r.What, cw, ind)...)
		out = append(out, "")
	}
	nameW := 0
	for _, c := range r.Checks {
		nameW = max(nameW, ansi.StringWidth(c.Name))
	}
	for _, c := range r.Checks {
		mark, role := "-", theme.Faint
		fancy := l.Glyphs.Tier == theme.TierFancy
		switch c.State {
		case Passed:
			mark, role = pick(fancy, "✓", "+"), theme.Success
		case Failed:
			mark, role = pick(fancy, "✗", "x"), theme.Error
		case NotRun:
		}
		detailRole := theme.Dim
		if c.State == Failed {
			detailRole = theme.Text
		}
		line := ind + l.Paint(role, mark) + " " + l.Paint(theme.Text, pad(c.Name, nameW)) + "  " + l.Paint(detailRole, c.Detail)
		out = append(out, l.Fit(line, v.Cols))
	}
	if len(r.Facts) > 0 && !noFacts {
		out = append(out, "")
	}
	for _, f := range r.Facts {
		if noFacts {
			break
		}
		if hasCheckDetail(r, f) {
			continue
		}
		out = append(out, l.Fit(ind+l.Paint(theme.Dim, pad(f.Label, 11))+l.Paint(theme.Text, f.Value), v.Cols))
	}
	if len(r.Fixes) > 0 {
		out = append(out, "", ind+l.Paint(theme.Primary, "What to do"))
		cmdW := 0
		for _, f := range r.Fixes {
			cmdW = max(cmdW, ansi.StringWidth(f.Cmd))
		}
		stack := cmdW+8+maxWhy(r.Fixes) > cw
		for i, f := range r.Fixes {
			mark := "  "
			cmd := l.Paint(theme.Primary, "$ "+f.Cmd)
			if i == v.Sel {
				mark = l.Paint(theme.Strong, pick(l.Glyphs.Tier == theme.TierFancy, "▸", ">")+" ")
				cmd = l.PaintSel(theme.Primary, "$ "+f.Cmd)
			}
			if stack {
				out = append(out, ind+mark+cmd, ind+"    "+l.Paint(theme.Dim, "# "+f.Why))
				continue
			}
			gap := strings.Repeat(" ", cmdW-ansi.StringWidth(f.Cmd)+2)
			out = append(out, l.Fit(ind+mark+cmd+gap+l.Paint(theme.Dim, "# "+f.Why), v.Cols))
		}
		if r.After != "" {
			out = append(out, wrap(l, theme.Faint, r.After, cw, ind+"  ")...)
		}
	}
	out = append(out, "", ind+l.Paint(theme.Faint, SameReport))
	if (r.Ran != "" || len(r.Raw) > 0) && !noRaw {
		out = append(out, "", ind+l.Paint(theme.Primary, "bd said")+l.Paint(theme.Faint, "  "+r.Ran))
		bar := l.Paint(theme.Rule, pick(l.Glyphs.Tier != theme.TierASCII, "│", "|")+" ")
		for _, line := range r.Raw {
			out = append(out, wrap(l, theme.Text, line, cw-2, ind+bar)...)
		}
	}
	return out
}

func hasCheckDetail(r Report, f Fact) bool {
	for _, c := range r.Checks {
		if c.State == Failed && strings.Contains(c.Detail, f.Value) {
			return true
		}
	}
	return false
}

func pick(fancy bool, a, b string) string {
	if fancy {
		return a
	}
	return b
}

func maxWhy(fs []Fix) int {
	m := 0
	for _, f := range fs {
		m = max(m, ansi.StringWidth(f.Why)+2)
	}
	return m
}

// TooSmall is the screen for terminals below the minimum size.
func TooSmall(l look.Look, cols, rows, needCols, needRows int) []string {
	lines := []string{
		centre(l.Paint(theme.Warning, "Terminal too small"), cols),
		centre(l.Paint(theme.Text, fmt.Sprintf("%dx%d", cols, rows))+l.Paint(theme.Dim, fmt.Sprintf(" - needs %dx%d", needCols, needRows)), cols),
		"",
		centre(HintLine(l, []keys.Hint{{Key: "q", Desc: "quit"}}), cols),
	}
	top := max(0, (rows-len(lines))/2)
	out := make([]string, 0, rows)
	for range top {
		out = append(out, "")
	}
	out = append(out, lines...)
	return pageOf(l, out, cols, rows)
}

// HintLine renders "key desc  key desc" with the key emphasised.
func HintLine(l look.Look, hs []keys.Hint) string {
	parts := make([]string, 0, len(hs))
	for _, h := range hs {
		part := l.Paint(theme.Strong, h.Key)
		if h.Desc != "" {
			part += " " + l.Paint(theme.Dim, h.Desc)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "  ")
}

// Notice is the one-line inline notice above the footer.
type Notice struct {
	Text string
	Warn bool
	// Keys are the hints shown on the right, e.g. "r retry".
	Keys []keys.Hint
}

// NoticeRow draws a notice as exactly cols cells.
func NoticeRow(l look.Look, n Notice, cols int) string {
	role := theme.Dim
	mark := l.Glyphs.Bullet
	if n.Warn {
		role, mark = theme.Warning, "!"
	}
	right := HintLine(l, n.Keys)
	rw := ansi.StringWidth(right)
	left := " " + l.Paint(role, mark) + " " + l.Paint(theme.Text, n.Text)
	if rw > 0 {
		lw := max(0, cols-rw-2)
		return l.Fit(left, lw) + "  " + right
	}
	return l.Fit(left, cols)
}

// Empty is the block shown in place of a list with nothing to list.
type Empty struct {
	Title string
	Body  string
	Hints []keys.Hint
}

// The empty states of the shell and its views.
var (
	EmptyReady    = Empty{Title: "Nothing is ready to work on.", Body: "Every open issue is blocked or already claimed."}
	EmptyBlocked  = Empty{Title: "Nothing is blocked.", Body: "No open issue waits on another."}
	EmptyScope    = Empty{Title: "The scope matches nothing.", Body: "Esc clears the scope."}
	EmptyMemories = Empty{Title: "No memories yet.", Body: "bd remember \"insight\" stores one."}
	EmptyGraph    = Empty{Title: "No dependencies to draw.", Body: "Link issues with bd dep add."}
)

// EmptyWorkspace is the block for a workspace without issues. Its hints are
// the actions; newIssue adds the shell's own key once it is bound.
func EmptyWorkspace(workspace string, newIssue bool) Empty {
	title := "No issues yet."
	if workspace != "" {
		title = "No issues in " + workspace + " yet."
	}
	e := Empty{Title: title, Body: "bdash picks up new issues within about 2 s, whoever creates them."}
	if newIssue {
		e.Hints = append(e.Hints, keys.Hint{Key: "n", Desc: "new issue"})
	}
	e.Hints = append(e.Hints, keys.Hint{Key: `bd create "First task" -t task`}, keys.Hint{Key: "bd quickstart"})
	return e
}

// RenderEmpty draws an empty state centred in w x h.
func RenderEmpty(l look.Look, e Empty, w, h int) []string {
	lines := []string{centre(l.Paint(theme.Strong, e.Title), w)}
	for _, s := range wrap(l, theme.Dim, e.Body, min(w-4, 72), "") {
		lines = append(lines, centre(s, w))
	}
	if len(e.Hints) > 0 {
		lines = append(lines, "", centre(HintLine(l, e.Hints), w))
	}
	top := max(0, (h-len(lines))/2)
	out := make([]string, 0, h)
	for range top {
		out = append(out, "")
	}
	out = append(out, lines...)
	return pageOf(l, out, w, h)
}

func pageOf(l look.Look, lines []string, w, h int) []string {
	out := make([]string, h)
	for i := range out {
		s := ""
		if i < len(lines) {
			s = lines[i]
		}
		out[i] = l.Fit(s, w)
	}
	return out
}

func centre(s string, w int) string {
	p := max(0, (w-ansi.StringWidth(s))/2)
	return strings.Repeat(" ", p) + s
}

func pad(s string, w int) string {
	if n := w - ansi.StringWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func wrap(l look.Look, r theme.Role, s string, w int, prefix string) []string {
	if w < 8 {
		w = 8
	}
	var out []string
	for _, line := range strings.Split(ansi.Wordwrap(s, w, ""), "\n") {
		out = append(out, prefix+l.Paint(r, line))
	}
	return out
}
