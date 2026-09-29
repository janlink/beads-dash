#!/usr/bin/env bash
# Fixture capture recipe for bd 1.2.2. Usage: capture.sh <empty-dir> <out-dir>
set -euo pipefail
WS=$1; OUT=$2
mkdir -p "$WS" "$OUT/envelope" "$OUT/errors"
cd "$WS"
git init -q
bd init --non-interactive --prefix r1 --stealth >/dev/null
bd config set status.custom "review:wip,parked:frozen" >/dev/null
bd config set types.custom "research,incident" >/dev/null 2>&1

c() { bd create --silent "$@"; }
E=$(c --title="Epic A" --type=epic --priority=1 --description="Epic **markdown**" --labels=area:core,ui)
A=$(c --title="Child task A1" --type=task --priority=0 --parent="$E" --assignee=alice --labels=ui)
B=$(c --title="Child bug A2" --type=bug --priority=2 --parent="$E" --assignee=agent-7)
C=$(c --title="Feature blocked by A1" --type=feature --priority=3 --parent="$E")
D=$(c --title="Chore standalone" --type=chore --priority=4)
G=$(c --title="Decision X" --type=decision --priority=2)
H=$(c --title="Spike Y" --type=spike --priority=2)
I=$(c --title="Story Z" --type=story --priority=2)
J=$(c --title="Milestone M" --type=milestone --priority=1)
K=$(c --title="Research custom type" --type=research --priority=2)
c --title="Grandchild of A1" --type=task --priority=2 --parent="$A" >/dev/null
M=$(c --title="Closed task" --type=task --priority=2)
N=$(c --title="Deferred task" --type=task --priority=2)
O=$(c --title="Pinned task" --type=task --priority=2)
P=$(c --title="Hooked task" --type=task --priority=2)
Q=$(c --title="Review custom status" --type=task --priority=2)
R=$(c --title="Explicit blocked status" --type=task --priority=2)
S=$(c --title="Blocked by closed issue" --type=task --priority=2)
E2=$(c --title="Epic B blocker" --type=epic --priority=2)

bd dep add "$C" "$A" >/dev/null
bd dep add "$S" "$M" >/dev/null
bd dep add "$D" "$B" --type related >/dev/null
bd dep add "$G" "$C" >/dev/null
bd dep add "$H" "$I" --type discovered-from >/dev/null
bd dep add "$J" external:other:cap-x >/dev/null
bd dep add "$E" "$E2" >/dev/null   # blocked epic -> children become blocked

bd close "$M" --reason="done for test" >/dev/null
bd update "$N" --status deferred >/dev/null
bd update "$O" --status pinned >/dev/null
bd update "$P" --status hooked >/dev/null
bd update "$Q" --status review >/dev/null
bd update "$R" --status blocked >/dev/null
bd update "$A" --status in_progress >/dev/null
bd update "$K" --status parked >/dev/null
bd comment "$A" "First comment by default actor" >/dev/null
BD_ACTOR=bob bd comment "$A" "Second comment **md**" >/dev/null
bd update "$B" --design="design text" --notes="some notes" --acceptance="AC here" \
  --estimate 90 --due 2026-10-15 --external-ref gh-12 >/dev/null
# orphan the custom status "parked" and custom type "research"
bd config set status.custom "review:wip" >/dev/null
bd config set types.custom "incident" >/dev/null 2>&1

cap() { local name=$1; shift; bd "$@" --json >"$OUT/$name.json" 2>"$OUT/$name.stderr" || true
        BD_JSON_ENVELOPE=1 bd "$@" --json >"$OUT/envelope/$name.json" 2>/dev/null || true; }
cap version version
cap where where
cap context context
cap statuses statuses
cap types types
cap list-all list --all --limit 0
cap ready ready --limit 0
cap ready-explain ready --explain --limit 0
cap blocked blocked
cap show-child show "$A"
cap show-full show "$A" --include-comments --include-dependents
cap show-epic show "$E"
cap children children "$E"
cap dep-list dep list "$C"
cap dep-list-up dep list "$A" --direction up
cap dep-tree dep tree "$G"
cap graph-epic graph "$E"
cap graph-all graph --all
cap show-missing show r1-zzz

mkdir -p "$WS/../notws"; cd "$WS/../notws"
env -u BEADS_DIR bd where --json >"$OUT/errors/where-not-workspace.stdout.json" 2>/dev/null || true
env -u BEADS_DIR bd list --json 2>"$OUT/errors/list-not-workspace.stderr.txt" >/dev/null || true
