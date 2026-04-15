#!/usr/bin/env bash
# Scenario 05: Internet Ingress with JWT Auth — assert.sh
# Requires: Cilium, Gateway API CRDs
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"
source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

APP_NAME="scen05-myapp"
PAYMENTS_NS="${APP_NAME}-payments"
CHO_SYSTEM="cho-system"

# ── Helpers ───────────────────────────────────────────────────────────────────

require_cilium() {
  if ! kctl get daemonset -n kube-system cilium &>/dev/null; then
    echo "[SKIP] Scenario 05 requires Cilium. Skipping."
    exit 0
  fi
}

require_gateway_api() {
  if ! kctl get crd gateways.gateway.networking.k8s.io &>/dev/null; then
    echo "[SKIP] Scenario 05 requires Gateway API CRDs. Skipping."
    exit 0
  fi
}

# Get the gateway address from the namespace
get_gateway_addr() {
  kctl get gateway -n "$PAYMENTS_NS" -o jsonpath='{.items[0].status.addresses[0].value}' 2>/dev/null || echo ""
}

# ── Setup ─────────────────────────────────────────────────────────────────────

setup() {
  # Deploy mock JWKS server in cho-system
  kctl apply -f "${SCRIPT_DIR}/fixtures/mock-jwks.yaml"
  wait_for_deployment_ready "$CHO_SYSTEM" "mock-jwks" 60

  # STUB: use kubectl to apply app
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-application.yaml"
  wait_for_namespace "$PAYMENTS_NS" 60

  # Apply ChoNetwork with JWT auth
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-network.yaml" -n "$PAYMENTS_NS"

  # Wait for HTTPRoute and Gateway to be created
  local elapsed=0
  while [[ "$elapsed" -lt 60 ]]; do
    if kctl get httproute -n "$PAYMENTS_NS" &>/dev/null && \
       [[ "$(kctl get httproute -n "$PAYMENTS_NS" --no-headers 2>/dev/null | wc -l)" -gt 0 ]]; then
      break
    fi
    sleep 3; elapsed=$((elapsed + 3))
  done

  # Deploy echo-api
  kctl apply -f "${SCRIPT_DIR}/fixtures/echo-api-compute.yaml" -n "$PAYMENTS_NS"
  wait_for_deployment_ready "$PAYMENTS_NS" "echo-api" 90
}

# ── 05-assert-no-auth-rejected ────────────────────────────────────────────────

assert_no_auth_rejected() {
  # Applying a ChoNetwork with internet ingress but NO auth should be rejected by webhook or controller
  local rc=0
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-network-no-auth.yaml" -n "$PAYMENTS_NS" 2>/dev/null || rc=$?
  if [[ "$rc" -ne 0 ]]; then
    _assert_pass "ChoNetwork with internet ingress and no auth is rejected"
  else
    # Check if the controller rejected it in status
    local phase
    phase="$(kctl get chonetwork -n "$PAYMENTS_NS" no-auth-network -o jsonpath='{.status.conditions[?(@.type=="Invalid")].status}' 2>/dev/null || echo "")"
    if [[ "$phase" == "True" ]]; then
      _assert_pass "ChoNetwork with internet ingress and no auth is rejected (controller)"
    else
      _assert_fail "ChoNetwork with internet ingress and no auth should be rejected" \
        "was accepted without error"
    fi
  fi
}

# ── 05-assert-healthz-anonymous ───────────────────────────────────────────────

assert_healthz_anonymous() {
  local gw_addr
  gw_addr="$(get_gateway_addr)"
  if [[ -z "$gw_addr" ]]; then
    _assert_fail "Gateway address available" "no gateway address found"
    return
  fi

  local rc=0
  local out
  out="$(curl -sf --max-time 5 "http://${gw_addr}/healthz" 2>&1)" || rc=$?
  if [[ "$rc" -eq 0 ]] || echo "$out" | grep -q "200\|ok"; then
    _assert_pass "/healthz is accessible anonymously"
  else
    _assert_fail "/healthz is accessible anonymously" "got: ${out}"
  fi
}

# ── 05-assert-api-requires-jwt ────────────────────────────────────────────────

assert_api_requires_jwt() {
  local gw_addr
  gw_addr="$(get_gateway_addr)"
  if [[ -z "$gw_addr" ]]; then
    _assert_fail "Gateway address for JWT test" "no gateway address"
    return
  fi

  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://${gw_addr}/api/users" 2>/dev/null || echo "000")"
  if [[ "$code" == "401" || "$code" == "403" ]]; then
    _assert_pass "/api/users requires JWT — returned HTTP ${code}"
  else
    _assert_fail "/api/users should require JWT (expect 401/403)" "got HTTP ${code}"
  fi
}

# ── 05-assert-api-with-valid-jwt ──────────────────────────────────────────────

assert_api_with_valid_jwt() {
  local gw_addr
  gw_addr="$(get_gateway_addr)"
  if [[ -z "$gw_addr" ]]; then
    _assert_fail "Gateway address for valid JWT test" "no gateway"
    return
  fi

  # Get a token from the mock JWKS server
  local token
  token="$(kctl exec -n "$CHO_SYSTEM" -l app=mock-jwks -- \
    wget -qO- http://localhost:8080/token 2>/dev/null || echo "")"

  if [[ -z "$token" ]]; then
    _assert_fail "Valid JWT from mock JWKS server" "could not obtain token"
    return
  fi

  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
    -H "Authorization: Bearer ${token}" \
    "http://${gw_addr}/api/users" 2>/dev/null || echo "000")"
  if [[ "$code" == "200" ]]; then
    _assert_pass "/api/users with valid JWT returns 200"
  else
    _assert_fail "/api/users with valid JWT should return 200" "got HTTP ${code}"
  fi
}

# ── 05-assert-api-with-invalid-jwt ────────────────────────────────────────────

assert_api_with_invalid_jwt() {
  local gw_addr
  gw_addr="$(get_gateway_addr)"
  if [[ -z "$gw_addr" ]]; then
    _assert_fail "Gateway address for invalid JWT test" "no gateway"
    return
  fi

  local code
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 \
    -H "Authorization: Bearer tampered.token.value" \
    "http://${gw_addr}/api/users" 2>/dev/null || echo "000")"
  if [[ "$code" == "401" || "$code" == "403" ]]; then
    _assert_pass "/api/users with tampered JWT is rejected (HTTP ${code})"
  else
    _assert_fail "/api/users with tampered JWT should be rejected" "got HTTP ${code}"
  fi
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

cleanup() {
  kctl delete choapplication "${APP_NAME}" --ignore-not-found=true 2>/dev/null || true
  kctl delete -f "${SCRIPT_DIR}/fixtures/mock-jwks.yaml" --ignore-not-found=true 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 05: Internet Ingress with JWT Auth ---"
  require_cilium
  require_gateway_api
  setup
  assert_no_auth_rejected
  assert_healthz_anonymous
  assert_api_requires_jwt
  assert_api_with_valid_jwt
  assert_api_with_invalid_jwt
  cleanup
  print_summary
}
main
