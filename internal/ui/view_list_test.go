package ui

import (
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/ui/keys"
)

func TestMoveTo(t *testing.T) {
	rows := []listRow{{"h1", false}, {"a", true}, {"b", true}, {"h2", false}, {"c", true}, {"d", true}}
	tests := []struct {
		name string
		from int
		act  keys.Action
		page int
		want int
	}{
		{"down skips a header", 2, keys.NavDown, 10, 4},
		{"up skips a header", 4, keys.NavUp, 10, 2},
		{"down at the end stays", 5, keys.NavDown, 10, 5},
		{"up at the top stays", 1, keys.NavUp, 10, 1},
		{"first skips the leading header", 4, keys.NavFirst, 10, 1},
		{"last", 1, keys.NavLast, 10, 5},
		{"page lands on a header, moves on", 1, keys.NavPageDown, 2, 4},
		{"half page up lands on a header, moves on", 5, keys.NavHalfUp, 4, 2},
		{"no cursor starts at the first row", -1, keys.NavDown, 10, 1},
	}
	for _, tt := range tests {
		if got := moveTo(rows, tt.from, tt.act, tt.page); got != tt.want {
			t.Errorf("%s: moveTo(%d, %s) = %d, want %d", tt.name, tt.from, tt.act, got, tt.want)
		}
	}
	if got := moveTo(nil, 0, keys.NavDown, 5); got != -1 {
		t.Errorf("empty list: %d", got)
	}
}

func TestScopeInfoFit(t *testing.T) {
	si := scopeInfo{active: true, query: "label:ui p1", marker: "⌕", ellipsis: "…", shown: 23, total: 140, closedHidden: true, plusClosed: 4, unknown: []string{"foo"}}
	if got, want := si.fit(200), "⌕ label:ui p1 · 23/140 · closed hidden · +4 closed · unknown: foo"; got != want {
		t.Errorf("full label %q, want %q", got, want)
	}
	if got := si.fit(30); len([]rune(got)) != 30 || !strings.Contains(got, "…") {
		t.Errorf("truncated label %q", got)
	}
	long := scopeInfo{active: true, query: "status:open type:bug assignee:alice label:checkout", marker: "⌕", ellipsis: "…", shown: 3, total: 140, closedHidden: true}
	if got := long.fit(50); len([]rune(got)) != 50 || !strings.HasSuffix(got, " · 3/140 · closed hidden") || !strings.HasPrefix(got, "⌕ status:") {
		t.Errorf("only the query is cut: %q", got)
	}
	if si.fit(5) != "" || (scopeInfo{}).fit(100) != "" {
		t.Error("no label when it does not fit or no scope is active")
	}
}
