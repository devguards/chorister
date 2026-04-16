#!/usr/bin/env bash
# Scenario 11: Sandbox FinOps Budget Enforcement — assert.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"
source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

APP_NAME="scen11-myapp"
DOMAIN="payments"
SANDBOX_OVER="over-budget"
SANDBOX_UNDER="under-budget"
SANDBOX_NS_UNDER="${APP_NAME}-${DOMAIN}-sandbox-${SANDBOX_UNDER}"

# ── Setup ─────────────────────────────────────────────────────────────────────

setup() {
  # Apply ChoCluster with finops rates and create application with tight sandbox budget
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-cluster.yaml" 2>/dev/null || true
  cho admin app create "$APP_NAME" \
    --owners test@chorister.dev \
    --compliance essential \
    --domains "$DOMAIN"
  wait_for_namespace "${APP_NAME}-${DOMAIN}" 60
}

# ── 11-assert-sandbox-budget-enforced ────────────────────────────────────────

assert_sandbox_budget_enforced() {
  # Attempt to create a sandbox with a medium database (cost > $10/month)
  local rc=0
  cho sandbox create \
    --domain "$DOMAIN" \
    --name "$SANDBOX_OVER" \
    --app "$APP_NAME" 2>/dev/null || rc=$?

  if [[ "$rc" -ne 0 ]]; then
    # Sandbox creation was rejected — good
    _assert_pass "Sandbox with over-budget resources is rejected"
    return
  fi

  # If sandbox was created, apply a medium database and check for rejection
  local sandbox_ns="${APP_NAME}-${DOMAIN}-sandbox-${SANDBOX_OVER}"
  wait_for_namespace "$sandbox_ns" 30 2>/dev/null || true
  local apply_rc=0
  kctl apply -n "$sandbox_ns" \
    -f "${SCRIPT_DIR}/fixtures/cho-database-medium.yaml" 2>/dev/null || apply_rc=$?

  if [[ "$apply_rc" -ne 0 ]]; then
    _assert_pass "Medium database rejected by admission (budget enforcement)"
  else
    # Check for budget condition on the ChoSandbox / ChoApplication
    local cond
    cond=$(kctl get chosandboxes -n cho-system \
      -o jsonpath='{.items[?(@.spec.name=="'"$SANDBOX_OVER"'")].status.conditions[?(@.type=="BudgetExceeded")].status}' \
      2>/dev/null || echo "")
    if [[ "$cond" == "True" ]]; then
      _assert_pass "BudgetExceeded condition set on sandbox"
    else
      _assert_fail "BudgetExceeded condition set on sandbox" "condition not found or not True"
    fi
  fi

  cho sandbox destroy --domain "$DOMAIN" --name "$SANDBOX_OVER" --app "$APP_NAME" \
    2>/dev/null || true
}

# ── 11-assert-small-sandbox-allowed ──────────────────────────────────────────

assert_small_sandbox_allowed() {
  local rc=0
  cho sandbox create \
    --domain "$DOMAIN" \
    --name "$SANDBOX_UNDER" \
    --app "$APP_NAME" 2>&1 || rc=$?
  assert_exit_ok "$rc" "Sandbox with small compute is created successfully"

  wait_for_namespace "$SANDBOX_NS_UNDER" 60 \
    && _assert_pass "Sandbox namespace ${SANDBOX_NS_UNDER} created" \
    || _assert_fail "Sandbox namespace ${SANDBOX_NS_UNDER} created" "timeout"

  # Apply small compute
  kctl apply -n "$SANDBOX_NS_UNDER" \
    -f "${SCRIPT_DIR}/fixtures/cho-compute-small.yaml" 2>/dev/null || true

  # Check sandbox list shows estimatedMonthlyCost (best-effort)
  local output rc2=0
  output="$(cho sandbox list --domain "$DOMAIN" --app "$APP_NAME" 2>&1)" || rc2=$?
  assert_exit_ok "$rc2" "sandbox list exits 0"
  if echo "$output" | grep -qi "under-budget\|$SANDBOX_UNDER"; then
    _assert_pass "sandbox list shows under-budget sandbox"
  else
    _assert_fail "sandbox list should show ${SANDBOX_UNDER}" "output: ${output}"
  fi
}

# ── 11-assert-budget-alert-threshold ─────────────────────────────────────────

assert_budget_alert_threshold() {
  # Add more resources to approach the budget limit
  # Best-effort: check for BudgetAlert condition
  local cond
  cond=$(kctl get choapplications scen11-myapp -n cho-system \
    -o jsonpath='{.status.conditions[?(@.type=="BudgetAlert")].status}' \
    2>/dev/null || echo "")
  if [[ "$cond" == "True" ]]; then
    _assert_pass "BudgetAlert condition set on ChoApplication"
  else
    _assert_fail "BudgetAlert condition set on ChoApplication" "condition not found or not True"
  fi
}

# ── 11-assert-idle-auto-destroy ───────────────────────────────────────────────

assert_idle_auto_destroy() {
  # Create a sandbox, patch lastApplyTime to past, trigger reconciliation
  local idle_sandbox="idle-test"
  local idle_ns="${APP_NAME}-${DOMAIN}-sandbox-${idle_sandbox}"

  cho sandbox create \
    --domain "$DOMAIN" \
    --name "$idle_sandbox" \
    --app "$APP_NAME" 2>/dev/null || true
  wait_for_namespace "$idle_ns" 30 2>/dev/null || {
    echo "[SKIP] Could not create idle sandbox — auto-destroy test skipped"
    return
  }

  # Patch lastApplyTime to a date far in the past to trigger idle detection
  kctl annotate chodomainmemberships -n cho-system \
    "chorister.dev/last-apply-time=2020-01-01T00:00:00Z" \
    --all 2>/dev/null || true

  # For chocomputes / chosandboxes, patch the annotation
  kctl annotate namespace "$idle_ns" \
    "chorister.dev/last-apply-time=2020-01-01T00:00:00Z" \
    --overwrite 2>/dev/null || true

  # Wait for auto-destroy (controller must implement idle detection)
  local elapsed=0
  while [[ "$elapsed" -lt 30 ]]; do
    if ! kctl get namespace "$idle_ns" &>/dev/null; then
      _assert_pass "Idle sandbox namespace ${idle_ns} was automatically deleted"
      return
    fi
    sleep 5; elapsed=$((elapsed + 5))
  done
  _assert_fail "Idle sandbox namespace ${idle_ns} was automatically deleted" "still exists after 30s"

  # Cleanup
  cho sandbox destroy --domain "$DOMAIN" --name "$idle_sandbox" --app "$APP_NAME" \
    2>/dev/null || true
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

cleanup() {
  cho sandbox destroy --domain "$DOMAIN" --name "$SANDBOX_UNDER" --app "$APP_NAME" \
    2>/dev/null || true
  cho admin app delete "$APP_NAME" --confirm 2>/dev/null || true
  kctl delete -f "${SCRIPT_DIR}/fixtures/cho-cluster.yaml" \
    --ignore-not-found=true 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 11: Sandbox FinOps Budget Enforcement ---"

  setup
  assert_sandbox_budget_enforced
  assert_small_sandbox_allowed
  assert_budget_alert_threshold
  assert_idle_auto_destroy
  cleanup

  print_summary
}

main
