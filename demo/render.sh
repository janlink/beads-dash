#!/usr/bin/env bash
# Records the demo and renders assets/demo.gif (140x38). Needs bd, git, python3
# and agg (https://github.com/asciinema/agg) on the PATH.
# Usage: demo/render.sh [bdash-binary]
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
bdash=${1:-}
work=$(mktemp -d)
trap 'find "$work" -delete' EXIT

for tool in bd git python3 agg; do
    command -v "$tool" >/dev/null || { echo "demo/render.sh: $tool is not installed" >&2; exit 1; }
done

if [ -z "$bdash" ]; then
    bdash="$work/bdash"
    (cd "$root" && go build -o "$bdash" ./cmd/bdash)
fi

export HOME="$work/home"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_STATE_HOME="$HOME/.local/state"
export BDASH_CONFIG_DIR="$HOME/.config/bdash"
export GIT_AUTHOR_NAME=demo GIT_AUTHOR_EMAIL=demo@example.com
export GIT_COMMITTER_NAME=demo GIT_COMMITTER_EMAIL=demo@example.com
export GIT_CONFIG_GLOBAL="$HOME/.gitconfig" GIT_CONFIG_SYSTEM=/dev/null
export DOLT_ROOT_PATH="$HOME"
mkdir -p "$HOME"

"$root/demo/seed.sh" "$work/ws"
python3 "$root/demo/record.py" "$work/ws" "$work/demo.cast" "$bdash"

mkdir -p "$root/assets"
agg --cols 140 --rows 38 --font-size 14 --speed 1 "$work/demo.cast" "$root/assets/demo.gif"
echo "wrote assets/demo.gif"
