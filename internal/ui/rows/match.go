package rows

import (
	"slices"
	"strings"
	"unicode"

	"github.com/janlink/beads-dash/internal/model"
	"github.com/janlink/beads-dash/internal/theme"
)

type needle struct {
	runes []rune
	exact bool
}

// SetMatch sets the free-text terms of the scope; ID and title cells paint
// their occurrences in the match role. Changed terms invalidate the cached
// rows.
func (r *Renderer) SetMatch(terms []model.MatchTerm) {
	next := make([]needle, 0, len(terms))
	for _, t := range terms {
		n := needle{[]rune(t.Text), t.Exact}
		if !t.Exact {
			n.runes = fold(n.runes)
		}
		next = append(next, n)
	}
	if slices.EqualFunc(next, r.terms, func(a, b needle) bool { return a.exact == b.exact && slices.Equal(a.runes, b.runes) }) {
		return
	}
	r.terms = next
	clear(r.cache)
}

func fold(rs []rune) []rune {
	out := make([]rune, len(rs))
	for i, c := range rs {
		out[i] = unicode.ToLower(c)
	}
	return out
}

// hl paints s in role with the occurrences of the scope's terms in the match
// role. Cell widths do not change.
func (r *Renderer) hl(paint func(theme.Role, string) string, role theme.Role, s string) string {
	if len(r.terms) == 0 {
		return paint(role, s)
	}
	runes := []rune(s)
	var lower []rune
	marked := make([]bool, len(runes))
	hit := false
	for _, t := range r.terms {
		hay := runes
		if !t.exact {
			if lower == nil {
				lower = fold(runes)
			}
			hay = lower
		}
		n := len(t.runes)
		for i := 0; n > 0 && i+n <= len(hay); {
			if slices.Equal(hay[i:i+n], t.runes) {
				for j := i; j < i+n; j++ {
					marked[j] = true
				}
				hit = true
				i += n
				continue
			}
			i++
		}
	}
	if !hit {
		return paint(role, s)
	}
	var b strings.Builder
	for i := 0; i < len(runes); {
		j := i
		for j < len(runes) && marked[j] == marked[i] {
			j++
		}
		seg := string(runes[i:j])
		if marked[i] {
			b.WriteString(paint(theme.Match, seg))
		} else {
			b.WriteString(paint(role, seg))
		}
		i = j
	}
	return b.String()
}
