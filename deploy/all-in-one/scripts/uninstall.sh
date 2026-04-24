#!/bin/sh
set -eu

MODE=uninstall
REMOVE_IMAGE=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    uninstall)
      MODE=uninstall
      ;;
    purge)
      MODE=purge
      ;;
    --remove-image)
      REMOVE_IMAGE=1
      ;;
    -h|--help)
      echo "用法: uninstall.sh [uninstall|purge] [--remove-image]"
      echo "  uninstall       停止并删除容器，保留 runtime 数据目录与 .env 配置。"
      echo "  purge           停止并删除容器，同时清空 runtime 数据目录和 backups。"
      echo "  --remove-image  额外删除本地离线镜像标签。"
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      exit 1
      ;;
  esac
  shift
done

if ! command -v docker >/dev/null 2>&1; then
  echo "docker 未安装，无法继续卸载" >&2
  exit 1
fi

if docker compose version >/dev/null 2>&1; then
  COMPOSE_CMD='docker compose'
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_CMD='docker-compose'
else
  echo "未找到 docker compose 或 docker-compose" >&2
  exit 1
fi

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
cd "$SCRIPT_DIR"

if [ -f .env ]; then
  # shellcheck disable=SC1091
  . ./.env
fi

if [ -f .release-manifest ]; then
  # shellcheck disable=SC1091
  . ./.release-manifest
fi

ASR_CONTAINER_NAME=${ASR_CONTAINER_NAME:-asr-all-in-one}
ASR_RELEASE_IMAGE=${ASR_RELEASE_IMAGE:-${RELEASE_IMAGE:-asr-all-in-one:latest}}

echo "停止并移除服务..."
if [ -f docker-compose.yml ]; then
  sh -c "$COMPOSE_CMD -f docker-compose.yml down --remove-orphans" || true
fi

docker rm -f "$ASR_CONTAINER_NAME" >/dev/null 2>&1 || true

if [ "$MODE" = "purge" ]; then
  echo "清理本地数据目录..."
  rm -rf runtime/mysql runtime/certs runtime/downloads runtime/tmp runtime/uploads backups
fi

if [ "$REMOVE_IMAGE" -eq 1 ]; then
  echo "删除本地镜像标签..."
  docker image rm "$ASR_RELEASE_IMAGE" >/dev/null 2>&1 || true
  docker image rm asr-all-in-one:latest >/dev/null 2>&1 || true
fi

echo "卸载完成。"
if [ "$MODE" = "purge" ]; then
  echo "已删除 runtime 数据目录和 backups。"
else
  echo "已保留 runtime 数据目录和 .env，可直接重新执行 install.sh 或 install.sh upgrade。"
fi
if [ "$REMOVE_IMAGE" -eq 1 ]; then
  echo "已尝试删除本地镜像标签。"
fi