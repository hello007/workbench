package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// AssetLibraryService 素材库管理服务（侧边栏「素材管理」MVP）。
//
// 职责：素材库注册表（data/assets/libraries.json，util.SaveJSON 原子写）+ 各库
// SQLite 索引连接缓存（data/assets/<library-id>.db，懒打开）+ 文件级元数据编辑
// + 库统计。目录扫描（StartScan）与 PPT 解析（parsePptxSlides）分文件实现，
// 均挂本服务。
//
// 并发模型：
//   - s.mu 保护注册表与 openDbs 连接缓存；库 db 查询经 withStore 在锁内取连接；
//   - 扫描串行保护：同库同时仅允许一个扫描（tryLockScan TryLock 互斥拒绝，
//     对齐 GitService.tryLockRepo 模式）；DeleteLibrary 以同把锁 TryLock 与在途
//     扫描互斥（扫描中禁删库，防连接被关闭）；
//   - 拆分串行保护：同库同时仅允许一个拆分任务（tryLockSplit，独立 splitLocks
//     map——拆分与扫描语义可并发：拆分不写 files/slides 索引表，仅读写
//     split_records，单连接池串行化；仅拆分自身互斥）；DeleteLibrary 以同把锁
//     TryLock 与在途拆分互斥（拆分中禁删库）；
//   - 取消注册表（cancels/splitCancels + opMu）：键 = normalizeRepoKey(libraryID)，
//     键空间独立——CancelScan 不得误杀拆分、CancelAssetSplit 亦不影响扫描；
//     defer LIFO 先注销后释放锁（docs/spec/logging-and-errors.md 取消链路契约）。
type AssetLibraryService struct {
	assetsDir  string // data/assets（注册表 libraries.json + 各库 <id>.db）
	sinkHolder        // 事件出口持有器（assets-scan:* 事件推送），构造注入 + serve 模式经 SetEventSink 切换

	mu     sync.Mutex              // 保护 libs/openDbs 的并发读写
	libs   []model.AssetLibrary    // 注册表内存态（loadRegistry 懒加载，变更即时原子落盘）
	loaded bool                    // 注册表是否已从磁盘加载
	open   map[string]*assetsStore // libraryID -> 已打开的库连接缓存

	opMu         sync.Mutex              // 保护 opLocks/cancels/splitLocks/splitCancels 的并发读写
	opLocks      map[string]*sync.Mutex  // libraryID（规范化）-> 扫描互斥锁（懒创建）
	cancels      map[string]*cancelEntry // libraryID（规范化）-> 在途扫描注册条目（复用 git.go 的 cancelEntry）
	splitLocks   map[string]*sync.Mutex  // libraryID（规范化）-> 拆分互斥锁（懒创建；与扫描锁独立）
	splitCancels map[string]*cancelEntry // libraryID（规范化）-> 在途拆分注册条目（assets_split.go）

	scanWG sync.WaitGroup // 在途扫描计数（runLibraryScan 增减），WaitScansDone 关停收口等待

	splitWG sync.WaitGroup // 在途拆分计数（拆分入口增减，对齐 scanWG），WaitSplitsDone 关停收口等待

	parseFn assetsParseFn // 解析注入点（nil = 生产实现 parsePptxSlides，单测注入 fake）

	// splitCoreFn 拆分核心注入点（nil = 生产实现 SplitPptxPerPageWithProgress，
	// 单测注入 fake 驱动阻塞/取消，assets_split.go）；openSplitPageFn 拆分单页
	// 打开动作注入点（默认 openWithSystemHandler 跨平台实现，单测注入 fake 隔离
	// 真实系统调用）
	splitCoreFn     assetsSplitCore
	openSplitPageFn func(path string) error

	thumbQ      *thumbQueue       // 缩略图串行队列（assets_thumbs.go，扫描完成后批量入队 + 打开跳页）
	thumbLedger *thumbCacheLedger // 缩略图记账 ledger（assets_thumbs_ledger.go，LRU 记账 + 失效 + 配额淘汰）

	thumbsCfgFn func() model.AssetThumbsSettings // 缩略图设置读取口（装配期 SetThumbsSettingsProvider 注入，nil 走默认值）
}

// ErrAssetsCancelled 素材扫描被用户取消（对齐 ErrOperationCancelled 模式）。
// 前端按 E_ASSETS_CANCELLED 走中性提示（warning），非 error 误报。
var ErrAssetsCancelled = model.NewAppError(model.ErrCodeAssetsCancelled, "扫描已取消")

// errAssetLibraryNotFound 素材库不存在或已删除（列表过期），前端刷新库列表。
var errAssetLibraryNotFound = model.NewAppError(model.ErrCodeAssetsLibNotFound, "素材库不存在或已被删除，请刷新素材库列表")

// mapAssetsCancelError 将 ctx 取消错误（%w 包装 context.Canceled 透传）映射为
// ErrAssetsCancelled sentinel，其余错误原样透传（单点实现防漂移）。
func mapAssetsCancelError(err error) error {
	if errors.Is(err, context.Canceled) {
		return ErrAssetsCancelled
	}
	return err
}

// assetLibraryRegistryJSON 素材库注册表文件结构（libraries.json）。
type assetLibraryRegistryJSON struct {
	Libraries []model.AssetLibrary `json:"libraries"`
}

// NewAssetLibraryService 创建素材库管理服务。assetsDir 为索引数据目录
// （生产装配传 data/assets），目录不存在时首次写注册表自动创建。
// ctx 仅用于构造注入 Wails 桌面事件出口（PptxSplitService 同款；允许 nil/
// Background——wailsEventSink 对非 Wails 上下文集中防护静默跳过）。
func NewAssetLibraryService(ctx context.Context, assetsDir string) *AssetLibraryService {
	s := &AssetLibraryService{
		assetsDir: assetsDir,
		open:      make(map[string]*assetsStore),
	}
	s.SetEventSink(NewWailsEventSink(ctx))
	// 拆分单页打开动作默认实现（方式 2，openWithSystemHandler 跨平台现成实现的
	// 导出薄封装；单测可注入 fake 隔离真实系统调用）
	s.openSplitPageFn = NewFileOperationService().OpenWithSystemHandler
	// 先建 ledger（storeFn/dirFn/quotaFn 注入宿主方法），再建 queue（传 ledger
	// 与档位读取口）——flusher goroutine 由首次 record 惰性启动（对齐
	// thumbQueue.start 模式）
	s.thumbLedger = newThumbCacheLedger(s.storeForThumbs, s.assetThumbsDir, s.thumbQuotaBytes)
	s.thumbQ = newThumbQueue(newThumbExporter(), s.eventSink, s.thumbLedger, s.resolvedThumbTiers)
	return s
}

// storeForThumbs 缩略图记账落盘专用连接获取：锁内先验库注册在册再取缓存连接
// （单次持锁完成校验与打开，防「校验通过→放锁→删库→重新拿锁惰性建库」窗口
// 重建已删库的孤儿 db——对齐 runLibraryScan 锁内复验的坑）。返回后 db 操作在
// 锁外执行（ledger flushLibrary 已保证）。
func (s *AssetLibraryService) storeForThumbs(libraryID string) (*assetsStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getLibraryLocked(libraryID); err != nil {
		return nil, err
	}
	return s.storeForLocked(libraryID)
}

// CreateLibrary 新建素材库：登记注册表并预建索引库文件（schema 初始化前移，
// 建库即暴露磁盘/驱动问题，而非等到首次扫描）。
// 校验：名称非空；根目录必须存在且为目录（索引模式只读引用，原始文件不动）。
func (s *AssetLibraryService) CreateLibrary(name, rootPath string) (model.AssetLibrary, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.AssetLibrary{}, errors.New("请输入素材库名称")
	}
	abs, err := filepath.Abs(rootPath)
	if err != nil {
		return model.AssetLibrary{}, fmt.Errorf("素材目录路径无效: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return model.AssetLibrary{}, fmt.Errorf("素材目录不存在或无法访问: %w", err)
	}
	if !info.IsDir() {
		return model.AssetLibrary{}, errors.New("素材路径必须是目录（索引模式引用原始文件所在目录）")
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	lib := model.AssetLibrary{
		ID:        newAssetLibraryID(),
		Name:      name,
		RootPath:  abs,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadRegistryLocked(); err != nil {
		return model.AssetLibrary{}, err
	}
	// 预建索引库文件：打开即完成 schema 迁移，失败则不入注册表（无半态库）
	if _, err := s.storeForLocked(lib.ID); err != nil {
		return model.AssetLibrary{}, err
	}
	s.libs = append(s.libs, lib)
	if err := s.saveRegistryLocked(); err != nil {
		// 回滚预建的索引库文件：注册表写失败若留下 <id>.db，库里无此库登记，
		// 形成孤儿文件（删库流程不会清理它）。先关连接再删（Windows 下打开句柄
		// 占用会致删除失败），WAL/SHM 伴生文件一并清理；回删失败仅告警不掩盖
		// 原始错误（下次建库走新 ID，旧文件无害）。
		if store, ok := s.open[lib.ID]; ok {
			if closeErr := store.Close(); closeErr != nil {
				Logger().Warn("close rolled-back assets store failed", "libraryId", lib.ID, "err", closeErr)
			}
			delete(s.open, lib.ID)
		}
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if rmErr := os.Remove(s.libraryDBPath(lib.ID) + suffix); rmErr != nil && !os.IsNotExist(rmErr) {
				Logger().Warn("remove rolled-back assets db failed", "libraryId", lib.ID, "err", rmErr)
			}
		}
		return model.AssetLibrary{}, err
	}
	return lib, nil
}

// ListLibraries 全部素材库（注册表顺序）。
func (s *AssetLibraryService) ListLibraries() ([]model.AssetLibrary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadRegistryLocked(); err != nil {
		return nil, err
	}
	libs := make([]model.AssetLibrary, len(s.libs))
	copy(libs, s.libs)
	return libs, nil
}

// getLibrary 取单个库注册信息（不存在返回 errAssetLibraryNotFound）。
func (s *AssetLibraryService) getLibrary(libraryID string) (model.AssetLibrary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLibraryLocked(libraryID)
}

func (s *AssetLibraryService) getLibraryLocked(libraryID string) (model.AssetLibrary, error) {
	if err := s.loadRegistryLocked(); err != nil {
		return model.AssetLibrary{}, err
	}
	for _, lib := range s.libs {
		if lib.ID == libraryID {
			return lib, nil
		}
	}
	return model.AssetLibrary{}, errAssetLibraryNotFound
}

// DeleteLibrary 删除素材库：取消缩略图批次 → 关连接 → 删 db 文件（含 WAL/SHM
// 伴生文件）→ 删缩略图目录 → 清记账 dirty → 删注册表项（原子落盘）。标签体系
// 随库文件删除天然隔离，无需逐表清理。
// 在途扫描保护：库正在扫描时拒绝删除（warning 引导稍后再试），防扫描写已关连接。
func (s *AssetLibraryService) DeleteLibrary(libraryID string) error {
	// 缩略图批次取消（进入删除前）：缩小「worker 向已删目录写 PNG」窗口。取消
	// 异步不等待——在途单文件（最长单文件超时）仍可能经 MkdirAll 重建空 tier
	// 目录并残留少量 PNG（已删库 ID 下不进任何清理链路），量级可手动清理，注释
	// 口径为「缩小窗口 + 残留可手动清理」而非完全防止。CancelFunc 幂等，无在途
	// 批次无害。
	s.CancelAssetThumbs(libraryID)

	// 在途扫描保护：tryLockScan 与扫描互斥（锁被占用 = 库正在扫描，互斥拒绝）
	release, err := s.tryLockScan(libraryID)
	if err != nil {
		return err
	}
	defer release()

	// 在途拆分保护：tryLockSplit 与拆分互斥（同扫描口径：拆分中禁删库，防拆分
	// 写已关连接；先扫描锁后拆分锁顺序获取，无反向获取路径不成环）
	releaseSplit, err := s.tryLockSplit(libraryID)
	if err != nil {
		return err
	}
	defer releaseSplit()

	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	if err := s.loadRegistryLocked(); err != nil {
		return err
	}
	for i, lib := range s.libs {
		if lib.ID == libraryID {
			index = i
			break
		}
	}
	if index < 0 {
		return errAssetLibraryNotFound
	}

	// 关闭连接（必须在删文件前：WAL 伴生文件在连接正常关闭时才收口）
	if store, ok := s.open[libraryID]; ok {
		if err := store.Close(); err != nil {
			return fmt.Errorf("关闭素材索引库失败: %w", err)
		}
		delete(s.open, libraryID)
	}
	// 删库文件（含 WAL/SHM 伴生文件；不存在忽略——从未扫描的库可能无 db 文件）。
	// 已知瞬态（独立审核遗留项 3 处置口径）：缩略图记账 flusher 持库连接事务中
	// （45s 周期 × 毫秒级单事务）并发删库时，Windows 上可能撞句柄致本次删除
	// 瞬态失败；删除可重试即可恢复，窗口极窄，不加锁不改链路（与 flusher 互斥
	// 会把删库阻塞在 flush 时长上，收益不成比例）。
	dbPath := s.libraryDBPath(libraryID)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除素材索引库文件失败: %w", err)
		}
	}
	// 删缩略图目录（库已删，目录该删；失败仅告警不阻断注册表删除——db 已删
	// 不会再生，残留目录可手动清理）
	if err := os.RemoveAll(s.assetThumbsDir(libraryID)); err != nil {
		Logger().Warn("删除缩略图目录失败（残留可手动清理）", "libraryId", libraryID, "err", err)
	}
	// 删拆分产物目录（derived 镜像源树；同缩略图目录口径：失败仅告警不阻断，
	// 残留可手动清理）
	if err := os.RemoveAll(s.assetDerivedDir(libraryID)); err != nil {
		Logger().Warn("删除拆分产物目录失败（残留可手动清理）", "libraryId", libraryID, "err", err)
	}
	// 清记账 dirty（防 flusher 经 storeForThumbs 惰性重建孤儿 db 的第二道收口）
	s.thumbLedger.dropLibrary(libraryID)

	s.libs = append(s.libs[:index], s.libs[index+1:]...)
	return s.saveRegistryLocked()
}

// GetLibraryStats 库统计：文件/幻灯片总数与失败/加密跳过数（库徽标与看板用）。
func (s *AssetLibraryService) GetLibraryStats(libraryID string) (model.AssetLibraryStats, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetLibraryStats{}, err
	}
	var stats model.AssetLibraryStats
	err := s.withStore(libraryID, func(store *assetsStore) error {
		fileCount, slideCount, failedCount, encryptedCount, err := store.libraryStats()
		if err != nil {
			return err
		}
		stats = model.AssetLibraryStats{
			LibraryID:      libraryID,
			FileCount:      fileCount,
			SlideCount:     slideCount,
			FailedCount:    failedCount,
			EncryptedCount: encryptedCount,
		}
		return nil
	})
	if err != nil {
		return model.AssetLibraryStats{}, err
	}
	return stats, nil
}

// ListFiles 文件索引条目（含标签），按相对路径序。
func (s *AssetLibraryService) ListFiles(libraryID string) ([]model.AssetFileEntry, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return nil, err
	}
	var entries []model.AssetFileEntry
	err := s.withStore(libraryID, func(store *assetsStore) (err error) {
		entries, err = store.listFileEntries()
		return err
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// ListSlides 幻灯片索引条目分页（标签继承归属文件），按文件路径 + 页序。
// 分页契约对齐 SearchAssets：limit ≤0 取默认 50（上限 200）、offset <0 按 0
// 处理，Total 为总条目数（不随页变化），翻页以 offset 递增 limit 驱动。
func (s *AssetLibraryService) ListSlides(libraryID string, limit, offset int) (model.AssetSlidePage, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return model.AssetSlidePage{}, err
	}
	if limit <= 0 {
		limit = assetSearchDefaultLimit
	}
	if limit > assetSearchMaxLimit {
		limit = assetSearchMaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	var page model.AssetSlidePage
	err := s.withStore(libraryID, func(store *assetsStore) (err error) {
		page, err = store.listSlideEntries(limit, offset)
		return err
	})
	if err != nil {
		return model.AssetSlidePage{}, err
	}
	return page, nil
}

// UpdateFileMeta 文件级元数据编辑（整体替换语义）：星级 0-5（0 = 清除）、备注、
// 标签集（整体替换；不存在的标签自动创建并按色板分配颜色）。
func (s *AssetLibraryService) UpdateFileMeta(libraryID string, fileID int64, meta model.AssetFileMetaUpdate) error {
	if meta.Rating < 0 || meta.Rating > 5 {
		return fmt.Errorf("星级取值须在 0-5 之间（0 = 清除评分），当前：%d", meta.Rating)
	}
	if _, err := s.getLibrary(libraryID); err != nil {
		return err
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	return s.withStore(libraryID, func(store *assetsStore) error {
		return store.updateFileMeta(fileID, meta.Rating, meta.Note, meta.Tags, now)
	})
}

// UpdateSlideMeta 幻灯片页级标注编辑（整体替换语义，对齐 UpdateFileMeta）：
// 业务备注/星级/页自有标签集（不存在的标签自动创建并按色板分配颜色）。
// 纯标注层：不触发重解析与缩略图失效，业务备注不入 FTS（SRS §2.2.4）。
func (s *AssetLibraryService) UpdateSlideMeta(libraryID string, fileID int64, slideNum int, meta model.AssetSlideMetaUpdate) error {
	if meta.Rating < 0 || meta.Rating > 5 {
		return fmt.Errorf("星级取值须在 0-5 之间（0 = 清除评分），当前：%d", meta.Rating)
	}
	if slideNum < 1 {
		return fmt.Errorf("页序须从 1 计，当前：%d", slideNum)
	}
	if _, err := s.getLibrary(libraryID); err != nil {
		return err
	}
	return s.withStore(libraryID, func(store *assetsStore) error {
		return store.updateSlideMeta(fileID, int64(slideNum), meta.Rating, meta.Note, meta.TagNames)
	})
}

// WaitScansDone 通知全部在途扫描取消并等待其退出（App.shutdown 关停收口，先于
// CloseAll 调用）：逐个触发取消注册表中的 CancelFunc（扫描在文件间检查点感知
// ctx 取消后收尾），再等扫描计数归零。没有本收口时，CloseAll 先关库连接、在途
// 扫描继续写已关连接，产生成片「sql: database is closed」噪音日志。
// 单文件解析内部无取消检查点，等待时长以其为上界（解析有界：zip 上限 + 单页
// 流式解析，秒级）；无在途扫描时立即返回。
func (s *AssetLibraryService) WaitScansDone() {
	s.opMu.Lock()
	entries := make([]*cancelEntry, 0, len(s.cancels))
	for _, entry := range s.cancels {
		entries = append(entries, entry)
	}
	s.opMu.Unlock()
	for _, entry := range entries {
		entry.cancel()
	}
	s.scanWG.Wait()
}

// CancelAllSplits 取消全部在途拆分（App.shutdown 关停收口，先于 WaitSplitsDone）：
// 快照拆分取消注册表逐个触发 CancelFunc（拆分在文件间检查点感知 ctx 取消后收尾，
// 等待可收敛）。与 WaitScansDone 分立：拆分注册表（splitCancels）键空间独立，
// CancelScan 不得误杀拆分，关停收口亦各自归属；只取消不等待，等待由
// WaitSplitsDone 承担（取消先于等待，对齐 WaitScansDone 内部两步顺序）。
func (s *AssetLibraryService) CancelAllSplits() {
	s.opMu.Lock()
	entries := make([]*cancelEntry, 0, len(s.splitCancels))
	for _, entry := range s.splitCancels {
		entries = append(entries, entry)
	}
	s.opMu.Unlock()
	for _, entry := range entries {
		entry.cancel()
	}
}

// WaitSplitsDone 等待全部在途拆分退出（App.shutdown 关停收口，CancelAllSplits
// 之后、CloseAll 之前调用）：无在途拆分立即返回；等待上界 = 剩余单文件拆分时长
// （取消检查点在文件间、单文件内不中断——与 WaitScansDone 的单文件解析上界同口径）。
func (s *AssetLibraryService) WaitSplitsDone() {
	s.splitWG.Wait()
}

// CloseAll 关闭全部已打开的库连接（App.shutdown 关停期调用；库删除路径经
// DeleteLibrary 单独收口）。调用方须先经 WaitScansDone 排空在途扫描、经
// CancelAllSplits + WaitSplitsDone 排空在途拆分，防扫描/拆分写已关连接。
func (s *AssetLibraryService) CloseAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, store := range s.open {
		if err := store.Close(); err != nil {
			Logger().Warn("close assets store failed", "libraryId", id, "err", err)
		}
	}
	s.open = make(map[string]*assetsStore)
}

// withStore 在锁内取（懒打开）库连接并执行 fn。连接缓存命中零开销。fn 执行期间
// 连接不会被 DeleteLibrary 关闭，保证来自 s.mu 全程互斥（本方法与 DeleteLibrary
// 的关连接段同锁串行）；长时间持连接的扫描另靠 tryLockScan 与删库互斥（不适用
// 本方法的短查询消费者，如拆分记录查询/打开单页）。
func (s *AssetLibraryService) withStore(libraryID string, fn func(store *assetsStore) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	store, err := s.storeForLocked(libraryID)
	if err != nil {
		return err
	}
	return fn(store)
}

// storeForLocked 取（懒打开）库连接缓存，须持 s.mu。打开失败不缓存（下次重试）。
func (s *AssetLibraryService) storeForLocked(libraryID string) (*assetsStore, error) {
	if store, ok := s.open[libraryID]; ok {
		return store, nil
	}
	store, err := openAssetsStore(s.libraryDBPath(libraryID))
	if err != nil {
		return nil, err
	}
	s.open[libraryID] = store
	return store, nil
}

// libraryDBPath 库 db 文件路径（data/assets/<library-id>.db）。
func (s *AssetLibraryService) libraryDBPath(libraryID string) string {
	return filepath.Join(s.assetsDir, libraryID+".db")
}

// registryPath 素材库注册表文件路径（data/assets/libraries.json）。
func (s *AssetLibraryService) registryPath() string {
	return filepath.Join(s.assetsDir, "libraries.json")
}

// loadRegistryLocked 从磁盘懒加载注册表（文件不存在 = 首次使用，空注册表），
// 须持 s.mu。
func (s *AssetLibraryService) loadRegistryLocked() error {
	if s.loaded {
		return nil
	}
	var reg assetLibraryRegistryJSON
	if err := util.LoadJSON(s.registryPath(), &reg); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("读取素材库注册表失败: %w", err)
		}
		// 首次使用：注册表文件尚不存在，空注册表
	}
	if reg.Libraries == nil {
		reg.Libraries = []model.AssetLibrary{}
	}
	s.libs = reg.Libraries
	s.loaded = true
	return nil
}

// saveRegistryLocked 注册表原子落盘（util.SaveJSON：temp → fsync → rename，
// docs/spec/data-atomic-write.md 契约，禁 os.WriteFile 直写），须持 s.mu。
func (s *AssetLibraryService) saveRegistryLocked() error {
	if err := util.SaveJSON(s.registryPath(), assetLibraryRegistryJSON{Libraries: s.libs}); err != nil {
		return fmt.Errorf("保存素材库注册表失败: %w", err)
	}
	return nil
}

// newAssetLibraryID 生成库唯一标识（16 位随机 hex）：同时用作 db 文件名，
// 无时序语义、无重合风险。
func newAssetLibraryID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand 失败极罕见（系统熵源不可用）：退化用纳秒时间戳兜底
		return fmt.Sprintf("lib%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// tryLockScan 尝试获取目标库的扫描互斥锁（TryLock 语义，锁被占用立即返回
// E_ASSETS_SCAN_IN_PROGRESS，不等待不排队）。成功返回 release 闭包，调用方须
// defer 调用。键经 normalizeRepoKey 规范化，与取消注册表同键口径。
// DeleteLibrary 复用本锁与在途扫描互斥（扫描中禁删库）。
func (s *AssetLibraryService) tryLockScan(libraryID string) (release func(), err error) {
	key := normalizeRepoKey(libraryID)

	s.opMu.Lock()
	mu, ok := s.opLocks[key]
	if !ok {
		mu = &sync.Mutex{}
		if s.opLocks == nil {
			s.opLocks = make(map[string]*sync.Mutex)
		}
		s.opLocks[key] = mu
	}
	s.opMu.Unlock()

	if !mu.TryLock() {
		return nil, model.NewAppError(model.ErrCodeAssetsScanInProgress,
			"该素材库正在扫描中，请等待当前扫描结束后再试")
	}
	return mu.Unlock, nil
}

// tryLockSplit 尝试获取目标库的拆分互斥锁（TryLock 语义，对齐 tryLockScan 模式；
// 独立 splitLocks map——拆分与扫描语义可并发，仅拆分自身互斥）。DeleteLibrary
// 复用本锁与在途拆分互斥（拆分中禁删库）。错误码复用 E_ASSETS_SCAN_IN_PROGRESS
// （素材域「操作进行中」既有一类，前端 warning 分流同款），消息按拆分措辞区分。
func (s *AssetLibraryService) tryLockSplit(libraryID string) (release func(), err error) {
	key := normalizeRepoKey(libraryID)

	s.opMu.Lock()
	mu, ok := s.splitLocks[key]
	if !ok {
		mu = &sync.Mutex{}
		if s.splitLocks == nil {
			s.splitLocks = make(map[string]*sync.Mutex)
		}
		s.splitLocks[key] = mu
	}
	s.opMu.Unlock()

	if !mu.TryLock() {
		return nil, model.NewAppError(model.ErrCodeAssetsScanInProgress,
			"该素材库正在拆分中，请等待当前拆分结束后再试")
	}
	return mu.Unlock, nil
}

// registerSplitCancel 登记在途拆分的可取消入口（独立 splitCancels 注册表——
// CancelScan 不得误杀拆分、CancelAssetSplit 亦不影响扫描；对齐 registerCancel：
// 同键覆盖写 + 注销按条目指针比对防误删）。
func (s *AssetLibraryService) registerSplitCancel(libraryID string, cancel context.CancelFunc) *cancelEntry {
	key := normalizeRepoKey(libraryID)
	entry := &cancelEntry{cancel: cancel}
	s.opMu.Lock()
	if s.splitCancels == nil {
		s.splitCancels = make(map[string]*cancelEntry)
	}
	s.splitCancels[key] = entry
	s.opMu.Unlock()
	return entry
}

// unregisterSplitCancel 拆分结束（成功/失败/取消/panic 经 defer）注销注册表，
// 防 CancelFunc 残留（对齐 unregisterCancel：删前校验条目指针同一，同键覆盖后
// 先结束方不误删后注册项）。
func (s *AssetLibraryService) unregisterSplitCancel(libraryID string, entry *cancelEntry) {
	if entry == nil {
		return
	}
	key := normalizeRepoKey(libraryID)
	s.opMu.Lock()
	if s.splitCancels[key] == entry {
		delete(s.splitCancels, key)
	}
	s.opMu.Unlock()
}

// registerCancel 登记在途扫描的可取消入口（对齐 PptxSplitService.registerCancel：
// 同键覆盖写 + 注销按条目指针比对防误删）。
func (s *AssetLibraryService) registerCancel(libraryID string, cancel context.CancelFunc) *cancelEntry {
	key := normalizeRepoKey(libraryID)
	entry := &cancelEntry{cancel: cancel}
	s.opMu.Lock()
	if s.cancels == nil {
		s.cancels = make(map[string]*cancelEntry)
	}
	s.cancels[key] = entry
	s.opMu.Unlock()
	return entry
}

// unregisterCancel 扫描结束（成功/失败/取消/panic 经 defer）注销注册表，防
// CancelFunc 残留。删前校验条目指针同一（同键覆盖后先结束方不误删后注册项）。
func (s *AssetLibraryService) unregisterCancel(libraryID string, entry *cancelEntry) {
	if entry == nil {
		return
	}
	key := normalizeRepoKey(libraryID)
	s.opMu.Lock()
	if s.cancels[key] == entry {
		delete(s.cancels, key)
	}
	s.opMu.Unlock()
}

// CancelScan 取消指定库的在途扫描。返回是否存在可取消的在途扫描；无在途扫描
// 幂等返回 false（CancelFunc 幂等，取消与注销的竞态无危害）。
func (s *AssetLibraryService) CancelScan(libraryID string) bool {
	key := normalizeRepoKey(libraryID)
	s.opMu.Lock()
	entry, ok := s.cancels[key]
	if ok {
		delete(s.cancels, key)
	}
	s.opMu.Unlock()
	if !ok {
		return false
	}
	entry.cancel()
	return true
}
