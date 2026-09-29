// Package cli parses bdash's command line and renders its help and version
// output. It has no side effects; cmd/bdash wires it to the process.
package cli

import (
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/janlink/beads-dash/internal/config"
)

// ExitUsage is the exit status for command-line usage errors.
const ExitUsage = 2

// Options is the parsed command line.
type Options struct {
	// Path is the optional workspace directory bd runs from.
	Path string
	// View is the --view value, empty when not given.
	View    string
	NoMouse bool
	Version bool
	Help    bool
}

// Flags returns the part of the command line that feeds settings.
func (o Options) Flags() config.Flags {
	return config.Flags{View: o.View, NoMouse: o.NoMouse}
}

// UsageError is a command-line mistake; the process exits with ExitUsage.
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

// Parse parses args (without the program name). Flags may appear before or
// after the path; "--" ends flag parsing. More than one path, an unknown flag
// or an invalid --view value is a UsageError.
func Parse(args []string) (Options, error) {
	var o Options
	var positional []string
	if i := slices.Index(args, "--"); i >= 0 {
		positional = append(positional, args[i+1:]...)
		args = args[:i]
	}

	var long, short, help, shortHelp bool
	fs := flag.NewFlagSet("bdash", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.View, "view", "", "")
	fs.BoolVar(&o.NoMouse, "no-mouse", false, "")
	fs.BoolVar(&long, "version", false, "")
	fs.BoolVar(&short, "v", false, "")
	fs.BoolVar(&help, "help", false, "")
	fs.BoolVar(&shortHelp, "h", false, "")

	var leading []string
	for {
		if err := fs.Parse(args); err != nil {
			return Options{}, &UsageError{Msg: err.Error()}
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		leading = append(leading, rest[0])
		args = rest[1:]
	}
	positional = append(leading, positional...)

	o.Version = long || short
	o.Help = help || shortHelp
	if o.View != "" && !slices.Contains(config.Views, o.View) {
		return Options{}, &UsageError{Msg: fmt.Sprintf("invalid --view %q: want %s", o.View, strings.Join(config.Views, "|"))}
	}
	if len(positional) > 1 {
		return Options{}, &UsageError{Msg: fmt.Sprintf("too many arguments: %s (at most one workspace path)", strings.Join(positional, " "))}
	}
	if len(positional) == 1 {
		o.Path = positional[0]
	}
	return o, nil
}
