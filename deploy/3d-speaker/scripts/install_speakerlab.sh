#!/usr/bin/env bash
set -euo pipefail

PIP_BIN="${1:-pip}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

if "${PIP_BIN}" show speakerlab >/dev/null 2>&1; then
    exit 0
fi

shopt -s nullglob
wheel_candidates=("${PROJECT_DIR}"/wheels/speakerlab-*.whl)
shopt -u nullglob

if [ ${#wheel_candidates[@]} -gt 0 ]; then
    exec "${PIP_BIN}" install "${wheel_candidates[0]}"
fi

if [ -n "${SPEAKERLAB_SOURCE:-}" ]; then
    exec "${PIP_BIN}" install "${SPEAKERLAB_SOURCE}"
fi

exec "${PIP_BIN}" install "git+https://github.com/modelscope/3D-Speaker.git"