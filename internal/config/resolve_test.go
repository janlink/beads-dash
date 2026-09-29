package config_test

import (
	"slices"
	"testing"

	"github.com/janlink/beads-dash/internal/config"
	"github.com/janlink/beads-dash/internal/model"
)

func loadedWith(t *testing.T, content string) config.Loaded {
	t.Helper()
	l := config.Load(writeConfig(t, content))
	if len(l.Warnings) != 0 {
		t.Fatalf("warnings: %v", l.Warnings)
	}
	return l
}

func TestResolveColourPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		env        map[string]string
		want       string
		wantOrigin config.Origin
	}{
		{"nothing", "", nil, "auto", config.Origin{}},
		{"config only", `color = "16"`, nil, "16", config.Origin{Kind: config.OriginConfig}},
		{"BDASH_COLOR beats config", `color = "16"`, map[string]string{"BDASH_COLOR": "256"}, "256", config.Origin{Kind: config.OriginEnv, Name: "BDASH_COLOR"}},
		{"NO_COLOR alone", "", map[string]string{"NO_COLOR": "1"}, "none", config.Origin{Kind: config.OriginEnv, Name: "NO_COLOR"}},
		{"NO_COLOR any non-empty value", "", map[string]string{"NO_COLOR": "false"}, "none", config.Origin{Kind: config.OriginEnv, Name: "NO_COLOR"}},
		{"empty NO_COLOR is unset", "", map[string]string{"NO_COLOR": ""}, "auto", config.Origin{}},
		{"config beats NO_COLOR", `color = "truecolor"`, map[string]string{"NO_COLOR": "1"}, "truecolor", config.Origin{Kind: config.OriginConfig}},
		{"config none with NO_COLOR", `color = "none"`, map[string]string{"NO_COLOR": "1"}, "none", config.Origin{Kind: config.OriginConfig}},
		{"explicit auto in config does not beat NO_COLOR", `color = "auto"`, map[string]string{"NO_COLOR": "1"}, "none", config.Origin{Kind: config.OriginEnv, Name: "NO_COLOR"}},
		{"BDASH_COLOR beats NO_COLOR", "", map[string]string{"BDASH_COLOR": "16", "NO_COLOR": "1"}, "16", config.Origin{Kind: config.OriginEnv, Name: "BDASH_COLOR"}},
		{"BDASH_COLOR=auto is explicit and beats NO_COLOR", "", map[string]string{"BDASH_COLOR": "auto", "NO_COLOR": "1"}, "auto", config.Origin{Kind: config.OriginEnv, Name: "BDASH_COLOR"}},
		{"case insensitive env", "", map[string]string{"BDASH_COLOR": "NONE"}, "none", config.Origin{Kind: config.OriginEnv, Name: "BDASH_COLOR"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := config.Resolve(loadedWith(t, tt.config), config.Flags{}, envOf(tt.env))
			if r.Settings.Color != tt.want {
				t.Errorf("color = %q, want %q", r.Settings.Color, tt.want)
			}
			if got := r.Origins[config.KeyColor]; got != tt.wantOrigin {
				t.Errorf("origin = %+v, want %+v", got, tt.wantOrigin)
			}
		})
	}
}

func TestResolveNoColorNeverCountsAsOverride(t *testing.T) {
	r := config.Resolve(loadedWith(t, ""), config.Flags{}, envOf(map[string]string{"NO_COLOR": "1"}))
	if note := r.OverrideNote(config.KeyColor); note != "" {
		t.Errorf("note = %q", note)
	}
}

func TestResolveGlyphsPrecedence(t *testing.T) {
	tests := []struct {
		name   string
		config string
		env    string
		want   string
		note   string
	}{
		{"default", "", "", "auto", ""},
		{"config", `glyphs = "safe"`, "", "safe", ""},
		{"env beats config", `glyphs = "safe"`, "ascii", "ascii", "overridden by BDASH_GLYPHS"},
		{"env alone", "", "fancy", "fancy", "overridden by BDASH_GLYPHS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := config.Resolve(loadedWith(t, tt.config), config.Flags{}, envOf(map[string]string{"BDASH_GLYPHS": tt.env}))
			if r.Settings.Glyphs != tt.want {
				t.Errorf("glyphs = %q, want %q", r.Settings.Glyphs, tt.want)
			}
			if got := r.OverrideNote(config.KeyGlyphs); got != tt.note {
				t.Errorf("note = %q, want %q", got, tt.note)
			}
		})
	}
}

func TestResolveFlagsBeatConfig(t *testing.T) {
	cfg := loadedWith(t, "view = \"tree\"\nmouse = true\n")
	r := config.Resolve(cfg, config.Flags{View: "kanban", NoMouse: true}, envOf(nil))
	if r.Settings.View != "kanban" || r.Settings.Mouse {
		t.Errorf("got %+v", r.Settings)
	}
	if got := r.OverrideNote(config.KeyView); got != "overridden by --view" {
		t.Errorf("view note %q", got)
	}
	if got := r.OverrideNote(config.KeyMouse); got != "overridden by --no-mouse" {
		t.Errorf("mouse note %q", got)
	}

	r = config.Resolve(cfg, config.Flags{}, envOf(nil))
	if r.Settings.View != "tree" || !r.Settings.Mouse || r.OverrideNote(config.KeyView) != "" {
		t.Errorf("without flags: %+v", r.Settings)
	}
}

func TestResolveInvalidEnvIsIgnoredWithWarning(t *testing.T) {
	r := config.Resolve(loadedWith(t, `glyphs = "safe"`), config.Flags{}, envOf(map[string]string{
		"BDASH_GLYPHS": "emoji", "BDASH_COLOR": "8", "BDASH_TIMEOUT_SCALE": "-2",
	}))
	if r.Settings.Glyphs != "safe" || r.Settings.Color != "auto" || r.TimeoutScale != 1 {
		t.Errorf("got %+v scale %v", r.Settings, r.TimeoutScale)
	}
	if len(r.Warnings) != 3 {
		t.Errorf("warnings = %v", r.Warnings)
	}
}

func TestResolveKeepsConfigWarningsAndOrigins(t *testing.T) {
	l := config.Load(writeConfig(t, "view = \"nope\"\ntheme = \"forest\"\n"))
	r := config.Resolve(l, config.Flags{}, envOf(nil))
	if len(r.Warnings) != 1 {
		t.Errorf("warnings = %v", r.Warnings)
	}
	if r.Origins[config.KeyTheme].Kind != config.OriginConfig {
		t.Errorf("theme origin = %+v", r.Origins[config.KeyTheme])
	}
	if _, ok := r.Origins[config.KeyView]; ok {
		t.Error("invalid view must have no origin")
	}
}

func TestResolveBdAndTimeoutScale(t *testing.T) {
	r := config.Resolve(loadedWith(t, ""), config.Flags{}, envOf(map[string]string{"BDASH_BD": "/opt/bd", "BDASH_TIMEOUT_SCALE": "2.5"}))
	if r.BdBinary != "/opt/bd" || r.TimeoutScale != 2.5 {
		t.Errorf("got %q %v", r.BdBinary, r.TimeoutScale)
	}
}

func TestNotifyKindsAreTheModelKinds(t *testing.T) {
	if !slices.Equal(config.NotifyKinds, model.KindNames()) {
		t.Errorf("NotifyKinds = %v, model kinds = %v", config.NotifyKinds, model.KindNames())
	}
	for _, k := range config.Defaults().NotifyKinds {
		if _, ok := model.ParseKind(k); !ok {
			t.Errorf("default notify kind %q is no model kind", k)
		}
	}
}
