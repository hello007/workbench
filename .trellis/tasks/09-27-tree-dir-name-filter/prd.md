# 文件树目录内按名筛选（截断优化配套，补大目录文件定位盲区）

## Goal

perf-baseline §15.6 截断优化（单层 2000 项截断）落地后，超限层的 2000 名外文件在前端无入口可定位（哨兵文案引导的 Ctrl+P 命令面板走 SearchFiles，其语义跳过 node_modules——`service/search.go:29` SkipDir——而扁平 node_modules 恰是截断三大触发面之一）。本任务在文件树内提供按名筛选入口，消除盲区并缩短「目录内找文件」动线；附带路线图勘误。

## Decision (ADR-lite)

**Context**: 截断后 2000 名外文件定位存在盲区，候选方案 A1（侵入树数据流）/ A2（覆盖层）/ B（哨兵弹窗）/ C（放开 node_modules）。

**Decision**: **A2 树内筛选 + 覆盖层实现**（用户确认「按推荐来」）。

**Consequences**:
- 筛选激活时 `tree-content` 内 `v-if` 切换为扁平命中列表（覆盖层），不碰 `loadTreeNode`/懒加载/el-tree store，六个哨兵过滤面零风险
- 数据源复用 `GetFileTree` + Go 既有缓存（10k 命中 ~35ms），零后端改动
- 否决 A1：侵入 el-tree lazy store 内部（childNodes 手动替换），refreshNode/展开恢复/哨兵过滤面全要过筛选态分支
- 否决 B：多一次弹窗动线，非截断大目录无入口
- 否决 C：改全局搜索语义，node_modules 海量文件拖慢 Ctrl+P，且 `service/search_test.go:105` 已锁定跳过语义

## Requirements

1. **筛选入口**：`tree-toolbar` 加筛选输入框（el-input small + clearable + 前缀搜索图标），尺寸/间距与现有按钮组一致（frontend-visual-conventions）
2. **筛选作用域**：当前选中目录节点（选中目录用其 path；选中文件或无选中用工作目录根层）；placeholder 显示作用域目录名
3. **匹配口径**：节点 `name` 子串包含、大小写不敏感（纯函数，独立于 SearchFiles fuzzyMatch）
4. **覆盖层交互**：
   - 输入防抖（~300ms）后 `GetFileTree(作用域)` 全量拉取 → 前端过滤 → 渲染扁平命中列表
   - 文件项点击 → `emit('select', node)` 预览，筛选态保持
   - 目录项点击 → 退出筛选 + 回树定位展开该目录
   - 无命中空态（el-empty）；右键菜单不做
5. **渲染上限**：命中列表复用 `truncateTreeNodes`（2000 上限 + 末尾哨兵），防命中过多渲染劣化；覆盖层哨兵弱化样式沿用 `.truncation-hint-node`
6. **退出路径**：清空输入（clearable）/ ESC / 切换工作目录 / refreshNode 前置退出筛选
7. **哨兵联动**：截断哨兵 `onNodeClick` 从纯 return 改为聚焦筛选框（仍不选中、不触发预览）；哨兵文案更新引导树内筛选；`.truncation-hint-node` class 保留（`scripts/frontend-memory-tree.mjs` 依赖）
8. **文档勘误**：
   - `docs/路线图.md:77-78`：「虚拟滚动技术 ✅」改为实况描述（懒加载 + 单层 2000 截断，虚拟滚动经 §15.6 评估否决）
   - perf-baseline §15.6「后续如有需求」句更新（树内筛选已落地）
   - `docs/功能说明.md` 文件树章节补筛选说明；确认 README 是否需更新

## Acceptance Criteria

- [ ] 扁平 node_modules（>2000 项）场景：树内筛选可定位任意文件（含 2000 名外），盲区消除
- [ ] 哨兵点击聚焦筛选框；哨兵过滤面六处行为不回归（不选中/不弹菜单/快捷键过滤/expandAll/collapseAll/locateNode）
- [ ] 筛选纯函数带单测（大小写不敏感、空 query、目录文件混合、超限截断）
- [ ] E2E file-tree.spec.js 全过，新增筛选用例（mock 数据源走 `wails-mock-defaults.js` 单一数据源）
- [ ] vitest 全绿；前端覆盖率 ≥70% 门禁
- [ ] 路线图 + perf-baseline §15.6 勘误落
- [ ] trellis-check + 子 agent 审核通过

## Definition of Done

- vitest + E2E 全绿
- 视觉守 frontend-visual-conventions（工具栏控件尺寸/间距与现有一致）
- 文档勘误全部落地

## Out of Scope

- 后端改动（新递归接口、SearchFiles 语义变更）
- 覆盖层右键菜单
- 递归子树筛选（仅单层作用域）
- 虚拟滚动

## Technical Notes

- 关键文件：
  - `frontend/src/components/FileTreePanel.vue`（工具栏 + tree-content 覆盖层 + 哨兵点击 + 退出路径）
  - `frontend/src/utils/treeTruncate.js`（复用 truncateTreeNodes；哨兵文案更新）
  - 新增 `frontend/src/utils/treeFilter.js`（过滤纯函数）
  - `frontend/src/test/wails-mock-defaults.js`（E2E/vitest mock 大目录数据源）
  - `docs/路线图.md` / `docs/spec/perf-baseline.md` / `docs/功能说明.md`
- 参照（不改）：`service/search.go:29`（node_modules SkipDir 语义）、`service/search_test.go:105`（语义锁定测试）
- 路线图勘误点：`docs/路线图.md:77-78`
- FileTreeNode 字段：id/name/path/type/isGitRepo/hasRemote/hasChildren/isLeaf（model/models.go:46）
- el-tree 结构：`tree-content` 内 el-tree（lazy）+ el-empty 二分支；覆盖层插为第三分支（筛选激活优先）
- 树重建机制：`treeKey = selectedDirectoryId_refreshCounter`，切换目录/刷新全量重建，筛选态随之重置

## Research References

（无需外部研究：实现路径全部由仓库现状推导）
