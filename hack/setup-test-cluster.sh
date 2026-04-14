#!/usr/bin/env bash

set -euo pipefail

CLUSTER_NAME="${KIND_CLUSTER:-chorister-test}"
KIND_IMAGE="${KIND_IMAGE:-}"
CILIUM_VERSION="${CILIUM_VERSION:-1.17.6}"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.3.0}"

DRY_RUN=0
VALIDATE_ONLY=0

usage() {
	cat <<'EOF'
Usage: hack/setup-test-cluster.sh [options]

Creates or updates a multi-node Kind cluster for chorister e2e work, disables the
default CNI, installs Cilium with Hubble enabled, waits for Cilium readiness, and
installs the Gateway API CRDs.

Options:
  --cluster-name NAME   Override Kind cluster name (default: $KIND_CLUSTER or chorister-test)
  --dry-run             Print the commands without executing them
  --validate-prereqs    Check required tools and exit
  -h, --help            Show this help message

Environment overrides:
  KIND_CLUSTER          Kind cluster name
  KIND_IMAGE            Optional Kind node image
  CILIUM_VERSION        Cilium Helm chart version
  GATEWAY_API_VERSION   Gateway API release version
EOF
}

log() {
	echo "[$(date +%H:%M:%S)] $*"
}

run() {
	log "+ $*"
	if [[ "$DRY_RUN" -eq 0 ]]; then
		"$@"
	fi
}

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		echo "missing required command: $cmd" >&2
		exit 1
	fi
}

ensure_prereqs() {
	require_cmd kind
	require_cmd kubectl
	require_cmd helm
	require_cmd cilium
}

kind_context() {
	printf 'kind-%s' "$CLUSTER_NAME"
}

kubectl_ctx() {
	kubectl --context "$(kind_context)" "$@"
}

cilium_ctx() {
	cilium --context "$(kind_context)" "$@"
}

kind_cluster_exists() {
	kind get clusters | grep -Fxq "$CLUSTER_NAME"
}

create_kind_cluster() {
	local kind_config
	kind_config="$(mktemp)"
	trap 'rm -f "$kind_config"' RETURN

	cat >"$kind_config" <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  disableDefaultCNI: true
nodes:
  - role: control-plane
  - role: worker
  - role: worker
EOF

	if kind_cluster_exists; then
		log "Kind cluster '$CLUSTER_NAME' already exists"
		return
	fi

	local args=(create cluster --name "$CLUSTER_NAME" --config "$kind_config")
	if [[ -n "$KIND_IMAGE" ]]; then
		args+=(--image "$KIND_IMAGE")
	fi

	run kind "${args[@]}"
}

wait_for_nodes_ready() {
	run kubectl_ctx wait --for=condition=Ready nodes --all --timeout=5m
}

install_cilium() {
	run helm repo add cilium https://helm.cilium.io
	run helm repo update
	run helm upgrade --install cilium cilium/cilium \
		--kube-context "$(kind_context)" \
		--namespace kube-system \
		--create-namespace \
		--version "$CILIUM_VERSION" \
		--set hubble.enabled=true \
		--set hubble.relay.enabled=true \
		--set operator.replicas=1 \
		--set ipam.mode=kubernetes \
		--set kubeProxyReplacement=false

	run kubectl_ctx -n kube-system rollout status daemonset/cilium --timeout=10m
	run kubectl_ctx -n kube-system rollout status deployment/cilium-operator --timeout=10m
	run cilium_ctx status --wait
}

install_gateway_api_crds() {
	local gateway_api_url
	gateway_api_url="https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml"
	run kubectl_ctx apply -f "$gateway_api_url"
}

print_summary() {
	run cilium_ctx status
	run kubectl_ctx get nodes -o wide
}

main() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
			--cluster-name)
				CLUSTER_NAME="$2"
				shift 2
				;;
			--dry-run)
				DRY_RUN=1
				shift
				;;
			--validate-prereqs)
				VALIDATE_ONLY=1
				shift
				;;
			-h|--help)
				usage
				exit 0
				;;
			*)
				echo "unknown argument: $1" >&2
				usage >&2
				exit 1
				;;
		esac
	done

	ensure_prereqs

	if [[ "$VALIDATE_ONLY" -eq 1 ]]; then
		log "All prerequisites are available"
		exit 0
	fi

	create_kind_cluster
	wait_for_nodes_ready
	install_cilium
	install_gateway_api_crds
	print_summary
}

main "$@"