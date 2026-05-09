#!/bin/sh
set -eu

ACTION=${1:-install}
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
MANIFEST_FILE="$SCRIPT_DIR/.release-manifest"

case "$ACTION" in
  install|upgrade|load-only)
    ;;
  -h|--help)
    echo "用法: install.sh [install|upgrade|load-only]"
    echo "  install/load-only 全程只使用安装包内文件，不会 pull 镜像或下载模型。"
    exit 0
    ;;
  *)
    echo "未知命令: $ACTION" >&2
    echo "用法: install.sh [install|upgrade|load-only]" >&2
    exit 1
    ;;
esac

if ! command -v docker >/dev/null 2>&1; then
  echo "docker 未安装，无法继续安装" >&2
  exit 1
fi

COMPOSE_FLAVOR=""
if docker compose version >/dev/null 2>&1; then
  COMPOSE_FLAVOR="v2"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_FLAVOR="v1"
else
  echo "未找到 docker compose 或 docker-compose" >&2
  exit 1
fi

RELEASE_VERSION=unknown
RELEASE_IMAGE=speaker-analysis-service:latest
RELEASE_IMAGE_ARCHIVE=image/speaker-analysis-service-image.tar.gz
RELEASE_IMAGE_ARCHIVE_SHA256=""
if [ -f "$MANIFEST_FILE" ]; then
  # shellcheck disable=SC1090
  . "$MANIFEST_FILE"
fi

IMAGE_ARCHIVE="$SCRIPT_DIR/$RELEASE_IMAGE_ARCHIVE"

update_env_value() {
  KEY="$1"
  VALUE="$2"
  FILE_PATH="$3"
  TMP_FILE=$(mktemp)

  if [ -f "$FILE_PATH" ]; then
    awk -v key="$KEY" -v value="$VALUE" '
      BEGIN { updated = 0 }
      index($0, key "=") == 1 {
        print key "=" value
        updated = 1
        next
      }
      { print }
      END {
        if (updated == 0)
          print key "=" value
      }
    ' "$FILE_PATH" > "$TMP_FILE"
  else
    printf '%s=%s\n' "$KEY" "$VALUE" > "$TMP_FILE"
  fi

  mv "$TMP_FILE" "$FILE_PATH"
}

sha256_file() {
  FILE_PATH="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$FILE_PATH" | awk '{print $1}'
    return
  fi
  shasum -a 256 "$FILE_PATH" | awk '{print $1}'
}

validate_archive() {
  if [ ! -f "$IMAGE_ARCHIVE" ]; then
    echo "缺少离线镜像包: $IMAGE_ARCHIVE" >&2
    exit 1
  fi
  if [ -n "$RELEASE_IMAGE_ARCHIVE_SHA256" ]; then
    ACTUAL_SHA256=$(sha256_file "$IMAGE_ARCHIVE")
    if [ "$ACTUAL_SHA256" != "$RELEASE_IMAGE_ARCHIVE_SHA256" ]; then
      echo "离线镜像包校验失败: $IMAGE_ARCHIVE" >&2
      echo "期望: $RELEASE_IMAGE_ARCHIVE_SHA256" >&2
      echo "实际: $ACTUAL_SHA256" >&2
      exit 1
    fi
  fi
}

validate_models() {
  if [ ! -d models/fsmn_vad ] || ! find models/fsmn_vad -type f ! -name '.gitkeep' -print -quit 2>/dev/null | grep -q .; then
    echo "模型目录不完整: models/fsmn_vad 为空" >&2
    exit 1
  fi
  if ! find models/eres2netv2 models/campplus -type f ! -name '.gitkeep' -print -quit 2>/dev/null | grep -q .; then
    echo "模型目录不完整: 需要 models/eres2netv2 或 models/campplus" >&2
    exit 1
  fi
  if ! find models/native_cache -type f -name 'campplus_cn_en_common.pt' -print -quit 2>/dev/null | grep -q .; then
    echo "警告: models/native_cache 缺少 campplus_cn_en_common.pt，原生 diarization 会回退兼容模式。" >&2
  fi
}

wait_for_service_health() {
  CONTAINER_NAME="$1"
  ATTEMPTS=${2:-60}
  INDEX=0
  LAST_STATUS=""

  while [ "$INDEX" -lt "$ATTEMPTS" ]; do
    STATUS=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$CONTAINER_NAME" 2>/dev/null || true)
    if [ -n "$STATUS" ] && [ "$STATUS" != "$LAST_STATUS" ]; then
      echo "当前容器状态: $STATUS"
      LAST_STATUS="$STATUS"
    fi
    case "$STATUS" in
      healthy)
        return 0
        ;;
      running)
        HAS_HEALTH=$(docker inspect -f '{{if .State.Health}}1{{else}}0{{end}}' "$CONTAINER_NAME" 2>/dev/null || printf '0')
        if [ "$HAS_HEALTH" = "0" ]; then
          return 0
        fi
        ;;
      unhealthy|exited|dead)
        return 1
        ;;
    esac
    INDEX=$((INDEX + 1))
    sleep 5
  done

  return 1
}

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

print_diagnostics() {
  CONTAINER_NAME="$1"
  echo "容器诊断信息:"
  docker inspect -f '  state={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}} exit={{.State.ExitCode}}' "$CONTAINER_NAME" 2>/dev/null || true
  echo "最近 200 行容器日志:"
  docker logs --tail 200 "$CONTAINER_NAME" 2>&1 || true
}

compose_up() {
  if [ "$COMPOSE_FLAVOR" = "v2" ]; then
    compose -f "$COMPOSE_FILE_PATH" up -d --force-recreate --remove-orphans --no-build --pull never
  else
    compose -f "$COMPOSE_FILE_PATH" up -d --force-recreate --remove-orphans --no-build
  fi
}

cd "$SCRIPT_DIR"
prepare_compose_file

validate_archive
validate_models
mkdir -p data logs config models backups

if [ -f .env.example ] && [ ! -f .env ]; then
  cp .env.example .env
fi

update_env_value SA_IMAGE "$RELEASE_IMAGE" .env
update_env_value SA_RELEASE_VERSION "$RELEASE_VERSION" .env

# shellcheck disable=SC1091
. ./.env

SA_IMAGE=${SA_IMAGE:-$RELEASE_IMAGE}
SA_CONTAINER_NAME=${SA_CONTAINER_NAME:-speaker-analysis}
SA_PORT=${SA_PORT:-10002}

BACKUP_DIR="$SCRIPT_DIR/backups/$(date +%Y%m%d%H%M%S)"
mkdir -p "$BACKUP_DIR"
[ -f .env ] && cp .env "$BACKUP_DIR/.env"
[ -f docker-compose.yml ] && cp docker-compose.yml "$BACKUP_DIR/docker-compose.yml"
[ -f .release-manifest ] && cp .release-manifest "$BACKUP_DIR/.release-manifest"

CURRENT_IMAGE=""
if docker container inspect "$SA_CONTAINER_NAME" >/dev/null 2>&1; then
  CURRENT_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$SA_CONTAINER_NAME" 2>/dev/null || true)
fi

echo "加载离线镜像: $IMAGE_ARCHIVE"
gzip -dc "$IMAGE_ARCHIVE" | docker load

if ! docker image inspect "$SA_IMAGE" >/dev/null 2>&1; then
  echo "镜像已加载，但未找到 .env 指定的镜像标签: $SA_IMAGE" >&2
  echo "请检查 .release-manifest 与 .env 中的 SA_IMAGE 是否一致。" >&2
  exit 1
fi

if [ "$ACTION" = "load-only" ]; then
  echo "镜像加载完成，已按 load-only 退出。"
  exit 0
fi

if [ -n "$CURRENT_IMAGE" ]; then
  echo "检测到已有实例，准备从 $CURRENT_IMAGE 升级到 $SA_IMAGE"
else
  echo "未检测到已有实例，执行首次安装"
fi

echo "启动服务（离线模式，不拉取镜像）..."
compose_up

echo "等待服务健康检查..."
if ! wait_for_service_health "$SA_CONTAINER_NAME" 90; then
  print_diagnostics "$SA_CONTAINER_NAME"
  echo "安装或升级后的服务未通过健康检查，请检查容器日志。" >&2
  exit 1
fi

echo "安装完成。"
echo "版本: $RELEASE_VERSION"
echo "镜像: $SA_IMAGE"
echo "备份目录: $BACKUP_DIR"
echo "访问地址: http://localhost:$SA_PORT/docs"
echo "健康检查: http://localhost:$SA_PORT/api/v1/health"
