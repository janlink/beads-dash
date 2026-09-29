package theme

import (
	"io"
	"strings"

	"github.com/charmbracelet/colorprofile"
)

// Depth is the colour depth bdash may use. It is fixed at startup.
type Depth int

const (
	DepthAuto Depth = iota
	DepthTrueColor
	Depth256
	Depth16
	DepthNone
)

var depthNames = [...]string{"auto", "truecolor", "256", "16", "none"}

func (d Depth) String() string {
	if d < 0 || int(d) >= len(depthNames) {
		return "unknown"
	}
	return depthNames[d]
}

// ParseDepth parses auto|truecolor|256|16|none.
func ParseDepth(s string) (Depth, bool) {
	for i, n := range depthNames {
		if s == n {
			return Depth(i), true
		}
	}
	return DepthAuto, false
}

// DetectDepth reports the depth the terminal supports. NO_COLOR is left to the
// caller, because colorprofile parses it as a boolean and bdash treats any
// non-empty value as set.
func DetectDepth(out io.Writer, environ []string) Depth {
	clean := make([]string, 0, len(environ))
	for _, kv := range environ {
		if !strings.HasPrefix(kv, "NO_COLOR=") {
			clean = append(clean, kv)
		}
	}
	switch colorprofile.Detect(out, clean) {
	case colorprofile.TrueColor:
		return DepthTrueColor
	case colorprofile.ANSI256:
		return Depth256
	case colorprofile.ANSI:
		return Depth16
	case colorprofile.ASCII, colorprofile.NoTTY, colorprofile.Unknown:
		return DepthNone
	}
	return DepthNone
}

// Background is the background mode: which theme variant applies.
type Background int

const (
	BackgroundAuto Background = iota
	BackgroundDark
	BackgroundLight
)

var backgroundNames = [...]string{"auto", "dark", "light"}

func (b Background) String() string {
	if b < 0 || int(b) >= len(backgroundNames) {
		return "unknown"
	}
	return backgroundNames[b]
}

// ParseBackground parses auto|dark|light.
func ParseBackground(s string) (Background, bool) {
	for i, n := range backgroundNames {
		if s == n {
			return Background(i), true
		}
	}
	return BackgroundAuto, false
}

// IsDark resolves a mode against the probed terminal background; auto follows
// the probe.
func (b Background) IsDark(probedDark bool) bool {
	switch b {
	case BackgroundDark:
		return true
	case BackgroundLight:
		return false
	case BackgroundAuto:
		return probedDark
	}
	return probedDark
}
