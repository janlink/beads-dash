#!/usr/bin/env bash
# Fail when the latest bd release is newer than the highest fixtured version
# (testdata/bd-<version>/). Needs the gh CLI and GH_TOKEN in CI.
set -euo pipefail

cd "$(dirname "$0")/.."
fixtured=$(find testdata -maxdepth 1 -type d -name 'bd-*' | sed 's#.*/bd-##' | sort -V | tail -n 1)
latest=$(gh api repos/steveyegge/beads/releases/latest --jq .tag_name)
latest=${latest#v}

echo "highest fixtured bd: $fixtured, latest release: $latest"
if [ "$latest" != "$fixtured" ]; then
	echo "newer bd released: capture fixtures and raise the supported ceiling" >&2
	exit 1
fi
