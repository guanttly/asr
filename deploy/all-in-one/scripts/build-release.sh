#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
DEPLOY_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
REPO_ROOT=$(CDPATH= cd -- "$DEPLOY_DIR/../.." && pwd)
DESKTOP_DIR="$REPO_ROOT/desktop"
DESKTOP_ELECTRON_DIR="$REPO_ROOT/desktop-electron"
DESKTOP_INSTALLER_DIR="$DESKTOP_DIR/src-tauri/target/x86_64-pc-windows-msvc/release/bundle/nsis"
DESKTOP_ELECTRON_INSTALLER_DIR="$DESKTOP_ELECTRON_DIR/release"
CURRENT_UID=$(id -u)
CURRENT_USER=$(id -un)
CURRENT_GROUP=$(id -gn)

VERSION=""
DESKTOP_VERSION=""
OUTPUT_ROOT="$DEPLOY_DIR/dist"
SKIP_DOCKER=0
SERVER_HOST="localhost"
HTTP_PORT="9855"
HTTP_PORT_EXPLICIT=0
HTTPS_PORT=""
ADMIN_USERNAME="admin"
ADMIN_PASSWORD="jusha1996"
ADMIN_DISPLAY_NAME="系统管理员"
MYSQL_PASSWORD=""
JWT_SECRET=""
ASR_SERVICE_URL="http://jusha-asr-asr:8000"
SPEAKER_SERVICE_URL="http://jusha-asr-speaker:8100"
DESKTOP_INSTALLER_OVERRIDE=""
DESKTOP_ELECTRON_INSTALLER_OVERRIDE=""
SKIP_ELECTRON_BUILD=0
DESKTOP_PACKAGE_OWNER=""
DESKTOP_CARGO_TOML_OWNER=""
DESKTOP_CARGO_LOCK_OWNER=""
DESKTOP_ELECTRON_PACKAGE_OWNER=""
OUTPUT_OWNER=""
PART_SIZE="${ASR_RELEASE_PART_SIZE:-${JUSHA_ASR_PART_SIZE:-500m}}"
KEEP_ARCHIVE="${ASR_RELEASE_KEEP_ARCHIVE:-${JUSHA_ASR_KEEP_ARCHIVE:-0}}"

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

ensure_pnpm_project_ready() {
  PROJECT_DIR="$1"
  PROJECT_LABEL="$2"
  REQUIRED_BIN="$3"

  if [ -x "$PROJECT_DIR/$REQUIRED_BIN" ]; then
    return 0
  fi

  echo "检测到 $PROJECT_LABEL 依赖未就绪，正在执行 pnpm install..." >&2
  if [ -f "$PROJECT_DIR/pnpm-lock.yaml" ]; then
    if ! (
      cd "$PROJECT_DIR"
      run_pnpm install --frozen-lockfile
    ) >&2; then
      echo "$PROJECT_LABEL 依赖安装失败。" >&2
      return 1
    fi
  else
    if ! (
      cd "$PROJECT_DIR"
      run_pnpm install
    ) >&2; then
      echo "$PROJECT_LABEL 依赖安装失败。" >&2
      return 1
    fi
  fi

  if [ ! -x "$PROJECT_DIR/$REQUIRED_BIN" ]; then
    echo "$PROJECT_LABEL 依赖安装完成，但仍缺少构建命令: $REQUIRED_BIN" >&2
    return 1
  fi
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
                                     HTTP 对外端口，默认 9855；若显式传入且未传 --https-port，则 HTTPS 自动取 http+1
  --https-port <port> | https-port <port>
                                     HTTPS 对外端口，默认 9856
  --admin-username <username> | admin-username <username>
                                     默认管理员用户名，默认 admin
  --admin-password <password> | admin-password <password>
                                     默认管理员密码，默认 jusha1996
  --admin-display-name <name> | admin-display-name <name>
                                     默认管理员显示名，默认 系统管理员
  --mysql-password <password> | mysql-password <password>
                                     MySQL root 密码，默认自动生成
  --jwt-secret <secret> | jwt-secret <secret>
                                     JWT 密钥，默认自动生成
  --asr-service-url <url> | asr-service-url <url>
                                     ASR 服务地址，默认 http://jusha-asr-asr:8000
  --speaker-service-url <url> | speaker-service-url <url>
                                     统一的人声服务地址，默认 http://jusha-asr-speaker:8100
  --desktop-installer <path> | desktop-installer <path>
                                     直接使用现成桌面端（Tauri Win10/11）安装包，不自动构建
  --desktop-version <version> | desktop-version <version>
                                     桌面端安装包版本，默认与发布版本号一致
  --desktop-electron-installer <path> | desktop-electron-installer <path>
                                     直接使用现成 Win7 兼容版（Electron）安装包，不自动构建
  --skip-electron | skip-electron   跳过 Win7 兼容版 Electron 安装包的构建与打包
  --dry-run | dry-run               跳过 Docker 镜像构建和桌面端自动构建
  -h, --help                        显示帮助

环境变量:
  ASR_RELEASE_PART_SIZE=500m         .run 分包大小，默认 500m
  ASR_RELEASE_KEEP_ARCHIVE=1         保留中间 .tar.gz，默认只保留 .run 与 .run.partNNN
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
  TARGET_VERSION="${1:-$VERSION}"
  if [ -n "$TARGET_VERSION" ] && ls "$DESKTOP_INSTALLER_DIR"/*"$TARGET_VERSION"*setup.exe >/dev/null 2>&1; then
    ls -t "$DESKTOP_INSTALLER_DIR"/*"$TARGET_VERSION"*setup.exe | head -n 1
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
  find_desktop_installer_for_version "$DESKTOP_VERSION" || find_latest_desktop_installer
}

find_desktop_electron_installer_for_version() {
  TARGET_VERSION="${1:-$VERSION}"
  if [ -n "$TARGET_VERSION" ] && ls "$DESKTOP_ELECTRON_INSTALLER_DIR"/*"$TARGET_VERSION"*win7*setup.exe >/dev/null 2>&1; then
    ls -t "$DESKTOP_ELECTRON_INSTALLER_DIR"/*"$TARGET_VERSION"*win7*setup.exe | head -n 1
    return 0
  fi
  return 1
}

find_latest_desktop_electron_installer() {
  if ls "$DESKTOP_ELECTRON_INSTALLER_DIR"/*win7*setup.exe >/dev/null 2>&1; then
    ls -t "$DESKTOP_ELECTRON_INSTALLER_DIR"/*win7*setup.exe | head -n 1
    return 0
  fi
  return 1
}

find_desktop_electron_installer() {
  find_desktop_electron_installer_for_version "$DESKTOP_VERSION" || find_latest_desktop_electron_installer
}

maybe_restore_owner() {
  OWNER="$1"
  TARGET_PATH="$2"
  if [ "$CURRENT_UID" -ne 0 ]; then
    return 0
  fi
  if [ -n "$OWNER" ] && [ -e "$TARGET_PATH" ]; then
    chown "$OWNER" "$TARGET_PATH"
  fi
}

maybe_restore_tree_owner() {
  OWNER="$1"
  TARGET_PATH="$2"
  if [ "$CURRENT_UID" -ne 0 ]; then
    return 0
  fi
  if [ -n "$OWNER" ] && [ -e "$TARGET_PATH" ]; then
    chown -R "$OWNER" "$TARGET_PATH"
  fi
}

resolve_path_owner() {
  TARGET_PATH="$1"
  if [ -e "$TARGET_PATH" ]; then
    stat -c '%u:%g' "$TARGET_PATH"
    return 0
  fi
  TARGET_PARENT=$(dirname "$TARGET_PATH")
  stat -c '%u:%g' "$TARGET_PARENT"
}

ensure_output_owner_matches_current_user() {
  TARGET_PATH="$1"
  if [ "$CURRENT_UID" -eq 0 ] || [ ! -e "$TARGET_PATH" ]; then
    return 0
  fi

  FOREIGN_PATH=$(find "$TARGET_PATH" ! -user "$CURRENT_USER" -print -quit 2>/dev/null || true)
  if [ -n "$FOREIGN_PATH" ]; then
    echo "当前发布目标路径存在非当前用户属主的旧产物，无法安全覆盖: $FOREIGN_PATH" >&2
    echo "请先执行: sudo chown -R $CURRENT_USER:$CURRENT_GROUP $TARGET_PATH" >&2
    exit 1
  fi
}

reset_staging_dir() {
  TARGET_PATH="$1"

  if [ ! -e "$TARGET_PATH" ]; then
    return 0
  fi

  rm -rf "$TARGET_PATH" 2>/dev/null || true
  if [ ! -e "$TARGET_PATH" ]; then
    return 0
  fi

  STALE_PATH="${TARGET_PATH}.stale.$(date +%Y%m%d%H%M%S).$$"
  if mv "$TARGET_PATH" "$STALE_PATH" 2>/dev/null; then
    echo "检测到旧发布目录包含非当前用户属主的运行残留，已移动到: $STALE_PATH" >&2
    return 0
  fi

  ensure_output_owner_matches_current_user "$TARGET_PATH"
  rm -rf "$TARGET_PATH"
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

restore_desktop_electron_version_file() {
  if [ -z "${DESKTOP_ELECTRON_VERSION_BACKUP_DIR:-}" ] || [ ! -d "$DESKTOP_ELECTRON_VERSION_BACKUP_DIR" ]; then
    return 0
  fi

  cp "$DESKTOP_ELECTRON_VERSION_BACKUP_DIR/package.json" "$DESKTOP_ELECTRON_DIR/package.json"
  maybe_restore_owner "$DESKTOP_ELECTRON_PACKAGE_OWNER" "$DESKTOP_ELECTRON_DIR/package.json"
  rm -rf "$DESKTOP_ELECTRON_VERSION_BACKUP_DIR"
  DESKTOP_ELECTRON_VERSION_BACKUP_DIR=""
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

prepare_desktop_electron_version_file() {
  TARGET_VERSION="$1"

  if ! command -v node >/dev/null 2>&1; then
    echo "未找到 node，无法在构建前同步 Win7 兼容版版本号。" >&2
    return 1
  fi

  DESKTOP_ELECTRON_VERSION_BACKUP_DIR=$(mktemp -d)
  DESKTOP_ELECTRON_PACKAGE_OWNER=$(stat -c '%u:%g' "$DESKTOP_ELECTRON_DIR/package.json")
  cp "$DESKTOP_ELECTRON_DIR/package.json" "$DESKTOP_ELECTRON_VERSION_BACKUP_DIR/package.json"

  TARGET_VERSION="$TARGET_VERSION" DESKTOP_ELECTRON_DIR="$DESKTOP_ELECTRON_DIR" node <<'EOF'
const fs = require('node:fs')
const path = require('node:path')

const version = process.env.TARGET_VERSION
const desktopElectronDir = process.env.DESKTOP_ELECTRON_DIR
const packageJsonPath = path.join(desktopElectronDir, 'package.json')

const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'))
packageJson.version = version
fs.writeFileSync(packageJsonPath, `${JSON.stringify(packageJson, null, 2)}\n`, 'utf8')
EOF
}

build_desktop_installer() {
  DEFAULT_CLIENT_URL="$1"
  FALLBACK_CLIENT_URL="$2"

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
    echo "构建桌面端安装包（Tauri Win10/11），默认服务地址: $DEFAULT_CLIENT_URL；回退地址: $FALLBACK_CLIENT_URL" >&2
    ensure_pnpm_project_ready "$DESKTOP_DIR" "桌面端（Tauri Win10/11）" "node_modules/.bin/tauri" || exit 1
    prepare_desktop_version_files "$DESKTOP_VERSION"
    if ! (
      cd "$DESKTOP_DIR"
      ASR_DESKTOP_IGNORE_CERT_ERRORS=1 \
      ASR_BUILD_DATE="$BUILD_DATE" \
      VITE_DEFAULT_SERVER_URL="$DEFAULT_CLIENT_URL" \
      VITE_FALLBACK_SERVER_URL="$FALLBACK_CLIENT_URL" \
      run_pnpm build:win
    ) >&2; then
      restore_desktop_version_files
      echo "桌面端构建失败。" >&2
      exit 1
    fi
    restore_desktop_version_files
    find_desktop_installer_for_version "$DESKTOP_VERSION" || {
      echo "桌面端构建完成，但未找到当前桌面端版本 $DESKTOP_VERSION 的 NSIS 安装包输出" >&2
      exit 1
    }
    return 0
  fi

  if find_desktop_installer_for_version "$DESKTOP_VERSION" >/dev/null 2>&1; then
    echo "警告: 未找到 pnpm，回退为使用已存在的同版本桌面端安装包: $DESKTOP_VERSION" >&2
    find_desktop_installer_for_version "$DESKTOP_VERSION"
    return 0
  fi

  echo "未找到 pnpm/corepack，且没有现成的同版本桌面端安装包可复用，已拒绝继续打包以避免混入旧客户端。" >&2
  echo "当前 PATH: $PATH" >&2
  exit 1
}

build_desktop_electron_installer() {
  DEFAULT_CLIENT_URL="$1"
  FALLBACK_CLIENT_URL="$2"

  if [ "$SKIP_ELECTRON_BUILD" -eq 1 ]; then
    return 1
  fi

  if [ -n "$DESKTOP_ELECTRON_INSTALLER_OVERRIDE" ]; then
    if [ ! -f "$DESKTOP_ELECTRON_INSTALLER_OVERRIDE" ]; then
      echo "指定的 Win7 兼容版安装包不存在: $DESKTOP_ELECTRON_INSTALLER_OVERRIDE" >&2
      exit 1
    fi
    printf '%s' "$DESKTOP_ELECTRON_INSTALLER_OVERRIDE"
    return 0
  fi

  if [ ! -d "$DESKTOP_ELECTRON_DIR" ]; then
    echo "未找到 desktop-electron 目录，跳过 Win7 兼容版打包: $DESKTOP_ELECTRON_DIR" >&2
    return 1
  fi

  if [ "$SKIP_DOCKER" -eq 1 ]; then
    find_desktop_electron_installer || return 1
    return 0
  fi

  if run_pnpm --version >/dev/null 2>&1; then
    echo "构建 Win7 兼容版安装包（Electron 22），默认服务地址: $DEFAULT_CLIENT_URL；回退地址: $FALLBACK_CLIENT_URL" >&2
    ensure_pnpm_project_ready "$DESKTOP_ELECTRON_DIR" "Win7 兼容版（Electron 22）" "node_modules/.bin/vite" || return 1
    prepare_desktop_electron_version_file "$DESKTOP_VERSION" || return 1
    if ! (
      cd "$DESKTOP_ELECTRON_DIR"
      ASR_BUILD_DATE="$BUILD_DATE" \
      VITE_DEFAULT_SERVER_URL="$DEFAULT_CLIENT_URL" \
      VITE_FALLBACK_SERVER_URL="$FALLBACK_CLIENT_URL" \
      run_pnpm build:win
    ) >&2; then
      restore_desktop_electron_version_file
      echo "Win7 兼容版（Electron）构建失败。" >&2
      return 1
    fi
    restore_desktop_electron_version_file
    find_desktop_electron_installer_for_version "$DESKTOP_VERSION" || {
      echo "Win7 兼容版构建完成，但未找到当前桌面端版本 $DESKTOP_VERSION 的安装包输出" >&2
      return 1
    }
    return 0
  fi

  if find_desktop_electron_installer_for_version "$DESKTOP_VERSION" >/dev/null 2>&1; then
    echo "警告: 未找到 pnpm，回退为使用已存在的同版本 Win7 兼容版安装包: $DESKTOP_VERSION" >&2
    find_desktop_electron_installer_for_version "$DESKTOP_VERSION"
    return 0
  fi

  echo "未找到 pnpm/corepack，且没有现成的同版本 Win7 兼容版安装包，已跳过 Win7 兼容版打包。" >&2
  return 1
}

split_payload_archive() {
  PAYLOAD_ARCHIVE="$1"
  RUN_PATH_VALUE="$2"

  if ! command -v split >/dev/null 2>&1; then
    echo "缺少 split 命令，无法生成分包发布文件" >&2
    exit 1
  fi

  rm -f "$RUN_PATH_VALUE".part[0-9][0-9][0-9]*
  if split --help 2>/dev/null | grep -q -- '--numeric-suffixes'; then
    split -b "$PART_SIZE" -d -a 3 --numeric-suffixes=1 "$PAYLOAD_ARCHIVE" "$RUN_PATH_VALUE.part"
  else
    split -b "$PART_SIZE" -d -a 3 "$PAYLOAD_ARCHIVE" "$RUN_PATH_VALUE.part"
  fi
}

create_self_extract_run() {
  RUN_PATH="$1"
  PAYLOAD_ARCHIVE="$2"
  TMP_RUN=$(mktemp)

  cat > "$TMP_RUN" <<'EOF'
#!/bin/sh
set -eu

SELF="$0"
SELF_DIR=$(CDPATH= cd -- "$(dirname "$SELF")" && pwd)
SELF_NAME=$(basename "$SELF")
TARGET_BASE=${ASR_RUN_TARGET_DIR:-$PWD}
TARGET_DIR="$TARGET_BASE/jusha-asr-business"
PART_FILES=$(find "$SELF_DIR" -maxdepth 1 -type f -name "$SELF_NAME.part[0-9][0-9][0-9]*" | sort)
EXTRACT_ROOT=

if [ -z "$PART_FILES" ]; then
  echo "无效的安装包：未找到分包文件 $SELF_NAME.part001" >&2
  exit 1
fi

cleanup() {
  if [ -n "${EXTRACT_ROOT:-}" ] && [ -d "$EXTRACT_ROOT" ]; then
    rm -rf "$EXTRACT_ROOT"
  fi
}

trap cleanup EXIT INT TERM

preserve_target_entry() {
  case "$1" in
    .env|runtime|backups)
      return 0
      ;;
  esac

  return 1
}

preserve_runtime_entry() {
  case "$1" in
    mysql|uploads|tmp|certs|term-catalog)
      return 0
      ;;
  esac

  return 1
}

sync_cert_dir() {
  SRC_CERT_DIR="$1"
  DEST_CERT_DIR="$2"

  mkdir -p "$DEST_CERT_DIR"

  find "$DEST_CERT_DIR" -mindepth 1 -maxdepth 1 | while read -r EXISTING_PATH; do
    CERT_NAME=$(basename "$EXISTING_PATH")
    case "$CERT_NAME" in
      tls.crt|tls.key)
        continue
        ;;
    esac

    if [ ! -e "$SRC_CERT_DIR/$CERT_NAME" ]; then
      rm -rf "$EXISTING_PATH"
    fi
  done

  find "$SRC_CERT_DIR" -mindepth 1 -maxdepth 1 | while read -r INCOMING_PATH; do
    CERT_NAME=$(basename "$INCOMING_PATH")
    case "$CERT_NAME" in
      tls.crt|tls.key)
        if [ -e "$DEST_CERT_DIR/$CERT_NAME" ]; then
          continue
        fi
        ;;
    esac

    rm -rf "$DEST_CERT_DIR/$CERT_NAME"
    cp -a "$INCOMING_PATH" "$DEST_CERT_DIR/$CERT_NAME"
  done
}

sync_runtime_dir() {
  SRC_RUNTIME_DIR="$1"
  DEST_RUNTIME_DIR="$2"

  mkdir -p "$DEST_RUNTIME_DIR"

  find "$DEST_RUNTIME_DIR" -mindepth 1 -maxdepth 1 | while read -r EXISTING_PATH; do
    ENTRY_NAME=$(basename "$EXISTING_PATH")
    if preserve_runtime_entry "$ENTRY_NAME"; then
      continue
    fi

    if [ ! -e "$SRC_RUNTIME_DIR/$ENTRY_NAME" ]; then
      rm -rf "$EXISTING_PATH"
    fi
  done

  find "$SRC_RUNTIME_DIR" -mindepth 1 -maxdepth 1 | while read -r INCOMING_PATH; do
    ENTRY_NAME=$(basename "$INCOMING_PATH")
    case "$ENTRY_NAME" in
      mysql|uploads|tmp|term-catalog)
        mkdir -p "$DEST_RUNTIME_DIR/$ENTRY_NAME"
        if [ "$ENTRY_NAME" = "tmp" ]; then
          chmod 1777 "$DEST_RUNTIME_DIR/$ENTRY_NAME" 2>/dev/null || true
        fi
        ;;
      certs)
        sync_cert_dir "$INCOMING_PATH" "$DEST_RUNTIME_DIR/$ENTRY_NAME"
        ;;
      *)
        rm -rf "$DEST_RUNTIME_DIR/$ENTRY_NAME"
        cp -a "$INCOMING_PATH" "$DEST_RUNTIME_DIR/$ENTRY_NAME"
        ;;
    esac
  done

  mkdir -p "$DEST_RUNTIME_DIR/mysql" "$DEST_RUNTIME_DIR/uploads" "$DEST_RUNTIME_DIR/tmp" "$DEST_RUNTIME_DIR/certs" "$DEST_RUNTIME_DIR/term-catalog"
  chmod 1777 "$DEST_RUNTIME_DIR/tmp" 2>/dev/null || true
}

if [ -f "$TARGET_DIR/.env" ]; then
  echo "检测到已有 .env，安装时将保留现有数据库和服务配置。"
fi

mkdir -p "$TARGET_BASE"
EXTRACT_ROOT=$(mktemp -d)
cat $PART_FILES | tar -xzf - -C "$EXTRACT_ROOT"

if [ ! -d "$EXTRACT_ROOT/jusha-asr-business" ]; then
  echo "无效的安装包：未找到 jusha-asr-business 目录" >&2
  exit 1
fi

if [ ! -d "$TARGET_DIR" ]; then
  mv "$EXTRACT_ROOT/jusha-asr-business" "$TARGET_DIR"
else
  find "$TARGET_DIR" -mindepth 1 -maxdepth 1 | while read -r EXISTING_PATH; do
    ENTRY_NAME=$(basename "$EXISTING_PATH")
    if preserve_target_entry "$ENTRY_NAME"; then
      continue
    fi

    if [ ! -e "$EXTRACT_ROOT/jusha-asr-business/$ENTRY_NAME" ]; then
      rm -rf "$EXISTING_PATH"
    fi
  done

  find "$EXTRACT_ROOT/jusha-asr-business" -mindepth 1 -maxdepth 1 | while read -r INCOMING_PATH; do
    ENTRY_NAME=$(basename "$INCOMING_PATH")
    case "$ENTRY_NAME" in
      .env)
        if [ -f "$TARGET_DIR/.env" ]; then
          continue
        fi
        ;;
      runtime)
        sync_runtime_dir "$INCOMING_PATH" "$TARGET_DIR/runtime"
        continue
        ;;
      backups)
        continue
        ;;
    esac

    rm -rf "$TARGET_DIR/$ENTRY_NAME"
    cp -a "$INCOMING_PATH" "$TARGET_DIR/$ENTRY_NAME"
  done
fi

cd "$TARGET_DIR"
sh install.sh
exit 0
EOF

  split_payload_archive "$PAYLOAD_ARCHIVE" "$RUN_PATH"
  chmod +x "$TMP_RUN"
  mv "$TMP_RUN" "$RUN_PATH"
}

cleanup_on_exit() {
  restore_desktop_version_files
  restore_desktop_electron_version_file
}

trap cleanup_on_exit EXIT HUP INT TERM

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
    --speaker-service-url|speaker-service-url)
      SPEAKER_SERVICE_URL="$2"
      shift 2
      ;;
    --diarization-service-url|diarization-service-url)
      echo "--diarization-service-url 已移除，请改用 --speaker-service-url" >&2
      exit 1
      ;;
    --speaker-analysis-service-url|speaker-analysis-service-url)
      echo "--speaker-analysis-service-url 已移除，请改用 --speaker-service-url" >&2
      exit 1
      ;;
    --desktop-installer|desktop-installer)
      DESKTOP_INSTALLER_OVERRIDE="$2"
      shift 2
      ;;
    --desktop-version|desktop-version)
      DESKTOP_VERSION="$2"
      shift 2
      ;;
    --desktop-electron-installer|desktop-electron-installer)
      DESKTOP_ELECTRON_INSTALLER_OVERRIDE="$2"
      shift 2
      ;;
    --skip-electron|skip-electron)
      SKIP_ELECTRON_BUILD=1
      shift
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
if [ -z "$DESKTOP_VERSION" ]; then
  DESKTOP_VERSION="$VERSION"
fi

BUILD_DATE=${ASR_BUILD_DATE:-$(date +%Y-%m-%d)}

SERVER_HOST=$(normalize_server_host "$SERVER_HOST")
validate_port "$HTTP_PORT" "HTTP 端口"
if [ -z "$HTTPS_PORT" ]; then
  if [ "$HTTP_PORT_EXPLICIT" -eq 1 ]; then
    HTTPS_PORT=$((HTTP_PORT + 1))
  else
    HTTPS_PORT=9856
  fi
fi
validate_port "$HTTPS_PORT" "HTTPS 端口"

if [ -z "$MYSQL_PASSWORD" ]; then
  MYSQL_PASSWORD=$(generate_secret)
fi
if [ -z "$JWT_SECRET" ]; then
  JWT_SECRET=$(generate_secret)
fi

IMAGE_TAG="jusha-asr-business:$VERSION"
PACKAGE_ROOT_NAME="jusha-asr-business"
PACKAGE_NAME="${PACKAGE_ROOT_NAME}-${VERSION}"
STAGING_DIR="$OUTPUT_ROOT/$PACKAGE_ROOT_NAME"
ARCHIVE_PATH="$OUTPUT_ROOT/$PACKAGE_NAME.tar.gz"
RUN_PATH="$OUTPUT_ROOT/$PACKAGE_NAME.run"
# 发行包默认优先走 HTTPS，自签证书场景由桌面客户端放宽校验；
# 保留 HTTP 作为回退地址，兼容未启用 TLS 的旧部署或应急直连。
DEFAULT_CLIENT_URL=$(build_https_origin "$SERVER_HOST" "$HTTPS_PORT")
DEFAULT_CLIENT_FALLBACK_URL=$(build_http_origin "$SERVER_HOST" "$HTTP_PORT")
TLS_ALT_NAMES=$(build_tls_alt_names "$SERVER_HOST")
OUTPUT_OWNER=$(resolve_path_owner "$OUTPUT_ROOT")

ensure_output_owner_matches_current_user "$ARCHIVE_PATH"
ensure_output_owner_matches_current_user "$RUN_PATH"
for EXISTING_PART in "$RUN_PATH".part[0-9][0-9][0-9]*; do
  [ -e "$EXISTING_PART" ] || continue
  ensure_output_owner_matches_current_user "$EXISTING_PART"
done

reset_staging_dir "$STAGING_DIR"
mkdir -p "$STAGING_DIR/image" "$STAGING_DIR/runtime/mysql" "$STAGING_DIR/runtime/certs" "$STAGING_DIR/runtime/downloads" "$STAGING_DIR/runtime/tmp" "$STAGING_DIR/runtime/uploads" "$STAGING_DIR/runtime/term-catalog"

cp "$DEPLOY_DIR/docker-compose.bundle.yml" "$STAGING_DIR/docker-compose.yml"
cp "$DEPLOY_DIR/README.md" "$STAGING_DIR/README.md"
cp "$SCRIPT_DIR/install.sh" "$STAGING_DIR/install.sh"
cp "$SCRIPT_DIR/uninstall.sh" "$STAGING_DIR/uninstall.sh"
chmod +x "$STAGING_DIR/install.sh"
chmod +x "$STAGING_DIR/uninstall.sh"

cat > "$STAGING_DIR/.env" <<EOF
ASR_RELEASE_IMAGE=jusha-asr-business:latest
ASR_RELEASE_VERSION=$VERSION
ASR_CONTAINER_NAME=jusha-asr-business
ASR_DOCKER_NETWORK_NAME=jusha-asr
ASR_DOCKER_SUBNET=
ASR_DOCKER_GATEWAY=
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
ASR_SERVICES_SPEAKER_SERVICE_URL=$SPEAKER_SERVICE_URL
ASR_SERVICES_SUMMARY_MODEL=qwen3-4b
EOF

cp "$STAGING_DIR/.env" "$STAGING_DIR/.env.example"

cat > "$STAGING_DIR/.release-manifest" <<EOF
RELEASE_VERSION=$VERSION
RELEASE_IMAGE=jusha-asr-business:latest
EOF

DESKTOP_INSTALLER_PATH=""
DESKTOP_ELECTRON_INSTALLER_PATH=""
DESKTOP_BASENAMES=""

# Tauri 打出来的 Win10/11 推荐版安装包
if DESKTOP_INSTALLER_PATH=$(build_desktop_installer "$DEFAULT_CLIENT_URL" "$DEFAULT_CLIENT_FALLBACK_URL"); then
  DESKTOP_INSTALLER_BASENAME=$(basename "$DESKTOP_INSTALLER_PATH")
  # 给 Tauri 产物补上 _win10_ 标识，让公共下载页前端能正确归类到「Win10/11 推荐版」
  case "$DESKTOP_INSTALLER_BASENAME" in
    *_win10_*|*-win10-*|*_win7_*|*-win7-*)
      RENAMED_TAURI="$DESKTOP_INSTALLER_BASENAME"
      ;;
    *)
      RENAMED_TAURI=$(printf '%s' "$DESKTOP_INSTALLER_BASENAME" | sed 's/_x64-setup\.exe$/_win10_x64-setup.exe/; s/-x64-setup\.exe$/-win10-x64-setup.exe/')
      if [ "$RENAMED_TAURI" = "$DESKTOP_INSTALLER_BASENAME" ]; then
        # 兜底：未匹配 _x64-setup 命名时，在扩展名前插入 .win10
        RENAMED_TAURI="${DESKTOP_INSTALLER_BASENAME%.exe}.win10.exe"
      fi
      ;;
  esac
  cp "$DESKTOP_INSTALLER_PATH" "$STAGING_DIR/runtime/downloads/$RENAMED_TAURI"
  DESKTOP_BASENAMES="$DESKTOP_BASENAMES\n  - $RENAMED_TAURI（Tauri，Win10/11 推荐版）"
else
  if [ "$SKIP_DOCKER" -eq 0 ]; then
    echo "未能自动构建或定位桌面端安装包（Tauri Win10/11 推荐版），发布失败。" >&2
    echo "可通过 --desktop-installer <path> 直接指定现成的安装包。" >&2
    exit 1
  fi
fi

# Electron 22 打出来的 Win7 兼容版安装包
if DESKTOP_ELECTRON_INSTALLER_PATH=$(build_desktop_electron_installer "$DEFAULT_CLIENT_URL" "$DEFAULT_CLIENT_FALLBACK_URL"); then
  ELECTRON_BASENAME=$(basename "$DESKTOP_ELECTRON_INSTALLER_PATH")
  cp "$DESKTOP_ELECTRON_INSTALLER_PATH" "$STAGING_DIR/runtime/downloads/$ELECTRON_BASENAME"
  DESKTOP_BASENAMES="$DESKTOP_BASENAMES\n  - $ELECTRON_BASENAME（Electron 22，Win7 兼容版）"
else
  if [ "$SKIP_ELECTRON_BUILD" -eq 1 ]; then
    echo "提示：已显式跳过 Win7 兼容版安装包（Electron 22）构建。" >&2
  else
    echo "未能自动构建或定位 Win7 兼容版安装包（Electron 22），发布失败。" >&2
    echo "可通过 --desktop-electron-installer <path> 指定现成安装包，或通过 --skip-electron 显式跳过。" >&2
    exit 1
  fi
fi

if [ -n "$DESKTOP_BASENAMES" ]; then
  printf "桌面端安装包已自动打入当前目录：%b\n客户端默认服务地址：%s\n公共下载页会自动读取并按目标系统分组展示这些文件。\n" "$DESKTOP_BASENAMES" "$DEFAULT_CLIENT_URL" > "$STAGING_DIR/runtime/downloads/README.txt"
else
  cat > "$STAGING_DIR/runtime/downloads/README.txt" <<EOF
未自动打入任何桌面端安装包。
- Win10/11 推荐版：在 desktop/ 下执行 pnpm build:win 或传入 --desktop-installer <path>
- Win7 兼容版：在 desktop-electron/ 下执行 pnpm build:win 或传入 --desktop-electron-installer <path>
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
  docker tag "$IMAGE_TAG" jusha-asr-business:latest

  echo "导出离线镜像..."
  docker save "$IMAGE_TAG" jusha-asr-business:latest | gzip -c > "$STAGING_DIR/image/jusha-asr-business-image.tar.gz"
else
  echo "dry-run 模式：跳过 Docker 构建与镜像导出"
fi

rm -f "$ARCHIVE_PATH"
rm -f "$RUN_PATH"
rm -f "$RUN_PATH".part[0-9][0-9][0-9]*
tar -czf "$ARCHIVE_PATH" -C "$OUTPUT_ROOT" "$PACKAGE_ROOT_NAME"
create_self_extract_run "$RUN_PATH" "$ARCHIVE_PATH"
if [ "$KEEP_ARCHIVE" != "1" ]; then
  rm -f "$ARCHIVE_PATH"
fi
maybe_restore_tree_owner "$OUTPUT_OWNER" "$STAGING_DIR"
maybe_restore_owner "$OUTPUT_OWNER" "$ARCHIVE_PATH"
maybe_restore_owner "$OUTPUT_OWNER" "$RUN_PATH"
for PART_PATH in "$RUN_PATH".part[0-9][0-9][0-9]*; do
  [ -e "$PART_PATH" ] || continue
  maybe_restore_owner "$OUTPUT_OWNER" "$PART_PATH"
done

echo "发布目录: $STAGING_DIR"
echo "一键安装包: $RUN_PATH"
echo "分包文件: $RUN_PATH.part001 ..."
if [ "$KEEP_ARCHIVE" = "1" ]; then
  echo "压缩包: $ARCHIVE_PATH"
fi
echo "默认客户端地址: $DEFAULT_CLIENT_URL"
echo "默认管理员账号: $ADMIN_USERNAME"
echo "默认管理员密码: $ADMIN_PASSWORD"
