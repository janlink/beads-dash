package keys_test

import (
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/ui/keys"
)

func TestDefaultMapHasNoConflicts(t *testing.T) {
	m := keys.Default()
	if got := m.Conflicts(); len(got) > 0 {
		t.Errorf("conflicts:\n%s", strings.Join(got, "\n"))
	}
}

func TestNoConflictsPerContext(t *testing.T) {
	m := keys.Default()
	for _, c := range keys.Contexts() {
		t.Run(c.String(), func(t *testing.T) {
			seen := map[string]keys.Action{}
			for _, b := range m.Bindings(c) {
				if len(b.Keys) == 0 || b.Action == "" || b.Desc == "" {
					t.Errorf("%+v is incomplete", b)
				}
				for _, k := range b.Keys {
					if prev, dup := seen[k]; dup {
						t.Errorf("%q is bound to %s and %s", k, prev, b.Action)
					}
					seen[k] = b.Action
				}
			}
		})
	}
}

func TestConflictsAreDetected(t *testing.T) {
	var m keys.Map
	keys.Add(&m, keys.View,
		keys.Binding{Keys: []string{"x"}, Action: "a", Desc: "a"},
		keys.Binding{Keys: []string{"x"}, Action: "b", Desc: "b"},
		keys.Binding{Keys: []string{"g"}, Action: "c", Desc: "c"},
		keys.Binding{Keys: []string{"g g"}, Action: "d", Desc: "d"},
	)
	keys.Add(&m, keys.Global, keys.Binding{Keys: []string{"x"}, Action: "e", Desc: "e"})
	got := strings.Join(m.Conflicts(), "\n")
	for _, want := range []string{`view: "x" is bound to a and b`, `view: "x" is bound to a and e`, `view: "g" is a key and the start of "g g"`} {
		if !strings.Contains(got, want) {
			t.Errorf("conflicts %q lack %q", got, want)
		}
	}
}

func TestReservedKeysAreClaimed(t *testing.T) {
	m := keys.Default()
	claimed := map[string]bool{}
	for _, c := range []keys.Context{keys.Global, keys.View, keys.Tree, keys.Panel} {
		for _, b := range m.Bindings(c) {
			for _, k := range b.Keys {
				claimed[k] = true
			}
		}
	}
	for _, k := range []string{"s", "p", "a", "#", "c", "y", "x", "n", "N", "e", "<", ">", "]", "[", "o", "m", "+", "-", "z h", "z M", "z R"} {
		if !claimed[k] {
			t.Errorf("%q is not reserved", k)
		}
	}
}

func TestLaterBindingsAreInactive(t *testing.T) {
	m := keys.Default()
	x := keys.NewMatcher(m)
	if _, r := x.Feed(keys.View, "o"); r != keys.NoMatch {
		t.Errorf("reserved o resolved: %v", r)
	}
	if b, r := x.Feed(keys.View, "j"); r != keys.Matched || b.Action != keys.NavDown {
		t.Errorf("j = %v %v", b.Action, r)
	}
}

func TestSequences(t *testing.T) {
	m := keys.Default()
	x := keys.NewMatcher(m)
	if _, r := x.Feed(keys.View, "g"); r != keys.Pending {
		t.Fatalf("g = %v, want pending", r)
	}
	if b, r := x.Feed(keys.View, "g"); r != keys.Matched || b.Action != keys.NavFirst {
		t.Errorf("gg = %v %v", b.Action, r)
	}
	x.Feed(keys.View, "g")
	if b, r := x.Feed(keys.View, "j"); r != keys.Matched || b.Action != keys.NavDown {
		t.Errorf("g then j = %v %v, want j on its own", b.Action, r)
	}
	if _, r := x.Feed(keys.View, "g"); r != keys.Pending {
		t.Error("pending sequence survived a match")
	}
}

func TestContextsResolveMostSpecificFirst(t *testing.T) {
	m := keys.Default()
	x := keys.NewMatcher(m)
	if b, _ := x.Feed(keys.Help, "esc"); b.Action != keys.Close {
		t.Errorf("help esc = %v", b.Action)
	}
	if b, r := x.Feed(keys.Help, "q"); r != keys.NoMatch {
		t.Errorf("help q = %v, want no match: dialogs own their keys (%v)", r, b.Action)
	}
	if b, r := x.Feed(keys.Startup, "ctrl+c"); r != keys.Matched || b.Action != keys.QuitForce {
		t.Errorf("startup ctrl+c = %v %v", b.Action, r)
	}
	if b, r := x.Feed(keys.View, "3"); r != keys.Matched || b.Action != keys.SwitchView {
		t.Errorf("3 = %v %v", b.Action, r)
	}
}

func TestHintsAreRankedAndSkipReserved(t *testing.T) {
	m := keys.Default()
	hints := m.Hints(keys.View)
	if len(hints) < 4 || hints[0].Key != "?" {
		t.Fatalf("hints = %v", hints)
	}
	for _, h := range hints {
		if h.Key == "s" {
			t.Errorf("reserved hint %v", h)
		}
	}
}

func TestSectionsListCurrentContextFirst(t *testing.T) {
	m := keys.Default()
	secs := m.Sections(keys.View)
	if len(secs) != 2 || secs[0].Title != "Lists" || secs[1].Title != "Everywhere" {
		t.Fatalf("sections = %+v", secs)
	}
	for _, s := range secs {
		for _, b := range s.Bindings {
			if b.Later {
				t.Errorf("reserved %v listed", b.Keys)
			}
		}
	}
	memories := m.Sections(keys.Memories)
	if memories[0].Title != "Lists" && memories[0].Title != "Memories" {
		t.Errorf("memories sections = %+v", memories)
	}
}

func TestQuitIsOnlyBoundAtBaseLevel(t *testing.T) {
	x := keys.NewMatcher(keys.Default())
	if b, r := x.Feed(keys.View, "q"); r != keys.Matched || b.Action != keys.Quit {
		t.Errorf("view q = %v %v", b.Action, r)
	}
	if _, r := x.Feed(keys.Panel, "q"); r != keys.NoMatch {
		t.Errorf("panel q = %v, want no match", r)
	}
}

func TestFoldAllKeysBelongToTheTree(t *testing.T) {
	x := keys.NewMatcher(keys.Default())
	if _, r := x.Feed(keys.Tree, "z"); r != keys.NoMatch && r != keys.Pending {
		t.Errorf("z = %v", r)
	}
	m := keys.Default()
	for _, k := range []string{"z M", "z R"} {
		found := false
		for _, b := range m.Bindings(keys.Tree) {
			for _, bk := range b.Keys {
				found = found || bk == k
			}
		}
		if !found {
			t.Errorf("%q is not reserved for the tree", k)
		}
	}
}

func TestDetailToggleIsGlobalAndFree(t *testing.T) {
	m := keys.Default()
	found := false
	for _, c := range keys.Contexts() {
		for _, b := range m.Active(c) {
			for _, k := range b.Keys {
				if k == "D" {
					found = found || (c == keys.Global && b.Action == keys.DetailToggle)
					if b.Action != keys.DetailToggle {
						t.Errorf("D is also bound to %s in %s", b.Action, c)
					}
				}
			}
		}
	}
	if !found {
		t.Error("D is not bound to the detail toggle in the global context")
	}
}
