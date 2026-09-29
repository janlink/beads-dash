// Package command is the grammar of the command bar: a table of named
// commands, a parser that reports where input goes wrong, live hints and Tab
// completion. It runs nothing; the shell binds each command to its effect.
package command

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Spec describes one command.
type Spec struct {
	// Name is the verb, lowercase. Aliases are other spellings of it.
	Name    string
	Aliases []string
	// Usage lists the arguments as typed after the verb, e.g. "<name>".
	Usage   string
	Summary string
	// Help is the longer text `:help <name>` shows.
	Help string
	// Min and Max bound the argument count; Max < 0 means no limit.
	Min, Max int
	// Args proposes completions for the next argument given the arguments
	// before it and the text typed so far for it. It may be nil.
	Args func(prev []string, prefix string) []string
}

// Names lists the verb and its aliases.
func (s Spec) Names() []string { return append([]string{s.Name}, s.Aliases...) }

// Line is the usage line: the verb followed by its arguments.
func (s Spec) Line() string {
	if s.Usage == "" {
		return ":" + s.Name
	}
	return ":" + s.Name + " " + s.Usage
}

// Table is the set of registered commands.
type Table struct {
	specs  map[string]Spec
	byName map[string]string
}

// NewTable returns an empty table.
func NewTable() *Table {
	return &Table{specs: map[string]Spec{}, byName: map[string]string{}}
}

// Register adds a command. A name or alias that is taken, an empty name or a
// name with whitespace is refused.
func (t *Table) Register(s Spec) error {
	s.Name = strings.ToLower(s.Name)
	aliases := make([]string, len(s.Aliases))
	for i, a := range s.Aliases {
		aliases[i] = strings.ToLower(a)
	}
	s.Aliases = aliases
	for _, n := range s.Names() {
		if n == "" || strings.IndexFunc(n, unicode.IsSpace) >= 0 || strings.HasPrefix(n, ":") {
			return fmt.Errorf("command: invalid name %q", n)
		}
		if owner, taken := t.byName[n]; taken {
			return fmt.Errorf("command: %q is already the name of %q", n, owner)
		}
	}
	t.specs[s.Name] = s
	for _, n := range s.Names() {
		t.byName[n] = s.Name
	}
	return nil
}

// Lookup finds a command by verb or alias, ignoring case.
func (t *Table) Lookup(name string) (Spec, bool) {
	owner, ok := t.byName[strings.ToLower(name)]
	if !ok {
		return Spec{}, false
	}
	return t.specs[owner], true
}

// Specs lists the commands sorted by name.
func (t *Table) Specs() []Spec {
	out := make([]Spec, 0, len(t.specs))
	for _, s := range t.specs {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Word is one whitespace-separated word of the input and where it starts, in
// runes.
type Word struct {
	Text string
	Pos  int
}

// Error is a problem with the input at a rune position.
type Error struct {
	Pos int
	Msg string
}

func (e *Error) Error() string { return e.Msg }

// Words splits input into words; double quotes group words with spaces. An
// unterminated quote is an error at the quote.
func Words(input string) ([]Word, *Error) {
	var out []Word
	var b strings.Builder
	start, quoteAt := -1, -1
	flush := func() {
		if start >= 0 {
			out = append(out, Word{b.String(), start})
		}
		b.Reset()
		start = -1
	}
	i := 0
	for _, r := range input {
		switch {
		case r == '"':
			if quoteAt >= 0 {
				quoteAt = -1
			} else {
				quoteAt = i
			}
			if start < 0 {
				start = i
			}
		case unicode.IsSpace(r) && quoteAt < 0:
			flush()
		default:
			if start < 0 {
				start = i
			}
			b.WriteRune(r)
		}
		i++
	}
	if quoteAt >= 0 {
		return nil, &Error{quoteAt, "unterminated quote"}
	}
	flush()
	return out, nil
}

// strip removes leading space and one colon, and returns how many runes it
// took off.
func strip(input string) (rest string, lead int) {
	rest = strings.TrimLeftFunc(input, unicode.IsSpace)
	if r, ok := strings.CutPrefix(rest, ":"); ok {
		rest = r
	}
	return rest, utf8.RuneCountInString(input) - utf8.RuneCountInString(rest)
}

// Call is a command with its arguments.
type Call struct {
	Spec Spec
	Args []Word
}

// Result is the outcome of parsing input. At most one of Call, ID and Err is
// set; all are empty for blank input.
type Result struct {
	Call *Call
	// ID is input that names no command and is one word: the shell tries it
	// as an issue ID; IDPos is where it starts.
	ID    string
	IDPos int
	Err   *Error
}

// Parse reads input as `verb args`, a leading colon allowed. Commands
// resolve first; only a single word that is not a command comes back as an
// ID. Arguments that are too few or too many are errors at the place they
// go wrong.
func (t *Table) Parse(input string) Result {
	trimmed, lead := strip(input)
	words, err := Words(trimmed)
	if err != nil {
		err.Pos += lead
		return Result{Err: err}
	}
	for i := range words {
		words[i].Pos += lead
	}
	if len(words) == 0 {
		return Result{}
	}
	verb := words[0]
	spec, ok := t.Lookup(verb.Text)
	if !ok {
		if len(words) == 1 {
			return Result{ID: verb.Text, IDPos: verb.Pos}
		}
		return Result{Err: &Error{verb.Pos, fmt.Sprintf("unknown command %q", verb.Text)}}
	}
	args := words[1:]
	end := lead + utf8.RuneCountInString(trimmed)
	switch {
	case len(args) < spec.Min:
		return Result{Err: &Error{end, fmt.Sprintf("%s needs %s", spec.Line(), argsNeeded(spec, len(args)))}}
	case spec.Max >= 0 && len(args) > spec.Max:
		extra := args[spec.Max]
		return Result{Err: &Error{extra.Pos, fmt.Sprintf("%s takes %s, not %q", spec.Line(), argCount(spec.Max), extra.Text)}}
	}
	return Result{Call: &Call{Spec: spec, Args: args}}
}

func argCount(n int) string {
	switch n {
	case 0:
		return "no arguments"
	case 1:
		return "one argument"
	}
	return fmt.Sprintf("%d arguments", n)
}

func argsNeeded(s Spec, have int) string {
	if have == 0 {
		return "an argument"
	}
	return fmt.Sprintf("%s (%d given)", argCount(s.Min), have)
}

// Hint says what the input recognised so far means, for the line under the
// bar: a command's usage and summary, the commands that continue a partial
// verb, or nothing.
func (t *Table) Hint(input string) string {
	trimmed, _ := strip(input)
	words, err := Words(trimmed)
	if err != nil || len(words) == 0 {
		return ""
	}
	if spec, ok := t.Lookup(words[0].Text); ok {
		return spec.Line() + "  " + spec.Summary
	}
	if len(words) == 1 && !endsWithSpace(trimmed) {
		var names []string
		for _, s := range t.Specs() {
			if slices.ContainsFunc(s.Names(), func(n string) bool { return strings.HasPrefix(n, strings.ToLower(words[0].Text)) }) {
				names = append(names, s.Name)
			}
		}
		if len(names) > 0 {
			return strings.Join(names, "  ")
		}
	}
	return ""
}

func endsWithSpace(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	return s != "" && unicode.IsSpace(r)
}

// Completion is the set of texts that can replace the word being typed.
type Completion struct {
	// Start is the rune position where the replaced word begins; the word
	// runs to the end of the input.
	Start int
	Cands []string
}

// Complete proposes replacements for the last word of input: command names
// and issue IDs (from ids) at the verb, the command's own arguments after
// it. A candidate that needs quotes comes back quoted.
func (t *Table) Complete(input string, ids func(prefix string) []string) Completion {
	trimmed, lead := strip(input)
	words, err := Words(trimmed)
	if err != nil {
		return Completion{}
	}
	fresh := endsWithSpace(trimmed)
	idx := len(words) - 1
	prefix, start := "", lead+utf8.RuneCountInString(trimmed)
	if fresh || len(words) == 0 {
		idx = len(words)
	} else {
		prefix, start = words[idx].Text, lead+words[idx].Pos
	}
	var cands []string
	if idx == 0 {
		low := strings.ToLower(prefix)
		for _, s := range t.Specs() {
			for _, n := range s.Names() {
				if strings.HasPrefix(n, low) {
					cands = append(cands, n)
				}
			}
		}
		if ids != nil {
			cands = append(cands, ids(prefix)...)
		}
	} else if spec, ok := t.Lookup(words[0].Text); ok && spec.Args != nil {
		prev := make([]string, 0, idx-1)
		for _, w := range words[1:idx] {
			prev = append(prev, w.Text)
		}
		cands = spec.Args(prev, prefix)
	}
	for i, c := range cands {
		if strings.ContainsAny(c, " \t") {
			cands[i] = `"` + c + `"`
		}
	}
	return Completion{Start: start, Cands: dedupe(cands)}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
