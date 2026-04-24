#!/bin/sh
set -eu

ACTION=${1:-install}
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
IMAGE_ARCHIVE="$SCRIPT_DIR/image/asr-all-in-one-image.tar.gz"
MANIFEST_FILE="$SCRIPT_DIR/.release-manifest"

case "$ACTION" in
  install|upgrade)
    ;;
  *)
    echo "用法: install.sh [install|upgrade]" >&2
    exit 1
    ;;
esac

if ! command -v docker >/dev/null 2>&1; then
  echo "docker 未安装，无法继续安装" >&2
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

RELEASE_VERSION=unknown
RELEASE_IMAGE=asr-all-in-one:latest
if [ -f "$MANIFEST_FILE" ]; then
  # shellcheck disable=SC1090
  . "$MANIFEST_FILE"
fi

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

detect_primary_ip() {
  if command -v hostname >/dev/null 2>&1; then
    hostname -I 2>/dev/null | awk '{print $1}'
  fi
}

get_certificate_san() {
  CERT_FILE_PATH="$1"

  if [ ! -f "$CERT_FILE_PATH" ]; then
    return 1
  fi

  if command -v openssl >/dev/null 2>&1; then
    openssl x509 -in "$CERT_FILE_PATH" -noout -ext subjectAltName 2>/dev/null | sed '1d' | tr -d '\n' | sed 's/^ *//;s/ *$//'
    return 0
  fi

  if docker exec "$ASR_CONTAINER_NAME" sh -lc "openssl x509 -in /var/lib/asr/certs/tls.crt -noout -ext subjectAltName 2>/dev/null" >/tmp/asr-cert-san.txt 2>/dev/null; then
    sed '1d' /tmp/asr-cert-san.txt | tr -d '\n' | sed 's/^ *//;s/ *$//'
    rm -f /tmp/asr-cert-san.txt
    return 0
  fi

  rm -f /tmp/asr-cert-san.txt
  return 1
}

build_https_url() {
  HOST_VALUE="$1"
  PORT_VALUE="$2"

  if [ -z "$HOST_VALUE" ]; then
    return 1
  fi

  if [ "$PORT_VALUE" = "443" ]; then
    printf 'https://%s' "$HOST_VALUE"
  else
    printf 'https://%s:%s' "$HOST_VALUE" "$PORT_VALUE"
  fi
}

build_http_url() {
  HOST_VALUE="$1"
  PORT_VALUE="$2"

  if [ -z "$HOST_VALUE" ]; then
    return 1
  fi

  if [ "$PORT_VALUE" = "80" ]; then
    printf 'http://%s' "$HOST_VALUE"
  else
    printf 'http://%s:%s' "$HOST_VALUE" "$PORT_VALUE"
  fi
}

print_access_summary() {
  PRIMARY_IP=$(detect_primary_ip || true)
  HOST_NAME=$(hostname 2>/dev/null || printf '')
  HTTPS_ENABLED=${ASR_ENABLE_HTTPS:-1}
  HTTP_PORT_VALUE=${ASR_HTTP_PORT:-80}
  HTTPS_PORT_VALUE=${ASR_HTTPS_PORT:-443}
  CERT_PATH="$SCRIPT_DIR/runtime/certs/tls.crt"
  CERT_SAN=$(get_certificate_san "$CERT_PATH" || true)

  if [ "$HTTPS_ENABLED" = "1" ]; then
    echo "证书 SAN: ${CERT_SAN:-未能解析，请检查 runtime/certs/tls.crt}"
  fi
  echo "访问地址:"
  if URL=$(build_http_url localhost "$HTTP_PORT_VALUE" 2>/dev/null); then
    echo "  桌面客户端/普通 HTTP: $URL"
  fi
  if [ "$HTTPS_ENABLED" = "1" ] && URL=$(build_https_url localhost "$HTTPS_PORT_VALUE" 2>/dev/null); then
    echo "  浏览器 HTTPS 下载页: $URL/downloads"
    echo "  浏览器 HTTPS 登录页: $URL/login"
  fi
  if [ -n "$HOST_NAME" ]; then
    if URL=$(build_http_url "$HOST_NAME" "$HTTP_PORT_VALUE" 2>/dev/null); then
      echo "  桌面客户端/普通 HTTP: $URL"
    fi
    if [ "$HTTPS_ENABLED" = "1" ] && URL=$(build_https_url "$HOST_NAME" "$HTTPS_PORT_VALUE" 2>/dev/null); then
      echo "  浏览器 HTTPS 下载页: $URL/downloads"
      echo "  浏览器 HTTPS 登录页: $URL/login"
    fi
  fi
  if [ -n "$PRIMARY_IP" ]; then
    if URL=$(build_http_url "$PRIMARY_IP" "$HTTP_PORT_VALUE" 2>/dev/null); then
      echo "  桌面客户端/普通 HTTP: $URL"
    fi
    if [ "$HTTPS_ENABLED" = "1" ] && URL=$(build_https_url "$PRIMARY_IP" "$HTTPS_PORT_VALUE" 2>/dev/null); then
      echo "  浏览器 HTTPS 下载页: $URL/downloads"
      echo "  浏览器 HTTPS 登录页: $URL/login"
    fi
  fi

  if [ "$HTTPS_ENABLED" = "1" ]; then
    echo "浏览器导入提示:"
    echo "  证书文件位置: $CERT_PATH"
    echo "  Windows Chrome/Edge: 双击 tls.crt -> 安装证书 -> 本地计算机 -> 将所有的证书都放入下列存储 -> 受信任的根证书颁发机构。"
    echo "  Firefox: 设置 -> 隐私与安全 -> 证书 -> 查看证书 -> 导入 -> 选择 tls.crt，并勾选信任此 CA 标识网站。"
    echo "  导入后请重新打开浏览器，再访问上面的 HTTPS 地址。"
  fi
}

ensure_tls_env_defaults() {
  PRIMARY_IP=$(detect_primary_ip || true)
  HOST_NAME=$(hostname 2>/dev/null || printf 'localhost')

  ASR_ENABLE_HTTPS_VALUE=${ASR_ENABLE_HTTPS:-1}
  ASR_TLS_COMMON_NAME_VALUE=${ASR_TLS_COMMON_NAME:-}
  ASR_TLS_ALT_NAMES_VALUE=${ASR_TLS_ALT_NAMES:-AUTO}

  if [ -z "$ASR_TLS_COMMON_NAME_VALUE" ] || [ "$ASR_TLS_COMMON_NAME_VALUE" = "localhost" ]; then
    if [ -n "$PRIMARY_IP" ]; then
      ASR_TLS_COMMON_NAME_VALUE="$PRIMARY_IP"
    else
      ASR_TLS_COMMON_NAME_VALUE="$HOST_NAME"
    fi
  fi

  if [ -z "$ASR_TLS_ALT_NAMES_VALUE" ] || [ "$ASR_TLS_ALT_NAMES_VALUE" = "AUTO" ]; then
    ASR_TLS_ALT_NAMES_VALUE="DNS:localhost,DNS:${HOST_NAME},IP:127.0.0.1"
    if [ -n "$PRIMARY_IP" ]; then
      ASR_TLS_ALT_NAMES_VALUE="$ASR_TLS_ALT_NAMES_VALUE,IP:${PRIMARY_IP}"
    fi
  fi

  update_env_value ASR_ENABLE_HTTPS "$ASR_ENABLE_HTTPS_VALUE" .env
  update_env_value ASR_HTTP_REDIRECT_TO_HTTPS "${ASR_HTTP_REDIRECT_TO_HTTPS:-0}" .env
  update_env_value ASR_TLS_COMMON_NAME "$ASR_TLS_COMMON_NAME_VALUE" .env
  update_env_value ASR_TLS_ALT_NAMES "$ASR_TLS_ALT_NAMES_VALUE" .env
}

wait_for_service_health() {
  CONTAINER_NAME="$1"
  ATTEMPTS=${2:-60}

  INDEX=0
  LAST_STATUS=""
  MISSING_CONTAINER_REPORTED=0
  while [ "$INDEX" -lt "$ATTEMPTS" ]; do
    STATUS=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$CONTAINER_NAME" 2>/dev/null || true)
    if [ -z "$STATUS" ] && [ "$MISSING_CONTAINER_REPORTED" -eq 0 ]; then
      echo "当前尚未发现容器 $CONTAINER_NAME，或健康状态还不可读。"
      MISSING_CONTAINER_REPORTED=1
    fi
    if [ -n "$STATUS" ] && [ "$STATUS" != "$LAST_STATUS" ]; then
      echo "当前容器状态: $STATUS"
      LAST_STATUS="$STATUS"
    fi
    case "$STATUS" in
      healthy|running)
        if docker exec "$CONTAINER_NAME" sh -lc '/app/scripts/healthcheck.sh' >/tmp/asr-manual-healthcheck.log 2>&1; then
          [ "$STATUS" = "running" ] && echo "容器内部健康检查已通过。"
          rm -f /tmp/asr-manual-healthcheck.log
          return 0
        fi
        ;;
      starting)
        if docker exec "$CONTAINER_NAME" sh -lc '/app/scripts/healthcheck.sh' >/tmp/asr-manual-healthcheck.log 2>&1; then
          echo "容器仍处于 starting，但内部健康检查已通过。"
          rm -f /tmp/asr-manual-healthcheck.log
          return 0
        fi
        ;;
      unhealthy|exited|dead)
        if docker exec "$CONTAINER_NAME" sh -lc '/app/scripts/healthcheck.sh' >/tmp/asr-manual-healthcheck.log 2>&1; then
          echo "Docker 健康状态为 $STATUS，但容器内部健康检查已通过。"
          rm -f /tmp/asr-manual-healthcheck.log
          return 0
        fi
        return 1
        ;;
    esac
    INDEX=$((INDEX + 1))
    sleep 5
  done

  if docker exec "$CONTAINER_NAME" sh -lc '/app/scripts/healthcheck.sh' >/tmp/asr-manual-healthcheck.log 2>&1; then
    echo "等待超时，但容器内部健康检查已通过。"
    rm -f /tmp/asr-manual-healthcheck.log
    return 0
  fi

  if [ -f /tmp/asr-manual-healthcheck.log ]; then
    echo "最后一次容器内部健康检查输出:"
    cat /tmp/asr-manual-healthcheck.log || true
    rm -f /tmp/asr-manual-healthcheck.log
  fi

  return 1
}

print_container_diagnostics() {
  CONTAINER_NAME="$1"

  if ! docker container inspect "$CONTAINER_NAME" >/dev/null 2>&1; then
    echo "容器诊断信息: 未找到容器 $CONTAINER_NAME" 
    echo "当前 Docker 容器列表:"
    docker ps -a --format '  {{.Names}}  status={{.Status}}  image={{.Image}}' || true
    return 0
  fi

  echo "容器诊断信息:"
  docker inspect -f '  state={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}} exit={{.State.ExitCode}} startedAt={{.State.StartedAt}}' "$CONTAINER_NAME" 2>/dev/null || true

  echo "最近的健康检查记录:"
  docker inspect -f '{{if .State.Health}}{{range .State.Health.Log}}---\nend={{.End}}\nexit={{.ExitCode}}\noutput={{.Output}}{{end}}{{else}}no healthcheck logs available{{end}}' "$CONTAINER_NAME" 2>/dev/null || true

  echo "最近 200 行容器日志:"
  docker logs --tail 200 "$CONTAINER_NAME" 2>&1 || true
}

rollback_previous_release() {
  PREVIOUS_IMAGE="$1"
  if [ -z "$PREVIOUS_IMAGE" ]; then
    return 1
  fi

  echo "升级后健康检查失败，开始回滚到 $PREVIOUS_IMAGE"
  update_env_value ASR_RELEASE_IMAGE "$PREVIOUS_IMAGE" .env
  sh -c "$COMPOSE_CMD -f docker-compose.yml up -d --force-recreate --remove-orphans"
  wait_for_service_health "$ASR_CONTAINER_NAME" 60
}

cd "$SCRIPT_DIR"

mkdir -p runtime/mysql runtime/certs runtime/downloads runtime/tmp runtime/uploads
chmod 1777 runtime/tmp

ENV_CREATED=0
if [ -f .env.example ] && [ ! -f .env ]; then
  cp .env.example .env
  ENV_CREATED=1
fi

if [ "$ENV_CREATED" = "1" ]; then
  echo "已生成 .env，请先修改后再重新执行 install.sh。" >&2
  echo "重要: MySQL root 密码会在首次初始化 runtime/mysql 时写入数据目录。" >&2
  echo "如果直接用默认值继续安装，后续再改 .env 密码会导致数据库鉴权失败。" >&2
  exit 1
fi

update_env_value ASR_RELEASE_IMAGE "$RELEASE_IMAGE" .env
update_env_value ASR_RELEASE_VERSION "$RELEASE_VERSION" .env
ensure_tls_env_defaults

# shellcheck disable=SC1091
. ./.env

ASR_CONTAINER_NAME=${ASR_CONTAINER_NAME:-asr-all-in-one}

BACKUP_DIR="$SCRIPT_DIR/backups/$(date +%Y%m%d%H%M%S)"
mkdir -p "$BACKUP_DIR"
[ -f .env ] && cp .env "$BACKUP_DIR/.env"
[ -f docker-compose.yml ] && cp docker-compose.yml "$BACKUP_DIR/docker-compose.yml"
[ -f .release-manifest ] && cp .release-manifest "$BACKUP_DIR/.release-manifest"
[ -f runtime/certs/tls.crt ] && cp runtime/certs/tls.crt "$BACKUP_DIR/tls.crt"
[ -f runtime/certs/tls.key ] && cp runtime/certs/tls.key "$BACKUP_DIR/tls.key"

CURRENT_IMAGE=""
if docker container inspect "$ASR_CONTAINER_NAME" >/dev/null 2>&1; then
  CURRENT_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$ASR_CONTAINER_NAME" 2>/dev/null || true)
fi

if [ ! -f "$IMAGE_ARCHIVE" ]; then
  echo "缺少离线镜像包: $IMAGE_ARCHIVE" >&2
  exit 1
fi

echo "加载离线镜像..."
gzip -dc "$IMAGE_ARCHIVE" | docker load

if [ -n "$CURRENT_IMAGE" ]; then
  echo "检测到已有实例，准备从 $CURRENT_IMAGE 升级到 $RELEASE_IMAGE"
else
  echo "未检测到已有实例，执行首次安装"
fi

echo "启动服务..."
sh -c "$COMPOSE_CMD -f docker-compose.yml up -d --force-recreate --remove-orphans"

echo "等待服务健康检查..."
if ! wait_for_service_health "$ASR_CONTAINER_NAME" 60; then
  print_container_diagnostics "$ASR_CONTAINER_NAME"
  if [ -n "$CURRENT_IMAGE" ] && rollback_previous_release "$CURRENT_IMAGE"; then
    echo "已回滚到上一版本: $CURRENT_IMAGE" >&2
  else
    echo "安装或升级后的服务未通过健康检查，请检查容器日志。" >&2
  fi
  exit 1
fi

echo "安装完成。"
echo "版本: $RELEASE_VERSION"
echo "镜像: $RELEASE_IMAGE"
echo "备份目录: $BACKUP_DIR"
echo "注意: 默认启用自签名 HTTPS，浏览器首次访问会提示证书不受信任；接受证书后即可正常使用网页端实时录音。"
print_access_summary