package form

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/look"
)

const labelWidth = 12

// Event is what a key press did to the form.
type Event int

const (
	// Nothing means the key changed nothing that matters to the owner.
	Nothing Event = iota
	// Edited means a value changed.
	Edited
	// Submit means Enter was pressed on the last field.
	Submit
	// Pick means Enter was pressed on a Links field: the owner opens the
	// picker for Focused.
	Pick
	// Moved means the focus moved.
	Moved
)

// Form is an ordered list of fields with one focus.
type Form struct {
	Fields []*Field
	// Open shows the advanced fields.
	Open bool
	// Track draws a bullet before every changed field.
	Track bool
	focus int
}

// New returns a form focused on its first field. The advanced fields open
// when any of them holds a value.
func New(fields ...*Field) *Form {
	f := &Form{Fields: fields}
	f.Open = f.AdvancedSet() > 0
	f.focus = f.first()
	f.Fields[f.focus].focus(true)
	return f
}

// Field returns the field with the key, or nil.
func (f *Form) Field(key string) *Field {
	for _, x := range f.Fields {
		if x.Key == key {
			return x
		}
	}
	return nil
}

// Focused is the field with the focus.
func (f *Form) Focused() *Field { return f.Fields[f.focus] }

// AdvancedSet counts the advanced fields that hold a value.
func (f *Form) AdvancedSet() int {
	n := 0
	for _, x := range f.Fields {
		if x.Advanced && !x.Skip && !x.Empty() {
			n++
		}
	}
	return n
}

func (f *Form) shown(x *Field) bool {
	return !x.Skip && (!x.Advanced || f.Open)
}

func (f *Form) visible() []int {
	var out []int
	for i, x := range f.Fields {
		if f.shown(x) {
			out = append(out, i)
		}
	}
	return out
}

func (f *Form) first() int {
	if v := f.visible(); len(v) > 0 {
		return v[0]
	}
	return 0
}

// Index is the position of the focus among the shown fields, and their count.
func (f *Form) Index() (at, of int) {
	v := f.visible()
	return slices.Index(v, f.focus) + 1, len(v)
}

// Changes counts the fields that differ from their original value.
func (f *Form) Changes() int {
	n := 0
	for _, x := range f.Fields {
		if !x.Skip && x.Changed() {
			n++
		}
	}
	return n
}

// FocusKey moves the focus to the field with the key, opening the advanced
// fields when it is one of them.
func (f *Form) FocusKey(key string) {
	for i, x := range f.Fields {
		if x.Key != key {
			continue
		}
		if x.Advanced {
			f.Open = true
		}
		f.setFocus(i)
		return
	}
}

func (f *Form) setFocus(i int) {
	f.Fields[f.focus].focus(false)
	f.focus = i
	f.Fields[f.focus].focus(true)
}

// Move moves the focus by n shown fields, wrapping.
func (f *Form) Move(n int) {
	v := f.visible()
	if len(v) == 0 {
		return
	}
	at := slices.Index(v, f.focus)
	if at < 0 {
		at = 0
	}
	f.setFocus(v[((at+n)%len(v)+len(v))%len(v)])
}

// Reveal opens the advanced fields if the focus would be hidden.
func (f *Form) Reveal() {
	if !f.shown(f.Fields[f.focus]) {
		f.setFocus(f.first())
	}
}

// Key applies a key press the owner's key map did not take. The command is
// what the focused field's widget wants run, such as a cursor blink.
func (f *Form) Key(k tea.KeyPressMsg) (Event, tea.Cmd) {
	cur := f.Focused()
	switch s := k.String(); s {
	case "up", "down":
		if cur.Kind != Area {
			f.Move(map[string]int{"up": -1, "down": 1}[s])
			return Moved, nil
		}
	case "enter":
		return f.enter(cur)
	}
	if cur.Kind == Fold {
		if s := k.String(); s == "space" || s == "right" || s == "left" {
			f.toggle()
		}
		return Nothing, nil
	}
	changed, cmd := cur.edit(k)
	return editEvent(changed), cmd
}

// enter acts on the focused field. On the fold row it toggles the advanced
// fields, open or closed alike; it never submits from there.
func (f *Form) enter(cur *Field) (Event, tea.Cmd) {
	switch cur.Kind {
	case Area:
		changed, cmd := cur.edit(tea.KeyPressMsg{Code: tea.KeyEnter})
		return editEvent(changed), cmd
	case Links:
		return Pick, nil
	case Fold:
		f.toggle()
		return Nothing, nil
	case Text, Choice, Tokens:
	}
	v := f.visible()
	if len(v) > 0 && v[len(v)-1] == f.focus {
		return Submit, nil
	}
	f.Move(1)
	return Moved, nil
}

func editEvent(changed bool) Event {
	if changed {
		return Edited
	}
	return Nothing
}

func (f *Form) toggle() {
	f.Open = !f.Open
	if !f.Open {
		f.Reveal()
	}
}

// Paste inserts pasted text into the focused field.
func (f *Form) Paste(s string) (Event, tea.Cmd) {
	changed, cmd := f.Focused().paste(s)
	return editEvent(changed), cmd
}

// Layout sizes and styles the multi-line fields for a form drawn in w cells.
// The owner calls it whenever the width or the look changes, before drawing
// and before keys reach the form: drawing itself changes nothing.
func (f *Form) Layout(l look.Look, w int) {
	for _, x := range f.Fields {
		if x.Kind == Area {
			x.area.SetStyles(areaStyles(l))
			x.area.SetWidth(max(w-2, 1))
			x.area.SetHeight(x.Rows)
		}
	}
}

// View draws the fields in w cells. It returns the lines and the span of the
// focused field in them.
func (f *Form) View(l look.Look, w int) (lines []string, from, to int) {
	for i, x := range f.Fields {
		if !f.shown(x) {
			continue
		}
		start := len(lines)
		lines = append(lines, f.draw(l, x, i == f.focus, w)...)
		if i == f.focus {
			from, to = start, len(lines)
		}
	}
	return lines, from, to
}

func (f *Form) draw(l look.Look, x *Field, focused bool, w int) []string {
	if x.Kind == Fold {
		return f.drawFold(l, focused)
	}
	mark := "  "
	if f.Track && x.Changed() {
		mark = l.Paint(theme.Changed, l.Glyphs.Bullet+" ")
	}
	label := x.Label
	if x.Required {
		label += "*"
	}
	labelRole := theme.Dim
	if focused {
		labelRole = theme.Primary
	}
	head := mark + l.Paint(labelRole, l.Fit(label, labelWidth)) + " "
	vw := max(w-2-labelWidth-1, 1)
	var out []string
	switch x.Kind {
	case Area:
		out = append(out, mark+l.Paint(labelRole, label)+l.Paint(theme.Faint, note(x.Note)))
		out = append(out, f.drawArea(l, x, focused, w-2)...)
	case Choice:
		out = append(out, head+f.drawChoice(l, x, focused, vw))
	case Links:
		out = append(out, head+f.drawLinks(l, x, focused, vw))
	case Text, Tokens:
		out = append(out, head+f.drawLine(l, x, focused, vw))
	case Fold:
	}
	pad := strings.Repeat(" ", 2+labelWidth+1)
	if focused {
		if s := x.Suggestions(); len(s) > 0 && x.Kind != Area {
			out = append(out, pad+l.Paint(theme.Faint, ansi.Truncate(l.Glyphs.Arrow+" "+strings.Join(s[:min(len(s), 6)], " · "), vw, l.Glyphs.Ellipsis)))
		}
	}
	if x.Err != "" {
		out = append(out, pad+l.Paint(theme.Error, ansi.Truncate(x.Err, vw, l.Glyphs.Ellipsis)))
	}
	return out
}

func note(s string) string {
	if s == "" {
		return ""
	}
	return "  " + s
}

func (f *Form) drawFold(l look.Look, focused bool) []string {
	arrow := strings.TrimSpace(l.Glyphs.FoldClosed)
	if f.Open {
		arrow = strings.TrimSpace(l.Glyphs.FoldOpen)
	}
	text := arrow + " Advanced"
	if n := f.AdvancedSet(); n > 0 && !f.Open {
		text += fmt.Sprintf(" (%d set)", n)
	}
	if focused {
		return []string{"  " + l.PaintSel(theme.Primary, text)}
	}
	return []string{"  " + l.Paint(theme.Dim, text)}
}

func (f *Form) drawLine(l look.Look, x *Field, focused bool, w int) string {
	tail := ""
	if x.Note != "" && !focused {
		tail = " " + x.Note
	}
	if x.Empty() && !focused && x.Placeholder != "" {
		return l.Paint(theme.Faint, l.Fit(x.Placeholder, w))
	}
	if focused && x.Note != "" {
		tail = " " + x.Note
	}
	room := w - ansi.StringWidth(tail)
	if room < 8 {
		tail, room = "", w
	}
	role := theme.Text
	if focused {
		role = theme.Strong
	}
	return x.line.View(l, role, room, focused) + l.Paint(theme.Faint, tail)
}

func (f *Form) drawChoice(l look.Look, x *Field, focused bool, w int) string {
	v := x.Value()
	text := v
	if focused {
		text = "< " + v + " >"
	}
	role := theme.Text
	if focused {
		role = theme.Strong
	}
	tail := ""
	if x.Note != "" {
		tail = "  " + x.Note
	}
	return l.Fit(l.Paint(role, text)+l.Paint(theme.Faint, tail), w)
}

func (f *Form) drawLinks(l look.Look, x *Field, focused bool, w int) string {
	text := strings.Join(x.ids, ", ")
	role := theme.Text
	if focused {
		role = theme.Strong
	}
	if text == "" {
		text = "-"
		role = theme.Faint
	}
	hint := ""
	if focused {
		hint = "  Enter picks"
	}
	return l.Fit(l.Paint(role, text)+l.Paint(theme.Faint, hint), w)
}

func (f *Form) drawArea(l look.Look, x *Field, focused bool, w int) []string {
	if x.Empty() && !focused && x.Placeholder != "" {
		return append([]string{"  " + l.Paint(theme.Faint, l.Fit(x.Placeholder, w))}, blank(x.Rows-1)...)
	}
	view := strings.Split(x.area.View(), "\n")
	out := make([]string, x.Rows)
	for i := range out {
		line := ""
		if i < len(view) {
			line = view[i]
		}
		out[i] = "  " + l.Fit(line, w)
	}
	return out
}

func blank(n int) []string { return make([]string, max(n, 0)) }

func areaStyles(l look.Look) textarea.Styles {
	text := l.Style(theme.Text)
	strong := l.Style(theme.Strong)
	base := lipgloss.NewStyle()
	focused := textarea.StyleState{Base: base, Text: strong, EndOfBuffer: text, Placeholder: l.Style(theme.Faint)}
	blurred := textarea.StyleState{Base: base, Text: text, EndOfBuffer: text, Placeholder: l.Style(theme.Faint)}
	return textarea.Styles{Focused: focused, Blurred: blurred, Cursor: textarea.CursorStyle{Shape: tea.CursorBlock}}
}

// Scroll returns the scroll offset that keeps the span from..to visible in a
// window of page lines over total lines, moving from prev as little as it
// can.
func Scroll(prev, from, to, page, total int) int {
	if total <= page {
		return 0
	}
	room := page - 1
	at := prev
	if from < at {
		at = from
	}
	if to > at+room {
		at = to - room
	}
	return min(max(at, 0), total-page)
}
