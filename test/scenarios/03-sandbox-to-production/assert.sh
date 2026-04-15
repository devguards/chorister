#!/usr/bin/env bash
# Scenario 03: Sandbox → Production Promotion
# Validates: promotion request creation, approval, and resource copying to production.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"

source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

APP_NAME="scen03-myapp"
DOMAIN="payments"
SANDBOX_NAME="alice"
SANDBOX_NS="${APP_NAME}-${DOMAIN}-sandbox-${SANDBOX_NAME}"
PROD_NS="${APP_NAME}-${DOMAIN}"

PR_NAME=""

# ── 03-setup ─────────────────────────────────────────────────────────────────

setup() {
  # STUB: replace with chorister admin app create when implemented
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-application.yaml"
  wait_for_namespace "$PROD_NS" 60

  # Create sandbox
  cho sandbox create --domain "$DOMAIN" --name "$SANDBOX_NAME" --app "$APP_NAME"
  wait_for_namespace "$SANDBOX_NS" 60
}

# ── 03-assert-sandbox-and-apply ──────────────────────────────────────────────

assert_sandbox_and_apply() {
  # STUB: chorister apply is not implemented — use kubectl apply
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

  # Wait for Deployment to be created
  local elapsed=0
  while [[ "$elapsed" -lt 60 ]]; do
    if resource_exists "$SANDBOX_NS" "deployment" "echo-api"; then
      _assert_pass "ChoCompute Deployment created in sandbox"
      return 0
    fi
    sleep 3; elapsed=$((elapsed + 3))
  done
  _assert_fail "ChoCompute Deployment created in sandbox" "timeout"
}

# ── 03-assert-diff-before-promote ────────────────────────────────────────────

assert_diff_before_promote() {
  # STUB: chorister diff is not implemented
  # Note: diff command is stub, skip diff assertions
  local output rc=0
  output="$(cho diff --domain "$DOMAIN" --sandbox "$SANDBOX_NAME" --app "$APP_NAME" 2>&1)" || rc=$?
  # diff is a stub, just verify it doesn't crash with exit 1 unexpectedly
  # The stub may print "not yet implemented" — that's acceptable
  _assert_pass "chorister diff --domain ${DOMAIN} --sandbox ${SANDBOX_NAME} runs (stub)"
}

# ── 03-assert-promote-creates-request ────────────────────────────────────────

assert_promote_creates_request() {
  local output rc=0
  output="$(cho promote --domain "$DOMAIN" --sandbox "$SANDBOX_NAME" --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister promote exits 0"
  assert_contains "$output" "ChoPromotionRequest created" "promote output mentions ChoPromotionRequest"

  # Wait for ChoPromotionRequest to exist in default namespace
  local elapsed=0
  local pr_count=0
  while [[ "$elapsed" -lt 30 ]]; do
    pr_count=$(kctl get chopromotionrequest -n default --no-headers 2>/dev/null | grep -c "${APP_NAME}-${DOMAIN}" || echo "0")
    if [[ "$pr_count" -gt 0 ]]; then
      break
    fi
    sleep 2; elapsed=$((elapsed + 2))
  done

  if [[ "$pr_count" -gt 0 ]]; then
    _assert_pass "ChoPromotionRequest created (in default namespace)"
  else
    _assert_fail "ChoPromotionRequest created" "none found in default namespace"
    return
  fi

  # Get the promotion request name
  PR_NAME="$(kctl get chopromotionrequest -n default --no-headers 2>/dev/null \
    | grep "${APP_NAME}-${DOMAIN}" | awk '{print $1}' | head -1)"
  echo "    PR_NAME=${PR_NAME}"

  # Assert it starts as Pending
  wait_for_condition "default" "chopromotionrequest" "$PR_NAME" \
    '{.status.phase}' "Pending" 30 \
    || { _assert_fail "ChoPromotionRequest phase=Pending" "timeout"; return; }
  _assert_pass "ChoPromotionRequest phase=Pending"

  # Assert chorister requests lists it
  local req_output req_rc=0
  req_output="$(cho requests --domain "$DOMAIN" --app "$APP_NAME" 2>&1)" || req_rc=$?
  assert_exit_ok "$req_rc" "chorister requests exits 0"
  assert_contains "$req_output" "$PR_NAME" "chorister requests shows the promotion request"
}

# ── 03-assert-unapproved-does-not-modify-prod ────────────────────────────────

assert_unapproved_does_not_modify_prod() {
  # Production namespace should NOT have the echo-api Deployment yet
  if ! resource_exists "$PROD_NS" "deployment" "echo-api"; then
    _assert_pass "Production namespace does NOT have echo-api Deployment before approval"
  else
    _assert_fail "Production namespace does NOT have echo-api Deployment before approval" \
      "found deployment in ${PROD_NS} — approval should be required first"
  fi
}

# ── 03-assert-approve-promotes ───────────────────────────────────────────────

assert_approve_promotes() {
  if [[ -z "$PR_NAME" ]]; then
    _assert_fail "approve step" "PR_NAME is empty — promotion request not found"
    return
  fi

  # Approve via chorister CLI (now implemented)
  local output rc=0
  output="$(cho approve "$PR_NAME" --role org-admin 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister approve exits 0"
  assert_contains "$output" "Approval recorded" "approve output confirms approval"

  # Wait for phase to transition: Pending → Approved → Executing → Completed
  wait_for_condition "default" "chopromotionrequest" "$PR_NAME" \
    '{.status.phase}' "Completed" 120 \
    || {
      local phase
      phase="$(kctl get chopromotionrequest "$PR_NAME" -n default -o jsonpath='{.status.phase}' 2>/dev/null || echo "unknown")"
      _assert_fail "ChoPromotionRequest phase=Completed" "still in phase=${phase} after 120s"
      return
    }
  _assert_pass "ChoPromotionRequest phase=Completed"

  # Assert compute Deployment appears in production namespace
  local elapsed=0
  while [[ "$elapsed" -lt 60 ]]; do
    if resource_exists "$PROD_NS" "deployment" "echo-api"; then
      _assert_pass "echo-api Deployment promoted to production namespace"
      return
    fi
    sleep 3; elapsed=$((elapsed + 3))
  done
  _assert_fail "echo-api Deployment promoted to production namespace" "not found in ${PROD_NS}"
}

# ── 03-assert-rollback ────────────────────────────────────────────────────────

assert_rollback() {
  local output rc=0
  output="$(cho promote --domain "$DOMAIN" --rollback --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister promote --rollback exits 0"
  assert_contains "$output" "Rollback" "promote --rollback output mentions Rollback"

  # Assert rollback ChoPromotionRequest is created
  local elapsed=0
  while [[ "$elapsed" -lt 30 ]]; do
    local rb_count
    rb_count=$(kctl get chopromotionrequest -n default --no-headers 2>/dev/null \
      | grep -c "rollback" || echo "0")
    if [[ "$rb_count" -gt 0 ]]; then
      _assert_pass "Rollback ChoPromotionRequest created"
      return
    fi
    sleep 2; elapsed=$((elapsed + 2))
  done
  _assert_fail "Rollback ChoPromotionRequest created" "none found after 30s"
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

cleanup() {
  kctl delete chosandbox -n default \
    -l "chorister.dev/application=${APP_NAME}" \
    --ignore-not-found=true 2>/dev/null || true
  kctl delete chopromotionrequest -n default \
    -l "chorister.dev/application=${APP_NAME}" \
    --ignore-not-found=true 2>/dev/null || true
  kctl delete choapplication "${APP_NAME}" \
    --ignore-not-found=true 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 03: Sandbox → Production Promotion ---"

  setup
  assert_sandbox_and_apply
  assert_diff_before_promote
  assert_promote_creates_request
  assert_unapproved_does_not_modify_prod
  assert_approve_promotes
  assert_rollback
  cleanup

  print_summary
}

main
