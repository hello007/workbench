# replace 归属复核与备份预检加固（P2）

## Goal

三处数据正确性缺口：

1. **replace 链路无落点归属校验**：`518ef7f` 只在 source/cache 接入 `splitRecordByOutputDir`；replace 仍靠 `ownsOutDir`（记录 target=replace）+ `firstReplaceArtifactName` 文件名启发式。旧记录指向该目录但产物已缺失时，同目录新同名源（或改名后的另一条记录）的 replace 产物会被旧记录认领，旧记录再次 replace 时按旧 `pageCount` 删除新产物与索引行（spec「无文件可覆盖、无数据丢失」结论不完整）。
2. **备份版本预检不校验 schema**：`verifySQLiteVersion` 只看 `user_version`。实测任意合法空 SQLite（`user_version=0`）可通过完整性 + 版本检查，还原后索引被清空为 0 文件。
3. **DSN 未转义**：`file:<path>?mode=ro` 直接拼接，路径含 `?`/`%`/`#` 时可能打开/校验到另一个文件，预检形同虚设（`verifySQLiteIntegrity` 同款）。

## Requirements

1. **replace 归属复核**：`splitOneFileReplace` 在写入/覆盖任何文件前调用 `splitRecordByOutputDir(outDir, rel)`；命中其他记录即拒绝（复用 `E_ASSETS_SPLIT_OUTPUT_CONFLICT`）。注意 `ownsOutDir` 分支语义保持（本记录拥有目录时放行）。
2. **备份 schema 预检**：`verifySQLiteVersion` 扩展为 `verifyBackupDatabase(path)`：
   - 只读打开，读 `PRAGMA user_version` 判版本；
   - 校验核心表存在（`sqlite_master` 中至少含 `files` 与 `split_records`），缺失 → 拒绝「不是有效的素材库备份（缺少核心表）」；
   - 版本 > 当前 → 拒绝（既有）。
3. **DSN 转义单点** `sqliteReadOnlyDSN(path string) string`：对路径做最小百分号编码（`%`→`%25`、`?`→`%3F`、`#`→`%23`），`verifySQLiteIntegrity` 与 `verifyBackupDatabase` 共用。
4. 测试：
   - replace 归属冲突：目录被另一记录占用且本次记录不拥有 → 拒绝，目录零改动；
   - 空 SQLite（`user_version=0`、无核心表）备份 → 拒绝且当前库不动；
   - 正常备份（含核心表）放行；
   - DSN 辅助：含 `?`/`%`/`#` 的路径编码正确，且能读到真实文件。
5. 文档：`docs/spec/pptx-split.md §9.5`、`docs/spec/assets-management.md §12.3`、错误码文档。

## Acceptance Criteria

* [ ] replace 冲突场景被拒，产物/索引行零改动。
* [ ] 空 SQLite / 无核心表备份被拒，当前索引未被清空。
* [ ] 含特殊字符路径的备份校验正常（单测）。
* [ ] 正常 replace、正常还原全绿；`go test ./...` 与覆盖率门禁通过。

## Out of Scope

* 不为 `output_dir` 加 UNIQUE 约束（历史数据可能冲突）。
* 不在临时副本上跑完整 migrate 校验（成本高，核心表 + 版本已覆盖主要风险）。
