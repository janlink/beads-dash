#!/usr/bin/env bash
# Renders the formula for a release and pushes it to the Homebrew tap.
# Needs HOMEBREW_TAP_TOKEN. Usage: scripts/publish-formula.sh <tag> [dist-dir]
set -euo pipefail

tag=${1:?usage: publish-formula.sh <tag> [dist-dir]}
dist=${2:-dist}
: "${HOMEBREW_TAP_TOKEN:?HOMEBREW_TAP_TOKEN is not set}"
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'find "$work" -delete' EXIT

git clone --depth 1 "https://x-access-token:${HOMEBREW_TAP_TOKEN}@github.com/janlink/homebrew-tap.git" "$work/tap"
mkdir -p "$work/tap/Formula"
"$root/scripts/render-formula.sh" "$tag" "$dist" > "$work/tap/Formula/bdash.rb"

cd "$work/tap"
if git diff --quiet && [ -z "$(git status --porcelain)" ]; then
    echo "formula is already current"
    exit 0
fi
git config user.name "bdash release"
git config user.email "noreply@github.com"
git add Formula/bdash.rb
git commit -q -m "bdash ${tag#v}"
git push origin HEAD
