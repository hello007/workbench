# retained 目录锚改造

## Goal

给 `retained_fingerprints` 的每个条目增加绝对目录锚（`dir`），使「清理失败未能删除、但仍属本记录历史产物」的凭证可以跨落点随记录携带，并在后续任意一次成功拆分中按自身目录重试删除。消除当前「异目录失败条目丢弃 + 人工清理」的永久残留路径。

## Requirements

* `splitArtifactFingerprint` 增加可选字段 `Dir`（JSON `dir,omitempty`），仅 retained 条目写入；`artifact_fingerprints` 语义不变（Dir 恒空，仍相对 `output_dir`）。
* 兼容旧数据：retained 条目 `dir` 为空时按 `record.output_dir` 解释（读取落库时归一或消费时回落），不清空、不回填。
* 消费 `cleanupRetainedSplitArtifacts` 改为按条目自身目录分组处理：逐条校验目录绝对路径且在库根内、文件名 basename、内容 SHA-256 全等；删除成功/文件不存在/内容不符 → 丢弃；删除失败 → 保留原条目（含 `dir`）继续携带。
* 归属查询 `ownershipFingerprintForName` 增加目录参数：retained 条目的目录必须与查询目录折叠相等才可命中；artifact 指纹保持既有口径。
* 新产物同名过滤 `recordWithoutRetainedNames` 按条目目录生效：仅剔除与**本次落点同目录**的同名条目，异目录同名条目保留。
* 合并 `mergeSplitFingerprintsByName` 改为按 `(dir, name)` 折叠去重；同名异目录条目不互相吞并。
* 删除两处调用链的「跨落点失败条目丢弃 + Warn」分支，失败条目正常续携。
* `cleanupStaleReplaceArtifacts` 产出的 retained 条目写入 `dir = outDir`。
* 目录迁移与重链同步改写锚：`rewriteSplitRecordKeyTx` 中 `oldOutDir → newOutDir` 的条目改锚；`migrateSplitRecordsTx` 前缀迁移按旧前缀重算所有位于该前缀下的锚目录。
* 同步文档：`docs/spec/pptx-split.md` §9.5/§9.7、`docs/spec/assets-management.md` §15、`service/assets_schema.go` v11 注释、`CLAUDE.md` 关键规则行（如超限则先下沉）。

## Acceptance Criteria

* [ ] 跨落点（replace → cache/source 或反向）旧落点残留删除失败后，新记录 `retained` 保留该条目且 `dir` 指向旧落点；磁盘残留保留。
* [ ] 后续任一次成功拆分可消费该跨目录条目：文件删除、索引行清理、条目自愈消失。
* [ ] 异目录同名条目与本次新产物同名时，新产物不被误删（按目录锚隔离）。
* [ ] 旧数据（无 `dir`）行为保持：按 `output_dir` 消费；存量记录可正常重拆。
* [ ] 目录级移动/重命名与文件级重链后，retained 条目的 `dir` 与新落点一致，可继续消费。
* [ ] 非法目录（相对路径/库根外）、非法文件名条目丢弃并 Warn，不触碰库外文件。
* [ ] `go test ./...`、覆盖率门禁、`gofmt`、`go vet ./...`、`GOOS=linux go vet ./service/` 通过。
* [ ] 变异验证：去掉目录匹配守卫、去掉迁移改锚、恢复异目录丢弃，对应测试分别变红。

## Definition of Done

* 单测覆盖：跨目录续携/消费、同名异目录隔离、旧格式兼容、迁移改锚、非法锚丢弃。
* 文档与代码口径一致；不新增依赖。
* 数据库 schema 版本不变（无 DDL），仅在现有 JSON 列元素内加字段。

## Technical Approach

1. **数据模型**（`assets_split_fingerprint.go`）
   - `splitArtifactFingerprint` 增 `Dir string \`json:"dir,omitempty"\``。
   - 新增 `retainedEntryDir(record, fp)`：`fp.Dir` 非空用自身，否则回落 `record.outputDir`。
2. **归属查询**
   - `retainedFingerprintForName(record, dir, name)`：在 retained 中按 `dir` 折叠相等 + 名称唯一命中。
   - `ownershipFingerprintForName(record, dir, name)`：先 artifact 指纹（兼容原口径），再 retained（带目录）。
   - 调用点补传当前操作目录（`outDir` / `dir`）。
3. **消费**（`cleanupRetainedSplitArtifacts`）
   - 签名改为 `(store, lib, record, excludeStem)`，删除 `dir` 参数。
   - 逐条：名称合法 → 锚目录合法（绝对、库根内）→ `os.Lstat` 存在 → `fingerprintMatch` → 删除；失败条目原样返回（保留 `Dir`）。
   - 删除成功的路径统一 `deleteIndexRowsForPaths`。
4. **写入**
   - `cleanupStaleReplaceArtifacts` 追加 retained 前设置 `ownership.Dir = outDir`。
   - 两处调用链删除异目录丢弃分支；`recordWithoutRetainedNames` 无条件按新落点目录过滤。
   - `mergeSplitFingerprintsByName` → 按 `(dir,name)` 去重。
5. **迁移**
   - `rewriteSplitRecordKeyTx`：读出旧 `retained_fingerprints`，把 `oldOutDir` 锚改 `newOutDir` 后写回（同事务）。
   - `migrateSplitRecordsTx`：源键命中与仅落点命中两路，对 retained 锚执行 `splitOutDirRelUnderPrefix + migratedRelPath` 前缀重写（库根外/不在前缀下保持原值）。
6. **测试**
   - 改写 `TestAssetSplitRetained_SourceCrossDirFailureDiscardedWithWarn` / `TestAssetSplitReplace_CrossDirRetainedFailureDiscardedWithWarn` 为「续携 + 可消费」。
   - 新增：同名异目录隔离、旧格式无 `dir` 兼容、迁移改锚（目录移动/前缀替换）、非法锚丢弃。
   - 更新 helper `seedReplaceRecordWithCoreFormRetained` 写入 `Dir`。

## Decision (ADR-lite)

**Context**: 异目录失败条目若原样写入新记录会错锚到新落点；此前的「丢弃 + Warn」恢复了不变式但留下永久孤儿，需人工清理。

**Decision**: 在 retained 条目内加绝对目录锚 `dir`，消费与归属查询均按条目自身目录解析；旧条目空 `dir` 回落 `record.output_dir` 保持兼容。不改列、不升 schema 版本。

**Consequences**: 跨目录残留可自动重试收敛，不再需要人工清理；JSON 元素结构变更需同步所有 retained 消费点与迁移路径，漏改会退化为错锚删除风险，故目录匹配守卫与迁移改锚须有变异测试守护。

## Out of Scope

* 前端改动。
* `artifact_fingerprints` 加目录锚（仍相对 `output_dir`）。
* 自动从目录内容反推 retained 条目；自动回填。
* 库根整体搬迁（外部移动库目录）后的绝对路径重定位。

## Technical Notes

* 关键文件：`service/assets_split_fingerprint.go`、`service/assets_split.go`、`service/assets_db.go`、`service/assets_schema.go`、`service/assets_split_retained_test.go`、`service/assets_split_review_fixes_test.go`、`service/assets_split_migrate_test.go`、`service/assets_split_container_migrate_test.go`。
* 现状：异目录丢弃分支位于 `assets_split.go:542-548`（source/cache）与 `:939-945`（replace）；`cleanupRetainedSplitArtifacts` 目录守卫在 `:2350`。
* 迁移单点：`rewriteSplitRecordKeyTx`（`assets_db.go:2549`）、`migrateSplitRecordsTx`（`assets_db.go:2661`）。
* 文档现状：`docs/spec/pptx-split.md` §9.5/§9.7 仍写「异目录失败条目丢弃 + Warn」，须改为目录锚续携。
