# replace 指纹保留归属跨落点收敛

## Goal

第二轮审核（`143ed8a`）发现：v11 `retained_fingerprints` 只在 replace「同 target 同落点」链路被消费，用户切 target（replace → source）时残留页未清理且 retained 被新记录整体覆盖，再切回 replace 被 `firstReplaceArtifactName` 按名永久拒绝。本任务闭合该链路，并修复守卫过匹配、R3 换回失败提示、文档三处矛盾与测试缺口。

## Requirements

### R1 中危：retained 跨 target / 跨落点消费

- 新增单点 `cleanupRetainedSplitArtifacts(store, lib, record, dir, excludeStem) []splitArtifactFingerprint`：
  - 只处理 `record.retained`，`dir` 必须与 `record.outputDir` 折叠相等（`samePathFold`），否则原样返回（不跨目录误删）。
  - 逐条：名字合法（`splitFingerprintNameValid`）、命中 `excludeStem` 跳过、`fingerprintMatch(dir/<name>, fp)` 才删（不符 = 文件已被替换/移走，条目丢弃）；删除成功或文件不存在 → 不保留；删除失败 → 保留原条目。
  - 删除函数经 `s.removeArtifactFn`（生产 `removeSplitArtifactFile`）。
- `cleanupSplitArtifactsInDirByPage` **不再**承担 retained 消费；`cleanupStaleReplaceArtifacts` 恢复为只按 `record.fingerprints` 判定（不再用 `ownershipFingerprintForName`），retained 统一由上面的单点处理，避免同一文件重复尝试与重复登记。
- 调用点：
  - source/cache：核心成功、旧页清理之后调用（`dir = record.outputDir`），返回值并入步骤 6 upsert（不再传 nil）。
  - replace：`cleanupStaleReplaceArtifacts` 之后调用（`dir = record.outputDir`），返回值与 `cleanupStaleReplaceArtifacts` 的失败条目按 name 去重合并后写入两处 upsert。
- `firstReplaceArtifactName` 拒绝分支的收口：`record != nil` 且目录内 stray 文件能按 `ownershipFingerprintForName` 命中且内容全等（retained 或指纹凭证）时，视为本记录拥有该落点——`oldArtifacts` 照常快照并继续流程（其余用户同名文件仍会在步骤 5 被拒）；否则维持拒绝。新增单点 `replaceOutDirOwnedByRecord(record, outDir, base)`。
- 语义：任一次成功拆分（任意 target、`output_dir` 相同）都会尝试消费 retained；仍删不掉则继续保留在新记录，不再静默清空。

### R2 低危：核心中间态守卫收紧到真实命名位数

`isReplaceCoreArtifactNameExact` 的页序段要求**至少 2 位**纯数字（历史 `%02d`、当前 `%03d`）：`方案-9.pptx` 不拦（核心永不写），`方案-09.pptx` / `方案-009.pptx` 拦截。同步更新表驱动用例与文档。

### R3 低危：复验换回失败的提示与备份可见性

`backupReplaceArtifactForSurrender` 换回失败时返回备份路径（空串 = 换回成功）；调用方在备份保留时返回文案：
`第 %d 页目标文件「%s」内容与拆分记录不一致（可能已被修改），备份已保留在「%s」，请人工确认后移走或还原`。
备份仍不登记进 `backups`（`removeReplaceArtifactBackups` 不删），保留人工恢复入口。

### R4 文档口径修正

- `docs/spec/pptx-split.md` §9.3 / §9.5 / §9.7、`docs/spec/assets-management.md` §15.6、`CLAUDE.md` 指纹规则行：
  - 删除/改写「不在指纹中的候选一律保留」与 `cleanupStaleReplaceArtifacts` 的旧口径（现为：指纹 allow-list；retained 由独立单点消费）。
  - 明确「mtime 回落」仅限 `cleanupSplitArtifactsInDirByPage` / `removeSplitArtifactsBeyond`；`cleanupStaleReplaceArtifacts` 与 retained 消费无 mtime 守卫。
  - retained 自愈范围限定：同一 `output_dir` 的任一次成功拆分消费；切 target、改名重链、目录迁移不丢失（随记录行迁移），跨目录不消费。
  - 补 R2 位数口径与 R3 备份保留文案。
- `CLAUDE.md` 两行保持 ≤350（钩子密集档）。

### R5 测试补强

- retained 跨 target 端到端：3 页 replace → 删页失败 + 删源失败（保留源行）→ 切 `target=source`（同落点）→ 残留页被删、retained 清空 → 再切回 `replace` 成功。
- retained 消费失败仍保留：切 target 后再次删除失败 → 新记录仍携 retained，且切回 replace 不被按名拒绝（`replaceOutDirOwnedByRecord` 放行）。
- retained 随备份快照、改名重链、目录迁移存续（扩展现有 fingerprint 迁移测试或 retained 测试）。
- 守卫位数：`方案-9.pptx` 不拦、`方案-09.pptx` 拦；最终产物名 `方案-2.pptx` 不拦。
- R3 文案映射单测（备份保留 / 已换回两态）。
- 变异验证：把 retained 跨 target 消费删掉，新增用例须变红。

## Acceptance Criteria

- [ ] 切 target 后残留页被消费或 retained 继续保留，再切回 replace 不被永久拒绝。
- [ ] 守卫不再误拦 1 位页序用户文件；2 位及以上仍拦截。
- [ ] 换回失败文案含备份绝对路径，备份不被自动删除。
- [ ] 三处文档与实现一致，关键规则行 ≤350。
- [ ] 新增/扩展测试覆盖 R1/R2/R3/R5；`go test ./...`、`scripts/coverage-check.sh` 通过。

## Definition of Done

- 单测 + 迁移测试通过；`gofmt`、`go vet`、`GOOS=linux go vet ./service/` 干净。
- README 无需更新。

## Technical Approach

- retained 消费收敛到单一函数，两个拆分链路各调一次；replace 链路与 `cleanupStaleReplaceArtifacts` 的返回值按 name 去重。
- `upsertSplitRecord` 参数不变（已支持 retained），source/cache 改传消费后的剩余条目。
- 落点归属放宽仅对「凭证内容全等」的文件生效，不放宽无凭证同形文件的拒绝。
- 守卫位数调整只改精确形态判定，折叠逻辑不变。

## Decision (ADR-lite)

**Context**：retained 是「清理失败仍属历史产物」的归属凭证，但消费面此前只覆盖 replace 同 target；切换落点会整体覆盖记录导致凭证丢失与残留页孤儿。

**Decision**：把 retained 消费提为独立单点，所有 target 的成功拆分都尝试消费；删除失败继续随记录保留；落点归属在凭证内容全等时放宽。

**Consequences**：多一次目录尝试删除；切换落点不再产生永久拒绝；文档需说明 retained 的存续范围（同 output_dir）。

## Out of Scope

- 不改 `fingerprintsUsable` 契约与 `fingerprints` 的 allow-list 语义。
- 不为 v11 前记录回填 retained。
- 不引入 UI 提示残留备份（仅日志 + 失败文案含路径）。

## Technical Notes

- 改动点：`service/assets_split.go`（cleanupStaleReplaceArtifacts、cleanupSplitArtifactsInDirByPage、新单点、两处调用、firstReplaceArtifactName 分支、守卫位数、R3 文案）、`service/assets_split_fingerprint.go`（如需去重工具）。
- 测试：`assets_split_retained_test.go`、`assets_split_replace_fingerprint_test.go`、`assets_split_fingerprint_test.go`。
- 文档：`docs/spec/pptx-split.md` §9.3/§9.5/§9.7、`docs/spec/assets-management.md` §15.6、`CLAUDE.md`。
