# Research: 本地 DAM 缩略图磁盘缓存管理实践（LRU 淘汰 + 大小上限 + 多分辨率）

- **Query**: 缩略图磁盘缓存的 LRU 记账方式、淘汰触发时机、上限默认值、命名布局、访问时间更新策略、失效联动；对标 Billfish/XnView/ACDSee/Eagle/digiKam/freedesktop 规范
- **Scope**: mixed（项目内部代码事实 + 官方文档核实 + 本机实测）
- **Date**: 2026-10-14

## 背景与项目约束（映射结论时须一并考虑）

| 约束 | 现状（来源） |
|---|---|
| 每库独立 SQLite | `data/assets/<library-id>.db`，modernc 纯 Go（`service/assets_db.go:35-54`） |
| 单写连接池 | `SetMaxOpenConns(1)` + WAL + busy_timeout（`assets_db.go:40-47`）；**rows 未 Close 就发第二条查询 = 永久死锁**（docs/spec/assets-management.md §3） |
| 迁移机制 | `PRAGMA user_version` 递增 + Go 层回填，现 v2（`service/assets_schema.go:21`），本批可升 v3 加表 |
| 缩略图落盘 | `data/assets/<lib-id>/thumbs/`（PRD 约定，本批新建） |
| 前端画廊 | 懒加载分页 50/页，缩略图按需 `ReadFileBytes` base64 回传 |
| 关键代码事实 | `files.id` 在重解析中**稳定不变**（仅 UPDATE 同一行，`assets_db.go:248-294`）；`slides.id` 为 `AUTOINCREMENT` + `replaceFileSlides` 删旧插新（`assets_schema.go:51`），**每次重解析全部页 id 都变**且 SQLite AUTOINCREMENT 保证不复用已删 id |

---

## 1. LRU 记账方式对比

**结论前置：选 (a) SQLite 表记账，放素材库 db 内新增 `thumbnails` 表（v3 迁移），访问时间走「内存 LRU + 低频批量落盘」；(b) 目录扫描仅作兜底对账，(c) 文件 atime 不可作为 LRU 依据。**

### 1.1 三方案对比

| 维度 | (a) SQLite 表记账 | (b) 目录文件 mtime 扫描 | (c) 文件 atime |
|---|---|---|---|
| 语义准确性 | 精确：应用自己定义「访问」，想记什么记什么 | **失真**：mtime 是「内容修改时间」不是「访问时间」；缩略图文件从不被修改，mtime ≈ 生成时间，用 mtime 做 LRU 等于 FIFO | **失真且不可控**：任何读进程（杀毒、备份、搜索索引器）都会刷新 atime；语义被全系统共享 |
| 淘汰成本 | O(log n)（按 atime 索引取最旧一批），size 合计一条 `SUM` | 每次全量 walk 缩略图目录（20 万文件的 `WalkDir` 秒级），size 需逐文件 Stat | 同左（全量 walk），且 Windows 大目录枚举慢 |
| 写放大 | 高频 UPDATE 有代价（见 §5 缓解） | 无记账写 | 无记账写（atime 由系统写，但见下方不可靠性） |
| 与 db 的联动 | 天然：删文件记录与缩略图记录同事务（对齐现有 `setFileOutcome`/`deleteFilesByPaths` 的 FTS 同步模式，`assets_db.go:214-338`） | 联动靠字符串路径匹配，易漏易孤儿 | 同左 |
| 崩溃恢复 | 记账与文件可能短暂不一致，可用「对账式自愈」修复（扫目录补记/删孤儿） | 无记账可失 | 无记账可失 |
| 实现成本 | 中（一张表 + 淘汰函数） | 低但每次淘汰贵 | 最低但不可用 |

### 1.2 atime 不可靠的证据链（一手核实）

- 微软官方文档（learn.microsoft.com `fsutil behavior`）：LastAccess **查询值准确但落盘延迟最多 1 小时**（NTFS 内存中即时更新，lazy write 异步落盘）——「One hour is the maximum amount of time that NTFS can defer updating Last Access Time on disk」；掉电即丢未落盘部分。
- 本机实测（Windows 11，2026-10-14）：`fsutil behavior query disablelastaccess` 返回 **2（System Managed, Last Access Time Updates ENABLED）**。System Managed 策略由系统按卷决定（公开资料：Windows 10 1803 起默认值，主要保证系统卷启用，非系统卷策略随版本而异），**应用无法控制目标机器上数据卷的行为**。实测在 D: 卷创建→读取文件，`LastAccessTimeUtc` 确实变化（内存层），但这不能推断其他机器/卷的行为。
- 结论：atime 的「是否更新、何时落盘」由操作系统策略和第三方读流量共同决定，作为 LRU 的持久化记账依据跨机器不可复现；(c) 排除。

### 1.3 记账放素材库 db 内 vs 独立缓存索引库

**结论：放素材库 `<lib-id>.db` 内新增表，不建独立缓存索引 db。**

- 素材库 db 与缩略图目录一一对应（库删除 = 删 db + 删 thumbs 目录），独立索引 db 多一个要同步生命周期的文件，违背本项目「删库即删文件」的既有布局（docs/spec/assets-management.md §2）。
- 放库内可直接 `REFERENCES slides(id)`/携带 `file_id` 列，删除联动走事务（复用 FTS 双写同款收口模式）；独立 db 做不到跨库事务。
- digiKam 是反例参照：它把缩略图放独立 `thumbnails-digikam.db`，是为了与核心照片库分离、可整库丢弃再生（官方文档 database_settings：「Thumbnails…can always be regenerated from scratch」）。本项目缩略图同样可再生，但规模小得多（单库万级页），分库收益不抵同步成本；若未来单库缩略图规模到百万级再考虑拆分。

---

## 2. 淘汰触发时机

**结论前置：不做「写入时同步淘汰」；用后台异步（写入后触发一次惰性检查 + size 阈值）+ 启动时惰性。**

| 时机 | 优劣 | 业界做法 |
|---|---|---|
| 写入时同步检查超限 | 实现直观；但淘汰（删文件 + 批量 DELETE）会拉长写路径，且本项目单写连接下淘汰事务会与扫描写入互相阻塞 | **无主流图片管理器这样做**；浏览器磁盘缓存（Chrome）是后台驱逐线程 |
| 后台定期 | 与前台解耦；缺点是超限窗口期内可超不少 | digiKam：无自动上限，靠**手动**维护工具（Maintenance > Database Cleaning：清孤儿条目 + VACUUM 收缩，官方文档 maintenance_database.html） |
| 启动时 | 启动变慢；20 万文件对账不该放在启动关键路径 | freedesktop 规范：缩略图删除是事件驱动（原文件消失即删；网络资源 30 天未访问删），非启动扫描 |

- 本项目落点：缩略图生成完成 / 淘汰检查在**后台 goroutine** 做（复用 assets-scan 的后台模式 + cancels 注册表取消），启动时不主动对账；「对账式自愈」只在用户显式触发重建/清理时跑。
- digiKam 官方文档未提供「自动按上限淘汰」（本次核实范围内），其官方态度是「警告用户大库 + 提供手动清理 + VACUUM」——说明自动 LRU 上限淘汰在 DAM 领域是加分项而非标配，实现从简即可领先对标。

---

## 3. 缓存上限默认值与磁盘占用估算

**结论前置：无行业标准统一值，主流产品均为「自管理或用户可配」；本项目建议默认 2GB（小图档与高清档独立配额），必须可配。**

### 3.1 对标产品的可核实信息

| 产品 | 缓存形态 | 上限/分辨率（已核实部分） |
|---|---|---|
| digiKam（开源 DAM，形态最接近本项目） | 缩略图存独立 SQLite（`thumbnails-digikam.db`），PGF 小波压缩 | 官方**无自动大小上限**；缩略图尺寸默认上限 256×256，4K 屏可开 512×512（开启后 db 显著变大，官方文档 maintenance_thumbnails.html 明示） |
| freedesktop 缩略图规范（Linux 桌面标准，gThumb/Nautilus 等遵循） | `$XDG_CACHE_HOME/thumbnails/{normal,large,x-large,xx-large}` | 四档固定尺寸 **128/256/512/1024px**；无全局上限，过期策略为「原文件消失即删、网络资源默认 30 天未访问删」（规范 delete.html 原文） |
| Eagle / Billfish / XnView MP / ACDSee | 社区通识：各自有缩略图缓存目录/数据库，提供清理入口 | **本次研究环境未能核实官方默认上限数值**（搜索引擎反爬），不引用未经验证的数字 |

- 参考：Windows 资源管理器 thumbcache（`%LocalAppData%\Microsoft\Windows\Explorer\thumbcache_*.db`，社区逆向工程知识，非官方文档）由系统自管，容量级通常在数 GB 内——可作为「桌面软件缓存默认几个 GB 内」的量级佐证。

### 3.2 万级素材占用估算（估算值，非实测）

设 1 万个 PPT × 平均 20 页 = **20 万张缩略图**，幻灯片 16:9：

| 档位 | 单张大小（估算） | 20 万张全量 |
|---|---|---|
| 小图 320px（320×180）PNG——PPT 内容以文字/扁平色块为主，PNG 压缩友好 | 30–150KB，取 50KB | **≈ 10 GB** |
| 高清 1600px（1600×900）PNG——含照片背景时可到数 MB | 200KB–1.5MB，取 500KB | **≈ 100 GB** |
| 高清 1600px JPEG q85（若 COM Export 支持 JPG 输出） | 120–250KB | ≈ 40 GB |
| 文件系统元数据（20 万个文件的 MFT + 目录索引，NTFS 每记录 ~1KB） | — | 数百 MB，且**单目录 20 万条目**会显著拖慢枚举/删除，须分桶子目录 |

- 推论 1：2GB 上限下，小图 PNG 档约容 4 万张、高清 JPEG 档约容 1 万张——万级素材全量缓存不现实，LRU 是必需而非可选。
- 推论 2：全量缓存 20 万张至少 10GB（全小图 PNG）起步；若产品预期「热区全命中」，默认上限 2–4GB + LRU 是务实平衡。
- 推论 3：格式是最大的容量杠杆（PNG→JPEG 约 2–5 倍差距），分辨率是第二杠杆（线性于边长平方）；建议两档独立配额、独立可配。

---

## 4. 缓存文件命名布局

**结论前置：`thumbs/<档位>/<file-id>-<3位补零页码>.png`（如 `small/102-007.png`），双档子目录 `small/`、`hd/`；禁用 slide-id 命名（重解析后全变）；hash 命名不取。**

### 4.1 三种命名对比

| 方案 | 优点 | 缺点 | 判定 |
|---|---|---|---|
| `<slide-id>.png` | 与 db 行一一对应 | **slides.id 每次重解析全部变化**（AUTOINCREMENT 删旧插新，`assets_db.go:270-292`），每轮重扫全部缩略图变孤儿，须靠记账表反查清理 | 否决 |
| `<file-id>-<page>.png` | file-id 重解析稳定（仅 UPDATE 同一行）；按文件清理 = 前缀删除 O(页数)；无记账也能定位 | 字符串前缀须防误伤（`102-` vs `1023-`，见 §4.3）；页码不补零则字典序乱 | **推荐** |
| hash 命名（freedesktop 式 `md5(URI).png`，或内容 hash） | 与位置解耦、可共享仓库；源变化 hash 变、天然无旧图复用 | 无法从文件名反推归属，按文件批量清理必须依赖记账索引；内容 hash 计算贵；本项目缓存是库内私有，共享收益不存在 | 否决（规范场景不同：freedesktop 设计目标是跨应用共享缓存，本项目不是） |

- freedesktop 规范的失效模型（`Thumb::MTime` PNG 内嵌属性 + 读取时校验，creation.html：「The mtime is needed to check if the thumbnail is valid yet」）说明「缩略图必须携带源版本判定信息」是通行需求——本项目由 db 的 mtime_ns 增量判定承担（缩略图生成的触发时机与扫描增量判定同源，无需内嵌）。
- 页码补零到 3 位（PPT 上限由解析侧决定，三位够用）：字典序 = 数值序，分页遍历/对账排序稳定。
- 双档**子目录**（`small/`、`hd/`）优于文件名后缀：分档配额独立统计与独立淘汰时，子目录是天然边界；对账 walk 只走目标档；删文件时对两个子目录各做一次前缀删除即可。

### 4.2 重建库 / 删除文件的清理联动

- **删除文件**：`deleteFilesByPaths`（`assets_db.go:298`）事务内追加该 file_id 的缩略图记账 DELETE；缩略图文件删除放事务提交后（文件系统操作不可回滚，db 先行、文件后删，对齐「建库回滚先关连接再删文件」的既有收口顺序，assets-management.md §3）。
- **重建库**：现有 `clearIndexForRebuild`（`assets_db.go:825`）清 files+FTS；缩略图应**整目录删除重建**（`thumbs/` 一次删光）而非逐条对账——重建语义下逐条保留没有意义（页 id 全变）。
- **库删除**：`DeleteAssetLibrary` 删 db（含 WAL/SHM）时同步删 `thumbs/` 目录。

### 4.3 前缀删除的精确性

按 file-id 前缀清理时禁用裸 `strings.HasPrefix(name, fmt.Sprint(fileID))`（`102` 误伤 `1023-…`）；用 `fileID + "-"` 完整前缀，或干脆以记账表为准（`DELETE FROM thumbnails WHERE file_id = ?` 后按记录删文件）——记账在，前缀匹配只是性能优化。

---

## 5. 访问时间更新策略（写放大治理）

**结论前置：前端浏览不逐次 UPDATE 数据库；进程内内存 LRU 记账 + 定时/阈值/关停批量落盘；懒加载画廊按「页」批量记一次。**

- 写放大量化：单连接池（`SetMaxOpenConns(1)`）下每条 UPDATE 是独立 WAL 事务（fsync 语义由 WAL + busy_timeout 兜底）；画廊一屏滚动可触发 50 张缩略图请求，若每张一条 UPDATE，检索查询（`searchFts`/`listSlideEntries`）要和它们轮流抢唯一连接，检索延迟直接受抖动。这正是「倾向低频写」约束的量化来源。
- 内存层：`map[key]entry` + 淘汰堆（或 `container/list` 双向链表），key = `<fileID>-<page>-<档位>`；读命中时仅改内存，零磁盘写。
- 落盘触发：攒批 ≥N（如 200）或间隔 ≥T（如 30–60s）或 App shutdown 时，**单事务** `UPDATE thumbnails SET atime_ns = CASE id ... END` 批量写（或逐行包在一个事务里）；崩溃丢失的只是「最后一次落盘后的访问时间偏移」，对 LRU 正确性无影响（只是新鲜度略旧）。
- 懒加载批量：一页 50 张的画廊请求在 service 层汇总为一次内存批量记账（同时一次批量读文件回传 base64，天然一页一事务的粒度）。
- 详情页打开高清大图属低频单点操作，即时落一条也可接受，但统一走内存+批量最简单。

---

## 6. 失效联动（源 PPT 变更 → 该文件全部页缩略图失效）

**结论前置：file-id 命名下，「文件重解析完成」即触发该文件缩略图整组失效——按 file_id 删旧图（或标记失效）+ 入队重新生成，O(页数)；slides.id 不可作为失效键。**

- 代码事实链：扫描增量判定为 mtime+size 失配 → `markProcessing` → `replaceFileSlides` 删旧插新（`assets_db.go:248`）。此时 files.id 不变、slides.id 全变、页数可能变。
- 联动设计要点：
  1. 失效粒度 = 文件：重解析成功后，删除 `small/<fileID>-*.png` 与 `hd/<fileID>-*.png`（精确 `fileID + "-"` 前缀或记账表反查），再把该文件入缩略图生成队列。
  2. 页数缩减（20 页 → 15 页）会遗留 5 张旧图：因页码是「补零数字 + 连字符」形态，无法从文件名单独判定越界页——**以记账表为准确删**（`DELETE WHERE file_id = ? AND page > 新页数`），或在生成完成回写时顺带对账删除多余页。这是「文件名不可作为唯一事实源、记账表必须存在」的第二个理由（第一个是 LRU）。
  3. 失败文件（COM 超时/无 PowerPoint）：缩略图生成失败不改 files 状态（文本索引仍有效），但须在记账表记「无缩略图」态，避免每次浏览都重复尝试生成——对标 freedesktop 规范的 **Fail 目录**设计（记录生成失败的文件，避免反复重试，directory.html）。
  4. 「文件被占用」跳过（SRS §7）：同上，记账态 + 支持重试入口。

---

## 对本项目的落地建议

1. **记账方式**：素材库 db 内新增 `thumbnails` 表（v3 迁移：`id/file_id/page/kind/atime_ns/size_bytes/status`，`idx_thumbnails_atime`、`idx_thumbnails_file` 两个索引），LRU 记账与失效记账同表；缩略图文件落 `data/assets/<lib-id>/thumbs/{small,hd}/`。
2. **禁用 slide_id 命名**：命名锚定 `files.id`（重解析稳定）+ 页码；`slides.id` AUTOINCREMENT 每轮重解析全变，作为缓存键必然每轮全量孤儿化。
3. **上限默认值**：小图档默认 1GB、高清档默认 1GB（合计 2GB），均可配（沿用 settings 服务模式）；参考 digiKam「无自动上限 + 手动清理」的现状，自动 LRU 是本项目超越对标的点，不必追求复杂——超过即从最旧 atime 起删到 ≤90% 水位（留余量防抖动）。
4. **淘汰触发**：后台 goroutine 惰性触发（缩略图写入完成后投递一个去抖信号）+ 用户显式「清理缓存」入口；启动路径不做全量对账；对账自愈仅在显式重建/清理时执行。
5. **访问时间低频写**：内存 LRU + 攒批（200 条或 30–60s 或关停时）单事务落盘；画廊一页（50 张）一次批量记账；严禁前端每次浏览逐条 UPDATE。
6. **失效联动**：重解析成功（`replaceFileSlides` 提交后）→ 按 file_id 记账删旧图 + 删文件 + 入队重生成；页数缩减靠记账表对账删除越界页（文件名无法判定越界）；前缀匹配必须用 `fileID + "-"` 完整形态防 `102`/`1023` 误伤。
7. **格式建议**：小图档 PNG（文字/色块清晰、体积可控）；高清档可评估 JPEG（COM Export 的 JPG 输出，容量杠杆 2–5 倍）——具体以 COM 研究（research/com-thumbnail-export.md）确认的导出能力为准。
8. **单目录规模**：thumbs 下按档位分目录后仍可能单档数万文件；如预期单库超 5 万页，增加一层 hash 分桶（`small/ab/<file-id>-007.png`，取 file-id 十六进制低两位）控制单目录条目数；万级页以下不必要，避免过度设计。
9. **删除/重建联动**：文件删除走事务删记账 + 事务外删文件；重建库 = 清表 + 整删 thumbs 目录；删库 = 删 db + 删 thumbs 目录（三个生命周期入口全部收口，防孤儿目录）。
10. **失败态记账**：COM 失败/文件被占用/无 PowerPoint 的页在记账表记失败态（对标 freedesktop Fail 目录），避免重复生成风暴；「文件被占用」支持用户手动重试清失败态。

## Caveats / Not Found

- **Eagle / Billfish / XnView MP / ACDSee 的默认缓存上限数值与分辨率设置项未能核实**：本次研究环境中通用搜索引擎被反爬拦截（Bing 返回无关结果、DuckDuckGo/Mojeek/SearX 不可达），仅 Eagle 官网首页可达但无缓存文档。上文对标表中这四款只给出社区通识级的形态描述，未引用任何未经一手核实的数字；如需精确对标值，建议人工查阅各软件偏好设置或官方帮助中心后补充。
- digiKam「无自动按大小上限淘汰」基于其官方手册（database_settings / maintenance_thumbnails / maintenance_database 三页）核实，未遍历其源码确认是否存在隐藏的自动清理；结论表述为「官方文档未提供」而非「不存在」。
- 磁盘占用估算为经验区间推算（PPT 截图 PNG 体积），未实际导出样本验证；实施前建议用真实 PPT 导出 20 张样本测均值校准。
- Windows 资源管理器 thumbcache 的内部结构为社区逆向工程知识（无微软官方文档），仅作量级佐证。

## 参考来源

| 来源 | 性质 | 要点 |
|---|---|---|
| learn.microsoft.com `fsutil behavior`（Windows Server 官方文档） | 一手 | atime 查询准、落盘延迟 ≤1h、`NtfsDisableLastAccessUpdate` 注册表项 |
| 本机实测（Windows 11，`fsutil` + PowerShell atime 读写实验） | 一手 | 本机 DisableLastAccess=2（System Managed, ENABLED）；D: 卷读取后 atime 变化（内存层） |
| docs.digikam.org database_settings / maintenance_thumbnails / maintenance_database（官方手册 9.2.0） | 一手 | 缩略图独立 SQLite + PGF 压缩；256/512px 尺寸上限；无自动容量上限；手动清理 + VACUUM；缩略图库可再生 |
| specifications.freedesktop.org Thumbnail Spec（latest） | 一手 | 四档尺寸 128/256/512/1024；`md5(URI).png` 命名；`Thumb::MTime` 失效校验；Fail 目录；原文件消失即删 / 网络资源 30 天未访问删 |
| `service/assets_db.go` / `service/assets_schema.go`（本仓库） | 一手 | files.id 稳定 / slides.id AUTOINCREMENT 不稳定；单连接池自锁约束；FTS 同步删除的事务收口模式 |
| docs/spec/assets-management.md（本仓库） | 一手 | 存储布局、迁移机制、扫描语义、单连接池 Gotcha |
