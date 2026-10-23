# 研究：WorkBench git 执行管道与新面板接入点

## 1. git 命令执行（util/git.go）

- `GitCommand` 统一封装：`run(ctx, workDir, acceptedExitCodes, args...)`
  - 取消 → `%w` 包装 `context.Canceled`（service 层 `mapCancelError` → `E_GIT_CANCELLED`）
  - 超时 → `E_GIT_TIMEOUT` AppError
  - index.lock → `E_GIT_LOCKED`
- 方法：`Execute` / `ExecuteWithContext` / `ExecuteWithCodes(WithContext)`；`NewGitCommandWithTimeout(d)` 可调超时（默认 30s）
- **缺口**：现无双进程管道执行器 → 本任务新增 `ExecutePipeline(ctx, workDir, first, second []string)`（cmd1.StdoutPipe → cmd2.Stdin，错误归一化对齐 run()）
- **注意**：现有代码未默认设置 UTF-8 `-c` 前缀（既有潜在不一致，本任务不全局改）；新服务自行携带 `-c i18n.logOutputEncoding=UTF-8 -c i18n.commitEncoding=UTF-8 -c core.quotepath=off`

## 2. 可直接复用的既有能力（零新后端）

| 能力 | 落点 |
|---|---|
| 分支列表 | `service/git.go:320 GetBranches(dirPath)` → `App.GetBranches`（app_git.go:668）；remote 分支名 `origin/xxx` 直接可作 rev |
| 当前分支 | `App.GetGitInfo(path)`（app_git.go:19）→ `.Branch`（detached 为空） |
| 单文件提交 diff | `service/git.go:978 GetCommitFileDiff` → `App.GetCommitFileDiff`（root commit 对空树） |
| 目录浏览 | `App.OpenDirectoryDialog`（app_preview.go:64）；serve 模式有 `wailsRuntimeUnavailable()` 守卫 |
| 日志解析范式 | `commit_history_cli.go`：`commitLogFormat = "%x00%H%x01%P%x01%an%x01%ae%x01%at%x01%ai%x01%B%x01"`（NUL 记录、%x01 字段） |

## 3. 装配点（docs/spec/app-services-assembly.md）

- `app_services.go`：`AppServices` 结构体加字段（**必须 package main**，跨包嵌入不提升未导出字段）+ `NewAppServices(ctx, dataDir, isDev)` 纯构造一行；启动副作用只放 `App.startup`（本任务无）
- `app_<feature>.go`：`*App` 薄委托 + 中文 doc 注释；Wails 自动暴露导出方法；文件头注释指向 `docs/spec/cross-layer-contracts.md`
- **App 方法不带 ctx**（对齐 GetCommitHistory 模式，规避 serve RPC 兼容面）

## 4. 并发/锁

- `tryLockRepo` 仅管**变更类**操作；只读查询（对齐 ListTags/ListRemotes）不抢锁
- 只读批量并发范式：`sem := make(chan struct{}, 8)` + WaitGroup

## 5. 错误码（docs/spec/logging-and-errors.md）

- `model/app_error.go` + `frontend/src/utils/error.js` 双侧同步；本任务**零新错误码**（复用 E_GIT_TIMEOUT / E_GIT_CANCELLED / E_GIT_LOCKED）
- 取消不新造码，复用 E_GIT_CANCELLED

## 6. 前端接入点

- 面板注册三件套：`components/ActivityBar.vue` panels 数组（icon 可自绘放 `components/icons/`）→ `store/ui.js` activePanel 并集注释 → `views/Home.vue` `v-show` 常驻挂载（**v-show 不是 v-if**）
- 绑定调用：`import { Xxx } from '../../wailsjs/go/main/App'`；浏览器模式由 `transport/index.js` RPC 代理透明化；`wailsjs/` gitignored，CI 先 `wails generate module`
- 惯例：`requestSeq` 守卫丢旧响应（StatsView.vue:66 范式）、`utils/gitCache.js` 缓存、`ElMessage` toast、`handleError` code 分流
- 视觉：`docs/spec/design-tokens.md` CSS 变量 + `frontend-visual-conventions.md`（ActivityBar 48px、active 指示 `left:0`）
- 中文硬编码文案，无 i18n

## 7. 测试范式（docs/spec/test-coverage-gate.md / test-stability.md）

- service 单测同包小写函数直测；**禁止 time.Sleep / 固定端口**
- 集成测试根目录 `//go:build integration`（参考 `git_flows_integration_test.go`），t.TempDir 真实 git fixture + 本地 bare remote，无网络
- 覆盖率门槛：service ≥76%、frontend ≥70%（`scripts/coverage-check.sh`）
- 前端：Vitest + jsdom（`src/test/setup.js` mock Wails；`src/test/wails-mock-defaults.js` 默认返回值）；Playwright mock channel 在 `frontend/e2e/`
- service 包有 goleak TestMain——服务方法不得泄漏 goroutine

## 8. 事件（本任务不涉及）

纯请求/响应、无事件 → `web_serve.go applySink` 不动。batch 2 若加取消/进度事件，须同步 applySink（docs/spec/browser-channel.md）。
