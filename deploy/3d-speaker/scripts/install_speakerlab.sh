#!/usr/bin/env bash
set -euo pipefail

PIP_BIN="${1:-pip}"
PYTHON_BIN="${2:-${PYTHON_BIN:-}}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
SPEAKERLAB_REPO="${SPEAKERLAB_REPO:-https://github.com/modelscope/3D-Speaker.git}"
SPEAKERLAB_REF="${SPEAKERLAB_REF:-main}"
GIT_CLONE_RETRIES="${GIT_CLONE_RETRIES:-3}"
PIP_INSTALL_ARGS=(--retries 5)

info() {
    echo "[INFO] $1"
}

warn() {
    echo "[WARN] $1"
}

error() {
    echo "[ERROR] $1" >&2
}

resolve_python_bin() {
    if [ -n "${PYTHON_BIN}" ]; then
        echo "${PYTHON_BIN}"
        return
    fi

    local pip_dir
    pip_dir="$(dirname "${PIP_BIN}")"
    if [ -x "${pip_dir}/python" ]; then
        echo "${pip_dir}/python"
        return
    fi
    if [ -x "${pip_dir}/python3" ]; then
        echo "${pip_dir}/python3"
        return
    fi
    if command -v python3 >/dev/null 2>&1; then
        echo "python3"
        return
    fi
    if command -v python >/dev/null 2>&1; then
        echo "python"
        return
    fi

    error "未找到可用的 Python 解释器"
    exit 1
}

runtime_ready() {
    "${PYTHON_BIN}" - <<'PY' >/dev/null 2>&1
import importlib

for module_name in (
    "speakerlab.process.processor",
    "speakerlab.utils.builder",
    "speakerlab.process.cluster",
):
    importlib.import_module(module_name)
PY
}

find_local_source_tree() {
    local candidate

    for candidate in \
        "${SPEAKERLAB_SOURCE:-}" \
        "${PROJECT_DIR}/vendor/3D-Speaker" \
        "${PROJECT_DIR}/.cache/3D-Speaker" \
        "${PROJECT_DIR}/3D-Speaker" \
        "${SPEAKERLAB_HOME:-}"; do
        if [ -z "${candidate}" ]; then
            continue
        fi
        if [ -d "${candidate}/speakerlab" ]; then
            echo "${candidate}"
            return 0
        fi
    done

    return 1
}

install_runtime_deps() {
    info "安装 speakerlab 运行依赖..."
    "${PIP_BIN}" install "${PIP_INSTALL_ARGS[@]}" \
        "numpy<2.0.0" \
        "scikit-learn>=1.3,<1.6" \
        "fastcluster>=1.2,<1.3" \
        "umap-learn>=0.5,<0.6" \
        "hdbscan>=0.8,<0.9"
}

register_source_tree() {
    local source_dir="$1"
    local site_packages

    site_packages="$(${PYTHON_BIN} - <<'PY'
import sysconfig

print(sysconfig.get_paths()["purelib"])
PY
)"

    mkdir -p "${site_packages}"
    printf '%s\n' "${source_dir}" > "${site_packages}/speakerlab-source.pth"
    info "已注册 speakerlab 源码路径: ${source_dir}"
}

ensure_source_tree() {
    local source_dir="$1"
    local existing_source_dir
    local attempt=1

    if [ -d "${source_dir}/speakerlab" ]; then
        echo "${source_dir}"
        return
    fi

    if existing_source_dir="$(find_local_source_tree 2>/dev/null)"; then
        info "复用本地 3D-Speaker 源码: ${existing_source_dir}" >&2
        echo "${existing_source_dir}"
        return
    fi

    if [ -n "${SPEAKERLAB_SOURCE:-}" ]; then
        error "SPEAKERLAB_SOURCE 不是有效的 3D-Speaker 源码目录: ${source_dir}"
        exit 1
    fi

    mkdir -p "$(dirname "${source_dir}")"
    while [ "${attempt}" -le "${GIT_CLONE_RETRIES}" ]; do
        rm -rf "${source_dir}"
        info "拉取 3D-Speaker 源码: ${SPEAKERLAB_REPO} (${SPEAKERLAB_REF}) [${attempt}/${GIT_CLONE_RETRIES}]" >&2
        if GIT_TERMINAL_PROMPT=0 git clone --depth 1 --branch "${SPEAKERLAB_REF}" "${SPEAKERLAB_REPO}" "${source_dir}"; then
            break
        fi
        if [ "${attempt}" -lt "${GIT_CLONE_RETRIES}" ]; then
            warn "源码拉取失败，2 秒后重试" >&2
            sleep 2
        fi
        attempt=$((attempt + 1))
    done

    if [ ! -d "${source_dir}/speakerlab" ]; then
        error "源码目录不完整，未找到 speakerlab/: ${source_dir}"
        exit 1
    fi

    echo "${source_dir}"
}

install_local_wheel_if_present() {
    local wheel_path

    shopt -s nullglob
    local wheel_candidates=("${PROJECT_DIR}"/wheels/speakerlab-*.whl)
    shopt -u nullglob

    if [ ${#wheel_candidates[@]} -eq 0 ]; then
        return
    fi

    wheel_path="${wheel_candidates[0]}"
    info "安装本地 speakerlab wheel: ${wheel_path}"
    "${PIP_BIN}" install "${PIP_INSTALL_ARGS[@]}" "${wheel_path}"
}

resolve_source_dir() {
    if [ -n "${SPEAKERLAB_SOURCE:-}" ]; then
        echo "${SPEAKERLAB_SOURCE}"
        return
    fi

    echo "${SPEAKERLAB_HOME:-${PROJECT_DIR}/vendor/3D-Speaker}"
}

PYTHON_BIN="$(resolve_python_bin)"

if runtime_ready; then
    info "speakerlab 运行环境已就绪"
    exit 0
fi

install_local_wheel_if_present

if runtime_ready; then
    info "speakerlab wheel 安装完成"
    exit 0
fi

SOURCE_DIR="$(ensure_source_tree "$(resolve_source_dir)")"
install_runtime_deps
register_source_tree "${SOURCE_DIR}"

if runtime_ready; then
    info "speakerlab 源码模式安装完成"
    exit 0
fi

error "speakerlab 安装后仍不可用，请检查依赖或源码目录"
exit 1