# PRD：修复 frontend-visual-conventions.md 审查发现 7 项

## 背景

commit 5b56c42 沉淀的 `docs/spec/frontend-visual-conventions.md` 经审查发现 2 major + 5 minor 问题，全为「文档描述与代码现状不符」。

## 修复原则

规范文档写目标状态；参照组件现状未达标的，标注「待迁移」而非改代码。

## 需修复项

1. [major] L217 弹窗分档表：RepoFilterDialog/AiTaskHistoryPanel 实际固定 `width="900px"/"920px"`，非 min() 模式——参照标注改「目标值 900/920（现状固定 px，待迁移 min() 模式）」
2. [major] L218 DashboardView 添加仓库 640px 同为固定像素——同上标注待迁移
3. [minor] L116 卡片 hover 过渡：示例 `--transition-fast`，实际 style.css `.card` 为 `--transition-normal`——标注「规范值 fast，现状 normal 待归一」
4. [minor] L112 指示条圆角：示例 `--radius-sm`，ActivityBar.vue:118 实际硬编码 2px——注明现状
5. [minor] L83 line-through 实际只作用于 `.repo-name.repo-missing` 非整行——改为「行整体 opacity:0.5 + 仓库名 line-through」
6. [minor] L324 沉淀来源任务路径已归档——改 `.trellis/tasks/archive/2026-09/09-16-workbench`
7. [minor] L204/L248/L283 代码块语言标注错：el-dialog 模板属性标成 css、html 块用 CSS 注释——改标 vue/html 并用对应注释语法

## 范围

仅改 `docs/spec/frontend-visual-conventions.md` 一个文件；不改代码、不改 README/CLAUDE.md 索引行（描述无变化）、不动 design-tokens.md。

## 验收

- 7 项全部修复
- 文档内无「参照组件违反自身规范」的自相矛盾
- grep 确认无失效任务路径残留
