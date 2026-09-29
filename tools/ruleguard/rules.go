//go:build ruleguard

package gorules

import "github.com/quasilyte/go-ruleguard/dsl"

func noShell(m dsl.Matcher) {
	m.Match(
		`exec.Command($sh, $*_)`,
		`exec.CommandContext($_, $sh, $*_)`,
	).
		Where(m["sh"].Text.Matches(`^"([^"]*[/\\])?(sh|bash|zsh|fish|dash|ash|ksh|cmd|powershell|pwsh)(\.exe)?"$`)).
		Report(`never run bd or anything else through a shell`)
}
