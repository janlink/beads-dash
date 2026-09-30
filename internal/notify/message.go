// Package notify turns activity events into notifications and delivers them
// through the desktop, the terminal or the bell.
package notify

import (
	"fmt"
	"strings"

	"github.com/janlink/beads-dash/internal/model"
)

// Message is one notification.
type Message struct {
	Title, Body string
}

// Line is the message as one line of text, for the footer.
func (m Message) Line() string {
	if m.Body == "" {
		return m.Title
	}
	return m.Title + ": " + m.Body
}

const appName = "bdash"

// Compose words the notification: one message per event, or one summary
// when the refresh produced more than [model.SummaryThreshold] of them.
func Compose(n model.Notification) []Message {
	switch {
	case len(n.Events) == 0:
		return nil
	case n.Summary || len(n.Events) > model.SummaryThreshold:
		return []Message{summary(n.Events)}
	}
	out := make([]Message, len(n.Events))
	for i, e := range n.Events {
		body := e.Title
		if e.Actor != "" {
			body += " (" + e.Actor + ")"
		}
		out[i] = Message{Title: fmt.Sprintf("%s: %s %s", appName, e.IssueID, e.Kind.Label()), Body: body}
	}
	return out
}

func summary(events []model.Event) Message {
	counts := map[model.Kind]int{}
	for _, e := range events {
		counts[e.Kind]++
	}
	var parts []string
	for _, k := range model.Kinds() {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k.Label()))
		}
	}
	return Message{Title: fmt.Sprintf("%s: %d changes", appName, len(events)), Body: strings.Join(parts, ", ")}
}
