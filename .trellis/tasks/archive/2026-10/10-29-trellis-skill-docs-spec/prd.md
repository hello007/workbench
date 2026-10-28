# trellis 沉淀链 skill 旧路径出清：指向 docs/spec 与覆盖层口径

## Goal

`.claude/` 下 trellis 系 skills/agents/hooks 仍全篇引用已废弃的 `.trellis/spec/` 目录（2026-09-07 移除）。项目侧靠 CLAUDE.md「规范沉淀规则」与 workflow.md Phase 3.3 覆盖段两次纠偏，但 skill 是 Phase 3.3 实际加载物——每次沉淀先读 357 行旧路径语境再靠覆盖层纠正，对抗性引导。本任务将沉淀链引导面出清至 `docs/spec/` + CLAUDE.md 覆盖层口径，加防再漂移声明，并补做 851c4e8 Q5 留尾的动态导流验证。

## Requirements

1. **trellis-update-spec/SKILL.md 深度出清**（沉淀主链路）：
   - description 与 3 处 `.trellis/spec/` 引用改为 `docs/spec/<topic>.md` + CLAUDE.md「规范沉淀规则」
   - `Spec Structure Overview` 目录树图改写为 docs/spec 现状
   - `guides/` 分类学段（Code-Spec vs Guide 二分）改写为 CLAUDE.md 两级分流映射（详细契约入 docs/spec vs 钩子级规则入关键规则清单）
   - 头部加权威声明：「项目特定口径（两级分流、两档上限、写入路径）以 CLAUDE.md『规范沉淀规则』为唯一权威，本 skill 只提供通用方法论」
   - 7 节强制模板、Update Templates、Quality Checklist 等通用方法论保留
2. **外围 skills 路径出清**：trellis-before-dev（3 处）、trellis-break-loop（3 处）、trellis-check（2 处）的 `.trellis/spec/` 引用改写为「CLAUDE.md 关键规则清单 + docs/spec/」口径
3. **agents 出清**：trellis-check.md（2 处）、trellis-implement.md（4 处）、trellis-research.md（2 处）同口径改写
4. **trellis-spec-bootstarp 弃用标注**：frontmatter description 前缀「本仓已废弃 .trellis/spec/，此 skill 不适用」，正文不动
5. **动态导流验证**（851c4e8 Q5 留尾并入）：读改后文本走查——新会话语境（CLAUDE.md + 改后 skill）下模拟判断「新规则写哪、按什么口径」，确认引导正确；不真跑任务；走查记录入任务目录
6. **白名单登记**：所有豁免物及理由登记于本 prd（见 Decision 节）

## Acceptance Criteria

- [x] update-spec/before-dev/break-loop/check 四 skill + 三 agents 文件内零 `.trellis/spec/` 活引用（grep 复核通过，2026-10-29）
- [x] spec-bootstarp 仅 description 弃用前缀为活改动；SKILL.md 正文 4 处 + references/ 5 处保留（白名单：弃用标注方案预期内，见 Decision 节裁定 2）
- [x] update-spec 头部权威声明到位，口径与 CLAUDE.md 两档（普通行 ≤250 / 钩子密集行与合并行 ≤350）及 finish-work 分档自检无矛盾（见导流走查记录.md 口径核对表）
- [x] 动态导流走查记录落盘（导流走查记录.md），结论「引导正确」，851c4e8 Q5 留尾闭环
- [x] workflow.md 零改动（git status 复核通过，2026-10-29）
- [x] 纯文档零代码（session-start.py 不动；改动面仅 .claude/ 8 个文档文件 + 本任务目录）
- [x] CLAUDE.md 零改动（预期达成——口径本体未动，无需升 v2.43）
- [ ] journal 记录（journal-3.md 已 1908/2000，收尾时留意轮转提示）

## Definition of Done

- 验收清单逐项复核通过
- 白名单口径在 prd 完整登记
- journal 收尾记录含轮转状态确认

## Decision (ADR-lite)

**Context**：`.trellis/spec/` 目录 2026-09-07 移除后，trellis 系引导文件仍指向旧路径；前两任务（cd5e7e2、851c4e8）刻意留尾不动 skill。摸排确认引用面远超 update-spec 单点。

**Decision**（2026-10-29 用户逐项确认）：
1. **trellis-meta 整体白名单豁免**（~29 处）：框架通用文档非本仓定制物，延续「框架文件不改、覆盖层纠偏」策略；本节即白名单登记处，验收按白名单算通过。
2. **trellis-spec-bootstarp 弃用标注**：用途（从零引导建 .trellis/spec/ 库）本仓已死，description 前缀防误触发，正文保留（白名单）。
3. **session-start.py 白名单豁免**：守住「纯文档零代码」底线；`spec_dir.is_dir()` 守卫完备运行时死路径无害；PATHS 静态文案轻微误导留待后续 hook 维护任务。
4. **清理深度=路径替换+旧语境段删改**：目录树图、guides/ 分类学改写，7 节模板等通用方法论保留；不做全文重构。
5. **动态导流验证并入**：skill 是导流链源头，修好后验证才有意义。

**Consequences**：
- 沉淀链加载语境与 CLAUDE.md 口径一致，消除每次沉淀的对抗性引导
- meta references 29 处旧路径仍存（白名单），若未来有人按 meta 文档操作 trellis 部署需人工知晓本仓差异——由本 prd 与 CLAUDE.md 覆盖层兜底
- skills 为一次性导入 fork 物（git log 证实无上游自动同步渠道），定制无 fork 约定约束；定制标记以本 prd + skill 头部声明承担

## Out of Scope

- trellis-meta references/ 全部文件（白名单豁免）
- session-start.py 任何改动（含 PATHS 静态文案）
- workflow.md Phase 3.3 正文重写
- docs/spec/ 内容本体修订
- update-spec 拆分 references/ 结构重组（trellis-meta 软约定建议 SKILL.md 短化，但超出出清目标）
- CLAUDE.md 口径本体改动

## Technical Notes

- 摸排命令：`grep -rn '\.trellis[/\]spec' .claude/`（2026-10-29 基线，结果见 git history 本任务 commit）
- 涉改文件清单（8 个）：
  - `.claude/skills/trellis-update-spec/SKILL.md`（357 行，深度改写）
  - `.claude/skills/trellis-before-dev/SKILL.md`、`.claude/skills/trellis-break-loop/SKILL.md`、`.claude/skills/trellis-check/SKILL.md`
  - `.claude/skills/trellis-spec-bootstarp/SKILL.md`（仅 description）
  - `.claude/agents/trellis-check.md`、`.claude/agents/trellis-implement.md`、`.claude/agents/trellis-research.md`
- 前序任务：cd5e7e2（CLAUDE.md 瘦身）、851c4e8（口径对齐，Q5 留尾本任务闭环）
- session-start.py spec 注入段：733-757 行（守卫完备）；PATHS 段静态文案行待后续 hook 任务定位
- journal-3.md 1908/2000 行，收尾时确认是否触发轮转
