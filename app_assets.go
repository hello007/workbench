package main

import (
	"context"

	"workbench/model"
)

// ===== 素材管理（侧边栏） =====
//
// 委托 AssetLibraryService：素材库 CRUD + 目录扫描（增量索引）+ 文件/幻灯片
// 列表 + 文件级元数据编辑。扫描进度/终态经 assets-scan:* 事件推送，扫描过程
// 可经 CancelAssetScan 取消（取消属预期拒绝，前端按 warning 中性提示）。跨层
// 契约见 docs/spec/cross-layer-contracts.md：新增 App 方法与 model 导出 struct
// 须经 wails generate module 同步 frontend/wailsjs/ 三处（App.js / App.d.ts /
// models.ts）。

// CreateAssetLibrary 新建素材库：登记注册表并预建索引库文件。校验名称非空、
// 根目录存在且为目录；索引模式只读引用原始文件（扫描不修改源文件）。
func (a *App) CreateAssetLibrary(name, rootPath string) (model.AssetLibrary, error) {
	return a.assetsSvc.CreateLibrary(name, rootPath)
}

// ListAssetLibraries 全部素材库（注册表顺序，供库切换下拉）。
func (a *App) ListAssetLibraries() ([]model.AssetLibrary, error) {
	return a.assetsSvc.ListLibraries()
}

// DeleteAssetLibrary 删除素材库：关连接 + 删 db 文件（含 WAL/SHM）+ 删注册表项。
// 标签体系随库文件删除天然隔离；库正在扫描时拒绝删除（E_ASSETS_SCAN_IN_PROGRESS）。
func (a *App) DeleteAssetLibrary(libraryID string) error {
	return a.assetsSvc.DeleteLibrary(libraryID)
}

// GetAssetLibraryStats 库统计：文件/幻灯片总数与失败/加密跳过数。
func (a *App) GetAssetLibraryStats(libraryID string) (model.AssetLibraryStats, error) {
	return a.assetsSvc.GetLibraryStats(libraryID)
}

// StartAssetScan 扫描指定素材库（增量索引，原始文件不动）：递归索引根目录下
// 全部 pptx，逐页解析文本/标题/备注入库。进度经 assets-scan:progress 事件推送，
// 终态经 assets-scan:done / assets-scan:error；同库并发扫描被拒
// （E_ASSETS_SCAN_IN_PROGRESS）。
func (a *App) StartAssetScan(libraryID string) (model.AssetScanResult, error) {
	return a.assetsSvc.StartScan(context.Background(), libraryID)
}

// CancelAssetScan 取消指定库的在途扫描。返回是否存在可取消的在途扫描；无在途
// 扫描幂等返回 false。
func (a *App) CancelAssetScan(libraryID string) bool {
	return a.assetsSvc.CancelScan(libraryID)
}

// ListAssetFiles 文件索引条目（含标签），按相对路径序（画廊文件视图数据源）。
func (a *App) ListAssetFiles(libraryID string) ([]model.AssetFileEntry, error) {
	return a.assetsSvc.ListFiles(libraryID)
}

// ListAssetSlides 幻灯片索引条目（标签继承归属文件），按文件路径 + 页序
// （画廊幻灯片视图数据源，页序以 sldIdLst 为准）。
func (a *App) ListAssetSlides(libraryID string) ([]model.AssetSlideEntry, error) {
	return a.assetsSvc.ListSlides(libraryID)
}

// UpdateAssetFileMeta 文件级元数据编辑（整体替换）：星级 0-5（0 = 清除）、备注、
// 标签集（整体替换；不存在的标签自动创建并分配默认色）。
func (a *App) UpdateAssetFileMeta(libraryID string, fileID int64, meta model.AssetFileMetaUpdate) error {
	return a.assetsSvc.UpdateFileMeta(libraryID, fileID, meta)
}

// SearchAssets 素材检索：关键词全文检索（文件名 > 幻灯片标题 > 页文本 > 备注
// bm25 加权，CJK 按字切分 phrase 匹配）与标签筛选（文件直接绑定、幻灯片继承
// 归属文件标签）可组合。Flatten=false 时同一 PPT 多页命中折叠为聚合条目
// （HitCount = 命中页数）；结果条目带类型标记（file/slide），幻灯片条目携带
// 归属文件信息供溯源。
func (a *App) SearchAssets(libraryID string, keyword string, opts model.AssetSearchOptions) (model.AssetSearchPage, error) {
	return a.assetsSvc.SearchAssets(libraryID, keyword, opts)
}

// RebuildAssetLibrary 手动全量重建：清该库文件/幻灯片/检索索引记录后全量重扫
// （索引自愈入口，统计口径漂移时使用）。文件标签绑定按相对路径映射重绑；
// 星级/备注不保留（重建语义）。进度/终态事件与增量扫描同通道
// （assets-scan:*），扫描中并发触发被拒（E_ASSETS_SCAN_IN_PROGRESS）。
func (a *App) RebuildAssetLibrary(libraryID string) (model.AssetScanResult, error) {
	return a.assetsSvc.RebuildScan(context.Background(), libraryID)
}
