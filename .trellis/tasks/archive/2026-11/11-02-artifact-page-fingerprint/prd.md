# 拆分产物页级指纹（P2）

## Goal

`cleanupSplitArtifactsInDirByPage` 现靠「记录主干 + 页序 + `mtime > split_at + 2s`」判定旧产物。用户文件若与产物同主干、同页序且 mtime 早于 `split_at`（复制、解压、手工改时间），仍会被删除，且不可恢复。

需引入页级指纹，使「是否为该记录产物」成为可验证事实，而非启发式。

## Requirements

1. **先 brainstorm 出 2-3 个方案**，至少覆盖：
   - a) `split_records` 增页级清单（页号/文件名/大小/哈希）JSON 列；
   - b) 独立表 `split_artifacts`（source_rel_path, page, name, size, hash）；
   - c) 仅存产物页内容 hash（重算成本与 OOXML 确定性评估）；
   - d) 维持现状（说明为何风险可接受）。
   每个方案给：误删率、漏清率、迁移成本、重拆/重链/目录迁移的同步点、失败回滚复杂度。
2. 推荐方案须明确：
   - schema 版本（v10）与迁移路径（列存在性预判 + 单事务）；
   - 写入时机（拆分成功后）；
   - 清理判定改用指纹的顺序（指纹缺失时的回落策略）；
   - 重拆页数变化、产物改名（大小写）、目录迁移、备份还原时的指纹一致性；
   - 指纹计算算法（页文件全文 SHA-256 或大小+mtime 组合）与性能预算。
3. 测试：指纹命中删除、指纹不符保留、指纹缺失回落、迁移/重链/还原后仍命中。
4. 文档：`docs/spec/pptx-split.md §9.5`、`docs/spec/assets-management.md` schema 章节。

## Acceptance Criteria

* [ ] brainstorm 结论写入 PRD（ADR），并经用户确认。
* [ ] 用户自建同主干同页序文件（任意 mtime）不再被删。
* [ ] 正常旧产物照常清理，无漏清回归；`go test ./...` 与覆盖率门禁通过。

## Out of Scope

* 不做页内容预览/缩略图联动改造。
* 不改变现有 mtime 守卫的回落地位（指纹缺失时仍作为回落）。
* 不向前端暴露指纹字段，`model.AssetSplitRecordView` 与 `frontend/wailsjs/` 零改动。
* 不新增「按页产物审计/统计」查询能力（独立表的规范化收益在本任务用不上）。

---

## ADR-lite：拆分产物归属判定改为页级指纹

- **状态**：已确认（2026-11-02 用户选定方案 a）
- **日期**：2026-11-02
- **依据**：[research/page-fingerprint.md](research/page-fingerprint.md)（代码逐行核实）

### 1. 决策驱动

1. 现行判定只依赖文件名形态与 mtime，用户可复制/解压保留旧 mtime，误删不可恢复。
2. 误删面实际有 **3 处**，只改任务点名的 `cleanupSplitArtifactsInDirByPage` 无法满足验收「任意 mtime 不删」：
   - `cleanupSplitArtifactsInDirByPage`：改名重链 / 跨落点 / 同目录改主干；
   - `removeSplitArtifactsBeyond`：普通同目录同主干重拆（页数减少、历史 `%02d` 形态）；
   - `cleanupStaleReplaceArtifacts`：replace 就地替换后删除本次新页之外的旧页。
3. 失效方向必须是「漏清不误删」：归属不可验证时保留，交人工清理。

### 2. 方案对比（摘要）

| 指标 | a) JSON 清单列 | b) 独立表 | c) 仅存 hash | d) 维持 mtime |
|---|---|---|---|---|
| 误删率 | 极低（name+size+sha 全等） | 极低（同 a） | 极低，但会删不同名字节副本 | **非零可达** |
| 漏清率 | 低 | 低，但多一套同步面 | 低-中（缺 name/size 预筛） | 低 |
| 迁移成本 | 低（单列 ALTER） | 中（建表+外键/级联+写路径事务） | 低（若 JSON 列） | 零 |
| 同步点 | 少（随记录行） | 多（键改写/删除/级联逐点验证） | 同 a 或 b | 无 |
| 失败回滚 | 低（同一条 upsert） | 中（多语句事务） | 低/中 | 无新增，但误删不可逆 |
| 性能 | 一次 O(总产物字节) 哈希 | 同 a + N 行写入 | 哈希面更大 | 零新增 |

### 3. 推荐决策

**采用方案 a（`split_records` 增 `artifact_fingerprints` JSON 列），判定核心吸收方案 c 的全文 SHA-256；`name` 与 `size` 作为保守守卫与快速预筛。**

每页存 `{page, name, size, sha256}`，删除条件四者全等（`name` 经 `sameNameFold` 比较）。理由：

1. 哈希把归属从启发式变为可验证事实，不同内容用户文件无论 mtime 均不删。
2. 改动面最小：单列 ALTER + 单条 upsert，与 v9 `artifact_stem` 同型；不引入新表、外键、级联与跨语句事务。
3. 同步点天然收敛：JSON 随 `split_records` 行；重链、目录迁移、记录删除、备份还原、重建索引均无需额外同步。
4. 规模匹配：一源一条记录、页数几十到几百，JSON 15–50 KB；无按页 SQL 查询需求。
5. 失败语义简单：指纹与记录同一条语句原子写入；哈希失败降级空指纹，不引入半态。

**不选 b**：当前无按页查询需求，外键/级联与三处删除点的同步验证成本不抵收益。
**不单用 c**：省下的存储可忽略，却失去 name/size 预筛，且会删除不同名的字节副本（扩大删除面）。
**不维持 d**：不满足验收，误删不可恢复；仅保留为指纹缺失时的回落。

### 4. Schema v10

```sql
ALTER TABLE split_records ADD COLUMN artifact_fingerprints TEXT NOT NULL DEFAULT '';
```

- 值：JSON 数组，按页序升序，每项 `{"page":1,"name":"讲义-001.pptx","size":7340032,"sha256":"<64位小写hex>"}`。
- `name` 只存文件名（basename，不含目录）；`output_dir` 提供目录。
- 存量行默认空串 = 无指纹，清理侧回落 mtime 守卫。
- **禁止自动回填**：现目录文件可能已被用户替换，回填会把用户文件认证为产物，后续清理即误删。指纹只能由成功拆分写入。

### 5. 迁移路径

1. `assetsSchemaVersion` 由 9 升至 10。
2. 新增 `migrateSchemaV10`：`pragma_table_info('split_records')` 列存在性预判 + `ALTER` + `PRAGMA user_version = 10`，全程单事务（与 v8/v9 同型，半迁移库可重开）。
3. `migrate()` 增加 `v == 10` 专用分支，避免半迁移撞 `duplicate column name`。
4. `assetsMigrations[10]` 保留迁移 SQL 作语义对照，不从迁移循环直接执行。
5. 备份还原：`split_records` 随 db 文件整体替换；v9 备份还原后由 `migrate()` 补空列。**不将新列加入 `verifyBackupDatabase` 核心表/列清单**，否则旧备份被误拒。

### 6. 数据模型与读写

```go
type splitArtifactFingerprint struct {
    Page   int    `json:"page"`
    Name   string `json:"name"`   // 拆分时产物文件名；重链/目录迁移不重写
    Size   int64  `json:"size"`
    SHA256 string `json:"sha256"` // 全文小写 hex
}
```

- `assetSplitRecord` 增 `fingerprints []splitArtifactFingerprint`。
- `getSplitRecord` / `splitRecordByOutputDir` 增列读取；JSON 解析失败按空指纹处理并 `Warn`，不返回错误（保持查询宽容语义）。
- `upsertSplitRecord` 增形参，内部 `json.Marshal`；SQL 增列与 `DO UPDATE SET artifact_fingerprints = excluded.artifact_fingerprints`。
- 指纹不进入任何 Wails 导出结构，前端零改动。

### 7. 写入时机

| 落点 | 时机 | 位置 |
|---|---|---|
| source / cache | 核心成功、旧产物清理完成后，与记录同一条 upsert 写入 | `assets_split.go:523` |
| replace 删源失败补写 | 逐页 rename + Stat 通过后计算，与补写记录同一条语句 | `assets_split.go:849` |
| replace 成功 | 同一份指纹与成功记录同一条语句 | `assets_split.go:878` |

- 页序取产物名解析结果，解析失败回退数组下标 + 1。
- 任一页 `hashFileSHA256` 失败：`Warn` 并整体写空指纹（降级为 mtime 回落），**不回滚**已成功的拆分与产物——产物可能已落盘甚至已删源，记账失败不应丢成果。
- 失败/取消路径不写记录，旧记录与旧指纹保持原样，被还原的旧产物仍与旧指纹一致。

### 8. 清理判定顺序（三处收口）

统一原则：**指纹非空时，指纹是 allow-list；不在指纹中或校验不符的文件一律保留。指纹为空/解析失败/页条目缺失时，回落现有 mtime 守卫。**

#### 8.1 `cleanupSplitArtifactsInDirByPage`（改名重链 / 跨落点 / 改主干）

1. 指纹非空：逐条指纹（`1..pageCount`）：
   - 校验 `name` 为纯 basename（`filepath.Base(name) == name`，非 `.`/`..`），防脏数据路径穿越；
   - `Lstat`：不存在跳过；非常规文件（符号链接/目录/设备）跳过并 `Warn`；
   - `size` 不等 → `Debug` 保留；重算 `sha256` 不等 → `Debug` 保留；
   - 四条件全等 → `removeSplitArtifactFile` 删除。
   - 指纹路径不再走 `collectSplitArtifactsByPage` + `dominantSplitArtifactStem`（指纹即 allow-list，天然覆盖 source/replace 两种命名形态，也消除「目录基名不匹配」「同页多候选」不可判定分支）。
2. 指纹为空：保持现有主干锚定 + mtime 守卫路径不变。
3. 删除后联动 `deleteIndexRowsForPaths`；目录删空后删除（非空保留）。

#### 8.2 `removeSplitArtifactsBeyond`（普通重拆 / 页数减少 / 历史 `%02d`）

- 传入旧记录指纹（同时保留 `splitAt` 供回落）。
- 指纹非空：候选文件须命中同页指纹且 `size + sha256` 相等才删；不在指纹中的候选（用户文件）一律保留。
- 指纹为空：保持现有形态 + mtime 守卫。

#### 8.3 `cleanupStaleReplaceArtifacts`（replace 陈旧页）

- 传入旧记录指纹。
- 指纹非空：`oldArtifacts` 中的名字只有在指纹中存在且 `size + sha256` 相等时才删；不在指纹中的名字视为用户文件保留。
- 指纹为空：保持现有按名字删除行为（兼容 v10 前记录）。

### 9. 指纹缺失回落

- 空串、JSON 解析失败、或条目数与 `page_count` 不一致（视为损坏）→ 整体按无指纹处理，走 `isSplitArtifactAfterSplitAt`。
- 有指纹但某页无条目 → 该页不是本记录产物，**保留**，不回落 mtime（避免把用户文件按旧启发式删掉）。
- v10 前旧记录在用户显式重拆前仍受 mtime 启发式保护——PRD 已接受的残留窗口。

### 10. 算法选型

| 选项 | 结论 |
|---|---|
| 全文 SHA-256 | **采用**。复用 `hashFileSHA256`，流式 O(1) 内存，抗碰撞 |
| size + mtime 组合 | 不采用；mtime 用户可改，正是要消除的启发式；size 仅作预筛 |
| 部分内容哈希 | 不采用；差异可能在包中部/rels/Content_Types，截断会误判 |
| 复用 `files.hash` | 不作为唯一来源；cache 产物不入索引，索引 hash 可能滞后 |
| 拆分核心写入时顺带哈希 | 本任务不做；需改 `model.PptxSplitResult` 并同步 wailsjs 三处 |

**字节确定性依据**（代码级核实）：`writeZipAtomic` 只保留源 zip 条目的 `Modified/Method` 与顺序，不写当前时间；产物落盘后不再被程序改写。故同一次拆分写入的基线哈希长期有效。需补一个「同一 fixture 连续拆两次、逐页 SHA-256 相等」的确定性回归测试作为证据。

### 11. 同步点矩阵

| 场景 | 是否需要动指纹 | 说明 |
|---|---|---|
| 重拆（页数/主干变化） | 写入新指纹 | 清理仍用旧记录指纹 |
| 文件重链（源键改写） | 否 | `rewriteSplitRecordKeyTx` 只改键/落点 |
| 目录迁移/重命名 | 否 | JSON 不含目录；产物文件名不变 |
| replace 逐页 rename | 写最终文件名 | 指纹在 rename 完成后计算 |
| 备份还原 | 否 | 整库替换；旧库迁移补空列 |
| 库删除 / 重建索引 | 否 | 删 db 文件；重建保留 `split_records` |
| 记录删除（force/lost/前缀） | 否 | 删整行即删 JSON |

### 12. 性能预算

假设 N=20–80 页、每页 0.5–20 MB、总产物 10 MB–1.6 GB：

| 项 | 量级 |
|---|---|
| 逐页 SHA-256 | 约 0.02–5 s（0.3–2 GB/s，含 SHA-NI 差异） |
| JSON 序列化/解析 | < 1 ms（100 页约 15–25 KB） |
| 记录写入 | 单语句 < 1 ms |
| 清理指纹路径 | O(N) Lstat + 命中文件哈希 |
| 内存 | O(1)：32 KB 流式缓冲 + 一个 JSON 串 |

批量拆分按文件累计；实现后用真实素材 benchmark 校准，若不可接受再评估「核心写入时顺带哈希」。

### 13. 测试要求

- 指纹命中删除（索引行联动）。
- 指纹不符保留：同主干同页序、内容不同、mtime 早于 `split_at`。
- 指纹缺失回落：空指纹下 mtime 早删除、晚保留；解析失败按缺失。
- 三处清理路径分别覆盖：`cleanupSplitArtifactsInDirByPage`、`removeSplitArtifactsBeyond`、`cleanupStaleReplaceArtifacts`。
- 迁移/重链/目录迁移/备份还原后指纹仍命中（扩展现有 `TestRelocateAssetFile_*`、目录迁移用例）。
- 大小写：`sameNameFold` 下 Windows 折叠命中，Linux 不折叠。
- schema：v9 库升 v10；半迁移（列已存在 + `user_version=9`）可重开；新库版本钉 10；v9 备份还原放行。
- 路径穿越：指纹 `name` 含目录分隔/`.`/`..` 时拒绝且不删。

### 14. 文档同步

- `docs/spec/pptx-split.md §9.5`：补 v10 指纹列、三处清理判定顺序、缺失回落、写入时机与失败降级。
- `docs/spec/assets-management.md`：schema v10 迁移、指纹随记录行同步、备份还原零额外同步、重链/目录迁移不重写指纹。

### 15. 后果与风险

- **正面**：用户自建同主干同页序文件（任意 mtime）不再被删；正常旧产物字节不变即命中，无漏清回归。
- **残留（已接受）**：v10 前旧记录在重拆前仍走 mtime 回落；批量拆分新增一次全量哈希耗时；用户把产物字节原样复制为另一文件名时，因 `name` 守卫保留（漏清不误删）。
- **待实证**：OOXML 字节确定性需测试取证；性能需真实素材校准。
