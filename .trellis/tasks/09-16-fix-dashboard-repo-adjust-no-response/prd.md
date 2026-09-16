# 修复状态看板调整仓库无反应

## Goal

状态看板点「跳转」或点击行跳转对应仓库时无反应。让点击正确切到该仓库并展示文件树定位节点。

## What I already know

- 看板组件 `DashboardView.vue`：行点击 `onRowClick` 与「跳转」按钮均 `emit('locate', row.path)`，失效仓库拦截警告。
- `Home.vue` 接 `<DashboardView @locate="onRepoLocate" />`。
- `onRepoLocate`（Home.vue:357）：规范化路径找 `targetDir` → 关 `repoFilterVisible` 弹窗 → 跨工作目录 `await onDirectorySelect` → `fileTreePanelRef.locateNode(repoPath)`。
- **根因**：`onRepoLocate` 全程不切 `uiStore.activePanel`。看板与三栏 `.main-panes` 同级 v-show 互斥：
  - `.main-panes` 仅 `activePanel==='directory'||'toolbox'` 显示（Home.vue:10）
  - 看板 `v-show="activePanel==='dashboard'"`（Home.vue:84）
  - 看板点跳转后 `activePanel` 仍 `'dashboard'` → 文件树所在三栏 `display:none` → locateNode 内部对隐藏 el-tree setCurrentKey/scrollBy 不可见 → 用户视觉「无反应」。
- 仓库筛选器路径（RepoFilterDialog）同走 `onRepoLocate` 但不卡，因筛选器是弹窗，打开时 `activePanel` 本就是 `directory`/`toolbox`，关弹窗后三栏本就可见。

## Assumptions (temporary)

- 预期行为：看板点行/「跳转」后应切到工作目录面板（`activePanel='directory'`）并定位到该仓库节点，与仓库筛选器跳转体感一致。
- 跨工作目录切换 + locateNode 时序已由 `onRepoLocate` 既有逻辑（treeReadyPromise 兜底）处理，无需改。

## Open Questions

- （已解决，见 Requirements）跳转后落点面板：固定切到 `directory`（三栏文件树），与仓库筛选器跳转一致。
- （已解决，见 Requirements）嵌套工作目录歧义：最长前缀匹配取最具体工作目录。
- （已解决，见 Requirements）工作目录移除后看板标记：前端前缀匹配判定 `orphaned`，零后端改动。

## Requirements (evolving)

- 看板行点击 / 「跳转」按钮触发后，切到工作目录面板（`activePanel='directory'`）并展开文件树定位目标仓库节点。
- 同工作目录与跨工作目录两种情形均生效。
- 失效仓库（missing）维持现状拦截，不进入跳转。
- **嵌套工作目录歧义**：仓库路径可能同时归属多个工作目录（如 `D:\projects` 与 `D:\projects\sub` 均已添加，仓库 `D:\projects\sub\repo`）。跳转时遍历所有前缀匹配的工作目录，取路径最长者（最具体工作目录）作为落点，避免 `find` 取第一个匹配选中外层导致 locateNode 在错误树里定位。
- pin 仍存纯路径不绑工作目录 ID（工作目录 ID 导入导出会重新生成，持久化不可靠；最长前缀匹配在跳转时动态推断即可）。
- **孤立条目标记**：pin 的仓库路径若不再属于任何已注册工作目录（工作目录被移除或路径被改），看板行标记「工作目录已移除」（orphaned），与 `missing`（路径目录被删）区分；展示优先级 missing > orphaned。复用现有「取消关注」按钮供用户移除，不加批量清理。

## Acceptance Criteria (evolving)

- [ ] 看板点击正常仓库行后，activePanel 切到 `directory`，文件树可见并定位高亮目标节点。
- [ ] 跨工作目录跳转：先切目录再定位，时序不竞争（复用既有 treeReadyPromise 兜底）。
- [ ] 嵌套工作目录（`D:\projects` 与 `D:\projects\sub` 并存，仓库在 `D:\projects\sub\repo`）：跳转选中 `D:\projects\sub`（最长前缀匹配），而非 `D:\projects`。
- [ ] 失效仓库点击仍提示「仓库路径已失效，无法跳转」不跳转。
- [ ] 仓库筛选器跳转路径不受回归影响（同样走改造后的 onRepoLocate）。
- [ ] pin 仓库路径不再属于任何已注册工作目录时，看板行标记「工作目录已移除」（orphaned），路径仍在文件系统时状态（dirty/ahead/behind）仍正常计算。
- [ ] 同时 missing + orphaned 时展示 missing（优先级 missing > orphaned）。
- [ ] 工作目录增删后看板 orphaned 标记自动重算（computed 响应 directories ref，无需手动刷新）。

## Definition of Done (team quality bar)

- 改动最小，不破坏既有 onRepoLocate 时序注释与跨目录逻辑。
- 前端 vitest 相关 mock 场景如有覆盖则补，否则手动验证。
- README/功能说明无需更新（行为对齐已文档化的「点击行跳转该仓」）。

## Out of Scope (explicit)

- 不改看板状态计算、pin 持久化、ahead/behind 语义。
- 不加新面板或新跳转入口。
- 不重构 onRepoLocate（仅补 activePanel 切换 + 最长前缀匹配）。
- 不加「批量清理孤立条目」按钮（单条「取消关注」够，批量留后续）。
- 不在后端 RepoStatus 加 orphaned 字段（前端纯计算，避免同步 wailsjs 绑定三处）。

## Technical Notes

- 关键文件：
  - `frontend/src/views/Home.vue:357` `onRepoLocate`（补 `uiStore.activePanel = 'directory'` + 最长前缀匹配）
  - `frontend/src/views/DashboardView.vue:157` `onRowClick` / 「跳转」按钮（不改，emit locate 不变）
- 参考时序：`.trellis/tasks/09-15-global-repo-status-dashboard` 既有 `cross-workdir-locate` 范式。
- 修复点：
  1. `onRepoLocate` 进入即设 `uiStore.activePanel = 'directory'`，放关闭弹窗之后、切换目录之前，保证三栏先可见再 locateNode。
  2. 工作目录查找从 `find(startsWith)` 改最长前缀匹配：`directories.filter(d => normTarget.startsWith(norm(d.path)))` 取 `d.path` 最长者；空匹配维持「未找到该仓库所属的工作目录」警告。
  3. 前缀匹配逻辑提取为共用工具（`frontend/src/utils/pathMatch.js` 或 store getter），`onRepoLocate` 与 DashboardView orphaned 判定共用，避免两处重复实现。
- 孤立条目标记实现：
  - `DashboardView` 用 `computed` 派生 `enrichedStatuses`：`statuses.value.map(r => ({...r, orphaned: r.missing ? false : !belongsToAnyDir(r.path)}))`，`belongsToAnyDir` 用共用前缀匹配判断。模板表格 `:data="enrichedStatuses"` 替代 `statuses`。
  - computed 自动响应 `statuses` 与 `directoryStore.directories` 两个 ref 变化，覆盖启动竞态（directories 后就绪自动重算）与工作目录增删后标记更新，无需 watch 副作用。
  - orphaned 是前端派生字段，不入后端 RepoStatus，不动 wailsjs 绑定。
  - 模板：orphaned 行展示 `el-tag type="info" size="small"`「工作目录已移除」；missing 优先（rowClass 仍按 missing 灰显，orphaned 且非 missing 不灰显，状态列正常展示）。
- 嵌套工作目录存在性依据：`service/directory.go:62` `Create` 仅去重完全相同路径，不拦截嵌套。
- pin 不绑 dirId 依据：工作目录 ID 导入导出重新生成（功能说明「另存为新项重新生成 ID」），持久化不可靠；路径主键已 `filepath.Abs` 规范化去重。
