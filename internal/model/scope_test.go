package model_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/janlink/beads-dash/internal/model"
)

func scopeFixture() *model.Snapshot {
	at := func(h int) func(*model.Issue) {
		return func(is *model.Issue) { is.CreatedAt = t0.Add(time.Duration(h) * time.Hour) }
	}
	with := func(f func(*model.Issue)) func(*model.Issue) { return f }
	return model.NewSnapshot([]model.Issue{
		issue("p-1", "open", with(func(is *model.Issue) {
			is.Title, is.IssueType, is.Priority, is.Assignee = "Checkout flow", "epic", 1, "alice"
			is.Labels = []string{"ui", "web"}
			is.Description = "Guest checkout with Stripe"
		})),
		issue("p-1.1", "open", parent("p-1"), with(func(is *model.Issue) {
			is.Title, is.Priority, is.Labels = "Address form", 1, []string{"ui"}
			is.Notes = "check the ZIP code rules"
		})),
		issue("p-1.1.1", "closed", parent("p-1.1"), with(func(is *model.Issue) { is.Title, is.Priority = "Zip lookup", 3 })),
		issue("p-1.2", "in_progress", parent("p-1"), with(func(is *model.Issue) {
			is.Title, is.IssueType, is.Assignee = "Confirmation mail", "feature", "bob"
		})),
		issue("p-2", "open", with(func(is *model.Issue) { is.Title, is.IssueType, is.Priority = "Fix crash on Save", "bug", 0 }), at(1)),
		issue("p-3", "closed", with(func(is *model.Issue) { is.Title, is.IssueType = "Old bug", "bug" })),
		issue("p-4", "deferred", with(func(is *model.Issue) { is.Title = "Search typos"; is.Labels = []string{"web"} })),
		issue("p-5", "open", with(func(is *model.Issue) { is.Title = "Blocked thing" })),
	}, model.Readiness{Ready: []string{"p-1.1", "p-2"}, Blocked: map[string][]string{"p-5": {"p-2"}}}, t0)
}

func shown(t *testing.T, query string, showClosed bool) []string {
	t.Helper()
	m := model.ParseScope(query, showClosed).Apply(scopeFixture(), model.BuiltinStatuses())
	return append([]string{}, m.IDs()...)
}

func TestScopeMatching(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"empty hides closed", "", []string{"p-1", "p-1.1", "p-1.2", "p-2", "p-4", "p-5"}},
		{"word in title", "checkout", []string{"p-1"}},
		{"word in description", "stripe", []string{"p-1"}},
		{"word in notes", "zip", []string{"p-1.1"}},
		{"word in id", "p-1.2", []string{"p-1.2"}},
		{"word in assignee", "alice", []string{"p-1"}},
		{"word in label", "web", []string{"p-1", "p-4"}},
		{"all words required", "guest stripe", []string{"p-1"}},
		{"words in different fields", "checkout alice", []string{"p-1"}},
		{"missing word", "checkout nothing", nil},
		{"smartcase lower is insensitive", "crash on save", []string{"p-2"}},
		{"smartcase upper is exact", "Save", []string{"p-2"}},
		{"smartcase upper misses", "SAVE", nil},
		{"smartcase mixed misses", "zIP", nil},
		{"phrase", `"crash on"`, []string{"p-2"}},
		{"phrase with space must be adjacent", `"on crash"`, nil},
		{"negated word", "-checkout p-1", []string{"p-1.1", "p-1.2"}},
		{"negated phrase", `-"crash on" bug`, nil},
		{"type", "type:bug", []string{"p-2"}},
		{"type or", "type:bug type:feature", []string{"p-1.2", "p-2"}},
		{"type list", "type:bug,feature", []string{"p-1.2", "p-2"}},
		{"type case", "type:BUG", []string{"p-2"}},
		{"negated type", "-type:bug -type:task", []string{"p-1", "p-1.2"}},
		{"label", "label:ui", []string{"p-1", "p-1.1"}},
		{"label or", "label:ui label:web", []string{"p-1", "p-1.1", "p-4"}},
		{"label and type", "label:web type:epic", []string{"p-1"}},
		{"negated label", "label:web -label:ui", []string{"p-4"}},
		{"priority word", "priority:0", []string{"p-2"}},
		{"priority p form", "priority:p1", []string{"p-1", "p-1.1"}},
		{"bare priority", "p0", []string{"p-2"}},
		{"bare priority upper", "P1", []string{"p-1", "p-1.1"}},
		{"priority or", "p0 p1", []string{"p-1", "p-1.1", "p-2"}},
		{"negated priority", "-p2 -p1", []string{"p-2"}},
		{"status presentation", "status:frozen", []string{"p-4"}},
		{"status in progress underscore", "status:in_progress", []string{"p-1.2"}},
		{"status in progress squashed", "status:inprogress", []string{"p-1.2"}},
		{"status blocked is derived", "status:blocked", []string{"p-5"}},
		{"status raw name", "status:deferred", []string{"p-4"}},
		{"status closed overrides hiding", "status:closed", []string{"p-1.1.1", "p-3"}},
		{"status open and closed", "status:open status:closed type:bug", []string{"p-2", "p-3"}},
		{"negated status matches raw names too", "-status:open", []string{"p-1.2", "p-4"}},
		{"assignee", "assignee:alice", []string{"p-1"}},
		{"at assignee", "@bob", []string{"p-1.2"}},
		{"assignee none", "assignee:none type:bug", []string{"p-2"}},
		{"assignee or", "@alice @bob", []string{"p-1", "p-1.2"}},
		{"negated assignee", "-@alice -@bob type:epic", nil},
		{"is ready", "is:ready", []string{"p-1.1", "p-2"}},
		{"not ready", "-is:ready", []string{"p-1", "p-1.2", "p-4", "p-5"}},
		{"parent descendants", "parent:p-1", []string{"p-1.1", "p-1.2"}},
		{"parent descendants with closed", "parent:p-1 status:closed", []string{"p-1.1.1"}},
		{"parent is transitive", "parent:p-1 status:open status:closed", []string{"p-1.1", "p-1.1.1"}},
		{"parent excludes itself", "parent:p-1.2", nil},
		{"negated parent", "-parent:p-1 type:task", []string{"p-4", "p-5"}},
		{"unknown parent", "parent:nope", nil},
		{"parent id ignores case", "parent:P-1 p1", []string{"p-1.1"}},
		{"facet without value is ignored", "type: label:", []string{"p-1", "p-1.1", "p-1.2", "p-2", "p-4", "p-5"}},
		{"lone dash is free text", "-", []string{"p-1", "p-1.1", "p-1.2", "p-2", "p-4", "p-5"}},
		{"unknown key is free text", "foo:bar", nil},
		{"unclosed quote takes the rest", `"crash on`, []string{"p-2"}},
		{"quoted facet value", `label:"ui"`, []string{"p-1", "p-1.1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shown(t, tc.query, false); !reflect.DeepEqual(got, tc.want) && (len(got) != 0 || len(tc.want) != 0) {
				t.Errorf("%q shows %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestScopeShowClosedSetting(t *testing.T) {
	got := shown(t, "type:bug", true)
	if want := []string{"p-2", "p-3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("show_closed: %v, want %v", got, want)
	}
	if got := shown(t, "type:bug", false); !reflect.DeepEqual(got, []string{"p-2"}) {
		t.Errorf("hidden: %v", got)
	}
}

func TestScopeUnknownKeysAreFlagged(t *testing.T) {
	sc := model.ParseScope(`foo:bar type:bug priority:9 is:done -zzz:1`, false)
	want := []string{"foo:bar", "priority:9", "is:done", "-zzz:1"}
	if got := sc.Unknown(); !reflect.DeepEqual(got, want) {
		t.Errorf("Unknown = %v, want %v", got, want)
	}
	if got := shown(t, "foo:bar", false); len(got) != 0 {
		t.Errorf("an unknown key must match as text, got %v", got)
	}
	if got := model.ParseScope("type:bug", false).Unknown(); len(got) != 0 {
		t.Errorf("Unknown = %v", got)
	}
}

func TestScopeHiddenClosedAndFacetsOnly(t *testing.T) {
	m := model.ParseScope("type:bug", false).Apply(scopeFixture(), model.BuiltinStatuses())
	if m.HiddenClosed != 1 {
		t.Errorf("HiddenClosed = %d, want 1", m.HiddenClosed)
	}
	if m.Has("p-3") || !m.Facets("p-3") {
		t.Error("p-3 is hidden only by status visibility")
	}
	if m.Facets("p-1") || m.Has("p-1") {
		t.Error("p-1 fails the facet")
	}
	if m.Len() != 1 {
		t.Errorf("Len = %d", m.Len())
	}
}

func TestScopeAccessors(t *testing.T) {
	sc := model.ParseScope("  type:epic  ", true)
	if sc.Query() != "type:epic" || !sc.Active() || !sc.ShowClosed() {
		t.Errorf("accessors: %q %v %v", sc.Query(), sc.Active(), sc.ShowClosed())
	}
	if model.ParseScope("", false).Active() {
		t.Error("the default scope is not active")
	}
	if model.ParseScope("a", false).Key() == model.ParseScope("a", true).Key() || model.ParseScope("a", false).Key() == model.ParseScope("b", false).Key() {
		t.Error("Key must tell scopes apart")
	}
	if !sc.NamesType("EPIC") || sc.NamesType("bug") || model.ParseScope("-type:epic", false).NamesType("epic") {
		t.Error("NamesType follows positive type facets only")
	}
}

func TestCountFacets(t *testing.T) {
	f := model.CountFacets(scopeFixture(), model.BuiltinStatuses())
	if got := f.Types[0]; got.Name != "bug" && got.Name != "task" || got.N == 0 {
		t.Errorf("types = %v", f.Types)
	}
	if want := []model.FacetCount{{"ui", 2}, {"web", 2}}; !reflect.DeepEqual(f.Labels, want) {
		t.Errorf("labels = %v, want %v", f.Labels, want)
	}
	if want := []model.FacetCount{{"alice", 1}, {"bob", 1}}; !reflect.DeepEqual(f.Assignees, want) {
		t.Errorf("assignees = %v", f.Assignees)
	}
	if f.Unassigned != 6 || f.Priorities != [5]int{1, 2, 4, 1, 0} {
		t.Errorf("unassigned %d priorities %v", f.Unassigned, f.Priorities)
	}
	want := [6]int{0, 3, 1, 1, 1, 2}
	if f.Statuses != want {
		t.Errorf("statuses = %v, want %v", f.Statuses, want)
	}
	if !strings.Contains(model.Blocked.String(), "Blocked") {
		t.Error("status names")
	}
}

func TestScopeParentSurvivesCycles(t *testing.T) {
	s := model.NewSnapshot([]model.Issue{
		issue("a", "open", parent("b")), issue("b", "open", parent("a")),
	}, model.Readiness{}, t0)
	m := model.ParseScope("parent:a", false).Apply(s, model.BuiltinStatuses())
	if !m.Has("b") {
		t.Errorf("b is a descendant of a: %v", m.IDs())
	}
}
