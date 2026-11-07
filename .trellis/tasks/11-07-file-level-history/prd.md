# 文件级提交历史

## Goal

CommitHistory 只有仓库级历史（已核对 2026-11-07），单文件演进史无法追查；重命名文件历史断链（需 --follow）。提供文件级提交历史：文件树/预览右键「查看历史」→ 该文件的 commit 列表（可查看任一条的 diff），与既有仓库级 CommitHistory 形成追溯闭环。

## What I already know

- CommitHistory.vue 已有关键词搜索与作者过滤（行 8-54），组件参数化复用是首选路径（传入 filePath 维度过滤）
- git 命令：`git log --follow -- <path>` 追溯重命名；涉及中文文件名/提交信息的调用须自带 UTF-8 `-c` 前缀（`i18n.logOutputEncoding`/`core.quotepath=off`）与 `%x00`/`%x01` 分隔符（branch-compare.md 规范，`GitCommand` 不默认带）
- diff 展示复用 FileDiffDialog（关闭释放双时机等契约见 perf-baseline.md §16.5）
- 右键菜单扩展点：FileTreePanel 右键菜单（onMenuCommand 分支）与 ContentPanel 重命名/删除事件模式
- 大文件历史性能：千级 commit 文件的分页/懒加载策略待定

## Open Questions（brainstorm 待讨论）

- 入口形态：文件树右键项 + 文件预览工具栏按钮，两处都要还是先做一处
- 与 CommitHistory 的组件关系：参数化复用同一组件 vs 轻量独立组件（--follow 语义、rename 展示差异）
- --follow 的限制（单路径生效）在目录级入口如何降级（目录右键是否提供「查看目录历史」非 follow 版）
- rename 检出（--name-status R 行）在列表中的展示

## Acceptance Criteria (evolving)

- [ ] 文件级历史列表 + 任一提交的 diff 查看
- [ ] 中文名文件历史无乱码；重命名历史不断链（--follow 生效）
- [ ] 组件单测 + 契约同步无回归

## Technical Notes

- 调研定位：高价值候选 #5，与候选 #4（blame）同属追溯域，blame 未立项可后续接力
