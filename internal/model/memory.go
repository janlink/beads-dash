package model

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Memory is a keyed note stored in the workspace. bd keeps no timestamp for
// it.
type Memory struct {
	Key     string
	Content string
}

// SortMemories orders memories by key, byte-wise: the order bd prime prints
// them in, and case-sensitive like bd's keys.
func SortMemories(ms []Memory) {
	slices.SortFunc(ms, func(a, b Memory) int { return strings.Compare(a.Key, b.Key) })
}

// FirstLine is the first line of content that holds anything but space,
// trimmed.
func FirstLine(content string) string {
	for _, l := range strings.Split(content, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// MemorySize counts the lines and characters of content.
func MemorySize(content string) (lines, chars int) {
	chars = utf8.RuneCountInString(content)
	if content == "" {
		return 0, 0
	}
	return strings.Count(strings.TrimRight(content, "\n"), "\n") + 1, chars
}

// primeOverhead is the text bd prime adds once when any memory exists: the
// section heading with a one-digit count and the sentence on bd remember.
const primeOverhead = 187

// PrimeChars is what the memories add to the output of bd prime, in
// characters: the section overhead, then per memory a heading line with the
// key, the content and the separating newlines.
func PrimeChars(ms []Memory) int {
	if len(ms) == 0 {
		return 0
	}
	n := primeOverhead + len(strconv.Itoa(len(ms))) - 1
	for _, m := range ms {
		n += len("### ") + utf8.RuneCountInString(m.Key) + 1 + utf8.RuneCountInString(m.Content) + 2
	}
	return n
}

// CompactCount writes n as 812 or 3.4k.
func CompactCount(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	s := strconv.FormatFloat(float64(n)/1000, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0") + "k"
}

// MemoryQuery is a memory search: every word or quoted phrase must occur in
// the key or the content, as a substring, case-insensitive unless the term
// has an uppercase letter (smartcase).
type MemoryQuery struct {
	text  string
	terms []MatchTerm
}

// ParseMemoryQuery parses a search text.
func ParseMemoryQuery(q string) MemoryQuery {
	mq := MemoryQuery{text: strings.TrimSpace(q)}
	for _, tok := range tokenize(q) {
		text := tok.text
		if strings.HasPrefix(text, `"`) {
			text = unquote(text)
		}
		if text = strings.TrimSpace(text); text != "" {
			mq.terms = append(mq.terms, MatchTerm{text, strings.IndexFunc(text, unicode.IsUpper) >= 0})
		}
	}
	return mq
}

// Text is the query as typed, trimmed.
func (q MemoryQuery) Text() string { return q.text }

// Active reports whether the query narrows anything.
func (q MemoryQuery) Active() bool { return len(q.terms) > 0 }

// Terms lists the terms, for highlighting.
func (q MemoryQuery) Terms() []MatchTerm { return slices.Clone(q.terms) }

// Match reports whether m satisfies every term.
func (q MemoryQuery) Match(m Memory) bool {
	for _, t := range q.terms {
		if !containsTerm(m.Key, t) && !containsTerm(m.Content, t) {
			return false
		}
	}
	return true
}

func containsTerm(s string, t MatchTerm) bool {
	if t.Exact {
		return strings.Contains(s, t.Text)
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(t.Text))
}

// Filter keeps the memories that match, in order.
func (q MemoryQuery) Filter(ms []Memory) []Memory {
	if !q.Active() {
		return ms
	}
	var out []Memory
	for _, m := range ms {
		if q.Match(m) {
			out = append(out, m)
		}
	}
	return out
}

// MemoryChange is what differs between two reads of the memories.
type MemoryChange struct {
	// Changed are the keys that are new or hold other content, in the new
	// order.
	Changed []string
	// Removed are the keys that are gone, in the old order.
	Removed []string
}

// DiffMemories compares two reads.
func DiffMemories(old, cur []Memory) MemoryChange {
	before := make(map[string]string, len(old))
	for _, m := range old {
		before[m.Key] = m.Content
	}
	now := make(map[string]struct{}, len(cur))
	var ch MemoryChange
	for _, m := range cur {
		now[m.Key] = struct{}{}
		if c, ok := before[m.Key]; !ok || c != m.Content {
			ch.Changed = append(ch.Changed, m.Key)
		}
	}
	for _, m := range old {
		if _, ok := now[m.Key]; !ok {
			ch.Removed = append(ch.Removed, m.Key)
		}
	}
	return ch
}

const (
	slugWords = 8
	slugRunes = 60
)

// MemorySlug suggests a key for content the way bd does when given none:
// lower case, the first eight words joined by dashes.
func MemorySlug(content string) string {
	words := strings.FieldsFunc(strings.ToLower(content), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) > slugWords {
		words = words[:slugWords]
	}
	s := strings.Join(words, "-")
	if r := []rune(s); len(r) > slugRunes {
		s = strings.TrimRight(string(r[:slugRunes]), "-")
	}
	return s
}
