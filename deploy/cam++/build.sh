#!/bin/bash
# ========================================
# 离线构建与部署脚本
# ========================================
#
# 使用方法：
#
# ---- 在联网机器上 ----
# 1. 构建镜像：  ./build.sh build
# 2. 导出镜像：  ./build.sh export
#    → 生成 speaker-diarization-offline.tar.gz
#    → 用 U 盘拷贝到目标服务器
#
# ---- 在离线服务器上 ----
# 3. 导入镜像：  ./build.sh import
# 4. 启动服务：  ./build.sh start
# 5. 测试服务：  ./build.sh test
# 6. 停止服务：  ./build.sh stop

set -e

IMAGE_NAME="speaker-diarization"
IMAGE_TAG="1.0.0"
FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"
EXPORT_FILE="speaker-diarization-offline.tar.gz"

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

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

detect_device() {
    if [ -n "${DEVICE}" ]; then
        echo "${DEVICE}"
        return
    fi
    echo "cuda:0"
}

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

    if ! docker info >/tmp/campp-docker-info.$$ 2>/dev/null; then
        warn "无法读取 docker info，跳过 Docker GPU 支持检查"
    else
        if ! grep -qi 'nvidia' /tmp/campp-docker-info.$$; then
            warn "docker info 中未发现 nvidia 相关 runtime / CDI 信息，请确认已安装 nvidia-container-toolkit"
        fi
        rm -f /tmp/campp-docker-info.$$
    fi

    if [ "${COMPOSE_IMPL}" = "docker-compose" ]; then
        warn "当前使用 legacy docker-compose，部分 deploy.gpu 配置可能不会完整生效；目标环境建议使用新的 docker compose"
    else
        info "当前使用 ${COMPOSE_IMPL}"
    fi
}

case "${1}" in

# ─── 构建镜像（联网环境）───
build)
    info "开始构建 Docker 镜像: ${FULL_IMAGE}"
    info "此过程需要联网下载模型，预计耗时 10-30 分钟..."
    docker build -t "${FULL_IMAGE}" .
    docker tag "${FULL_IMAGE}" "${IMAGE_NAME}:latest"
    info "镜像构建完成: ${FULL_IMAGE}"
    docker images | grep "${IMAGE_NAME}"
    ;;

# ─── 导出镜像为 tar.gz（联网环境）───
export)
    info "导出镜像: ${FULL_IMAGE} → ${EXPORT_FILE}"
    docker save "${FULL_IMAGE}" | gzip > "${EXPORT_FILE}"
    SIZE=$(du -h "${EXPORT_FILE}" | cut -f1)
    info "导出完成: ${EXPORT_FILE} (${SIZE})"
    info "请将此文件拷贝到目标离线服务器"
    ;;

# ─── 导入镜像（离线环境）───
import)
    if [ ! -f "${EXPORT_FILE}" ]; then
        error "镜像文件不存在: ${EXPORT_FILE}"
        error "请先将联网环境导出的镜像文件拷贝到当前目录"
        exit 1
    fi
    info "导入镜像: ${EXPORT_FILE}"
    gunzip -c "${EXPORT_FILE}" | docker load
    docker tag "${FULL_IMAGE}" "${IMAGE_NAME}:latest"
    info "导入完成"
    docker images | grep "${IMAGE_NAME}"
    ;;

# ─── 启动服务 ───
start)
    info "启动说话人分离服务..."
    if ! docker image inspect "${FULL_IMAGE}" >/dev/null 2>&1 && ! docker image inspect "${IMAGE_NAME}:latest" >/dev/null 2>&1; then
        error "未找到可用镜像: ${FULL_IMAGE}"
        error "请先执行 ./build.sh import，或在联网环境执行 ./build.sh build"
        exit 1
    fi
    check_gpu_readiness
    SD_IMAGE="${FULL_IMAGE}" compose up -d --no-build
    info "服务启动中，首次启动需加载模型，约 60-120 秒..."
    info "可通过以下命令查看日志: ./build.sh logs"
    info "API 文档: http://localhost:8080/docs"
    ;;

# ─── 停止服务 ───
stop)
    info "停止服务..."
    compose down
    info "服务已停止"
    ;;

# ─── 测试服务 ───
test)
    info "测试健康检查接口..."
    HEALTH=$(curl -s http://localhost:8080/health 2>/dev/null)
    if [ $? -eq 0 ]; then
        info "健康检查响应: ${HEALTH}"
    else
        warn "服务可能还在启动中，请稍后再试"
        warn "查看日志: ./build.sh logs"
        exit 1
    fi

    # 如果提供了测试音频文件
    if [ -n "${2}" ]; then
        info "测试说话人分离: ${2}"
        curl -s -X POST "http://localhost:8080/diarize" \
            -F "file=@${2}" \
            | python3 -m json.tool
    else
        info "如需测试分离功能，请运行: ./build.sh test <音频文件路径>"
    fi
    ;;

# ─── 查看日志 ───
logs)
    compose logs -f
    ;;

# ─── 帮助 ───
*)
    echo ""
    echo "说话人分离服务 - 构建与部署工具"
    echo ""
    echo "用法: $0 <命令>"
    echo ""
    echo "命令（联网环境）:"
    echo "  build    构建 Docker 镜像（需联网下载模型）"
    echo "  export   导出镜像为 tar.gz 文件"
    echo ""
    echo "命令（离线环境）:"
    echo "  import   从 tar.gz 导入镜像"
    echo "  start    启动服务"
    echo "  stop     停止服务"
    echo "  test     测试服务 [可选: 音频文件路径]"
    echo "  logs     查看服务日志"
    echo ""
    ;;
esac
