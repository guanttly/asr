#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
DEPLOY_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
REPO_ROOT=$(CDPATH= cd -- "$DEPLOY_DIR/../.." && pwd)
DESKTOP_DIR="$REPO_ROOT/desktop"
DESKTOP_INSTALLER_DIR="$DESKTOP_DIR/src-tauri/target/x86_64-pc-windows-msvc/release/bundle/nsis"

VERSION=""
OUTPUT_ROOT="$DEPLOY_DIR/dist"
SKIP_DOCKER=0
SERVER_HOST="localhost"
HTTP_PORT="80"
HTTP_PORT_EXPLICIT=0
HTTPS_PORT=""
ADMIN_USERNAME="admin"
ADMIN_PASSWORD="change-me-now"
ADMIN_DISPLAY_NAME="系统管理员"
MYSQL_PASSWORD=""
JWT_SECRET=""
ASR_SERVICE_URL="http://host.docker.internal:8000"
DESKTOP_INSTALLER_OVERRIDE=""
DESKTOP_PACKAGE_OWNER=""
DESKTOP_CARGO_TOML_OWNER=""
DESKTOP_CARGO_LOCK_OWNER=""

append_path_if_dir() {
  DIR_PATH="$1"
  if [ ! -d "$DIR_PATH" ]; then
    return 0
  fi
  case ":$PATH:" in
    *":$DIR_PATH:"*)
      ;;
    *)
      PATH="$DIR_PATH:$PATH"
      ;;
  esac
}

append_latest_nvm_bin() {
  HOME_DIR="$1"
  NVM_ROOT="$HOME_DIR/.nvm/versions/node"
  if [ ! -d "$NVM_ROOT" ]; then
    return 0
  fi

  LATEST_NVM_BIN=$(find "$NVM_ROOT" -mindepth 2 -maxdepth 2 -type d -name bin 2>/dev/null | sort -V | tail -n 1)
  if [ -n "$LATEST_NVM_BIN" ]; then
    append_path_if_dir "$LATEST_NVM_BIN"
  fi
}

bootstrap_node_path() {
  append_path_if_dir "$HOME/.local/share/pnpm"
  append_path_if_dir "$HOME/.local/bin"
  append_latest_nvm_bin "$HOME"

  if [ -n "${SUDO_USER:-}" ]; then
    SUDO_HOME=$(getent passwd "$SUDO_USER" 2>/dev/null | cut -d: -f6)
    if [ -n "$SUDO_HOME" ] && [ "$SUDO_HOME" != "$HOME" ]; then
      append_path_if_dir "$SUDO_HOME/.local/share/pnpm"
      append_path_if_dir "$SUDO_HOME/.local/bin"
      append_latest_nvm_bin "$SUDO_HOME"
    fi
  fi
}

run_pnpm() {
  if command -v pnpm >/dev/null 2>&1; then
    pnpm "$@"
    return 0
  fi
  if command -v corepack >/dev/null 2>&1 && command -v node >/dev/null 2>&1; then
    corepack pnpm "$@"
    return 0
  fi
  return 127
}

bootstrap_node_path

usage() {
  cat <<EOF
用法: build-release.sh [选项]

选项:
  --version <version> | version <version>
                                     发布版本号，默认读取 desktop/package.json
  --output-dir <dir> | output-dir <dir>
                                     输出目录，默认 deploy/all-in-one/dist
  --server-host <host> | server-host <host>
                                     服务器 IP 或域名，用于客户端默认地址和 TLS 证书
  --http-port <port> | http-port <port>
                                     HTTP 对外端口，默认 80；若显式传入且未传 --https-port，则 HTTPS 自动取 http+1
  --https-port <port> | https-port <port>
                                     HTTPS 对外端口，默认 443
  --admin-username <username> | admin-username <username>
                                     默认管理员用户名，默认 admin
  --admin-password <password> | admin-password <password>
                                     默认管理员密码，默认 change-me-now
  --admin-display-name <name> | admin-display-name <name>
                                     默认管理员显示名，默认 系统管理员
  --mysql-password <password> | mysql-password <password>
                                     MySQL root 密码，默认自动生成
  --jwt-secret <secret> | jwt-secret <secret>
                                     JWT 密钥，默认自动生成
  --asr-service-url <url> | asr-service-url <url>
                                     外部 ASR 服务地址，默认 http://host.docker.internal:8000
  --desktop-installer <path> | desktop-installer <path>
                                     直接使用现成桌面端安装包，不自动构建
  --dry-run | dry-run               跳过 Docker 镜像构建和桌面端自动构建
  -h, --help                        显示帮助
EOF
}

normalize_server_host() {
  VALUE=$(printf '%s' "$1" | sed 's#^https\?://##; s#/.*$##')
  printf '%s' "$VALUE"
}

is_ipv4_address() {
  case "$1" in
    ''|*[!0-9.]* )
      return 1
      ;;
    *.*.*.*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

validate_port() {
  VALUE="$1"
  NAME="$2"
  case "$VALUE" in
    ''|*[!0-9]*)
      echo "$NAME 必须是纯数字端口，当前值: $VALUE" >&2
      exit 1
      ;;
  esac
  if [ "$VALUE" -lt 1 ] || [ "$VALUE" -gt 65535 ]; then
    echo "$NAME 超出有效端口范围: $VALUE" >&2
    exit 1
  fi
}

generate_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
    return
  fi
  date +%s | sha256sum | awk '{print $1}'
}

build_https_origin() {
  HOST="$1"
  PORT="$2"
  if [ "$PORT" = "443" ]; then
    printf 'https://%s' "$HOST"
  else
    printf 'https://%s:%s' "$HOST" "$PORT"
  fi
}

build_http_origin() {
  HOST="$1"
  PORT="$2"
  if [ "$PORT" = "80" ]; then
    printf 'http://%s' "$HOST"
  else
    printf 'http://%s:%s' "$HOST" "$PORT"
  fi
}

build_tls_alt_names() {
  HOST="$1"
  if [ "$HOST" = "localhost" ]; then
    printf 'DNS:localhost,IP:127.0.0.1'
    return
  fi
  if is_ipv4_address "$HOST"; then
    printf 'DNS:localhost,IP:127.0.0.1,IP:%s' "$HOST"
  else
    printf 'DNS:localhost,DNS:%s,IP:127.0.0.1' "$HOST"
  fi
}

find_desktop_installer_for_version() {
  if [ -n "$VERSION" ] && ls "$DESKTOP_INSTALLER_DIR"/*"$VERSION"*setup.exe >/dev/null 2>&1; then
    ls -t "$DESKTOP_INSTALLER_DIR"/*"$VERSION"*setup.exe | head -n 1
    return 0
  fi
  return 1
}

find_latest_desktop_installer() {
  if ls "$DESKTOP_INSTALLER_DIR"/*setup.exe >/dev/null 2>&1; then
    ls -t "$DESKTOP_INSTALLER_DIR"/*setup.exe | head -n 1
    return 0
  fi
  return 1
}

find_desktop_installer() {
  find_desktop_installer_for_version || find_latest_desktop_installer
}

maybe_restore_owner() {
  OWNER="$1"
  TARGET_PATH="$2"
  if [ -n "$OWNER" ] && [ -e "$TARGET_PATH" ]; then
    chown "$OWNER" "$TARGET_PATH"
  fi
}

restore_desktop_version_files() {
  if [ -z "${DESKTOP_VERSION_BACKUP_DIR:-}" ] || [ ! -d "$DESKTOP_VERSION_BACKUP_DIR" ]; then
    return 0
  fi

  cp "$DESKTOP_VERSION_BACKUP_DIR/package.json" "$DESKTOP_DIR/package.json"
  cp "$DESKTOP_VERSION_BACKUP_DIR/Cargo.toml" "$DESKTOP_DIR/src-tauri/Cargo.toml"
  if [ -f "$DESKTOP_VERSION_BACKUP_DIR/Cargo.lock" ]; then
    cp "$DESKTOP_VERSION_BACKUP_DIR/Cargo.lock" "$DESKTOP_DIR/src-tauri/Cargo.lock"
  fi
  maybe_restore_owner "$DESKTOP_PACKAGE_OWNER" "$DESKTOP_DIR/package.json"
  maybe_restore_owner "$DESKTOP_CARGO_TOML_OWNER" "$DESKTOP_DIR/src-tauri/Cargo.toml"
  maybe_restore_owner "$DESKTOP_CARGO_LOCK_OWNER" "$DESKTOP_DIR/src-tauri/Cargo.lock"
  rm -rf "$DESKTOP_VERSION_BACKUP_DIR"
  DESKTOP_VERSION_BACKUP_DIR=""
}

prepare_desktop_version_files() {
  TARGET_VERSION="$1"

  if ! command -v node >/dev/null 2>&1; then
    echo "未找到 node，无法在构建前同步桌面端版本号。" >&2
    exit 1
  fi

  DESKTOP_VERSION_BACKUP_DIR=$(mktemp -d)
  DESKTOP_PACKAGE_OWNER=$(stat -c '%u:%g' "$DESKTOP_DIR/package.json")
  DESKTOP_CARGO_TOML_OWNER=$(stat -c '%u:%g' "$DESKTOP_DIR/src-tauri/Cargo.toml")
  cp "$DESKTOP_DIR/package.json" "$DESKTOP_VERSION_BACKUP_DIR/package.json"
  cp "$DESKTOP_DIR/src-tauri/Cargo.toml" "$DESKTOP_VERSION_BACKUP_DIR/Cargo.toml"
  if [ -f "$DESKTOP_DIR/src-tauri/Cargo.lock" ]; then
    DESKTOP_CARGO_LOCK_OWNER=$(stat -c '%u:%g' "$DESKTOP_DIR/src-tauri/Cargo.lock")
    cp "$DESKTOP_DIR/src-tauri/Cargo.lock" "$DESKTOP_VERSION_BACKUP_DIR/Cargo.lock"
  fi

  TARGET_VERSION="$TARGET_VERSION" DESKTOP_DIR="$DESKTOP_DIR" node <<'EOF'
const fs = require('node:fs')
const path = require('node:path')

const version = process.env.TARGET_VERSION
const desktopDir = process.env.DESKTOP_DIR
const packageJsonPath = path.join(desktopDir, 'package.json')
const cargoTomlPath = path.join(desktopDir, 'src-tauri', 'Cargo.toml')
const cargoLockPath = path.join(desktopDir, 'src-tauri', 'Cargo.lock')

const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'))
packageJson.version = version
fs.writeFileSync(packageJsonPath, `${JSON.stringify(packageJson, null, 2)}\n`, 'utf8')

const cargoToml = fs.readFileSync(cargoTomlPath, 'utf8')
if (!/^version\s*=\s*"[^"]+"/m.test(cargoToml))
  throw new Error('failed to locate desktop/src-tauri/Cargo.toml version field')
const nextCargoToml = cargoToml.replace(/^version\s*=\s*"[^"]+"/m, `version = "${version}"`)
fs.writeFileSync(cargoTomlPath, nextCargoToml, 'utf8')

if (fs.existsSync(cargoLockPath)) {
  const cargoLock = fs.readFileSync(cargoLockPath, 'utf8')
  const lockPattern = /(\[\[package\]\]\nname = "asr-desktop"\nversion = )"[^"]+"/m
  const nextCargoLock = cargoLock.replace(lockPattern, `$1"${version}"`)
  fs.writeFileSync(cargoLockPath, nextCargoLock, 'utf8')
}
EOF
}

build_desktop_installer() {
  DEFAULT_CLIENT_URL="$1"

  if [ -n "$DESKTOP_INSTALLER_OVERRIDE" ]; then
    if [ ! -f "$DESKTOP_INSTALLER_OVERRIDE" ]; then
      echo "指定的桌面端安装包不存在: $DESKTOP_INSTALLER_OVERRIDE" >&2
      exit 1
    fi
    printf '%s' "$DESKTOP_INSTALLER_OVERRIDE"
    return 0
  fi

  if [ "$SKIP_DOCKER" -eq 1 ]; then
    find_desktop_installer || return 1
    return 0
  fi

  if run_pnpm --version >/dev/null 2>&1; then
    echo "构建桌面端安装包，默认服务地址: $DEFAULT_CLIENT_URL" >&2
    prepare_desktop_version_files "$VERSION"
    if ! (
      cd "$DESKTOP_DIR"
      ASR_DESKTOP_IGNORE_CERT_ERRORS=1 \
      ASR_BUILD_DATE="$BUILD_DATE" \
      VITE_DEFAULT_SERVER_URL="$DEFAULT_CLIENT_URL" \
      run_pnpm build:win
    ) >&2; then
      restore_desktop_version_files
      echo "桌面端构建失败。" >&2
      exit 1
    fi
    restore_desktop_version_files
    find_desktop_installer_for_version || {
      echo "桌面端构建完成，但未找到当前版本 $VERSION 的 NSIS 安装包输出" >&2
      exit 1
    }
    return 0
  fi

  if find_desktop_installer_for_version >/dev/null 2>&1; then
    echo "警告: 未找到 pnpm，回退为使用已存在的同版本桌面端安装包: $VERSION" >&2
    find_desktop_installer_for_version
    return 0
  fi

  echo "未找到 pnpm/corepack，且没有现成的同版本桌面端安装包可复用，已拒绝继续打包以避免混入旧客户端。" >&2
  echo "当前 PATH: $PATH" >&2
  exit 1
}

create_self_extract_run() {
  RUN_PATH="$1"
  PAYLOAD_ARCHIVE="$2"
  TMP_RUN=$(mktemp)

  cat > "$TMP_RUN" <<'EOF'
#!/bin/sh
set -eu

SELF="$0"
TARGET_BASE=${ASR_RUN_TARGET_DIR:-$PWD}
PAYLOAD_LINE=$(awk '/^__ASR_ARCHIVE_BELOW__$/ {print NR + 1; exit 0; }' "$SELF")

if [ -z "${PAYLOAD_LINE:-}" ]; then
  echo "无效的安装包：未找到内置归档数据" >&2
  exit 1
fi

mkdir -p "$TARGET_BASE"
tail -n +"$PAYLOAD_LINE" "$SELF" | tar -xzf - -C "$TARGET_BASE"

cd "$TARGET_BASE/asr-all-in-one"
sh install.sh
exit 0
__ASR_ARCHIVE_BELOW__
EOF

  cat "$PAYLOAD_ARCHIVE" >> "$TMP_RUN"
  chmod +x "$TMP_RUN"
  mv "$TMP_RUN" "$RUN_PATH"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version|version)
      VERSION="$2"
      shift 2
      ;;
    --output-dir|output-dir)
      OUTPUT_ROOT="$2"
      shift 2
      ;;
    --server-host|server-host)
      SERVER_HOST="$2"
      shift 2
      ;;
    --http-port|http-port)
      HTTP_PORT="$2"
      HTTP_PORT_EXPLICIT=1
      shift 2
      ;;
    --https-port|https-port)
      HTTPS_PORT="$2"
      shift 2
      ;;
    --admin-username|admin-username)
      ADMIN_USERNAME="$2"
      shift 2
      ;;
    --admin-password|admin-password)
      ADMIN_PASSWORD="$2"
      shift 2
      ;;
    --admin-display-name|admin-display-name)
      ADMIN_DISPLAY_NAME="$2"
      shift 2
      ;;
    --mysql-password|mysql-password)
      MYSQL_PASSWORD="$2"
      shift 2
      ;;
    --jwt-secret|jwt-secret)
      JWT_SECRET="$2"
      shift 2
      ;;
    --asr-service-url|asr-service-url)
      ASR_SERVICE_URL="$2"
      shift 2
      ;;
    --desktop-installer|desktop-installer)
      DESKTOP_INSTALLER_OVERRIDE="$2"
      shift 2
      ;;
    --dry-run|dry-run)
      SKIP_DOCKER=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      exit 1
      ;;
  esac
done

if [ -z "$VERSION" ]; then
  if command -v node >/dev/null 2>&1; then
    VERSION=$(node -p "require('$REPO_ROOT/desktop/package.json').version")
  else
    VERSION=$(date +%Y%m%d%H%M%S)
  fi
fi

BUILD_DATE=${ASR_BUILD_DATE:-$(date +%Y-%m-%d)}

SERVER_HOST=$(normalize_server_host "$SERVER_HOST")
validate_port "$HTTP_PORT" "HTTP 端口"
if [ -z "$HTTPS_PORT" ]; then
  if [ "$HTTP_PORT_EXPLICIT" -eq 1 ]; then
    HTTPS_PORT=$((HTTP_PORT + 1))
  else
    HTTPS_PORT=443
  fi
fi
validate_port "$HTTPS_PORT" "HTTPS 端口"

if [ -z "$MYSQL_PASSWORD" ]; then
  MYSQL_PASSWORD=$(generate_secret)
fi
if [ -z "$JWT_SECRET" ]; then
  JWT_SECRET=$(generate_secret)
fi

IMAGE_TAG="asr-all-in-one:$VERSION"
PACKAGE_ROOT_NAME="asr-all-in-one"
PACKAGE_NAME="${PACKAGE_ROOT_NAME}-${VERSION}"
STAGING_DIR="$OUTPUT_ROOT/$PACKAGE_ROOT_NAME"
ARCHIVE_PATH="$OUTPUT_ROOT/$PACKAGE_NAME.tar.gz"
RUN_PATH="$OUTPUT_ROOT/$PACKAGE_NAME.run"
DEFAULT_CLIENT_URL=$(build_https_origin "$SERVER_HOST" "$HTTPS_PORT")
TLS_ALT_NAMES=$(build_tls_alt_names "$SERVER_HOST")

rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR/image" "$STAGING_DIR/runtime/mysql" "$STAGING_DIR/runtime/certs" "$STAGING_DIR/runtime/downloads" "$STAGING_DIR/runtime/tmp" "$STAGING_DIR/runtime/uploads"

cp "$DEPLOY_DIR/docker-compose.bundle.yml" "$STAGING_DIR/docker-compose.yml"
cp "$DEPLOY_DIR/README.md" "$STAGING_DIR/README.md"
cp "$SCRIPT_DIR/install.sh" "$STAGING_DIR/install.sh"
cp "$SCRIPT_DIR/uninstall.sh" "$STAGING_DIR/uninstall.sh"
chmod +x "$STAGING_DIR/install.sh"
chmod +x "$STAGING_DIR/uninstall.sh"

cat > "$STAGING_DIR/.env" <<EOF
ASR_RELEASE_IMAGE=asr-all-in-one:latest
ASR_RELEASE_VERSION=$VERSION
ASR_CONTAINER_NAME=asr-all-in-one
ASR_ENABLE_HTTPS=1
ASR_HTTP_REDIRECT_TO_HTTPS=0
ASR_HTTP_PORT=$HTTP_PORT
ASR_HTTPS_PORT=$HTTPS_PORT
ASR_TLS_COMMON_NAME=$SERVER_HOST
ASR_TLS_ALT_NAMES=$TLS_ALT_NAMES
ASR_MYSQL_ROOT_PASSWORD=$MYSQL_PASSWORD
ASR_MYSQL_DATABASE=asr
ASR_BOOTSTRAP_ADMIN_USERNAME=$ADMIN_USERNAME
ASR_BOOTSTRAP_ADMIN_PASSWORD=$ADMIN_PASSWORD
ASR_BOOTSTRAP_ADMIN_DISPLAY_NAME=$ADMIN_DISPLAY_NAME
ASR_JWT_SECRET=$JWT_SECRET
ASR_SERVICES_ASR=$ASR_SERVICE_URL
ASR_SERVICES_ASR_STREAM=
ASR_SERVICES_DIARIZATION_URL=
ASR_SERVICES_SPEAKER_ANALYSIS_URL=
ASR_SERVICES_SUMMARY_MODEL=qwen3-4b
EOF

cp "$STAGING_DIR/.env" "$STAGING_DIR/.env.example"

cat > "$STAGING_DIR/.release-manifest" <<EOF
RELEASE_VERSION=$VERSION
RELEASE_IMAGE=asr-all-in-one:latest
EOF

DESKTOP_INSTALLER_PATH=""
if DESKTOP_INSTALLER_PATH=$(build_desktop_installer "$DEFAULT_CLIENT_URL"); then
  cp "$DESKTOP_INSTALLER_PATH" "$STAGING_DIR/runtime/downloads/"
  DESKTOP_INSTALLER_BASENAME=$(basename "$DESKTOP_INSTALLER_PATH")
  cat > "$STAGING_DIR/runtime/downloads/README.txt" <<EOF
桌面端安装包已自动打入当前目录：$DESKTOP_INSTALLER_BASENAME
客户端默认服务地址：$DEFAULT_CLIENT_URL
公共下载页会自动读取并展示这些文件。
EOF
else
  if [ "$SKIP_DOCKER" -eq 0 ]; then
    echo "未能自动构建或定位桌面端安装包，发布失败。请检查 pnpm build:win 环境，或改用 --desktop-installer <path>。" >&2
    exit 1
  fi
  cat > "$STAGING_DIR/runtime/downloads/README.txt" <<EOF
未自动打入桌面端安装包。
如果需要，请先执行 desktop/pnpm build:win 或在打包时传入 --desktop-installer <path>。
EOF
fi

cat > "$STAGING_DIR/runtime/certs/README.txt" <<EOF
默认启用 HTTPS。首次安装时如果当前目录没有证书，容器会根据 .env 中的 ASR_TLS_COMMON_NAME 和 ASR_TLS_ALT_NAMES 自动生成自签名证书。
如果你已有证书，可直接把 tls.crt 和 tls.key 放到当前目录，安装或升级时会优先复用现有文件。
EOF

if [ "$SKIP_DOCKER" -eq 0 ]; then
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker 未安装，无法构建离线发布包" >&2
    exit 1
  fi

  echo "构建 Docker 镜像: $IMAGE_TAG"
  docker build --build-arg ASR_APP_VERSION="$VERSION" --build-arg ASR_BUILD_DATE="$BUILD_DATE" -f "$DEPLOY_DIR/Dockerfile" -t "$IMAGE_TAG" "$REPO_ROOT"
  docker tag "$IMAGE_TAG" asr-all-in-one:latest

  echo "导出离线镜像..."
  docker save "$IMAGE_TAG" asr-all-in-one:latest | gzip -c > "$STAGING_DIR/image/asr-all-in-one-image.tar.gz"
else
  echo "dry-run 模式：跳过 Docker 构建与镜像导出"
fi

rm -f "$ARCHIVE_PATH"
rm -f "$RUN_PATH"
tar -czf "$ARCHIVE_PATH" -C "$OUTPUT_ROOT" "$PACKAGE_ROOT_NAME"
create_self_extract_run "$RUN_PATH" "$ARCHIVE_PATH"

echo "发布目录: $STAGING_DIR"
echo "压缩包: $ARCHIVE_PATH"
echo "一键安装包: $RUN_PATH"
echo "默认客户端地址: $DEFAULT_CLIENT_URL"
echo "默认管理员账号: $ADMIN_USERNAME"
echo "默认管理员密码: $ADMIN_PASSWORD"