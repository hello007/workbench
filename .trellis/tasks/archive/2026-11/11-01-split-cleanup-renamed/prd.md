# 重拆清旧页兼容改名重链

## Goal

改名重链后再次拆分（产物缺失触发重拆）时，清理阶段仍按当前 `relPath` 推导主干删除旧页，导致旧主干产物残留并继续留在 files 索引与批量拆分源中。清理面复用产物定位同源口径，把「当前主干产物 + 记录反查得到的旧主干产物」一并清理。

## 背景

- `RelocateAssetFile` 支持改名重链：`split_records` 源键迁到新路径，`output_dir` 保持原产物目录（提交 7bdf29f）。
- 产物定位已收口 `splitArtifactPath(record, relPath, slideNum)`（`service/assets_split.go`）：当前主干直拼失败时在 `output_dir` 内按页序反查唯一候选（提交 7715ff4）。
- 残留缺口：`splitOneFile` 重拆清理仍按当前主干调用 `removeSplitArtifacts(outDir, baseName)`，改名重链后 `旧名-001.pptx` 不被清理，重拆后新旧主干产物并存，旧产物仍在索引内可再次进批量源。

## Requirements

1. 重拆清理的主干解析复用 `splitArtifactPath` 同源口径：清理面覆盖「当前主干产物 + 记录反查得到的旧主干产物」，禁另造主干推导逻辑。
2. 源目录落点（`outputToSourceDir`）与 replace 落点的清理形态分别对齐核心命名 `<主干>-<页序>.pptx` 与 `isReplaceArtifactName`；derived 落点 `RemoveAll` 保持不变。
3. 清理只删产物命名形态命中的文件，目录内用户自建内容一律不动（与 `removeSplitArtifacts` 既有契约一致）。
4. 不引入新依赖；不改 `split_records` 表结构。

## Acceptance Criteria

- [ ] 改名重链 + 产物缺失触发重拆后，旧主干产物被清理、新主干产物生成。
- [ ] 重拆后 files 索引中不含旧主干产物记录。
- [ ] 用户自建同名异构文件（如 `<主干>-草稿.pptx`）不被误删。
- [ ] replace 落点改名重链重拆的清理形态正确。
- [ ] `go test ./...` 全通过；覆盖率门禁不降。

## Technical Approach

- **旧目录产物清理由页序反查驱动**：记录存在且 `record.outputDir` 与本次落点目录不等价时，对 `1..record.pageCount` 逐页调用 `findSplitArtifactByPage(record.outputDir, record.target, n)`，命中即删该文件并删其索引行/缩略图。不反推旧主干，不新造命名推导逻辑。
- 多候选（用户手工放入同页序文件）时该方法返回未命中——仅跳过该页，不牵连其余页。
- **当前落点清理保持原口径**：source 落点 `removeSplitArtifacts(outDir, 当前主干)`；derived 落点 `os.RemoveAll(outDir)`；replace 落点沿用 `oldArtifacts` 延迟清理。
- **replace 落点旧目录清理挂在成功收尾**（`cleanupStaleReplaceArtifacts` 同阶段，删源前）：失败/取消路径不得删旧产物，与既有延迟清理契约一致。
- 旧目录删空后尝试 `os.Remove`（失败或非空忽略：用户自建内容不动），不留空目录。
- 删除失败仍按既有口径仅告警继续（残留由下次重拆收敛）。
- 索引行清理复用 `deleteIndexRowsForReplacePaths`（提升为通用路径删除语义，重命名或补注释，调用点同步）。

## Decision (ADR-lite)

**Context**：改名重链后 `split_records` 键迁移但 `output_dir` 冻结在旧产物目录，旧产物文件名主干与新源名不一致；按主干推导清理需反向还原命名（含 replace 的 `-p` 后缀与 `%02d` 历史形态），判定面大且易误删。

**Decision**：采用「按页序反查」（方案 A）——复用已存在的 `findSplitArtifactByPage` 单点，以记录页数枚举旧页，精确删除命中文件。

**Consequences**：清理精度高、误删面小、无需反转命名；代价是依赖 `record.pageCount` 的准确性（记录缺失或页数失真时不清旧目录，下次重拆或重扫收敛）；旧目录内用户自建的同页序文件会因多候选不可判定而被整体保留（安全侧失败）。

## Out of Scope

- 产物目录整体搬迁（属子任务 `11-01-relink-artifact-dirsync`）。
- 前端交互改动。

## Technical Notes

- 单连接池纪律不涉及（纯文件系统清理段）。
- 参考：`service/assets_split.go`（`splitArtifactPath` / `splitArtifactPathForStem` / `findSplitArtifactByPage` / `removeSplitArtifacts`）。
