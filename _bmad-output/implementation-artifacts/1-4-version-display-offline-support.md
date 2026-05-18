# Story 1.4: 版本号显示与离线支持

Status: done

## Story

As a 开发者,
I want 应用显示当前版本号，且所有本地功能离线可用,
so that 我知道当前版本，且在无网络环境下仍可使用本地功能。

## Acceptance Criteria

1. **AC1 - 版本号显示（FR32）**：应用启动后，界面显示当前版本号（格式如 `v1.0.0`），版本号来源于 Go 编译时 ldflags 注入
2. **AC2 - 离线可用（FR33）**：应用在无网络环境下运行时，所有本地功能（文件浏览、文件操作、提交历史查看）正常工作；Git 网络操作（clone/pull）失败时给出明确提示，不影响其他功能

## Tasks / Subtasks

- [x] Task 1: 暴露版本信息到前端（AC: #1）
  - [x] 1.1 阅读 main.go 的 version/buildTime ldflags 变量，确认版本注入机制
  - [x] 1.2 阅读 app.go 确认无现有 GetVersion 绑定方法
  - [x] 1.3 在 app.go 新增 `GetAppVersion() string` 绑定方法，返回 version 变量值
  - [x] 1.4 确认 `wails dev` 启动后前端能通过 Wails 绑定调用 `GetAppVersion()`

- [x] Task 2: 前端显示版本号（AC: #1）
  - [x] 2.1 阅读 Home.vue 布局，确定版本号显示位置（建议 DirectoryTree 面板底部）
  - [x] 2.2 在 DirectoryTree.vue 的 dir-toolbar-title 旁或面板底部添加版本号显示
  - [x] 2.3 在 Home.vue 的 onMounted 中调用 GetAppVersion() 获取版本号并传递给子组件
  - [x] 2.4 更新 setup.js 添加 GetAppVersion mock
  - [x] 2.5 验证版本号在开发模式和构建模式下均正确显示

- [x] Task 3: 验证离线功能可用（AC: #2）
  - [x] 3.1 阅读 GitService 中 clone/pull 相关方法，确认网络错误处理链路
  - [x] 3.2 阅读 app.go 中 PullRepo、ScanAndPullRepos 等方法，确认错误返回格式
  - [x] 3.3 验证现有代码：网络操作失败时已通过 ElMessage 提示，不影响本地功能
  - [x] 3.4 手动测试：断开网络后本地功能（文件浏览、文件操作、提交历史）正常工作

- [x] Task 4: 编写测试（AC: #1, #2）
  - [x] 4.1 编写 app.go 的 GetAppVersion 测试（或验证 main.go ldflags 机制）
  - [x] 4.2 编写前端 DirectoryTree 版本显示测试（接收 version prop 并渲染）
  - [x] 4.3 运行全量测试确认无回归

## Dev Notes

### 棕地项目背景

**版本号显示为全新功能**（main.go 已有 ldflags 变量但未暴露给前端）。**离线支持属于验证性质**——Wails 桌面应用本身是离线可用的（前端资源嵌入 Go 二进制），需确认 Git 网络操作的错误处理已足够友好。

### 现有实现分析

**Go 后端 — 版本信息：**

- `main.go:14-17` — ldflags 变量：
  ```go
  var (
      version   = "dev"
      buildTime = "unknown"
  )
  ```
- `main.go:23-26` — 支持 `--version` 命令行参数
- **`app.go` 无 `GetVersion` 绑定方法** — 需要新增
- `wails.json:16` — `"productVersion": "1.0.0"`（构建元数据，前端无法读取）

**前端 — 版本显示：**

- `Home.vue` — 三栏布局（directory-aside + file-tree-aside + content-main），无版本显示区域
- `DirectoryTree.vue` — 工具栏有 `dir-toolbar-title`（"工作目录"），版本号可在此附近显示

**离线支持分析：**

- Wails 应用前端资源通过 `//go:embed all:frontend/dist` 嵌入，天然离线可用
- Git 网络操作（PullRepo、ScanAndPullRepos）使用 `exec.Cmd("git pull")`，失败时返回错误字符串
- 前端已通过 `try/catch + ElMessage.error` 处理网络错误
- 本地功能（文件树、文件操作、提交历史）使用 `go-git` 库直接读本地 `.git` 目录，不依赖网络

### 版本号显示位置建议

**推荐位置：DirectoryTree.vue 工具栏底部或 dir-toolbar-title 旁**

理由：
- DirectoryTree 是最左侧面板，版本号常驻可见
- 不需要新增全局 header（避免改动布局结构）
- 工具栏区域已有文字元素，添加版本号不突兀

### 架构约束

- **app.go 调度层**：`GetAppVersion()` 方法应 ≤10 行，直接返回全局变量
- **禁止**：引入新依赖、TypeScript、CSS 框架
- **版本信息不需要 service 层**：纯静态数据，app.go 直接读取 main.go 全局变量即可

### 前一个 Story 的经验教训（Story 1-3）

1. **setup.js mock 必须同步更新**：新增 Go 绑定方法后，必须在 setup.js 添加对应 mock
2. **Home.spec.js mock 路径**：使用 `vi.importMock('../../../wailsjs/go/main/App')` 获取 mock（三级 `../`）
3. **mockClear 优于 restoreAllMocks**：避免破坏 setup.js 全局 mock
4. **测试使用 t.TempDir()**：Go 测试使用真实文件系统

### 测试注意事项

**前端测试（DirectoryTree.spec.js 扩展）：**
- 新增 version prop 测试：传入版本号字符串，验证渲染
- setup.js 需添加 `GetAppVersion: vi.fn(() => Promise.resolve('dev'))`

**Go 测试：**
- `GetAppVersion()` 是纯静态方法，测试价值有限
- 重点验证 ldflags 在 `wails build` 时正确注入（构建验证，非单元测试）

### References

- [Source: main.go:14-17] — version/buildTime ldflags 变量
- [Source: main.go:23-26] — --version 命令行支持
- [Source: app.go] — Wails 绑定方法列表（无 GetVersion）
- [Source: frontend/src/views/Home.vue] — 三栏布局，onMounted 中可获取版本号
- [Source: frontend/src/components/DirectoryTree.vue] — dir-toolbar-title 区域
- [Source: wails.json:16] — productVersion "1.0.0"
- [Source: frontend/src/test/setup.js] — 全局 mock 配置
- [Source: docs/project-context.md] — 编码规范和架构约束

## Dev Agent Record

### Agent Model Used

### Debug Log References

### Completion Notes List

### File List
