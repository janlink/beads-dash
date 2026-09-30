#!/usr/bin/env bash
# Builds the workspace the demo records: a fixed set of issues with explicit
# IDs, so every run shows the same screens. Usage: demo/seed.sh <dir>
set -euo pipefail

dir=${1:?usage: demo/seed.sh <dir>}
bd=${BDASH_BD:-bd}

mkdir -p "$dir"
cd "$dir"
if [ -e .beads ]; then
    echo "demo/seed.sh: $dir already holds a workspace" >&2
    exit 1
fi

git init -q .
"$bd" init --non-interactive --prefix demo --stealth >/dev/null

create() {
    "$bd" create --silent "$@" >/dev/null
}

create --id=demo-e1 --type=epic --priority=1 --title="Checkout rework" \
    --description=$'Replace the legacy checkout.\n\n- guest orders\n- saved payment methods\n- confirmation mail'
create --id=demo-e2 --type=epic --priority=2 --title="Observability" \
    --description="Traces and alerts for the order pipeline."
create --id=demo-a1 --type=task --priority=1 --title="Address form validation" --labels=ui,checkout --assignee=ana
create --id=demo-a2 --type=feature --priority=1 --title="Guest order confirmation mail" --labels=mail,checkout --assignee=ben
create --id=demo-a3 --type=bug --priority=0 --title="Payment provider timeout retries" --labels=payments --assignee=ana \
    --description=$'Retries stack up when the provider answers slowly.\n\nSteps:\n1. Throttle the sandbox\n2. Place an order'
create --id=demo-a4 --type=task --priority=2 --title="Saved payment methods" --labels=payments,checkout
create --id=demo-b1 --type=task --priority=3 --title="Trace the order pipeline" --labels=ops
create --id=demo-b2 --type=chore --priority=3 --title="Alert on failed webhooks" --labels=ops --assignee=ben
create --id=demo-b3 --type=task --priority=4 --title="Dashboard for queue depth" --labels=ops

dep() {
    "$bd" dep add "$@" >/dev/null
}

dep demo-a1 demo-e1 --type=parent-child
dep demo-a2 demo-e1 --type=parent-child
dep demo-a3 demo-e1 --type=parent-child
dep demo-a4 demo-e1 --type=parent-child
dep demo-b1 demo-e2 --type=parent-child
dep demo-b2 demo-e2 --type=parent-child
dep demo-b3 demo-e2 --type=parent-child
dep demo-a4 demo-a3
dep demo-b2 demo-b1

"$bd" update demo-a3 --status=in_progress >/dev/null
"$bd" update demo-a1 --status=in_progress >/dev/null
"$bd" close demo-b3 --reason="Covered by the tracing dashboard" >/dev/null
"$bd" comments add demo-a3 "Reproduced with a 12 s provider delay." >/dev/null
"$bd" comments add demo-a3 "Backoff needs a cap; proposing 3 tries." >/dev/null
