# 重链拆分记录口径修复：改名产物定位与目录迁移落点

## Goal

上一任务（11-01-relink-split-records，提交 7bdf29f）把文件级重链纳入拆分记录迁移面，但审核发现三处口径缺口：改名式重链后产物定位仍按新主干拼接而失败；目录级迁移会无条件重算已冻结的 `output_dir`；丢失记录清理与 force 重链不处理拆分记录，留下孤儿或误导入口。本任务按审核结论全量修复，不改 `split_records` 表结构。

## 审核依据（两路独立复核 + 实测复现）

1. **P1 改名重链打开单页失效（已实测）**：`OpenAssetSplitPage` / 导出 / 增量判定用「当前 `relPath` 的主干」推导产物名；重链改名后记录键已换、产物仍按旧主干命名，实测报「拆分产物不存在或已被清理（新名-001.pptx）」，同时 `UpToDate=true` 显示「已最新」，自相矛盾。
2. **P2 目录级迁移重算冻结落点**：`migrateSplitRecordsTx` 源键命中分支对 source/replace 无条件按「库根 + 新源目录 + 新源主干」重算 `output_dir`，隐含「产物在源目录下」前提，与文件级重链「产物不随源移动」直接冲突——重链到 B 后再移动目录 B，记录被改到 `C/x`，产物实在 `A/x`，打开单页再次失效。
3. **P3 记录生命周期缺口**：
   - `clearLostFiles` 只删 `files` 行，不删对应 `split_records`；键迁移后孤儿记录落在新路径，后续同名文件会命中他文件的记录；
   - force 重链（内容已变）仍保留旧记录，`Exists=true` 让「打开拆分单页」指向与当前源不对应的历史产物，前端无任何过期提示；
   - replace 记录被再次拆分覆盖后旧产物脱离 `isReplaceArtifactOutput` 保护，可被拆成「页的页」。

## Requirements

1. **产物路径解析单点**（P1）：新增 `splitArtifactPath(record, relPath, slideNum)`——先按记录键主干直拼，直拼不存在时在 `record.outputDir` 内按页序反查（唯一候选才命中）。`OpenAssetSplitPage`、导出 `singlePptxPath`、拆分增量判定（首页存在性）、replace 产物识别统一消费该口径。反查失败保留直拼路径供错误消息。
2. **replace 产物识别兼容改名**（P3-3）：`isReplaceArtifactOutput` 在同一 `output_dir` 内追加宽松形态判定 `<纯数字>-<任意主干>.pptx`（该目录由记录独占），防旧产物再次进入批量源。
3. **目录级迁移落点判定**（P2）：`migrateSplitRecordsTx` 源键命中分支重算前，先用 `filepath.Rel(库根, output_dir)` + `stripDirPathPrefix(oldPrefix)` 判定现有落点是否位于被迁移前缀下；命中才重算（产物确实随目录移动），否则保持原绝对路径。cache 目标行为不变。
4. **清理丢失记录联动**（P3-1）：`clearLostFiles` 在删除 `files` 行的同一事务内删除该行路径对应的 `split_records` 行。
5. **force 重链删除失效记录**（P3-2）：哈希不等且 force 确认的重链（内容已变）在同一事务内删除该文件旧键的拆分记录——旧产物与当前源不再对应，保留会展示必然过期/错误的入口；用户重新拆分后重建记录。
6. 不改 `split_records` 表结构；不引入新依赖；不改前端。

## Acceptance Criteria

- [ ] 哈希相等 + 改名重链后，`GetAssetSplitRecord(新路径)` 命中且 `OpenAssetSplitPage` 打开成功（产物按旧主干定位）。
- [ ] 导出 `single_pptx_path` 在改名重链后仍能导出（不静默导空）；replace 记录导出按 `<页序>-<主干>.pptx` 定位。
- [ ] 增量判定在改名重链后命中「已最新」跳过，不产生重复拆分与孤儿。
- [ ] 重链后移动所在目录：`output_dir` 保持在产物实际目录，不被重算到源的新位置。
- [ ] 正常目录移动/前缀替换的 `output_dir` 重算行为不变（回归用例守护）。
- [ ] 清理丢失记录后对应 `split_records` 同事务删除。
- [ ] force 重链（内容已变）后旧拆分记录消失；哈希相等重链（含改名）记录保留。
- [ ] replace 旧产物改名后仍被 `filterReplaceArtifactSources` 剔除（不拆「页的页」）。
- [ ] `go test ./...` 全通过；覆盖率门禁不降。

## Definition of Done

- 新增/更新 service 层用例覆盖上述断言（含改名、目录迁移回归、清理联动、force 删除）。
- `docs/spec/assets-management.md`、`docs/spec/pptx-split.md` 相关节与 `CLAUDE.md` 规则行同步。
- `go test ./...` 与 `scripts/coverage-check.sh` 通过。

## Decision (ADR-lite)

**Context**：记录未存产物命名主干列（PRD 禁改表），而产物定位依赖主干；`output_dir` 在文件级重链上是物理事实、在目录级迁移上是可推导值，两口径冲突。

**Decision**：
- 主干解析走「直拼 + 目录反查」两级，不改表；output_dir 为记录独占目录，页序反查唯一候选才采纳（多候选视为不可判定，保留直拼供报错）。
- 目录迁移保留「命中前缀才重算」的前提校验，使 `output_dir` 始终指向产物物理位置。
- force 重链删除记录，而非保留 + 前端提示（前端 `UpToDate` 未消费，补提示需跨层扩面且仍留下错误入口）。

**Consequences**：
- 改名重链的产物定位依赖目录内文件名形态，用户在该目录手工放入同页序文件会干扰反查——仅在直拼失败时生效，且要求唯一候选；目录本就由记录独占。
- force 重链后用户须重新拆分，历史产物目录保留在磁盘（不自动删除，与既有「旧产物残留」限制一致）。

## Out of Scope

- `split_records` 增列（主干/落点类型）与表结构迁移。
- 重链时物理迁移或重命名产物目录。
- 前端交互与提示改动。
- 与本次审核项无关的重构。

## Technical Notes

- 单连接池纪律：新增事务内操作全程用 `tx`。
- `pageSeqFromArtifactName` 可解析 `<主干>-<NNN>.pptx`；replace 形态 `<NNN>-<主干>.pptx` 需前缀解析。
- 模型：`model.AssetSplitTargetReplace` 判定命名形态。
