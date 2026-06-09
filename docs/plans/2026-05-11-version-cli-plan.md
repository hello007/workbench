# workbench --version CLI 支持实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 让打包后的 workbench.exe 支持 `--version` 输出版本信息，构建时通过 `-ldflags` 自动注入语义版本和构建时间戳。

**Architecture:** 在 `main.go` 中声明 `version`/`buildTime` 变量供链接器注入，`main()` 入口拦截 `--version` 参数后直接打印并退出。新建 `scripts/build.sh` 封装 `wails build` 命令，自动从 `wails.json` 读取版本号并注入时间戳。

**Tech Stack:** Go ldflags 链接器注入、bash 脚本、wails build

---

### Task 1: main.go 添加版本变量和 CLI 参数解析

**Files:**
- Modify: `main.go:1-15`（import 区）和 `main.go:15-36`（main 函数）

**Step 1: 添加版本变量**

在 `main.go` 中 `import` 块后、`//go:embed` 声明前，添加版本变量：

```go
var (
	version   = "dev"
	buildTime = "unknown"
)
```

同时在 import 中添加 `"fmt"` 和 `"os"`（`fmt` 已有则跳过，检查是否已导入 `os`）。

**Step 2: 添加 --version 参数拦截**

在 `main()` 函数最开头（`app := NewApp()` 之前）插入：

```go
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("workbench v%s (build %s)\n", version, buildTime)
		os.Exit(0)
	}
```

**Step 3: 运行测试确认不破坏现有功能**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go test ./... -v -count=1`
Expected: 所有现有测试 PASS

**Step 4: 手动验证 --version（开发模式）**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go run . --version`
Expected: 输出 `workbench vdev (build unknown)`（未注入 ldflags 时使用默认值）

**Step 5: 提交**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench
git add main.go
git commit -m "feat: add --version CLI flag with ldflags injection support"
```

---

### Task 2: 新建 scripts/build.sh 构建脚本

**Files:**
- Create: `scripts/build.sh`

**Step 1: 创建构建脚本**

```bash
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
    VERSION=$(python3 -c "import json; print(json.load(open('wails.json'))['info']['productVersion'])" 2>/dev/null || \
              grep -o '"productVersion"[[:space:]]*:[[:space:]]*"[^"]*"' wails.json | grep -o '"[^"]*"$' | tr -d '"')
fi

BUILD_TIME=$(date +"%Y%m%d-%H%M%S")

echo "构建 workbench"
echo "  版本: $VERSION"
echo "  时间: $BUILD_TIME"

LDFLAGS="-X main.version=$VERSION -X main.buildTime=$BUILD_TIME"

wails build -ldflags "$LDFLAGS"

echo ""
echo "构建完成: build/bin/workbench.exe"
echo "版本验证:"
./build/bin/workbench.exe --version
```

**Step 2: 添加执行权限**

Run: `chmod +x d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench/scripts/build.sh`

**Step 3: 验证脚本语法**

Run: `bash -n d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench/scripts/build.sh`
Expected: 无输出（语法正确）

**Step 4: 提交**

```bash
cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench
git add scripts/build.sh
git commit -m "feat: add build script with automatic version injection"
```

---

### Task 3: 集成验证

**Step 1: 使用构建脚本完整构建**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && ./scripts/build.sh`
Expected:
- 输出 `版本: 1.0.0`、`时间: <当前时间>`
- 构建成功
- 最后输出 `workbench v1.0.0 (build <时间戳>)`

**Step 2: 手动指定版本构建**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && ./scripts/build.sh 2.0.0-test`
Expected: 输出 `workbench v2.0.0-test (build <时间戳>)`

**Step 3: 使用 install.sh 安装并验证**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && ./scripts/install.sh`
Expected: 安装成功，末尾输出 `workbench v1.0.0 (build <时间戳>)`
