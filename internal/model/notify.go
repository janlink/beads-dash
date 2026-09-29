package model

// HighlightIDs is the change-highlight trigger set: every issue that had an
// activity event in one refresh, in first-seen order, except issues that
// were deleted and prefill events.
func HighlightIDs(events []Event) []string {
	deleted := map[string]struct{}{}
	for _, e := range events {
		if e.Kind == KindDeleted && !e.Prefill {
			deleted[e.IssueID] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	var out []string
	for _, e := range events {
		if e.Prefill {
			continue
		}
		if _, gone := deleted[e.IssueID]; gone {
			continue
		}
		if _, dup := seen[e.IssueID]; dup {
			continue
		}
		seen[e.IssueID] = struct{}{}
		out = append(out, e.IssueID)
	}
	return out
}

// SummaryThreshold is how many candidate events one refresh may produce
// before they collapse into a single summary notification.
const SummaryThreshold = 3

// Notification is the data of what to tell the viewer about one refresh.
// Delivering it is someone else's job.
type Notification struct {
	// Events are the candidates, in event order.
	Events []Event
	// Summary is set when there are more than [SummaryThreshold] events, so
	// they are announced together.
	Summary bool
}

// Notify picks the notification candidates among the events of one refresh:
// kinds in the set only, never prefill and never the effect of the viewer's
// own write on the written issue. An empty result means nothing to say.
func Notify(events []Event, kinds KindSet) Notification {
	var n Notification
	for _, e := range events {
		if e.Prefill || e.Own || !kinds.Has(e.Kind) {
			continue
		}
		n.Events = append(n.Events, e)
	}
	n.Summary = len(n.Events) > SummaryThreshold
	return n
}
