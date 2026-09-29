package cli

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/janlink/beads-dash/internal/config"
)

// BuildInfo identifies this bdash build.
type BuildInfo struct {
	Version   string
	Commit    string
	GoVersion string
}

// NewBuildInfo fills the gaps of the linker-provided version and commit from
// the Go build info and the running toolchain.
func NewBuildInfo(version, commit string) BuildInfo {
	b := BuildInfo{Version: version, Commit: commit, GoVersion: runtime.Version()}
	if b.Version == "" {
		b.Version = "dev"
	}
	if b.Commit == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					b.Commit = s.Value
				}
			}
		}
	}
	return b
}

func (b BuildInfo) String() string {
	commit := b.Commit
	if len(commit) > 7 {
		commit = commit[:7]
	}
	if commit == "" {
		return fmt.Sprintf("bdash %s (%s)", b.Version, b.GoVersion)
	}
	return fmt.Sprintf("bdash %s (commit %s, %s)", b.Version, commit, b.GoVersion)
}

// BdVersionFunc describes the bd binary bdash would use and whether it is
// supported, e.g. "bd 1.2.2 at /usr/bin/bd: supported". An empty result omits
// the line. The bd boundary provides it; it may be nil.
type BdVersionFunc func(ctx context.Context) string

// VersionText renders --version output: the bdash build and, when available,
// the bd support line.
func VersionText(ctx context.Context, b BuildInfo, bd BdVersionFunc) string {
	out := b.String() + "\n"
	if bd != nil {
		if line := bd(ctx); line != "" {
			out += line + "\n"
		}
	}
	return out
}

// UsageHint is printed after a usage error.
const UsageHint = "Try 'bdash --help' for usage."

// HelpText renders --help: usage, flags, resolved file locations and every
// environment variable bdash reads. pathsErr is set when no location could be
// resolved.
func HelpText(p config.Paths, pathsErr error) string {
	var b strings.Builder
	b.WriteString(`Usage: bdash [flags] [path]

Terminal dashboard for the bd (beads) issue tracker.
[path] is the workspace directory bd runs from (default: current directory).

Flags:
  --view <name>   start view: ` + strings.Join(config.Views, "|") + `
  --no-mouse      turn mouse support off
  -v, --version   print the bdash version and the bd support check, then exit
  -h, --help      print this help, then exit

Files:
`)
	if pathsErr != nil {
		fmt.Fprintf(&b, "  unavailable: %v\n", pathsErr)
	} else {
		fmt.Fprintf(&b, "  config     %s\n  history    %s\n  workspaces %s\n", p.ConfigFile, p.HistoryFile, p.WorkspacesFile)
	}
	b.WriteString(`
Environment:
  BDASH_CONFIG_DIR      directory holding config.toml
  BDASH_GLYPHS          glyph tier: ` + strings.Join(config.GlyphTiers, "|") + `
  BDASH_COLOR           colour depth: ` + strings.Join(config.Colors, "|") + `
  NO_COLOR              any non-empty value turns colour off (config color wins)
  BDASH_BD              path to the bd binary
  BDASH_TIMEOUT_SCALE   factor applied to bd timeouts
  BEADS_DIR             passed through to bd, which reads it itself

Precedence: flag > environment > config file > default.
`)
	return b.String()
}
