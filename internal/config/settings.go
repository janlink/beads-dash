package config

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
)

// Views lists the values of the view setting and the --view flag.
var Views = []string{"overview", "tree", "kanban", "ready", "memories", "graph"}

// Refresh holds the refresh tunables as plain seconds (and a factor). What
// they mean is defined by the refresh engine.
type Refresh struct {
	GatePolling     int
	GateEvents      int
	SafetyPolling   int
	SafetyEvents    int
	PlainPoll       int
	UnfocusedFactor int
}

// Settings is the consolidated settings schema.
type Settings struct {
	View             string
	Mouse            bool
	Theme            string
	Background       string
	Color            string
	Glyphs           string
	Ambiguous        string
	ShowClosed       bool
	HighlightSeconds int
	DetailDocked     bool
	ExportDir        string
	NotifyMethod     string
	NotifyKinds      []string
	Refresh          Refresh
}

// Defaults returns the default settings.
func Defaults() Settings {
	return Settings{
		View:             "overview",
		Mouse:            true,
		Theme:            theme.DefaultName,
		Background:       "auto",
		Color:            "auto",
		Glyphs:           "auto",
		Ambiguous:        "auto",
		HighlightSeconds: 10,
		DetailDocked:     true,
		NotifyMethod:     "auto",
		NotifyKinds:      []string{"closed", "blocked", "ready"},
		Refresh: Refresh{
			GatePolling:     2,
			GateEvents:      10,
			SafetyPolling:   30,
			SafetyEvents:    120,
			PlainPoll:       5,
			UnfocusedFactor: 5,
		},
	}
}

// Key names of the settings schema, as written in config.toml.
const (
	KeyView                   = "view"
	KeyMouse                  = "mouse"
	KeyTheme                  = "theme"
	KeyBackground             = "background"
	KeyColor                  = "color"
	KeyGlyphs                 = "glyphs"
	KeyAmbiguous              = "ambiguous"
	KeyShowClosed             = "show_closed"
	KeyHighlightSeconds       = "highlight_seconds"
	KeyDetailDocked           = "detail.docked"
	KeyExportDir              = "export.dir"
	KeyNotifyMethod           = "notify.method"
	KeyNotifyKinds            = "notify.kinds"
	KeyRefreshGatePolling     = "refresh.gate_polling"
	KeyRefreshGateEvents      = "refresh.gate_events"
	KeyRefreshSafetyPolling   = "refresh.safety_polling"
	KeyRefreshSafetyEvents    = "refresh.safety_events"
	KeyRefreshPlainPoll       = "refresh.plain_poll"
	KeyRefreshUnfocusedFactor = "refresh.unfocused_factor"
	maxHighlightSeconds       = 120
	maxRefreshSeconds         = 3600
	maxRefreshUnfocusedFactor = 100
)

// keyDef describes one settings key: how to validate a raw value and where it
// lives in Settings.
type keyDef struct {
	name  string
	parse func(raw any) (any, error)
	ptr   func(*Settings) any
}

func (k keyDef) get(s *Settings) any { return reflect.ValueOf(k.ptr(s)).Elem().Interface() }

func (k keyDef) set(s *Settings, v any) { reflect.ValueOf(k.ptr(s)).Elem().Set(reflect.ValueOf(v)) }

func enumKey(name string, allowed []string, f func(*Settings) *string) keyDef {
	return keyDef{
		name: name,
		ptr:  func(s *Settings) any { return f(s) },
		parse: func(raw any) (any, error) {
			str, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("want one of %s", strings.Join(allowed, "|"))
			}
			str = strings.ToLower(str)
			if !slices.Contains(allowed, str) {
				return nil, fmt.Errorf("%q is not one of %s", str, strings.Join(allowed, "|"))
			}
			return str, nil
		},
	}
}

func boolKey(name string, f func(*Settings) *bool) keyDef {
	return keyDef{
		name: name,
		ptr:  func(s *Settings) any { return f(s) },
		parse: func(raw any) (any, error) {
			b, ok := raw.(bool)
			if !ok {
				return nil, fmt.Errorf("want true or false")
			}
			return b, nil
		},
	}
}

func intKey(name string, lo, hi int, f func(*Settings) *int) keyDef {
	return keyDef{
		name: name,
		ptr:  func(s *Settings) any { return f(s) },
		parse: func(raw any) (any, error) {
			var n int64
			switch v := raw.(type) {
			case int64:
				n = v
			case int:
				n = int64(v)
			default:
				return nil, fmt.Errorf("want a whole number %d..%d", lo, hi)
			}
			if n < int64(lo) || n > int64(hi) {
				return nil, fmt.Errorf("%d is outside %d..%d", n, lo, hi)
			}
			return int(n), nil
		},
	}
}

func stringKey(name string, f func(*Settings) *string) keyDef {
	return keyDef{
		name: name,
		ptr:  func(s *Settings) any { return f(s) },
		parse: func(raw any) (any, error) {
			str, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("want a string")
			}
			return str, nil
		},
	}
}

// NotifyKinds lists the activity-event kinds notifications can be enabled
// for: every kind of the model.
var NotifyKinds = model.KindNames()

func stringsKey(name string, allowed []string, f func(*Settings) *[]string) keyDef {
	return keyDef{
		name: name,
		ptr:  func(s *Settings) any { return f(s) },
		parse: func(raw any) (any, error) {
			var items []any
			switch v := raw.(type) {
			case []any:
				items = v
			case []string:
				for _, e := range v {
					items = append(items, e)
				}
			default:
				return nil, fmt.Errorf("want a list of strings")
			}
			out := make([]string, 0, len(items))
			for _, it := range items {
				str, ok := it.(string)
				if !ok {
					return nil, fmt.Errorf("want a list of strings from %s", strings.Join(allowed, "|"))
				}
				str = strings.ToLower(strings.TrimSpace(str))
				if !slices.Contains(allowed, str) {
					return nil, fmt.Errorf("%q is not one of %s", str, strings.Join(allowed, "|"))
				}
				if !slices.Contains(out, str) {
					out = append(out, str)
				}
			}
			return out, nil
		},
	}
}

func themeKey() keyDef {
	k := stringKey(KeyTheme, func(s *Settings) *string { return &s.Theme })
	k.parse = func(raw any) (any, error) {
		str, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("want one of %s", strings.Join(theme.Names(), "|"))
		}
		t, found := theme.Lookup(str)
		if !found {
			return nil, fmt.Errorf("%q is not one of %s", str, strings.Join(theme.Names(), "|"))
		}
		return t.Name, nil
	}
	return k
}

// Allowed enum values, shared with flag and environment parsing.
var (
	Backgrounds = []string{"auto", "dark", "light"}
	Colors      = []string{"auto", "truecolor", "256", "16", "none"}
	GlyphTiers  = []string{"auto", "fancy", "safe", "ascii"}
	Ambiguities = []string{"auto", "narrow", "wide"}
	NotifyModes = []string{"auto", "desktop", "terminal", "bell", "off"}
)

var schema = []keyDef{
	enumKey(KeyView, Views, func(s *Settings) *string { return &s.View }),
	boolKey(KeyMouse, func(s *Settings) *bool { return &s.Mouse }),
	themeKey(),
	enumKey(KeyBackground, Backgrounds, func(s *Settings) *string { return &s.Background }),
	enumKey(KeyColor, Colors, func(s *Settings) *string { return &s.Color }),
	enumKey(KeyGlyphs, GlyphTiers, func(s *Settings) *string { return &s.Glyphs }),
	enumKey(KeyAmbiguous, Ambiguities, func(s *Settings) *string { return &s.Ambiguous }),
	boolKey(KeyShowClosed, func(s *Settings) *bool { return &s.ShowClosed }),
	intKey(KeyHighlightSeconds, 0, maxHighlightSeconds, func(s *Settings) *int { return &s.HighlightSeconds }),
	boolKey(KeyDetailDocked, func(s *Settings) *bool { return &s.DetailDocked }),
	stringKey(KeyExportDir, func(s *Settings) *string { return &s.ExportDir }),
	enumKey(KeyNotifyMethod, NotifyModes, func(s *Settings) *string { return &s.NotifyMethod }),
	stringsKey(KeyNotifyKinds, NotifyKinds, func(s *Settings) *[]string { return &s.NotifyKinds }),
	intKey(KeyRefreshGatePolling, 1, maxRefreshSeconds, func(s *Settings) *int { return &s.Refresh.GatePolling }),
	intKey(KeyRefreshGateEvents, 1, maxRefreshSeconds, func(s *Settings) *int { return &s.Refresh.GateEvents }),
	intKey(KeyRefreshSafetyPolling, 1, maxRefreshSeconds, func(s *Settings) *int { return &s.Refresh.SafetyPolling }),
	intKey(KeyRefreshSafetyEvents, 1, maxRefreshSeconds, func(s *Settings) *int { return &s.Refresh.SafetyEvents }),
	intKey(KeyRefreshPlainPoll, 1, maxRefreshSeconds, func(s *Settings) *int { return &s.Refresh.PlainPoll }),
	intKey(KeyRefreshUnfocusedFactor, 1, maxRefreshUnfocusedFactor, func(s *Settings) *int { return &s.Refresh.UnfocusedFactor }),
}

func lookupKey(name string) (keyDef, bool) {
	for _, k := range schema {
		if k.name == name {
			return k, true
		}
	}
	return keyDef{}, false
}

// Keys lists every settings key in schema order.
func Keys() []string {
	out := make([]string, len(schema))
	for i, k := range schema {
		out[i] = k.name
	}
	return out
}
