#!/usr/bin/env bash
# Scenario 01: Platform Bootstrap
# Validates: CRD installation, controller health, basic CLI commands.
# No Cilium required.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"

source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

APP_NAME="scen01-myapp"

# ── 01-assert-setup ───────────────────────────────────────────────────────────

assert_setup_dry_run() {
  local output rc=0
  output="$(cho setup --dry-run 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister setup --dry-run exits 0"
  assert_contains "$output" "Dry run" "setup --dry-run prints Dry run message"
}

assert_setup_idempotent() {
  local rc=0
  # First actual setup
  cho setup 2>&1 || rc=$?
  assert_exit_ok "$rc" "chorister setup exits 0 (first run)"

  # Second setup — idempotent, should also succeed
  rc=0
  cho setup 2>&1 || rc=$?
  assert_exit_ok "$rc" "chorister setup exits 0 (second run — idempotent)"

  # Verify CRDs still registered after double setup
  assert_crds_registered
}

# ── 01-assert-cli-version ─────────────────────────────────────────────────────

assert_cli_version() {
  local output rc=0
  output="$(cho version 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister version exits 0"
  assert_not_empty "$output" "chorister version prints non-empty output"
  assert_contains "$output" "chorister" "version output contains 'chorister'"
}

# ── 01-assert-crds ───────────────────────────────────────────────────────────

assert_crds_registered() {
  local crds=(
    choapplications.chorister.dev
    chosandboxes.chorister.dev
    chocomputes.chorister.dev
    chodatabases.chorister.dev
    choqueues.chorister.dev
    chocaches.chorister.dev
    chonetworks.chorister.dev
    chostorages.chorister.dev
    chodomainmemberships.chorister.dev
    chopromotionrequests.chorister.dev
    chovulnerabilityreports.chorister.dev
    choclusters.chorister.dev
  )
  local ok=0
  local fail=0
  for crd in "${crds[@]}"; do
    if resource_exists_cluster "crd" "$crd"; then
      ok=$((ok + 1))
    else
      fail=$((fail + 1))
      echo "[FAIL] CRD not registered: ${crd}"
    fi
  done
  if [[ "$fail" -eq 0 ]]; then
    _assert_pass "All 12 CRDs are registered (${ok}/12)"
  else
    _assert_fail "CRD registration" "${fail} CRDs missing, ${ok}/12 registered"
  fi
}

# ── 01-assert-cluster-bootstrap ──────────────────────────────────────────────

assert_app_list_empty() {
  local output rc=0
  output="$(cho admin app list 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister admin app list exits 0 (even when empty)"
}

# ── 01-assert-app-create ─────────────────────────────────────────────────────

assert_app_create() {
  cho admin app create "${APP_NAME}" \
    --owners platform-admin@example.com \
    --compliance essential \
    --domains payments,auth

  # Wait for controller to create domain namespaces
  wait_for_namespace "${APP_NAME}-payments" 60 \
    || { _assert_fail "Domain namespace ${APP_NAME}-payments created" "timeout"; return; }
  _assert_pass "Domain namespace ${APP_NAME}-payments created"

  wait_for_namespace "${APP_NAME}-auth" 60 \
    || { _assert_fail "Domain namespace ${APP_NAME}-auth created" "timeout"; return; }
  _assert_pass "Domain namespace ${APP_NAME}-auth created"

  # Assert default-deny NetworkPolicy exists in each namespace
  if resource_exists "${APP_NAME}-payments" "networkpolicy" "default-deny"; then
    _assert_pass "default-deny NetworkPolicy in ${APP_NAME}-payments"
  else
    _assert_fail "default-deny NetworkPolicy in ${APP_NAME}-payments" "not found"
  fi

  if resource_exists "${APP_NAME}-auth" "networkpolicy" "default-deny"; then
    _assert_pass "default-deny NetworkPolicy in ${APP_NAME}-auth"
  else
    _assert_fail "default-deny NetworkPolicy in ${APP_NAME}-auth" "not found"
  fi
}

# ── 01-assert-status ─────────────────────────────────────────────────────────

assert_status_shows_domains() {
  local output rc=0
  output="$(cho status --app "${APP_NAME}" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister status --app ${APP_NAME} exits 0"
  assert_contains "$output" "payments" "status output mentions payments domain"
  assert_contains "$output" "auth" "status output mentions auth domain"
}

# ── 01-cleanup ────────────────────────────────────────────────────────────────

cleanup() {
  kctl delete -f "${SCRIPT_DIR}/fixtures/cho-application.yaml" \
    --ignore-not-found=true 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 01: Platform Bootstrap ---"

  assert_setup_dry_run
  assert_setup_idempotent
  assert_cli_version
  assert_crds_registered
  assert_app_list_empty
  assert_app_create
  assert_status_shows_domains
  cleanup

  print_summary
}

main
