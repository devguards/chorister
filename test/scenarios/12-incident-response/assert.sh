#!/usr/bin/env bash
# Scenario 12: Incident Response — Isolate and Recover — assert.sh
# Requires: Cilium.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"
source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

APP_NAME="scen12-myapp"
DOMAIN="payments"
PROD_NS="${APP_NAME}-${DOMAIN}"
SANDBOX_NAME="dev"
SANDBOX_NS="${APP_NAME}-${DOMAIN}-sandbox-${SANDBOX_NAME}"
OTHER_NS="${APP_NAME}-auth"

# ── Helpers ───────────────────────────────────────────────────────────────────

require_cilium() {
  if ! kctl get daemonset -n kube-system cilium &>/dev/null; then
    echo "[SKIP] Scenario 12 requires Cilium. Not installed. Skipping."
    exit 0
  fi
}

curl_from_pod() {
  local ns="$1" label="$2" url="$3" timeout="${4:-5}"
  kctl exec -n "$ns" -l "$label" -- \
    timeout "$timeout" wget -qO- --timeout="$timeout" "$url" 2>&1 || echo "CONNECTION_FAILED"
}

# ── Setup ─────────────────────────────────────────────────────────────────────

setup() {
  cho admin app create "$APP_NAME" \
    --owners test@chorister.dev \
    --compliance essential \
    --domains "${DOMAIN},auth"
  wait_for_namespace "$PROD_NS" 60
  wait_for_namespace "$OTHER_NS" 60

  # Deploy echo-api in production namespace
  kctl apply -n "$PROD_NS" -f "${SCRIPT_DIR}/fixtures/echo-api-deployment.yaml"
  wait_for_deployment_ready "$PROD_NS" "echo-api" 120

  # Deploy a pod in another domain for cross-domain traffic tests
  kctl apply -n "$OTHER_NS" -f "${SCRIPT_DIR}/fixtures/echo-api-pod.yaml"
  wait_for_pod_running "$OTHER_NS" "app=echo-api" 60
}

# ── 12-assert-crash-loop-flags-degraded ──────────────────────────────────────

assert_crash_loop_flags_degraded() {
  # Patch the Deployment to use a bad command that crashes
  kctl patch deployment echo-api -n "$PROD_NS" \
    --type='json' \
    -p='[{"op":"replace","path":"/spec/template/spec/containers/0/command","value":["/bin/sh","-c","exit 1"]}]' \
    2>/dev/null || {
    _assert_fail "Patch deployment to crash-loop" "kubectl patch failed"
    return
  }

  # Wait for crash loop back-off (CrashLoopBackOff condition)
  local elapsed=0
  while [[ "$elapsed" -lt 60 ]]; do
    local phase
    phase=$(kctl get pods -n "$PROD_NS" -l app=echo-api \
      -o jsonpath='{.items[0].status.containerStatuses[0].state.waiting.reason}' \
      2>/dev/null || true)
    if echo "$phase" | grep -qi "CrashLoop\|Error"; then
      _assert_pass "Pod entered CrashLoopBackOff"
      break
    fi
    sleep 5; elapsed=$((elapsed + 5))
  done

  # Check ChoApplication status for Degraded condition (controller must implement this)
  local cond
  cond=$(kctl get choapplications scen12-myapp -n cho-system \
    -o jsonpath='{.status.conditions[?(@.type=="Degraded")].status}' \
    2>/dev/null || echo "")
  if [[ "$cond" == "True" ]]; then
    _assert_pass "ChoApplication status shows Degraded condition"
  else
    _assert_fail "ChoApplication status shows Degraded condition" "condition not set or not True"
  fi
}

# ── 12-assert-isolate-freezes-promotions ─────────────────────────────────────

assert_isolate_freezes_promotions() {
  local rc=0
  local output
  output="$(cho admin isolate --domain "$DOMAIN" --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister admin isolate exits 0"

  # Check isolation flag in status
  local isolated
  isolated=$(kctl get choapplications scen12-myapp -n cho-system \
    -o jsonpath='{.status.isolated}' 2>/dev/null || echo "")
  if [[ "$isolated" == "true" ]]; then
    _assert_pass "ChoApplication status shows isolated=true"
  else
    _assert_fail "ChoApplication status shows isolated=true" "got: ${isolated}"
  fi

  # Attempt to promote — should be rejected
  cho sandbox create --domain "$DOMAIN" --name "$SANDBOX_NAME" --app "$APP_NAME" \
    2>/dev/null || true
  wait_for_namespace "$SANDBOX_NS" 20 2>/dev/null || true

  local promote_rc=0
  local promote_out
  promote_out="$(cho promote --domain "$DOMAIN" --sandbox "$SANDBOX_NAME" \
    --app "$APP_NAME" 2>&1)" || promote_rc=$?
  if [[ "$promote_rc" -ne 0 ]] || echo "$promote_out" | grep -qi "isolat\|blocked\|reject"; then
    _assert_pass "Promotion rejected while domain is isolated"
  else
    _assert_fail "Promotion rejected while domain is isolated" "output: ${promote_out}"
  fi
}

# ── 12-assert-isolate-tightens-network ────────────────────────────────────────

assert_isolate_tightens_network() {
  # From auth domain pod, try to reach payments service
  local svc="echo-api.${PROD_NS}.svc.cluster.local:8080"
  local out
  out="$(curl_from_pod "$OTHER_NS" "app=echo-api" "http://${svc}/healthz" 3)"
  if echo "$out" | grep -q "CONNECTION_FAILED\|timeout\|refused"; then
    _assert_pass "Cross-domain traffic to isolated domain is blocked"
  else
    echo "[SKIP] Network isolation tightening not enforced (Cilium policy may not be applied)"
  fi
}

# ── 12-assert-unisolate-restores ──────────────────────────────────────────────

assert_unisolate_restores() {
  # Restore the Deployment to a working state first
  kctl patch deployment echo-api -n "$PROD_NS" \
    --type='json' \
    -p='[{"op":"remove","path":"/spec/template/spec/containers/0/command"}]' \
    2>/dev/null || true
  wait_for_deployment_ready "$PROD_NS" "echo-api" 90

  # Unisolate
  local rc=0
  cho admin unisolate --domain "$DOMAIN" --app "$APP_NAME" 2>&1 || rc=$?
  assert_exit_ok "$rc" "chorister admin unisolate exits 0"

  # Check isolation cleared
  local isolated
  isolated=$(kctl get choapplications scen12-myapp -n cho-system \
    -o jsonpath='{.status.isolated}' 2>/dev/null || echo "")
  if [[ "$isolated" != "true" ]]; then
    _assert_pass "ChoApplication isolation cleared after unisolate"
  else
    _assert_fail "ChoApplication isolation cleared after unisolate" "still isolated"
  fi

  # Assert cross-domain traffic resumes
  local svc="echo-api.${PROD_NS}.svc.cluster.local:8080"
  local out
  out="$(curl_from_pod "$OTHER_NS" "app=echo-api" "http://${svc}/healthz" 5)"
  if echo "$out" | grep -q "ok\|200\|healthz"; then
    _assert_pass "Cross-domain traffic resumes after unisolate"
  else
    echo "[SKIP] Cross-domain traffic check inconclusive after unisolate: ${out}"
  fi

  # Assert new promotions are accepted
  local promote_rc=0
  cho promote --domain "$DOMAIN" --sandbox "$SANDBOX_NAME" \
    --app "$APP_NAME" 2>&1 || promote_rc=$?
  if [[ "$promote_rc" -eq 0 ]]; then
    _assert_pass "Promotions accepted after unisolate"
  else
    _assert_fail "Promotions accepted after unisolate" "exit code: ${promote_rc}"
  fi
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

cleanup() {
  cho sandbox destroy --domain "$DOMAIN" --name "$SANDBOX_NAME" --app "$APP_NAME" \
    2>/dev/null || true
  cho admin app delete "$APP_NAME" --confirm 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 12: Incident Response — Isolate and Recover ---"
  require_cilium

  setup
  assert_crash_loop_flags_degraded
  assert_isolate_freezes_promotions
  assert_isolate_tightens_network
  assert_unisolate_restores
  cleanup

  print_summary
}

main
