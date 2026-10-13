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
ListAssetFiles(id) / ListAssetSlides(id, limit, offset) (model.AssetSlidePage) / UpdateAssetFileMeta(fileId, AssetFileMetaUpdate)
SearchAssets(id, keyword, AssetSearchOptions)
OpenDirectoryDialog(title) (string, error)   // app_preview.go，serve 无头模式 wailsRuntimeUnavailable 拒绝
```

> ListAssetSlides 自 2026-10-14 起分页（对齐 SearchAssets 分页契约：limit ≤0 取 50、>200 钳 200、offset <0 按 0；Total 恒为总条目数，翻页以 offset 递增驱动），前端幻灯片视图按页懒加载（复用 PAGE_SIZE=50）。

### 事件通道

| 事件 | 载荷 | 前端行为 |
|---|---|---|
| `assets-scan:progress` | 带 `libraryId` | 按库过滤更新进度条 |
| `assets-scan:done` | 带 `libraryId` | **仅复位进度态，不刷新数据**——数据刷新由扫描 Promise 成功分支统一执行（done 事件先于 Promise 返回，双路径各刷一次会使大库全量清单拉取翻倍，2026-10-14 收口） |
| `assets-scan:error` | 带 `libraryId` | **只复位进度不弹提示**（防与 Promise catch 的 handleError 双弹） |

`web_serve.go` `applySink` 须含 `assetsSvc.SetEventSink`（现七服务，漏加则浏览器通道静默收不到）。

## 3. Contracts

### FTS5 检索链路（核心契约）

- **表形态**：standalone FTS5 表 `search_fts` + 事务双写（external content 须配触发器/rebuild、contentless 无 snippet，均弃）。列：ref_type/ref_id/file_id 三个 UNINDEXED + file_name/file_note/slide_title/slide_text/slide_notes 五个检索列。
- **`detail=full` 必须显式保留**：phrase 查询依赖全量位置信息，降档致短语匹配失效。
- **切分同源（最高优先级坑）**：写入与查询必须调用同一 `splitFtsText`——CJK 逐字切开、拉丁/数字保持词内、混合边界插空格。两处各自实现 = 写入切分与查询切分不一致 = 静默检索漏命中，且无报错。
- **查询构造**：`buildFtsQuery` 按空白拆词 + 双引号 phrase + 多词 AND + 双引号翻倍转义；空查询返回空串 → 调用方直接返回空结果，禁把空串喂给 MATCH。**纯标点/符号词（切分后不含字母/数字/CJK rune）防御性跳过**（`hasTokenizableRune`，2026-10-14）：unicode61 对零 token phrase 实测不报错而是恒 0 行（审核标 syntax error 为 unverified 不成立），跳过走「无有效查询词 → 空结果」路径，语义明确且不依赖分词器实现细节，实测守护 `TestSearchFts_ZeroTokenPhraseIsSafe`。
- **多词摘要逐词定位**：FTS 多词语义为逐词 AND，原文通常不含多词连续字面形态——`snippetFromTexts` 整体定位失败时逐词定位取首个命中词窗口（列优先级仍高于词序），否则多词关键词摘要恒空（2026-10-14 修复）。
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
- 根目录直置的 `xxx-001.pptx`、同名异构文件不误伤；**`parentBase == "."` 提前 return false**（拆分产物恒落同名真实子目录，根目录形态不可能是派生；不提前排除时 `.-01.pptx` 这类「.」主干文件名恰与「.」前缀匹配被误判漏索引，2026-10-14 修复）。
- zip 解压字节上限（2026-10-14）：素材解析侧经 `readZipPackageWithLimits`（单条目 256MB / 全包 1GB，超限按「文件损坏」降级不中断批量），防 zip 炸弹类畸形包批量解析 OOM；**共用函数 `readZipPackage` 不设限透传，拆分链路行为逐字不变**（硬约束）。

### 扫描语义

- 增量判定：mtime+size 未变跳过（不计算哈希）；变更才 sha256；processing 残留必重处理（自愈）。
- 状态机：处理中/成功/失败(损坏)/跳过(加密)/跳过(权限)；单文件失败记日志继续，不中断批量。
- 串行保护：同库 TryLock 互斥（E_ASSETS_SCAN_IN_PROGRESS）；取消走 registerCancel/unregisterCancel defer LIFO（先注销后释放锁），映射 E_ASSETS_CANCELLED（warning）。
- 关停收口：`WaitScansDone`（2026-10-14 新增，App.shutdown 先于 `CloseAll` 调用）——逐个触发取消注册表 CancelFunc + `scanWG.Wait()` 等全部在途扫描退出；漏调则 CloseAll 先关连接、在途扫描继续写已关连接报成片 `sql: database is closed` 噪音。单文件解析内部无取消检查点，等待上界 = 单文件解析时长（zip 上限 + 流式解析，秒级）。
- 建库回滚：CreateLibrary 注册表写失败须回删预建 `<id>.db`（含 -wal/-shm，先关连接再删防 Windows 句柄占用），防孤儿 db 文件残留（2026-10-14 修复）。
- 重建：备份标签绑定 → 清 files+FTS（standalone 表**不支持 `delete-all'` 命令**，用 `DELETE FROM search_fts`）→ 全量重扫 → 按路径重绑标签（星级/备注属重建语义不保留）。
- 锁内库存在复验：`getLibrary` 校验与加锁之间有窗口，窗口内删库 + sqlite 惰性建库 = 孤儿 db 文件复活。

### 前端异步写回守卫（2026-10-14）

- 双代次守卫：`librarySeq`（库切换落定/扫描刷新自增）+ `searchSeq`（改词/切库/平铺切换/退出检索自增），参照 CommandPalette `searchVersion` 模式；await 写回前比对，不一致丢弃——切库在途时 loadStats/loadView/loadSlidesPage/ensureFileEntries 写回全部受守卫，检索在途改词/切库旧响应丢弃。**未显式传代的调用点（画廊滚动追加 loadSlidesPage(false)、溯源点击 ensureFileEntries()）由函数内部在调用时快照当前 `librarySeq`**，滚动追加在途切库同样被拦截（check 阶段补齐，勿回退为 null 恒放行）。
- `E_ASSETS_LIB_NOT_FOUND` 自愈：loadStats/loadView/loadSlidesPage/ensureFileEntries 捕获该 code 静默重拉库列表并重新落定剩余库（error.js 行为承诺落地），不弹 error。
- 视图切换兜底：目标视图清单为空即拉取（文件底册为标签聚合 tagOptions 与溯源共用数据源，库落定在幻灯片/检索视图时也预载）；检索空态命中存在但被当前视图类型过滤时引导切换视图/开平铺（不改 searchTotal 口径——其同时驱动懒加载判定）。
- 聚合展开：加载态按文件路径集合记录（`aggLoadingPaths`，并发互不覆盖）；写回前比对发起时 activeKeyword/selectedTagIds **与 `librarySeq`**（aggChildren 残留会被 toggleAgg 早退命中，同路径跨库串数据），条件变更丢弃旧响应。
- 幻灯片滚动追加防重：`loadSlidesPage(false)` 在途须置 `loadingMore`，否则滚动事件在途窗口内连续触发以相同 offset 重复追加条目（onGalleryScroll 的防重检查依赖该标志）。

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
| 纯标点/符号关键词（如 `!!!`） | 词级跳过 → 空结果路径，不报错（零 token phrase 实测恒 0 行非报错，防御性收口） |
| zip 解压超上限（单条目/全包） | 按「文件损坏」分类降级，日志，不中断批量 |
| 关停时有在途扫描 | `WaitScansDone` 先取消后等待退出，再关库连接 |
| 建库注册表写失败 | 回删预建 `<id>.db`（含 WAL/SHM）+ 清连接缓存，不留孤儿文件 |
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

**沉淀来源**：任务 10-13-dam-ppt（2026-10-13，素材管理 MVP 三批实施 + 质量检查修复）；任务 10-14-fts-14（2026-10-14，审核修复 14 项：FTS 纯标点词防御跳过与实测结论、多词摘要逐词定位、zip 解压上限、ListAssetSlides 分页、建库孤儿 db 回滚、WaitScansDone 关停收口、`.-01.pptx` 误判回归、前端双代次写回守卫 / 视图切换兜底 / 扫描单刷新 / 聚合在途守卫 / LIB_NOT_FOUND 自愈）。
