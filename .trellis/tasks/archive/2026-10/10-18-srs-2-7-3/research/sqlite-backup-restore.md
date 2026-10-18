# Research: 素材库 SQLite 备份链路（VACUUM INTO / integrity_check / 还原替换）

- **Query**: modernc.org/sqlite 对 VACUUM INTO / integrity_check 的支持验证；还原链文件替换正确姿势；单连接池并发面；备份失败策略参考；go.mod 版本核实
- **Scope**: mixed（内部代码 + mod 缓存源码核实 + 临时 Go 模块实证）
- **Date**: 2026-10-18

## 摘要（结论前置）

| # | 研究问题 | 结论 |
|---|---|---|
| 1 | VACUUM INTO 支持性 | **完全支持**。`db.Exec("VACUUM INTO '<path>'")` 即可；目标已存在报错、事务内报错；WAL 未 checkpoint 数据完整进入备份 |
| 2 | integrity_check 用法 | 用 `Query`/`QueryRow` 读返回行，健康库 1 行 `"ok"`；**Exec 方式拿不到结果**（不报错但结果被丢弃） |
| 3 | 还原步骤 | 关连接（close 后 -wal/-shm 实测自动收口）→ 防御性删 -wal/-shm → **os.Rename 直接覆盖**（Windows 走 MoveFileEx REPLACE_EXISTING，无需先 Remove）→ 惰性重开 |
| 4 | 并发防护 | 复用 DeleteLibrary 既有组合：CancelAssetThumbs + tryLockScan + tryLockSplit + s.mu；单连接池下 VACUUM INTO 执行期其他查询排队（连接占用，非 SQLite 锁） |
| 5 | 备份失败策略 | 重建前失败继续执行风险低；删库前失败继续执行存在「零恢复点删除」残余风险，建议确认文案明示（决策归 PRD/主 agent） |
| 6 | 版本要求 | go.mod `modernc.org/sqlite v1.60.1` 内嵌 SQLite **3.53.4**；两特性均远低于此版本要求，现存版本直接可用 |

---

## 1. 实证方法与环境

验证脚本为独立临时 Go 模块（`module backupcheck`，require modernc.org/sqlite v1.60.1，go.sum 取自主项目），**DSN 与连接池参数与 `service/assets_db.go` 完全一致**：

```go
dsn := "file:" + filepath.ToSlash(path) +
    "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
db.SetMaxOpenConns(1)
```

数据规模：60 万行单表（源库约 12.6 MB）。全部用例通过，无失败项。

## 2. VACUUM INTO 可行性结论

### 2.1 支持性

| 事实 | 证据 |
|---|---|
| v1.60.1 内嵌 SQLite 3.53.4 | mod 缓存 `modernc.org/sqlite@v1.60.1/lib/sqlite.go`：`SQLITE_VERSION = "3.53.4"` |
| VACUUM INTO 自 SQLite 3.27.0（2019-02）引入 | SQLite 官方 lang_vacuum.html（本次网络受限未能在线抓取，属长期稳定的公开事实） |
| `db.Exec` 一条语句即可完成备份 | 实测：60 万行 12.6 MB 库备份成功，耗时 127 ms |
| 错误消息字面量在 SQLite 核心（非驱动层） | mod 缓存 `lib/sqlite_g_*.go`（ccgo 转译自 sqlite3.c）中存在 `"output file already exists"` 与 `"cannot VACUUM from within a transaction"` |

### 2.2 行为细节（全部实测）

| 行为 | 实测结果 | 实现含义 |
|---|---|---|
| 目标文件已存在 | `SQL logic error: output file already exists (1)` | 执行前须确保目标不存在（时间戳命名天然唯一 + 防御性 `os.Remove` 兜底） |
| 事务内执行 | `SQL logic error: cannot VACUUM from within a transaction (1)` | **不能包在 Tx 里**；database/sql 自动提交下单条 Exec 无碍 |
| WAL 未 checkpoint 数据 | 写入 60 万行后未 checkpoint（`-wal` 文件存在）立即 VACUUM INTO，备份独立打开 count=600000 | **无需预 checkpoint**，读快照天然覆盖 -wal 内容 |
| 输出紧凑性 | 删除 30 万行后源库 12.58 MB → 备份 6.30 MB | 输出为紧凑独立 db（消空闲页），即 PRD「独立紧凑 db」预期 |
| 路径分隔符 | 正斜杠、反斜杠路径均成功 | SQLite 字符串字面量不处理反斜杠转义；为对齐项目 DSN 惯例（assets_db.go:40 `filepath.ToSlash`），建议 SQL 字面量也统一 ToSlash |
| 耗时量级 | 60 万行 127 ms；删除后 30 万行 80 ms（本地 NVMe） | 素材库数据量（数千~数万幻灯片行 + FTS）远小于此，预计毫秒~百毫秒级 |

### 2.3 单连接池下的阻塞面

| 场景 | 实测结果 | 定性 |
|---|---|---|
| 同池（SetMaxOpenConns(1)）VACUUM INTO 执行期间另发查询 | 查询等待 131 ms ≈ VACUUM 剩余全程，VACUUM 结束后才返回 | **池上排队**：唯一连接被 VACUUM 占用，扫描/检索/缩略图记账等所有经该 `*sql.DB` 的操作串行等待 |
| 独立连接池（另一个 `sql.Open` 同文件）期间读 | err=nil，rows=600000，53 ms | VACUUM INTO 在源库只持**读事务**，SQLite 层不阻塞其他连接的读 |
| 独立连接池期间写 | err=nil，1 ms | WAL 模式下写也可并发进行 |

结论：阻塞完全来自 **database/sql 连接池占用**（连接被占 → 池上等待），不是 SQLite 锁。对素材库（数据量小、单库操作本就经同一连接串行）影响可接受；可加「备份中」防重复触发态，无需为备份改连接池拓扑。

## 3. integrity_check 用法

### 3.1 执行方式

| 方式 | 实测结果 | 结论 |
|---|---|---|
| `db.Query("PRAGMA integrity_check")` + 循环 `Scan` 全部行后 `Close` | 健康：1 行 `"ok"` | **推荐**（循环读完再 Close，符合单连接池 rows 未关自锁禁令，见 `service/assets_db.go:612` 既有注释） |
| `db.QueryRow("PRAGMA integrity_check").Scan(&s)` | 健康：`s == "ok"` | 可用（Scan 完即释放连接，无自锁风险） |
| `db.Exec("PRAGMA integrity_check")` | **err=nil 但结果集被丢弃** | 禁用于校验：拿不到结果，形同没校验 |

`PRAGMA quick_check` 同样返回 1 行 `"ok"`；区别是 quick_check 跳过索引内容交叉校验，更快。备份校验场景数据量小，按 PRD 用 integrity_check 全量校验。

### 3.2 损坏库的返回形态（实测）

对备份文件中部多页写 `0xFF` 破坏后，`PRAGMA integrity_check` 返回**单行**聚合文本（不是多行）：

```
*** in database main ***
Tree 2 page 2: btreeInitPage() returns error code 11
Page 3: never used
...
```

注意两点：

- 实测版本（3.53.4）将全部问题聚合在**一行**内（错误项以 `\n` 内嵌在 Go string 中）。实现上判断「首行值 != ok」即判失败，足够稳健；不必依赖多行形态。
- 错误文本含换行，若需前端展示须留意换行处理。

### 3.3 校验失败处置

PRD 已定：非 ok 删备份文件并报错（备份失败不静默）。实测损坏可被 integrity_check 稳定检出，该链路成立。FTS 表随库文件整库拷贝，无需单独维护。

## 4. 还原步骤序列（含实测与内部先例对齐）

### 4.1 步骤

1. **取消在途缩略图批次**：`CancelAssetThumbs(libraryID)`（幂等，无在途无害）——先例 `service/assets_library.go:246`。
2. **在途扫描/拆分互斥**：`tryLockScan` → `tryLockSplit`（顺序同 DeleteLibrary：先扫描后拆分，防反向获取成环）——先例 `service/assets_library.go:249-261`。
3. **关连接**：`store.Close()`（`sql.DB.Close`）。**实测：WAL 模式连接正常关闭后 `-wal`/`-shm` 由 SQLite 自动 checkpoint 收口并删除**（close 后两文件均不存在）——与 `assets_db.go:56` 注释口径一致。先例 `assets_library.go:279-285`（含 `delete(s.open, ...)`）。
4. **防御性删除 `-wal`/`-shm` 残留**：异常退出可能残留；**必须在重开前删**，否则旧 WAL 与新 db 内容错位会恢复出错误状态。先例 `assets_library.go:292-296` 三后缀循环（`""`/`-wal`/`-shm`，不存在忽略）。
5. **备份文件替换 `<id>.db`**：直接 `os.Rename(备份路径, <id>.db)`。
6. **重开**：`delete(s.open, libraryID)` 后下次访问经 `storeForX` 惰性重开走 `openAssetsStore`（migrate 幂等）；或显式重开。
7. **前端提示重扫**（PRD R3）。

### 4.2 Windows 下 os.Rename 对已存在目标的行为

| 事实 | 证据 |
|---|---|
| **os.Rename 直接覆盖已存在普通文件，成功** | 本次实证：备份 Rename 覆盖在用源库成功，内容整体替换，重开后数据完整 |
| 底层为 `MoveFileEx(MOVEFILE_REPLACE_EXISTING)` 同卷原子替换 | `util/json.go:32-33` 注释 + `util/json_test.go` `TestSaveJSON_OverwriteExisting` 锚定（项目既有契约） |
| 无需「先 Remove 再 Rename」 | 两步法引入「Remove 成功、Rename 失败」的中间态丢文件窗口；直接 Rename 失败时原文件仍在，语义更安全 |
| 已知瞬态失败面 | 缩略图记账 flusher 45s 周期事务持库连接时，Windows 可能撞句柄致删除/替换瞬态失败，可重试恢复——先例 `assets_library.go:287-290` 注释（还原链在步骤 1 已取消批次、步骤 3 已关连接，窗口比删库更窄） |

备份文件与 `<id>.db` 同在 `data/assets/` 卷下，满足同卷原子替换前提。

### 4.3 窗口期防并发手段

现状手段为「服务层锁组合」而非拒绝请求（DeleteLibrary 同款）：

| 手段 | 覆盖面 |
|---|---|
| `tryLockScan(libraryID)` | 与在途扫描互斥（扫描中禁还原，防扫描写已关连接） |
| `tryLockSplit(libraryID)` | 与在途拆分互斥 |
| `CancelAssetThumbs` | 缩略图批次取消（异步，残留窗口与 DeleteLibrary 同口径：缩小而非根除） |
| `s.mu` | 注册表/连接表读写保护 |

还原与删库的差异点：还原后库仍在 `s.libs` 注册表中，第 6 步重开即恢复全部服务；窗口内到达的检索/浏览请求经惰性重开拿到新文件连接，语义安全（新文件由 integrity_check 校验过的备份整体替换）。

## 5. 备份失败策略参考

PRD 立场（R3）：重大操作自动备份失败 → 告警但不中止原操作，研究冲突时以研究为准。

**内部先例**：DeleteLibrary 对缩略图目录删除失败、拆分产物删除失败均为「失败仅告警不阻断」（辅助动作失败不放大故障面），`service/assets_library.go:297-306`。

**风险面分析**（按主操作可逆性分档）：

| 主操作 | 可逆性 | 备份失败后继续的风险 |
|---|---|---|
| 重建索引 | 可重扫恢复（标注有 §14.16 哈希兜底） | 低：重建本身失败/成功均有下界，备份缺失损失有限 |
| 删除库 | **不可逆** | 中：备份失败常见因（磁盘满/权限/目录不可写）下删库本身仍会成功（删除释放空间），此后**零恢复点**；且这些环境性问题同时预示后续操作（重扫写库）可能失败 |

业界对「不可逆操作前 best-effort 备份」失败无统一惯例：保守实现（IDE 删除前快照类）中止或强确认，宽松实现仅告警。研究侧结论：**「告警不中止」对重建前备份成立无反例；对删库前备份存在残余风险，缓解手段是不阻断但升级告知强度**（如确认弹窗文案明示「备份生成失败，删除后无法恢复」），既不放大故障面也不静默。是否采纳属策略决策，归主 agent/PRD。

## 6. 版本核实

| 项 | 值 | 出处 |
|---|---|---|
| go.mod 驱动版本 | `modernc.org/sqlite v1.60.1` | `go.mod:19` |
| 内嵌 SQLite 版本 | `3.53.4` | mod 缓存 `lib/sqlite.go` `SQLITE_VERSION` 常量 |
| Go 版本要求 | v1.60.0 起要求 Go ≥ 1.26 | 既有研究 `.trellis/tasks/archive/2026-10/10-13-dam-ppt/research/sqlite-fts5-chinese.md`（go 1.26.6 满足） |
| VACUUM INTO 版本要求 | SQLite ≥ 3.27.0 | 官方 changelog（长期稳定事实） |
| integrity_check / quick_check 版本要求 | 远古 PRAGMA，无门槛 | — |

**结论：现存 v1.60.1 对两特性零版本诉求，无需升级。** 升级驱动时维持既有硬约束：`modernc.org/libc` 与上游 pin 对齐（同上研究文档 §1.3）。

## 7. 相关文件

| 文件 | 关联点 |
|---|---|
| `service/assets_db.go:35-62` | openAssetsStore DSN/SetMaxOpenConns(1)/Close；:612 单连接池 rows 未关自锁禁令 |
| `service/assets_library.go:236-312` | DeleteLibrary：还原链全部可复用先例（取消/锁/关连接/删三后缀/瞬态窗口注释） |
| `util/json.go:19-67` | SaveJSON 原子写（os.Rename = MoveFileEx REPLACE_EXISTING 契约注释） |
| `util/json_test.go:77-78` | TestSaveJSON_OverwriteExisting：Windows 覆盖语义锚定测试 |
| `.trellis/tasks/archive/2026-10/10-13-dam-ppt/research/sqlite-fts5-chinese.md` | 驱动选型与版本约束先行研究 |

## Caveats / Not Found

- SQLite 官方文档（sqlite.org）在线抓取因本机网络受限失败（连接重置）；VACUUM INTO 引入版本（3.27.0）与官方语义描述以 mod 缓存转译源码错误字面量 + 实测行为双重佐证，可信度不受影响。
- modernc 仓库 issue tracker 未检索（同网络原因）；实证脚本全用例通过本身即证明无阻塞性 issue。
- 「备份中」防重复触发、删库前失败告知强度为策略层建议，采信与落地由主 agent 决策。
- 实证数据规模（60 万行/12.6 MB）为参考量级；素材库真实规模更小，实测耗时只会更低。
