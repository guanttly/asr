#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

MODE="all"
VERSION="${JUSHA_ASR_RELEASE_VERSION:-$(date +%Y%m%d%H%M%S)}"
OUTPUT_ROOT="${SCRIPT_DIR}/dist"
KEEP_WORK=0
BUSINESS_ARGS=()
SPEAKER_ARGS=()
PART_SIZE="${JUSHA_ASR_PART_SIZE:-500m}"
KEEP_ARCHIVE="${JUSHA_ASR_KEEP_ARCHIVE:-0}"

usage() {
  cat <<'EOF'
用法: build-release.sh [选项]

选项:
  --mode <all|business|models>  发布形态，默认 all
      business                  只构建业务服务包 jusha-asr-business-<version>.run
      models                    构建 ASR + 3D-Speaker 模型服务组合包 jusha-asr-models-<version>.run
      all                       构建大包 jusha-asr-all-<version>.run，内含业务包和模型服务组合包
  --version <version>           统一版本号，默认当前时间戳
  --output-dir <dir>            输出目录，默认 deploy/jusha-asr/dist
  --business-arg <arg>          透传给业务服务打包脚本，可重复
  --speaker-arg <arg>           透传给 deploy/3d-speaker/scripts/build-offline-run.sh，可重复
  --keep-work                   保留临时工作目录，便于排查
  -h, --help                    显示帮助

常用环境变量:
  SOURCE_DIR=deploy/qwen3-asr              Qwen3-ASR 源目录，默认仓库内 deploy/qwen3-asr
  CONTAINER=qwen3-asr-serve                Qwen3-ASR 源容器名
  SOURCE_IMAGE=qwenllm/qwen3-asr:latest    Qwen3-ASR 源镜像，源容器不存在时回退使用
  GPU_COUNT=1 / GPU_DEVICE_IDS=0           Qwen3-ASR GPU 配置
  SA_IMAGE_NAME=jusha-asr-speaker          3D-Speaker 镜像名
  SA_GPU_ID=0                              3D-Speaker GPU ID
  JUSHA_ASR_PART_SIZE=500m                 传给 split -b 的分包大小，支持 500m/2g/524288000
  JUSHA_ASR_KEEP_ARCHIVE=1                 保留中间 .tar.gz，默认只保留 .run 与 .run.partNNN

示例:
  ./build-release.sh --mode all --version 1.2.0
  ./build-release.sh --mode business --version 1.2.0 --business-arg --dry-run
  JUSHA_ASR_PART_SIZE=2g ./build-release.sh --mode models --version 1.2.0
  ./build-release.sh --mode models --version 1.2.0 --speaker-arg --allow-incomplete-native-cache
EOF
}

info() { printf '[INFO] %s\n' "$*" >&2; }
die() { printf '[ERR ] %s\n' "$*" >&2; exit 1; }

normalize_split_size() {
  local size="$1"

  # GNU split rejects lowercase power suffixes like 2g, but our public examples use them.
  if [[ "$size" =~ ^([0-9]+)([kmgtepzyrq])$ ]]; then
    printf '%s%s' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]^^}"
    return
  fi

  if [[ "$size" =~ ^([0-9]+)([kmgtepzyrq])([bB])$ ]]; then
    printf '%s%sB' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]^^}"
    return
  fi

  printf '%s' "$size"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --mode)
      MODE="$2"
      shift 2
      ;;
    --version)
      VERSION="$2"
      shift 2
      ;;
    --output-dir)
      OUTPUT_ROOT="$2"
      shift 2
      ;;
    --business-arg)
      BUSINESS_ARGS+=("$2")
      shift 2
      ;;
    --speaker-arg)
      SPEAKER_ARGS+=("$2")
      shift 2
      ;;
    --keep-work)
      KEEP_WORK=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "未知参数: $1"
      ;;
  esac
done

case "$MODE" in
  all|business|models) ;;
  *) die "--mode 仅支持 all、business、models" ;;
esac

OUTPUT_ROOT="$(mkdir -p "$OUTPUT_ROOT" && cd "$OUTPUT_ROOT" && pwd)"
WORK_BASE="${JUSHA_ASR_WORK_ROOT:-$OUTPUT_ROOT}"
WORK_BASE="$(mkdir -p "$WORK_BASE" && cd "$WORK_BASE" && pwd)"
WORK_ROOT="$(mktemp -d "$WORK_BASE/.jusha-asr-work.XXXXXX")"
if [ "$KEEP_WORK" -eq 0 ]; then
  trap 'rm -rf "$WORK_ROOT"' EXIT
else
  info "保留临时目录: $WORK_ROOT"
fi

find_latest_run() {
  local dir="$1"
  local pattern="$2"
  find "$dir" -maxdepth 1 -type f -name "$pattern" | sort | tail -n 1
}

split_payload_archive() {
  local payload_archive="$1"
  local run_path="$2"
  local split_size

  if ! command -v split >/dev/null 2>&1; then
    die "缺少 split 命令，无法生成分包发布文件"
  fi

  split_size="$(normalize_split_size "$PART_SIZE")"
  rm -f "$run_path".part[0-9][0-9][0-9]*
  if split --help 2>/dev/null | grep -q -- '--numeric-suffixes'; then
    split -b "$split_size" -d -a 3 --numeric-suffixes=1 "$payload_archive" "$run_path.part"
  else
    split -b "$split_size" -d -a 3 "$payload_archive" "$run_path.part"
  fi
}

copy_run_with_parts() {
  local run_path="$1"
  local target_dir="$2"
  local part_path

  cp "$run_path" "$target_dir/"
  for part_path in "$run_path".part[0-9][0-9][0-9]*; do
    [ -e "$part_path" ] || continue
    cp "$part_path" "$target_dir/"
  done
}

create_self_extract_run() {
  local run_path="$1"
  local payload_archive="$2"
  local package_root_name="$3"
  local tmp_run
  tmp_run="$(mktemp)"

  cat > "$tmp_run" <<'EOF'
#!/bin/sh
set -eu

SELF="$0"
SELF_DIR=$(CDPATH= cd -- "$(dirname "$SELF")" && pwd)
SELF_NAME=$(basename "$SELF")
TARGET_BASE=${JUSHA_ASR_RUN_TARGET_DIR:-$PWD}
PACKAGE_ROOT_NAME="__PACKAGE_ROOT_NAME__"
PART_FILES=$(find "$SELF_DIR" -maxdepth 1 -type f -name "$SELF_NAME.part[0-9][0-9][0-9]*" | sort)

if [ -z "$PART_FILES" ]; then
  echo "无效的安装包：未找到分包文件 $SELF_NAME.part001" >&2
  exit 1
fi

mkdir -p "$TARGET_BASE"
cat $PART_FILES | tar -xzf - -C "$TARGET_BASE"

cd "$TARGET_BASE/$PACKAGE_ROOT_NAME"
sh install.sh "$@"
exit 0
EOF

  sed -i "s|__PACKAGE_ROOT_NAME__|$package_root_name|g" "$tmp_run"
  split_payload_archive "$payload_archive" "$run_path"
  chmod +x "$tmp_run"
  mv "$tmp_run" "$run_path"
}

write_models_installer() {
  local staging_dir="$1"
  cat > "$staging_dir/install.sh" <<'EOF'
#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)

find_package() {
  PATTERN="$1"
  find "$SCRIPT_DIR/packages" -maxdepth 1 -type f -name "$PATTERN" | sort | tail -n 1
}

ASR_RUN=$(find_package 'jusha-asr-asr-*.run')
SPEAKER_RUN=$(find_package 'jusha-asr-speaker-*.run')

if [ -z "$ASR_RUN" ]; then
  echo "缺少 ASR 离线安装包: packages/jusha-asr-asr-*.run" >&2
  exit 1
fi
if [ -z "$SPEAKER_RUN" ]; then
  echo "缺少 3D-Speaker 离线安装包: packages/jusha-asr-speaker-*.run" >&2
  exit 1
fi
if ! command -v bash >/dev/null 2>&1; then
  echo "缺少 bash，无法执行 ASR 自解压包" >&2
  exit 1
fi

echo "安装 ASR 推理服务..."
EXTRACT_DIR="$SCRIPT_DIR/jusha-asr-asr" bash "$ASR_RUN"

echo "安装 3D-Speaker 服务..."
SA_RUN_TARGET_DIR="$SCRIPT_DIR" sh "$SPEAKER_RUN"

echo "模型服务安装完成。"
EOF

  cat > "$staging_dir/uninstall.sh" <<'EOF'
#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)

compose_down() {
  DIR="$1"
  if [ ! -f "$DIR/docker-compose.yml" ]; then
    return 0
  fi
  if docker compose version >/dev/null 2>&1; then
    (cd "$DIR" && docker compose --env-file .env -f docker-compose.yml down --remove-orphans) || true
  elif command -v docker-compose >/dev/null 2>&1; then
    (cd "$DIR" && docker-compose --env-file .env -f docker-compose.yml down --remove-orphans) || true
  fi
}

if [ -x "$SCRIPT_DIR/jusha-asr-speaker/uninstall.sh" ]; then
  sh "$SCRIPT_DIR/jusha-asr-speaker/uninstall.sh" "$@" || true
fi

compose_down "$SCRIPT_DIR/jusha-asr-asr"
docker rm -f jusha-asr-asr >/dev/null 2>&1 || true

echo "模型服务卸载完成。"
EOF

  chmod +x "$staging_dir/install.sh" "$staging_dir/uninstall.sh"
}

write_all_installer() {
  local staging_dir="$1"
  cat > "$staging_dir/install.sh" <<'EOF'
#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)

find_package() {
  PATTERN="$1"
  find "$SCRIPT_DIR/packages" -maxdepth 1 -type f -name "$PATTERN" | sort | tail -n 1
}

MODELS_RUN=$(find_package 'jusha-asr-models-*.run')
BUSINESS_RUN=$(find_package 'jusha-asr-business-*.run')

if [ -z "$MODELS_RUN" ]; then
  echo "缺少模型服务组合包: packages/jusha-asr-models-*.run" >&2
  exit 1
fi
if [ -z "$BUSINESS_RUN" ]; then
  echo "缺少业务服务包: packages/jusha-asr-business-*.run" >&2
  exit 1
fi

echo "安装 ASR + 3D-Speaker 模型服务..."
JUSHA_ASR_RUN_TARGET_DIR="$SCRIPT_DIR" sh "$MODELS_RUN"

echo "安装业务服务..."
ASR_RUN_TARGET_DIR="$SCRIPT_DIR" sh "$BUSINESS_RUN"

echo "Jusha ASR 大包安装完成。"
EOF

  cat > "$staging_dir/uninstall.sh" <<'EOF'
#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)

if [ -x "$SCRIPT_DIR/jusha-asr-business/uninstall.sh" ]; then
  sh "$SCRIPT_DIR/jusha-asr-business/uninstall.sh" "$@" || true
fi
if [ -x "$SCRIPT_DIR/jusha-asr-models/uninstall.sh" ]; then
  sh "$SCRIPT_DIR/jusha-asr-models/uninstall.sh" "$@" || true
fi

echo "Jusha ASR 大包卸载完成。"
EOF

  chmod +x "$staging_dir/install.sh" "$staging_dir/uninstall.sh"
}

write_readme() {
  local staging_dir="$1"
  local title="$2"
  cat > "$staging_dir/README.md" <<EOF
# $title

默认 Docker 网络: \`jusha-asr\`

默认端口:

- 业务 HTTP: \`9855\`
- 业务 HTTPS: \`9856\`
- ASR: \`9851\`
- 3D-Speaker: \`9852\`

安装:

\`\`\`bash
sh install.sh
\`\`\`

卸载:

\`\`\`bash
sh uninstall.sh
\`\`\`
EOF
}

build_business_package() {
  local output_dir="$1"
  mkdir -p "$output_dir"
  info "构建业务服务包"
  if ! bash "$DEPLOY_DIR/all-in-one/scripts/build-release.sh" \
    --version "$VERSION" \
    --output-dir "$output_dir" \
    "${BUSINESS_ARGS[@]}" >&2; then
    return 1
  fi
  find_latest_run "$output_dir" 'jusha-asr-business-*.run'
}

build_asr_package() {
  local output_dir="$1"
  local asr_work_root="$WORK_ROOT/qwen3-asr-work"
  mkdir -p "$output_dir"
  info "构建 ASR 推理服务包"
  mkdir -p "$asr_work_root"
  if ! (cd "$DEPLOY_DIR/qwen3-asr" && OUTPUT_DIR="$output_dir" RELEASE_VERSION="$VERSION" QWEN3_ASR_PACK_WORK_ROOT="$asr_work_root" bash ./pack.sh) >&2; then
    return 1
  fi
  find_latest_run "$output_dir" 'jusha-asr-asr-*.run'
}

build_speaker_package() {
  local output_dir="$1"
  mkdir -p "$output_dir"
  info "构建 3D-Speaker 服务包"
  if ! sh "$DEPLOY_DIR/3d-speaker/scripts/build-offline-run.sh" \
    --version "$VERSION" \
    --output-dir "$output_dir" \
    "${SPEAKER_ARGS[@]}" >&2; then
    return 1
  fi
  find_latest_run "$output_dir" 'jusha-asr-speaker-*.run'
}

package_staging_dir() {
  local package_root="$1"
  local package_name="$2"
  local output_root="$3"
  local staging_dir="$WORK_ROOT/$package_root"
  local archive_path="$output_root/$package_name.tar.gz"
  local run_path="$output_root/$package_name.run"

  mkdir -p "$output_root"
  rm -f "$archive_path" "$run_path"
  tar -czf "$archive_path" -C "$WORK_ROOT" "$package_root"
  create_self_extract_run "$run_path" "$archive_path" "$package_root"
  if [ "$KEEP_ARCHIVE" != "1" ]; then
    rm -f "$archive_path"
  fi

  info "发布目录: $staging_dir"
  info "一键安装包: $run_path"
  info "分包文件: $run_path.part001 ..."
  if [ "$KEEP_ARCHIVE" = "1" ]; then
    info "压缩包: $archive_path"
  fi
}

build_models_bundle() {
  local output_dir="$1"
  local asr_output="$WORK_ROOT/asr-output"
  local speaker_output="$WORK_ROOT/speaker-output"
  local asr_run
  local speaker_run
  local package_root="jusha-asr-models"
  local package_name="${package_root}-${VERSION}"
  local staging_dir="$WORK_ROOT/$package_root"

  asr_run="$(build_asr_package "$asr_output")" || die "ASR 推理服务包构建失败"
  speaker_run="$(build_speaker_package "$speaker_output")" || die "3D-Speaker 服务包构建失败"
  [ -n "$asr_run" ] || die "未找到 ASR .run 输出"
  [ -n "$speaker_run" ] || die "未找到 3D-Speaker .run 输出"

  rm -rf "$staging_dir"
  mkdir -p "$staging_dir/packages" "$output_dir"
  copy_run_with_parts "$asr_run" "$staging_dir/packages"
  copy_run_with_parts "$speaker_run" "$staging_dir/packages"
  write_models_installer "$staging_dir"
  write_readme "$staging_dir" "Jusha ASR 模型服务组合包"

  package_staging_dir "$package_root" "$package_name" "$output_dir"
}

build_all_bundle() {
  local business_output="$WORK_ROOT/business-output"
  local models_output="$WORK_ROOT/models-output"
  local business_run
  local models_run
  local package_root="jusha-asr-all"
  local package_name="${package_root}-${VERSION}"
  local staging_dir="$WORK_ROOT/$package_root"

  business_run="$(build_business_package "$business_output")" || die "业务服务包构建失败"
  build_models_bundle "$models_output"
  models_run="$(find_latest_run "$models_output" 'jusha-asr-models-*.run')"
  [ -n "$business_run" ] || die "未找到业务服务 .run 输出"
  [ -n "$models_run" ] || die "未找到模型服务组合 .run 输出"

  rm -rf "$staging_dir"
  mkdir -p "$staging_dir/packages"
  copy_run_with_parts "$business_run" "$staging_dir/packages"
  copy_run_with_parts "$models_run" "$staging_dir/packages"
  write_all_installer "$staging_dir"
  write_readme "$staging_dir" "Jusha ASR 大包"

  package_staging_dir "$package_root" "$package_name" "$OUTPUT_ROOT"
}

case "$MODE" in
  business)
    business_run="$(build_business_package "$OUTPUT_ROOT")"
    [ -n "$business_run" ] || die "未找到业务服务 .run 输出"
    ;;
  models)
    build_models_bundle "$OUTPUT_ROOT"
    ;;
  all)
    build_all_bundle
    ;;
esac

info "完成: mode=$MODE version=$VERSION output=$OUTPUT_ROOT"