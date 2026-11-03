# Research: PPT 拆分产物页级指纹选型

- **Query**：为「拆分产物页级指纹」评估 4 个方案，覆盖误删率、漏清率、迁移成本、同步点、失败回滚、性能预算，并给出推荐方案与落地草案
- **Scope**：internal（Go service + SQLite schema + OOXML 拆分核心）+ 少量外部事实（Go archive/zip 行为）
- **Date**：2026-11-02
- **任务目录**：`.trellis/tasks/11-02-artifact-page-fingerprint/`

> 本文只陈述已核实的代码事实与基于代码的推算。凡无法从代码或文档直接确认的，均标注「未确认」并写明需要的证据。

---

## 0. 事实基线（已逐行核实）

### 0.1 现有清理逻辑与误删面

| 事实 | 位置 | 说明 |
|---|---|---|
| 旧产物按页清理主函数 | `service/assets_split.go:1573` `cleanupSplitArtifactsInDirByPage` | 仅在拆分成功后调用（source 落点核心返回成功后、replace 删源前） |
| 归属判定现为「记录主干 + 页序」 | `service/assets_split.go:1597-1655` | 先用 `artifact_stem` 选定产物主干组，再按页序取组内唯一候选 |
| mtime 守卫 | `service/assets_split.go:1528` `isSplitArtifactAfterSplitAt` | `mtime > split_at + 2s` 视为用户后放入文件并保留；否则删除 |
| 守卫容差 | `service/assets_split.go:1522` `splitArtifactUserFileGrace = 2 * time.Second` | 吸收文件系统时间戳粒度与拆分收尾耗时 |
| 清理决策门控 | `service/assets_split.go:1502` `splitArtifactCleanupPlan` | 异目录 / 跨 target / 同目录改主干三种形态才进入清理 |
| 超界旧页清理 | `service/assets_split.go:1412` `removeSplitArtifactsBeyond` | 普通同目录同主干重拆时，页数减少或历史 `%02d` 形态的旧页在此删除，同样只看形态 + mtime |
| replace 陈旧页清理 | `service/assets_split.go:1835` `cleanupStaleReplaceArtifacts` | 按拆分前 `oldArtifacts` 名字集合删除本次新页之外的旧页，不做内容校验 |

**误删面结论**：任务描述只点名 `cleanupSplitArtifactsInDirByPage`，但同类误删面实际有 **三处**：

1. `cleanupSplitArtifactsInDirByPage`：改名重链 / 跨落点切换 / 同目录改主干；
2. `removeSplitArtifactsBeyond`：普通同目录同主干重拆（页数减少、历史 `%02d`）；
3. `cleanupStaleReplaceArtifacts`：replace 就地替换成功后删除本次新页之外的旧产物。

只修第 1 处无法满足 PRD 验收「用户自建同主干同页序文件（任意 mtime）不再被删」——用户把 `讲义-999.pptx` 放进普通重拆目录且 mtime 早于上次 `split_at` 时，第 2 处仍会删除。

### 0.2 拆分记录与 schema

| 事实 | 位置 |
|---|---|
| 当前 schema 版本 9 | `service/assets_schema.go:21` |
| `split_records` v4 定义（`source_rel_path` 主键、`source_hash`、`output_dir`、`page_count`、`split_at`） | `service/assets_schema.go:141` |
| v8 增 `target` / `source_removed` | `service/assets_schema.go:248` |
| v9 增 `artifact_stem` | `service/assets_schema.go:265` |
| 迁移入口按 `PRAGMA user_version` 逐级执行 | `service/assets_db.go:117` |
| v8 / v9 走「列存在性预判 + 单事务」 | `service/assets_db.go:166` / `:203` |
| 单连接池与 `foreign_keys(1)` DSN | `service/assets_db.go:43` |
| 记录结构 | `service/assets_db.go:2333` `assetSplitRecord` |
| 读记录 | `service/assets_db.go:2347` `getSplitRecord` |
| 写记录（单语句 upsert） | `service/assets_db.go:2407` `upsertSplitRecord` |
| 键改写单点 | `service/assets_db.go:2458` `rewriteSplitRecordKeyTx` |
| 目录级迁移 | `service/assets_db.go:2570` `migrateSplitRecordsTx` |
| 记录删除点 | `assets_db.go:639`（force 重链）、`:876`（lost 清理）、`:2668`（目录前缀删除） |

### 0.3 写入点与产物命名

| 落点 | 写入点 | 产物名 |
|---|---|---|
| source / cache | `service/assets_split.go:523`（核心成功后） | `<源名主干>-NNN.pptx` |
| replace（删源失败补写） | `service/assets_split.go:849` | `<NNN>-<名字主干>.pptx` |
| replace（成功） | `service/assets_split.go:878` | `<NNN>-<名字主干>.pptx` |

- 核心生成：`service/pptx_split.go:41` `SplitPptxPerPage`，主循环 `:133-156`，输出名 `:140`。
- replace 的逐页 rename、备份与回滚：`service/assets_split.go:703-742`。
- 产物定位单点 `splitArtifactPath`：`service/assets_split.go:1185`；命名解析 `parseSplitArtifactName`：`:1212`。

### 0.4 OOXML 产物字节确定性（关键）

**代码事实**：

- `writeSplitOutput` 只修改 `presentation.xml`、rels、`[Content_Types].xml`、保留母版 XML 的指定片段，其余 part **逐字节原样拷贝**（`service/pptx_split.go:668-735`）。
- 重写 zip 时按源包条目顺序输出（`:723-733`），每条 header 只设置 `Name`、`Method`、`Modified`（`:752`）。
- `Modified` 直接来自源 zip 条目（`readZipPackageWithLimits` 保存 `f.Modified`，`:224`），**不写入当前时间**。
- Go `archive/zip.Writer.CreateHeader` 在 `Modified` 非零时，用该时间生成 DOS 时间并追加 extended timestamp extra（Go 1.26.10 `src/archive/zip/writer.go:306-331`），没有随机盐或当前时钟。
- 落盘为 `CreateTemp -> Write -> Sync -> Rename`（`service/pptx_split.go:740-777`），不改变字节内容。

**结论**：同一份源 pptx（字节相同）在相同核心代码下重复拆分，产出的单页 pptx **字节稳定**。因此「先存哈希、清理时重算比对」在「产物落盘后不再被程序改写」的前提下成立。

**边界**：

- 源 pptx 被 PowerPoint/WPS 重新保存（即使只改元数据）后，源哈希变化，产物字节也变化；这属于「另一次拆分」，不能拿旧指纹去要求新产物匹配。
- 若未来核心改为「只重写变化页」或改写 zip header 策略，仍在原地复用的旧产物必须同步更新指纹；当前核心每次重拆都会重写页序 ≤ 新页数的产物，不依赖该假设做删旧。
- 未做实测。建议补一个确定性回归：同一 fixture 连续拆分两次，断言各页 SHA-256 相同；`content` 与 `mtime` 分离断言。所需证据：一次 `go test` 实测结果。

### 0.5 备份与还原

- 备份/还原是**整个 SQLite 文件替换**（`service/assets_backup.go` 还原链路；`verifyBackupDatabase` 在 `:451`）。
- 备份预检只要求 `files` 与 `split_records` 两核心表同时存在（`:465-479`），版本高于当前拒绝（`:461`）。
- `split_records` 随 db 文件整体迁移；还原旧版本备份后由 `migrate()` 逐级升级。

### 0.6 现有哈希能力与产物索引关系

- 现成 SHA-256：`service/assets_scan.go:1108` `hashFileSHA256`（流式 `io.Copy`，hex 小写）。
- source / replace 落点产物在库内，可能已有 `files.hash`；cache 落点在 `data/assets/<库>/derived`（库根外），不入索引。**不能假设所有产物都有索引哈希**。
- replace 产物在 `processScanFile` 后会写 `files.hash`（`service/assets_split.go:775-790`），但那是索引字段，可能因后续操作变化；指纹应以「落盘字节」为准，不直接依赖 `files.hash`。

### 0.7 文件重链与目录迁移对记录的影响

- `rewriteSplitRecordKeyTx` 只改 `source_rel_path` 与可选 `output_dir`，**不改 `artifact_stem`**（`service/assets_db.go:2458-2517`）。
- `migrateSplitRecordsTx` 目录迁移同样只改源键与落点（`:2570-2665`）。
- 文件级重链的产物容器随源搬迁例外见 `resolveRelocatedSplitOutDir`（`docs/spec/pptx-split.md §9.5`），也只改 `output_dir`，不改产物文件名。
- 因此：**产物文件名在重链/目录迁移后保持不变**；若指纹存 `name`，只要「name 记录的是拆分时产物名、迁移时不重写 name」，就仍然与磁盘一致。任务描述中「重链后按 name 精确匹配会漏清」的担心，只在实现把 name 改成新源主干时成立；正确实现不应改它。

---

## 1. 结论摘要表

评分口径：高 = 风险/成本高；中 = 可控但需要额外工作；低 = 风险/成本低。误删率、漏清率按「单次清理事件」评估，未做现场统计，除代码可证明的边界外不虚构百分比。

| 指标 | a) `split_records` 增页级 JSON 清单 | b) 独立表 `split_artifacts` | c) 仅存页内容 SHA-256 | d) 维持 mtime 现状 |
|---|---|---|---|---|
| **误删率** | 极低：仅当「页+名+size+sha256」全等才删；精确命中 = 字节相同副本；哈希碰撞 ≈ 2^-256 | 极低（若同样以 hash 判定）；若只按 name 判定则不达标 | 极低（hash 判定）；若只按页序+hash 扫描，误删面仍取决于「是否允许删除不同名的字节副本」 | **非零且可达**：同主干同页序且 `mtime <= split_at+2s` 即删；复制/解压保留 mtime 可触发；不可恢复 |
| **漏清率** | 低：正常旧产物字节不变 → 命中；触发漏清的条件见 §2.1 | 低（实现正确时同 a；多一套同步面会放大实现遗漏风险） | 低-中：hash 稳，但若按 name 定位，改名/大小写变体会漏；若遍历全部候选则哈希面扩大 | 低：正常产物 mtime 恒早于 `split_at`；但以误删为代价 |
| **迁移成本** | 低：单列 `ALTER` + 列存在性预判 + 单事务；存量行默认空串；无 Go 回填 | 中：建表 + 外键/级联策略 + 写路径显式事务 + 键改写/删除级联验证 | 低（JSON 列）/中（独立表） | 零 |
| **同步点覆盖** | 少：写路径 3 处加字段；键改写/目录迁移/删除/备份恢复均自动跟随 | 多：写路径事务、`rewriteSplitRecordKeyTx`、`migrateSplitRecordsTx`、3 处删除、目录前缀删除、外键级联均需验证 | 同 a 或 b（取决于存法） | 无新增同步点 |
| **失败回滚复杂度** | 低：与记录同一单语句 upsert，无额外事务；哈希失败可降级空指纹 | 中：upsert + 删旧行/插新行需同事务；冲突删除/键更新依赖级联，需事务回滚测试 | 低/中（同存法） | 无新增，但误删不可逆 |
| **性能预算** | 新增 O(总产物字节) 一次哈希 + O(N) JSON；清理 O(N) 指纹 + 命中文件哈希；内存 O(1) | 同 a，另加 N 行 DB 写入（量级 < 数 ms） | 同 a；若不带 name/size，负向预筛变弱，可能哈希更多候选 | 零新增 |

**推荐：方案 a（`split_records` 增 JSON 列）并吸收方案 c 的内容哈希判定**。即 JSON 每页存 `page / name / size / sha256`，删除判定以 `size + sha256` 为权威、`name` 作为保守守卫与候选定位；迁移时**不重写 name**。理由见 §3。

---

## 2. 逐方案详述

### 2.1 方案 a：`split_records` 增页级清单 JSON 列

**存储形态**

```sql
ALTER TABLE split_records ADD COLUMN artifact_fingerprints TEXT NOT NULL DEFAULT '';
```

```json
[
  {"page":1,"name":"讲义-001.pptx","size":7340032,"sha256":"9f2c...e1"},
  {"page":2,"name":"讲义-002.pptx","size":8123456,"sha256":"31ab...07"}
]
```

- `name` 只存**文件名**，不存目录；目录由 `output_dir` 提供，目录迁移不需要改指纹。
- 页序升序；`page` 与记录 `page_count` 对齐；不存 `mtime`（mtime 可被用户任意修改，不是内容事实）。
- 存量行默认 `''` = 缺失指纹，回落现有 mtime 守卫。

**误删率**

- 删除条件（推荐）：`page` 命中、`name` 经 `sameNameFold` 命中、`size` 相等、`sha256` 相等。
- 精确命中：用户把旧产物原样复制/改名回来且名字也相同——内容与产物字节完全一致，语义上就是产物副本；删除可接受。
- 哈希碰撞：SHA-256 抗碰撞，实际概率 2^-256；且同时要求 size 相等。
- 不同内容的用户文件：size 或 hash 必有一项不同，保留。
- 结论：对「用户自建同主干同页序文件（任意 mtime）」验收成立。

**漏清率与触发条件**

指纹存在时，正常旧产物（落盘后未被程序改写、未被用户改名/改内容）hash 必然命中；漏清只在这些条件触发：

1. 用户在两次拆分之间打开旧产物并保存，字节变化 → hash 不符 → 保留（安全方向）；
2. 用户手工重命名旧产物（不再等于记录 name）→ 按 name 定位失败 → 保留（安全方向）；若改用「页序+hash、name 仅诊断」可覆盖，但会扩大到「不同名的字节副本」删除面，见 §2.3；
3. 记录为 v10 前旧记录（空指纹）→ 回落 mtime，等价现状；
4. 指纹写入失败（哈希失败 / DB 写失败）→ 空指纹 → 回落；
5. 指纹 JSON 损坏/解析失败 → 按缺失处理 → 回落；
6. 文件被占用导致删除失败 → 保留（现有重试一次语义）；
7. 产物被外部工具截断/替换 → hash 不符 → 保留。

**迁移成本**

- `assetsSchemaVersion` 9 → 10；`migrateSchemaV10` 单列、列存在性预判、单事务（与 v9 同型）。
- 存量行 `DEFAULT ''`，无需 Go 回填；新记录首次拆分自然填充。
- 半迁移可重入：列已加、`user_version` 未落时，重开走列存在性分支后提交版本。
- 新库：v4 建表 → v10 补列，逐级执行。
- **不能安全回填**：现目录内的文件可能已被用户替换；把当前字节回填为「产物指纹」会把用户文件认证为产物，后续清理误删。回填只能由用户显式重拆产生。代价：v10 前已拆分的文件在用户重拆前仍保留 P1 窗口（PRD Out of Scope 已接受 mtime 回落）。

**同步点覆盖**

| 场景 | 是否需额外同步 | 落点 |
|---|---|---|
| 重拆（页数增减 / 主干变化） | 需要：写新指纹 | `assets_split.go:523`、`:849`、`:878`；旧记录在写入前仍用于清理 |
| 文件重链（源键改写） | 不需要：JSON 随行 | `rewriteSplitRecordKeyTx` 只改键/落点；force 重链/ lost 清理删行时指纹随行删除 |
| 目录迁移 / 重命名（`output_dir` 改写） | 不需要：指纹不含目录 | `migrateSplitRecordsTx` 不改指纹 |
| replace 逐页 rename / 回滚 | 需要：指纹按最终 `renamed` 文件名计算 | 写记录前计算；回滚时旧记录未变，旧指纹仍描述被还原的旧产物 |
| 备份还原 | 不需要：整库文件替换 | 旧版本备份还原后 `migrate` 补空列 |
| 库删除 / 重建索引 | 不需要 | 删库删 db 文件；重建只清 `files/slides/FTS`，`split_records` 保留 |
| 记录删除（force / lost / 目录前缀） | 不需要 | 删除整行即删除 JSON |

**失败回滚复杂度**

- 写入与记录同一条 `INSERT ... ON CONFLICT DO UPDATE`，单语句原子，不需要额外事务/临时表。
- 唯一新增失败面是「哈希计算失败」：建议降级为空指纹 + `Warn`（产物已成功落盘，不应因指纹记账失败整体回滚），下次清理回落 mtime。
- 崩溃窗口与现状相同：产物先落盘、记录后写。若在两者之间崩溃，旧记录/旧指纹仍在；新产物 hash 不符 → 保留（漏清），不会误删。

**性能预算**

- 哈希：流式读全部新产物，O(总字节)。Go `crypto/sha256` 在 x86-64 有 SHA-NI 时约 1–2 GB/s，无硬件加速时约 0.3–0.5 GB/s（量级估算，未实测）。
- 典型：20–80 页、每页 0.5–20 MB，总产物 10 MB–1.6 GB → 约 0.02–5 s；刚写完的文件通常仍在页缓存，额外磁盘读少，CPU 为主。
- JSON：每页约 150–250 字节（中文名 + 64 位 hex），100 页约 15–25 KB；解析/序列化 < 1 ms。
- 清理：O(N) 指纹遍历 + Lstat + 命中文件哈希；不再全目录解析主干，复杂度不高于现状。
- 内存：32 KB 流式缓冲 + 一个 JSON 字符串，常量级。

---

### 2.2 方案 b：独立表 `split_artifacts`

**存储形态**

```sql
CREATE TABLE IF NOT EXISTS split_artifacts (
    source_rel_path TEXT    NOT NULL REFERENCES split_records(source_rel_path)
                            ON DELETE CASCADE ON UPDATE CASCADE,
    page            INTEGER NOT NULL,
    name            TEXT    NOT NULL,
    size            INTEGER NOT NULL,
    sha256          TEXT    NOT NULL,
    PRIMARY KEY (source_rel_path, page)
);
CREATE INDEX IF NOT EXISTS idx_split_artifacts_rel ON split_artifacts(source_rel_path);
```

**误删率**：若删除判定同样用 `size + sha256`，与 a 相同；若只按 `name` 判定则不达标（同主干同页序、不同内容仍会删）。**必须带 hash**。

**漏清率**：实现正确时与 a 相同；但多一套「记录 ↔ 子表」同步面，任何一处漏同步都可能产生漏清或误删：

- `upsertSplitRecord` 的 `ON CONFLICT DO UPDATE` 不会触发子表更新，必须在同一事务内先 `DELETE FROM split_artifacts WHERE source_rel_path=?` 再逐行 `INSERT`；
- `rewriteSplitRecordKeyTx` 的 `UPDATE split_records SET source_rel_path=?` 依赖 `ON UPDATE CASCADE`；冲突分支的 `DELETE` 依赖 `ON DELETE CASCADE`；
- `deleteSplitRecordsByPathPrefix` 的多行 `DELETE` 依赖级联；
- 若外键未声明或 DSN 未启用，所有这些点都要手写同步，漏一处即脏数据。

**迁移成本**：中。建表本身简单，但需要：

- v10 迁移建表 + 索引；
- 写路径改成显式事务（upsert + 清/插子表），与现有「单语句 upsert」口径不同；
- 验证 `modernc.org/sqlite v1.60.1` 下 `ON UPDATE CASCADE` / `ON DELETE CASCADE` 行为（**未实测**；SQLite 文档支持，但本项目未用过该级联形态，需单测取证）；
- 备份预检是否要求新表：**不应**加入 `verifyBackupDatabase` 核心表清单，否则 v10 之前的合法备份会被拒；缺表时迁移会创建空表，指纹退化为空。

**同步点覆盖**

| 场景 | 需要做什么 |
|---|---|
| 重拆 | 事务内 upsert 记录 + 删旧子表行 + 插新行 |
| 文件重链 | 依赖 `ON UPDATE CASCADE`；冲突删除依赖 `ON DELETE CASCADE` |
| 目录迁移 | 只改 `output_dir`，子表无需动；若源键变，依赖级联 |
| replace rename / 回滚 | 指纹按最终文件名写；回滚不改记录则子表不变 |
| 备份还原 | 整库替换；旧备份迁移建空表 |
| 库删除 / 重建 | 删库删文件；重建保留表 |
| 记录删除 | 依赖 `ON DELETE CASCADE`（三处删除点逐一验证） |

**失败回滚复杂度**：中。多语句写必须事务；事务失败时记录与子表要么同时回滚、要么同时提交。`upsertSplitRecord` 现有调用点分布三处，改签名/事务边界影响面大于 a。级联冲突删除（保留 `source_removed=1` 的一条）会连带删子表行，需测试确认「旧行子表随删除消失、胜出行子表保留」。

**性能预算**：与 a 同量级的哈希成本；额外 N 行 DB 写入（100 页 ≈ 100 行，单事务 < 数 ms）；查询按页可走主键，但当前清理是整组读取，优势不明显。

---

### 2.3 方案 c：仅存页内容 SHA-256

**存储形态（二选一）**

- JSON 列：`artifact_hashes TEXT NOT NULL DEFAULT ''`，形如 `[{"page":1,"sha256":"..."}]`；或紧凑串 `1:9f2c...,2:31ab...`。
- 独立表：`split_artifact_hashes(source_rel_path, page, sha256)`，外键/级联问题同 b。

**误删率**：若按「页序 + sha256」匹配，不同内容的用户文件不会命中；若用户放入的是产物的字节副本（不同名），会被命中并删除。是否可接受取决于产品语义：它是「产物副本」还是「用户备份」。**建议至少保留 name 或只删除 name 相同的候选**，否则会扩大删除面。

**漏清率**：

- hash 本身稳定，正常旧产物命中；
- 但只有 hash、没有 name/size 时，清理需要枚举目录内所有形态匹配候选并逐个读取计算 hash（无法先用 name 精确命中，也无法用 size 快速排除），哈希面从「N 个预期文件」扩大到「目录内所有同页序候选」；
- 若为了省哈希而只按 name 定位，则 name 改名/大小写变体导致漏清（见下）；
- 若按页序 + hash 定位，则可覆盖「产物被改名」的场景，但会删除不同名的字节副本。

**关于「重算比对」是否成立**

成立，但前提是**必须存基线哈希**：

- 拆分时对每个落盘页文件算 SHA-256 存入记录；
- 清理时对候选文件重算 SHA-256，与记录中该页的基线比较；
- 「重算」不能替代存储：没有基线就没有比对对象；
- 也不能在清理时「重新拆分源文件再比对」：replace 源可能已进回收站，且成本远高于读产物。

字节稳定性依据见 §0.4：同一源 + 同一核心，产物字节稳定；产物落盘后不被程序改写，故基线长期有效。

**迁移成本**：JSON 列 = 低（同 a）；独立表 = 中（同 b）。

**同步点覆盖**：取决于存法，同 a 或 b。

**失败回滚复杂度**：JSON 列 = 低（同 a）；表 = 中（同 b）。

**性能预算**：JSON 每页约 80–90 字节（64 hex + JSON 开销），比 a 少；但由于缺少 name/size 预筛，清理时可能哈希更多候选，实际 CPU 成本可能高于 a。若同时保留 size，则接近 a。

**与「name 在重链后变化」的关系**

- 已核实：重链/目录迁移**不改产物文件名**，记录 name 若按拆分时写入则仍与磁盘一致；因此「name 会漏清」不是必然，而是「实现错误地重写 name」才会发生。
- 推荐设计：name 记录**拆分时产物名**，迁移/重链不改；清理时 name 作为保守条件。若担心用户改名，可增加「name 不匹配但 page+size+hash 匹配」的二级判定，但必须明确这会删除改名后的字节副本——本报告建议不做，保持「漏清但不误删」。

---

### 2.4 方案 d：维持 mtime 现状

**误删率**：非零、可达。触发条件：

1. 用户文件位于拆分产物目录；
2. 文件名符合当前记录的产物形态（同主干、同页序、`.pptx`）；
3. `mtime <= split_at + 2s`（复制/解压保留旧 mtime、手工改时间、从备份还原）；
4. 该文件与产物同页序且同主干组内唯一。

满足即删除且不可恢复。无法量化概率（取决于用户行为），但 PRD 已将其定性为 P1 数据丢失风险，且「任意 mtime」验收明确不满足。

**漏清率**：低。正常产物在记录写入前落盘，mtime 必然早于 `split_at`，`isSplitArtifactAfterSplitAt` 返回 false，照常删除。`Stat` 失败按不可判定保留，`splitAt` 解析失败守卫不生效（保持旧行为）。

**迁移成本**：零。

**同步点覆盖**：无新增。

**失败回滚复杂度**：无新增；但误删不可逆，且删除后记录主干/落点被新值覆盖，残留页不再被任何记录引用，无法自动收敛。

**性能预算**：零新增。`Stat` 成本 O(N)，远低于哈希。

**结论**：不满足 PRD 验收，不能作为最终方案；只能作为指纹缺失时的回落路径。

---

## 3. 推荐方案与理由

### 3.1 推荐

**方案 a（`split_records` 增 `artifact_fingerprints` JSON 列），判定核心采用方案 c 的全文 SHA-256；`name` 与 `size` 作为保守守卫与快速预筛。**

即：

- 每页存 `{page, name, size, sha256}`；
- 删除条件：`page` 命中 + `name`（`sameNameFold`）命中 + `size` 相等 + `sha256` 相等；
- 指纹缺失（空串 / 解析失败 / v10 前旧记录）→ 回落现有 mtime 守卫；
- 重链/目录迁移**不重写指纹中的 name**。

### 3.2 理由

1. **数据正确性**：hash 把「是否为本记录产物」从启发式变成可验证事实；不同内容的用户文件无论 mtime 如何都不会被删，直接满足 PRD 验收。
2. **改动面最小**：单列 `ALTER` + 单语句 upsert，与 v9 `artifact_stem` 的落地模式完全同型；不需要新表、外键、级联、写路径事务改造。
3. **同步点天然收敛**：JSON 随 `split_records` 行；键改写、目录迁移、记录删除、备份还原、重建索引都不需要额外同步。独立表 b 的最大代价正是这些同步面。
4. **规模匹配**：一个源文件一条记录、页数通常几十到几百，JSON 15–50 KB 完全可接受；不需要按页 SQL 查询，独立表的规范化收益用不上。
5. **失败语义简单**：指纹与记录同一条语句原子写入；哈希失败可降级空指纹，不引入新的半态。
6. **name 不重写的正确性已核实**：重链/迁移只改源键与 `output_dir`，产物文件名不变；因此 name 记录拆分时产物名是稳定的。保留 name 可避免「不同名字节副本」被误删，符合「失效方向为漏清而非误删」的既有安全侧原则。

### 3.3 不推荐 b / c 单独 / d 的原因

- **b**：当前没有按页查询/统计需求；引入外键与级联后，写路径、键改写、三处删除、目录前缀删除都要重新验证，收益不抵成本。若未来要做「按页产物审计/统计」再升级表结构。
- **c 单独**：省下的存储（每页约 100 字节）可忽略，但失去 name/size 预筛，清理哈希面扩大，且容易误删不同名的字节副本；作为 a 的判定核心吸收即可。
- **d**：不满足验收，且误删不可恢复。

---

## 4. 推荐方案落地要点草案

### 4.1 schema v10 DDL

`service/assets_schema.go`：

```go
const assetsSchemaVersion = 10

// assetsSchemaV10SQL v10 迁移：拆分记录页级产物指纹（2026-11-02）。
// 语义：JSON 数组，按页序升序；每页 {"page","name","size","sha256"}。
// 存量记录默认空串 = 无指纹，清理侧回落 mtime 守卫（禁自动回填）。
// 实际执行走 assets_db.go 的 migrateSchemaV10（列存在性预判 + 单事务）。
const assetsSchemaV10SQL = `
ALTER TABLE split_records ADD COLUMN artifact_fingerprints TEXT NOT NULL DEFAULT '';
`
```

`assetsMigrations[10] = assetsSchemaV10SQL` 仅作语义对照；`migrate()` 对 v10 走专用分支（与 v8/v9 一致，避免半迁移撞 `duplicate column`）。

### 4.2 迁移函数形态

```go
// migrateSchemaV10 split_records 页级产物指纹列迁移：列存在性预判使半迁移库
// 可重入，DDL 与 user_version 同事务提交（SQLite DDL 可回滚）。
func (s *assetsStore) migrateSchemaV10() error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer func() { _ = tx.Rollback() }()

    var n int
    if err := tx.QueryRow(
        "SELECT COUNT(*) FROM pragma_table_info('split_records') WHERE name = 'artifact_fingerprints'",
    ).Scan(&n); err != nil {
        return fmt.Errorf("读取 split_records.artifact_fingerprints 列信息失败: %w", err)
    }
    if n == 0 {
        if _, err := tx.Exec(
            "ALTER TABLE split_records ADD COLUMN artifact_fingerprints TEXT NOT NULL DEFAULT ''",
        ); err != nil {
            return err
        }
    }
    if _, err := tx.Exec("PRAGMA user_version = 10"); err != nil {
        return err
    }
    return tx.Commit()
}
```

`migrate()` 增加：

```go
if v == 10 {
    if err := s.migrateSchemaV10(); err != nil {
        return fmt.Errorf("素材索引库迁移到 v10 失败: %w", err)
    }
    continue
}
```

### 4.3 数据模型与读写

```go
type splitArtifactFingerprint struct {
    Page   int    `json:"page"`
    Name   string `json:"name"`   // 拆分时产物文件名（basename，不含目录）；迁移/重链不重写
    Size   int64  `json:"size"`
    SHA256 string `json:"sha256"` // 小写 hex
}

type assetSplitRecord struct {
    // ...现有字段...
    fingerprints []splitArtifactFingerprint // 空 = 无指纹（v10 前旧记录 / 解析失败）
}
```

- `getSplitRecord` / `splitRecordByOutputDir` 的 SELECT 增列并 `Scan`；解析失败 `Logger().Warn` 后置空，**不返回错误**（保持查询宽容语义）。
- `upsertSplitRecord` 增形参 `fingerprints []splitArtifactFingerprint`，内部 `json.Marshal`；序列化失败按空串处理并 `Warn`（理论不可达）。
- 写入 SQL 增列，`ON CONFLICT DO UPDATE SET artifact_fingerprints = excluded.artifact_fingerprints`。
- 三个调用点更新：`assets_split.go:523`、`:849`、`:878`。
- 指纹不进入任何 Wails 导出模型；`AssetSplitRecordView` 不变，前端零改动。

### 4.4 写入时机

**source / cache（`splitOneFileWithHash`）**

1. 核心成功（`:488` 之后）；
2. 先执行现有清理（`:509`、`:518`）——清理只使用**旧记录**，与新指纹无关；
3. 对 `result.Files` 逐页计算指纹（页序优先用 `pageSeqFromArtifactName`，失败回退 `i+1`）；
4. 与记录同一 `upsertSplitRecord` 写入（`:523`）。

**replace（`splitOneFileReplace`）**

1. 逐页 rename + Stat 通过后（`:703-742`）；
2. 对 `renamed` 逐页计算指纹（`pages` 数组提供页序）；
3. 在删源失败补写（`:849`）与成功写记录（`:878`）两处都写入同一份指纹；
4. 回滚路径不写记录，旧记录与旧指纹保持原样，被还原的旧产物仍与旧指纹一致。

**哈希失败语义**

- 任一页 `hashFileSHA256` 失败：`Logger().Warn`，该记录写**空指纹**（整体降级为 mtime 回落），不因此回滚已成功的拆分与产物；
- 理由：产物已完整落盘并可能已删源（replace），因记账失败回滚会丢失成功成果；空指纹的失效方向是漏清，不是误删。

### 4.5 清理判定顺序（推荐实现）

**`cleanupSplitArtifactsInDirByPage`**

1. 现有前置：`record == nil` / `dir == ""` / `pageCount <= 0` 直接返回。
2. **若 `record.fingerprints` 非空**：
   1. 逐条指纹（`1 <= page <= record.pageCount`）：
      - 校验 `name` 为纯 basename（`filepath.Base(name) == name`，非 `.`/`..`），防止脏数据路径穿越；
      - `path := filepath.Join(dir, name)`；
      - `os.Lstat`：不存在 → 跳过；非常规文件（符号链接/目录/设备）→ 跳过并 `Warn`；
      - 若 `excludeStem != ""` 且该文件解析主干与 `excludeStem` 精确相等 → 跳过（防御同目录 M1，正常指纹名是旧主干，不会命中）；
      - `size` 不等 → `Debug` 保留（用户改过内容）；
      - 计算 `sha256`；不等 → `Debug` 保留；相等 → `removeSplitArtifactFile` 删除；
   2. 收集删除路径，`deleteIndexRowsForPaths`；
   3. 目录空则删除（现有语义）。
3. **否则（无指纹 / 解析失败）**：完全走现有 `collectSplitArtifactsByPage` + `dominantSplitArtifactStem` + `isSplitArtifactAfterSplitAt` 路径，行为不变。

> 指纹非空时不再做目录主干启发式选择——指纹本身就是 allow-list，可避免现有「目录基名不匹配」「同页多候选」等不可判定分支，同时天然覆盖 replace/source 两种命名形态。

**`removeSplitArtifactsBeyond`**

- 签名增加旧记录的指纹（或直接传 `*assetSplitRecord`，同时保留 `splitAt` 用于回落）。
- 指纹非空：遍历指纹中 `page > keepPage` 或 `name != fmt.Sprintf("%s-%03d.pptx", baseName, page)` 的条目，按 §4.5 的 size+hash 校验后删除；不在指纹中的候选（用户文件）一律不删。
- 指纹为空：保持现有 mtime 守卫。

**`cleanupStaleReplaceArtifacts`**

- 传入旧记录；对 `oldArtifacts` 中每个名字，查找同名的指纹条目：
  - 命中 → size+hash 校验后删除；
  - 未命中（用户文件 / 无指纹旧记录）→ 保持现有按名字删除（兼容旧行为）或保守保留（需产品确认；建议旧记录保持现状，指纹记录下未命中则保留）。
- 这样可把「用户放在 replace 产物目录、恰好与旧产物同名但内容不同」的误删面也收掉。

### 4.6 指纹缺失回落

- `artifact_fingerprints == ""`、JSON 解析失败、或 `fingerprints` 长度与 `page_count` 不一致（视为损坏）→ 一律按「无指纹」处理，走 `isSplitArtifactAfterSplitAt`。
- 有指纹但某页无条目 → 该页不是本记录产物，**保留**，不回落 mtime（避免把用户文件按旧启发式删掉）。
- **禁止自动回填**：现有目录内容可能已被用户替换，回填即把用户文件认证为产物。指纹只能由成功拆分写入。
- 残留风险：v10 前已拆分文件在用户显式重拆前仍受 mtime 启发式保护；这是 PRD 已接受的回落语义。

### 4.7 算法选型

| 选项 | 结论 |
|---|---|
| 全文 SHA-256 | **采用**。与现有 `hashFileSHA256` 同源，无新依赖，抗碰撞；流式读取内存 O(1) |
| size + mtime 组合 | 不采用。mtime 用户可改，正是本次要消除的启发式；size 仅作预筛 |
| 部分内容哈希（前 N KB） | 不采用。页 pptx 的差异可能在包中部/rels/Content_Types，截断哈希会误判 |
| 复用 `files.hash` | 不采用作为唯一来源。cache/derived 产物不在索引；索引 hash 可能滞后于磁盘 |
| 哈希写入时顺带计算 | 可选优化，但需改 `writeZipAtomic` / `model.PptxSplitResult`（导出 struct 会触发 wailsjs 三处同步），本任务不建议 |

### 4.8 性能预算（假设与量级）

假设：单文件拆分为 N=20–80 页；每页产物 0.5–20 MB；总产物 S=10 MB–1.6 GB。

| 项 | 量级 | 说明 |
|---|---|---|
| 拆分后逐页 SHA-256 | 约 S / (0.3–2 GB/s) ≈ 0.02–5 s | 刚写完的文件多在页缓存；无硬件 SHA 扩展时取低值 |
| JSON 序列化/反序列化 | < 1 ms（100 页约 15–25 KB） | 每页约 150–250 B |
| 记录写入 | 单语句，< 1 ms | 与现有 upsert 同量级 |
| 清理（指纹路径） | O(N) Lstat + 命中文件哈希；正常全命中 ≈ S / 吞吐 | 只在改名/跨落点/改主干/超界页场景执行，非每次拆分 |
| 清理（旧记录回落） | 与现状相同 | 目录扫描 O(M) + Stat |
| 内存 | O(1)：32 KB 缓冲 + 一个 JSON 串 | 不整页读入内存 |

对批量拆分：新增耗时按文件累计；若一批 50 个文件、平均总产物 100 MB，则新增约 2.5–16 s（按 0.3–2 GB/s）。这是本方案最主要的性能代价，需在实现后用真实素材 benchmark 校准；若不可接受，再评估「写入时顺带哈希」的优化（需跨层同步）。

### 4.9 测试要点

**纯逻辑单测（无需临时文件）**

- JSON 编解码：正常、空、非法 JSON、重复页、页序乱序；
- `fingerprintMatch`：page/name/size/hash 四条件逐项不匹配；
- 缺失回落决策：空指纹 / 解析失败 / 页条目缺失；
- 删除判定顺序：excludeStem、路径穿越名、非常规文件。

**临时文件单测（`t.TempDir` + `testutil.WriteFile`）**

- 指纹命中删除：写入与指纹一致的字节 → 删除，索引行联动清理；
- 指纹不符保留：同主干同页序、内容不同、mtime 早于 `split_at` → 保留（PRD 验收核心）；
- 指纹缺失回落：空指纹 + mtime 早 → 按旧逻辑删除；mtime 晚 → 保留；
- `removeSplitArtifactsBeyond`：超界旧页命中删除；用户同形文件不在指纹 → 保留；
- `cleanupStaleReplaceArtifacts`：旧产物命中删除；同名不同内容保留（若采纳 §4.5 第三条）；
- Windows 大小写：`sameNameFold` 下 `Abc-001.pptx` 与 `abc-001.pptx` 视同一物理文件；Linux 大小写敏感不折叠；
- 迁移：v9 库升级到 v10；列已存在 + `user_version=9` 的半迁移重入；新库版本钉到 10；
- 备份：`assetsSchemaVersion-1`（即 v9）备份还原后迁移到 v10，`verifyBackupDatabase` 放行；
- 重链/目录迁移/重建索引后指纹仍命中（现有 `TestRelocateAssetFile_*` 扩展断言）。

**覆盖率门禁**

- 后端 service 基线 76%（`docs/spec/test-coverage-gate.md`）；新增分支必须有直接用例，尤其是「指纹命中 / 不符 / 缺失」三态与回退路径。
- 现有断言 `assetsSchemaVersion` 的测试会自动期望 10；需全量跑 `go test ./...` 确认无硬编码 9 的残留。

---

## 5. 待确认风险清单

| # | 风险 / 未确认项 | 影响 | 需要的证据或决策 |
|---|---|---|---|
| 1 | OOXML 产物字节确定性仅做了代码推理，未实测 | 方案 a/c 的「重算比对」前提 | 加一个确定性测试：同一 fixture 拆两次，逐页 SHA-256 相等；建议在 Windows 与 Linux CI 各跑一次 |
| 2 | `modernc.org/sqlite` 下 `ON UPDATE CASCADE` / `ON DELETE CASCADE` 行为未实测 | 仅影响方案 b | 若仍考虑 b，写最小事务测试取证；否则选 a 可规避 |
| 3 | 验收范围是否覆盖三处删除路径 | 只改 `cleanupSplitArtifactsInDirByPage` 不能满足「任意 mtime 不删」 | 需确认 `removeSplitArtifactsBeyond`、`cleanupStaleReplaceArtifacts` 是否纳入本任务 |
| 4 | v10 前旧记录无指纹，重拆前仍受 mtime 启发式 | 残留 P1 窗口 | PRD Out of Scope 已接受回落；若要求存量立即安全，需另设计「无指纹时不删」的更强保守策略（会影响正常清理） |
| 5 | 指纹哈希失败时写空指纹 vs 整体失败 | 前者保留成果但产生漏清；后者回滚已成功产物 | 建议前者；需产品确认 |
| 6 | 扫描/拆分大文件时哈希耗时 | 批量拆分总耗时增加 | 用真实素材 benchmark（50–200 页、总产物 100 MB–2 GB），决定是否做「写入时顺带哈希」优化 |
| 7 | 用户把产物字节原样复制成另一个文件名 | 若只按 hash+page 判定会删除该副本；按 name 守卫则保留 | 本报告建议保留 name 守卫（漏清不误删）；需确认产品语义 |
| 8 | 指纹 JSON 损坏/手工改库 | 解析失败回落 mtime，不崩溃 | 明确「解析失败 Warn + 空指纹」，并用单测覆盖 |
| 9 | 备份预检是否应检查新列/新表 | 若硬性要求，旧备份会被误拒 | 建议不加入核心表/列校验；旧备份迁移补空列即可 |
| 10 | 是否向前端暴露指纹状态 | 会扩大 Wails 模型与 wailsjs 同步面 | 建议不暴露；`AssetSplitRecordView` 不变 |
| 11 | 并发拆分/清理与单连接池 | 多语句事务（仅 b）可能自锁 | 选 a 时无新增事务；若 b，须遵守「先收齐 rows 再写」纪律 |
| 12 | 未来核心若改为原地增量重写 | 旧指纹可能与新产物不一致 | 届时必须在重写后同步更新指纹；当前核心全量重写页序 ≤ 新页数，不受影响 |

---

## 6. 关键代码位置索引

| 主题 | 文件:行 |
|---|---|
| 清理主函数 | `service/assets_split.go:1573` |
| mtime 守卫 | `service/assets_split.go:1528`、`:1522` |
| 清理决策门控 | `service/assets_split.go:1502` |
| 超界旧页清理 | `service/assets_split.go:1412` |
| replace 陈旧页清理 | `service/assets_split.go:1835` |
| 产物定位 | `service/assets_split.go:1185` |
| 产物命名解析 | `service/assets_split.go:1212` |
| 大小写折叠 | `service/assets_split.go:2014`、`:2024` |
| 记录结构 / 读写 / 键改写 / 目录迁移 | `service/assets_db.go:2333`、`:2347`、`:2407`、`:2458`、`:2570` |
| schema 版本与迁移表 | `service/assets_schema.go:21`、`:141`、`:248`、`:265` |
| 迁移执行 | `service/assets_db.go:117`、`:166`、`:203` |
| 拆分核心与 zip 重写 | `service/pptx_split.go:41`、`:83`、`:668`、`:740`、`:752` |
| 现成 SHA-256 | `service/assets_scan.go:1108` |
| 备份预检 | `service/assets_backup.go:451` |
| 记录删除点 | `service/assets_db.go:639`、`:876`、`:2668` |
| 库删除清理 derived | `service/assets_library.go:339-344` |
| 重建索引保留 split_records | `service/assets_scan.go:321`（`rebuildLibrary`） |
| 规格文档 | `docs/spec/pptx-split.md §9.5`、`docs/spec/assets-management.md`（v9/备份章节） |
