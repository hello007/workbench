# memory 目录迁移至 docs/memory 并提供自动加载机制

## Goal

将 Claude Code 自动记忆目录（`C:\Users\liuyang\.claude\projects\D--workspace-workspace-ai-demo-OpenSpec-git-tools-git-manager\memory\`）中的内容迁移到项目内 `docs/memory/`，使其纳入 git 版本管理、跨机器可用；并提供会话自动加载机制，替代原 user 级 MEMORY.md 自动注入行为。

## What I already know

* 现有 memory 内容（共 3 文件）：
  * `MEMORY.md` — 索引（2 条）
  * `workbench-data-write-constraint.md` — data/*.json 手改前须关进程（feedback 类）
  * `trellis-finish-work-entry.md` — 收尾走 /trellis:finish-work（feedback 类）
* 项目已有 `session-start.py` hook（`.claude/settings.json` SessionStart，matcher 覆盖 startup/clear/compact），是现成注入点
* 项目 CLAUDE.md 已自动加载，另有「规范沉淀规则」两级分流先例（docs/spec/ + 关键规则清单）
* memory 文件格式：frontmatter（name/description/metadata.type）+ 正文 + `[[链接]]`

## Assumptions (temporary)

* 迁移后 docs/memory/ 为唯一权威源，user 级 memory 目录废弃
* 注入深度与原机制对齐：只注入索引 MEMORY.md，正文按需读取

## Decision (ADR-lite)

1. **机制选型：方案 B — 新增独立 hook 脚本**（用户 2026-10-20 选定）
   * `.claude/hooks/inject-memory.py`，settings.json SessionStart 加独立条目
   * 职责单一，不与 session-start.py 耦合

1. **迁移方式：Move**（用户 2026-10-20 选定）
   * 内容迁至 `docs/memory/`，user 级目录删除
   * 注入内容声明 docs/memory 为权威源、新记忆写入 docs/memory
   * harness 若再写 user 级目录，人工合并回 docs/memory

3. **注入深度：仅索引**（用户 2026-10-20 选定）
   * 只注入 `docs/memory/MEMORY.md`，正文按需 Read
   * 与原 user 级机制行为一致，体积可控

## Requirements

* `docs/memory/` 目录：迁移 MEMORY.md + workbench-data-write-constraint.md + trellis-finish-work-entry.md（格式保持 frontmatter + 正文 + [[链接]] 不变）
* 新增 `.claude/hooks/inject-memory.py`：SessionStart（startup/clear/compact 三 matcher）注入 docs/memory/MEMORY.md 内容 + 权威源声明（新记忆写入 docs/memory，正文按需读取）
* `.claude/settings.json` SessionStart 追加独立 hook 条目（timeout 15）
* 删除 user 级 memory 目录 `C:\Users\liuyang\.claude\projects\D--workspace-workspace-ai-demo-OpenSpec-git-tools-git-manager\memory\`
* CLAUDE.md / README.md 相应更新

## Acceptance Criteria

* [ ] docs/memory/ 含 MEMORY.md + 2 个 memory 文件，内容与迁移前一致
* [ ] settings.json SessionStart 含 inject-memory.py 条目，startup/clear/compact 均触发
* [ ] 手动运行 `python .claude/hooks/inject-memory.py`（喂入 SessionStart JSON stdin）输出含 memory 索引内容的 additionalContext；docs/memory 缺失时静默降级不报错
* [ ] user 级 memory 目录已删除
* [ ] CLAUDE.md/README.md 更新（说明 docs/memory 机制与新记忆写入约定）

## Definition of Done

* hook 手动回归验证通过
* 文档更新完成

## Out of Scope

* 双写/同步脚本（Windows junction 等）
* memory 条目增删工具化（直接手编 markdown）
* session-start.py 改造

## Technical Notes

* 注入点候选：`.claude/hooks/session-start.py`（已有，输出 additionalContext）
* 原 user 级目录由 Claude Code harness 自动写入（系统提示内置 auto-memory 指令），迁移后新记忆仍可能落到 user 级，需在注入内容中声明 docs/memory 为权威源并指示新记忆写入位置
