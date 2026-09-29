package bd

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
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

// Create implements [Client]. It adds an open issue and leaves readiness
// alone.
func (f *Fake) Create(ctx context.Context, spec CreateSpec) (string, error) {
	if err := f.enter(ctx, "Create"); err != nil {
		return "", err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if spec.Title == "" {
		return "", &Error{Class: ClassRejected, Command: "create", Message: "title required"}
	}
	f.nextID++
	id := fmt.Sprintf("%s-new%d", f.workspace.Prefix, f.nextID)
	now := f.now()
	is := model.Issue{
		ID: id, Title: spec.Title, Status: "open", IssueType: orDefault(spec.Type, "task"), Priority: 2,
		Description: spec.Description, Assignee: spec.Assignee, Parent: spec.Parent,
		Labels: slices.Clone(spec.Labels), CreatedAt: now, UpdatedAt: now,
	}
	if spec.Priority != nil {
		is.Priority = *spec.Priority
	}
	if spec.Parent != "" {
		is.Dependencies = []model.Edge{{From: id, To: spec.Parent, Type: model.EdgeParentChild}}
	}
	f.issues = append(f.issues, is)
	return id, nil
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// Update implements [Client].
func (f *Fake) Update(ctx context.Context, id string, spec UpdateSpec) error {
	if err := f.enter(ctx, "Update"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i, err := f.find(id)
	if err != nil {
		return err
	}
	is := &f.issues[i]
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
	if spec.Priority != nil {
		is.Priority = *spec.Priority
	}
	for _, l := range spec.AddLabels {
		if !slices.Contains(is.Labels, l) {
			is.Labels = append(is.Labels, l)
		}
	}
	is.Labels = slices.DeleteFunc(is.Labels, func(l string) bool { return slices.Contains(spec.RemoveLabels, l) })
	is.UpdatedAt = f.now()
	is.Raw = nil
	return nil
}

func (f *Fake) setStatus(ids []string, status string) ([]string, error) {
	var done []string
	for _, id := range ids {
		i, err := f.find(id)
		if err != nil {
			continue
		}
		f.issues[i].Status = status
		f.issues[i].UpdatedAt = f.now()
		f.issues[i].Raw = nil
		done = append(done, id)
	}
	if len(done) == 0 {
		return nil, &Error{Class: ClassRejected, Message: "no issue found"}
	}
	return done, nil
}

// Close implements [Client]; unknown IDs are skipped like bd does.
func (f *Fake) Close(ctx context.Context, ids []string, reason string) ([]string, error) {
	if err := f.enter(ctx, "Close"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	done, err := f.setStatus(ids, "closed")
	for _, id := range done {
		i, _ := f.find(id)
		f.issues[i].CloseReason = reason
		f.issues[i].ClosedAt = f.now()
	}
	return done, err
}

// Reopen implements [Client]; unknown IDs are skipped like bd does.
func (f *Fake) Reopen(ctx context.Context, ids []string) ([]string, error) {
	if err := f.enter(ctx, "Reopen"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.setStatus(ids, "open")
}

// DepAdd implements [Client].
func (f *Fake) DepAdd(ctx context.Context, from, to, depType string) error {
	if err := f.enter(ctx, "DepAdd"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	i, err := f.find(from)
	if err != nil {
		return err
	}
	f.issues[i].Dependencies = append(f.issues[i].Dependencies, model.Edge{From: from, To: to, Type: orDefault(depType, "blocks")})
	f.issues[i].UpdatedAt = f.now()
	f.issues[i].Raw = nil
	return nil
}

// DepRemove implements [Client].
func (f *Fake) DepRemove(ctx context.Context, from, to string) error {
	if err := f.enter(ctx, "DepRemove"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.memories[key] = content
	return nil
}

// Forget implements [Client].
func (f *Fake) Forget(ctx context.Context, key string) error {
	if err := f.enter(ctx, "Forget"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.memories, key)
	return nil
}

// ConfigSet implements [Client].
func (f *Fake) ConfigSet(ctx context.Context, key, value string) error {
	if err := f.enter(ctx, "ConfigSet"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.config[key] = ConfigValue{Key: key, Value: value, Location: "config.yaml"}
	return nil
}
