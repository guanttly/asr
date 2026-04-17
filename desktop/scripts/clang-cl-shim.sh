#!/usr/bin/env bash

set -euo pipefail

exec clang --driver-mode=cl "$@"