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
      echo "  uninstall       停止并删除容器，保留 data/logs/models/config。"
      echo "  purge           停止并删除容器，同时清空 data/logs/backups，保留 models/config/image。"
      echo "  --remove-image  额外删除本地镜像标签。"
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
  COMPOSE_FLAVOR="v2"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_FLAVOR="v1"
else
  echo "未找到 docker compose 或 docker-compose" >&2
  exit 1
fi

prepare_compose_file() {
  COMPOSE_FILE_PATH=docker-compose.yml
  if [ "$COMPOSE_FLAVOR" = "v1" ] && grep -q '^[[:space:]]*pull_policy:' docker-compose.yml 2>/dev/null; then
    COMPOSE_FILE_PATH=.docker-compose.runtime.yml
    awk '$1 == "pull_policy:" { next } { print }' docker-compose.yml > "$COMPOSE_FILE_PATH"
  fi
}

compose() {
  if [ "$COMPOSE_FLAVOR" = "v2" ]; then
    docker compose "$@"
  else
    docker-compose "$@"
  fi
}

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
cd "$SCRIPT_DIR"
prepare_compose_file

if [ -f .env ]; then
  # shellcheck disable=SC1091
  . ./.env
fi

if [ -f .release-manifest ]; then
  # shellcheck disable=SC1091
  . ./.release-manifest
fi

SA_CONTAINER_NAME=${SA_CONTAINER_NAME:-speaker-analysis}
SA_IMAGE=${SA_IMAGE:-${RELEASE_IMAGE:-speaker-analysis-service:latest}}

echo "停止并移除服务..."
if [ -f docker-compose.yml ]; then
  compose -f "$COMPOSE_FILE_PATH" down --remove-orphans || true
fi

docker rm -f "$SA_CONTAINER_NAME" >/dev/null 2>&1 || true

if [ "$MODE" = "purge" ]; then
  echo "清理运行数据目录..."
  rm -rf data logs backups
fi

if [ "$REMOVE_IMAGE" -eq 1 ]; then
  echo "删除本地镜像标签..."
  docker image rm "$SA_IMAGE" >/dev/null 2>&1 || true
fi

echo "卸载完成。"
if [ "$MODE" = "purge" ]; then
  echo "已删除 data/logs/backups，保留 models/config/image。"
else
  echo "已保留 data/logs/models/config，可重新执行 install.sh。"
fi
