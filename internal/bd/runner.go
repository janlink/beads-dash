package bd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Result is what a bd process produced. A non-zero ExitCode is data, not a
// Go error.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Runner is the process seam: it runs bd with an argv slice and reports the
// outcome. It returns an error only when the process could not be run to
// completion (not found, killed by the context).
type Runner interface {
	Run(ctx context.Context, argv []string) (Result, error)
}

// ExecRunner runs a real bd binary. It never goes through a shell.
type ExecRunner struct {
	// Bin is the bd binary; empty means "bd" on PATH.
	Bin string
	// Dir is the workspace directory bd runs in; empty means the current one.
	Dir string
	// Env adds or overrides variables on top of the process environment,
	// e.g. BEADS_DIR=<path>. BEADS_DIR from the process environment passes
	// through untouched.
	Env []string
}

const killGrace = 2 * time.Second

// Run implements [Runner]. It sets BD_JSON_ENVELOPE=1 and BD_DISABLE_METRICS=1
// (bd's opt-out from anonymous usage metrics, so polling is not counted),
// drops BEADS_MAX_ROWS (bd exits with code 2 when it trips) and, when the
// context ends, kills the whole process group.
func (r ExecRunner) Run(ctx context.Context, argv []string) (Result, error) {
	bin := r.Bin
	if bin == "" {
		bin = "bd"
	}
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Dir = r.Dir
	cmd.Env = r.environ()
	cmd.WaitDelay = killGrace
	setProcessGroup(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return res, ctxErr
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, err
}

func (r ExecRunner) environ() []string {
	base := os.Environ()
	env := make([]string, 0, len(base)+len(r.Env)+1)
	for _, kv := range base {
		if strings.HasPrefix(kv, "BEADS_MAX_ROWS=") || strings.HasPrefix(kv, "BD_JSON_ENVELOPE=") || strings.HasPrefix(kv, "BD_DISABLE_METRICS=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, r.Env...)
	return append(env, "BD_JSON_ENVELOPE=1", "BD_DISABLE_METRICS=1")
}
