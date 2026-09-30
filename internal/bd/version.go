package bd

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed bd release number.
type Version struct {
	Major, Minor, Patch int
}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// Compare returns -1, 0 or 1.
func (v Version) Compare(o Version) int {
	for _, d := range [][2]int{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		switch {
		case d[0] < d[1]:
			return -1
		case d[0] > d[1]:
			return 1
		}
	}
	return 0
}

// ParseVersion reads "1.3.0", "v1.3.0" or "1.3.1-rc.1"; a pre-release counts
// as its base version.
func ParseVersion(s string) (Version, error) {
	raw := strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(raw, "-+"); i >= 0 {
		raw = raw[:i]
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("bd version %q is not major.minor.patch", s)
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return Version{}, fmt.Errorf("bd version %q is not major.minor.patch", s)
		}
		n[i] = v
	}
	return Version{n[0], n[1], n[2]}, nil
}

// MinSupported is the oldest bd release bdash works with. 1.2.0 and 1.2.1
// were retracted and fall below it.
var MinSupported = Version{1, 2, 2}

// TestedCeiling is the newest major.minor line bdash holds fixtures for; any
// patch release of it is fully supported. Raise it together with the
// fixtures for a new line.
var TestedCeiling = Version{1, 3, 0}

// Support says how far bdash trusts a bd version.
type Support int

const (
	// Supported versions lie between [MinSupported] and [TestedCeiling].
	Supported Support = iota
	// Untested versions are newer than [TestedCeiling]: bdash runs with the
	// capabilities of the highest tested line and warns.
	Untested
	// Unsupported versions are refused.
	Unsupported
)

func (s Support) String() string {
	switch s {
	case Supported:
		return "supported"
	case Untested:
		return "untested"
	case Unsupported:
		return "unsupported"
	}
	return "unknown"
}

// capabilities are the version-dependent behaviours bdash branches on. They
// are read only inside package bd.
type capabilities struct {
	// eventsJournal: the events journal and bd events exist.
	eventsJournal bool
}

// capabilitiesFor returns the capabilities of a version. Versions newer than
// the tested ceiling get those of the ceiling line.
func capabilitiesFor(v Version) capabilities {
	if v.Compare(TestedCeiling) > 0 {
		v = TestedCeiling
	}
	return capabilities{eventsJournal: v.Compare(Version{1, 3, 0}) >= 0}
}

// VersionInfo is what bd version reports plus bdash's verdict on it.
type VersionInfo struct {
	// Raw is the version string as bd printed it.
	Raw    string
	Parsed Version
	Build  string
	Commit string
	Branch string
	// Support is the verdict; Reason explains anything but Supported.
	Support Support
	Reason  string
	caps    capabilities
}

// HasJournal reports whether this bd has an events journal to switch on.
func (v VersionInfo) HasJournal() bool { return v.caps.eventsJournal }

// CheckVersion judges a version string. It returns the info even for an
// unsupported version, together with a [ClassUnsupported] error.
func CheckVersion(raw string) (VersionInfo, error) {
	info := VersionInfo{Raw: raw}
	v, err := ParseVersion(raw)
	if err != nil {
		info.Support = Unsupported
		info.Reason = err.Error()
		return info, &Error{Class: ClassUnsupported, Command: "version", Message: info.Reason}
	}
	info.Parsed = v
	info.caps = capabilitiesFor(v)
	switch {
	case v.Compare(MinSupported) < 0 || (v.Compare(MinSupported) == 0 && isPrerelease(raw)):
		info.Support = Unsupported
		info.Reason = fmt.Sprintf("found bd %s, requires %s or newer", v, MinSupported)
		return info, &Error{Class: ClassUnsupported, Command: "version", Message: info.Reason}
	case v.Major > TestedCeiling.Major || (v.Major == TestedCeiling.Major && v.Minor > TestedCeiling.Minor):
		info.Support = Untested
		info.Reason = fmt.Sprintf("bd %s is newer than the newest tested line %d.%d", v, TestedCeiling.Major, TestedCeiling.Minor)
	}
	return info, nil
}

func isPrerelease(raw string) bool {
	return strings.Contains(strings.TrimSpace(raw), "-")
}

// Probe asks the bd binary at bin (empty for "bd" on PATH) for its version
// with the probe timeout of t (zero means [DefaultTimeouts]; use
// [Timeouts.Scaled] for BDASH_TIMEOUT_SCALE). It needs no workspace. The
// returned info is filled in whenever bd answered, also alongside an
// unsupported error.
func Probe(ctx context.Context, bin string, t Timeouts) (VersionInfo, error) {
	return NewExec(ExecOptions{Bin: bin, Timeouts: t}).Version(ctx)
}
