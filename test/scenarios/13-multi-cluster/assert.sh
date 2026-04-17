#!/usr/bin/env bash
# Scenario 13: Multi-Cluster Sandbox & Production Routing
#
# Story: A platform admin registers a sandbox cluster and a production cluster.
# Developers' sandboxes should be created on the sandbox cluster.
# Promoted resources should land on the production cluster.
# The CLI always targets the home cluster, independent of kubectl context.
#
# Validates:
#   1. ChoCluster accepts spec.clusters with role and secretRef
#   2. Controller reports cluster connectivity in status
#   3. Sandbox namespace is created on the sandbox-role cluster
#   4. Production resources land on the production-role cluster after promotion
#   5. CLI commands use chorister config, not kubectl context
#   6. chorister status shows cluster assignments per environment

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB_DIR="${SCRIPT_DIR}/../lib"

source "${LIB_DIR}/assert.sh"
source "${LIB_DIR}/kubectl.sh"
source "${LIB_DIR}/chorister.sh"

APP_NAME="scen13-multicluster"
DOMAIN="payments"
SANDBOX_NAME="alice"
SANDBOX_NS="${APP_NAME}-${DOMAIN}-sandbox-${SANDBOX_NAME}"
PROD_NS="${APP_NAME}-${DOMAIN}"
CHOCLUSTER_NAME="scen13-cluster-config"
PR_NAME=""

# ── 13-setup ─────────────────────────────────────────────────────────────────

setup() {
  echo "[setup] Creating cho-system namespace if needed..."
  kctl create namespace cho-system --dry-run=client -o yaml | kctl apply -f - 2>/dev/null || true

  echo "[setup] Creating kubeconfig Secrets for registered clusters..."
  # In a real multi-cluster setup, these would contain actual kubeconfigs.
  # For now they're placeholder data — the controller should read them and
  # report connectivity status (Connected/Unreachable).
  kctl create secret generic sandbox-pool-kubeconfig \
    -n cho-system \
    --from-literal=kubeconfig="placeholder-sandbox-kubeconfig" \
    --dry-run=client -o yaml | kctl apply -f - 2>/dev/null

  kctl create secret generic prod-cell-1-kubeconfig \
    -n cho-system \
    --from-literal=kubeconfig="placeholder-prod-kubeconfig" \
    --dry-run=client -o yaml | kctl apply -f - 2>/dev/null

  echo "[setup] Applying ChoCluster with clusters field..."
  kctl apply -f "${SCRIPT_DIR}/fixtures/cho-cluster-multicluster.yaml"

  echo "[setup] Creating ChoApplication..."
  cho admin app create "${APP_NAME}" \
    --owners platform-admin@example.com \
    --compliance essential \
    --domains "${DOMAIN}"
  wait_for_namespace "$PROD_NS" 60
}

# ── 13-assert: ChoCluster CRD accepts clusters field ─────────────────────────

assert_chocluster_accepts_clusters() {
  echo ""
  echo "── 13.1 ChoCluster accepts spec.clusters ──"

  local clusters_json
  clusters_json="$(kctl get chocluster "$CHOCLUSTER_NAME" -o jsonpath='{.spec.clusters}' 2>&1)" || {
    _assert_fail "ChoCluster spec.clusters readable" "$clusters_json"
    return
  }

  if [[ -n "$clusters_json" && "$clusters_json" != "null" ]]; then
    _assert_pass "ChoCluster spec.clusters field accepted by CRD"
  else
    _assert_fail "ChoCluster spec.clusters field accepted by CRD" \
      "field is empty or null — CRD schema may not include clusters"
  fi

  # Verify two entries exist.
  local count
  count="$(kctl get chocluster "$CHOCLUSTER_NAME" -o jsonpath='{.spec.clusters[*].name}' 2>/dev/null \
    | wc -w | tr -d ' ')"
  assert_eq "2" "$count" "ChoCluster has 2 registered clusters"
}

# ── 13-assert: Controller reports cluster connectivity ────────────────────────

assert_controller_reports_connectivity() {
  echo ""
  echo "── 13.2 Controller reports cluster connectivity in status ──"

  # The controller should reconcile ChoCluster and populate
  # status.clusterConnectivity for each registered cluster.
  local elapsed=0
  local connectivity=""
  while [[ "$elapsed" -lt 60 ]]; do
    connectivity="$(kctl get chocluster "$CHOCLUSTER_NAME" \
      -o jsonpath='{.status.clusterConnectivity}' 2>/dev/null || true)"
    if [[ -n "$connectivity" && "$connectivity" != "{}" && "$connectivity" != "null" ]]; then
      break
    fi
    sleep 3; elapsed=$((elapsed + 3))
  done

  if [[ -n "$connectivity" && "$connectivity" != "{}" && "$connectivity" != "null" ]]; then
    _assert_pass "Controller populated status.clusterConnectivity"
  else
    _assert_fail "Controller populated status.clusterConnectivity" \
      "still empty after 60s — controller does not process clusters[] yet"
  fi

  # Check that sandbox-pool appears in connectivity map.
  local sandbox_status
  sandbox_status="$(kctl get chocluster "$CHOCLUSTER_NAME" \
    -o jsonpath='{.status.clusterConnectivity.sandbox-pool}' 2>/dev/null || true)"
  assert_not_empty "$sandbox_status" "sandbox-pool has a connectivity status"

  # Check that prod-cell-1 appears in connectivity map.
  local prod_status
  prod_status="$(kctl get chocluster "$CHOCLUSTER_NAME" \
    -o jsonpath='{.status.clusterConnectivity.prod-cell-1}' 2>/dev/null || true)"
  assert_not_empty "$prod_status" "prod-cell-1 has a connectivity status"
}

# ── 13-assert: Sandbox created on sandbox cluster ─────────────────────────────

assert_sandbox_on_sandbox_cluster() {
  echo ""
  echo "── 13.3 Sandbox namespace targets sandbox-role cluster ──"

  cho sandbox create --domain "$DOMAIN" --name "$SANDBOX_NAME" --app "$APP_NAME"

  # In multi-cluster mode the sandbox namespace should be created on the
  # SANDBOX cluster (sandbox-pool), NOT the home cluster.
  #
  # We verify this by checking that:
  #   a) The ChoSandbox status references the target cluster.
  #   b) The sandbox namespace does NOT exist on the home cluster.
  #
  # Until multi-cluster routing is implemented, this will fail because
  # everything runs on the home cluster.

  # Give the controller time to process.
  sleep 10

  # Check ChoSandbox status for target cluster info.
  local target_cluster
  target_cluster="$(kctl get chosandbox -n default -o jsonpath='{.items[0].status.cluster}' 2>/dev/null || true)"

  if [[ "$target_cluster" == "sandbox-pool" ]]; then
    _assert_pass "ChoSandbox status.cluster = sandbox-pool"
  else
    _assert_fail "ChoSandbox status.cluster = sandbox-pool" \
      "got '${target_cluster}' — controller does not route sandboxes to registered cluster yet"
  fi

  # Sandbox namespace should NOT exist on home cluster in multi-cluster mode.
  if namespace_exists "$SANDBOX_NS"; then
    _assert_fail "Sandbox namespace NOT on home cluster" \
      "${SANDBOX_NS} exists on home cluster — multi-cluster routing not active"
  else
    _assert_pass "Sandbox namespace NOT on home cluster (routed to sandbox-pool)"
  fi
}

# ── 13-assert: Promotion routes to production cluster ─────────────────────────

assert_promotion_to_production_cluster() {
  echo ""
  echo "── 13.4 Promoted resources target production-role cluster ──"

  # Apply a compute resource to the sandbox.
  cho apply --file "${SCRIPT_DIR}/fixtures/cho-compute-api.yaml" \
    --domain "$DOMAIN" --sandbox "$SANDBOX_NAME" --app "$APP_NAME"
  sleep 5

  # Promote.
  local output rc=0
  output="$(cho promote --domain "$DOMAIN" --sandbox "$SANDBOX_NAME" --app "$APP_NAME" 2>&1)" || rc=$?
  assert_exit_ok "$rc" "chorister promote exits 0"

  # Find the promotion request and approve it.
  sleep 5
  PR_NAME="$(kctl get chopromotionrequest -n default --no-headers 2>/dev/null \
    | grep "${APP_NAME}-${DOMAIN}" | awk '{print $1}' | head -1 || true)"

  if [[ -z "$PR_NAME" ]]; then
    _assert_fail "ChoPromotionRequest created" "none found"
    return
  fi
  _assert_pass "ChoPromotionRequest created"

  # Approve.
  output="$(cho approve "$PR_NAME" --role org-admin 2>&1)" || true

  # Wait for completion.
  wait_for_condition "default" "chopromotionrequest" "$PR_NAME" \
    '{.status.phase}' "Completed" 120 || true

  # In multi-cluster mode, the production Deployment should appear on the
  # PRODUCTION cluster (prod-cell-1), NOT the home cluster.
  sleep 5

  # Check ChoPromotionRequest for target cluster info.
  local target
  target="$(kctl get chopromotionrequest "$PR_NAME" -n default \
    -o jsonpath='{.status.targetCluster}' 2>/dev/null || true)"

  if [[ "$target" == "prod-cell-1" ]]; then
    _assert_pass "ChoPromotionRequest status.targetCluster = prod-cell-1"
  else
    _assert_fail "ChoPromotionRequest status.targetCluster = prod-cell-1" \
      "got '${target}' — controller does not route promotions to registered cluster yet"
  fi

  # Production Deployment should NOT exist on the home cluster.
  if resource_exists "$PROD_NS" "deployment" "echo-api"; then
    _assert_fail "Production Deployment NOT on home cluster" \
      "echo-api exists on home cluster — multi-cluster routing not active"
  else
    _assert_pass "Production Deployment NOT on home cluster (routed to prod-cell-1)"
  fi
}

# ── 13-assert: CLI decoupled from kubectl context ─────────────────────────────

assert_cli_decoupled_from_kubectl() {
  echo ""
  echo "── 13.5 CLI uses chorister config, not kubectl context ──"

  # The chorister CLI should support a --home-cluster flag or a config file
  # so it can always target the home cluster regardless of kubectl context.

  local output rc=0
  output="$(cho status --help 2>&1)" || rc=$?

  if echo "$output" | grep -q "home-cluster\|home_cluster\|server"; then
    _assert_pass "CLI supports home-cluster / server flag"
  else
    _assert_fail "CLI supports home-cluster / server flag" \
      "chorister status --help does not mention home-cluster or server flag"
  fi

  # chorister login --help should mention server/endpoint configuration.
  output="$(cho login --help 2>&1)" || rc=$?

  if echo "$output" | grep -q "server\|endpoint\|cluster\|url"; then
    _assert_pass "chorister login --help mentions server/endpoint/cluster config"
  else
    _assert_fail "chorister login --help mentions server/endpoint/cluster config" \
      "login help does not mention server configuration"
  fi
}

# ── 13-assert: chorister status shows cluster assignments ─────────────────────

assert_status_shows_clusters() {
  echo ""
  echo "── 13.6 chorister status shows cluster assignments ──"

  local output rc=0
  output="$(cho status --domain "$DOMAIN" --app "$APP_NAME" 2>&1)" || rc=$?

  # Status output should indicate which cluster each environment is assigned to.
  if echo "$output" | grep -qi "cluster\|sandbox-pool\|prod-cell-1"; then
    _assert_pass "chorister status output references clusters"
  else
    _assert_fail "chorister status output references clusters" \
      "no cluster info in status output — multi-cluster status not implemented"
  fi
}

# ── Cleanup ───────────────────────────────────────────────────────────────────

cleanup() {
  echo ""
  echo "[cleanup] Removing test resources..."

  kctl delete chosandbox -n default \
    -l "chorister.dev/application=${APP_NAME}" \
    --ignore-not-found=true 2>/dev/null || true
  kctl delete chopromotionrequest -n default \
    -l "chorister.dev/application=${APP_NAME}" \
    --ignore-not-found=true 2>/dev/null || true
  kctl delete choapplication "${APP_NAME}" \
    --ignore-not-found=true 2>/dev/null || true
  kctl delete chocluster "$CHOCLUSTER_NAME" \
    --ignore-not-found=true 2>/dev/null || true
  kctl delete secret sandbox-pool-kubeconfig prod-cell-1-kubeconfig \
    -n cho-system --ignore-not-found=true 2>/dev/null || true
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  echo "--- Scenario 13: Multi-Cluster Sandbox & Production Routing ---"

  setup
  assert_chocluster_accepts_clusters
  assert_controller_reports_connectivity
  assert_sandbox_on_sandbox_cluster
  assert_promotion_to_production_cluster
  assert_cli_decoupled_from_kubectl
  assert_status_shows_clusters
  cleanup

  print_summary
}

main
