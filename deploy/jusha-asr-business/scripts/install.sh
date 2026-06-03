#!/bin/sh
set -eu

ACTION=${1:-install}
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
IMAGE_ARCHIVE="$SCRIPT_DIR/image/jusha-asr-business-image.tar.gz"
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
RELEASE_IMAGE=jusha-asr-business:latest
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

backup_optional_file() {
  SRC_PATH="$1"
  DEST_PATH="$2"
  LABEL="$3"

  if [ ! -e "$SRC_PATH" ]; then
    return 0
  fi

  if [ ! -r "$SRC_PATH" ]; then
    echo "警告: 跳过备份 ${LABEL}，当前用户无权读取 $SRC_PATH；安装不会覆盖现有文件。" >&2
    return 0
  fi

  cp "$SRC_PATH" "$DEST_PATH"
}

canonical_path() {
  TARGET_PATH="$1"
  if [ -d "$TARGET_PATH" ]; then
    CDPATH= cd -- "$TARGET_PATH" && pwd -P
    return 0
  fi

  TARGET_PARENT=$(dirname "$TARGET_PATH")
  TARGET_NAME=$(basename "$TARGET_PATH")
  if [ -d "$TARGET_PARENT" ]; then
    PARENT_REAL=$(CDPATH= cd -- "$TARGET_PARENT" && pwd -P)
    printf '%s/%s\n' "$PARENT_REAL" "$TARGET_NAME"
    return 0
  fi

  printf '%s\n' "$TARGET_PATH"
}

container_mount_source() {
  CONTAINER_NAME_VALUE="$1"
  DESTINATION_PATH="$2"

  docker inspect -f '{{range .Mounts}}{{.Destination}}{{printf "\t"}}{{.Source}}{{printf "\n"}}{{end}}' "$CONTAINER_NAME_VALUE" 2>/dev/null |
    awk -F '\t' -v destination="$DESTINATION_PATH" '$1 == destination { print $2; exit }'
}

assert_existing_runtime_matches_install_dir() {
  if [ "${ASR_INSTALL_ALLOW_RUNTIME_SWITCH:-0}" = "1" ]; then
    echo "警告: 已设置 ASR_INSTALL_ALLOW_RUNTIME_SWITCH=1，跳过已有容器 runtime 挂载一致性检查。" >&2
    return 0
  fi

  if ! docker container inspect "$ASR_CONTAINER_NAME" >/dev/null 2>&1; then
    return 0
  fi

  for MOUNT_PAIR in \
    "/var/lib/asr/mysql:runtime/mysql:MySQL 数据目录" \
    "/var/lib/asr/uploads:runtime/uploads:上传文件目录" \
    "/var/lib/asr/term-catalog:runtime/term-catalog:影像术语库目录" \
    "/var/log/asr:runtime/logs:日志目录"
  do
    CONTAINER_DEST=$(printf '%s\n' "$MOUNT_PAIR" | cut -d: -f1)
    LOCAL_REL=$(printf '%s\n' "$MOUNT_PAIR" | cut -d: -f2)
    LABEL=$(printf '%s\n' "$MOUNT_PAIR" | cut -d: -f3)
    CURRENT_SOURCE=$(container_mount_source "$ASR_CONTAINER_NAME" "$CONTAINER_DEST" || true)
    if [ -z "$CURRENT_SOURCE" ]; then
      continue
    fi

    EXPECTED_SOURCE=$(canonical_path "$SCRIPT_DIR/$LOCAL_REL")
    CURRENT_SOURCE_REAL=$(canonical_path "$CURRENT_SOURCE")
    if [ "$CURRENT_SOURCE_REAL" = "$EXPECTED_SOURCE" ]; then
      continue
    fi

    echo "检测到已有容器 $ASR_CONTAINER_NAME 使用的 ${LABEL} 不属于当前安装目录，已拒绝继续升级。" >&2
    echo "  容器当前挂载: $CONTAINER_DEST -> $CURRENT_SOURCE_REAL" >&2
    echo "  当前安装目录: $EXPECTED_SOURCE" >&2
    echo "请在旧安装目录的父目录执行新的 .run，或设置 ASR_RUN_TARGET_DIR 指向旧安装目录的父目录。" >&2
    echo "如果你已经完整迁移 runtime 数据并确认要切换目录，可显式设置 ASR_INSTALL_ALLOW_RUNTIME_SWITCH=1 后重试。" >&2
    exit 1
  done
}

path_has_content() {
  TARGET_PATH="$1"
  [ -d "$TARGET_PATH" ] && [ -n "$(find "$TARGET_PATH" -mindepth 1 -print -quit 2>/dev/null)" ]
}

backup_directory_archive() {
  SRC_DIR="$1"
  ARCHIVE_PATH="$2"
  LABEL="$3"

  if ! path_has_content "$SRC_DIR"; then
    return 0
  fi

  SRC_PARENT=$(dirname "$SRC_DIR")
  SRC_NAME=$(basename "$SRC_DIR")
  if tar -czf "$ARCHIVE_PATH" -C "$SRC_PARENT" "$SRC_NAME"; then
    echo "已备份 ${LABEL}: $ARCHIVE_PATH"
    return 0
  fi

  echo "备份 ${LABEL} 失败: $SRC_DIR" >&2
  return 1
}

backup_mysql_before_upgrade() {
  if [ "${ASR_INSTALL_SKIP_DATA_BACKUP:-0}" = "1" ]; then
    echo "警告: 已设置 ASR_INSTALL_SKIP_DATA_BACKUP=1，跳过升级前 MySQL 备份。" >&2
    return 0
  fi

  if docker container inspect "$ASR_CONTAINER_NAME" >/dev/null 2>&1; then
    CONTAINER_STATE=$(docker inspect -f '{{.State.Status}}' "$ASR_CONTAINER_NAME" 2>/dev/null || true)
    if [ "$CONTAINER_STATE" = "running" ]; then
      DUMP_PATH="$BACKUP_DIR/mysql-${ASR_MYSQL_DATABASE:-asr}.sql"
      echo "正在备份 MySQL 数据库到: ${DUMP_PATH}.gz"
      if docker exec "$ASR_CONTAINER_NAME" sh -lc 'mysqldump --single-transaction --quick --routines --triggers -uroot -p"$ASR_MYSQL_ROOT_PASSWORD" "$ASR_MYSQL_DATABASE"' > "$DUMP_PATH"; then
        gzip -f "$DUMP_PATH"
        return 0
      fi
      rm -f "$DUMP_PATH"
      echo "MySQL 逻辑备份失败，已中止升级以保护现有数据。" >&2
      echo "确认已有外部备份后，可设置 ASR_INSTALL_SKIP_DATA_BACKUP=1 强制跳过。" >&2
      exit 1
    fi
  fi

  if backup_directory_archive "$SCRIPT_DIR/runtime/mysql" "$BACKUP_DIR/runtime-mysql.tar.gz" "MySQL 数据目录"; then
    return 0
  fi

  echo "MySQL 数据目录备份失败，已中止升级以保护现有数据。" >&2
  echo "确认已有外部备份后，可设置 ASR_INSTALL_SKIP_DATA_BACKUP=1 强制跳过。" >&2
  exit 1
}

backup_runtime_data_before_upgrade() {
  backup_mysql_before_upgrade
  if ! backup_directory_archive "$SCRIPT_DIR/runtime/term-catalog" "$BACKUP_DIR/runtime-term-catalog.tar.gz" "影像术语库目录"; then
    echo "影像术语库目录备份失败，已中止升级以保护现有数据。" >&2
    echo "确认已有外部备份后，可设置 ASR_INSTALL_SKIP_DATA_BACKUP=1 强制跳过。" >&2
    exit 1
  fi
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
  HTTP_PORT_VALUE=${ASR_HTTP_PORT:-9855}
  HTTPS_PORT_VALUE=${ASR_HTTPS_PORT:-9856}
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

print_hardware_requirements() {
  EDITION=${ASR_PRODUCT_EDITION:-${PRODUCT_EDITION:-standard}}
  case "$EDITION" in
    advanced)
      echo "硬件要求（高级版）: 最低 CPU 16核 / 内存 32G / 存储 500G SSD / 算力 A10；推荐 A100。"
      ;;
    *)
      echo "硬件要求（标准版）: 最低 CPU 8核 / 内存 16G / 存储 200G SSD / 算力 RTX 3090；推荐 CPU 16核 / 内存 32G / 存储 500G SSD / 算力 A10 或 A100。"
      ;;
  esac
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

docker_subnet_candidates() {
  if [ -n "${ASR_DOCKER_SUBNET_CANDIDATES:-}" ]; then
    printf '%s\n' "$ASR_DOCKER_SUBNET_CANDIDATES" | tr ',;' '\n' | awk 'NF { print $1 }'
    return 0
  fi

  cat <<'EOF'
10.248.0.0/24
10.249.0.0/24
10.250.0.0/24
10.251.0.0/24
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
  CURRENT_SUBNET=${ASR_DOCKER_SUBNET:-}
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
    FOREIGN_CONTAINER=$(printf '%s\n' "$ATTACHED_CONTAINERS" | awk -v own="$ASR_CONTAINER_NAME" '$0 != own { print; exit }')
    if [ -n "$FOREIGN_CONTAINER" ]; then
      echo "Docker 网络 $NETWORK_NAME 已存在但网段配置不一致，且仍有非本应用容器连接: $FOREIGN_CONTAINER" >&2
      echo "请先处理该网络，或在 .env 中设置 ASR_DOCKER_NETWORK_NAME 为新的网络名。" >&2
      return 1
    fi

    echo "Docker 网络 $NETWORK_NAME 的网段配置需要切换，先停止旧实例..."
    sh -c "$COMPOSE_CMD -f docker-compose.yml down --remove-orphans" || true
    docker rm -f "$ASR_CONTAINER_NAME" >/dev/null 2>&1 || true
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
  NETWORK_NAME=${ASR_DOCKER_NETWORK_NAME:-jusha-asr}
  case "$NETWORK_NAME" in
    ''|AUTO|auto)
      NETWORK_NAME=jusha-asr
      ;;
  esac

  CURRENT_SUBNET=${ASR_DOCKER_SUBNET:-}
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
      echo "未能找到可用的 Docker 内部网段，请在 .env 中手动设置 ASR_DOCKER_SUBNET。" >&2
      return 1
    fi
    SELECTED_GATEWAY=$(cidr_gateway "$SELECTED_SUBNET")
  fi

  update_env_value ASR_DOCKER_NETWORK_NAME "$NETWORK_NAME" .env
  update_env_value ASR_DOCKER_SUBNET "$SELECTED_SUBNET" .env
  update_env_value ASR_DOCKER_GATEWAY "$SELECTED_GATEWAY" .env

  ASR_DOCKER_NETWORK_NAME="$NETWORK_NAME"
  ASR_DOCKER_SUBNET="$SELECTED_SUBNET"
  ASR_DOCKER_GATEWAY="$SELECTED_GATEWAY"

  ensure_docker_network_matches_config "$ASR_DOCKER_NETWORK_NAME" "$ASR_DOCKER_SUBNET" "$ASR_DOCKER_GATEWAY"
  ensure_docker_network_exists "$ASR_DOCKER_NETWORK_NAME" "$ASR_DOCKER_SUBNET" "$ASR_DOCKER_GATEWAY"
  echo "Docker 内部网络: $ASR_DOCKER_NETWORK_NAME ($ASR_DOCKER_SUBNET, gateway $ASR_DOCKER_GATEWAY)"
}

compose_up() {
  ensure_docker_network_env
  sh -c "$COMPOSE_CMD -f docker-compose.yml up -d --force-recreate --remove-orphans"
}

wait_for_service_health() {
  CONTAINER_NAME="$1"
  ATTEMPTS=${2:-90}

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
      unhealthy)
        if docker exec "$CONTAINER_NAME" sh -lc '/app/scripts/healthcheck.sh' >/tmp/asr-manual-healthcheck.log 2>&1; then
          echo "Docker 健康状态为 $STATUS，但容器内部健康检查已通过。"
          rm -f /tmp/asr-manual-healthcheck.log
          return 0
        fi
        ;;
      exited|dead)
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
  compose_up
  wait_for_service_health "$ASR_CONTAINER_NAME" 60
}

cd "$SCRIPT_DIR"

mkdir -p runtime/mysql runtime/certs runtime/downloads runtime/tmp runtime/uploads runtime/term-catalog runtime/logs
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

ASR_CONTAINER_NAME=${ASR_CONTAINER_NAME:-jusha-asr-business}
ensure_docker_network_env

# shellcheck disable=SC1091
. ./.env

ASR_CONTAINER_NAME=${ASR_CONTAINER_NAME:-jusha-asr-business}
assert_existing_runtime_matches_install_dir

BACKUP_DIR="$SCRIPT_DIR/backups/$(date +%Y%m%d%H%M%S)"
mkdir -p "$BACKUP_DIR"
[ -f .env ] && cp .env "$BACKUP_DIR/.env"
[ -f docker-compose.yml ] && cp docker-compose.yml "$BACKUP_DIR/docker-compose.yml"
[ -f .release-manifest ] && cp .release-manifest "$BACKUP_DIR/.release-manifest"
backup_optional_file runtime/certs/tls.crt "$BACKUP_DIR/tls.crt" "现有 TLS 证书"
backup_optional_file runtime/certs/tls.key "$BACKUP_DIR/tls.key" "现有 TLS 私钥"

CURRENT_IMAGE=""
if docker container inspect "$ASR_CONTAINER_NAME" >/dev/null 2>&1; then
  CURRENT_IMAGE=$(docker inspect -f '{{.Config.Image}}' "$ASR_CONTAINER_NAME" 2>/dev/null || true)
fi

backup_runtime_data_before_upgrade

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
compose_up

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
print_hardware_requirements
print_access_summary
