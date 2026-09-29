// Command bdash is a terminal dashboard for the bd (beads) issue tracker.
package main

import "os"

// Set by the linker (goreleaser).
var (
	version = ""
	commit  = ""
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, systemDeps()))
}
