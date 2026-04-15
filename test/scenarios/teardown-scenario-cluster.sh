#!/usr/bin/env bash
# Tear down a scenario Kind cluster.
#
# Usage:
#   test/scenarios/teardown-scenario-cluster.sh [--cluster-name NAME]

set -euo pipefail

CLUSTER_NAME="${SCENARIO_CLUSTER:-chorister-scenario}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster-name) CLUSTER_NAME="$2"; shift 2 ;;
    -h|--help)
      echo "Usage: $0 [--cluster-name NAME]"
      exit 0
      ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

if kind get clusters 2>/dev/null | grep -Fxq "$CLUSTER_NAME"; then
  echo "[$(date +%H:%M:%S)] Deleting Kind cluster '${CLUSTER_NAME}'"
  kind delete cluster --name "$CLUSTER_NAME"
else
  echo "[$(date +%H:%M:%S)] Kind cluster '${CLUSTER_NAME}' does not exist — nothing to do"
fi
