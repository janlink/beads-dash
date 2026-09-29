#!/usr/bin/env bash
# Install a pinned bd release into <cache>/<version>/bd, verified against the
# release's checksums.txt. Usage: install-bd.sh <version> [cache-dir]
# Prints the path of the installed binary on stdout.
set -euo pipefail

version=${1:?usage: install-bd.sh <version> [cache-dir]}
version=${version#v}
cache=${2:-${BDASH_BD_CACHE:-.cache/bd}}
dest="$cache/$version"
repo=${BDASH_BD_REPO:-steveyegge/beads}

if [ -x "$dest/bd" ]; then
	echo "$dest/bd"
	exit 0
fi

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) echo "install-bd: unsupported OS $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) echo "install-bd: unsupported arch $(uname -m)" >&2; exit 1 ;;
esac

asset="beads_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/v$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL --retry 3 -o "$tmp/checksums.txt" "$base/checksums.txt"
curl -fsSL --retry 3 -o "$tmp/$asset" "$base/$asset"

want=$(awk -v a="$asset" '$2 == a { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || { echo "install-bd: $asset not listed in checksums.txt" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
else
	got=$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')
fi
if [ "$got" != "$want" ]; then
	echo "install-bd: checksum mismatch for $asset (want $want, got $got)" >&2
	exit 1
fi

mkdir "$tmp/x"
tar -xzf "$tmp/$asset" -C "$tmp/x"
bin=$(find "$tmp/x" -type f -name bd | head -n 1)
[ -n "$bin" ] || { echo "install-bd: no bd binary in $asset" >&2; exit 1; }
mkdir -p "$dest"
mv "$bin" "$dest/bd"
chmod +x "$dest/bd"
echo "$dest/bd"
