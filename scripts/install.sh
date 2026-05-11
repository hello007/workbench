#!/bin/bash
# 安装 git-manager 到系统目录
# 用法: ./scripts/install.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SOURCE="$PROJECT_DIR/build/bin/git-manager.exe"
TARGET_DIR="/d/Program Files/GitManager"
TARGET="$TARGET_DIR/git-manager.exe"

if [ ! -f "$SOURCE" ]; then
    echo "错误: 找不到构建产物 $SOURCE"
    echo "请先执行构建命令生成可执行文件"
    exit 1
fi

echo "源文件: $SOURCE"
echo "目标:   $TARGET"

# 创建目标目录
if [ ! -d "$TARGET_DIR" ]; then
    echo "创建目录: $TARGET_DIR"
    mkdir -p "$TARGET_DIR"
fi

# 备份旧版本
if [ -f "$TARGET" ]; then
    BACKUP="$TARGET_DIR/git-manager.exe.bak"
    echo "备份旧版本到: $BACKUP"
    mv -f "$TARGET" "$BACKUP"
fi

# 复制文件
cp "$SOURCE" "$TARGET"
echo "安装完成: $TARGET"
echo "版本信息:"
"$TARGET" --version 2>/dev/null || true
