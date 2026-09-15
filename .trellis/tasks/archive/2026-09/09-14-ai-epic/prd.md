# AI 开发辅助 epic - 提交信息生成与代码审查

## Goal

在 WorkBench 提交流程中引入 AI 辅助：基于暂存区 diff 自动生成 Conventional Commits 规范的提交信息候选；基于未提交变更或指定 commit diff 进行结构化代码审查。两功能共享 v1.3 AI 执行链路，合一个 epic 分 3 PR 推进，复用基础设施省重复造轮子。作为 v1.3 AI 主线延续、v1.5 推进项。

## What I already know（复用点已 codegraph 验证）

### AI 执行链路（service/ai_function.go）
- `RunStage(functionID, prompt, resumeSessionID)` (ai_function.go:338) — 起 claude 子进程，经 Wails 事件 `ai-task:output`/`ai-task:done` 推送前端，含排队/并发槽位/超时/取消
- `LoadAiFunctions` (ai_function.go:132) — schema v2 加载，v1→v2 自动迁移 + 损坏回种 seed
- `BuildStagePrompt(command, spec, params)` (ai_function.go:895) — 支持 `none`/`file`/`text`/`form` 四类参数；**form 类型经 `PromptTemplate` `{{key}}` 占位符渲染**，天然支持 diff 文本注入，无需新增 prompt 组装 helper
- `SaveAiFunctions(funcs)` (ai_function.go:181) → `saveConfig` 落 schema v2 `data/ai_functions.json`
- `AiFunction` 结构完整：`Command`/`Cwd`/`AddDirs`/`Env`/`Mcp`/`PermissionMode`/`TimeoutMinutes`/`Params`/`FollowUps`/`Tags`/`Pinned`
- `AiParamSpec` form 类型：`PromptTemplate` + `Fields[]AiFormField`（Key/Label/Type/Required）
- `AiTaskRunResult.Output` 仅末尾预览 ~4KB，全量输出落盘 `data/ai_task_output/<id>.txt`，前端经 `GetAiTaskOutput` 全量读
- `AiConcurrencyStatus{Running,Queued,Max}` — 标题栏 N/M 并发占用

### diff/commit 数据源（app_git.go / service/git.go）
- `GetLocalChanges(path)` (app_git.go:574) → `[]model.FileChange`（Staged 字段驱动单表分组，待实现期确认字段名）
- `GetDiff(repoPath, file)` (git.go:669) — **语义修正（research 2 核证）**：走 `git diff HEAD -- <file>` 对比 HEAD 与工作区，**非暂存区 diff**；未跟踪 `git diff --no-index /dev/null <file>` 展示新增全文。代码审查（审未提交变更）可直接复用；**提交信息生成须暂存区 diff，不能复用 GetDiff**，须新增 staged-diff 变体（`git diff --cached`）——PR1 链路正确性前提
- `GetRangeDiff(path, baseSHA, headSHA)` (app_git.go:630) — commit 区间全文件 unified diff
- `CommitFiles(path, message, filePaths)` (app_git.go:590) → `gitSvc.Commit`（pathspec 选择性提交，先 add 再 commit --）

### 跨层契约（须遵守 spec）
- 新增 App 方法签名变更须同步 `frontend/wailsjs/{App.js,App.d.ts}` + `models.ts` 三处（cross-layer-contracts.md）
- 新增 service 须 `AppServices` struct 加字段 + `NewAppServices` 加构造行 2 处（app-services-assembly.md）
- 错误用 `AppError{Code,Message}` 跨层分流，新增错误码同步 `model/app_error.go` + `frontend/src/utils/error.js`（logging-and-errors.md）
- 覆盖率门禁零回归：service ≥76% model/server ≥80% 前端 ≥70%（test-coverage-gate.md）

## Assumptions（待 research / 实现期验证）

- claude CLI 非 interactive 模式可输出结构化 JSON（`--output-format json` 或 prompt 约束）— research 主题 3 核证
- diff 截断阈值可基于 claude 上下文窗口（200K tokens）估算字节数上限
- `model.FileChange` 有 `Staged` bool 字段 — 实现期 codegraph_node 确认
- `GetCommitFileDiff`（单 commit 单文件 diff）存在 — 实现期确认；不存在则用 `GetRangeDiff` 或 `git show <sha> -- <file>` 补
- `SkillDiscoveryService` 存在且可复用 — 实现期确认

## Open Questions（次要，research 已给默认值，待用户确认或实现期调）

- ~~claude CLI 结构化输出方式~~ → 已决方案 C（见 ADR-lite）
- diff 截断阈值：采纳 research 1 建议值 `MaxDiffBytes=200K`/`MaxDiffFiles=20`/`MaxDiffLines=50K`，截断后提示「已截断，剩余 N 文件 M 行」格式；实现期可调
- 代码审查 JSON schema：采纳 research 1 草案（`severity` enum critical|warning|info / `category` enum bug|style|security|performance|improvement / `confidence` 0-1 / `diffRef` 行号引用 / `description` / `suggestion`），类别×级别矩阵约束（style/improvement 不得标 critical）
- few-shot 策略：取最近 3 条非噪声 subject（过滤 `chore: record journal`/`chore(task): archive`），service helper 跑 `git log --format=%s` 取，无需新 Wails 绑定
- 候选提交信息数量：2-3 个（用户描述已定）

## Requirements（evolving）

### PR1：AI 基础接入扩展
- **staged-diff 变体（research 2 核证，链路正确性前提）**：`GetDiff` 走 `git diff HEAD` 非 staged，提交信息生成须新增 staged-diff helper（`git diff --cached`）；代码审查审未提交变更可复用 `GetDiff`
- **diff 聚合 + 截断 util**：`GetLocalChanges` 筛 Staged + 逐文件 diff 聚合单段文本；截断阈值（research 1 建议 `MaxDiffBytes=200K`/`MaxDiffFiles=20`/`MaxDiffLines=50K`），超长截断 + 提示剩余行数/文件数；独立 util helper 不污染 AI service，留 service 层不经 App 暴露规避 wailsjs 同步
- **结构化输出链路 7 处改动（方案 C）**：`model/ai_function.go` 的 `AiFunction` 加 `OutputSchema json.RawMessage` 字段（可选）+ `AiTaskRunResult`/`AiTaskState` 加 `StructuredOutput` 字段；`service/ai_function.go` 的 `streamEvent` 加 `StructuredOutput` 字段、`parseStreamLine` 提取、`buildClaudeArgs` 追加 `--json-schema`（OutputSchema 非 nil 时）+ 确认 `--verbose` 已加；同步 `frontend/wailsjs/{App.js,App.d.ts,models.ts}` 三处
- **skill 模板**：新增 commit-message / code-review 两个内置 skill 配置项，Command 指向 claude，Params 用 form 类型 + PromptTemplate 含 `{{repoPath}}`/`{{file}}`/`{{diff}}`/`{{history}}` 等占位符，`OutputSchema` 配对应 JSON schema，复用 `SaveAiFunctions` 落 `data/ai_functions.json`
- **规范注入**：代码审查预设 `data/code_review_rules.md` 单一数据源（research 1 建议，勿整份 CLAUDE.md 灌入挤占 diff 上下文）；提交信息生成注入 Conventional Commits 规范 + 历史 few-shot
- **结果展示**：复用 AI 历史面板，不新建 UI 容器；结构化输出完成后渲染结构化面板（候选列表 / 问题清单），流式过程展示原始文本进度

### PR2：AI 提交信息生成（轻量先行）
- 入口：提交面板提交信息输入框旁「AI 生成」按钮
- 输入：当前暂存区 diff（未暂存不纳入，避免误判；无暂存文件时禁用按钮 + 提示）
- 输出：2-3 个候选提交信息（Conventional Commits 规范：type + 可选 scope + 描述），结构化返回前端供选择
- 交互：候选列表点击填入提交框，可编辑后提交（走现有 `CommitFiles`）
- 规范注入：prompt 注入提交规范（feat/fix/docs/refactor/perf/test/chore）+ 历史 commit 风格 few-shot（取最近 10 条 commit message）

### PR3：AI 代码审查（重，多文件结构化）
- 入口：提交面板「AI 审查」按钮（审未提交变更）+ 提交历史详情「审查此 commit」
- 输入：未提交变更全量 diff 或指定 commit diff（`GetRangeDiff`/`GetCommitFileDiff`）
- 输出：结构化问题清单（文件/行号/级别 critical|warning|info/类别 bug|规范|安全|改进/描述/建议），JSON 格式便于前端渲染
- 审查维度：bug 风险、编码规范、安全漏洞、性能问题、改进建议（分级）
- 展示：问题清单面板（按级别分组 + 文件定位 + 一键跳转 diff 行），复用 AI 历史输出落盘 + `GetAiTaskOutput`

## Acceptance Criteria（evolving）

- [ ] diff 聚合 helper 单测覆盖：暂存筛选、截断阈值、空 diff、超大 diff
- [ ] commit-message / code-review skill 配置项默认值落 `data/ai_functions.json`，schema v2
- [ ] 无暂存文件时「AI 生成」按钮禁用 + 提示
- [ ] 候选提交信息点击填入提交框，可编辑后 `CommitFiles` 提交成功
- [ ] 代码审查问题清单按级别分组渲染，文件行号可跳转 diff
- [ ] 覆盖率门禁零回归（service ≥76% model/server ≥80% 前端 ≥70%）
- [ ] 跨层契约三处同步零 diff

## Definition of Done

- 后端单测：diff 组装 helper（截断/暂存筛选）、prompt 组装、skill 配置项默认值
- 前端单测：候选选择交互、问题清单渲染、按钮态（无暂存禁用）
- E2E：提交信息生成填入 + 提交流程、审查结果展示（mock claude 输出）
- 覆盖率门禁零回归
- 跨层契约三处同步
- README.md / 路线图「AI 能力」章节同步

## Out of Scope（MVP 不做，但预留扩展点）

- 自动修复（只审查/建议，不改代码）— schema 预留 `suggestion` 字段为未来「一键应用修复」铺路
- 跨仓库批量审查
- 自定义审查规则配置（内置维度先够用）— `data/code_review_rules.md` 单一数据源已为未来用户自定义铺路
- 重复造 AI 并发控制 / 输出截断 / 历史归档（复用现有）
- 提交信息生成 FollowUp「生成 body 详细说明」（复用现有 `AiFollowUp` 机制，MVP 不做）

## Edge Cases（须覆盖）

- 大 diff 超上下文窗口 → 截断保护（PR1 util，阈值见 Open Questions）
- `structured_output` 解析失败（OutputSchema 配错）→ 降级展示原始文本，不阻塞
- 无暂存文件 → 「AI 生成」按钮禁用 + 提示（PR2）
- diff 为空（新建文件无改动）→ 提示无变更
- claude 子进程超时 → 复用现有 RunStage 超时机制
- `--verbose` 未加 → stream-json 启动失败（PR1 须确认 `buildClaudeArgs`，未加须补）
- 候选提交信息全部不满意 → 用户手编辑提交框（现有交互保留）
- 代码审查无问题 → schema 约束返回空数组 + 前端展示「未发现问题」（research 1 grounding 规则）
- glm-5 provider tool_use 可用（research 3 实测），换 provider 须冒烟

## Research References

- [`research/ai-code-review-prompt.md`](research/ai-code-review-prompt.md) — 推荐方案 A（stream-json 零改动 + `extractReviewJson` 类比 `extractTable` + 截断 util + `data/code_review_rules.md` 规范注入）；含 JSON schema 草案 + 五维分级矩阵 + grounding 规则
- [`research/conventional-commits-generation.md`](research/conventional-commits-generation.md) — 推荐方案 B（多候选 + 动态 few-shot + JSON）；核证 GetDiff 非 staged 语义；few-shot 须过滤 `chore: record journal`/`chore(task): archive` 归档噪声
- [`research/claude-cli-structured-json.md`](research/claude-cli-structured-json.md) — 推荐方案 C（`--json-schema` 强制结构化，tool use 机制，实测可靠）；`--output-format json` 单独不够；流式与结构化不冲突；解析推荐后端透传。**主 agent 实测补充（2026-09-15）**：`stream-json`+`--verbose`+`--json-schema` 组合通过，result 事件含 `structured_output` 字段严格符合 schema，与自由文本解耦（自由文本带 markdown 包裹不影响结构化输出）；stop_reason=`end_turn` 非 tool_use，structured_output 不依赖特定 stop_reason；**须配 `--verbose`**（`--print`+`stream-json` 硬性要求），实现期确认 `buildClaudeArgs` 是否已加未加须补

## Decision (ADR-lite) — 结构化输出方案

**Context**：AI 提交信息生成（2-3 候选数组）/代码审查（问题清单 JSON）需结构化输出。方案 A（prompt 约束 + 后端容错解析）0 跨层改动但非 100% 可靠；方案 C（`--json-schema` 强制结构化）7 处改动但可靠。

**Decision**：采纳方案 C（`--json-schema`）。research 3 实测 + 主 agent 补测（`stream-json`+`--verbose`+`--json-schema` 组合通过，structured_output 与自由文本解耦，自由文本带 markdown 包裹不影响结构化输出）双重背书。`OutputSchema` 字段可选（nil 不加 `--json-schema` flag），兼容现有 skill 零回归。

**Consequences**：
- PR1 一次性投入 7 处改动（见下 PR1 改动清单），PR2/PR3 直接复用
- 触发 wailsjs 三处同步（`AiFunction.OutputSchema` + `AiTaskRunResult.StructuredOutput` 新字段）
- 须确认 `buildClaudeArgs` 是否已加 `--verbose`（`--print`+`stream-json` 硬性要求），未加须补
- 解析在后端 `parseStreamLine` 提取 `structured_output` 经 `AiTaskRunResult.StructuredOutput` 透传前端，前端只渲染不解析
- 流式 UX 保留：`stream-json` 的 `assistant` 文本增量照推前端展示进度，`structured_output` 只在最终 `result` 事件出现，完成后渲染结构化面板

## Technical Notes

- 验证工具：codegraph_context + codegraph_explore（已确认复用点真伪）
- 关键约束：diff 注入须截断保护（大 diff 超 claude 上下文窗口）；skill 配置项走 schema v2 复用导入导出 + 迁移机制
- PR 序列依赖：PR1 基础设施（diff helper + skill 模板）→ PR2 轻量先行验证链路 → PR3 重结构化输出
