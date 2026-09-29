package model

import (
	"slices"
	"strings"
)

// FacetGroup is a facet category the filter bar edits as a set of values.
type FacetGroup uint8

// The groups of the filter bar, in the order it shows them.
const (
	GroupStatus FacetGroup = iota
	GroupType
	GroupPriority
	GroupAssignee
	GroupLabel

	groupCount
)

var groupKeys = [groupCount]string{
	GroupStatus: "status", GroupType: "type", GroupPriority: "priority", GroupAssignee: "assignee", GroupLabel: "label",
}

// StatusOrder lists the presentation statuses in the order the filter bar
// shows them.
var StatusOrder = [6]PresentationStatus{Open, InProgress, Blocked, Frozen, Closed, Other}

// StatusToken is how a status facet spells a presentation status.
func StatusToken(p PresentationStatus) string {
	if p == InProgress {
		return "in_progress"
	}
	return statusName(p)
}

func statusFromToken(v string) (PresentationStatus, bool) {
	v = strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(v))
	for _, p := range StatusOrder {
		if statusName(p) == v {
			return p, true
		}
	}
	return Other, false
}

type tokenKind uint8

const (
	kindFree tokenKind = iota
	kindFacet
	kindAdvanced
)

type classified struct {
	kind   tokenKind
	group  FacetGroup
	values []string
}

// classify sorts a query token into free text, a facet the filter bar can
// represent (a positive token of one group), or an advanced token it must
// leave alone: negations, phrases, parent:, is: and facets with a value it
// cannot edit.
func classify(tok token) classified {
	raw := tok.text
	if strings.HasPrefix(raw, "-") && len(raw) > 1 {
		return classified{kind: kindAdvanced}
	}
	if strings.HasPrefix(raw, `"`) {
		return classified{kind: kindAdvanced}
	}
	if name, ok := strings.CutPrefix(raw, "@"); ok {
		if name = unquote(name); name != "" {
			return classified{kindFacet, GroupAssignee, []string{name}}
		}
		return classified{}
	}
	if isBarePriority(raw) {
		return classified{kindFacet, GroupPriority, []string{raw[1:]}}
	}
	key, val, ok := strings.Cut(raw, ":")
	if !ok {
		return classified{}
	}
	fk, known := facetKeys[strings.ToLower(key)]
	if !known {
		return classified{}
	}
	if fk == facetParent || fk == facetIs {
		return classified{kind: kindAdvanced}
	}
	var vals []string
	for _, v := range strings.Split(unquote(val), ",") {
		if v = strings.TrimSpace(v); v != "" {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return classified{}
	}
	var g FacetGroup
	switch fk {
	case facetType:
		g = GroupType
	case facetLabel:
		g = GroupLabel
	case facetAssignee:
		g = GroupAssignee
	case facetPriority:
		g, vals = GroupPriority, trimPriorities(vals)
	case facetStatus:
		g, vals = GroupStatus, canonStatuses(vals)
	case facetIs, facetParent, facetCount:
	}
	if vals == nil {
		return classified{kind: kindAdvanced}
	}
	return classified{kindFacet, g, vals}
}

func trimPriorities(vals []string) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		v = strings.TrimPrefix(strings.ToLower(v), "p")
		if len(v) != 1 || v[0] < '0' || v[0] > '4' {
			return nil
		}
		out[i] = v
	}
	return out
}

func canonStatuses(vals []string) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		p, ok := statusFromToken(v)
		if !ok {
			return nil
		}
		out[i] = StatusToken(p)
	}
	return out
}

// Selection is what the filter bar shows of a query: the values of each
// facet group, and the tokens it cannot represent.
type Selection struct {
	values [groupCount][]string
	// Advanced holds the tokens the filter bar shows read-only and leaves
	// untouched, in query order.
	Advanced []string
}

// SelectionOf reads the facet groups out of a query.
func SelectionOf(query string) Selection {
	var s Selection
	for _, tok := range tokenize(query) {
		c := classify(tok)
		switch c.kind {
		case kindFacet:
			for _, v := range c.values {
				if !s.Has(c.group, v) {
					s.values[c.group] = append(s.values[c.group], v)
				}
			}
		case kindAdvanced:
			s.Advanced = append(s.Advanced, tok.text)
		case kindFree:
		}
	}
	return s
}

// Values lists the selected values of a group in query order.
func (s Selection) Values(g FacetGroup) []string { return slices.Clone(s.values[g]) }

// Has reports whether a group selects v, ignoring case.
func (s Selection) Has(g FacetGroup, v string) bool {
	return slices.ContainsFunc(s.values[g], func(x string) bool { return strings.EqualFold(x, v) })
}

// StatusShown reports which presentation statuses the query lets through,
// indexed by [PresentationStatus]. Without a status facet the default
// visibility applies.
func (s Selection) StatusShown(showClosed bool) [6]bool {
	var out [6]bool
	if len(s.values[GroupStatus]) == 0 {
		for _, p := range StatusOrder {
			out[p] = showClosed || p != Closed
		}
		return out
	}
	for _, v := range s.values[GroupStatus] {
		if p, ok := statusFromToken(v); ok {
			out[p] = true
		}
	}
	return out
}

// FacetValueOK reports whether a facet token can carry v: the grammar has no
// escape for commas and quotes.
func FacetValueOK(v string) bool {
	return strings.TrimSpace(v) != "" && !strings.ContainsAny(v, `,"`)
}

// WithFacet rewrites the facet tokens of group g in query so that they
// select exactly values, leaving every other token and the order as they are.
// The new token takes the place of the first old one, or goes last. Values
// the grammar cannot carry are dropped.
func WithFacet(query string, g FacetGroup, values []string) string {
	var parts []string
	for _, v := range values {
		if !FacetValueOK(v) {
			continue
		}
		if strings.ContainsAny(v, " \t") {
			v = `"` + v + `"`
		}
		parts = append(parts, v)
	}
	repl := ""
	if len(parts) > 0 {
		repl = groupKeys[g] + ":" + strings.Join(parts, ",")
	}
	var out []string
	placed := false
	for _, tok := range tokenize(query) {
		if c := classify(tok); c.kind == kindFacet && c.group == g {
			if !placed && repl != "" {
				out = append(out, repl)
			}
			placed = true
			continue
		}
		out = append(out, tok.text)
	}
	if !placed && repl != "" {
		out = append(out, repl)
	}
	return strings.Join(out, " ")
}

// ToggleFacetValue flips v in group g of query, keeping the rest.
func ToggleFacetValue(query string, g FacetGroup, v string) string {
	sel := SelectionOf(query)
	var next []string
	found := false
	for _, x := range sel.values[g] {
		if strings.EqualFold(x, v) {
			found = true
			continue
		}
		next = append(next, x)
	}
	if !found {
		next = append(next, v)
	}
	return WithFacet(query, g, next)
}

// ToggleStatus flips the visibility of one presentation status in query. The
// query ends up with no status facet when the result is the default
// visibility, and refuses to hide every status.
func ToggleStatus(query string, showClosed bool, p PresentationStatus) string {
	shown := SelectionOf(query).StatusShown(showClosed)
	shown[p] = !shown[p]
	var names []string
	count := 0
	for _, q := range StatusOrder {
		if shown[q] {
			names = append(names, StatusToken(q))
			count++
		}
	}
	if count == 0 {
		return query
	}
	if def := SelectionOf("").StatusShown(showClosed); def == shown {
		return WithFacet(query, GroupStatus, nil)
	}
	return WithFacet(query, GroupStatus, names)
}

// MatchTerm is a free-text term of a scope, for highlighting matches.
type MatchTerm struct {
	Text string
	// Exact reports a case-sensitive match, as smartcase asks for terms with
	// an uppercase letter.
	Exact bool
}

// MatchTerms lists the positive free-text terms, which rows highlight.
func (s Scope) MatchTerms() []MatchTerm {
	var out []MatchTerm
	for _, t := range s.free {
		if !t.neg && strings.TrimSpace(t.text) != "" {
			out = append(out, MatchTerm{t.text, t.exact})
		}
	}
	return out
}
