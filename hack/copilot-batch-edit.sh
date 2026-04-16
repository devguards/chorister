#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  hack/copilot-batch-edit.sh --prompt "..." [options] --target path1 --target path2
  hack/copilot-batch-edit.sh --prompt "..." [options] -- path1 path2

Options:
  -p, --prompt TEXT          Prompt applied to each target scope
  -t, --test-cmd CMD         Test command to run after each edit (default: make test)
  -m, --model MODEL          Copilot model name, for example gpt-5.3-codex
      --agent NAME           Optional custom Copilot agent name
      --auto-approve         Run Copilot with --allow-all
      --commit-prefix TEXT   Commit prefix (default: copilot)
      --target PATH          File or directory to process; may be repeated
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

path_within_target() {
  local changed_path="$1"
  local target_path="$2"

  if [[ "$target_path" == "." ]]; then
    return 0
  fi

  case "$changed_path" in
    "$target_path"|"$target_path"/*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

collect_changed_files() {
  local output_file="$1"

  : > "$output_file"
  git diff --name-only HEAD -- | sed '/^$/d' >> "$output_file"
  git ls-files --others --exclude-standard >> "$output_file"
  sort -u "$output_file" -o "$output_file"
}

assert_changes_within_target() {
  local target_rel="$1"
  local changed_file_list="$2"
  local outside_changes=0

  while IFS= read -r changed_path; do
    [[ -n "$changed_path" ]] || continue
    if ! path_within_target "$changed_path" "$target_rel"; then
      echo "Unexpected change outside scope: $changed_path" >&2
      outside_changes=1
    fi
  done < "$changed_file_list"

  [[ "$outside_changes" -eq 0 ]]
}

PROMPT=""
TEST_CMD="make test"
MODEL=""
AGENT=""
AUTO_APPROVE=0
COMMIT_PREFIX="copilot"
TARGETS=()

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

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

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
You are editing exactly one scope in this repository.

Scope type: $scope_label
Scope path: $target_rel

Task:
$PROMPT

Rules:
- Only modify files inside $target_rel.
- Keep changes minimal and focused on the task.
- Update tests in this scope if needed.
- Do not run tests; the wrapper script will run them after you finish.
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

  "${copilot_cmd[@]}"

  changed_file_list="$tmp_dir/changed-files.txt"
  collect_changed_files "$changed_file_list"

  if [[ ! -s "$changed_file_list" ]]; then
    printf 'No changes for %s, skipping tests and commit.\n' "$target_rel"
    continue
  fi

  assert_changes_within_target "$target_rel" "$changed_file_list" || fail "Copilot changed files outside target scope: $target_rel"

  printf 'Running tests: %s\n' "$TEST_CMD"
  eval "$TEST_CMD"

  collect_changed_files "$changed_file_list"
  assert_changes_within_target "$target_rel" "$changed_file_list" || fail "Tests left changes outside target scope: $target_rel"

  git add -A -- "$target_rel"

  if git diff --cached --quiet -- "$target_rel"; then
    printf 'No staged diff remains for %s after tests, skipping commit.\n' "$target_rel"
    continue
  fi

  git commit -m "$COMMIT_PREFIX: $target_rel"
  require_clean_tree || fail "working tree is not clean after committing $target_rel"
done

printf '\nAll targets processed successfully.\n'