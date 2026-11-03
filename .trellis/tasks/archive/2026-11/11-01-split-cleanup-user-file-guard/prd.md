# 重拆清理误删收口：旧目录唯一同页序用户文件

## Goal

`cleanupSplitArtifactsInDirByPage`（service/assets_split.go）在改名重链后按 `record.pageCount` 逐页反查旧产物目录，**同页序唯一候选即删**。当原产物某页已缺失、而用户在旧目录放了自建 `笔记-001.pptx`（source/cache 形态 `<主干>-<NNN>.pptx` 同页序）时，用户文件被当作旧产物删除，且联动删除同名索引行与缩略图。本任务收紧判定面：保留用户自建同页序文件，同时照常清理旧主干产物并同步索引行。

## What I already know

* 清理单点：`service/assets_split.go:1190 cleanupSplitArtifactsInDirByPage`，两处调用（`:477` source 链路、`:763` replace 链路），均在拆分成功后、删源前。
* 逐页判定单点：`findSplitArtifactByPage(outputDir, target, slideNum)`（`service/assets_split.go:1115`）——目录内按命名形态匹配页序，**唯一命中返回路径，多候选返回 false（跳过该页）**。
  * source/cache 形态 `<主干>-<NNN>.pptx`（`pageSeqFromArtifactName`）；replace 形态 `<纯数字页序>-<主干>.pptx`。
  * 用户例 `笔记-001.pptx` 属 source/cache 形态命中。
* `assetSplitRecord` 只有 `sourceRelPath/sourceHash/outputDir/pageCount/splitAt/target/sourceRemoved`，**无产物主干列，也无页产物哈希**（spec 明文「不改表结构」）。
* 改名重链后 `sourceRelPath` 已是新名，产物主干仍是旧源名 → 不能由记录反推产物主干（spec §9.5 已明确此点）。
* 索引行同步走 `deleteIndexRowsForPaths`（按库根换算相对路径查 files 行 + 删缩略图）；cache/source 落点不写索引行，仅 replace 落点有。
* 既有用例：`TestCleanupSplitArtifactsInDirByPage_MultiCandidateKept`（多候选跳过）、`_RemovesEmptyDir`、`_EmptyRecordNoop`。

## Requirements

* 用户自建同页序文件（含原产物该页缺失的场景）必须保留，不删文件、不删索引行。
* 旧主干产物（正常完整目录）照常清理，索引行与缩略图同步删除；删空目录仍删目录。
* 不可判定场景按安全侧失败：保留文件 + Warn 日志（含目录与页序），不静默。
* 判定逻辑收敛在清理单点内，不改 `findSplitArtifactByPage` 的「打开/导出单页定位」语义（那里只定位不删除）。
* 不改 `split_records` 表结构。

## Acceptance Criteria

* [ ] 旧目录内 `笔记-001.pptx`（用户自建）替代已缺失的页 1，页 2..N 旧产物完整 → 只有页 1 保留，页 2..N 被清理，页 1 索引行保留、页 2..N 索引行删除。
* [ ] 旧目录产物完整（无用户文件）→ 全部清理，行为与现状一致。
* [ ] 同页序多候选 → 该页跳过（行为与现状一致）。
* [ ] 目录仅剩单个形态匹配文件且无法认定为产物组（覆盖页数不足）→ 全部保留 + Warn。
* [ ] `go test ./...` 通过，覆盖率门禁不降。

## Definition of Done

* 单测覆盖上述四类场景 + 主干分组平票场景。
* `docs/spec/pptx-split.md §9.5` 更新判定面描述；必要时 `assets-management.md §11.3` 不改（无记录口径变更）。
* 覆盖率门禁（service ≥76%）不降。

## Technical Approach

产物主干分组 + 覆盖门禁（用户 2026-11-01 确认方案 4）：

1. `collectSplitArtifactsByPage(dir, target)` 单点：一次读取 `os.ReadDir`，按页序收集形态匹配候选，同时按主干分组（主干提取 `splitArtifactStem` 单点：source/cache 去末尾 `-<纯数字>`，replace 去首个 `<纯数字>-`）。
2. 选产物组：覆盖页数（去重页序数）最多的主干组；须满足「覆盖 ≥2 页 或 `record.pageCount == 1`」；无组/平票/覆盖不足 → 不可判定，整目录零删除 + Warn。
3. 逐页：候选唯一且主干 == 产物组主干 → 删文件收路径；多候选或主干不符 → 保留（不猜）。
4. 收集成功删除的路径批量 `deleteIndexRowsForPaths`；目录删空才 `os.Remove`（不变）。

## Decision (ADR-lite)

**Context**：清理靠「命名形态 + 页序唯一」判定，无法区分产物与用户自建同形态文件；错误删除用户数据不可恢复。
**Decision**：用目录内主干一致性 + 页覆盖证据判定产物组，仅删产物组主干文件。
**Consequences**：误删率接近 0，正常路径零漏清理；产物大部分缺失或主干平票时保留残留（安全侧），需人工清理。`findSplitArtifactByPage` 保持原语义（单页定位），不复用于清理判定。

## Out of Scope

* 不改 `findSplitArtifactByPage`（打开/导出/增量判定路径）。
* 不新增 split_records 列（产物哈希/主干）。
* 不做前端提示（仅后端 Warn 日志）。

## Technical Notes

* 关键代码：`service/assets_split.go`（清理与判定）、`service/assets_split_cleanup_renamed_test.go`（测试）。
* spec：`docs/spec/pptx-split.md §9.5` 末条「改名重链后的旧产物目录清理」。
