# split_records 持久化产物主干（清理误删/漏清总收口）

## Goal

两轮审核（对象 `739d85c`、`887a1a6`）确认：旧产物清理判定在「目录基名 == 产物主干」这一启发式上仍有 1 个必然发生的漏清（replace + ≥3 位纯数字主干，`replaceArtifactBase` 追加 `-p`）和 3 个既有归属缺陷（搬迁后同目录不清理、重链探测采纳非独占目录、源→cache 切换不清理旧落点）。根因同一：**`split_records` 不存产物命名主干**，所有判定只能靠目录名/文件名反推。本任务新增 `artifact_stem` 列作为唯一权威，收口打平：清理锚定、重链探测归属校验、跨落点清理、同目录重拆清理全部按记录主干判定。

## What I already know

* schema：`split_records`（`service/assets_schema.go:141`）现有 `source_rel_path/source_hash/output_dir/page_count/split_at/target/source_removed`；当前版本 8，迁移经 `assets_db.go:117 migrate()` 按 `PRAGMA user_version` 递增，v8 走 `migrateSchemaV8`（列存在性预判 + 单事务 + 版本号同事务）。
* 产物主干来源：source/cache 落点 = `splitSourceBaseName(rel)`；replace 落点 = `replaceArtifactBase(base)`（纯数字且长度 ≥3 追加 `-p`，`assets_split.go:1403`）。落点目录 `splitSourceOutputDir` 不经 `-p`，故 replace 数字主干目录名与产物主干本来就不同。
* 写入点：`splitOneFileWithHash`（source/cache，success 后 `upsertSplitRecord`）、`splitOneFileReplace`；读取点 `getSplitRecord`；键迁移 `rewriteSplitRecordKeyTx`。
* 清理门控：source 路径 `outputToSourceDir && record != nil && !samePathFold(record.outputDir, outDir)`（`:478`）；replace 路径 `record != nil && !samePathFold(...)`（`:765`）。
* 搬迁探测 `resolveRelocatedSplitOutDir`（`assets_relink.go:263`）只验候选目录存在且首页产物形态命中，不验归属。
* 审核实测：replace `123.pptx` 改名重链后旧目录 `001-123-p.pptx`/`002-123-p.pptx` 及其 files 行永不清理；source 对照正常。

## Requirements

1. **schema v9**：`split_records` 增 `artifact_stem TEXT NOT NULL DEFAULT ''`。迁移走 `migrateSchemaV9`（列存在性预判 + 单事务 + `PRAGMA user_version = 9`），`assetsSchemaVersion` 升 9，`assetsMigrations[9]` 保留语义对照 SQL；半迁移库可重开。
2. **写入**：拆分成功后写入本次实际产物主干（source/cache = `splitSourceBaseName(rel)`；replace = `replaceArtifactBase(splitSourceBaseName(rel))`），随 `upsertSplitRecord` 落库。`getSplitRecord` 与 `assetSplitRecord` 读回。
3. **清理锚定**：`cleanupSplitArtifactsInDirByPage` 锚点 = `record.artifactStem`；为空（v9 前旧记录）回落 `{filepath.Base(dir), replaceArtifactBase(filepath.Base(dir))}` 精确匹配，再走唯一折叠。不再作为「实际不可达」的安全承诺。
4. **同目录重拆清理（M1）**：搬迁后 `output_dir == 新落点` 时仍调用清理，锚定旧 `artifactStem`，并排除本次新主干候选（`excludeStem`），防删本次新产物。
5. **跨落点清理（M4）**：source 路径去掉 `outputToSourceDir` 门控，与 replace 一致——记录 `output_dir` 与本次落点不同即清理旧落点（cache→source、source→cache 双向）。
6. **重链探测归属校验（M2）**：`resolveRelocatedSplitOutDir` 采纳候选前，校验候选目录首页产物的解析主干 == `record.artifactStem`（为空则 == `filepath.Base(record.outputDir)`）；不符不采纳。
7. **不可判定告警**：同主干同页序多候选（`ambiguous`）输出一次 Warn（含 dir/page/stem），不再静默；基名/记录主干双失配保持 Warn。
8. **文档**：`docs/spec/pptx-split.md §9.5`、`docs/spec/assets-management.md §11.3`（记录列与探测校验）、`CLAUDE.md` 关键规则行同步；删除「残余风险实际不可达」等错误绝对化表述，如实写「同主干同页序用户文件仍可能被删（低概率、可达）」。
9. **测试注释**：清理测试文件头与各处旧口径（「覆盖 ≥2 页」「按主干 + 页覆盖」）统一为记录主干锚定。

## Acceptance Criteria

* [ ] replace `123.pptx` 改名/移动后重拆：旧目录 `001-123-p.pptx`、`002-123-p.pptx` 与索引行清理干净，无重复 Warn（端到端用例）。
* [ ] 搬迁后 `output_dir == outDir` 重拆：旧主干产物清理、本次新主干产物保留（M1 用例）。
* [ ] 重链探测：候选目录首页产物主干与记录不符时不采纳 `output_dir`（M2 用例）。
* [ ] source→cache / cache→source 切换：旧落点产物与索引行清理（M4 用例）。
* [ ] v9 迁移：新库直建、v8 库升级、半迁移库（列已存在）三种路径均可打开；旧记录 `artifact_stem=''` 回落仍能清理。
* [ ] `ambiguous` 分支有 Warn 与用例。
* [ ] `go test ./...` 通过；覆盖率门禁 service ≥76% 不降；Linux/macOS 交叉编译通过。
* [ ] spec / CLAUDE 行（钩子密集档 ≤350 字符）与实现逐条一致。

## Definition of Done

* 上述用例全部落地，其中至少 4 条在回退到 `887a1a6` 实现时变红（blocker、M1、M2、M4）。
* 所有 `upsertSplitRecord` 调用点（生产 2 处 + 测试约 20 处）签名同步，无遗漏。
* 文档无绝对化错误表述。

## Technical Approach

1. `assets_schema.go`：新增 `assetsSchemaV9SQL`（ADD COLUMN 语义对照）+ `assetsMigrations[9]`；`assetsSchemaVersion = 9`。
2. `assets_db.go`：`migrate()` 对 v9 走 `migrateSchemaV9()`；`assetSplitRecord` 增 `artifactStem string`；`getSplitRecord` SELECT/Scan 增列；`upsertSplitRecord` 增 `artifactStem` 形参并写列；`rewriteSplitRecordKeyTx` 不动（主干与路径无关）。
3. `assets_split.go`：
   - 两处成功写入传本次主干（source 用 `baseName`，replace 用 `replaceArtifactBase(baseName)`）。
   - `cleanupSplitArtifactsInDirByPage(store, lib, record, dir, excludeStem)`：锚点优先 `record.artifactStem`；回落双候选；删除循环跳过 `excludeStem`；`ambiguous` 加 Warn。
   - 两处调用点统一门控：`record != nil && !samePathFold(record.outputDir, outDir)`（去掉 `outputToSourceDir`）；同目录时 `excludeStem = 本次主干`，异目录传空。
4. `assets_relink.go`：`resolveRelocatedSplitOutDir` 首页命中后解析其主干，校验记录主干（回落到 `filepath.Base(record.outputDir)`）。
5. 测试：`assets_split_cleanup_renamed_test.go` 加 4 条端到端/单元用例；`assets_db`/迁移测试补 v9；更新既有 `upsertSplitRecord` 调用点；补 ambiguous Warn。
6. 文档同步。

## Decision (ADR-lite)

**Context**：两轮审核证明「目录基名/覆盖页数」推断产物归属存在必然漏清与可达误删；`replaceArtifactBase` 使基名与产物主干合法不一致。
**Decision**：`split_records` 增 `artifact_stem`（schema v9），记录实际产物主干，作为清理锚定与重链归属校验的唯一权威；旧记录空值回落启发式保持兼容。
**Consequences**：一次性 schema 迁移与约 20 处调用点改动；根除基名启发式的必然漏清与探测误采纳；同主干同页序用户文件误删仍未根除（无产物哈希），spec 如实记录为低概率可达风险。

## Out of Scope

* 不新增产物页哈希或页级指纹（彻底解决用户同主干同页序误删需另立任务）。
* 不改 `findSplitArtifactByPage` 的单页定位语义。
* 不改前端；`ArtifactStem` 不暴露到 `model.AssetSplitRecordView`（内部诊断字段）。

## Technical Notes

* 审核来源：commit `887a1a6` 两路子代理审核（逻辑：blocker + 4 major；测试：端到端复现 blocker、回退实验表）。
* 相关 spec：`docs/spec/pptx-split.md §9.5`、`docs/spec/assets-management.md §11.3`、`docs/spec/data-atomic-write.md`（DB 无关，不涉及）。
* 迁移先例：`assets_db.go:158 migrateSchemaV8`（列存在性预判 + 单事务）。
