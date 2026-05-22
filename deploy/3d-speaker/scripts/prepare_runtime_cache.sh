#!/bin/sh
set -eu

SOURCE_DIR="${NATIVE_MODEL_CACHE_SEED_DIR:-/app/models/native_cache}"
TARGET_DIR="${MODELSCOPE_CACHE:-/app/data/native_cache}"
ALLOW_INCOMPLETE_NATIVE_CACHE="${ALLOW_INCOMPLETE_NATIVE_CACHE:-0}"

has_native_file() {
    BASE_DIR="$1"
    MODEL_DIR="$2"
    FILE_NAME="$3"
    find "${BASE_DIR}/${MODEL_DIR}" -type f -name "${FILE_NAME}" -print -quit 2>/dev/null | grep -q .
}

has_complete_native_cache() {
    BASE_DIR="$1"
    has_native_file "${BASE_DIR}" "iic/speech_campplus_sv_zh_en_16k-common_advanced" "campplus_cn_en_common.pt" \
        && has_native_file "${BASE_DIR}" "iic/speech_fsmn_vad_zh-cn-16k-common-pytorch" "configuration.json" \
        && has_native_file "${BASE_DIR}" "iic/speech_fsmn_vad_zh-cn-16k-common-pytorch" "model.pt"
}

mkdir -p "${TARGET_DIR}"

if [ -d "${SOURCE_DIR}" ] && [ "${SOURCE_DIR}" != "${TARGET_DIR}" ]; then
    if ! has_complete_native_cache "${TARGET_DIR}" && find "${SOURCE_DIR}" -type f | grep -q .; then
        echo "[INFO] 预热 native diarization 运行时缓存: ${SOURCE_DIR} -> ${TARGET_DIR}"
        cp -a "${SOURCE_DIR}/." "${TARGET_DIR}/"
    fi
fi

if ! has_complete_native_cache "${TARGET_DIR}"; then
    if [ -d "${SOURCE_DIR}" ]; then
        if find "${SOURCE_DIR}" -type f | grep -q .; then
            echo "[ERROR] native diarization 缓存不完整: source=${SOURCE_DIR}, target=${TARGET_DIR}" >&2
        else
            echo "[ERROR] native diarization 种子目录为空: ${SOURCE_DIR}" >&2
        fi
    else
        echo "[ERROR] native diarization 种子目录不存在: ${SOURCE_DIR}" >&2
    fi
    echo "[ERROR] 需要 models/native_cache 同时包含 iic/speech_campplus_sv_zh_en_16k-common_advanced/campplus_cn_en_common.pt 和 iic/speech_fsmn_vad_zh-cn-16k-common-pytorch/{configuration.json,model.pt}" >&2
    if [ "${ALLOW_INCOMPLETE_NATIVE_CACHE}" = "1" ]; then
        echo "[WARN] ALLOW_INCOMPLETE_NATIVE_CACHE=1，继续启动但不会使用原生 speakerlab 分离流水线。" >&2
        exit 0
    fi
    exit 1
fi

exit 0