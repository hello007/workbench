# 还原备份前版本预检（P0 数据安全）

## Goal

`RestoreAssetLibrary` 覆盖当前索引前只做 `integrity_check`，不校验备份库 `user_version`。版本过新（如 v10 > 当前 v9）的备份会先 `os.Rename` 覆盖 `<id>.db`，随后 `withStore` 打开时 `migrate` 报「版本过新」——备份文件已被消费、原库已丢失，**双失且不可逆**。审核已实测复现（marker 行消失、`user_version=10`、备份文件不存在）。

## Requirements

1. 覆盖前读取备份库 `PRAGMA user_version`（只读打开，不触发迁移）；`> assetsSchemaVersion` 时拒绝还原，**当前索引与备份文件均保持原样**。
2. 版本预检与 `integrity_check` 同阶段（rename 之前），任一失败零改动。
3. 错误文案明确：「备份库版本 v%d 高于当前支持的 v%d，请升级 WorkBench 后再还原（当前索引未改动）」。
4. 修正 `:299` 附近「完整性复验未通过」文案在版本问题下的误导性（版本错误应在预检阶段拦下，复验阶段只报完整性）。
5. 测试：版本过新备份拒绝且原库/备份均保留；正常版本备份还原成功；损坏备份仍按完整性拒绝。

## Acceptance Criteria

* [ ] 构造 v10 备份（写 `PRAGMA user_version=10` 后 `VACUUM INTO`）：还原返回版本错误，原库记录仍可读、备份文件仍存在。
* [ ] 正常 v9 备份还原成功，数据与新列（`artifact_stem`）保留。
* [ ] `go test ./...` 通过，覆盖率门禁不降。

## Technical Approach

* `assets_backup.go` 新增 `verifySQLiteVersion(path string) error`：`sql.Open("sqlite", "file:<path>?mode=ro")` 读 `PRAGMA user_version`；`> assetsSchemaVersion` 返回含版本号的错误；打开/查询失败按「无法校验」拒绝（安全侧）。
* `RestoreAssetLibrary` 在 `verifySQLiteIntegrity` 之后、`CancelScan` 之前调用。
* 复用 `modernc.org/sqlite` 驱动；只读模式避免对备份文件产生写副作用。

## Out of Scope

* 备份库版本过旧（< 当前）不拒绝——`migrate` 会逐级升级，保持既有行为。
