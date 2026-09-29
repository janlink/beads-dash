package dialog

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/theme"
	"github.com/janlink/beads-dash/internal/ui/keys"
	"github.com/janlink/beads-dash/internal/ui/look"
)

// Outcome is what a key did to the Appearance dialog.
type Outcome int

const (
	// Nothing means the dialog is unchanged.
	Nothing Outcome = iota
	// Preview means the choices changed and the view behind should follow.
	Preview
	// Apply means the choices are to be kept and persisted.
	Apply
	// Cancel means the dialog closed and the opening choices are to return.
	Cancel
)

// Choices are the settings the Appearance dialog edits, as written in the
// config file.
type Choices struct {
	Theme      string
	Background string
	Glyphs     string
}

// Changed lists the settings keys whose value differs from o, with the new
// value.
func (c Choices) Changed(o Choices) map[string]string {
	out := map[string]string{}
	if c.Theme != o.Theme {
		out[config.KeyTheme] = c.Theme
	}
	if c.Background != o.Background {
		out[config.KeyBackground] = c.Background
	}
	if c.Glyphs != o.Glyphs {
		out[config.KeyGlyphs] = c.Glyphs
	}
	return out
}

type row struct {
	label  string
	key    string
	values []string
	at     int
}

// Appearance is the model of the Appearance dialog: theme, background mode
// and glyph tier with a live preview. It does no I/O.
type Appearance struct {
	rows  []row
	sel   int
	orig  Choices
	notes map[string]string
}

// NewAppearance opens the dialog on the current choices. notes maps a settings
// key to the text shown beside its row when a flag or environment variable
// overrides it.
func NewAppearance(cur Choices, notes map[string]string) *Appearance {
	pick := func(values []string, v string) int { return max(slices.Index(values, v), 0) }
	themes := theme.Names()
	return &Appearance{
		orig:  cur,
		notes: notes,
		rows: []row{
			{"Theme", config.KeyTheme, themes, pick(themes, cur.Theme)},
			{"Background", config.KeyBackground, config.Backgrounds, pick(config.Backgrounds, cur.Background)},
			{"Glyphs", config.KeyGlyphs, config.GlyphTiers, pick(config.GlyphTiers, cur.Glyphs)},
		},
	}
}

// Choices is the current selection.
func (a *Appearance) Choices() Choices {
	return Choices{
		Theme:      a.rows[0].values[a.rows[0].at],
		Background: a.rows[1].values[a.rows[1].at],
		Glyphs:     a.rows[2].values[a.rows[2].at],
	}
}

// Original is what the dialog opened with.
func (a *Appearance) Original() Choices { return a.orig }

// Handle applies a key-map action.
func (a *Appearance) Handle(act keys.Action) Outcome {
	r := &a.rows[a.sel]
	switch act { //nolint:exhaustive // the action set is open: dialogs take the ones they know
	case keys.NavDown:
		a.sel = (a.sel + 1) % len(a.rows)
	case keys.NavUp:
		a.sel = (a.sel + len(a.rows) - 1) % len(a.rows)
	case keys.Next:
		r.at = (r.at + 1) % len(r.values)
		return Preview
	case keys.Prev:
		r.at = (r.at + len(r.values) - 1) % len(r.values)
		return Preview
	case keys.Apply:
		return Apply
	case keys.Close:
		return Cancel
	}
	return Nothing
}

// Frame draws the dialog with l, the look the preview is drawn in.
func (a *Appearance) Frame(l look.Look, hints []keys.Hint) Frame {
	labelW := 0
	for _, r := range a.rows {
		labelW = max(labelW, len(r.label))
	}
	var body []string
	for i, r := range a.rows {
		cursor, name := strings.Repeat(" ", ansi.StringWidth(l.Glyphs.FoldClosed)+1), l.Paint(theme.Dim, fmt.Sprintf("%-*s", labelW, r.label))
		value := l.Paint(theme.Text, r.values[r.at])
		if i == a.sel {
			cursor = l.Paint(theme.Primary, l.Glyphs.FoldClosed) + " "
			name = l.Paint(theme.Strong, fmt.Sprintf("%-*s", labelW, r.label))
			value = l.Paint(theme.Strong, "< "+r.values[r.at]+" >")
		} else {
			value = "  " + value
		}
		line := cursor + name + "  " + value
		if note := a.notes[r.key]; note != "" {
			line += "  " + l.Paint(theme.Warning, note)
		}
		body = append(body, line)
	}
	body = append(body, "", l.Paint(theme.Dim, "Preview"))
	body = append(body, previewLines(l)...)

	aside := ""
	if n := len(a.Choices().Changed(a.orig)); n > 0 {
		aside = fmt.Sprintf("%d changed", n)
	}
	return Frame{Title: "Appearance", Aside: aside, Hints: hints, Body: body, KeepBackdrop: true}
}

func previewLines(l look.Look) []string {
	g := l.Glyphs
	names := []string{"open", "in progress", "blocked", "frozen", "closed", "other"}
	var st []string
	for i, n := range names {
		st = append(st, l.Paint(theme.StatusRole(i), g.Status[i])+" "+l.Paint(theme.Text, n))
	}
	var pr []string
	for p := range 5 {
		pr = append(pr, l.Paint(theme.PriorityRole(p), fmt.Sprintf("P%d", p)))
	}
	sel := l.PaintSel(theme.Text, " ") + l.PaintSel(theme.Primary, g.Band) + l.PaintSel(theme.Strong, g.Mark) +
		l.PaintSel(theme.Changed, g.Change) + l.PaintSel(theme.Text, " selected row ")
	return []string{
		strings.Join(st[:3], "  "),
		strings.Join(st[3:], "  "),
		strings.Join(pr, " ") + "  " + l.Paint(theme.Match, "match") + "  " + l.Paint(theme.Error, "error") + "  " + l.Paint(theme.Success, "ok") + "  " + l.Paint(theme.Warning, "warn"),
		sel,
	}
}
