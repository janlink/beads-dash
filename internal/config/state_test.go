package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/janlink/beads-dash/internal/config"
)

func newState(t *testing.T) (*config.State, config.Paths) {
	t.Helper()
	dir := t.TempDir()
	p := config.Paths{
		StateDir:       filepath.Join(dir, "state"),
		HistoryFile:    filepath.Join(dir, "state", "history"),
		WorkspacesFile: filepath.Join(dir, "state", "workspaces.json"),
	}
	return config.NewState(p), p
}

func TestHistoryKeepsLast100(t *testing.T) {
	s, _ := newState(t)
	if got := s.History(); len(got) != 0 {
		t.Fatalf("empty history = %v", got)
	}
	for i := range 130 {
		if err := s.AppendHistory(fmt.Sprintf("cmd %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	got := s.History()
	if len(got) != config.HistoryLimit || got[0] != "cmd 30" || got[99] != "cmd 129" {
		t.Errorf("len %d first %q last %q", len(got), got[0], got[len(got)-1])
	}
}

func TestHistoryNormalisesEntries(t *testing.T) {
	s, _ := newState(t)
	for _, c := range []string{"  status open ", "", "   ", "status open", "a\nb", "a b"} {
		if err := s.AppendHistory(c); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := s.History(), []string{"status open", "a b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestJournalAnswerPerWorkspace(t *testing.T) {
	s, p := newState(t)
	if got := s.Journal("/w/a"); got != config.JournalUnasked {
		t.Errorf("unasked = %q", got)
	}
	if err := s.SetJournal("/w/a", config.JournalDeclined); err != nil {
		t.Fatal(err)
	}
	if err := s.SetJournal("/w/b", config.JournalEnabled); err != nil {
		t.Fatal(err)
	}
	if s.Journal("/w/a") != config.JournalDeclined || s.Journal("/w/b") != config.JournalEnabled || s.Journal("/w/c") != config.JournalUnasked {
		t.Error("answers mixed up")
	}
	if err := s.SetJournal("/w/a", config.JournalUnasked); err != nil {
		t.Fatal(err)
	}
	if s.Journal("/w/a") != config.JournalUnasked {
		t.Error("answer not cleared")
	}
	if _, err := os.Stat(p.WorkspacesFile); err != nil {
		t.Error(err)
	}
}

func TestSetJournalKeepsUnknownFields(t *testing.T) {
	s, p := newState(t)
	if err := os.MkdirAll(p.StateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.WorkspacesFile, []byte(`{"/w/a":{"future":1},"/w/z":{"journal":"declined"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetJournal("/w/a", config.JournalEnabled); err != nil {
		t.Fatal(err)
	}
	body := readFile(t, p.WorkspacesFile)
	for _, want := range []string{`"future": 1`, `"/w/z"`, `"journal": "enabled"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in\n%s", want, body)
		}
	}
}

func TestSetJournalNeverOverwritesBrokenFile(t *testing.T) {
	s, p := newState(t)
	if err := os.MkdirAll(p.StateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const broken = "{not json"
	if err := os.WriteFile(p.WorkspacesFile, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetJournal("/w/a", config.JournalEnabled); err == nil {
		t.Error("write over a broken file succeeded")
	}
	if readFile(t, p.WorkspacesFile) != broken {
		t.Error("broken file was modified")
	}
	if s.Journal("/w/a") != config.JournalUnasked {
		t.Error("broken file must read as unasked")
	}
}
