package proc

import (
	"context"
	"os/exec"
	"strings"
	"sync"
)

// Call is one command a [Fake] was asked to run.
type Call struct {
	Argv  []string
	Stdin []byte
}

// Line is the command as one string.
func (c Call) Line() string { return strings.Join(c.Argv, " ") }

// Fake is a [Runner] for tests: it knows which programs exist, records every
// run and answers with canned results.
type Fake struct {
	mu    sync.Mutex
	paths map[string]bool
	calls []Call
	rules []rule
}

type rule struct {
	prefix string
	err    error
	out    string
}

// NewFake returns a fake on which only the named programs are found.
func NewFake(programs ...string) *Fake {
	f := &Fake{paths: map[string]bool{}}
	for _, p := range programs {
		f.paths[p] = true
	}
	return f
}

// FailWith makes every run of a command that starts with prefix (matched
// against the command line) fail with err. The first rule that matches wins.
func (f *Fake) FailWith(prefix string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules = append(f.rules, rule{prefix: prefix, err: err})
}

// Output makes every run of a command starting with prefix print out. The
// first rule that matches wins.
func (f *Fake) Output(prefix, out string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rules = append(f.rules, rule{prefix: prefix, out: out})
}

// LookPath implements [Runner].
func (f *Fake) LookPath(file string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.paths[file] {
		return "/fake/" + file, nil
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

// Run implements [Runner].
func (f *Fake) Run(_ context.Context, argv []string, stdin []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := Call{Argv: append([]string(nil), argv...), Stdin: append([]byte(nil), stdin...)}
	f.calls = append(f.calls, c)
	line := c.Line()
	for _, r := range f.rules {
		if strings.HasPrefix(line, r.prefix) {
			if r.err != nil {
				return nil, r.err
			}
			return []byte(r.out), nil
		}
	}
	return nil, nil
}

// Calls returns the commands run so far.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}
