// Package appearance turns the resolved settings and the terminal probes into
// the palette, glyph set and footer notice a session draws with. Colour depth,
// glyph tier and ambiguous width are fixed here at startup.
package appearance

import (
	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/term"
	"github.com/janlink/beads-dash/internal/theme"
)

// Prober runs the terminal probes selected by the options.
type Prober func(term.Options) term.Result

// Input is everything Resolve reads from the outside world.
type Input struct {
	Settings config.Settings
	Getenv   func(string) string
	// DetectDepth reports the terminal's colour depth; used for color = auto.
	DetectDepth func() theme.Depth
	// Probe queries the terminal; it is only called when a probe is needed.
	Probe Prober
}

// Appearance is the startup outcome.
type Appearance struct {
	Theme   theme.Theme
	Dark    bool
	Depth   theme.Depth
	Tier    theme.Tier
	Palette theme.Palette
	Glyphs  theme.Glyphs
	// Notice is the footer text when the glyph tier fell back to ascii.
	Notice string
	// TrackScheme asks for mode 2031 reports: the background mode is auto and a
	// terminal was opened, so the background is re-queried when the terminal's
	// scheme changes.
	TrackScheme bool
	// ProbedDark is the background found by the probe, dark when unknown.
	ProbedDark bool
	// Background is the background mode in force.
	Background theme.Background
	// AmbiguousWide is set when ambiguous-width glyphs are drawn wide, which
	// keeps the glyph tier at ascii.
	AmbiguousWide bool
}

// Resolve computes the appearance. Unknown probe answers fall back to a dark
// background and narrow ambiguous glyphs.
func Resolve(in Input) Appearance {
	s := in.Settings
	th, ok := theme.Lookup(s.Theme)
	if !ok {
		th = theme.Default()
	}
	bgMode, _ := theme.ParseBackground(s.Background)
	tier, _ := theme.ParseTier(s.Glyphs)
	tier = theme.ResolveTier(tier, in.Getenv)

	needBackground := bgMode == theme.BackgroundAuto
	needAmbiguous := tier != theme.TierASCII && s.Ambiguous == "auto"
	res := term.Result{Dark: true}
	if (needBackground || needAmbiguous) && in.Probe != nil {
		res = in.Probe(term.Options{Background: needBackground, Ambiguous: needAmbiguous})
	}

	wide := res.AmbiguousWide
	switch s.Ambiguous {
	case "wide":
		wide = true
	case "narrow":
		wide = false
	}
	requested := tier
	tier, notice := theme.FallbackForAmbiguous(tier, wide && tier != theme.TierASCII)

	depth, _ := theme.ParseDepth(s.Color)
	if depth == theme.DepthAuto && in.DetectDepth != nil {
		depth = in.DetectDepth()
	}
	if depth == theme.DepthAuto {
		depth = theme.DepthTrueColor
	}

	dark := bgMode.IsDark(res.Dark)
	return Appearance{
		Theme:         th,
		Dark:          dark,
		Depth:         depth,
		Tier:          tier,
		Palette:       theme.NewPalette(th, dark, depth),
		Glyphs:        theme.GlyphsFor(tier),
		Notice:        notice,
		TrackScheme:   needBackground && res.Interactive,
		ProbedDark:    res.Dark,
		Background:    bgMode,
		AmbiguousWide: wide && requested != theme.TierASCII,
	}
}

// Preview re-resolves the appearance for other choices without probing the
// terminal again: theme name, background mode (auto follows the probed
// background) and glyph tier (auto|fancy|safe|ascii). Unknown values leave that
// part unchanged. The colour depth and the ambiguous-width verdict stay.
func (a Appearance) Preview(getenv func(string) string, themeName, background, glyphs string) Appearance {
	if th, ok := theme.Lookup(themeName); ok {
		a.Theme = th
	}
	if bg, ok := theme.ParseBackground(background); ok {
		a.Background = bg
		a.Dark = bg.IsDark(a.ProbedDark)
	}
	if tier, ok := theme.ParseTier(glyphs); ok {
		tier = theme.ResolveTier(tier, getenv)
		tier, a.Notice = theme.FallbackForAmbiguous(tier, a.AmbiguousWide)
		a.Tier = tier
		a.Glyphs = theme.GlyphsFor(tier)
	}
	a.Palette = theme.NewPalette(a.Theme, a.Dark, a.Depth)
	return a
}

// WithBackground returns the appearance re-resolved for a freshly probed
// background, e.g. after a mode 2031 report and an OSC 11 re-query. It only has
// an effect while the background mode is auto.
func (a Appearance) WithBackground(dark bool) Appearance {
	if !a.TrackScheme {
		return a
	}
	a.ProbedDark = dark
	if a.Background != theme.BackgroundAuto {
		return a
	}
	a.Dark = dark
	a.Palette = theme.NewPalette(a.Theme, dark, a.Depth)
	return a
}

// Update handles the messages that change the background. A mode 2031 report
// returns a command asking for the terminal background; the answer re-resolves
// the palette. Any other message, or a fixed background mode, leaves the
// appearance alone.
func (a Appearance) Update(msg tea.Msg) (Appearance, tea.Cmd) {
	if !a.TrackScheme {
		return a, nil
	}
	switch m := msg.(type) {
	case tea.BackgroundColorMsg:
		return a.WithBackground(m.IsDark()), nil
	default:
		if term.IsColorSchemeEvent(msg) {
			return a, tea.RequestBackgroundColor
		}
	}
	return a, nil
}
