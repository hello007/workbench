# AI 功能自然语言自动生成配置

## Goal

在侧边栏「AI 功能」的配置管理中，支持用户用自然语言描述需求（如「把会议纪要转成周报」），并结合可选的仓库路径、指定 skill 等上下文，由 AI 自动生成一个可直接运行的 AI 功能配置草稿，免去逐项手工录入。草稿回填到现有配置表单供预览微调，用户确认保存后即可使用。

## Requirements

### 入口与交互

- 配置管理弹窗（`AiFunctionConfigDialog.vue`）左侧列表新增「✨ 自然语言创建」按钮，点击打开独立生成弹窗。
- 生成弹窗包含：
  - 需求描述 textarea（必填）；
  - 工作目录选择器（选填，复用现有目录选择能力）；
  - 指定 skill 下拉（选填，候选 = 全局已发现 skill ∪ 所选目录下 skill；选目录后实时刷新）；
  - 「生成」按钮与可取消的 loading 状态。
- 生成完成后在弹窗内展示草稿预览（字段只读 + 警告列表）；用户点「填入配置」后，草稿作为**新增项**追加到左侧功能列表并选中，右侧现有表单可继续编辑；点「取消/关闭」不产生任何落盘副作用。
- 关闭生成弹窗或点取消时中止进行中的生成调用。

### 生成规则

- 优先包装用户指定或匹配到的已有 skill：填 `Command`（含命名空间）与 `Cwd`；
- 未命中合适 skill 时生成纯 prompt 功能：`Command` 为空，`Params.Type=form` 且 `Params.PromptTemplate` 非空；
- 常规字段由 AI 生成：`ID`、`Name`、`Description`、`Icon`、`Tags`、`Params`、`Completion`、`TimeoutMinutes`、`PermissionMode`；
- 高级字段（`Env`/`Mcp`/`AddDirs`/`FollowUps`/`OutputSchema`/`Pinned`）仅当需求描述明确提及时生成，否则留空；
- 用户指定的 skill 即使 AI 判断不匹配，仍以用户选择为准生成，并在警告列表中说明原因；
- ID 生成时避开已有 ID；若仍冲突，前端回填前自动加 `-2`/`-3` 后缀并在预览中提示。

### 可用性校验（静态，不试跑）

- `ID` 非空且唯一（避让后仍冲突则阻断）；
- `Name` 非空；
- `Command` 与 `Params.PromptTemplate` 至少一个非空；
- 若 `Command` 非空，需命中 skill 候选清单，未命中写入警告（不阻断）；
- 生成结果须能通过 `validateFunctions` 校验。

### 后端接口

- 新增 `App.GenerateAiFunctionDraft(req model.AiFunctionDraftRequest) (*model.AiFunctionDraftResult, error)`；
- 新增 `App.CancelAiFunctionDraft() bool`（中止当前生成）；
- 新增 `App.GetDiscoveredSkillsIn(dir string) []*model.SkillDescriptor`（扫描指定目录 `.claude/skills`，复用 `scanSkillsDir`）；
- 三个方法均为纯桌面能力，须加入 `RPCHiddenMethods`。

## Acceptance Criteria

- [ ] 配置弹窗可打开「自然语言创建」弹窗并完成一次生成（前端 mock 可验证）
- [ ] 输入一句自然语言（无上下文）可得到纯 prompt 功能草稿，且通过 `validateFunctions`
- [ ] 选定已有 skill 时草稿的 `Command`/`Cwd` 与所选 skill 一致
- [ ] 生成结果 ID 与已有项冲突时自动加后缀并提示
- [ ] 关闭生成弹窗会取消进行中的后端生成
- [ ] 取消/关闭弹窗不修改 `data/ai_functions.json`
- [ ] 所选目录下的 skill 出现在候选下拉中

## Technical Approach

### 生成执行链路（方案 A：独立生成链路）

- `AiFunctionService` 新增 `GenerateAiFunctionDraft(...)`：内部构造临时 `model.AiFunction`（固定 `OutputSchema`、`PermissionMode=bypassPermissions`、超时 2 分钟），复用 `claudeCommandResolver` + `buildClaudeArgs` + `parseStreamLine`，同步等待进程结束并返回 `structured_output`；
- 复用全局 `concurrencySem` 控制并发；生成任务**不发 `ai-task:*` 事件、不进任务 map、不进历史**；
- 取消：service 内以 mutex 保存当前生成的 `context.CancelFunc`，`CancelAiFunctionDraft` 触发取消；超时返回明确错误码/文案；
- 生成 prompt 与输出 JSON Schema 作为代码内常量（`aiFunctionDraftPrompt` / `aiFunctionDraftSchema`），不允许用户修改。

### 后端装配

- `app_ai.go` 负责组装请求：补全候选 skill 清单（`skillDiscoverySvc.GetCached()` + `GetDiscoveredSkillsIn(cwd)` 去重）、已有 ID 清单，再调 service；
- 新增模型放在 `model/ai_function.go`（`AiFunctionDraftRequest`/`AiFunctionDraftResult`）或独立 `model/ai_function_draft.go`；
- 新增方法后同步 `frontend/wailsjs/{App.js,App.d.ts,models.ts}` 三处（`cross-layer-contracts.md` 规则）。

### 前端

- 新增组件 `frontend/src/components/AiFunctionCreateDialog.vue`；
- `AiFunctionConfigDialog.vue` 引入组件、增加左侧入口按钮、处理 `created` 事件（追加 + 选中）；
- 生成中禁止重复提交；错误经 `handleError` 分流；`ElMessageBox` 等交互遵循 `confirm-dialog.md`；
- 视觉遵循 `frontend-visual-conventions.md`（完整 flex 约束链、图标走 `components/icons/` 或 EP 图标名）。

## Decision (ADR-lite)

**Context**：配置项字段多达 15 个，手工录入成本高；已有 skill 发现与结构化输出两条链路可复用。

**Decision**：
- 生成产物优先包装已有 skill，未命中生成纯 prompt 功能（选项 B）；
- 独立生成弹窗入口（D2）；
- 自然语言 + 结构化上下文控件（D3）；
- 静态校验不试跑（D5）；
- 单条生成（D6）；
- 独立生成链路、prompt/schema 内置（D8）。

**Consequences**：生成过程无流式输出、不可观察中间态；高级字段生成保守，用户可能仍需手工补充；后续如需批量生成或自动建 SKILL.md，可在此基础上扩展。

## Out of Scope

- 自动创建/修改 `SKILL.md` 文件；
- 生成后自动试跑或提供试运行按钮；
- 一次生成多个功能；
- 生成任务进入 AI 任务面板或历史记录；
- 用户自定义生成 prompt / 输出 schema。

## Technical Notes

- 关键文件：
  - `model/ai_function.go`：`AiFunction`、`AiParamSpec`、`AiFollowUp`
  - `service/ai_function.go`：`RunStage`、`buildClaudeArgs`、`parseStreamLine`、`validateFunctions`、`concurrencySem`
  - `service/skill_discovery.go`：`scanSkillsDir`、`GetCached`、`Refresh`
  - `app_ai.go`：`GetAiFunctions`、`SaveAiFunctions`、`GetDiscoveredSkills`
  - `frontend/src/components/AiFunctionConfigDialog.vue`、`AiFunctionPanel.vue`
  - `frontend/src/test/wails-mock-defaults.js`（测试单一数据源）
- 相关规范：
  - `docs/spec/ai-structured-output.md` — `--json-schema` 与 `structured_output` 链路
  - `docs/spec/cross-layer-contracts.md` — wailsjs 三处同步、`AppError`
  - `docs/spec/logging-and-errors.md` — 错误码三处同步
  - `docs/spec/browser-channel.md` — `RPCHiddenMethods`
  - `docs/spec/test-coverage-gate.md`、`docs/spec/e2e-testing.md`
- 待实现时确认：现有目录选择绑定方法名（`OpenDirectoryDialog`/等价）与 `scanSkillsDir` 是否可直接导出复用。
