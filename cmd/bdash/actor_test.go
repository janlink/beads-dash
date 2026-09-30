package main

import "testing"

func TestResolveActorPrecedence(t *testing.T) {
	git := func(s string) func() string { return func() string { return s } }
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		name string
		env  map[string]string
		git  string
		want string
	}{
		{"beads actor wins", map[string]string{"BEADS_ACTOR": "ci-bot", "USER": "jan"}, "Jan Link", "ci-bot"},
		{"bd actor after beads actor", map[string]string{"BD_ACTOR": "bot", "USER": "jan"}, "Jan Link", "bot"},
		{"git user next", map[string]string{"USER": "jan"}, "Jan Link\n", "Jan Link"},
		{"user last", map[string]string{"USER": "jan"}, "", "jan"},
		{"nothing", nil, "", ""},
	}
	for _, tt := range tests {
		if got := resolveActor(env(tt.env), git(tt.git)); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}
