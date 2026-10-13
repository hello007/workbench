# 素材管理契约（assets）

> 适用范围：素材管理 MVP（任务 10-13-dam-ppt）全部后端链路——SQLite 数据层、FTS5 中文检索、增量扫描、库管理、前端面板。改动这些文件前必读；新增同类能力（如图片/PDF 索引、缩略图）须沿用本文契约。

## 1. Scope / Trigger

- 触发：新增数据库 schema/迁移、检索链路变更、扫描判定变更、新事件通道、新对话框方法。
- 涉及文件：`service/assets_*.go`、`model/assets.go`、`app_assets.go`、`frontend/src/components/AssetsPanel.vue`。

## 2. Signatures

### 存储布局

| 产物 | 路径 | 说明 |
|---|---|---|
| 每库索引 | `data/assets/<library-id>.db` | 删库即删文件（含 WAL/SHM），标签体系天然隔离 |
| 库注册表 | `data/assets/libraries.json` | 必须走 `util.SaveJSON` 原子写（data-atomic-write 契约） |

### SQLite 驱动约束

- **只用 `modernc.org/sqlite`（纯 Go 零 CGO）**，禁 `mattn/go-sqlite3` 等 CGO 驱动（破坏 `CGO_ENABLED=0` 交叉编译与 Linux 构建）。
- 连接配置：WAL + busy_timeout + foreign_keys（DSN per-connection）；**单写连接 `SetMaxOpenConns(1)`**。
- 迁移：`PRAGMA user_version` 递增 + Go 层回填函数（v2 = FTS 表建立 + `backfillFtsOnUpgradeV2` 存量回填，切分经 Go 函数与生产写路径同源）；db 版本**过新**（高版本 app 打开旧库产物）须拒绝打开，禁静默降级。

### App 方法（11 + 1 对话框）

```
CreateAssetLibrary(name, rootPath) / ListAssetLibraries / DeleteAssetLibrary(id)
GetAssetLibraryStats(id) / StartAssetScan(id) / CancelAssetScan(id) / RebuildAssetLibrary(id)
ListAssetFiles(id) / ListAssetSlides(id) / UpdateAssetFileMeta(fileId, AssetFileMetaUpdate)
SearchAssets(id, keyword, AssetSearchOptions)
OpenDirectoryDialog(title) (string, error)   // app_preview.go，serve 无头模式 wailsRuntimeUnavailable 拒绝
```

### 事件通道

| 事件 | 载荷 | 前端行为 |
|---|---|---|
| `assets-scan:progress` | 带 `libraryId` | 按库过滤更新进度条 |
| `assets-scan:done` | 带 `libraryId` | 刷新当前视图 |
| `assets-scan:error` | 带 `libraryId` | **只复位进度不弹提示**（防与 Promise catch 的 handleError 双弹） |

`web_serve.go` `applySink` 须含 `assetsSvc.SetEventSink`（现七服务，漏加则浏览器通道静默收不到）。

## 3. Contracts

### FTS5 检索链路（核心契约）

- **表形态**：standalone FTS5 表 `search_fts` + 事务双写（external content 须配触发器/rebuild、contentless 无 snippet，均弃）。列：ref_type/ref_id/file_id 三个 UNINDEXED + file_name/file_note/slide_title/slide_text/slide_notes 五个检索列。
- **`detail=full` 必须显式保留**：phrase 查询依赖全量位置信息，降档致短语匹配失效。
- **切分同源（最高优先级坑）**：写入与查询必须调用同一 `splitFtsText`——CJK 逐字切开、拉丁/数字保持词内、混合边界插空格。两处各自实现 = 写入切分与查询切分不一致 = 静默检索漏命中，且无报错。
- **查询构造**：`buildFtsQuery` 按空白拆词 + 双引号 phrase + 多词 AND + 双引号翻倍转义；空查询返回空串 → 调用方直接返回空结果，禁把空串喂给 MATCH。
- **FTS rowid 编码**：文件行 = `+files.id`，幻灯片行 = `-slides.id`——删除/解码 O(1)，禁查表反推。
- **bm25 列加权**：file_name 4.0 > slide_title 3.0 > slide_text 2.0 > 备注 1.0（字段加权方向：文件名 > 标题 > 正文 > 备注）。
- **禁用 trigram tokenizer**：对 2 字中文词（如「获客」）MATCH 恒 0 行且**无报错**（静默漏检）；unicode61 + 按字切分 + phrase 是唯一同时满足「2 字词必命中」与性能的方案（10 万行 p95 11-25ms）。
- **禁对 FTS 表隐藏列做 LIKE**：实测恒 0 行；模糊匹配走业务表列。
- **摘要不使用 FTS `snippet()`**：输出含切分空格需后处理，坑多于收益；改 `snippetFromTexts` 原文子串窗口（命中前 20 字后 40 字，rune 口径截取防截半）。
- **bm25 IDF 钳制现象**：微型语料全命中时 IDF 触及 1e-6 下限，列权重保持方向但同权重行间顺序由长度归一定——测试断言排序须构造确定性分数分层（标题命中 vs 正文命中），勿依赖小语料 bm25 绝对值。

### 单连接池自锁（Gotcha）

> **Warning**：`SetMaxOpenConns(1)` 下，前一条查询的 `*sql.Rows` 未 `Close` 就发起第二条查询 = 连接被占 = **永久死锁**（无超时、无报错、GUI 卡死）。

修复模式：`QueryRow+Scan`（单行）或显式 `rows.Close()` 后再查（多行）；需要「多条统计 + 列表」时先收完前者再发后者。

### 派生产物扫描排除（口径与拆分实际输出严格一致）

- 判定 `isPptxSplitDerivative`：按 `pptx_split.go` **真实输出布局**——`<源文件名>/<源文件名>-NN.pptx`（同名文件夹 + 三位页序后缀），禁凭空发明目录名（初版曾用不存在的 `derived/` 目录，拆分副本被重复索引，质量检查修复）。
- 根目录直置的 `xxx-001.pptx`、同名异构文件不误伤；用户自组织同名形态按防重优先排除（取舍已在代码注释声明）。

### 扫描语义

- 增量判定：mtime+size 未变跳过（不计算哈希）；变更才 sha256；processing 残留必重处理（自愈）。
- 状态机：处理中/成功/失败(损坏)/跳过(加密)/跳过(权限)；单文件失败记日志继续，不中断批量。
- 串行保护：同库 TryLock 互斥（E_ASSETS_SCAN_IN_PROGRESS）；取消走 registerCancel/unregisterCancel defer LIFO（先注销后释放锁），映射 E_ASSETS_CANCELLED（warning）。
- 重建：备份标签绑定 → 清 files+FTS（standalone 表**不支持 `delete-all'` 命令**，用 `DELETE FROM search_fts`）→ 全量重扫 → 按路径重绑标签（星级/备注属重建语义不保留）。
- 锁内库存在复验：`getLibrary` 校验与加锁之间有窗口，窗口内删库 + sqlite 惰性建库 = 孤儿 db 文件复活。

## 4. Validation & Error Matrix

| 条件 | 行为 |
|---|---|
| 库不存在/已删 | `E_ASSETS_LIB_NOT_FOUND`（error） |
| 同库扫描进行中再启动 | `E_ASSETS_SCAN_IN_PROGRESS`（warning） |
| 用户取消扫描 | `E_ASSETS_CANCELLED`（warning，中性提示） |
| 加密 pptx（OLE 魔数） | 跳过(加密)，日志，不中断批量 |
| 损坏/非法 pptx | 失败(文件损坏)，日志，继续 |
| 权限不足 | 跳过(权限)，日志，继续 |
| serve 无头模式调对话框 | 普通 error「无头服务模式下原生对话框不可用」（RPC 层映射 E_RPC_INTERNAL），三对话框方法同构 |
| db 版本过新 | 拒绝打开，报错不静默降级 |
| 空关键词检索 | 返回空结果，不报错，禁空串喂 MATCH |
| 检索分页期间条目被删 | 条目跳过，Total 不回改（页内可少于 Limit，已注释声明） |

## 5. Good/Base/Bad Cases

- **Good**：改 FTS 相关任何代码前先确认写入/查询切分同源；新增可检索列 → schema v3 迁移 + 回填 + 双写三处同步。
- **Base**：新增扫描状态 → model 常量（普通 string 常量，禁具名 string 类型——models.ts 别名坑）+ 前端渲染分支。
- **Bad**：新建第二条 FTS 查询路径绕过 `buildFtsQuery`；用 `mattn/go-sqlite3`；扫描排除逻辑硬编码新目录名不核对拆分实际输出。

## 6. Tests Required

- 切分器/查询构造/摘要窗口：CJK/拉丁/混合/标点/空串 + **写入查询同源回归**。
- 检索：中文词命中（「城商行」「获客」2 字词）、聚合折叠/平铺、bm25 排序分层（确定性构造）、标签继承筛选、分页边界、FTS 行数同步断言（删除/重解析后）。
- 扫描：增量判定**注入陈旧记录 SQL 回拨**驱动（禁依赖 NTFS mtime 时序）；取消用预取消 ctx；异常分类 parseFn 注入。
- 基准：`BenchmarkAssetSearch` 10 万行（直插 SQL 绕过解析）+ `TestAssetSearch_PerfBudget` 1 万行 CI 守护档（p95 < 1s，`-short` 跳过）。
- fixture 程序化构造 pptx，禁依赖本机 Office（pptx-split 契约）。

## 7. Wrong vs Correct

### Wrong（切分不同源——静默漏检）

```go
// 写入侧用 A 函数，查询侧用 B 函数（或查询侧直接拼原始关键词）
db.Exec(`INSERT INTO search_fts ... VALUES (?, ?)`, refID, splitFtsText(text))
rows, err := db.Query(`SELECT ... FROM search_fts WHERE search_fts MATCH ?`, keyword) // 未切分未 phrase
```

### Correct（同源 + phrase）

```go
split := splitFtsText(text)                 // 写入
query := buildFtsQuery(keyword)             // 查询——同一实现族；空串短路返回空结果
rows, err := db.Query(`SELECT ... FROM search_fts WHERE search_fts MATCH ?`, query)
```

---

**沉淀来源**：任务 10-13-dam-ppt（2026-10-13，素材管理 MVP 三批实施 + 质量检查修复）。
