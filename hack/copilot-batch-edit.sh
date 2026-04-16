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
MODEL=""
AGENT=""
AUTO_APPROVE=0
COMMIT_PREFIX="copilot"
TARGETS=()
SUCCEEDED_TARGETS=()
NO_CHANGE_TARGETS=()
FAILED_TARGETS=()
STASHED_TARGETS=()

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

  copilot_cmd=(copilot -p "$scoped_prompt" -s --no-ask-user)

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

  if "${copilot_cmd[@]}"; then
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
  if eval "$TEST_CMD"; then
    :
  else
    test_status=$?
    printf 'Final verification failed for %s with exit code %d\n' "$target_rel" "$test_status"
    stash_failed_target "$target_rel" "final verification failed with status $test_status"
    continue
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