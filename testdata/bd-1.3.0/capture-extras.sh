#!/usr/bin/env bash
# Extra captures on top of ../bd-1.2.2/capture.sh. Usage: capture-extras.sh <ws-seeded-by-capture.sh> <out-dir>
# Mutates the workspace (kv, memories, a dependency cycle), so run it after capture.sh.
set -euo pipefail
WS=$1; OUT=$2
mkdir -p "$OUT/envelope"
cd "$WS"
id() { bd list --all --limit 0 --json | jq -r --arg t "$1" '.[] | select(.title == $t) | .id'; }
E=$(id "Epic A"); A=$(id "Child task A1")

cap() { local name=$1; shift; bd "$@" --json >"$OUT/$name.json" 2>"$OUT/$name.stderr" || true
        BD_JSON_ENVELOPE=1 bd "$@" --json >"$OUT/envelope/$name.json" 2>/dev/null || true; }
cap list-default list --limit 0
cap list-status-all list --status all --limit 0
cap list-status-open list --status open --limit 0
cap list-parent-open list --parent "$E" --status open
cap ready-parent-unassigned ready --parent "$E" --unassigned
cap ready-unassigned ready --unassigned --limit 0
cap search-task search task
cap search-task-open search task --status open
cap search-task-multi search task --status open,in_progress
cap search-task-all search task --status all
cap comments comments "$A"
cap history history "$A"
cap stale stale
cap status status --no-activity
cap vc-status vc status
cap diff diff HEAD~1 HEAD

bd kv set bdash.test value1 >/dev/null
bd remember "fixture memory" --key fixture-mem >/dev/null
cap kv-list kv list
cap memories memories
cap recall recall fixture-mem

c() { bd create --silent "$@"; }
X=$(c --title="Cycle X" --type=task --priority=2)
Y=$(c --title="Cycle Y" --type=task --priority=2)
bd dep add "$X" "$Y" >/dev/null
bd dep add "$Y" "$X" >"$OUT/dep-add-cycle.stdout" 2>"$OUT/dep-add-cycle.stderr" || true
bd dep add "$Y" "$X" --no-cycle-check >/dev/null
cap dep-cycles dep cycles
