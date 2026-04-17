#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

export PATH="$HOME/.cargo/bin:$PATH"

if ! command -v clang-cl >/dev/null 2>&1 && command -v clang >/dev/null 2>&1; then
  export CC_x86_64_pc_windows_msvc="$PROJECT_DIR/scripts/clang-cl-shim.sh"
  export CXX_x86_64_pc_windows_msvc="$PROJECT_DIR/scripts/clang-cl-shim.sh"
fi

cd "$PROJECT_DIR"

bash ./scripts/doctor-windows-cross.sh

pnpm tauri build \
  "$@" \
  --runner cargo-xwin \
  --target x86_64-pc-windows-msvc \
  --config src-tauri/tauri.windows-xbuild.conf.json

printf '\nWindows NSIS bundle output should be under:\n'
printf 'src-tauri/target/x86_64-pc-windows-msvc/release/bundle/nsis/\n'