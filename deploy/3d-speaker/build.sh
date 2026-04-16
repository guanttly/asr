#!/bin/bash
# ========================================
# Speaker Analysis Service 构建与部署工具
# ========================================
#
# 使用方式：
#
# ---- 在联网机器上 ----
# 1. 下载模型： ./build.sh download-models
# 2. 构建镜像： ./build.sh build
# 3. 导出镜像： ./build.sh export
#
# ---- 在离线机器上 ----
# 4. 导入镜像： ./build.sh import
# 5. 启动服务： ./build.sh start
# 6. 验证服务： ./build.sh test [音频文件路径]
# 7. 查看日志： ./build.sh logs

set -e

# 镜像与离线包命名。
IMAGE_NAME="speaker-analysis-service"
IMAGE_TAG="1.1.0"
FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"
EXPORT_FILE="speaker-analysis-service-offline.tar.gz"
MODEL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/models"
DOWNLOAD_IMAGE="${DOWNLOAD_IMAGE:-python:3.10-slim}"
PIP_INDEX_URL="${PIP_INDEX_URL:-https://mirrors.aliyun.com/pypi/simple/}"
PIP_TRUSTED_HOST="${PIP_TRUSTED_HOST:-mirrors.aliyun.com}"
APT_MIRROR="${APT_MIRROR:-https://mirrors.aliyun.com/debian}"
APT_SECURITY_MIRROR="${APT_SECURITY_MIRROR:-https://mirrors.aliyun.com/debian-security}"
WORKSPACE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPEAKERLAB_VENDOR_DIR="${WORKSPACE_DIR}/vendor/3D-Speaker"
SPEAKERLAB_REPO_MIRROR="${SPEAKERLAB_REPO_MIRROR:-https://gitcode.com/mirrors/modelscope/3D-Speaker.git}"
SPEAKERLAB_REPO="${SPEAKERLAB_REPO:-https://github.com/modelscope/3D-Speaker.git}"
SPEAKERLAB_REPO_LIST="${SPEAKERLAB_REPO_LIST:-}"
SPEAKERLAB_REF="${SPEAKERLAB_REF:-main}"
SPEAKERLAB_CLONE_IMAGE="${SPEAKERLAB_CLONE_IMAGE:-alpine/git:latest}"
SPEAKERLAB_CLONE_RETRIES="${SPEAKERLAB_CLONE_RETRIES:-3}"
SPEAKERLAB_CLONE_IMAGE_READY=0

# 终端输出颜色。
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

require_docker() {
    if ! command -v docker >/dev/null 2>&1; then
        error "未检测到 Docker，无法使用容器模式下载模型"
        exit 1
    fi
}

speakerlab_wheel_present() {
    compgen -G "${WORKSPACE_DIR}/wheels/speakerlab-*.whl" >/dev/null
}

speakerlab_source_ready() {
    [ -d "${SPEAKERLAB_VENDOR_DIR}/speakerlab" ]
}

speakerlab_repo_candidates() {
    if [ -n "${SPEAKERLAB_REPO_LIST}" ]; then
        printf '%s\n' "${SPEAKERLAB_REPO_LIST}" | tr ', ' '\n\n' | awk 'NF && !seen[$0]++'
        return
    fi

    printf '%s\n' "${SPEAKERLAB_REPO_MIRROR}" "${SPEAKERLAB_REPO}" | awk 'NF && !seen[$0]++'
}

clone_speakerlab_with_git() {
    local repo="$1"

    GIT_TERMINAL_PROMPT=0 git clone --depth 1 --branch "${SPEAKERLAB_REF}" "${repo}" "${SPEAKERLAB_VENDOR_DIR}"
}

clone_speakerlab_with_docker() {
    local repo="$1"

    require_docker
    if [ "${SPEAKERLAB_CLONE_IMAGE_READY}" -ne 1 ]; then
        info "宿主机未检测到 git，使用容器拉取源码: ${SPEAKERLAB_CLONE_IMAGE}"
        docker pull "${SPEAKERLAB_CLONE_IMAGE}"
        SPEAKERLAB_CLONE_IMAGE_READY=1
    fi
    docker run --rm \
        -v "${WORKSPACE_DIR}:/workspace" \
        -w /workspace \
        "${SPEAKERLAB_CLONE_IMAGE}" \
        clone --depth 1 --branch "${SPEAKERLAB_REF}" "${repo}" "vendor/3D-Speaker"
}

clone_speakerlab_once() {
    local repo

    while IFS= read -r repo; do
        [ -n "${repo}" ] || continue
        info "尝试拉取 3D-Speaker 源码: ${repo} (${SPEAKERLAB_REF})"
        if command -v git >/dev/null 2>&1; then
            if clone_speakerlab_with_git "${repo}"; then
                return 0
            fi
        else
            if clone_speakerlab_with_docker "${repo}"; then
                return 0
            fi
        fi
        warn "3D-Speaker 源码地址不可达: ${repo}"
        rm -rf "${SPEAKERLAB_VENDOR_DIR}"
    done < <(speakerlab_repo_candidates)

    return 1
}

ensure_speakerlab_source() {
    local attempt=1

    if speakerlab_wheel_present; then
        info "检测到本地 speakerlab wheel，跳过源码预拉取"
        return
    fi

    if speakerlab_source_ready; then
        info "检测到本地 3D-Speaker 源码缓存: ${SPEAKERLAB_VENDOR_DIR}"
        return
    fi

    mkdir -p "$(dirname "${SPEAKERLAB_VENDOR_DIR}")"
    while [ "${attempt}" -le "${SPEAKERLAB_CLONE_RETRIES}" ]; do
        info "预拉取 3D-Speaker 源码到本地缓存 [${attempt}/${SPEAKERLAB_CLONE_RETRIES}]"
        rm -rf "${SPEAKERLAB_VENDOR_DIR}"

        if clone_speakerlab_once; then
            break
        fi

        if [ "${attempt}" -lt "${SPEAKERLAB_CLONE_RETRIES}" ]; then
            warn "3D-Speaker 源码拉取失败，2 秒后重试"
            sleep 2
        fi
        attempt=$((attempt + 1))
    done

    if ! speakerlab_source_ready; then
        error "3D-Speaker 源码预拉取失败，请检查网络或镜像配置，或提前将源码放到 ${SPEAKERLAB_VENDOR_DIR}"
        exit 1
    fi

    info "3D-Speaker 源码已缓存: ${SPEAKERLAB_VENDOR_DIR}"
}

# 兼容 docker compose 与 legacy docker-compose 两种命令形态。
compose_version() {
    if docker compose version >/dev/null 2>&1; then
        echo "docker compose"
        return
    fi
    if command -v docker-compose >/dev/null 2>&1; then
        echo "docker-compose"
        return
    fi
    echo ""
}

# 统一的 compose 调用入口，避免后续命令重复判断。
compose() {
    if docker compose version >/dev/null 2>&1; then
        docker compose "$@"
        return
    fi
    if command -v docker-compose >/dev/null 2>&1; then
        docker-compose "$@"
        return
    fi
    error "未检测到 Docker Compose，请安装 docker compose 插件或 docker-compose"
    exit 1
}

detect_device_from_compose() {
    local compose_output

    compose_output="$(compose config 2>/dev/null || true)"
    if [ -z "${compose_output}" ]; then
        return 1
    fi

    printf '%s\n' "${compose_output}" | awk '
        /^[[:space:]]+environment:$/ {
            in_env = 1
            next
        }
        in_env && /^[[:space:]]+[A-Za-z0-9_]+:/ {
            key = $1
            sub(/:$/, "", key)
            if (key == "DEVICE") {
                value = $2
                gsub(/"/, "", value)
                print value
                exit
            }
            next
        }
        in_env && /^[^[:space:]]/ {
            in_env = 0
        }
    '
}

# 读取目标设备；默认按 CPU 模式运行。
detect_device() {
    local compose_device

    if [ -n "${DEVICE:-}" ]; then
        echo "${DEVICE}"
        return
    fi

    compose_device="$(detect_device_from_compose || true)"
    if [ -n "${compose_device}" ]; then
        echo "${compose_device}"
        return
    fi

    echo "cpu"
}

# 构建镜像前校验必须的本地模型目录是否存在。
check_models() {
    local missing=0
    if { [ ! -d "${MODEL_DIR}/eres2netv2" ] || [ -z "$(ls -A "${MODEL_DIR}/eres2netv2" 2>/dev/null)" ]; } \
        && { [ ! -d "${MODEL_DIR}/campplus" ] || [ -z "$(ls -A "${MODEL_DIR}/campplus" 2>/dev/null)" ]; }; then
        warn "嵌入模型缺失: 需要准备 models/eres2netv2 或 models/campplus"
        missing=1
    fi

    if [ ! -d "${MODEL_DIR}/fsmn_vad" ] || [ -z "$(ls -A "${MODEL_DIR}/fsmn_vad" 2>/dev/null)" ]; then
        warn "VAD 模型缺失: ${MODEL_DIR}/fsmn_vad"
        missing=1
    fi

    if [ ${missing} -ne 0 ]; then
        error "构建前需要先准备模型目录。请执行 ./build.sh download-models"
        exit 1
    fi
}

# 在 GPU 模式启动前做宿主机运行条件检查。
check_gpu_readiness() {
    DEVICE_VALUE="$(detect_device)"
    COMPOSE_IMPL="$(compose_version)"

    if [[ "${DEVICE_VALUE}" != cuda* ]]; then
        info "当前 DEVICE=${DEVICE_VALUE}，按 CPU 模式启动"
        return
    fi

    info "当前 DEVICE=${DEVICE_VALUE}，准备按 GPU 模式启动"

    if ! command -v nvidia-smi >/dev/null 2>&1; then
        warn "宿主机未检测到 nvidia-smi，容器大概率无法使用 GPU"
    else
        GPU_SUMMARY="$(nvidia-smi -L 2>/dev/null | head -3 || true)"
        if [ -n "${GPU_SUMMARY}" ]; then
            info "宿主机 GPU:"
            echo "${GPU_SUMMARY}"
        else
            warn "nvidia-smi 可执行，但未读取到 GPU 列表"
        fi
    fi

    if ! docker info >/tmp/speaker-analysis-docker-info.$$ 2>/dev/null; then
        warn "无法读取 docker info，跳过 Docker GPU 支持检查"
    else
        if ! grep -qi 'nvidia' /tmp/speaker-analysis-docker-info.$$; then
            warn "docker info 中未发现 nvidia 相关 runtime / CDI 信息，请确认已安装 nvidia-container-toolkit"
        fi
        rm -f /tmp/speaker-analysis-docker-info.$$
    fi

    if [ "${COMPOSE_IMPL}" = "docker-compose" ]; then
        warn "当前使用 legacy docker-compose，部分 deploy.gpu 配置可能不会完整生效；目标环境建议使用新的 docker compose"
    else
        info "当前使用 ${COMPOSE_IMPL}"
    fi
}

download_models_locally() {
    info "使用宿主机 Python 下载模型..."
    bash scripts/download_models.sh
}

can_download_models_locally() {
    if ! command -v python3 >/dev/null 2>&1; then
        return 1
    fi
    if python3 -c "import modelscope" >/dev/null 2>&1; then
        return 0
    fi
    if python3 -m pip --version >/dev/null 2>&1; then
        return 0
    fi
    return 1
}

download_models_via_docker() {
    require_docker
    info "使用容器模式下载模型，不依赖宿主机 Python"
    info "拉取辅助镜像: ${DOWNLOAD_IMAGE}"
    docker pull "${DOWNLOAD_IMAGE}"
    docker run --rm \
        -e PYTHON_BIN=python3 \
        -e PIP_INDEX_URL="${PIP_INDEX_URL}" \
        -e PIP_TRUSTED_HOST="${PIP_TRUSTED_HOST}" \
        -v "${WORKSPACE_DIR}:/workspace" \
        -w /workspace \
        "${DOWNLOAD_IMAGE}" \
        sh ./scripts/download_models.sh
}

download_models() {
    MODE="${DOWNLOAD_MODE:-auto}"
    case "${MODE}" in
        local)
            download_models_locally
            ;;
        docker)
            download_models_via_docker
            ;;
        auto)
            if can_download_models_locally; then
                download_models_locally
            else
                warn "宿主机缺少可用的 Python/modelscope/pip 运行环境，自动切换到容器模式"
                download_models_via_docker
            fi
            ;;
        *)
            error "未知 DOWNLOAD_MODE=${MODE}，可选值: auto / local / docker"
            exit 1
            ;;
    esac
}

case "${1}" in
# 联网环境：下载模型权重到本地 models/ 目录。
download-models)
    info "下载模型权重..."
    download_models
    ;;

# 联网环境：强制使用 Docker 容器下载模型，不依赖宿主机 Python。
download-models-docker)
    info "下载模型权重（容器模式）..."
    download_models_via_docker
    ;;

# 联网环境：基于根目录 Dockerfile 构建服务镜像。
build)
    check_models
    ensure_speakerlab_source
    info "开始构建 Docker 镜像: ${FULL_IMAGE}"
    DOCKER_BUILDKIT=1 docker build \
        --build-arg PIP_INDEX="${PIP_INDEX_URL}" \
        --build-arg PIP_TRUSTED_HOST="${PIP_TRUSTED_HOST}" \
        --build-arg APT_MIRROR="${APT_MIRROR}" \
        --build-arg APT_SECURITY_MIRROR="${APT_SECURITY_MIRROR}" \
        -t "${FULL_IMAGE}" .
    docker tag "${FULL_IMAGE}" "${IMAGE_NAME}:latest"
    info "镜像构建完成: ${FULL_IMAGE}"
    docker images | grep "${IMAGE_NAME}"
    ;;

# 联网环境：导出镜像为离线可传输的 tar.gz 文件。
export)
    info "导出镜像: ${FULL_IMAGE} → ${EXPORT_FILE}"
    docker save "${FULL_IMAGE}" | gzip > "${EXPORT_FILE}"
    SIZE=$(du -h "${EXPORT_FILE}" | cut -f1)
    info "导出完成: ${EXPORT_FILE} (${SIZE})"
    ;;

# 离线环境：导入先前导出的镜像文件。
import)
    if [ ! -f "${EXPORT_FILE}" ]; then
        error "镜像文件不存在: ${EXPORT_FILE}"
        exit 1
    fi
    info "导入镜像: ${EXPORT_FILE}"
    gunzip -c "${EXPORT_FILE}" | docker load
    docker tag "${FULL_IMAGE}" "${IMAGE_NAME}:latest"
    info "导入完成"
    docker images | grep "${IMAGE_NAME}"
    ;;

# 启动容器服务；若 DEVICE=cuda:* 则附带 GPU 运行条件检查。
start)
    info "启动 Speaker Analysis Service..."
    if ! docker image inspect "${FULL_IMAGE}" >/dev/null 2>&1 && ! docker image inspect "${IMAGE_NAME}:latest" >/dev/null 2>&1; then
        error "未找到可用镜像: ${FULL_IMAGE}"
        error "请先执行 ./build.sh import，或在联网环境执行 ./build.sh build"
        exit 1
    fi
    check_gpu_readiness
    SA_IMAGE="${FULL_IMAGE}" compose up -d --no-build
    info "服务启动中，首次启动需加载模型，约 60-120 秒..."
    info "API 文档: http://localhost:${SA_PORT:-8100}/docs"
    ;;

# 停止 compose 管理的服务实例。
stop)
    info "停止服务..."
    compose down
    info "服务已停止"
    ;;

# 快速连通性测试：默认检查 health，可选再调用一次 VAD 接口。
test)
    info "测试健康检查接口..."
    HEALTH=$(curl -s "http://localhost:${SA_PORT:-8100}/api/v1/health" 2>/dev/null)
    if [ $? -eq 0 ]; then
        info "健康检查响应: ${HEALTH}"
    else
        warn "服务可能还在启动中，请稍后再试"
        warn "查看日志: ./build.sh logs"
        exit 1
    fi

    if [ -n "${2}" ]; then
        info "测试 VAD 接口: ${2}"
        curl -s -X POST "http://localhost:${SA_PORT:-8100}/api/v1/vad" \
            -F "file=@${2}" \
            | python3 -m json.tool
    else
        info "如需测试 VAD 功能，请运行: ./build.sh test <音频文件路径>"
    fi
    ;;

# 持续查看服务日志。
logs)
    compose logs -f
    ;;

# 未提供命令时输出帮助信息。
*)
    echo ""
    echo "Speaker Analysis Service - 构建与部署工具"
    echo ""
    echo "用法: $0 <命令>"
    echo ""
    echo "命令（联网环境）:"
    echo "  download-models  下载模型权重到 models/"
    echo "  download-models-docker  通过 Docker 拉取辅助镜像下载模型（无需宿主机 Python）"
    echo "  build            构建 Docker 镜像（自动缓存 3D-Speaker 源码与 pip 下载）"
    echo "  export           导出镜像为 tar.gz 文件"
    echo ""
    echo "命令（离线环境）:"
    echo "  import           从 tar.gz 导入镜像"
    echo "  start            启动服务"
    echo "  stop             停止服务"
    echo "  test             测试服务 [可选: 音频文件路径]"
    echo "  logs             查看服务日志"
    echo ""
    ;;
esac
