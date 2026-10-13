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
	Tags         []AssetTag `json:"tags"`         // 标签：继承归属文件标签（本批幻灯片无自有标签）
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

// AssetFileMetaUpdate 文件级元数据编辑入参（UpdateAssetFileMeta 整体替换语义：
// 星级/备注/标签以本次提交值为准，未提及的标签即移除）。
type AssetFileMetaUpdate struct {
	Rating int      `json:"rating"` // 星级 0-5（0 = 清除评分）
	Note   string   `json:"note"`   // 备注（整体替换）
	Tags   []string `json:"tags"`   // 标签名列表（整体替换；不存在的标签自动创建）
}

// 素材检索条目类型标记（AssetSearchHit.RefType 取值，普通 string 常量非具名
// 类型，不经 Wails 绑定层别名机制）。
const (
	AssetRefTypeFile  = "file"  // 文件条目（整份 pptx 命中，如文件名/备注命中）
	AssetRefTypeSlide = "slide" // 幻灯片条目（单页命中）
)

// AssetSearchOptions 素材检索入参（SearchAssets）。关键词与标签筛选可组合；
// 关键词为空且给定标签时退化为纯标签浏览。
type AssetSearchOptions struct {
	Flatten bool    `json:"flatten"` // true = 平铺返回全部命中；false = 同 PPT 多页命中折叠为聚合条目
	TagIDs  []int64 `json:"tagIds"`  // 标签筛选：文件条目须直接绑定其中之一，幻灯片条目须归属文件绑定其中之一（继承）；空 = 不筛选
	Limit   int     `json:"limit"`   // 分页大小（≤0 取默认 50，上限 200）
	Offset  int     `json:"offset"`  // 分页偏移（<0 按 0 处理）
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
}

// AssetSearchPage 素材检索结果页（分页契约：Total 为过滤/聚合后的总条目数，
// 翻页以 Offset 递增 Limit 驱动，Total 不随页变化）。
type AssetSearchPage struct {
	Items     []AssetSearchHit `json:"items"`     // 当前页条目（按相关性降序；纯标签筛选按路径序）
	Total     int              `json:"total"`     // 过滤/聚合后的总条目数
	Keyword   string           `json:"keyword"`   // 本次检索关键词（原样回显）
	Flattened bool             `json:"flattened"` // 是否平铺模式（与入参 Flatten 一致）
}
