package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// OriginKind says which layer decided a setting.
type OriginKind int

const (
	OriginDefault OriginKind = iota
	OriginConfig
	OriginEnv
	OriginFlag
)

// Origin names the layer that decided one setting, and the flag or variable
// when it was one of those.
type Origin struct {
	Kind OriginKind
	Name string
}

// Overrides reports whether the origin is a flag or an environment variable
// that beats the config file, so a dialog must say the choice is overridden.
// NO_COLOR ranks below config and never overrides it.
func (o Origin) Overrides() bool {
	return (o.Kind == OriginFlag || o.Kind == OriginEnv) && o.Name != EnvNoColor
}

// Flags are the command-line values that feed settings; zero values mean "not
// given".
type Flags struct {
	View    string
	NoMouse bool
}

// Resolved is the effective configuration after applying the precedence
// flag > env > config > default (colour: flag > BDASH_COLOR > config >
// NO_COLOR > auto, following no-color.org).
type Resolved struct {
	Settings Settings
	Origins  map[string]Origin
	Warnings []string

	// BdBinary is BDASH_BD, the path to the bd binary; empty means look up bd.
	BdBinary string
	// TimeoutScale is BDASH_TIMEOUT_SCALE, a positive factor on bd timeouts.
	TimeoutScale float64
}

// Override returns the origin of key when a flag or environment variable
// overrides what the config file says.
func (r Resolved) Override(key string) (Origin, bool) {
	o, ok := r.Origins[key]
	return o, ok && o.Overrides()
}

// OverrideNote renders "overridden by BDASH_GLYPHS" for dialogs, or "".
func (r Resolved) OverrideNote(key string) string {
	if o, ok := r.Override(key); ok {
		return "overridden by " + o.Name
	}
	return ""
}

// Resolve applies flags and environment on top of a loaded config.
func Resolve(cfg Loaded, flags Flags, getenv func(string) string) Resolved {
	r := Resolved{
		Settings:     cfg.Settings,
		Origins:      map[string]Origin{},
		Warnings:     slices.Clone(cfg.Warnings),
		TimeoutScale: 1,
	}
	for name := range cfg.Set {
		r.Origins[name] = Origin{Kind: OriginConfig}
	}

	env := func(key, envName string, allowed []string) {
		v := strings.ToLower(getenv(envName))
		if v == "" {
			return
		}
		if !slices.Contains(allowed, v) {
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s=%q ignored: want one of %s", envName, v, strings.Join(allowed, "|")))
			return
		}
		k, _ := lookupKey(key)
		k.set(&r.Settings, v)
		r.Origins[key] = Origin{Kind: OriginEnv, Name: envName}
	}
	env(KeyGlyphs, EnvGlyphs, GlyphTiers)
	env(KeyColor, EnvColor, Colors)

	explicit := r.Origins[KeyColor].Kind == OriginEnv || (cfg.Set[KeyColor] && r.Settings.Color != "auto")
	if !explicit && getenv(EnvNoColor) != "" {
		r.Settings.Color = "none"
		r.Origins[KeyColor] = Origin{Kind: OriginEnv, Name: EnvNoColor}
	}

	if flags.View != "" {
		r.Settings.View = flags.View
		r.Origins[KeyView] = Origin{Kind: OriginFlag, Name: "--view"}
	}
	if flags.NoMouse {
		r.Settings.Mouse = false
		r.Origins[KeyMouse] = Origin{Kind: OriginFlag, Name: "--no-mouse"}
	}

	r.BdBinary = getenv(EnvBd)
	if v := getenv(EnvTimeoutScale); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			r.TimeoutScale = f
		} else {
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s=%q ignored: want a positive number", EnvTimeoutScale, v))
		}
	}
	return r
}
