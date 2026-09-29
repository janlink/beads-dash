// Package config owns bdash's settings schema, config.toml, the state
// directory and the precedence between flags, environment, config and
// defaults.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Environment variable names.
const (
	EnvConfigDir    = "BDASH_CONFIG_DIR"
	EnvGlyphs       = "BDASH_GLYPHS"
	EnvColor        = "BDASH_COLOR"
	EnvNoColor      = "NO_COLOR"
	EnvTimeoutScale = "BDASH_TIMEOUT_SCALE"
	EnvBd           = "BDASH_BD"
	EnvBeadsDir     = "BEADS_DIR"
)

// Paths are the resolved locations of bdash's files.
type Paths struct {
	ConfigDir      string
	StateDir       string
	ConfigFile     string
	HistoryFile    string
	WorkspacesFile string
}

// ErrNoConfigDir is returned when neither a config nor a state directory can
// be derived from the environment.
var ErrNoConfigDir = errors.New("config: no config or state directory (set HOME, XDG_CONFIG_HOME or BDASH_CONFIG_DIR)")

// ResolvePaths computes the locations for one OS. goos, home and getenv are
// injected so every OS can be tested anywhere.
//
// Linux and macOS follow XDG (config $XDG_CONFIG_HOME/bdash or
// ~/.config/bdash, state $XDG_STATE_HOME/bdash or ~/.local/state/bdash);
// Windows uses %AppData%\bdash for config and %LocalAppData%\bdash for state.
// BDASH_CONFIG_DIR overrides the config directory only.
func ResolvePaths(getenv func(string) string, goos, home string) (Paths, error) {
	join := joiner(goos)
	var configDir, stateDir string
	if goos == "windows" {
		configDir = getenv("AppData")
		if configDir == "" && home != "" {
			configDir = join(home, "AppData", "Roaming")
		}
		stateDir = getenv("LocalAppData")
		if stateDir == "" && home != "" {
			stateDir = join(home, "AppData", "Local")
		}
	} else {
		configDir = xdgDir(getenv("XDG_CONFIG_HOME"), home, join, ".config")
		stateDir = xdgDir(getenv("XDG_STATE_HOME"), home, join, ".local", "state")
	}
	if configDir != "" {
		configDir = join(configDir, "bdash")
	}
	if stateDir != "" {
		stateDir = join(stateDir, "bdash")
	}
	if override := getenv(EnvConfigDir); override != "" {
		configDir = override
	}
	if configDir == "" || stateDir == "" {
		return Paths{}, ErrNoConfigDir
	}
	return Paths{
		ConfigDir:      configDir,
		StateDir:       stateDir,
		ConfigFile:     join(configDir, "config.toml"),
		HistoryFile:    join(stateDir, "history"),
		WorkspacesFile: join(stateDir, "workspaces.json"),
	}, nil
}

// SystemPaths resolves the paths for the running system.
func SystemPaths() (Paths, error) {
	home, _ := os.UserHomeDir()
	return ResolvePaths(os.Getenv, runtime.GOOS, home)
}

func xdgDir(env, home string, join func(...string) string, fallback ...string) string {
	if filepath.IsAbs(env) || strings.HasPrefix(env, "/") {
		return env
	}
	if home == "" {
		return ""
	}
	return join(append([]string{home}, fallback...)...)
}

func joiner(goos string) func(...string) string {
	sep := "/"
	if goos == "windows" {
		sep = `\`
	}
	return func(elems ...string) string {
		var out string
		for _, e := range elems {
			switch {
			case e == "":
			case out == "":
				out = e
			default:
				out = strings.TrimRight(out, sep) + sep + strings.TrimLeft(e, sep)
			}
		}
		return out
	}
}
