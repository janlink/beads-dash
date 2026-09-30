package form

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/proc"
)

// EditorDone is the outcome of an external edit. Owner is the dialog that asked
// and Key the field it was for.
type EditorDone struct {
	Owner any
	Key   string
	Text  string
	Err   error
}

// Edit opens text in the external editor on a temporary file and returns the
// command that runs it; the result arrives as an [EditorDone] message. The file
// is written when the command runs, not when Edit returns. It reports false
// when no editor is set.
func Edit(getenv func(string) string, owner any, key, text string) (tea.Cmd, bool) {
	argv := proc.EditorArgv(getenv)
	if argv == nil {
		return nil, false
	}
	return func() tea.Msg {
		path, err := writeTemp(text)
		if err != nil {
			return EditorDone{Owner: owner, Key: key, Err: err}
		}
		run := tea.ExecProcess(proc.Editor(argv, path), func(err error) tea.Msg { return Read(owner, key, path, err) })
		return run()
	}, true
}

func writeTemp(text string) (string, error) {
	file, err := os.CreateTemp("", "bdash-*.md")
	if err != nil {
		return "", err
	}
	_, werr := file.WriteString(text)
	if cerr := file.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(file.Name())
		return "", werr
	}
	return file.Name(), nil
}

// Read collects the temporary file of an external edit and removes it.
func Read(owner any, key, path string, runErr error) EditorDone {
	out := EditorDone{Owner: owner, Key: key, Err: runErr}
	data, err := os.ReadFile(path)
	_ = os.Remove(path)
	switch {
	case runErr != nil:
	case err != nil:
		out.Err = err
	default:
		out.Text = strings.TrimSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	}
	return out
}
