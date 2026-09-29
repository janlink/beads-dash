package ui

import (
	"slices"
	"testing"

	"github.com/janlink/beads-dash/internal/config"
)

func TestRecallBrowsesAndRestoresDraft(t *testing.T) {
	r := newRecall([]string{"a", "b", "c"})
	if _, ok := r.newer(); ok {
		t.Fatal("newer without browsing should do nothing")
	}
	got := []string{}
	for range 4 {
		if s, ok := r.older("draft"); ok {
			got = append(got, s)
		}
	}
	if !slices.Equal(got, []string{"c", "b", "a"}) {
		t.Fatalf("older = %v", got)
	}
	var fwd []string
	for {
		s, ok := r.newer()
		if !ok {
			break
		}
		fwd = append(fwd, s)
	}
	if !slices.Equal(fwd, []string{"b", "c", "draft"}) {
		t.Fatalf("newer = %v", fwd)
	}
	if r.browsing() {
		t.Fatal("still browsing at the draft")
	}
}

func TestRecallAddSkipsRepeatsAndCaps(t *testing.T) {
	r := newRecall(nil)
	r.add("x")
	r.add("x")
	r.add("")
	if len(r.items) != 1 {
		t.Fatalf("items = %v", r.items)
	}
	for i := range config.HistoryLimit + 10 {
		r.add(string(rune('A'+i%26)) + string(rune('a'+i/26)))
	}
	if len(r.items) != config.HistoryLimit {
		t.Fatalf("len = %d", len(r.items))
	}
}

func TestSplitHistory(t *testing.T) {
	s, c := splitHistory([]string{"/status:open", ":view tree", "junk", "/", ":clear"})
	if !slices.Equal(s, []string{"status:open"}) || !slices.Equal(c, []string{"view tree", "clear"}) {
		t.Fatalf("search %v command %v", s, c)
	}
}
