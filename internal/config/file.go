package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

const configHeader = "# written by bdash; comments are not preserved\n"

// Loaded is the result of reading config.toml. Loading never fails: problems
// become Warnings and the affected keys keep their defaults.
type Loaded struct {
	Settings Settings
	// Set holds the keys present and valid in the file.
	Set map[string]bool
	// Warnings are shown once in the footer.
	Warnings []string
	// Broken is true after a syntax error: the session runs on defaults and
	// bdash never writes the file.
	Broken bool
}

// Load reads and validates the config file. A missing file is not a problem.
func Load(path string) Loaded {
	out := Loaded{Settings: Defaults(), Set: map[string]bool{}}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return out
	case err != nil:
		out.Broken = true
		out.Warnings = append(out.Warnings, fmt.Sprintf("config not read (%v); using defaults", err))
		return out
	}
	raw := map[string]any{}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		out.Broken = true
		out.Warnings = append(out.Warnings, fmt.Sprintf("config has a syntax error (%v); using defaults and not saving changes", err))
		return out
	}
	flat := map[string]any{}
	flatten("", raw, flat)
	names := make([]string, 0, len(flat))
	for n := range flat {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		k, ok := lookupKey(name)
		if !ok {
			out.Warnings = append(out.Warnings, fmt.Sprintf("config: unknown key %q ignored", name))
			continue
		}
		v, err := k.parse(flat[name])
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("config: %s: %v; using the default", name, err))
			continue
		}
		k.set(&out.Settings, v)
		out.Set[name] = true
	}
	return out
}

// flatten turns nested tables into dotted keys. Arrays and scalars are leaves.
func flatten(prefix string, in map[string]any, out map[string]any) {
	for k, v := range in {
		name := prefix + k
		if sub, ok := v.(map[string]any); ok {
			flatten(name+".", sub, out)
			continue
		}
		out[name] = v
	}
}

// ErrReadOnly is returned by Store.Set after a syntax error: bdash never
// writes a config file it could not parse.
var ErrReadOnly = errors.New("config: file has a syntax error, not writing")

// Store writes single settings keys to config.toml.
type Store struct {
	path     string
	readOnly bool
}

// NewStore returns a store for path. Pass Loaded.Broken as readOnly.
func NewStore(path string, readOnly bool) *Store { return &Store{path: path, readOnly: readOnly} }

// Set stores one key: it re-reads the file, changes only that key, drops it
// when the value equals the default, and replaces the file atomically. Unknown
// keys survive. Invalid values are rejected.
func (s *Store) Set(name string, value any) error {
	if s.readOnly {
		return ErrReadOnly
	}
	k, ok := lookupKey(name)
	if !ok {
		return fmt.Errorf("config: unknown key %q", name)
	}
	v, err := k.parse(value)
	if err != nil {
		return fmt.Errorf("config: %s: %w", name, err)
	}

	raw := map[string]any{}
	switch data, err := os.ReadFile(s.path); {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	default:
		if _, err := toml.Decode(string(data), &raw); err != nil {
			s.readOnly = true
			return ErrReadOnly
		}
	}

	def := Defaults()
	if reflect.DeepEqual(v, k.get(&def)) {
		deleteNested(raw, strings.Split(name, "."))
	} else if err := setNested(raw, strings.Split(name, "."), v); err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString(configHeader)
	if len(raw) > 0 {
		buf.WriteString("\n")
		if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
			return err
		}
	}
	return writeFileAtomic(s.path, buf.Bytes(), 0o644)
}

func setNested(m map[string]any, path []string, v any) error {
	for _, p := range path[:len(path)-1] {
		next, ok := m[p]
		if !ok {
			sub := map[string]any{}
			m[p] = sub
			m = sub
			continue
		}
		sub, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("config: %q is not a table", p)
		}
		m = sub
	}
	m[path[len(path)-1]] = v
	return nil
}

func deleteNested(m map[string]any, path []string) {
	if len(path) == 1 {
		delete(m, path[0])
		return
	}
	sub, ok := m[path[0]].(map[string]any)
	if !ok {
		return
	}
	deleteNested(sub, path[1:])
	if len(sub) == 0 {
		delete(m, path[0])
	}
}
