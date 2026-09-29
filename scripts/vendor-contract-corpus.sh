#!/usr/bin/env bash
# Download bd's contract corpus (released with bd >= 1.3.0) into
# testdata/bd-<version>/contract-corpus/, verified against the release's
# checksums.txt. Usage: vendor-contract-corpus.sh <version> [out-dir]
set -euo pipefail

version=${1:?usage: vendor-contract-corpus.sh <version> [out-dir]}
version=${version#v}
out=${2:-testdata/bd-$version/contract-corpus}
repo=${BDASH_BD_REPO:-steveyegge/beads}

asset="beads_${version}_contract_corpus.tar.gz"
base="https://github.com/$repo/releases/download/v$version"
tmp=$(mktemp -d)
trap 'find "$tmp" -delete' EXIT

curl -fsSL --retry 3 -o "$tmp/checksums.txt" "$base/checksums.txt"
curl -fsSL --retry 3 -o "$tmp/$asset" "$base/$asset"

want=$(awk -v a="$asset" '$2 == a { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || { echo "vendor-contract-corpus: $asset not listed in checksums.txt" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
else
	got=$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')
fi
if [ "$got" != "$want" ]; then
	echo "vendor-contract-corpus: checksum mismatch for $asset (want $want, got $got)" >&2
	exit 1
fi

mkdir -p "$out"
tar -xzf "$tmp/$asset" -C "$tmp"
cp -R "$tmp/corpus/." "$out/"
