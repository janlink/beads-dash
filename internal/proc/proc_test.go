package proc_test

import (
	"slices"
	"testing"

	"github.com/janlink/beads-dash/internal/proc"
)

func TestWords(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"vi", []string{"vi"}},
		{"code --wait", []string{"code", "--wait"}},
		{`"/Program Files/Vim/vim.exe" -f`, []string{"/Program Files/Vim/vim.exe", "-f"}},
		{`emacsclient -a '' -c`, []string{"emacsclient", "-a", "", "-c"}},
		{`nvim -c "set tw=72"`, []string{"nvim", "-c", "set tw=72"}},
		{`my\ editor x`, []string{"my editor", "x"}},
		{`a"b c"d`, []string{"ab cd"}},
		{`x "open`, []string{"x", "open"}},
	}
	for _, tt := range tests {
		if got := proc.Words(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("Words(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEditorArgvPrefersVisual(t *testing.T) {
	env := map[string]string{"VISUAL": "code -w", "EDITOR": "vi"}
	get := func(k string) string { return env[k] }
	if got := proc.EditorArgv(get); !slices.Equal(got, []string{"code", "-w"}) {
		t.Errorf("got %q", got)
	}
	delete(env, "VISUAL")
	if got := proc.EditorArgv(get); !slices.Equal(got, []string{"vi"}) {
		t.Errorf("got %q", got)
	}
	delete(env, "EDITOR")
	if proc.EditorArgv(get) != nil {
		t.Error("no editor set must give nil")
	}
}
