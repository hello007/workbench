# B 方案 PR1：后端就地替换落点 + 删源 + 标注迁移

父任务：`10-31-b`（方案与决策见父任务 prd.md）

## 范围

1. 落点三态：`cache` / `source` / `replace`，落点解析单点收敛；App 绑定参数变更并同步 `frontend/wailsjs` 三处。
2. replace 落点产物命名：`<3位页序>-<源名主干>.pptx`，不命中 `isPptxSplitDerivative`；产物目录 `<源目录>/<源名主干>/`。
3. 删源：全页成功 + 逐页产物 Stat 校验通过才删；任一失败/取消 → 保留源并回滚已产出页。
4. 标注迁移：文件级标签/星级/备注复制到每个产物；页级标注 `(file_id, k)` → 产物 `(newFileID, 1)`；事务内完成，失败不删源。
5. 源记录处理：迁移成功后删除源 `files` 行（级联清 slides/标注/FTS/记账），不走 lost。
6. `split_records` 标记落点与 `source-removed`。
7. 新增错误码 `E_ASSETS_SPLIT_SOURCE_DELETE_FAILED`（`app_error.go` + `error.js` ErrorCode/WARNING_CODES 三处同步）。

## 约束

- 前两落点行为零变化（含取消保留已产出、增量跳过、重拆清旧页）。
- 删除源前必须完成标注迁移；迁移失败 → 中止并回滚已迁移数据。
- 单库拆分互斥与取消链路沿用既有 `tryLockSplit` / `splitCancels`。

## 验收

- [ ] 三落点回归：cache/source 行为与改动前一致。
- [ ] replace 成功：源消失、产物按页生成、每页成为独立素材（跑一次扫描后 `ListAssetFiles` 可见且页数均为 1）。
- [ ] 某页失败：源保留、已产出页回滚删除、错误明细含失败页。
- [ ] 取消：源保留、已产出页回滚。
- [ ] 源被占用导致删除失败：返回 `E_ASSETS_SPLIT_SOURCE_DELETE_FAILED`，源保留、产物保留（已成功拆分，仅删源失败，可重试删源）。
- [ ] 标注迁移：源文件标签/星级/备注出现在每个产物；第 k 页标签出现在第 k 个产物的第 1 页。
- [ ] 覆盖率门禁通过；`go test ./...` 全绿。

## 评审决策（2026-10-31 确认）

- **D1 源记录处理**：标注迁移成功后**直接删除源 `files` 行**（级联清 slides/标注/FTS/缩略图记账），不走 `markFileLost`。理由：标注已完整迁移到产物，lost 行对用户只是无法清理的僵尸条目；`split_records` 已留痕。
- **D2 删源方式**：**移入系统回收站**，非永久删除（用户可恢复）。Windows 经 `SHFileOperationW` + `FOF_ALLOWUNDO`；Linux 按 freedesktop Trash 规范移入 `$XDG_DATA_HOME/Trash/files`。新增 `util.MoveToRecycleBin`（`_windows`/`_other` 双文件），service 层经注入点隔离（单测用 fake）。
- **D3 索引写入时序（偏离原「不写索引表」表述）**：R4 要求把标注迁移到产物 `(newFileID, 1)`，而 newFileID 只有索引行存在才可得。故 replace 落点在产物写盘 + Stat 校验通过后，**复用 `processScanFile` 单文件重链**为每页产物建索引行，再在同一事务内迁移标注。因此 replace 落点需与扫描互斥：先 `tryLockScan`（失败即 `E_ASSETS_SCAN_IN_PROGRESS`）再 `tryLockSplit`，锁序与 `DeleteLibrary` 一致（scan → split）。cache/source 落点仍只取 split 锁，行为不变。
- **D4 删源失败口径**：全页成功 + 逐页 Stat 校验 + 标注迁移事务均通过后，才执行回收站移入。移入失败 → 源保留、产物与索引保留、源标注保留，返回 `E_ASSETS_SPLIT_SOURCE_DELETE_FAILED`，用户可重试拆分（重拆会覆盖产物）。此前的任一环节失败 → 保留源 + 回滚删除已产出页及其索引行。

## 技术要点

- 标注迁移读写均在 `assets_db.go` 事务方法内，禁跨事务两段写。
- 删源使用 `util.MoveToRecycleBin`（D2，移入回收站）；移入失败不删记录、不回滚产物。
- 产物的 `files.id` 在拆分链路内即时分配（D3，复用 `processScanFile` 单文件重链）；前端完成后仍需 `refreshDataAfterMutation` 重拉（父任务 R7），无需额外全库扫描。
