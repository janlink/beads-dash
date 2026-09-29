package bd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
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

// Streamer is the runner extension for long-lived commands whose output is
// read while they run.
type Streamer interface {
	Stream(ctx context.Context, argv []string) (ProcessStream, error)
}

// ProcessStream is a running bd process. Read yields its stdout. Wait
// returns the exit code and stderr once stdout has ended, or after Close.
// Close kills the process group and is safe to call repeatedly.
type ProcessStream interface {
	io.Reader
	Wait() (Result, error)
	Close() error
}

var _ Streamer = ExecRunner{}

// Stream implements [Streamer]. The process is killed with its group when
// ctx ends or the stream is closed.
func (r ExecRunner) Stream(ctx context.Context, argv []string) (ProcessStream, error) {
	bin := r.Bin
	if bin == "" {
		bin = "bd"
	}
	sctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(sctx, bin, argv...)
	cmd.Dir = r.Dir
	cmd.Env = r.environ()
	cmd.WaitDelay = killGrace
	setProcessGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	p := &execStream{cmd: cmd, out: stdout, cancel: cancel, ctx: sctx}
	cmd.Stderr = &p.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	return p, nil
}

type execStream struct {
	cmd    *exec.Cmd
	out    io.Reader
	ctx    context.Context
	cancel context.CancelFunc
	stderr limitedBuffer

	once sync.Once
	res  Result
	err  error
}

func (p *execStream) Read(b []byte) (int, error) { return p.out.Read(b) }

func (p *execStream) Wait() (Result, error) {
	p.once.Do(func() {
		err := p.cmd.Wait()
		p.res.Stderr = p.stderr.Bytes()
		var exitErr *exec.ExitError
		switch {
		case p.ctx.Err() != nil:
			p.err = p.ctx.Err()
		case errors.As(err, &exitErr):
			p.res.ExitCode = exitErr.ExitCode()
		default:
			p.err = err
		}
	})
	return p.res, p.err
}

func (p *execStream) Close() error {
	p.cancel()
	_, _ = p.Wait()
	return nil
}

// limitedBuffer keeps the first stderrKeep*2 bytes written and discards the
// rest, so a chatty follower cannot grow memory.
type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *limitedBuffer) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := stderrKeep*2 - l.buf.Len(); room > 0 {
		l.buf.Write(b[:min(room, len(b))])
	}
	return len(b), nil
}

func (l *limitedBuffer) Bytes() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return bytes.Clone(l.buf.Bytes())
}
