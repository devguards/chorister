#!/usr/bin/env bash
# Scenario 04: Network Isolation and Cross-Domain Traffic — assert.sh
# Requires: Cilium
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"
source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

APP_NAME="scen04-myapp"
PAYMENTS_NS="${APP_NAME}-payments"
AUTH_NS="${APP_NAME}-auth"
UNRELATED_NS="${APP_NAME}-unrelated"

# ── Helpers ───────────────────────────────────────────────────────────────────

require_cilium() {
  if ! kctl get daemonset -n kube-system cilium &>/dev/null; then
    echo "[SKIP] Scenario 04 requires Cilium. Cluster does not have Cilium installed. Skipping."
    exit 0
  fi
}

curl_from_pod() {
  local ns="$1" pod_label="$2" url="$3" timeout="${4:-5}"
  kctl exec -n "$ns" -l "$pod_label" -- \
    timeout "$timeout" wget -qO- --timeout="$timeout" "$url" 2>&1 || echo "CONNECTION_FAILED"
}

# ── Setup ─────────────────────────────────────────────────────────────────────

setup() {
  # STUB: chorister admin app create is not implemented — use kubectl
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-application.yaml"
  wait_for_namespace "$PAYMENTS_NS" 60
  wait_for_namespace "$AUTH_NS" 60
  wait_for_namespace "$UNRELATED_NS" 60

  # Deploy echo-api pod in each namespace
  for ns in "$PAYMENTS_NS" "$AUTH_NS" "$UNRELATED_NS"; do
    kctl apply -n "$ns" -f "${SCRIPT_DIR}/fixtures/echo-api-pod.yaml"
  done

  # Wait for pods to be ready
  for ns in "$PAYMENTS_NS" "$AUTH_NS" "$UNRELATED_NS"; do
    wait_for_pod_running "$ns" "app=echo-api" 90
  done
}

# ── 04-assert-cross-domain-allowed ────────────────────────────────────────────

assert_cross_domain_allowed() {
  # payments declares "consumes auth:8080" → should be allowed by Cilium
  local svc="echo-api.${AUTH_NS}.svc.cluster.local:8080"
  local out
  out="$(curl_from_pod "$PAYMENTS_NS" "app=echo-api" "http://${svc}/healthz")"
  if echo "$out" | grep -q "ok\|200\|healthz"; then
    _assert_pass "payments pod can reach auth:8080 (declared consume)"
  else
    _assert_fail "payments pod can reach auth:8080 (declared consume)" \
      "response: ${out}"
  fi
}

# ── 04-assert-wrong-port-blocked ─────────────────────────────────────────────

assert_wrong_port_blocked() {
  # payments only declared port 8080 for auth; port 9090 should be blocked
  local svc="echo-api.${AUTH_NS}.svc.cluster.local:9090"
  local out
  out="$(curl_from_pod "$PAYMENTS_NS" "app=echo-api" "http://${svc}/healthz" 3)"
  if echo "$out" | grep -q "CONNECTION_FAILED\|timeout\|refused"; then
    _assert_pass "payments pod cannot reach auth:9090 (undeclared port)"
  else
    _assert_fail "payments pod cannot reach auth:9090 should be blocked" \
      "got: ${out}"
  fi
}

# ── 04-assert-unrelated-blocked ───────────────────────────────────────────────

assert_unrelated_blocked() {
  # unrelated domain has no consume declaration for auth
  local svc="echo-api.${AUTH_NS}.svc.cluster.local:8080"
  local out
  out="$(curl_from_pod "$UNRELATED_NS" "app=echo-api" "http://${svc}/healthz" 3)"
  if echo "$out" | grep -q "CONNECTION_FAILED\|timeout\|refused"; then
    _assert_pass "unrelated pod cannot reach auth:8080 (no consume declaration)"
  else
    _assert_fail "unrelated pod should not reach auth:8080" "got: ${out}"
  fi
}

# ── 04-assert-reverse-blocked ────────────────────────────────────────────────

assert_reverse_blocked() {
  # auth does not declare consumes payments → should be blocked
  local svc="echo-api.${PAYMENTS_NS}.svc.cluster.local:8080"
  local out
  out="$(curl_from_pod "$AUTH_NS" "app=echo-api" "http://${svc}/healthz" 3)"
  if echo "$out" | grep -q "CONNECTION_FAILED\|timeout\|refused"; then
    _assert_pass "auth pod cannot reach payments (no reverse consume)"
  else
    _assert_fail "auth pod should not reach payments" "got: ${out}"
  fi
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

cleanup() {
  kctl delete choapplication "${APP_NAME}" --ignore-not-found=true 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 04: Network Isolation and Cross-Domain Traffic ---"
  require_cilium
  setup
  assert_cross_domain_allowed
  assert_wrong_port_blocked
  assert_unrelated_blocked
  assert_reverse_blocked
  cleanup
  print_summary
}
main
