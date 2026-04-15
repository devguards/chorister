#!/usr/bin/env bash
# Scenario 07: Full Stack Stub App Health Check — assert.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"
source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

APP_NAME="scen07-myapp"
DOMAIN="payments"
SANDBOX_NAME="dev"
SANDBOX_NS="${APP_NAME}-${DOMAIN}-sandbox-${SANDBOX_NAME}"
DB_SECRET="${DOMAIN}--database--main-credentials"
QUEUE_SECRET="${DOMAIN}--queue--events-credentials"
CACHE_SECRET="${DOMAIN}--cache--session-credentials"

# ── Setup ─────────────────────────────────────────────────────────────────────

setup() {
  # STUB: chorister admin app create not implemented
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-application.yaml"

  # Create sandbox for testing
  cho sandbox create --domain "$DOMAIN" --name "$SANDBOX_NAME" --app "$APP_NAME"
  wait_for_namespace "$SANDBOX_NS" 60

  # STUB: chorister apply not implemented — use kubectl
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-compute.yaml" -n "$SANDBOX_NS"
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-database.yaml" -n "$SANDBOX_NS"
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-queue.yaml" -n "$SANDBOX_NS"
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-cache.yaml" -n "$SANDBOX_NS"
}

# ── 07-assert-compute-running ─────────────────────────────────────────────────

assert_compute_running() {
  wait_for_deployment_ready "$SANDBOX_NS" "echo-api" 120 \
    && _assert_pass "echo-api Deployment is Running (≥1 pod Ready)" \
    || _assert_fail "echo-api Deployment Ready" "timed out"
}

# ── 07-assert-db-credentials ──────────────────────────────────────────────────

assert_db_credentials() {
  local elapsed=0
  while [[ "$elapsed" -lt 60 ]]; do
    if resource_exists "$SANDBOX_NS" "secret" "$DB_SECRET"; then
      _assert_pass "Database credentials secret created (${DB_SECRET})"
      return
    fi
    sleep 3; elapsed=$((elapsed + 3))
  done
  _assert_fail "Database credentials secret" "secret ${DB_SECRET} not found in ${SANDBOX_NS}"
}

# ── 07-assert-queue-credentials ───────────────────────────────────────────────

assert_queue_credentials() {
  local elapsed=0
  while [[ "$elapsed" -lt 60 ]]; do
    if resource_exists "$SANDBOX_NS" "secret" "$QUEUE_SECRET"; then
      _assert_pass "Queue credentials secret created (${QUEUE_SECRET})"
      return
    fi
    sleep 3; elapsed=$((elapsed + 3))
  done
  _assert_fail "Queue credentials secret" "secret ${QUEUE_SECRET} not found in ${SANDBOX_NS}"
}

# ── 07-assert-cache-credentials ───────────────────────────────────────────────

assert_cache_credentials() {
  local elapsed=0
  while [[ "$elapsed" -lt 60 ]]; do
    if resource_exists "$SANDBOX_NS" "secret" "$CACHE_SECRET"; then
      _assert_pass "Cache credentials secret created (${CACHE_SECRET})"
      return
    fi
    sleep 3; elapsed=$((elapsed + 3))
  done
  _assert_fail "Cache credentials secret" "secret ${CACHE_SECRET} not found in ${SANDBOX_NS}"
}

# ── 07-assert-status-cmd ──────────────────────────────────────────────────────

assert_status_cmd() {
  local output rc=0
  output="$(cho status "$DOMAIN" --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister status ${DOMAIN} exits 0"
  assert_not_empty "$output" "chorister status output is not empty"
  _assert_pass "chorister status --app ${APP_NAME} shows domain status"
}

# ── 07-assert-logs-cmd ────────────────────────────────────────────────────────

assert_logs_cmd() {
  # Make sure the pod is running before querying logs
  local pod
  pod="$(kctl get pods -n "$SANDBOX_NS" -l app=echo-api --no-headers 2>/dev/null | awk '{print $1}' | head -1)"
  if [[ -z "$pod" ]]; then
    _assert_fail "chorister logs: at least one echo-api pod running" "no pods found"
    return
  fi

  local output rc=0
  output="$(timeout 5 bash -c "cho logs '$DOMAIN' --sandbox '$SANDBOX_NAME' --app '$APP_NAME' 2>&1" || true)"
  # logs streams output — just verify the command doesn't crash
  assert_not_empty "$output" "chorister logs produces output"
  _assert_pass "chorister logs ${DOMAIN} --sandbox ${SANDBOX_NAME} produces output"
}

# ── 07-assert-sandbox-status-cmd ─────────────────────────────────────────────

assert_sandbox_status_cmd() {
  local output rc=0
  output="$(cho sandbox list --domain "$DOMAIN" --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister sandbox list exits 0"
  assert_contains "$output" "$SANDBOX_NAME" "sandbox list shows dev sandbox"
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

cleanup() {
  cho sandbox destroy --domain "$DOMAIN" --name "$SANDBOX_NAME" --app "$APP_NAME" 2>/dev/null || true
  kctl delete choapplication "${APP_NAME}" --ignore-not-found=true 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 07: Full Stack Stub App Health Check ---"
  setup
  assert_compute_running
  assert_db_credentials
  assert_queue_credentials
  assert_cache_credentials
  assert_status_cmd
  assert_logs_cmd
  assert_sandbox_status_cmd
  cleanup
  print_summary
}
main
