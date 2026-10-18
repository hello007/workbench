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
ListAssetSavedSearches / SaveAssetSavedSearch(AssetSavedSearch) / DeleteAssetSavedSearch(id)   // 保存检索条件，2026-10-16 第七批
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

### facet 多维筛选（SRS §2.5，2026-10-16 第七批）

- **条件构造单点**：`assetsFacetFilter`（service/assets_db.go）是三条 SQL 路径共用的唯一 facet 条件构造（`active()` / `whereClause(colPrefix)`）——keyword 批量判定 / tagsOnly 两查询追加 / facets-only 数据源；**禁单一路径自行拼条件**（口径分叉 = 判定不一致）。`active()` 判 `len(ratings)>0 || 四区间字段任一非零`；sizeFrom=0 语义「不设下界」（前端 <1MB 档 sizeFrom:0 经 sizeTo!=0 激活，0 字节文件由 `size<=上界` 天然覆盖）。
- **空查询语义重定义**：无关键词且无标签**但有 facet** → `searchByFacetsOnly`（按 facet 条件查 files 返回**文件条目**，复用 pageAndFillEntries 分页，Score=0、无聚合折叠）；关键词、标签、facet 全空仍返空页（原不变式保留）。facets-only 返回文件条目是文件维度筛选的天然清单语义，禁扩幻灯片条目（48 页 PPT 会灌满结果）。
- **幻灯片条目 facet 继承**：keyword/tagsOnly 路径幻灯片条目按**归属文件**属性判定（星级/时间/大小均文件级口径，D1——页级评分 slide_meta.rating 纯展示不参与筛选）；keyword 路径防 N+1：FTS 命中 fileID 去重全集一次 `filterFileIDsByFacet`（WHERE id IN 分批 500），循环按 fileID 查集合 O(1) 判定；判定在分页前完成（Total 按过滤后口径计）。
- **前端换算单点**：AssetFilterPopover.vue 模块级具名导出（`buildFacetOptions`/`resolveMtimeRangeNs`/`resolveSizeRangeBytes`/`countActiveFacets`/`restoreFacetStateFromSnapshot`/`describeFacetState`）——相对档→纳秒区间**每次检索现算**（防「今天」档冻结漏当日新文件）、时间相对档语义单一数据源在前端（后端只认区间）；自定义档 mtime 上界可逆锚点与快照反推详见 §8.6。`toggleAgg` 聚合子项查询刻意不带 facet（文件维度判定，父卡通过即全页通过）。

### 保存检索条件存储（service/asset_saved_search.go）

- data/asset_saved_searches.json，util.SaveJSON 原子写（整体覆盖 last-writer-wins，沿用 settings 约束）；损坏降级空列表 + slog.Warn；`sync.Mutex` 跨读改写全程；自增 ID = max+1；Save 为 upsert（ID=0 新增 / 非 0 覆盖保留原 createdAt），重名拒绝排除自身 ID。App 绑定：`ListAssetSavedSearches` / `SaveAssetSavedSearch` / `DeleteAssetSavedSearch`。

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

### 幻灯片独立标注（schema v5，任务 10-14-srs-2-2-4）

- **表形态**：`slide_tags` + `slide_meta` 独立表，**键 `(file_id, slide_num)`**，外键 `files(id)`/`tags(id)` 双 CASCADE。**禁用 slides.id 作键**——slides 每轮重解析删旧插新全变（files.id UPDATE 同行稳定），键 slides.id 则增量重解析必丢标注。无时间戳列（updateSlideMeta 无 now 参数）。
- **增量语义（位置标注）**：页数不变标注原样保留；页数减少 `deleteStaleSlideAnnotationsTx`（挂 replaceFileSlides 事务内）清失配；页内容移位标注跟随页码不跟随内容（与缩略图缓存键 file_id+page 同口径）；非 ok 终态（setFileOutcome）slides 删而标注保留，文件修复重解析后现存页序标注按位置复活。
- **重建保留**：`slideAnnotationsByPath` 按 `files.path + slide_num` 备份（两表全量）→ 重绑 defer 化（备份成功后注册，**扫描失败/取消路径也执行**——失败时部分重绑：已重扫文件恢复、未扫文件无 slides 行自然丢弃，优于全丢且二次重建可自愈；备份自身失败仍 fail-fast 中止于清库前）；`rebindSlideAnnotationsAfterRebuild` 按新 slides 现存页号写回（INSERT OR REPLACE/IGNORE），页消失静默丢弃；重绑 error 只 slog 不覆盖扫描主错误；`clearIndexForRebuild` 对两表显式 DELETE 双保险（FK 级联已覆盖，对齐 thumb_cache 先例）。
- **标签继承三契约（单点后端）**：存储层 file_tags 与 slide_tags 互不写入；筛选层默认合并【页自身 ∪ 归属文件标签】、`AssetSearchOptions.SlideTagOnlyOwn=true` 仅页自身——FTS 检索路径 `slideTagRowMatches` 与纯标签浏览 `listTagFilteredSlides` SQL 子查询消费同源关系表与同一开关，禁两套口径；文件条目不受 OnlyOwn 影响；关键词命中与标签无关不受开关影响。
- **FTS 红线**：标签与幻灯片业务备注均不入 FTS（关键词检索行为不变）；备注可检索性文件/幻灯片不对称是已知限制。`searchAssetsInStore` 早退分流：纯标签浏览路径不发 `fileIDsForTagFilter`/`slideOwnTagFilterSet` 预取查询（其结果仅 FTS 关键词路径消费）。
- **前端保存链守卫（onSaveSlideMeta）**：刷新 await 后双代次守卫（`searchSeq`/`librarySeq` 快照比对，语境变更整体 return 不写回 selected 不 reset 草稿）；保存成功先清聚合展开态（`expandedAggPaths/aggChildren/aggLoadingPaths`——页标注变化改变命中口径，旧 `aggChildren` 早退缓存=回读保存前标注再保存写回旧值）再分流命中/miss。
- **绑定**：`UpdateAssetSlideMeta(libraryID, fileID, slideNum, AssetSlideMetaUpdate{note,rating,tagNames})` 整体替换语义（对齐 updateFileMeta）；条目（AssetSlideEntry/AssetSearchHit）携带 `ownTags/note/rating` 三字段，文件条目 OwnTags 恒空切片（nonNull 契约）；保存不触发重解析/缩略图失效。



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

## 8. 视图与选中体系（SRS §2.3，2026-10-15 第五批沉淀）

### 8.1 选中键契约（第六批批量操作的消费入口）

- 键形态**单点** `frontend/src/utils/assetSelection.js` 的 `assetSelectionKey(card)`：文件 `'file:<fileId>'`、幻灯片 `'slide:<fileId>:<slideNum>'`。**禁用 slides.id 作选中键**（每轮重解析全变，与缩略图缓存键用 files.id 同理由）；类型前缀防批量消费时类型歧义。新增卡片消费点必须经该函数，禁第二处手拼键。
- 宿主持 `selectedKeys = ref(new Set())`，响应式走**整体替换**（`new Set(selectedKeys.value)` 拷贝后增删再赋值），禁原地变异（Vue 不拦截 Set 变异方法）。
- 选中维度跟随 `viewMode`（文件视图选文件/幻灯片视图选页），不设独立开关；呈现切换（网格↔列表 displayMode）**不清空**（同数据语境）。

### 8.2 清空点位矩阵（语境变更即清，漏点=幽灵选中）

| 点位 | 函数 | 行为 |
|---|---|---|
| 切库 | `watch(currentLibraryId)` | 清选中 + 关预览；关键词/标签/facet **保留**（facet 星级/时间/大小库无关，跨库保留对齐关键词/标签先例，库落定按检索语境在新库重查） |
| 检索条件变更 | `applySearch` / `onFlattenChange` / `onSlideTagOnlyOwnChange` | 清选中 + 关预览（facet 保留——与关键词/标签组合检索语义） |
| viewMode 切换 | `onViewModeChange` | 清选中 + 关预览（维度不通约） |
| 扫描刷新 | `resetAndReloadView` | 清选中 + 关预览（底册 id 重解析） |
| **溯源跳转** | `traceToSourceFile` | 清选中 + 关预览——**直赋 `viewMode.value='file'` 不经 applySearch/onViewModeChange**，是唯一同时命中 #2/#3 却绕过常规入口的路径，新增语境变更函数时对照本表逐一过；facet 同点复位（第七批：直赋路径须连 facet 一并清，否则 searchMode 残留致画廊仍显检索态而非底册） |
| **facet 变更**（第七批，双语境共用） | `onFacetChange` | 清选中 + 关预览 + 关详情 + 清文件过滤 + 清聚合展开缓存（数据过滤语境变更，按检索条件变更同类点位定性；浏览语境 facet 激活切 facets-only 链路、清除回底册） |
| **清除全部筛选**（第七批空态引导，D6） | `clearAllFilters` | 清 facet + 标签（关键词保留属「搜索」非「筛选」），其余复位面同 facet 变更；全清后回底册、关键词仍在回关键词检索 |
| **双击文件过滤进入** | `filterByFile` | 退检索分支连 facet 一并复位（过滤浏览与检索/facet 语境互斥，对齐退检索清标签同款语义） |
| 呈现切换 | displayMode radio | 不清（同数据语境） |
| **批量打标签完成** | `refreshAfterBatchTag` | **不清选中**（数据变更点位非语境变更，D5：支持对同批继续操作与失败手动重试；选中键 file:<id>/slide:<fileId>:<slideNum> 均稳定无脏键风险） |
| 标签管理变更 | `refreshAfterTagChange` | 不清选中（同上性质；检索残留已删标签 ID 命中为空无害，用户手动清） |
| **保存条件调取**（第七批 PR3） | `applySavedSearch` | 清选中 + 关预览 + 关详情 + 清文件过滤 + 清聚合展开缓存——调取即检索条件变更语境：回填（关键词含空串回填/标签/facet/两开关）后走 `applySearch` 同路径，复位面与 searchSeq 代次守卫由其统一执行；facet 快照经 `restoreFacetStateFromSnapshot` 反推（详见 §8.6） |
| 全屏预览翻页 | — | 不清（预览即详情导航，侧栏标注编辑照常可用） |

- 全选/反选作用域 = **当前渲染独立卡键集**（检索模式含已加载段，懒加载未加载段不在范围）；反选 = 渲染集 ∖ 当前选中（渲染集外键结构性清零防幽灵）。聚合父卡（hitCount>1）与聚合展开子行无复选框（勾选语义歧义）。

### 8.3 全屏预览层

- **层级选型**：预览层 z-index **10、浮动操作条 5——面板内局部值**，定位语境 `.assets-view-wrap`（画廊列自身 relative，**非** `.assets-body`——以 body 为 containing block 会连详情侧栏一并覆盖，违反「预览态详情可编辑」）；不触碰全局右键菜单 2000 / EP 弹窗 2001+ 惯例档。预览层内 `.preview-nav` 须显式 `z-index:2` 压过 `.preview-thumb` 的 `z-index:1`（hd 图 ready 后铺满 stage，漏设则按钮被绘制层盖死——jsdom 无布局引擎测不出，真机冒烟项）。
- **翻页序列** = `displayCards.filter(kind==='slide')`（浏览清单已加载段滚动续拉 / 检索命中序列；聚合展开子项不在序列，从子项详情打开按归属文件路径回退定位）。零后端改动；「单文件内翻全部页」需 `ListAssetFileSlides` 类只读绑定，留后续批独立增量。
- **键盘守卫**：document keydown 过滤 `event.defaultPrevented`、目标 `.el-overlay`/`.xterm-helper-textarea`（终端全屏 ESC 惯例）+ **方向键追加可编辑元素聚焦跳过**（INPUT/TEXTAREA/SELECT/contenteditable——预览态侧栏标注编辑时方向键是光标移动语义，翻页会打断编辑）；`onBeforeUnmount` 注销监听。
- **序列收缩兜底**：`watch(previewSlides.length)` 越界即关预览——浏览态保存标注触发 `loadSlidesPage(true)` 复位首页后序列可能短于当前下标，漏兜底则 overlay 进入「非空 index 但不渲染」的不可见卡死态。
- 保存标注触发的检索重跑**不关预览**（同条件重跑序列内容不变，预览态编辑属预期 UX），越界由收缩兜底覆盖。

### 8.4 失败缩略图自动补（恰一轮）

- `onThumbsDone` 内 `failedThumbPaths` 非空且本轮未自动重试过 → 逐个 `RetryAssetThumb`（fileId 从 fileEntries 按 path 反查，miss 跳过）→ 普通 let 标记已重试；下轮 done 不再触发（损坏类必然再失败，一轮即止不放大）；切库 watch 复位标记（新库新批次语义）。fire-and-forget 不弹提示，epoch 代次驱动 miss 卡补图不变。

### 8.5 列表视图（AssetListView）

- 受控组件零业务状态：选中/详情/检索代次/分页判定全留宿主；行点击 emit `card-click`、滚动到底 emit `load-more`（防重与分页知识在宿主 `loadMoreInView` 单点，网格 scroll 与列表/预览 load-more 三入口共用）。
- 列集分语境（prd D3）：幻灯片与检索条目的 size/modTime 经 fileEntries 底册按 filePath 反查 join（归属文件属性口径，表头 tooltip 声明），底册 miss 显 `—` 不猜值；聚合条目大小 `—`；星级 `0` 显 `—` 且**不落底册兜底**（`0` 非 nullish，页级标注独立于文件标注，兜底会伪造页星级）。检索幻灯片条目追加命中摘要列。

### 8.6 保存检索条件（SRS §2.5 PR3，2026-10-16 第七批沉淀）

- **快照口径**：`buildSavedSearchSnapshot` 单点产出 `model.AssetSavedSearch` 全维字段（keyword/tagIds/tagNames 冗余/ratings/mtimeFrom/mtimeTo/sizeFrom/sizeTo/flatten/slideTagOnlyOwn/libraryId；id=0 新增、非 0 覆盖，createdAt 后端权威生成不传）。tagNames 从共享标签列表按 id 反查、与 tagIds **同序对应**（映射 miss 成对丢弃防两份数据错位）；facet 存**区间原始值**（`buildFacetOptions` 换算单点产出），摘要预览（`describeFacetState`）时间维展示解析后的绝对区间与还原口径一致，禁摘要显档位而数据存区间的两套口径。
- **区间反推可逆锚点（换算单点在 AssetFilterPopover 模块）**：自定义档 mtime 上界取**止日末毫秒**（非 1ns）——纳秒值量级 ~1.8e18 超出 JS Number 2^53 安全整数，1ns 尾差经双精度舍入（该量级 ULP≈256ns）完全丢失不可反推，毫秒尾差可精确还原（毫秒级尾差在文件系统 mtime 实际粒度 NTFS 100ns / ext4 ns 下闭区间含止日全天语义等价）；`restoreFacetStateFromSnapshot` 反推：mtime 双侧非零 → custom 冻结区间（**相对档保存即冻结为绝对区间，保存的是什么区间调取就是什么区间**）、双侧反推毫秒用 `Math.round` 而非 floor（JSON 传输存在 ±128ns 双精度舍入，floor 在舍入向下时跨过整毫秒边界落错一日）；size 精确匹配四档闭区间、未匹配防御回退不限（无自定义大小档 UI 不为异常数据造档）；mtime 单侧非零为异常数据回不限档（本组件换算恒双侧成对产出）。
- **调取即语境变更**（§8.2 矩阵登记）：回填走 `applySearch` 同路径，与手动改条件无分叉；关键词空串照回填（facet-only 条件调取后正确退关键词）；异库条目降级（D4）——tagIds 属保存时所在库不可复用，按 tagNames 在当前库标签表重查 ID，miss 丢弃该标签并计数提示（warning toast），关键词/facet/两开关照回填。
- **覆盖边界**：覆盖 = 以当前检索状态 Save 同 id（名称沿用原名，MVP 无改名语义，createdAt 后端保留原值）；**仅本库条目可覆盖**——覆盖按当前库重写 libraryId 会使条目静默搬家；非 searchMode（无可快照条件）禁用。重名拒绝/名称校验由后端 Save 返回经 handleError 透传（弹窗保持打开可改名重试）。
- **列表权威**：「已存搜索」弹层打开（@show）与保存/覆盖/删除成功后均 List 重拉（本地不缓存增删）；行级防重 `savedSearchMutatingId` 占位（覆盖/删除在途禁同行重复提交）。

## 9. 批量打标签与多级标签树（SRS §2.4，2026-10-16 第六批沉淀）

### 9.1 标签树数据源与级联命中

- **数据源**：单一 tags 表，`parent_id` 启用（0=根；v1 基线建表已含该列，**零迁移**）。父标签同为 tags 表一行；标签自动创建（遇新名）落为根标签。标签选项独立 API `ListAssetTags`（全量含未使用标签 + 直接绑定计数 = file_tags 与 slide_tags 两处绑定行数合计；父节点子树合计**前端自算** `sumSubtreeCounts`）——前端 `tagOptions` 底册聚合已降级为兜底，主源切该 API。
- **级联命中（筛选聚合语义）**：`searchAssetsInStore` **两路径分流前**单点调 `expandTagDescendants` 把所选 tagIDs 展开为「自身 ∪ 全部后代」闭包集（FTS 关键词路径与纯标签浏览路径同源，禁只展开一路）；平铺老数据（全 parent_id=0）展开=自身集合，行为与历史完全一致。
- **级联边界**：**批量打标签勾父标签只绑定被勾标签行本身，不做级联绑定**——绑定是精确动作，筛选才是聚合语义（混用会静默污染未勾选条目）。

### 9.2 批量 API 契约（每条目一事务 + 明细）

- 两独立方法按形态分叉：`BatchUpdateAssetFileTags(libraryID, fileIDs, op)` / `BatchUpdateAssetSlideTags(libraryID, targets[]{fileId,slideNum}, op)`；**禁 kind 参数单方法**（service 层两套存储分流在签名内显式化）。
- `AssetBatchTagOp{AddTags, RemoveTags []string}`（标签名）：语义 = 每条目「现标签集 − remove ∪ add」，**相对条目现状追加/移除非整体替换**；只动标签不动星级/备注。交集非空/两数组全空/目标空 = 参数错误（跟随 UpdateFileMeta 既有普通 error 形态，**未新增错误码**，前端弹窗提交禁用为第一防线）。
- **事务边界：每条目一个事务**逐条目 commit，失败记 `Item{fileId, slideNum, status, message}` 继续下一条（部分失败不中断不回滚，对齐拆分 splitOutcome 明细先例）；返回 `AssetBatchTagResult{Total, Succeeded, Failed, Items}`。
- **批量内标签创建单点**：`ensureTagsByNames` 批量前一次性 ensure 全部 add 名（自动建根标签 + 库内 name UNIQUE 兜底），禁逐条目重复 ensure（同名词重复创建竞态 + 每条目重复查询浪费）。
- slide_tags 批量变更**无需 FTS 写入**：标签不入 FTS 检索列（既有契约），筛选命中口径由关系表实时读天然同步。

### 9.3 标签 CRUD 契约

- 删除 = `deleteTagTree` **单事务**：先闭包收集子树 ID 集，逐 ID 显式 DELETE 两绑定表（**双保险**——不解绑直接删标签会留悬挂绑定行），返回解除关联绑定行数；SRS「删除标签不删素材」逐级适用。确认弹窗影响面：N=前端子树后代数、M=子树计数合计，删除后以后端返回值为权威展示。
- 防环 = `updateTagFull` 沿 parent 链向上遍历途经自身即拒绝（visited 防脏数据死循环）；前端管理弹窗父级选项编辑态排除自身与子树（预拦截，后端主防线）。
- 重名拒绝（库内 name UNIQUE）；`ListTags` 计数 SQL 用两独立相关子查询 COALESCE 归一。

### 9.4 前端组件契约

- **AssetTagTreeSelect 单组件三处复用**（筛选下拉/详情编辑/批量弹窗）：基于 el-tree-select（multiple + filterable + checkbox）；**组件统一 ID 口径**，消费方映射（详情编辑经 detailTagIds/slideTagIds 可写计算属性转标签名数组提交，保持 UpdateAssetFileMeta 名称契约不变）；挂载即拉 + 下拉展开重拉 + libraryId watch 重拉，**fetchSeq 代次守卫**防快速切库旧库慢响应写回；`loading` 刻意不透传 select（EP 透传会隐藏下拉主体且 disable 关闭打断展开重拉，重拉期间树保持可见原位更新）。
- **AssetBatchTagDialog 不感知 viewMode**：只收 `targets`（含宿主拼装定位 label）与 `execute` 分派回调 props，宿主按 viewMode 决定调哪个批量 API；失败明细 label 反查 `(t.slideNum || 0) === (item.slideNum || 0)` 双侧归一（文件形态后端 omitempty 省略 slideNum）；`succeeded > 0` 恒上报 completed（每条目一事务成功已写入）。
- **批量刷新链** `refreshAfterBatchTag` / **标签管理刷新链** `refreshAfterTagChange`：树 reload + 底册重拉 + 检索态重搜 + **检索态清聚合展开缓存**（expandedAggPaths/aggChildren/aggLoadingPaths——删除/移除页自有标签改变继承合并命中口径，旧 aggChildren 缓存早退会回读变更前数据）+ **librarySeq/searchSeq 双代次快照守卫**（await 期间切库/退检索防旧数据写回，对齐 onSaveSlideMeta 先例）。刷新链刻意不触碰已打开详情草稿（防覆盖未保存编辑，重选条目即得新数据）。
- 树构建 `buildTagTree`（utils/assetTagTree.js）对悬空父引用/自引用按根处理（容错不炸渲染）；插槽作用域解构须 `scope && scope.data` 守卫（宿主测试环境未注册 el-tree-select 时无参插槽解构炸渲染链，Home.spec 曾 79 用例全挂）。
- **过滤浏览语境刷新缺口（真机冒烟修复）**：`fileFilterPath` 客户端过滤语境下，任何对幻灯片清单 `loadSlidesPage(true)` 复位首页的刷新链（`refreshAfterBatchTag` / `onSaveSlideMeta` 浏览分支）**必须接 `ensureFilteredSlidesLoaded()` 续拉兜底**——复位后目标文件页大概率不在已加载段，过滤收敛 0 卡（用户见「全部卡片消失」）；`watch(fileFilterPath)` 续拉只在过滤值变化时触发，刷新场景过滤值不变接不上。非过滤语境该函数空目标直接返回零行为变化。

### 9.5 标签兄弟序与拖拽排序（D8#1 真机增量，schema v6）

- **schema v6 迁移**：`ALTER TABLE tags ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0` + 迁移内纯 SQL 回填——按迁移前展示序（同父内 name 升序、同名按 id，对齐 v5 `ORDER BY name` 口径）经相关子查询计数赋组内 0..n-1，升级后视觉顺序与迁移前完全一致，用户再拖拽自定义；**v1 基线 tags 表不含该列**（基线含列时旧库 ALTER 会撞 duplicate column），新旧库统一经 v6 补列；回填仅随迁移执行一次（user_version 门禁），新库 tags 空表天然空操作。
- **兄弟序写入契约（组尾追加 + 连续化重排）**：`createTag` / `ensureTagTx`（批量自动建根标签）/ `updateTagFull` 移父三类入口经 `nextTagSortOrder`（组内 MAX+1，空组 0）追加组尾，防 DEFAULT 0 插队组首——updateTagFull 移父**仅追加新组组尾、旧组留断号**（ORDER BY sort_order, id 相对序语义不受影响，仅拖拽路径 MoveTag 做连续化）；`MoveTag` 单事务 = 防环（tx 内读边集沿 parent 链向上遍历，收集与落位同事务无「校验与写入之间树被改」窗口）+ index 越界钳 [0, 组尾]（前端落点正常不越界，钳制为防御）+ **目标组整组连续化重排 0..n-1**（写整组而非只写落位标签，防断号失档）+ 跨父时旧组剩余兄弟连续化（消除断号，同父移动由目标组重排覆盖）；`ListAssetTags` 平铺序 = 父级分组（根组在前按父 ID 升序）+ 组内 sort_order、同序按 id——前端 `buildTagTree` 保序消费即兄弟展示序单一来源，前端不重排。
- **前端拖拽换算单点**：`utils/assetTagTree.js` 的 `resolveTagMoveDrop`（before/after = 同级排序，取落点节点在排除自身后兄弟列表中的序位；inner = 变子级组尾追加）与 `applyTagMoveToFlat`（乐观更新平铺清单，返回新数组不改入参）；el-tree `allow-drop` 预拦截与 `node-drop` 落点换算**必须同走 resolveTagMoveDrop**（两处口径分叉 = 预拦截放行而落点拒绝的错位形态）；防环两层（前端拖入自身子树一律拒绝 + 后端 moveTag 沿 parent 链主防线）；乐观更新失败回滚 + 重拉保权威，落位在途（moveBusy）暂停 draggable 防并发拖拽叠加。
- **flatten 默认 true（D8#2）**：检索聚合折叠/平铺开关默认平铺打开（同一 PPT 多页命中逐页平铺为主路径），聚合折叠按需关回；聚合形态相关断言须先显式回关再触发（勿依赖旧默认 false）。

---

## 10. 检索排序器与批量评分（SRS §2.5.3 / §2.6，2026-10-16 第八批沉淀）

### 10.1 检索排序契约

- **排序参数**：`AssetSearchOptions.SortBy`（"" / `AssetSortByRelevance`="relevance" 默认 / `AssetSortByMtime`="mtime" / `AssetSortByRating`="rating"；非法值按相关性处理）+ `SortDir`（"" / "desc" 默认降序 / "asc"；仅 mtime/rating 有意义）。常量为**普通 string 常量**非具名 string 类型（规避 wails models.ts 别名缺失坑，见 cross-layer-contracts）。
- **排序口径统一文件级**（与星级 facet 第七批 D1 同源）：mtime 档 = `files.mtime_ns`、rating 档 = `files.rating`，幻灯片条目/聚合条目**继承归属文件**；页级 `slide_meta.rating` 纯展示与详情编辑，不参与排序。相关性档 = FTS bm25 序现状不动（聚合组首 = 组内最高分页，SRS「聚合按最高单页相关性」天然满足）。
- **排序实现三层单点**：`assetSortKey`（入参归一化）→ `sortEntriesByOptions`（Go 层稳定排序唯一出口，三路径共用禁分叉）→ 主键方向随 SortDir 翻转、**次级键链方向恒定**（score → sortPath → sortNum → refID）保证同值条目跨分页稳定。**次级键必须含 sortNum**：同路径文件条目与页条目主键/路径同值时若无 sortNum，落到 refID（files.id 与 slides.id 独立自增序列）决序，「文件本体先于其页」语义依赖 ID 分配顺序这一脆弱隐式依赖（重解析后页 ID 大增即破）。
- **执行位置**：严格在**聚合之前、分页切片之前**（聚合组首 = 排序后首现条目，文件维度单值继承天然正确；排序后聚合无需改 `aggregateSlideEntries`）。相关性档 `sortEntriesByOptions` 直接原样返回，零查询零重排。
- **关键词路径排序键获取**：FTS 命中 fileID 全集一次 `fileSortPropsByIDs` 批量 IN 查询（分批 `assetsQueryIDBatch` 防 N+1）；标签/facets-only 路径 SELECT 扩列（mtime_ns/rating，幻灯片 JOIN files 继承）。**props 缺键 = 文件行已消失（并发删除窗口）→ 该 fileID 全部条目剔除**而非零值参与排序（零值 mtime 把已删文件排到序端成可见异常序；剔除语义与 pageAndFillEntries 填充层「元数据消失跳过」一致，且剔除发生在 total 计数前，total 收敛为存活条目）。
- **保存检索条件快照**：`AssetSavedSearch` 补 `sortBy`/`sortDir`；JSON 容错反序列化零值 = 相关性（旧快照无版本号向后兼容）；前端 `applySavedSearch` 还原且 undefined 容错回退。
- **前端**：检索工具行独立排序下拉（不入 AssetFilterPopover——排序高频切换与 facet 低频筛选不同频）；相关性无升降、mtime/rating 同档再点翻转升降（切档回默认降序）；排序变更 = 检索条件变更点位走 `applySearch` 同路径（searchSeq 守卫 + 复位面统一执行，§8.2 矩阵登记）；排序参数随 `SearchAssets` opts 下发，**前端零内存排序**（分页懒加载下前端排序必错段）；聚合展开子项查询（toggleAgg）刻意不传排序参数（同文件子项主键同值，次级键序与 FTS 序一致零行为差异）。排序状态跨库保留（对齐 facet 切库保留先例）。

### 10.2 批量修改评分契约（SRS §2.6 P0）

- **API**：`BatchSetAssetRating(libraryID, targets []AssetRatingTarget{FileID, SlideNum}, rating int64)` 单一入口混合文件/页目标（slideNum 0 = 文件条目）——与批量打标签「两方法按形态分叉」刻意不同：评分无标签树联动、无 Add/Remove 相对语义，单方法足矣。rating 0-5 越界/空目标/库不存在 = 参数错误普通 error（未新增错误码）；0 = 清除评分。
- **每条目一事务**逐条目 commit（splitOutcome 先例，部分失败不中断）；返回 `AssetBatchRatingResult{Total, Succeeded, Failed, Items}`，Item 含后端拼装 `Label`（条目事务内存在性校验处顺带 SELECT path，零额外开销；弹窗以后端 Label 第一数据源、targets 反查 `slideNum||0` 兜底）。Status 复用 `AssetBatchTagStatus*` 常量。
- **页条目 upsert slide_meta 键 (file_id, slide_num) 禁 slides.id**；INSERT OR REPLACE 整行必须**回读现存业务备注再整行写回**（漏回写清备注）；文件存在性+页行校验在写入同事务内。
- **前端**：AssetSelectionBar「评分」按钮 + `AssetBatchRatingDialog`（复刻 AssetBatchTagDialog 形态：targets+execute props、不感知 viewMode）；el-rate 星级 0-5 单选 clearable（0=清除）；targets 反查收敛共享单点 `collectSelectionTargets`（打标签/评分两消费方 slideNum `||0` 双侧归一）；刷新链抽 `refreshAfterBatchAnnotation` 共享体（评分跳过标签树 reload），completed 后**不清选中**（§8.2 数据变更点位）；星级 facet 命中联动属正常数据变更不清选中。
- **评分不入 FTS**（评分非检索列，五契约「切分写入/查询同源」不涉及；rating 排序键实时读 files 表天然同步）。

---

## 11. 导出元数据与文件丢失重链（SRS §2.6.4 / §2.6.5，2026-10-17 第九批沉淀）

### 11.1 导出契约（service/assets_export.go）

- **API**：`ExportAssetMetadata(libraryID string, opts model.AssetExportOptions) (model.AssetExportResult, error)` + `CancelAssetExport(libraryID) bool`（App 层透传 `context.Background()`，对齐 StartAssetScan 形态——绑定签名不带 ctx）。
- **范围三态（D2）**：选中（`FileIDs` 非空，**条目集 = 行集一一对应**：文件条目出文件行、页条目出页行，不做文件展开全部页——与 collectSelectionTargets 批量操作语义对称）> 检索重放（`Search` 非零值，复用 `searchAssetsInStore` 三路径，Flatten 恒 true 分页循环取满 Total，前端已加载页不参与）> 全库（双 nil 不设界）。**排序档单独存在不构成检索语境**（前后端同口径）。
- **三段式锁解耦（P1 教训）**：`withStore` 持全部库共享一把 `s.mu`，万文件级导出全程包单会话 = 分钟级阻塞素材域全部操作（检索/统计/扫描/拆分）。正确形态：①短会话物化行键（`assetExportRowKey{fileID,path,slideNum}`）→ ②行键分批（`assetsQueryIDBatch=500`）每批**新开短 withStore 会话**水合 → ③批内产物**锁外**写汇 + os.Stat 存在性探测。**批间禁 OFFSET 续读**（释放锁后并发写插入使分页漂移），物化快照语义：物化后新增文件不进本次导出、水合 miss（物化后被删）跳过计数。
- **物化 SQL 文件行必须独立 UNION ALL 一路**：单条 `LEFT JOIN + COALESCE` 只为**无页文件**产出文件行、有页文件不产文件行（thumb/split 探测用例捕获）——文件行一路 + 页行 JOIN 一路，`ORDER BY path, row_num` 作用于复合查询整体（SQLite 合法）。
- **CSV**：BOM 三字节（0xEF 0xBB 0xBF）+ `encoding/csv` `UseCRLF:true` 全字段引号；目标打开 overwrite=false 走 `O_WRONLY|O_CREATE|O_EXCL`（失败且目标实存 → `E_ASSETS_EXPORT_TARGET_EXISTS` warning 确认型），overwrite=true `O_TRUNC` 直写——入口 os.Stat 预检仅作 UX 提前返回（TOCTOU 收口在打开原语）。中途失败 `w.abort()` 删半成品 CSV。
- **MD**：目录形态每页一卡 `<3位序号>-<标题清洗>.md`（Windows 非法字符/控制符/尾点空格清洗 + 60 rune 截断，**全局流水号前缀兜重名碰撞**）；YAML 路径反斜杠统一 `/`（`yamlAssetPath` 单点）；覆盖 = 同名卡覆盖写不清理陌生文件。
- **路径口径（D5）**：`source_file_path` 默认库相对（基准 root_path，`filepath.Join(rootPath, FromSlash(rel))`）可选绝对；`thumbnail_path` **恒绝对**（缓存在 data/ 不在库内，small 档 `thumbPNGPath` 单点同口径，缺档导空）；`single_pptx_path` 跟随 source 口径 + **split_records 为权威探测**（库内产物 relative 化正斜杠、库外 derived 恒绝对、页序越界导空）。
- **字段与粒度（D3/D4）**：SRS 11 列清单，`ocr_text`/`summary` 保留列恒空（向前兼容 OCR/AI 摘要）；页行（slide_num≥1，note/rating 页级 slide_meta）+ 文件行（slide_num=0，文件级）；tags 继承合并**后端单点 mergedTags**（CSV/MD 共用，禁导出层第二口径）。
- **中止通道（真中止，镜像扫描/拆分形态）**：`exportCancels` 注册表三件套（同键覆盖写 + 条目指针比对防误删 + 无在途幂等 false）；`CancelExport` 仅取 `opMu` **不触 `s.mu`**（否则被在途导出 withStore 会话阻塞，取消恰恰在导出进行中调用）；ctx 检查点在行键分批环顶部（物化与单批水合有界不可中断，头注释声明）；取消错误复用 `E_ASSETS_CANCELLED` + 导出措辞消息（`mapAssetExportCancelError` 单点）。abort 一次性语义：每次调用全新 writer，失败/中止统一 abort 删半成品。
- **弹窗放弃语义**：提交中关闭 = 标记 `abandoned` + 触发 cancel + 放行 done（不做硬阻断——无取消出口时用户锁死弹窗）；放弃后成功竞态（后端先于取消完成）= info「已中止等待，导出已在后台完成」+ 照常 emit exported 不重复关窗；放弃后失败（含 E_ASSETS_CANCELLED）静默；覆盖确认期间放弃不重调；footer 中止按钮显式走 abandonExport 单点。

### 11.2 lost 状态机（扫描/清理/统计）

- **标记**：扫描消失分支 DELETE → `markFileLost`（仅非 lost 行幂等；**size/mtime/hash 原值保留**作复活与重链判定基准；FTS 行保留——lost 文件仍可关键词检索辅助重链；标签/页标注/评分/缩略图记账与 PNG 全存续）。`AssetScanResult` 加 `Lost`（本次新标记）/`Revived`（本次复活）计数，`Removed` 兼容保留恒 0。
- **复活资格门槛 `fileHasSlides` 单点**（扫描复活与前缀替换匹配收敛**共用**，禁第二口径）：有解析产物（曾成功解析）才直复活 ok；`failed/encrypted/denied` 或 0 页空演示文稿行回归**落既有重解析链路按实际内容重分类**——直复活会让无产物行谎报 ok 且此后 `unchanged` 跳过永不纠正。**processing 行永不直复活**（扫描 `markProcessing` 自愈不变式「必重处理」，重链 relocateFileOK 与替换匹配收敛两口径对齐，防崩溃残留行绕过自愈）。
- **复活语义**：磁盘命中且 mtime+size 未变 → 直 ok 不重解析（内容未变旧页/缩略图有效）；已变走既有重解析（失效缩略图联动）。
- **清理 `ClearLostAssetFiles`**：事务内逐条 `SELECT status` **复验资格**（同写事务串行化无复验窗口；并发复活行跳过含 FTS/thumb_cache 清理）+ `DELETE AND status='lost'` 双保险 + **RowsAffected 实际删除数回传**（返回数语义 = 实际删除，非收集快照数）；等效旧 deleteFilesByPaths 删除语义（FTS 文件行+页行显式删 + thumb_cache 显式删 + files 四表级联 + 记账 dirty 失效 + PNG 事后删）。与扫描并发定性接受（最坏 FK 日志噪音，下次扫描自愈；换取清理入口不被长扫描阻塞）。
- **统计两口径**：`GetLibraryStats.LostCount`（存量）与扫描结果 Lost/Revived（本次增量）分离呈现，勿混用。

### 11.3 手动重链与前缀替换（service/assets_relink.go）

- **`RelocateAssetFile(libraryID, fileID, newAbsPath, force)`**：越界遏制（IsAbs + Rel + `..` 前缀，对齐 RetryAssetThumb 先例，`E_ASSETS_RELINK_OUT_OF_LIB`）；目标 path UNIQUE 冲突预检（owner==自身放行；预检-UPDATE 间隙 TOCTOU 定性接受——单用户桌面 + 前端按钮级防重，UNIQUE 约束兜底非裸 500）；哈希相等**且非 processing** → 单 UPDATE 改 path + 刷 mtime/size + status=ok 直复活（防「下次扫描误判变更」）；不等 → `E_ASSETS_RELINK_MISMATCH` warning 确认型，force=true 后改 path + markProcessing + `processScanFile` 重解析（tryLockScan 与扫描互斥 + 锁内复验库存在）；hash 空记录（加密/权限跳过）跳过校验。不做扫描时自动哈希匹配（万文件级成本，SRS 仅要求手动重定位）。
- **前缀替换目录边界语义**：命中 = `path == oldPrefix OR path LIKE oldPrefix || '/%'`（**`escapeLikePattern` 转义单点** `%`/`_`/转义符自身 + `ESCAPE '\\'`）；Preview 与执行同一构造禁两口径。越界判定 = 新路径逐条 IsAbs+Rel，**「中段 `..` 清理后仍逃逸」才真越界**（`旧/../逃逸` 被 filepath.Clean 消化不越界，勿凭字面误拒）；任一越界整体拒绝（零半态）。**冲突预检排除替换集自身记录**（其旧路径在本事务内全部腾空，「旧目录 → 旧目录/归档」归档重组合法，勿误拒）。
- **嵌套腾位与探测配对（P0 教训双坑）**：renames 按 **oldPath 长度降序**落库（嵌套链式腾位安全序：碰撞链中占位方 oldPath 恒严格长于被占位方新路径，无环，长度降序即事务内无瞬态 UNIQUE 撞击；`sort.SliceStable` 无输入序假设）；**三态探测配对禁下标 zip**——renames 已按长度降序重排、records 保持 path 升序，两者不同序，`records[i]` 配 `renames[i]` = 页索引/哈希/FTS 串写进错误记录 + 现存文件误标 lost（混合长度路径即触发；既有嵌套用例因 UTF-8 字节序巧合未暴露，`TestAssetPrefixReplace_MixedLengthPairing` 以两序分化夹具 + 前置断言守护）。正确形态：renames 遍历 + `map[int64]assetFileRecord` 按 fileID 取记录（缺键防御上抛——结构性不可达但防未来来源变更零值静默错写）。
- **写面 RowsAffected 校验（并发清理窗口）**：`replaceFilePaths`/`relocateFileOK`/`relocateFileProcessing` 校验 RowsAffected，零命中（记录已被并发 ClearLostAssetFiles 删除）= `sql.ErrNoRows` 上抛 → 调用层转「记录已被清理，请刷新后重试」可读错误；替换零命中即事务整体回滚（旁观行不落库，探测段自然不进入）——防 Replaced 按名册计数虚报 + 幽灵行探测段 FK 违例模糊错误。
- **执行链**：单事务批量 UPDATE → 事务后逐条探测三态（匹配复活[非 processing]/已变 markProcessing+重解析/缺失保持 lost）→ **锁释放后** `StartScan` 自动收敛（锁未放自取锁会自撞 SCAN_IN_PROGRESS）。
- **前端**：重链结果 toast 分型**先判 `reparsed`** 再判 `hashMatched`/`skippedHashCheck`（processing 行后端返回 hashMatched=true 且 reparsed=true，hashMatched 首判误报「元数据直接恢复」）；维护下拉（前缀替换/清理丢失，零丢失禁用）；`refreshAfterMaintain` 共享刷新链双代次守卫；AssetPrefixReplaceDialog 防抖预览（`previewCount` -1 哨兵区分统计中/零命中两态 + 输入即置 null 回占位防陈旧 0 误拦提交 + 代次守卫）。

### 11.4 Wrong vs Correct

**Wrong（导出锁形态）**：
```go
func (s *AssetLibraryService) ExportAssetMetadata(...) {
    return s.withStore(libraryID, func(store *assetsStore) error {
        // 全程单会话：物化 + 水合 + 写汇全在锁内，分钟级阻塞素材域
    })
}
```
**Correct**：
```go
keys := s.withStore(...materializeRowKeys...)   // ① 短会话物化
for batch := range batches(keys) {              // ② 分批
    rows := s.withStore(...hydrateBatch...)     //    每批新开短会话
    writeToSink(rows)                           // ③ 锁外写汇 + os.Stat
}
```

**Wrong（探测配对）**：
```go
for i, rec := range records {
    probe(records[i], renames[i].newRel) // renames 已重排，下标 zip 串写
}
```
**Correct**：
```go
recByID := map[int64]assetFileRecord{...}
for _, rn := range renames { // 按 renames（腾位安全序）迭代
    rec, ok := recByID[rn.fileID] // fileID 配对，缺键防御
}
```

---

## 12. 索引配置（排除规则 + excluded 态）与素材库备份（SRS §2.7.3 / §2.7.5 / §14.16，2026-10-18 第十批沉淀）

### 12.1 排除规则匹配语义（service/assets_exclude.go）

- **ExcludeDirs 每项一个 glob 模式**（`filepath.Match` 单段语法 `*` `?` `[...]`），对相对库根路径的**每一层目录段名**做完整匹配（等价 gitignore「无斜杠模式」语义），**大小写不敏感**（模式与段名各自 ToLower 后 Match）；**不支持 `/` 与 `**` 多段路径模式**（四个对标工具 Billfish/Eagle/Bridge/Everything 均无多段 glob 排除，标准库零新增依赖即足额——语义定单段后 doublestar/gobwas/gitignore 包全部不需要）。目录剪枝用**目录基名**匹配（`d.Name()`），无需 Rel，天然规避 Windows 下 `filepath.Match` 的「/」不匹配「\」分隔符坑；**root 自身豁免**（`p == root` 不参与匹配，防整库剪空）。
- **ExcludeExts 黑名单**存储归一带点小写（`.tmp`），`normalizeAssetExcludeExt` 单点：TrimSpace → 剥 `*.` → 剥前导点 → 补点 → ToLower，`tmp`/`.tmp`/`*.tmp` 三形态等价；匹配 `matchExt`（`path.Ext` 对 slash 路径取段，与 `isPptxSplitDerivative` 口径一致）。
- **两条链共用同一 matcher（禁两套口径）**：收集层磁盘过滤（`collectPptxEntries` WalkDir——目录段命中 `SkipDir` 剪整棵子树、后缀命中不入清单）与 excluded 判定（`matchPath`：存量索引 slash 相对路径按段拆，任一目录段命中或文件后缀命中即命中；**文件基名不参与目录规则**）在扫描入口 `newAssetExcludeMatcher` 编译一次闭包捕获复用，禁每文件重编译/重复 ToLower。
- **坏规则双防线**：保存 API `normalizeAssetExcludeRules`（第一道）——含路径分隔符（`/` 与 `\`，单段语义下永不命中，拒绝防误配静默失效）与坏 glob（`filepath.Match` ErrBadPattern 探测）拒绝、空白项丢弃、后缀归一；matcher 编译期（第二道）——手改 libraries.json 绕过校验的坏模式**剔除并 slog.Warn**（该规则不生效而非扫描报错），`matchDir`/`matchExt` 对 nil receiver 与运行期 Match 错误恒不命中。
- **保存即生效**：`UpdateLibraryRules`（整体替换语义）→ `setLibraryRules`（s.mu 持锁改注册表两字段 + UpdatedAt，util.SaveJSON 原子写）→ 自动 `StartScan` 增量重扫（复用既有串行锁/取消注册/assets-scan:* 事件通道，可取消）；**同库扫描进行中规则已保存但重扫被拒**（E_ASSETS_SCAN_IN_PROGRESS——在途扫描按启动时快照的旧规则执行，结束后手动重扫生效）。
- **优先级**：`isPptxSplitDerivative` 强制排除（系统内置）先于用户规则，两者均排除无包含语义。

### 12.2 excluded 状态机（与 lost 的同与异）

- **判定顺序（先排除后存在性）**：扫描先对存量索引逐条 `matchPath`，命中即 `markFileExcluded`（**不做磁盘存在性探测**——命中的磁盘文件也不会出现在 entries，收集层已过滤）；任意现存态命中即标（含 lost→excluded），幂等（已 excluded 行跳过不重复计数，`AssetScanResult.Excluded` 仅记新增）。
- **无损保留（对齐 lost）**：`markFileExcluded` 仅翻 status + updated_at——size/mtime_ns/hash 原值保留（复活判定与 lost 同口径走 mtime+size 匹配），slides/标注/标签/FTS 行/缩略图记账与 PNG 全保留。
- **复活**：规则改宽后（能进 entries 即本轮不命中）磁盘仍在 → 走增量三态，unchanged 行经 **fileHasSlides 资格门槛单点**直复活 ok（与 lost 回归共用同一分支与门槛，禁第二口径）；excluded 且规则仍命中即使文件消失**保持 excluded 不转 lost**（规则优先——gone 收集循环对规则命中行 `continue`），规则改宽后消失的文件才可见 lost。
- **可见性分叉（与 lost 相反）**：检索/标签浏览/facet **全入口不可见**——关键词路径 FTS 行保留但 `excludedFileIDSet` 批量预取（分批 IN，形态对齐 filterFileIDsByFacet）循环内按 fileID 剔除（幻灯片条目随归属文件一并出局，判定在分页前 Total 收敛）；**标签浏览与 facets-only 路径 SQL 恒 `status='ok'`**，excluded 天然排除零额外查询。**导出照导**（行键无 status 过滤，`TestAssetExport_IncludesExcludedRows` 锚定）；库统计 `ExcludedCount` 计入（FileCount 含各状态）。
- **本批无清理入口**（后批复用第九批 lost 清理链路）；文件底册（ListAssetFiles）恒全状态返回，excluded 行在底册可见（用户感知面）。
- **直复活必须同事务重建 FTS 行（审核修复 🔴）**：excluded/lost 期间用户经文件底册可编辑该文件备注/标题——`updateFileMeta` 经 `syncFtsFileRowTx` 对非 ok 行**清** FTS 文件行；此后规则改宽 mtime+size 未变走直复活（每轮 unchanged 永跳过、永不重解析），若复活仅翻 status 不重建 FTS 行，该文件**关键词检索永久漏**（无任何报错）。`reviveLostFile` 已收口为单事务 UPDATE + `syncFtsFileRowTx`（按现存 title/note 先删后插幂等），扫描复活与前缀替换收敛两调用方共用；新增非 ok→ok 翻转路径必须复用该函数，禁裸 UPDATE。

### 12.3 素材库备份与还原（service/assets_backup.go）

- **实现基座（research 实证定稿）**：`VACUUM INTO '<路径>'` 单语句产出独立紧凑 db——**禁裸拷贝 db 文件**（WAL 未 checkpoint 数据拷主文件必丢）；**禁包显式 Tx**（事务内执行报错，database/sql 自动提交单条 Exec 即可）；目标已存在报 `output file already exists`（时间戳命名天然唯一 + 同秒 `_N` 深序防冲突）。**`integrity_check` 必须 Query 读结果集**（Exec 不报错但结果集被丢弃，形同没校验）；健康库单行 `ok`，损坏库全部问题聚合单行内嵌换行，首行 != ok 即判失败；**非 ok 删备份文件并报错**（带病备份比无备份更危险）。
- **命名与淘汰**：`data/assets/backups/<libId>/<yyyyMMdd-HHmmss>[_N].db`；深序分隔符取 `_`（0x5F > `.`）——**文件名字典序与时间序严格一致**（`-` 作分隔符时 `_1.db` 排裸名前时间序颠倒，禁用），保留淘汰与清单倒序纯按名排序；每库最近 5 份（`assetBackupRetention` 硬编码），超出淘汰最旧，淘汰失败仅告警下次重试；时钟经 `assetBackupClock` var 注入（CreatedAt 与文件名同源单次读取；测试假时钟驱动，禁 sleep 等真实跨秒）。清单仅收正则形态文件（用户误放文件不进清单与淘汰面），从未备份返回空清单非错误。
- **触发点**：手动 `BackupAssetLibrary` + 重大操作 `backupLibraryForCriticalOp`（手动与自动共用 `createAssetBackup` 唯一实现，禁第二口径）——重建前失败仅 Warn 不中止（重建可重扫恢复，best-effort 不放大故障面）；**删库前失败返回 `E_ASSETS_DELETE_BACKUP_FAILED` 确认型拒绝**（删除未发生，`allowNoBackup=false`），前端弹「无备份删除」error 语义强确认后带 `true` 重调（research §5 零恢复点删除残余风险以升级告知强度缓解，形态对齐 RelocateAssetFile force 重调）。备份目录**不随删库清理**（删库后备份是唯一恢复点）。**同库并发备份互斥（审核修复）**：`backupLocks` 每库 mutex 包 `createAssetBackup` 全程（选名+VACUUM+校验+淘汰），锁序恒最内层（rebuild/DeleteLibrary 持扫描/拆分锁后取备份锁，备份锁内禁反向取外层锁，无成环）；VACUUM 失败清理走 `removeFailedVacuumTarget` **仅删零字节残留**（互斥下非空文件必非本调用产物，跨进程理论窗口可能是并发方有效备份，不删——冗余由还原前完整性预检拒载兜底）。
- **还原步骤序（research §4.1 定稿，乱序即半态）**：库存在预检 → 备份路径防呆（Abs+Rel 越界判定，**必须位于该库备份目录内**，防任意文件覆盖 `<id>.db`）→ 覆盖前预检备份完整性（零半态：损坏备份不动当前库）→ `CancelScan` + `lockScanBlocking` **阻塞等扫描真正退出**（取消在文件间检查点生效，TryLock 会立即拒绝未退出的在途扫描）→ `tryLockSplit`（先扫描锁后拆分锁，与 DeleteLibrary 同序无成环）→ 锁内复验库存在 → `CancelAssetThumbs` → **`CancelExport` 命中在途则 `WaitExportsDone` 等退出**（审核修复：导出「物化行键 + 分批短会话懒重开」，还原 rename 后下一批水合打开新库，旧库物化行键按 fileID 静默水合错误行产出无声混批 CSV；等待不持 s.mu 防与批间短会话死锁）→ **持 `s.mu` 复查 `exportCancels` 注册表（`hasPendingExportLocked`），非空拒还原**（复用 `E_ASSETS_SCAN_IN_PROGRESS` warning 引导重试，`tryLockSplit` 复用先例）——封死「CancelExport 遍历与注册的竞态 + 物化恰在旧库完成」窗口：复查必须与关连接/rename 同锁串行，已注册导出物化被 s.mu 阻塞必落新库自洽，复查放行后新注册导出同理自洽 → 关连接（WAL 伴生文件连接正常关闭才收口）→ 删 `-wal/-shm` 残留（必须在重开前，防旧 WAL 与新 db 内容错位）→ **`os.Rename` 直接覆盖**（Windows MoveFileEx REPLACE_EXISTING 原子替换，禁先 Remove 再 Rename——两步法引入丢文件中间态）→ 惰性重开 + 落库后 integrity_check 复验。还原 move 消费备份文件（原子替换语义）；缩略图 PNG 不清理（键失配进 LRU 淘汰 + 补缺链路重导出）。锁序 s.mu→opMu 单向（opMu 临界区均为注册表/锁表纯 map 操作，从不反向嵌套 s.mu，无 ABBA 环）。

### 12.4 重建后哈希恢复（SRS §14.16 补链）

- **备份条目扩 hash 维度**：`fileTagBindingsByPath`/`slideAnnotationsByPath` 值形态加 `hash`（files.hash，加密/权限跳过的空 hash 不参与兜底）。
- **两级匹配**：先路径直命中（现状语义不变），文件级 path miss（重建后改名/移动）按 hash 兜底定位新文件再按页序映射；**hash 多命中取首个**（files 按 id 升序 = 确定性最小 id）并 slog.Warn（同内容多副本误绑概率低，标注语义损失小）。**多个旧绑定同 hash 落同一新文件按「旧路径字典序后者胜」**（审核修复：slide_meta INSERT OR REPLACE 后写覆盖，解析与写入两遍同序遍历保证跨运行确定；slide_tags OR IGNORE 并集无冲突；同 hash 组级 slog.Warn 含源路径列表）。`rebindFileTagsAfterRebuild`/`rebindSlideAnnotationsAfterRebuild` 返回双计数（pathRebound/hashRebound，文件/页口径，日志可观测）；事务内 rows 收齐显式 Close 再发写语句（单连接池自锁纪律）。
- **重建前整库磁盘备份**挂点在标注内存备份成功后、清库前（失败仅告警不中止）——页级标注重绑只覆盖标签/标注，文件级星级/备注与解析产物随清库重建，磁盘备份是回到重建前时点的唯一手段。

---

**沉淀来源**：任务 10-13-dam-ppt（2026-10-13，素材管理 MVP 三批实施 + 质量检查修复）；任务 10-14-fts-14（2026-10-14，审核修复 14 项：FTS 纯标点词防御跳过与实测结论、多词摘要逐词定位、zip 解压上限、ListAssetSlides 分页、建库孤儿 db 回滚、WaitScansDone 关停收口、`.-01.pptx` 误判回归、前端双代次写回守卫 / 视图切换兜底 / 扫描单刷新 / 聚合在途守卫 / LIB_NOT_FOUND 自愈）；任务 10-14-srs-2-2-4（2026-10-15，幻灯片独立标注体系：v5 两表键 (file_id, slide_num)、继承合并筛选 SlideTagOnlyOwn、重建 path+slide_num 重绑、保存刷新 miss 回填权威源修复）；任务 10-15-srs-2-3（2026-10-15，视图与选中体系 §8：选中键契约/清空点位矩阵/预览层局部 z-index/失败自动补恰一轮，纯前端批零 service 改动）；任务 10-15-srs-2-4（2026-10-16，批量打标签 + 多级标签树 §9：tags.parent_id 启用/级联命中两路径同源/每条目一事务明细/删除级联双保险/TreeSelect 单组件 ID 口径/双刷新链守卫，两轮独立审核零实质缺陷）；同任务 D8 真机冒烟增量（2026-10-16，§9.5：sort_order v6 迁移名称序回填/组尾追加与连续化重排/拖拽换算单点 resolveTagMoveDrop/flatten 默认平铺）；任务 10-16-facet-srs-2-5（2026-10-16，检索面 §2.5 第七批：PR2 facet 三维双语境接入与清空点位矩阵登记 + PR3 保存检索条件 UI §8.6——快照全维/区间反推可逆锚点止日末毫秒/跨库按名重映射降级/覆盖仅本库边界）；任务 10-16-srs-2-5-3-srs-2-6-p0（2026-10-16，第八批 §10：三路径排序单点 sortEntriesByOptions/次级键含 sortNum 防 refID 隐式依赖/并发删除窗口 props 缺键剔除/快照补排序字段零值兼容/批量评分单方法每条目一事务整行回写备注/collectSelectionTargets 与刷新链共享化）；任务 10-16-srs-2-6-4-srs-2-6-5（2026-10-17，第九批 §11：导出三段式锁解耦/文件行独立 UNION ALL 路/批间禁 OFFSET 续读/O_EXCL 覆盖收口/真中止 exportCancels 走 opMu/弹窗放弃语义含成功竞态收口；lost 状态机标记保留 hash 基准/复活资格门槛 fileHasSlides/processing 永不直复活/清理事务内资格复验 RowsAffected；重链哈希校验 force/嵌套腾位长度降序 + 探测配对禁下标 zip（P0 串写教训）/写面 RowsAffected 零命中上抛/前端 toast 先判 reparsed）；任务 10-18-srs-2-7-3（2026-10-18，第十批 §12：排除规则单段 glob 大小写不敏感 root 豁免/收集层与存量判定共用 matcher/坏规则保存拒绝 + 编译期剔除双防线；excluded 先排除后存在性/markFileExcluded 仅翻状态/复活共用 fileHasSlides 门槛/规则优先不转 lost/关键词路径 excludedFileIDSet 批量剔除而标签与 facets-only 恒 status=ok 天然排除/导出照导；备份 VACUUM INTO 禁 Tx 与裸拷贝/integrity_check Query 非 Exec/_N 深序分隔符字典序=时间序/还原步骤序含阻塞等扫描退出与删 wal-shm 后 Rename 覆盖/删库前失败 E_ASSETS_DELETE_BACKUP_FAILED 升级确认 allowNoBackup 重调/备份目录不随删库清理；重建备份条目扩 hash 维度两级匹配多命中取首个告警）。
