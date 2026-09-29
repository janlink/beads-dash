package appearance_test

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/janlink/beads-dash/internal/appearance"
	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/term"
	"github.com/janlink/beads-dash/internal/theme"
)

type probeLog struct {
	calls []term.Options
	res   term.Result
}

func (p *probeLog) probe(o term.Options) term.Result {
	p.calls = append(p.calls, o)
	return p.res
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func input(s config.Settings, e map[string]string, p *probeLog) appearance.Input {
	return appearance.Input{
		Settings:    s,
		Getenv:      env(e),
		DetectDepth: func() theme.Depth { return theme.Depth256 },
		Probe:       p.probe,
	}
}

func settings(mod func(*config.Settings)) config.Settings {
	s := config.Defaults()
	if mod != nil {
		mod(&s)
	}
	return s
}

func TestResolveDefaultsProbeBothAndUseAnswers(t *testing.T) {
	p := &probeLog{res: term.Result{Dark: false, BackgroundKnown: true, AmbiguousKnown: true, Interactive: true}}
	a := appearance.Resolve(input(settings(nil), nil, p))
	if len(p.calls) != 1 || !p.calls[0].Background || !p.calls[0].Ambiguous {
		t.Fatalf("probe calls = %+v", p.calls)
	}
	if a.Dark || a.Tier != theme.TierFancy || a.Depth != theme.Depth256 || !a.TrackScheme || a.Notice != "" {
		t.Errorf("got %+v", a)
	}
	if a.Palette.Dark() || a.Palette.Depth() != theme.Depth256 {
		t.Error("palette does not follow the resolved background and depth")
	}
}

func TestResolveExplicitChoicesSkipProbes(t *testing.T) {
	p := &probeLog{res: term.Result{Dark: false}}
	a := appearance.Resolve(input(settings(func(s *config.Settings) {
		s.Background = "dark"
		s.Glyphs = "ascii"
		s.Color = "16"
	}), nil, p))
	if len(p.calls) != 0 {
		t.Errorf("unexpected probe: %+v", p.calls)
	}
	if !a.Dark || a.Tier != theme.TierASCII || a.Depth != theme.Depth16 || a.TrackScheme {
		t.Errorf("got %+v", a)
	}
}

func TestResolveProbesOnlyWhatIsNeeded(t *testing.T) {
	p := &probeLog{}
	appearance.Resolve(input(settings(func(s *config.Settings) { s.Background = "light" }), nil, p))
	if len(p.calls) != 1 || p.calls[0].Background || !p.calls[0].Ambiguous {
		t.Errorf("calls = %+v, want ambiguous only", p.calls)
	}

	p = &probeLog{}
	appearance.Resolve(input(settings(func(s *config.Settings) { s.Ambiguous = "narrow" }), nil, p))
	if len(p.calls) != 1 || !p.calls[0].Background || p.calls[0].Ambiguous {
		t.Errorf("calls = %+v, want background only", p.calls)
	}
}

func TestResolveWideAmbiguousFallsBackToASCIIWithNotice(t *testing.T) {
	p := &probeLog{res: term.Result{Dark: true, AmbiguousWide: true, AmbiguousKnown: true}}
	a := appearance.Resolve(input(settings(nil), nil, p))
	if a.Tier != theme.TierASCII || a.Notice != theme.AmbiguousNotice {
		t.Errorf("got tier %s notice %q", a.Tier, a.Notice)
	}
	if a.Glyphs.Tier != theme.TierASCII {
		t.Error("glyph set does not follow the fallback")
	}
}

func TestResolveAmbiguousOverride(t *testing.T) {
	wide := &probeLog{res: term.Result{Dark: true}}
	a := appearance.Resolve(input(settings(func(s *config.Settings) { s.Ambiguous = "wide" }), nil, wide))
	if a.Tier != theme.TierASCII || a.Notice == "" {
		t.Errorf("ambiguous=wide: tier %s notice %q", a.Tier, a.Notice)
	}

	narrow := &probeLog{res: term.Result{Dark: true, AmbiguousWide: true}}
	a = appearance.Resolve(input(settings(func(s *config.Settings) { s.Ambiguous = "narrow" }), nil, narrow))
	if a.Tier != theme.TierFancy || a.Notice != "" {
		t.Errorf("ambiguous=narrow: tier %s notice %q", a.Tier, a.Notice)
	}
}

func TestResolveASCIITierNeverProbesWidthOrShowsNotice(t *testing.T) {
	p := &probeLog{res: term.Result{Dark: true, AmbiguousWide: true}}
	a := appearance.Resolve(input(settings(func(s *config.Settings) { s.Glyphs = "ascii"; s.Ambiguous = "wide" }), nil, p))
	if a.Notice != "" || a.Tier != theme.TierASCII {
		t.Errorf("got tier %s notice %q", a.Tier, a.Notice)
	}
}

func TestResolveLocaleDrivesAutoTier(t *testing.T) {
	p := &probeLog{}
	a := appearance.Resolve(input(settings(nil), map[string]string{"LANG": "C"}, p))
	if a.Tier != theme.TierASCII {
		t.Errorf("tier = %s, want ascii for C locale", a.Tier)
	}
	if len(p.calls) != 1 || p.calls[0].Ambiguous {
		t.Errorf("ascii by locale must not probe width: %+v", p.calls)
	}
}

func TestResolveSilentTerminalFallsBackDarkNarrow(t *testing.T) {
	p := &probeLog{res: term.Result{Dark: true}}
	a := appearance.Resolve(input(settings(nil), nil, p))
	if !a.Dark || a.Tier != theme.TierFancy {
		t.Errorf("got %+v", a)
	}
}

func TestResolveColourDepth(t *testing.T) {
	tests := []struct {
		color string
		want  theme.Depth
	}{
		{"auto", theme.Depth256}, {"truecolor", theme.DepthTrueColor}, {"256", theme.Depth256}, {"16", theme.Depth16}, {"none", theme.DepthNone},
	}
	for _, tt := range tests {
		a := appearance.Resolve(input(settings(func(s *config.Settings) {
			s.Color = tt.color
			s.Background = "dark"
			s.Glyphs = "ascii"
		}), nil, &probeLog{}))
		if a.Depth != tt.want {
			t.Errorf("color %s: depth %s, want %s", tt.color, a.Depth, tt.want)
		}
	}
}

func TestWithBackgroundOnlyFollowsInAuto(t *testing.T) {
	auto := appearance.Resolve(input(settings(nil), nil, &probeLog{res: term.Result{Dark: true, Interactive: true}}))
	if got := auto.WithBackground(false); got.Dark || got.Palette.Dark() {
		t.Error("auto mode must follow the re-queried background")
	}
	fixed := appearance.Resolve(input(settings(func(s *config.Settings) { s.Background = "dark" }), nil, &probeLog{}))
	if got := fixed.WithBackground(false); !got.Dark || !got.Palette.Dark() {
		t.Error("explicit background must not change")
	}
}

func TestResolveUnknownThemeFallsBackToDefault(t *testing.T) {
	a := appearance.Resolve(input(settings(func(s *config.Settings) { s.Theme = "gone" }), nil, &probeLog{}))
	if a.Theme.Name != theme.DefaultName {
		t.Errorf("theme = %s", a.Theme.Name)
	}
}

func TestNoTerminalNoSchemeTracking(t *testing.T) {
	p := &probeLog{res: term.Result{Dark: true}}
	if a := appearance.Resolve(input(settings(nil), nil, p)); a.TrackScheme {
		t.Error("scheme tracking needs an opened terminal")
	}
}

func TestUpdateSchemeEventRequestsBackgroundThenApplies(t *testing.T) {
	p := &probeLog{res: term.Result{Dark: true, Interactive: true}}
	a := appearance.Resolve(input(settings(nil), nil, p))

	got, cmd := a.Update(uv.LightColorSchemeEvent{})
	if cmd == nil || !got.Dark {
		t.Fatalf("scheme event: cmd %v dark %v; want a query and no change yet", cmd, got.Dark)
	}
	if cmd() == nil {
		t.Error("command must yield a message")
	}

	got, cmd = a.Update(tea.BackgroundColorMsg{Color: color.White})
	if cmd != nil || got.Dark || got.Palette.Dark() {
		t.Errorf("light background: dark %v palette dark %v cmd %v", got.Dark, got.Palette.Dark(), cmd)
	}

	if _, cmd = a.Update(tea.KeyPressMsg{}); cmd != nil {
		t.Error("unrelated message must be ignored")
	}

	fixed := appearance.Resolve(input(settings(func(s *config.Settings) { s.Background = "dark" }), nil, p))
	if got, cmd = fixed.Update(uv.LightColorSchemeEvent{}); cmd != nil || !got.Dark {
		t.Error("fixed background must ignore scheme events")
	}
}

func TestPreviewChangesChoicesWithoutProbing(t *testing.T) {
	p := &probeLog{res: term.Result{Dark: false, Interactive: true}}
	a := appearance.Resolve(input(settings(nil), nil, p))
	calls := len(p.calls)

	b := a.Preview(env(nil), "ocean", "dark", "ascii")
	if len(p.calls) != calls {
		t.Error("Preview probed the terminal")
	}
	if b.Theme.Name != "ocean" || !b.Dark || b.Tier != theme.TierASCII || b.Glyphs.Tier != theme.TierASCII || !b.Palette.Dark() {
		t.Errorf("preview = %+v", b)
	}
	if b.Depth != a.Depth {
		t.Error("Preview changed the colour depth")
	}
	if c := b.Preview(env(nil), "nope", "auto", "nope"); c.Theme.Name != "ocean" || c.Dark || c.Tier != theme.TierASCII {
		t.Errorf("unknown theme/glyphs must leave that part alone, auto follows the probe: %+v", c)
	}
}

func TestPreviewKeepsAmbiguousFallback(t *testing.T) {
	p := &probeLog{res: term.Result{AmbiguousWide: true, AmbiguousKnown: true, Dark: true}}
	a := appearance.Resolve(input(settings(nil), nil, p))
	if a.Tier != theme.TierASCII || a.Notice == "" {
		t.Fatalf("setup: %+v", a)
	}
	if b := a.Preview(env(nil), "default", "auto", "fancy"); b.Tier != theme.TierASCII || b.Notice == "" {
		t.Errorf("fancy on a wide terminal = %v %q, want the ascii fallback", b.Tier, b.Notice)
	}
}

func TestFixedBackgroundIgnoresSchemeReports(t *testing.T) {
	p := &probeLog{res: term.Result{Dark: true, Interactive: true}}
	a := appearance.Resolve(input(settings(nil), nil, p)).Preview(env(nil), "default", "dark", "auto")
	b, _ := a.Update(tea.BackgroundColorMsg{Color: color.White})
	if !b.Dark {
		t.Error("a fixed dark background followed the terminal")
	}
	if b.ProbedDark {
		t.Error("the probed background was not remembered")
	}
	if c := b.Preview(env(nil), "default", "auto", "auto"); c.Dark {
		t.Error("switching back to auto ignored the remembered background")
	}
}
