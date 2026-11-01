# 重链时识别产物目录已随源搬迁

## Goal

用户把源文件连同同名产物容器目录一起搬走后重链时，`output_dir` 仍指旧位置，导致「打开拆分单页」失效且再次 replace 拆分对新位置同名产物报「已存在同名文件」，无自动收敛路径。重链时探测新位置是否存在匹配的产物目录，并按明确语义收敛记录。

## 背景

- `RelocateAssetFile` 只改索引指针，`output_dir` 保持原绝对路径（ADR：产物不随源移动；重算会让打开单页指向不存在目录）。
- 该决策对「源单独搬走、产物留在原处」正确；但用户实际也可能把源与其产物容器目录**一起**搬到新位置——此时新位置已有 `<新源目录>/<新源主干>/` 且内含产物，旧 `output_dir` 失效。
- 现状后果：`OpenAssetSplitPage` 报「拆分产物不存在」；再次 replace 拆分时 `ownsOutDir=false`，新位置同名产物被 `firstReplaceArtifactName` 判为用户自建内容并拒绝，形成死路。

## Requirements

（brainstorm 阶段先给推荐方案再实施）

1. 重链时探测新位置是否存在同名产物容器目录，且内含匹配产物形态的文件。
2. 明确语义选择（二选一，需给出推荐）：
   - A：把 `output_dir` 迁移到新位置（只改记录，不移动文件）；
   - B：拒绝重链并提示走目录移动入口（`MoveAssetFolder` / `RelocateAssetFile` 之外的正规入口）。
3. 迁移须与 `files.path` 同一事务，冲突与幂等口径复用 `rewriteSplitRecordKeyTx` 单点，禁另造一套。
4. 不改表结构；不引入新依赖；不做文件系统移动。

## Acceptance Criteria

- [ ] 新位置产物目录存在且含匹配产物 → 记录 `output_dir` 指向新位置，打开单页可用。
- [ ] 新位置产物目录不存在 → 保持原 `output_dir`（当前行为不变）。
- [ ] 新位置存在同名目录但产物形态不匹配 → 不误迁移。
- [ ] 事务回滚保护（记录写失败时 `files.path` 与 `output_dir` 均不变）。
- [ ] `go test ./...` 全通过；覆盖率门禁不降。

## Technical Approach（初稿，brainstorm 时确认）

- 探测复用产物形态判定单点（`isReplaceArtifactName` / `pageSeqFromArtifactName` / `splitArtifactPath` 家族），禁新写文件名解析。
- 与 `split_records` 键迁移同事务：`relocateFileOK` / `relocateFileProcessing` 现有事务内追加落点更新。

## Out of Scope

- 物理搬运产物目录。
- 前端交互与提示。
- 与本次识别逻辑无关的重链改造。

## Technical Notes

- 依赖子任务 `11-01-split-cleanup-renamed` 完成后的主干解析口径（避免两处各推一套）。
- 参考：`service/assets_relink.go`、`service/assets_db.go`（`rewriteSplitRecordKeyTx`）、`service/assets_split.go`（产物形态判定）。
