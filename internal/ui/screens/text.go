package screens

import (
	"strings"
)

// Text renders a report as plain text: the checklist the startup screen
// shows, for bdash -v.
func Text(r Report) string {
	var b strings.Builder
	b.WriteString(r.Title + "\n")
	if r.What != "" {
		b.WriteString(r.What + "\n")
	}
	b.WriteString("\n")
	nameW := 0
	for _, c := range r.Checks {
		nameW = max(nameW, len(c.Name))
	}
	for _, c := range r.Checks {
		mark := "-"
		switch c.State {
		case Passed:
			mark = "+"
		case Failed:
			mark = "x"
		case NotRun:
		}
		b.WriteString(mark + " " + pad(c.Name, nameW) + "  " + c.Detail + "\n")
	}
	if len(r.Fixes) > 0 {
		b.WriteString("\nWhat to do\n")
		for _, f := range r.Fixes {
			b.WriteString("  $ " + f.Cmd + "  # " + f.Why + "\n")
		}
	}
	if r.After != "" {
		b.WriteString("\n" + r.After + "\n")
	}
	if r.Ran != "" || len(r.Raw) > 0 {
		b.WriteString("\nbd said  " + r.Ran + "\n")
		for _, line := range r.Raw {
			b.WriteString("| " + line + "\n")
		}
	}
	return b.String()
}
