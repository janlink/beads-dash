package ui

import "github.com/janlink/beads-dash/internal/config"

// recall is the history of one bar, oldest first, with a cursor for the
// Up/Down browsing. The text typed before browsing started is kept as the
// draft and comes back when browsing ends.
type recall struct {
	items []string
	at    int
	draft string
}

func newRecall(items []string) *recall {
	r := &recall{items: append([]string(nil), items...)}
	r.reset()
	return r
}

func (r *recall) reset() {
	r.at = len(r.items)
	r.draft = ""
}

func (r *recall) browsing() bool { return r.at < len(r.items) }

// older steps back in time; cur is remembered as the draft on the first step.
func (r *recall) older(cur string) (string, bool) {
	if r.at == 0 {
		return "", false
	}
	if !r.browsing() {
		r.draft = cur
	}
	r.at--
	return r.items[r.at], true
}

// newer steps forward, ending at the draft.
func (r *recall) newer() (string, bool) {
	if !r.browsing() {
		return "", false
	}
	r.at++
	if !r.browsing() {
		return r.draft, true
	}
	return r.items[r.at], true
}

// add records s unless it repeats the last entry, keeping the last
// config.HistoryLimit.
func (r *recall) add(s string) {
	defer r.reset()
	if s == "" || (len(r.items) > 0 && r.items[len(r.items)-1] == s) {
		return
	}
	r.items = append(r.items, s)
	if len(r.items) > config.HistoryLimit {
		r.items = r.items[len(r.items)-config.HistoryLimit:]
	}
}

// History is the persisted bar history: one line per entry, oldest first.
type History interface {
	History() []string
	AppendHistory(entry string) error
}

// The entries of the shared history file start with the mark of the bar
// they came from.
const (
	searchMark  = "/"
	commandMark = ":"
)

func splitHistory(all []string) (search, command []string) {
	for _, e := range all {
		switch {
		case len(e) > 1 && e[:1] == searchMark:
			search = append(search, e[1:])
		case len(e) > 1 && e[:1] == commandMark:
			command = append(command, e[1:])
		}
	}
	return search, command
}
