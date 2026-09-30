package bd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// A write command's exit code is necessary but not sufficient: bd exits 0
// when a close guard refuses one issue of a batch or when one ID of an update
// does not resolve, and exits 1 for a batch that changed some issues. The
// result of a write is judged from the exit code, the IDs bd returns and the
// messages on both streams together.

// writeReport is what a finished write command said.
type writeReport struct {
	exit int
	// ids are the issues bd returned as changed.
	ids []string
	// failed are the per-issue failures bd listed in JSON.
	failed []WriteFailure
	// msg is bd's error message from JSON, empty when it gave none.
	msg string
	// lines are the plain-text lines bd wrote to stderr.
	lines []string
	skew  bool
}

func readWrite(cmd string, res Result) (writeReport, error) {
	r := writeReport{exit: res.ExitCode}
	if body := bytes.TrimSpace(res.Stdout); len(body) > 0 {
		payload, err := unwrap(cmd, body)
		if err != nil && IsClass(err, ClassUnsupported) {
			return r, err
		}
		if err == nil {
			r.ids = payloadIDs(payload)
		}
	}
	r.scan(res.Stdout, false)
	r.scan(res.Stderr, true)
	return r, nil
}

// scan reads a stream that holds one JSON document or lines of text and JSON;
// text lines count only on stderr.
func (r *writeReport) scan(b []byte, text bool) {
	if body := bytes.TrimSpace(b); len(body) > 0 && body[0] == '{' && json.Valid(body) {
		r.jsonLine(string(body))
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		switch line = strings.TrimSpace(line); {
		case line == "":
		case line[0] == '{':
			r.jsonLine(line)
		case text:
			if t, ok := stderrText(line); ok {
				r.lines = append(r.lines, t)
			}
		}
	}
}

// jsonLine reads one JSON object bd printed as an error.
func (r *writeReport) jsonLine(line string) {
	var top struct {
		Error      string          `json:"error"`
		Message    string          `json:"message"`
		SchemaSkew json.RawMessage `json:"schema_skew"`
		Data       *struct {
			Error   string `json:"error"`
			Message string `json:"message"`
			Failed  []struct {
				ID    string `json:"id"`
				Error string `json:"error"`
			} `json:"failed"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(line), &top) != nil {
		return
	}
	msg := firstNonEmpty(top.Message, top.Error)
	if top.Data != nil {
		msg = firstNonEmpty(top.Data.Message, top.Data.Error, msg)
		for _, f := range top.Data.Failed {
			r.failed = append(r.failed, WriteFailure{ID: f.ID, Message: f.Error})
		}
	}
	if len(top.SchemaSkew) > 0 {
		r.skew = true
	}
	if r.msg == "" {
		r.msg = msg
	}
}

// stderrText keeps the lines of stderr that are bd's own message, not the
// notice about several bd binaries on the PATH.
func stderrText(line string) (string, bool) {
	switch {
	case strings.HasPrefix(line, "Warning: multiple"), strings.HasPrefix(line, "The first one is being used"),
		strings.HasPrefix(line, "/") && strings.HasSuffix(line, "/bd"):
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "Error: ")), true
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// payloadIDs lists the IDs in a write's answer: an array of issues, one
// issue, or a dependency edge.
func payloadIDs(payload json.RawMessage) []string {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return nil
	}
	type item struct {
		ID      string `json:"id"`
		IssueID string `json:"issue_id"`
	}
	var items []item
	switch payload[0] {
	case '[':
		if json.Unmarshal(payload, &items) != nil {
			return nil
		}
	case '{':
		var one item
		if json.Unmarshal(payload, &one) != nil {
			return nil
		}
		items = []item{one}
	}
	var out []string
	for _, it := range items {
		if id := firstNonEmpty(it.ID, it.IssueID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// warnings are the stderr lines of a write that exited 0 but reports part of
// it as undone, such as a dependency bd could not add to a new issue.
func (r writeReport) warnings() []string {
	var out []string
	for _, l := range r.lines {
		if strings.HasPrefix(l, "Warning: failed") {
			out = append(out, strings.TrimPrefix(l, "Warning: "))
		}
	}
	return out
}

// message is the one-line reason for the whole write.
func (r writeReport) message() string {
	if len(r.lines) > 0 {
		return strings.Join(r.lines, "; ")
	}
	return r.msg
}

// reasonFor is bd's reason for leaving id alone.
func (r writeReport) reasonFor(id string) string {
	for _, f := range r.failed {
		if f.ID == id && f.Message != "" {
			return f.Message
		}
	}
	for _, l := range r.lines {
		if mentions(l, id) {
			return l
		}
	}
	return firstNonEmpty(r.msg, strings.Join(r.lines, "; "))
}

// mentions reports whether line names id as a whole token, so "x-1" does not
// match "x-10".
func mentions(line, id string) bool {
	isID := func(r rune) bool {
		return r == '-' || r == '.' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	for _, tok := range strings.FieldsFunc(line, func(r rune) bool { return !isID(r) }) {
		if tok == id {
			return true
		}
	}
	return false
}

// judge decides the outcome of a write on ids: nil when every ID was
// changed, otherwise a [*Error] that says which were not. An answer without
// IDs (a create) passes ids nil and only the exit code counts.
func (c *ExecClient) judge(cmd string, res Result, r writeReport, ids []string) error {
	var missing []string
	for _, id := range ids {
		if !slices.Contains(r.ids, id) {
			missing = append(missing, id)
		}
	}
	if r.exit == 0 && len(missing) == 0 {
		return nil
	}
	e := &Error{Class: ClassRejected, Command: cmd, ExitCode: r.exit, Stderr: clip(res.Stderr), Message: r.message()}
	if r.skew {
		e.Class = ClassUnsupported
		return e
	}
	for _, id := range ids {
		if slices.Contains(r.ids, id) {
			e.Applied = append(e.Applied, id)
		}
	}
	for _, id := range missing {
		e.Failed = append(e.Failed, WriteFailure{ID: id, Message: r.reasonFor(id)})
	}
	if len(e.Applied) > 0 && len(missing) > 0 {
		e.Class = ClassPartialWrite
	}
	if e.Message == "" {
		e.Message = "bd changed nothing"
	}
	return e
}

// write runs a mutating bd command and returns what it said.
func (c *ExecClient) write(ctx context.Context, argv ...string) (Result, writeReport, string, error) {
	return c.writeInput(ctx, nil, argv...)
}

func (c *ExecClient) writeInput(ctx context.Context, stdin []byte, argv ...string) (Result, writeReport, string, error) {
	cmd := commandName(argv)
	cctx, cancel := context.WithTimeout(ctx, c.timeouts.of(kindWrite))
	defer cancel()
	var res Result
	var err error
	if in, ok := c.runner.(InputRunner); ok && stdin != nil {
		res, err = in.RunInput(cctx, argv, stdin)
	} else if stdin != nil {
		return Result{}, writeReport{}, cmd, fmt.Errorf("bd: this runner cannot pass text on stdin")
	} else {
		res, err = c.runner.Run(cctx, argv)
	}
	if err != nil {
		return res, writeReport{}, cmd, c.runError(ctx, cctx, cmd, err)
	}
	r, err := readWrite(cmd, res)
	return res, r, cmd, err
}

func checkIDs(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("bd: no issue given")
	}
	for _, id := range ids {
		if err := checkID(id); err != nil {
			return err
		}
	}
	return nil
}

func checkLabels(labels []string) error {
	for _, l := range labels {
		if l == "" || strings.ContainsAny(l, ",\n") {
			return fmt.Errorf("bd: invalid label %q", l)
		}
	}
	return nil
}

func flag(name, value string) string { return "--" + name + "=" + value }

const (
	// longText is the size from which a description or design travels by
	// stdin or a file instead of the command line.
	longText = 8 << 10
	// maxArg is what one argument may hold; bd reads acceptance and notes
	// from the command line only.
	maxArg = 100 << 10
)

const designPath = "<design-file>"

// spill holds the texts of a write that do not travel on the command line.
type spill struct {
	stdin  []byte
	design string
}

func (sp *spill) flag(name, value string) (string, error) {
	switch {
	case name == "description" && len(value) >= longText:
		sp.stdin = []byte(value)
		return "--body-file=-", nil
	case name == "design" && len(value) >= longText:
		sp.design = value
		return "--design-file=" + designPath, nil
	case len(value) > maxArg:
		return "", fmt.Errorf("bd: %s is longer than the command line allows (%d KB)", name, maxArg>>10)
	}
	return flag(name, value), nil
}

// run starts bd with sp's texts in place.
func (c *ExecClient) writeWith(ctx context.Context, sp spill, argv ...string) (Result, writeReport, string, error) {
	if sp.design != "" {
		f, err := os.CreateTemp("", "bdash-design-*.md")
		if err != nil {
			return Result{}, writeReport{}, commandName(argv), err
		}
		defer os.Remove(f.Name())
		_, werr := f.WriteString(sp.design)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return Result{}, writeReport{}, commandName(argv), werr
		}
		argv = slices.Clone(argv)
		for i, a := range argv {
			if strings.HasSuffix(a, designPath) {
				argv[i] = "--design-file=" + f.Name()
			}
		}
	}
	return c.writeInput(ctx, sp.stdin, argv...)
}

func createArgv(s CreateSpec) ([]string, spill, error) {
	var sp spill
	if err := checkLabels(s.Labels); err != nil {
		return nil, sp, err
	}
	argv := []string{"create", "--json", flag("title", s.Title)}
	var terr error
	add := func(name, value string) {
		if value == "" {
			return
		}
		a, err := sp.flag(name, value)
		terr = errors.Join(terr, err)
		argv = append(argv, a)
	}
	add("type", s.Type)
	if s.Priority != nil {
		argv = append(argv, flag("priority", strconv.Itoa(*s.Priority)))
	}
	add("description", s.Description)
	add("parent", s.Parent)
	add("assignee", s.Assignee)
	if len(s.Labels) > 0 {
		argv = append(argv, flag("labels", strings.Join(s.Labels, ",")))
	}
	if s.NoInheritLabels {
		argv = append(argv, "--no-inherit-labels")
	}
	add("design", s.Design)
	add("acceptance", s.Acceptance)
	add("notes", s.Notes)
	add("external-ref", s.ExternalRef)
	add("due", s.Due)
	add("defer", s.Defer)
	if s.Estimate != nil {
		argv = append(argv, flag("estimate", strconv.Itoa(*s.Estimate)))
	}
	if len(s.Deps) > 0 {
		argv = append(argv, flag("deps", strings.Join(s.Deps, ",")))
	}
	return argv, sp, terr
}

func updateArgv(ids []string, s UpdateSpec) ([]string, spill, error) {
	var sp spill
	if err := checkIDs(ids); err != nil {
		return nil, sp, err
	}
	if err := checkLabels(slices.Concat(s.AddLabels, s.RemoveLabels)); err != nil {
		return nil, sp, err
	}
	argv := append([]string{"update"}, ids...)
	argv = append(argv, "--json")
	var terr error
	text := func(name string, v *string) {
		if v == nil {
			return
		}
		a, err := sp.flag(name, *v)
		terr = errors.Join(terr, err)
		argv = append(argv, a)
	}
	text("title", s.Title)
	text("description", s.Description)
	text("design", s.Design)
	text("acceptance", s.Acceptance)
	text("notes", s.Notes)
	text("status", s.Status)
	text("assignee", s.Assignee)
	text("type", s.Type)
	text("external-ref", s.ExternalRef)
	text("due", s.Due)
	text("defer", s.Defer)
	text("parent", s.Parent)
	if s.Priority != nil {
		argv = append(argv, flag("priority", strconv.Itoa(*s.Priority)))
	}
	if s.Estimate != nil {
		argv = append(argv, flag("estimate", strconv.Itoa(*s.Estimate)))
	}
	for _, l := range s.AddLabels {
		argv = append(argv, flag("add-label", l))
	}
	for _, l := range s.RemoveLabels {
		argv = append(argv, flag("remove-label", l))
	}
	if s.Claim {
		argv = append(argv, "--claim")
	}
	return argv, sp, terr
}

// Create implements [Client]. It returns the new issue's ID.
func (c *ExecClient) Create(ctx context.Context, spec CreateSpec) (string, error) {
	if strings.TrimSpace(spec.Title) == "" {
		return "", &Error{Class: ClassRejected, Command: "create", Message: "title required"}
	}
	argv, sp, err := createArgv(spec)
	if err != nil {
		return "", err
	}
	res, r, cmd, err := c.writeWith(ctx, sp, argv...)
	if err != nil {
		return "", err
	}
	if err := c.judge(cmd, res, r, nil); err != nil {
		return "", err
	}
	if len(r.ids) != 1 {
		return "", &Error{Class: ClassRejected, Command: cmd, Message: "bd did not say which issue it created", Stderr: clip(res.Stderr)}
	}
	id := r.ids[0]
	if w := r.warnings(); len(w) > 0 {
		return id, &Error{
			Class: ClassPartialWrite, Command: cmd, Applied: []string{id}, Message: strings.Join(w, "; "), Stderr: clip(res.Stderr),
		}
	}
	return id, nil
}

// Update implements [Client].
func (c *ExecClient) Update(ctx context.Context, ids []string, spec UpdateSpec) error {
	if spec.Empty() {
		return nil
	}
	argv, sp, err := updateArgv(ids, spec)
	if err != nil {
		return err
	}
	res, r, cmd, err := c.writeWith(ctx, sp, argv...)
	if err != nil {
		return err
	}
	return c.judge(cmd, res, r, ids)
}

// Close implements [Client]. bd's close guards (a blocked issue, an epic with
// open children) refuse an issue with exit 0 when others in the batch closed,
// so the IDs in the answer decide, not the exit code.
func (c *ExecClient) Close(ctx context.Context, ids []string, reason string) ([]string, error) {
	return c.status(ctx, "close", ids, reason)
}

// Reopen implements [Client].
func (c *ExecClient) Reopen(ctx context.Context, ids []string, reason string) ([]string, error) {
	return c.status(ctx, "reopen", ids, reason)
}

func (c *ExecClient) status(ctx context.Context, verb string, ids []string, reason string) ([]string, error) {
	if err := checkIDs(ids); err != nil {
		return nil, err
	}
	argv := append([]string{verb}, ids...)
	argv = append(argv, "--json")
	if reason != "" {
		argv = append(argv, flag("reason", reason))
	}
	res, r, cmd, err := c.write(ctx, argv...)
	if err != nil {
		return nil, err
	}
	if err := c.judge(cmd, res, r, ids); err != nil {
		return r.ids, err
	}
	return r.ids, nil
}

// DepAdd implements [Client].
func (c *ExecClient) DepAdd(ctx context.Context, from, to, depType string) error {
	if err := checkIDs([]string{from, to}); err != nil {
		return err
	}
	argv := []string{"dep", "add", from, to, "--json"}
	if depType != "" {
		argv = append(argv, flag("type", depType))
	}
	return c.dep(ctx, argv, "added")
}

// DepRemove implements [Client].
func (c *ExecClient) DepRemove(ctx context.Context, from, to string) error {
	if err := checkIDs([]string{from, to}); err != nil {
		return err
	}
	return c.dep(ctx, []string{"dep", "rm", from, to, "--json"}, "removed")
}

func (c *ExecClient) dep(ctx context.Context, argv []string, status string) error {
	res, r, cmd, err := c.write(ctx, argv...)
	if err != nil {
		return err
	}
	if err := c.judge(cmd, res, r, nil); err != nil {
		return err
	}
	var payload struct {
		Status string `json:"status"`
	}
	if data, err := unwrap(cmd, res.Stdout); err == nil {
		_ = json.Unmarshal(data, &payload)
	}
	if payload.Status != status {
		return &Error{Class: ClassRejected, Command: cmd, Message: "bd did not confirm the dependency change", Stderr: clip(res.Stderr)}
	}
	return nil
}
