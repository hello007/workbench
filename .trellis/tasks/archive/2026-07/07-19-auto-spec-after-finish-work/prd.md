# brainstorm: finish-work 后自动沉淀知识到 CLAUDE.md（非 trellis 目录）

## Goal

在收尾阶段自动触发知识沉淀（复用 `trellis-update-spec` 能力），把本次任务中值得长期保留的开发经验/约定/坑沉淀到 **非 trellis 目录**：坑/规范进 `docs/常见问题.md` + `docs/开发规范.md`，少量全局必读约定进 `CLAUDE.md`，避免遗忘，让后续每个 session 受益。

> 说明：经澄清，"自动"指流程强制执行（非手动想起），不要求时机必须在 finish-work 命令之后。最终方案保留现有 Phase 3.3 时机（finish-work 之前），只改沉淀目标位置。

## What I already know

### finish-work 命令现状
- 路径 `.claude/commands/trellis/finish-work.md`，slash command，AI 按 4 步执行：survey -> 脏文件分类（拒绝脏树）-> `task.py archive` 归档 -> `add_session.py` 记录 journal。
- finish-work 本身不沉淀知识、不做代码 commit（commit 在 Phase 3.4）。
- 执行主体是 AI；"自动执行"= 在流程里强制 AI 做这一步。

### trellis-update-spec skill 现状
- 路径 `.claude/skills/trellis-update-spec/SKILL.md`，默认沉淀目标 `.trellis/spec/<layer>/*.md` 或 `.trellis/spec/guides/*.md`。
- 内置丰富的沉淀模板（Design Decision / Common Mistake / Forbidden Pattern / Gotcha 等），可映射到 docs/ 的两个文件。
- **本项目不改 SKILL.md**：Phase 3.3 walkthrough body 的指令措辞足够强，会覆盖 skill 默认；且 SKILL.md 是模板管理文件，改它有 trellis update 覆盖风险。

### 现有 workflow 中 update-spec 的时机
- `workflow.md` Phase 3.3 `[required · once]`：finish-work **之前**加载 trellis-update-spec，沉淀到 `.trellis/spec/`。
- breadcrumb `[workflow-state:in_progress]` 已强制 AI 走 `trellis-implement -> trellis-check -> trellis-update-spec -> commit -> /trellis:finish-work`。**"自动触发"已由现有机制保证**，只需改"沉淀到哪里"和"怎么确认"。

### 项目既有的沉淀习惯与目标格式
- git log `13b8a68` 把开发经验沉淀到 `docs/常见问题.md` + `docs/开发规范.md`（非 trellis）。
- `docs/常见问题.md`：Q&A 格式，每条含 **症状 / 原因 / 解决方案 / 调试步骤 / 相关文件**，分"开发问题/使用问题/性能问题/调试技巧"。
- `docs/开发规范.md`：分"代码风格/调试日志/错误处理/Git 提交/测试/性能/安全"，每条含 **✅正确 / ❌错误 代码示例 + 原因**。

### 模板管理现状（关键约束）
- `.trellis/.template-hashes.json` 跟踪 `workflow.md`、`finish-work.md`、`trellis-update-spec/SKILL.md` 等文件。
- 但 `workflow.md` 的 "Customizing Trellis" 明确允许编辑 walkthrough body（官方定制点），编辑后重启 session 即生效，非 template maintainer 无需 `trellis update`。
- 改 walkthrough body、不动 `[required·once]` 标记 -> 不触发 breadcrumb 契约同步（见 workflow.md "Editing checklist"）。

## Decision (ADR-lite)

**Context**: 现有 Phase 3.3 在 finish-work 前沉淀到 .trellis/spec/；用户想要收尾时自动沉淀到非 trellis 目录（CLAUDE.md 或 docs/）。

**Decision**:
1. **时机**：保留 Phase 3.3（finish-work 前），不挪到 finish-work 之后--任务上下文最完整、不动 breadcrumb 契约、改动最小。
2. **位置**：两者结合--坑/问题进 `docs/常见问题.md`，规范/约定进 `docs/开发规范.md`，少量"全局必读约定"进 `CLAUDE.md`。
3. **确认**：`docs/` 直接写（低风险、既有习惯）；`CLAUDE.md` 起草后用户确认再写（核心文件保护）。
4. **实现**：**只改 `workflow.md` Phase 3.3 walkthrough body**（官方定制点）。**不改 `trellis-update-spec/SKILL.md`**--Phase 3.3 指令措辞足够强（"本项目覆盖，非 .trellis/spec/"）会覆盖 skill 默认，且 SKILL.md 是模板管理文件、改它有 trellis update 覆盖风险。

**Consequences**:
- 避免双沉淀与内容重叠；只保留一次沉淀。
- 时机在 finish-work 前：用户执行 finish-work 时沉淀已完成，符合"收尾时自动沉淀"心智，但严格说不是"finish-work 命令之后"。
- 只改 workflow.md 一个文件（官方定制点，低风险）；不改 SKILL.md 以规避模板覆盖风险。
- 不改任何脚本、不动 hook、不动 breadcrumb 契约。
- 取舍：单独调用 `/trellis:update-spec`（非 Phase 3.3 流程）仍走 skill 默认 .trellis/spec/，不在本诉求范围内。

## Requirements

- R1 执行 finish-work 收尾流程时，自动进行一次知识沉淀（由 Phase 3.3 现有 `[required·once]` 机制保证触发）。
- R2 沉淀目标为非 trellis 目录：坑/问题 -> `docs/常见问题.md`；规范/约定 -> `docs/开发规范.md`；全局必读约定 -> `CLAUDE.md`。
- R3 沉淀内容贴合两个 docs 文件的既有格式（Q&A / ✅❌ 示例）。
- R4 `docs/` 写入：AI 直接写；`CLAUDE.md` 写入：AI 起草拟改片段，用户确认后再写。
- R5 不破坏现有 finish-work 的归档 + journal 流程，不动 workflow breadcrumb 契约。
- R6 即使结论是"本次无内容可沉淀"，也要走一遍判断并说明（沿用现有 Phase 3.3 约束）。

## Acceptance Criteria

- [x] AC1 `workflow.md` Phase 3.3 walkthrough body 明确指向 docs/+CLAUDE.md，并写明确认策略；`[required·once]` 标记不变。
- [x] AC2 ~~改 SKILL.md~~（已否决）：不改 SKILL.md，仅靠 workflow.md Phase 3.3 指令覆盖 skill 默认。
- [x] AC3 沉淀引导给出 docs/常见问题.md 与 docs/开发规范.md 的格式映射（坑->Q&A，规范->✅❌示例）。
- [x] AC4 明确 CLAUDE.md 的沉淀边界（仅"全局必读约定"）与确认流程。
- [x] AC5 不改 `.trellis/scripts/`、不改 hook 注册、不改 `[workflow-state:*]` tag block、不改 SKILL.md。
- [ ] AC6 走查一遍：模拟一个任务收尾，验证 AI 按 Phase 3.3 沉淀到 docs/ 而非 .trellis/spec/。

## Definition of Done

- workflow.md 改动完成，措辞与既有文档风格一致（中文、正式、不口语化）。
- 不破坏 trellis 工作流一致性（breadcrumb 契约未动）。
- 按项目 CLAUDE.md 要求确认是否更新 README.md。
- Rollout/rollback：改动均为本地 .trellis 文件，回滚即 `git revert`。

## Technical Approach

### 核心思路
"自动触发"已由 Phase 3.3 的 `[required·once]` + breadcrumb 保证，无需新增 hook 或改命令。本任务只改"沉淀目标"与"确认策略"，通过编辑 workflow.md Phase 3.3 walkthrough body 让 AI 在加载 trellis-update-spec 时按新目标执行。Phase 3.3 指令措辞足够强（"本项目覆盖，非 .trellis/spec/"），会覆盖 SKILL.md 的默认指向，无需改 skill。

### 改动 1（已实施）：`.trellis/workflow.md` Phase 3.3 walkthrough body
- 现状：`Update the docs under .trellis/spec/ accordingly. Even if the conclusion is "nothing to update", walk through the judgment.`
- 改为（要点）：
  - 沉淀目标改为 `docs/常见问题.md`（坑/问题/非显而易见行为，Q&A 格式）+ `docs/开发规范.md`（规范/约定/正确做法，✅❌ 示例）。
  - 仅"每个 session 必读的全局约定"才进 `CLAUDE.md`，且起草后须用户确认再写入。
  - `docs/` 可直接写入。
  - 保留"即使无内容可沉淀也要走判断"。
- 约束：只改 walkthrough body，不改 `[required·once]` 标记 -> 无需同步 `[workflow-state:in_progress]` tag block。

### 改动 2（已否决）：不改 `trellis-update-spec/SKILL.md`
- 原计划在 SKILL.md 顶部加"本项目沉淀目标覆盖"段 + Spec Structure Overview 指针。
- 否决原因：SKILL.md 是模板管理文件（`.template-hashes.json` 跟踪），改它有 `trellis update` 覆盖风险；而 workflow.md Phase 3.3 指令措辞足够强，会覆盖 skill 默认，无需改 skill。实现中已试改并回退（`git checkout`）。

### 不改的部分
- `finish-work.md`（沉淀在它之前的 Phase 3.3，命令本身无需动）。
- `trellis-update-spec/SKILL.md`（已否决，见上）。
- `.trellis/scripts/`、`.trellis/config.yaml`、`.claude/settings.json`、`.claude/hooks/*`。
- `[workflow-state:*]` tag block（因未改 `[required·once]` 标记）。

## Out of Scope

- 不改 trellis 上游脚本（`task.py` 等）。
- 不引入新 hook、不新增外部依赖。
- 不把沉淀挪到 finish-work 命令之后（已确认保留 Phase 3.3 时机）。
- 不做"双沉淀"（.trellis/spec/ 与 docs/ 同时写）。
- 不改 `trellis-update-spec/SKILL.md`（已否决，规避模板覆盖风险）。

## Technical Notes

- 关键文件：`.trellis/workflow.md`（Phase 3.3 + Customizing Trellis 章节）、`docs/常见问题.md`、`docs/开发规范.md`、`CLAUDE.md`。
- 模板冲突风险：workflow.md 是官方定制点（低风险）；SKILL.md 不改，无冲突。
- breadcrumb 契约：见 workflow.md "WORKFLOW-STATE BREADCRUMB CONTRACT"。本方案不改 `[required·once]`，故不触发契约同步。
- docs/ 格式参考：常见问题.md 的"虚拟滚动滚动条"条目（多层原因+诊断+修复+排查顺序）是高质量沉淀范例；开发规范.md 的"Git 仓库检测"条目（✅❌+原因）是规范范例。

## Implementation Plan

改动量小，单 commit：

- **Commit**：改 `.trellis/workflow.md` Phase 3.3 walkthrough body -> 沉淀目标指向 docs/+CLAUDE.md，写明确认策略，保留"无内容也要走判断"。
- **验证**：人工走查一个收尾场景，确认 AI 按 Phase 3.3 沉淀到 docs/ 而非 .trellis/spec/；确认 CLAUDE.md 改动走确认流程。
- **收尾**：确认是否更新 README.md（项目 CLAUDE.md 要求）；走 `/trellis:finish-work` 归档本任务。
