# brainstorm: CLAUDE.md 内容迁移至 AGENTS.md 与 docs/agent-resources 整理

## Goal

将仓根 `CLAUDE.md` 的全部通用内容迁入 `AGENTS.md`（CLAUDE.md 缩为仅 `@AGENTS.md` 一行），做结构精简；文档索引收口 `docs-management.md`；整理 `docs/` 两个遗留项。目标：消除 CLAUDE.md 与 AGENTS.md 双头维护，所有 AI agent（Claude Code / Codex / 其他）共享单一入口。

## Requirements

1. **CLAUDE.md 缩为 1 行**：仅保留 `@AGENTS.md` import（用户确认：整表迁 AGENTS.md 方案）
2. **AGENTS.md 承载全部通用内容**（管理块外）：
   - 项目概述 + 技术栈表 → 并入 §1 项目概览（技术栈用表格形式替换现有文字行）
   - 常用命令表 → 并入 §2，补 3 条缺失命令（soak 深跑 / 查看端口 / 停止进程）
   - 规范沉淀规则（6 行）+ 关键规则表（89 行）→ 新节，**逐行原样迁移，不改语义**
   - 项目记忆机制（8 行）→ 新节（inject-memory.py 是本仓 `.claude/` 配置的实现描述，随机制迁入）
   - 中文文档规则 → 并入引言或硬性约束节
   - 项目结构树（16 行）→ 删除（reference/architecture.md §2 已有更详细版本）
3. **文档索引表（20 行）收口 `docs-management.md`**：与其「目录结构」节合并为一张权威入口表；删除 project-context.md 死链行；AGENTS.md §7 留一行指向
4. **引用同步（7 文件）**：「仓根 CLAUDE.md」→「AGENTS.md」：
   - `.claude/skills/trellis-before-dev/SKILL.md`（2 处）
   - `.claude/skills/trellis-update-spec/SKILL.md`（10 处）
   - `.trellis/workflow.md`（3 处，仅改正文描述，不碰 [required · once] 标记）
   - `docs/spec/README.md`（3 处）
   - `docs/agent-resources/reference/docs-management.md`（多处，含索引表并入）
   - `docs/agent-resources/reference/config-variables.md`（1 处）
   - `docs/agent-resources/conventions/testing-strategy.md`（1 处权威声明）
5. **docs/ 整理**：
   - `docs/浏览器模式-二期扩展.md` → 移入 `docs/archive/`
   - project-context.md 死链行删除（随索引收口完成）
   - `docs/research/`、`docs/原始需求/` 维持现状
   - 正式文档（快速入门/功能说明/FAQ/路线图/开发工作流/测试策略/部署/开发规范/架构/API参考/贡献指南）一律不迁 agent-resources

## Acceptance Criteria

- [ ] CLAUDE.md 仅含 `@AGENTS.md` 一行
- [ ] AGENTS.md 承载全部通用内容，管理块未被改动
- [ ] 关键规则表 89 行逐行内容不变（迁移 ≠ 改写）
- [ ] 规范沉淀规则节原样保留（含两档上限/下沉判据）
- [ ] 7 文件引用同步完成；grep 「CLAUDE.md」残留为零（豁免：任务归档/journal/git 历史）
- [ ] 文档索引仅存在于 docs-management.md 一处，无死链
- [ ] 浏览器二期扩展文档位于 docs/archive/，原位置无残留
- [ ] 新开 Claude 会话关键规则注入正常（@import 生效验证）

## Definition of Done

- grep 验证引用同步完整
- 关键规则表 diff 逐行比对（仅表壳位置变化，行内容零 diff）
- 文档版本尾注更新（AGENTS.md 或 docs-management.md 记录变更）

## Decision (ADR-lite)

**Context**: CLAUDE.md 与 AGENTS.md 双头维护，约 40% 内容重复；关键规则表 89 行是每次会话必知规则，被 7 个文件硬引用。
**Decision**: 整表迁 AGENTS.md、CLAUDE.md 缩为 1 行（方案 A）；结构合并为主、关键规则表逐行原样迁移（方案 A）；文档索引收口 docs-management.md（方案 A）。
**Consequences**: 单一真相源，Codex 等其他 agent 同等受益；代价是 7 文件引用一次性同步；Claude 侧依赖 `@AGENTS.md` import 机制（Claude Code 原生支持，无风险）。

## Out of Scope

- 关键规则表内容改写/规则增删（纯迁移）
- `docs/spec/`（32 文件）与 `docs/memory/` 内部整理
- Trellis 管理块内内容变更
- `.claude/rules/`、`.claude/skills/` 机制重构
- workbench/ 子项目内文档（DEVELOPMENT.md / BUILD_SUMMARY.md）整理

## Technical Notes

- AGENTS.md 管理块边界：`Managed by Trellis` 块内禁改，全部新增内容加块外
- CLAUDE.md 技术栈表比 AGENTS.md §1 文字版多测试框架/路由/构建工具行 → 采用表格形式
- 关键规则表最后一行链接 `docs/开发规范.md#git-提交规范`——路径不变无影响
- `.claude/rules/` 8 文件引用 `conventions/` 对应文件，与本迁移无联动
- AGENTS.md 新节编号建议：规范沉淀+关键规则作为新节插在 §4 开发规范之前，后续节顺延重编号；项目记忆节放 §7 附近
- 验证命令：`grep -rn "CLAUDE.md" --include="*.md" --exclude-dir={node_modules,.git,.trellis/tasks,.trellis/workspace}` 应仅剩 @AGENTS.md 自身与豁免项
