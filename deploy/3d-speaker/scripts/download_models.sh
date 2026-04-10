#!/usr/bin/env bash
# =============================================================================
# 模型权重离线下载脚本
# 在有网络的构建机上执行，下载完成后将 models/ 目录拷贝到离线环境
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODEL_DIR="${SCRIPT_DIR}/../models"
mkdir -p "${MODEL_DIR}"

echo "=========================================="
echo " 3D-Speaker 模型权重下载"
echo " 目标目录: ${MODEL_DIR}"
echo "=========================================="

pip install -U modelscope -q 2>/dev/null

# ERes2NetV2（推荐，精度更高）
echo ""
echo "[1/3] 下载 ERes2NetV2 说话人嵌入模型..."
python3 -c "
from modelscope.hub.snapshot_download import snapshot_download
snapshot_download('iic/speech_eres2netv2_sv_zh-cn_16k-common', cache_dir='${MODEL_DIR}/eres2netv2')
print('ERes2NetV2 下载完成')
"

# CAM++（轻量备选）
echo ""
echo "[2/3] 下载 CAM++ 说话人嵌入模型..."
python3 -c "
from modelscope.hub.snapshot_download import snapshot_download
snapshot_download('iic/speech_campplus_sv_zh-cn_16k-common', cache_dir='${MODEL_DIR}/campplus')
print('CAM++ 下载完成')
"

# FSMN-VAD
echo ""
echo "[3/3] 下载 FSMN-VAD 语音活动检测模型..."
python3 -c "
from modelscope.hub.snapshot_download import snapshot_download
snapshot_download('iic/speech_fsmn_vad_zh-cn-16k-common-pytorch', cache_dir='${MODEL_DIR}/fsmn_vad')
print('FSMN-VAD 下载完成')
"

echo ""
echo "=========================================="
echo " 所有模型下载完成"
echo " 目录: ${MODEL_DIR}"
du -sh "${MODEL_DIR}"/*
echo "=========================================="
echo ""
echo " 离线部署: 将 models/ 目录整体拷贝到目标服务器"
