# replace 指纹校验审核整改

## Goal

`d69e832`（replace 备份让位前指纹校验）审核发现 2 条中危 + 8 条低危问题。本任务一次性整改：补齐测试对 SHA-256 的钉扎、修复「旧页清理失败后归属丢失致后续增页重拆永久拒绝」的边界回归、消除校验与备份之间 TOCTOU、区分失败文案、修正文档口径，并按项目平台约定拆分测试文件。

## Requirements

### R1 中危：测试钉住 SHA-256（不再只被 size 预筛拦截）

`service/assets_split_replace_fingerprint_test.go` 拒绝类用例的用户内容与旧产物改为**等长不同内容**（如 9 字节 `userbytes` vs `old-bytes`），使 `fingerprintMatch` 必须走到 SHA-256 比较；并补「同长不同哈希」正负对照（命中放行 / 不符拒绝）。

### R2 中危：旧页清理失败不再丢失归属（schema v11）

- 新增 `split_records.retained_fingerprints TEXT NOT NULL DEFAULT ''`（v11，`migrateSchemaV11` 列存在性预判 + 单事务，`assetsSchemaVersion=11`，`assetsSchemaV11SQL` 注释与新库语义对照）。
- 语义：**清理失败未能删除、但仍属本记录的历史产物凭证**，元素结构同 `artifact_fingerprints`。仅 replace 同目录旧页清理失败时写入，由后续重拆的备份让位/清理消费。
- `assetSplitRecord.retained` 读取；`upsertSplitRecord` 增 `retained []splitArtifactFingerprint` 参数并同步所有调用点（source/cache 传 nil）。
- 归属查询单点 `ownershipFingerprintForName(record, name)`：先查 `fingerprints`，未命中再查 `retained`，均要求唯一命中（多命中不可判定）；备份让位与 `cleanupStaleReplaceArtifacts` 共用。
- `cleanupStaleReplaceArtifacts` 删除失败时把该页已有归属条目（指纹或旧 retained）收进返回的 retained 列表；成功删除或文件已不存在则不保留。调用方把返回列表写进本次 upsert（删源失败补写与成功记录两处一致）。
- 下次重拆：`oldArtifacts` 命中 + `retained` 内容全等 → 允许备份让位，成功后新记录不再携带该 retained 条目（自愈）。

### R3 低危：TOCTOU 收口

备份 rename 后对备份文件复验 `fingerprintMatch(bak, fp)`；不符则尝试换回原路径并拒绝覆盖（同 R4 文案）。复验失败且换回失败保留备份并 Warn（与还原失败口径一致）。

### R4 低危：失败文案区分

- 归属存在（指纹或 retained 命中）但内容不符：`第 %d 页目标文件「%s」内容与拆分记录不一致（可能已被修改），请确认后移走或还原`。
- 无任何归属条目（用户同名文件）：沿用 `第 %d 页目标文件「%s」已存在且不属于历史拆分产物，请先移走或重命名后重试`。

### R5 低危：核心中间态同名用户文件保护

`ownsOutDir` 为真、调拆分核心**之前**检查 outDir 内是否存在 `<base>-<纯数字>.pptx` 核心中间态文件：存在即拒绝（`failed`，核心不调用、目录零改动），提示 `目录中已存在与拆分核心产物同名的文件（%s），请先移走或重命名后重试`。新增单点 `firstReplaceCoreArtifactName(dir, base)`（大小写折叠经 `sameNameFold`）。

### R6 低危：文档口径修正

- `docs/spec/pptx-split.md` §9.3 / §9.5 / §9.7、`docs/spec/assets-management.md` §15.6、`CLAUDE.md` 关键规则：区分「清理三处指纹缺失回落 mtime」与「replace 备份让位指纹缺失按名认定」；「拒绝路径零 `.replace-bak` 残留」限定为「还原成功时零残留，还原失败保留备份待人工恢复」；补 v11 retained 契约与新文案。
- 说明 Windows 折叠放行分支仅 Windows 本机可执行（CI 仅 ubuntu），属已知覆盖缺口。

### R7 低危：测试补强与平台拆分

- 失败回滚后断言还原页 `files.size/hash` 与旧值一致（索引行自愈），不只断条数。
- 折叠多命中改为端到端：记录写两条同名指纹（精确重复），经 `splitOneFileReplace` 断言拒绝与回滚。
- 平台用例拆 `assets_split_replace_fingerprint_windows_test.go` + `_other_test.go`（项目平台断言拆双文件约定；共享 fixture 留在原文件）。
- 新增：retained 往返/迁移 v11、清理失败留存归属并随后增页重拆成功、retained 自愈清空、中间态守卫拒绝且核心零调用、备份复验不符拒绝。
- 测试删除失败经注入点模拟：`AssetLibraryService` 增 `removeArtifactFn func(string) error`（默认 `removeSplitArtifactFile`，仅 `cleanupStaleReplaceArtifacts` 使用），与 `splitCoreFn`/`parseFn` 同风格。

## Acceptance Criteria

- [ ] 拒绝类用例在「同长不同内容」下仍拒绝；把 `fingerprintMatch` 的 SHA-256 比较改为恒真，拒绝类用例必须变红（变异验证）。
- [ ] 清理失败留存 `retained_fingerprints`；随后页数恢复重拆成功且 retained 清空。
- [ ] v10 库升级 v11 后列存在、默认空串；记录读写往返携带 retained。
- [ ] 备份 rename 后复验不符 → 换回、拒绝、无新产物。
- [ ] 中间态同名文件存在 → 核心零调用、文件字节不变、返回 failed。
- [ ] 两类失败文案按 R4 区分。
- [ ] 平台用例按 `_windows_test.go` / `_other_test.go` 拆分。
- [ ] `go test ./...`、`scripts/coverage-check.sh` 通过；文档三处同步。

## Definition of Done

- 单测覆盖 R1–R7；`gofmt`/`go vet` 干净。
- 文档同步且关键规则行 ≤350（钩子密集档）。
- README 无需更新（内部行为修正）。

## Technical Approach

- 迁移照 v9/v10 形态新增 v11；`upsertSplitRecord` 加参数，SQL 列与 select 列表同步。
- 归属查询集中到 `ownershipFingerprintForName`，禁止各调用点自行拼接「先指纹后 retained」顺序。
- retained 只由 `cleanupStaleReplaceArtifacts` 产出，不自动回填、不从目录内容反推。
- 中间态守卫挂在 `ownsOutDir` 分支、核心调用之前（早于任何写入）。
- 备份复验紧贴备份 rename 之后，失败走同一 `rollbackFailed`。

## Decision (ADR-lite)

**Context**：旧页删除失败后记录被新页数覆盖，残留文件脱离指纹 allow-list；若放宽 `fingerprintsUsable` 会把损坏判定与保留凭证混为一谈。

**Decision**：独立 `retained_fingerprints` 列承载「清理失败但仍属历史产物」的归属，指纹契约（条目数 = page_count）不变。

**Consequences**：多一列与一次迁移；清理失败不再导致永久拒绝；文档需说明 retained 的写入与自愈时机。

## Out of Scope

- 不新增 Windows CI job（成本高，覆盖缺口在文档记明）。
- 不改三处清理路径对 `fingerprints` 的既有 allow-list 语义。
- 不为 v11 前记录回填 retained。

## Technical Notes

- 改动点：`service/assets_split.go`（步骤 5、cleanupStaleReplaceArtifacts、中间态守卫）、`service/assets_split_fingerprint.go`（归属单点）、`service/assets_db.go`（结构、select、upsert、迁移）、`service/assets_schema.go`（v11 SQL/版本）。
- 测试：`assets_split_replace_fingerprint_test.go`、新增 `_windows_test.go`/`_other_test.go`、`assets_split_fingerprint_test.go`、迁移测试。
- 注入点先例：`splitCoreFn`、`parseFn`、`moveToRecycleBinFn`。
