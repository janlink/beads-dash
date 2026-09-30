#!/usr/bin/env bash
# Write fixtures: create, update, claim, close, reopen and dep results and
# failures in envelope mode, with exit codes and the state bd left behind
# after a failed batch. Runs the bd on PATH.
# Usage: capture-m8.sh <empty-dir> <out-dir>
set -euo pipefail
WS=$1; OUT=$2
mkdir -p "$WS" "$OUT"
OUT=$(cd "$OUT" && pwd)
export HOME="$WS/../home-m8"
mkdir -p "$HOME"
export GIT_CONFIG_GLOBAL="$HOME/.gitconfig" GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME=tester GIT_AUTHOR_EMAIL=tester@example.com
export GIT_COMMITTER_NAME=tester GIT_COMMITTER_EMAIL=tester@example.com
unset BEADS_DIR BD_ACTOR
export BD_DISABLE_METRICS=1 BEADS_ACTOR=tester
cd "$WS"
git init -q
bd init --non-interactive --prefix m8 --stealth >/dev/null 2>&1

cap() { # name argv...: stdout, stderr and exit code with BD_JSON_ENVELOPE=1
  local name=$1; shift
  local rc=0
  BD_JSON_ENVELOPE=1 bd "$@" >"$OUT/$name.json" 2>"$OUT/$name.stderr" || rc=$?
  echo "$rc" >"$OUT/$name.rc"
}
state() { cap "state-$1" list --all --limit 0 --json; }
mk() { bd create --silent "$@"; }

cap create-ok create --json --title "First" --type bug --priority 1 --labels ui,backend --description "desc" --assignee alice
cap create-empty-title create --json --title ""
cap create-bad-priority create --json --title "Bad" --priority 9
cap create-bad-type create --json --title "Bad" --type nonsense
cap create-deps-missing create --json --title "Dep" --deps m8-zzz

E=$(mk --title="Epic" --type=epic --labels=team)
cap create-child create --json --title "Child" --parent "$E"
cap create-child-no-inherit create --json --title "Child2" --parent "$E" --no-inherit-labels

A=$(mk --title="Alpha" --type=task)
B=$(mk --title="Beta" --type=task)
C=$(mk --title="Gamma" --type=task)
D=$(mk --title="Delta" --type=task)

cap update-ok update "$A" --json --title "Alpha 2" --priority 3 --add-label x --add-label y
cap update-remove-label update "$A" --json --remove-label x
cap update-unassign update "$A" --json --assignee ""
cap update-nochange update "$A" --json
cap update-missing update m8-zzz --json --priority 3
cap update-partial update "$A" m8-zzz --json --priority 4
cap update-bad-priority update "$A" --json --priority 9
cap update-bad-status update "$A" --json --status nonsense
cap update-clear-text update "$A" --json --description "text" --notes "n"
cap update-cleared update "$A" --json --description= --notes=
cap update-estimate update "$A" --json --estimate=30
cap update-estimate-clear update "$A" --json --estimate=0
cap update-due update "$A" --json --due=2030-01-15
cap update-due-clear update "$A" --json --due=
cap update-claim update "$B" --json --claim
BEADS_ACTOR=other cap update-claim-conflict update "$B" --json --claim
cap update-claim-batch update "$C" "$B" --json --claim
state after-claim

cap dep-add dep add "$B" "$A" --json
cap dep-add-cycle dep add "$A" "$B" --json
cap dep-add-missing dep add "$A" m8-zzz --json
cap dep-add-duplicate dep add "$B" "$A" --json
cap dep-remove dep rm "$B" "$A" --json
cap dep-remove-absent dep rm "$B" "$A" --json
bd dep add "$B" "$A" >/dev/null

cap close-blocked-single close "$B" --json --reason "nope"
cap close-blocked-batch close "$B" "$C" --json --reason "batch"
state after-close-blocked-batch
cap close-missing-batch close "$D" m8-zzz --json --reason "with missing"
state after-close-missing-batch
cap close-missing-single close m8-zzz --json --reason "missing"
cap close-open-children close "$E" --json --reason "epic"
state after-close-open-children
cap close-ok close "$A" --json --reason "done it"
cap close-again close "$A" --json --reason "again"
cap close-no-reason close "$B" "$E" --json
state after-close-no-reason

cap reopen-ok reopen "$A" --json --reason "back"
cap reopen-already-open reopen "$A" --json
cap reopen-missing-batch reopen "$D" m8-zzz --json

# Memories: empty store, upserts, recalled shape, odd keys, failures.
cap memories-empty memories --json
cap forget-missing forget --json -- nokey
cap remember-empty-content remember --key k --json -- ""
cap remember-ok remember --key first-key --json -- "first content"
cap remember-overwrite remember --key first-key --json -- "second content"
cap remember-recalled-shape remember --json -- "some content words here for the slug"
cap remember-leading-dash remember --key "-dash" --json -- "-content"
cap remember-schema-version remember --key schema_version --json -- "v"
cap remember-spaced-key remember --key "spaced key" --json -- "line1
line2"
cap remember-mixed-case-upper remember --key "Weird.Key" --json -- "a"
cap remember-mixed-case-lower remember --key "weird.key" --json -- "b"
cap memories-odd memories --json
cap memories-odd-plain memories
cap forget-ok forget --json -- first-key
cap forget-leading-dash forget --json -- "-dash"
cap forget-again forget --json -- first-key
cap memories-after-forget memories --json

# bd prime without memories and with two, in a fresh workspace: the difference
# is the footprint the Memories view reports.
PRIME=$(mktemp -d)
(cd "$PRIME" && git init -q && bd init --non-interactive --prefix pr --stealth >/dev/null 2>&1 \
  && bd prime >"$OUT/prime-none.txt" \
  && bd remember "hello world" --key k1 >/dev/null \
  && bd remember "two
lines ä" --key k2 >/dev/null \
  && bd prime >"$OUT/prime-two.txt")
