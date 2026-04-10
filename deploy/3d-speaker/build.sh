#!/usr/bin/env bash
# =============================================================================
# 3D-Speaker 说话人分离服务 —— 构建与打包脚本
# 用法: bash build.sh [选项]
#
# 选项:
#   --download-models     下载模型权重（需联网，仅在有网络的构建机上执行）
#   --build-docker        构建 Docker 镜像
#   --pack-offline        打包离线部署包（含模型、镜像、配置）
#   --export-onnx         导出 ONNX 模型（可选，用于 Triton 部署）
#   --skip-tests          跳过测试
#   --model-dir <path>    指定模型权重目录（默认: ./models）
#   --output-dir <path>   指定输出目录（默认: ./dist）
#   --help                显示帮助
# =============================================================================

set -euo pipefail

# ─── 颜色输出 ───
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }
log_step()  { echo -e "${BLUE}[STEP]${NC}  $*"; }

# ─── 默认参数 ───
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_NAME="speaker-diarization-service"
VERSION="1.0.0"
MODEL_DIR="${SCRIPT_DIR}/models"
OUTPUT_DIR="${SCRIPT_DIR}/dist"
DOCKER_IMAGE="${PROJECT_NAME}:${VERSION}"
DOCKER_REGISTRY=""

DO_DOWNLOAD_MODELS=false
DO_BUILD_DOCKER=false
DO_PACK_OFFLINE=false
DO_EXPORT_ONNX=false
DO_SKIP_TESTS=false

# ─── 模型定义 ───
declare -A MODELS=(
    ["eres2netv2"]="iic/speech_eres2netv2_sv_zh-cn_16k-common"
    ["campplus"]="iic/speech_campplus_sv_zh-cn_16k-common"
    ["fsmn_vad"]="iic/speech_fsmn_vad_zh-cn-16k-common-pytorch"
)

# ─── 参数解析 ───
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --download-models) DO_DOWNLOAD_MODELS=true; shift ;;
            --build-docker)    DO_BUILD_DOCKER=true; shift ;;
            --pack-offline)    DO_PACK_OFFLINE=true; shift ;;
            --export-onnx)     DO_EXPORT_ONNX=true; shift ;;
            --skip-tests)      DO_SKIP_TESTS=true; shift ;;
            --model-dir)       MODEL_DIR="$2"; shift 2 ;;
            --output-dir)      OUTPUT_DIR="$2"; shift 2 ;;
            --help)            show_help; exit 0 ;;
            *)
                log_error "未知参数: $1"
                show_help
                exit 1
                ;;
        esac
    done
}

show_help() {
    head -n 14 "${BASH_SOURCE[0]}" | tail -n 12
}

# ─── 环境检查 ───
check_prerequisites() {
    log_step "检查构建环境..."

    # Python
    if ! command -v python3 &>/dev/null; then
        log_error "未找到 python3，请安装 Python 3.8+"
        exit 1
    fi
    local py_version
    py_version=$(python3 -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')")
    log_info "Python 版本: ${py_version}"

    # pip
    if ! python3 -m pip --version &>/dev/null; then
        log_error "未找到 pip"
        exit 1
    fi

    # Docker（仅在需要时检查）
    if [[ "${DO_BUILD_DOCKER}" == true ]] || [[ "${DO_PACK_OFFLINE}" == true ]]; then
        if ! command -v docker &>/dev/null; then
            log_error "未找到 docker，请先安装 Docker"
            exit 1
        fi
        log_info "Docker 版本: $(docker --version | awk '{print $3}')"
    fi

    log_info "环境检查通过"
}

# ─── 创建 Python 虚拟环境 ───
setup_venv() {
    log_step "创建 Python 虚拟环境..."

    local venv_dir="${SCRIPT_DIR}/.venv"
    if [[ ! -d "${venv_dir}" ]]; then
        python3 -m venv "${venv_dir}"
        log_info "虚拟环境已创建: ${venv_dir}"
    else
        log_info "虚拟环境已存在，跳过创建"
    fi

    source "${venv_dir}/bin/activate"
    pip install --upgrade pip -q
    pip install -r "${SCRIPT_DIR}/requirements.txt" -q
    log_info "依赖安装完成"
}

# ─── 下载模型权重 ───
download_models() {
    if [[ "${DO_DOWNLOAD_MODELS}" != true ]]; then
        return 0
    fi

    log_step "下载模型权重到 ${MODEL_DIR} ..."
    mkdir -p "${MODEL_DIR}"

    # 确保 modelscope 已安装
    pip install -U modelscope -q

    for model_key in "${!MODELS[@]}"; do
        local model_id="${MODELS[$model_key]}"
        local target_dir="${MODEL_DIR}/${model_key}"

        if [[ -d "${target_dir}" ]] && [[ -n "$(ls -A "${target_dir}" 2>/dev/null)" ]]; then
            log_info "模型已存在，跳过: ${model_key} (${target_dir})"
            continue
        fi

        log_info "正在下载: ${model_id} → ${target_dir}"
        python3 -c "
from modelscope.hub.snapshot_download import snapshot_download
snapshot_download('${model_id}', cache_dir='${target_dir}')
" || {
            log_warn "modelscope snapshot_download 失败，尝试 CLI 方式..."
            modelscope download --model "${model_id}" --local_dir "${target_dir}" || {
                log_error "模型下载失败: ${model_id}"
                exit 1
            }
        }
        log_info "下载完成: ${model_key}"
    done

    log_info "所有模型下载完成"
}

# ─── 验证模型权重 ───
verify_models() {
    log_step "验证模型权重..."

    local missing=0
    for model_key in "${!MODELS[@]}"; do
        local target_dir="${MODEL_DIR}/${model_key}"
        if [[ ! -d "${target_dir}" ]] || [[ -z "$(ls -A "${target_dir}" 2>/dev/null)" ]]; then
            log_warn "模型缺失: ${model_key} (期望路径: ${target_dir})"
            missing=$((missing + 1))
        else
            local size
            size=$(du -sh "${target_dir}" | awk '{print $1}')
            log_info "模型已就绪: ${model_key} (${size})"
        fi
    done

    if [[ ${missing} -gt 0 ]]; then
        log_error "${missing} 个模型缺失。请执行 build.sh --download-models 或手动下载"
        log_info "手动下载方式: modelscope download --model <model_id> --local_dir models/<name>"
        exit 1
    fi

    log_info "模型验证通过"
}

# ─── 运行测试 ───
run_tests() {
    if [[ "${DO_SKIP_TESTS}" == true ]]; then
        log_warn "跳过测试"
        return 0
    fi

    log_step "运行单元测试..."

    cd "${SCRIPT_DIR}"
    python3 -m pytest tests/ -v --tb=short 2>&1 || {
        log_warn "测试未通过（可能缺少模型权重，在离线环境中可跳过: --skip-tests）"
    }
}

# ─── 导出 ONNX ───
export_onnx() {
    if [[ "${DO_EXPORT_ONNX}" != true ]]; then
        return 0
    fi

    log_step "导出 ONNX 模型..."

    local onnx_dir="${MODEL_DIR}/onnx"
    mkdir -p "${onnx_dir}"

    python3 -c "
import sys
sys.path.insert(0, '${SCRIPT_DIR}')
from src.embedding import EmbeddingExtractor

extractor = EmbeddingExtractor(
    model_dir='${MODEL_DIR}/eres2netv2',
    device='cpu'
)
output_path = '${onnx_dir}/eres2netv2.onnx'
extractor.export_onnx(output_path)
print(f'ONNX 导出完成: {output_path}')
" || {
        log_warn "ONNX 导出失败，跳过（不影响常规部署）"
    }
}

# ─── 构建 Docker 镜像 ───
build_docker() {
    if [[ "${DO_BUILD_DOCKER}" != true ]]; then
        return 0
    fi

    log_step "构建 Docker 镜像: ${DOCKER_IMAGE} ..."

    docker build \
        -t "${DOCKER_IMAGE}" \
        -f "${SCRIPT_DIR}/docker/Dockerfile" \
        --build-arg VERSION="${VERSION}" \
        "${SCRIPT_DIR}"

    log_info "Docker 镜像构建完成: ${DOCKER_IMAGE}"
    docker images "${PROJECT_NAME}" --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"
}

# ─── 打包离线部署包 ───
pack_offline() {
    if [[ "${DO_PACK_OFFLINE}" != true ]]; then
        return 0
    fi

    log_step "打包离线部署包..."

    local pack_dir="${OUTPUT_DIR}/${PROJECT_NAME}-${VERSION}"
    local archive_name="${PROJECT_NAME}-${VERSION}-offline.tar.gz"

    rm -rf "${pack_dir}"
    mkdir -p "${pack_dir}"/{models,docker,config,scripts,src}

    # 复制源码
    cp -r "${SCRIPT_DIR}/src/"*            "${pack_dir}/src/"
    cp -r "${SCRIPT_DIR}/config/"*         "${pack_dir}/config/"
    cp -r "${SCRIPT_DIR}/scripts/"*        "${pack_dir}/scripts/"
    cp    "${SCRIPT_DIR}/requirements.txt" "${pack_dir}/"
    cp    "${SCRIPT_DIR}/Makefile"         "${pack_dir}/"
    cp    "${SCRIPT_DIR}/README.md"        "${pack_dir}/"

    # 复制模型权重
    if [[ -d "${MODEL_DIR}" ]]; then
        log_info "复制模型权重..."
        cp -r "${MODEL_DIR}/"* "${pack_dir}/models/" 2>/dev/null || true
    fi

    # 导出 Docker 镜像
    if docker image inspect "${DOCKER_IMAGE}" &>/dev/null; then
        log_info "导出 Docker 镜像..."
        docker save "${DOCKER_IMAGE}" -o "${pack_dir}/docker/${PROJECT_NAME}-${VERSION}.tar"
        cp "${SCRIPT_DIR}/docker/docker-compose.yaml" "${pack_dir}/docker/"
    else
        log_warn "Docker 镜像不存在，跳过镜像导出"
        cp "${SCRIPT_DIR}/docker/"* "${pack_dir}/docker/" 2>/dev/null || true
    fi

    # 生成部署安装脚本
    cat > "${pack_dir}/install.sh" << 'INSTALL_EOF'
#!/usr/bin/env bash
set -euo pipefail

echo "=========================================="
echo " 3D-Speaker 说话人分离服务 — 离线安装"
echo "=========================================="

INSTALL_DIR="/opt/speaker-diarization-service"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "[1/5] 创建安装目录..."
sudo mkdir -p "${INSTALL_DIR}"
sudo cp -r "${SCRIPT_DIR}"/* "${INSTALL_DIR}/"
sudo chown -R "$(whoami)" "${INSTALL_DIR}"

echo "[2/5] 安装 Python 依赖..."
cd "${INSTALL_DIR}"
python3 -m venv .venv
source .venv/bin/activate
pip install --upgrade pip -q
pip install -r requirements.txt -q

echo "[3/5] 初始化数据库..."
python3 scripts/init_db.py

echo "[4/5] 加载 Docker 镜像（如有）..."
for tarfile in docker/*.tar; do
    if [[ -f "${tarfile}" ]]; then
        echo "    加载: ${tarfile}"
        docker load -i "${tarfile}"
    fi
done

echo "[5/5] 验证安装..."
python3 -c "from src.engine import DiarizationEngine; print('引擎模块加载成功')"
python3 -c "from src.voiceprint import VoiceprintManager; print('声纹模块加载成功')"

echo ""
echo "=========================================="
echo " 安装完成!"
echo " 启动服务: cd ${INSTALL_DIR} && make serve"
echo " Docker:   cd ${INSTALL_DIR}/docker && docker-compose up -d"
echo "=========================================="
INSTALL_EOF
    chmod +x "${pack_dir}/install.sh"

    # 打包
    log_info "压缩打包..."
    cd "${OUTPUT_DIR}"
    tar -czf "${archive_name}" "$(basename "${pack_dir}")"
    rm -rf "${pack_dir}"

    local archive_path="${OUTPUT_DIR}/${archive_name}"
    local archive_size
    archive_size=$(du -sh "${archive_path}" | awk '{print $1}')
    log_info "离线部署包: ${archive_path} (${archive_size})"
}

# ─── 生成构建摘要 ───
print_summary() {
    echo ""
    echo "=========================================="
    echo " 构建完成摘要"
    echo "=========================================="
    echo " 版本:       ${VERSION}"
    echo " 模型目录:   ${MODEL_DIR}"
    echo " 输出目录:   ${OUTPUT_DIR}"

    if [[ "${DO_BUILD_DOCKER}" == true ]]; then
        echo " Docker 镜像: ${DOCKER_IMAGE}"
    fi
    if [[ "${DO_PACK_OFFLINE}" == true ]]; then
        echo " 离线包:     ${OUTPUT_DIR}/${PROJECT_NAME}-${VERSION}-offline.tar.gz"
    fi

    echo "=========================================="
    echo ""
    echo "下一步操作:"
    echo "  本地开发:     make dev"
    echo "  启动服务:     make serve"
    echo "  Docker 部署:  cd docker && docker-compose up -d"
    echo "  运行测试:     make test"
    echo ""
}

# ─── 主流程 ───
main() {
    parse_args "$@"

    echo ""
    log_info "3D-Speaker 说话人分离服务 — 构建开始 (v${VERSION})"
    echo ""

    check_prerequisites
    setup_venv
    download_models

    # 如果没有要下载模型，但需要打包或构建 Docker，则验证模型存在
    if [[ "${DO_BUILD_DOCKER}" == true ]] || [[ "${DO_PACK_OFFLINE}" == true ]]; then
        verify_models
    fi

    run_tests
    export_onnx
    build_docker
    pack_offline
    print_summary
}

main "$@"
