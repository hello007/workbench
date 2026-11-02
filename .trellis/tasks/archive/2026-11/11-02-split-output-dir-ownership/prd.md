# 拆分落点目录归属校验（P0 数据正确性）

## Goal

`splitOneFileWithHash` 的 source/cache 落点由当前文件名直接推导后**无条件** `removeSplitArtifacts(outDir, baseName)` / `os.RemoveAll(outDir)`，不校验该目录是否已被 `split_records` 中**另一条记录**占用。可达场景：`A.pptx` 拆分后改名为 `B.pptx`（重链只改记录键，`output_dir` 仍为 `A` 目录、`artifact_stem` 仍为 `A`），随后放入新的 `A.pptx` 拆分——新 A 的落点是同一 `A` 目录，直接删除并覆盖 B 的产物。同时 `splitArtifactPath` 的页序反查只按页序匹配、不校验主干归属，B 的增量判定/打开单页会命中 A 的产物。

## Requirements

1. 拆分前（source/cache 落点）检查 `split_records` 是否存在**其他** `source_rel_path`（不等于当前源）且 `output_dir` 与本次落点等价（`samePathFold`）的记录；命中则拒绝拆分，返回可读错误，禁止删除/覆盖该目录。
2. 新增错误码（若现有错误码无合适语义）：`E_ASSETS_SPLIT_OUTPUT_CONFLICT`，三处同步（`model/app_error.go`、前端 `error.js` 的 `ErrorCode` 与 `WARNING_CODES`、错误码文档）；错误语义为 warning（用户可改名/移动源后重试）。
3. `splitArtifactPath` 的页序反查命中后校验主干：解析候选主干，须属于 `splitArtifactStemCandidates(record, record.outputDir)`（精确或 `EqualFold`）；不符则不返回命中（回退直拼路径，供报错指向）。
4. 目录级归属查询单点：`assetsStore.splitRecordByOutputDir(outputDir, exceptRel string) (*assetSplitRecord, error)`，供拆分入口与后续任务复用。
5. 测试：
   - 归属冲突：A 拆分成产 → 改名/重链为 B → 新建 A.pptx → 拆分拒绝，B 产物与索引行原样保留。
   - 归属校验放行：正常重复拆分同一源（记录 `source_rel_path` 相同）不受影响。
   - `splitArtifactPath` 反查主干不符 → 不命中。
6. 文档：`docs/spec/pptx-split.md §9.5`、`CLAUDE.md` 相关行同步。

## Acceptance Criteria

* [ ] 冲突场景拆分被拒，错误码与文案可读，B 的产物/索引行零改动。
* [ ] 同源重拆、改名重链后重拆（记录键同名）不受影响。
* [ ] 反查主干校验有用例。
* [ ] `go test ./...` 通过，覆盖率不降；前端既有错误码测试（如有）同步。

## Technical Approach

* 新增 `assetsStore.splitRecordByOutputDir`：`SELECT ... FROM split_records WHERE source_rel_path <> ?` 后 Go 侧 `samePathFold` 比较（`output_dir` 为绝对路径，SQL 无法跨平台折叠）。
* `splitOneFileWithHash` 在 `outDir` 计算后、任何删除动作前调用；命中返回 `model.NewAppError(ErrCodeAssetsSplitOutputConflict, ...)`。
* `splitArtifactPath` 反查前构造候选集合，命中后 `parseSplitArtifactName` + `splitStemInCandidates`。

## Out of Scope

* replace 落点已有 `firstReplaceArtifactName` 兜底，不改其语义（除错误文案一致性）。
* 不引入 `output_dir` UNIQUE 约束（历史数据可能冲突，迁移风险高）。
