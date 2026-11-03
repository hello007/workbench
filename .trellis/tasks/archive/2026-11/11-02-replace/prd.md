# replace 备份让位前的产物指纹校验

## Goal

v10 页级指纹（`split_records.artifact_fingerprints`：page/name/size/sha256）已用于三类清理路径，但**就地替换（replace）链路的「备份让位」仍只按文件名形态认定历史产物**：目标名命中 `oldArtifacts` 即把旧文件改名为 `.replace-bak`，新页占位，成功路径删源前删除全部备份。若用户把容器目录内某页产物替换成自有内容（同名），重拆会把该文件当历史产物备份、随后删除，用户内容不可恢复。

本任务在备份让位**之前**增加指纹归属校验：旧记录指纹可用时，只有 `size + SHA-256` 与该页指纹全等的文件才按历史产物备份覆盖；不符或无对应指纹条目视为用户文件，拒绝覆盖并回滚本次已产出页，保留用户内容。

## Requirements

1. **判定顺序**（`splitOneFileReplace` 步骤 5，`service/assets_split.go` rename 循环内）：
   - `record == nil` 或 `fingerprintsUsable(record) == false`：保持现行行为（兼容 v10 前记录）。
   - 指纹可用：对命中 `oldArtifacts` 的目标名执行
     `fingerprintForName(record, oldName)`（内部经 `sameNameFold`，仅 Windows 折叠大小写）+ `fingerprintMatch(<outDir>/<oldName>, fp)`（size + 全文 SHA-256，按磁盘实际名取路径）。
   - 命中 → 走现有「先备份再让位」流程。
   - 不命中（无对应指纹条目 / 大小或哈希不符 / 名字折叠多命中不可判定）→ 复用现有失败语义「第 N 页目标文件「…」已存在且不属于历史拆分产物，请先移走或重命名后重试」，经 `rollbackFailed` 回滚本次已产出页与索引行。
2. **大小写语义不破坏**：Windows 仅大小写改名后，磁盘实际名与目标名不同但折叠相等，仍须正确备份与还原；按实际名取路径做哈希与备份。Linux/macOS 大小写敏感，不折叠。
3. **失败/取消路径不产生 `.replace-bak` 残留**：指纹校验失败发生在备份 rename 之前，当前页无备份；前序页已有备份由 `rollbackArtifacts` → `restoreReplaceArtifactBackups` 原样还原。
4. **正常旧产物仍可重拆**：未被用户修改的历史产物指纹命中，备份让位成功，备份在删源前由 `removeReplaceArtifactBackups` 删除。
5. **不改 mtime 守卫主判据**；指纹缺失/损坏仍回落现行行为。
6. 禁改前端与 `frontend/wailsjs`；注释/文档全中文。

## Acceptance Criteria

- [ ] 用户同名不同内容文件：重拆返回失败（`rolledBack=true`），该文件字节不变，目录内无 `.replace-bak` 残留，源保留。
- [ ] 旧产物字节未变：备份让位成功，重拆成功，备份清理。
- [ ] 指纹缺失（空 / 条目数不符 / v10 前记录）：保持现行按名覆盖行为。
- [ ] Windows 大小写变体：按磁盘实际名校验、备份与还原；非 Windows 断言不折叠（大小写变体视为用户文件拒绝）。
- [ ] 指纹命中但大小写不同（Windows）：按实际路径哈希，不误判用户文件。
- [ ] 回滚路径：前序页备份还原后 `.replace-bak` 不残留。
- [ ] `go test ./...` 通过；`scripts/coverage-check.sh` 达标。

## Definition of Done

- 单元测试覆盖上述六类场景（含平台分侧断言）。
- `docs/spec/pptx-split.md` §9.3 步骤 5 与 §9.5 补充指纹归属校验；`docs/spec/assets-management.md` §15.6 同步。
- `go test ./...`、`scripts/coverage-check.sh` 通过。
- 确认 README 是否需更新（预期：否，属内部行为修正）。

## Technical Approach

在 rename 循环的 `if oldName, isOld := foldNameInSet(oldArtifacts, dstName); isOld` 分支内、构造备份路径之前插入校验：

```go
oldPath := filepath.Join(outDir, oldName)
if fingerprintsUsable(record) {
    fp, ok := fingerprintForName(record, oldName)
    if !ok || !fingerprintMatch(oldPath, fp) {
        return rollbackFailed("第 %d 页目标文件「%s」已存在且不属于历史拆分产物，请先移走或重命名后重试", page, dstName)
    }
}
bak := replaceArtifactBackupPath(outDir, oldName)
...
```

复用既有单点 `fingerprintForName` / `fingerprintMatch` / `fingerprintsUsable`，不新增指纹逻辑。失败消息与现有 `fileExists(dst)` 分支完全一致，批量入口按业务失败逐项继续的语义不变。

## Decision (ADR-lite)

**Context**：备份让位把「命名形态」当归属事实，用户同名产物被覆盖后备份即删，内容不可恢复；v10 指纹已能验证内容归属，但未接入该路径。

**Decision**：仅当指纹可用时启用内容校验，且校验点为备份 rename 之前（而非之后），使拒绝路径天然零备份残留；指纹不可用时保持现状，避免 v10 前记录的行为回退。

**Consequences**：旧产物每页多一次 SHA-256 读取（单页 pptx，成本可忽略）；用户修改过的同名产物将导致重拆失败并提示移走，属有意的安全侧失败；指纹缺失记录的旧风险保留，由 v10 已写入指纹的新记录逐步覆盖。

## Out of Scope

- 不改 `cleanupStaleReplaceArtifacts` / `removeSplitArtifactsBeyond` / `cleanupSplitArtifactsInDirByPage` 的既有指纹逻辑。
- 不引入指纹自动回填。
- 不改前端提示文案（复用现有失败消息）。
- 不为指纹缺失记录补算指纹。

## Technical Notes

- 改动点：`service/assets_split.go:716-732`（步骤 5 rename 循环）。
- 复用单点：`service/assets_split_fingerprint.go` 的 `fingerprintsUsable`（:103）、`fingerprintMatch`（:123）、`fingerprintForName`（:155）。
- 大小写折叠：`sameNameFold` / `foldNameInSet`（`assets_split.go:1939`），仅 Windows 折叠。
- 回滚：`rollbackReplaceArtifacts`（:949）、`restoreReplaceArtifactBackups`（:2052）、`removeReplaceArtifactBackups`（:2039）。
- 测试基线：`service/assets_split_replace_test.go`（`fakeReplaceCore`、`fakeRecycleBin`、`oneSlideParse`）、`service/assets_split_fingerprint_test.go`（`splitFingerprintOfTest`、`TestFingerprintForName_CaseFoldByPlatform`）、`service/assets_split_replace_review2_test.go`（`assertNoReplaceBackup`）。
- 文档：`docs/spec/pptx-split.md` §9.3 / §9.5、`docs/spec/assets-management.md` §15.6。
