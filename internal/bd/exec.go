package bd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"

	"github.com/janlink/beads-dash/internal/model"
)

// ExecOptions configures [NewExec]. The zero value runs "bd" from PATH in the
// current directory with the default timeouts.
type ExecOptions struct {
	// Bin is the bd binary (BDASH_BD); empty means "bd".
	Bin string
	// Dir is the workspace directory bd runs in.
	Dir string
	// Env adds variables such as BEADS_DIR=<path>.
	Env []string
	// Timeouts defaults to [DefaultTimeouts].
	Timeouts Timeouts
	// Runner replaces the process seam; Bin, Dir and Env are then unused.
	Runner Runner
}

// ExecClient is the [Client] backed by the bd CLI. The read methods are
// implemented; the write methods and EventsFollow return
// [ErrNotImplemented].
type ExecClient struct {
	runner   Runner
	timeouts Timeouts
}

// NewExec builds a client on a real bd binary or a supplied runner.
func NewExec(o ExecOptions) *ExecClient {
	r := o.Runner
	if r == nil {
		r = ExecRunner{Bin: o.Bin, Dir: o.Dir, Env: o.Env}
	}
	t := o.Timeouts
	if t == (Timeouts{}) {
		t = DefaultTimeouts()
	}
	return &ExecClient{runner: r, timeouts: t}
}

var _ Client = (*ExecClient)(nil)

// commandName names the bd subcommand of an argv for error reports, leaving
// out arguments such as issue IDs.
func commandName(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	switch argv[0] {
	case "vc", "dep", "config", "events":
		if len(argv) > 1 && !strings.HasPrefix(argv[1], "-") {
			return argv[0] + " " + argv[1]
		}
	}
	return argv[0]
}

const stderrKeep = 2048

func clip(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > stderrKeep {
		s = s[:stderrKeep]
	}
	return s
}

// call runs one bd command and returns the payload of its JSON answer.
func (c *ExecClient) call(ctx context.Context, kind callKind, argv ...string) ([]byte, error) {
	cmd := commandName(argv)
	cctx, cancel := context.WithTimeout(ctx, c.timeouts.of(kind))
	defer cancel()
	res, err := c.runner.Run(cctx, argv)
	if err != nil {
		return nil, c.runError(ctx, cctx, cmd, err)
	}
	if res.ExitCode != 0 {
		return nil, c.exitError(kind, cmd, res)
	}
	data, err := unwrap(cmd, res.Stdout)
	if err != nil {
		var be *Error
		if errors.As(err, &be) {
			be.Stderr = clip(res.Stderr)
		}
		return nil, err
	}
	return data, nil
}

func (c *ExecClient) runError(parent, cctx context.Context, cmd string, err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(cctx.Err(), context.DeadlineExceeded):
		if errors.Is(parent.Err(), context.Canceled) {
			return context.Canceled
		}
		return &Error{Class: ClassTimeout, Command: cmd, Message: "no answer in time", Err: context.DeadlineExceeded}
	case errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
		return &Error{Class: ClassBdMissing, Command: cmd, Message: "cannot start bd", Err: err}
	}
	return &Error{Class: ClassTransient, Command: cmd, Message: "cannot run bd", Err: err}
}

func (c *ExecClient) exitError(kind callKind, cmd string, res Result) error {
	e := &Error{Class: ClassTransient, Command: cmd, ExitCode: res.ExitCode, Stderr: clip(res.Stderr)}
	f, found := parseFailure(res.Stdout, res.Stderr)
	e.Code, e.Message = f.Code, f.Message
	switch {
	case found && f.Skew:
		e.Class = ClassUnsupported
		if e.Message == "" {
			e.Message = "database schema does not match this bd"
		}
	case kind == kindWrite:
		e.Class = ClassRejected
	case cmd == "where" && f.Code == CodeNoBeadsDirectory:
		e.Class = ClassNotWorkspace
	}
	return e
}

func (c *ExecClient) decodeErr(cmd string, err error) error {
	return &Error{Class: ClassTransient, Command: cmd, Message: "unexpected JSON shape", Err: err}
}

func checkID(id string) error {
	if id == "" || strings.HasPrefix(id, "-") {
		return fmt.Errorf("bd: invalid issue ID %q", id)
	}
	return nil
}

// Version implements [Client].
func (c *ExecClient) Version(ctx context.Context) (VersionInfo, error) {
	data, err := c.call(ctx, kindProbe, "version", "--json")
	if err != nil {
		return VersionInfo{}, err
	}
	info, err := parseVersionInfo(data)
	if err != nil {
		var be *Error
		if errors.As(err, &be) {
			return info, err
		}
		return VersionInfo{}, c.decodeErr("version", err)
	}
	return info, nil
}

// Where implements [Client].
func (c *ExecClient) Where(ctx context.Context) (Workspace, error) {
	data, err := c.call(ctx, kindProbe, "where", "--json")
	if err != nil {
		return Workspace{}, err
	}
	w, err := parseWhere(data)
	if err != nil {
		return Workspace{}, c.decodeErr("where", err)
	}
	return w, nil
}

// List implements [Client].
func (c *ExecClient) List(ctx context.Context) ([]model.Issue, error) {
	data, err := c.call(ctx, kindRead, "list", "--all", "--limit", "0", "--json")
	if err != nil {
		return nil, err
	}
	issues, err := parseIssues(data)
	if err != nil {
		return nil, c.decodeErr("list", err)
	}
	return issues, nil
}

// Ready implements [Client].
func (c *ExecClient) Ready(ctx context.Context) (model.Readiness, error) {
	data, err := c.call(ctx, kindRead, "ready", "--explain", "--limit", "0", "--json")
	if err != nil {
		return model.Readiness{}, err
	}
	r, err := parseReadiness(data)
	if err != nil {
		return model.Readiness{}, c.decodeErr("ready", err)
	}
	return r, nil
}

// Statuses implements [Client].
func (c *ExecClient) Statuses(ctx context.Context) (model.Statuses, error) {
	data, err := c.call(ctx, kindRead, "statuses", "--json")
	if err != nil {
		return model.Statuses{}, err
	}
	s, err := parseStatuses(data)
	if err != nil {
		return model.Statuses{}, c.decodeErr("statuses", err)
	}
	return s, nil
}

// Types implements [Client].
func (c *ExecClient) Types(ctx context.Context) ([]TypeInfo, error) {
	data, err := c.call(ctx, kindRead, "types", "--json")
	if err != nil {
		return nil, err
	}
	t, err := parseTypes(data)
	if err != nil {
		return nil, c.decodeErr("types", err)
	}
	return t, nil
}

// VCStatus implements [Client].
func (c *ExecClient) VCStatus(ctx context.Context) (VCStatus, error) {
	data, err := c.call(ctx, kindRead, "vc", "status", "--json")
	if err != nil {
		return VCStatus{}, err
	}
	v, err := parseVCStatus(data)
	if err != nil {
		return VCStatus{}, c.decodeErr("vc status", err)
	}
	return v, nil
}

// Comments implements [Client].
func (c *ExecClient) Comments(ctx context.Context, id string) ([]Comment, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	data, err := c.call(ctx, kindRead, "comments", id, "--json")
	if err != nil {
		return nil, err
	}
	out, err := parseComments(data)
	if err != nil {
		return nil, c.decodeErr("comments", err)
	}
	return out, nil
}

// History implements [Client].
func (c *ExecClient) History(ctx context.Context, id string) ([]HistoryEntry, error) {
	if err := checkID(id); err != nil {
		return nil, err
	}
	data, err := c.call(ctx, kindRead, "history", id, "--json")
	if err != nil {
		return nil, err
	}
	out, err := parseHistory(data)
	if err != nil {
		return nil, c.decodeErr("history", err)
	}
	return out, nil
}

// Memories implements [Client].
func (c *ExecClient) Memories(ctx context.Context) ([]Memory, error) {
	data, err := c.call(ctx, kindRead, "memories", "--json")
	if err != nil {
		return nil, err
	}
	out, err := parseMemories(data)
	if err != nil {
		return nil, c.decodeErr("memories", err)
	}
	return out, nil
}

// ConfigGet implements [Client].
func (c *ExecClient) ConfigGet(ctx context.Context, key string) (ConfigValue, error) {
	if err := checkID(key); err != nil {
		return ConfigValue{}, err
	}
	data, err := c.call(ctx, kindRead, "config", "get", key, "--json")
	if err != nil {
		return ConfigValue{}, err
	}
	v, err := parseConfigValue(data)
	if err != nil {
		return ConfigValue{}, c.decodeErr("config get", err)
	}
	return v, nil
}

// EventsFollow is not implemented yet.
func (c *ExecClient) EventsFollow(context.Context, int64) (EventStream, error) {
	return nil, ErrNotImplemented
}

// Create is not implemented yet.
func (c *ExecClient) Create(context.Context, CreateSpec) (string, error) {
	return "", ErrNotImplemented
}

// Update is not implemented yet.
func (c *ExecClient) Update(context.Context, string, UpdateSpec) error { return ErrNotImplemented }

// Close is not implemented yet.
func (c *ExecClient) Close(context.Context, []string, string) ([]string, error) {
	return nil, ErrNotImplemented
}

// Reopen is not implemented yet.
func (c *ExecClient) Reopen(context.Context, []string) ([]string, error) {
	return nil, ErrNotImplemented
}

// DepAdd is not implemented yet.
func (c *ExecClient) DepAdd(context.Context, string, string, string) error {
	return ErrNotImplemented
}

// DepRemove is not implemented yet.
func (c *ExecClient) DepRemove(context.Context, string, string) error { return ErrNotImplemented }

// Comment is not implemented yet.
func (c *ExecClient) Comment(context.Context, string, string) error { return ErrNotImplemented }

// Remember is not implemented yet.
func (c *ExecClient) Remember(context.Context, string, string) error { return ErrNotImplemented }

// Forget is not implemented yet.
func (c *ExecClient) Forget(context.Context, string) error { return ErrNotImplemented }

// ConfigSet is not implemented yet.
func (c *ExecClient) ConfigSet(context.Context, string, string) error { return ErrNotImplemented }
