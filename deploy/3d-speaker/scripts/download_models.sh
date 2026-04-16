#!/bin/sh
# =============================================================================
# 模型权重离线下载脚本
# 在有网络的构建机上执行，下载完成后将 models/ 目录拷贝到离线环境
# =============================================================================
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
MODEL_DIR="${SCRIPT_DIR}/../models"
mkdir -p "${MODEL_DIR}"

PYTHON_BIN="${PYTHON_BIN:-python3}"

echo "=========================================="
echo " 3D-Speaker 模型权重下载"
echo " 目标目录: ${MODEL_DIR}"
echo "=========================================="

if ! command -v "${PYTHON_BIN}" >/dev/null 2>&1; then
	echo "[ERROR] 未找到 ${PYTHON_BIN}，请先安装 Python 3" >&2
	exit 1
fi

install_modelscope() {
	if [ -n "${PIP_INDEX_URL:-}" ] && [ -n "${PIP_TRUSTED_HOST:-}" ]; then
		"${PYTHON_BIN}" -m pip install -U modelscope -i "${PIP_INDEX_URL}" --trusted-host "${PIP_TRUSTED_HOST}"
		return
	fi
	if [ -n "${PIP_INDEX_URL:-}" ]; then
		"${PYTHON_BIN}" -m pip install -U modelscope -i "${PIP_INDEX_URL}"
		return
	fi
	if [ -n "${PIP_TRUSTED_HOST:-}" ]; then
		"${PYTHON_BIN}" -m pip install -U modelscope --trusted-host "${PIP_TRUSTED_HOST}"
		return
	fi
	"${PYTHON_BIN}" -m pip install -U modelscope
}

echo ""
echo "[0/5] 检查 ModelScope 依赖..."
if ! "${PYTHON_BIN}" -c "import modelscope" >/dev/null 2>&1; then
	echo "未检测到 modelscope，开始安装..."
	if ! "${PYTHON_BIN}" -m pip --version >/dev/null 2>&1; then
		echo "[ERROR] 当前 ${PYTHON_BIN} 缺少 pip，无法在宿主机安装 modelscope" >&2
		echo "[ERROR] 请改用容器模式: bash build.sh download-models-docker" >&2
		echo "[ERROR] 或者: DOWNLOAD_MODE=docker bash build.sh download-models" >&2
		exit 1
	fi
	install_modelscope
else
	echo "modelscope 已安装，跳过依赖安装"
fi

# ERes2NetV2（推荐，精度更高）
echo ""
echo "[1/3] 下载 ERes2NetV2 说话人嵌入模型..."
"${PYTHON_BIN}" -c "
from modelscope.hub.snapshot_download import snapshot_download
snapshot_download('iic/speech_eres2netv2_sv_zh-cn_16k-common', cache_dir='${MODEL_DIR}/eres2netv2')
print('ERes2NetV2 下载完成')
"

# CAM++（轻量备选）
echo ""
echo "[2/3] 下载 CAM++ 说话人嵌入模型..."
"${PYTHON_BIN}" -c "
from modelscope.hub.snapshot_download import snapshot_download
snapshot_download('iic/speech_campplus_sv_zh-cn_16k-common', cache_dir='${MODEL_DIR}/campplus')
print('CAM++ 下载完成')
"

# FSMN-VAD
echo ""
echo "[3/5] 下载 FSMN-VAD 语音活动检测模型..."
"${PYTHON_BIN}" -c "
from modelscope.hub.snapshot_download import snapshot_download
snapshot_download('iic/speech_fsmn_vad_zh-cn-16k-common-pytorch', cache_dir='${MODEL_DIR}/fsmn_vad')
print('FSMN-VAD 下载完成')
"

echo ""
echo "[4/5] 下载原生 diarization CAM++ 模型缓存..."
"${PYTHON_BIN}" -c "
from modelscope.hub.snapshot_download import snapshot_download
snapshot_download('iic/speech_campplus_sv_zh_en_16k-common_advanced', cache_dir='${MODEL_DIR}/native_cache')
print('native diarization CAM++ 下载完成')
"

echo ""
echo "[5/5] 下载原生 diarization VAD 模型缓存..."
"${PYTHON_BIN}" -c "
from modelscope.hub.snapshot_download import snapshot_download
snapshot_download('iic/speech_fsmn_vad_zh-cn-16k-common-pytorch', cache_dir='${MODEL_DIR}/native_cache')
print('native diarization VAD 下载完成')
"

echo ""
echo "=========================================="
echo " 所有模型下载完成"
echo " 目录: ${MODEL_DIR}"
du -sh "${MODEL_DIR}"/*
echo "=========================================="
echo ""
echo " 离线部署: 将 models/ 目录整体拷贝到目标服务器"
