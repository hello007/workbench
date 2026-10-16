// model/assets.go
package model

// 素材管理（侧边栏「素材管理」，本地 DAM MVP）跨层模型。
//
// 索引库以 SQLite 文件落盘（data/assets/<library-id>.db），本文件只定义经 Wails
// 绑定传前端的导出 struct 与取值常量。字段均为普通类型/普通 string，未引入具名
// string 类型（具名 string 作绑定方法参数/返回值时 wails 不为 models.ts 生成
// 类型别名，须手动补，见 docs/spec/cross-layer-contracts.md）。

// AssetLibrary 素材库注册信息（data/assets/libraries.json 注册表 + 前端库切换下拉）。
type AssetLibrary struct {
	ID        string `json:"id"`        // 库唯一标识（随机 hex），同时是库 db 文件名（<id>.db）
	Name      string `json:"name"`      // 库显示名（用户输入，可重名）
	RootPath  string `json:"rootPath"`  // 素材根目录绝对路径（扫描范围，原始文件不动）
	CreatedAt string `json:"createdAt"` // 创建时间（"2006-01-02 15:04:05"）
	UpdatedAt string `json:"updatedAt"` // 最近更新时间（同上格式）
}

// AssetTag 标签（平铺单级；ParentID 预留多级树，本批不建 UI）。
type AssetTag struct {
	ID       int64  `json:"id"`       // 标签自增主键
	Name     string `json:"name"`     // 标签名（库内唯一）
	Color    string `json:"color"`    // 展示色（#RRGGBB；自动创建时从默认色板分配）
	ParentID int64  `json:"parentId"` // 父标签 ID，0 = 无父级（预留字段，恒 0）
}

// 素材文件索引状态机取值（files.status 列与前端状态标记共用）。
// 单文件解析结果分类：成功 / 失败(损坏等) / 跳过(加密) / 跳过(权限不足)；
// processing 为扫描过程中的瞬态（崩溃残留时下次扫描会重新处理）。
const (
	AssetFileStatusProcessing = "processing" // 解析中（瞬态）
	AssetFileStatusOK         = "ok"         // 解析成功
	AssetFileStatusFailed     = "failed"     // 解析失败（损坏 / 非演示文稿）
	AssetFileStatusEncrypted  = "encrypted"  // 加密文件，跳过索引
	AssetFileStatusDenied     = "denied"     // 权限不足，跳过索引
)

// AssetFileEntry 素材文件索引条目（画廊文件视图卡片 + 详情侧边栏元数据编辑载体）。
type AssetFileEntry struct {
	ID        int64      `json:"id"`        // 文件记录自增主键
	Path      string     `json:"path"`      // 相对库根目录路径（正斜杠分隔，库内唯一）
	Size      int64      `json:"size"`      // 文件字节数
	ModTime   string     `json:"modTime"`   // 最近修改时间（展示格式）
	Status    string     `json:"status"`    // 索引状态（AssetFileStatus* 取值）
	Title     string     `json:"title"`     // 文件展示标题（源文件名去扩展名）
	Note      string     `json:"note"`      // 用户备注
	Rating    int        `json:"rating"`    // 用户星级 0-5（0 = 未评分）
	Tags      []AssetTag `json:"tags"`      // 文件标签（幻灯片视图经归属文件继承）
	CreatedAt string     `json:"createdAt"` // 入库时间
	UpdatedAt string     `json:"updatedAt"` // 最近索引时间
}

// AssetSlideEntry 幻灯片单页索引条目（画廊幻灯片视图卡片 + 检索命中单元）。
type AssetSlideEntry struct {
	ID           int64      `json:"id"`           // 幻灯片记录自增主键
	FileID       int64      `json:"fileId"`       // 归属文件记录 ID（溯源主键）
	FilePath     string     `json:"filePath"`     // 归属文件相对路径（溯源展示）
	SlideNum     int        `json:"slideNum"`     // 展示页序（从 1 计，以 sldIdLst 为准）
	Title        string     `json:"title"`        // 页标题（回退链结果，空页兜底「[无标题]」）
	Text         string     `json:"text"`         // 页文本框正文
	SpeakerNotes string     `json:"speakerNotes"` // 演讲者备注
	Tags         []AssetTag `json:"tags"`         // 标签：继承归属文件标签（继承语义不变，详情面板只读弱化展示）
	OwnTags      []AssetTag `json:"ownTags"`      // 页自有标签（slide_tags 独立存储，与文件标签互不写入）
	Note         string     `json:"note"`         // 页业务备注（用户标注，区别于解析自带演讲者备注；不入 FTS）
	Rating       int        `json:"rating"`       // 页星级 0-5（0 = 未评分）
}

// AssetSlidePage 幻灯片索引分页结果（ListAssetSlides 返回）。分页契约与
// AssetSearchPage 一致：Total 为总条目数（不随页变化），翻页以 Offset 递增
// Limit 驱动。
type AssetSlidePage struct {
	Items []AssetSlideEntry `json:"items"` // 当前页条目（按文件路径 + 页序）
	Total int               `json:"total"` // 幻灯片总条目数
}

// AssetLibraryStats 素材库统计（GetAssetLibraryStats 返回，库切换/看板徽标用）。
type AssetLibraryStats struct {
	LibraryID      string `json:"libraryId"`      // 所属库 ID
	FileCount      int    `json:"fileCount"`      // 文件索引总数（含各状态）
	SlideCount     int    `json:"slideCount"`     // 幻灯片索引总数
	FailedCount    int    `json:"failedCount"`    // 解析失败文件数（status=failed）
	EncryptedCount int    `json:"encryptedCount"` // 加密跳过文件数（status=encrypted）
}

// AssetScanProgress 扫描进度事件载荷（assets-scan:progress 事件，EventSink 下发）。
type AssetScanProgress struct {
	LibraryID   string `json:"libraryId"`   // 正在扫描的库 ID
	Done        int    `json:"done"`        // 已处理文件数（含未变跳过，从 1 计）
	Total       int    `json:"total"`       // 本次扫描发现的 pptx 总数
	CurrentFile string `json:"currentFile"` // 当前处理文件的相对路径
}

// AssetScanResult 单次扫描结果汇总（assets-scan:done 事件载荷 + 日志口径）。
type AssetScanResult struct {
	Total     int `json:"total"`     // 发现的 pptx 总数
	New       int `json:"new"`       // 新入库文件数
	Updated   int `json:"updated"`   // 变更重解析文件数
	Unchanged int `json:"unchanged"` // mtime+size 未变跳过数
	Failed    int `json:"failed"`    // 解析失败数
	Encrypted int `json:"encrypted"` // 加密跳过数
	Denied    int `json:"denied"`    // 权限不足跳过数
	Removed   int `json:"removed"`   // 清理的已删文件记录数
}

// AssetThumbsProgress 缩略图批量生成进度事件载荷（assets-thumbs:progress 事件，
// EventSink 下发）。计数口径按文件（done/total 为文件数，非页数——单页耗时毫秒级
// 无感知价值）；Failed 含超时/损坏/占用等生成失败文件。
type AssetThumbsProgress struct {
	LibraryID   string `json:"libraryId"`   // 正在生成缩略图的库 ID
	Done        int    `json:"done"`        // 已处理文件数（含失败，从 1 计）
	Total       int    `json:"total"`       // 本次批次文件总数
	CurrentFile string `json:"currentFile"` // 当前处理文件的相对路径
	Failed      int    `json:"failed"`      // 已失败文件数（前端进度提示用）
}

// AssetThumbsResult 单批缩略图生成结果汇总（assets-thumbs:done 事件载荷）。
// Generated 为成功生成全部页双档缩略图的文件数；Skipped 为缓存命中/源未变跳过数。
type AssetThumbsResult struct {
	LibraryID string `json:"libraryId"` // 所属库 ID
	Total     int    `json:"total"`     // 批次文件总数
	Generated int    `json:"generated"` // 生成成功文件数
	Skipped   int    `json:"skipped"`   // 跳过文件数（缓存命中）
	Failed    int    `json:"failed"`    // 生成失败文件数
	Canceled  bool   `json:"canceled"`  // 是否被用户取消中断
}

// AssetFileMetaUpdate 文件级元数据编辑入参（UpdateAssetFileMeta 整体替换语义：
// 星级/备注/标签以本次提交值为准，未提及的标签即移除）。
type AssetFileMetaUpdate struct {
	Rating int      `json:"rating"` // 星级 0-5（0 = 清除评分）
	Note   string   `json:"note"`   // 备注（整体替换）
	Tags   []string `json:"tags"`   // 标签名列表（整体替换；不存在的标签自动创建）
}

// AssetSlideMetaUpdate 幻灯片页级标注编辑入参（UpdateAssetSlideMeta 整体替换
// 语义，对齐 AssetFileMetaUpdate）：业务备注/星级/自有标签以本次提交值为准；
// 不触碰解析链路与缩略图缓存（纯标注层）。标签写入 slide_tags（页自有），
// 与文件标签（file_tags）存储完全独立。
type AssetSlideMetaUpdate struct {
	Note     string   `json:"note"`     // 业务备注（整体替换）
	Rating   int      `json:"rating"`   // 星级 0-5（0 = 清除评分）
	TagNames []string `json:"tagNames"` // 页自有标签名列表（整体替换；不存在的标签自动创建）
}

// AssetBatchTagOp 批量打标签操作入参（BatchUpdateAssetFileTags /
// BatchUpdateAssetSlideTags 共用）：标签名列表（非 ID——不存在的添加名自动创建
// 为根标签）。语义 = 每条目「现标签集 − RemoveTags ∪ AddTags」，相对条目现状
// 追加/移除，非整体替换；两数组交集须为空（参数校验拒绝，前后端双防线）。
type AssetBatchTagOp struct {
	AddTags    []string `json:"addTags"`    // 追加标签名列表（可空）
	RemoveTags []string `json:"removeTags"` // 移除标签名列表（可空）
}

// AssetSlideTarget 批量页标签操作的目标页定位。键契约 = (fileId, slideNum)，
// 禁 slides.id（每轮重解析删旧插新全变，见 docs/spec/assets-management.md
// 幻灯片独立标注节，与 slide_tags 存储键同口径）。
type AssetSlideTarget struct {
	FileID   int64 `json:"fileId"`   // 归属文件记录 ID
	SlideNum int   `json:"slideNum"` // 展示页序（从 1 计，以 sldIdLst 为准）
}

// 素材批量打标签单条目结果状态取值（AssetBatchTagItem.Status，普通 string 常量
// 非具名类型，不经 Wails 绑定层别名机制）。
const (
	AssetBatchTagStatusOK     = "ok"     // 写入成功
	AssetBatchTagStatusFailed = "failed" // 写入失败（Message 带原因，批量不中断）
)

// AssetBatchTagItem 批量打标签单条目结果明细（异常隔离，对齐
// AssetSplitFileOutcome 先例）。
type AssetBatchTagItem struct {
	FileID   int64  `json:"fileId"`             // 条目归属文件记录 ID
	SlideNum int    `json:"slideNum,omitempty"` // 页序（页标签条目从 1 计；文件标签条目恒 0 省略）
	Status   string `json:"status"`             // AssetBatchTagStatus* 取值
	Message  string `json:"message,omitempty"`  // 失败原因（成功为空）
}

// AssetBatchTagResult 批量打标签结果汇总。事务边界 = 每条目一事务逐条目提交，
// 部分失败不回滚已成功条目（成功条目已写入）。
type AssetBatchTagResult struct {
	Total     int                 `json:"total"`     // 提交条目总数
	Succeeded int                 `json:"succeeded"` // 成功条目数
	Failed    int                 `json:"failed"`    // 失败条目数
	Items     []AssetBatchTagItem `json:"items"`     // 逐条目明细（顺序同提交序，恒非 nil）
}

// AssetTagListItem 标签清单条目（ListAssetTags 返回，标签树构建数据源）：标签
// 本体五字段 + 直接绑定计数。Count = file_tags 绑定行数 + slide_tags 绑定行数
// 之和（父节点子树合计由前端按树结构自算，后端不聚合）；含未使用标签（Count=0）。
// SortOrder 为同父兄弟内的展示序号（PRD D8#1 拖拽排序），平铺清单按「父级分组 +
// 组内 SortOrder 升序、同序按 id」返回，前端树构建保序消费即得兄弟序。
type AssetTagListItem struct {
	ID        int64  `json:"id"`        // 标签 ID
	Name      string `json:"name"`      // 标签名（库内唯一）
	Color     string `json:"color"`     // 展示色（#RRGGBB）
	ParentID  int64  `json:"parentId"`  // 父标签 ID，0 = 根标签
	Count     int    `json:"count"`     // 直接绑定计数（文件绑定 + 页自有绑定行数之和）
	SortOrder int    `json:"sortOrder"` // 同父兄弟内展示序号（拖拽排序契约，PRD D8#1）
}

// 素材检索条目类型标记（AssetSearchHit.RefType 取值，普通 string 常量非具名
// 类型，不经 Wails 绑定层别名机制）。
const (
	AssetRefTypeFile  = "file"  // 文件条目（整份 pptx 命中，如文件名/备注命中）
	AssetRefTypeSlide = "slide" // 幻灯片条目（单页命中）
)

// AssetSearchOptions 素材检索入参（SearchAssets）。关键词与标签筛选可组合；
// 关键词为空且给定标签时退化为纯标签浏览；关键词与标签均为空但有 facet 条件
// 时走 facets-only 文件清单路径；全空返回空结果页。
type AssetSearchOptions struct {
	Flatten bool    `json:"flatten"` // true = 平铺返回全部命中；false = 同 PPT 多页命中折叠为聚合条目
	TagIDs  []int64 `json:"tagIds"`  // 标签筛选：文件条目须直接绑定其中之一，幻灯片条目默认【归属文件绑定 ∪ 页自身绑定】命中；空 = 不筛选
	// SlideTagOnlyOwn 标签筛选继承开关（仅作用于标签筛选，关键词命中不受影响；
	// 文件条目不受影响）：false（默认，向后兼容）= 幻灯片条目【页自身标签 ∪ 归属
	// 文件标签】合并命中；true = 幻灯片条目仅页自身标签命中。
	SlideTagOnlyOwn bool `json:"slideTagOnlyOwn"`
	// Ratings 星级档筛选（facet，文件级口径，SRS §2.5 D1）：nil = 不筛选；
	// 含 0 表示「未评分」（files.rating=0），非零值按等值匹配 files.rating。
	// 幻灯片条目不按页级评分（slide_meta.rating）参与筛选，而是继承归属文件
	// 的星级判定（与修改时间/文件大小 facet 同为文件维度，三 facet 口径一致）。
	Ratings []int64 `json:"ratings"`
	// MtimeFrom/MtimeTo 修改时间区间筛选（facet，文件维度）：纳秒 Unix 时间戳
	// （与 files.mtime_ns 同单位），0 = 不设该侧边界；闭区间 [MtimeFrom, MtimeTo]。
	// 相对时间档（今天/近 7 天/近 30 天）由前端换算为绝对区间传入，后端只认区间
	// （档位语义单一数据源在前端）。幻灯片条目按归属文件的 mtime 判定（继承）。
	MtimeFrom int64 `json:"mtimeFrom"`
	MtimeTo   int64 `json:"mtimeTo"`
	// SizeFrom/SizeTo 文件大小区间筛选（facet，文件维度）：字节，0 = 不设该侧
	// 边界；闭区间 [SizeFrom, SizeTo]。幻灯片条目按归属文件的大小判定（继承）。
	SizeFrom int64 `json:"sizeFrom"`
	SizeTo   int64 `json:"sizeTo"`
	Limit    int   `json:"limit"`  // 分页大小（≤0 取默认 50，上限 200）
	Offset   int   `json:"offset"` // 分页偏移（<0 按 0 处理）
}

// AssetSearchHit 素材检索单元条目（平铺模式的一行；聚合模式的一个聚合条目或
// 独立文件条目）。幻灯片条目携带归属文件信息（FileID/FilePath）供溯源跳转。
type AssetSearchHit struct {
	RefType  string     `json:"refType"`  // 条目类型（AssetRefTypeFile / AssetRefTypeSlide）
	RefID    int64      `json:"refId"`    // 条目记录 ID：文件条目 = files.id，幻灯片条目 = slides.id
	FileID   int64      `json:"fileId"`   // 归属文件记录 ID（幻灯片溯源主键；文件条目 = 自身 ID）
	FilePath string     `json:"filePath"` // 归属文件相对库根目录路径（文件条目 = 自身路径）
	Title    string     `json:"title"`    // 展示标题：文件条目 = 文件标题，幻灯片条目 = 页标题
	SlideNum int        `json:"slideNum"` // 幻灯片条目页序（从 1 计）；文件条目 = 0
	Snippet  string     `json:"snippet"`  // 命中摘要（原文子串窗口，含省略号；纯标签筛选时为空）
	Score    float64    `json:"score"`    // bm25 相关性（FTS5 负值口径：越小越相关，仅排序用）；纯标签筛选 = 0
	HitCount int        `json:"hitCount"` // 聚合条目命中页数（平铺条目与文件条目 = 1）
	Tags     []AssetTag `json:"tags"`     // 文件标签（幻灯片条目继承归属文件标签）
	// 以下三字段仅幻灯片条目填充（页自有标注，详情面板数据源）；文件条目恒零值
	// （文件自有标注经 ListAssetFiles 的 AssetFileEntry 携带）。
	OwnTags []AssetTag `json:"ownTags"` // 页自有标签（幻灯片条目；文件条目恒空）
	Note    string     `json:"note"`    // 页业务备注（幻灯片条目；文件条目恒空串）
	Rating  int        `json:"rating"`  // 页星级 0-5（幻灯片条目；文件条目恒 0）
}

// AssetSearchPage 素材检索结果页（分页契约：Total 为过滤/聚合后的总条目数，
// 翻页以 Offset 递增 Limit 驱动，Total 不随页变化）。
type AssetSearchPage struct {
	Items     []AssetSearchHit `json:"items"`     // 当前页条目（按相关性降序；纯标签筛选按路径序）
	Total     int              `json:"total"`     // 过滤/聚合后的总条目数
	Keyword   string           `json:"keyword"`   // 本次检索关键词（原样回显）
	Flattened bool             `json:"flattened"` // 是否平铺模式（与入参 Flatten 一致）
}

// 素材拆分单文件处理结果状态取值（AssetSplitFileOutcome.Status，普通 string
// 常量非具名类型，不经 Wails 绑定层别名机制）。
const (
	AssetSplitStatusSplit   = "split"   // 拆分成功
	AssetSplitStatusSkipped = "skipped" // 增量命中跳过（已拆分且源未变化）
	AssetSplitStatusFailed  = "failed"  // 拆分失败（损坏/加密/只读等，批量异常隔离计数）
)

// AssetSplitResult 单文件拆分结果（SplitAssetFile 返回）。增量命中跳过时
// Skipped=true 且 Files 为空（产物未重写），PageCount 取已拆分记录的页数。
type AssetSplitResult struct {
	OutputDir  string   `json:"outputDir"`  // 拆分产物目录绝对路径
	Files      []string `json:"files"`      // 本次生成的单页文件绝对路径列表（按页序；跳过时为空）
	PageCount  int      `json:"pageCount"`  // 拆出页数（跳过时为记录页数）
	Skipped    bool     `json:"skipped"`    // 是否命中增量判定被跳过（已拆分且源未变化）
	SkipReason string   `json:"skipReason"` // 跳过原因（用户可读；Skipped=false 时为空）
}

// AssetSplitFileOutcome 批量拆分单文件处理结果条目（异常隔离明细）。
type AssetSplitFileOutcome struct {
	RelPath string `json:"relPath"` // 文件相对库根目录路径（正斜杠）
	Status  string `json:"status"`  // 处理结果（AssetSplitStatus* 取值）
	Message string `json:"message"` // 补充说明（跳过原因/失败原因；成功为空）
}

// AssetSplitBatchResult 批量拆分结果汇总（SplitAssetLibrary 返回 +
// assets-split:done 事件载荷；单文件入口同形状：Total=1）。批量中单文件失败
// 隔离计数不入 Items 的不算错误——整体失败（取消/库不存在/db 系统性错误）走
// assets-split:error 事件与 error 返回。
type AssetSplitBatchResult struct {
	Total   int                     `json:"total"`   // 启动时快照圈定的文件总数
	Split   int                     `json:"split"`   // 成功拆分文件数
	Skipped int                     `json:"skipped"` // 增量命中跳过数
	Failed  int                     `json:"failed"`  // 失败文件数（逐文件记日志跳过，批量不中断）
	Items   []AssetSplitFileOutcome `json:"items"`   // 逐文件明细（顺序同处理序）
}

// AssetSplitRecordView 拆分记录查询视图（GetAssetSplitRecord 返回，前端判定
// 「打开拆分单页」按钮展示与「已最新」提示）。
type AssetSplitRecordView struct {
	Exists    bool   `json:"exists"`    // 是否已有拆分记录
	OutputDir string `json:"outputDir"` // 拆分产物目录绝对路径
	PageCount int    `json:"pageCount"` // 已拆分页数
	SplitAt   string `json:"splitAt"`   // 最近拆分时间（"2006-01-02 15:04:05"）
	UpToDate  bool   `json:"upToDate"`  // 记录哈希与当前源哈希一致（源未变化）
}

// AssetSplitProgress 拆分进度事件载荷（assets-split:progress 事件，EventSink
// 下发）。Done/Total 为文件级计数（批量驱动总进度条），PageDone/PageTotal 为
// 当前文件页级透传（拆分核心逐页回调）。文件页级回调期间 Done = 已完成文件数
// （当前文件进行中）；文件收尾事件 Done 计入该文件（完成数口径，对齐
// assets-scan:progress）。
type AssetSplitProgress struct {
	LibraryID   string `json:"libraryId"`   // 正在拆分的库 ID
	Done        int    `json:"done"`        // 已完成文件数（含跳过/失败）
	Total       int    `json:"total"`       // 本次任务文件总数（单文件恒 1）
	CurrentFile string `json:"currentFile"` // 当前文件相对路径
	PageDone    int    `json:"pageDone"`    // 当前文件已完成页数（页级回调透传）
	PageTotal   int    `json:"pageTotal"`   // 当前文件总页数
}

// AssetSavedSearch 保存的素材检索条件（data/asset_saved_searches.json 全局存储，
// 命名/调取/删除/覆盖，SRS §2.5）。快照 = 检索语境完整还原面：关键词 + 标签 +
// facet 三维 + flatten + 页标签继承开关。
//
// 跨库调取降级契约（PRD D4）：TagIDs 为保存时所在库的标签 ID，仅同库调取直接
// 可用；TagNames 按名冗余一份，调取异库时前端按名在当前库标签清单中重查 ID，
// 查不到的标签丢弃（TagIDs 异库一律弃用，TagNames 仍可重映射）。关键词 /
// facet / Flatten / SlideTagOnlyOwn 跨库照常回填（不依赖库内容）。
type AssetSavedSearch struct {
	ID        int64  `json:"id"`        // 条目自增 ID（新增时由后端生成；非 0 = 更新覆盖语义）
	Name      string `json:"name"`      // 用户命名（全局唯一，重名拒绝）
	LibraryID string `json:"libraryId"` // 保存时所在素材库 ID（全局存储带库归属，供前端区分同/异库调取）
	Keyword   string `json:"keyword"`   // 关键词（空 = 无关键词）
	// TagIDs 保存时所在库的标签 ID 列表（仅同库调取直接可用，异库由前端按
	// TagNames 重映射）
	TagIDs []int64 `json:"tagIds"`
	// TagNames 标签名冗余快照（跨库重映射数据源；与 TagIDs 同序对应，保存时同步写入）
	TagNames []string `json:"tagNames"`
	// Ratings/MtimeFrom/MtimeTo/SizeFrom/SizeTo facet 三维快照，语义与
	// AssetSearchOptions 同名字段一致（文件级口径 + 幻灯片继承规则同源）
	Ratings         []int64 `json:"ratings"`         // 星级档（nil = 未启用；含 0 = 未评分）
	MtimeFrom       int64   `json:"mtimeFrom"`       // 修改时间区间下界（纳秒，0 = 不设界）
	MtimeTo         int64   `json:"mtimeTo"`         // 修改时间区间上界（纳秒，0 = 不设界）
	SizeFrom        int64   `json:"sizeFrom"`        // 文件大小区间下界（字节，0 = 不设界）
	SizeTo          int64   `json:"sizeTo"`          // 文件大小区间上界（字节，0 = 不设界）
	Flatten         bool    `json:"flatten"`         // 平铺开关快照（检索语境完整还原）
	SlideTagOnlyOwn bool    `json:"slideTagOnlyOwn"` // 页标签继承开关快照
	CreatedAt       string  `json:"createdAt"`       // 保存时间（RFC3339；更新覆盖时保留原值）
}
