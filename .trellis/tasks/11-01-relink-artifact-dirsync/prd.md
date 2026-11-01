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

## Technical Approach

- **语义采用方案 A**：重链时若探测到产物目录已随源搬迁，只把记录 `output_dir` 改指新位置，不做物理移动。
- 候选目录 = `<newAbs 父目录>/<新源名主干>`（与 `splitOutputDir` 同布局）；仅 `target ∈ {source, replace}` 参与（cache 产物位于应用数据目录，用户不会搬动，纳入判定只会放大误判面）。
- 命中判定：候选目录存在、为目录，且 `findSplitArtifactByPage(候选目录, record.target, 1)` 命中（复用产物形态单点，与「增量判定首页存在」同口径）；否则保持原 `output_dir`（现有行为不变）。
- 候选目录与原 `output_dir` 等价（`samePathFold`）时不产生变更。
- 落库复用 `rewriteSplitRecordKeyTx` 的 `newOutDir` 形参：`relocateFileOK` / `relocateFileProcessing` 增加入参并透传，`files.path` 与 `output_dir` 同一事务；`dropSplitRecord` 分支（force 内容已变）不涉及落点。
- 拆分记录读取失败按系统性错误返回（与既有 db 错误口径一致），不静默降级。

## Decision (ADR-lite)

**Context**：`RelocateAssetFile` 只改索引指针、`output_dir` 冻结原绝对路径——该决策对「源单独搬走」正确，但用户把源与其同名产物目录一起搬走时旧落点失效，打开单页报错、再次 replace 拆分被「同形文件」拒绝，形成死路。

**Decision**：选择方案 A（迁移记录落点）。用户已在资源管理器完成搬运，方案 B 的「走目录移动入口」无法补救既成事实，只会把用户堵在死路。

**Consequences**：重链后打开单页/导出/增量判定自动恢复；只改写记录，磁盘零操作，幂等（重复重链再次探测到等价落点即无变更）。风险面：新位置恰好存在同名且含同页序产物的目录时会误迁移——以「仅 source/replace」+「页序反查唯一命中」双重收敛；搬走产物目录但未搬走首页产物的场景不迁移（安全侧失败，用户可重新拆分）。

## Out of Scope

- 物理搬运产物目录。
- 前端交互与提示。
- 与本次识别逻辑无关的重链改造。

## Technical Notes

- 依赖子任务 `11-01-split-cleanup-renamed` 完成后的主干解析口径（避免两处各推一套）。
- 参考：`service/assets_relink.go`、`service/assets_db.go`（`rewriteSplitRecordKeyTx`）、`service/assets_split.go`（产物形态判定）。
