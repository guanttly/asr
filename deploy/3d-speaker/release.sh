#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_SCRIPT="${SCRIPT_DIR}/build.sh"
DEFAULT_TARGET="ubuntu@192.168.40.221:/data/ganttly/"

TARGET="${RELEASE_TARGET:-${DEFAULT_TARGET}}"
PORT="${RELEASE_PORT:-}"
SKIP_BUILD=0
DRY_RUN=0

usage() {
    cat <<EOF
用法: ./release.sh [--target user@host:/path/] [--port 22] [--skip-build] [--dry-run]

默认目标: ${DEFAULT_TARGET}

参数:
  --target      指定 scp 目标，默认 ${DEFAULT_TARGET}
  --port        指定 scp 端口
  --skip-build  跳过 ./build.sh export-run，直接上传现有产物
  --dry-run     只打印命令，不实际执行
  -h, --help    显示帮助
EOF
}

run_cmd() {
    if [ "${DRY_RUN}" -eq 1 ]; then
        printf '+'
        printf ' %q' "$@"
        printf '\n'
        return
    fi

    "$@"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --target)
            if [ "$#" -lt 2 ]; then
                echo "缺少 --target 参数值" >&2
                exit 1
            fi
            TARGET="$2"
            shift 2
            ;;
        --port)
            if [ "$#" -lt 2 ]; then
                echo "缺少 --port 参数值" >&2
                exit 1
            fi
            PORT="$2"
            shift 2
            ;;
        --skip-build)
            SKIP_BUILD=1
            shift
            ;;
        --dry-run)
            DRY_RUN=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "未知参数: $1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

IMAGE_TAG="$(sed -n 's/^IMAGE_TAG="\([^"]*\)"$/\1/p' "${BUILD_SCRIPT}" | head -n 1)"
if [ -z "${IMAGE_TAG}" ]; then
    echo "未能从 ${BUILD_SCRIPT} 解析 IMAGE_TAG" >&2
    exit 1
fi

PACKAGE_NAME="speaker-analysis-service-${IMAGE_TAG}.run"
PACKAGE_PATH="${SCRIPT_DIR}/dist/${PACKAGE_NAME}"

SCP_ARGS=()
if [ -n "${PORT}" ]; then
    SCP_ARGS=(-P "${PORT}")
fi

if [ "${SKIP_BUILD}" -ne 1 ]; then
    run_cmd bash "${BUILD_SCRIPT}" export-run
fi

if [ ! -f "${PACKAGE_PATH}" ]; then
    echo "未找到离线安装包: ${PACKAGE_PATH}" >&2
    exit 1
fi

echo "上传文件: ${PACKAGE_PATH}"
echo "目标地址: ${TARGET}"
run_cmd scp "${SCP_ARGS[@]}" "${PACKAGE_PATH}" "${TARGET}"