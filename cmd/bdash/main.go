// Command bdash is a terminal dashboard for the bd (beads) issue tracker.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/janlink/beads-dash/internal/ui"
)

func main() {
	if _, err := tea.NewProgram(ui.New()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "bdash:", err)
		os.Exit(1)
	}
}
