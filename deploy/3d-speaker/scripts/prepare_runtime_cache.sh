#!/bin/sh
set -eu

SOURCE_DIR="${NATIVE_MODEL_CACHE_SEED_DIR:-/app/models/native_cache}"
TARGET_DIR="${MODELSCOPE_CACHE:-/app/data/native_cache}"
REQUIRED_FILE="campplus_cn_en_common.pt"

mkdir -p "${TARGET_DIR}"

if [ -d "${SOURCE_DIR}" ] && [ "${SOURCE_DIR}" != "${TARGET_DIR}" ]; then
    if [ ! -f "${TARGET_DIR}/${REQUIRED_FILE}" ] && find "${SOURCE_DIR}" -type f | grep -q .; then
        echo "[INFO] 预热 native diarization 运行时缓存: ${SOURCE_DIR} -> ${TARGET_DIR}"
        cp -a "${SOURCE_DIR}/." "${TARGET_DIR}/"
    fi
fi

if ! find "${TARGET_DIR}" -type f -name "${REQUIRED_FILE}" | grep -q .; then
    if [ -d "${SOURCE_DIR}" ]; then
        if find "${SOURCE_DIR}" -type f | grep -q .; then
            echo "[WARN] native diarization 预热后仍缺少 ${REQUIRED_FILE}: source=${SOURCE_DIR}, target=${TARGET_DIR}" >&2
        else
            echo "[WARN] native diarization 种子目录为空: ${SOURCE_DIR}" >&2
        fi
    else
        echo "[WARN] native diarization 种子目录不存在: ${SOURCE_DIR}" >&2
    fi
fi

exit 0