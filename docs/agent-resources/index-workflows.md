# 工作流索引

> 按场景划分的行为指南，进入对应工作模式时加载。
> 所有工作流位于 `docs/agent-resources/workflows/` 目录。

| 文档 | 名称 | 适用场景 |
|------|------|----------|
| `docs/agent-resources/workflows/development.md` | 开发/修改功能 | 新功能开发、现有功能修改、代码重构 |
| `docs/agent-resources/workflows/bugfix.md` | 修复 Bug | Bug 定位、问题修复、缺陷消除 |
| `docs/agent-resources/workflows/documentation.md` | 写文档 | 文档生成、补全、修订、治理 |
| `docs/agent-resources/workflows/code-review.md` | Code Review | 代码变更质量审查 |

> **提示**：新增 workflow 时需在上方表格登记，且文件名与 frontmatter 的 `name` / `title` / `description` 保持一致。
> 本仓任务分流（问答 / 建任务 / 内联小修）由 Trellis workflow-state 每回合注入承担；版本发布走仓根 `/release` skill（`scripts/release.sh`）。
