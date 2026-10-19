package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// AssetCartService 素材购物栏服务（素材管理「购物栏」：跨库页级收集 + 按序合并导出）。
//
// 存储 data/asset_cart.json（全局单文件，条目带 libraryId 库归属），走 util.SaveJSON
// 原子写（temp → fsync → rename，docs/spec/data-atomic-write.md 契约，禁 os.WriteFile
// 截断写）。整体覆盖模型（内存列表全量序列化覆盖文件）：last-writer-wins，与
// asset_saved_searches.json 同口径——运行期手改该文件前须关闭 workbench.exe。
//
// 损坏降级：读取失败回退空购物栏 + slog.Warn，不阻塞调用链路（AssetSavedSearchService
// 同模式；降级后下一次写入即以空购物栏重建文件）。
//
// 并发口径：mu 保护「读-改-写」全程（List 返回深拷贝快照；Add/Remove/Reorder 全程
// 持锁落盘）。合并导出为长操作：exporting 标志互斥（重复导出报
// E_CART_EXPORT_IN_PROGRESS，warning 引导级），导出执行在锁外（启动导出时在锁内
// 快照条目清单后放锁——导出期间购物栏仍可查看，快照语义下中途增删不影响本次导出，
// 前端禁改为第一防线）；取消注册表复用 PptxMergeService（键 = 输出路径规范化，
// CancelAssetCartExport 全量取消），取消错误经 mapCancelError 统一映射
// ErrOperationCancelled（E_GIT_CANCELLED，前端 warning 中性提示）。
//
// 依赖注入：mergeSvc 为 PptxMergeService 实例（合并编排与 assets-cart:* 事件出口
// 持有者，AppServices 装配后 applySink 同步切换其事件通道）。
type AssetCartService struct {
	mergeSvc   *PptxMergeService
	configPath string

	mu        sync.Mutex
	loaded    bool                  // 首次 List/Add 懒加载标记（素材域持久化服务同模式）
	items     []model.AssetCartItem // 内存态（变更即时原子落盘）
	exporting bool                  // 合并导出进行中标志（重复导出互斥）
}

// assetCartJSON 落盘文件顶层结构（对象包装便于后续扩展伴随字段，对齐
// assetSavedSearchesJSON 先例）。
type assetCartJSON struct {
	Items []model.AssetCartItem `json:"items"`
}

// NewAssetCartService 构造购物栏服务（configPath 通常为 <dataDir>/asset_cart.json；
// mergeSvc 为购物栏导出复用的合并编排服务）。
func NewAssetCartService(configPath string, mergeSvc *PptxMergeService) *AssetCartService {
	return &AssetCartService{configPath: configPath, mergeSvc: mergeSvc}
}

// List 全部购物栏条目（按购物栏顺序）。返回深拷贝快照；逐条按 SrcPath 快照做
// os.Stat 存在性探测（轻量、非扫描），源文件不存在的条目 Stale 置位（前端置灰 +
// 可手动移除，不自动清除；导出跳过并在结果 SkippedItems 中报告）。
func (s *AssetCartService) List() ([]model.AssetCartItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return nil, err
	}
	out := make([]model.AssetCartItem, len(s.items))
	for i, item := range s.items {
		out[i] = item
		if _, err := os.Stat(item.SrcPath); err != nil {
			out[i].Stale = true
		}
	}
	return out, nil
}

// Add 批量加入购物栏（幂等去重）：键 (libraryId, fileId, slideNum) 已在栏的条目
// 跳过计入 Duplicates，不在栏的条目按入参顺序追加（批量加页的顺序语义 = 入参顺序）。
// AddedAt 后端权威生成；display 快照字段取调用方提交值。全部条目均已在栏时仅返回
// 计数不落盘。
func (s *AssetCartService) Add(items []model.AssetCartItem) (model.AssetCartAddResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return model.AssetCartAddResult{}, err
	}
	seen := make(map[string]bool, len(s.items)+len(items))
	for _, item := range s.items {
		seen[item.Key()] = true
	}
	addedAt := time.Now().Format(time.RFC3339)
	result := model.AssetCartAddResult{}
	for _, item := range items {
		if item.LibraryID == "" || item.FileID <= 0 || item.SlideNum < 1 {
			return model.AssetCartAddResult{}, fmt.Errorf("购物栏条目无效（libraryId/fileId/slideNum 必填，页码须为 1 起展示页序）")
		}
		key := item.Key()
		if seen[key] {
			result.Duplicates++
			continue
		}
		seen[key] = true
		item.AddedAt = addedAt
		s.items = append(s.items, item)
		result.Added++
	}
	if result.Added > 0 {
		if err := s.saveLocked(); err != nil {
			return model.AssetCartAddResult{}, err
		}
	}
	return result, nil
}

// Remove 按条目键批量移除（键形态 model.AssetCartItem.Key）。不存在的键容忍跳过
// （前端列表过期/重复点击幂等）；全部键均未命中时报错提示刷新。
func (s *AssetCartService) Remove(keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	remove := make(map[string]bool, len(keys))
	for _, key := range keys {
		remove[key] = true
	}
	kept := s.items[:0:0]
	removed := 0
	for _, item := range s.items {
		if remove[item.Key()] {
			removed++
			continue
		}
		kept = append(kept, item)
	}
	if removed == 0 {
		return errors.New("购物栏条目不存在或已被移除，请刷新列表")
	}
	s.items = kept
	return s.saveLocked()
}

// Clear 清空购物栏（购物栏为空时幂等返回，仍照常落盘保持空态一致）。
func (s *AssetCartService) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	s.items = nil
	return s.saveLocked()
}

// Reorder 按键列表重排购物栏顺序（拖拽排序回写）：orderedKeys 须为当前条目键的
// 完整排列（长度不符或含未知键拒绝，防静默丢条目；购物栏为空时该调用拒绝）。
func (s *AssetCartService) Reorder(orderedKeys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return err
	}
	if len(orderedKeys) != len(s.items) {
		return fmt.Errorf("排序清单与购物栏条目数不一致（%d ≠ %d），请刷新后重试", len(orderedKeys), len(s.items))
	}
	byKey := make(map[string]model.AssetCartItem, len(s.items))
	for _, item := range s.items {
		byKey[item.Key()] = item
	}
	reordered := make([]model.AssetCartItem, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		item, ok := byKey[key]
		if !ok {
			return fmt.Errorf("排序清单含未知条目（%s），请刷新后重试", key)
		}
		reordered = append(reordered, item)
	}
	s.items = reordered
	return s.saveLocked()
}

// MergeCartToPptx 把购物栏内全部条目按购物栏顺序合并导出为单一 .pptx（outPath 由
// 前端经 SaveFileDialog 选定）。
//
// 导出互斥：在途导出未结束前再次调用报 E_CART_EXPORT_IN_PROGRESS（warning 引导级）。
// 导出执行在锁外（启动时锁内快照条目 + 复位置位；快照语义下导出中购物栏变更不影响
// 本次导出，前端禁改为第一防线）。取消：转发 PptxMergeService 取消注册表
// （CancelAssetCartExport），取消错误经 mapCancelError 统一映射 E_GIT_CANCELLED。
// 空购物栏报引导错误；失效条目不预先剔除——合并核心对打不开的源文件按 SkippedItems
// 报告（导出结果统一出口，避免两处跳过口径漂移）。
func (s *AssetCartService) MergeCartToPptx(ctx context.Context, outPath string) (model.PptxMergeResult, error) {
	s.mu.Lock()
	if s.exporting {
		s.mu.Unlock()
		return model.PptxMergeResult{}, model.NewAppError(model.ErrCodeCartExportInProgress,
			"已有合并导出任务进行中，请等待完成或先取消")
	}
	if err := s.loadLocked(); err != nil {
		s.mu.Unlock()
		return model.PptxMergeResult{}, err
	}
	if len(s.items) == 0 {
		s.mu.Unlock()
		return model.PptxMergeResult{}, errors.New("购物栏为空，请先加入要导出的页")
	}
	mergeItems := make([]model.PptxMergeItem, 0, len(s.items))
	for _, item := range s.items {
		mergeItems = append(mergeItems, model.PptxMergeItem{SrcPath: item.SrcPath, PageNum: item.SlideNum})
	}
	s.exporting = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.exporting = false
		s.mu.Unlock()
	}()

	if s.mergeSvc == nil {
		return model.PptxMergeResult{}, errors.New("合并导出服务未初始化")
	}
	return s.mergeSvc.Merge(ctx, mergeItems, outPath)
}

// CancelAssetCartExport 取消全部在途购物栏合并导出（无路径参数：对齐
// CancelPptxSplit/CancelPptxMerge 的全量取消语义）。返回是否取消了至少一个在途
// 操作；无在途操作幂等返回 false。
func (s *AssetCartService) CancelAssetCartExport() bool {
	if s.mergeSvc == nil {
		return false
	}
	return s.mergeSvc.CancelPptxMerge()
}

// loadLocked 懒加载（文件不存在 = 首次使用，空购物栏；损坏降级空购物栏 + 告警日志，
// 不阻塞调用链路），须持 s.mu。
func (s *AssetCartService) loadLocked() error {
	if s.loaded {
		return nil
	}
	if !util.FileExists(s.configPath) {
		s.items = []model.AssetCartItem{}
		s.loaded = true
		return nil
	}
	var data assetCartJSON
	if err := util.LoadJSON(s.configPath, &data); err != nil {
		// 损坏降级：回退空购物栏（下一次写入即以空购物栏重建文件），不阻塞调用链路
		Logger().Warn("购物栏文件读取失败，降级为空购物栏", "path", s.configPath, "err", err)
		s.items = []model.AssetCartItem{}
		s.loaded = true
		return nil
	}
	if data.Items == nil {
		data.Items = []model.AssetCartItem{}
	}
	s.items = data.Items
	s.loaded = true
	return nil
}

// saveLocked 原子落盘（util.SaveJSON：temp → fsync → rename，禁 os.WriteFile 直写），
// 须持 s.mu。
func (s *AssetCartService) saveLocked() error {
	if err := util.SaveJSON(s.configPath, assetCartJSON{Items: s.items}); err != nil {
		return fmt.Errorf("保存购物栏失败: %w", err)
	}
	return nil
}
