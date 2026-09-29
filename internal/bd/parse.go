package bd

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

// lenientTime reads an RFC 3339 timestamp and turns anything else into the
// zero time, so one odd value never fails a whole list.
type lenientTime time.Time

func (t *lenientTime) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	if v, err := time.Parse(time.RFC3339Nano, s); err == nil {
		*t = lenientTime(v)
	}
	return nil
}

func (t lenientTime) time() time.Time { return time.Time(t) }

type wireEdge struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
}

// wireIssue lists the fields bdash reads. Unknown keys are ignored, so
// additive bd changes never break decoding.
type wireIssue struct {
	ID                 string      `json:"id"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	Design             string      `json:"design"`
	AcceptanceCriteria string      `json:"acceptance_criteria"`
	Notes              string      `json:"notes"`
	Status             string      `json:"status"`
	IssueType          string      `json:"issue_type"`
	Priority           int         `json:"priority"`
	Assignee           string      `json:"assignee"`
	Owner              string      `json:"owner"`
	CreatedBy          string      `json:"created_by"`
	Parent             string      `json:"parent"`
	Labels             []string    `json:"labels"`
	CreatedAt          lenientTime `json:"created_at"`
	UpdatedAt          lenientTime `json:"updated_at"`
	StartedAt          lenientTime `json:"started_at"`
	ClosedAt           lenientTime `json:"closed_at"`
	DueAt              lenientTime `json:"due_at"`
	CloseReason        string      `json:"close_reason"`
	ExternalRef        string      `json:"external_ref"`
	EstimatedMinutes   int         `json:"estimated_minutes"`
	CommentCount       int         `json:"comment_count"`
	Dependencies       []wireEdge  `json:"dependencies"`
}

func (w *wireIssue) issue(raw json.RawMessage) model.Issue {
	is := model.Issue{
		ID: w.ID, Title: w.Title, Description: w.Description, Design: w.Design,
		AcceptanceCriteria: w.AcceptanceCriteria, Notes: w.Notes,
		Status: w.Status, IssueType: w.IssueType, Priority: w.Priority,
		Assignee: w.Assignee, Owner: w.Owner, CreatedBy: w.CreatedBy, Parent: w.Parent,
		Labels:    w.Labels,
		CreatedAt: w.CreatedAt.time(), UpdatedAt: w.UpdatedAt.time(),
		StartedAt: w.StartedAt.time(), ClosedAt: w.ClosedAt.time(), DueAt: w.DueAt.time(),
		CloseReason: w.CloseReason, ExternalRef: w.ExternalRef,
		EstimatedMinutes: w.EstimatedMinutes, CommentCount: w.CommentCount,
		Raw: raw,
	}
	if len(w.Dependencies) > 0 {
		is.Dependencies = make([]model.Edge, len(w.Dependencies))
		for i, e := range w.Dependencies {
			from := e.IssueID
			if from == "" {
				from = w.ID
			}
			is.Dependencies[i] = model.Edge{From: from, To: e.DependsOnID, Type: e.Type}
		}
	}
	return is
}

// parseIssues decodes the data of bd list. Each issue keeps its own JSON.
func parseIssues(data json.RawMessage) ([]model.Issue, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, err
	}
	out := make([]model.Issue, 0, len(raws))
	for _, raw := range raws {
		var w wireIssue
		if err := json.Unmarshal(raw, &w); err != nil {
			return nil, err
		}
		if w.ID == "" {
			continue
		}
		out = append(out, w.issue(raw))
	}
	return out, nil
}

type blockerRef string

func (b *blockerRef) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*b = blockerRef(s)
		return nil
	}
	var o struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &o); err != nil {
		return err
	}
	*b = blockerRef(o.ID)
	return nil
}

// parseReadiness decodes the data of bd ready --explain. blocked_by entries
// are objects there and bare IDs in bd blocked; both are accepted.
func parseReadiness(data json.RawMessage) (model.Readiness, error) {
	var w struct {
		Ready []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"ready"`
		Blocked []struct {
			ID        string       `json:"id"`
			BlockedBy []blockerRef `json:"blocked_by"`
		} `json:"blocked"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return model.Readiness{}, err
	}
	r := model.Readiness{
		Ready:   make([]string, 0, len(w.Ready)),
		Blocked: make(map[string][]string, len(w.Blocked)),
		Reason:  make(map[string]string, len(w.Ready)),
	}
	for _, x := range w.Ready {
		r.Ready = append(r.Ready, x.ID)
		if x.Reason != "" {
			r.Reason[x.ID] = x.Reason
		}
	}
	for _, x := range w.Blocked {
		by := make([]string, 0, len(x.BlockedBy))
		for _, b := range x.BlockedBy {
			if b != "" {
				by = append(by, string(b))
			}
		}
		r.Blocked[x.ID] = by
	}
	return r, nil
}

func parseStatuses(data json.RawMessage) (model.Statuses, error) {
	var w struct {
		Builtin []struct {
			Name, Category, Icon, Description string
		} `json:"built_in_statuses"`
		Custom []struct {
			Name, Category, Icon, Description string
		} `json:"custom_statuses"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return model.Statuses{}, err
	}
	infos := make([]model.StatusInfo, 0, len(w.Builtin)+len(w.Custom))
	for _, s := range w.Builtin {
		infos = append(infos, model.StatusInfo{
			Name: s.Name, Category: model.ParseCategory(s.Category), Icon: s.Icon, Description: s.Description,
		})
	}
	for _, s := range w.Custom {
		infos = append(infos, model.StatusInfo{
			Name: s.Name, Category: model.ParseCategory(s.Category), Icon: s.Icon, Description: s.Description, Custom: true,
		})
	}
	return model.NewStatuses(infos), nil
}

func parseTypes(data json.RawMessage) ([]TypeInfo, error) {
	var w struct {
		Core []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"core_types"`
		Custom []json.RawMessage `json:"custom_types"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	out := make([]TypeInfo, 0, len(w.Core)+len(w.Custom))
	for _, t := range w.Core {
		out = append(out, TypeInfo{Name: t.Name, Description: t.Description})
	}
	for _, raw := range w.Custom {
		var name string
		if json.Unmarshal(raw, &name) == nil {
			out = append(out, TypeInfo{Name: name, Custom: true})
			continue
		}
		var o struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if json.Unmarshal(raw, &o) == nil && o.Name != "" {
			out = append(out, TypeInfo{Name: o.Name, Description: o.Description, Custom: true})
		}
	}
	return out, nil
}

func parseWhere(data json.RawMessage) (Workspace, error) {
	var w struct {
		Path         string `json:"path"`
		Prefix       string `json:"prefix"`
		DatabasePath string `json:"database_path"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return Workspace{}, err
	}
	return Workspace(w), nil
}

func parseVCStatus(data json.RawMessage) (VCStatus, error) {
	var w struct {
		Branch string `json:"branch"`
		Commit string `json:"commit"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return VCStatus{}, err
	}
	return VCStatus(w), nil
}

// parseComments orders comments by created_at and keeps bd's order for equal
// timestamps. Comment IDs carry no order.
func parseComments(data json.RawMessage) ([]Comment, error) {
	var w []struct {
		ID        string      `json:"id"`
		IssueID   string      `json:"issue_id"`
		Author    string      `json:"author"`
		Text      string      `json:"text"`
		CreatedAt lenientTime `json:"created_at"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	out := make([]Comment, len(w))
	for i, c := range w {
		out[i] = Comment{ID: c.ID, IssueID: c.IssueID, Author: c.Author, Text: c.Text, CreatedAt: c.CreatedAt.time()}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

// parseHistory keeps bd's order. The keys are PascalCase in bd's output.
func parseHistory(data json.RawMessage) ([]HistoryEntry, error) {
	var w []struct {
		CommitHash string          `json:"CommitHash"`
		Committer  string          `json:"Committer"`
		CommitDate lenientTime     `json:"CommitDate"`
		Issue      json.RawMessage `json:"Issue"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, err
	}
	out := make([]HistoryEntry, len(w))
	for i, h := range w {
		out[i] = HistoryEntry{CommitHash: h.CommitHash, Committer: h.Committer, CommitDate: h.CommitDate.time()}
		if len(h.Issue) > 0 {
			var wi wireIssue
			if err := json.Unmarshal(h.Issue, &wi); err != nil {
				return nil, err
			}
			out[i].Issue = wi.issue(h.Issue)
		}
	}
	return out, nil
}

// parseMemories reads the key to content object and sorts by key.
func parseMemories(data json.RawMessage) ([]Memory, error) {
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	out := make([]Memory, 0, len(m))
	for k, v := range m {
		out = append(out, Memory{Key: k, Content: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func parseConfigValue(data json.RawMessage) (ConfigValue, error) {
	var w struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Location string `json:"location"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return ConfigValue{}, err
	}
	return ConfigValue(w), nil
}

func parseVersionInfo(data json.RawMessage) (VersionInfo, error) {
	var w struct {
		Version string `json:"version"`
		Build   string `json:"build"`
		Commit  string `json:"commit"`
		Branch  string `json:"branch"`
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return VersionInfo{}, err
	}
	info, err := CheckVersion(strings.TrimSpace(w.Version))
	info.Build, info.Commit, info.Branch = w.Build, w.Commit, w.Branch
	return info, err
}
