#!/usr/bin/env bash
# Prints the CHANGELOG.md section of a version (v-prefix optional) for the
# release notes; fails when the section is missing or empty. With
# REQUIRE_DATED=1 it also fails while the heading still says Unreleased.
# Usage: changelog-section.sh <version> [file]
set -euo pipefail

version=${1:?usage: changelog-section.sh <version> [file]}
version=${version#v}
file=${2:-CHANGELOG.md}

heading=$(grep -F -m1 "## [$version]" "$file" || true)
if [ -n "${REQUIRE_DATED:-}" ] && [[ "$heading" == *Unreleased* ]]; then
    echo "changelog heading for $version is still marked Unreleased: $heading" >&2
    exit 1
fi

section=$(awk -v v="$version" '
  /^## \[/ {
    if (on) exit
    on = index($0, "## [" v "]") == 1
    next
  }
  on { print }
' "$file")

if [ -z "$(printf '%s' "$section" | tr -d '[:space:]')" ]; then
    echo "no changelog section for $version in $file" >&2
    exit 1
fi
printf '%s\n' "$section" | awk 'NF { started = 1 } started { lines[++n] = $0 } END { while (n > 0 && lines[n] == "") n--; for (i = 1; i <= n; i++) print lines[i] }'
