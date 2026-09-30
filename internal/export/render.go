package export

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
)

const timeLayout = "2006-01-02 15:04 -07:00"

const textRule = 72

type section struct {
	name string
	body string
}

type entry struct {
	id, title string
	facts     string
	dates     string
	sections  []section
}

func renderDoc(in Input, md bool) string {
	var b strings.Builder
	n := len(in.IDs)
	if n != 1 {
		header(&b, in, md)
	}
	for i, id := range in.IDs {
		is, _ := in.Snap.Issue(id)
		e := build(in, is, md)
		switch {
		case n == 1:
			e.write(&b, 1, md)
		case md:
			if i > 0 {
				b.WriteString("\n---\n\n")
			}
			e.write(&b, 2, md)
		default:
			b.WriteString("\n" + strings.Repeat("=", textRule) + "\n\n")
			e.write(&b, 2, md)
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func header(b *strings.Builder, in Input, md bool) {
	prefix := in.Prefix
	if prefix == "" {
		prefix = "bdash"
	}
	title := fmt.Sprintf("%s · %s", prefix, issueWord(len(in.IDs)))
	meta := []string{in.Set.String()}
	if in.Set == Scope && in.Query != "" {
		meta = append(meta, "query "+in.Query)
	}
	meta = append(meta, "exported "+in.Now.In(in.loc()).Format(timeLayout))
	if in.Version != "" {
		meta = append(meta, "bdash "+in.Version)
	}
	if md {
		b.WriteString("# " + title + "\n\n" + strings.Join(meta, " · ") + "\n\n")
		b.WriteString("| ID | Title | Status | P |\n| --- | --- | --- | --- |\n")
		for _, id := range in.IDs {
			is, _ := in.Snap.Issue(id)
			pres := in.Snap.Present(id, in.Statuses)
			fmt.Fprintf(b, "| %s | %s | %s | P%d |\n", cell(id), cell(is.Title), cell(statusName(pres, is.Status, false)), is.Priority)
		}
		b.WriteString("\n---\n\n")
		return
	}
	b.WriteString(title + "\n" + strings.Repeat("=", ansi.StringWidth(title)) + "\n\n" + strings.Join(meta, " · ") + "\n")
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.Join(strings.Fields(s), " ")
}

func issueWord(n int) string {
	if n == 1 {
		return "1 issue"
	}
	return fmt.Sprintf("%d issues", n)
}

// statusName is the presentation status, the raw status in parentheses when
// it says something else.
func statusName(pres model.Presentation, raw string, marker bool) string {
	name := pres.Status.String()
	if strings.ReplaceAll(strings.ToLower(name), " ", "_") != raw && raw != "" {
		name += " (" + raw + ")"
	}
	if marker && pres.BlockedMarker {
		name += " · blocked"
	}
	return name
}

func build(in Input, is *model.Issue, md bool) entry {
	pres := in.Snap.Present(is.ID, in.Statuses)
	loc := in.loc()
	parts := []string{statusName(pres, is.Status, true)}
	parts = append(parts, fmt.Sprintf("P%d", is.Priority))
	if is.IssueType != "" {
		parts = append(parts, is.IssueType)
	}
	if is.Assignee != "" {
		parts = append(parts, "@"+is.Assignee)
	}
	if len(is.Labels) > 0 {
		labels := make([]string, len(is.Labels))
		for i, l := range is.Labels {
			labels[i] = l
			if md {
				labels[i] = "`" + l + "`"
			}
		}
		parts = append(parts, strings.Join(labels, " "))
	}
	e := entry{id: is.ID, title: oneLine(is.Title), facts: strings.Join(parts, " · ")}

	var dates []string
	stamp := func(label string, t time.Time) {
		if !t.IsZero() {
			dates = append(dates, label+" "+t.In(loc).Format(timeLayout))
		}
	}
	stamp("Created", is.CreatedAt)
	stamp("updated", is.UpdatedAt)
	stamp("started", is.StartedAt)
	if !is.ClosedAt.IsZero() {
		closed := "closed " + is.ClosedAt.In(loc).Format(timeLayout)
		if r := oneLine(is.CloseReason); r != "" {
			closed += " (" + r + ")"
		}
		dates = append(dates, closed)
	}
	if len(dates) > 0 {
		dates[0] = capitalize(dates[0])
	}
	e.dates = strings.Join(dates, " · ")

	add := func(name, body string) {
		if body = tidy(body); body != "" {
			e.sections = append(e.sections, section{name, body})
		}
	}
	add("Description", is.Description)
	add("Design", is.Design)
	add("Acceptance criteria", is.AcceptanceCriteria)
	add("Notes", is.Notes)
	add("Relations", strings.Join(relations(in, is), "\n"))
	if in.WithComments {
		add("Comments", commentsBody(in.Comments[is.ID], loc, md))
	}
	return e
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func tidy(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	return strings.Trim(s, "\n \t")
}

func (e entry) write(b *strings.Builder, level int, md bool) {
	head := e.id + " · " + e.title
	if md {
		b.WriteString(strings.Repeat("#", level) + " " + head + "\n\n")
	} else {
		b.WriteString(head + "\n" + strings.Repeat("=", ansi.StringWidth(head)) + "\n\n")
	}
	b.WriteString(e.facts + "\n")
	if e.dates != "" {
		b.WriteString(e.dates + "\n")
	}
	for _, s := range e.sections {
		if md {
			b.WriteString("\n" + strings.Repeat("#", level+1) + " " + s.name + "\n\n")
		} else {
			b.WriteString("\n" + s.name + "\n" + strings.Repeat("-", utf8.RuneCountInString(s.name)) + "\n\n")
		}
		b.WriteString(s.body + "\n")
	}
}

type related struct {
	label string
	text  string
}

// relations lists the issue's edges in glossary words. Incoming edges come
// from the snapshot's dependents, so each is inverted client-side.
func relations(in Input, is *model.Issue) []string {
	snap := in.Snap
	var out []related
	entryText := func(id string) string {
		o, ok := snap.Issue(id)
		if !ok {
			return id
		}
		pres := snap.Present(id, in.Statuses)
		return fmt.Sprintf("%s %s (%s)", id, oneLine(o.Title), pres.Status)
	}
	parent := is.Parent
	for _, e := range is.Dependencies {
		if e.Type == model.EdgeParentChild && e.To != "" && parent == "" {
			parent = e.To
		}
	}
	if parent != "" {
		out = append(out, related{"Parent", entryText(parent)})
	}
	for _, c := range snap.Children(is.ID) {
		out = append(out, related{"Children", entryText(c)})
	}
	for _, e := range is.Dependencies {
		if e.To == "" || e.Type == model.EdgeParentChild {
			continue
		}
		out = append(out, related{outgoing(e.Type), entryText(e.To)})
	}
	for _, e := range snap.Dependents(is.ID) {
		out = append(out, related{incoming(e.Type), entryText(e.From)})
	}
	rank := func(label string) int {
		i := slices.Index(relationOrder, label)
		if i < 0 {
			return len(relationOrder)
		}
		return i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := rank(out[i].label), rank(out[j].label); ri != rj {
			return ri < rj
		}
		if rank(out[i].label) == len(relationOrder) && out[i].label != out[j].label {
			return out[i].label < out[j].label
		}
		return false
	})
	lines := make([]string, len(out))
	for i, r := range out {
		lines[i] = "- " + r.label + ": " + r.text
	}
	return lines
}

var relationOrder = []string{"Parent", "Children", "Waits on", "Holds up", "Related", "Discovered from", "Discovered"}

func outgoing(t string) string {
	switch {
	case model.BlockingEdge(t):
		return "Waits on"
	case t == "related":
		return "Related"
	case t == "discovered-from":
		return "Discovered from"
	}
	return t
}

func incoming(t string) string {
	switch {
	case model.BlockingEdge(t):
		return "Holds up"
	case t == "related":
		return "Related"
	case t == "discovered-from":
		return "Discovered"
	}
	return t + " (incoming)"
}

func commentsBody(list []bd.Comment, loc *time.Location, md bool) string {
	sorted := slices.Clone(list)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].CreatedAt.Before(sorted[j].CreatedAt) })
	var parts []string
	for _, c := range sorted {
		who := c.Author
		if md {
			who = "**" + who + "**"
		}
		parts = append(parts, who+" · "+c.CreatedAt.In(loc).Format(timeLayout)+"\n"+tidy(c.Text))
	}
	return strings.Join(parts, "\n\n")
}
