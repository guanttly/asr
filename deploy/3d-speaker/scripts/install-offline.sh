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
RELEASE_IMAGE=jusha-asr-speaker:latest
RELEASE_IMAGE_ARCHIVE=image/jusha-asr-speaker-image.tar.gz
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
  if [ "${ALLOW_INCOMPLETE_NATIVE_CACHE:-0}" = "1" ]; then
    echo "警告: ALLOW_INCOMPLETE_NATIVE_CACHE=1，跳过 native_cache 完整性强校验。" >&2
    return 0
  fi
  if ! find models/native_cache/iic/speech_campplus_sv_zh_en_16k-common_advanced -type f -name 'campplus_cn_en_common.pt' -print -quit 2>/dev/null | grep -q .; then
    echo "模型目录不完整: 缺少 models/native_cache/iic/speech_campplus_sv_zh_en_16k-common_advanced/campplus_cn_en_common.pt" >&2
    exit 1
  fi
  if ! find models/native_cache/iic/speech_fsmn_vad_zh-cn-16k-common-pytorch -type f -name 'configuration.json' -print -quit 2>/dev/null | grep -q . \
    || ! find models/native_cache/iic/speech_fsmn_vad_zh-cn-16k-common-pytorch -type f -name 'model.pt' -print -quit 2>/dev/null | grep -q .; then
    echo "模型目录不完整: 缺少 models/native_cache/iic/speech_fsmn_vad_zh-cn-16k-common-pytorch/{configuration.json,model.pt}" >&2
    exit 1
  fi
}

docker_subnet_candidates() {
  if [ -n "${SA_DOCKER_SUBNET_CANDIDATES:-}" ]; then
    printf '%s\n' "$SA_DOCKER_SUBNET_CANDIDATES" | tr ',;' '\n' | awk 'NF { print $1 }'
    return 0
  fi

  cat <<'EOF'
10.248.10.0/24
10.249.10.0/24
10.250.10.0/24
10.251.10.0/24
192.168.248.0/24
192.168.249.0/24
192.168.250.0/24
100.64.248.0/24
100.65.248.0/24
100.127.248.0/24
EOF
}

is_valid_cidr() {
  CIDR_VALUE="$1"
  printf '%s\n' "$CIDR_VALUE" | awk '
    function valid_ip(ip, parts, i) {
      if (split(ip, parts, ".") != 4)
        return 0
      for (i = 1; i <= 4; i++) {
        if (parts[i] !~ /^[0-9]+$/ || parts[i] < 0 || parts[i] > 255)
          return 0
      }
      return 1
    }
    {
      if (split($0, parts, "/") != 2)
        exit 1
      if (!valid_ip(parts[1]) || parts[2] !~ /^[0-9]+$/ || parts[2] < 16 || parts[2] > 30)
        exit 1
      exit 0
    }
  '
}

cidr_gateway() {
  CIDR_VALUE="$1"
  printf '%s\n' "$CIDR_VALUE" | awk '
    function ip2int(ip, parts) {
      split(ip, parts, ".")
      return (((parts[1] * 256 + parts[2]) * 256 + parts[3]) * 256 + parts[4])
    }
    function int2ip(value, a, b, c, d) {
      a = int(value / 16777216)
      value -= a * 16777216
      b = int(value / 65536)
      value -= b * 65536
      c = int(value / 256)
      d = value - c * 256
      printf "%d.%d.%d.%d\n", a, b, c, d
    }
    {
      split($0, parts, "/")
      block = 2 ^ (32 - parts[2])
      start = int(ip2int(parts[1]) / block) * block
      int2ip(start + 1)
    }
  '
}

own_docker_bridge_dev() {
  NETWORK_NAME="$1"
  if [ -z "$NETWORK_NAME" ] || ! docker network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
    return 0
  fi

  NETWORK_ID=$(docker network inspect -f '{{.Id}}' "$NETWORK_NAME" 2>/dev/null || true)
  if [ -n "$NETWORK_ID" ]; then
    printf 'br-%.12s\n' "$NETWORK_ID"
  fi
}

list_host_route_cidrs() {
  NETWORK_NAME="$1"
  EXCLUDE_DEV=$(own_docker_bridge_dev "$NETWORK_NAME" || true)

  if command -v ip >/dev/null 2>&1; then
    ip -4 route show 2>/dev/null | awk -v exclude_dev="$EXCLUDE_DEV" '
      $1 == "default" { next }
      $1 !~ /^[0-9]+\./ { next }
      {
        dev = ""
        for (i = 1; i < NF; i++) {
          if ($i == "dev")
            dev = $(i + 1)
        }
        if (exclude_dev != "" && dev == exclude_dev)
          next
        print $1
      }
    '
    return 0
  fi

  if command -v route >/dev/null 2>&1; then
    route -n 2>/dev/null | awk -v exclude_dev="$EXCLUDE_DEV" '
      function mask_prefix(mask, parts, i, j, octet, bits) {
        split(mask, parts, ".")
        bits = 0
        for (i = 1; i <= 4; i++) {
          octet = parts[i] + 0
          for (j = 7; j >= 0; j--) {
            if (octet >= 2 ^ j) {
              bits++
              octet -= 2 ^ j
            }
          }
        }
        return bits
      }
      NR > 2 && $1 ~ /^[0-9]+\./ && $1 != "0.0.0.0" {
        if (exclude_dev != "" && $8 == exclude_dev)
          next
        print $1 "/" mask_prefix($3)
      }
    '
  fi
}

list_docker_network_cidrs() {
  SKIP_NETWORK_NAME="$1"
  docker network ls --format '{{.Name}}' 2>/dev/null | while IFS= read -r NETWORK_NAME; do
    if [ -z "$NETWORK_NAME" ] || [ "$NETWORK_NAME" = "$SKIP_NETWORK_NAME" ]; then
      continue
    fi
    docker network inspect -f '{{range .IPAM.Config}}{{println .Subnet}}{{end}}' "$NETWORK_NAME" 2>/dev/null | awk '/^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\// { print }' || true
  done
}

find_cidr_conflict() {
  CIDR_VALUE="$1"
  NETWORK_NAME="$2"
  {
    list_host_route_cidrs "$NETWORK_NAME"
    list_docker_network_cidrs "$NETWORK_NAME"
  } | awk -v candidate="$CIDR_VALUE" '
    function valid_ip(ip, parts, i) {
      if (split(ip, parts, ".") != 4)
        return 0
      for (i = 1; i <= 4; i++) {
        if (parts[i] !~ /^[0-9]+$/ || parts[i] < 0 || parts[i] > 255)
          return 0
      }
      return 1
    }
    function ip2int(ip, parts) {
      split(ip, parts, ".")
      return (((parts[1] * 256 + parts[2]) * 256 + parts[3]) * 256 + parts[4])
    }
    function parse_range(cidr, range, parts, prefix, block, start) {
      gsub(/^[ \t]+|[ \t]+$/, "", cidr)
      if (cidr == "" || cidr == "default")
        return 0
      split(cidr, parts, "/")
      if (!valid_ip(parts[1]))
        return 0
      prefix = parts[2]
      if (prefix == "")
        prefix = 32
      if (prefix !~ /^[0-9]+$/ || prefix < 0 || prefix > 32)
        return 0
      block = 2 ^ (32 - prefix)
      start = int(ip2int(parts[1]) / block) * block
      range["start"] = start
      range["end"] = start + block - 1
      return 1
    }
    BEGIN {
      if (!parse_range(candidate, wanted))
        exit 0
    }
    {
      if (!parse_range($1, used))
        next
      if (used["start"] <= wanted["end"] && wanted["start"] <= used["end"]) {
        print $1
        exit 0
      }
    }
  ' | head -n 1
}

cidr_is_available() {
  CIDR_VALUE="$1"
  NETWORK_NAME="$2"
  if ! is_valid_cidr "$CIDR_VALUE"; then
    return 1
  fi

  CONFLICT_CIDR=$(find_cidr_conflict "$CIDR_VALUE" "$NETWORK_NAME" || true)
  [ -z "$CONFLICT_CIDR" ]
}

choose_docker_subnet() {
  NETWORK_NAME="$1"
  CURRENT_SUBNET=${SA_DOCKER_SUBNET:-}
  case "$CURRENT_SUBNET" in
    ''|AUTO|auto)
      ;;
    *)
      if cidr_is_available "$CURRENT_SUBNET" "$NETWORK_NAME"; then
        printf '%s\n' "$CURRENT_SUBNET"
        return 0
      fi
      CONFLICT_CIDR=$(find_cidr_conflict "$CURRENT_SUBNET" "$NETWORK_NAME" || true)
      echo "检测到当前 Docker 内部网段 $CURRENT_SUBNET 与宿主机/已有 Docker 网络冲突${CONFLICT_CIDR:+: $CONFLICT_CIDR}，将自动切换。" >&2
      ;;
  esac

  for CIDR_VALUE in $(docker_subnet_candidates); do
    if cidr_is_available "$CIDR_VALUE" "$NETWORK_NAME"; then
      printf '%s\n' "$CIDR_VALUE"
      return 0
    fi
  done

  return 1
}

docker_network_matches_config() {
  NETWORK_NAME="$1"
  SUBNET_VALUE="$2"
  GATEWAY_VALUE="$3"

  docker network inspect -f '{{range .IPAM.Config}}{{println .Subnet .Gateway}}{{end}}' "$NETWORK_NAME" 2>/dev/null | awk -v subnet="$SUBNET_VALUE" -v gateway="$GATEWAY_VALUE" '
    $1 == subnet && $2 == gateway { found = 1 }
    END { exit found ? 0 : 1 }
  '
}

network_attached_container_names() {
  NETWORK_NAME="$1"
  docker network inspect -f '{{range .Containers}}{{println .Name}}{{end}}' "$NETWORK_NAME" 2>/dev/null | awk 'NF { print }' || true
}

ensure_docker_network_matches_config() {
  NETWORK_NAME="$1"
  SUBNET_VALUE="$2"
  GATEWAY_VALUE="$3"

  if ! docker network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
    return 0
  fi

  if docker_network_matches_config "$NETWORK_NAME" "$SUBNET_VALUE" "$GATEWAY_VALUE"; then
    return 0
  fi

  ATTACHED_CONTAINERS=$(network_attached_container_names "$NETWORK_NAME")
  if [ -n "$ATTACHED_CONTAINERS" ]; then
    FOREIGN_CONTAINER=$(printf '%s\n' "$ATTACHED_CONTAINERS" | awk -v own="$SA_CONTAINER_NAME" '$0 != own { print; exit }')
    if [ -n "$FOREIGN_CONTAINER" ]; then
      echo "Docker 网络 $NETWORK_NAME 已存在但网段配置不一致，且仍有非本应用容器连接: $FOREIGN_CONTAINER" >&2
      echo "请先处理该网络，或在 .env 中设置 SA_DOCKER_NETWORK_NAME 为新的网络名。" >&2
      return 1
    fi

    echo "Docker 网络 $NETWORK_NAME 的网段配置需要切换，先停止旧实例..."
    compose -f "$COMPOSE_FILE_PATH" down --remove-orphans || true
    docker rm -f "$SA_CONTAINER_NAME" >/dev/null 2>&1 || true
  fi

  docker network rm "$NETWORK_NAME" >/dev/null 2>&1 || {
    echo "无法删除旧 Docker 网络 $NETWORK_NAME，请检查是否仍有容器占用。" >&2
    return 1
  }
}

existing_docker_network_config() {
  NETWORK_NAME="$1"
  docker network inspect -f '{{range .IPAM.Config}}{{println .Subnet .Gateway}}{{end}}' "$NETWORK_NAME" 2>/dev/null | awk 'NF { print; exit }' || true
}

ensure_docker_network_exists() {
  NETWORK_NAME="$1"
  SUBNET_VALUE="$2"
  GATEWAY_VALUE="$3"

  if docker network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
    return 0
  fi

  docker network create --driver bridge --subnet "$SUBNET_VALUE" --gateway "$GATEWAY_VALUE" "$NETWORK_NAME" >/dev/null
}

ensure_docker_network_env() {
  NETWORK_NAME=${SA_DOCKER_NETWORK_NAME:-jusha-asr}
  case "$NETWORK_NAME" in
    ''|AUTO|auto)
      NETWORK_NAME=jusha-asr
      ;;
  esac

  CURRENT_SUBNET=${SA_DOCKER_SUBNET:-}
  EXISTING_CONFIG=""
  if [ -z "$CURRENT_SUBNET" ] || [ "$CURRENT_SUBNET" = "AUTO" ] || [ "$CURRENT_SUBNET" = "auto" ]; then
    EXISTING_CONFIG=$(existing_docker_network_config "$NETWORK_NAME")
  fi

  if [ -n "$EXISTING_CONFIG" ]; then
    SELECTED_SUBNET=$(printf '%s\n' "$EXISTING_CONFIG" | awk '{print $1}')
    SELECTED_GATEWAY=$(printf '%s\n' "$EXISTING_CONFIG" | awk '{print $2}')
    if [ -z "$SELECTED_GATEWAY" ]; then
      SELECTED_GATEWAY=$(cidr_gateway "$SELECTED_SUBNET")
    fi
  else
    if ! SELECTED_SUBNET=$(choose_docker_subnet "$NETWORK_NAME"); then
      echo "未能找到可用的 Docker 内部网段，请在 .env 中手动设置 SA_DOCKER_SUBNET。" >&2
      return 1
    fi
    SELECTED_GATEWAY=$(cidr_gateway "$SELECTED_SUBNET")
  fi

  update_env_value SA_DOCKER_NETWORK_NAME "$NETWORK_NAME" .env
  update_env_value SA_DOCKER_SUBNET "$SELECTED_SUBNET" .env
  update_env_value SA_DOCKER_GATEWAY "$SELECTED_GATEWAY" .env

  SA_DOCKER_NETWORK_NAME="$NETWORK_NAME"
  SA_DOCKER_SUBNET="$SELECTED_SUBNET"
  SA_DOCKER_GATEWAY="$SELECTED_GATEWAY"

  ensure_docker_network_matches_config "$SA_DOCKER_NETWORK_NAME" "$SA_DOCKER_SUBNET" "$SA_DOCKER_GATEWAY"
  ensure_docker_network_exists "$SA_DOCKER_NETWORK_NAME" "$SA_DOCKER_SUBNET" "$SA_DOCKER_GATEWAY"
  echo "Docker 内部网络: $SA_DOCKER_NETWORK_NAME ($SA_DOCKER_SUBNET, gateway $SA_DOCKER_GATEWAY)"
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
  NEED_RUNTIME_COMPOSE=0
  REMOVE_PULL_POLICY=0
  REMOVE_GPU_DEVICES=0
  RUNTIME_DEVICE=${SA_DEVICE:-${DEVICE:-cpu}}

  if [ "$COMPOSE_FLAVOR" = "v1" ] && grep -q '^[[:space:]]*pull_policy:' docker-compose.yml 2>/dev/null; then
    REMOVE_PULL_POLICY=1
    NEED_RUNTIME_COMPOSE=1
  fi

  case "$RUNTIME_DEVICE" in
    cuda|cuda:*) ;;
    *)
      if grep -q 'driver:[[:space:]]*nvidia' docker-compose.yml 2>/dev/null; then
        REMOVE_GPU_DEVICES=1
        NEED_RUNTIME_COMPOSE=1
      fi
      ;;
  esac

  if [ "$NEED_RUNTIME_COMPOSE" -eq 1 ]; then
    COMPOSE_FILE_PATH=.docker-compose.runtime.yml
    awk -v remove_pull="$REMOVE_PULL_POLICY" -v remove_gpu="$REMOVE_GPU_DEVICES" '
      function indent_of(line) {
        match(line, /^[[:space:]]*/)
        return RLENGTH
      }
      remove_pull == 1 && $1 == "pull_policy:" { next }
      remove_gpu == 1 && $0 ~ /^[[:space:]]*devices:[[:space:]]*$/ {
        skip = 1
        skip_indent = indent_of($0)
        next
      }
      skip == 1 {
        if ($0 ~ /^[[:space:]]*$/)
          next
        current_indent = indent_of($0)
        if (current_indent > skip_indent)
          next
        skip = 0
      }
      { print }
    ' docker-compose.yml > "$COMPOSE_FILE_PATH"
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

remove_legacy_compose_containers() {
  if [ "$COMPOSE_FLAVOR" != "v1" ]; then
    return 0
  fi

  CONTAINER_IDS=$(docker ps -aq --filter "name=${SA_CONTAINER_NAME}" 2>/dev/null || true)
  if [ -z "$CONTAINER_IDS" ]; then
    return 0
  fi

  echo "检测到 legacy docker-compose，先删除旧容器以规避 ContainerConfig 兼容问题..."
  printf '%s\n' "$CONTAINER_IDS" | while IFS= read -r CONTAINER_ID; do
    [ -n "$CONTAINER_ID" ] || continue
    docker rm -f "$CONTAINER_ID" >/dev/null 2>&1 || true
  done
}

compose_up() {
  ensure_docker_network_env
  if [ "$COMPOSE_FLAVOR" = "v2" ]; then
    compose -f "$COMPOSE_FILE_PATH" up -d --force-recreate --remove-orphans --no-build --pull never
  else
    remove_legacy_compose_containers
    compose -f "$COMPOSE_FILE_PATH" up -d --force-recreate --remove-orphans --no-build
  fi
}

cd "$SCRIPT_DIR"

validate_archive
mkdir -p data logs config models backups

if [ -f .env.example ] && [ ! -f .env ]; then
  cp .env.example .env
fi

update_env_value SA_IMAGE "$RELEASE_IMAGE" .env
update_env_value SA_RELEASE_VERSION "$RELEASE_VERSION" .env

# shellcheck disable=SC1091
. ./.env

SA_IMAGE=${SA_IMAGE:-$RELEASE_IMAGE}
SA_CONTAINER_NAME=${SA_CONTAINER_NAME:-jusha-asr-speaker}
SA_PORT=${SA_PORT:-9852}

prepare_compose_file
validate_models

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
