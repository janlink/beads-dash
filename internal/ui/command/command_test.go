package command_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/ui/command"
)

func table(t *testing.T) *command.Table {
	t.Helper()
	tb := command.NewTable()
	for _, s := range []command.Spec{
		{Name: "view", Usage: "<name|1-6>", Summary: "switch view", Min: 1, Max: 1, Args: func(prev []string, p string) []string {
			if len(prev) > 0 {
				return nil
			}
			return filter([]string{"overview", "tree", "kanban", "ready"}, p)
		}},
		{Name: "go", Usage: "<id>", Summary: "jump to an issue", Min: 1, Max: 1},
		{Name: "clear", Summary: "clear the scope", Max: 0},
		{Name: "quit", Aliases: []string{"q"}, Summary: "quit", Max: 0},
		{Name: "close", Usage: "[reason]", Summary: "close", Min: 0, Max: -1},
		{Name: "assign", Usage: "<who>", Summary: "assign", Min: 1, Max: 1, Args: func(_ []string, p string) []string {
			return filter([]string{"me", "Jan Link", "-"}, p)
		}},
	} {
		if err := tb.Register(s); err != nil {
			t.Fatal(err)
		}
	}
	return tb
}

func texts(ws []command.Word) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.Text
	}
	return out
}

func filter(all []string, prefix string) []string {
	var out []string
	for _, s := range all {
		if strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix)) {
			out = append(out, s)
		}
	}
	return out
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		verb    string
		args    []string
		id      string
		errPos  int
		errText string
	}{
		{name: "blank"},
		{name: "spaces only", in: "   "},
		{name: "colon only", in: ":"},
		{name: "command", in: "clear", verb: "clear"},
		{name: "leading colon", in: ":clear", verb: "clear"},
		{name: "alias", in: "q", verb: "quit"},
		{name: "case folded", in: "VIEW Tree", verb: "view", args: []string{"Tree"}},
		{name: "argument", in: "view kanban", verb: "view", args: []string{"kanban"}},
		{name: "quoted argument", in: `assign "Jan Link"`, verb: "assign", args: []string{"Jan Link"}},
		{name: "free tail", in: "close not needed any more", verb: "close", args: []string{"not", "needed", "any", "more"}},
		{name: "id fallback", in: "ws-4k2", id: "ws-4k2"},
		{name: "id fallback after colon", in: ":ws-4k2.1", id: "ws-4k2.1"},
		{name: "explicit go", in: "go ws-4k2", verb: "go", args: []string{"ws-4k2"}},

		{name: "kanban is not a command", in: "kanban", id: "kanban"},
		{name: "kanban with a colon is not a view", in: ":kanban", id: "kanban"},
		{name: "view prefix is not a command", in: "vie kanban", errPos: 0, errText: `unknown command "vie"`},
		{name: "v shorthand is not a command", in: "v 3", errPos: 0, errText: `unknown command "v"`},
		{name: "glued argument", in: "view3", id: "view3"},
		{name: "bang is not quit", in: "q!", id: "q!"},
		{name: "unknown with args", in: ":kanban board", errPos: 1, errText: `unknown command "kanban"`},
		{name: "missing argument", in: "view", errPos: 4, errText: ":view <name|1-6> needs an argument"},
		{name: "missing argument after colon", in: ":go", errPos: 3, errText: ":go <id> needs an argument"},
		{name: "extra argument", in: "view tree kanban", errPos: 10, errText: `takes one argument, not "kanban"`},
		{name: "extra after colon and space", in: ":  clear now", errPos: 9, errText: `takes no arguments, not "now"`},
		{name: "unterminated quote", in: `assign "Jan`, errPos: 7, errText: "unterminated quote"},
	}
	tb := table(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tb.Parse(tc.in)
			switch {
			case tc.errText != "":
				if r.Err == nil || r.Err.Pos != tc.errPos || !strings.Contains(r.Err.Msg, tc.errText) {
					t.Fatalf("Parse(%q) = %+v, want error %q at %d", tc.in, r, tc.errText, tc.errPos)
				}
			case r.Err != nil:
				t.Fatalf("Parse(%q) failed: %v", tc.in, r.Err)
			case tc.verb != "":
				if r.Call == nil || r.Call.Spec.Name != tc.verb || !slices.Equal(texts(r.Call.Args), tc.args) {
					t.Fatalf("Parse(%q) = %+v, want %s %q", tc.in, r, tc.verb, tc.args)
				}
			case r.Call != nil || r.ID != tc.id:
				t.Fatalf("Parse(%q) = %+v, want id %q", tc.in, r, tc.id)
			}
		})
	}
}

func TestRegisterRefusesClashes(t *testing.T) {
	tb := table(t)
	for _, s := range []command.Spec{
		{Name: "view"}, {Name: "x", Aliases: []string{"q"}}, {Name: ""}, {Name: "two words"}, {Name: ":colon"},
	} {
		if err := tb.Register(s); err == nil {
			t.Errorf("Register(%+v) accepted", s)
		}
	}
	if _, ok := tb.Lookup("Q"); !ok {
		t.Error("lookup is case-insensitive")
	}
}

func TestHint(t *testing.T) {
	tb := table(t)
	tests := map[string]string{
		"":         "",
		"view":     ":view <name|1-6>  switch view",
		":view t":  ":view <name|1-6>  switch view",
		"c":        "clear  close",
		"cl":       "clear  close",
		"nothing":  "",
		"clear ":   ":clear  clear the scope",
		`assign "`: "",
	}
	for in, want := range tests {
		if got := tb.Hint(in); got != want {
			t.Errorf("Hint(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestComplete(t *testing.T) {
	tb := table(t)
	ids := func(p string) []string { return filter([]string{"ws-4k2", "ws-9qe", "cl-1"}, p) }
	tests := []struct {
		in    string
		start int
		want  []string
	}{
		{"", 0, []string{"assign", "clear", "close", "go", "q", "quit", "view", "ws-4k2", "ws-9qe", "cl-1"}},
		{"cl", 0, []string{"clear", "close", "cl-1"}},
		{":cl", 1, []string{"clear", "close", "cl-1"}},
		{"ws-", 0, []string{"ws-4k2", "ws-9qe"}},
		{"view ", 5, []string{"overview", "tree", "kanban", "ready"}},
		{"view k", 5, []string{"kanban"}},
		{"view tree ", 10, nil},
		{"assign j", 7, []string{`"Jan Link"`}},
		{"assign  ", 8, []string{"me", `"Jan Link"`, "-"}},
		{"go ws", 3, nil},
		{`assign "Jan`, 0, nil},
	}
	for _, tc := range tests {
		got := tb.Complete(tc.in, ids)
		if tc.in == "" {
			slices.Sort(got.Cands)
			slices.Sort(tc.want)
		}
		if got.Start != tc.start && len(tc.want) > 0 || !slices.Equal(got.Cands, tc.want) {
			t.Errorf("Complete(%q) = %+v, want start %d %q", tc.in, got, tc.start, tc.want)
		}
	}
}

func TestRegisterDoesNotTouchTheCallersAliases(t *testing.T) {
	aliases := []string{"Ex"}
	if err := command.NewTable().Register(command.Spec{Name: "exit", Aliases: aliases}); err != nil {
		t.Fatal(err)
	}
	if aliases[0] != "Ex" {
		t.Errorf("aliases rewritten to %q", aliases)
	}
}

func TestCompleteGivesEarlierArgumentsToArgs(t *testing.T) {
	tb := command.NewTable()
	var seen []string
	_ = tb.Register(command.Spec{Name: "link", Min: 2, Max: 2, Args: func(prev []string, _ string) []string {
		seen = prev
		return []string{"x"}
	}})
	tb.Complete("link a ", nil)
	if !slices.Equal(seen, []string{"a"}) {
		t.Errorf("Args got %q", seen)
	}
	tb.Complete("link a b", nil)
	if !slices.Equal(seen, []string{"a"}) {
		t.Errorf("Args got %q for the second argument being typed", seen)
	}
}
