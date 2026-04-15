#!/usr/bin/env bash
# Run all scenario tests (sequential by default, or parallel with --parallel).
#
# Usage:
#   test/scenarios/run-all.sh [options]
#
# Options:
#   --cluster-name NAME   Kind cluster name (default: chorister-scenario)
#   --parallel            Run each scenario in its own cluster concurrently
#   --skip-setup          Skip cluster creation (assumes cluster exists)
#   --skip-teardown       Keep cluster after tests
#   --scenario N          Run only scenario N (e.g. --scenario 01)
#   -h, --help

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CLUSTER_NAME="${SCENARIO_CLUSTER:-chorister-scenario}"
PARALLEL=0
SKIP_SETUP=0
SKIP_TEARDOWN=0
ONLY_SCENARIO=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster-name) CLUSTER_NAME="$2"; shift 2 ;;
    --parallel) PARALLEL=1; shift ;;
    --skip-setup) SKIP_SETUP=1; shift ;;
    --skip-teardown) SKIP_TEARDOWN=1; shift ;;
    --scenario) ONLY_SCENARIO="$2"; shift 2 ;;
    -h|--help)
      sed -n '/^# /p' "$0" | sed 's/^# //'
      exit 0
      ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

export SCENARIO_CLUSTER="$CLUSTER_NAME"

# ── Cluster lifecycle ─────────────────────────────────────────────────────────

setup_cluster() {
  if [[ "$SKIP_SETUP" -eq 1 ]]; then
    echo "[setup] Skipping cluster creation (--skip-setup)"
    return 0
  fi
  bash "${SCRIPT_DIR}/setup-scenario-cluster.sh" \
    --cluster-name "$CLUSTER_NAME" \
    --plain-kind \
    --skip-apps
}

teardown_cluster() {
  if [[ "$SKIP_TEARDOWN" -eq 1 ]]; then
    echo "[teardown] Keeping cluster (--skip-teardown)"
    return 0
  fi
  bash "${SCRIPT_DIR}/teardown-scenario-cluster.sh" \
    --cluster-name "$CLUSTER_NAME"
}

# ── Scenario discovery ────────────────────────────────────────────────────────

find_scenarios() {
  local dirs=()
  if [[ -n "$ONLY_SCENARIO" ]]; then
    # Support both "01" and "01-platform-bootstrap"
    local pattern="${ONLY_SCENARIO}-"
    for dir in "${SCRIPT_DIR}"/[0-9][0-9]-*/; do
      if [[ "$(basename "$dir")" == "${ONLY_SCENARIO}"* ]]; then
        dirs+=("$dir")
      fi
    done
    if [[ "${#dirs[@]}" -eq 0 ]]; then
      echo "No scenario found matching: ${ONLY_SCENARIO}" >&2
      exit 1
    fi
  else
    for dir in "${SCRIPT_DIR}"/[0-9][0-9]-*/; do
      [[ -f "${dir}/run.sh" ]] && dirs+=("$dir")
    done
  fi
  printf '%s\n' "${dirs[@]}"
}

# ── Sequential run ────────────────────────────────────────────────────────────

run_sequential() {
  local pass=0
  local fail=0
  local results=()

  while IFS= read -r scenario_dir; do
    local name
    name="$(basename "$scenario_dir")"
    echo ""
    echo "════════════════════════════════════════"
    echo "  Scenario: ${name}"
    echo "════════════════════════════════════════"

    local rc=0
    bash "${scenario_dir}/run.sh" \
      --cluster-name "$CLUSTER_NAME" \
      --skip-setup \
      --skip-teardown \
      2>&1 || rc=$?

    if [[ "$rc" -eq 0 ]]; then
      pass=$((pass + 1))
      results+=("[PASS] ${name}")
    else
      fail=$((fail + 1))
      results+=("[FAIL] ${name}")
    fi
  done < <(find_scenarios)

  echo ""
  echo "════════════════════════════════════════"
  echo "  Summary"
  echo "════════════════════════════════════════"
  for r in "${results[@]}"; do
    echo "  $r"
  done
  echo ""
  echo "  Total: $((pass + fail)) | Passed: ${pass} | Failed: ${fail}"

  [[ "$fail" -eq 0 ]]
}

# ── Parallel run ──────────────────────────────────────────────────────────────

run_parallel() {
  local pids=()
  local names=()
  local tmpdir
  tmpdir="$(mktemp -d)"

  local i=0
  while IFS= read -r scenario_dir; do
    local name
    name="$(basename "$scenario_dir")"
    local cluster_name="chorister-scenario-${name:0:2}"
    local rc_file="${tmpdir}/${name}.rc"
    local log_file="${tmpdir}/${name}.log"

    (
      bash "${SCRIPT_DIR}/setup-scenario-cluster.sh" \
        --cluster-name "$cluster_name" \
        --plain-kind \
        --skip-apps \
        >"$log_file" 2>&1

      bash "${scenario_dir}/run.sh" \
        --cluster-name "$cluster_name" \
        --skip-setup \
        >>"$log_file" 2>&1
      echo $? >"$rc_file"

      bash "${SCRIPT_DIR}/teardown-scenario-cluster.sh" \
        --cluster-name "$cluster_name" \
        >>"$log_file" 2>&1
    ) &
    pids+=($!)
    names+=("$name")
    i=$((i + 1))
  done < <(find_scenarios)

  # Wait for all
  for j in "${!pids[@]}"; do
    wait "${pids[$j]}" || true
  done

  local pass=0
  local fail=0
  for name in "${names[@]}"; do
    local rc_file="${tmpdir}/${name}.rc"
    local log_file="${tmpdir}/${name}.log"
    local rc=0
    [[ -f "$rc_file" ]] && rc="$(cat "$rc_file")"
    if [[ "$rc" -eq 0 ]]; then
      pass=$((pass + 1))
      echo "[PASS] ${name}"
    else
      fail=$((fail + 1))
      echo "[FAIL] ${name}"
      echo "--- Log: ---"
      cat "$log_file"
      echo "--- End log ---"
    fi
  done

  rm -rf "$tmpdir"
  echo "Total: $((pass + fail)) | Passed: ${pass} | Failed: ${fail}"
  [[ "$fail" -eq 0 ]]
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  if [[ "$PARALLEL" -eq 1 ]]; then
    run_parallel
  else
    setup_cluster
    run_sequential
    local rc=$?
    teardown_cluster
    return $rc
  fi
}

main
