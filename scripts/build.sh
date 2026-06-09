#!/bin/bash
# workbench 构建脚本
# 用法: ./scripts/build.sh [版本号]
# 示例: ./scripts/build.sh          # 从 wails.json 读取版本
#       ./scripts/build.sh 2.0.0    # 手动指定版本

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

# 读取版本号：优先使用命令行参数，否则从 wails.json 读取
if [ -n "$1" ]; then
    VERSION="$1"
else
    VERSION=$(grep -o '"productVersion"[[:space:]]*:[[:space:]]*"[^"]*"' wails.json | grep -o '"[^"]*"$' | tr -d '"')
fi

BUILD_TIME=$(date +"%Y%m%d-%H%M%S")

echo "构建 WorkBench"
echo "  版本: $VERSION"
echo "  时间: $BUILD_TIME"

LDFLAGS="-X main.version=$VERSION -X main.buildTime=$BUILD_TIME"

wails build -ldflags "$LDFLAGS"

echo ""
echo "构建完成: build/bin/workbench.exe"
echo "版本验证:"
./build/bin/workbench.exe --version | cat
