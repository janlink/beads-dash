package ui

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/bd"
	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/refresh"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/form"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

const (
	maxTitle     = 200
	depBlocks    = "blocks"
	priorityBase = "P"
)

var (
	priorityOptions = []string{"P0", "P1", "P2", "P3", "P4"}
	fallbackTypes   = []string{"task", "bug", "feature", "epic", "chore"}
)

// Keys of the issue form's fields.
const (
	fTitle       = "title"
	fType        = "type"
	fPriority    = "priority"
	fStatus      = "status"
	fReason      = "reason"
	fAssignee    = "assignee"
	fLabels      = "labels"
	fParent      = "parent"
	fDescription = "description"
	fBlockedBy   = "blockedby"
	fBlocks      = "blocks"
	fNotes       = "notes"
	fDesign      = "design"
	fAcceptance  = "acceptance"
	fDue         = "due"
	fDefer       = "defer"
	fEstimate    = "estimate"
	fExtRef      = "extref"
)

// conflictChange is a field that changed under an open edit form.
type conflictChange struct {
	key, label string
	from, to   string
	// both is set when the user changed the field too; submit then waits
	// for a decision.
	both, decided bool
}

// issueForm creates an issue or edits one. In edit mode it sends only the
// fields the user changed.
type issueForm struct {
	formDialog
	edit bool
	id   string
	// opened is the updated_at the edit started from.
	opened time.Time
	// applied are the values the last write tried to set, by field key: a
	// snapshot that shows them is the user's own write coming back.
	applied   map[string]string
	conflicts []conflictChange
	// closing is set while a save includes the close step.
	closing   bool
	inherited []string
	assignees []string
	labels    []string
}

func (a *App) typeNames() []string {
	var out []string
	for _, t := range a.bds.Types {
		out = append(out, t.Name)
	}
	if len(out) == 0 {
		return slices.Clone(fallbackTypes)
	}
	return out
}

func (a *App) statusNames() []string {
	var out []string
	for _, s := range a.bds.Statuses.All() {
		out = append(out, s.Name)
	}
	return out
}

func parentOf(is *model.Issue) string {
	if is.Parent != "" {
		return is.Parent
	}
	for _, e := range is.Dependencies {
		if e.Type == model.EdgeParentChild {
			return e.To
		}
	}
	return ""
}

func priorityText(p int) string { return priorityBase + strconv.Itoa(p) }

func dateText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.Local).Format("2006-01-02")
}

// issueValues is the value each form field has for an issue, by field key.
func issueValues(snap *model.Snapshot, is *model.Issue) map[string]string {
	v := map[string]string{
		fTitle: is.Title, fType: is.IssueType, fPriority: priorityText(is.Priority), fStatus: is.Status,
		fAssignee: is.Assignee, fLabels: strings.Join(is.Labels, " "), fParent: parentOf(is),
		fDescription: is.Description, fNotes: is.Notes, fDesign: is.Design, fAcceptance: is.AcceptanceCriteria,
		fDue: dateText(is.DueAt), fDefer: dateText(is.DeferUntil), fExtRef: is.ExternalRef,
	}
	if is.EstimatedMinutes > 0 {
		v[fEstimate] = strconv.Itoa(is.EstimatedMinutes)
	}
	var by, blocks []string
	for _, e := range is.Dependencies {
		if e.Type == depBlocks {
			by = append(by, e.To)
		}
	}
	for _, e := range snap.Dependents(is.ID) {
		if e.Type == depBlocks {
			blocks = append(blocks, e.From)
		}
	}
	v[fBlockedBy], v[fBlocks] = strings.Join(by, " "), strings.Join(blocks, " ")
	return v
}

func (a *App) issueSuggestions() (assignees, labels []string) {
	seenA, seenL := map[string]bool{}, map[string]bool{}
	if a.snap != nil {
		for _, id := range a.snap.IDs() {
			is, _ := a.snap.Issue(id)
			if is.Assignee != "" && !seenA[is.Assignee] {
				seenA[is.Assignee] = true
				assignees = append(assignees, is.Assignee)
			}
			for _, l := range is.Labels {
				if !seenL[l] {
					seenL[l] = true
					labels = append(labels, l)
				}
			}
		}
	}
	sort.Strings(assignees)
	sort.Strings(labels)
	return assignees, labels
}

func complete(pool []string) func(string) []string {
	return func(prefix string) []string {
		var out []string
		for _, s := range pool {
			if strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix)) {
				out = append(out, s)
			}
		}
		return out
	}
}

func (a *App) newIssueForm(edit bool, id, title string) *issueForm {
	d := &issueForm{edit: edit, id: id, applied: map[string]string{}}
	d.assignees, d.labels = a.issueSuggestions()
	vals := map[string]string{fType: "", fPriority: "P2"}
	title2 := "New issue"
	if edit {
		is, _ := a.snap.Issue(id)
		vals = issueValues(a.snap, is)
		d.opened = is.UpdatedAt
		title2 = "Edit " + id
	} else {
		vals[fTitle] = title
		if types := a.typeNames(); slices.Contains(types, "task") {
			vals[fType] = "task"
		} else if len(types) > 0 {
			vals[fType] = types[0]
		}
		if cur := a.sess.Current(); cur != "" && a.snap != nil {
			if is, ok := a.snap.Issue(cur); ok && is.IssueType == "epic" {
				vals[fParent] = cur
				d.inherited = slices.Clone(is.Labels)
				vals[fLabels] = strings.Join(is.Labels, " ")
			}
		}
	}
	fields := []*form.Field{
		required(form.NewText(fTitle, "Title", vals[fTitle])),
		form.NewChoice(fType, "Type", a.typeNames(), vals[fType]),
		form.NewChoice(fPriority, "Priority", priorityOptions, vals[fPriority]),
	}
	if edit {
		status := form.NewChoice(fStatus, "Status", a.statusNames(), vals[fStatus])
		reason := form.NewText(fReason, "Reason", "")
		reason.Skip = true
		reason.Placeholder = "optional"
		fields = append(fields, status, reason)
	}
	assignee := form.NewText(fAssignee, "Assignee", vals[fAssignee])
	assignee.Suggest = complete(append([]string{"me"}, d.assignees...))
	labels := form.NewTokens(fLabels, "Labels", strings.Fields(vals[fLabels]))
	labels.Suggest = complete(d.labels)
	parent := form.NewLinks(fParent, "Parent", strings.Fields(vals[fParent]))
	desc := form.NewArea(fDescription, "Description", vals[fDescription], 4)
	fields = append(fields, assignee, labels, parent, desc, form.NewFold(),
		adv(form.NewLinks(fBlockedBy, "Blocked by", strings.Fields(vals[fBlockedBy]))),
		adv(form.NewLinks(fBlocks, "Blocks", strings.Fields(vals[fBlocks]))),
		adv(form.NewArea(fNotes, "Notes", vals[fNotes], 3)),
		adv(form.NewArea(fDesign, "Design", vals[fDesign], 3)),
		adv(form.NewArea(fAcceptance, "Acceptance", vals[fAcceptance], 3)),
		adv(form.NewText(fDue, "Due", vals[fDue])),
		adv(form.NewText(fDefer, "Defer", vals[fDefer])),
		adv(form.NewText(fEstimate, "Estimate", vals[fEstimate])),
		adv(form.NewText(fExtRef, "External ref", vals[fExtRef])),
	)
	d.f = form.New(fields...)
	d.f.Track = edit
	d.f.Field(fDue).Placeholder, d.f.Field(fDefer).Placeholder = "date, e.g. 2026-10-01 or +1d", "date, e.g. +1d"
	d.f.Field(fEstimate).Placeholder = "minutes"
	d.formDialog = formDialog{a: a, h: d, self: d, title: title2, f: d.f}
	d.noteStatus()
	return d
}

func required(f *form.Field) *form.Field { f.Required = true; return f }

func adv(f *form.Field) *form.Field { f.Advanced = true; return f }

// openNew opens the form for a new issue with an optional title.
func (a *App) openNew(title string) tea.Cmd {
	if a.snap == nil {
		a.hint = "bd has not answered yet"
		return nil
	}
	a.pushDialog(a.newIssueForm(false, "", title))
	return nil
}

// openEdit opens the form for the current issue.
func (a *App) openEdit() tea.Cmd {
	id := a.sess.Current()
	if a.snap == nil || id == "" {
		a.hint = "no issue to edit"
		return nil
	}
	if _, ok := a.snap.Issue(id); !ok {
		a.hint = "no issue to edit"
		return nil
	}
	a.pushDialog(a.newIssueForm(true, id, ""))
	return nil
}

func (d *issueForm) dirty() bool { return d.f.Changes() > 0 }

func (d *issueForm) aside() string {
	var parts []string
	if d.edit {
		switch n := d.f.Changes(); n {
		case 0:
		case 1:
			parts = append(parts, "1 change")
		default:
			parts = append(parts, fmt.Sprintf("%d changes", n))
		}
	}
	if n := d.undecided(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d to decide", n))
	}
	return strings.Join(parts, " · ")
}

func (d *issueForm) undecided() int {
	n := 0
	for _, c := range d.conflicts {
		if c.both && !c.decided {
			n++
		}
	}
	return n
}

func (d *issueForm) conflicted() bool { return len(d.conflicts) > 0 }

func (d *issueForm) extra(act keys.Action) tea.Cmd {
	switch act { //nolint:exhaustive // only the conflict actions are extra
	case keys.KeepMine:
		d.decide(false)
	case keys.TakeTheirs:
		d.decide(true)
	case keys.Reload:
		d.reload()
	}
	return nil
}

func (d *issueForm) conflictAt(key string) *conflictChange {
	for i := range d.conflicts {
		if d.conflicts[i].key == key {
			return &d.conflicts[i]
		}
	}
	return nil
}

func (d *issueForm) decide(theirs bool) {
	f := d.f.Focused()
	c := d.conflictAt(f.Key)
	if c == nil || !c.both || c.decided {
		return
	}
	if theirs {
		f.Set(c.to)
	}
	f.Orig = c.to
	f.Note, f.Err = "", ""
	c.decided = true
}

// reload drops the user's edits and reads the issue afresh.
func (d *issueForm) reload() {
	is, ok := d.a.snap.Issue(d.id)
	if !ok {
		return
	}
	vals := issueValues(d.a.snap, is)
	for _, f := range d.f.Fields {
		if v, has := vals[f.Key]; has {
			f.Set(v)
			f.Rebase()
			f.Note, f.Err = "", ""
		}
	}
	d.opened, d.conflicts = is.UpdatedAt, nil
	d.errShort, d.errFull = "", nil
	d.noteStatus()
}

// Snapshot merges what changed in the issue since the form last saw it.
func (d *issueForm) Snapshot() {
	if !d.edit || d.busy {
		return
	}
	is, ok := d.a.snap.Issue(d.id)
	if !ok {
		d.errShort = d.id + " no longer exists"
		d.errFull = []string{d.errShort}
		return
	}
	if is.UpdatedAt.Equal(d.opened) {
		return
	}
	vals := issueValues(d.a.snap, is)
	for _, f := range d.f.Fields {
		sv, tracked := vals[f.Key]
		if !tracked || f.Skip || form.Same(f.Kind, sv, f.Orig) {
			continue
		}
		mine := f.Value()
		switch {
		case form.Same(f.Kind, sv, mine), form.Same(f.Kind, sv, d.applied[f.Key]) && d.applied[f.Key] != "":
			f.Orig = sv
		case !f.Changed():
			d.note(conflictChange{key: f.Key, label: f.Label, from: f.Orig, to: sv})
			f.Set(sv)
			f.Rebase()
		default:
			c := conflictChange{key: f.Key, label: f.Label, from: f.Orig, to: sv, both: true}
			d.note(c)
			f.Note = "changed underneath: now " + shortValue(sv)
		}
	}
	d.opened = is.UpdatedAt
	d.noteStatus()
}

func (d *issueForm) note(c conflictChange) {
	if old := d.conflictAt(c.key); old != nil {
		c.from = old.from
		*old = c
		return
	}
	d.conflicts = append(d.conflicts, c)
}

func shortValue(s string) string {
	s = oneLineText(s)
	if utf8.RuneCountInString(s) > 24 {
		return string([]rune(s)[:23]) + "…"
	}
	if s == "" {
		return "empty"
	}
	return s
}

func (d *issueForm) banner(l look.Look, w int) []string {
	if len(d.conflicts) == 0 {
		return nil
	}
	parts := make([]string, 0, len(d.conflicts))
	for _, c := range d.conflicts {
		parts = append(parts, fmt.Sprintf("%s %s %s %s", strings.ToLower(c.label), shortValue(c.from), l.Glyphs.Arrow, shortValue(c.to)))
	}
	var out []string
	for _, line := range wrapText("Changed since opened: "+strings.Join(parts, ", "), w-2) {
		out = append(out, l.Paint(theme.Warning, "! "+line))
	}
	if n := d.undecided(); n > 0 {
		out = append(out, l.Paint(theme.Warning, "  Decide on the marked fields: keep mine or take theirs."))
	}
	return append(out, "")
}

// noteStatus keeps the status hints and the reason field in line with the
// chosen status.
func (d *issueForm) noteStatus() {
	if !d.edit {
		return
	}
	status, reason := d.f.Field(fStatus), d.f.Field(fReason)
	switch v := status.Value(); {
	case v == "blocked":
		status.Note = "raw status; for a dependency use Blocked by"
	case d.a.snap != nil && d.a.snap.IsBlocked(d.id) && !d.a.closesTo(v):
		status.Note = "blocked by " + d.a.snap.FirstBlocker(d.id) + " (dependency)"
	default:
		if status.Note == "" || !strings.HasPrefix(status.Note, "changed underneath") {
			status.Note = ""
		}
	}
	reason.Skip = !status.Changed() || (!d.a.closesTo(status.Value()) && status.Orig != "closed")
	reason.Placeholder = "optional, sent to bd close or reopen"
}

func (d *issueForm) edited(f *form.Field) {
	if f.Key == fStatus {
		d.noteStatus()
	}
	if c := d.conflictAt(f.Key); c != nil && c.both && !c.decided {
		f.Note = "changed underneath: now " + shortValue(c.to)
	}
}

func (d *issueForm) pick(f *form.Field) tea.Cmd {
	a := d.a
	self := d.id
	switch f.Key {
	case fParent:
		a.pushDialog(a.newPicker(pickerOpts{
			Title: "Parent",
			Valid: func(id string) (bool, string) {
				if self != "" && (id == self || a.isDescendant(id, self)) {
					return false, "is the issue or below it"
				}
				return true, ""
			},
			Done: func(_ *App, ids []string) (tea.Cmd, bool) {
				d.setParent(ids[0])
				return nil, true
			},
		}))
	case fBlockedBy, fBlocks:
		blockedBy := f.Key == fBlockedBy
		title := "Blocked by"
		if !blockedBy {
			title = "Blocks"
		}
		a.pushDialog(a.newPicker(pickerOpts{
			Title: title, Multi: true,
			Valid: func(id string) (bool, string) {
				switch {
				case self == "":
					return true, ""
				case id == self:
					return false, "is this issue"
				case blockedBy && a.dependsOn(id, self):
					return false, "would make a cycle"
				case !blockedBy && a.dependsOn(self, id):
					return false, "would make a cycle"
				}
				return true, ""
			},
			Done: func(_ *App, ids []string) (tea.Cmd, bool) {
				for _, id := range ids {
					f.Add(id)
				}
				return nil, true
			},
		}))
	}
	return nil
}

// setParent sets the parent field. On create the parent's labels come along
// visibly, in place of those the previous parent brought.
func (d *issueForm) setParent(id string) {
	d.f.Field(fParent).SetList([]string{id})
	if d.edit {
		return
	}
	labels := d.f.Field(fLabels)
	keep := slices.DeleteFunc(labels.List(), func(l string) bool { return slices.Contains(d.inherited, l) })
	d.inherited = nil
	if is, ok := d.a.snap.Issue(id); ok {
		d.inherited = slices.Clone(is.Labels)
	}
	for _, l := range d.inherited {
		if !slices.Contains(keep, l) {
			keep = append(keep, l)
		}
	}
	labels.Set(strings.Join(keep, " "))
	labels.Rebase()
}

// isDescendant reports whether id sits below ancestor.
func (a *App) isDescendant(id, ancestor string) bool {
	seen := map[string]bool{}
	var walk func(string) bool
	walk = func(p string) bool {
		for _, c := range a.snap.Children(p) {
			if c == id || (!seen[c] && func() bool { seen[c] = true; return walk(c) }()) {
				return true
			}
		}
		return false
	}
	return walk(ancestor)
}

// dependsOn reports whether from reaches to over blocking edges.
func (a *App) dependsOn(from, to string) bool {
	seen := map[string]bool{from: true}
	stack := []string{from}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		is, ok := a.snap.Issue(id)
		if !ok {
			continue
		}
		for _, e := range is.Dependencies {
			if !model.BlockingEdge(e.Type) {
				continue
			}
			if e.To == to {
				return true
			}
			if !seen[e.To] {
				seen[e.To] = true
				stack = append(stack, e.To)
			}
		}
	}
	return false
}

func (d *issueForm) invalid(key, msg string) bool {
	f := d.f.Field(key)
	if f.Err == "" {
		f.Err = msg
	}
	return true
}

// validate checks what bd would refuse on sight and reports whether all is
// well; the first bad field takes the focus.
func (d *issueForm) validate() bool {
	bad := ""
	flag := func(key, msg string) {
		d.invalid(key, msg)
		if bad == "" {
			bad = key
		}
	}
	title := strings.TrimSpace(d.f.Field(fTitle).Value())
	switch {
	case title == "":
		flag(fTitle, "Title is required")
	case utf8.RuneCountInString(title) > maxTitle:
		flag(fTitle, fmt.Sprintf("Title is longer than %d characters", maxTitle))
	}
	if e := d.f.Field(fEstimate).Value(); e != "" {
		if n, err := strconv.Atoi(e); err != nil || n < 0 {
			flag(fEstimate, "Estimate is a whole number of minutes")
		}
	}
	if d.val(fAssignee) == "me" && d.a.actor() == "" {
		flag(fAssignee, errNoActor)
	}
	for _, c := range d.conflicts {
		if c.both && !c.decided {
			flag(c.key, "Changed underneath: keep mine or take theirs")
		}
	}
	if bad != "" {
		d.f.FocusKey(bad)
		return false
	}
	return true
}

func (d *issueForm) submit() tea.Cmd {
	for _, f := range d.f.Fields {
		f.Err = ""
	}
	if !d.validate() {
		return nil
	}
	if d.edit {
		return d.submitEdit()
	}
	return d.submitCreate()
}

func (d *issueForm) val(key string) string { return d.f.Field(key).Value() }

func priorityOf(v string) *int {
	if len(v) == 2 && v[0] == priorityBase[0] && v[1] >= '0' && v[1] <= '4' {
		n := int(v[1] - '0')
		return &n
	}
	return nil
}

func (d *issueForm) actorFor(v string) string {
	switch v {
	case "me":
		return d.a.actor()
	case "-":
		return ""
	}
	return v
}

func minutes(v string) *int {
	if v == "" {
		n := 0
		return &n
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

func (d *issueForm) submitCreate() tea.Cmd {
	spec := bd.CreateSpec{
		Title: d.val(fTitle), Type: d.val(fType), Priority: priorityOf(d.val(fPriority)),
		Description: d.val(fDescription), Parent: firstOr(d.f.Field(fParent).List()),
		Assignee: d.actorFor(d.val(fAssignee)), Labels: d.f.Field(fLabels).List(),
		Design: d.val(fDesign), Acceptance: d.val(fAcceptance), Notes: d.val(fNotes),
		ExternalRef: d.val(fExtRef), Due: d.val(fDue), Defer: d.val(fDefer),
		Deps: d.f.Field(fBlockedBy).List(),
	}
	if e := d.val(fEstimate); e != "" {
		spec.Estimate = minutes(e)
	}
	if spec.Parent != "" {
		if is, ok := d.a.snap.Issue(spec.Parent); ok {
			for _, l := range is.Labels {
				if !slices.Contains(spec.Labels, l) {
					spec.NoInheritLabels = true
				}
			}
		}
	}
	blocks := d.f.Field(fBlocks).List()
	ids := slices.Concat(spec.Deps, blocks)
	if spec.Parent != "" {
		ids = append(ids, spec.Parent)
	}
	return d.start(writeOp{
		ids: ids,
		run: func(ctx context.Context, c bd.Client) (string, error) {
			id, err := c.Create(ctx, spec)
			if id != "" {
				refresh.Own(ctx, id)
			}
			if err != nil {
				return id, err
			}
			for _, other := range blocks {
				if err := c.DepAdd(ctx, other, id, depBlocks); err != nil {
					return id, err
				}
			}
			return id, nil
		},
	})
}

func firstOr(l []string) string {
	if len(l) == 0 {
		return ""
	}
	return l[0]
}

// diff is what an edit changes: an update, a close or reopen, and the edges.
type diff struct {
	update       bd.UpdateSpec
	closeIt      bool
	reopen       bool
	reason       string
	depAdd       [][2]string
	depRemove    [][2]string
	applied      map[string]string
	touched      []string
	changedCount int
}

func (d *issueForm) diff() diff {
	out := diff{applied: map[string]string{}, touched: []string{d.id}}
	changed := func(key string) (*form.Field, bool) {
		f := d.f.Field(key)
		if f.Skip || !f.Changed() {
			return f, false
		}
		out.applied[key] = f.Value()
		out.changedCount++
		return f, true
	}
	text := func(key string, set **string) {
		if f, ok := changed(key); ok {
			v := f.Value()
			*set = &v
		}
	}
	u := &out.update
	text(fTitle, &u.Title)
	text(fType, &u.Type)
	text(fDescription, &u.Description)
	text(fNotes, &u.Notes)
	text(fDesign, &u.Design)
	text(fAcceptance, &u.Acceptance)
	text(fExtRef, &u.ExternalRef)
	text(fDue, &u.Due)
	text(fDefer, &u.Defer)
	if f, ok := changed(fPriority); ok {
		u.Priority = priorityOf(f.Value())
	}
	if f, ok := changed(fAssignee); ok {
		v := d.actorFor(f.Value())
		u.Assignee = &v
	}
	if f, ok := changed(fEstimate); ok {
		u.Estimate = minutes(f.Value())
	}
	if f, ok := changed(fParent); ok {
		v := firstOr(f.List())
		u.Parent = &v
	}
	if f, ok := changed(fLabels); ok {
		was, now := strings.Fields(f.Orig), f.List()
		for _, l := range now {
			if !slices.Contains(was, l) {
				u.AddLabels = append(u.AddLabels, l)
			}
		}
		for _, l := range was {
			if !slices.Contains(now, l) {
				u.RemoveLabels = append(u.RemoveLabels, l)
			}
		}
	}
	if f, ok := changed(fStatus); ok {
		out.reason = d.val(fReason)
		switch {
		case d.a.closesTo(f.Value()):
			out.closeIt = f.Orig != "closed"
		case f.Orig == "closed":
			out.reopen = true
			if f.Value() != "open" {
				v := f.Value()
				u.Status = &v
			}
		default:
			v := f.Value()
			u.Status = &v
		}
	}
	edges := func(key string, add func(other string) [2]string) {
		f, ok := changed(key)
		if !ok {
			return
		}
		was, now := strings.Fields(f.Orig), f.List()
		for _, x := range now {
			if !slices.Contains(was, x) {
				out.depAdd = append(out.depAdd, add(x))
				out.touched = append(out.touched, x)
			}
		}
		for _, x := range was {
			if !slices.Contains(now, x) {
				out.depRemove = append(out.depRemove, add(x))
				out.touched = append(out.touched, x)
			}
		}
	}
	edges(fBlockedBy, func(x string) [2]string { return [2]string{d.id, x} })
	edges(fBlocks, func(x string) [2]string { return [2]string{x, d.id} })
	return out
}

func (d *issueForm) submitEdit() tea.Cmd {
	df := d.diff()
	if df.changedCount == 0 {
		d.errShort, d.errFull = "Nothing changed", nil
		return nil
	}
	d.applied = df.applied
	d.closing = df.closeIt
	id := d.id
	return d.start(writeOp{
		ids: df.touched,
		run: func(ctx context.Context, c bd.Client) (string, error) {
			ids := []string{id}
			if df.reopen {
				if _, err := c.Reopen(ctx, ids, df.reason); err != nil {
					return "", err
				}
			}
			if !df.update.Empty() {
				if err := c.Update(ctx, ids, df.update); err != nil {
					return "", err
				}
			}
			for _, e := range df.depAdd {
				if err := c.DepAdd(ctx, e[0], e[1], depBlocks); err != nil {
					return "", err
				}
			}
			for _, e := range df.depRemove {
				if err := c.DepRemove(ctx, e[0], e[1]); err != nil {
					return "", err
				}
			}
			if df.closeIt {
				if _, err := c.Close(ctx, ids, df.reason); err != nil {
					return "", err
				}
			}
			return "", nil
		},
	})
}

func (d *issueForm) finish(res writeResult) bool {
	a := d.a
	switch {
	case res.err == nil && d.edit:
		a.toast("updated " + d.id)
		return true
	case res.err == nil:
		a.toast("created " + res.created)
		a.focusIssue(res.created)
		return true
	case res.created != "":
		a.warn(fmt.Sprintf("created %s, but %s", res.created, writeSummary(res.err, 1)))
		a.focusIssue(res.created)
		return true
	}
	if d.edit && d.closing {
		cd := a.newCloseDialog([]string{d.id}, false)
		if cd.stuck = cd.stuckOn(res.err); len(cd.stuck) > 0 {
			cd.text = writeDetail(res.err)
			a.pushDialog(cd)
			return true
		}
	}
	d.fail(res.err)
	d.attribute(res.err)
	d.Snapshot()
	return false
}

// attribute marks the field a bd refusal names, so the message also stands
// under the value that caused it.
func (d *issueForm) attribute(err error) {
	if !bd.IsClass(err, bd.ClassRejected) {
		return
	}
	msg := strings.ToLower(d.errShort)
	for _, key := range []string{fPriority, fEstimate, fTitle, fParent, fDue, fDefer, fType, fStatus, fAssignee} {
		f := d.f.Field(key)
		if !f.Skip && strings.Contains(msg, key) {
			f.Err = shortLine(d.errShort)
			return
		}
	}
}
