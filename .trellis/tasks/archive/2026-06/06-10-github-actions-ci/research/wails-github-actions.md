# 研究: Wails v2 在 GitHub Actions 上构建 Windows 桌面应用的最佳实践

- **查询**: Wails v2 在 GitHub Actions 上构建 Windows 桌面应用的最佳实践（依赖安装、ldflags 注入、Release 发布）
- **范围**: 混合（内部项目分析 + 外部最佳实践）
- **日期**: 2026-06-10

## 研究发现

### 一、GitHub Actions `windows-latest` Runner 依赖分析

#### 1.1 Runner 预装情况

`windows-latest`（当前为 Windows Server 2022）已预装以下关键软件：

| 依赖项 | 是否预装 | 说明 |
|---|---|---|
| **Go** | 多版本预装 | 建议使用 `actions/setup-go@v5` 精确控制版本 |
| **Node.js** | 多版本预装 | 建议使用 `actions/setup-node@v4` 精确控制版本 |
| **Git** | 已预装 | 默认包含 |
| **MinGW-w64 (gcc)** | 已预装 | `C:\ProgramData\chocolatey\lib\mingw\tools\install\mingw64\bin\gcc.exe`，**无需额外安装** |
| **WebView2 Runtime** | 已预装 | Microsoft Edge 内置 WebView2，**无需额外安装** |
| **npm** | 随 Node.js 预装 | - |

**结论**: `windows-latest` 已预装 Wails 构建所需的全部 C 编译器和 WebView2 运行时，无需额外安装 MinGW-w64 或 WebView2。

#### 1.2 本项目具体版本要求

| 依赖项 | 项目要求 | 配置来源 |
|---|---|---|
| **Go** | `>= 1.24.0` | `go.mod` 第 3 行: `go 1.24.0` |
| **Node.js** | `>= 16`（推荐 18+） | `DEVELOPMENT.md` 环境要求 |
| **Wails CLI** | `v2.5+`（项目用 v2.12.0） | `go.mod` 第 9 行: `github.com/wailsapp/wails/v2 v2.12.0` |

#### 1.3 需要在 Workflow 中显式安装的依赖

仅 **Wails CLI** 需要显式安装：

```yaml
- name: 安装 Wails CLI
  run: go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

**关键点**: 安装后需确保 `$(go env GOPATH)/bin` 在 PATH 中。`actions/setup-go` 默认会将 `$(go env GOPATH)/bin` 加入 PATH，所以只需确保 Wails 安装步骤在 `setup-go` 之后即可。

---

### 二、Wails v2 的 `wails build` 通过 ldflags 注入版本信息

#### 2.1 项目中的版本变量

文件 `main.go` 第 17-19 行：

```go
var (
    version   = "dev"
    buildTime = "unknown"
)
```

#### 2.2 本项目已有的构建脚本

文件 `scripts/build.sh` 展示了完整的 ldflags 注入模式：

```bash
VERSION=$(grep -o '"productVersion"[[:space:]]*:[[:space:]]*"[^"]*"' wails.json | grep -o '"[^"]*"$' | tr -d '"')
BUILD_TIME=$(date +"%Y%m%d-%H%M%S")
LDFLAGS="-X main.version=$VERSION -X main.buildTime=$BUILD_TIME"
wails build -ldflags "$LDFLAGS"
```

#### 2.3 GitHub Actions 中的 ldflags 注入方式

在 GitHub Actions 中，有两种方式获取版本号注入 ldflags：

**方式 A：从 tag 名称提取版本号（推荐）**

```yaml
- name: 构建 Wails 应用
  env:
    VERSION: ${{ github.ref_name }}  # 例如 v1.0.8
    BUILD_TIME: ${{ github.event.head_commit.timestamp }}
  run: |
    # 去掉 tag 前缀 'v'
    VER="${VERSION#v}"
    BT=$(echo "$BUILD_TIME" | sed 's/[:+]/-/g' | cut -dT -f1-2)
    wails build -ldflags "-X main.version=$VER -X main.buildTime=$BT"
```

**方式 B：从 wails.json 读取版本号（与本地构建脚本一致）**

```yaml
- name: 构建 Wails 应用
  run: |
    VERSION=$(grep -o '"productVersion"[[:space:]]*:[[:space:]]*"[^"]*"' wails.json | grep -o '"[^"]*"$' | tr -d '"')
    BUILD_TIME=$(date +"%Y%m%d-%H%M%S")
    wails build -ldflags "-X main.version=$VERSION -X main.buildTime=$BUILD_TIME"
```

**方式 A 的优势**: tag 版本号是 GitHub Release 的唯一真实来源，避免 wails.json 忘记更新导致版本不一致。

#### 2.4 ldflags 语法要点

- `-X` 标志格式: `-X main.version=1.0.8`（注意是等号赋值）
- 多个变量: 用空格分隔，如 `-X main.version=1.0.8 -X main.buildTime=20260610-120000`
- `wails build -ldflags` 会将 ldflags 透传给 Go 链接器
- 值中不能包含空格，构建时间建议用 `YYYYMMDD-HHMMSS` 格式

---

### 三、Wails v2 + GitHub Actions 的完整 Workflow 模板

#### 3.1 推荐的 Workflow 完整结构

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  build:
    runs-on: windows-latest

    steps:
      - name: 检出代码
        uses: actions/checkout@v4
        with:
          fetch-depth: 0  # 获取完整历史，用于生成 Release Notes

      - name: 安装 Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      - name: 安装 Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
          cache-dependency-path: frontend/package-lock.json

      - name: 安装前端依赖
        run: cd frontend && npm ci

      - name: 安装 Wails CLI
        run: go install github.com/wailsapp/wails/v2/cmd/wails@latest

      - name: 构建 Wails 应用
        run: |
          $VERSION = "${{ github.ref_name }}".TrimStart('v')
          $BUILD_TIME = Get-Date -Format "yyyyMMdd-HHmmss"
          wails build -ldflags "-X main.version=$VERSION -X main.buildTime=$BUILD_TIME"

      - name: 验证构建产物
        run: |
          if (Test-Path "build/bin/workbench.exe") {
            Write-Host "构建成功: build/bin/workbench.exe"
            ./build/bin/workbench.exe --version
          } else {
            Write-Error "构建失败: workbench.exe 未生成"
            exit 1
          }

      - name: 生成 Release Notes
        id: release_notes
        run: |
          $PREV_TAG = git describe --tags --abbrev=0 HEAD^ 2>$null
          if ($LASTEXITCODE -ne 0) {
            $COMMITS = git log --pretty=format:"- %s (%h)" HEAD
          } else {
            $COMMITS = git log --pretty=format:"- %s (%h)" "${PREV_TAG}..HEAD"
          }
          $COMMITS | Out-File -FilePath release_notes.txt -Encoding utf8

      - name: 创建 Release 并上传
        uses: softprops/action-gh-release@v2
        with:
          name: ${{ github.ref_name }}
          body_path: release_notes.txt
          files: build/bin/workbench.exe
```

#### 3.2 关键配置说明

| 配置项 | 值 | 说明 |
|---|---|---|
| `on.push.tags` | `v*` | 仅 tag 推送触发 |
| `permissions.contents` | `write` | Release 创建需要写权限 |
| `fetch-depth` | `0` | 完整历史，用于生成 commit 摘要 |
| `go-version` | `'1.24'` | 匹配项目 go.mod |
| `node-version` | `'20'` | Node.js 20 LTS |
| `cache` | `'npm'` | 加速前端依赖安装 |
| PowerShell | 默认 shell | `windows-latest` 默认使用 PowerShell |

#### 3.3 注意事项

1. **PowerShell 语法**: `windows-latest` 默认 shell 为 PowerShell，字符串操作使用 `.TrimStart()` 而非 bash 的 `${VAR#v}`
2. **`npm ci`**: 使用 `npm ci`（而非 `npm install`）确保 CI 环境依赖一致性，但需要 `package-lock.json` 文件存在
3. **构建产物路径**: `wails build` 输出到 `build/bin/<outputfilename>.exe`，`outputfilename` 由 `wails.json` 中的 `outputfilename` 字段决定，本项目为 `workbench`
4. **Wails CLI 版本**: `go install ...@latest` 安装最新版 Wails CLI。如需锁定版本，可使用 `@v2.12.0` 指定

---

### 四、`softprops/action-gh-release` 自动创建 Release 并上传产物

#### 4.1 基本用法

```yaml
- name: 创建 Release
  uses: softprops/action-gh-release@v2
  with:
    # tag 名称（默认为 github.ref_name，通常不需要手动指定）
    tag_name: ${{ github.ref_name }}
    # Release 标题
    name: ${{ github.ref_name }}
    # 是否标记为预发布（draft: true 则不公开发布）
    draft: false
    # 是否标记为预发布版本
    prerelease: false
    # Release 正文（可从文件读取）
    body_path: release_notes.txt
    # 上传文件（支持多文件、通配符）
    files: |
      build/bin/workbench.exe
      build/bin/*.exe
```

#### 4.2 核心参数

| 参数 | 默认值 | 说明 |
|---|---|---|
| `tag_name` | `${{ github.ref_name }}` | Release 关联的 tag |
| `name` | tag 名称 | Release 标题 |
| `body` | - | Release 正文（与 `body_path` 二选一） |
| `body_path` | - | 从文件读取 Release 正文 |
| `files` | - | 上传文件列表 |
| `draft` | `false` | 是否为草稿 |
| `prerelease` | `false` | 是否为预发布 |

#### 4.3 权限要求

必须在 job 或 workflow 级别声明：

```yaml
permissions:
  contents: write
```

#### 4.4 Release Notes 自动生成策略

有三种策略可选：

**策略 1：git log commit 摘要（本项目 PRD 选择）**

```yaml
- name: 生成 Release Notes
  run: |
    $PREV_TAG = git describe --tags --abbrev=0 HEAD^ 2>$null
    if ($LASTEXITCODE -ne 0) {
      $COMMITS = git log --pretty=format:"- %s (%h)" HEAD
    } else {
      $COMMITS = git log --pretty=format:"- %s (%h)" "${PREV_TAG}..HEAD"
    }
    $COMMITS | Out-File -FilePath release_notes.txt -Encoding utf8
```

**策略 2：使用 GitHub 自动生成的 Release Notes**

```yaml
- uses: softprops/action-gh-release@v2
  with:
    generate_release_notes: true  # GitHub 自动按 PR 分类汇总
```

**策略 3：手动拼接（更精细控制）**

```yaml
- uses: softprops/action-gh-release@v2
  with:
    body: |
      ## 变更内容
      ${{ steps.changelog.outputs.text }}
```

---

### 五、完整推荐 Workflow（面向本项目的最终版本）

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

permissions:
  contents: write

jobs:
  release:
    runs-on: windows-latest
    steps:
      # 1. 检出代码
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      # 2. 安装 Go 1.24
      - uses: actions/setup-go@v5
        with:
          go-version: '1.24'

      # 3. 安装 Node.js 20
      - uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'
          cache-dependency-path: frontend/package-lock.json

      # 4. 安装前端依赖
      - run: cd frontend && npm ci

      # 5. 安装 Wails CLI
      - run: go install github.com/wailsapp/wails/v2/cmd/wails@latest

      # 6. 构建应用（注入版本信息）
      - name: 构建应用
        run: |
          $VER = "${{ github.ref_name }}".TrimStart('v')
          $BT = Get-Date -Format "yyyyMMdd-HHmmss"
          wails build -ldflags "-X main.version=$VER -X main.buildTime=$BT"

      # 7. 验证构建产物
      - run: ./build/bin/workbench.exe --version

      # 8. 创建 GitHub Release 并上传
      - uses: softprops/action-gh-release@v2
        with:
          name: ${{ github.ref_name }}
          generate_release_notes: true
          files: build/bin/workbench.exe
```

---

### 六、已回答的问题汇总

| 问题 | 答案 |
|---|---|
| windows-latest 需安装哪些依赖？ | 仅需安装 Wails CLI（`go install`），Go 和 Node.js 通过 actions/setup-* 控制，MinGW-w64 和 WebView2 已预装 |
| Go 版本要求 | 项目 `go.mod` 指定 `go 1.24.0`，workflow 中使用 `go-version: '1.24'` |
| Node.js 版本要求 | 项目推荐 18+，workflow 中使用 `node-version: '20'`（当前 LTS） |
| Wails CLI 安装方式 | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| 是否需要额外安装 MinGW-w64？ | **不需要**，windows-latest 已预装 |
| WebView2 是否预装？ | **是**，通过 Microsoft Edge 内置 |
| wails build ldflags 注入 | `wails build -ldflags "-X main.version=$VER -X main.buildTime=$BT"` |
| Workflow 模板 | 见上方第五节完整推荐 Workflow |
| Release 自动创建 | 使用 `softprops/action-gh-release@v2`，设置 `generate_release_notes: true` |

---

### 七、项目文件参考

| 文件路径 | 说明 |
|---|---|
| `main.go:17-19` | `version` 和 `buildTime` ldflags 变量声明 |
| `go.mod:3` | Go 版本约束 `go 1.24.0` |
| `go.mod:9` | Wails 依赖 `github.com/wailsapp/wails/v2 v2.12.0` |
| `wails.json` | 构建配置（`outputfilename: "workbench"`, `productVersion: "1.0.8"`） |
| `scripts/build.sh` | 本地构建脚本，展示 ldflags 注入模式 |
| `frontend/package.json` | 前端依赖和构建命令 |
| `.trellis/tasks/06-10-github-actions-ci/prd.md` | PRD 文档，包含需求定义 |

---

## 注意事项 / 未解决的问题

1. **package-lock.json**: 当前项目中 `frontend/` 目录下可能缺少 `package-lock.json` 文件。如果缺失，`npm ci` 会失败，需改用 `npm install` 或先在本地生成 `package-lock.json`
2. **Git Remote 策略**: 当前主仓库在 Gitee，GitHub 作为镜像。Workflow 需要在 GitHub 仓库上触发 tag 推送，需要配置 Gitee 到 GitHub 的镜像同步或在 GitHub 上直接推送 tag
3. **Wails CLI 版本锁定**: `@latest` 可能导致未来版本不兼容。建议考虑锁定为 `@v2.12` 或通过 `go.mod` 中的工具链管理
4. **`npm ci` 要求**: `npm ci` 需要 `package-lock.json` 存在。如果项目尚未提交该文件，需要先执行 `npm install` 并将 `package-lock.json` 提交到仓库
5. **Windows PowerShell**: `windows-latest` 默认使用 PowerShell（非 bash），字符串操作语法不同（`.TrimStart()` vs `${VAR#v}`）
