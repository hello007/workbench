# replace 产物形态位数与归属口径修正

## Goal

第三轮审核（`5cf2f4f`）发现 1 高 2 中 2 低缺陷：R2 守卫收紧只改守卫未改产物形态判定，导致 1 位页序用户文件进入 `oldArtifacts` 后被旧记录按名删除（数据丢失回归）；跨目录按 basename 过滤 retained 致真实残留孤儿；`ownsOutDir` 分支守卫不认 retained 归属使锁定文件重试永久拒绝。本任务全部修正。

## Requirements

### R1 高危：1 位页序用户文件不得进入历史产物集合

- `isReplaceArtifactNameExact` 两个分支的页序段统一要求 **≥2 位纯数字**（核心形态 `<base>-NNN`、已改名形态 `<NNN>-<stem>`；历史 `%02d`、当前 `%03d` 均满足）。
- 影响面：`snapshotReplaceArtifactNames`、`firstReplaceArtifactName`、`rollbackReplaceArtifacts`、批量剔除等所有 `isReplaceArtifactName` 消费点自动一致；`方案-9.pptx` 不再进入 `oldArtifacts`，旧记录（指纹不可用）按名清理不再删它，也不再被当作同形文件拒绝。
- 与 `isReplaceCoreArtifactNameExact`（R2 上轮已 ≥2）口径完全一致；新增断言两函数对 1 位页序一律 false。

### R2 中危：retained 新页同名过滤限定同目录

- source/cache 链路与 replace 链路调用 `recordWithoutRetainedNames` 前，先判 `samePathFold(record.outputDir, 本次产物目录)`：同目录才按新页名过滤；异目录（如 replace → cache，新落点在 B、凭证在 A）不过滤，全部条目交 `cleanupRetainedSplitArtifacts(record.outputDir)` 消费。
- replace 链路「本次产物目录」取 `outDir`；source/cache 取 `result.OutputDir`。
- 新增 cache 异目录端到端用例：replace 记录 A 带 retained，切 `target=cache`（落点 B）且新页数与残留页同名 → A 中残留被消费、retained 不因 basename 相同被误丢。

### R3 中危：ownsOutDir 分支守卫认 retained 归属

- `ownsOutDir` 分支的核心中间态守卫改走 `firstUnownedReplaceCoreArtifactName`（与放宽分支同口径）：凭证（指纹或 retained）按名命中且内容全等的历史产物不拦截；无凭证/内容不符的中间态同名文件仍拒绝。
- 覆盖场景：source→replace 同目录切换、旧 source 残留以 `<stem>-NNN.pptx` 形式留在 retained，占用解除后重试必须成功（消费/覆盖该文件），不再永久拒绝。
- 已知限制保持并记明：核心直接写落点，凭证覆盖的中间态文件在拆分失败时可能被核心同名覆盖（无临时目录），不可回滚。

### R4 低危：文档口径

- `docs/spec/assets-management.md` §15.6：区分「指纹可用 allow-list」与「指纹不可用时 `cleanupStaleReplaceArtifacts` 按名删除（v10 前兼容）」；retained 消费目录 = `record.outputDir`（旧落点），与新 target/新落点无关，新页同名过滤仅同目录生效。
- `docs/spec/pptx-split.md` §9.3/§9.5/§9.7：同步位数口径（≥2 位，两形态）、retained 消费目录、owns 分支守卫改 `firstUnowned...`。
- `CLAUDE.md` 指纹规则行澄清同上（≤350）。

### R5 测试

- 1 位页序用户文件 + 指纹不可用旧记录 + replace 重拆：文件保留、拆分成功、无误删。
- `isReplaceArtifactName` / `isReplaceCoreArtifactName` 位数表驱动：1 位 false、2/3 位 true、最终产物名不误判。
- cache 异目录 retained 消费（R2）。
- owns 分支 retained 核心形态重试成功（R3；注入删除失败一次制造 retained）。
- 变异验证：恢复任意位数判定 → 1 位删除用例变红；去掉同目录条件 → cache 用例变红；owns 分支改回无条件守卫 → 重试用例变红。

## Acceptance Criteria

- [ ] `方案-9.pptx` 在指纹不可用旧记录重拆中不被删除、不被拒绝。
- [ ] cache 异目录切换后 retained 在旧目录被消费，不产生孤儿残留。
- [ ] owns 分支 + retained 核心形态文件占用解除后重试成功。
- [ ] 三处文档与实现一致，关键规则行 ≤350。
- [ ] `go test ./...`、`scripts/coverage-check.sh`、`gofmt`、`go vet`、`GOOS=linux go vet ./service/` 通过。

## Definition of Done

- 新增/扩展测试覆盖 R1/R2/R3；变异验证三处均变红。
- README 无需更新。

## Technical Approach

- 形态判定：仅调整页序段长度条件，折叠与主干逻辑不变。
- 过滤条件：在调用侧加 `samePathFold` 判断，不改 `recordWithoutRetainedNames` 本身。
- 守卫：owns 分支复用 `firstUnownedReplaceCoreArtifactName`，删除 `firstReplaceCoreArtifactName` 若无其他调用点。

## Decision (ADR-lite)

**Context**：守卫与产物形态判定是同一事实的两处表达；只改一处会让文件从「拒绝」滑向「按名删除」。

**Decision**：`isReplaceArtifactName` 与 `isReplaceCoreArtifactName` 共享 ≥2 位页序口径；retained 过滤按目录生效；守卫统一认归属凭证。

**Consequences**：1 位页序文件在所有路径均不被视为产物；异目录 retained 不再丢失；锁定文件重试恢复可用。

## Out of Scope

- 不改 source/cache `removeSplitArtifactsBeyond` 的页序解析与 mtime 守卫（source 形态判定，非本次范围）。
- 不引入核心临时目录写方案。
- 不为 v11 前记录回填 retained。

## Technical Notes

- 改动点：`service/assets_split.go`（isReplaceArtifactNameExact、两处 retained 过滤调用、owns 分支守卫）。
- 测试：`assets_split_replace_fingerprint_test.go`、`assets_split_retained_test.go`、`assets_split_failure_semantics_test.go`（位数表驱动）。
- 文档：三处。
