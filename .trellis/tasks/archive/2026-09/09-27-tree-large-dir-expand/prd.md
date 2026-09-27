# 文件树大目录展开优化（perf-baseline §15.4 量化目标落地）

## Goal

单目录万级文件（日志目录、数据导出目录、扁平 node_modules）展开时前端渲染卡死级劣化
（10k 节点 ~4.4s / +311MB；100k 节点 ~49.2s / +3.14GB，可致 WebView2 OOM）。
本任务选定并实施优化方案，达成量化目标，回填 perf-baseline.md before/after 对比表，
勾选路线图「限制文件树节点数量」子项。

## What I already know（代码摸底事实）

### 量化基线（perf-baseline §15.2，GC 口径，勿重复测量）

| 单层节点量 | Go GetChildren | 前端渲染耗时 | JSHeap 增量 | JSHeap 驻留 |
|---|---|---|---|---|
| 1k | 1.31 ms | ~346 ms | ~32 MB | ~44 MB |
| 10k | 35.2 ms | ~4.36 s | ~311 MB | ~326 MB |
| 100k | 306 ms | ~49.2 s | ~3.14 GB | ~3.15 GB |

瓶颈定性在前端渲染（el-tree 无虚拟滚动，展开即全量渲染），Go 侧非瓶颈。

### 现状契约面（FileTreePanel.vue，1663 行）

- `el-tree` + `lazy` + `:load="loadTreeNode"` + `node-key="path"`（FileTreePanel.vue:13-28）
- `loadTreeNode`（:543）：`GetFileTree(path)` 单层拉取 → map `isLeaf` → resolve；
  根节点加载完成触发 `treeReadyResolve`
- `refreshNode`（:644）深度依赖 el-tree 内部结构：
  `fileTreeRef.value.store.nodesMap`、`store.root`、`target.loaded/loading/expand()`、
  `InvalidateFileTreeCache` + loadData 重建 + 子树展开态恢复（getExpandedPathsOf /
  restoreExpandedPaths / waitForNodeLoaded 轮询 node.loaded）
- `expandAll`（:718）递归 `node.childNodes`；`collapseAll`（:746）遍历 `store.nodesMap`
- 展开状态恢复：`useTreeState`（localStorage，MAX_EXPANDED_PATHS=200）+ restoreTreeState
  深度分组逐层 expand
- 事件：`node-click` / `node-contextmenu` / `node-expand` / `node-collapse`
- E2E `frontend/e2e/file-tree.spec.js` 依赖 `.el-tree-node` 选择器（hasText 定位）
- 测量脚本 `scripts/frontend-memory-tree.mjs` waitForFunction 判定 `.el-tree-node` 计数达标
- 组件单测 `frontend/src/components/__tests__/FileTreePanel.spec.js` 已存在

### 路线 b 关键约束（决定性）

- el-tree-v2 在本代码库零使用（grep 全 0）
- element-plus 2.13.7
- **el-tree-v2 不支持懒加载（lazy/load 属性不存在）**——待 research 确认并沉淀
  research/el-tree-v2-capability.md。若确认成立，路线 b 须改全量拉取模式：
  Go 侧新增递归全量接口（100k 全树 JSON 传输 + 前端一次性构建 data）或前端
  expand-hack 异步填充 children，两者均显著劣化首屏且迁移面大。

## Assumptions (temporary)

- el-tree-v2 无懒加载支持（research 验证中）
- file-tree.spec.js 现有用例目录规模 <2k，截断方案不触发、不影响现有断言

## Decision (ADR-lite)

**Context**：两路线权衡。Research 确认决定性约束——el-tree-v2（EP 2.13.7/2.14.5）
不支持懒加载（20 项属性无 lazy/load），迁移须走 expand-hack（node-expand 异步填
children 后替换 data 顶层引用，非官方支持路径），且 refreshNode 依赖的
store.nodesMap/loadData 在 v2 全不存在须重写，改动面 3-5 倍于路线 a。
AskUserQuestion 未获答复，按任务原文授权（「按工作量与风险给推荐」）采用推荐项。

**Decision**：
1. **路线 a 轻量版：截断 + 哨兵提示节点**，不做新建筛选 UI——调研确认
   `SearchFiles`（service/search.go，按文件名模糊 + 评分排序）+ CommandPalette
   （Ctrl+P）已构成文件名搜索闭环，哨兵文案引导即可。
2. **阈值硬编码常量 2000**（`utils/treeTruncate.js` export）。理由：纯前端渲染
   性能参数，99% 用户不动；settings 化成本 = model.AppSettings 加字段 + wailsjs
   绑定三处同步 + SettingsPanel UI + 持久化测试（跨层契约），而调大=自找卡死、
   调小无必要，收益不抵；后续 settings 化增量容易（常量改读 store 一处）。
3. **量化目标（严格档）**：10k/100k 档展开首屏 <1s / JSHeap 增量 <100MB /
   驻留 <150MB。依据：2k 截断与规模解耦，10k 档 4.4s 线性推算 2k ≈ 0.9s /
   +62MB，可达成且留余量。

**Consequences**：
- 契约面零变化（refreshNode/useTreeState/E2E 现有断言不动）
- OOM 结构性防死（渲染量与目录规模解耦）
- >2000 名单外文件不可逐个浏览；SearchFiles 跳过 node_modules 的语义使
  「扁平 node_modules 找包内文件」无前端入口（可接受：此类操作惯用终端）
- 未来 el-tree-v2 迁移可在此之上叠加不冲突（截断阈值调大 + 虚拟化正交）

## Research References

- [`research/el-tree-v2-capability.md`](research/el-tree-v2-capability.md) —
  el-tree-v2 无 lazy/load（官方 tarball 双验证）；expand-hack 模式与 API 差异清单；
  DOM 保留 .el-tree-node 但扁平无嵌套容器

## Requirements (evolving)

- 单层大目录展开防卡死/OOM（量化目标见 Open Questions 3）
- 保持懒加载契约（node-key=path、refreshNode 局部刷新+子树展开态恢复、右键菜单、E2E 全过）
- after 同口径复测（frontend-memory-tree.mjs 10k/100k 档）+ perf-baseline.md 回填
- 路线图「限制文件树节点数量」子项勾选 + 量化收益标注

## Acceptance Criteria (evolving)

- [ ] 10k/100k 档渲染耗时与 JSHeap 达到 brainstorm 定档目标
- [ ] E2E（file-tree.spec.js）+ vitest 全绿（前端覆盖率 ≥70% 门禁）
- [ ] 新增逻辑（截断/提示条/虚拟列表适配）带单测
- [ ] perf-baseline.md before/after 对比表（§15.2 同格式）
- [ ] 路线图子项勾选 + 量化收益标注

## Definition of Done (team quality bar)

- 单测/E2E 全绿，覆盖率门禁过
- perf-baseline.md、路线图、README.md（如需）更新
- trellis-check + 子 agent 审核过

## Out of Scope (explicit)

- Go 侧 GetChildren 性能优化（已证非瓶颈）
- 文件树全局搜索/筛选体系重建（仅按所选方案配套最小能力）
- 大文件分块读取（§15.3 已定性不动）

## Technical Notes

- 测量脚本复用：`node scripts/frontend-memory-tree.mjs [--runs 3] [--scales 1000,10000,100000]`
- Go 侧 bench：`go test -bench=BenchmarkFileTreeGetChildren_Scale -benchmem ./service/`
- 契约面文档：docs/spec/cross-layer-contracts.md（本任务若不改 App/model 契约则不涉及）
