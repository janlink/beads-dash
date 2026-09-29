package main

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// resolveActor picks the name bd credits changes to: BEADS_ACTOR, then git's
// user.name, then $USER.
func resolveActor(getenv func(string) string, gitUser func() string) string {
	if a := strings.TrimSpace(getenv("BEADS_ACTOR")); a != "" {
		return a
	}
	if a := strings.TrimSpace(gitUser()); a != "" {
		return a
	}
	return strings.TrimSpace(getenv("USER"))
}

func gitUserName() string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "config", "user.name").Output() //nolint:forbidigo // git, not bd: the confinement guards bd calls
	if err != nil {
		return ""
	}
	return string(out)
}
