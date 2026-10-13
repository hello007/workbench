# 工作目录树复制路径——绝对/相对/文件引用 三级复制菜单

## Goal

工作目录树支持复制文件夹和文件的相对路径（相对当前工作目录根），右键菜单与内容面板均提供对应功能。将现有右键「复制文件名」改造为二级菜单「复制路径」，展开为：复制绝对路径、复制相对路径、文件引用路径（相对路径前加 `@`）。文件夹右键当前缺少复制名/路径分组能力，需一并对齐。

## What I already know

- 右键菜单实现：`frontend/src/components/FileTreePanel.vue` 自定义绝对定位 `<ul class="context-menu">`（非 Element Plus 组件），分三块模板：空白区（~L266）、目录节点（~L289）、文件节点（~L368）。
- 现状：
  - 目录节点菜单：有 `copyPath`（绝对路径，L322），无 `copyName`。
  - 文件节点菜单：有 `copyPath`（L394）+ `copyName`（L397）。
  - 命令分发 `onMenuCommand`（L1143）：`copyPath` 走 `data.path.replaceAll('\\','/')`；`copyName` 走 `data.name`。
  - 剪贴板辅助 `copyToClipboard(text, label)`（~L1429）。
- 工作目录根：`directoryStore.currentDirPath`（store/directory.js，directories + selectedDirectoryId 派生），FileTreePanel 内多处已有「取 selectedDirectoryId 对应 dir.path + 分隔符规范化」的先例（L817-822、L1725-1731）。
- 内容面板：`frontend/src/components/ContentPanel.vue` header 有「复制名称」图标（L9 handleCopyName）+ 路径行「复制路径」按钮（L26 handleCopyPath，L850-869）。Element Plus 体系。
- 已有测试：`FileTreePanel.spec.js`（copyPath/copyName 调 clipboard，L1664+）、`ContentPanel.spec.js`（复制路径/文件名，L832+）。
- 相对路径计算：前端纯函数（normalize 分隔符 + strip 根前缀），无后端改动。

## Assumptions (temporary)

- 相对路径分隔符统一 `/`（与现有 copyPath 的 replaceAll 规范化一致）。
- 节点即为工作目录根本身时相对路径为 `.`、引用路径为 `@.`（罕见，树中根不是节点，仅 ContentPanel 理论可达）。

## Decision (ADR-lite)

**Context**: 二级菜单形态、文件名去留、内容面板承载方式均有多选。
**Decision**（用户未答，采推荐项，可推翻）:
- 文件节点「复制文件名」保留为二级菜单第 4 项「复制文件名」。
- 内容面板路径行「复制路径」按钮改 el-dropdown（绝对/相对/@引用 三项）。
- 自定义右键菜单二级子菜单用 hover 展开飞出（自绘体系内实现，不嵌 EP dropdown）。
**Consequences**: 菜单项 +1（4 项）；ContentPanel 路径行交互由单击变下拉两跳；子菜单 hover 展开需处理菜单边界定位（L1041/L1121）与全局关闭逻辑兼容。

## Requirements (evolving)

- 文件树右键（文件 + 目录节点）：「复制文件名」/「复制路径」合并为二级菜单「复制路径」，含：
  1. 复制绝对路径（现状 copyPath 行为）
  2. 复制相对路径（相对当前工作目录根，`/` 分隔）
  3. 文件引用路径（`@` + 相对路径）
  4. 复制文件名（原 copyName 保留，仅文件节点；目录节点是否带第 4 项实现时定）
- 目录节点同样获得完整分组（新增复制能力对齐文件节点）。
- 内容面板：路径行「复制路径」改 el-dropdown 三项（绝对/相对/@引用）；header「复制名称」图标保留不动。
- 新增前端纯函数计算相对路径（utils 层，输入 nodePath + rootPath，输出 `/` 分隔相对路径），带单元测试。

## Acceptance Criteria (evolving)

- [ ] 文件节点右键二级菜单展开三项，各点分别写入剪贴板：绝对路径 / 相对路径 / `@相对路径`
- [ ] 目录节点右键同上（新增能力）
- [ ] 相对路径以 `directoryStore.currentDirPath` 为根，分隔符统一 `/`
- [ ] 内容面板可复制相对路径与 `@` 引用路径
- [ ] FileTreePanel.spec.js / ContentPanel.spec.js 覆盖新增分支
- [ ] vitest 全绿

## Definition of Done

- 单测覆盖新分支，`cd frontend && npm test` 全绿
- README/功能说明文档同步
- 视觉守 frontend-visual-conventions（自定义右键菜单样式一致性）

## Out of Scope

- 空白区右键菜单不动
- 后端零改动
- 多选节点批量复制路径

## Technical Notes

- 二级菜单需在现有 `.context-menu` 自定义体系上扩展（现有无 submenu 先例）；注意 L1041/L1121 的菜单边界定位逻辑与 `onGlobalContextMenu` 关闭逻辑兼容。
- 相对路径计算可参考 L1725-1731 现有 rootPath 规范化写法。
