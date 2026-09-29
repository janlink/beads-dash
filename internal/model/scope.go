package model

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type facetKey uint8

const (
	facetType facetKey = iota
	facetLabel
	facetPriority
	facetStatus
	facetAssignee
	facetIs
	facetParent
	facetCount
)

var facetKeys = map[string]facetKey{
	"type": facetType, "label": facetLabel, "priority": facetPriority, "status": facetStatus,
	"assignee": facetAssignee, "is": facetIs, "parent": facetParent,
}

// assigneeNone is the assignee facet value that matches unassigned issues.
const assigneeNone = "none"

type facetTerm struct {
	key facetKey
	// val is lowercased; priorities are the digit.
	val string
	neg bool
}

type textTerm struct {
	text string
	// exact turns case folding off: smartcase applies it to terms with an
	// uppercase letter.
	exact bool
	neg   bool
}

// Scope is the one filter every view shares: a query in the search grammar
// plus the status visibility. It is immutable.
//
// Grammar: whitespace-separated tokens; a leading - negates a token. type:,
// label:, priority: (or bare p0-p4), status: (presentation status or raw
// status), assignee: (or @name, assignee:none), is:ready and parent:<id>
// (all descendants) are facets: terms of one facet are alternatives, facets
// combine with AND. Everything else is free text: quoted phrases and words,
// all required, matched as substrings with smartcase. Unknown keys fall back
// to free text and are reported by [Scope.Unknown]. A facet token without a
// value, as while typing, is ignored.
type Scope struct {
	query      string
	showClosed bool
	facets     []facetTerm
	free       []textTerm
	unknown    []string
}

// ParseScope parses a query. showClosed is the default visibility of closed
// issues; an explicit status:closed shows them whatever it is.
func ParseScope(query string, showClosed bool) Scope {
	s := Scope{query: strings.TrimSpace(query), showClosed: showClosed}
	for _, tok := range tokenize(query) {
		s.add(tok)
	}
	return s
}

// Query is the query text as given, trimmed.
func (s Scope) Query() string { return s.query }

// ShowClosed reports the default visibility of closed issues.
func (s Scope) ShowClosed() bool { return s.showClosed }

// Active reports whether the scope narrows anything beyond the default
// visibility.
func (s Scope) Active() bool { return s.query != "" }

// Key identifies the scope for caches: equal keys filter identically.
func (s Scope) Key() string { return strconv.FormatBool(s.showClosed) + "\x00" + s.query }

// Unknown lists the tokens with an unknown key or value that were taken as
// free text.
func (s Scope) Unknown() []string { return slices.Clone(s.unknown) }

// NamesType reports whether the scope asks for issues of type t by a
// positive type facet.
func (s Scope) NamesType(t string) bool {
	for _, f := range s.facets {
		if f.key == facetType && !f.neg && f.val == strings.ToLower(t) {
			return true
		}
	}
	return false
}

// NamesStatus reports whether the scope has a status facet, positive or
// negated.
func (s Scope) NamesStatus() bool {
	return slices.ContainsFunc(s.facets, func(f facetTerm) bool { return f.key == facetStatus })
}

type token struct {
	text   string
	quoted bool
}

func tokenize(q string) []token {
	var out []token
	var b strings.Builder
	inQuote, quoted, started := false, false, false
	flush := func() {
		if started {
			out = append(out, token{b.String(), quoted})
		}
		b.Reset()
		inQuote, quoted, started = false, false, false
	}
	for _, r := range q {
		switch {
		case r == '"':
			inQuote, quoted, started = !inQuote, true, true
			b.WriteRune(r)
		case unicode.IsSpace(r) && !inQuote:
			flush()
		default:
			started = true
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

func unquote(s string) string {
	s = strings.ReplaceAll(s, `"`, "")
	return s
}

func (s *Scope) add(tok token) {
	raw, neg := tok.text, false
	if strings.HasPrefix(raw, "-") && len(raw) > 1 {
		raw, neg = raw[1:], true
	}
	if strings.HasPrefix(raw, `"`) {
		s.addText(unquote(raw), neg)
		return
	}
	if name, ok := strings.CutPrefix(raw, "@"); ok {
		if name = unquote(name); name != "" {
			s.facets = append(s.facets, facetTerm{facetAssignee, strings.ToLower(name), neg})
		}
		return
	}
	if isBarePriority(raw) {
		s.facets = append(s.facets, facetTerm{facetPriority, raw[1:], neg})
		return
	}
	if key, val, ok := strings.Cut(raw, ":"); ok {
		if fk, known := facetKeys[strings.ToLower(key)]; known {
			s.addFacet(fk, unquote(val), neg, tok.text)
			return
		}
		s.unknown = append(s.unknown, tok.text)
	}
	s.addText(unquote(raw), neg)
}

func isBarePriority(raw string) bool {
	return len(raw) == 2 && (raw[0] == 'p' || raw[0] == 'P') && raw[1] >= '0' && raw[1] <= '4'
}

func (s *Scope) addText(text string, neg bool) {
	if text == "" {
		return
	}
	exact := strings.IndexFunc(text, unicode.IsUpper) >= 0
	s.free = append(s.free, textTerm{text, exact, neg})
}

func (s *Scope) addFacet(k facetKey, val string, neg bool, whole string) {
	for _, v := range strings.Split(val, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		v = strings.ToLower(v)
		switch k {
		case facetPriority:
			v = strings.TrimPrefix(v, "p")
			if len(v) != 1 || v[0] < '0' || v[0] > '4' {
				s.unknown = append(s.unknown, whole)
				s.addText(whole, neg)
				return
			}
		case facetIs:
			if v != "ready" {
				s.unknown = append(s.unknown, whole)
				s.addText(whole, neg)
				return
			}
		case facetType, facetLabel, facetStatus, facetAssignee, facetParent, facetCount:
		}
		s.facets = append(s.facets, facetTerm{k, v, neg})
	}
}

const (
	flagFacets uint8 = 1 << iota
	flagVisible
)

// Matches is the outcome of applying a scope to a snapshot.
type Matches struct {
	flags map[string]uint8
	ids   []string
	// HiddenClosed counts the closed issues that match everything but are
	// hidden by the status visibility.
	HiddenClosed int
}

// Has reports whether the scope shows the issue.
func (m *Matches) Has(id string) bool { return m.flags[id]&flagVisible != 0 }

// Facets reports whether the issue passes the facets and the free text,
// whatever the status visibility says.
func (m *Matches) Facets(id string) bool { return m.flags[id]&flagFacets != 0 }

// IDs lists the shown issues sorted by ID.
func (m *Matches) IDs() []string { return m.ids }

// Len is the number of shown issues.
func (m *Matches) Len() int { return len(m.ids) }

// Apply evaluates the scope over every issue of snap.
func (s Scope) Apply(snap *Snapshot, st Statuses) *Matches {
	m := &Matches{flags: make(map[string]uint8, snap.Len())}
	parents := s.parentSets(snap)
	needLower := slices.ContainsFunc(s.free, func(t textTerm) bool { return !t.exact })
	for _, id := range snap.IDs() {
		is, _ := snap.Issue(id)
		pres := snap.Present(id, st)
		if !s.matchFacets(snap, is, pres, parents) || !s.matchText(is, needLower) {
			continue
		}
		f := flagFacets
		if s.visible(is, pres) {
			f |= flagVisible
			m.ids = append(m.ids, id)
		} else if pres.Status == Closed {
			m.HiddenClosed++
		}
		m.flags[id] = f
	}
	return m
}

func (s Scope) parentSets(snap *Snapshot) map[string]map[string]struct{} {
	var out map[string]map[string]struct{}
	for _, f := range s.facets {
		if f.key != facetParent {
			continue
		}
		if out == nil {
			out = map[string]map[string]struct{}{}
		}
		if _, ok := out[f.val]; ok {
			continue
		}
		root := ""
		for _, id := range snap.IDs() {
			if strings.EqualFold(id, f.val) {
				root = id
				break
			}
		}
		set := map[string]struct{}{}
		stack := slices.Clone(snap.Children(root))
		for len(stack) > 0 {
			c := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, dup := set[c]; dup || c == root {
				continue
			}
			set[c] = struct{}{}
			stack = append(stack, snap.Children(c)...)
		}
		out[f.val] = set
	}
	return out
}

func statusName(p PresentationStatus) string {
	return strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "").Replace(p.String()))
}

func sameStatus(val string, is *Issue, pres Presentation) bool {
	fold := strings.NewReplacer("_", "", "-", "")
	v := fold.Replace(val)
	return v == statusName(pres.Status) || strings.EqualFold(val, is.Status) || v == fold.Replace(strings.ToLower(is.Status))
}

func (s Scope) termMatches(f facetTerm, snap *Snapshot, is *Issue, pres Presentation, parents map[string]map[string]struct{}) bool {
	switch f.key {
	case facetType:
		return strings.EqualFold(is.IssueType, f.val)
	case facetLabel:
		return slices.ContainsFunc(is.Labels, func(l string) bool { return strings.EqualFold(l, f.val) })
	case facetPriority:
		return strconv.Itoa(is.Priority) == f.val
	case facetStatus:
		return sameStatus(f.val, is, pres)
	case facetAssignee:
		if f.val == assigneeNone {
			return is.Assignee == ""
		}
		return strings.EqualFold(is.Assignee, f.val)
	case facetIs:
		return snap.IsReady(is.ID)
	case facetParent:
		_, ok := parents[f.val][is.ID]
		return ok
	case facetCount:
	}
	return false
}

func (s Scope) matchFacets(snap *Snapshot, is *Issue, pres Presentation, parents map[string]map[string]struct{}) bool {
	var pos, hit [facetCount]bool
	for _, f := range s.facets {
		ok := s.termMatches(f, snap, is, pres, parents)
		if f.neg {
			if ok {
				return false
			}
			continue
		}
		pos[f.key] = true
		hit[f.key] = hit[f.key] || ok
	}
	for k := range pos {
		if pos[k] && !hit[k] {
			return false
		}
	}
	return true
}

func (s Scope) matchText(is *Issue, needLower bool) bool {
	if len(s.free) == 0 {
		return true
	}
	hay := strings.Join([]string{
		is.ID, is.Title, is.Description, is.Notes, is.Design, is.AcceptanceCriteria, is.Assignee, strings.Join(is.Labels, " "),
	}, "\n")
	var lower string
	if needLower {
		lower = strings.ToLower(hay)
	}
	for _, t := range s.free {
		var found bool
		if t.exact {
			found = strings.Contains(hay, t.text)
		} else {
			found = strings.Contains(lower, strings.ToLower(t.text))
		}
		if found == t.neg {
			return false
		}
	}
	return true
}

// visible applies the status visibility: closed issues are hidden unless the
// default shows them or a status facet names them.
func (s Scope) visible(is *Issue, pres Presentation) bool {
	if pres.Status != Closed || s.showClosed {
		return true
	}
	for _, f := range s.facets {
		if f.key == facetStatus && !f.neg && sameStatus(f.val, is, pres) {
			return true
		}
	}
	return false
}

// FacetCount is one value of a facet and how many issues carry it.
type FacetCount struct {
	Name string
	N    int
}

// Facets holds the values the filter offers, counted over the whole
// snapshot. Lists are ordered by count, then name.
type Facets struct {
	Types      []FacetCount
	Labels     []FacetCount
	Assignees  []FacetCount
	Priorities [5]int
	// Statuses is indexed by [PresentationStatus].
	Statuses   [6]int
	Unassigned int
}

// CountFacets counts the facet values of every issue in snap.
func CountFacets(snap *Snapshot, st Statuses) Facets {
	var f Facets
	types, labels, who := map[string]int{}, map[string]int{}, map[string]int{}
	for _, id := range snap.IDs() {
		is, _ := snap.Issue(id)
		if is.IssueType != "" {
			types[is.IssueType]++
		}
		for _, l := range is.Labels {
			labels[l]++
		}
		if is.Assignee == "" {
			f.Unassigned++
		} else {
			who[is.Assignee]++
		}
		if is.Priority >= 0 && is.Priority < len(f.Priorities) {
			f.Priorities[is.Priority]++
		}
		f.Statuses[snap.Present(id, st).Status]++
	}
	f.Types, f.Labels, f.Assignees = ranked(types), ranked(labels), ranked(who)
	return f
}

func ranked(m map[string]int) []FacetCount {
	out := make([]FacetCount, 0, len(m))
	for n, c := range m {
		out = append(out, FacetCount{n, c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Name < out[j].Name
	})
	return out
}
