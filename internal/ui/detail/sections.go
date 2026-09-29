package detail

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// statusName is how the header names a presentation status, with the raw
// status appended when it says something else.
func statusName(pres model.Presentation, raw string) string {
	name := pres.Status.String()
	if strings.ReplaceAll(strings.ToLower(name), " ", "_") != raw && raw != "" {
		name += " (" + raw + ")"
	}
	return name
}

func (p *Panel) headLine(in Input, is *model.Issue) string {
	l := in.Look
	g := l.Glyphs
	pres := in.Snap.Present(is.ID, in.Statuses)
	idx := look.StatusIndex(int(pres.Status))
	statusRole := theme.StatusRole(idx)
	parts := []string{
		l.Paint(theme.Strong, is.ID),
		l.Paint(statusRole, strings.TrimRight(g.Status[idx], " ")+" "+statusName(pres, is.Status)),
	}
	if pres.BlockedMarker {
		parts = append(parts, l.Paint(theme.StatusBlocked, "blocked"))
	}
	if is.IssueType != "" {
		parts = append(parts, l.Paint(theme.TypeRole(is.IssueType), oneLine(is.IssueType)))
	}
	parts = append(parts, l.Paint(theme.PriorityRole(is.Priority), fmt.Sprintf("P%d", min(max(is.Priority, 0), 9))))
	if is.Assignee != "" {
		parts = append(parts, l.Paint(theme.Dim, "@"+oneLine(is.Assignee)))
	}
	if prog, ok := in.Snap.Progress(is.ID, in.Statuses); ok {
		s := l.Paint(theme.Text, fmt.Sprintf("%d/%d", prog.Direct.Closed, prog.Direct.Total))
		if prog.Descendants.Total > 0 {
			full := prog.Descendants.Closed * barW / prog.Descendants.Total
			s += " " + l.Paint(theme.Success, strings.Repeat(strings.TrimRight(g.BarFull, " "), full)) +
				l.Paint(theme.Faint, strings.Repeat(strings.TrimRight(g.BarEmpty, " "), barW-full))
		}
		parts = append(parts, s)
	}
	return " " + strings.Join(parts, "  ")
}

// changedLine lists the events behind the issue's live highlight.
func (p *Panel) changedLine(in Input) string {
	l := in.Look
	var texts []string
	for _, e := range in.Events {
		t := e.Kind.Label()
		switch {
		case e.Detail != "" && (e.Kind == model.KindPriorityChanged || e.Kind == model.KindStatusChanged):
			t = string(e.Kind) + " " + e.Detail
		case e.Detail != "":
			t += " " + e.Detail
		}
		if !slices.Contains(texts, t) {
			texts = append(texts, t)
		}
	}
	const shown = 3
	if len(texts) > shown {
		texts = append(texts[len(texts)-shown:], fmt.Sprintf("+%d earlier", len(texts)-shown))
	}
	last := in.Events[len(in.Events)-1]
	when := ago(in.Now.Sub(last.Time))
	if last.Actor != "" {
		when += " by " + last.Actor
	}
	return " " + l.Paint(theme.Changed, l.Glyphs.Change+" Changed") + "  " + l.Paint(theme.Text, strings.Join(texts, " · ")+" · ") + l.Paint(theme.Dim, when)
}

func ago(d time.Duration) string {
	d = max(d, 0)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s ago", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d/time.Hour))
	}
	return fmt.Sprintf("%d d ago", int(d/(24*time.Hour)))
}

// ref is one line naming another issue: status glyph, ID and title.
func ref(in Input, id string, w int) string {
	l := in.Look
	is, ok := in.Snap.Issue(id)
	if !ok {
		return l.Paint(theme.Faint, ansi.Truncate("? "+id+" (not in the snapshot)", w, l.Glyphs.Ellipsis))
	}
	pres := in.Snap.Present(id, in.Statuses)
	idx := look.StatusIndex(int(pres.Status))
	glyph := strings.TrimRight(l.Glyphs.Status[idx], " ")
	idRole, titleRole := theme.Dim, theme.Text
	if pres.Status == model.Closed {
		idRole, titleRole = theme.Faint, theme.Dim
	}
	head := glyph + " " + id + " "
	room := max(w-ansi.StringWidth(head), 0)
	return l.Paint(theme.StatusRole(idx), glyph) + " " + l.Paint(idRole, id) + " " + l.Paint(titleRole, ansi.Truncate(oneLine(is.Title), room, l.Glyphs.Ellipsis))
}

func (p *Panel) dependencies(in Input, is *model.Issue, bw int) block {
	l := in.Look
	snap := in.Snap
	var waits, holds, related []string
	seen := map[string]bool{}
	parent := is.Parent
	for _, e := range is.Dependencies {
		if e.To == "" || (e.From != "" && e.From != is.ID) {
			continue
		}
		switch {
		case e.Type == model.EdgeParentChild:
			if parent == "" {
				parent = e.To
			}
		case model.BlockingEdge(e.Type):
			if !seen[e.To] {
				seen[e.To] = true
				waits = append(waits, e.To)
			}
		default:
			related = append(related, e.To)
		}
	}
	for _, id := range snap.BlockedBy(is.ID) {
		if !seen[id] {
			seen[id] = true
			waits = append(waits, id)
		}
	}
	for _, e := range snap.Dependents(is.ID) {
		if model.BlockingEdge(e.Type) && !slices.Contains(holds, e.From) {
			holds = append(holds, e.From)
		}
	}
	open := 0
	for _, id := range waits {
		if _, ok := snap.Issue(id); !ok || snap.Present(id, in.Statuses).Status != model.Closed {
			open++
		}
	}
	if parent == "" && len(waits) == 0 && len(holds) == 0 && len(related) == 0 {
		return block{none: true}
	}
	var sum []string
	if parent != "" {
		sum = append(sum, "parent "+parent)
	}
	if len(waits) > 0 {
		sum = append(sum, fmt.Sprintf("waits on %d (%d open)", len(waits), open))
	}
	if len(holds) > 0 {
		sum = append(sum, fmt.Sprintf("holds up %d", len(holds)))
	}
	b := block{summary: strings.Join(sum, " · ")}
	label := func(s string) string { return l.Paint(theme.Faint, fmt.Sprintf("%-9s", s)) }
	target := func(id string) {
		if _, ok := snap.Issue(id); ok {
			b.targets = append(b.targets, target{id: id, line: len(b.body)})
		}
	}
	if parent != "" {
		target(parent)
		b.body = append(b.body, label("parent")+ref(in, parent, bw-9))
	}
	list := func(head string, ids []string) {
		if len(ids) == 0 {
			return
		}
		b.body = append(b.body, l.Paint(theme.Dim, head))
		for _, id := range ids[:min(len(ids), refCap)] {
			target(id)
			b.body = append(b.body, "  "+ref(in, id, bw-2))
		}
		if len(ids) > refCap {
			b.body = append(b.body, "  "+l.Paint(theme.Faint, fmt.Sprintf("+%d more", len(ids)-refCap)))
		}
	}
	list(fmt.Sprintf("waits on %d (%d open)", len(waits), open), waits)
	list(fmt.Sprintf("holds up %d", len(holds)), holds)
	if len(related) > 0 {
		b.body = append(b.body, label("related")+l.Paint(theme.Dim, ansi.Truncate(strings.Join(related, ", "), max(bw-9, 0), l.Glyphs.Ellipsis)))
	}
	b.body = append(b.body, l.Paint(theme.Faint, "focus graph comes with the graph view"))
	return b
}

func (p *Panel) children(in Input, is *model.Issue, bw int) block {
	l := in.Look
	live, closed := model.SortedChildren(in.Snap, in.Statuses, is.ID)
	if len(live)+len(closed) == 0 {
		return block{none: true}
	}
	b := block{}
	if prog, ok := in.Snap.Progress(is.ID, in.Statuses); ok {
		b.summary = fmt.Sprintf("%d/%d closed", prog.Direct.Closed, prog.Direct.Total)
	}
	add := func(ids []string) {
		for _, id := range ids[:min(len(ids), childCap)] {
			b.targets = append(b.targets, target{id: id, line: len(b.body)})
			b.body = append(b.body, in.Rows.Tree(model.TreeRow{Kind: model.TreeIssue, ID: id})(bw, false))
		}
		if len(ids) > childCap {
			b.body = append(b.body, l.Paint(theme.Faint, fmt.Sprintf("+%d more", len(ids)-childCap)))
		}
	}
	add(live)
	if len(closed) > 0 {
		b.targets = append(b.targets, target{fold: true, line: len(b.body)})
		b.body = append(b.body, in.Rows.Tree(model.TreeRow{Kind: model.TreeClosedFold, ID: is.ID, Foldable: true, Folded: !p.closedOpen, Closed: len(closed)})(bw, false))
		if p.closedOpen {
			add(closed)
		}
	}
	return b
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

func (p *Panel) details(in Input, is *model.Issue, bw int) block {
	l := in.Look
	b := block{}
	row := func(name, value string) {
		if value != "" {
			b.body = append(b.body, l.Paint(theme.Faint, fmt.Sprintf("%-9s", name))+value)
		}
	}
	when := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return l.Paint(theme.Text, stamp(t)) + l.Paint(theme.Dim, " · "+ago(in.Now.Sub(t)))
	}
	if len(is.Labels) > 0 {
		row("labels", labelList(l, is.Labels, bw-9))
	}
	created := when(is.CreatedAt)
	if created != "" && is.CreatedBy != "" {
		created += l.Paint(theme.Dim, " · "+oneLine(is.CreatedBy))
	}
	row("created", created)
	row("updated", when(is.UpdatedAt))
	row("started", when(is.StartedAt))
	closed := when(is.ClosedAt)
	if closed != "" && is.CloseReason != "" {
		closed += l.Paint(theme.Dim, " · "+oneLine(is.CloseReason))
	}
	row("closed", closed)
	row("due", when(is.DueAt))
	if is.Owner != "" {
		row("owner", l.Paint(theme.Text, oneLine(is.Owner)))
	}
	if is.ExternalRef != "" {
		row("ref", l.Paint(theme.Text, oneLine(is.ExternalRef)))
	}
	if is.EstimatedMinutes > 0 {
		row("estimate", l.Paint(theme.Text, fmt.Sprintf("%d min", is.EstimatedMinutes)))
	}
	if is.CommentCount > 0 {
		row("comments", l.Paint(theme.Text, fmt.Sprint(is.CommentCount)))
	}
	sum := []string{"created " + shortAgo(in.Now, is.CreatedAt)}
	if !is.UpdatedAt.IsZero() {
		sum = append(sum, "updated "+shortAgo(in.Now, is.UpdatedAt))
	}
	if n := len(is.Labels); n > 0 {
		sum = append(sum, fmt.Sprintf("%d labels", n))
	}
	b.summary = strings.Join(sum, " · ")
	if len(b.body) == 0 {
		return block{none: true}
	}
	return b
}

func shortAgo(now, t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return strings.TrimSuffix(ago(now.Sub(t)), " ago") + " ago"
}

// labelList joins labels and ends with +n when they do not all fit in w.
func labelList(l look.Look, labels []string, w int) string {
	var out []string
	used := 0
	for i, name := range labels {
		name = oneLine(name)
		rest := len(labels) - i
		cost := ansi.StringWidth(name)
		if len(out) > 0 {
			cost += 2
		}
		reserve := 0
		if rest > 1 {
			reserve = 2 + len(fmt.Sprintf("+%d", rest-1))
		}
		if len(out) > 0 && used+cost+reserve > w {
			out = append(out, l.Paint(theme.Dim, fmt.Sprintf("+%d", rest)))
			break
		}
		out = append(out, l.Paint(theme.Primary, name))
		used += cost
	}
	return strings.Join(out, "  ")
}
