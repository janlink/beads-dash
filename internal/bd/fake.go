package bd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

// Fake is an in-memory [Client] for tests of the UI and the refresh engine.
// It never derives ready or blocked: tests set the readiness explicitly, and
// fake writes change issues only. All methods are safe for concurrent use.
type Fake struct {
	mu        sync.Mutex
	version   VersionInfo
	workspace Workspace
	issues    []model.Issue
	readiness model.Readiness
	statuses  model.Statuses
	types     []TypeInfo
	vc        VCStatus
	commits   int
	comments  map[string][]Comment
	history   map[string][]HistoryEntry
	memories  map[string]string
	config    map[string]ConfigValue
	errs      map[string]error
	calls     []string
	nextID    int
	now       func() time.Time
	hook      func(method string)
	journal   journalState
	writes    []FakeWrite
	refused   map[string]string
	actor     string
}

// NewFake returns a fake speaking as bd 1.3.0 in workspace /fake with the
// built-in statuses and types.
func NewFake() *Fake {
	info, _ := CheckVersion("1.3.0")
	info.Build, info.Commit, info.Branch = "fake", "fake", "HEAD"
	return &Fake{
		version:   info,
		workspace: Workspace{Path: "/fake/workspace", Prefix: "f", DatabasePath: "/fake/workspace/db"},
		statuses:  model.BuiltinStatuses(),
		types: []TypeInfo{
			{Name: "task"}, {Name: "bug"}, {Name: "feature"}, {Name: "chore"}, {Name: "epic"},
		},
		readiness: model.Readiness{Blocked: map[string][]string{}},
		vc:        VCStatus{Branch: "main", Commit: "c0"},
		comments:  map[string][]Comment{},
		history:   map[string][]HistoryEntry{},
		memories:  map[string]string{},
		config:    map[string]ConfigValue{},
		errs:      map[string]error{},
		refused:   map[string]string{},
		actor:     "fake",
		now:       time.Now,
	}
}

// SetVersion replaces the version report; it runs the real version check so
// support and capabilities follow.
func (f *Fake) SetVersion(raw string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.version, _ = CheckVersion(raw)
}

// SetWorkspace replaces the where answer.
func (f *Fake) SetWorkspace(w Workspace) { f.locked(func() { f.workspace = w }) }

// SetIssues replaces every issue. List synthesises Raw JSON for issues
// without one.
func (f *Fake) SetIssues(issues ...model.Issue) {
	f.locked(func() { f.issues = slices.Clone(issues) })
}

// SetReadiness sets bd's ready and blocked verdict verbatim; reasons maps a
// ready ID to the reason bd gives for it and may be nil.
func (f *Fake) SetReadiness(ready []string, blocked map[string][]string, reasons ...map[string]string) {
	f.locked(func() {
		f.readiness = model.Readiness{Ready: slices.Clone(ready), Blocked: map[string][]string{}, Reason: map[string]string{}}
		for _, m := range reasons {
			for k, v := range m {
				f.readiness.Reason[k] = v
			}
		}
		for k, v := range blocked {
			f.readiness.Blocked[k] = slices.Clone(v)
		}
	})
}

// SetStatuses replaces the status table.
func (f *Fake) SetStatuses(s model.Statuses) { f.locked(func() { f.statuses = s }) }

// SetTypes replaces the type list.
func (f *Fake) SetTypes(t []TypeInfo) { f.locked(func() { f.types = slices.Clone(t) }) }

// SetVCStatus replaces the version-control position.
func (f *Fake) SetVCStatus(v VCStatus) { f.locked(func() { f.vc = v }) }

// SetComments sets an issue's comments.
func (f *Fake) SetComments(id string, c ...Comment) {
	f.locked(func() { f.comments[id] = slices.Clone(c) })
}

// SetHistory sets an issue's history.
func (f *Fake) SetHistory(id string, h ...HistoryEntry) {
	f.locked(func() { f.history[id] = slices.Clone(h) })
}

// SetMemory stores a memory.
func (f *Fake) SetMemory(key, content string) { f.locked(func() { f.memories[key] = content }) }

// SetConfig sets a config key's value.
func (f *Fake) SetConfig(key, value string) {
	f.locked(func() { f.config[key] = ConfigValue{Key: key, Value: value, Location: "config.yaml"} })
}

// FailWith makes every later call of the named method (e.g. "List") return
// err; nil clears it. Use a *[Error] to simulate one of the error classes.
func (f *Fake) FailWith(method string, err error) {
	f.locked(func() {
		if err == nil {
			delete(f.errs, method)
			return
		}
		f.errs[method] = err
	})
}

// SetHook runs fn at the start of every client call, before the injected
// failure is looked up and outside the fake's lock. Tests use it to advance a
// fake clock so a call takes time, or to change the fake's state at a precise
// moment.
func (f *Fake) SetHook(fn func(method string)) { f.locked(func() { f.hook = fn }) }

// Calls returns the names of the methods called so far, in order.
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// Issues returns a copy of the fake's current issues.
func (f *Fake) Issues() []model.Issue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.issues)
}

func (f *Fake) locked(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

// enter records the call and returns the injected error, honouring the
// context first like a real bd process would.
func (f *Fake) enter(ctx context.Context, method string) error {
	f.mu.Lock()
	f.calls = append(f.calls, method)
	hook := f.hook
	f.mu.Unlock()
	if hook != nil {
		hook(method)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.errs[method]
}

var _ Client = (*Fake)(nil)

// Version implements [Client]. Like the exec client it returns the info
// together with the error for an unsupported version.
func (f *Fake) Version(ctx context.Context) (VersionInfo, error) {
	if err := f.enter(ctx, "Version"); err != nil {
		return VersionInfo{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.version.Support == Unsupported {
		return f.version, &Error{Class: ClassUnsupported, Command: "version", Message: f.version.Reason}
	}
	return f.version, nil
}

// Where implements [Client].
func (f *Fake) Where(ctx context.Context) (Workspace, error) {
	if err := f.enter(ctx, "Where"); err != nil {
		return Workspace{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.workspace, nil
}

// List implements [Client]. Issues come back in insertion order; callers
// must not rely on it.
func (f *Fake) List(ctx context.Context) ([]model.Issue, error) {
	if err := f.enter(ctx, "List"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.issues)
	for i := range out {
		if len(out[i].Raw) == 0 {
			out[i].Raw = synthRaw(out[i])
		}
	}
	return out, nil
}

func synthRaw(is model.Issue) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"id": is.ID, "title": is.Title, "status": is.Status, "issue_type": is.IssueType,
		"priority": is.Priority, "assignee": is.Assignee, "parent": is.Parent, "labels": is.Labels,
	})
	return raw
}

// Ready implements [Client].
func (f *Fake) Ready(ctx context.Context) (model.Readiness, error) {
	if err := f.enter(ctx, "Ready"); err != nil {
		return model.Readiness{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r := model.Readiness{Ready: slices.Clone(f.readiness.Ready), Blocked: map[string][]string{}, Reason: map[string]string{}}
	for k, v := range f.readiness.Reason {
		r.Reason[k] = v
	}
	for k, v := range f.readiness.Blocked {
		r.Blocked[k] = slices.Clone(v)
	}
	return r, nil
}

// Statuses implements [Client].
func (f *Fake) Statuses(ctx context.Context) (model.Statuses, error) {
	if err := f.enter(ctx, "Statuses"); err != nil {
		return model.Statuses{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statuses, nil
}

// Types implements [Client].
func (f *Fake) Types(ctx context.Context) ([]TypeInfo, error) {
	if err := f.enter(ctx, "Types"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.types), nil
}

// VCStatus implements [Client].
func (f *Fake) VCStatus(ctx context.Context) (VCStatus, error) {
	if err := f.enter(ctx, "VCStatus"); err != nil {
		return VCStatus{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vc, nil
}

// Comments implements [Client].
func (f *Fake) Comments(ctx context.Context, id string) ([]Comment, error) {
	if err := f.enter(ctx, "Comments"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.comments[id])
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// History implements [Client].
func (f *Fake) History(ctx context.Context, id string) ([]HistoryEntry, error) {
	if err := f.enter(ctx, "History"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.history[id]), nil
}

// Memories implements [Client].
func (f *Fake) Memories(ctx context.Context) ([]Memory, error) {
	if err := f.enter(ctx, "Memories"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Memory, 0, len(f.memories))
	for k, v := range f.memories {
		out = append(out, Memory{Key: k, Content: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ConfigGet implements [Client].
func (f *Fake) ConfigGet(ctx context.Context, key string) (ConfigValue, error) {
	if err := f.enter(ctx, "ConfigGet"); err != nil {
		return ConfigValue{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.config[key]; ok {
		return v, nil
	}
	return ConfigValue{Key: key}, nil
}

func (f *Fake) find(id string) (int, error) {
	for i := range f.issues {
		if f.issues[i].ID == id {
			return i, nil
		}
	}
	return -1, &Error{Class: ClassRejected, Message: fmt.Sprintf("no issue found matching %q", id)}
}

// FakeWrite records one write the fake took, so tests can tell one bd call
// with several IDs from several calls.
type FakeWrite struct {
	Method string
	IDs    []string
	Update UpdateSpec
	Create CreateSpec
	Reason string
	// Key is the memory a Remember or Forget named; Content is set by
	// Remember only.
	Key, Content string
}

// Writes returns the write calls so far, in order.
func (f *Fake) Writes() []FakeWrite {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.writes)
}

// Refuse makes a later close of id fail with msg, as one of bd's close
// guards would; "" clears it.
func (f *Fake) Refuse(id, msg string) {
	f.locked(func() {
		if msg == "" {
			delete(f.refused, id)
			return
		}
		f.refused[id] = msg
	})
}

// SetActor sets who the fake acts as; a claim sets the assignee to it.
func (f *Fake) SetActor(name string) { f.locked(func() { f.actor = name }) }

func (f *Fake) record(w FakeWrite) { f.writes = append(f.writes, w) }

func writeErr(cmd string, applied []string, failed []WriteFailure) error {
	if len(failed) == 0 {
		return nil
	}
	e := &Error{Class: ClassRejected, Command: cmd, ExitCode: 1, Applied: applied, Failed: failed, Message: failed[0].Message}
	if len(applied) > 0 {
		e.Class = ClassPartialWrite
	}
	return e
}

func (f *Fake) validPriority(p *int) error {
	if p != nil && (*p < 0 || *p > 4) {
		return &Error{Class: ClassRejected, Message: fmt.Sprintf("invalid priority %d (expected 0-4)", *p)}
	}
	return nil
}

func (f *Fake) validType(t string) error {
	if t == "" || slices.ContainsFunc(f.types, func(i TypeInfo) bool { return i.Name == t }) {
		return nil
	}
	return &Error{Class: ClassRejected, Message: fmt.Sprintf("invalid issue type: %s", t)}
}

func (f *Fake) validStatus(s string) error {
	if _, ok := f.statuses.Lookup(s); ok {
		return nil
	}
	return &Error{Class: ClassRejected, Message: fmt.Sprintf("invalid status %q", s)}
}

// Create implements [Client]. It adds an open issue and leaves readiness
// alone.
func (f *Fake) Create(ctx context.Context, spec CreateSpec) (string, error) {
	if err := f.enter(ctx, "Create"); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(FakeWrite{Method: "Create", Create: spec})
	if spec.Title == "" {
		return "", &Error{Class: ClassRejected, Command: "create", Message: "title required"}
	}
	if err := errors.Join(f.validPriority(spec.Priority), f.validType(spec.Type)); err != nil {
		return "", err
	}
	labels := slices.Clone(spec.Labels)
	if spec.Parent != "" {
		pi, err := f.find(spec.Parent)
		if err != nil {
			return "", err
		}
		if !spec.NoInheritLabels {
			for _, l := range f.issues[pi].Labels {
				if !slices.Contains(labels, l) {
					labels = append(labels, l)
				}
			}
		}
	}
	f.nextID++
	id := fmt.Sprintf("%s-new%d", f.workspace.Prefix, f.nextID)
	now := f.now()
	is := model.Issue{
		ID: id, Title: spec.Title, Status: "open", IssueType: orDefault(spec.Type, "task"), Priority: 2,
		Description: spec.Description, Assignee: spec.Assignee, Parent: spec.Parent,
		Design: spec.Design, AcceptanceCriteria: spec.Acceptance, Notes: spec.Notes,
		ExternalRef: spec.ExternalRef, Labels: labels, CreatedAt: now, UpdatedAt: now,
	}
	if spec.Estimate != nil {
		is.EstimatedMinutes = *spec.Estimate
	}
	if spec.Priority != nil {
		is.Priority = *spec.Priority
	}
	is.DueAt, is.DeferUntil = parseDate(spec.Due), parseDate(spec.Defer)
	if !is.DeferUntil.IsZero() {
		is.Status = "deferred"
	}
	if spec.Parent != "" {
		is.Dependencies = append(is.Dependencies, model.Edge{From: id, To: spec.Parent, Type: model.EdgeParentChild})
	}
	for _, d := range spec.Deps {
		typ, to := "blocks", d
		if t, target, ok := strings.Cut(d, ":"); ok {
			typ, to = t, target
		}
		is.Dependencies = append(is.Dependencies, model.Edge{From: id, To: to, Type: typ})
	}
	f.issues = append(f.issues, is)
	return id, nil
}

// parseDate reads a YYYY-MM-DD date as local midnight; anything else is no date.
func parseDate(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// Update implements [Client]. Unknown IDs and claims on an issue someone else
// holds fail per ID; the rest change, as in bd.
func (f *Fake) Update(ctx context.Context, ids []string, spec UpdateSpec) error {
	if err := f.enter(ctx, "Update"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(FakeWrite{Method: "Update", IDs: slices.Clone(ids), Update: spec})
	if spec.Empty() {
		return nil
	}
	checks := []error{f.validPriority(spec.Priority), f.validType(deref(spec.Type))}
	if spec.Status != nil {
		checks = append(checks, f.validStatus(*spec.Status))
	}
	if err := errors.Join(checks...); err != nil {
		return err
	}
	var applied []string
	var failed []WriteFailure
	for _, id := range ids {
		i, err := f.find(id)
		if err != nil {
			failed = append(failed, WriteFailure{ID: id, Message: errMessage(err)})
			continue
		}
		is := &f.issues[i]
		if spec.Claim && is.Assignee != "" && is.Assignee != f.actor && is.Status == "in_progress" {
			failed = append(failed, WriteFailure{ID: id, Message: "issue already claimed by " + is.Assignee})
			continue
		}
		f.apply(is, spec)
		applied = append(applied, id)
	}
	return writeErr("update", applied, failed)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (f *Fake) apply(is *model.Issue, spec UpdateSpec) {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = *src
		}
	}
	set(&is.Title, spec.Title)
	set(&is.Description, spec.Description)
	set(&is.Design, spec.Design)
	set(&is.AcceptanceCriteria, spec.Acceptance)
	set(&is.Notes, spec.Notes)
	set(&is.Status, spec.Status)
	set(&is.Assignee, spec.Assignee)
	set(&is.IssueType, spec.Type)
	set(&is.ExternalRef, spec.ExternalRef)
	if spec.Priority != nil {
		is.Priority = *spec.Priority
	}
	if spec.Estimate != nil {
		is.EstimatedMinutes = *spec.Estimate
	}
	if spec.Due != nil {
		is.DueAt = parseDate(*spec.Due)
	}
	if spec.Defer != nil {
		is.DeferUntil = parseDate(*spec.Defer)
		switch {
		case !is.DeferUntil.IsZero() && spec.Status == nil:
			is.Status = "deferred"
		case is.DeferUntil.IsZero() && is.Status == "deferred":
			is.Status = "open"
		}
	}
	if spec.Parent != nil {
		is.Parent = *spec.Parent
		is.Dependencies = slices.DeleteFunc(is.Dependencies, func(e model.Edge) bool { return e.Type == model.EdgeParentChild })
		if *spec.Parent != "" {
			is.Dependencies = append(is.Dependencies, model.Edge{From: is.ID, To: *spec.Parent, Type: model.EdgeParentChild})
		}
	}
	if spec.Claim {
		is.Assignee, is.Status = f.actor, "in_progress"
	}
	for _, l := range spec.AddLabels {
		if !slices.Contains(is.Labels, l) {
			is.Labels = append(is.Labels, l)
		}
	}
	is.Labels = slices.DeleteFunc(is.Labels, func(l string) bool { return slices.Contains(spec.RemoveLabels, l) })
	is.UpdatedAt = f.now()
	is.Raw = nil
}

// closeRefusal is the close guard's verdict on an issue, "" when it may
// close: an injected refusal, an open blocker or an open child.
func (f *Fake) closeRefusal(id string) string {
	if msg, ok := f.refused[id]; ok {
		return msg
	}
	if open := f.openIDs(f.readiness.Blocked[id]); len(open) > 0 {
		return fmt.Sprintf("cannot close blocked issue: %s is blocked by [%s] (use --force to override)", id, strings.Join(open, " "))
	}
	n := 0
	for _, c := range f.issues {
		if c.Parent == id && c.Status != "closed" {
			n++
		}
	}
	if n > 0 {
		return fmt.Sprintf("cannot close %s: %d open child issue(s); close children first or use --force to override", id, n)
	}
	return ""
}

func (f *Fake) openIDs(ids []string) []string {
	var out []string
	for _, id := range ids {
		if i, err := f.find(id); err == nil && f.issues[i].Status != "closed" {
			out = append(out, id)
		}
	}
	return out
}

// Close implements [Client]. Like bd it closes the issues its guards allow
// and reports the others as failures; unknown IDs fail too.
func (f *Fake) Close(ctx context.Context, ids []string, reason string) ([]string, error) {
	if err := f.enter(ctx, "Close"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(FakeWrite{Method: "Close", IDs: slices.Clone(ids), Reason: reason})
	var done []string
	var failed []WriteFailure
	for _, id := range ids {
		i, err := f.find(id)
		if err != nil {
			failed = append(failed, WriteFailure{ID: id, Message: errMessage(err)})
			continue
		}
		if msg := f.closeRefusal(id); msg != "" {
			failed = append(failed, WriteFailure{ID: id, Message: msg})
			continue
		}
		is := &f.issues[i]
		is.Status, is.CloseReason, is.ClosedAt, is.UpdatedAt, is.Raw = "closed", orDefault(reason, "Closed"), f.now(), f.now(), nil
		done = append(done, id)
	}
	return done, writeErr("close", done, failed)
}

// Reopen implements [Client]. An issue that is already open fails like it
// does in bd.
func (f *Fake) Reopen(ctx context.Context, ids []string, reason string) ([]string, error) {
	if err := f.enter(ctx, "Reopen"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(FakeWrite{Method: "Reopen", IDs: slices.Clone(ids), Reason: reason})
	var done []string
	var failed []WriteFailure
	for _, id := range ids {
		i, err := f.find(id)
		switch {
		case err != nil:
			failed = append(failed, WriteFailure{ID: id, Message: errMessage(err)})
		case f.issues[i].Status != "closed":
			failed = append(failed, WriteFailure{ID: id, Message: id + " is already open"})
		default:
			is := &f.issues[i]
			is.Status, is.CloseReason, is.ClosedAt, is.UpdatedAt, is.Raw = "open", "", time.Time{}, f.now(), nil
			done = append(done, id)
		}
	}
	return done, writeErr("reopen", done, failed)
}

// DepAdd implements [Client].
func (f *Fake) DepAdd(ctx context.Context, from, to, depType string) error {
	if err := f.enter(ctx, "DepAdd"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(FakeWrite{Method: "DepAdd", IDs: []string{from, to}})
	i, err := f.find(from)
	if err != nil {
		return err
	}
	if _, err := f.find(to); err != nil {
		return err
	}
	if f.reaches(to, from) {
		return &Error{Class: ClassRejected, Command: "dep add", Message: "adding dependency would create a cycle"}
	}
	f.issues[i].Dependencies = append(f.issues[i].Dependencies, model.Edge{From: from, To: to, Type: orDefault(depType, "blocks")})
	f.issues[i].UpdatedAt = f.now()
	f.issues[i].Raw = nil
	return nil
}

// reaches reports whether a depends on b through any chain of edges.
func (f *Fake) reaches(a, b string) bool {
	seen := map[string]bool{}
	var walk func(id string) bool
	walk = func(id string) bool {
		if id == b {
			return true
		}
		if seen[id] {
			return false
		}
		seen[id] = true
		i, err := f.find(id)
		if err != nil {
			return false
		}
		for _, e := range f.issues[i].Dependencies {
			if walk(e.To) {
				return true
			}
		}
		return false
	}
	return walk(a)
}

// DepRemove implements [Client].
func (f *Fake) DepRemove(ctx context.Context, from, to string) error {
	if err := f.enter(ctx, "DepRemove"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(FakeWrite{Method: "DepRemove", IDs: []string{from, to}})
	i, err := f.find(from)
	if err != nil {
		return err
	}
	f.issues[i].Dependencies = slices.DeleteFunc(f.issues[i].Dependencies, func(e model.Edge) bool { return e.To == to })
	f.issues[i].UpdatedAt = f.now()
	f.issues[i].Raw = nil
	return nil
}

// Comment implements [Client].
func (f *Fake) Comment(ctx context.Context, id, text string) error {
	if err := f.enter(ctx, "Comment"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i, err := f.find(id)
	if err != nil {
		return err
	}
	f.issues[i].CommentCount++
	f.issues[i].Raw = nil
	f.comments[id] = append(f.comments[id], Comment{
		ID: fmt.Sprintf("c%d", len(f.comments[id])+1), IssueID: id, Author: "fake", Text: text, CreatedAt: f.now(),
	})
	return nil
}

// Remember implements [Client].
func (f *Fake) Remember(ctx context.Context, key, content string) error {
	if err := f.enter(ctx, "Remember"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(FakeWrite{Method: "Remember", Key: key, Content: content})
	if err := CheckMemoryKey(key); err != nil {
		return &Error{Class: ClassRejected, Command: "remember", Message: err.Error()}
	}
	if strings.TrimSpace(content) == "" {
		return &Error{Class: ClassRejected, Command: "remember", Message: "content is empty"}
	}
	f.memories[key] = content
	f.bumpVC()
	return nil
}

// Forget implements [Client]. A key the fake does not hold gives
// [ErrMemoryGone].
func (f *Fake) Forget(ctx context.Context, key string) error {
	if err := f.enter(ctx, "Forget"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(FakeWrite{Method: "Forget", Key: key})
	if _, ok := f.memories[key]; !ok {
		return ErrMemoryGone
	}
	delete(f.memories, key)
	f.bumpVC()
	return nil
}

// bumpVC moves the commit hash the way a bd write does.
func (f *Fake) bumpVC() {
	f.commits++
	f.vc.Commit = fmt.Sprintf("m%d", f.commits)
}

// ConfigSet implements [Client].
func (f *Fake) ConfigSet(ctx context.Context, key, value string) error {
	if err := f.enter(ctx, "ConfigSet"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(FakeWrite{Method: "ConfigSet", Key: key, Content: value})
	f.config[key] = ConfigValue{Key: key, Value: value, Location: "config.yaml"}
	return nil
}

func errMessage(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Message
	}
	return err.Error()
}
