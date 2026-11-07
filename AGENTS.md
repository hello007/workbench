# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

> **本仓覆盖层纠偏**（2026-10-29 追加，位于管理块外，`trellis update` 不会覆盖）：块内 `.trellis/spec/` 描述已失效——该目录已于 2026-09-07 废弃移除，规范沉淀实际位于 `docs/spec/`（索引见 docs/spec/README.md）与本文「关键规则」清单，勿按块内旧路径查找或写入规范。

---

# 项目约定（AI 编程 Harness 导览）

> 仓根 `CLAUDE.md` 仅含 `@AGENTS.md` 一行（Claude Code 经 import 加载本文）；本文是全部 agent（Claude Code / Codex / 其他）的唯一约定入口：任务分流走 Trellis，深度契约走 `docs/spec/` 与本文「关键规则」，本文承载**通用工程导览与关键规则清单**（概览、命令、架构速查、规范入口）。
> **文档语言约定**：所有文档一律使用中文描述，且每次功能完成后都需要确认是否需要更新 README.md。
> 文中路径一律用反引号标注，表示**按需读取**——需要时再打开，不要一次性全读。

## 1. 项目概览

- **项目名称**：WorkBench - 开发者工作台
- **项目类型**：Wails 桌面应用（前后端同仓，另支持浏览器访问模式）
- **项目位置**：本仓根（Go 后端与 `frontend/` Vue 前端同仓）
- **开始时间**：2026-04-28
- **当前状态**：活跃开发中（v1.0.6+，持续迭代新功能）
- **交付形态**：Windows / Linux 桌面可执行文件（Linux 需 `-tags webkit2_41`）
- **版本管理**：版本号真相源在 `wails.json`，发版走 `/release` skill（`scripts/release.sh`）

**技术栈**：

|层级|技术|版本|
|---|---|---|
|后端语言|Go|1.26.10|
|桌面框架|Wails|v2.16.0|
|前端框架|Vue 3 (Composition API)|3.5.44|
|UI组件|Element Plus|2.14.5|
|路由|Vue Router|5.4.0|
|构建工具|Vite|8.3.4|
|后端测试|Go testing|-|
|前端测试|Vitest + Vue Test Utils|-|

**影响面说明**：
- App 方法签名与 `model/` 导出 struct 是前后端契约真相源，变更必须同步前端消费面。
- `frontend/wailsjs/` 与 `frontend/dist/` 为生成产物不入库，任何层禁止手改。

## 2. 常用命令

**执行前提**（构建拓扑闭环，必须成组执行）：
- 工作目录：Go 命令从**仓根**执行，前端命令在 `frontend/` 下执行；
- **干净检出后**，任何 `go build` / `go vet` / `go test` / `npm run build` 之前必须先：
  ```bash
  wails generate module                    # 生成 frontend/wailsjs/
  mkdir -p frontend/dist/assets && echo '<!doctype html>' > frontend/dist/index.html && echo '/* placeholder */' > frontend/dist/assets/app.js
  ```
  否则 `//go:embed all:frontend/dist` 报 no matching files found。

**常用命令**：

```bash
# 本地运行 / 调试
wails dev

# 构建应用
wails build                      # Linux 加 -tags webkit2_41

# Go 静态检查与测试（仓根）
go vet ./...
go test ./...
go test -tags=integration ./...
go test ./service/ -soak-rounds=2000 -run TestSoak -v   # 长跑稳定性深跑

# 前端构建与测试（frontend/ 下）
npm run build
npm test
npm run test:coverage
npm run e2e                      # Playwright E2E，首次先 npm run e2e:install

# 安装依赖
cd frontend && npm install

# 依赖一致性（仓根，零安装零网络）
node scripts/check-dep-consistency.mjs
```

**进程排查**（本机常驻 workbench.exe 监听 34115）：

```bash
netstat -ano | findstr ":34115"          # 查看端口占用
taskkill /F /IM workbench.exe            # 停止进程
```

本仓无 lint / format 命令（现状守护决策，门禁以 CI 为准，详见 `docs/agent-resources/conventions/process-build-quality.md`）。

## 3. 架构速查

```
Vue 组件 → transport（双通道收口）
   ├─ 桌面模式：Wails 绑定（wailsjs 生成）
   └─ 浏览器模式：WebSocket + HTTP RPC（server/ 通道）
        ↓
Go 桥接层（app_*.go）→ AppServices（app_services.go）→ service/ 业务层
        ↓
model/（契约真相源）、util/（工具）、server/（浏览器通道）
```

依赖方向严格单向向下；跨层契约与存量例外以 `docs/agent-resources/conventions/project-boundaries.md` 与 `docs/spec/` 为准。详细架构见 `docs/agent-resources/reference/architecture.md`。

## 4. 关键规则与规范沉淀

执行 trellis 工作流的 spec 沉淀（`trellis-update-spec`，workflow Phase 3.3）时，按以下两级分流写入：

1. **详细契约 / 主题文档**（跨层契约、签名同步规则等需完整上下文的内容）写入 `docs/spec/<topic>.md`，**禁止写入 `.trellis/spec/`**。该目录已废弃移除；本规则覆盖 `trellis-update-spec` skill 内的默认路径指令——即使 skill 文本仍指向旧路径，以本节为准。
2. **钩子级关键规则**（每次会话必知），追加到下方「关键规则」清单：行内 = 规则名 + 单点/禁令钩子 + 新增行指引，附指向 `docs/spec/` 详细文档的链接。**两档上限**（整行字符数，含文档列链接开销）：普通行 ≤250；钩子密集行（钩子 ≥7 个）与分期/合并行 ≤350。**下沉判据**：「下次写代码要照着做的」（禁令、单点函数名、错误码、同步点位）留行内；「为什么/怎么踩的」（机理、日期、事故经过、签名演变史）下沉 `docs/spec/`。**新行超上限须当场把超出细节下沉 `docs/spec/` 再回写钩子，禁先落行后治理**；分档细则与现存行归属见 [docs/spec/README.md](docs/spec/README.md)「两级分流规则」。

### 关键规则

|规则|详细文档|
|---|---|
|go-git 依赖已出清（v6 仅 beta 不升级），**禁重新引入**；HEAD 元数据（SHA/分支名/detached/unborn）走 `util.ResolveHead`（unborn 不报错由调用方分流）；remote 判定走 `repoHasRemote` 手写解析；refs 枚举走 `git for-each-ref`|[go-git-deprecation.md](docs/spec/go-git-deprecation.md)|
|修改 `app.go` App 方法签名或 `model/` 导出 struct 字段时，须手动同步 `frontend/wailsjs/` 绑定（App.js / App.d.ts / models.ts 三处）|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|`model/` 下 `type X string` 具名 string 类型作 Wails 绑定参数/返回值时，wails generate 不生成 models.ts 别名，须手动补 `export type X = string`，否则 `npm run build` 报 MISSING_EXPORT（vitest 不报易漏）|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|定制 `.trellis/workflow.md` 只改正文描述；增删 `[required · once]` 标记或步骤须同步对应 `[workflow-state:*]` 标签块，否则 trellis 回归测试失败|[workflow.md Customizing 章节](.trellis/workflow.md)|
|测试覆盖率分层门禁：后端 model/server ≥80% + service ≥76% 基线 + util ≥40%（排除 pty_windows.go）+ 主包不设门禁；前端 ≥70% 硬失败（exclude wailsjs）；改阈值须同步本文档 + docs/测试策略.md|[test-coverage-gate.md](docs/spec/test-coverage-gate.md)|
|新增 service 须在 `app_services.go` 加 `AppServices` 字段 + `NewAppServices` 构造行（2 处）；`AppServices` 必须在 package main；构造纯 new，生命周期副作用留 startup/shutdown|[app-services-assembly.md](docs/spec/app-services-assembly.md)|
|后端日志用 `log/slog`（禁 `println`）；service 调 `Logger()`，App 层调 `slog.X`；前端分流错误用 `model.AppError` 经 ErrorFormatter → `handleError` 按 code 分流；新增错误码三处同步（app_error.go + error.js ErrorCode/WARNING_CODES）；**wails 升级回归 ErrorFormatter reject 链路**|[logging-and-errors.md](docs/spec/logging-and-errors.md)|
|E2E 从 `frontend/e2e/fixtures.js` import `{ test, expect }`（禁直 import @playwright/test）；mock 单一数据源 `wails-mock-defaults.js`；禁 `waitForTimeout`；`wailsjs/` 不入库 CI 先 `wails generate module`；**新增 spec 目录须同步 vitest `test.exclude`**；挂起解析 `{__deferred__}`+`releaseDeferred`；fixture 必须对齐后端 model 真实形状|[e2e-testing.md](docs/spec/e2e-testing.md)|
|E2E window 级监听计数 hook **禁绝对基线断言**（基线取相对增量）；常驻 v-show 面板组件的 window 监听须随使用态挂载（FolderTree 先例）；单测 spy 须 afterEach restore + enableAutoUnmount 防实例泄漏；游离 DOM（未 attachTo）trigger 传播链不含 window|[e2e-testing.md §12](docs/spec/e2e-testing.md)|
|组件单测与 E2E 同用语义锚：活动栏高亮/点击禁 `findAll(...)[n]`，改用 `data-panel-id`/`data-action`；入口总数断言可保留；表格列断言禁硬编码 `td.nth(n)`（列增删下标漂移+零值文本撞车=假绿），经表头文本解析列下标（`columnIndexOf` 先例），负断言须升级正向内容断言|[e2e-testing.md §5.2/§5.3](docs/spec/e2e-testing.md)|
|前端还原 `ReadFileBytes` 返回的 base64 为文本用 `utils/base64.js` 的 `decodeBase64Utf8`，禁裸 `atob`（中文双重编码乱码，单测须用中文数据）|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|后端跨包共用测试辅助入 `util/testutil`（RunGit/WriteFile/InitTempRepo 等六个），与前端 `wails-mock-defaults.js` 单一数据源对齐；专用辅助留 `*_test_helper.go` 不导出；**禁各 `_test.go` 重复定义 git 命令/临时仓库辅助**；集成 `it*` helper 不并入 testutil；辅助参数用 `testing.TB`|[perf-baseline.md §17](docs/spec/perf-baseline.md)|
|依赖安全扫描：Go 用 `govulncheck ./...`，npm 用 `npm audit --registry=https://registry.npmjs.org --audit-level=high`（绕过 npmmirror）；CI security job 不阻塞 PR；标准库漏洞只能升 `go.mod` `toolchain` directive；升级后重跑 govulncheck 清零；npm 传递依赖用 `npm update <pkg>` 在 lock 内升，禁 `npm audit fix`/`--force`；moderate dev-only 见 §5.6|[security-scan.md](docs/spec/security-scan.md)|
|崩溃恢复 UI 快照 `data/session.json`；`Terminal` 字段用指针避 `omitempty` 失效（nil→前端判冷启动）；类型名 `TerminalSnapshot` 避让运行时会话；Load 损坏降级空快照；字段变更同步 wailsjs 三处|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|AI 结构化输出走 `--json-schema`：`AiFunction.OutputSchema` → `buildClaudeArgs` → `parseStreamLine` 提取 `structured_output` → 前端只渲染不解析；新增结构化 skill 零 service 代码；须配 `--verbose`|[ai-structured-output.md](docs/spec/ai-structured-output.md)|
|AI 功能项支持纯 prompt 模式（`Command` 空 + `Params.PromptTemplate` 驱动），`validateFunctions` 放宽为至少一非空；斜杠命令 skill 与纯 prompt skill 两类并存；纯 prompt 经 `BuildStagePrompt` 模板 `{{key}}` 占位注入|[ai-structured-output.md](docs/spec/ai-structured-output.md)|
|新增内置 seed skill 须加入 `mergeMissingSeedSkills`（service/ai_function.go）白名单，否则老用户配置报「功能不存在」；旧 seed 不进白名单尊重用户删除；用户自定义同 ID 不覆盖|[ai-structured-output.md](docs/spec/ai-structured-output.md)|
|Wails 事件多组件共听须用 `EventsOn` 返回闭包精准注销，禁 `EventsOff('eventName')` 全局移除；组件 repoPath 切换/卸载须重置 AI 任务态 + `CancelAiTask` 防串入 + loading 卡死|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|前端设计令牌：改 `--primary-color` 须同步 EP `--el-color-primary` 派生色阶否则组件割裂；冷暖灰统一蓝灰 Slate 禁混；暗色阴影禁纯黑带背景色相；`--terminal-bg` 与 `useTerminal.js` 终端主题严格一致；Geist 四字重 + font-display:swap|[design-tokens.md](docs/spec/design-tokens.md)|
|前端视觉风格守 frontend-visual-conventions：配色语义选色+蓝灰色阶；三列布局；nav 指示条 `left:0` 禁负值；字重梯度 h1-h3 600/h4-h6 500；**Home 面板根元素完整 flex 约束链**（缺则塌陷）；EP textarea 拖拽下限 `!important`；highlight.js 含 plaintext 回退；深色容器图标内联 SVG `fill="currentColor"` 禁 PNG，元素级图标走 `components/icons/` SFC|[frontend-visual-conventions.md](docs/spec/frontend-visual-conventions.md)|
|代码预览语法高亮按主题切换：深色色板在 `utils/codemirrorHighlight.js`（对齐 `.hljs-*` 暗色主题），Compartment 重配不重建编辑器；新 token 色须过 `#1e293b` 对比度 ≥3:1 单测|[frontend-visual-conventions.md](docs/spec/frontend-visual-conventions.md)|
|前端列表项选中只留 `@click` 单一入口，**禁 mousedown+click 双绑同一 handler**（单击 emit 两次致父级切换链重复执行）；单测须断言 emit 次数与 mousedown 不触发|[frontend-visual-conventions.md](docs/spec/frontend-visual-conventions.md)|
|service 发前端事件必须走 EventSink（`emitEvent(s.eventSink(), ...)`），禁直调 `runtime.EventsEmit`（非 Wails ctx 会杀进程）；serve 模式禁调窗口对话框；WSHub 事件带全局单调 seq，改 `Emit`/广播须维持「占号-入环-广播」同 `seqMu` 临界区与锁序，断线续传见 §2.4|[browser-channel.md](docs/spec/browser-channel.md)|
|终端多 tab：xterm 禁重挂载/搬 DOM（v-show+激活 refit）；tab watch 须 getter 投影；快照 v2 消费旧 `workDir` 后置空；全屏 z-index 1500 守 `fullscreen ⇒ visible`，ESC 过滤 defaultPrevented；外观热更后 refit；`el-input-number` 必带 `:value-on-clear`|[terminal-multi-session.md](docs/spec/terminal-multi-session.md)|
|终端关闭回收整树：Windows 每会话绑定 `KILL_ON_JOB_CLOSE` Job（`createKillOnCloseJob`/`bindJobOrWarn`，失败仅 Warn 降级）；`Close` 幂等且先断 ConPTY 后关 Job；Unix 仅 SIGTERM shell，语义不对称|[terminal-multi-session.md](docs/spec/terminal-multi-session.md)|
|终端退出检测：`terminal-exit` 唯一出口 `emitExitIfRunning`+`MarkExited`（在册检查与跃迁同持 `s.mu`，禁另发）；Windows 走 `ExitCh` 事件通道（Close 须 SetEvent→等 waitDone→关句柄），Unix/nil 回退 500ms 轮询；新平台 `PtyProcess` 须实现 `ExitCh`|[terminal-multi-session.md](docs/spec/terminal-multi-session.md)|
|终端 addon 面（xterm 6）：WebGL 先探测后挂载（渲染器构造在 open 首帧事件内，loadAddon catch 接不住）；无 onTextPaste，多行粘贴确认走容器捕获 DOM paste；serialize 在旧实例 dispose 前取文本；OSC `9;4` 恒放行、开关 handler 内实时判定；搜索条 Esc `.stop.prevent`；升 xterm major 四 addon 同批升级|[terminal-multi-session.md §11](docs/spec/terminal-multi-session.md)|
|新增带事件的 service 须同步 `web_serve.go` `applySink` 补 `SetEventSink` 行（漏加=浏览器通道静默收不到全部事件）；ChatService：chat-task:* 事件流隔离、同会话串行保护、assistant 回复 done 前落盘、选择题 chat-question 块（`--disallowedTools` 禁自动应答）、**spawn `--permission-mode` 恒传**（省略则绕过审批协议）|[ai-chat-service.md](docs/spec/ai-chat-service.md)|
|Linux 支持：Wails 桌面构建须 `-tags webkit2_41`；平台抽象——编译期字段独有走 `_windows/_other` 双文件、短分叉走运行时 GOOS 分支、纯逻辑抽无 tag 文件双侧同测；自更新 Linux 资产名与 release.yml 严格一致；`build/` 分发文件白名单放行；测试平台断言拆双平台文件禁共享；前端兜底禁 'powershell'/'C:\\' 字面量|[linux-platform.md](docs/spec/linux-platform.md)|
|测试禁隐式依赖环境外部状态：mtime 时序与固定端口（本机常驻 workbench.exe 监听 36115）均致偶发挂；web serve 启动测试注入随机端口（`:0`），能直绑 :0 禁新增 freePort 探测；mtime 判定用注入陈旧缓存驱动，禁 sleep 等待|[test-stability.md](docs/spec/test-stability.md)|
|统计类聚合缓存复用 `commitHistoryCacheKey` 键与 headSHA+TTL 判定，失效为二态；须在 `InvalidateCommitHistoryCache` 与 `ClearAllCommitHistoryCache` **两入口联动清除**，漏联动则行数与提交数口径漂移|[perf-baseline.md §18](docs/spec/perf-baseline.md)|
|data/ 目录 JSON 持久化必须走 `util.SaveJSON` 原子写，**禁新增 os.WriteFile 直写与自建 temp+rename**（已收敛委托）；崩溃残留 `.tmp-*` 点文件无害不自动清扫；原子写不解决整体覆盖丢数据，手改 data 文件前须关 workbench.exe|[data-atomic-write.md](docs/spec/data-atomic-write.md)|
|损坏文件留底备份统一走 `util.BackupCorruptFile`（`<路径>.bak.<SHA-256 前 12 位>`）：同内容恒一份、异内容各一份；**损坏留底禁时间戳命名（写前保险不适用）**（跨秒重复读取致备份无上限），测试可断言「恰好 1 份」|[data-atomic-write.md](docs/spec/data-atomic-write.md)|
|soak/泄漏三档轮次（`-soak-rounds` > `-short` > 默认档）；泄漏断言=双 `runtime.GC()` 后 heap 增量（service 8MB/主包 16MB）+ goroutine Δ≤2，辅助收敛 `util/testutil/soak.go`；goleak 仅 service 两档（Short 跳过/其余 VerifyTestMain 硬门禁）；带 goroutine 测试辅助必须 Cleanup 全链；两包同名 flag 分开传|[perf-baseline.md 第 13/14 节](docs/spec/perf-baseline.md)|
|文件树单层超 2000 截断（`utils/treeTruncate.js`）：截前 2000+哨兵节点（`truncation-hint`，NUL 合成 key）；**哨兵过滤面**：不选中/不弹右键/快捷键过滤/expandAll 兼容；覆盖层右键复用 `onNodeContextMenu`，rename/delete 走 `refreshAfterFilterOp`；paste/create/copyTo 经 `refreshAfterFileOp` 出口分流；渲染脚本依赖 `.truncation-hint-node` class|[perf-baseline.md §15.6](docs/spec/perf-baseline.md)|
|FileDiffDialog 关闭释放双时机：清空挂 `@closed`（渐隐期挂 watch 闪空）+ `loadSeq++` 复位挂关闭即时 watch；新增状态字段同步 `releaseDiffData` 清理；`getCurrentNode()` 返回 data 对象非 Node 实例|[perf-baseline.md §16.5](docs/spec/perf-baseline.md)|
|Git 取消链路：util 层 `ExecuteWithContext` 等 ctx 变体（`%w` wrap Canceled）；service 接入 = 自建 ctx + `registerCancel`/`unregisterCancel` LIFO + `mapCancelError`；覆盖 Pull/Clone/BatchPull+冲突七方法，**Abort 刻意无 ctx 变体**；锁残留仅 Warn 禁自动删；锁占用识别 `E_GIT_LOCKED`；超时立即返回 `E_GIT_TIMEOUT`；单测 = ctx 预先 cancel|[logging-and-errors.md](docs/spec/logging-and-errors.md)|
|内容搜索超时收口：`searchWithRipgrep` 失败分支**先判 `ctx.Err()`** 返 `E_SEARCH_TIMEOUT` 再走 exit 1 无匹配；rg 超时**禁回退** `searchWithGo`；部分超时 `TimedOut` 标志 append 独立 `timedOutGroups`，`mergeTimeoutGroups` 三态直测；前端标志组组级 warning（弃 ElMessage）；测试用 rg stub 与预取消 ctx|[logging-and-errors.md](docs/spec/logging-and-errors.md)|
|进程终止（工具箱）：Windows 优雅停止能否生效取决于目标进程是否响应无法预判，统一「发请求→5s 轮询存活→未退强杀」，**禁 taskkill SUCCESS 当已退出**；自我保护判据只用 PID+父进程链；`IsSelf`/`IsSystem` 后端填充前端只消费；终止命令必带超时且拼 `ExitError.Stderr`；端口占用一览零后端新增（防重按 PID）|[process-management.md](docs/spec/process-management.md)|
|多超时目录批量重试：前端编排 N 个单目录调用+信号量 3（每目录独立 60s ctx）；`retryOneDir(group, version)` 单目录/批量共用；`searchVersion` ref 守卫（写回比对丢弃、worker 开头提前 return、finally 不更新重试态）；批量预占 `retryingRepoPaths`；banner 条件 `batchRetrying \|\| timedOutCount >= 2`；service 零改动|[logging-and-errors.md](docs/spec/logging-and-errors.md)|
|PPT 拆分 GC 与素材拆分：GC=保留页链路（页→版式→母版→主题）rels 闭包剪除，**联动四处**（母版 lst 剪枝+rels 悬挂清理+母版 rels 删关系+**母版本体 sldLayoutIdLst 删条目**——漏四处=修复弹窗）；任一跳不可解析整段降级；页序号 %03d；素材拆分增量判定=记录+哈希+产物三重；**源副本代拆**；重拆**禁 RemoveAll 整目录**；事件 assets-split:* 独立前缀；单文件取消复用 E_ASSETS_CANCELLED|[pptx-split.md](docs/spec/pptx-split.md)|
|PPT 按页拆分：纯 Go OOXML（**禁 COM/外部 Office**）；页序三跳映射（slide 文件名≠页序，sldIdLst→rId→rels→part 名）；XML「解析定位+字节删除」禁 Encoder 重序列化；OPC Target 相对关系源 part 目录；取消复用 E_GIT_CANCELLED；事件 `pptx-split:*` 仅内嵌面板监听；fixture 程序化构造|[pptx-split.md](docs/spec/pptx-split.md)|
|PPT 文件夹批量拆分：`SplitFolder` 递归串行；产物排除复用 `isPptxSplitDerivative`（rel 先 ToSlash）；整批一个可取消 ctx 双检查点，Items 仅含已完成；**批量不逐文件发事件**（Item.Error 是失败唯一通道）；整批取消丢弃返回值（勿承诺保留部分结果）|[pptx-split.md](docs/spec/pptx-split.md)|
|工具箱面板化：第二个独占一级面板；7 工具 Dialog→Panel 去壳内嵌（el-dialog→div、初始化迁 onMounted、事件监听闭包注销）；**新工具禁走 el-dialog**，一律 XxxPanel.vue 经 `<component :is>` 切换；根元素完整 flex 约束链|[frontend-visual-conventions.md](docs/spec/frontend-visual-conventions.md)|
|素材管理（assets）：SQLite 只用 `modernc.org/sqlite` 纯 Go 零 CGO；FTS5 五契约——切分同源 `splitFtsText`、`detail=full`、standalone 事务双写、**trigram 禁用**（2 字词恒 0 行）、弃 snippet() 改原文窗口；单连接池 rows 未 Close 再查=死锁；FTS rowid 文件=+files.id/幻灯片=-slides.id；派生产物排除 `isPptxSplitDerivative`；事件带 libraryId，error 只复位不弹|[assets-management.md](docs/spec/assets-management.md)|
|素材缩略图 COM 队列：COM 调用锁专用 STA goroutine；**Presentations/Slides 是属性须 GetProperty**；超时唯一解=杀自启 COM 进程（快照 diff，{powerpnt.exe, wpp.exe}，**禁 taskkill /IM 全杀**）；`jr.Result` 缓冲 1 **恰一次写回**；缓存键 `<fileID>-<3位页>.png` **禁 slides.id**；fixture 一律 COM 自产；fake 测试须断言 worker 存活|[assets-thumbs-com.md](docs/spec/assets-thumbs-com.md)|
|素材缩略图导出与跳转：**Export 路径必须绝对**（由 COM 进程 cwd 解析）；**Slide.Export 不自建父目录**（先 MkdirAll）；补缺判定=**PNG 存在唯一权威**；thumb_cache 事务 `WHERE EXISTS(files)`；**瞬态重试**（3 次退避，ctx 取消绝不重试）+ **Discard 重建落定等待**；**跳转零可见双根因**：①崩溃残留误判（`com_owned_pids.json` 持久化、回收后并回）②WPS 默认不可见（`PutProperty Visible msoTrue`）|[assets-thumbs-com.md](docs/spec/assets-thumbs-com.md)|
|素材缩略图设置与消费（PR3）：设置段 `AppSettings.assets` 五项；**SettingsService 内存缓存**——Load 返回共享指针只读禁改；分辨率变更清档链=Cancel→删档→dropTier→RemoveAll（重试一次）；前端 GetAssetThumb **直拼 `data:image/png;base64,`**（禁 decodeBase64Utf8/atob）；失败收集 done 与**切库 watch 双复位**；RetryAssetThumb 过 IsAbs+Rel 越界遏制；设置弹窗失败基底 null **禁保存**|[assets-thumbs-com.md §9](docs/spec/assets-thumbs-com.md)|
|幻灯片独立标注（v5）：slide_tags/slide_meta 键 **`(file_id, slide_num)` 禁 slides.id**；继承合并单点后端（两路径同源）；重建按 path+slide_num 备份重绑（**defer 化**）；增量重解析事务内失配清理；非 ok 终态标注保留；标签备注不入 FTS；ownTags 恒空切片；前端 onSaveSlideMeta **双代次守卫+先清聚合展开态**，miss 以提交值回填，命中才 resetSlideDraft|[assets-management.md「幻灯片独立标注」节](docs/spec/assets-management.md)|
|批量打标签+标签树（v6）：tags.parent_id 零迁移；级联=`expandTagDescendants` 单点展开（**批量绑定不级联**）；批量 API 两方法禁 kind 单方法；**每条目一事务**；删除=单事务闭包+两绑定表 DELETE、防环 visited；AssetTagTreeSelect 三处复用（ID 口径映射、fetchSeq 守卫、loading 不透传）；AssetBatchTagDialog 不感知 viewMode（明细反查 `slideNum\|\|0` 归一）|[assets-management.md §9](docs/spec/assets-management.md)|
|v6 刷新链与拖拽（D8）：两刷新链带双代次守卫+清聚合缓存+**不清选中**；过滤语境 loadSlidesPage(true) 复位链必接 `ensureFilteredSlidesLoaded` 兜底；迁移 ALTER 加 sort_order+名称序回填；MoveTag 单事务防环+index 钳+组内连续化；el-tree 两落点换算同走 `resolveTagMoveDrop` 单点+失败回滚重拉；检索 flatten 默认 true（断言先回关）|[assets-management.md §9](docs/spec/assets-management.md)|
|facet 筛选+保存条件（v7）：构造单点 `assetsFacetFilter`（三路径共用；sizeFrom=0=不设下界）；空查询=三者**全空**才空页；有 facet 无关键词走 `searchByFacetsOnly` 返**文件条目**；keyword 防 N+1=`filterFileIDsByFacet` 分批 500；前端换算单点 AssetFilterPopover（自定义档**止日末毫秒**锚点，反推 Math.round）；保存 SaveJSON upsert；异库调取 tagIds 按 tagNames 重映射成对丢弃|[assets-management.md §3/§8.2/§8.6](docs/spec/assets-management.md)|
|检索排序（v8）：三路径共用 `sortEntriesByOptions` 单点（聚合前分页前）；口径统一文件级；次级键链 score→sortPath→sortNum→refID 保跨页稳定（**sortNum 必含**）；关键词路径 `fileSortPropsByIDs` 批量 IN，**props 缺键=剔除该 fileID 全部条目**；前端零内存排序|[assets-management.md §10.1](docs/spec/assets-management.md)|
|批量评分（v8 P0）：`BatchSetAssetRating` 单一入口混合文件/页（slideNum 0=文件）；每条目一事务；页条目 REPLACE **必须回读备注再整行写回**；前端 targets 收敛 `collectSelectionTargets` 单点+刷新链 `refreshAfterBatchAnnotation` 共享体；0=清除、rating 不入 FTS|[assets-management.md §10.2](docs/spec/assets-management.md)|
|导出元数据（§11.1）：长活导出**禁全程单 withStore**——三段式（物化行键用独立 UNION ALL→分批短会话水合→锁外写汇）；**批间禁 OFFSET 续读**；CSV 覆盖收口在 O_EXCL；CancelExport 仅取 opMu；弹窗提交中关闭=放弃语义；thumbnail_path 恒绝对|[assets-management.md §11.1](docs/spec/assets-management.md)|
|lost 状态机与重链（§11.2/11.3）：消失 DELETE 改 markFileLost（原值保留+FTS/标注存续）；**复活门槛 fileHasSlides 单点**（failed/encrypted/denied/0 页走重解析）+**processing 永不直复活**；前缀替换 renames 按 oldPath 长度降序+**配对禁下标 zip**（fileID map）；写面零命中=ErrNoRows 上抛；前端 toast 先判 reparsed 再判 hashMatched|[assets-management.md §11.2/§11.3](docs/spec/assets-management.md)|
|排除规则与 excluded（§12.1/12.2）：ExcludeDirs **单段 glob**（禁多段）大小写不敏感、基名匹配、root 豁免；**剪枝与判定共用同一 matcher**；判定**先排除后存在性**；excluded 检索不可见**导出照导**——**关键词路径须 excludedFileIDSet 显式剔除**；非 ok→ok **必须走 reviveLostFile 重建 FTS**（裸 UPDATE=检索永久漏文件）|[assets-management.md §12.1/§12.2](docs/spec/assets-management.md)|
|备份与还原（§12.3/12.4）：`VACUUM INTO` 单语句（**禁显式 Tx、禁裸拷贝 db**）；`integrity_check` **必须 Query 非 Exec**；备份名分隔符 `_`（保字典序=时间序）；**还原乱序即半态**：覆盖前完整性+版本预检（版本过新拒还原）→等扫描退出→CancelExport+Wait→**持 s.mu 复查 exportCancels 拒还原**→关连接→删 -wal/-shm→Rename 覆盖；删库前备份失败走 `allowNoBackup` 确认；哈希恢复两级匹配（路径优先、hash 兜底）|[assets-management.md §12.3/§12.4](docs/spec/assets-management.md)|
|素材购物栏与合并导出：条目键 `<libraryId>\|<fileId>\|<slideNum>` 三处同构，fileId 用 files.id 禁 slides.id；sldId/sldMasterId 数字解析**禁 `xml:"id,attr"`**（按 Name.Space 解析，parsePptxListIDs 单点）；三重冲突消解与失效跳过**单点在合并核心**；装配 pptxMergeSvc+assetCartSvc+applySink 补 SetEventSink；互斥 E_CART_EXPORT_IN_PROGRESS 三处同步|[pptx-merge.md](docs/spec/pptx-merge.md)|
|素材域刷新统一入口：任何变更点位刷新一律走 `refreshDataAfterMutation`（差异面经参数表达），**禁新增手写刷新编排**；非幻灯片视图置 `slidesStale` 惰性重拉；复位集中 loadSlidesPage(true) 与 resetAndReloadView；检索态重搜+清聚合缓存、双代次守卫、续拉兜底内聚入口|[assets-management.md §8.7](docs/spec/assets-management.md)|
|el-tree 批量展开/收起**禁直改 `expandedKeys`**（default-expanded-keys 仅新建 Node 生效，store 直赋 expanded 不触发事件）；须经 `store.nodesMap` 置 expanded，操作后统一 `syncExpandedKeysFromStore()` 单点回写，防 reload 展开面漂移|[assets-management.md §8.7](docs/spec/assets-management.md)|
|pptx 文件树预览：任务排 assets-thumbs 既有 STA worker（**禁第二 COM 队列**）；**失败/超时必须 Discard() 置脏**；**复用用户实例禁 SaveAs**（走 ExportAsFixedFormat），自启 SaveAs(32) 失败回退 (2)；缓存键=srcPath+size+mtime、LRU 512MB；前端代次守卫+onBeforeUnmount 复位转换态；**dev 模式 Vite fallback 吞 /preview-pdf**——vite.config.js 前置中间件提前 404（新增同源路由同步加前缀判定）|[pptx-preview.md](docs/spec/pptx-preview.md)|
|ElMessageBox.confirm 三形态：递归恒弹（仅素材导出）/顺序两段（Obsidian 注册链、删除会话、备份升级确认）/单次确认；EP 2.14 起关闭后实例驻留 DOM——E2E 递归/两段链须 `.first()`/`.last()` 或 `:visible`+`filter(hasText)`，裸选择器仅单次链安全；mock 是文件级 `vi.mock('element-plus')`；同构组件须各补守护|[confirm-dialog.md](docs/spec/confirm-dialog.md)|
|前端依赖分档钳制：39 顶层依赖四档——exact（element-plus 2.14.5+icons 禁漂移）/tilde（运行时 UI `~x.y.0`）/caret（工具链）/url（xlsx）；**新增依赖须先在 `check-dep-consistency.mjs` 四清单登记再改 package.json**；tilde minor 升级以 lock 实际落点重写声明；脚本改判定先过 `--selftest`；EP 升级走 SOP 3.4 专项|[frontend-dep-upgrade.md](docs/spec/frontend-dep-upgrade.md)|
|mermaid 渲染链：锁定 `import('mermaid')` 说明符（禁 dist 直指）+ initialize 五键 + `run({nodes, suppressErrors:true})`（v12 默认 false 会 throw）；**双键 `layout:'dagre'`+`look:'classic'` 有意决策**（去键吃 ELK/redux 新观感）；engines node>=22.12 CI 不得再降；E2E mock `PreviewFile` 必含 `kind:'text'`|[mermaid-rendering.md](docs/spec/mermaid-rendering.md)|
|前端依赖 major 升级要点：js-yaml v5 无 ESM default export 须 namespace import；markdown-it v15 不随包发 CHANGELOG（npm tarball diff 取证）；0.x 包 tilde 全锁；vue-router 5 零代码改动；breaking 评估全仓 grep 消费点（含 ：deep 样式）三态判定落 research|[frontend-dep-upgrade.md §3.6](docs/spec/frontend-dep-upgrade.md)|
|浏览器通道安全与连接管控（三期安全+一~十期，明细见 browser-channel.md 分期锚点）：**跨期通用钩子**——① 新增纯桌面数据面方法同步 `RPCHiddenMethods` 现清单 ② 新增认证判定两认证点同接（HTTP `requireToken`+WS `ServeHTTP`）③ 新错误码三处同步 ④ 他人 IP/token 明细只走桌面出口，浏览器载荷恒 count/网段粒度 ⑤ 归因复用 `rpcAuditTarget` 装饰器分叉 ⑥ 内存态封禁重启清零勿持久化；各期语义见 §2.2/§2.3/§3/§4/§6|[browser-channel.md](docs/spec/browser-channel.md)|
|审计留存扩展（十一期）：轮转链单文件 5MB 深度 N 档（默认 6）；`rotateLocked` 链式 rename（任一环失败 Warn 停链）；**Windows 读句柄致 sharing violation 停链、释放后自愈**——竞速断言平台分侧禁共享；`readSnapshot` 锁内快照多档合并（消费面透明受益）；`AuditSettings` 段+`EnsureAuditDefaults` 收编+`SetAuditConfig` 「保存+生效」单点；不做读侧缓存/.gz/时间触发轮转|[logging-and-errors.md §8.2](docs/spec/logging-and-errors.md)|
|设置段增删跨层同步双单点：`AppSettings.EnsureAllDefaults()`（三处消费）+ `AppSettings.PrivateCopy()`（指针段私有拷贝，三写入口合并写基底）；**新增设置段仅两单点各加一行**，禁消费点回退手抄串联/拷贝块；GetSettings 正常路径禁对共享缓存指针调 Ensure|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|el-tree 展开面维护：FolderTree `syncExpandedKeysFromStore` 单点=可达展开面过滤（收起须连带剔除其展开中子孙，防 EP default-expanded-keys 回放祖先链翻回；收起须 nextTick（EP 事件早于落值））；展开/收起行为断言禁自造 el-tree 桩（假绿），须真实 EP mount|[el-tree-expand-state.md](docs/spec/el-tree-expand-state.md)|
|素材扫描 pipeline：scanLibrary 两阶段（串行判定+快路径 / `processPendingScanFiles` 并行解析零 DB+串行落库，worker=`min(GOMAXPROCS,4)`）；解析段取消=整批丢弃零落库；进度 done=完成计数；子目录重建 `RebuildAssetFolder`（LIKE 转义单点、无 VACUUM 备份、范围外零触碰）；`processScanFile` 组合仅供单文件重链|[assets-management.md §14](docs/spec/assets-management.md)|
|素材缩略图 hd 按需与增量重建：hd 退出批量补缺（`batchBacklogTiers` 只取 small），按需走 `demandCh` 独立通道（**不与批量互相取消**）；页白名单批次**禁 pruneBeyond**（会误删后续页产物）；重建保留 fileID（占位行显式主键、序列不回退）+ hash 相同保留 PNG + 收尾清消失行与孤儿 PNG|[assets-thumbs-com.md §9.2](docs/spec/assets-thumbs-com.md)|
|素材拆分 replace 落点：产物名 `<3位页序>-<名字主干>.pptx` 不命中 isPptxSplitDerivative（数字主干加 `-p`，改判定回归）；全页 Stat+索引+标注三单事务过才移源（禁 FOF_NOCONFIRMATION）；旧产物延迟清理（失败/取消经 `.replace-bak` 还原）；源哈希必须重算；空主干/二次拆分拒绝（批量按记录剔除）；不增量跳过；3-7 失败/取消回滚，删源失败不回滚且补写记录（source_removed=0）；`split_records` v8 target/source_removed|[pptx-split.md](docs/spec/pptx-split.md)|
|素材拆分批量取消保留已完成：返回 Items+`Canceled=true`+**nil error**、终态 `assets-split:done`（单文件仍 E_ASSETS_CANCELLED）；Items 仅已完成、Total 仍快照总数；循环后补判 ctx 竞态|[assets-management.md §15.7](docs/spec/assets-management.md)|
|目录级路径迁移：拆分记录随目录重命名/移动/前缀替换同事务迁移（`migrateSplitRecordsTx`，与 files.path/folder 键同事务）；source_removed=0 守卫已解除；source/replace 的 output_dir 按库根重算（容器目录被移动时按旧前缀剥离迁移，cache 保持）；前缀剥离 `stripDirPathPrefix` 用 EqualFold 禁 TrimPrefix；嵌套前缀按旧路径长度降序防误删；冲突保留 source_removed=1；单页产物禁单独移动|[assets-management.md §8.7](docs/spec/assets-management.md)|
|单文件移动拆分键：`moveFilePath` 必走 `rewriteSplitRecordKeyTx`（禁裸 `UPDATE split_records`），`newOutDir` 传空串保持 output_dir；冲突保留 `source_removed=1` 并 Warn，幂等；磁盘 rename 先行的既有半态语义不变|[assets-management.md §8.7](docs/spec/assets-management.md)|
|文件级重链拆分记录口径：RelocateAssetFile 须与 files.path 同事务迁移 split_records 源键，output_dir 恒保原绝对路径（重算致单页失效）；产物定位一律走 splitArtifactPath（直拼+目录反查，覆盖改名重链）；目录迁移重算落点前须判 output_dir 在旧前缀下；force 重链与清理丢失同事务删记录|[assets-management.md §11.3](docs/spec/assets-management.md)|
|重拆清旧页锚定 `split_records.artifact_stem`（v9，空值回落）：`splitArtifactCleanupPlan` 四分支（M1/M4）只删主干同页唯一常规文件，`excludeStem` 兜 M1；跳过只比 target；mtime>split_at+2s 保留；主干须属候选（M2）；不可判定零删+Warn；删后联 `deleteIndexRowsForPaths`|[pptx-split.md §9.5](docs/spec/pptx-split.md)|
|拆分产物指纹（v10/v11）：指纹 allow-list 全等删，不可用时 `cleanupStaleReplaceArtifacts` 按名删、另两处回 mtime；retained 交 `cleanupRetainedSplitArtifacts`（按条目目录锚消费，失败续携，旧数据空锚回落 output_dir）；凭证放行；**页序解析点全 ≥2 位**；换回文案含备份|[pptx-split.md §9.5](docs/spec/pptx-split.md)|
|拆分失败语义：source/cache **核心成功后才清旧页**（`removeSplitArtifactsBeyond` 删页序>keepPage+历史 `%02d`，禁前置删/RemoveAll）；折叠**仅 Windows**，`isReplaceArtifactName`/`foldNameInSet`/`resolveExistingDirCase`/`isReplaceCoreArtifactName` 同口径；source/cache 失败仅 Warn（禁称收敛），replace 转存 v11 `retained_fingerprints`（重拆消费自愈）|[pptx-split.md §9.5](docs/spec/pptx-split.md)|
|重链落点随源搬迁：`resolveRelocatedSplitOutDir` 探测 `<新源目录>/<新主干>`，仅 source/replace 且 `findSplitArtifactByPage(候选,target,1)` 命中才改 `output_dir`（`newOutDir` 经 `rewriteSplitRecordKeyTx` 同事务），等价/形态不符/cache 保持旧值；落点布局统一 `splitSourceOutputDir`|[assets-management.md §11.3](docs/spec/assets-management.md)|
|拆分落点归属：source/cache 算出 outDir 后先查 `splitRecordByOutputDir(outDir, rel)`（同源排除 + `samePathFold`），命中他人记录返回 `E_ASSETS_SPLIT_OUTPUT_CONFLICT`（warning）禁删覆盖；`splitArtifactPath` 反查主干不符不采纳|[pptx-split.md §9.5](docs/spec/pptx-split.md)|
|新增解析提交日志/中文名文件的 git 调用须自带 UTF-8 `-c` 前缀（`-c i18n.logOutputEncoding=UTF-8 -c i18n.commitEncoding=UTF-8 -c core.quotepath=off`，`GitCommand` 不默认带）+ `%x00` 记录 `%x01` 字段分隔（禁 `|` 等可出现在正文中的分隔符）；双进程管道（如 `log -p | patch-id`）走 `util.GitCommand.ExecutePipeline`（git 参数切片不含程序名），错误归一化镜像 run()，失败可降级不阻塞|[branch-compare.md](docs/spec/branch-compare.md)|
|冲突文件标记解析（utils/conflictMarkers.js）行尾 `\r` 必须逐字节保留（按 `\n` 切分、分隔行正则容忍 `\r?`，CRLF 往返回归测试锁死），块间上下文经每块 `lead` 行保序；rebase 冲突 ours/theirs 语义反转（以当前为准=变基目标侧），改冲突 UI 不得丢该提示；冲突类新方法须前置冲突态 + `--diff-filter=U` 成员校验，变更类走 tryLockRepo|[conflict-resolve-inapp.md](docs/spec/conflict-resolve-inapp.md)|
|stash 域：git 调用全带 utf8Prefix（含 status 查冲突，中文名否则八进制转义）；pop/apply 冲突判定走 porcelain 未合并八态非 stderr 文案；冲突详情须 ExecuteWithCodesAndStderr（CONFLICT 行走 stderr）；冲突态正常返回 ConflictFiles 零错误码；变更类同 Commit 先例不注册取消|[stash-management.md](docs/spec/stash-management.md)|
|终端联动（chatMarkdown 代码块「在终端执行」）写入 PTY 的命令结尾**禁带换行**（=自动回车）且经 `pre code` textContent 取值（禁 data 属性存命令）；fence 渲染形态改动须同步 renderMsgMarkdown 的 replaceAll 锚点与 chatMarkdown.spec 整串断言；uiStore 信箱 seq 单调递增、消费即清空，TerminalPanel 消费直调 WriteTerminalInput（不经 useTerminal.onData）|[terminal-chat-link.md](docs/spec/terminal-chat-link.md)|
|http(s) 外链打开唯一出口 runtime.BrowserOpenURL（出口内前置校验非 http(s) 不打开），禁 window.open/location.href（Wails WebView2 下内嵌导航不经系统 shell）；serve 桥属出口实现豁免；先例 AiChatPanel/GitInfo/FilePreviewRenderer/useTerminal|[url-opening.md](docs/spec/url-opening.md)|
|提交前强制同步：`git fetch origin` → `git rebase origin/master`（未提交改动先 stash/提交），冲突 `git add` + `git rebase --continue`；禁 merge 提交、禁对共享 master `--force`；推送被拒重跑 fetch+rebase；origin=GitHub SSH|[开发规范.md §Git 提交规范](docs/开发规范.md#git-提交规范)|
|聚合视图：根层 `GetAggregatedRootNodes`（`RPCHiddenMethods`）；文件收藏跳过；失效 Missing（`isLeaf=true`+同步`deepCopyNode`）；状态 key `treeState:__aggregated__`；筛选无目录回退、回溯最长前缀；相对/引用基准单点 `relativePathBase`=所属收藏根；空白区仅刷新/粘贴；收藏变更 watch 去抖代次；根刷新清徽标重建；选工作目录=退出聚合(`onDirectorySelect`)+序号守卫；失效提示 `MISSING_FAVORITE_HINT`|[aggregated-workspace.md](docs/spec/aggregated-workspace.md)|
|文件级历史（--follow）：git 调用须 `:(literal)` pathspec（文件名可含 glob 元字符）+ utf8Prefix + %x00/%x01；diff 查看 file 恒取 `pathAtCommit`（旧段路径漂移）；入口仅文件节点（目录禁入，聚合恒显示后端兜底）；弹窗关闭双时机 @closed + loadSeq 守卫|[file-level-history.md](docs/spec/file-level-history.md)|
|行级归属（git blame）：blame 路径参数是字面路径**禁 `:(literal)`**（与 log 域相反）；porcelain author-time/author-tz 分行到齐合成作者墙钟；工作树 blame 零 SHA=未提交禁跳转；超 10000 行返 E_BLAME_FILE_TOO_LARGE（warning）；行数守卫≠文档行数整组复位禁静默错位；JSON 派生视图隐藏入口；前端委托点击用例须 attachTo（游离树冒泡不到 document）|[git-blame.md](docs/spec/git-blame.md)|
|提交历史→文件级追溯入口：分流单点 `useTraceBlame`（CheckFileExists+blameCapableExt 双预判，降级恒转 FileHistoryDialog 带 initialSha）；uiStore `pushTraceBlameRequest` 信箱（seq 单调递增、消费即清）→Home onNodeSelect；`autoBlameSeq` 须与 filePreview 赋值同批+消费点 nextTick 防复位竞态；FileDiffDialog↔FileHistoryDialog 互嵌挂载点必 v-if 外控（静态环栈溢出）；CheckFileExists 纯桌面面已入 RPCHiddenMethods|[file-level-history.md §7](docs/spec/file-level-history.md)|
## 5. 项目长期记忆（docs/memory）

项目长期记忆存放于 `docs/memory/`（git 版本管理，跨机器可用），**是唯一权威源**，user 级自动记忆目录已废弃（2026-10-20 迁移）。

- 每条记忆一个 markdown 文件：frontmatter（`name`/`description`/`metadata.type`，type 取 user/feedback/project/reference）+ 正文，正文内用 `[[name]]` 互链
- 索引统一登记在 `docs/memory/MEMORY.md`，一行一条（标题 + 一句话摘要）
- 自动加载：SessionStart（startup/clear/compact）经 `.claude/hooks/inject-memory.py` 注入索引内容，正文按索引相对路径按需读取
- **新增记忆**：写入 `docs/memory/<name>.md` 并同步更新 `MEMORY.md` 索引；若发现 user 级目录（`~/.claude/projects/<工程>/memory/`）出现索引未收录的新条目（harness auto-memory 仍可能写入），先人工合并回 `docs/memory/` 再消费

## 6. 开发规范

通用工程规范在 `docs/agent-resources/conventions/`（入口：`docs/agent-resources/index-convention.md`），写代码前按需读取。

**体系分工**（本仓三层规范，冲突时优先级从高到低）：
1. 本文「关键规则」+ `docs/spec/` — 本项目踩坑沉淀的深度契约（最优先）
2. `docs/agent-resources/conventions/` — 跨技术栈通用工程规范
3. `.trellis/workflow.md` — 任务分流与流程

## 7. 硬性约束

### Git
- **不主动提交、不主动推送**：agent 不得未经触发自行执行 `git commit` / `git push`；完成任务后停止并报告，提交由用户决定。
- **用户显式触发提交流程时必须执行，不得推回**：用户明确指定提交方式时按其要求完成。
- **push 始终手动**；共享 master 禁 `--force`，推送前 fetch + rebase（细则见 `docs/开发规范.md`）。

### 安全与兼容性
- **绝不提交敏感凭据**：不提交真实密码、Token、生产证书或私钥；内网地址与凭据不得写入本仓库（远端为 GitHub）。
- **向后兼容**：绑定方法、`model/` struct、配置 key、错误码变更默认保持向后兼容。
- **禁止降级检查**：严禁为了测试通过而删除校验逻辑、跳过安全检查或吞没异常。

### 工作流程（任务分流）

**任务分流由 Trellis 承担**：每回合 workflow-state 注入判定（问答直答 / 建任务走 `trellis-brainstorm` / 内联小修须用户明示）。场景级分步剧本（开发、修 Bug、写文档、Code Review）按需读 `docs/agent-resources/workflows/`（索引：`index-workflows.md`）。

### 代码搜索与检索
- 代码结构、调用链、影响面优先使用 CodeGraph（`codegraph_*` 工具）；
- 文本与关键字检索使用 grep；
- 已知具体文件直接使用阅读工具按需查看。

## 8. 常见坑点

1. **生成产物缺失假失败**：干净检出直接跑 `go build`/`vite build` 因 `wailsjs/`、`dist/` 缺失报错——先按 §2 前置链生成，不是代码问题。
2. **wailsjs 手改失效**：`frontend/wailsjs/` 是生成产物，手改下次 generate 即被覆盖；契约变更必须改 Go 侧真相源。
3. **中文编码双重乱码**：前端还原 base64 二进制文本时裸 `atob` 会把中文搞乱，须走项目 `decodeBase64Utf8`。
4. **模式分流时机**：桌面/浏览器双模式判定依赖启动前安装顺序（`installBrowserTransport()` 在 mount 前），调整 import 顺序可能静默破坏浏览器模式。

其余已知坑查阅 `docs/agent-resources/index-troubleshooting.md`（初始为空，典型排障与解法应沉淀至此，避免重复排查）。

## 9. 该读哪份文件

`docs/agent-resources/` 下按**内容性质**分类，都入库共享、agent 通用，按需读取（总导览与 SSOT 裁判表：`docs/agent-resources/README.md`）：

| 类别 | 位置 | 性质 |
|------|------|------|
| 约定 | `docs/agent-resources/conventions/` + `index-convention.md` | 该怎么做。祈使句，能判定违规 |
| 决策记录 | `docs/agent-resources/adrs/` + `index-adr.md` | 当初为什么这么定。不可变，状态机 |
| 参考 | `docs/agent-resources/reference/` + `index-reference.md` | 现状是什么。客观描述，易腐 |
| 排障库 | `docs/agent-resources/troubleshooting/` + `index-troubleshooting.md` | 已验证报错解法，带审核到期 |
| 工作流 | `docs/agent-resources/workflows/` + `index-workflows.md` | 怎么做某类事。场景剧本 |
| 临时会话记忆 | `docs/agent-resources/temp-memory/` | 待办/决策/套路。每条都有出口 |

常用入口：
- 查通用代码规范 → `docs/agent-resources/index-convention.md`
- 查本项目深度契约 → 本文「关键规则」+ `docs/spec/`
- 查正式文档目录结构与入口清单 → `docs/agent-resources/reference/docs-management.md`
- 查看待办与欠账 → `docs/agent-resources/temp-memory/TEMP-MEM-activeContext.md`
- 选择场景剧本 → `docs/agent-resources/index-workflows.md`

---

**最后更新：** 2026-11-07
**文档版本：** v2.49
