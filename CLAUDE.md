# WorkBench 项目

> 所有文档一律使用中文描述，且每次功能完成后都需要确认是否需要更新 README.md

## 项目概述

**项目名称：** WorkBench - 开发者工作台
**项目路径：** `d:\workspace\workspace_ai\demo_OpenSpec\git_tools\workbench\`
**Git 仓库：** `d:\workspace\workspace_ai\demo_OpenSpec\git_tools\workbench\`
**开始时间：** 2026-04-28
**当前状态：** 活跃开发中（v1.0.6+，持续迭代新功能）

## 技术栈

|层级|技术|版本|
|---|---|---|
|后端语言|Go|1.26.6|
|桌面框架|Wails|v2.16.0|
|前端框架|Vue 3 (Composition API)|3.5.33|
|UI组件|Element Plus|2.13.7|
|路由|Vue Router|4.6.4|
|构建工具|Vite|8.0.10|
|后端测试|Go testing|-|
|前端测试|Vitest + Vue Test Utils|-|

## 项目结构

```text
workbench/
├── main.go              # 主入口
├── app.go               # 应用结构体（前后端桥接）
├── model/               # 数据模型层
├── service/             # 业务逻辑层
├── util/                # 工具层
├── data/                # 数据配置（不提交Git）
├── frontend/            # Vue3前端
├── build/               # 构建输出
├── wails.json           # Wails配置
├── DEVELOPMENT.md       # 开发运维文档
└── BUILD_SUMMARY.md     # 构建摘要
```

## 规范沉淀规则

执行 trellis 工作流的 spec 沉淀（`trellis-update-spec`，workflow Phase 3.3）时，按以下两级分流写入：

1. **详细契约 / 主题文档**（跨层契约、签名同步规则等需完整上下文的内容）写入 `docs/spec/<topic>.md`，**禁止写入 `.trellis/spec/`**。该目录已废弃移除；本规则覆盖 `trellis-update-spec` skill 内的默认路径指令——即使 skill 文本仍指向旧路径，以本节为准。
2. **一两句级每次会话必知的关键规则**，直接追加到下方「关键规则」清单，并附指向 `docs/spec/` 详细文档的链接（如有）。

### 关键规则

|规则|详细文档|
|---|---|
|修改 `app.go` App 方法签名或 `model/` 导出 struct 字段时，须手动同步 `frontend/wailsjs/` 绑定（App.js / App.d.ts / models.ts 三处）|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|`model/` 下 `type X string` 具名 string 类型作 Wails 绑定方法参数/返回值时，wails generate 不为它在 models.ts 生成 `export type X = string` 别名，须手动补，否则 `npm run build` 报 MISSING_EXPORT（vitest 不报、易漏）|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|定制 `.trellis/workflow.md` 时只改正文描述；若增删 `[required · once]` 标记或步骤，必须同步修改对应 `[workflow-state:*]` 标签块，否则 trellis 回归测试失败|[workflow.md Customizing 章节](.trellis/workflow.md)|
|测试覆盖率分层门禁：后端 model/server ≥80% + service ≥76% 基线 + util ≥40%（排除 pty_windows.go）+ 主包不设门禁；前端 ≥70% 硬失败（exclude wailsjs）；改阈值须同步本文档 + docs/测试策略.md|[test-coverage-gate.md](docs/spec/test-coverage-gate.md)|
|新增 service 须在 `app_services.go` 的 `AppServices` struct 加字段 + `NewAppServices` 加构造行（2 处）；`AppServices` 必须在 package main（跨包内嵌未导出字段不提升）；App 内嵌 `*AppServices` 字段提升保 133 委托方法与 Wails 绑定零 diff；构造纯 new，生命周期副作用留 startup/shutdown|[app-services-assembly.md](docs/spec/app-services-assembly.md)|
|后端日志用 `log/slog`（禁 `println`，无级别无落盘 GUI 不可见）；service 包内调 `Logger()`（SetLogger 注入），App 层调 `slog.X`；需前端分流的错误用 `model.AppError{Code,Message}` 经 `main.go` ErrorFormatter 转 `{code,message}` 传前端，前端 `handleError` 按 code 分流；新增错误码同步 `model/app_error.go` 常量表 + `frontend/src/utils/error.js` ErrorCode/WARNING_CODES|[logging-and-errors.md](docs/spec/logging-and-errors.md)|
|E2E 用例从 `frontend/e2e/fixtures.js` import `{test,expect}`（自动注入 Wails mock，禁直接 import `@playwright/test`）；mock 返回值改 `src/test/wails-mock-defaults.js` 单一数据源（vitest/E2E 共用）；禁 `waitForTimeout`，靠 expect 自动重试；后端集成测试文件头 `//go:build integration`（默认 go test 不编译，CI 显式跑 `-tags=integration`）；`frontend/wailsjs/` 不入库，CI 须先 `wails generate module` 再 build|[e2e-testing.md](docs/spec/e2e-testing.md)|
|前端还原 `ReadFileBytes` 返回的 base64 为文本时用 `utils/base64.js` 的 `decodeBase64Utf8`，禁裸 `atob`（Latin-1 逐字节还原，中文双重编码乱码；ASCII 内容恰好正确故单测须用中文数据）|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|后端跨包共用测试辅助入 `util/testutil`（`RunGit`/`WriteFile`/`InitTempRepo`/`SetupMasterBranch`/`SetupFFRepo`/`SetupConflictRepo`），与前端 `wails-mock-defaults.js` 单一数据源模式对齐；包内专用辅助留 `*_test_helper.go` 不导出；禁各 `_test.go` 重复定义 git 命令执行/临时仓库构造辅助；集成测试 `it*` helper 语义更严（autocrlf/gpgsign）不并入 testutil；辅助函数参数用 `testing.TB`（`*testing.T`/`*testing.B` 共同接口）使 benchmark 可复用 fixture 构造|[perf-baseline.md](docs/spec/perf-baseline.md)|
|依赖安全扫描：Go 用 `govulncheck ./...`（调用链分析，非全依赖树），npm 用 `npm audit --registry=https://registry.npmjs.org --audit-level=high`（本地 npmmirror 不支持安全端点须绕过）；CI security job `continue-on-error` 不阻塞 PR；Go 标准库漏洞只能升 `go.mod` `toolchain` directive 修复（非 `go get`），依赖升级后须重跑 govulncheck 确认清零|[security-scan.md](docs/spec/security-scan.md)|
|崩溃恢复 UI 状态快照独立持久化 `data/session.json`（`SessionState`：selectedDirectoryId/activePanel/terminal）；`Terminal` 字段须用指针避 `omitempty` 空快照失效（nil→前端判冷启动）；类型名 `TerminalSnapshot` 避让 `model/terminal.go` 运行时会话；Load 损坏降级空快照不阻塞启动（复用 SettingsService 模式 + slog.Warn）；字段变更须同步 `frontend/wailsjs/` 绑定三处|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|AI 结构化输出走 `--json-schema`（claude CLI tool use 强制，与自由文本 `result` 解耦，自由文本带 markdown 包裹不影响）：`AiFunction.OutputSchema`（`json.RawMessage`，可选 nil 不加 flag 兼容现有 skill）→ `buildClaudeArgs` 追加 `--json-schema` → `parseStreamLine` 提取 result 事件 `structured_output` → `AiTaskRunResult/AiTaskState.StructuredOutput` 透传前端只渲染不解析；新增结构化 AI skill 零新增 service 代码（链路现成）；须配 `--verbose`（`--print`+`stream-json` 硬性要求，`buildClaudeArgs` 已加）|[ai-structured-output.md](docs/spec/ai-structured-output.md)|
|AI 功能项支持纯 prompt 模式（`Command` 空 + `Params.PromptTemplate` 驱动，`Cwd` 空继承父进程目录），`validateFunctions` 校验放宽为 Command 与 PromptTemplate 至少一非空；斜杠命令 skill（Command 非空）与纯 prompt skill（如 AI 提交信息生成/代码审查）两类并存；纯 prompt skill 经 `BuildStagePrompt` form 模板 `{{key}}` 占位注入 diff/历史等参数|[ai-structured-output.md](docs/spec/ai-structured-output.md)|
|新增内置 seed skill 须加入 `mergeMissingSeedSkills`（service/ai_function.go）白名单（现仅 `commit-message`/`code-review`），否则 PR1 前已建 `data/ai_functions.json` 老用户配置不含该 skill、`RunAiFunction` 报「功能不存在」；旧 seed skill 不进白名单尊重用户删除决策，用户自定义同 ID 项不覆盖|[ai-structured-output.md](docs/spec/ai-structured-output.md)|
|Wails 事件多组件共监听同事件（如 `ai-task:done` 被 AiFunctionPanel/LocalChanges/CommitHistory 共听）须用 `EventsOn` 返回闭包精准注销本组件监听器，禁 `EventsOff('eventName')` 全局移除（清全部同名监听器误删他组件）；组件 repoPath 切换/卸载须重置 AI 任务态 + `CancelAiTask` 在途任务防旧仓库结果串入 + loading 卡死|[cross-layer-contracts.md](docs/spec/cross-layer-contracts.md)|
|前端设计令牌：改 `--primary-color` 须同步 Element Plus `--el-color-primary` 派生（light-3~9 实色阶，暗色 light-9 用 rgba 透明度）否则 el-button/el-tag 割裂；冷暖灰统一蓝灰 Slate 色相禁混；暗色阴影禁纯黑须带背景色相 `rgba(2,6,23,…)`；`--terminal-bg` 须与 `useTerminal.js` LIGHT/DARK_TERMINAL_THEME.background 严格一致（改 `--bg-primary` 不动终端背景）；Geist 四字重 + font-display:swap + fallback 链|[design-tokens.md](docs/spec/design-tokens.md)|
|前端新增页面/组件视觉风格须守 frontend-visual-conventions（与 design-tokens 互补，本文指南层/契约层）：配色按语义选色（primary/success/warning/danger/info）+ 蓝灰三档中性色阶；VSCode 式三列布局（ActivityBar 48px + FileTreePanel + ContentPanel）+ 面板 nav+content 二分；nav active 左侧指示条 `left:0` 防 overflow 裁剪（禁负 left）+ 卡片 hover `--shadow-sm`→`--shadow-md` 层级 + section-title 字重梯度（h1-h3 用 600 + `letter-spacing:-0.01em`、h4-h6 用 500）+ kbd `'Geist','Consolas','Monaco',monospace` 字体族 + 行内 style 禁用全迁语义类；四档间距/圆角内紧外松就近归并（容器 `--radius-lg` 较软、内元素 `--radius-md` 较紧）；弹窗 `min(像素,视口比例)` 响应式尺寸分档（设置 960×620、命令面板 720×480、内容多大弹窗 900+、小弹窗 420-600）+ `top` 用 vh 响应式；Home 一级面板根元素必须带完整 flex 约束链（`flex:1`+`min-height:0`+`min-width:0`+`display:flex`+`flex-direction:column`+`overflow:hidden`，缺失则面板塌为内容高度且 flex 收缩分摊压缩 TerminalPanel inline height 致终端显示异常）；EP 非 autosize textarea 挂载时写内联 `min-height:31px` 压过类规则，自定义拖拽下限须 `!important` 覆盖；highlight.js 按需注册须含 plaintext 回退名（highlight 回调对未识别语言固定回退该名，漏注册 console 稳定报错）|[frontend-visual-conventions.md](docs/spec/frontend-visual-conventions.md)|
|service 层发前端事件必须走 EventSink（`emitEvent(s.eventSink(), ...)`），禁直调 `runtime.EventsEmit`（非 Wails ctx 触发 log.Fatalf 杀整个进程，serve 浏览器通道也收不到）；serve 模式禁调窗口对话框类方法（`wailsRuntimeUnavailable` 守卫返回错误）；/api/rpc、WS 帧、token 三通道与热轮换、`E_RPC_*` 错误码三处同步规则见详细文档|[browser-channel.md](docs/spec/browser-channel.md)|
|终端多 tab：xterm 实例禁重挂载/搬 DOM（v-show 保 DOM + 激活 refit，隐藏容器 fit 静默失败）；tab 列表 watch 须 getter 投影（数组 splice 不触发多源引用比较，仅「关闭非活动 tab」路径能暴露）；快照 v2 归一化消费旧 `workDir` 后必须置空防复写；全屏层 z-index 1500（<右键菜单 2000 ≤ EP 弹窗 2001+）且守 `fullscreen ⇒ visible` 不变式，ESC 处理须过滤 `defaultPrevented` 与 `.el-overlay/.xterm-helper-textarea`，全屏动画仅 opacity（尺寸 transition 引发 fit 风暴）；外观字号/字体热更后须 refit+ResizeTerminal、scrollback 仅新建生效、`el-input-number` 必带 `:value-on-clear` 防清空写 null 脏值|[terminal-multi-session.md](docs/spec/terminal-multi-session.md)|
|新增带事件的 service 须同步 `web_serve.go` `applySink` 补 `SetEventSink` 行（现五服务：terminal/update/aiFunc/git/chat），漏加则浏览器通道静默收不到该服务全部事件（桌面端正常、极易漏测）；AI 对话 ChatService 契约：chat-task:* 事件流与 ai-task:* 隔离、data/ai_chat/ 存储布局（索引与消息分离+损坏降级）、--resume 链路 session_id 回写、同会话串行保护（E_CHAT_IN_PROGRESS）、processFactory 注入点（多 CLI 扩展位）、assistant 回复在 done 事件前落盘、选择题走 chat-question 块契约（headless AskUserQuestion 被 CLI 自动应答须 `--disallowedTools` 禁用 + 首轮声明注入，声明不进落盘消息，前端 fence 渲染分流）|[ai-chat-service.md](docs/spec/ai-chat-service.md)|
|Linux 支持：Wails 桌面构建须 `-tags webkit2_41`（Ubuntu 24.04 已移除 webkit2gtk-4.0 包）；无标签时桌面前端 CGO 不进依赖图（internal/app 桩实现），后端子包 build/test 无需 GTK 依赖；平台抽象选型——编译期字段独有走 `_windows/_other` 双文件、短行为分叉走运行时 GOOS 分支、纯逻辑抽无 tag 文件双侧同测；自更新 Linux 资产名 `workbench-linux-amd64.tar.gz` 与 release.yml 打包名严格一致；`build/` 在 gitignore，需分发文件须白名单放行（`!build/README-linux.md` 先例，漏放发版必挂）；测试平台断言拆 `_windows_test.go`/`_linux_test.go`，禁两平台共享一份平台耦合断言；前端 shell/目录兜底禁 'powershell'、'C:\\' 字面量，统一走 settings store `FALLBACK_SHELL` 与 `fallbackTerminalDir()`|[linux-platform.md](docs/spec/linux-platform.md)|
|测试禁隐式依赖环境外部状态：文件 mtime 时序（NTFS 同 tick 不变）与固定端口（本机常驻 workbench.exe 监听 36115）均致偶发挂；web serve 启动测试注入随机端口（settings BindAddress 写 :0 或 webServeListenOverride，直调 Start 直接传参），能直绑 :0 的用例禁新增非必要 freePort 探测（TOCTOU 定性接受仅限结构性须预知端口的用例），mtime 判定用注入陈旧缓存驱动，禁 sleep 等待|[test-stability.md](docs/spec/test-stability.md)|

## 文档索引

|文档|说明|
|---|---|
|[快速入门.md](docs/快速入门.md)|安装、首次配置、核心流程上手指南|
|[功能说明.md](docs/功能说明.md)|工作目录管理、文件树、文件操作、Git集成、导航中心、终端、快捷键、浏览器访问|
|[开发工作流.md](docs/开发工作流.md)|启动开发、运行测试、构建、运行应用|
|[测试策略.md](docs/测试策略.md)|单元测试、集成测试、E2E 测试、覆盖率门禁|
|[部署说明.md](docs/部署说明.md)|生产构建、分发、浏览器访问模式部署、配置文件|
|[开发规范.md](docs/开发规范.md)|代码风格、调试、错误处理、提交规范|
|[架构设计.md](docs/架构设计.md)|分层架构、Wails 桥接、浏览器访问通道、跨层契约、错误处理链路|
|[API参考.md](docs/API参考.md)|Wails 绑定方法清单（153 方法按域分组）与数据模型|
|[贡献指南.md](docs/贡献指南.md)|环境准备、开发工作流、测试规范、PR 流程、spec 沉淀|
|[常见问题.md](docs/常见问题.md)|常见问题|
|[路线图.md](docs/路线图.md)|发展路线图|
|[项目上下文.md](docs/project-context.md)|AI Agent 编码规则和模式|
|[开发运维.md](workbench/DEVELOPMENT.md)|开发运维详细文档|
|[构建摘要.md](workbench/BUILD_SUMMARY.md)|构建摘要|
|[规范沉淀](docs/spec/README.md)|跨层契约等项目规范沉淀（原 .trellis/spec 迁移）|

## 常用命令

|操作|命令|
|---|---|
|开发调试|`wails dev`|
|构建应用|`wails build`|
|后端测试|`go test ./...`|
|后端集成测试|`go test -tags=integration ./...`|
|前端测试|`cd frontend && npm test`|
|E2E 测试（首次）|`cd frontend && npm run e2e:install`|
|E2E 测试|`cd frontend && npm run e2e`|
|安装依赖|`cd frontend && npm install`|
|查看端口|`netstat -ano \| findstr ":34115"`|
|停止进程|`taskkill /F /IM workbench.exe`|

---

**最后更新：** 2026-09-23
**文档版本：** v2.10
