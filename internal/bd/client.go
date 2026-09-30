package bd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

// ErrNotImplemented is returned by Client methods whose exec implementation
// is not built yet.
var ErrNotImplemented = errors.New("bd: not implemented")

// Client is the whole bd contract. Views and the refresh engine know only
// this interface, never argv. Every method takes the caller's context;
// cancelling it kills the bd process.
type Client interface {
	// Version runs bd version and judges the result. The info is returned
	// with the error for an unsupported version.
	Version(ctx context.Context) (VersionInfo, error)
	// Where resolves the workspace bd sees.
	Where(ctx context.Context) (Workspace, error)
	// List returns every issue, closed and pinned included, with its outgoing
	// edges.
	List(ctx context.Context) ([]model.Issue, error)
	// Ready returns bd's ready and blocked verdict.
	Ready(ctx context.Context) (model.Readiness, error)
	// Statuses returns the status table with categories.
	Statuses(ctx context.Context) (model.Statuses, error)
	Types(ctx context.Context) ([]TypeInfo, error)
	VCStatus(ctx context.Context) (VCStatus, error)
	// Comments returns an issue's comments ordered by created_at.
	Comments(ctx context.Context, id string) ([]Comment, error)
	// History returns the issue's versions as bd lists them, newest first as
	// bd orders them.
	History(ctx context.Context, id string) ([]HistoryEntry, error)
	// Memories returns the workspace memories sorted by key.
	Memories(ctx context.Context) ([]Memory, error)
	ConfigGet(ctx context.Context, key string) (ConfigValue, error)
	// EventsFollow streams the events journal from sequence since on.
	EventsFollow(ctx context.Context, since int64) (EventStream, error)

	// Create returns the new issue's ID.
	Create(ctx context.Context, spec CreateSpec) (string, error)
	// Update applies one change to every ID in one bd call. A change that
	// reached only some of them is a [ClassPartialWrite] error whose Applied
	// and Failed say which.
	Update(ctx context.Context, ids []string, spec UpdateSpec) error
	// Close returns the IDs bd reports closed so a caller can detect a
	// partial write. A refusal by one of bd's close guards is a
	// [ClassRejected] or [ClassPartialWrite] error, never a success.
	Close(ctx context.Context, ids []string, reason string) ([]string, error)
	// Reopen returns the IDs bd reports reopened.
	Reopen(ctx context.Context, ids []string, reason string) ([]string, error)
	// DepAdd makes from depend on to.
	DepAdd(ctx context.Context, from, to, depType string) error
	DepRemove(ctx context.Context, from, to string) error
	Comment(ctx context.Context, id, text string) error
	Remember(ctx context.Context, key, content string) error
	Forget(ctx context.Context, key string) error
	// ConfigSet exists only for the events-journal opt-in.
	ConfigSet(ctx context.Context, key, value string) error
}

// Workspace is the answer of bd where.
type Workspace struct {
	Path         string
	Prefix       string
	DatabasePath string
}

// TypeInfo is one issue type from bd types.
type TypeInfo struct {
	Name        string
	Description string
	Custom      bool
}

// VCStatus is the version-control position of the store: the cheap change
// signal.
type VCStatus struct {
	Branch string
	Commit string
}

// Comment is one issue comment.
type Comment struct {
	ID        string
	IssueID   string
	Author    string
	Text      string
	CreatedAt time.Time
}

// HistoryEntry is one committed version of an issue.
type HistoryEntry struct {
	CommitHash string
	Committer  string
	CommitDate time.Time
	Issue      model.Issue
}

// Memory is a keyed note stored in the workspace.
type Memory struct {
	Key     string
	Content string
}

// ConfigValue is the answer of bd config get. Value is empty for an unset
// key.
type ConfigValue struct {
	Key      string
	Value    string
	Location string
}

// Event is one events-journal record.
type Event struct {
	Seq     int64
	Time    time.Time
	Op      string
	IssueID string
	// Actor is empty for derived maintenance.
	Actor string
	// Issue is the issue after the mutation; nil for a delete.
	Issue *model.Issue
	// Blocked is the record's is_blocked flag for Issue.
	Blocked bool
	Raw     json.RawMessage
}

// Record converts the event to the model's journal record.
func (e Event) Record() model.JournalRecord {
	return model.JournalRecord{
		Seq: e.Seq, Time: e.Time, Op: e.Op, IssueID: e.IssueID, Actor: e.Actor,
		Issue: e.Issue, Blocked: e.Blocked,
	}
}

// EventStream yields journal events until the stream ends or fails. Next
// blocks until the next record; it returns [io.EOF] when bd exited cleanly,
// a [*JournalTruncatedError] when the requested start fell below the
// retained window, and a bd [*Error] for any other failure. Close stops the
// process and unblocks Next; it is safe to call more than once and from
// another goroutine.
type EventStream interface {
	Next() (Event, error)
	Close() error
}

// JournalTruncatedError says that bd pruned records the follower still
// needed. Resuming from Floor-1 continues with a known gap.
type JournalTruncatedError struct {
	Since, Floor, Head int64
}

func (e *JournalTruncatedError) Error() string {
	return fmt.Sprintf("bd events tail: journal truncated: since %d is below the retained floor %d (head %d)", e.Since, e.Floor, e.Head)
}

// CreateSpec describes a new issue. Zero fields are left to bd's defaults.
type CreateSpec struct {
	Title       string
	Type        string
	Priority    *int
	Description string
	Parent      string
	Assignee    string
	Labels      []string
	// NoInheritLabels stops a child from taking its parent's labels.
	NoInheritLabels bool

	Design      string
	Acceptance  string
	Notes       string
	ExternalRef string
	// Due and Defer are dates in any format bd reads.
	Due   string
	Defer string
	// Estimate is in minutes; nil leaves it unset.
	Estimate *int
	// Deps are issues the new one depends on: an ID, or "type:id".
	Deps []string
}

// UpdateSpec changes fields of an issue; nil pointers leave a field alone
// and a pointer to "" clears a text field.
type UpdateSpec struct {
	Title       *string
	Description *string
	Design      *string
	Acceptance  *string
	Notes       *string
	Status      *string
	Priority    *int
	// Assignee "" unassigns.
	Assignee    *string
	Type        *string
	ExternalRef *string
	// Due and Defer take a date; "" clears.
	Due   *string
	Defer *string
	// Estimate is in minutes; 0 clears.
	Estimate *int
	// Parent "" removes the parent.
	Parent       *string
	AddLabels    []string
	RemoveLabels []string
	// Claim sets the assignee to the actor and the status to in progress in
	// one step, and fails when someone else holds the issue.
	Claim bool
}

// Empty reports whether the spec changes nothing.
func (u UpdateSpec) Empty() bool {
	return u.Title == nil && u.Description == nil && u.Design == nil && u.Acceptance == nil &&
		u.Notes == nil && u.Status == nil && u.Priority == nil && u.Assignee == nil && u.Type == nil &&
		u.ExternalRef == nil && u.Due == nil && u.Defer == nil && u.Estimate == nil && u.Parent == nil &&
		len(u.AddLabels) == 0 && len(u.RemoveLabels) == 0 && !u.Claim
}

// journalEnabled reports whether the workspace's events journal is on. It
// is false without asking bd when the version has no journal.
func journalEnabled(ctx context.Context, c Client, caps capabilities) (bool, error) {
	if !caps.eventsJournal {
		return false, nil
	}
	v, err := c.ConfigGet(ctx, "events-journal")
	if err != nil {
		return false, err
	}
	switch v.Value {
	case "true", "1", "on", "yes":
		return true, nil
	}
	return false, nil
}
