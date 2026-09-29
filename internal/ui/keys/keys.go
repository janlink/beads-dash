// Package keys is bdash's declarative key map. Key handling, the help overlay,
// the footer hints and the dialog frame hints are all generated from Bindings.
package keys

import (
	"fmt"
	"slices"
	"strings"
)

// Context is a place the keys act in. A key press is resolved against the
// contexts active at the top layer, most specific first.
type Context int

const (
	// Always holds the keys that work in every layer.
	Always Context = iota
	// Global holds the base-level keys that work in any view and in panel focus.
	Global
	// View holds the list-view keys shared by every view.
	View
	// Memories holds the keys only the Memories view adds or overrides.
	Memories
	// Tree holds the keys only the Tree view adds.
	Tree
	// Overview holds the keys the Overview overrides.
	Overview
	// Panel holds the keys of a focused detail panel or Overview panel.
	Panel
	// Bar holds the keys of the docked search, filter and command bars.
	Bar
	// Form holds the keys inside form dialogs.
	Form
	// Help holds the keys of the help overlay.
	Help
	// Appearance holds the keys of the Appearance dialog.
	Appearance
	// Details holds the keys of the error details overlay.
	Details
	// Startup holds the keys of the full-screen startup and fatal screens.
	Startup
	// TooSmall holds the keys of the terminal too small screen.
	TooSmall

	contextCount
)

var contextNames = [contextCount]string{
	Always: "always", Global: "global", View: "view", Memories: "memories", Tree: "tree", Overview: "overview", Panel: "panel",
	Bar: "docked bar", Form: "form", Help: "help", Appearance: "appearance",
	Details: "details", Startup: "startup", TooSmall: "too small",
}

func (c Context) String() string {
	if c < 0 || c >= contextCount {
		return "unknown"
	}
	return contextNames[c]
}

// Contexts lists every context.
func Contexts() []Context {
	out := make([]Context, contextCount)
	for i := range out {
		out[i] = Context(i)
	}
	return out
}

// Action names what a binding does. The shell and the views switch on it.
type Action string

// Binding is one entry of the key map.
type Binding struct {
	// Keys are the alternatives that trigger the action. A key is one
	// tea.KeyPressMsg.String() value, or several separated by a space for a
	// sequence such as "g g".
	Keys   []string
	Action Action
	// Label is how help and hints spell the keys; it defaults to the keys
	// joined by "/".
	Label string
	Desc  string
	// HintKey and HintDesc replace Text and Desc in hints, for bindings that
	// share a hint with their counterpart such as j and k.
	HintKey, HintDesc string
	// Hint ranks the binding for the footer and dialog hints: 1 is dropped
	// last, 0 never shown.
	Hint int
	// Later marks a binding reserved for a feature that has not landed. It
	// takes part in conflict checks but is neither active nor listed.
	Later bool
}

// Text is the key label help and hints show.
func (b Binding) Text() string {
	if b.Label != "" {
		return b.Label
	}
	return strings.Join(b.Keys, "/")
}

func (b Binding) hint() Hint {
	h := Hint{b.Text(), b.Desc}
	if b.HintKey != "" {
		h.Key = b.HintKey
	}
	if b.HintDesc != "" {
		h.Desc = b.HintDesc
	}
	return h
}

// Map is the key map: bindings per context, in the order help lists them.
type Map struct {
	by [contextCount][]Binding
}

// Bindings returns the bindings of one context.
func (m *Map) Bindings(c Context) []Binding { return m.by[c] }

// Active returns the bindings of c that are not reserved.
func (m *Map) Active(c Context) []Binding {
	var out []Binding
	for _, b := range m.by[c] {
		if !b.Later {
			out = append(out, b)
		}
	}
	return out
}

// Layered lists the contexts a key press in c is resolved against, most
// specific first.
func Layered(c Context) []Context {
	if c == Memories || c == Tree || c == Overview {
		return []Context{c, View, Global, Always}
	}
	if c == View || c == Panel {
		return []Context{c, Global, Always}
	}
	if c == Always {
		return []Context{Always}
	}
	return []Context{c, Always}
}

// checked lists the contexts whose keys must not collide with those of c. The
// Memories and Overview views override View keys on purpose, so the two are not compared.
func checked(c Context) []Context {
	if c == Memories || c == Tree || c == Overview {
		return []Context{c, Global, Always}
	}
	return Layered(c)
}

// Result is the outcome of feeding a key to a [Matcher].
type Result int

const (
	// NoMatch means no binding starts with the pending keys.
	NoMatch Result = iota
	// Pending means the keys so far are the start of a longer sequence.
	Pending
	// Matched means a binding completed.
	Matched
)

// Matcher resolves key presses against contexts, remembering the first keys
// of a sequence between presses.
type Matcher struct {
	m       *Map
	pending []string
}

// NewMatcher returns a matcher over m.
func NewMatcher(m *Map) *Matcher { return &Matcher{m: m} }

// Feed adds key and resolves it against the live contexts of c, c first. A
// key that does not continue a pending sequence starts over.
func (x *Matcher) Feed(c Context, key string) (Binding, Result) {
	if b, r := x.try(c, append(slices.Clone(x.pending), key)); r != NoMatch {
		return b, r
	}
	if len(x.pending) > 0 {
		x.pending = nil
		return x.try(c, []string{key})
	}
	return Binding{}, NoMatch
}

func (x *Matcher) try(c Context, seq []string) (Binding, Result) {
	joined := strings.Join(seq, " ")
	partial := false
	for _, lc := range Layered(c) {
		for _, b := range x.m.Active(lc) {
			for _, k := range b.Keys {
				switch {
				case k == joined:
					x.pending = nil
					return b, Matched
				case strings.HasPrefix(k, joined+" "):
					partial = true
				}
			}
		}
	}
	if partial {
		x.pending = seq
		return Binding{}, Pending
	}
	return Binding{}, NoMatch
}

// Conflicts lists the problems of the map: a key bound twice among the
// contexts that are live together, or a key that is also the start of a
// longer sequence.
func (m *Map) Conflicts() []string {
	var out []string
	for _, c := range Contexts() {
		bound := map[string]Action{}
		var keys []string
		for _, lc := range checked(c) {
			for _, b := range m.by[lc] {
				for _, k := range b.Keys {
					if prev, dup := bound[k]; dup {
						out = append(out, fmt.Sprintf("%s: %q is bound to %s and %s", c, k, prev, b.Action))
						continue
					}
					bound[k] = b.Action
					keys = append(keys, k)
				}
			}
		}
		for _, a := range keys {
			for _, b := range keys {
				if strings.HasPrefix(b, a+" ") {
					out = append(out, fmt.Sprintf("%s: %q is a key and the start of %q", c, a, b))
				}
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Add appends bindings to a context; it is how tests and views
// extend a map.
func Add(m *Map, c Context, bs ...Binding) { m.by[c] = append(m.by[c], bs...) }
