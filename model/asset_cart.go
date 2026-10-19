package model

import "strconv"

// AssetCartItem 素材购物栏条目（素材管理「购物栏」：跨库页级收集 + 按序合并导出）。
//
// 身份键 = (LibraryID, FileID, SlideNum)：FileID 为 files.id（跨重解析稳定，禁
// slides.id——每轮重解析删旧插新全变，与选中体系/缩略图缓存键同口径契约）；跨库
// 条目混装（FileID 仅库内唯一），LibraryID 必填。
//
// 快照字段（SrcPath/FileName/SlideTitle/LibraryName）入栏时定格，仅作展示与失效
// 探测基准，不随后端重解析回写——源文件 lost/重链后展示旧名属登记取舍，条目可
// 手动移除。Stale 为 List 时按 SrcPath 存在性探测（os.Stat，轻量非扫描）动态计算
// 的非持久化字段：置灰展示 + 导出跳过（合并核心对缺失源文件按 SkippedItems 报告），
// 不自动移除。
type AssetCartItem struct {
	LibraryID   string `json:"libraryId"`             // 所属素材库 ID（跨库条目混装的身份分量）
	FileID      int64  `json:"fileId"`                // 源文件记录 ID（files.id，重解析稳定）
	SlideNum    int    `json:"slideNum"`              // 页序（1 起展示页序，sldIdLst 三跳映射口径）
	SrcPath     string `json:"srcPath"`               // 源文件绝对路径快照（失效探测基准 + 合并清单来源）
	FileName    string `json:"fileName"`              // 展示文件名快照
	SlideTitle  string `json:"slideTitle,omitempty"`  // 页标题快照（来源页条目 title，可空）
	LibraryName string `json:"libraryName,omitempty"` // 库名快照（来源行「库名/文件名/页码」展示）
	AddedAt     string `json:"addedAt"`               // 入栏时间（RFC3339，后端权威生成）
	Stale       bool   `json:"stale,omitempty"`       // 源文件已不存在（List 时探测，非持久化字段）
}

// Key 条目键（Remove/Reorder 与前端已收集态判定的统一形态）：
// `<libraryId>|<fileId>|<slideNum>`。libraryId 为随机 hex、fileId/slideNum 为数字，
// 竖线分隔无歧义；前端 utils/assetCart.js 的 assetCartItemKey 同构实现（跨层键形态
// 契约，两处禁漂移）。
func (i AssetCartItem) Key() string {
	return i.LibraryID + "|" + strconv.FormatInt(i.FileID, 10) + "|" + strconv.Itoa(i.SlideNum)
}

// AssetCartAddResult 加入购物栏结果（Add 幂等去重语义）。
//
// Added = 本次新增条目数；Duplicates = 键已在栏被去重跳过的条目数（重复加号操作
// 幂等：added=0 且 duplicates>0 时前端提示「已在购物栏」）。
type AssetCartAddResult struct {
	Added      int `json:"added"`
	Duplicates int `json:"duplicates"`
}
