#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  hack/copilot-batch-edit.sh --prompt "..." [options] --target path1 --target path2
  hack/copilot-batch-edit.sh --prompt "..." [options] -- path1 path2

Options:
  -p, --prompt TEXT          Prompt applied to each target iteration
  -t, --test-cmd CMD         Test command to run after each edit (default: make test)
  --fix-attempts N       Retry Copilot after failed verification up to N times (default: 100)
  -m, --model MODEL          Copilot model name, for example gpt-5.3-codex
      --agent NAME           Optional custom Copilot agent name
      --auto-approve         Run Copilot with --allow-all
      --commit-prefix TEXT   Commit prefix (default: copilot)
      --target PATH        File or directory to use as the starting point; may be repeated
  -h, --help                 Show this help text

Examples:
  hack/copilot-batch-edit.sh \
    --prompt "Refactor for clarity and add missing tests" \
    --model gpt-5.3-codex \
    --auto-approve \
    --test-cmd "go test ./..." \
    --target cmd/chorister \
    --target internal/diff

  hack/copilot-batch-edit.sh \
    --prompt "Tighten validation and update affected tests" \
    --commit-prefix batch-edit \
    -- api/v1alpha1 internal/validation
EOF
}

fail() {
  echo "error: $*" >&2
  exit 1
}

has_repo_changes() {
  ! git diff --quiet || return 0
  ! git diff --cached --quiet || return 0
  [[ -n "$(git ls-files --others --exclude-standard)" ]]
}

require_clean_tree() {
  git diff --quiet || return 1
  git diff --cached --quiet || return 1
  [[ -z "$(git ls-files --others --exclude-standard)" ]] || return 1
}

to_abs_path() {
  local input="$1"

  if [[ -d "$input" ]]; then
    (
      cd "$input"
      pwd -P
    )
    return 0
  fi

  if [[ -f "$input" ]]; then
    local dir base
    dir=$(dirname "$input")
    base=$(basename "$input")
    (
      cd "$dir"
      printf '%s/%s\n' "$(pwd -P)" "$base"
    )
    return 0
  fi

  return 1
}

to_repo_rel() {
  local abs_path="$1"

  if [[ "$abs_path" == "$REPO_ROOT" ]]; then
    printf '.\n'
    return 0
  fi

  case "$abs_path" in
    "$REPO_ROOT"/*)
      printf '%s\n' "${abs_path#$REPO_ROOT/}"
      ;;
    *)
      return 1
      ;;
  esac
}

PROMPT=""
TEST_CMD="make test"
FIX_ATTEMPTS=100
MODEL=""
AGENT=""
AUTO_APPROVE=0
COMMIT_PREFIX="copilot"
TARGETS=()
SUCCEEDED_TARGETS=()
NO_CHANGE_TARGETS=()
FAILED_TARGETS=()
STASHED_TARGETS=()
CURRENT_CHILD_PID=""
CURRENT_CHILD_LABEL=""
INTERRUPTED=0
REPO_BIN=""
LAST_VERIFICATION_OUTPUT=""

forward_signal() {
  local signal="$1"

  INTERRUPTED=1

  if [[ -n "$CURRENT_CHILD_PID" ]]; then
    printf '\nForwarding SIG%s to %s (pid %s)\n' "$signal" "$CURRENT_CHILD_LABEL" "$CURRENT_CHILD_PID" >&2
    kill -s "$signal" "$CURRENT_CHILD_PID" 2>/dev/null || true
  fi
}

handle_interrupt() {
  forward_signal INT
}

handle_termination() {
  forward_signal TERM
}

run_tracked_command() {
  local label="$1"
  shift

  "$@" &
  CURRENT_CHILD_PID=$!
  CURRENT_CHILD_LABEL="$label"

  wait "$CURRENT_CHILD_PID"
  local status=$?

  CURRENT_CHILD_PID=""
  CURRENT_CHILD_LABEL=""

  if [[ "$INTERRUPTED" -eq 1 ]]; then
    exit 130
  fi

  return "$status"
}

run_tracked_shell_command() {
  local label="$1"
  local command_text="$2"

  run_tracked_command "$label" bash -lc "$command_text"
}

run_copilot_prompt() {
  local prompt_text="$1"
  local -a copilot_cmd

  copilot_cmd=(copilot -p "$prompt_text" -s --no-ask-user)

  if [[ -n "$MODEL" ]]; then
    copilot_cmd+=(--model "$MODEL")
  fi

  if [[ -n "$AGENT" ]]; then
    copilot_cmd+=(--agent "$AGENT")
  fi

  if [[ "$AUTO_APPROVE" -eq 1 ]]; then
    copilot_cmd+=(--allow-all)
  else
    copilot_cmd+=(--allow-tool=read,write,shell --allow-all-paths)
  fi

  run_tracked_command "copilot" "${copilot_cmd[@]}"
}

run_logged_shell_command() {
  local label="$1"
  local command_text="$2"
  local output_path="$3"

  run_tracked_command "$label" bash -lc "set -o pipefail; { $command_text; } 2>&1 | tee '$output_path'"
}

test_output_requires_chorister_build() {
  local output_path="$1"

  [[ -f "$output_path" ]] || return 1

  grep -Fq 'exec: "chorister": executable file not found in $PATH' "$output_path" || \
    grep -Fq 'exec: "chorister": executable file not found' "$output_path"
}

ensure_chorister_cli() {
  if [[ -x "$REPO_BIN/chorister" ]]; then
    return 0
  fi

  printf 'Building local chorister CLI for verification recovery\n'
  run_tracked_shell_command "build chorister CLI" "go build -o '$REPO_BIN/chorister' ./cmd/chorister"
}

run_final_verification() {
  local target_rel="$1"
  local output_path
  output_path=$(mktemp)
  LAST_VERIFICATION_OUTPUT=""

  if run_logged_shell_command "final verification" "$TEST_CMD" "$output_path"; then
    rm -f "$output_path"
    return 0
  fi

  local test_status=$?

  if test_output_requires_chorister_build "$output_path"; then
    printf 'Final verification for %s needs the local chorister binary; building it and retrying once.\n' "$target_rel"
    if ensure_chorister_cli && run_logged_shell_command "final verification retry" "$TEST_CMD" "$output_path"; then
      rm -f "$output_path"
      return 0
    fi
    test_status=$?
  fi

  LAST_VERIFICATION_OUTPUT=$(tail -n 200 "$output_path")

  rm -f "$output_path"
  return "$test_status"
}

build_recovery_prompt() {
  local scope_label="$1"
  local target_rel="$2"
  local attempt_number="$3"

  cat <<EOF
Start by reading this $scope_label again: $target_rel

Task:
$PROMPT

Recovery context:
- The previous edit attempt did not pass final verification.
- Investigate the verification output below, reason about the most likely root cause, and fix the underlying problem instead of papering over the symptom.
- After reasoning through the failure, inspect any other files you need, apply a fix, and rerun the relevant tests until the failure is resolved.
- This is recovery attempt $attempt_number of $FIX_ATTEMPTS for this target.
- Suggested verification command (hint only, adjust as needed): $TEST_CMD

Failed verification output:
$LAST_VERIFICATION_OUTPUT
EOF
}

trap handle_interrupt INT
trap handle_termination TERM

stash_failed_target() {
  local target_rel="$1"
  local reason="$2"
  local stash_label="$COMMIT_PREFIX failed: $target_rel"

  if has_repo_changes; then
    printf 'Stashing partial changes for %s\n' "$target_rel"
    git stash push -u -m "$stash_label" >/dev/null
    STASHED_TARGETS+=("$target_rel -> $stash_label")
  fi

  FAILED_TARGETS+=("$target_rel ($reason)")
}

print_summary() {
  printf '\nBatch summary:\n'
  printf '  Successful commits: %d\n' "${#SUCCEEDED_TARGETS[@]}"
  for target in "${SUCCEEDED_TARGETS[@]}"; do
    printf '    - %s\n' "$target"
  done

  printf '  No-change targets: %d\n' "${#NO_CHANGE_TARGETS[@]}"
  for target in "${NO_CHANGE_TARGETS[@]}"; do
    printf '    - %s\n' "$target"
  done

  printf '  Failed targets: %d\n' "${#FAILED_TARGETS[@]}"
  for target in "${FAILED_TARGETS[@]}"; do
    printf '    - %s\n' "$target"
  done

  printf '  Stashed failures: %d\n' "${#STASHED_TARGETS[@]}"
  for target in "${STASHED_TARGETS[@]}"; do
    printf '    - %s\n' "$target"
  done
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -p|--prompt)
      [[ $# -ge 2 ]] || fail "missing value for $1"
      PROMPT="$2"
      shift 2
      ;;
    -t|--test-cmd)
      [[ $# -ge 2 ]] || fail "missing value for $1"
      TEST_CMD="$2"
      shift 2
      ;;
    --fix-attempts)
      [[ $# -ge 2 ]] || fail "missing value for $1"
      [[ "$2" =~ ^[0-9]+$ ]] || fail "--fix-attempts must be a non-negative integer"
      FIX_ATTEMPTS="$2"
      shift 2
      ;;
    -m|--model)
      [[ $# -ge 2 ]] || fail "missing value for $1"
      MODEL="$2"
      shift 2
      ;;
    --agent)
      [[ $# -ge 2 ]] || fail "missing value for $1"
      AGENT="$2"
      shift 2
      ;;
    --auto-approve)
      AUTO_APPROVE=1
      shift
      ;;
    --commit-prefix)
      [[ $# -ge 2 ]] || fail "missing value for $1"
      COMMIT_PREFIX="$2"
      shift 2
      ;;
    --target)
      [[ $# -ge 2 ]] || fail "missing value for $1"
      TARGETS+=("$2")
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --)
      shift
      while [[ $# -gt 0 ]]; do
        TARGETS+=("$1")
        shift
      done
      ;;
    *)
      TARGETS+=("$1")
      shift
      ;;
  esac
done

[[ -n "$PROMPT" ]] || fail "--prompt is required"
[[ ${#TARGETS[@]} -gt 0 ]] || fail "at least one target is required"

command -v git >/dev/null 2>&1 || fail "git is required"
command -v copilot >/dev/null 2>&1 || fail "copilot is required"

REPO_ROOT=$(git rev-parse --show-toplevel)
cd "$REPO_ROOT"
REPO_BIN="$REPO_ROOT/bin"
export PATH="$REPO_BIN:$PATH"

require_clean_tree || fail "git working tree must be clean before running this script"

for raw_target in "${TARGETS[@]}"; do
  abs_target=$(to_abs_path "$raw_target") || fail "target does not exist: $raw_target"
  target_rel=$(to_repo_rel "$abs_target") || fail "target is outside the git repository: $raw_target"

  if [[ -d "$abs_target" ]]; then
    scope_label="directory"
  else
    scope_label="file"
  fi

  printf '\n==> Processing %s: %s\n' "$scope_label" "$target_rel"

scoped_prompt=$(cat <<EOF
Start by reading this $scope_label: $target_rel

Task:
$PROMPT

Guidelines:
- Use this target as the starting point, then read and modify any other repository files you need.
- Follow repository default instructions and any instruction files you find.
- Run tests, review failures, and keep iterating until the result is correct and the tests you judge relevant pass.
- If a tool call or test run fails, inspect it, recover when possible, and continue working instead of stopping at the first failure.
- Suggested test command (hint only, adjust as needed): $TEST_CMD
EOF
)

  if run_copilot_prompt "$scoped_prompt"; then
    :
  else
    copilot_status=$?
    printf 'Copilot failed for %s with exit code %d\n' "$target_rel" "$copilot_status"
    stash_failed_target "$target_rel" "copilot exited with status $copilot_status"
    continue
  fi

  if ! has_repo_changes; then
    printf 'No changes for %s, skipping tests and commit.\n' "$target_rel"
    NO_CHANGE_TARGETS+=("$target_rel")
    continue
  fi

  printf 'Running final verification: %s\n' "$TEST_CMD"
  if run_final_verification "$target_rel"; then
    :
  else
    test_status=$?

    for ((attempt = 1; attempt <= FIX_ATTEMPTS; attempt++)); do
      printf 'Verification failed for %s; asking Copilot to investigate and fix it (attempt %d/%d).\n' "$target_rel" "$attempt" "$FIX_ATTEMPTS"
      recovery_prompt=$(build_recovery_prompt "$scope_label" "$target_rel" "$attempt")

      if run_copilot_prompt "$recovery_prompt"; then
        :
      else
        copilot_status=$?
        printf 'Copilot recovery failed for %s with exit code %d\n' "$target_rel" "$copilot_status"
        stash_failed_target "$target_rel" "copilot recovery exited with status $copilot_status"
        continue 2
      fi

      printf 'Re-running final verification: %s\n' "$TEST_CMD"
      if run_final_verification "$target_rel"; then
        test_status=0
        break
      fi

      test_status=$?
    done

    if [[ "$test_status" -ne 0 ]]; then
      printf 'Final verification failed for %s with exit code %d\n' "$target_rel" "$test_status"
      stash_failed_target "$target_rel" "final verification failed with status $test_status"
      continue
    fi
  fi

  git add -A

  if git diff --cached --quiet; then
    printf 'No staged diff remains for %s after tests, skipping commit.\n' "$target_rel"
    NO_CHANGE_TARGETS+=("$target_rel")
    continue
  fi

  if git commit -m "$COMMIT_PREFIX: $target_rel"; then
    :
  else
    commit_status=$?
    printf 'Commit failed for %s with exit code %d\n' "$target_rel" "$commit_status"
    stash_failed_target "$target_rel" "git commit failed with status $commit_status"
    continue
  fi

  if ! require_clean_tree; then
    printf 'Working tree was not clean after committing %s\n' "$target_rel"
    stash_failed_target "$target_rel" "working tree not clean after commit"
    continue
  fi

  SUCCEEDED_TARGETS+=("$target_rel")
done

print_summary

if [[ ${#FAILED_TARGETS[@]} -gt 0 ]]; then
  exit 1
fi

printf '\nAll targets processed successfully.\n'