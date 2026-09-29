package model

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash"
	"sort"
	"strings"
)

var volatileRootKeys = []string{"dependency_count", "dependent_count"}

var volatileEdgeKeys = []string{"created_at", "created_by", "metadata"}

// canonicalRaw renders bd's JSON for one issue in a stable form: sorted
// top-level keys, sorted labels and edges, without per-call derived values.
// It returns "" for an empty or invalid input. Values below the top level are
// kept as bd wrote them; bd's encoder emits those deterministically.
func canonicalRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, k := range volatileRootKeys {
		delete(m, k)
	}
	if v, ok := m["dependencies"]; ok {
		m["dependencies"] = canonicalEdges(v)
	}
	if v, ok := m["labels"]; ok {
		m["labels"] = sortedElements(v)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b []byte
	for _, k := range keys {
		b = append(b, k...)
		b = append(b, 0)
		b = append(b, m[k]...)
		b = append(b, 0)
	}
	return string(b)
}

func canonicalEdges(v json.RawMessage) json.RawMessage {
	var edges []map[string]json.RawMessage
	if json.Unmarshal(v, &edges) != nil {
		return v
	}
	parts := make([]string, len(edges))
	for i, e := range edges {
		for _, k := range volatileEdgeKeys {
			delete(e, k)
		}
		keys := make([]string, 0, len(e))
		for k := range e {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b []byte
		for _, k := range keys {
			b = append(b, k...)
			b = append(b, 0)
			b = append(b, e[k]...)
			b = append(b, 0)
		}
		parts[i] = string(b)
	}
	sort.Strings(parts)
	return json.RawMessage(strings.Join(parts, "\x01"))
}

func sortedElements(v json.RawMessage) json.RawMessage {
	var elems []json.RawMessage
	if json.Unmarshal(v, &elems) != nil {
		return v
	}
	parts := make([]string, len(elems))
	for i, e := range elems {
		parts[i] = string(e)
	}
	sort.Strings(parts)
	return json.RawMessage(strings.Join(parts, "\x01"))
}

type fingerprintHasher struct{ h hash.Hash }

func (f *fingerprintHasher) init() { f.h = sha256.New() }

func (f *fingerprintHasher) str(s string) {
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(len(s)))
	f.h.Write(n[:])
	f.h.Write([]byte(s))
}

func (f *fingerprintHasher) strs(ss ...string) {
	for _, s := range ss {
		f.str(s)
	}
}

func (f *fingerprintHasher) ints(vs ...int64) {
	var n [8]byte
	for _, v := range vs {
		binary.LittleEndian.PutUint64(n[:], uint64(v))
		f.h.Write(n[:])
	}
}

func (f *fingerprintHasher) list(ss []string) {
	f.ints(int64(len(ss)))
	f.strs(ss...)
}

func (f *fingerprintHasher) sum() string { return hex.EncodeToString(f.h.Sum(nil)) }
