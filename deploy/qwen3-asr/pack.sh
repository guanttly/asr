#!/usr/bin/env bash
# =============================================================================
# Qwen3-ASR 打包脚本（基于 docker compose）
#
# 作用：
#   在源服务器上，把 qwen3-asr-serve 容器镜像、模型目录打包，并生成一份
#   可迁移的 docker-compose.yml + deploy.sh。
#
# 关键改进：
#   1. 修正容器端口识别被二次覆盖的问题。
#   2. 目标机部署时自动扫描宿主机路由 / 网卡地址 / Docker 既有网络，
#      选择一个不冲突的 Docker bridge 子网，并写入 .env。
#   3. 默认使用 GPU count: 1，避免把源机器 GPU ID 硬编码到客户机。
#      目标机 deploy.sh 会识别 docker compose / docker-compose，并校验 GPU DeviceRequests。
#      如确需指定 GPU，可在打包时设置 GPU_DEVICE_IDS="1"。
#   4. 生成的 compose 使用相对路径，可整体迁移。
#
# 用法（源服务器）：
#   bash pack.sh
#
# 可选环境变量：
#   SOURCE_DIR=/data/ganttly/qwen3-asr
#   OUTPUT_DIR=$(pwd)
#   CONTAINER=qwen3-asr-serve  # 源服务器上用于抓取配置的容器名
#   HOST_PORT=9851
#   SERVICE_NAME=jusha-asr-asr
#   DEPLOY_CONTAINER_NAME=jusha-asr-asr
#   DEPLOY_IMAGE=jusha-asr-asr:latest
#   GPU_COUNT=1
#   GPU_DEVICE_IDS=1          # 可选；设置后使用 device_ids，未设置时使用 count
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ---------- 配置 ----------
SOURCE_DIR="${SOURCE_DIR:-${SCRIPT_DIR}}"
CONTAINER="${CONTAINER:-qwen3-asr-serve}"
SOURCE_IMAGE="${SOURCE_IMAGE:-qwenllm/qwen3-asr:latest}"
SERVICE_NAME="${SERVICE_NAME:-jusha-asr-asr}"
DEPLOY_CONTAINER_NAME="${DEPLOY_CONTAINER_NAME:-jusha-asr-asr}"
DEPLOY_IMAGE="${DEPLOY_IMAGE:-jusha-asr-asr:latest}"
HOST_PORT="${HOST_PORT:-9851}"
GPU_COUNT="${GPU_COUNT:-1}"
GPU_DEVICE_IDS="${GPU_DEVICE_IDS:-}"
RELEASE_VERSION="${RELEASE_VERSION:-}"
MODEL_SUBDIR="${MODEL_SUBDIR:-}"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
BUNDLE_VERSION="${RELEASE_VERSION:-${TIMESTAMP}}"
BUNDLE_NAME="jusha-asr-asr-${BUNDLE_VERSION}"
PACK_WORK_ROOT="${QWEN3_ASR_PACK_WORK_ROOT:-${TMPDIR:-/tmp}}"
PACK_WORK_ROOT="$(mkdir -p "${PACK_WORK_ROOT}" && cd "${PACK_WORK_ROOT}" && pwd)"
WORK_DIR="${PACK_WORK_ROOT}/${BUNDLE_NAME}-${TIMESTAMP}"
OUTPUT_DIR="${OUTPUT_DIR:-$(pwd)}"
TARBALL="${OUTPUT_DIR}/${BUNDLE_NAME}.tar.gz"
INSTALLER="${OUTPUT_DIR}/${BUNDLE_NAME}.run"

# ---------- 工具 ----------
info()  { echo -e "\033[0;32m[INFO]\033[0m $*"; }
warn()  { echo -e "\033[0;33m[WARN]\033[0m $*"; }
error() { echo -e "\033[0;31m[ERR ]\033[0m $*" >&2; }

bytes_to_gib() {
    awk -v bytes="$1" 'BEGIN { printf "%.1fGB", bytes / 1024 / 1024 / 1024 }'
}

check_available_space() {
    local path="$1"
    local required_bytes="$2"
    local available_bytes

    [ "${required_bytes}" -gt 0 ] || return 0
    available_bytes=$(df -PB1 "${path}" 2>/dev/null | awk 'NR == 2 {print $4}')
    [ -n "${available_bytes}" ] || return 0

    if [ "${available_bytes}" -lt "${required_bytes}" ]; then
        error "工作目录空间不足: ${path}"
        error "  可用: $(bytes_to_gib "${available_bytes}")，至少需要: $(bytes_to_gib "${required_bytes}")"
        error "  请清理磁盘，或设置 QWEN3_ASR_PACK_WORK_ROOT / TMPDIR 到空间更大的目录"
        exit 1
    fi
}

escape_yaml_double_quoted() {
    # 用于 YAML 双引号字符串，避免 command/env 中出现反斜杠或双引号时破坏 yml。
    sed 's/\\/\\\\/g; s/"/\\"/g'
}

detect_default_model_subdir() {
    if [ -n "${MODEL_SUBDIR}" ]; then
        printf '%s\n' "${MODEL_SUBDIR}"
        return 0
    fi

    if [ -d "${SOURCE_DIR}/models/Qwen3-ASR-1.7B" ]; then
        printf '%s\n' 'Qwen3-ASR-1.7B'
        return 0
    fi

    find "${SOURCE_DIR}/models" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' 2>/dev/null | sort | head -n 1
}

# ---------- 预检 ----------
command -v docker >/dev/null || { error "docker 未安装"; exit 1; }
[ -d "${SOURCE_DIR}" ]       || { error "源目录不存在: ${SOURCE_DIR}"; exit 1; }

PRIMARY_MODEL_SUBDIR="$(detect_default_model_subdir || true)"
[ -n "${PRIMARY_MODEL_SUBDIR}" ] || { error "${SOURCE_DIR}/models 下未找到可打包模型目录"; exit 1; }

CONTAINER_EXISTS=0
if docker inspect "${CONTAINER}" >/dev/null 2>&1; then
    CONTAINER_EXISTS=1
fi

if [ "${CONTAINER_EXISTS}" -eq 1 ]; then
    SOURCE_IMAGE_REF=$(docker inspect --format '{{.Config.Image}}' "${CONTAINER}")
else
    SOURCE_IMAGE_REF="${SOURCE_IMAGE}"
    docker image inspect "${SOURCE_IMAGE_REF}" >/dev/null 2>&1 || {
        error "源容器不存在且本地镜像不存在: ${CONTAINER} / ${SOURCE_IMAGE_REF}"
        exit 1
    }
fi

mkdir -p "${OUTPUT_DIR}"

info "源目录:   ${SOURCE_DIR}"
if [ "${CONTAINER_EXISTS}" -eq 1 ]; then
    info "源容器:   ${CONTAINER}"
else
    warn "源容器不存在，回退为本地镜像模式: ${SOURCE_IMAGE_REF}"
fi
info "源镜像:   ${SOURCE_IMAGE_REF}"
info "模型目录: ${PRIMARY_MODEL_SUBDIR}"
info "服务名:   ${SERVICE_NAME}"
info "部署容器: ${DEPLOY_CONTAINER_NAME}"
info "部署镜像: ${DEPLOY_IMAGE}"
info "宿主端口: ${HOST_PORT}"
info "输出:     ${INSTALLER}"
if [ -n "${GPU_DEVICE_IDS}" ]; then
    info "GPU:      device_ids=[${GPU_DEVICE_IDS}]"
else
    info "GPU:      count=${GPU_COUNT}"
fi
echo

rm -rf "${WORK_DIR}"
mkdir -p "${WORK_DIR}"/{images,models}

# =============================================================================
# 1. 导出镜像
# =============================================================================
info "[1/5] 导出 Docker 镜像"
IMAGE="${SOURCE_IMAGE_REF}"
IMAGE_FILE=$(echo "${DEPLOY_IMAGE}" | tr '/:' '__').tar
image_size_bytes=$(docker image inspect "${IMAGE}" --format '{{.Size}}' 2>/dev/null || echo 0)
case "${image_size_bytes}" in
    ''|*[!0-9]*) image_size_bytes=0 ;;
esac
size_hint=$(bytes_to_gib "${image_size_bytes}")
info "  ${IMAGE} -> ${DEPLOY_IMAGE} (${size_hint})"
check_available_space "${WORK_DIR}/images" "$((image_size_bytes + 1024 * 1024 * 1024))"
docker tag "${IMAGE}" "${DEPLOY_IMAGE}"
docker save "${DEPLOY_IMAGE}" -o "${WORK_DIR}/images/${IMAGE_FILE}"

# =============================================================================
# 2. 捕获容器启动参数、端口、环境变量、挂载
# =============================================================================
info "[2/5] 捕获容器配置"

# 容器 Cmd。每行一个参数；如果原 command 是 shell 字符串，通常会表现为单行长字符串。
if [ "${CONTAINER_EXISTS}" -eq 1 ]; then
    container_cmd=$(docker inspect --format \
        '{{if .Config.Cmd}}{{range .Config.Cmd}}{{.}}{{"\n"}}{{end}}{{end}}' \
        "${CONTAINER}")
else
    container_cmd=$(docker image inspect --format \
        '{{if .Config.Cmd}}{{range .Config.Cmd}}{{.}}{{"\n"}}{{end}}{{end}}' \
        "${IMAGE}" 2>/dev/null || true)
fi

if [ -z "${container_cmd}" ]; then
    container_cmd=$(printf 'qwen-asr-serve\n/models/%s\n--host\n0.0.0.0\n--port\n8000\n--gpu-memory-utilization\n0.45\n--max-model-len\n8192\n' "${PRIMARY_MODEL_SUBDIR}")
fi

# 智能识别容器内部服务端口：command --port > ExposedPorts > 8000
cmd_port=""
if [ -n "${container_cmd}" ]; then
    # 同时兼容两种形式：
    #   --port\n8000
    #   qwen-asr-serve ... --port 8000 ...
    cmd_port=$(printf '%s\n' "${container_cmd}" | awk '
        BEGIN { prev_port = 0 }
        $0 == "--port" { prev_port = 1; next }
        prev_port == 1 && $0 ~ /^[0-9]+$/ { print $0; exit }
        {
          for (i = 1; i <= NF; i++) {
            if ($i == "--port" && (i + 1) <= NF && $(i + 1) ~ /^[0-9]+$/) {
              print $(i + 1); exit
            }
          }
        }
    ' || true)
fi

if [ "${CONTAINER_EXISTS}" -eq 1 ]; then
    exposed_port=$(docker inspect --format \
        '{{range $p, $_ := .Config.ExposedPorts}}{{$p}}{{"\n"}}{{end}}' \
        "${CONTAINER}" | head -1 | sed 's|/tcp||' || true)
else
    exposed_port=$(docker image inspect --format \
        '{{range $p, $_ := .Config.ExposedPorts}}{{$p}}{{"\n"}}{{end}}' \
        "${IMAGE}" | head -1 | sed 's|/tcp||' || true)
fi

container_port="${cmd_port:-${exposed_port:-8000}}"
info "  容器服务端口: ${container_port}"

# 容器环境变量（过滤系统/NVIDIA/CUDA 注入变量）
user_envs=()
while IFS= read -r env_kv; do
    [ -z "${env_kv}" ] && continue
    case "${env_kv}" in
        PATH=*|HOSTNAME=*|HOME=*|TERM=*|LANG=*|LC_*=*) continue ;;
        NVIDIA_*=*|CUDA_*=*|NVARCH=*|NV_*=*) continue ;;
    esac
    user_envs+=("${env_kv}")
done < <(
    if [ "${CONTAINER_EXISTS}" -eq 1 ]; then
        docker inspect --format '{{range .Config.Env}}{{.}}{{"\n"}}{{end}}' "${CONTAINER}"
    else
        docker image inspect --format '{{range .Config.Env}}{{.}}{{"\n"}}{{end}}' "${IMAGE}" 2>/dev/null || true
    fi
)

# shm_size
if [ "${CONTAINER_EXISTS}" -eq 1 ]; then
    shm_size=$(docker inspect --format '{{.HostConfig.ShmSize}}' "${CONTAINER}" 2>/dev/null || echo "0")
else
    shm_size=$((4 * 1024 * 1024 * 1024))
fi

# 容器挂载。优先从 HostConfig.Binds 读取，因为它最接近 compose 的 bind 语义。
model_mounts=()
if [ "${CONTAINER_EXISTS}" -eq 1 ]; then
    while IFS= read -r bind; do
        [ -z "${bind}" ] && continue
        host_path="${bind%%:*}"
        rest="${bind#*:}"
        container_path="${rest%%:*}"
        # 只迁移 /models 下的模型挂载，避免把日志、缓存等杂项错误放进 ./models。
        case "${container_path}" in
            /models|/models/*)
                model_subdir=$(basename "${host_path}")
                model_mounts+=("./models/${model_subdir}:${container_path}")
                ;;
        esac
    done < <(docker inspect --format '{{range .HostConfig.Binds}}{{.}}{{"\n"}}{{end}}' "${CONTAINER}")
fi

# 如果 inspect 没拿到 binds，则按默认模型目录兜底。
if [ ${#model_mounts[@]} -eq 0 ]; then
    warn "  未从运行中容器识别到 /models bind mount，使用默认挂载 ./models/${PRIMARY_MODEL_SUBDIR}:/models/${PRIMARY_MODEL_SUBDIR}"
    model_mounts+=("./models/${PRIMARY_MODEL_SUBDIR}:/models/${PRIMARY_MODEL_SUBDIR}")
fi

# =============================================================================
# 3. 生成 docker-compose.yml（使用相对路径 + 动态网络变量）
# =============================================================================
info "[3/5] 生成 docker-compose.yml"

COMPOSE_FILE="${WORK_DIR}/docker-compose.yml"

{
    cat << COMPOSE_HEAD
services:
  ${SERVICE_NAME}:
    image: ${DEPLOY_IMAGE}
    container_name: ${DEPLOY_CONTAINER_NAME}
    restart: unless-stopped
COMPOSE_HEAD

    # shm_size
    if [ -n "${shm_size}" ] && [ "${shm_size}" != "0" ]; then
        # 尽量保留为 gb；不足 1GB 时转 mb。
        if [ "${shm_size}" -ge 1073741824 ]; then
            shm_gb=$(awk "BEGIN{printf \"%.0f\", ${shm_size}/1024/1024/1024}")
            echo "    shm_size: '${shm_gb}gb'"
        else
            shm_mb=$(awk "BEGIN{printf \"%.0f\", ${shm_size}/1024/1024}")
            echo "    shm_size: '${shm_mb}mb'"
        fi
    fi

    # 端口
    echo "    ports:"
    echo "      - \"${HOST_PORT}:${container_port}\""

    # 卷挂载
    echo "    volumes:"
    for mount in "${model_mounts[@]}"; do
        echo "      - ${mount}"
    done

    # 环境变量
    if [ ${#user_envs[@]} -gt 0 ]; then
        echo "    environment:"
        for kv in "${user_envs[@]}"; do
            key="${kv%%=*}"
            value="${kv#*=}"
            safe_value=$(printf '%s' "${value}" | escape_yaml_double_quoted)
            echo "      ${key}: \"${safe_value}\""
        done
    fi

    # GPU。默认 count: 1 更适合客户机，避免源机器 GPU ID 在目标机不存在。
    echo "    deploy:"
    echo "      resources:"
    echo "        reservations:"
    echo "          devices:"
    echo "            - driver: nvidia"
    if [ -n "${GPU_DEVICE_IDS}" ]; then
        ids_yaml=$(printf '%s' "${GPU_DEVICE_IDS}" | awk -F',' '{
            printf "[";
            for (i = 1; i <= NF; i++) {
                gsub(/^ +| +$/, "", $i);
                if ($i != "") {
                    if (n++ > 0) printf ", ";
                    printf "\047%s\047", $i;
                }
            }
            printf "]";
        }')
        echo "              device_ids: ${ids_yaml}"
    else
        echo "              count: ${GPU_COUNT}"
    fi
    echo "              capabilities: [gpu]"

    # 动态网络：deploy.sh 会检测不冲突的子网并写入 .env。
    echo "    networks:"
    echo "      - jusha_asr_net"

    # command
    if [ -n "${container_cmd}" ]; then
        line_count=$(printf '%s\n' "${container_cmd}" | sed '/^$/d' | wc -l | awk '{print $1}')
        if [ "${line_count}" -eq 1 ] && printf '%s' "${container_cmd}" | grep -q '[[:space:]]'; then
            # 原 command 很可能是 shell/string 形式，保持字符串形式。
            echo "    command: >"
            printf '%s\n' "${container_cmd}" | sed '/^$/d' | sed 's/^/      /'
        else
            # exec array 形式。
            echo "    command:"
            while IFS= read -r line; do
                [ -z "${line}" ] && continue
                safe_line=$(printf '%s' "${line}" | escape_yaml_double_quoted)
                echo "      - \"${safe_line}\""
            done <<< "${container_cmd}"
        fi
    fi

    cat << 'COMPOSE_NET'

networks:
    jusha_asr_net:
        name: ${QWEN3_ASR_NETWORK_NAME:-jusha-asr}
        external: true
COMPOSE_NET
} > "${COMPOSE_FILE}"

info "  生成完成"

# =============================================================================
# 4. 拷贝模型
# =============================================================================
info "[4/5] 拷贝模型"
if [ -d "${SOURCE_DIR}/models" ]; then
    models_size=$(du -sh "${SOURCE_DIR}/models" | cut -f1)
    info "  模型目录大小: ${models_size}"
    cp -a "${SOURCE_DIR}/models/." "${WORK_DIR}/models/"
else
    warn "  ${SOURCE_DIR}/models 不存在，跳过"
fi

# =============================================================================
# 5. 生成 deploy.sh、README，打包
# =============================================================================
info "[5/5] 生成部署脚本并打包"

cat > "${WORK_DIR}/deploy.sh" << 'DEPLOY_EOF'
#!/usr/bin/env bash
# Qwen3-ASR 一键部署（目标机执行）
set -euo pipefail

BUNDLE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE_PATH="${BUNDLE_ROOT}/docker-compose.yml"
ENV_FILE="${BUNDLE_ROOT}/.env"
SERVICE_NAME="__SERVICE_NAME__"

COMPOSE_BIN=""
COMPOSE_SUPPORTS_ENV_FILE="true"
COMPOSE_LEGACY_V1="false"

info()  { echo -e "\033[0;32m[INFO]\033[0m $*"; }
warn()  { echo -e "\033[0;33m[WARN]\033[0m $*"; }
error() { echo -e "\033[0;31m[ERR ]\033[0m $*" >&2; }

run_compose_raw() {
    local cmd="$1"
    shift

    case "${cmd}" in
        "docker compose")
            docker compose "$@"
            ;;
        "docker-compose")
            docker-compose "$@"
            ;;
        *)
            error "未知 compose 命令: ${cmd}"
            return 127
            ;;
    esac
}

compose_version_text() {
    local cmd="$1"
    local v=""

    v="$(run_compose_raw "${cmd}" version --short 2>/dev/null || true)"
    if [ -z "${v}" ]; then
        v="$(run_compose_raw "${cmd}" version 2>/dev/null | grep -oE 'v?[0-9]+(\.[0-9]+)+' | head -1 || true)"
    fi

    echo "${v:-unknown}"
}

compose_major_version() {
    local version="$1"
    version="${version#v}"
    echo "${version%%.*}"
}

try_compose_candidate() {
    local cmd="$1"
    local version
    local major
    local err_file
    local out_file

    version="$(compose_version_text "${cmd}")"
    major="$(compose_major_version "${version}")"

    info "检测 Compose 候选: ${cmd} (${version})"

    if [ "${cmd}" = "docker-compose" ] && [ "${major}" = "1" ]; then
        warn "  docker-compose v1 是旧版实现，GPU deploy.resources 可能不会被正确转换为 Docker DeviceRequests"
        warn "  脚本不会直接否定它，但会在启动后做 GPU DeviceRequests 校验"
    fi

    err_file="$(mktemp)"
    out_file="$(mktemp)"

    # 优先使用显式 --env-file。高版本 docker compose / docker-compose 都应支持。
    if (cd "${BUNDLE_ROOT}" && run_compose_raw "${cmd}" --env-file "${ENV_FILE}" -f "${COMPOSE_FILE_PATH}" config >"${out_file}" 2>"${err_file}"); then
        COMPOSE_BIN="${cmd}"
        COMPOSE_SUPPORTS_ENV_FILE="true"
        [ "${major}" = "1" ] && COMPOSE_LEGACY_V1="true" || COMPOSE_LEGACY_V1="false"
        rm -f "${err_file}" "${out_file}"
        return 0
    fi

    # 兼容极老版本：不用 --env-file，依赖当前目录 .env 自动加载。
    if (cd "${BUNDLE_ROOT}" && run_compose_raw "${cmd}" -f "${COMPOSE_FILE_PATH}" config >"${out_file}" 2>"${err_file}"); then
        COMPOSE_BIN="${cmd}"
        COMPOSE_SUPPORTS_ENV_FILE="false"
        [ "${major}" = "1" ] && COMPOSE_LEGACY_V1="true" || COMPOSE_LEGACY_V1="false"
        warn "  ${cmd} 不支持或未接受 --env-file，已回退为包目录 .env 自动加载"
        rm -f "${err_file}" "${out_file}"
        return 0
    fi

    warn "  ${cmd} 无法解析当前 docker-compose.yml"
    sed 's/^/    /' "${err_file}" >&2 || true
    rm -f "${err_file}" "${out_file}"
    return 1
}

select_compose_cmd() {
    local candidates=()

    if docker compose version >/dev/null 2>&1; then
        candidates+=("docker compose")
    fi

    if command -v docker-compose >/dev/null 2>&1; then
        candidates+=("docker-compose")
    fi

    if [ ${#candidates[@]} -eq 0 ]; then
        error "未检测到 docker compose 或 docker-compose"
        error "推荐安装官方 compose 插件：sudo apt-get install docker-compose-plugin"
        exit 1
    fi

    local cmd
    for cmd in "${candidates[@]}"; do
        if try_compose_candidate "${cmd}"; then
            info "使用 Compose: ${COMPOSE_BIN} ($(compose_version_text "${COMPOSE_BIN}"))"
            return 0
        fi
    done

    error "所有 Compose 候选都无法解析当前 docker-compose.yml"
    error "建议升级 Docker Compose V2，或执行以下命令查看具体错误："
    error "  cd ${BUNDLE_ROOT}"
    error "  docker compose --env-file .env -f docker-compose.yml config"
    exit 1
}

compose_call() {
    if [ -z "${COMPOSE_BIN}" ]; then
        error "COMPOSE_BIN 未初始化"
        return 1
    fi

    if [ "${COMPOSE_SUPPORTS_ENV_FILE}" = "true" ]; then
        run_compose_raw "${COMPOSE_BIN}" --env-file "${ENV_FILE}" "$@"
    else
        (cd "${BUNDLE_ROOT}" && run_compose_raw "${COMPOSE_BIN}" "$@")
    fi
}

collect_used_cidrs() {
    # 1. 宿主机路由表：客户机 VPN / 内网 / 物理网段都在这里体现。
    if command -v ip >/dev/null 2>&1; then
        ip -o -4 route show 2>/dev/null | awk '{print $1}' | while read -r dst; do
            [ -z "${dst}" ] && continue
            [ "${dst}" = "default" ] && continue
            if echo "${dst}" | grep -q '/'; then
                echo "${dst}"
            elif echo "${dst}" | grep -Eq '^[0-9]+(\.[0-9]+){3}$'; then
                echo "${dst}/32"
            fi
        done

        # 2. 网卡自身地址。某些环境路由不完整时，用地址兜底。
        ip -o -4 addr show 2>/dev/null | awk '{print $4}' | sed '/^$/d'
    fi

    # 3. Docker 已有 bridge 网络，避免和客户机已有 Docker 服务冲突。
    docker network inspect $(docker network ls -q) \
        --format '{{range .IPAM.Config}}{{if .Subnet}}{{.Subnet}}{{"\n"}}{{end}}{{end}}' \
        2>/dev/null | sed '/^$/d' || true
}

existing_network_subnet() {
    local network_name="$1"
    docker network inspect -f '{{range .IPAM.Config}}{{println .Subnet}}{{end}}' "${network_name}" 2>/dev/null | sed '/^$/d' | head -1 || true
}

gateway_for_subnet() {
    local subnet="$1"
    python3 - "${subnet}" << 'PY'
import ipaddress
import sys

network = ipaddress.ip_network(sys.argv[1], strict=False)
print(network.network_address + 1)
PY
}

ensure_shared_network() {
    local network_name="$1"
    local subnet="$2"
    local gateway="${QWEN3_ASR_GATEWAY:-}"

    if docker network inspect "${network_name}" >/dev/null 2>&1; then
        return 0
    fi

    if [ -z "${gateway}" ]; then
        gateway="$(gateway_for_subnet "${subnet}")"
    fi

    docker network create --driver bridge --subnet "${subnet}" --gateway "${gateway}" "${network_name}" >/dev/null
}

choose_free_subnet() {
    local used_file
    used_file=$(mktemp)
    collect_used_cidrs | sort -u > "${used_file}"

    if command -v python3 >/dev/null 2>&1; then
        python3 - "${used_file}" << 'PY'
import ipaddress
import sys

used_file = sys.argv[1]
used = []
with open(used_file, "r", encoding="utf-8") as f:
    for line in f:
        s = line.strip()
        if not s or s == "default":
            continue
        try:
            used.append(ipaddress.ip_network(s, strict=False))
        except Exception:
            pass

candidates = []

# 第一优先级：避开 172.16.0.0/12，解决客户机 172.x 内网/VPN 与 Docker 默认网段冲突问题。
# 选择高位 10.x / 192.168.x，降低与常见办公网段冲突概率。
for third in range(240, 256):
    candidates.append(f"10.255.{third}.0/24")
for second in range(200, 256):
    candidates.append(f"10.{second}.255.0/24")
for third in range(240, 256):
    candidates.append(f"192.168.{third}.0/24")

# 第二优先级：CGNAT 地址段。若宿主机已有路由会被 overlap 过滤。
for third in range(240, 256):
    candidates.append(f"100.64.{third}.0/24")

# 最后兜底才使用 172.30/31 的高位 /24。
for second in range(30, 32):
    for third in range(240, 256):
        candidates.append(f"172.{second}.{third}.0/24")

for cidr in candidates:
    net = ipaddress.ip_network(cidr, strict=True)
    if all(not net.overlaps(u) for u in used):
        print(cidr)
        sys.exit(0)

sys.exit(2)
PY
        local rc=$?
        rm -f "${used_file}"
        return ${rc}
    fi

    rm -f "${used_file}"
    error "目标机未安装 python3，无法进行可靠 CIDR 冲突检测"
    error "请安装 python3，或手动指定：QWEN3_ASR_SUBNET=10.255.240.0/24 bash deploy.sh"
    return 2
}

prepare_network_env() {
    local subnet="${QWEN3_ASR_SUBNET:-}"
    local net_name="${QWEN3_ASR_NETWORK_NAME:-jusha-asr}"

    if [ -z "${subnet}" ]; then
        subnet="$(existing_network_subnet "${net_name}")"
        if [ -n "${subnet}" ]; then
            info "复用已存在 Docker 网络 ${net_name}: ${subnet}"
        else
            info "检测宿主机路由和 Docker 网络，自动选择不冲突的容器网段..."
            subnet=$(choose_free_subnet) || {
                error "无法自动找到可用 Docker 子网"
                error "可手动指定，例如：QWEN3_ASR_SUBNET=10.255.250.0/24 bash deploy.sh"
                exit 1
            }
        fi
    fi

    ensure_shared_network "${net_name}" "${subnet}"

    cat > "${ENV_FILE}" << ENV
QWEN3_ASR_SUBNET=${subnet}
QWEN3_ASR_NETWORK_NAME=${net_name}
ENV

    info "容器网络: ${net_name}"
    info "容器子网: ${subnet}"
}

check_host_port() {
    local host_port
    host_port=$(grep -E '^[[:space:]]+- "?[0-9]+:[0-9]+"?' "${COMPOSE_FILE_PATH}" | head -1 | sed -E 's/.*"?([0-9]+):[0-9]+"?.*/\1/' || true)
    [ -z "${host_port}" ] && return 0

    if command -v ss >/dev/null 2>&1; then
        if ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "(^|:)${host_port}$"; then
            error "宿主机端口 ${host_port} 已被占用，请修改 docker-compose.yml 的 ports 后重试"
            exit 1
        fi
    fi
}

gpu_requested_in_compose() {
    grep -Eq 'capabilities:[[:space:]]*\[[^]]*gpu[^]]*\]' "${COMPOSE_FILE_PATH}" || \
    grep -Eq '^[[:space:]]*-[[:space:]]*gpu[[:space:]]*$' "${COMPOSE_FILE_PATH}"
}

parse_gpu_device_ids_from_compose() {
    # 仅解析本脚本生成的 inline device_ids: ['0', '1'] 形式。
    grep -E '^[[:space:]]*device_ids:' "${COMPOSE_FILE_PATH}" | head -1 | \
        sed -E "s/.*\[(.*)\].*/\1/" | tr -d "'\"" | tr ',' ' ' | xargs || true
}

parse_gpu_count_from_compose() {
    grep -E '^[[:space:]]*count:[[:space:]]*[0-9]+' "${COMPOSE_FILE_PATH}" | head -1 | \
        sed -E 's/.*count:[[:space:]]*([0-9]+).*/\1/' || true
}

host_gpu_count() {
    if command -v nvidia-smi >/dev/null 2>&1; then
        nvidia-smi -L 2>/dev/null | grep -c '^GPU ' || true
    else
        echo "0"
    fi
}

check_gpu_environment() {
    if ! gpu_requested_in_compose; then
        info "当前 compose 未声明 GPU 设备，跳过 GPU 检测"
        return 0
    fi

    info "检测 GPU 环境"

    local gpu_count
    gpu_count="$(host_gpu_count)"

    if command -v nvidia-smi >/dev/null 2>&1; then
        if [ "${gpu_count}" -gt 0 ]; then
            info "  NVIDIA 驱动正常，检测到 GPU 数量: ${gpu_count}"
            nvidia-smi -L 2>/dev/null | sed 's/^/    /' || true
        else
            warn "  nvidia-smi 存在，但未检测到可用 GPU"
        fi
    else
        warn "  未检测到 nvidia-smi，可能未安装 NVIDIA 驱动或当前环境不是 NVIDIA GPU 机器"
    fi

    if docker run --help 2>/dev/null | grep -q -- '--gpus'; then
        info "  Docker CLI 支持 --gpus 参数"
    else
        warn "  Docker CLI 未检测到 --gpus 参数，Docker 版本可能过旧"
    fi

    if command -v nvidia-container-cli >/dev/null 2>&1; then
        if nvidia-container-cli info >/dev/null 2>&1; then
            info "  NVIDIA Container Toolkit 基础检查通过: nvidia-container-cli info"
        else
            warn "  nvidia-container-cli 存在，但 info 检查失败；可能 runtime 未正确配置"
        fi
    elif command -v nvidia-ctk >/dev/null 2>&1 || command -v nvidia-container-runtime >/dev/null 2>&1; then
        info "  检测到 NVIDIA Container Toolkit 相关命令"
    else
        warn "  未检测到 nvidia-container-cli / nvidia-ctk / nvidia-container-runtime"
        warn "  如容器启动后无法使用 GPU，请安装并配置 nvidia-container-toolkit"
    fi

    local device_ids
    device_ids="$(parse_gpu_device_ids_from_compose)"
    if [ -n "${device_ids}" ] && [ "${gpu_count}" -gt 0 ]; then
        local id
        for id in ${device_ids}; do
            if ! echo "${id}" | grep -Eq '^[0-9]+$'; then
                warn "  device_ids 中包含非数字 GPU ID: ${id}，请确认目标机支持该写法"
                continue
            fi
            if [ "${id}" -ge "${gpu_count}" ]; then
                error "compose 指定了 GPU ${id}，但目标机只有 ${gpu_count} 块 GPU，可用 ID 范围是 0 到 $((gpu_count - 1))"
                error "请修改 docker-compose.yml 的 device_ids，或重新打包时不要硬编码 GPU_DEVICE_IDS"
                exit 1
            fi
        done
        info "  GPU device_ids 合法: ${device_ids}"
    fi

    local requested_count
    requested_count="$(parse_gpu_count_from_compose)"
    if [ -n "${requested_count}" ] && [ "${gpu_count}" -gt 0 ] && [ "${requested_count}" -gt "${gpu_count}" ]; then
        error "compose 请求 GPU count=${requested_count}，但目标机只有 ${gpu_count} 块 GPU"
        exit 1
    fi
}

verify_container_gpu_request() {
    if ! gpu_requested_in_compose; then
        return 0
    fi

    local cid
    cid="$(compose_call -f "${COMPOSE_FILE_PATH}" ps -q "${SERVICE_NAME}" 2>/dev/null || true)"

    if [ -z "${cid}" ]; then
        warn "未找到服务容器 ID，跳过 GPU DeviceRequests 校验"
        return 0
    fi

    local device_requests
    device_requests="$(docker inspect --format '{{json .HostConfig.DeviceRequests}}' "${cid}" 2>/dev/null || true)"

    if echo "${device_requests}" | grep -qi 'gpu'; then
        info "GPU DeviceRequests 已写入容器配置"
    else
        warn "未在容器 HostConfig.DeviceRequests 中看到 GPU 请求"
        warn "这通常说明当前 Compose 实现没有正确应用 deploy.resources.reservations.devices"
        warn "建议升级 Docker Compose V2，或检查 docker-compose 是否为过旧 v1 版本"
    fi
}

echo
info "=============================="
info "  Qwen3-ASR 一键部署"
info "=============================="
info "包目录: ${BUNDLE_ROOT}"
echo

# 1. 预检 Docker
command -v docker >/dev/null || { error "docker 未安装"; exit 1; }
docker info >/dev/null 2>&1 || { error "当前用户无法访问 Docker，请检查 Docker 服务或用户权限"; exit 1; }

# 2. 准备动态网络，规避客户机现有路由冲突
prepare_network_env

# 3. 选择可用 Compose。注意：docker-compose 高版本也可能是 V2，不能简单视为旧版。
select_compose_cmd

# 4. 检查 GPU 环境。这里是软/硬结合检查：明显错误直接中断，不确定项只警告。
check_gpu_environment

# 5. 检查宿主机端口占用
check_host_port

# 6. 加载镜像
info "[1/2] 加载 Docker 镜像"
shopt -s nullglob
image_files=("${BUNDLE_ROOT}"/images/*.tar)
if [ ${#image_files[@]} -eq 0 ]; then
    error "未找到镜像文件: ${BUNDLE_ROOT}/images/*.tar"
    exit 1
fi
for tarfile in "${image_files[@]}"; do
    size=$(du -h "${tarfile}" | cut -f1)
    info "  load: $(basename "${tarfile}") (${size})"
    docker load -i "${tarfile}"
done

# 7. 启动
info "[2/2] 启动容器"
cd "${BUNDLE_ROOT}"
compose_call -f "${COMPOSE_FILE_PATH}" up -d

# 8. 启动后校验 Compose 是否真正把 GPU 请求写入 Docker 容器配置
verify_container_gpu_request

echo
info "=============================="
info "  部署完成 ✅"
info "=============================="
info "容器状态:"
compose_call -f "${COMPOSE_FILE_PATH}" ps

echo
info "常用命令（须在包目录 ${BUNDLE_ROOT} 下执行）:"
if [ "${COMPOSE_SUPPORTS_ENV_FILE}" = "true" ]; then
    info "  查看日志: ${COMPOSE_BIN} --env-file .env -f docker-compose.yml logs -f"
    info "  查看状态: ${COMPOSE_BIN} --env-file .env -f docker-compose.yml ps"
    info "  停止:     ${COMPOSE_BIN} --env-file .env -f docker-compose.yml down"
    info "  启动:     ${COMPOSE_BIN} --env-file .env -f docker-compose.yml up -d"
    info "  重启:     ${COMPOSE_BIN} --env-file .env -f docker-compose.yml restart"
else
    info "  查看日志: ${COMPOSE_BIN} -f docker-compose.yml logs -f"
    info "  查看状态: ${COMPOSE_BIN} -f docker-compose.yml ps"
    info "  停止:     ${COMPOSE_BIN} -f docker-compose.yml down"
    info "  启动:     ${COMPOSE_BIN} -f docker-compose.yml up -d"
    info "  重启:     ${COMPOSE_BIN} -f docker-compose.yml restart"
fi
echo
DEPLOY_EOF

# 写入真实服务名
sed -i "s|__SERVICE_NAME__|${SERVICE_NAME}|g" "${WORK_DIR}/deploy.sh"

chmod +x "${WORK_DIR}/deploy.sh"

cat > "${WORK_DIR}/README.md" << 'README_EOF'
# Jusha ASR 推理服务离线部署包

## 一键部署

```bash
bash jusha-asr-asr-*.run
```

或手动解压：

```bash
tar xzf jusha-asr-asr-*.tar.gz
cd jusha-asr-asr-*
./deploy.sh
```

## 前置要求

- Linux（推荐 Ubuntu 22.04/24.04）
- Docker 24+，带 compose 插件
- NVIDIA 驱动 + nvidia-container-toolkit
- GPU 显存满足模型推理要求

## 网络避让机制

部署脚本会在目标机启动前扫描：

- 宿主机 IPv4 路由表
- 宿主机网卡 IPv4 地址
- Docker 已有网络的 IPAM 子网

然后自动选择一个不重叠的 Docker bridge 子网，并写入 `.env`：

```bash
QWEN3_ASR_SUBNET=10.255.240.0/24
QWEN3_ASR_NETWORK_NAME=jusha-asr
```

这样可以避免客户机已有 172.x 内网、VPN、Docker 默认网段与本服务容器网络冲突。

如果需要手动指定：

```bash
QWEN3_ASR_SUBNET=10.255.250.0/24 bash deploy.sh
```

## 目录结构

```text
jusha-asr-asr-<timestamp>/
├── deploy.sh            一键部署入口
├── docker-compose.yml   服务编排，使用相对路径
├── .env                 首次部署时自动生成，记录自动选择的网络
├── images/              docker save 导出的镜像
├── models/              Qwen3-ASR 模型权重
└── README.md
```

## 运维

所有命令必须在包目录下执行：

```bash
cd /path/to/jusha-asr-asr-*

docker compose --env-file .env ps
docker compose --env-file .env logs -f
docker compose --env-file .env restart
docker compose --env-file .env down
docker compose --env-file .env up -d
```
README_EOF

# 打包 tar.gz
info "  生成 tar.gz..."
tar czf "${TARBALL}" -C "$(dirname "${WORK_DIR}")" "$(basename "${WORK_DIR}")"

# 生成自解压安装包
info "  生成自解压安装包..."
HEADER_TMP=$(mktemp)
cat > "${HEADER_TMP}" << 'HEADER_EOF'
#!/usr/bin/env bash
# Qwen3-ASR 自解压安装器
set -euo pipefail

SELF="${BASH_SOURCE[0]}"
EXTRACT_DIR="${EXTRACT_DIR:-$(pwd)/jusha-asr-asr}"

echo "[INFO] 解压到 ${EXTRACT_DIR}"
mkdir -p "${EXTRACT_DIR}"

ARCHIVE_LINE=$(awk '/^__ARCHIVE_BELOW__$/ {print NR + 1; exit 0; }' "${SELF}")
tail -n +${ARCHIVE_LINE} "${SELF}" | tar xzf - -C "${EXTRACT_DIR}" --strip-components=1

echo "[INFO] 开始部署"
cd "${EXTRACT_DIR}"
exec ./deploy.sh

exit 0

__ARCHIVE_BELOW__
HEADER_EOF

cat "${HEADER_TMP}" "${TARBALL}" > "${INSTALLER}"
chmod +x "${INSTALLER}"
rm -f "${HEADER_TMP}"

# =============================================================================
# 完成
# =============================================================================
size_tar=$(du -h "${TARBALL}" | cut -f1)
size_run=$(du -h "${INSTALLER}" | cut -f1)

echo
info "=============================="
info "  打包完成 ✅"
info "=============================="
info ""
info "  tar 包:        ${TARBALL}  (${size_tar})"
info "  自解压安装包:  ${INSTALLER}  (${size_run})"
info ""
info "目标服务器使用（推荐，一条命令）："
info "  bash $(basename "${INSTALLER}")"
info ""
info "或手动解压："
info "  tar xzf $(basename "${TARBALL}") && cd ${BUNDLE_NAME} && ./deploy.sh"
echo
info "生成的 docker-compose.yml 预览:"
echo "─────────────────────────────────"
cat "${COMPOSE_FILE}"
echo "─────────────────────────────────"