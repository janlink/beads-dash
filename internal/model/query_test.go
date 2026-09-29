package model_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/janlink/beads-dash/internal/model"
)

func TestSelectionOf(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		group    model.FacetGroup
		want     []string
		advanced []string
	}{
		{"type list", "type:bug,task crash", model.GroupType, []string{"bug", "task"}, nil},
		{"repeated tokens merge", "label:ui label:web", model.GroupLabel, []string{"ui", "web"}, nil},
		{"bare priority", "p1 fix p0", model.GroupPriority, []string{"1", "0"}, nil},
		{"priority prefix", "priority:p2,3", model.GroupPriority, []string{"2", "3"}, nil},
		{"at assignee", "@alice crash", model.GroupAssignee, []string{"alice"}, nil},
		{"assignee none", "assignee:none", model.GroupAssignee, []string{"none"}, nil},
		{"quoted assignee", `assignee:"Jan Link",bob`, model.GroupAssignee, []string{"Jan Link", "bob"}, nil},
		{"status canonical", "status:in-progress,Frozen", model.GroupStatus, []string{"in_progress", "frozen"}, nil},
		{"duplicates collapse", "type:bug type:BUG", model.GroupType, []string{"bug"}, nil},
		{"negation is advanced", "-label:ui label:web", model.GroupLabel, []string{"web"}, []string{"-label:ui"}},
		{"parent is advanced", "parent:ws-1 type:bug", model.GroupType, []string{"bug"}, []string{"parent:ws-1"}},
		{"is is advanced", "is:ready", model.GroupType, nil, []string{"is:ready"}},
		{"phrase is advanced", `"fix crash" p1`, model.GroupPriority, []string{"1"}, []string{`"fix crash"`}},
		{"negated word is advanced", "-wip", model.GroupType, nil, []string{"-wip"}},
		{"raw status is advanced", "status:in_review", model.GroupStatus, nil, []string{"status:in_review"}},
		{"bad priority is advanced", "priority:9", model.GroupPriority, nil, []string{"priority:9"}},
		{"unknown key is free text", "color:red", model.GroupType, nil, nil},
		{"empty value is free text", "type:", model.GroupType, nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sel := model.SelectionOf(tc.query)
			if got := sel.Values(tc.group); !slices.Equal(got, tc.want) {
				t.Errorf("Values = %q, want %q", got, tc.want)
			}
			if !slices.Equal(sel.Advanced, tc.advanced) {
				t.Errorf("Advanced = %q, want %q", sel.Advanced, tc.advanced)
			}
		})
	}
}

func TestWithFacet(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		group  model.FacetGroup
		values []string
		want   string
	}{
		{"append when absent", "crash save", model.GroupType, []string{"bug"}, "crash save type:bug"},
		{"replace in place", "crash type:bug save", model.GroupType, []string{"task", "epic"}, "crash type:task,epic save"},
		{"collapse repeats at the first", "type:bug crash type:task", model.GroupType, []string{"epic"}, "type:epic crash"},
		{"remove", "a type:bug b", model.GroupType, nil, "a b"},
		{"other groups untouched", "label:ui p1 type:bug", model.GroupType, []string{"task"}, "label:ui p1 type:task"},
		{"advanced untouched", `-type:bug "fix it" type:task parent:x`, model.GroupType, []string{"epic"}, `-type:bug "fix it" type:epic parent:x`},
		{"quotes values with spaces", "", model.GroupAssignee, []string{"Jan Link", "bob"}, `assignee:"Jan Link",bob`},
		{"drops values without an escape", "", model.GroupLabel, []string{"a,b", `c"d`, "ok"}, "label:ok"},
		{"at token is rewritten", "@alice crash", model.GroupAssignee, []string{"alice", "none"}, "assignee:alice,none crash"},
		{"unknown facet key stays free text", "color:red type:bug", model.GroupType, []string{"task"}, "color:red type:task"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := model.WithFacet(tc.query, tc.group, tc.values); got != tc.want {
				t.Errorf("WithFacet = %q, want %q", got, tc.want)
			}
		})
	}
}

// The filter bar and the search box edit one query text; rewriting a group
// with its own selection must not change what the scope shows.
func TestFilterRoundTripKeepsScope(t *testing.T) {
	queries := []string{
		"", "checkout", "type:bug crash", "crash type:bug,feature label:ui", "p0 p1 web @alice",
		"status:closed old", "status:in-progress,open", "assignee:none label:web", "-label:ui checkout",
		`"fix crash" type:bug`, "parent:p-1 type:task", "is:ready p1", "Fix type:bug",
	}
	groups := []model.FacetGroup{model.GroupStatus, model.GroupType, model.GroupPriority, model.GroupAssignee, model.GroupLabel}
	snap := scopeFixture()
	for _, q := range queries {
		before := model.ParseScope(q, false).Apply(snap, model.BuiltinStatuses()).IDs()
		free := freeWords(q)
		for _, g := range groups {
			rewritten := model.WithFacet(q, g, model.SelectionOf(q).Values(g))
			after := model.ParseScope(rewritten, false).Apply(snap, model.BuiltinStatuses()).IDs()
			if !slices.Equal(before, after) {
				t.Errorf("group %d of %q rewritten to %q: shows %v, was %v", g, q, rewritten, after, before)
			}
			if got := freeWords(rewritten); !slices.Equal(got, free) {
				t.Errorf("group %d of %q rewritten to %q: free words %v, want %v", g, q, rewritten, got, free)
			}
		}
	}
}

func freeWords(q string) []string {
	var out []string
	for _, w := range splitFields(q) {
		if !isFacetLike(w) {
			out = append(out, w)
		}
	}
	return out
}

func TestToggleFacetValueAndStatus(t *testing.T) {
	q := model.ToggleFacetValue("crash", model.GroupLabel, "ui")
	q = model.ToggleFacetValue(q, model.GroupLabel, "web")
	if q != "crash label:ui,web" {
		t.Fatalf("q = %q", q)
	}
	q = model.ToggleFacetValue(q, model.GroupLabel, "UI")
	if q != "crash label:web" {
		t.Errorf("q = %q", q)
	}
	q = model.ToggleFacetValue(q, model.GroupLabel, "web")
	if q != "crash" {
		t.Errorf("q = %q", q)
	}
}

func TestToggleStatus(t *testing.T) {
	q := model.ToggleStatus("crash", false, model.Closed)
	if q != "crash status:open,in_progress,blocked,frozen,closed,other" {
		t.Errorf("showing closed: %q", q)
	}
	if got := model.ToggleStatus(q, false, model.Closed); got != "crash" {
		t.Errorf("hiding closed again must drop the facet: %q", got)
	}
	q = model.ToggleStatus("", false, model.Frozen)
	if q != "status:open,in_progress,blocked,other" {
		t.Errorf("hiding frozen: %q", q)
	}
	if got := model.ToggleStatus("", true, model.Closed); got != "status:open,in_progress,blocked,frozen,other" {
		t.Errorf("hiding closed with show_closed: %q", got)
	}
	if got := model.ToggleStatus("status:open", false, model.Open); got != "status:open" {
		t.Errorf("the last status stays: %q", got)
	}
	shown := model.SelectionOf(q).StatusShown(false)
	if shown[model.Frozen] || !shown[model.Open] || shown[model.Closed] {
		t.Errorf("shown = %v", shown)
	}
}

func TestMatchTerms(t *testing.T) {
	got := model.ParseScope(`crash "Save now" -old type:bug`, false).MatchTerms()
	want := []model.MatchTerm{{Text: "crash"}, {Text: "Save now", Exact: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("terms = %v, want %v", got, want)
	}
}

func splitFields(q string) []string {
	var out []string
	var b []rune
	in := false
	flush := func() {
		if len(b) > 0 {
			out = append(out, string(b))
		}
		b = nil
	}
	for _, r := range q {
		switch {
		case r == '"':
			in = !in
			b = append(b, r)
		case r == ' ' && !in:
			flush()
		default:
			b = append(b, r)
		}
	}
	flush()
	return out
}

func isFacetLike(w string) bool {
	for _, p := range []string{"type:", "label:", "priority:", "status:", "assignee:", "@", "is:", "parent:", "-", `"`} {
		if len(w) > len(p)-1 && w[:len(p)] == p {
			return true
		}
	}
	return len(w) == 2 && w[0] == 'p' && w[1] >= '0' && w[1] <= '4'
}
