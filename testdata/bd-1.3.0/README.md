# bd 1.3.0 fixtures

Real `--json` output captured from bd 1.3.0 (f45b249ce) in a throwaway workspace. Parser tests use these; never hand-edit them to match bd's docs.

- `recipe-run/`: output of one `../bd-1.2.2/capture.sh` run with the 1.3.0 binary, unchanged recipe. Same layout as `../bd-1.2.2/recipe-run/` (`*.json` stdout plus `*.stderr`, `envelope/`, `errors/`).
- `capture-extras.sh <ws-seeded-by-capture.sh> <out-dir>`: commands `capture.sh` does not cover (`list` default and `--status` filters, `search`, `ready --unassigned`, `comments`, `history`, `status`, `stale`, `vc status`, `diff`, `kv list`, `memories`, `recall`, `dep cycles`). It mutates the workspace, so run it after `capture.sh`.
- `extras/`: `capture-extras.sh` output from bd 1.3.0, in the same workspace as `recipe-run/`.
- `extras-1.2.2/`: `capture-extras.sh` output from bd 1.2.2 in a fresh `capture.sh` workspace, as the baseline for `extras/`.

`dep-cycles.json` is `[]` in both versions: bd refuses to create a blocking cycle through `dep add`, `--no-cycle-check` and `import` alike.

Workspace paths were rewritten to `/tmp/` and author e-mails to `tester@example.com`.
