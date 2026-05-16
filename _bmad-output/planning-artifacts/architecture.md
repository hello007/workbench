---
stepsCompleted:
  - step-01-init
  - step-02-context
  - step-03-starter
  - step-04-decisions
  - step-05-patterns
  - step-06-structure
  - step-07-validation
  - step-08-complete
inputDocuments:
  - '_bmad-output/planning-artifacts/prd.md'
  - '_bmad-output/project-context.md'
  - 'docs/project-context.md'
  - 'docs/功能说明.md'
  - 'docs/路线图.md'
  - 'docs/开发规范.md'
  - 'docs/常见问题.md'
  - 'docs/开发工作流.md'
  - 'docs/测试策略.md'
  - 'docs/部署说明.md'
workflowType: 'architecture'
project_name: 'git-manager'
user_name: 'Liuyang'
date: '2026-05-15'
lastStep: 8
status: 'complete'
completedAt: '2026-05-16'
---

# Architecture Decision Document

_This document builds collaboratively through step-by-step discovery. Sections are appended as we work through each architectural decision together._

## Project Context Analysis

### Requirements Overview

**Functional Requirements:**

33 条 FR 分布在 6 个能力领域，架构影响分析如下：

| 能力领域 | FR 数量 | 架构影响 |
|----------|---------|----------|
| 工作目录管理 | 5 | JSON 持久化配置，路径验证 |
| 文件树浏览 | 6 | 懒加载 + 缓存策略，Git 检测 |
| 文件操作 | 9 | 文件系统 I/O，右键菜单系统，剪贴板操作 |
| Git 仓库操作 | 7 | 双引擎协调，并发批处理，进度事件流 |
| 外部工具集成 | 3 | 进程调用（exec.Cmd），平台适配 |
| 应用框架 | 3 | 三栏布局，版本管理，离线支持 |

**Non-Functional Requirements:**

9 条 NFR 驱动的架构决策：

- **性能（NFR1-7）：** 冷启动 < 3s、文件树加载 < 1s、并发上限 5、内存 < 300MB — 要求懒加载、缓存、并发控制
- **安全（NFR8-9）：** 路径规范化、删除确认 — 要求输入校验层

**Scale & Complexity:**

- 主要技术域：桌面全栈（Go + Vue 3 + 文件系统 + Git）
- 复杂度等级：低到中等
- 预估架构组件：8-10 个核心模块

### Technical Constraints & Dependencies

**已确定的技术约束（来自 Project Context）：**

- **框架绑定：** Wails v2 — app.go 公开方法自动生成 JS 绑定，service 层禁止导入 Wails
- **双 Git 引擎边界：** go-git 读 / exec.Cmd 写，同一功能内不混用
- **Windows 平台：** exec.Cmd 必须调用 HideCommandWindow，需保留 exec_other.go
- **前端约束：** 纯 JS（无 TypeScript），无新 npm 依赖，Composition API + `<script setup>`
- **数据契约：** Go json 标签 ↔ 前端隐式定义，修改需双向同步

### Cross-Cutting Concerns Identified

| 关注点 | 影响范围 | 当前策略 |
|--------|----------|----------|
| 路径安全 | 所有文件/Git 操作 | filepath.Clean + 范围校验 |
| 异步防护 | 所有 Wails 调用 | 前端 disable 按钮 + loading 状态 |
| 事件清理 | 所有 EventsOn 调用 | onBeforeUnmount 中 EventsOff |
| 错误处理 | service → app.go → 前端 | service 返回 error，app.go 日志，前端 ElMessage |
| 并发控制 | 批量 Git 操作 | WaitGroup + 信号量 + Mutex |
| 平台适配 | exec.Cmd 调用 | build tags + HideCommandWindow |

## Starter Template Evaluation

### Primary Technology Domain

桌面全栈应用 — 已投产运行，技术栈已确定。

### Selected Starter: Wails v2（已使用）

**项目已采用的技术栈决策：**

**语言与运行时：**

- 后端：Go 1.24.0+
- 前端：JavaScript（纯 JS，无 TypeScript），Vue 3.5.33 Composition API

**UI 框架：**

- Element Plus 2.13.7（组件库）
- 手写 CSS（无 Tailwind/UnoCSS）

**构建工具：**

- Wails v2.12.0（桌面框架 + 构建管道）
- Vite 8.0.10（前端构建）
- `//go:embed all:frontend/dist` 嵌入前端资源

**测试框架：**

- 后端：Go testing + 表驱动测试
- 前端：Vitest 4.1.5 + Vue Test Utils 2.4.9 + jsdom

**Git 操作：**

- go-git/v5 5.18.0（读操作：log/status/diff）
- exec.Cmd("git")（写操作：clone/pull）

**代码组织：**

```
git-manager/
├── app.go           # Wails 绑定调度层（公开方法 → service）
├── model/           # 数据结构 + 验证（带 json 标签）
├── service/         # 业务逻辑（不依赖 Wails）
├── util/            # 工具函数（按功能分文件）
├── frontend/
│   └── src/
│       ├── views/       # 页面组件
│       ├── components/  # 通用组件
│       └── test/        # 测试 setup
├── data/            # 运行时数据（不提交 Git）
└── build/           # 构建输出
```

**开发体验：**

- `wails dev` — 前后端热重载
- `go test ./...` — 后端测试
- `cd frontend && npm test` — 前端测试
- `wails build` → `buildAndInstall.sh` — 构建 + 安装

## Core Architectural Decisions

### Decision Priority Analysis

**Critical Decisions (Block Implementation):** 无 — 所有决策已在棕地项目中确定

**Important Decisions (Shape Architecture):** 无 — 当前架构满足 PRD Phase 1 & 2 全部需求

**Deferred Decisions (Post-MVP):**
- 多工具模块架构：PRD Phase 3 愿景，未来需要时再重构

### Data Architecture

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 持久化方案 | JSON 文件（`data/` 目录） | 桌面应用，无需数据库 |
| 数据读写 | `util.LoadJSON` / `util.SaveJSON` | 统一封装 |
| 文件系统操作 | `os.ReadDir` + `filepath.Join` | 原生 Go，懒加载按需读取 |
| Git 数据 | go-git（读）+ exec.Cmd（写） | 双引擎边界，同一功能内不混用 |
| 缓存 | `sync.Map`（Git 仓库检测缓存） | 进程内缓存，无持久化 |

### Security

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 认证 | 无 | 单用户桌面应用 |
| 路径安全 | `filepath.Clean` + 范围校验 | 防止路径遍历 |
| 删除保护 | 前端二次确认 | 防止误删 |
| 敏感文件 | 暂无特殊处理 | 已记录为 deferred work |

### API & Communication Patterns

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 前后端通信 | Wails 绑定（Go → JS Promise） | `app.go` 公开方法自动生成 JS |
| 实时推送 | Wails 事件系统 | 批量操作进度通知 |
| 事件安全 | `safeEmit` 包装 | 非 Wails 上下文静默跳过 |
| 错误传递 | service error → app.go 日志 → 前端 ElMessage | 三层错误处理链 |
| 数据契约 | Go json 标签 ↔ 前端隐式定义 | 修改需双向同步 |

### Frontend Architecture

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 框架 | Vue 3 Composition API + `<script setup>` | 项目约定 |
| UI 组件库 | Element Plus 2.13.7 | ElTree 懒加载、ElDialog、ElForm |
| 状态管理 | ref/reactive + Props/Events | 规模不需要 Pinia |
| 路由 | Vue Router 4.6.4 Hash 模式 | Wails 不支持 Browser History |
| 右键菜单 | 自定义 div + 绝对定位 | el-dropdown 不支持 contextmenu |
| 异步防护 | disable 按钮 + loading ref | 防止并发操作 |

### Infrastructure & Deployment

| 决策项 | 选择 | 理由 |
|--------|------|------|
| 构建 | `wails build` → `buildAndInstall.sh` | 单命令构建 + 安装 |
| 前端嵌入 | `//go:embed all:frontend/dist` | 单一二进制文件分发 |
| 分发 | Windows 可执行文件 | 无安装包，直接运行 |
| CI/CD | 无 | 单人开发，手动构建 |
| 配置 | `data/` 目录（不提交 Git） | 运行时数据本地存储 |

### Decision Impact Analysis

**Implementation Sequence:**

Phase 2 新增功能直接在现有架构上扩展，无架构变更需求。

**Cross-Component Dependencies:**

| 依赖关系 | 说明 |
|----------|------|
| app.go ↔ service/ | app.go 只做调度，业务逻辑全在 service |
| service ↔ util/ | 工具函数按功能分文件 |
| 前端 ↔ Wails 绑定 | 通过 wailsjs/go/main/App.js 调用 |
| 前端 ↔ 事件系统 | EventsOn 必须在 onBeforeUnmount 中 EventsOff |

## Implementation Patterns & Consistency Rules

### Critical Conflict Points Identified

6 个领域存在 AI Agent 可能做出不同选择的潜在冲突点：命名、结构、格式、通信、流程、平台。

### Naming Patterns

**Go 命名规范：**

| 类别 | 规范 | 示例 |
|------|------|------|
| 导出函数 | PascalCase | `GetDirectories()`, `ScanGitRepos()` |
| 未导出函数 | camelCase | `safeEmit()`, `getCommitFiles()` |
| Service 构造函数 | `New{ServiceName}()` | `NewGitService()`, `NewDirectoryService()` |
| 接收者变量 | 类型首字母小写 | `(a *App)`, `(s *GitService)` |
| 错误变量 | `Err` 前缀或内联 | `fmt.Errorf("描述: %w", err)` |
| 常量 | PascalCase 或全大写 | `maxSize`（包内）, `DefaultPageSize`（导出） |

**前端命名规范：**

| 类别 | 规范 | 示例 |
|------|------|------|
| 组件文件 | PascalCase.vue | `Home.vue`, `CommitHistory.vue` |
| 事件名 | kebab-case | `@node-click`, `@batch-pull` |
| emit 事件 | kebab-case | `'latest-commit'`, `'refresh-node'` |
| 事件处理函数 | camelCase + `handle`/`on` 前缀 | `handleSearch()`, `onDirectorySelect()` |
| ref/reactive | camelCase | `loading`, `treeData`, `currentDir` |

### Structure Patterns

**三层架构职责边界：**

```
app.go（调度层）
  ├── 参数校验（仅空值/格式）
  ├── 调用 service 方法
  ├── 错误日志（println）
  └── 返回值包装（nil → 空集合）

service/（业务层）
  ├── 业务逻辑实现
  ├── 调用 util 工具函数
  ├── 返回 (result, error)
  └── 禁止导入 Wails 包

util/（工具层）
  ├── 纯函数，无状态
  ├── 按功能分文件（json.go, git.go, file.go）
  └── 可被 service/ 和 app.go 调用
```

**SFC 文件内部顺序：**

```vue
<template>...</template>
<script setup>
// 1. imports
// 2. props/emits 定义
// 3. reactive state
// 4. computed
// 5. 生命周期钩子（onMounted, onBeforeUnmount）
// 6. methods
</script>
<style scoped>...</style>
```

**测试文件位置：**

| 层 | 位置 | 命名 |
|----|------|------|
| Go 后端 | 与源码同目录 | `*_test.go` |
| Go 测试函数 | `Test{FunctionName}_{Scenario}` | `TestScanGitRepos_SingleRepo` |
| 前端 | `views/__tests__/` | `{ComponentName}.spec.js` |

### Format Patterns

**Wails 数据契约：**

Go struct 使用 `json` 标签定义字段名，前端通过 Wails 生成的 JS 绑定隐式获得类型：

```go
type Directory struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Path      string `json:"path"`
    IsDefault bool   `json:"isDefault"`
}
```

**修改契约的流程：** Go struct 改 `json` 标签 → 前端对应字段同步修改。

**错误格式规范：**

| 层 | 格式 | 示例 |
|----|------|------|
| service → app.go | Go error | `fmt.Errorf("未找到目录: %s", id)` |
| app.go → 前端（返回 error） | error 值 | 前端通过 catch 或 null 返回值判断 |
| app.go → 前端（返回 string） | `"错误: " + err.Error()` | `"错误: Git仓库已存在"` |
| 前端 → 用户 | ElMessage | `ElMessage.error('操作失败')` |

**字符串返回值约定：**

- 成功：描述性文本（如 `"克隆成功"`）或空字符串
- 失败：`"错误: " + err.Error()`

### Communication Patterns

**Wails 事件系统：**

| 规则 | 说明 |
|------|------|
| 事件名格式 | `{action}:{target}` 或 `{action}` |
| 发送方式 | `safeEmit(ctx, eventName, data)` — 非 Wails 上下文静默跳过 |
| 监听注册 | `EventsOn(eventName, handler)` |
| 必须清理 | `onBeforeUnmount` 中调用 `EventsOff(eventName)` |

**组件通信：**

| 场景 | 方式 |
|------|------|
| 父 → 子 | Props（`defineProps`） |
| 子 → 父 | Emit（`defineEmits`） |
| 跨组件 | Wails 事件系统 或 Props 逐层传递 |

### Process Patterns

**异步操作防护：**

```javascript
// 前端异步操作标准模式
const loading = ref(false)

async function handleAction() {
  if (loading.value) return  // 防止并发
  loading.value = true
  try {
    const result = await WindowAction(params)
    // 处理结果
  } catch (e) {
    ElMessage.error('操作失败')
  } finally {
    loading.value = false
  }
}
```

**exec.Cmd 调用模式（Windows）：**

```go
// 必须使用 HideCommandWindow 防止控制台窗口闪烁
cmd := exec.Command("git", "pull")
cmd.Dir = workDir
util.HideCommandWindow(cmd)  // Windows 专用，通过 build tags 区分
output, err := cmd.CombinedOutput()
```

### Enforcement Guidelines

**所有 AI Agent 必须遵守：**

1. **Service 层禁止导入 Wails** — 业务逻辑不依赖框架
2. **app.go 只做调度** — 不包含业务逻辑，只处理参数校验、调用 service、错误日志
3. **错误必须向上传递** — service 返回 error，app.go 打印日志并包装返回值
4. **前端事件必须清理** — 所有 `EventsOn` 必须在 `onBeforeUnmount` 中对应 `EventsOff`
5. **异步操作必须防护** — loading ref + 按钮禁用，防止并发操作
6. **exec.Cmd 必须隐藏窗口** — 所有外部进程调用必须使用 `HideCommandWindow`

**Anti-Patterns（禁止）：**

- 在 app.go 中编写业务逻辑
- 在 service 中导入 `github.com/wailsapp/wails/v2`
- 混用 go-git 和 exec.Cmd 实现同一功能
- 在前端使用 `EventEmit` 而不注册对应的 `EventsOff`
- 创建全局可变状态（Go 全局变量或前端全局 reactive）
- 在 model struct 中省略 `json` 标签

## Project Structure & Boundaries

### Complete Project Directory Structure

```
git-manager/
├── main.go                    # 应用入口（Wails 初始化）
├── app.go                     # 调度层（Wails 绑定方法 → service）
├── app_test.go                # app.go 测试
├── console_windows.go         # Windows 控制台（build tag）
├── console_other.go           # 非 Windows 控制台（build tag）
├── wails.json                 # Wails 框架配置
├── model/                     # 数据模型层
│   ├── models.go              # Directory, FileTreeNode, GitRepoInfo, GitRemoteInfo, PullSummary, PageResult
│   ├── models_test.go         # 模型验证测试
│   ├── commit.go              # Commit 结构体
│   └── commit_test.go         # Commit 测试
├── service/                   # 业务逻辑层（不依赖 Wails）
│   ├── directory.go           # 工作目录管理（FR1-5）
│   ├── filetree.go            # 文件树浏览（FR6-11）
│   ├── filetree_test.go       # 文件树测试
│   ├── fileoperation.go       # 文件操作（FR12-20）
│   ├── fileoperation_test.go  # 文件操作测试
│   ├── git.go                 # Git 操作（FR21-27）
│   ├── git_test.go            # Git 测试
│   └── clipboard.go           # 剪贴板操作
├── util/                      # 工具层（按功能分文件）
│   ├── json.go                # JSON 持久化（LoadJSON/SaveJSON）
│   ├── git.go                 # Git 辅助（FindGitRoot, GitCommand）
│   ├── file.go                # 文件辅助（IsPreviewable, FormatFileSize）
│   ├── clipboard_windows.go   # Windows 剪贴板
│   ├── exec_windows.go        # Windows exec.Cmd（HideCommandWindow）
│   └── exec_other.go          # 非 Windows exec（build tag）
├── frontend/
│   ├── index.html             # Wails 入口 HTML
│   ├── package.json           # 前端依赖
│   ├── vite.config.js         # Vite 构建配置
│   ├── vitest.config.js       # Vitest 测试配置
│   └── src/
│       ├── main.js            # Vue 应用入口
│       ├── App.vue            # 根组件
│       ├── app.css            # 全局样式
│       ├── style.css          # 基础样式
│       ├── router/index.js    # Vue Router（Hash 模式）
│       ├── utils/
│       │   ├── gitCache.js    # Git 仓库检测缓存
│       │   └── debug.js       # 调试工具
│       ├── views/
│       │   ├── Home.vue       # 主页面（三栏布局）
│       │   └── __tests__/     # 前端测试
│       │       ├── Home.spec.js
│       │       └── ContextMenu.spec.js
│       └── components/
│           ├── DirectoryTree.vue   # 工作目录列表
│           ├── FileTreePanel.vue   # 文件树面板
│           ├── ContentPanel.vue    # 内容预览面板
│           ├── GitInfo.vue         # Git 仓库信息
│           └── CommitHistory.vue   # 提交历史
├── data/                      # 运行时数据（.gitignore）
│   └── directories.json       # 工作目录配置
├── scripts/
│   ├── build.sh               # 构建脚本
│   ├── buildAndInstall.sh     # 构建 + 安装
│   └── install.sh             # 安装脚本
├── build/                     # 构建输出
│   └── bin/                   # 可执行文件
└── docs/                      # 项目文档
    ├── 功能说明.md
    ├── 开发工作流.md
    ├── 测试策略.md
    ├── 部署说明.md
    ├── 开发规范.md
    ├── 常见问题.md
    ├── 路线图.md
    ├── project-context.md
    └── plans/                 # 设计方案归档
```

### Requirements to Structure Mapping

| FR 能力领域 | Go 文件 | 前端组件 | 数据文件 |
|-------------|---------|----------|----------|
| 工作目录管理（FR1-5） | `service/directory.go` | `DirectoryTree.vue` | `data/directories.json` |
| 文件树浏览（FR6-11） | `service/filetree.go` | `FileTreePanel.vue` | — |
| 文件操作（FR12-20） | `service/fileoperation.go` | `FileTreePanel.vue` | — |
| Git 仓库操作（FR21-27） | `service/git.go`, `app.go` | `GitInfo.vue`, `CommitHistory.vue` | — |
| 外部工具集成（FR28-30） | `service/fileoperation.go` | `FileTreePanel.vue` | — |
| 应用框架（FR31-33） | `main.go`, `app.go` | `Home.vue`, `App.vue` | — |

### Architectural Boundaries

**层间通信边界：**

```
前端 → Wails JS 绑定 → app.go → service/ → util/
                          ↓
                    Wails 事件系统
```

- **前端 → app.go**：通过 `wailsjs/go/main/App.js` 自动生成的绑定调用
- **app.go → service**：直接方法调用，app.go 只做参数校验和错误日志
- **service → util**：直接函数调用
- **service → 事件**：通过 `safeEmit` 向前端推送进度
- **禁止**：service 层导入 Wails 包

**数据边界：**

| 数据类型 | 读 | 写 | 格式 |
|----------|----|----|------|
| 工作目录配置 | `util.LoadJSON` | `util.SaveJSON` | JSON 文件 |
| 文件树数据 | `os.ReadDir` | — | 实时读取文件系统 |
| Git 信息 | `go-git`（读） | `exec.Cmd`（写） | 文件系统 + Git |
| 系统剪贴板 | `util/clipboard_windows.go` | `util/clipboard_windows.go` | Windows API |

### Integration Points

| 集成类型 | 入口 | 说明 |
|----------|------|------|
| VS Code | `exec.Cmd("code")` | 通过 PATH 调用 |
| 资源管理器 | `exec.Cmd("explorer")` | Windows 原生命令 |
| 系统默认程序 | `exec.Cmd("cmd", "/c", "start")` | Windows 文件关联 |
| Git CLI | `exec.Cmd("git")` | 写操作（clone/pull） |
| go-git | `git.PlainOpen` | 读操作（log/status/diff） |

## Architecture Validation Results

### Coherence Validation ✅

**Decision Compatibility：** 所有技术选型已投产验证，版本兼容无冲突。Wails v2 + Go 1.24+ + Vue 3.5.33 + Element Plus 2.13.7 组合稳定运行。

**Pattern Consistency：** 命名规范（Go PascalCase、JS camelCase、事件 kebab-case）、结构模式（三层架构）、格式模式（json 标签契约）、通信模式（Wails 事件 + Props/Emit）全部与实际代码库一致。

**Structure Alignment：** 项目结构完整支持所有架构决策，层间边界明确，集成点已映射。

### Requirements Coverage Validation ✅

**Functional Requirements Coverage：** 33 条 FR 全部映射到具体 Go 文件、前端组件和数据文件，覆盖率 100%。

**Non-Functional Requirements Coverage：** 9 条 NFR 全部有架构层面的支撑机制，覆盖率 100%。

### Implementation Readiness Validation ✅

**Decision Completeness：** 所有决策已记录并附带具体版本号和代码示例。

**Structure Completeness：** 完整目录结构已定义，每个文件都有职责说明。

**Pattern Completeness：** 6 个冲突点领域的模式已定义，含正面示例和 Anti-Patterns。

### Gap Analysis Results

- **Critical Gaps：** 无
- **Important Gaps：** 无
- **Deferred Items：** `.env` 预览安全性、`node_modules` 递归性能 — 已在 `deferred-work.md` 中记录

### Validation Issues Addressed

无阻塞性问题。棕地项目所有架构决策已投产验证。

### Architecture Completeness Checklist

**Requirements Analysis**

- [x] Project context thoroughly analyzed
- [x] Scale and complexity assessed
- [x] Technical constraints identified
- [x] Cross-cutting concerns mapped

**Architectural Decisions**

- [x] Critical decisions documented with versions
- [x] Technology stack fully specified
- [x] Integration patterns defined
- [x] Performance considerations addressed

**Implementation Patterns**

- [x] Naming conventions established
- [x] Structure patterns defined
- [x] Communication patterns specified
- [x] Process patterns documented

**Project Structure**

- [x] Complete directory structure defined
- [x] Component boundaries established
- [x] Integration points mapped
- [x] Requirements to structure mapping complete

### Architecture Readiness Assessment

**Overall Status:** READY FOR IMPLEMENTATION

**Confidence Level:** high — 棕地项目，所有决策已投产验证

**Key Strengths：**

- 三层架构职责边界清晰，app.go 只做调度
- 双 Git 引擎策略成熟，读/写分离无冲突
- 实现模式文档化，含强制规则和 Anti-Patterns
- 前端事件清理机制防止内存泄漏

**Areas for Future Enhancement：**

- Phase 3 多工具模块架构重构
- `.env` 等敏感文件预览安全机制
- 大型目录（`node_modules`）排除规则

### Implementation Handoff

**AI Agent Guidelines：**

- 遵循所有架构决策，不做未授权的技术选型变更
- 保持三层架构边界：app.go 调度、service 业务、util 工具
- 使用 Implementation Patterns 中的命名、结构、格式规范
- Service 层禁止导入 Wails 包
- 所有 exec.Cmd 调用必须使用 HideCommandWindow
