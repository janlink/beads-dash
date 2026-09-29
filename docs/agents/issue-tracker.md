# Issue tracker

This repo tracks all work in **bd (Beads)**, local to the repo (`.beads/`, no remote). Use the `bd` CLI only; never TodoWrite or markdown TODO files. All issue text is written in English.

## Basics

- Create: `bd create --title="..." --description="..." --type=<type> --priority=<0-4> [--labels=a,b] [--parent=<id>]`
- Show: `bd show <id> --json`
- Comment: `bd comment <id> --file <path>` (or `--stdin`)
- Close: `bd close <id> --reason="..."`
- Never use `bd edit` (opens `$EDITOR`).

## Wayfinding operations

| Wayfinder concept | bd expression |
|---|---|
| Map | issue of type `epic`, label `wayfinder:map`; body = map body (`--body-file`) |
| Map reference material | the map's `design` field (e.g. the baseline plan, `--design-file`) |
| Ticket | child issue: `bd create --parent=<map-id> --labels=wayfinder:<type>` |
| Ticket type → bd type | `research` → `spike`; `grilling`, `prototype` → `decision`; `task` → `task` |
| Claim | `bd update <id> --assignee=janlink` (assignee is the claim; do not use `--claim`, it also flips status) |
| Blocking | native: `bd dep add <blocked-id> <blocker-id>` (type `blocks`) |
| Frontier | `bd ready --parent <map-id> --unassigned --json` |
| All open tickets | `bd list --parent <map-id> --status open --json` |
| Resolution | `bd comment <id> --file <answer.md>`, then `bd close <id> --reason="<one-line gist>"`, then append a line to the map's *Decisions so far* (`bd update <map-id> --body-file ...`) |
| Assets | long findings (research reports, prototype notes) go in a separate comment on the ticket before the resolution comment; code prototypes live on a `prototype/<name>` branch linked from the ticket |
| Out of scope | `bd close <id> --reason="out of scope: ..."`, plus a line in the map's *Out of scope* |

Refer to tickets by title in anything a human reads; the id rides along in parentheses.
