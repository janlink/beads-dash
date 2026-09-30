package form

import (
	"slices"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/ui/input"
)

// Kind is how a field edits its value.
type Kind int

const (
	// Text is one line.
	Text Kind = iota
	// Area is several lines.
	Area
	// Choice is one of Options; left and right cycle, a letter jumps.
	Choice
	// Tokens is a list typed as words separated by spaces or commas.
	Tokens
	// Links is a list of issue IDs picked from the picker.
	Links
	// Fold is the row that opens and closes the advanced fields.
	Fold
)

// Field is one row of a form.
type Field struct {
	// Key names the field for the owner; Label is what the form shows.
	Key, Label string
	Kind       Kind
	Required   bool
	// Advanced fields sit behind the Fold row.
	Advanced bool
	// Skip hides the field while the owner does not need it.
	Skip bool
	// Note is dim text after the value.
	Note        string
	Placeholder string
	// Options are the values of a Choice.
	Options []string
	// Suggest completes the last word of a Text or Tokens field.
	Suggest func(prefix string) []string
	// Rows is the height of an Area.
	Rows int
	// Orig is the value the field is compared with to find changes.
	Orig string
	// Err is shown under the field.
	Err string

	line   *input.Field
	area   textarea.Model
	choice int
	ids    []string
}

// NewText returns a one-line field holding value; Orig starts as value.
func NewText(key, label, value string) *Field {
	f := &Field{Key: key, Label: label, Kind: Text, line: input.New(value)}
	f.Orig = f.Value()
	return f
}

// NewTokens returns a field of words.
func NewTokens(key, label string, words []string) *Field {
	f := &Field{Key: key, Label: label, Kind: Tokens, line: input.New(strings.Join(words, " "))}
	f.Orig = f.Value()
	return f
}

// NewArea returns a multi-line field of the given height.
func NewArea(key, label, value string, rows int) *Field {
	a := textarea.New()
	a.ShowLineNumbers = false
	a.Prompt = ""
	a.CharLimit = 0
	a.MaxHeight = 1000
	a.SetVirtualCursor(true)
	a.SetValue(value)
	a.MoveToBegin()
	f := &Field{Key: key, Label: label, Kind: Area, Rows: max(rows, 2), area: a}
	f.Orig = f.Value()
	return f
}

// NewChoice returns a field cycling over options, on value when it is one of
// them. A value outside the options is appended, so an unknown value is not
// lost by opening the form.
func NewChoice(key, label string, options []string, value string) *Field {
	f := &Field{Key: key, Label: label, Kind: Choice, Options: slices.Clone(options)}
	if i := slices.Index(f.Options, value); i >= 0 {
		f.choice = i
	} else if value != "" {
		f.Options = append(f.Options, value)
		f.choice = len(f.Options) - 1
	}
	f.Orig = f.Value()
	return f
}

// NewLinks returns a field of issue IDs.
func NewLinks(key, label string, ids []string) *Field {
	f := &Field{Key: key, Label: label, Kind: Links, ids: slices.Clone(ids)}
	f.Orig = f.Value()
	return f
}

// NewFold returns the row that opens the advanced fields.
func NewFold() *Field { return &Field{Key: "advanced", Kind: Fold} }

// Value is the field's value as one string: the text, the chosen option, or
// the words or IDs joined by single spaces.
func (f *Field) Value() string {
	switch f.Kind {
	case Text:
		return strings.TrimSpace(f.line.Text())
	case Tokens:
		return strings.Join(f.List(), " ")
	case Area:
		return f.area.Value()
	case Choice:
		if f.choice < len(f.Options) {
			return f.Options[f.choice]
		}
	case Links:
		return strings.Join(f.ids, " ")
	case Fold:
	}
	return ""
}

// List is the words of a Tokens field or the IDs of a Links field, without
// repeats.
func (f *Field) List() []string {
	switch f.Kind {
	case Links:
		return slices.Clone(f.ids)
	case Tokens:
		var out []string
		for _, w := range strings.FieldsFunc(f.line.Text(), func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
			if !slices.Contains(out, w) {
				out = append(out, w)
			}
		}
		return out
	case Text, Area, Choice, Fold:
	}
	return nil
}

// Set replaces the value; a list takes words or IDs separated like Tokens.
func (f *Field) Set(v string) {
	switch f.Kind {
	case Text, Tokens:
		f.line.Set(v)
	case Area:
		f.area.SetValue(v)
		f.area.MoveToBegin()
	case Choice:
		if i := slices.Index(f.Options, v); i >= 0 {
			f.choice = i
		}
	case Links:
		f.ids = nil
		for _, id := range strings.Fields(v) {
			if !slices.Contains(f.ids, id) {
				f.ids = append(f.ids, id)
			}
		}
	case Fold:
	}
}

// SetList replaces the IDs of a Links field.
func (f *Field) SetList(ids []string) { f.ids = slices.Clone(ids) }

// Rebase makes the current value the one changes are measured from.
func (f *Field) Rebase() { f.Orig = f.Value() }

// Empty reports whether the field holds nothing.
func (f *Field) Empty() bool { return f.Value() == "" }

// Changed reports whether the value differs from Orig; lists compare as sets.
func (f *Field) Changed() bool {
	if f.Kind == Fold {
		return false
	}
	return !Same(f.Kind, f.Value(), f.Orig)
}

// Same compares two values of a kind: lists as sets, text as it stands.
func Same(k Kind, a, b string) bool {
	if k != Tokens && k != Links {
		return a == b
	}
	x, y := strings.Fields(a), strings.Fields(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

// Add appends an ID to a Links field unless it is there.
func (f *Field) Add(id string) {
	if !slices.Contains(f.ids, id) {
		f.ids = append(f.ids, id)
	}
}

func (f *Field) focus(on bool) {
	if f.Kind != Area {
		return
	}
	if on {
		f.area.Focus()
	} else {
		f.area.Blur()
	}
}

// edit applies a key the form leaves to the field and reports whether the
// value changed, with the command the field's widget wants run.
func (f *Field) edit(k tea.KeyPressMsg) (bool, tea.Cmd) {
	switch f.Kind {
	case Text, Tokens:
		if k.String() == "right" && f.line.Pos() == len([]rune(f.line.Text())) && f.complete() {
			return true, nil
		}
		return f.line.Update(k), nil
	case Area:
		before := f.area.Value()
		var cmd tea.Cmd
		f.area, cmd = f.area.Update(k)
		return f.area.Value() != before, cmd
	case Choice:
		return f.cycle(k), nil
	case Links:
		if s := k.String(); (s == "backspace" || s == "delete") && len(f.ids) > 0 {
			f.ids = f.ids[:len(f.ids)-1]
			return true, nil
		}
	case Fold:
	}
	return false, nil
}

func (f *Field) cycle(k tea.KeyPressMsg) bool {
	n := len(f.Options)
	if n == 0 {
		return false
	}
	prev := f.choice
	switch s := k.String(); s {
	case "right", "l":
		f.choice = (f.choice + 1) % n
	case "left", "h":
		f.choice = (f.choice + n - 1) % n
	default:
		if k.Text == "" || k.Mod&(tea.ModCtrl|tea.ModAlt) != 0 {
			return false
		}
		for i := 1; i <= n; i++ {
			j := (f.choice + i) % n
			if strings.HasPrefix(strings.ToLower(f.Options[j]), strings.ToLower(k.Text)) {
				f.choice = j
				break
			}
		}
	}
	return f.choice != prev
}

func (f *Field) paste(s string) (bool, tea.Cmd) {
	switch f.Kind {
	case Text, Tokens:
		return f.line.Paste(s), nil
	case Area:
		before := f.area.Value()
		var cmd tea.Cmd
		f.area, cmd = f.area.Update(tea.PasteMsg{Content: s})
		return f.area.Value() != before, cmd
	case Choice, Links, Fold:
	}
	return false, nil
}

// lastWord is the word the cursor ends: what completion replaces.
func (f *Field) lastWord() (head, word string) {
	text := f.line.Text()
	if f.Kind == Text {
		return "", text
	}
	i := strings.LastIndexFunc(text, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) + 1
	return text[:i], text[i:]
}

// Suggestions lists the completions of the last word.
func (f *Field) Suggestions() []string {
	if f.Suggest == nil || (f.Kind != Text && f.Kind != Tokens) {
		return nil
	}
	_, word := f.lastWord()
	if f.Kind == Tokens && word == "" {
		return nil
	}
	out := f.Suggest(word)
	if f.Kind == Tokens {
		have := f.List()
		out = slices.DeleteFunc(slices.Clone(out), func(s string) bool { return slices.Contains(have, s) })
	}
	if len(out) == 1 && out[0] == word {
		return nil
	}
	return out
}

func (f *Field) complete() bool {
	s := f.Suggestions()
	if len(s) == 0 {
		return false
	}
	head, _ := f.lastWord()
	f.line.Set(head + s[0])
	return true
}
