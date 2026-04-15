#!/usr/bin/env bash
# Setup a Kind cluster for scenario tests.
# Wraps hack/setup-test-cluster.sh and adds scenario-specific options.
#
# Usage:
#   test/scenarios/setup-scenario-cluster.sh [options]
#
# Options:
#   --cluster-name NAME      Kind cluster name (default: chorister-scenario)
#   --with-stackgres         Install StackGres operator (for real DB scenarios)
#   --with-nats              Install NATS operator
#   --with-tetragon          Install Tetragon (for security scenarios)
#   --plain-kind             Skip Cilium — use plain Kind (faster, no network tests)
#   --skip-apps              Skip building and loading stub apps
#   --skip-controller        Skip deploying chorister controller
#   --dry-run                Print what would happen without doing it
#   -h, --help               Show this message

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

CLUSTER_NAME="${SCENARIO_CLUSTER:-chorister-scenario}"
WITH_STACKGRES=0
WITH_NATS=0
WITH_TETRAGON=0
PLAIN_KIND=0
SKIP_APPS=0
SKIP_CONTROLLER=0
DRY_RUN=0

usage() {
  cat <<'EOF'
Usage: test/scenarios/setup-scenario-cluster.sh [options]

Creates or updates a Kind cluster for scenario tests. Installs chorister CRDs
and controller, then optionally loads stub apps and extra operators.

Options:
  --cluster-name NAME      Kind cluster name (default: chorister-scenario)
  --with-stackgres         Install StackGres operator
  --with-nats              Install NATS operator
  --with-tetragon          Install Tetragon
  --plain-kind             Use plain Kind (no Cilium)
  --skip-apps              Skip building stub apps
  --skip-controller        Skip deploying chorister controller
  --dry-run                Dry run
  -h, --help               Show this message
EOF
}

log() { echo "[$(date +%H:%M:%S)] $*"; }
run() {
  log "+ $*"
  if [[ "$DRY_RUN" -eq 0 ]]; then
    "$@"
  fi
}

kind_context() { printf 'kind-%s' "$CLUSTER_NAME"; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cluster-name) CLUSTER_NAME="$2"; shift 2 ;;
    --with-stackgres) WITH_STACKGRES=1; shift ;;
    --with-nats) WITH_NATS=1; shift ;;
    --with-tetragon) WITH_TETRAGON=1; shift ;;
    --plain-kind) PLAIN_KIND=1; shift ;;
    --skip-apps) SKIP_APPS=1; shift ;;
    --skip-controller) SKIP_CONTROLLER=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
  esac
done

# ── Cluster creation ──────────────────────────────────────────────────────────

create_cluster() {
  if kind get clusters 2>/dev/null | grep -Fxq "$CLUSTER_NAME"; then
    log "Kind cluster '${CLUSTER_NAME}' already exists — skipping creation"
    return 0
  fi

  if [[ "$PLAIN_KIND" -eq 1 ]]; then
    log "Creating plain Kind cluster '${CLUSTER_NAME}'"
    run kind create cluster --name "$CLUSTER_NAME"
  else
    log "Creating Cilium-backed Kind cluster '${CLUSTER_NAME}'"
    run bash "${PROJECT_ROOT}/hack/setup-test-cluster.sh" \
      --cluster-name "$CLUSTER_NAME"
  fi
}

# ── Controller + CRDs ─────────────────────────────────────────────────────────

deploy_controller() {
  if [[ "$SKIP_CONTROLLER" -eq 1 ]]; then
    log "Skipping controller deploy (--skip-controller)"
    return 0
  fi

  log "Building chorister binaries"
  run make -C "$PROJECT_ROOT" build

  local IMG="${CONTROLLER_IMG:-controller:latest}"

  log "Building controller image: ${IMG}"
  run make -C "$PROJECT_ROOT" docker-build IMG="$IMG"

  log "Loading controller image into Kind cluster ${CLUSTER_NAME}"
  run kind load docker-image "$IMG" --name "$CLUSTER_NAME"

  log "Installing CRDs into cluster"
  run kubectl --context "$(kind_context)" apply \
    -k "${PROJECT_ROOT}/config/crd"

  log "Deploying chorister controller manager"
  run kubectl --context "$(kind_context)" apply \
    -k "${PROJECT_ROOT}/config/default" || {
    # config/default may need an image — fall back to just CRDs and a simple manager pod
    log "Warning: config/default deploy failed — CRDs are still installed"
  }

  # Wait for CRDs to be established
  local crds=(
    choapplications.chorister.dev
    chosandboxes.chorister.dev
    chocomputes.chorister.dev
    chodatabases.chorister.dev
    choqueues.chorister.dev
    chocaches.chorister.dev
    chonetworks.chorister.dev
    chostorages.chorister.dev
    chodomainmemberships.chorister.dev
    chopromotionrequests.chorister.dev
    chovulnerabilityreports.chorister.dev
    choclusters.chorister.dev
  )
  for crd in "${crds[@]}"; do
    run kubectl --context "$(kind_context)" wait crd/"${crd}" \
      --for=condition=Established --timeout=60s || {
      log "Warning: CRD ${crd} not ready (may not exist yet)"
    }
  done
}

# ── Stub apps ────────────────────────────────────────────────────────────────

build_and_load_apps() {
  if [[ "$SKIP_APPS" -eq 1 ]]; then
    log "Skipping stub app build (--skip-apps)"
    return 0
  fi

  local apps_dir="${SCRIPT_DIR}/apps"
  for app_dir in "${apps_dir}"/*/; do
    local app_name
    app_name="$(basename "$app_dir")"
    local image="chorister-scenario/${app_name}:latest"
    log "Building stub app: ${app_name} → ${image}"
    run docker build \
      --platform linux/arm64,linux/amd64 \
      --tag "$image" \
      "$app_dir" || {
      # Try single-platform if multi-arch fails
      run docker build --tag "$image" "$app_dir"
    }
    log "Loading ${image} into Kind cluster ${CLUSTER_NAME}"
    run kind load docker-image "$image" --name "$CLUSTER_NAME"
  done
}

# ── Optional operators ────────────────────────────────────────────────────────

install_nats() {
  if [[ "$WITH_NATS" -eq 1 ]]; then
    log "Installing NATS operator"
    run helm repo add nats https://nats-io.github.io/k8s/helm/charts/ --force-update
    run helm upgrade --install nats nats/nats \
      --kube-context "$(kind_context)" \
      --namespace nats-system --create-namespace \
      --set config.jetstream.enabled=true \
      --wait --timeout 5m
  fi
}

install_tetragon() {
  if [[ "$WITH_TETRAGON" -eq 1 ]]; then
    log "Installing Tetragon"
    run helm repo add cilium https://helm.cilium.io --force-update
    run helm upgrade --install tetragon cilium/tetragon \
      --kube-context "$(kind_context)" \
      --namespace kube-system \
      --wait --timeout 5m
  fi
}

# ── Main ──────────────────────────────────────────────────────────────────────

main() {
  create_cluster
  deploy_controller
  build_and_load_apps
  install_nats
  install_tetragon
  log "Cluster '${CLUSTER_NAME}' is ready for scenario tests"
  log "  export KUBECONFIG=\$(kind get kubeconfig --name ${CLUSTER_NAME})"
  log "  or use: kubectl --context kind-${CLUSTER_NAME}"
}

main
