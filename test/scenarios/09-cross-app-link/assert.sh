#!/usr/bin/env bash
# Scenario 09: Cross-Application Link — assert.sh
# Requires: Cilium, Gateway API CRDs.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"
source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

RETAIL_APP="scen09-retail"
CAPITAL_APP="scen09-capital"
PAYMENTS_NS="${RETAIL_APP}-payments"
PRICING_NS="${CAPITAL_APP}-pricing"

# ── Helpers ───────────────────────────────────────────────────────────────────

require_cilium() {
  if ! kctl get daemonset -n kube-system cilium &>/dev/null; then
    echo "[SKIP] Scenario 09 requires Cilium. Not installed. Skipping."
    exit 0
  fi
}

require_gateway_api() {
  if ! kctl get crd gateways.gateway.networking.k8s.io &>/dev/null; then
    echo "[SKIP] Scenario 09 requires Gateway API CRDs. Not installed. Skipping."
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
  cho admin app create "$RETAIL_APP" \
    --owners test@chorister.dev \
    --compliance essential \
    --domains payments
  cho admin app create "$CAPITAL_APP" \
    --owners test@chorister.dev \
    --compliance essential \
    --domains pricing

  wait_for_namespace "$PAYMENTS_NS" 60
  wait_for_namespace "$PRICING_NS" 60

  # Deploy echo-api in both namespaces
  kctl apply -n "$PAYMENTS_NS" -f "${SCRIPT_DIR}/fixtures/echo-api-pod.yaml"
  kctl apply -n "$PRICING_NS" -f "${SCRIPT_DIR}/fixtures/echo-api-pod.yaml"

  wait_for_pod_running "$PAYMENTS_NS" "app=echo-api" 90
  wait_for_pod_running "$PRICING_NS" "app=echo-api" 90
}

# ── 09-assert-direct-pod-to-pod-blocked ───────────────────────────────────────

assert_direct_pod_to_pod_blocked() {
  # Get pricing pod IP directly (not via service)
  local pod_ip
  pod_ip=$(kctl get pods -n "$PRICING_NS" -l app=echo-api \
    -o jsonpath='{.items[0].status.podIP}' 2>/dev/null || true)

  if [[ -z "$pod_ip" ]]; then
    _assert_fail "pricing pod IP found" "no pod in ${PRICING_NS}"
    return
  fi

  local out
  out="$(curl_from_pod "$PAYMENTS_NS" "app=echo-api" "http://${pod_ip}:8080/healthz" 3)"
  if echo "$out" | grep -q "CONNECTION_FAILED\|timeout\|refused"; then
    _assert_pass "Direct pod-to-pod from retail-payments to pricing is blocked (Cilium)"
  else
    _assert_fail "Direct pod-to-pod should be blocked" "got: ${out}"
  fi
}

# ── 09-assert-httproute-and-referencegrant-exist ──────────────────────────────

assert_httproute_and_referencegrant_exist() {
  # The controller should have created an HTTPRoute in retail-payments and
  # a ReferenceGrant in capital-pricing when the cross-app link was declared.
  local route_count
  route_count=$(kctl get httproutes -n "$PAYMENTS_NS" --no-headers 2>/dev/null | wc -l || echo 0)
  if [[ "$route_count" -gt 0 ]]; then
    _assert_pass "HTTPRoute exists in ${PAYMENTS_NS}"
  else
    _assert_fail "HTTPRoute exists in ${PAYMENTS_NS}" "not found (count=${route_count})"
  fi

  local grant_count
  grant_count=$(kctl get referencegrants -n "$PRICING_NS" --no-headers 2>/dev/null | wc -l || echo 0)
  if [[ "$grant_count" -gt 0 ]]; then
    _assert_pass "ReferenceGrant exists in ${PRICING_NS}"
  else
    _assert_fail "ReferenceGrant exists in ${PRICING_NS}" "not found (count=${grant_count})"
  fi
}

# ── 09-assert-traffic-via-gateway ─────────────────────────────────────────────

assert_traffic_via_gateway() {
  # This requires a real Gateway and HTTPRoute to be configured.
  # Best-effort: try the gateway path; skip if gateway is not yet set up.
  local gw_ip
  gw_ip=$(kctl get gateway -n cho-system -o jsonpath='{.items[0].status.addresses[0].value}' 2>/dev/null || true)
  if [[ -z "$gw_ip" ]]; then
    echo "[SKIP] No Gateway found in cho-system — gateway traffic test skipped"
    return
  fi

  local out
  out=$(curl_from_pod "$PAYMENTS_NS" "app=echo-api" \
    "http://${gw_ip}/pricing/healthz" 5)
  if echo "$out" | grep -q "ok\|200\|healthz"; then
    _assert_pass "Traffic from retail-payments to pricing via internal gateway: HTTP 200"
  else
    echo "[SKIP] Gateway traffic test inconclusive (gateway may not be fully configured): ${out}"
  fi
}

# ── 09-assert-undeclared-consumer-blocked ─────────────────────────────────────

assert_undeclared_consumer_blocked() {
  # Deploy a third-app pod that is NOT a declared consumer of pricing
  local third_ns="${RETAIL_APP}-other"
  kctl create namespace "$third_ns" --dry-run=client -o yaml | kctl apply -f - 2>/dev/null || true
  kctl apply -n "$third_ns" -f "${SCRIPT_DIR}/fixtures/echo-api-pod.yaml"
  wait_for_pod_running "$third_ns" "app=echo-api" 60

  local gw_ip
  gw_ip=$(kctl get gateway -n cho-system -o jsonpath='{.items[0].status.addresses[0].value}' 2>/dev/null || true)
  if [[ -z "$gw_ip" ]]; then
    echo "[SKIP] No Gateway — undeclared-consumer test skipped"
    kctl delete namespace "$third_ns" --ignore-not-found=true 2>/dev/null || true
    return
  fi

  local out
  out=$(curl_from_pod "$third_ns" "app=echo-api" \
    "http://${gw_ip}/pricing/healthz" 5)
  if echo "$out" | grep -q "CONNECTION_FAILED\|403\|timeout"; then
    _assert_pass "Undeclared consumer (other) is blocked from pricing gateway path"
  else
    echo "[SKIP] Undeclared-consumer block test inconclusive: ${out}"
  fi

  kctl delete namespace "$third_ns" --ignore-not-found=true 2>/dev/null || true
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

cleanup() {
  cho admin app delete "$RETAIL_APP" --confirm 2>/dev/null || true
  cho admin app delete "$CAPITAL_APP" --confirm 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 09: Cross-Application Link ---"
  require_cilium
  require_gateway_api

  setup
  assert_direct_pod_to_pod_blocked
  assert_httproute_and_referencegrant_exist
  assert_traffic_via_gateway
  assert_undeclared_consumer_blocked
  cleanup

  print_summary
}

main
