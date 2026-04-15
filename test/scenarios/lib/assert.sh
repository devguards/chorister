#!/usr/bin/env bash
# Shared assertion helpers for chorister scenario tests.
# Source this file at the top of every assert.sh:
#   source "$(dirname "$0")/../lib/assert.sh"

set -euo pipefail

PASS_COUNT=0
FAIL_COUNT=0

# Internal pass/fail tracking
_assert_pass() {
  local desc="$1"
  PASS_COUNT=$((PASS_COUNT + 1))
  echo "[PASS] ${desc}"
}

_assert_fail() {
  local desc="$1"
  local detail="${2:-}"
  FAIL_COUNT=$((FAIL_COUNT + 1))
  echo "[FAIL] ${desc}"
  if [[ -n "$detail" ]]; then
    echo "       Detail: ${detail}"
  fi
}

# assert_eq <expected> <actual> <description>
assert_eq() {
  local expected="$1"
  local actual="$2"
  local desc="$3"
  if [[ "$expected" == "$actual" ]]; then
    _assert_pass "$desc"
  else
    _assert_fail "$desc" "expected=${expected} actual=${actual}"
  fi
}

# assert_ne <unexpected> <actual> <description>
assert_ne() {
  local unexpected="$1"
  local actual="$2"
  local desc="$3"
  if [[ "$unexpected" != "$actual" ]]; then
    _assert_pass "$desc"
  else
    _assert_fail "$desc" "unexpected value appeared: ${actual}"
  fi
}

# assert_contains <haystack> <needle> <description>
assert_contains() {
  local haystack="$1"
  local needle="$2"
  local desc="$3"
  if echo "$haystack" | grep -qF "$needle"; then
    _assert_pass "$desc"
  else
    _assert_fail "$desc" "needle=${needle} not found in output"
  fi
}

# assert_not_contains <haystack> <needle> <description>
assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  local desc="$3"
  if ! echo "$haystack" | grep -qF "$needle"; then
    _assert_pass "$desc"
  else
    _assert_fail "$desc" "unexpected needle=${needle} found in output"
  fi
}

# assert_empty <value> <description>
assert_empty() {
  local value="$1"
  local desc="$2"
  if [[ -z "$value" ]]; then
    _assert_pass "$desc"
  else
    _assert_fail "$desc" "expected empty, got: ${value}"
  fi
}

# assert_not_empty <value> <description>
assert_not_empty() {
  local value="$1"
  local desc="$2"
  if [[ -n "$value" ]]; then
    _assert_pass "$desc"
  else
    _assert_fail "$desc" "expected non-empty value"
  fi
}

# assert_exit_ok <exit_code> <description>
assert_exit_ok() {
  local code="$1"
  local desc="$2"
  if [[ "$code" -eq 0 ]]; then
    _assert_pass "$desc"
  else
    _assert_fail "$desc" "exit code ${code} (expected 0)"
  fi
}

# assert_exit_fail <exit_code> <description>
assert_exit_fail() {
  local code="$1"
  local desc="$2"
  if [[ "$code" -ne 0 ]]; then
    _assert_pass "$desc"
  else
    _assert_fail "$desc" "expected non-zero exit code, got 0"
  fi
}

# assert_matches <regex> <value> <description>
assert_matches() {
  local regex="$1"
  local value="$2"
  local desc="$3"
  if echo "$value" | grep -qE "$regex"; then
    _assert_pass "$desc"
  else
    _assert_fail "$desc" "regex=${regex} did not match: ${value}"
  fi
}

# print_summary — call at end of assert.sh to print totals and exit with correct code
print_summary() {
  local total=$((PASS_COUNT + FAIL_COUNT))
  echo ""
  echo "Results: ${PASS_COUNT}/${total} passed"
  if [[ "$FAIL_COUNT" -gt 0 ]]; then
    return 1
  fi
  return 0
}
