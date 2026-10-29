# 素材管理：文件夹树收起 bug 修复 + 扫描/重建索引并发提效 + 子目录级重建索引

## Goal

三合一任务：
1. **bugfix**：素材管理「文件夹树」子文件夹点击/双击均无法收起（真机失效，单测假绿）
2. **提效**：「扫描」「重建索引」多线程并发提升吞吐（当前逐文件串行解析+哈希）
3. **功能**：子文件夹右键菜单「重建索引」，只重建该子树，避免全库重建的性能代价

## What I already know

### Bug 1（已复现+根因锁定，2026-10-29 真实 EP 复现测试实证）

- **现象**：双击子文件夹 toggle 收起失效；单击语义为选中过滤（`expand-on-click-node=false`）不收起是设计，但用户期望的双击收起通道同样失效 → 感知上「点击、双击都不会收起」
- **根因链**（`frontend/src/components/FolderTree.vue:269-275`）：
  1. `onNodeDblClick` 直赋 `node.expanded = false` → `syncExpandedKeysFromStore()` 收紧 `expandedKeys`（移除该 key）
  2. `expandedKeys` 是 `default-expanded-keys` prop → 真实 el-tree `watch(defaultExpandedKeys)` 触发 `store.setDefaultExpandedKeys(keys)`
  3. `setDefaultExpandedKeys` 对每个 key 调 `node.expand(null, this.autoExpandParent)`，**EP tree props `autoExpandParent` 默认 true**
  4. `expand(null, true)` 的 `done()` 里 `if (expandParent) { while (parent...) parent.expanded = true }` —— **该目录的子孙节点若仍在 keys 中，回放时强制展开祖先链，把刚收起的目录翻回 true**
  5. 自闭环死锁：收起 → keys 收紧（子孙仍在）→ 回放展开祖先 → 收起失效
- **单测假绿**：`FolderTree.spec.js` 的 `ElTreeStub` 未实现「default-expanded-keys 变化 → 展开祖先链」语义（桩注释「已挂载 Node 不响应该 prop 变化」与真实 EP 相反——真实 EP 有 watch 回放），桩下测试全过、真机必挂
- **修复方向（已定）**：`syncExpandedKeysFromStore` 升级为「可达展开面过滤」——只保留「根到该节点路径上全部展开」的 key；收起 X 后 X 的子孙因祖先链断被一并剔除，回放时无 expand 落 X 祖先链 → X 保持收起。全收只剩 `['']`、全展全部保留、reload 回放正确，三条路径（双击/全展/全收）统一收口该单点
- **EP 实测细节**（复现测试取证）：EP Node 经 `reactive()` 包裹直赋响应式生效；el-tree `watch(data, {deep:true})`；`ref(new TreeStore())` 深 reactive（测试 spy 经 `toRaw` 剥壳失效的取证经验，写单测时避坑）

### 需求 2（扫描/重建并发提效）

- 现状：`scanLibrary`（service/assets_scan.go:403）逐文件串行 `processScanFile`：占位 markProcessing → invalidateFileThumbs → `parse`（纯 Go OOXML 解析，CPU/IO 密集）→ `hashFileSHA256`（流式 SHA256，IO 密集）→ 终态落库
- **瓶颈**：解析+哈希占大头，完全串行；万文件级库耗时线性放大
- **约束**：
  - SQLite `modernc.org/sqlite` 纯 Go 单连接池，**并发查写不可行**（单连接池 rows 未 Close 再查=死锁，见 docs/spec/assets-management.md）
  - `RebuildScan` = 备份标注 → 清表 → `scanLibrary`（同链路自动受益）
  - 增量扫描不变式：未变文件跳过（mtime+size 判定）；`processing` 残留自愈；取消检查点逐文件
- **技术方向（pipeline）**：worker pool 并行「parse + hash」（纯磁盘/CPU，零 DB），结果回流主 goroutine 串行落库（markProcessing → invalidateThumbs → 终态写）；进度事件 done 语义从「序号」改「完成计数」；取消即时停止派发新任务

### 需求 3（子目录级重建索引）

- 现有全量重建 `RebuildScan` → `rebuildLibrary`（assets_scan.go:307）：取消在途缩略图 → 备份标注（file_tags/slide_tags/slide_meta 按 path）→ defer 重绑 → VACUUM INTO 整库备份 → `clearIndexForRebuild` 清全部 → 清缩略图目录 → 全量扫描
- 子目录级 = 范围收窄版：**只清 folderPath 前缀的 files 及关联行（slides/FTS/标注级联/缩略图记账+PNG）→ 只扫该子目录**（`collectPptxEntries(root=子目录绝对路径)` 复用，rel 拼前缀还原库内路径）
- 前端：`FolderTree.vue` 节点菜单加「重建索引」项（排除/派生容器禁用；根节点禁用——全库重建走既有入口），宿主 `AssetsPanel.vue` 分派 + toast + 刷新
- 跨层契约：`app_assets.go` 新绑定方法 → `frontend/wailsjs/` 三处手动同步（App.js/App.d.ts/models.ts）；`RPCHiddenMethods` 同步（纯桌面数据面）；复用既有 `E_ASSETS_*` 错误码不新增

## Decision (ADR-lite)

**Context**：三项偏好决策询问用户未应答（2026-10-29），按推荐默认推进，用户可随时推翻。
**Decision**：
1. **单击语义保持现状**：单击=选中过滤、双击=展开/收起切换（本任务只修双击失效 bug，不改交互模型）
2. **并发度固定常量 `min(GOMAXPROCS, 4)`**：零设置面扩张，不加设置项
3. **子目录重建不做自动备份**：范围小可重跑，标注仍按 path+hash 重绑保留
**Consequences**：交互语义零变化（回归风险最低）；大库扫描吞吐约 2-4 倍提升（IO 混合负载下）；子树重建轻量快速，无恢复点（风险=操作中取消半态由 defer 重绑+幂等重扫兜底）。

## Assumptions (temporary)

- 扫描进度事件 done 语义从「顺序序号」改「完成计数」：前端进度条仅展示 done/total 比例与当前文件名，乱序完成不破坏展示（实现时核对前端消费面）

## Open Questions

（已收敛，见 Decision）

## Requirements (evolving)

- R1 双击子文件夹目录名可正常收起/展开（真实 EP 链路）；「全部收起」「全部展开」行为不回归；树数据局部 reload 后展开面不漂移（既有契约）
- R2 单元测试必须用**真实 el-tree**（禁桩 reproducing 假绿）；保留既有桩测试的展示/上报契约面
- R3 扫描/重建索引对需要重处理的文件并发解析+哈希，落库仍串行单点；增量跳过/取消/进度事件/lost 标记/excluded 标注语义全部保持
- R4 子目录右键「重建索引」：只清+只扫该子树；标注按 path 重绑同全量版；完成后刷新树与列表
- R5 跨层契约同步：wailsjs 三处 + RPCHiddenMethods + 事件面（assets-scan:* / assets-rebuild:* 前缀确认）

## Acceptance Criteria (evolving)

- [ ] 真实 el-tree 测试：双击收起后子层级 DOM 隐藏、expandedKeys 不含该节点；再双击展开恢复
- [ ] 真实 el-tree 测试：收起中间层目录后其展开中的子孙节点从可达展开面剔除（防回放翻回）
- [ ] 全收/全展/reload 漂移守护测试不回归
- [ ] 扫描并发化后：`go test ./service/` 全绿（含取消/增量/lost/excluded/soak 各档）
- [ ] 子目录重建：仅该子树 files/slides/FTS/缩略图被清+重建；子树外记录零影响；标注 path 重绑生效
- [ ] 子目录重建在扫描在途时正确互斥（tryLockScan）
- [ ] 前端菜单禁用面（根/排除/派生）与后端守卫双防线
- [ ] wailsjs 三处同步；`npm run build` 与 vitest 全绿

## Definition of Done

- 后端 model/server ≥80% + service ≥76% 门禁；前端 ≥70% 硬失败
- `go vet` / `npm run build` / vitest / E2E 相关面全绿
- README.md 与 docs/测试策略.md（若门禁或流程变化）确认是否需更新
- spec 沉淀：FolderTree「真实 EP default-expanded-keys 回放展开祖先链」钩子规则 + 单测禁桩假绿教训追加

## Out of Scope (explicit)

- 单击节点切换展开/收起（保持选中过滤语义）
- 扫描多库并行（单库内并发已足）
- 子目录级「扫描」（只做重建；普通扫描本就增量高效）
- 设置项化并发度（除非用户要求）
- el-tree 升级/替换

## Technical Notes

- 复现取证：真实 EP 复现 spec 已删除（定案后正式测试落 FolderTree.spec.js 真实树用例）
- 关键源码位置：FolderTree.vue:226-275（expandedKeys watch/sync/双击）、tree-store.mjs setDefaultExpandedKeys、node.mjs expand(done 祖先链)、tree.vue watch(defaultExpandedKeys/data deep)、tree-node.vue setup expanded ref
- 后端：assets_scan.go（scanLibrary/processScanFile/collectPptxEntries/rebuildLibrary）、assets_folders.go（resolveLibraryRelPath/tryLockScan 模式）、assets_db.go（store 层）、app_assets.go（绑定层）
- EP 版本 2.14.5（locked）
