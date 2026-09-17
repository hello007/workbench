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

## 文档索引

|文档|说明|
|---|---|
|[快速入门.md](docs/快速入门.md)|安装、首次配置、核心流程上手指南|
|[功能说明.md](docs/功能说明.md)|工作目录管理、文件树、文件操作、Git集成、导航中心、终端、快捷键|
|[开发工作流.md](docs/开发工作流.md)|启动开发、运行测试、构建、运行应用|
|[测试策略.md](docs/测试策略.md)|单元测试、集成测试、E2E 测试、覆盖率门禁|
|[部署说明.md](docs/部署说明.md)|生产构建、分发、配置文件|
|[开发规范.md](docs/开发规范.md)|代码风格、调试、错误处理、提交规范|
|[架构设计.md](docs/架构设计.md)|分层架构、Wails 桥接、跨层契约、错误处理链路|
|[API参考.md](docs/API参考.md)|Wails 绑定方法清单（139 方法按域分组）与数据模型|
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

**最后更新：** 2026-09-16
**文档版本：** v2.7
