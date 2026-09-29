#!/usr/bin/env bash
# Events fixtures: bd events tail record shapes for every op, the truncation
# failure and the disabled-journal note. Needs bd >= 1.3.0 on PATH.
# Usage: capture-m2.sh <empty-dir> <out-dir>
set -euo pipefail
WS=$1; OUT=$2
mkdir -p "$OUT"; OUT=$(cd "$OUT" && pwd)
mkdir -p "$WS" "$OUT"
export HOME="$WS/../home-m2"
mkdir -p "$HOME"
export GIT_CONFIG_GLOBAL="$HOME/.gitconfig" GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME=tester GIT_AUTHOR_EMAIL=tester@example.com
export GIT_COMMITTER_NAME=tester GIT_COMMITTER_EMAIL=tester@example.com
unset BEADS_DIR BD_ACTOR
export BD_DISABLE_METRICS=1 BEADS_ACTOR=tester
cd "$WS"
git init -q
bd init --non-interactive --prefix m2 --stealth >/dev/null 2>&1
bd config set events-journal true >/dev/null

A=$(bd create --silent --title="First" --type=task)
B=$(bd create --silent --title="Second" --type=task)
bd update "$B" --status in_progress >/dev/null
bd comment "$A" "a comment" >/dev/null
bd dep add "$A" "$B" >/dev/null
bd close "$B" --reason done >/dev/null
C=$(bd create --silent --title="Child" --type=task --parent="$A")
bd dep remove "$A" "$B" >/dev/null
bd delete "$A" --force >/dev/null 2>&1

BD_JSON_ENVELOPE=1 bd events tail --since 0 --json >"$OUT/tail-all.jsonl" 2>"$OUT/tail-all.stderr"

bd config set events-journal-retain-days 0 >/dev/null
bd config set events-journal-retain-rows 0 >/dev/null
bd events prune --before 6 >/dev/null
rc=0
BD_JSON_ENVELOPE=1 bd events tail --since 2 --follow --json >"$OUT/truncated-follow.stdout" 2>"$OUT/truncated-follow.stderr" || rc=$?
echo "$rc" >"$OUT/truncated-follow.rc"

NOJ="$WS/../nojournal"
mkdir -p "$NOJ"
cd "$NOJ"
git init -q
bd init --non-interactive --prefix nj --stealth >/dev/null 2>&1
rc=0
BD_JSON_ENVELOPE=1 bd events tail --since 0 --json >"$OUT/disabled.stdout" 2>"$OUT/disabled.stderr" || rc=$?
echo "$rc" >"$OUT/disabled.rc"

UC="$WS/../updclosed"
mkdir -p "$UC"
cd "$UC"
git init -q
bd init --non-interactive --prefix uc --stealth >/dev/null 2>&1
bd config set events-journal true >/dev/null
U=$(bd create --silent --title="Closed by update" --type=task)
bd update "$U" --status closed >/dev/null
BD_JSON_ENVELOPE=1 bd events tail --since 0 --json >"$OUT/update-closed.jsonl"
