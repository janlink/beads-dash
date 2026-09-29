# bd 1.2.2 fixtures

Real `--json` output captured from bd 1.2.2 in a throwaway workspace. Parser tests use these; never hand-edit them to match bd's docs, which disagree with the real output in several places.

- `capture.sh <empty-dir> <out-dir>`: reproducible capture recipe (seeds epics, children, every status and type, custom statuses/types, blocking deps, comments).
- `recipe-run/`: output of one `capture.sh` run, `*.json` stdout plus `*.stderr`, with `envelope/` (`BD_JSON_ENVELOPE=1`) and `errors/` (no workspace, missing issue).
- Top level, `envelope/`, `errors/`: the original exploratory captures, including extras such as `show-long.json`, `show-multi.json` and `show-child-with-comments.json`.

Workspace paths were rewritten to `/tmp/` and author e-mails to `tester@example.com`.
- `history-memories/`: `bd history`, `bd diff`, `bd comments`, `bd memories`/`recall`/`remember`, `bd kv`, `bd status`, `bd stale`, `bd vc status` samples (not covered by `capture.sh`). `history-scale-413.json` is one issue's history in a workspace with ~413 Dolt commits.
- `writes/`: stdout (`*.out`) and stderr (`*.err`) of `bd create`, `update`, `close`, `reopen`, `priority`, `assign`, `label`, plus `err_*` failure cases and `vc status` samples. Large 500/5000-issue list dumps were left out.
