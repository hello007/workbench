# 仓库信息面板样式修复与仓库统计跳转

## Goal

修复右侧栏 Git 仓库信息面板三个样式 bug（标签不滚动、远程地址与按钮不对齐、变基模式开关致切换分支按钮跳动），并在仓库统计页展示当前仓库信息、为工作目录与文件树右键菜单增加「跳转仓库统计」一键入口，补齐统计页的上下文与可达性。

## What I already know

### 代码定位（前端 Vue3 + Element Plus）

* 右侧栏 Git 面板聚合于 `frontend/src/components/ContentPanel.vue`：
  * `.git-actions`（line 32-45）：拉取更新 / 变基模式 el-switch / 切换分支按钮。
  * `.git-tabs`（line 50-87）：8 个 tab pane，标签/远程/分支等卡片挂在此。
* `ContentPanel.vue` 样式：`.git-tabs :deep(.el-tab-pane){height:100%;overflow:hidden}`（line 1487-1490）—— tab 内容区固定高度且裁剪溢出。
* 子卡片组件：
  * `GitTags.vue`：`.tags-container{min-height:60px}` 无 max-height / overflow。
  * `GitRemotes.vue`：`.remote-row` 为 block；`.remote-main`（name+url flex-wrap）在上，`.remote-actions{margin-top:6px;justify-content:flex-end}` 独占下行。
  * `GitBranches.vue`：分支行同 remote 模式（用户未报，但结构一致）。
* 仓库统计页 `frontend/src/views/StatsView.vue`：
  * header 仅「仓库统计」标题 + 时间档位 radio + 刷新按钮，无当前仓库信息。
  * `repoPath = workspaceStore.selectedNode?.path`；`stats` 对象字段：`dateRange/totalCommits/contributors/granularity/sampled`，无仓库名/根路径/分支。
  * `loadStats` 仅 `uiStore.activePanel==='stats'` 时调后端；切面板/切仓库自动重载。
* 导航跳转：`ActivityBar.vue` 设 `uiStore.activePanel = 'stats'` 即切到统计页。
* 右键菜单两处：
  * `DirectoryTree.vue`（工作目录右键）：`onMenuCommand` switch（line 348-385），菜单项 line 70-105；dir 对象 `{id,name,path,isGitRepo,hasRemote,isDefault}`。
  * `FileTreePanel.vue`（文件树右键）：`onMenuCommand` switch（line 832-912），分空白区/目录/文件三模板；节点 data `{name,path,type,isGitRepo,...}`；已 import `useDirectoryStore/useSettingsStore/useFavoritesStore`，未引 workspace/ui store。
* `workspaceStore.selectedNode` 结构：`{name,path,type,isGitRepo,...}`（文件树节点）。

## Requirements (evolving)

### Bug1 — Git 标签过多不滚动

* `GitTags.vue` 标签列表区加滚动：`.tags-container` 或 `.tag-list` 设 `max-height` + `overflow-y:auto`，标签超出时显示滚动条可查看完整列表。

### Bug2 — Git 远程仓库地址与按钮不对齐

* `GitRemotes.vue` `.remote-row` 改横向 flex 布局：地址块（name+url）左侧 `flex:1;min-width:0`，操作按钮（拉取/删除）右侧 `flex-shrink:0`，垂直居中对齐，地址与按钮同行。

### Bug3 — 变基模式开关致切换分支按钮跳动

* `ContentPanel.vue` `.git-actions` 改 flex + gap 布局，增大组件间距（移除 switch 的 `margin-left`）。
* el-switch 固定宽度（或两态等长文字），消除 on/off 状态宽度变化，避免「切换分支」按钮位置随之跳动。

### 功能2 — 仓库统计页显示当前仓库信息

* `StatsView.vue` header（或摘要区）展示当前统计所对应的仓库信息（具体字段待定，见 Open Questions）。

### 功能3 — 右键「跳转仓库统计」

* `DirectoryTree.vue` 工作目录右键 + `FileTreePanel.vue` 文件树右键（目录节点/空白区）增加「跳转仓库统计」菜单项。
* 点击后：设 `workspaceStore.selectedNode` 为目标节点，切 `uiStore.activePanel='stats'`，StatsView 自动加载该仓库统计。

## Acceptance Criteria (evolving)

* [ ] Git 标签数量超出可视区时出现滚动条，可滚动查看全部标签。
* [ ] Git 远程仓库每行地址与「拉取/删除」按钮同行垂直居中对齐。
* [ ] 反复切换「变基模式」开关，「切换分支」按钮位置不随之跳动；两组件间距合理（不紧挨）。
* [ ] 仓库统计页展示当前仓库信息，切仓库/切目录后信息同步更新。
* [ ] 工作目录右键「跳转仓库统计」可一键跳转并加载该仓库统计。
* [ ] 文件树右键（目录/空白区）「跳转仓库统计」可一键跳转并加载。
* [ ] 非统计页不触发无谓后端统计请求（维持现有 loadStats 面板可见性判断）。

## Definition of Done

* 前端单测更新（GitTags/GitRemotes/StatsView/DirectoryTree/FileTreePanel 受影响用例）。
* `cd frontend && npm test` 绿；覆盖率不低于门禁（前端 ≥70%）。
* 视觉回归：`wails dev` 实测三个 bug 消失 + 跳转可用。
* README / docs 视行为变化决定是否更新（功能说明.md 右键菜单 / 仓库统计章节）。

## Out of Scope (explicit)

* 后端新增接口（仓库信息复用现有 selectedNode / GetGitRemoteURL，不新增 Go 方法）。
* 统计图表本身重构。

## Technical Notes

* Bug1 滚动加在 `.tags-container`（tab-pane 已 overflow:hidden 固定高度，card 内列表区滚动即可）。
* Bug3 el-switch 固定宽度：Element Plus el-switch 无 width prop，通过 inline style 或 `:deep(.el-switch__core)` 设 width；inline-prompt 模式滑块宽度随内容变化是跳动根因。
* 功能3 FileTreePanel 需新增 import `useWorkspaceStore, useUiStore`；DirectoryTree 已 import `useUiStore`，需补 `useWorkspaceStore`。
* 跳转后 StatsView 加载链路：设 selectedNode → repoPath 变 → watch(repoPath) 调 loadStats（但非 stats 面板 return）→ 设 activePanel='stats' → watch(activePanel) 触发 loadStats。顺序先 selectedNode 后 activePanel。

## Decision (ADR-lite)

**Context**：4 个 preference 决策点（Bug1 修复范围、Bug3 开关修复方式、功能2 统计页信息字段、功能3 跳转入口范围）需定案，用户跳过逐项确认，按推荐默认收敛。

**Decision**：

* **Bug1**：GitTags + GitBranches + GitRemotes 三卡片一致加 `max-height` + `overflow-y:auto` 滚动，防同款溢出裁剪 bug。
* **Bug3**：el-switch 设固定 `width`（消除 inline-prompt on/off 文字长度差致宽度跳动）+ `.git-actions` 改 flex + gap 增大间距，移除 switch 的 `margin-left`。
* **功能2**：StatsView header 展示「仓库名 + 路径 + 当前分支 + 远程地址」，分支/远程地址调 `GetGitRemoteURL`（复用 `gitCache`，与 GitInfo 同键同源）。
* **功能3**：仅 git 仓库节点（工作目录 `dir.isGitRepo` / 文件树 `data.isGitRepo`）显示「跳转仓库统计」菜单项，非 git 节点不显示。

**Consequences**：三卡片一致改动稍大但防后续同款 bug；固定 width 视觉恒定但需测开关文字不溢出；统计页增一次 `GetGitRemoteURL` 调用（复用缓存开销可控）；非 git 节点无入口，用户需先选 git 仓库才能跳转统计。
