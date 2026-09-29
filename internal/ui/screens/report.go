// Package screens draws the states that replace or interrupt a view: the
// full-screen startup check and fatal errors, the terminal too small screen,
// the empty-state blocks and the inline notice row.
package screens

import (
	"errors"
	"fmt"
	"strings"

	"github.com/janlink/beads-dash/internal/bd"
)

// Kind is a full-screen startup or fatal state.
type Kind int

const (
	BdMissing Kind = iota
	BdNoAnswer
	BdTooOld
	BdRefused
	SchemaMismatch
	NotWorkspace
	Unreadable
	FirstSnapshot
	Vanished
)

// Step is one link of the startup chain.
type Step int

const (
	StepBd Step = iota
	StepVersion
	StepSchema
	StepWorkspace
	StepSnapshot
	stepCount
)

var stepNames = [stepCount]string{"bd", "version", "JSON schema", "workspace", "snapshot"}

// CheckState is how a step of the chain went.
type CheckState int

const (
	NotRun CheckState = iota
	Passed
	Failed
)

// Check is one line of the startup checklist.
type Check struct {
	Name   string
	State  CheckState
	Detail string
}

// Fact is a labelled value under the headline.
type Fact struct{ Label, Value string }

// Fix is a command that repairs the state, with the reason to run it.
type Fix struct{ Cmd, Why string }

// Report is everything a full-screen state shows.
type Report struct {
	Kind   Kind
	Title  string
	What   string
	Facts  []Fact
	Checks []Check
	// Ran names the failed bd command; Raw is bd's own output for it.
	Ran   string
	Raw   []string
	Fixes []Fix
	After string
}

// Input describes what went wrong and the surroundings the fix texts depend
// on. Err is what failed; Session is whatever the probes learned before.
type Input struct {
	Err     error
	Session bd.Session
	// FirstSnapshot is set when the session opened but no snapshot ever
	// arrived, Vanished when the workspace disappeared while running.
	FirstSnapshot bool
	Vanished      bool
	// BdPath is the resolved bd binary, Dir the directory bd runs in.
	BdPath   string
	Dir      string
	BeadsDir string
	// BdashViaBrew is set when bdash itself came from Homebrew.
	BdashViaBrew bool
	BdashVersion string
}

const (
	goInstallBd    = "go install github.com/gastownhall/beads/cmd/bd@latest"
	goInstallBdash = "go install github.com/janlink/beads-dash/cmd/bdash@latest"
)

// Diagnose turns a failure into the report the startup screen shows. The
// failed step and the state come from the error class and the version verdict,
// never from bd's text.
func Diagnose(in Input) Report {
	step, kind := classify(in)
	r := Report{Kind: kind}
	r.Checks = checks(in, step)
	r.Ran, r.Raw = ranAndRaw(in)
	fill(&r, in)
	r.Checks[step].State = Failed
	r.Checks[step].Detail = failDetail(in, kind)
	for i := int(step) + 1; i < int(stepCount); i++ {
		r.Checks[i].State, r.Checks[i].Detail = NotRun, "not run"
	}
	return r
}

func classify(in Input) (Step, Kind) {
	v := in.Session.Version
	switch {
	case in.Vanished:
		return StepWorkspace, Vanished
	case bd.IsClass(in.Err, bd.ClassBdMissing):
		return StepBd, BdMissing
	case bd.IsClass(in.Err, bd.ClassUnsupported):
		if v.Support == bd.Unsupported {
			if v.Parsed.Major == bd.MinSupported.Major && v.Parsed.Minor == bd.MinSupported.Minor && v.Parsed.Compare(bd.MinSupported) < 0 {
				return StepVersion, BdRefused
			}
			return StepVersion, BdTooOld
		}
		return StepSchema, SchemaMismatch
	case bd.IsClass(in.Err, bd.ClassNotWorkspace):
		return StepWorkspace, NotWorkspace
	case in.FirstSnapshot:
		return StepSnapshot, FirstSnapshot
	case v.Raw == "":
		return StepVersion, BdNoAnswer
	case in.Session.Workspace.Path == "":
		return StepWorkspace, Unreadable
	}
	return StepSnapshot, Unreadable
}

func checks(in Input, failed Step) []Check {
	s := in.Session
	out := make([]Check, stepCount)
	for i := range out {
		out[i].Name = stepNames[i]
	}
	if failed > StepBd {
		out[StepBd] = Check{Name: "bd", State: Passed, Detail: orElse(in.BdPath, "found")}
	}
	if failed > StepVersion {
		detail := s.Version.Raw
		if s.Version.Build != "" {
			detail += " (" + s.Version.Build + ")"
		}
		out[StepVersion] = Check{Name: "version", State: Passed, Detail: detail + " - supported " + supportRange()}
	}
	if failed > StepSchema {
		out[StepSchema] = Check{Name: "JSON schema", State: Passed, Detail: "1"}
	}
	if failed > StepWorkspace {
		out[StepWorkspace] = Check{Name: "workspace", State: Passed, Detail: s.Workspace.Path}
	}
	return out
}

func supportRange() string {
	return fmt.Sprintf("%s – %d.%d.x", bd.MinSupported, bd.TestedCeiling.Major, bd.TestedCeiling.Minor)
}

func failDetail(in Input, k Kind) string {
	v := in.Session.Version
	switch k {
	case BdMissing:
		return "not found on PATH"
	case BdNoAnswer:
		return "bd version gave no answer"
	case BdTooOld:
		return fmt.Sprintf("%s - needs %s or newer", verText(v), bd.MinSupported)
	case BdRefused:
		return fmt.Sprintf("%s - withdrawn release, needs %s+", verText(v), bd.MinSupported)
	case SchemaMismatch:
		return "not 1 - bdash reads 1"
	case NotWorkspace:
		return "none found from " + orElse(in.Dir, ".") + " upward"
	case Vanished:
		return in.Session.Workspace.Path + " was here and is gone"
	case Unreadable, FirstSnapshot:
		var be *bd.Error
		if errors.As(in.Err, &be) {
			if be.Class == bd.ClassTimeout {
				return "bd " + be.Command + " timed out"
			}
			if be.ExitCode != 0 {
				return fmt.Sprintf("bd %s failed, exit %d", be.Command, be.ExitCode)
			}
			return "bd " + be.Command + " failed"
		}
		return "bd failed"
	}
	return "failed"
}

func ranAndRaw(in Input) (string, []string) {
	var be *bd.Error
	if !errors.As(in.Err, &be) {
		if in.Err != nil {
			return "", splitLines(in.Err.Error())
		}
		return "", nil
	}
	ran := "bd"
	if be.Command != "" {
		ran += " " + be.Command
	}
	if be.Class == bd.ClassBdMissing {
		ran = "exec " + ran
	}
	if be.ExitCode != 0 {
		ran += fmt.Sprintf(" - exit %d", be.ExitCode)
	}
	raw := splitLines(be.Stderr)
	if len(raw) == 0 {
		raw = splitLines(be.Message)
	}
	if len(raw) == 0 && be.Err != nil {
		raw = splitLines(be.Err.Error())
	}
	return ran, raw
}

func splitLines(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func orElse(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func verText(v bd.VersionInfo) string {
	if v.Parsed == (bd.Version{}) {
		return orElse(v.Raw, "unknown")
	}
	return v.Parsed.String()
}

func isBrew(v bd.VersionInfo) bool { return strings.Contains(strings.ToLower(v.Build), "homebrew") }

func fill(r *Report, in Input) {
	v := in.Session.Version
	dir := orElse(in.Dir, ".")
	bdFix := Fix{goInstallBd, "or build it with Go"}
	if isBrew(v) {
		bdFix = Fix{"brew upgrade beads", "your bd came from Homebrew"}
	}
	upgrade := func() []Fix {
		if isBrew(v) {
			return []Fix{bdFix, {goInstallBd, "or build it with Go"}}
		}
		return []Fix{{goInstallBd, "your bd was built with Go"}, {"brew upgrade beads", "or with Homebrew"}}
	}
	switch r.Kind {
	case BdMissing:
		r.Title = "bd is not installed"
		r.What = "bdash reads and writes everything through the bd CLI, and there is no bd on PATH."
		r.Facts = []Fact{{"looked for", "bd on $PATH"}}
		r.Fixes = []Fix{
			{"brew install beads", "Homebrew (macOS, Linux)"},
			{goInstallBd, "Go toolchain"},
			{"BDASH_BD=/path/to/bd bdash", "bd is installed somewhere else"},
		}
		r.After = "Installed already? Put its directory on PATH and press r."
	case BdNoAnswer:
		r.Title = "bd did not answer"
		r.What = "bdash asked bd for its version and got no usable answer."
		r.Facts = []Fact{{"path", orElse(in.BdPath, "bd on $PATH")}}
		r.Fixes = []Fix{{"bd version", "run it by hand and look at the output"}}
	case BdTooOld:
		r.Title = fmt.Sprintf("bd %s is too old", verText(v))
		r.What = fmt.Sprintf("bdash needs bd %s or newer; older versions lack the JSON output it reads.", bd.MinSupported)
		r.Facts = versionFacts(v, in, fmt.Sprintf("%s or newer - tested up to %d.%d.x", bd.MinSupported, bd.TestedCeiling.Major, bd.TestedCeiling.Minor))
		r.Fixes = upgrade()
		r.After = "Upgrading bd does not touch your issues."
	case BdRefused:
		r.Title = fmt.Sprintf("bd %s is not supported", verText(v))
		r.What = fmt.Sprintf("bd 1.2.0 and 1.2.1 were withdrawn releases; %s re-released the tested line. bdash does not run on them.", bd.MinSupported)
		r.Facts = versionFacts(v, in, fmt.Sprintf("%s or newer, not 1.2.0 / 1.2.1", bd.MinSupported))
		r.Fixes = upgrade()
		r.After = "Upgrading bd does not touch your issues."
	case SchemaMismatch:
		r.Title = "bd speaks a different JSON format"
		r.What = "bd answered in a JSON schema this bdash does not read. Reading on could show wrong data, so bdash stops."
		r.Facts = []Fact{
			{"found", "bd " + verText(v)},
			{"needs", "JSON schema 1"},
			{"bdash", orElse(in.BdashVersion, "unknown")},
		}
		bdashFix := Fix{goInstallBdash, "a newer bdash may read this schema"}
		if in.BdashViaBrew {
			bdashFix = Fix{"brew upgrade janlink/tap/bdash", "a newer bdash may read this schema"}
		}
		r.Fixes = []Fix{bdashFix, {fmt.Sprintf("go install github.com/gastownhall/beads/cmd/bd@v%s", bd.TestedCeiling), "or go back to a tested bd"}}
	case NotWorkspace:
		r.Title = "Not a Beads workspace"
		r.What = fmt.Sprintf("bd found no .beads directory it can read in %s or any parent directory.", dir) // lintcheck:allow user-facing text
		r.Facts = []Fact{{"directory", dir}, {"BEADS_DIR", orElse(in.BeadsDir, "not set")}}
		r.Fixes = []Fix{
			{"bd init", "create a workspace here"},
			{"bdash ~/code/other-project", "open another directory"},
			{"BEADS_DIR=/path/to/.beads bdash", "point bd at an existing workspace"}, // lintcheck:allow user-facing text
		}
		r.After = "bdash starts as soon as bd sees a workspace."
	case Vanished:
		path := in.Session.Workspace.Path
		r.Title = "The workspace is gone"
		r.What = fmt.Sprintf("%s was here and is gone. Three refreshes in a row found no workspace, so the last snapshot was dropped.", path)
		r.Facts = []Fact{{"directory", dir}, {"was", path}}
		r.Fixes = []Fix{
			{"bd init", "create a workspace again"},
			{"BEADS_DIR=/path/to/.beads bdash", "point bd at an existing workspace"}, // lintcheck:allow user-facing text
		}
		r.After = "bdash comes back by itself when bd sees a workspace again."
	case Unreadable, FirstSnapshot:
		r.Title = "bd couldn't read this workspace"
		r.What = "The workspace exists, but bd failed to answer. Nothing was loaded yet, so there is nothing to show."
		r.Facts = []Fact{{"workspace", orElse(in.Session.Workspace.Path, dir)}, {"bd", orElse(v.Raw, "unknown")}}
		r.Fixes = []Fix{
			{"bd doctor", "bd's own health check"},
			{"ls -la .beads", "check owner and permissions"}, // lintcheck:allow user-facing text
		}
		r.After = "Another bd process holding the database? Wait for it, then r."
	}
}

func versionFacts(v bd.VersionInfo, in Input, needs string) []Fact {
	found := "bd " + verText(v)
	if v.Build != "" {
		found += " (" + v.Build + ")"
	}
	facts := []Fact{{"found", found}, {"needs", needs}}
	if in.BdPath != "" {
		facts = append(facts, Fact{"path", in.BdPath})
	}
	return facts
}
