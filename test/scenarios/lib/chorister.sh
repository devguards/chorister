#!/usr/bin/env bash
# Thin wrappers for chorister CLI calls with logging.
# Source this file at the top of every run.sh/assert.sh that calls the CLI:
#   source "$(dirname "$0")/../lib/chorister.sh"

set -euo pipefail

# Path to the chorister binary — default assumes running from project root.
CHORISTER="${CHORISTER:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)/bin/chorister}"

# cho — run chorister with optional KUBECONFIG propagation
cho() {
  echo "[CLI] chorister $*" >&2
  "$CHORISTER" "$@"
}

# cho_ok <description> <args...>
# Runs chorister, asserts exit 0, prints output.
cho_ok() {
  local desc="$1"
  shift
  local output
  local rc=0
  output="$("$CHORISTER" "$@" 2>&1)" || rc=$?
  if [[ "$rc" -eq 0 ]]; then
    echo "[PASS] ${desc}"
  else
    echo "[FAIL] ${desc} (exit ${rc})"
    echo "       Output: ${output}"
  fi
  echo "$output"
}

# cho_fail <description> <args...>
# Runs chorister, asserts exit non-zero.
cho_fail() {
  local desc="$1"
  shift
  local output
  local rc=0
  output="$("$CHORISTER" "$@" 2>&1)" || rc=$?
  if [[ "$rc" -ne 0 ]]; then
    echo "[PASS] ${desc}"
  else
    echo "[FAIL] ${desc} (expected failure, got exit 0)"
    echo "       Output: ${output}"
  fi
  echo "$output"
}

# cho_output <args...>
# Runs chorister and returns output. Fails if exit code is non-zero.
cho_output() {
  "$CHORISTER" "$@" 2>&1
}

# cho_sandbox_create <domain> <name> [--app <app>]
cho_sandbox_create() {
  local domain="$1"
  local name="$2"
  shift 2
  cho sandbox create --domain "$domain" --name "$name" "$@"
}

# cho_sandbox_destroy <domain> <name> [--app <app>]
cho_sandbox_destroy() {
  local domain="$1"
  local name="$2"
  shift 2
  cho sandbox destroy --domain "$domain" --name "$name" "$@"
}

# cho_sandbox_list [args...]
cho_sandbox_list() {
  cho sandbox list "$@"
}

# cho_promote <domain> <sandbox> [--app <app>]
cho_promote() {
  local domain="$1"
  local sandbox="$2"
  shift 2
  cho promote --domain "$domain" --sandbox "$sandbox" "$@"
}

# cho_approve <id>
cho_approve() {
  cho approve "$1"
}

# cho_requests [args...]
cho_requests() {
  cho requests "$@"
}

# cho_status [args...]
cho_status() {
  cho status "$@"
}

# cho_version
cho_version() {
  cho version
}
