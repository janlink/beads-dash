// Package proc starts the user's own programs, such as their editor. Process
// creation is confined to this package, internal/bd and internal/testbd.
package proc

import (
	"os/exec"
	"strings"
	"unicode"
)

// Editor is the command that opens path in the editor argv names, the editor
// followed by its arguments.
func Editor(argv []string, path string) *exec.Cmd {
	return exec.Command(argv[0], append(argv[1:], path)...) //nolint:gosec // the editor is the user's own choice
}

// EditorArgv is the external editor from $VISUAL or $EDITOR, split like a
// shell splits words; nil when neither is set.
func EditorArgv(getenv func(string) string) []string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if w := Words(getenv(name)); len(w) > 0 {
			return w
		}
	}
	return nil
}

// Words splits s at white space, honouring single quotes, double quotes and
// backslash escapes. An unterminated quote runs to the end.
func Words(s string) []string {
	var out []string
	var cur strings.Builder
	inWord := false
	var quote rune
	escaped := false
	flush := func() {
		if inWord {
			out = append(out, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case quote == '"':
			if r == '"' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case unicode.IsSpace(r):
			flush()
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	flush()
	return out
}
