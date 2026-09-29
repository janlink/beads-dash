#!/usr/bin/env bash
# M1 fixture gaps that capture.sh and capture-extras.sh do not cover: custom
# statuses without a category, config get, error shapes in envelope mode with
# exit codes, and the schema-skew error. Runs the bd on PATH.
# Usage: capture-m1.sh <empty-dir> <out-dir>
# With BD_OLD=<older bd binary> the schema-skew error is captured by running
# the older binary on a workspace created by the bd on PATH.
set -euo pipefail
WS=$1; OUT=$2
mkdir -p "$WS" "$OUT"
export HOME="$WS/../home-m1"
mkdir -p "$HOME"
export GIT_CONFIG_GLOBAL="$HOME/.gitconfig" GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME=tester GIT_AUTHOR_EMAIL=tester@example.com
export GIT_COMMITTER_NAME=tester GIT_COMMITTER_EMAIL=tester@example.com
unset BEADS_DIR BD_ACTOR
cd "$WS"
git init -q
bd init --non-interactive --prefix m1 --stealth >/dev/null 2>&1

c() { bd create --silent "$@"; }
E=$(c --title="Epic" --type=epic)
A=$(c --title="Plain custom" --type=task --parent="$E")
B=$(c --title="Review custom" --type=task --parent="$E")
bd config set status.custom "review:wip,plain,cold:frozen" >/dev/null 2>&1
bd update "$A" --status plain >/dev/null
bd update "$B" --status review >/dev/null

cap() { # name argv...: stdout, stderr and exit code with BD_JSON_ENVELOPE=1
  local name=$1; shift
  local rc=0
  BD_JSON_ENVELOPE=1 bd "$@" >"$OUT/$name.json" 2>"$OUT/$name.stderr" || rc=$?
  echo "$rc" >"$OUT/$name.rc"
}
cap statuses-custom statuses --json
cap list-custom-status list --all --limit 0 --json
cap ready-explain-custom ready --explain --limit 0 --json
cap config-get-journal-off config get events-journal --json
cap config-get-unset config get no-such-key --json
bd config set events-journal true >/dev/null 2>&1 || true
cap config-get-journal-on config get events-journal --json
bd config set events-journal false >/dev/null 2>&1 || true
cap comments-missing-issue comments m1-zzz --json
cap history-missing-issue history m1-zzz --json

if [ -n "${BD_OLD:-}" ]; then
  oldcap() {
    local name=$1; shift
    local rc=0
    BD_JSON_ENVELOPE=1 "$BD_OLD" "$@" >"$OUT/$name.json" 2>"$OUT/$name.stderr" || rc=$?
    echo "$rc" >"$OUT/$name.rc"
  }
  oldcap skew-list list --all --limit 0 --json
  oldcap skew-where where --json
fi

NOWS="$WS/../notws-m1"
mkdir -p "$NOWS"
cd "$NOWS"
cap notws-where where --json
cap notws-list list --all --limit 0 --json
cap notws-ready ready --explain --limit 0 --json
