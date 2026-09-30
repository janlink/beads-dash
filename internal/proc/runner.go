package proc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Runner runs the helper programs bdash talks to for the clipboard and for
// notifications. Code that needs one takes a Runner, so tests can replace
// the processes with a [Fake].
type Runner interface {
	// LookPath finds a program the way the shell would.
	LookPath(file string) (string, error)
	// Run runs argv to completion with stdin as its standard input and
	// returns what it wrote. There is no shell: argv is the whole command.
	Run(ctx context.Context, argv []string, stdin []byte) ([]byte, error)
}

// waitDelay bounds how long Run waits for a helper's output after it exits:
// clipboard tools such as xclip and wl-copy leave a background child that
// keeps the pipes open.
const waitDelay = 300 * time.Millisecond

// System is the [Runner] that starts real processes.
type System struct{}

// LookPath implements [Runner].
func (System) LookPath(file string) (string, error) { return exec.LookPath(file) }

// Run implements [Runner].
func (System) Run(ctx context.Context, argv []string, stdin []byte) ([]byte, error) {
	if len(argv) == 0 {
		return nil, errors.New("proc: empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //nolint:gosec // argv[0] is a helper bdash picked; the arguments carry user text but no shell reads them
	cmd.Stdin = bytes.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.WaitDelay = waitDelay
	err := cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	if err != nil {
		if msg := strings.TrimSpace(out.String()); msg != "" {
			err = fmt.Errorf("%w: %s", err, firstLine(msg))
		}
	}
	return out.Bytes(), err
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}
