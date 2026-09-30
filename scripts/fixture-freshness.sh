#!/usr/bin/env bash
# Re-run the fixture capture scripts against the pinned bd binaries and fail
# when the normalised output differs from the committed testdata.
# Usage: fixture-freshness.sh [version...]   (default: every testdata/bd-<version>)
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
work=$(mktemp -d)
trap 'find "$work" -delete' EXIT

if [ "$#" -gt 0 ]; then versions=("$@"); else
	versions=()
	for d in testdata/bd-*/; do d=${d%/}; versions+=("${d##*/bd-}"); done
fi

bd_bin() { echo "$root/.cache/bd/$1/bd"; }

norm_json='
def scrub:
  gsub("[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+(Z|[+-][0-9:]+)?"; "<TS>")
  | gsub("\\b(r1|m1|m2|m8|uc)-[a-z0-9]+(\\.[0-9]+)*"; "<ID>")
  | gsub("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"; "<UUID>")
  | gsub("/tmp/[^\" ]*"; "<PATH>")
  | gsub("\\b[0-9a-f]{7,40}\\b"; "<HASH>");
def masks: {
  created_by: "<ACTOR>", updated_by: "<ACTOR>", closed_by: "<ACTOR>", owner: "<ACTOR>",
  author: "<ACTOR>", actor: "<ACTOR>", Author: "<ACTOR>", Actor: "<ACTOR>",
  created_at: "<TS>", updated_at: "<TS>", closed_at: "<TS>", Timestamp: "<TS>", timestamp: "<TS>",
  branch: "<REF>", build: "<REF>", commit: "<REF>", CommitHash: "<HASH>", revision: "<HASH>",
  depth: "<N>", Position: "<N>", Root: "<ROOT>"
};
def mask: with_entries(if (masks[.key] != null) and (.value != null) then .value = masks[.key] else . end);
def clean:
  if type == "object" then
    mask as $o
    | if ($o | keys | any(test("^(r1|m1|m2|m8|uc)-"))) then
        $o | to_entries | map({k: (.key | scrub), v: (.value | clean)}) | sort_by(tojson)
      else $o | with_entries(.value |= clean) end
  elif type == "array" then map(clean) | sort_by(tojson)
  elif type == "string" then scrub
  else . end;
clean'

normalise() { # file -> stdout
	if [[ $1 == *.json ]] && jq -e . "$1" >/dev/null 2>&1; then
		jq -S "$norm_json" "$1"
	else
		sed -E \
			-e 's#\b(r1|m1|m2|m8|uc)-[a-z0-9]+(\.[0-9]+)*#<ID>#g' \
			-e 's#[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}#<UUID>#g' \
			-e 's#/tmp/[^" ]*#<PATH>#g' \
			-e 's#[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]+(Z|[+-][0-9:]+)?#<TS>#g' \
			-e 's#\b[0-9a-f]{7,40}\b#<HASH>#g' "$1" |
			grep -v -e '^Warning: multiple .bd. binaries' -e '^  /.*/bd$' -e '^The first one is being used' -e '^$' || true
	fi
}

capture() { # script args...: a failing capture script fails the run
	if ! bash "$@" >"$work/capture.log" 2>&1; then
		echo "capture script failed: $*" >&2
		tail -20 "$work/capture.log" >&2
		exit 1
	fi
}

fail=0
compare() { # generated-dir committed-dir
	local gen=$1 com=$2 f rel
	while IFS= read -r f; do
		rel=${f#"$gen"/}
		[ "${rel##*/}" = version.json ] && continue # build metadata differs per binary build
		if [ ! -e "$com/$rel" ]; then
			echo "missing committed fixture: $com/$rel" >&2; fail=1; continue
		fi
		if ! diff -u <(normalise "$com/$rel") <(normalise "$f") >"$work/diff.out"; then
			echo "stale fixture: $com/$rel" >&2; head -40 "$work/diff.out" >&2; fail=1
		fi
	done < <(find "$gen" -type f | sort)
	while IFS= read -r f; do
		rel=${f#"$com"/}
		case ${rel##*/} in version.json | *.diff) continue ;; esac # build metadata and derived comparisons
		if [ ! -e "$gen/$rel" ]; then
			echo "committed fixture not regenerated: $com/$rel" >&2; fail=1
		fi
	done < <(find "$com" -type f | sort)
}

for v in "${versions[@]}"; do
	bin=$(bd_bin "$v")
	[ -x "$bin" ] || { echo "missing $bin (run just bd-install)" >&2; exit 1; }
	echo "== bd $v"
	(
		export PATH="$(dirname "$bin"):$PATH"
		export HOME="$work/home-$v"; mkdir -p "$HOME"
		export GIT_CONFIG_GLOBAL="$HOME/.gitconfig" GIT_CONFIG_SYSTEM=/dev/null
		export GIT_AUTHOR_NAME=tester GIT_AUTHOR_EMAIL=tester@example.com
		export GIT_COMMITTER_NAME=tester GIT_COMMITTER_EMAIL=tester@example.com
		unset BEADS_DIR BD_ACTOR
		capture testdata/bd-1.2.2/capture.sh "$work/ws-$v" "$work/recipe-$v"
		if [ -f "testdata/bd-$v/capture-extras.sh" ]; then
			capture "testdata/bd-$v/capture-extras.sh" "$work/ws-$v" "$work/extras-$v"
		fi
		old=
		if [ -x "$(bd_bin 1.2.2)" ] && [ "$v" != 1.2.2 ]; then old=$(bd_bin 1.2.2); fi
		BD_OLD=$old capture testdata/capture-m1.sh "$work/m1ws-$v" "$work/m1-$v"
		if [ -d "testdata/bd-$v/events" ]; then
			capture testdata/capture-m2.sh "$work/m2ws-$v" "$work/m2-$v"
		fi
		capture testdata/capture-m8.sh "$work/m8ws-$v" "$work/m8-$v"
	)
	compare "$work/recipe-$v" "testdata/bd-$v/recipe-run"
	compare "$work/m1-$v" "testdata/bd-$v/m1"
	if [ -d "testdata/bd-$v/events" ]; then compare "$work/m2-$v" "testdata/bd-$v/events"; fi
	compare "$work/m8-$v" "testdata/bd-$v/m8"
	if [ -d "$work/extras-$v" ]; then compare "$work/extras-$v" "testdata/bd-$v/extras"; fi
done

for v in "${versions[@]}"; do
	if [ -d "testdata/bd-$v/contract-corpus" ]; then
		scripts/vendor-contract-corpus.sh "$v" "$work/corpus-$v" >/dev/null
		diff -r "$work/corpus-$v" "testdata/bd-$v/contract-corpus" >/dev/null || { echo "stale contract corpus for $v" >&2; fail=1; }
	fi
done

[ "$fail" -eq 0 ] && echo "fixtures fresh"
exit "$fail"
