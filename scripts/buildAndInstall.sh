#!/bin/bash
# 构建并安装 git-manager
# 用法: ./scripts/buildAndInstall.sh [版本号]

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

"$SCRIPT_DIR/build.sh" "$@"

"$SCRIPT_DIR/install.sh"
