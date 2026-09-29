package keys

import (
	"slices"
	"sort"
)

// Hint is one footer or dialog-frame hint.
type Hint struct {
	Key, Desc string
}

// Hints returns the hints of the contexts live in c, most important first, so
// a caller that runs out of width drops from the end.
func (m *Map) Hints(c Context) []Hint {
	type ranked struct {
		Hint
		rank, order int
	}
	var all []ranked
	n := 0
	var claimed claims
	for _, lc := range Layered(c) {
		bs := claimed.unshadowed(m.Active(lc))
		for _, b := range bs {
			n++
			if b.Hint > 0 {
				all = append(all, ranked{b.hint(), b.Hint, n})
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].rank != all[j].rank {
			return all[i].rank < all[j].rank
		}
		return all[i].order < all[j].order
	})
	out := make([]Hint, len(all))
	for i, r := range all {
		out[i] = r.Hint
	}
	return out
}

// Section is one block of the help overlay.
type Section struct {
	Title    string
	Bindings []Binding
}

var sectionTitles = [contextCount]string{
	Always:     "Everywhere",
	Global:     "Everywhere",
	View:       "Lists",
	Memories:   "Memories",
	Tree:       "Tree",
	Overview:   "Overview",
	Graph:      "Graph",
	Panel:      "Panel",
	Bar:        "Docked bar",
	Form:       "Forms",
	Help:       "Help",
	Appearance: "Appearance",
	Details:    "Error details",
	Startup:    "Startup",
	TooSmall:   "Terminal too small",
}

// Sections returns what the help overlay lists for c: c's own keys first, then
// the global ones. Reserved bindings are left out; contexts with the same
// title merge.
func (m *Map) Sections(c Context) []Section {
	var out []Section
	index := map[string]int{}
	var claimed claims
	for _, lc := range Layered(c) {
		bs := claimed.unshadowed(m.Active(lc))
		if len(bs) == 0 {
			continue
		}
		title := sectionTitles[lc]
		i, ok := index[title]
		if !ok {
			i = len(out)
			index[title] = i
			out = append(out, Section{Title: title})
		}
		out[i].Bindings = append(out[i].Bindings, bs...)
	}
	return out
}

// claims are the keys a more specific context already binds; the same key in
// a later context is shadowed and neither listed nor hinted.
type claims map[string]bool

// unshadowed returns the bindings of bs whose keys no earlier context bound,
// then claims their keys.
func (c *claims) unshadowed(bs []Binding) []Binding {
	if *c == nil {
		*c = claims{}
	}
	var out []Binding
	for _, b := range bs {
		if !slices.ContainsFunc(b.Keys, func(k string) bool { return (*c)[k] }) {
			out = append(out, b)
		}
	}
	for _, b := range bs {
		for _, k := range b.Keys {
			(*c)[k] = true
		}
	}
	return out
}
