# 落点切换生效与清理 mtime 守卫（两轮审核剩余两点）

## Goal

`c7f5d58`（v9 artifact_stem）之后两路审核实测确认两个未修点：

1. **落点切换被增量跳过**：`splitOneFileWithHash` 的跳过判定只比 `sourceHash` 与首页产物存在性，不比 `target`/预期落点。source→cache 且源未变时直接 `skipped`，M4 清理不触发，结果还报告 `Target=cache`（与实际落点不符），旧 source 产物永久残留。
2. **同主干同页序用户文件误删**：产物主干锚定后，用户在旧产物目录放入同名主干、同页序文件（如 `旧名-004.pptx`）且该页产物缺失时，仍被当作旧产物删除。

审核同时实证：同目录跨落点（replace→source）在主干同名时不清理，旧 `001-主干.pptx` 与索引行残留；`artifact_stem` 写入值无任何断言守护（变异测试全绿）。本任务一并收口（同族必要项）。

## What I already know

* 跳过判定在 `splitOneFileWithHash`（`assets_split.go:411` 附近）：`record.sourceHash == hash && splitArtifactPath(record, rel, 1)` 命中即 `Skipped`，不 upsert，但结果结构体 `Target: target` 用本次入参。
* 本次落点：source/cache 由 `splitOutputDir(lib, rel, absPath, outputToSourceDir)` 计算；replace 在 `splitOneFileReplace` 内用 `splitSourceOutputDir` 计算，且 replace 本来不做增量跳过。
* 落点切换清理门控三分支现状：异目录清理；同目录且 `artifactStem != curStem` 清理 + `excludeStem`；同目录同主干不调用。缺「同目录跨 target」分支。
* 旧产物命名形态由 `record.target` 解析（`parseSplitArtifactName`），跨 target 时旧产物形态与新产物形态不同（`<主干>-NNN` vs `<NNN>-<主干>`），collect 按旧 target 过滤天然不收集新形态产物。
* 审核实测：`excludeStem` 在仅大小写不同时折叠相等会整组跳过（`:1412`）。
* 审核实测：写入主干变异为 `""` 或去掉 `-p` 后所有测试仍绿，写入路径无守护。
* 审核建议的误删收口手段：索引行不可行（source/cache 产物不入 files 表）；页内容指纹无哈希列；`split_at`+mtime 守卫成本最低且失效方向为漏清（安全侧）；产物清单表需新表，成本中等。

## Requirements

1. **落点一致性纳入跳过判定**（点①）：`splitOneFileWithHash` 的跳过条件增加 `record.target == target` 且 `samePathFold(record.outputDir, outDir)`（`outDir` 为本次实际落点）。任一不符不跳过，走完整重拆，从而触发 M4 清理；跳过的结果必须与实际记录一致。
2. **同目录跨落点清理**：门控增加分支——同目录但 `record.target != target` 时调用清理，`excludeStem` 传空（新产物形态不同，collect 按旧 target 解析不会收集它）。
3. **修复 `excludeStem` 折叠误跳过**：删除循环的 `excludeStem` 比较改回精确相等；同目录且 `EqualFold(record.artifactStem, curStem)`（仅大小写差异）时不调用 M1 清理分支（本次产物覆盖同名物理文件，旧页由既有链路处理），避免整组跳过或误删新产物。
4. **清理 mtime 守卫**（点②）：删除候选前 `Stat`，若 `ModTime` 晚于 `record.splitAt` 加 2 秒容差，视为用户文件跳过（不加告警噪音，可在 Debug/Info 记录）；`Stat` 失败按不可判定保留。`splitAt` 解析失败时守卫不生效（保持原行为）。
5. **写入值守护**：三种落点成功用例断言 `getSplitRecord().artifactStem`（source/cache = 源主干；replace = `replaceArtifactBase(源主干)`，含 ≥3 位纯数字 `-p`）；删源失败补写分支同样断言。
6. **测试补齐**：source→cache 不跳过并清理旧产物；cache→source 反向；同目录 replace→source 清理旧形态 + 索引行；mtime 守卫（新文件保留、旧产物删除、Stat 失败保留）；仅大小写改名不误删新产物；target 变化时结果 `Target`/`OutputDir` 与实际一致。
7. **文档**：`docs/spec/pptx-split.md §9.5/§9.7` 补落点一致性与 mtime 守卫、写值断言要求；`CLAUDE.md:123` 修正 M2 表述（「候选集合精确/折叠 + 空值回落」），保持 ≤350 字符。

## Acceptance Criteria

* [ ] source→cache、源未变：不再 `Skipped`，重拆后旧 source 产物与索引行清理，新记录 `target=cache`、`artifact_stem` 正确。
* [ ] cache→source 反向同理。
* [ ] 同目录 replace→source（主干同名）：旧 `<页序>-主干.pptx` 与索引行清理，新 `<主干>-NNN.pptx` 保留。
* [ ] `旧名-004.pptx`（mtime 晚于 `splitAt`）在页 4 产物缺失时保留；旧产物（mtime 早于 `splitAt`）照常清理。
* [ ] 仅大小写 rename 重拆：不删本次新产物，旧页由既有链路处理。
* [ ] 三种落点写入的 `artifact_stem` 有断言，变异（`""` / 缺 `-p`）会变红。
* [ ] `go test ./...` 通过；覆盖率 service ≥76% 不降。

## Definition of Done

* 新增/更新用例在回退到 `c7f5d58` 实现时对应项变红（至少点①、点②、同目录跨落点三条）。
* 文档与实现逐条一致，无绝对化表述。

## Technical Approach

1. `splitOneFileWithHash`：
   - `record == nil` 分支不变；
   - 跳过条件改为 `record.sourceHash == hash && record.target == target && samePathFold(record.outputDir, outDir) && artifactOK`；不满足则继续重拆。
   - 跳过结果中的 `Target`/`OutputDir`/`PageCount` 保持来自记录（现状已如此，确认）。
2. 清理门控（source 与 replace 两处）统一为三分支：
```go
switch {
case !samePathFold(record.outputDir, outDir):
    cleanup(record.outputDir, "")
case record.target != target:
    cleanup(record.outputDir, "")
case record.artifactStem != "" && record.artifactStem != curStem &&
     !strings.EqualFold(record.artifactStem, curStem):
    cleanup(record.outputDir, curStem)
}
```
3. `cleanupSplitArtifactsInDirByPage`：删除循环前 `isSplitArtifactRecent(cand.path, record.splitAt)` 守卫；`excludeStem` 精确比较；新增辅助 `parseSplitAt` + 2s 容差常量。
4. 测试与文档。

## Decision (ADR-lite)

**Context**：误删用户文件的根因是产物无内容指纹，无法从文件本身证明归属；引入指纹需新表或哈希列。
**Decision**：本任务采用 `split_at`+mtime 守卫——产物 mtime 恒早于记录写入时间，用户后放入的文件通常更晚；失效方向为「漏清」而非「误删」，符合风险不对称原则。同主干同页序且 mtime 早于 split_at 的用户文件仍可能被删（低概率可达），如实记录。
**Consequences**：复制/解压保留旧 mtime 的用户文件仍可能被删；带新 mtime 的产物复制会被保留（漏清）。彻底解决需产物清单表（另立任务）。

## Out of Scope

* 落点目录归属唯一性校验（审核 blocker `:430`/`:1121`）——另立任务。
* 备份还原版本预检（审核 blocker `assets_backup.go:299`）——另立任务，数据安全优先级最高。
* source 落点核心失败前删旧页（`:430`）、Windows 大小写 replace 重拆失败（`:661`）、删除失败不可收敛（`:1430`）、跨卷 derived 告警噪音（`:851`）——另立任务。
* 产物清单表 / 页级哈希。

## Technical Notes

* 审核来源：`c7f5d58` 两路子代理审核（逻辑：2 blocker + 5 major；测试：回退实验表、缺口实证、备份还原兼容）。
* 关键文件：`service/assets_split.go`、`service/assets_split_artifact_stem_test.go`、`docs/spec/pptx-split.md`、`CLAUDE.md`。
