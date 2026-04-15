#!/usr/bin/env bash
# Scenario 10: Domain Membership, RBAC, and Expiry — run.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIOS_DIR="${SCRIPT_DIR}/.."

CLUSTER_NAME="${SCENARIO_CLUSTER:-chorister-scenario-10}"
SKIP_SETUP=0
SKIP_TEARDOWN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster-name) CLUSTER_NAME="$2"; shift 2 ;;
    --skip-setup) SKIP_SETUP=1; shift ;;
    --skip-teardown) SKIP_TEARDOWN=1; shift ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

export SCENARIO_CLUSTER="$CLUSTER_NAME"
export KUBECTL_CONTEXT="kind-${CLUSTER_NAME}"

if [[ "$SKIP_SETUP" -eq 0 ]]; then
  bash "${SCENARIOS_DIR}/setup-scenario-cluster.sh" \
    --cluster-name "$CLUSTER_NAME" \
    --plain-kind
fi

bash "${SCRIPT_DIR}/assert.sh"
RC=$?

if [[ "$SKIP_TEARDOWN" -eq 0 ]]; then
  bash "${SCENARIOS_DIR}/teardown-scenario-cluster.sh" \
    --cluster-name "$CLUSTER_NAME"
fi

exit $RC
