package model

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unicode/utf8"
)

func mems(kv ...string) []Memory {
	var out []Memory
	for i := 0; i < len(kv); i += 2 {
		out = append(out, Memory{kv[i], kv[i+1]})
	}
	return out
}

func keysOf(ms []Memory) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Key)
	}
	return out
}

func TestSortMemoriesIsByteWise(t *testing.T) {
	ms := mems("weird.key", "", "-dash", "", "Weird.Key", "", "alpha", "", "Zed", "", "spaced key", "")
	SortMemories(ms)
	want := []string{"-dash", "Weird.Key", "Zed", "alpha", "spaced key", "weird.key"}
	if got := keysOf(ms); !reflect.DeepEqual(got, want) {
		t.Errorf("order = %q, want %q", got, want)
	}
}

func TestFirstLineSkipsBlankLines(t *testing.T) {
	for in, want := range map[string]string{
		"one\ntwo": "one", "\n\n  lead  \nx": "lead", "": "", " \n ": "",
	} {
		if got := FirstLine(in); got != want {
			t.Errorf("FirstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMemorySize(t *testing.T) {
	for _, c := range []struct {
		in           string
		lines, chars int
	}{{"", 0, 0}, {"abc", 1, 3}, {"a\nb\n", 2, 4}, {"äö", 1, 2}} {
		if l, n := MemorySize(c.in); l != c.lines || n != c.chars {
			t.Errorf("MemorySize(%q) = %d, %d; want %d, %d", c.in, l, n, c.lines, c.chars)
		}
	}
}

func TestPrimeCharsMatchesCapturedBdPrime(t *testing.T) {
	dirs, err := filepath.Glob("../../testdata/bd-*/m8")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	ms := mems("k1", "hello world", "k2", "two\nlines ä")
	for _, dir := range dirs {
		read := func(name string) int {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			return utf8.RuneCount(b)
		}
		if got, want := PrimeChars(ms), read("prime-two.txt")-read("prime-none.txt"); got != want {
			t.Errorf("%s: PrimeChars = %d, bd prime grows by %d", dir, got, want)
		}
	}
	if PrimeChars(nil) != 0 {
		t.Error("no memories cost nothing")
	}
}

func TestCompactCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1k", 3400: "3.4k", 12345: "12.3k"} {
		if got := CompactCount(n); got != want {
			t.Errorf("CompactCount(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestMemoryQuerySmartcaseAnd(t *testing.T) {
	ms := mems("dolt-tips", "Use the Dolt server", "other", "nothing here", "Dolt", "x")
	for q, want := range map[string][]string{
		"":                {"dolt-tips", "other", "Dolt"},
		"dolt":            {"dolt-tips", "Dolt"},
		"Dolt":            {"dolt-tips", "Dolt"},
		"DOLT":            nil,
		"dolt server":     {"dolt-tips"},
		`"the dolt"`:      {"dolt-tips"},
		`"dolt the"`:      nil,
		"dolt missing":    nil,
		"NOTHING":         nil,
		"nothing   here ": {"other"},
	} {
		got := keysOf(ParseMemoryQuery(q).Filter(ms))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("query %q = %q, want %q", q, got, want)
		}
	}
}

func TestMemoryQueryTermsForHighlight(t *testing.T) {
	q := ParseMemoryQuery(`dolt "Two Words"`)
	want := []MatchTerm{{"dolt", false}, {"Two Words", true}}
	if got := q.Terms(); !reflect.DeepEqual(got, want) || !q.Active() || q.Text() != `dolt "Two Words"` {
		t.Errorf("terms = %v", got)
	}
	if ParseMemoryQuery("  ").Active() {
		t.Error("blank query is active")
	}
}

func TestDiffMemories(t *testing.T) {
	old := mems("a", "1", "b", "2", "c", "3")
	cur := mems("a", "1", "b", "changed", "d", "4")
	ch := DiffMemories(old, cur)
	if !reflect.DeepEqual(ch.Changed, []string{"b", "d"}) || !reflect.DeepEqual(ch.Removed, []string{"c"}) {
		t.Errorf("diff = %+v", ch)
	}
	if ch := DiffMemories(old, old); len(ch.Changed)+len(ch.Removed) != 0 {
		t.Errorf("no change = %+v", ch)
	}
}

func TestMemorySlug(t *testing.T) {
	for in, want := range map[string]string{
		"some content words here for the slug and more": "some-content-words-here-for-the-slug-and",
		"  Hello, World!  ":                             "hello-world",
		"-content":                                      "content",
		"!!!":                                           "",
		"Straße über":                                   "straße-über",
	} {
		if got := MemorySlug(in); got != want {
			t.Errorf("MemorySlug(%q) = %q, want %q", in, got, want)
		}
	}
}
