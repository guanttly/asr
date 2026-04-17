#!/usr/bin/env bash

set -euo pipefail

RED='\033[31m'
YELLOW='\033[33m'
GREEN='\033[32m'
NC='\033[0m'

missing=0

check_command() {
  local name="$1"
  local hint="$2"
  if command -v "$name" >/dev/null 2>&1; then
    printf "${GREEN}OK${NC} %s -> %s\n" "$name" "$(command -v "$name")"
  else
    printf "${RED}MISS${NC} %s\n" "$name"
    printf "      %s\n" "$hint"
    missing=1
  fi
}

check_clang_driver() {
  if command -v clang-cl >/dev/null 2>&1; then
    printf "${GREEN}OK${NC} clang-cl -> %s\n" "$(command -v clang-cl)"
  elif command -v clang >/dev/null 2>&1; then
    printf "${GREEN}OK${NC} clang -> %s (will use clang-cl shim)\n" "$(command -v clang)"
  else
    printf "${RED}MISS${NC} clang-cl / clang\n"
    printf "      Ubuntu: sudo apt install clang\n"
    missing=1
  fi
}

printf "Checking Ubuntu -> Windows cross-build prerequisites for desktop...\n"

check_command pnpm "Install Node.js and pnpm first"
check_command rustup "Install Rust toolchain first"
check_command cargo "Ensure Cargo is on PATH: export PATH=\"$HOME/.cargo/bin:$PATH\""
check_command cargo-xwin "Run: cargo install --locked cargo-xwin"
check_clang_driver
check_command llvm-rc "Ubuntu: sudo apt install llvm"
check_command lld-link "Ubuntu: sudo apt install lld llvm"
check_command makensis "Ubuntu: sudo apt install nsis"

if command -v rustup >/dev/null 2>&1; then
  if rustup target list --installed | grep -qx 'x86_64-pc-windows-msvc'; then
    printf "${GREEN}OK${NC} rust target x86_64-pc-windows-msvc installed\n"
  else
    printf "${RED}MISS${NC} rust target x86_64-pc-windows-msvc\n"
    printf "      Run: rustup target add x86_64-pc-windows-msvc\n"
    missing=1
  fi
fi

if [[ ! -f src-tauri/tauri.windows-xbuild.conf.json ]]; then
  printf "${RED}MISS${NC} src-tauri/tauri.windows-xbuild.conf.json\n"
  missing=1
fi

if [[ ! -f src-tauri/icons/icon.ico ]]; then
  printf "${RED}MISS${NC} src-tauri/icons/icon.ico\n"
  printf "      Windows bundle requires an .ico file. Generate one from your PNG icon before building.\n"
  missing=1
fi

if [[ ! -f src-tauri/icons/icon.png ]]; then
  printf "${YELLOW}WARN${NC} src-tauri/icons/icon.png not found\n"
fi

if [[ $missing -ne 0 ]]; then
  printf "\n${RED}Windows cross-build environment is not ready.${NC}\n"
  exit 1
fi

printf "\n${GREEN}Windows cross-build environment looks ready.${NC}\n"
printf "Run: pnpm build:win\n"