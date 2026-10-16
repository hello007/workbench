package main

import (
	"context"

	"workbench/model"
)

// ===== 素材管理（侧边栏） =====
//
// 委托 AssetLibraryService：素材库 CRUD + 目录扫描（增量索引）+ 文件/幻灯片
// 列表 + 文件级元数据编辑 + PPT 按页拆分（单文件/库级批量 + 增量判定 + 跳转
// 方式 2）。扫描进度/终态经 assets-scan:* 事件推送（可经 CancelAssetScan 取消），
// 拆分进度/终态经 assets-split:* 事件推送（可经 CancelAssetSplit 取消；取消属
// 预期拒绝，前端按 warning 中性提示）。跨层契约见 docs/spec/cross-layer-contracts.md：
// 新增 App 方法与 model 导出 struct 须经 wails generate module 同步 frontend/wailsjs/
// 三处（App.js / App.d.ts / models.ts）。

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

// ListAssetSlides 幻灯片索引条目分页（标签继承归属文件），按文件路径 + 页序
// （画廊幻灯片视图数据源，页序以 sldIdLst 为准）。分页契约对齐 SearchAssets：
// limit ≤0 取默认 50（上限 200）、offset <0 按 0 处理，Total 为总条目数。
func (a *App) ListAssetSlides(libraryID string, limit, offset int) (model.AssetSlidePage, error) {
	return a.assetsSvc.ListSlides(libraryID, limit, offset)
}

// UpdateAssetFileMeta 文件级元数据编辑（整体替换）：星级 0-5（0 = 清除）、备注、
// 标签集（整体替换；不存在的标签自动创建并分配默认色）。
func (a *App) UpdateAssetFileMeta(libraryID string, fileID int64, meta model.AssetFileMetaUpdate) error {
	return a.assetsSvc.UpdateFileMeta(libraryID, fileID, meta)
}

// UpdateAssetSlideMeta 幻灯片页级标注编辑（整体替换）：业务备注/星级/页自有标签
// （区别于解析自带演讲者备注；与文件标签存储完全独立）。纯标注层——不触发重解析
// 与缩略图失效，业务备注不入 FTS。
func (a *App) UpdateAssetSlideMeta(libraryID string, fileID int64, slideNum int, meta model.AssetSlideMetaUpdate) error {
	return a.assetsSvc.UpdateSlideMeta(libraryID, fileID, slideNum, meta)
}

// SearchAssets 素材检索：关键词全文检索（文件名 > 幻灯片标题 > 页文本 > 备注
// bm25 加权，CJK 按字切分 phrase 匹配）与标签筛选（文件直接绑定、幻灯片继承
// 归属文件标签）可组合。Flatten=false 时同一 PPT 多页命中折叠为聚合条目
// （HitCount = 命中页数）；结果条目带类型标记（file/slide），幻灯片条目携带
// 归属文件信息供溯源。
func (a *App) SearchAssets(libraryID string, keyword string, opts model.AssetSearchOptions) (model.AssetSearchPage, error) {
	return a.assetsSvc.SearchAssets(libraryID, keyword, opts)
}

// ListAssetTags 全量标签清单（父级分组 + 组内 sort_order 序，D8#1 拖拽排序契约）：
// 标签树构建数据源（含未使用标签），条目带直接绑定计数（文件绑定 + 页自有绑定
// 行数之和）、父级 ID（0 = 根标签，前端按 ParentID 组织树形）与组内序号。
func (a *App) ListAssetTags(libraryID string) ([]model.AssetTagListItem, error) {
	return a.assetsSvc.ListTags(libraryID)
}

// CreateAssetTag 新建标签：重名拒绝；parentID ≠ 0 须为已存在标签（新建无子代
// 不存在成环）；color 空则按标签名确定性分配默认色板。
func (a *App) CreateAssetTag(libraryID, name string, parentID int64, color string) (model.AssetTag, error) {
	return a.assetsSvc.CreateTag(libraryID, name, parentID, color)
}

// UpdateAssetTag 标签编辑（重命名/改色/移动父级整体提交）：重名拒绝；移动父级
// 防环（新父不能是自身或自身后代，会形成循环引用）。
func (a *App) UpdateAssetTag(libraryID string, tagID int64, name string, parentID int64, color string) error {
	return a.assetsSvc.UpdateTag(libraryID, tagID, name, parentID, color)
}

// DeleteAssetTag 删除标签：级联删除自身 + 全部后代并解除全部绑定（删除标签不删
// 素材，单事务）。返回解除关联的绑定行数（前端确认弹窗影响面展示口径）。
func (a *App) DeleteAssetTag(libraryID string, tagID int64) (int64, error) {
	return a.assetsSvc.DeleteTag(libraryID, tagID)
}

// MoveAssetTag 标签拖拽排序/移动父级（PRD D8#1 真机增量，标签管理弹窗 el-tree
// 拖拽入口）：newParentID = 0 表示移到根级；index = 落位后自身在目标父下兄弟组
// 内的最终序号（before/after 落点 = 同级排序、inner 落点 = 变子级组尾，前端经
// resolveTagMoveDrop 换算）。事务内防环（新父不能是自身/自身后代）+ 目标组与
// 旧组兄弟序号连续化重排。
func (a *App) MoveAssetTag(libraryID string, tagID int64, newParentID int64, index int) error {
	return a.assetsSvc.MoveTag(libraryID, tagID, newParentID, index)
}

// BatchUpdateAssetFileTags 文件批量打标签（SRS §2.4，文件模式选中集操作）：
// op 为相对语义「现标签集 − removeTags ∪ addTags」（非整体替换），不存在的
// 添加标签名自动创建为根标签。每条目一事务逐条目提交，部分失败不中断不回滚，
// 逐条目明细返回（Total/Succeeded/Failed + Items）。
func (a *App) BatchUpdateAssetFileTags(libraryID string, fileIDs []int64, op model.AssetBatchTagOp) (model.AssetBatchTagResult, error) {
	return a.assetsSvc.BatchUpdateFileTags(libraryID, fileIDs, op)
}

// BatchUpdateAssetSlideTags 页自有标签批量打标签（幻灯片模式选中集操作）：目标
// 键 (fileId, slideNum)。纯标注层——不触发重解析与缩略图失效；明细与参数校验
// 口径对齐 BatchUpdateAssetFileTags。
func (a *App) BatchUpdateAssetSlideTags(libraryID string, targets []model.AssetSlideTarget, op model.AssetBatchTagOp) (model.AssetBatchTagResult, error) {
	return a.assetsSvc.BatchUpdateSlideTags(libraryID, targets, op)
}

// RebuildAssetLibrary 手动全量重建：清该库文件/幻灯片/检索索引记录后全量重扫
// （索引自愈入口，统计口径漂移时使用）。文件标签绑定与幻灯片页级标注（标签/
// 业务备注/星级）按「相对路径 + 页序」映射重绑（页已消失的页标注丢弃）；文件级
// 星级/备注不保留（文件元数据重建语义）。进度/终态事件与增量扫描同通道
// （assets-scan:*），扫描中并发触发被拒（E_ASSETS_SCAN_IN_PROGRESS）。
func (a *App) RebuildAssetLibrary(libraryID string) (model.AssetScanResult, error) {
	return a.assetsSvc.RebuildScan(context.Background(), libraryID)
}

// AssetThumbsAvailable 探测本机 PowerPoint COM 可用性（前端 D4 降级依据：
// false 时画廊提示条常驻 + 跳转入口隐藏）。结果由 service 层按进程生命周期缓存。
func (a *App) AssetThumbsAvailable() bool {
	return a.assetsSvc.ThumbsAvailable()
}

// JumpToAssetSlide 打开 PPT 并跳到指定页（SRS §2.2.5；PowerPoint COM）。
// 同步语义：批量生成在途时经页间插队执行（PRD D3）。无 PowerPoint 环境返回
// E_ASSETS_THUMBS_UNAVAILABLE（前端入口已隐藏，此为防御）。
func (a *App) JumpToAssetSlide(absPath string, slideNum int) error {
	return a.assetsSvc.JumpToSlide(absPath, slideNum)
}

// CancelAssetThumbs 取消指定库的缩略图批量生成（对齐 CancelAssetScan 交互语义）。
func (a *App) CancelAssetThumbs(libraryID string) bool {
	return a.assetsSvc.CancelAssetThumbs(libraryID)
}

// GetAssetThumb 读取单页缩略图 PNG（base64 标准编码裸串）。tierIndex：0=small
// （画廊卡片）1=hd（详情大图）；未生成/已淘汰返回空串（非错误，前端回退占位卡）。
// 前端消费契约：返回值直接拼 data URL（`data:image/png;base64,<返回串>`），
// 禁 decodeBase64Utf8（文本专用）禁裸 atob（PR3 画廊接入时遵守）。
func (a *App) GetAssetThumb(libraryID string, fileID int64, slideNum, tierIndex int) (string, error) {
	return a.assetsSvc.GetAssetThumb(libraryID, fileID, slideNum, tierIndex)
}

// RetryAssetThumb 单文件缩略图重试（D4）：画廊/详情「重试」按钮入口，复用缩略图
// 队列链路（同库旧批次取消语义已有）。无 PowerPoint 返回 E_ASSETS_THUMBS_UNAVAILABLE。
func (a *App) RetryAssetThumb(libraryID string, fileID int64, relPath string) error {
	return a.assetsSvc.RetryAssetThumb(libraryID, fileID, relPath)
}

// SplitAssetFile 拆分单个素材文件（SRS §2.2.3，画廊卡/详情侧边栏入口）：复用
// 工具箱拆分核心（纯 Go OOXML），落点二选一——默认素材库缓存目录（derived 镜像
// 源树，库根之外天然隔离扫描）；outputToSourceDir=true 落源目录（与源同目录，
// isPptxSplitDerivative 已自动排除索引）。增量命中跳过返回 Skipped=true（前端
// 提示「已最新」），核心失败（损坏/加密/只读等）返回错误。进度/终态经
// assets-split:* 事件推送（素材面板独听），在途任务可经 CancelAssetSplit 取消。
func (a *App) SplitAssetFile(libraryID, relPath string, outputToSourceDir bool) (model.AssetSplitResult, error) {
	return a.assetsSvc.SplitAssetFile(context.Background(), libraryID, relPath, outputToSourceDir)
}

// SplitAssetLibrary 库级批量拆分（SRS §2.2.3「批量目录拆分」，库面板「拆分全部
// PPT」入口）：启动时快照圈定全部 ok 状态文件逐个处理；单文件失败逐文件记日志
// 跳过，批量不中断，返回汇总（total/split/skipped/failed + 逐文件明细）。同库
// 并发拆分被拒（E_ASSETS_SCAN_IN_PROGRESS），在途任务可经 CancelAssetSplit 取消。
func (a *App) SplitAssetLibrary(libraryID string, outputToSourceDir bool) (model.AssetSplitBatchResult, error) {
	return a.assetsSvc.SplitAssetLibrary(context.Background(), libraryID, outputToSourceDir)
}

// CancelAssetSplit 取消指定库的在途拆分（对齐 CancelAssetScan 交互语义，取消
// 错误前端按 E_ASSETS_CANCELLED warning 中性提示）。返回是否存在可取消的在途拆分。
func (a *App) CancelAssetSplit(libraryID string) bool {
	return a.assetsSvc.CancelAssetSplit(libraryID)
}

// GetAssetSplitRecord 查询文件拆分记录（跳转方式 2 判定面）：Exists=false 未拆分；
// UpToDate=true 源未变化（前端提示「已最新」）。PageCount 供页序校验展示。
func (a *App) GetAssetSplitRecord(libraryID, relPath string) (model.AssetSplitRecordView, error) {
	return a.assetsSvc.GetAssetSplitRecord(libraryID, relPath)
}

// OpenAssetSplitPage 打开拆分单页产物（SRS §2.2.5 方式 2，无 PowerPoint COM 依赖，
// 与方式 1 JumpToAssetSlide 并存）：按拆分记录定位 <产物目录>/<源名>-NNN.pptx，
// 页序越界或产物缺失报错，系统默认关联程序打开。
func (a *App) OpenAssetSplitPage(libraryID, relPath string, slideNum int) error {
	return a.assetsSvc.OpenAssetSplitPage(libraryID, relPath, slideNum)
}
