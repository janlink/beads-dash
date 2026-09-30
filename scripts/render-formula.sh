#!/usr/bin/env bash
# Renders the Homebrew formula from packaging/homebrew/bdash.rb.tmpl and the
# checksums of a GoReleaser dist directory. Prints the formula to stdout.
# Usage: scripts/render-formula.sh <version> [dist-dir] [repo]
set -euo pipefail

version=${1:?usage: render-formula.sh <version> [dist-dir] [repo]}
version=${version#v}
dist=${2:-dist}
repo=${3:-janlink/beads-dash}
root=$(cd "$(dirname "$0")/.." && pwd)
sums="$dist/checksums.txt"

sha() {
    local name="bdash_${version}_$1.tar.gz" sum
    sum=$(awk -v n="$name" '$2 == n { print $1 }' "$sums")
    if [ -z "$sum" ]; then
        echo "render-formula.sh: no checksum for $name in $sums" >&2
        exit 1
    fi
    printf '%s' "$sum"
}

base="https://github.com/$repo/releases/download/v$version"
sed \
    -e "s|@VERSION@|$version|g" \
    -e "s|@BASE@|$base|g" \
    -e "s|@SHA_DARWIN_ARM64@|$(sha darwin_arm64)|" \
    -e "s|@SHA_DARWIN_AMD64@|$(sha darwin_amd64)|" \
    -e "s|@SHA_LINUX_ARM64@|$(sha linux_arm64)|" \
    -e "s|@SHA_LINUX_AMD64@|$(sha linux_amd64)|" \
    "$root/packaging/homebrew/bdash.rb.tmpl"
