// Package input is the single-line text field of the docked bars and the
// pickers: editing keys, and a view that scrolls to keep the cursor visible.
package input

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// Field is a line of text with a cursor.
type Field struct {
	text []rune
	pos  int
}

// New returns a field holding s with the cursor at the end.
func New(s string) *Field {
	f := &Field{}
	f.Set(s)
	return f
}

// Text is the content.
func (f *Field) Text() string { return string(f.text) }

// Set replaces the content and moves the cursor to the end.
func (f *Field) Set(s string) {
	f.text = []rune(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s))
	f.pos = len(f.text)
}

// Pos is the cursor position in runes.
func (f *Field) Pos() int { return f.pos }

// Update applies a key press and reports whether the text changed. Keys the
// field does not know are ignored.
func (f *Field) Update(k tea.KeyPressMsg) bool {
	switch k.String() {
	case "left", "ctrl+b":
		f.pos = max(f.pos-1, 0)
	case "right", "ctrl+f":
		f.pos = min(f.pos+1, len(f.text))
	case "home", "ctrl+a":
		f.pos = 0
	case "end", "ctrl+e":
		f.pos = len(f.text)
	case "backspace", "ctrl+h":
		if f.pos > 0 {
			f.text = append(f.text[:f.pos-1], f.text[f.pos:]...)
			f.pos--
			return true
		}
	case "delete", "ctrl+d":
		if f.pos < len(f.text) {
			f.text = append(f.text[:f.pos], f.text[f.pos+1:]...)
			return true
		}
	case "ctrl+u":
		if f.pos > 0 {
			f.text = append([]rune(nil), f.text[f.pos:]...)
			f.pos = 0
			return true
		}
	case "ctrl+k":
		if f.pos < len(f.text) {
			f.text = f.text[:f.pos]
			return true
		}
	case "ctrl+w", "alt+backspace":
		return f.deleteWord()
	default:
		if k.Text != "" && k.Mod&(tea.ModCtrl|tea.ModAlt) == 0 {
			return f.insert(k.Text)
		}
	}
	return false
}

// Paste inserts pasted text at the cursor, turning line breaks into spaces,
// and reports whether the text changed.
func (f *Field) Paste(s string) bool {
	return f.insert(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(s))
}

func (f *Field) insert(s string) bool {
	in := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
	if len(in) == 0 {
		return false
	}
	tail := append([]rune(nil), f.text[f.pos:]...)
	f.text = append(append(f.text[:f.pos], in...), tail...)
	f.pos += len(in)
	return true
}

func (f *Field) deleteWord() bool {
	i := f.pos
	for i > 0 && unicode.IsSpace(f.text[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(f.text[i-1]) {
		i--
	}
	if i == f.pos {
		return false
	}
	f.text = append(f.text[:i], f.text[f.pos:]...)
	f.pos = i
	return true
}

// View draws the field in exactly w cells, scrolled so the cursor shows;
// the cursor cell is drawn as the selected style when cursor is set.
func (f *Field) View(l look.Look, role theme.Role, w int, cursor bool) string {
	if w <= 0 {
		return ""
	}
	start := 0
	for start < f.pos && runesWidth(f.text[start:f.pos]) >= w {
		start++
	}
	var before, at, after strings.Builder
	used := 0
	for i := start; i < len(f.text); i++ {
		rw := ansi.StringWidth(string(f.text[i]))
		if used+rw > w {
			break
		}
		switch {
		case i < f.pos:
			before.WriteRune(f.text[i])
		case i == f.pos && cursor:
			at.WriteRune(f.text[i])
		default:
			after.WriteRune(f.text[i])
		}
		used += rw
	}
	out := l.Paint(role, before.String())
	if cursor {
		c := at.String()
		if c == "" && used < w {
			c = " "
			used++
		}
		out += l.PaintSel(role, c)
	}
	out += l.Paint(role, after.String())
	if pad := w - used; pad > 0 {
		out += strings.Repeat(" ", pad)
	}
	return out
}

func runesWidth(r []rune) int { return ansi.StringWidth(string(r)) }
