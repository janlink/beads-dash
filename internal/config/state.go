package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode/utf8"
)

// HistoryLimit is how many entries the history file keeps for each kind of
// entry, a kind being the first character of the line.
const HistoryLimit = 100

// JournalAnswer records what the viewer said to the events-journal opt-in
// prompt for one workspace.
type JournalAnswer string

const (
	JournalUnasked   JournalAnswer = ""
	JournalEnabled   JournalAnswer = "enabled"
	JournalDeclined  JournalAnswer = "declined"
	workspaceJournal               = "journal"
)

// State is the machine-written state directory: command history and per
// workspace answers. Every write re-reads the file and replaces it atomically.
type State struct {
	historyMu      sync.Mutex
	historyFile    string
	workspacesFile string
}

// NewState returns the state files named by paths. With empty paths every
// method is a no-op, so callers never need a nil check.
func NewState(p Paths) *State {
	return &State{historyFile: p.HistoryFile, workspacesFile: p.WorkspacesFile}
}

// History returns the stored history, oldest first.
func (s *State) History() []string {
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	return s.readHistory()
}

func (s *State) readHistory() []string {
	if s.historyFile == "" {
		return nil
	}
	data, err := os.ReadFile(s.historyFile)
	if err != nil {
		return nil
	}
	return capPerKind(splitLines(string(data)), HistoryLimit)
}

// AppendHistory adds one entry, keeping the last HistoryLimit of its kind.
// Blank entries and immediate repeats within a kind are skipped.
func (s *State) AppendHistory(entry string) error {
	entry = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(entry))
	if entry == "" || s.historyFile == "" {
		return nil
	}
	s.historyMu.Lock()
	defer s.historyMu.Unlock()
	lines := s.readHistory()
	kind := kindOf(entry)
	for i := len(lines) - 1; i >= 0; i-- {
		if kindOf(lines[i]) == kind {
			if lines[i] == entry {
				return nil
			}
			break
		}
	}
	lines = capPerKind(append(lines, entry), HistoryLimit)
	return writeFileAtomic(s.historyFile, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func kindOf(line string) rune {
	r, _ := utf8.DecodeRuneInString(line)
	return r
}

func splitLines(data string) []string {
	var lines []string
	for _, l := range strings.Split(data, "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// capPerKind keeps the last n lines of each kind, in their original order.
func capPerKind(lines []string, n int) []string {
	seen := map[rune]int{}
	keep := make([]bool, len(lines))
	kept := 0
	for i := len(lines) - 1; i >= 0; i-- {
		k := kindOf(lines[i])
		if seen[k] < n {
			seen[k]++
			keep[i] = true
			kept++
		}
	}
	out := make([]string, 0, kept)
	for i, l := range lines {
		if keep[i] {
			out = append(out, l)
		}
	}
	return out
}

// Journal returns the stored opt-in answer for a workspace, keyed by the
// absolute path bd reports.
func (s *State) Journal(workspace string) JournalAnswer {
	if s.workspacesFile == "" {
		return JournalUnasked
	}
	entries, err := s.readWorkspaces()
	if err != nil {
		return JournalUnasked
	}
	v, _ := entries[workspace][workspaceJournal].(string)
	switch a := JournalAnswer(v); a {
	case JournalEnabled, JournalDeclined:
		return a
	case JournalUnasked:
	}
	return JournalUnasked
}

// SetJournal stores the opt-in answer for a workspace, leaving other entries
// and fields alone. It refuses to overwrite a workspaces file it cannot parse.
func (s *State) SetJournal(workspace string, a JournalAnswer) error {
	if workspace == "" {
		return errors.New("config: empty workspace path")
	}
	if s.workspacesFile == "" {
		return nil
	}
	entries, err := s.readWorkspaces()
	if err != nil {
		return err
	}
	entry := entries[workspace]
	if entry == nil {
		entry = map[string]any{}
		entries[workspace] = entry
	}
	if a == JournalUnasked {
		delete(entry, workspaceJournal)
	} else {
		entry[workspaceJournal] = string(a)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.workspacesFile, append(data, '\n'), 0o600)
}

func (s *State) readWorkspaces() (map[string]map[string]any, error) {
	entries := map[string]map[string]any{}
	data, err := os.ReadFile(s.workspacesFile)
	if errors.Is(err, os.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("config: %s is not valid JSON, not touching it: %w", s.workspacesFile, err)
	}
	if entries == nil {
		entries = map[string]map[string]any{}
	}
	return entries, nil
}
