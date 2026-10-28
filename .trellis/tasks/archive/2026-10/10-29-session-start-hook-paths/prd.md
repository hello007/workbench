# session-start hook 旧路径死代码出清与 PATHS 文案修正

## Goal

`.claude/hooks/session-start.py` 及关联 trellis 系文件仍存在 `.trellis/spec/`（2026-09-07 移除）残留：死代码路径（守卫静默跳过）、静态文案（每次会话可见）、以及一处**活引导误导**（Phase 1.3 文案引导 AI 往 implement.jsonl 填废弃路径）。前任务（10-29-trellis-skill-docs-spec，commit adcb9d3）裁定 session-start.py 白名单豁免并明言「PATHS 静态文案留待后续 hook 维护任务」——本任务即该后续任务。目标：全仓（含 `.trellis/scripts/`）零 `.trellis/spec/` 活引用或白名单登记，hook 实跑输出 diff 干净。

## What I already know（摸排事实，2026-10-29）

### session-start.py 引用全貌（用户所述「3 处」之外的扩展）

| # | 位置 | 性质 | 危害 |
|---|---|---|---|
| 1 | line 369-371（Phase 1.3 Next-Action） | **活引导误导**：文案明示「Only spec paths (`.trellis/spec/**/*.md`)」+「run get_context.py --mode packages to list available specs」 | **最重**：planning 阶段主会话照做会往 implement.jsonl 填不存在路径（本项目实际口径=docs/spec + CLAUDE.md 关键规则行，与 workflow.md Phase 1.3 覆盖段冲突） |
| 2 | line 72（first-reply-notice 文案） | 静态文案「spec 索引已加载」 | 轻微：实际无 spec 索引注入 |
| 3 | line 704-706（guidelines 段头文案） | 「Project spec indexes are listed by path below」 | 轻微：下方 spec 段死路径永不输出 |
| 4 | line 724-728（guides_index 注入） | 死路径：`spec/guides/index.md` is_file() 守卫恒 false | 无害 |
| 5 | line 730-757（spec 扫描块） | 死路径：spec_dir.is_dir() 守卫 | 无害 |
| 6 | line 759-763（`if paths:` 输出段） | 死路径：paths 恒空 | 无害 |
| 7 | line 765-768（Discover more via --mode packages） | 文案承接 spec 段；single-repo 下 --mode packages 输出仅「Single-repo project」无信息量 | 轻微 |
| 8 | line 441-484 `_check_legacy_spec` + 691-693 调用 + migration-warning 输出 | 死路径：is_mono 短路（本项目 False）+ spec_dir.is_dir() 双守卫恒 None | 无害 |
| 9 | line 487-535 `_resolve_spec_scope` + 672-677 config 加载（is_mono/packages/scope_config/allowed_pkgs） | 级联死代码：allowed_pkgs 唯一消费方是 #5 扫描块；#8 删后 packages 仅剩 #9 消费 | 无害（实施时核实 packages 无其他消费方） |

### 其他文件

| 文件 | 位置 | 性质 | 处置候选 |
|---|---|---|---|
| `.trellis/scripts/common/session_context.py` | line 645 `Spec: {DIR_WORKFLOW}/{DIR_SPEC}/` | **PATHS 段静态打印**，每次会话 current-state 可见（任务标题主体） | 删行 |
| `.trellis/scripts/common/packages_context.py` | `_scan_spec_layers` 全链（is_dir 守卫恒空）、line 198/200/207 spec 路径打印 | 上游 monorepo 通用代码，single-repo 恒空输出 | 倾向白名单（框架文件不改策略） |
| `.trellis/scripts/common/paths.py` | line 395 `get_spec_dir()` 全仓零调用方 | 死函数（上游通用 API） | 倾向白名单 |
| `.trellis/scripts/task.py` | line 336 help 示例 `.trellis/spec/cli/backend/auth.md` | 静态示例文案 | 改为 docs/spec 示例（低风险） |
| `AGENTS.md` | line 9 `.trellis/spec/` 描述为活路径 | **活误导**；但位于 TRELLIS:START/END 管理块内（块内改动会被 `trellis update` 覆盖） | 块外加覆盖层纠偏行（升级安全） |
| `.claude/skills/trellis-before-dev/SKILL.md` | line 12 `get_context.py --mode packages` | 文件路径引用非 spec 字面；single-repo 输出无 spec 字样 | 倾向不动（输出无误导） |

### 白名单沿用（前任务裁定）

- trellis-meta references/ 全部（~29 处）：框架通用文档
- trellis-spec-bootstarp 正文：已加弃用标注
- 历史记录（journal-\*、tasks/archive/、CHANGELOG.md）：天然豁免

### 已证伪：活化方案（docs/spec 索引注入）

CLAUDE.md（每次会话自动加载）关键规则表每行已带 docs/spec 链接，文档索引表含「规范沉淀 → docs/spec/README.md」行；索引功能已由 CLAUDE.md 承担，hook 活化=每次会话膨胀 context、收益≈0。**活化出局**。

### 机制确认

- **模板 hash**：`.trellis/.template-hashes.json` 登记全部 trellis 系文件；hash 仅被上游 `trellis update` 消费（本地 safe_commit.py 仅列忽略清单）。workflow.md 早已改动且 hash 失配无事故——「改文件留 hash 失配」是既定实践，无阻塞。
- **pytest 覆盖**：session-start.py 无任何 pytest 测试；hook 不在任何测试门禁（Go/npm）范围。回归验证=实跑比对。
- **journal 轮转**：add_session.py `create_new_journal_file` 自动创建 journal-4（带 Continuation 头），机制存在；收尾实测确认。
- **AGENTS.md hash**：登记于 hash 表，块外追加同样造成失配，无阻塞。

## Requirements

**session-start.py**（9 项，活文案 3 + 死代码 6）：

1. line 369-371 Phase 1.3 Next-Action 文案改写：`.trellis/spec/**/*.md` → `docs/spec/**/*.md`；「get_context.py --mode packages to list available specs」→ 指向 `docs/spec/README.md` 索引（与 workflow.md 390 行口径一致）
2. line 72 first-reply-notice 文案：删「、spec 索引已加载」
3. line 704-706 guidelines 段头：删「Project spec indexes are listed by path below」语境句，保留 jsonl 注入 + dispatch 机制说明
4. 删 line 724-728 guides_index 注入段
5. 删 line 730-757 spec 扫描块
6. 删 line 759-763 `if paths:` 输出段
7. 删 line 765-768 Discover more 行
8. 删 line 441-484 `_check_legacy_spec` 函数 + line 691-693 调用 + migration-warning 输出
9. 删 line 487-535 `_resolve_spec_scope` + line 672-677 级联加载（is_mono/packages/scope_config/allowed_pkgs）——实施时先核实 packages 无其他消费方再删

**其他文件**（3 项）：

10. session_context.py:645：删 `Spec: {DIR_WORKFLOW}/{DIR_SPEC}/` 行
11. AGENTS.md：TRELLIS:END 块后追加纠偏行（`.trellis/spec/` 已废弃、规范位于 `docs/spec/` + CLAUDE.md 关键规则），块内不动（升级安全）
12. task.py:336：help 示例路径 `.trellis/spec/cli/backend/auth.md` → `docs/spec/cross-layer-contracts.md`（真实存在文件）

## Decision (ADR-lite)

**Context**：前任务 adcb9d3 Decision 3 明言 PATHS 文案「留待后续 hook 维护任务」；本轮摸排发现引用面远超 3 处字面引用，且含一处活引导误导（Phase 1.3 文案）。清理深度三选（A 仅文案 / B 文案+hook 死代码 / C 全出清含上游脚本）。

**Decision**（2026-10-29 用户裁定）：

1. **深度=B**：活文案全改 + hook 内死代码全删 + PATHS 行 + AGENTS.md 块外纠偏 + task.py 示例改。「死代码出清」任务名落实，一轮收干净。
2. **上游脚本白名单**：packages_context.py `_scan_spec_layers` 链、paths.py `get_spec_dir()`（零调用死函数）、get_context.py packages 模式——延续前任务「框架文件不改、覆盖层纠偏」策略（skills 已判定「一次性导入 fork 物，无上游自动同步」，scripts 同理）；恒空守卫无害，monorepo 通用逻辑保留。
3. **活化方案证伪出局**：CLAUDE.md 关键规则表每行已带 docs/spec 链接 + 文档索引表直达 docs/spec/README.md，hook 活化=纯 context 膨胀。
4. **trellis-meta / trellis-spec-bootstarp / 历史记录白名单沿用**前任务裁定。
5. **AGENTS.md 走块外纠偏**：块内改动会被 `trellis update` 覆盖。

**Consequences**：

- 每次会话注入不再出现任何 spec 死文案，Phase 1.3 引导与 CLAUDE.md 口径一致
- hook 净瘦约 120 行死代码；若未来恢复 `.trellis/spec/` 目录（含 monorepo 场景），hook 内注入能力需重新引入——由本 prd 与 git history 兜底
- 上游脚本 3 处残留按白名单登记，全仓验收按白名单算通过

## Acceptance Criteria

- [ ] 全仓（含 .trellis/scripts/）grep `.trellis/spec` 零活引用或白名单登记于本 prd（白名单：上游脚本 3 处 + trellis-meta + spec-bootstarp 正文 + before-dev line 12 + 历史记录）
- [ ] hook 实跑改前/改后输出 diff：仅预期删除段差异，其余逐字节一致（改前先留存基线输出）
- [ ] SessionStart 注入不再出现 spec 相关段落与「Spec: .trellis/spec/」行
- [ ] first-reply-notice 与 Phase 1.3 文案无 `.trellis/spec` 字样
- [ ] 纯 hook/trellis 脚本改动，零 workbench 业务代码
- [ ] workflow.md / CLAUDE.md 零改动
- [x] journal 收尾记录，确认 journal-4 轮转行为并记录（Session 168，33 行追加后 1974/2000 **未触发轮转**，journal-3 余 26 行；轮转机制=add_session.py 自动创建 journal-4 带 Continuation 头，已从代码确认，留待下次会话触发）

## Definition of Done

- 验收清单逐项复核通过
- 白名单口径在本 prd 完整登记
- journal 收尾记录含轮转状态确认

## Out of Scope (explicit)

- workbench 业务代码（Go/前端）任何改动
- workflow.md / CLAUDE.md 改动
- docs/spec/ 内容本体修订
- trellis-meta / trellis-spec-bootstarp（前任务白名单沿用）
- 历史记录（journal、archive、CHANGELOG）

## Technical Notes

- 前序任务：adcb9d3（10-29-trellis-skill-docs-spec，Decision 3 明言本任务）；cd5e7e2、851c4e8（更早口径对齐）
- 摸排基线：2026-10-29，grep `.trellis[/\]spec` 全仓 92 文件命中，分类后活引用面见上表
- hook 高风险文件：改后必须实跑 `python .claude/hooks/session-start.py`（含 hook 输入 JSON 的真实调用方式实施时确认）比对
- journal-3.md 当前 1941/2000 行，本轮收尾大概率触发轮转
