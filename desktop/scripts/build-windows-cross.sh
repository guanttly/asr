#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

resolve_effective_home() {
  if [[ ${EUID:-$(id -u)} -eq 0 && -n "${SUDO_USER:-}" ]]; then
    getent passwd "$SUDO_USER" | cut -d: -f6
    return
  fi
  printf '%s\n' "$HOME"
}

append_path_if_dir() {
  local dir_path="$1"
  [[ -d "$dir_path" ]] || return 0
  case ":$PATH:" in
    *":$dir_path:"*) ;;
    *) export PATH="$dir_path:$PATH" ;;
  esac
}

append_latest_nvm_bin() {
  local home_dir="$1"
  local nvm_root="$home_dir/.nvm/versions/node"
  [[ -d "$nvm_root" ]] || return 0
  local latest_nvm_bin
  latest_nvm_bin=$(find "$nvm_root" -mindepth 2 -maxdepth 2 -type d -name bin 2>/dev/null | sort -V | tail -n 1)
  [[ -n "$latest_nvm_bin" ]] && append_path_if_dir "$latest_nvm_bin"
}

EFFECTIVE_HOME="$(resolve_effective_home)"
export HOME="$EFFECTIVE_HOME"
export CARGO_HOME="${CARGO_HOME:-$EFFECTIVE_HOME/.cargo}"
export RUSTUP_HOME="${RUSTUP_HOME:-$EFFECTIVE_HOME/.rustup}"

append_path_if_dir "$CARGO_HOME/bin"
append_path_if_dir "$EFFECTIVE_HOME/.local/share/pnpm"
append_path_if_dir "$EFFECTIVE_HOME/.local/bin"
append_latest_nvm_bin "$EFFECTIVE_HOME"

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