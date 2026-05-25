#!/usr/bin/env bash
set -Eeuo pipefail

REMOTE_USER_HOST="${REMOTE_USER_HOST:-ubuntu@192.168.40.221}"
REMOTE_DIR="${REMOTE_DIR:-/data/ganttly/fama}"
SOURCE_DIR="${SOURCE_DIR:-}"
DELETE_REMOTE_EXTRAS=0
DRY_RUN=0
BATCH_MODE=0
USE_GITIGNORE=0
CHECKSUM_MODE=0
FORCE_TRANSFER=0
EXCLUDES=()
GITIGNORE_EXCLUDE_FILE=""

cleanup() {
  if [[ -n "$GITIGNORE_EXCLUDE_FILE" && -f "$GITIGNORE_EXCLUDE_FILE" ]]; then
    rm -f "$GITIGNORE_EXCLUDE_FILE"
  fi
}
trap cleanup EXIT

usage() {
  cat <<'USAGE'
Usage: scripts/sync_to_fama.sh [options]

Sync the current git worktree to ubuntu@192.168.40.221:/data/ganttly/fama.
Same-path files on the remote side are overwritten by default.

Options:
  --source DIR       Source directory. Defaults to the git repository root.
  --remote HOST     Remote SSH target. Default: ubuntu@192.168.40.221
  --remote-dir DIR  Remote destination directory. Default: /data/ganttly/fama
  --delete          Delete files on the remote side that no longer exist locally.
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

if [[ "$USE_GITIGNORE" -eq 1 ]]; then
  if ! git -C "$SOURCE_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "--ignore requires SOURCE_DIR to be inside a git worktree." >&2
    exit 1
  fi

  GIT_TOPLEVEL="$(git -C "$SOURCE_DIR" rev-parse --show-toplevel)"
  GIT_TOPLEVEL="$(cd "$GIT_TOPLEVEL" && pwd)"
  if [[ "$SOURCE_DIR" != "$GIT_TOPLEVEL" ]]; then
    echo "--ignore currently requires SOURCE_DIR to be the git worktree root:" >&2
    echo "  source: $SOURCE_DIR" >&2
    echo "  root:   $GIT_TOPLEVEL" >&2
    exit 1
  fi

  GITIGNORE_EXCLUDE_FILE="$(mktemp)"
  git -C "$SOURCE_DIR" ls-files \
    --others \
    --ignored \
    --exclude-standard \
    --directory \
    -z >"$GITIGNORE_EXCLUDE_FILE"
fi

SSH_OPTS=(
  -o ConnectTimeout=10
  -o ServerAliveInterval=15
  -o ServerAliveCountMax=2
)

if [[ "$BATCH_MODE" -eq 1 ]]; then
  SSH_OPTS+=(-o BatchMode=yes)
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

for pattern in "${EXCLUDES[@]}"; do
  RSYNC_ARGS+=(--exclude "$pattern")
done

echo "Source:      ${SOURCE_DIR}/"
echo "Destination: ${REMOTE_USER_HOST}:${REMOTE_DIR}/"
if [[ "$DELETE_REMOTE_EXTRAS" -eq 1 ]]; then
  echo "Mode:        overwrite and delete remote extras"
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

ssh "${SSH_OPTS[@]}" "$REMOTE_USER_HOST" "mkdir -p '$REMOTE_DIR'"

rsync "${RSYNC_ARGS[@]}" \
  -e "ssh ${SSH_OPTS[*]}" \
  "${SOURCE_DIR}/" \
  "${REMOTE_USER_HOST}:${REMOTE_DIR}/"
