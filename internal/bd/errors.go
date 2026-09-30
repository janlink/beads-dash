package bd

import (
	"errors"
	"fmt"
	"strings"
)

// Class is one of the seven ways a bd interaction fails. It is decided by
// exit code, the JSON error key and which command ran, never by error text.
type Class int

const (
	// ClassTransient is any read failure not covered below.
	ClassTransient Class = iota
	// ClassBdMissing means the bd binary could not be started.
	ClassBdMissing
	// ClassUnsupported means the bd version, schema version or database
	// schema is one bdash refuses to work with.
	ClassUnsupported
	// ClassNotWorkspace means bd found no workspace.
	ClassNotWorkspace
	// ClassTimeout means a call outlived its deadline.
	ClassTimeout
	// ClassRejected means bd refused a write.
	ClassRejected
	// ClassPartialWrite means a write succeeded for some IDs only.
	ClassPartialWrite
)

func (c Class) String() string {
	switch c {
	case ClassTransient:
		return "transient"
	case ClassBdMissing:
		return "bd missing"
	case ClassUnsupported:
		return "unsupported"
	case ClassNotWorkspace:
		return "not a workspace"
	case ClassTimeout:
		return "timeout"
	case ClassRejected:
		return "rejected"
	case ClassPartialWrite:
		return "partial write"
	}
	return fmt.Sprintf("class(%d)", int(c))
}

// CodeNoBeadsDirectory is the JSON error key bd uses when it finds no
// workspace.
const CodeNoBeadsDirectory = "no_beads_directory"

// Error is a classified bd failure.
type Error struct {
	Class Class
	// Command is the bd subcommand, e.g. "list".
	Command string
	// ExitCode is bd's exit code, 0 when bd did not run to completion.
	ExitCode int
	// Code is bd's JSON "error" key when the output carried one.
	Code string
	// Message is bd's error message or a description of the failure.
	Message string
	// Stderr is the leading part of bd's stderr, for diagnostics only.
	Stderr string
	// Applied lists the IDs a write changed, and Failed the IDs it left
	// alone with bd's reason. A write error carries them when bd said which.
	Applied []string
	Failed  []WriteFailure
	Err     error
}

// WriteFailure is one issue a write did not change.
type WriteFailure struct {
	ID      string
	Message string
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("bd")
	if e.Command != "" {
		b.WriteString(" " + e.Command)
	}
	b.WriteString(": " + e.Class.String())
	if e.Code != "" {
		b.WriteString(" (" + e.Code + ")")
	}
	if e.ExitCode != 0 {
		fmt.Fprintf(&b, " exit %d", e.ExitCode)
	}
	if e.Message != "" {
		b.WriteString(": " + e.Message)
	}
	if e.Err != nil {
		b.WriteString(": " + e.Err.Error())
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.Err }

// ClassOf returns the class of err, and false when err carries no
// [*Error]. A context cancellation is not a bd failure and has no class.
func ClassOf(err error) (Class, bool) {
	var be *Error
	if errors.As(err, &be) {
		return be.Class, true
	}
	return 0, false
}

// IsClass reports whether err is a bd failure of class c.
func IsClass(err error, c Class) bool {
	got, ok := ClassOf(err)
	return ok && got == c
}

// CodeOf returns the JSON error key carried by err, if any.
func CodeOf(err error) string {
	var be *Error
	if errors.As(err, &be) {
		return be.Code
	}
	return ""
}
