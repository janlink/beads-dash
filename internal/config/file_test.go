package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/janlink/beads-dash/internal/config"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bdash", "config.toml")
	if content != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLoadMissingFileGivesDefaults(t *testing.T) {
	got := config.Load(writeConfig(t, ""))
	if got.Broken || len(got.Warnings) != 0 || len(got.Set) != 0 {
		t.Errorf("got %+v", got)
	}
	if !reflect.DeepEqual(got.Settings, config.Defaults()) {
		t.Errorf("settings differ from defaults: %+v", got.Settings)
	}
}

func TestLoadReadsEveryKey(t *testing.T) {
	path := writeConfig(t, `
view = "kanban"
mouse = false
theme = "Ocean"
background = "light"
color = "256"
glyphs = "safe"
ambiguous = "wide"
show_closed = true
highlight_seconds = 30

[detail]
docked = false

[export]
dir = "/tmp/out"

[notify]
method = "bell"
kinds = ["closed", "Priority", "closed"]

[refresh]
gate_polling = 3
gate_events = 11
safety_polling = 31
safety_events = 121
plain_poll = 6
unfocused_factor = 7
`)
	got := config.Load(path)
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings: %v", got.Warnings)
	}
	want := config.Settings{
		View: "kanban", Mouse: false, Theme: "ocean", Background: "light", Color: "256",
		Glyphs: "safe", Ambiguous: "wide", ShowClosed: true, HighlightSeconds: 30,
		DetailDocked: false, ExportDir: "/tmp/out",
		NotifyMethod: "bell", NotifyKinds: []string{"closed", "priority"},
		Refresh: config.Refresh{GatePolling: 3, GateEvents: 11, SafetyPolling: 31, SafetyEvents: 121, PlainPoll: 6, UnfocusedFactor: 7},
	}
	if !reflect.DeepEqual(got.Settings, want) {
		t.Errorf("got  %+v\nwant %+v", got.Settings, want)
	}
	if len(got.Set) != len(config.Keys()) {
		t.Errorf("Set has %d keys, want %d", len(got.Set), len(config.Keys()))
	}
}

func TestLoadFallsBackPerKey(t *testing.T) {
	path := writeConfig(t, `
view = "nope"
mouse = "yes"
theme = "forest"
highlight_seconds = 500
glyphs = 3
mystery = 1

[refresh]
plain_poll = 0
gate_events = 12
`)
	got := config.Load(path)
	if got.Broken {
		t.Fatal("type and value errors must not mark the file broken")
	}
	def := config.Defaults()
	if got.Settings.View != def.View || got.Settings.Mouse != def.Mouse || got.Settings.HighlightSeconds != def.HighlightSeconds ||
		got.Settings.Glyphs != def.Glyphs || got.Settings.Refresh.PlainPoll != def.Refresh.PlainPoll {
		t.Errorf("invalid keys did not fall back: %+v", got.Settings)
	}
	if got.Settings.Theme != "forest" || got.Settings.Refresh.GateEvents != 12 {
		t.Errorf("valid keys were lost: %+v", got.Settings)
	}
	joined := strings.Join(got.Warnings, "\n")
	for _, key := range []string{"view", "mouse", "highlight_seconds", "glyphs", "mystery", "refresh.plain_poll"} {
		if !strings.Contains(joined, key) {
			t.Errorf("no warning mentions %q:\n%s", key, joined)
		}
	}
	if got.Set["view"] || !got.Set["theme"] {
		t.Errorf("Set = %v", got.Set)
	}
}

func TestLoadSyntaxErrorIsBrokenAndDefault(t *testing.T) {
	path := writeConfig(t, "theme = \"ocean\"\nview = = broken\n")
	got := config.Load(path)
	if !got.Broken || len(got.Warnings) != 1 {
		t.Fatalf("got %+v", got)
	}
	if !reflect.DeepEqual(got.Settings, config.Defaults()) {
		t.Errorf("broken file must yield defaults: %+v", got.Settings)
	}
}

func TestStoreNeverWritesAfterSyntaxError(t *testing.T) {
	const broken = "view = = broken\n"
	path := writeConfig(t, broken)
	loaded := config.Load(path)
	store := config.NewStore(path, loaded.Broken)
	if err := store.Set(config.KeyTheme, "ocean"); !errors.Is(err, config.ErrReadOnly) {
		t.Errorf("err = %v, want ErrReadOnly", err)
	}
	if readFile(t, path) != broken {
		t.Error("file was modified")
	}
}

func TestStoreRefusesToOverwriteFileBrokenSinceLoad(t *testing.T) {
	path := writeConfig(t, "theme = \"ocean\"\n")
	store := config.NewStore(path, false)
	const broken = "theme = = \n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(config.KeyView, "tree"); !errors.Is(err, config.ErrReadOnly) {
		t.Errorf("err = %v, want ErrReadOnly", err)
	}
	if readFile(t, path) != broken {
		t.Error("file was modified")
	}
	if err := store.Set(config.KeyView, "tree"); !errors.Is(err, config.ErrReadOnly) {
		t.Errorf("second write err = %v, want ErrReadOnly (session stays read-only)", err)
	}
}

func TestStoreRoundTripKeepsUnknownKeys(t *testing.T) {
	path := writeConfig(t, `# my comment
theme = "ocean"
future_flag = true

[future]
level = 3
names = ["a", "b"]

[refresh]
plain_poll = 9
`)
	store := config.NewStore(path, false)
	if err := store.Set(config.KeyGlyphs, "safe"); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if _, err := toml.Decode(readFile(t, path), &raw); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"theme":       "ocean",
		"glyphs":      "safe",
		"future_flag": true,
		"future":      map[string]any{"level": int64(3), "names": []any{"a", "b"}},
		"refresh":     map[string]any{"plain_poll": int64(9)},
	}
	if !reflect.DeepEqual(raw, want) {
		t.Errorf("got  %#v\nwant %#v", raw, want)
	}
	if !strings.HasPrefix(readFile(t, path), "# written by bdash; comments are not preserved\n") {
		t.Errorf("missing header:\n%s", readFile(t, path))
	}
}

func TestStoreStoresNonDefaultsOnly(t *testing.T) {
	path := writeConfig(t, "")
	store := config.NewStore(path, false)
	if err := store.Set(config.KeyTheme, "sunset"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(config.KeyRefreshPlainPoll, 8); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(config.KeyNotifyKinds, []string{"closed", "blocked", "ready"}); err != nil {
		t.Fatal(err)
	}
	got := config.Load(path)
	if len(got.Set) != 2 || !got.Set["theme"] || !got.Set["refresh.plain_poll"] {
		t.Errorf("Set = %v; default-valued kinds must not be stored", got.Set)
	}

	if err := store.Set(config.KeyRefreshPlainPoll, config.Defaults().Refresh.PlainPoll); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(config.KeyTheme, "default"); err != nil {
		t.Fatal(err)
	}
	body := readFile(t, path)
	if strings.Contains(body, "theme") || strings.Contains(body, "refresh") {
		t.Errorf("default values must remove their keys and empty tables:\n%s", body)
	}
}

func TestStoreCreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "config.toml")
	if err := config.NewStore(path, false).Set(config.KeyMouse, false); err != nil {
		t.Fatal(err)
	}
	if got := config.Load(path); got.Settings.Mouse {
		t.Error("mouse=false not persisted")
	}
}

func TestStoreRejectsInvalidInput(t *testing.T) {
	path := writeConfig(t, "")
	store := config.NewStore(path, false)
	for name, v := range map[string]any{
		config.KeyTheme: "neon", config.KeyGlyphs: "emoji", config.KeyHighlightSeconds: 999,
		config.KeyMouse: "on", "nope": 1,
	} {
		if err := store.Set(name, v); err == nil {
			t.Errorf("Set(%s, %v) succeeded", name, v)
		}
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("invalid input created a file")
	}
}

func TestStoreLeavesNoTempFiles(t *testing.T) {
	path := writeConfig(t, "")
	store := config.NewStore(path, false)
	for _, theme := range []string{"ocean", "forest", "sunset"} {
		if err := store.Set(config.KeyTheme, theme); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %d entries, want only config.toml", len(entries))
	}
}
