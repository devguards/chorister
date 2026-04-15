#!/usr/bin/env bash
# Scenario 02: Developer Sandbox Workflow
# Validates: sandbox create/destroy, applying compute/database resources.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"

source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

APP_NAME="scen02-myapp"
DOMAIN="payments"
SANDBOX_NAME="alice"
SANDBOX_NS="${APP_NAME}-${DOMAIN}-sandbox-${SANDBOX_NAME}"

# ── 02-setup ─────────────────────────────────────────────────────────────────

setup() {
  # Create ChoApplication (CLI stub — use kubectl)
  # STUB: replace with chorister admin app create when implemented
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-application.yaml"
  wait_for_namespace "${APP_NAME}-${DOMAIN}" 60
}

# ── 02-assert-sandbox-create ─────────────────────────────────────────────────

assert_sandbox_create() {
  local output rc=0
  output="$(cho sandbox create --domain "$DOMAIN" --name "$SANDBOX_NAME" --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister sandbox create exits 0"
  assert_contains "$output" "${SANDBOX_NAME}" "sandbox create output mentions sandbox name"

  # Wait for controller to create namespace
  wait_for_namespace "$SANDBOX_NS" 60 \
    || { _assert_fail "Sandbox namespace ${SANDBOX_NS} created" "timeout"; return; }
  _assert_pass "Sandbox namespace ${SANDBOX_NS} created"

  # Assert default-deny NetworkPolicy
  if resource_exists "$SANDBOX_NS" "networkpolicy" "default-deny"; then
    _assert_pass "default-deny NetworkPolicy in sandbox namespace"
  else
    _assert_fail "default-deny NetworkPolicy in sandbox namespace" "not found"
  fi
}

# ── 02-assert-sandbox-list ────────────────────────────────────────────────────

assert_sandbox_list() {
  local output rc=0
  output="$(cho sandbox list --domain "$DOMAIN" --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister sandbox list exits 0"
  assert_contains "$output" "$SANDBOX_NAME" "sandbox list shows alice"
}

# ── 02-assert-apply-compute ──────────────────────────────────────────────────

assert_apply_compute() {
  # STUB: chorister apply is not implemented — use kubectl apply
  # STUB: replace with 'chorister apply --domain $DOMAIN --sandbox $SANDBOX_NAME --file ...' when implemented
  kctl apply -f - -n "$SANDBOX_NS" <<EOF
apiVersion: chorister.dev/v1alpha1
kind: ChoCompute
metadata:
  name: echo-api
  namespace: ${SANDBOX_NS}
spec:
  application: ${APP_NAME}
  domain: ${DOMAIN}
  image: nginx:latest
  replicas: 1
  port: 8080
  variant: long-running
EOF

  # Wait for Deployment to be created by controller
  local elapsed=0
  local found=0
  while [[ "$elapsed" -lt 60 ]]; do
    if resource_exists "$SANDBOX_NS" "deployment" "echo-api"; then
      found=1
      break
    fi
    sleep 3
    elapsed=$((elapsed + 3))
  done

  if [[ "$found" -eq 1 ]]; then
    _assert_pass "Deployment echo-api created in sandbox namespace"
  else
    _assert_fail "Deployment echo-api created in sandbox namespace" "timeout after 60s"
  fi

  # Check Service is created when port is declared
  elapsed=0
  found=0
  while [[ "$elapsed" -lt 30 ]]; do
    if resource_exists "$SANDBOX_NS" "service" "echo-api"; then
      found=1
      break
    fi
    sleep 3
    elapsed=$((elapsed + 3))
  done
  if [[ "$found" -eq 1 ]]; then
    _assert_pass "Service echo-api created (port declared)"
  else
    _assert_fail "Service echo-api created (port declared)" "timeout after 30s"
  fi
}

# ── 02-assert-apply-database ─────────────────────────────────────────────────

assert_apply_database() {
  # STUB: chorister apply is not implemented — use kubectl apply
  kctl apply -f - -n "$SANDBOX_NS" <<EOF
apiVersion: chorister.dev/v1alpha1
kind: ChoDatabase
metadata:
  name: db
  namespace: ${SANDBOX_NS}
spec:
  engine: postgres
  ha: false
  size: small
EOF

  # Wait for credentials Secret to be created by controller
  local secret_name="${DOMAIN}--database--db-credentials"
  local elapsed=0
  local found=0
  while [[ "$elapsed" -lt 60 ]]; do
    if resource_exists "$SANDBOX_NS" "secret" "$secret_name"; then
      found=1
      break
    fi
    sleep 3
    elapsed=$((elapsed + 3))
  done

  if [[ "$found" -eq 1 ]]; then
    _assert_pass "Database credentials secret created"
  else
    _assert_fail "Database credentials secret created" "expected secret ${secret_name} in ${SANDBOX_NS}"
  fi
}

# ── 02-assert-sandbox-status ─────────────────────────────────────────────────

assert_sandbox_status() {
  local output rc=0
  output="$(cho status "$DOMAIN" --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister status ${DOMAIN} exits 0"
}

# ── 02-assert-sandbox-destroy ────────────────────────────────────────────────

assert_sandbox_destroy() {
  local output rc=0
  output="$(cho sandbox destroy --domain "$DOMAIN" --name "$SANDBOX_NAME" --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister sandbox destroy exits 0"

  # Wait for namespace to be deleted by controller
  wait_for_namespace_gone "$SANDBOX_NS" 90 \
    || { _assert_fail "Sandbox namespace ${SANDBOX_NS} deleted" "timeout"; return; }
  _assert_pass "Sandbox namespace ${SANDBOX_NS} deleted"

  # Verify sandbox is no longer listed
  local list_output list_rc=0
  list_output="$(cho sandbox list --domain "$DOMAIN" --app "$APP_NAME" 2>&1)" || list_rc=$?
  assert_exit_ok "$list_rc" "sandbox list after destroy exits 0"
  assert_not_contains "$list_output" "$SANDBOX_NAME" "sandbox list no longer shows alice after destroy"
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

cleanup() {
  kctl delete chosandbox -n default \
    -l "chorister.dev/application=${APP_NAME}" \
    --ignore-not-found=true 2>/dev/null || true
  kctl delete choapplication "${APP_NAME}" \
    --ignore-not-found=true 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 02: Developer Sandbox Workflow ---"

  setup
  assert_sandbox_create
  assert_sandbox_list
  assert_apply_compute
  assert_apply_database
  assert_sandbox_status
  assert_sandbox_destroy
  cleanup

  print_summary
}

main
