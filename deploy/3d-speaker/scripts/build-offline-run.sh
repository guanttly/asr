#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
DEPLOY_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
BUILD_SH="$DEPLOY_DIR/build.sh"

IMAGE_NAME=${SA_IMAGE_NAME:-speaker-analysis-service}
IMAGE_REF=""
IMAGE_REF_EXPLICIT=0
VERSION=${SA_RELEASE_VERSION:-}
OUTPUT_ROOT="$DEPLOY_DIR/dist"
PACKAGE_ROOT_NAME=${SA_PACKAGE_ROOT_NAME:-speaker-analysis-service}
MODEL_SOURCE_DIR="$DEPLOY_DIR/models"
CONFIG_SOURCE_DIR="$DEPLOY_DIR/config"
ALLOW_INCOMPLETE_NATIVE_CACHE=0
COPY_EXISTING_ENV=1

usage() {
  cat <<EOF
用法: build-offline-run.sh [选项]

作用:
  将服务器本地已有的 3D-Speaker/Speaker Analysis 镜像、models/、config/ 和安装脚本
  打成一个可执行的 .run 离线一键安装包。目标服务器执行 bash xxx.run 即可解包、
  docker load 并用 docker compose 启动服务，全程不会主动拉取镜像、模型或 Python 包。

选项:
  --image <image:tag>          要导出的本地镜像，默认 speaker-analysis-service:<build.sh 中的 IMAGE_TAG>
  --version <version>          发布版本号，默认读取 build.sh 中的 IMAGE_TAG
  --output-dir <dir>           输出目录，默认 deploy/3d-speaker/dist
  --models-dir <dir>           要打入安装包的模型目录，默认 deploy/3d-speaker/models
  --config-dir <dir>           要打入安装包的配置目录，默认 deploy/3d-speaker/config
  --allow-incomplete-native-cache
                              允许 native_cache 缺少 campplus_cn_en_common.pt；不建议离线交付使用
  --no-copy-env                不复制当前目录现有 .env，改用脚本生成的默认 .env
  -h, --help                   显示帮助

示例:
  ./scripts/build-offline-run.sh
  ./scripts/build-offline-run.sh --image speaker-analysis-service:1.1.7
  ./scripts/build-offline-run.sh --models-dir /data/speaker-analysis/models
EOF
}

read_build_version() {
  if [ -f "$BUILD_SH" ]; then
    sed -n 's/^IMAGE_TAG="\([^"]*\)".*/\1/p' "$BUILD_SH" | head -n 1
  fi
}

image_tag_from_ref() {
  VALUE="$1"
  case "$VALUE" in
    *:*)
      printf '%s\n' "${VALUE##*:}"
      ;;
    *)
      printf ''
      ;;
  esac
}

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

dir_has_payload_files() {
  TARGET_DIR="$1"
  [ -d "$TARGET_DIR" ] || return 1
  find "$TARGET_DIR" -type f ! -name '.gitkeep' -print -quit 2>/dev/null | grep -q .
}

require_model_payload() {
  if ! dir_has_payload_files "$MODEL_SOURCE_DIR/fsmn_vad"; then
    echo "缺少 VAD 模型目录或目录为空: $MODEL_SOURCE_DIR/fsmn_vad" >&2
    echo "请先在联网服务器执行 ./build.sh download-models，或通过 --models-dir 指向已下载完整模型的目录。" >&2
    exit 1
  fi

  if ! dir_has_payload_files "$MODEL_SOURCE_DIR/eres2netv2" && ! dir_has_payload_files "$MODEL_SOURCE_DIR/campplus"; then
    echo "缺少嵌入模型: 至少需要 models/eres2netv2 或 models/campplus 其中之一。" >&2
    exit 1
  fi

  if [ "$ALLOW_INCOMPLETE_NATIVE_CACHE" -eq 0 ]; then
    if ! find "$MODEL_SOURCE_DIR/native_cache" -type f -name 'campplus_cn_en_common.pt' -print -quit 2>/dev/null | grep -q .; then
      echo "native_cache 不完整: 未找到 campplus_cn_en_common.pt" >&2
      echo "离线交付建议重新执行 ./build.sh download-models，确保 models/native_cache 一并打包。" >&2
      echo "如确实只接受兼容回退模式，可追加 --allow-incomplete-native-cache。" >&2
      exit 1
    fi
  fi
}

require_file() {
  FILE_PATH="$1"
  if [ ! -f "$FILE_PATH" ]; then
    echo "缺少必要文件: $FILE_PATH" >&2
    exit 1
  fi
}

copy_dir_contents() {
  SOURCE_DIR="$1"
  TARGET_DIR="$2"
  mkdir -p "$TARGET_DIR"
  cp -a "$SOURCE_DIR/." "$TARGET_DIR/"
}

sha256_file() {
  FILE_PATH="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$FILE_PATH" | awk '{print $1}'
    return
  fi
  shasum -a 256 "$FILE_PATH" | awk '{print $1}'
}

create_self_extract_run() {
  RUN_PATH="$1"
  PAYLOAD_ARCHIVE="$2"
  TMP_RUN=$(mktemp)

  cat > "$TMP_RUN" <<'EOF'
#!/bin/sh
set -eu

SELF="$0"
TARGET_BASE=${SA_RUN_TARGET_DIR:-$PWD}
PAYLOAD_LINE=$(awk '/^__SPEAKER_ANALYSIS_ARCHIVE_BELOW__$/ {print NR + 1; exit 0; }' "$SELF")

if [ -z "${PAYLOAD_LINE:-}" ]; then
  echo "无效的安装包：未找到内置归档数据" >&2
  exit 1
fi

mkdir -p "$TARGET_BASE"
tail -n +"$PAYLOAD_LINE" "$SELF" | tar -xzf - -C "$TARGET_BASE"

cd "$TARGET_BASE/speaker-analysis-service"
sh install.sh "$@"
exit 0
__SPEAKER_ANALYSIS_ARCHIVE_BELOW__
EOF

  cat "$PAYLOAD_ARCHIVE" >> "$TMP_RUN"
  chmod +x "$TMP_RUN"
  mv "$TMP_RUN" "$RUN_PATH"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --image)
      IMAGE_REF="$2"
      IMAGE_REF_EXPLICIT=1
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
    --models-dir)
      MODEL_SOURCE_DIR="$2"
      shift 2
      ;;
    --config-dir)
      CONFIG_SOURCE_DIR="$2"
      shift 2
      ;;
    --allow-incomplete-native-cache)
      ALLOW_INCOMPLETE_NATIVE_CACHE=1
      shift
      ;;
    --no-copy-env)
      COPY_EXISTING_ENV=0
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

if [ -z "$VERSION" ]; then
  if [ "$IMAGE_REF_EXPLICIT" -eq 1 ]; then
    VERSION=$(image_tag_from_ref "$IMAGE_REF")
  fi
  if [ -z "$VERSION" ]; then
    VERSION=$(read_build_version || true)
  fi
  if [ -z "$VERSION" ]; then
    VERSION=$(date +%Y%m%d%H%M%S)
  fi
fi

if [ -z "$IMAGE_REF" ]; then
  IMAGE_REF="$IMAGE_NAME:$VERSION"
fi

MODEL_SOURCE_DIR=$(CDPATH= cd -- "$MODEL_SOURCE_DIR" && pwd)
CONFIG_SOURCE_DIR=$(CDPATH= cd -- "$CONFIG_SOURCE_DIR" && pwd)
OUTPUT_ROOT=$(mkdir -p "$OUTPUT_ROOT" && CDPATH= cd -- "$OUTPUT_ROOT" && pwd)

require_file "$DEPLOY_DIR/docker-compose.yml"
require_file "$SCRIPT_DIR/install-offline.sh"
require_file "$SCRIPT_DIR/uninstall-offline.sh"
require_model_payload

if ! command -v docker >/dev/null 2>&1; then
  echo "docker 未安装，无法导出本地镜像。" >&2
  exit 1
fi

if ! docker image inspect "$IMAGE_REF" >/dev/null 2>&1; then
  echo "本地不存在待导出的镜像: $IMAGE_REF" >&2
  echo "请先在服务器上完成部署/构建，或用 --image 指定 docker images 中已有的镜像标签。" >&2
  exit 1
fi

PACKAGE_NAME="$PACKAGE_ROOT_NAME-$VERSION"
STAGING_DIR="$OUTPUT_ROOT/$PACKAGE_ROOT_NAME"
ARCHIVE_PATH="$OUTPUT_ROOT/$PACKAGE_NAME.tar.gz"
RUN_PATH="$OUTPUT_ROOT/$PACKAGE_NAME.run"
IMAGE_ARCHIVE_NAME="speaker-analysis-service-image.tar.gz"
IMAGE_ARCHIVE_PATH="$STAGING_DIR/image/$IMAGE_ARCHIVE_NAME"
BUILD_DATE=${SA_BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}

rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR/image" "$STAGING_DIR/data" "$STAGING_DIR/logs"

cp "$DEPLOY_DIR/docker-compose.yml" "$STAGING_DIR/docker-compose.yml"
cp "$DEPLOY_DIR/build.sh" "$STAGING_DIR/build.sh"
cp "$DEPLOY_DIR/README.md" "$STAGING_DIR/README.md"
cp "$SCRIPT_DIR/install-offline.sh" "$STAGING_DIR/install.sh"
cp "$SCRIPT_DIR/uninstall-offline.sh" "$STAGING_DIR/uninstall.sh"
chmod +x "$STAGING_DIR/install.sh" "$STAGING_DIR/uninstall.sh" "$STAGING_DIR/build.sh"

if [ -f "$DEPLOY_DIR/docs/deployment_guide.md" ]; then
  mkdir -p "$STAGING_DIR/docs"
  cp "$DEPLOY_DIR/docs/deployment_guide.md" "$STAGING_DIR/docs/deployment_guide.md"
fi

copy_dir_contents "$CONFIG_SOURCE_DIR" "$STAGING_DIR/config"
copy_dir_contents "$MODEL_SOURCE_DIR" "$STAGING_DIR/models"

if [ "$COPY_EXISTING_ENV" -eq 1 ] && [ -f "$DEPLOY_DIR/.env" ]; then
  cp "$DEPLOY_DIR/.env" "$STAGING_DIR/.env"
else
  cat > "$STAGING_DIR/.env" <<EOF
SA_IMAGE=$IMAGE_REF
SA_RELEASE_VERSION=$VERSION
SA_CONTAINER_NAME=${SA_CONTAINER_NAME:-speaker-analysis}
SA_PORT=${SA_PORT:-10002}
SA_GPU_ID=${SA_GPU_ID:-2}
SA_DEVICE=${SA_DEVICE:-cuda:0}
WORKERS=${WORKERS:-1}
EOF
fi

update_env_value SA_IMAGE "$IMAGE_REF" "$STAGING_DIR/.env"
update_env_value SA_RELEASE_VERSION "$VERSION" "$STAGING_DIR/.env"
update_env_value SA_CONTAINER_NAME "${SA_CONTAINER_NAME:-speaker-analysis}" "$STAGING_DIR/.env"
cp "$STAGING_DIR/.env" "$STAGING_DIR/.env.example"

echo "导出离线镜像: $IMAGE_REF"
docker save "$IMAGE_REF" | gzip -c > "$IMAGE_ARCHIVE_PATH"
IMAGE_ARCHIVE_SHA256=$(sha256_file "$IMAGE_ARCHIVE_PATH")

cat > "$STAGING_DIR/.release-manifest" <<EOF
RELEASE_VERSION=$VERSION
RELEASE_IMAGE=$IMAGE_REF
RELEASE_IMAGE_ARCHIVE=image/$IMAGE_ARCHIVE_NAME
RELEASE_IMAGE_ARCHIVE_SHA256=$IMAGE_ARCHIVE_SHA256
RELEASE_CREATED_AT=$BUILD_DATE
EOF

rm -f "$ARCHIVE_PATH" "$RUN_PATH"
tar -czf "$ARCHIVE_PATH" -C "$OUTPUT_ROOT" "$PACKAGE_ROOT_NAME"
create_self_extract_run "$RUN_PATH" "$ARCHIVE_PATH"

ARCHIVE_SIZE=$(du -h "$ARCHIVE_PATH" | awk '{print $1}')
RUN_SIZE=$(du -h "$RUN_PATH" | awk '{print $1}')

echo "发布目录: $STAGING_DIR"
echo "压缩包: $ARCHIVE_PATH ($ARCHIVE_SIZE)"
echo "一键安装包: $RUN_PATH ($RUN_SIZE)"
echo "模型目录已打入: $MODEL_SOURCE_DIR"
echo "离线安装: bash $RUN_PATH"
