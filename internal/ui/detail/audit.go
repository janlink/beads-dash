package detail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
)

const (
	commentLines = 12
	maxAudit     = 64
)

// AuditKind names one of the bd reads behind the audit trail.
type AuditKind uint8

// The reads.
const (
	AuditComments AuditKind = iota
	AuditHistory

	auditKinds
)

// Fetch is one bd read the panel wants. Seq identifies it: a result is only
// taken while it carries the Seq the panel last issued for its issue and kind.
type Fetch struct {
	ID   string
	Kind AuditKind
	Seq  int
}

// FetchResult is a finished Fetch.
type FetchResult struct {
	Fetch
	Comments []bd.Comment
	History  []bd.HistoryEntry
	Err      error
}

type loadState uint8

const (
	loadNone loadState = iota
	loadLoading
	loadDone
	loadFailed
)

type change struct {
	when time.Time
	text string
}

type auditPart struct {
	state    loadState
	err      string
	comments []bd.Comment
	changes  []change
}

// auditEntry is what was fetched for one issue as of its updated_at and
// comment count; adding a comment does not touch updated_at.
type auditEntry struct {
	updated  int64
	comments int
	used     int
	parts    [auditKinds]auditPart
}

type auditKey struct {
	id   string
	kind AuditKind
}

// switchTo resets the state that belongs to the issue shown.
func (p *Panel) switchTo(id string) {
	if id == p.forID {
		return
	}
	p.forID = id
	p.expanded = [sectionCount]bool{}
	p.scroll, p.cursor, p.row, p.closedOpen, p.historyOpen = 0, Description, -1, false, false
}

// PlanAudit lists the bd reads the audit trail needs for the issue shown and
// has not got: comments while the section is open and the issue has any,
// history while its subsection is open. Cached reads stay until the issue's
// updated_at or comment count changes. cancel lists reads still running that are no longer
// wanted; their results are dropped.
func (p *Panel) PlanAudit(in Input) (start, cancel []Fetch) {
	p.switchTo(in.ID)
	var is *model.Issue
	if in.Snap != nil && in.ID != "" {
		is, _ = in.Snap.Issue(in.ID)
	}
	var want [auditKinds]bool
	if is != nil {
		if e := p.audit[is.ID]; e != nil && (e.updated != is.UpdatedAt.UnixNano() || e.comments != is.CommentCount) {
			for k := range auditKinds {
				if seq, ok := p.pending[auditKey{is.ID, k}]; ok {
					cancel = append(cancel, Fetch{is.ID, k, seq})
					delete(p.pending, auditKey{is.ID, k})
				}
			}
			delete(p.audit, is.ID)
		}
		want[AuditComments] = p.open[Audit] && is.CommentCount > 0
		want[AuditHistory] = p.open[Audit] && p.historyOpen
	}
	for k, seq := range p.pending {
		if is == nil || k.id != is.ID || !want[k.kind] {
			cancel = append(cancel, Fetch{k.id, k.kind, seq})
			delete(p.pending, k)
			if e := p.audit[k.id]; e != nil && e.parts[k.kind].state == loadLoading {
				e.parts[k.kind] = auditPart{}
			}
		}
	}
	if is == nil {
		return start, cancel
	}
	e := p.entry(is)
	p.tick++
	e.used = p.tick
	for k := range auditKinds {
		if !want[k] || e.parts[k].state != loadNone {
			continue
		}
		p.seq++
		p.pending[auditKey{is.ID, k}] = p.seq
		e.parts[k].state = loadLoading
		start = append(start, Fetch{is.ID, k, p.seq})
	}
	return start, cancel
}

// CancelAudit drops every running read, for when the detail is not shown.
func (p *Panel) CancelAudit() []Fetch {
	var cancel []Fetch
	for k, seq := range p.pending {
		cancel = append(cancel, Fetch{k.id, k.kind, seq})
		delete(p.pending, k)
		if e := p.audit[k.id]; e != nil && e.parts[k.kind].state == loadLoading {
			e.parts[k.kind] = auditPart{}
		}
	}
	return cancel
}

func (p *Panel) entry(is *model.Issue) *auditEntry {
	if e := p.audit[is.ID]; e != nil {
		return e
	}
	if len(p.audit) >= maxAudit {
		oldest := ""
		for id, o := range p.audit {
			_, c := p.pending[auditKey{id, AuditComments}]
			_, h := p.pending[auditKey{id, AuditHistory}]
			if id != is.ID && !c && !h && (oldest == "" || o.used < p.audit[oldest].used) {
				oldest = id
			}
		}
		delete(p.audit, oldest)
	}
	e := &auditEntry{updated: is.UpdatedAt.UnixNano(), comments: is.CommentCount}
	p.audit[is.ID] = e
	return e
}

// ApplyAudit stores a finished read; one that was cancelled or superseded is
// dropped.
func (p *Panel) ApplyAudit(r FetchResult) {
	key := auditKey{r.ID, r.Kind}
	if seq, ok := p.pending[key]; !ok || seq != r.Seq {
		return
	}
	delete(p.pending, key)
	e := p.audit[r.ID]
	if e == nil {
		return
	}
	part := &e.parts[r.Kind]
	if r.Err != nil {
		*part = auditPart{state: loadFailed, err: oneLine(r.Err.Error())}
		return
	}
	*part = auditPart{state: loadDone}
	switch r.Kind {
	case AuditComments:
		part.comments = make([]bd.Comment, len(r.Comments))
		for i, c := range r.Comments {
			part.comments[len(r.Comments)-1-i] = c
		}
	case AuditHistory:
		part.changes = reduceHistory(r.History)
	case auditKinds:
	}
}

// RetryAudit forgets failed reads so the next plan asks again.
func (p *Panel) RetryAudit() {
	for _, e := range p.audit {
		for k := range auditKinds {
			if e.parts[k].state == loadFailed {
				e.parts[k] = auditPart{}
			}
		}
	}
}

// reduceHistory keeps the entries that changed something, newest first; the
// oldest entry stands for the creation.
func reduceHistory(h []bd.HistoryEntry) []change {
	var out []change
	for i, e := range h {
		if i == len(h)-1 {
			out = append(out, change{e.CommitDate, "created"})
			break
		}
		if s := model.ChangeSummary(&h[i+1].Issue, &e.Issue); len(s) > 0 {
			out = append(out, change{e.CommitDate, strings.Join(s, "; ")})
		}
	}
	return out
}

func (p *Panel) onHistory() bool {
	t := p.targets[p.cursor]
	return p.row >= 0 && p.row < len(t) && t[p.row].hist
}

func (p *Panel) historyRow() int {
	for i, t := range p.targets[Audit] {
		if t.hist {
			return i
		}
	}
	return -1
}

func (p *Panel) auditTrail(in Input, is *model.Issue, bw, pw int) block {
	l := in.Look
	g := l.Glyphs
	b := block{summary: "no comments"}
	if is.CommentCount > 0 {
		b.summary = fmt.Sprintf("%d comments", is.CommentCount)
	}
	if !p.open[Audit] {
		return b
	}
	var parts [auditKinds]auditPart
	if e := p.audit[is.ID]; e != nil {
		parts = e.parts
	}
	note := func(s string) { b.body = append(b.body, l.Paint(theme.Faint, s)) }
	failed := func(what string, part auditPart) {
		b.body = append(b.body, l.Paint(theme.Error, ansi.Truncate(what+" failed: "+part.err, bw, g.Ellipsis)))
	}
	switch c := parts[AuditComments]; {
	case is.CommentCount == 0:
		note("no comments")
	case c.state == loadFailed:
		failed("comments", c)
	case c.state != loadDone:
		note("loading comments" + g.Ellipsis)
	default:
		for _, cm := range c.comments {
			b.body = append(b.body, p.commentLines(in, cm, pw)...)
		}
	}

	h := parts[AuditHistory]
	title := "Recorded changes"
	if h.state == loadDone {
		title += fmt.Sprintf(" (%d)", len(h.changes))
	}
	mark := g.FoldClosed
	if p.historyOpen {
		mark = g.FoldOpen
	}
	b.targets = append(b.targets, target{hist: true, line: len(b.body)})
	b.body = append(b.body, l.Paint(theme.Dim, mark+" ")+l.Paint(theme.Text, title)+l.Paint(theme.Faint, " · no author recorded"))
	if !p.historyOpen {
		return b
	}
	switch {
	case h.state == loadFailed:
		failed("history", h)
	case h.state != loadDone:
		note("  loading history" + g.Ellipsis)
	case len(h.changes) == 0:
		note("  no recorded changes")
	default:
		for _, c := range h.changes {
			line := "  " + l.Paint(theme.Dim, stamp(c.when)) + "  " + l.Paint(theme.Text, c.text)
			b.body = append(b.body, ansi.Truncate(line, bw, g.Ellipsis))
		}
	}
	return b
}

func (p *Panel) commentLines(in Input, c bd.Comment, w int) []string {
	l := in.Look
	who := oneLine(c.Author)
	if who == "" {
		who = "unknown"
	}
	head := l.Paint(theme.Strong, who)
	if !c.CreatedAt.IsZero() {
		head += l.Paint(theme.Dim, " · "+ago(in.Now.Sub(c.CreatedAt)))
	}
	out := []string{head}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(strings.ReplaceAll(c.Text, "\r\n", "\n")), "\n") {
		lines = append(lines, strings.Split(ansi.Wrap(oneLine(line), max(w-2, 1), ""), "\n")...)
	}
	for i, line := range lines {
		if i == commentLines {
			out = append(out, "  "+l.Paint(theme.Faint, fmt.Sprintf("%s %d more lines", l.Glyphs.Ellipsis, len(lines)-commentLines)))
			break
		}
		out = append(out, "  "+l.Paint(theme.Text, line))
	}
	return out
}
