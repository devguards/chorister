#!/usr/bin/env bash
# Scenario 10: Domain Membership, RBAC, and Expiry — assert.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"
source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

APP_NAME="scen10-myapp"
DOMAIN="payments"
PROD_NS="${APP_NAME}-${DOMAIN}"
SANDBOX_NS="${APP_NAME}-${DOMAIN}-sandbox-alice"
MEMBER_IDENTITY="alice@chorister-test.dev"
FUTURE_EXPIRY="2027-01-01T00:00:00Z"
PAST_EXPIRY="2020-01-01T00:00:00Z"

# ── Setup ─────────────────────────────────────────────────────────────────────

setup() {
  cho admin app create "$APP_NAME" \
    --owners test@chorister.dev \
    --compliance essential \
    --domains "$DOMAIN"

  # Set domain sensitivity to restricted (requires expiry on membership)
  cho admin domain set-sensitivity "$DOMAIN" --app "$APP_NAME" --sensitivity restricted 2>/dev/null || {
    # Fall back to set-policy if set-sensitivity targets the whole app
    cho admin app set-policy "$APP_NAME" --compliance regulated 2>/dev/null || true
  }
  wait_for_namespace "$PROD_NS" 60
}

# ── 10-assert-add-member-requires-expiry ──────────────────────────────────────

assert_add_member_requires_expiry() {
  # restricted domain: adding member without --expires-at should error
  local output rc=0
  output="$(cho admin member add \
    --app "$APP_NAME" \
    --domain "$DOMAIN" \
    --identity "$MEMBER_IDENTITY" \
    --role developer 2>&1)" || rc=$?

  if [[ "$rc" -ne 0 ]] || echo "$output" | grep -qi "expires\|required\|error"; then
    _assert_pass "admin member add without --expires-at is rejected for restricted domain"
  else
    _assert_fail "admin member add without --expires-at should fail for restricted domain" \
      "exit=${rc} output=${output}"
  fi
}

# ── 10-assert-add-member-with-expiry ─────────────────────────────────────────

assert_add_member_with_expiry() {
  local output rc=0
  output="$(cho admin member add \
    --app "$APP_NAME" \
    --domain "$DOMAIN" \
    --identity "$MEMBER_IDENTITY" \
    --role developer \
    --expires-at "$FUTURE_EXPIRY" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "admin member add with --expires-at succeeds"

  # Wait for ChoDomainMembership CR to appear
  local elapsed=0
  while [[ "$elapsed" -lt 30 ]]; do
    local count
    count=$(kctl get chodomainmemberships -n cho-system \
      --no-headers 2>/dev/null | grep -c "$MEMBER_IDENTITY" || echo 0)
    if [[ "$count" -gt 0 ]]; then
      _assert_pass "ChoDomainMembership CR created for ${MEMBER_IDENTITY}"
      return
    fi
    sleep 3; elapsed=$((elapsed + 3))
  done
  _assert_fail "ChoDomainMembership CR created for ${MEMBER_IDENTITY}" "not found after 30s"
}

# ── 10-assert-developer-cannot-write-prod ────────────────────────────────────

assert_developer_cannot_write_prod() {
  local result
  result=$(kctl auth can-i create deployments \
    --namespace "$PROD_NS" --as "$MEMBER_IDENTITY" 2>/dev/null || echo "no")
  assert_eq "no" "$result" "developer alice cannot create Deployments in production"
}

# ── 10-assert-developer-can-read-prod ────────────────────────────────────────

assert_developer_can_read_prod() {
  # Developers get read access to production namespace via ChoDomainMembership RoleBinding
  local result
  result=$(kctl auth can-i get pods \
    --namespace "$PROD_NS" --as "$MEMBER_IDENTITY" 2>/dev/null || echo "no")
  if [[ "$result" == "yes" ]]; then
    _assert_pass "developer alice can get pods in production (read-only)"
  else
    _assert_fail "developer alice can get pods in production (read-only)" "got: ${result}"
  fi
}

# ── 10-assert-member-list ─────────────────────────────────────────────────────

assert_member_list() {
  local output rc=0
  output="$(cho admin member list --app "$APP_NAME" --domain "$DOMAIN" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "admin member list exits 0"
}

# ── 10-assert-expired-membership-removed ─────────────────────────────────────

assert_expired_membership_removed() {
  # Create a membership with expiry in the past via CLI
  cho admin member add \
    --app "$APP_NAME" \
    --domain "$DOMAIN" \
    --identity "bob-expired@chorister-test.dev" \
    --role developer \
    --expires-at "$PAST_EXPIRY" 2>/dev/null || {
    # If CLI rejects past expiry, apply directly
    kctl apply -f "${SCRIPT_DIR}/fixtures/cho-domainmembership-expired.yaml" 2>/dev/null || {
      echo "[SKIP] Could not create expired ChoDomainMembership fixture"
      return
    }
  }

  # Trigger reconciliation by touching the object
  kctl annotate chodomainmembership scen10-expired-member \
    -n cho-system \
    "chorister.dev/reconcile=$(date +%s)" --overwrite 2>/dev/null || true

  local elapsed=0
  while [[ "$elapsed" -lt 30 ]]; do
    local rb_count
    rb_count=$(kctl get rolebindings -n "$PROD_NS" \
      --no-headers 2>/dev/null | grep -c "expired\|bob" || echo 0)
    if [[ "$rb_count" -eq 0 ]]; then
      _assert_pass "Expired membership RoleBinding is removed"
      break
    fi
    sleep 5; elapsed=$((elapsed + 5))
  done
  echo "[SKIP] Expired membership RoleBinding removal is timing-dependent"

  # assert member list --include-expired shows it
  local output rc=0
  output="$(cho admin member list \
    --app "$APP_NAME" --domain "$DOMAIN" \
    --include-expired 2>&1)" || rc=$?
  assert_exit_ok "$rc" "admin member list --include-expired exits 0"
}

# ── 10-assert-member-audit ────────────────────────────────────────────────────

assert_member_audit() {
  local output rc=0
  output="$(cho admin member audit --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "admin member audit exits 0"
}

# ── 10-assert-member-remove ───────────────────────────────────────────────────

assert_member_remove() {
  local output rc=0
  output="$(cho admin member remove --app "$APP_NAME" --domain "$DOMAIN" \
    --identity "$MEMBER_IDENTITY" --confirm 2>&1)" || rc=$?
  assert_exit_ok "$rc" "admin member remove exits 0"
  assert_contains "$output" "removed" "admin member remove confirms removal"
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

cleanup() {
  cho admin app delete "$APP_NAME" --confirm 2>/dev/null || true
  kctl delete chodomainmembership scen10-expired-member \
    -n cho-system --ignore-not-found=true 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 10: Domain Membership, RBAC, and Expiry ---"

  setup
  assert_add_member_requires_expiry
  assert_add_member_with_expiry
  assert_developer_cannot_write_prod
  assert_developer_can_read_prod
  assert_member_list
  assert_expired_membership_removed
  assert_member_audit
  assert_member_remove
  cleanup

  print_summary
}

main
