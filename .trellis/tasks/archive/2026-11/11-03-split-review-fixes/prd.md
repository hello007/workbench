# 0eb330b 审核发现全量修复

## Goal

修复提交 0eb330b 只读审核发现的 P1×2、P2×6、P3×2，使「replace 产物形态位数口径」与「retained 归属凭证」在全部消费点一致，消除异目录残留孤儿与用户文件误删路径，并同步文档与测试。

## What I already know

- 审核结论：无 P0；主修复方向正确；4 个新增用例变异验证成立。
- P1-1：异目录消费失败后 retained 条目被写入新落点记录，旧落点残留失去凭证（`service/assets_split.go:529-537`、`:915-924`、`:975-977`）。
- P1-2：`parseSplitArtifactName`（`service/assets_split.go:1314-1323`）replace 形态页序位数不限；指纹不可用时清理路径可删 1 位页序用户文件（`:1389`、`:1746`、`:1818`），定位路径可误绑（`:1355`、`:1287`、`:440`）。
- P2-1：`mergeSplitFingerprintsByName` 只按 name 去重，跨目录同名丢旧落点条目（`service/assets_split_fingerprint.go:208-230`）。
- P2-2：`firstUnownedReplaceCoreArtifactName` / `replaceOutDirOwnedByRecord` 的「凭证存在但内容不符」分支无 E2E；cache 到 replace 方向无 E2E。
- P2-3：`isReplaceArtifactNameAnyStem` 仍任意位数（`service/assets_split.go:2431-2441`），与 ≥2 口径分叉，影响移动拦截/批量剔除/二次拆分拒绝。
- P2-4：`cleanupSplitArtifactsByFingerprint` 删除旧落点产物失败仅 Warn，不登记 retained（`:1888-1891`）。
- P2-5：文档三处口径矛盾（`docs/spec/pptx-split.md:124`、`:183`，`docs/spec/assets-management.md:490`）。
- P2-6：seed 夹具 `sourceRemoved=false` 与真实双失败态不完全等价；已存在产物未写索引行。
- P3-1：`service/assets_split.go:1996-1998` 的 `İ` 字节长度示例与 Go simple mapping 不符。
- P3-2：页序位数表驱动缺 4 位（≥1000 页）用例。

## Assumptions (temporary)

- 真实产物页序恒 ≥2 位（历史 `%02d`、当前 `%03d`、≥1000 页 4 位），不会出现 1 位合法产物。
- 异目录 retained 删除失败属低概率（文件被占用），允许人工清理，但不得错锚到新落点。

## Open Questions

- 无（用户指示「全修」，P1-1 采用推荐路线，见 Decision）。

## Decision (ADR-lite)

**Context**：异目录消费时删除失败的 retained 条目原样写入新记录，而凭证名相对 `record.outputDir` 解析，导致旧落点残留失去凭证。两条路线：条目加目录锚（更稳，但需改 JSON 结构、消费守卫与路径合法性校验）或异目录失败条目丢弃（最小改动，恢复不变式）。

**Decision**：采用「异目录消费失败条目丢弃 + Warn」。仅在本次落点与 `record.outputDir` 异目录时生效；同目录失败条目继续续携。`mergeSplitFingerprintsByName` 因跨目录条目已在合并前丢弃，同目录场景按 name 去重仍正确。

**Consequences**：异目录删除失败从「错锚续携」改为「显式告警 + 人工清理」，与既有 `cleanupSplitArtifactsByFingerprint` 删除失败口径一致；后续如需自动重试，再引入目录锚结构。

## Requirements (evolving)

- R1：消除 retained 跨目录错锚。异目录消费失败条目丢弃并 Warn，不得以新 `output_dir` 写入；同目录失败条目继续续携。
- R2：页序位数口径单点化。`parseSplitArtifactName` 两形态、`isReplaceArtifactNameAnyStem`、`splitArtifactPageSeq` 等解析点统一 ≥2 位；1 位页序用户文件在所有删除/定位/剔除路径均不被当作产物。
- R3：跨目录同名条目不互相吞并（异目录条目在合并前已丢弃）；同目录按 name 去重保持；补断言。
- R4：补「凭证内容不符」两分支 E2E；补 cache 到 replace 跨 target E2E。
- R5：旧落点指纹清理删除失败的处理口径明确（登记或告警 + 文档），不得留无凭证残留而不自知。
- R6：同步 `docs/spec/pptx-split.md` §9.3/§9.5/§9.7、`docs/spec/assets-management.md` §15.6、`CLAUDE.md` 规则行。
- R7：修正 seed 夹具注释与保真度；补 4 位页序表驱动用例；修正 `İ` 注释示例。
- R8：全量测试、覆盖率、gofmt（本任务涉及文件）、go vet、GOOS=linux vet 通过。

## Acceptance Criteria (evolving)

- [ ] 异目录 retained 删除失败后，新记录不得携带指向旧目录的错锚条目；旧残留有明确告警或凭证。
- [ ] 1 位页序用户文件（`方案-9.pptx` / `9-方案.pptx`）在指纹不可用的清理/定位/移动/批量剔除路径均不被删除或误绑。
- [ ] `go test ./...` 两次通过；相关用例 `-count=3` 通过。
- [ ] 变异验证：恢复任意位数判定 / 恢复无条件过滤 / 守卫不认凭证，对应用例变红。
- [ ] 文档与代码口径一致，无自相矛盾句。

## Definition of Done (team quality bar)

- 单元/集成测试覆盖新增分支，变异可验证。
- `go test ./...`、`scripts/coverage-check.sh`、`gofmt`（涉及文件）、`go vet ./...`、`GOOS=linux go vet ./service/` 全绿。
- 文档与 CLAUDE.md 规则同步。
- 不引入新依赖，不改数据库 schema（除非选定方案要求且已评估迁移）。

## Out of Scope (explicit)

- 拆分核心改临时目录 + 原子移动（既有已知限制）。
- 重构整个 split_records 存储模型。
- 前端改动。

## Technical Notes

- 审核报告来源：子 agent 四路并行审核，结论见本任务 journal。
- 关键文件：`service/assets_split.go`、`service/assets_split_fingerprint.go`、`service/assets_split_retained_test.go`、`service/assets_split_replace_fingerprint_test.go`、`service/assets_split_failure_semantics_test.go`、`docs/spec/pptx-split.md`、`docs/spec/assets-management.md`、`CLAUDE.md`。
