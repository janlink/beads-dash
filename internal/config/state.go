package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// HistoryLimit is how many command-bar entries the history file keeps.
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
	historyFile    string
	workspacesFile string
}

// NewState returns the state files named by paths. With empty paths every
// method is a no-op, so callers never need a nil check.
func NewState(p Paths) *State {
	return &State{historyFile: p.HistoryFile, workspacesFile: p.WorkspacesFile}
}

// History returns the stored command-bar history, oldest first.
func (s *State) History() []string {
	if s.historyFile == "" {
		return nil
	}
	data, err := os.ReadFile(s.historyFile)
	if err != nil {
		return nil
	}
	return lastLines(string(data), HistoryLimit)
}

// AppendHistory adds one command, keeping the last HistoryLimit entries.
// Blank commands and immediate repeats are skipped.
func (s *State) AppendHistory(cmd string) error {
	cmd = strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(cmd))
	if cmd == "" || s.historyFile == "" {
		return nil
	}
	lines := s.History()
	if len(lines) > 0 && lines[len(lines)-1] == cmd {
		return nil
	}
	lines = append(lines, cmd)
	if len(lines) > HistoryLimit {
		lines = lines[len(lines)-HistoryLimit:]
	}
	return writeFileAtomic(s.historyFile, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func lastLines(data string, n int) []string {
	var lines []string
	for _, l := range strings.Split(data, "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
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
