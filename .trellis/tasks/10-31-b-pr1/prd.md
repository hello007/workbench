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

## 技术要点

- 标注迁移读写均在 `assets_db.go` 事务方法内，禁跨事务两段写。
- 删源使用 `os.Remove`；失败不删记录、不回滚产物。
- 产物的 `files.id` 由下一次扫描分配（replace 不直接写索引，遵循「拆分不写索引表」现状）；若产品要求拆分完成即刷新素材库，需前端在完成后触发增量扫描（父任务 R7）。
