#!/usr/bin/env bash
# kubectl helpers for chorister scenario tests.
# Source this file at the top of every run.sh/assert.sh that needs kubectl:
#   source "$(dirname "$0")/../lib/kubectl.sh"

set -euo pipefail

# KUBECTL_CONTEXT is set by setup scripts via KUBECONFIG or context flag.
KUBECTL_CONTEXT="${KUBECTL_CONTEXT:-}"

# kctl — run kubectl with optional context override
kctl() {
  if [[ -n "$KUBECTL_CONTEXT" ]]; then
    kubectl --context "$KUBECTL_CONTEXT" "$@"
  else
    kubectl "$@"
  fi
}

# wait_for_condition <namespace> <resource> <name> <jsonpath> <expected_value> [timeout=120s]
# Polls until the jsonpath field on the resource equals the expected value.
wait_for_condition() {
  local namespace="$1"
  local resource="$2"
  local name="$3"
  local jsonpath="$4"
  local expected="$5"
  local timeout="${6:-120}"

  local elapsed=0
  local delay=2
  while true; do
    local actual
    actual="$(kctl get "$resource" "$name" -n "$namespace" -o jsonpath="${jsonpath}" 2>/dev/null || true)"
    if [[ "$actual" == "$expected" ]]; then
      return 0
    fi
    if [[ "$elapsed" -ge "$timeout" ]]; then
      echo "[TIMEOUT] wait_for_condition: ${namespace}/${resource}/${name} jsonpath=${jsonpath} expected=${expected} actual=${actual}" >&2
      return 1
    fi
    sleep "$delay"
    elapsed=$((elapsed + delay))
    delay=$((delay < 16 ? delay * 2 : 16))
  done
}

# wait_for_condition_cluster <resource> <name> <jsonpath> <expected_value> [timeout=120s]
# Same as wait_for_condition but for cluster-scoped resources.
wait_for_condition_cluster() {
  local resource="$1"
  local name="$2"
  local jsonpath="$3"
  local expected="$4"
  local timeout="${5:-120}"

  local elapsed=0
  local delay=2
  while true; do
    local actual
    actual="$(kctl get "$resource" "$name" -o jsonpath="${jsonpath}" 2>/dev/null || true)"
    if [[ "$actual" == "$expected" ]]; then
      return 0
    fi
    if [[ "$elapsed" -ge "$timeout" ]]; then
      echo "[TIMEOUT] wait_for_condition_cluster: ${resource}/${name} jsonpath=${jsonpath} expected=${expected} actual=${actual}" >&2
      return 1
    fi
    sleep "$delay"
    elapsed=$((elapsed + delay))
    delay=$((delay < 16 ? delay * 2 : 16))
  done
}

# wait_for_deployment_ready <namespace> <name> [timeout=120s]
wait_for_deployment_ready() {
  local namespace="$1"
  local name="$2"
  local timeout="${3:-120}"
  kctl rollout status deployment/"$name" -n "$namespace" --timeout="${timeout}s"
}

# wait_for_pod_running <namespace> <label_selector> [timeout=120s]
wait_for_pod_running() {
  local namespace="$1"
  local selector="$2"
  local timeout="${3:-120}"
  kctl wait pod -n "$namespace" -l "$selector" \
    --for=condition=Ready --timeout="${timeout}s"
}

# wait_for_namespace <name> [timeout=60s]
wait_for_namespace() {
  local name="$1"
  local timeout="${2:-60}"
  local elapsed=0
  local delay=2
  while true; do
    if kctl get namespace "$name" >/dev/null 2>&1; then
      return 0
    fi
    if [[ "$elapsed" -ge "$timeout" ]]; then
      echo "[TIMEOUT] wait_for_namespace: ${name} not found after ${timeout}s" >&2
      return 1
    fi
    sleep "$delay"
    elapsed=$((elapsed + delay))
    delay=$((delay < 16 ? delay * 2 : 16))
  done
}

# wait_for_namespace_gone <name> [timeout=60s]
wait_for_namespace_gone() {
  local name="$1"
  local timeout="${2:-60}"
  local elapsed=0
  local delay=2
  while true; do
    if ! kctl get namespace "$name" >/dev/null 2>&1; then
      return 0
    fi
    if [[ "$elapsed" -ge "$timeout" ]]; then
      echo "[TIMEOUT] wait_for_namespace_gone: ${name} still exists after ${timeout}s" >&2
      return 1
    fi
    sleep "$delay"
    elapsed=$((elapsed + delay))
    delay=$((delay < 16 ? delay * 2 : 16))
  done
}

# namespace_exists <name> → returns 0 if exists, 1 if not
namespace_exists() {
  local name="$1"
  kctl get namespace "$name" >/dev/null 2>&1
}

# resource_exists <namespace> <resource> <name>
resource_exists() {
  local namespace="$1"
  local resource="$2"
  local name="$3"
  kctl get "$resource" "$name" -n "$namespace" >/dev/null 2>&1
}

# resource_exists_cluster <resource> <name>
resource_exists_cluster() {
  local resource="$1"
  local name="$2"
  kctl get "$resource" "$name" >/dev/null 2>&1
}

# pod_exec <namespace> <pod_selector> <command...>
pod_exec() {
  local namespace="$1"
  local selector="$2"
  shift 2
  local pod
  pod="$(kctl get pod -n "$namespace" -l "$selector" -o jsonpath='{.items[0].metadata.name}')"
  kctl exec -n "$namespace" "$pod" -- "$@"
}

# apply_yaml <file>
apply_yaml() {
  local file="$1"
  kctl apply -f "$file"
}

# wait_for_crd <name> [timeout=60s]
wait_for_crd() {
  local name="$1"
  local timeout="${2:-60}"
  local elapsed=0
  local delay=2
  while true; do
    if kctl get crd "$name" >/dev/null 2>&1; then
      return 0
    fi
    if [[ "$elapsed" -ge "$timeout" ]]; then
      echo "[TIMEOUT] wait_for_crd: ${name} not registered after ${timeout}s" >&2
      return 1
    fi
    sleep "$delay"
    elapsed=$((elapsed + delay))
    delay=$((delay < 16 ? delay * 2 : 16))
  done
}

# count_resources <namespace> <resource> <label_selector>
count_resources() {
  local namespace="$1"
  local resource="$2"
  local selector="${3:-}"
  local args=("get" "$resource" "-n" "$namespace" "--no-headers")
  if [[ -n "$selector" ]]; then
    args+=("-l" "$selector")
  fi
  kctl "${args[@]}" 2>/dev/null | wc -l | tr -d ' '
}
