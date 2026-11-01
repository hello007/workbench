# 素材文件重链同步迁移 split_records

## Goal

`RelocateAssetFile`（手动重链）当前只改 `files.path`，不改 `split_records` 的 `source_rel_path` 键。丢失源文件被重链到新路径后，`GetAssetSplitRecord(新路径)` 查不到记录，「打开拆分单页」入口消失；用户再次拆分时以新键另写一条记录，旧记录永久滞留。本任务把文件级重链纳入拆分记录迁移面，与目录级迁移（`replaceFilePaths` + `migrateSplitRecordsTx`，提交 798fc2f / 9e74263）同口径收口。

## What I already know

- `split_records` 以 `source_rel_path` 为唯一键，`output_dir` 存绝对路径（`service/assets_db.go:2233` 起）。
- 目录级迁移已收口：`migrateSplitRecordsTx`（`service/assets_db.go:2321`）在同事务内迁移源键与 `output_dir`，冲突策略为「保留 `source_removed=1` 的一条 + Warn，落败行删除，迁移不中断」。
- 文件级重链落库单点：`relocateFileOK`（哈希匹配，`assets_db.go:530`）与 `relocateFileProcessing`（需重解析，`assets_db.go:552`），当前均为单语句 `Exec`，无事务。
- 重链语义 = 只把索引记录重指向库内现存文件，**不做任何文件系统写**（越界校验后仅 `os.Stat`）。
- source/replace 落点公式：`<库根>/<源所在目录>/<源主干>`（`splitOutputDir` / `splitOneFileReplace` 的 `filepath.Join(filepath.Dir(absPath), base)`）；cache 落点 = `data/assets/<库>/derived/...`（库根外）。
- `OpenAssetSplitPage` / 导出 / `isReplaceArtifactOutput` 均以 `split_records.output_dir` 为产物定位权威。

## Requirements (evolving)

1. `RelocateAssetFile` 在更新 `files.path` 的**同一事务**内，把该文件对应的 `split_records` 行键从旧路径改写为新路径。
2. `output_dir` 处理（决策见 ADR-lite）：target=cache/source/replace 一律**保持原绝对路径**，文件级重链只改写 `source_rel_path` 键。
3. 冲突策略：新键已存在时沿用 `migrateSplitRecordsTx` 现有口径（`source_removed=1` 保留 + Warn，禁另造一套）。
4. 幂等：重复重链零变更；旧键不存在时零操作不报错。
5. 不改 `split_records` 表结构；不引入新依赖。

## Open Questions

（无阻塞项。Q1/Q2 已按推荐方案定稿，见 ADR-lite。）

## Decision (ADR-lite)

**Context**：重链只改变 `files.path`，物理文件系统零操作。source/replace 的 `output_dir` 是产物目录绝对路径事实；产物物理未随源文件移动。

**Decision**：

- Q1 选 A：`output_dir` 保持原绝对路径不变。重链只改写 `source_rel_path` 键。理由：`OpenAssetSplitPage` / 导出 / `isReplaceArtifactOutput` 均以 `output_dir` 定位权威，产物未搬时重算到新路径会让打开单页直接失效；且 `os.Rename` 搬产物目录违背「事务原子 + 重链零文件系统写」既有语义。
- Q2 选「不加前端字段」：不扩 `model.AssetRelocateResult`，仅 `Logger().Warn` 留痕（冲突落败也已在单点 Warn）。

**Consequences**：

- 重链后再次拆分：`source_rel_path` 已迁移，`upsertSplitRecord` 覆盖同一行，不再「另写记录 + 旧记录滞留」。
- 旧产物目录残留：与 cache 落点「下次重拆在新 derived 路径再生成一份」同类的已登记限制；旧目录内同名产物在再次 replace 拆分时由 `ownsOutDir=false` 分支识别为「用户自建内容」并拒绝覆盖，安全侧不劣化。

## Acceptance Criteria (evolving)

- [ ] 丢失源重链后 `GetAssetSplitRecord(新路径)` 命中，`Exists=true`，`OpenAssetSplitPage` 可打开单页产物。
- [ ] cache / source / replace 三类 target 记录的键均迁移，`output_dir` 行为符合 Q1 决策。
- [ ] 新键冲突保留 `source_removed=1` 的记录并 Warn。
- [ ] 重复重链幂等；旧键不存在零操作。
- [ ] 触发器注入失败时 `files.path` 与 `split_records` 均不变（事务回滚）。
- [ ] `go test ./...` 全通过；覆盖率门禁不降。

## Definition of Done

- 新增/更新 service 层单测覆盖上述用例（含事务回滚）。
- `docs/spec/assets-management.md` 相关节与 `CLAUDE.md` 规则行同步。
- `go test ./...` 与覆盖率门禁通过。

## Out of Scope

- 与本次无关的重构（不改 `migrateSplitRecordsTx` 以外的目录迁移逻辑）。
- 重链时物理迁移产物目录。
- `split_records` 表结构变更、新依赖。

## Technical Notes

- 单连接池纪律：事务内禁调 `s.db.*`（rows 未关写语句会死锁），须全程用 `tx`。
- 复用冲突消解：把 `migrateSplitRecordsTx` 源键分支的冲突四态抽成单点，目录级与文件级共用。
