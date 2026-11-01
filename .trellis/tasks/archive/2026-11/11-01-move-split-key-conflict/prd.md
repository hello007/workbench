# moveFilePath 拆分键冲突消解收口

## Goal

单文件移动（`MoveAssetFiles` / `moveFilePath`）对 `split_records` 键是裸 `UPDATE`、无冲突消解；撞主键时事务回滚，但磁盘 `os.Rename` 已执行，留下索引与磁盘短暂不一致。收口为与重链/目录迁移同源的冲突消解口径。

## 背景

- 目录级迁移（`migrateSplitRecordsTx`）与文件级重链（`rewriteSplitRecordKeyTx`）均有冲突消解：保留 `source_removed=1` 的一条并 Warn。
- `service/assets_folders.go` 的单文件移动分段对 `split_records.source_rel_path` 直接 `UPDATE`，无预检、无落败消解；两张记录撞键时事务回滚，而磁盘文件已改名 → 索引指向旧路径、磁盘已是新路径，需靠后续扫描收敛。
- 键迁移面扩大后（重链/目录迁移都会产生新键占用），该缺口被放大。

## Requirements

1. `moveFilePath` 的拆分键改写改为调用 `rewriteSplitRecordKeyTx` 单点（或在同一事务内复用其冲突四态），禁裸 `UPDATE`。
2. 冲突行为与非冲突行为均保持既有可见语义：非冲突正常迁移；冲突按 `source_removed=1` 优先保留并 Warn。
3. 幂等：同键改写结果相同则零变更。
4. 不改表结构；不引入新依赖。

## Acceptance Criteria

- [ ] 单文件移动后拆分键正确迁移，无冲突路径行为不变（回归用例守护）。
- [ ] 目标键已被 `source_removed=1` 记录占用时，移动不因主键冲突回滚，记录按既有口径收敛并 Warn。
- [ ] 事务回滚用例：记录写失败时 files.path 与 split_records 均不变。
- [ ] `go test ./...` 全通过；覆盖率门禁不降。

## Technical Approach

- `moveFilePath`（service/assets_folders.go:882）内 `UPDATE split_records ...` 裸语句替换为 `s.rewriteSplitRecordKeyTx(tx, r.oldRel, r.newRel, "")`：
  - `newOutDir` 传空串 = 保持原 `output_dir`（单文件移动只改源键，产物目录不随动，与既有口径一致；产物目录随源搬迁由 `RelocateAssetFile` 的 `resolveRelocatedSplitOutDir` 负责，不在本任务范围）。
  - 复用单点的同键零变更、目标键冲突四态（`source_removed=1` 优先保留既有行并 Warn、落败行删除）与旧键不存在零操作。
- 事务边界不变（仍每条目一事务）；磁盘 `os.Rename` 与事务先后关系不变。
- 返回值 `changed` 无需上抛（移动语义不区分是否产生记录变更）。

## Decision (ADR-lite)

**Context**：裸 UPDATE 撞主键时事务回滚，但磁盘 rename 已发生，索引指向旧路径、磁盘已是新路径，只能靠后续扫描收敛；重链与目录迁移都会产生新键占用，放大该缺口。

**Decision**：直接复用 `rewriteSplitRecordKeyTx`（唯一键改写/冲突消解单点），不新写预检或第二套冲突逻辑。

**Consequences**：冲突不再让移动事务回滚，按既有口径收敛并留 Warn；非冲突路径行为与幂等语义不变。代价：冲突时可能删除旧键记录（既有约定的知情取舍，用户重新拆分可重建）。

## Out of Scope

- 磁盘 rename 与事务原子性的整体重设计。
- 批量移动（`MoveAssetFiles`）的其他行为调整。

## Technical Notes

- 参考：`service/assets_db.go`（`rewriteSplitRecordKeyTx`）、`service/assets_folders.go`（单文件移动段）。
