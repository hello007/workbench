# 仓库自动打标签与筛选数据格式增强

## Goal

对 `D:/workspace/workspace_ai/github` 下开源 git 仓库批量生成分类标签（每个≥1），
写入 WorkBench 产物数据 `D:\Program Files\WorkBench\data\repo_meta.json`，
使仓库筛选器可按标签分类。**本次为数据操作，不改 WorkBench 代码/数据格式。**

## Decision (ADR-lite)

**Context**：WorkBench 已有完整 tag+筛选系统（`RepoMeta.Tags` + `RepoFilterDialog` 标签 OR 筛选），
但 github 下 58 个仓库仅 7 个有手动 tag，风格不统一（harness/mcp 英文小写 vs PPT 大写）。
用户首次使用需批量初始化标签。

**Decision**：不在 WorkBench 新增自动推断功能；本次会话由 LLM 读取各仓库 README/技术栈，
按"预设 13 类中文 + 术语/库名英文原样"推断，**统一覆盖**写入现有 Tags 字段。
后续新增仓库用户手动维护。作用域=repo_meta.json 里 workspace_ai/github 全部 58 条目。

**Consequences**：零代码改动、零格式变更、向后兼容；标签质量取决于本次推断，可随时手动修正。
预设 13 类：Claude生态 / MCP / Agent / AI框架 / 前端UI / Skill技能包 / 开发工具 / 工作流编排 / 文档翻译 / 知识图谱 / PPT演示 / 股票量化 / 其他。

## Requirements

- ✅ github 下 58 个仓库每个≥1 分类标签（预设类目 + 自由补充）
- ✅ 统一覆盖现有 7 个旧 tag
- ✅ 写入 repo_meta.json 现有 Tags 字段（不改格式/代码）
- ✅ 中文类目 + 英文术语/库名原样（MCP/spring-ai/harness/OpenSpec）
- ✅ 其他 188 个仓库不变

## Acceptance Criteria

- [x] 58 个 github 仓库全部有 tag（验证：tagged=58）
- [x] JSON 合法、total repos 仍 246（其他仓库未受影响）
- [x] updatedAt 已刷新（2026-07-20T11:06:41+08:00）
- [x] 写入前 WorkBench 已关闭（tasklist 原子校验）
- [x] 双备份：`repo_meta.json.bak-20260720` / `repo_meta.json.bak-apply`

## Out of Scope

- 不在 WorkBench 新增"自动推断 tag"功能（用户明确：后续手动维护）
- 不改数据格式/代码（现有 Tags 字段已支持筛选）
- 不处理 repo_meta.json 中非 github 的 188 个仓库

## Technical Notes

- 产物数据：`D:\Program Files\WorkBench\data\repo_meta.json`（**非**源码 `git-manager/data/`）
- 结构：`{repos: {path: {path, summary, tags, readmeSummary, missing, updatedAt, lastScanAt}}}`
- 执行脚本：`.trellis/tasks/07-20-auto-tag-repo-filter/apply_tags.py`（默认 dry-run，`--apply` 写入）
- 辅助：`scan_repos.py`（提取 README/技术栈）、`tags_preview.md`（方案预览）
- **写入约束**：WorkBench 运行时 `RepoMetaService` 整体覆盖 repo_meta.json，会丢数据 → 写前必须关闭 workbench.exe
- 不改代码 → README.md / docs 无需更新（CLAUDE.md 要求已评估）
