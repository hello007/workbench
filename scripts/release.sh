#!/bin/bash
# workbench 发版脚本
# 用法: ./scripts/release.sh [选项]
# 示例:
#   ./scripts/release.sh --dry-run --bump patch --allow-dirty   # 试跑：补丁版本
#   ./scripts/release.sh --bump minor --yes                     # 次版本递增并直接推送
#   ./scripts/release.sh --version 1.2.3 --yes                  # 手动指定版本
#
# 版本号唯一来源: wails.json 的 info.productVersion
# 注意: 顶层 "version": "2" 是 wails schema 版本，本脚本严禁修改

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

# wails.json 路径
WAILS_JSON="wails.json"

# ---------- 参数解析 ----------
BUMP=""          # patch | minor | major
NEW_VERSION=""   # 手动指定的版本号
DRY_RUN=0        # 试跑模式：只打印计划，不执行写操作
ASSUME_YES=0     # 跳过 push 前确认
ALLOW_DIRTY=0    # 跳过工作区干净检查

print_usage() {
    cat <<'EOF'
用法: ./scripts/release.sh [选项]

发版选项（二选一，必填）:
  --bump patch|minor|major   按级别递增当前版本
  --version X.Y.Z            手动指定新版本号（与 --bump 互斥）

控制选项:
  --dry-run                  只打印计划，不执行任何写操作
  --yes                      跳过 push 前确认
  --allow-dirty              跳过工作区干净检查
  --help                     打印此帮助

示例:
  ./scripts/release.sh --dry-run --bump patch --allow-dirty
  ./scripts/release.sh --bump minor --yes
  ./scripts/release.sh --version 1.2.3 --yes

说明:
  版本号唯一来源为 wails.json 的 info.productVersion。
  执行后会: 提交 wails.json -> 打 tag -> 推送，触发 .github/workflows/release.yml 自动构建发布。
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --bump)
            BUMP="$2"
            shift 2
            ;;
        --version)
            NEW_VERSION="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=1
            shift
            ;;
        --yes)
            ASSUME_YES=1
            shift
            ;;
        --allow-dirty)
            ALLOW_DIRTY=1
            shift
            ;;
        --help|-h)
            print_usage
            exit 0
            ;;
        *)
            echo "错误: 未知参数 '$1'" >&2
            print_usage >&2
            exit 1
            ;;
    esac
done

# --bump 与 --version 互斥，且至少提供一个
if [ -n "$BUMP" ] && [ -n "$NEW_VERSION" ]; then
    echo "错误: --bump 与 --version 互斥，请二选一" >&2
    exit 1
fi
if [ -z "$BUMP" ] && [ -z "$NEW_VERSION" ]; then
    echo "错误: 必须提供 --bump 或 --version 之一" >&2
    echo "提示: 运行 ./scripts/release.sh --help 查看用法" >&2
    exit 1
fi

# 校验 --bump 取值
if [ -n "$BUMP" ]; then
    case "$BUMP" in
        patch|minor|major) ;;
        *)
            echo "错误: --bump 取值必须为 patch|minor|major，当前为 '$BUMP'" >&2
            exit 1
            ;;
    esac
fi

# ---------- 读取当前版本 ----------
CURRENT_VERSION=$(grep -o '"productVersion"[[:space:]]*:[[:space:]]*"[^"]*"' "$WAILS_JSON" | grep -o '"[^"]*"$' | tr -d '"')
if [ -z "$CURRENT_VERSION" ]; then
    echo "错误: 无法从 $WAILS_JSON 解析 productVersion" >&2
    exit 1
fi

# ---------- 计算新版本号 ----------
# 将版本号拆分为三段数字（保持原 shell 兼容，不依赖 bash 数组下标扩展）
bump_version() {
    local cur="$1"
    local level="$2"
    # shellcheck disable=SC2001
    local major minor patch
    major=$(echo "$cur" | sed 's/^\([0-9]*\)\..*$/\1/')
    minor=$(echo "$cur" | sed 's/^[0-9]*\.\([0-9]*\)\..*$/\1/')
    patch=$(echo "$cur" | sed 's/^[0-9]*\.[0-9]*\.\([0-9]*\)$/\1/')
    case "$level" in
        major)
            major=$((major + 1))
            minor=0
            patch=0
            ;;
        minor)
            minor=$((minor + 1))
            patch=0
            ;;
        patch)
            patch=$((patch + 1))
            ;;
    esac
    echo "${major}.${minor}.${patch}"
}

if [ -n "$NEW_VERSION" ]; then
    TARGET_VERSION="$NEW_VERSION"
else
    TARGET_VERSION=$(bump_version "$CURRENT_VERSION" "$BUMP")
fi

# ---------- 校验链 ----------
echo "发版计划"
echo "  当前版本: $CURRENT_VERSION"
echo "  目标版本: $TARGET_VERSION"
echo "  目标 tag : v$TARGET_VERSION"
echo ""

# 校验 1: 新版本号格式
if ! echo "$TARGET_VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "错误: 新版本号格式非法，必须形如 X.Y.Z（当前: $TARGET_VERSION）" >&2
    exit 1
fi

# 校验 2: 当前分支必须为 master
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$CURRENT_BRANCH" != "master" ]; then
    echo "错误: 当前分支为 '$CURRENT_BRANCH'，发版必须在 master 分支执行" >&2
    exit 1
fi

# 校验 3: 工作区必须干净（除非 --allow-dirty）
if [ "$ALLOW_DIRTY" -eq 0 ]; then
    if [ -n "$(git status --porcelain)" ]; then
        echo "错误: 工作区不干净，请先提交或暂存改动" >&2
        echo "提示: 如确需在脏工作区发版，可加 --allow-dirty（不推荐）" >&2
        exit 1
    fi
fi

# 校验 4: 目标 tag 不存在
if git rev-parse --verify --quiet "v$TARGET_VERSION" >/dev/null; then
    echo "错误: tag v$TARGET_VERSION 已存在" >&2
    exit 1
fi

echo "校验通过" >&2
echo ""

# 校验 5: dry-run 在此打印完整计划后退出
if [ "$DRY_RUN" -eq 1 ]; then
    echo "[dry-run] 计划如下（不会执行任何写操作）:"
    echo "  1. sed 替换 $WAILS_JSON 中 productVersion: $CURRENT_VERSION -> $TARGET_VERSION"
    echo "  2. git add $WAILS_JSON"
    echo "  3. git commit -m \"chore: bump version to $TARGET_VERSION\""
    echo "  4. git tag v$TARGET_VERSION"
    echo "  5. 遍历所有 remote 依次执行 git push <remote> && git push <remote> --tags"
    echo ""
    echo "[dry-run] 完成，未产生任何改动"
    exit 0
fi

# ---------- 执行步骤 ----------
# 记录已完成步骤，便于 push 失败时回滚
STEP_COMMIT_DONE=0
STEP_TAG_DONE=0

echo "1/5 更新 $WAILS_JSON"
# 精准替换 productVersion 那一行，保留其余 JSON 结构
# 注意：仅匹配 info.productVersion，顶层 "version" 不受影响（值不同且非同行）
sed -i.bak -E "s/(\"productVersion\"[[:space:]]*:[[:space:]]*\")$CURRENT_VERSION(\")/\1$TARGET_VERSION\2/" "$WAILS_JSON"
# sed -i 在 macOS 与 GNU 行为不同，备份文件统一删除
rm -f "${WAILS_JSON}.bak"
git add "$WAILS_JSON"
echo "   完成: $CURRENT_VERSION -> $TARGET_VERSION"

echo "2/5 提交改动"
git commit -m "chore: bump version to $TARGET_VERSION" >/dev/null
STEP_COMMIT_DONE=1
echo "   完成"

echo "3/5 打 tag"
git tag "v$TARGET_VERSION"
STEP_TAG_DONE=1
echo "   完成: v$TARGET_VERSION"

echo "4/5 推送前确认"
if [ "$ASSUME_YES" -eq 1 ]; then
    echo "   已通过 --yes 跳过确认"
else
    if [ -t 0 ]; then
        read -p "   即将推送 v$TARGET_VERSION 到 origin，不可撤销，确认？[y/N] " CONFIRM
        case "$CONFIRM" in
            y|Y|yes|YES) ;;
            *)
                echo "   已取消推送" >&2
                echo "   回滚命令: git tag -d v$TARGET_VERSION && git reset --hard HEAD~1" >&2
                exit 1
                ;;
        esac
    else
        echo "错误: 非交互环境，无法确认。请加 --yes 后重试推送" >&2
        echo "      已完成 commit/tag，回滚命令: git tag -d v$TARGET_VERSION && git reset --hard HEAD~1" >&2
        exit 1
    fi
fi

echo "5/5 推送到所有 remote"
# 本仓库约定（见 README）：远程 origin 为 Gitee，为唯一主仓库。Gitee 收到推送后会
# 自动镜像到 GitHub，从而触发运行在 GitHub Actions 上的 release.yml 监听 v* tag。
# 这里遍历所有已配置 remote 依次推送 commit 与 tags（当前仅有 origin，未来加 remote
# 也兼容），任一失败给出明确回滚指引。
PUSH_FAILED_REMOTES=""
for remote in $(git remote); do
    echo "  -> 推送 $remote ..."
    if git push "$remote" && git push "$remote" --tags; then
        :
    else
        echo "  警告: 推送到 $remote 失败" >&2
        PUSH_FAILED_REMOTES="$PUSH_FAILED_REMOTES $remote"
    fi
done

if [ -n "$PUSH_FAILED_REMOTES" ]; then
    echo "" >&2
    echo "错误: 以下 remote 推送失败:$PUSH_FAILED_REMOTES" >&2
    echo "      本地已完成 commit/tag，回滚命令: git tag -d v$TARGET_VERSION && git reset --hard HEAD~1" >&2
    echo "      若部分 remote 已成功，回滚前请先到对应平台删除已推送的 tag" >&2
    exit 1
fi

echo ""
echo "发版完成"
echo "  新版本: $TARGET_VERSION"
echo "  tag   : v$TARGET_VERSION"
echo "  远程  : $(git remote | tr '\n' ' ')"
echo ""
# CI 提示：Gitee 会自动镜像到 GitHub，release.yml 据此自动构建发布，无需额外操作。
echo "CI 提示: 已推送至 origin（Gitee），Gitee 会自动镜像到 GitHub"
echo "         .github/workflows/release.yml 据此自动构建并发布 Release"
