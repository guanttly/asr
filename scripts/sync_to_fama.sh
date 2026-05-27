#!/usr/bin/env bash
set -Eeuo pipefail

REMOTE_USER_HOST="${REMOTE_USER_HOST:-ubuntu@192.168.40.221}"
REMOTE_DIR="${REMOTE_DIR:-/data/ganttly/fama}"
SOURCE_DIR="${SOURCE_DIR:-}"
DIRECTION="${DIRECTION:-push}"
DELETE_REMOTE_EXTRAS=0
DRY_RUN=0
BATCH_MODE=0
USE_GITIGNORE=0
CHECKSUM_MODE=0
FORCE_TRANSFER=0
EXCLUDES=()
GITIGNORE_EXCLUDE_FILE=""
TRACKED_FILE_LIST=""
TRACKED_FILE_LIST_RAW=""
REVERSE_FILE_LIST_SOURCE=""

cleanup() {
  local temp_file
  for temp_file in "$GITIGNORE_EXCLUDE_FILE" "$TRACKED_FILE_LIST" "$TRACKED_FILE_LIST_RAW"; do
    if [[ -n "$temp_file" && -f "$temp_file" ]]; then
      rm -f "$temp_file"
    fi
  done
}
trap cleanup EXIT

require_source_git_root() {
  local feature="$1"
  local git_toplevel

  if ! git -C "$SOURCE_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "$feature requires SOURCE_DIR to be inside a git worktree." >&2
    exit 1
  fi

  git_toplevel="$(git -C "$SOURCE_DIR" rev-parse --show-toplevel)"
  git_toplevel="$(cd "$git_toplevel" && pwd)"
  if [[ "$SOURCE_DIR" != "$git_toplevel" ]]; then
    echo "$feature currently requires SOURCE_DIR to be the git worktree root:" >&2
    echo "  source: $SOURCE_DIR" >&2
    echo "  root:   $git_toplevel" >&2
    exit 1
  fi
}

is_reverse_prohibited_path() {
  local path="$1"

  case "$path" in
    ""|/*|../*|*/../*) return 0 ;;
    .git|.git/*|*/.git/*) return 0 ;;
    bin|bin/*|*.exe|*.dll|*.so|*.dylib) return 0 ;;
    *.tar|*.tar.gz|*.tgz|*.zip|*.7z|*.rar|*.whl) return 0 ;;
    *.onnx|*.bin|*.pt|*.pth|*.safetensors|*.gguf) return 0 ;;
    backend/vendor|backend/vendor/*) return 0 ;;
    backend/cmd/*/__debug_bin*) return 0 ;;
    backend/admin-api|backend/asr-api|backend/gateway|backend/nlp-api) return 0 ;;
    backend/rules-catalog-xlsx|backend/term-catalog-xlsx) return 0 ;;
    desktop/src-tauri/vendor/yy-thunks/case-shim|desktop/src-tauri/vendor/yy-thunks/case-shim/*) return 0 ;;
    frontend/node_modules|frontend/node_modules/*) return 0 ;;
    frontend/dist|frontend/dist/*|frontend/.vite|frontend/.vite/*) return 0 ;;
    frontend/test-results|frontend/test-results/*|frontend/playwright-report|frontend/playwright-report/*) return 0 ;;
    desktop/node_modules|desktop/node_modules/*|desktop/dist|desktop/dist/*|desktop/.vite|desktop/.vite/*) return 0 ;;
    desktop-electron/node_modules|desktop-electron/node_modules/*) return 0 ;;
    desktop-electron/dist|desktop-electron/dist/*|desktop-electron/dist-electron|desktop-electron/dist-electron/*) return 0 ;;
    desktop-electron/release|desktop-electron/release/*) return 0 ;;
    *.tsbuildinfo) return 0 ;;
    .vscode/settings.json|.idea|.idea/*|*.swp|*.swo|*~) return 0 ;;
    .DS_Store|Thumbs.db) return 0 ;;
    .env|.env.local|.env.*.local|frontend/.env.development|backend/configs/config.yaml) return 0 ;;
    *.log|backend/uploads|backend/uploads/*) return 0 ;;
    deploy/jusha-asr-business/dist|deploy/jusha-asr-business/dist/*) return 0 ;;
    deploy/jusha-asr-business/dist-fixed|deploy/jusha-asr-business/dist-fixed/*) return 0 ;;
    speaker-diarization-offline.tar.gz) return 0 ;;
    __pycache__|__pycache__/*|*/__pycache__|*/__pycache__/*|*.pyc|*.pyo) return 0 ;;
    dist-*|dist-*/*|.cache|.cache/*|*/.cache|*/.cache/*|tmp|tmp/*) return 0 ;;
    dist|dist/*|*/dist|*/dist/*) return 0 ;;
    .pytest_cache|.pytest_cache/*|*/.pytest_cache|*/.pytest_cache/*) return 0 ;;
    wheels|wheels/*|*/wheels|*/wheels/*) return 0 ;;
    deploy/3d-speaker/models|deploy/3d-speaker/models/*) return 0 ;;
    deploy/qwen3-asr/models|deploy/qwen3-asr/models/*) return 0 ;;
    deploy/qwen3-asr/image|deploy/qwen3-asr/image/*) return 0 ;;
    deploy/cam++/models|deploy/cam++/models/*) return 0 ;;
    deploy/jusha-asr/models|deploy/jusha-asr/models/*) return 0 ;;
  esac

  return 1
}

write_reverse_file_list() {
  local raw_file="$1"
  local output_file="$2"
  local path

  : >"$output_file"
  while IFS= read -r -d '' path; do
    if is_reverse_prohibited_path "$path"; then
      continue
    fi
    printf '%s\0' "$path" >>"$output_file"
  done <"$raw_file"
}

usage() {
  cat <<'USAGE'
Usage: scripts/sync_to_fama.sh [options]

Sync the current git worktree with ubuntu@192.168.40.221:/data/ganttly/fama.
Push mode is the default. Same-path files on the destination side are overwritten.

Reverse mode pulls from the remote side to local and only syncs Git-tracked
paths by default. Runtime artifacts, build outputs, model files, caches, and
local secrets are never pulled by reverse mode.

Options:
  --source DIR       Source directory. Defaults to the git repository root.
  --remote HOST     Remote SSH target. Default: ubuntu@192.168.40.221
  --remote-dir DIR  Remote destination directory. Default: /data/ganttly/fama
  --reverse, --pull  Pull from remote to local using the safe tracked-file scope.
  --push            Push from local to remote. This is the default.
  --delete          Delete files on the destination side that no longer exist in source.
  --ignore          Exclude files ignored by gitignore/exclude rules.
  --checksum        Compare file content checksums instead of only size/mtime.
  --force-transfer  Transfer every non-excluded file even if rsync thinks it is unchanged.
  --exclude PATTERN Exclude an rsync pattern. Can be specified multiple times.
  --dry-run         Show what would be copied without changing the remote side.
  --batch           Disable password prompts and fail fast if SSH keys do not work.
  -h, --help        Show this help.

Environment overrides:
  REMOTE_USER_HOST, REMOTE_DIR, SOURCE_DIR
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source)
      SOURCE_DIR="${2:?missing value for --source}"
      shift 2
      ;;
    --remote)
      REMOTE_USER_HOST="${2:?missing value for --remote}"
      shift 2
      ;;
    --remote-dir)
      REMOTE_DIR="${2:?missing value for --remote-dir}"
      shift 2
      ;;
    --reverse|--pull)
      DIRECTION="pull"
      shift
      ;;
    --push)
      DIRECTION="push"
      shift
      ;;
    --delete)
      DELETE_REMOTE_EXTRAS=1
      shift
      ;;
    --ignore)
      USE_GITIGNORE=1
      shift
      ;;
    --checksum)
      CHECKSUM_MODE=1
      shift
      ;;
    --force-transfer)
      FORCE_TRANSFER=1
      shift
      ;;
    --exclude)
      EXCLUDES+=("${2:?missing value for --exclude}")
      shift 2
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --batch)
      BATCH_MODE=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$SOURCE_DIR" ]]; then
  SOURCE_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi

SOURCE_DIR="$(cd "$SOURCE_DIR" && pwd)"

if ! command -v rsync >/dev/null 2>&1; then
  echo "rsync is required but was not found locally." >&2
  exit 1
fi

SSH_OPTS=(
  -o ConnectTimeout=10
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=2
)

if [[ "$BATCH_MODE" -eq 1 ]]; then
  SSH_OPTS+=(-o BatchMode=yes)
fi

case "$DIRECTION" in
  push|pull) ;;
  *)
    echo "Unknown direction: $DIRECTION" >&2
    usage >&2
    exit 2
    ;;
esac

if [[ "$USE_GITIGNORE" -eq 1 ]]; then
  require_source_git_root "--ignore"

  GITIGNORE_EXCLUDE_FILE="$(mktemp)"
  git -C "$SOURCE_DIR" ls-files \
    --others \
    --ignored \
    --exclude-standard \
    --directory \
    -z >"$GITIGNORE_EXCLUDE_FILE"
fi

if [[ "$DIRECTION" == "pull" ]]; then
  require_source_git_root "--reverse"

  if ! ssh "${SSH_OPTS[@]}" "$REMOTE_USER_HOST" "test -d '$REMOTE_DIR'"; then
    echo "Unable to reach remote directory: ${REMOTE_USER_HOST}:${REMOTE_DIR}" >&2
    exit 1
  fi

  TRACKED_FILE_LIST_RAW="$(mktemp)"
  TRACKED_FILE_LIST="$(mktemp)"

  if ssh "${SSH_OPTS[@]}" "$REMOTE_USER_HOST" "git -C '$REMOTE_DIR' rev-parse --is-inside-work-tree >/dev/null 2>&1"; then
    ssh "${SSH_OPTS[@]}" "$REMOTE_USER_HOST" "git -C '$REMOTE_DIR' ls-files -z" >"$TRACKED_FILE_LIST_RAW"
    REVERSE_FILE_LIST_SOURCE="remote git index"
  else
    git -C "$SOURCE_DIR" ls-files -z >"$TRACKED_FILE_LIST_RAW"
    REVERSE_FILE_LIST_SOURCE="local git index"
  fi

  write_reverse_file_list "$TRACKED_FILE_LIST_RAW" "$TRACKED_FILE_LIST"
fi

RSYNC_ARGS=(
  -a
  --human-readable
  --info=progress2
  --partial
  --force
)

if [[ "$DELETE_REMOTE_EXTRAS" -eq 1 ]]; then
  RSYNC_ARGS+=(--delete)
fi

if [[ "$CHECKSUM_MODE" -eq 1 ]]; then
  RSYNC_ARGS+=(--checksum)
fi

if [[ "$FORCE_TRANSFER" -eq 1 ]]; then
  RSYNC_ARGS+=(--ignore-times)
fi

if [[ "$DRY_RUN" -eq 1 ]]; then
  RSYNC_ARGS+=(--dry-run --itemize-changes)
fi

if [[ "$USE_GITIGNORE" -eq 1 ]]; then
  RSYNC_ARGS+=(--from0 --exclude-from "$GITIGNORE_EXCLUDE_FILE")
fi

if [[ "$DIRECTION" == "pull" ]]; then
  RSYNC_ARGS+=(--from0 --files-from "$TRACKED_FILE_LIST")
fi

for pattern in "${EXCLUDES[@]}"; do
  RSYNC_ARGS+=(--exclude "$pattern")
done

if [[ "$DIRECTION" == "pull" ]]; then
  echo "Direction:   remote -> local"
  echo "Source:      ${REMOTE_USER_HOST}:${REMOTE_DIR}/"
  echo "Destination: ${SOURCE_DIR}/"
  echo "Scope:       git-tracked files only (${REVERSE_FILE_LIST_SOURCE}); artifacts/models/caches excluded"
else
  echo "Direction:   local -> remote"
  echo "Source:      ${SOURCE_DIR}/"
  echo "Destination: ${REMOTE_USER_HOST}:${REMOTE_DIR}/"
fi
if [[ "$DELETE_REMOTE_EXTRAS" -eq 1 ]]; then
  echo "Mode:        overwrite and delete destination extras"
else
  echo "Mode:        overwrite only"
fi
if [[ "$DRY_RUN" -eq 1 ]]; then
  echo "Dry run:     yes"
fi
if [[ "$USE_GITIGNORE" -eq 1 ]]; then
  echo "Gitignore:   enabled"
fi
if [[ "$CHECKSUM_MODE" -eq 1 ]]; then
  echo "Compare:     checksum"
fi
if [[ "$FORCE_TRANSFER" -eq 1 ]]; then
  echo "Transfer:    force all non-excluded files"
fi

if [[ "$DIRECTION" == "pull" ]]; then
  rsync "${RSYNC_ARGS[@]}" \
    -e "ssh ${SSH_OPTS[*]}" \
    "${REMOTE_USER_HOST}:${REMOTE_DIR}/" \
    "${SOURCE_DIR}/"
else
  ssh "${SSH_OPTS[@]}" "$REMOTE_USER_HOST" "mkdir -p '$REMOTE_DIR'"

  rsync "${RSYNC_ARGS[@]}" \
    -e "ssh ${SSH_OPTS[*]}" \
    "${SOURCE_DIR}/" \
    "${REMOTE_USER_HOST}:${REMOTE_DIR}/"
fi
