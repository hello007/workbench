package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// PptxPreviewService pptx/ppsx 文件树预览转换编排服务（任务 10-20）：
// 文件树点击 .pptx/.ppsx 时经本机 PowerPoint/WPS COM 整体转换为 PDF，前端复用
// 现成 pdfjs viewer（kind=pdf iframe 链路）高保真预览。本服务只负责编排：
//
//   - 缓存索引 + LRU 淘汰：缓存目录 data/pptx_preview_pdf/，产物文件名
//     <源路径哈希>-<size>-<mtime纳秒>.pdf（缓存键 = srcPath + size + mtime，
//     源文件任一变化即新键重新转换，旧产物留待 LRU 淘汰）；sidecar 索引
//     index.json 走 util.SaveJSON 原子写（data-atomic-write 契约，禁
//     os.WriteFile 截断写），损坏降级空索引 + slog.Warn 不阻塞调用链路；
//   - 在途防抖：同文件并发请求共享同一次转换（第二个调用等待第一个完成，
//     不重复排队；COM 队列本身单 worker 串行，防抖避免重复入队放大等待）；
//   - 瞬态失败重试 1 次（COM 层冷启动/忙态类失败；ctx 已取消/超时后不重试）；
//   - 超时 60s（context.WithTimeout 兜底；COM 阻塞调用不可中断，超时强杀
//     自启实例由 COM 执行器 waitFor 收口路径处理）。
//
// COM 执行经 PptxPdfConverter 接口注入（生产 = assets 缩略图同一
// goOleThumbExporter STA 队列实例——单队列串行避免双 COM 队列进程互杀；
// 测试 = fake 注入），本服务无自有 worker goroutine。

// PptxPdfConverter PPTX→PDF 转换执行器抽象。生产实现 = assets 缩略图的
// goOleThumbExporter（Windows，转换任务排进同一 STA worker 单队列）/
// unavailableThumbExporter（非 Windows 桩，Available 恒 false 驱动降级）。
type PptxPdfConverter interface {
	// Available 探测本机 Office COM 可用性（实现内部缓存探测结果）。
	Available() bool
	// ConvertToPdf 把 srcPath 的演示文稿整体转换为 PDF 落到 dstPath。
	// 实现契约：src/dst 必须绝对路径（COM 服务器进程按自身 cwd 解析路径参数，
	// 相对路径 = 确定性失败——assets-thumbs 历史 P0）；dst 已存在时静默覆盖；
	// 转换成功保证 dst 存在且非空。ctx 取消/超时由实现经「杀自启 COM 进程」
	// 收口（阻塞 Invoke 不可中断）。
	ConvertToPdf(ctx context.Context, srcPath, dstPath string) error
}

// pptxConvertTimeout 单文件转换超时（复杂大文件 SaveAs 可达数十秒，60s 与
// PRD 一致；var 注入点，测试无需真实等待——fake 转换器即时返回）。
var pptxConvertTimeout = 60 * time.Second

// pptxPreviewCacheLimitBytes 缓存 LRU 上限 512MB（硬编码，后续再暴露设置；
// 按 index 记账的产物字节总和判定，超限按 lastAccess 淘汰最旧）。
const pptxPreviewCacheLimitBytes int64 = 512 << 20

// PptxPreviewService 构造字段与并发状态（mu 保护 entries / inFlight / loaded）。
type PptxPreviewService struct {
	conv      PptxPdfConverter
	cacheDir  string
	indexPath string

	mu       sync.Mutex
	loaded   bool
	entries  map[string]pptxPreviewEntry
	inFlight map[string]*pptxConvertCall
}

// pptxPreviewEntry 索引条目（index.json 单行）。
type pptxPreviewEntry struct {
	SrcPath    string `json:"srcPath"`
	Size       int64  `json:"size"`
	MtimeNs    int64  `json:"mtimeNs"`
	LastAccess int64  `json:"lastAccessNs"` // 应用层自记时间戳（禁依赖文件系统 atime/mtime 做访问序，对齐 thumb_cache LRU 口径）
	PdfName    string `json:"pdfName"`
	PdfSize    int64  `json:"pdfSize"`
}

// pptxPreviewIndexJSON index.json 顶层结构（对象包装便于后续扩展伴随字段，
// 对齐 assetCartJSON 先例）。
type pptxPreviewIndexJSON struct {
	Entries map[string]pptxPreviewEntry `json:"entries"`
}

// pptxConvertCall 在途转换共享句柄：leader 执行转换，follower 等待 done。
type pptxConvertCall struct {
	done chan pptxConvertResult // 缓冲 1：leader 恰一次写入，无泄漏
}

// pptxConvertResult 转换结果（成功带 pdfPath，失败带 err）。
type pptxConvertResult struct {
	pdfPath string
	err     error
}

// NewPptxPreviewService 构造转换编排服务（cacheDir 通常为
// <dataDir>/pptx_preview_pdf；conv 为 COM 执行器，见 PptxPdfConverter 注释）。
func NewPptxPreviewService(cacheDir string, conv PptxPdfConverter) *PptxPreviewService {
	return &PptxPreviewService{
		conv:      conv,
		cacheDir:  cacheDir,
		indexPath: filepath.Join(cacheDir, "index.json"),
		entries:   map[string]pptxPreviewEntry{},
		inFlight:  map[string]*pptxConvertCall{},
	}
}

// ConvertForPreview 把 srcPath（.pptx/.ppsx 绝对路径）转换为 PDF 并返回缓存
// PDF 绝对路径。缓存命中（索引存在 + PDF 文件存在）直接返回，无 COM 延迟；
// 未命中执行转换（在途防抖共享同一次）。
//
// 错误分流（前端保留降级提示 + 「用默认程序打开」按钮，error 级走 handleError
// 正常分流）：
//   - E_PPTX_PREVIEW_UNAVAILABLE：环境不可用（非 Windows / 未检测到 Office COM）
//   - E_PPTX_PREVIEW_FAILED：转换失败（文件损坏/COM 调用失败/超时，重试 1 次后仍失败）
func (s *PptxPreviewService) ConvertForPreview(ctx context.Context, srcPath string) (string, error) {
	if s.conv == nil || !s.conv.Available() {
		return "", model.NewAppError(model.ErrCodePptxPreviewUnavailable,
			"未检测到 PowerPoint/WPS，无法内嵌预览 PPT 文件")
	}
	abs, err := filepath.Abs(srcPath)
	if err != nil {
		return "", model.WrapAppError(model.ErrCodePptxPreviewFailed, "解析文件路径失败", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", model.WrapAppError(model.ErrCodePptxPreviewFailed, "源文件不存在或无法访问", err)
	}
	if info.IsDir() {
		return "", model.NewAppError(model.ErrCodePptxPreviewFailed, "源路径是目录，无法转换预览")
	}
	key := pptxPreviewCacheKey(abs, info.Size(), info.ModTime().UnixNano())

	// 缓存命中：索引存在 && PDF 文件存在（键 = srcPath+size+mtime，命中当前键
	// 即键字段一致）。命中即 touch lastAccess 并落盘（LRU 新鲜度）。
	if path, ok := s.lookup(key, abs); ok {
		return path, nil
	}

	// 在途防抖：同键并发请求共享同一次转换（follower 等待 leader 完成）。
	s.mu.Lock()
	if call, ok := s.inFlight[key]; ok {
		s.mu.Unlock()
		select {
		case res := <-call.done:
			if res.err != nil {
				return "", res.err
			}
			return res.pdfPath, nil
		case <-ctx.Done():
			return "", model.WrapAppError(model.ErrCodePptxPreviewFailed, "转换等待被取消或超时", ctx.Err())
		}
	}
	call := &pptxConvertCall{done: make(chan pptxConvertResult, 1)}
	s.inFlight[key] = call
	s.mu.Unlock()

	res := s.convert(ctx, key, abs, info)
	// 结果广播（缓冲 1，恒成功写入）后清理注册项；follower 已持有 call 引用
	// 不受清理影响。
	call.done <- res
	s.mu.Lock()
	if cur, ok := s.inFlight[key]; ok && cur == call {
		delete(s.inFlight, key)
	}
	s.mu.Unlock()
	if res.err != nil {
		return "", res.err
	}
	return res.pdfPath, nil
}

// lookup 缓存查询与 touch（须持外判断，内部自取锁）。
func (s *PptxPreviewService) lookup(key, absPath string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return "", false
	}
	entry, ok := s.entries[key]
	if !ok || entry.SrcPath != absPath || entry.PdfName == "" {
		return "", false
	}
	path := filepath.Join(s.cacheDir, entry.PdfName)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		// 产物缺失（手动清理/盘异常）：索引项同步剔除，防永久死索引
		delete(s.entries, key)
		return "", false
	}
	entry.LastAccess = time.Now().UnixNano()
	s.entries[key] = entry
	if err := s.saveLocked(); err != nil {
		Logger().Warn("预览缓存索引落盘失败（不影响本次命中返回）", "path", s.indexPath, "err", err)
	}
	return path, true
}

// convert 执行一次转换（含超时兜底 + 瞬态重试 1 次 + 成功后记账 + LRU 淘汰）。
// 不持有服务锁（转换在锁外，仅记账/淘汰短持锁）。
func (s *PptxPreviewService) convert(ctx context.Context, key, absPath string, info os.FileInfo) pptxConvertResult {
	dst := filepath.Join(s.cacheDir, key+".pdf")
	// 缓存目录预建（COM 侧 SaveAs 不自建父目录——assets-thumbs P0 教训同源；
	// 编排层收口后 COM 实现与测试 fake 均可直接写产物）
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return pptxConvertResult{err: model.WrapAppError(model.ErrCodePptxPreviewFailed, "创建 PDF 缓存目录失败", err)}
	}
	cctx, cancel := context.WithTimeout(ctx, pptxConvertTimeout)
	defer cancel()

	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		if attempt > 1 {
			// ctx 已取消/超时后重试必然立即失败且掩盖根因——直接终止
			if ctxErr := cctx.Err(); ctxErr != nil {
				lastErr = ctxErr
				break
			}
		}
		// 先删旧产物：使「产物存在且非空」校验只对本次转换结果负责（SaveAs
		// 虽静默覆盖，但若本次调用失败而旧文件残留，校验会误判成功）
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			return pptxConvertResult{err: model.WrapAppError(model.ErrCodePptxPreviewFailed, "清理旧 PDF 产物失败", err)}
		}
		if err := s.conv.ConvertToPdf(cctx, absPath, dst); err != nil {
			lastErr = err
			// 瞬态失败重试 1 次（COM 冷启动/忙态类失败；判定简化为对全部
			// 转换错误重试一次——ctx 取消/超时由上方准入排除）
			Logger().Warn("PPTX 转 PDF 失败", "path", absPath, "attempt", attempt, "err", err)
			continue
		}
		pinfo, statErr := os.Stat(dst)
		if statErr != nil {
			lastErr = fmt.Errorf("转换后 PDF 产物缺失: %w", statErr)
			continue
		}
		if pinfo.Size() == 0 {
			lastErr = fmt.Errorf("转换后 PDF 产物为空文件")
			continue
		}
		// 记账 + LRU 淘汰（短持锁）
		s.mu.Lock()
		if err := s.loadLocked(); err == nil {
			s.entries[key] = pptxPreviewEntry{
				SrcPath:    absPath,
				Size:       info.Size(),
				MtimeNs:    info.ModTime().UnixNano(),
				LastAccess: time.Now().UnixNano(),
				PdfName:    key + ".pdf",
				PdfSize:    pinfo.Size(),
			}
			s.evictLocked()
			if err := s.saveLocked(); err != nil {
				Logger().Warn("预览缓存索引落盘失败（产物已生成，下次命中不受影响）",
					"path", s.indexPath, "err", err)
			}
		} else {
			Logger().Warn("预览缓存索引加载失败，本次产物不入索引", "path", s.indexPath, "err", err)
		}
		s.mu.Unlock()
		return pptxConvertResult{pdfPath: dst}
	}
	if errors.Is(lastErr, context.Canceled) || errors.Is(lastErr, context.DeadlineExceeded) {
		return pptxConvertResult{err: model.WrapAppError(model.ErrCodePptxPreviewFailed,
			"转换超时或已取消，请稍后重试", lastErr)}
	}
	return pptxConvertResult{err: model.WrapAppError(model.ErrCodePptxPreviewFailed,
		"PPT 转换 PDF 失败，请用默认程序打开查看", lastErr)}
}

// evictLocked LRU 淘汰：记账总大小超上限时按 lastAccess 最旧优先删除文件 +
// 索引项，直至回落到上限内。须持 s.mu。
func (s *PptxPreviewService) evictLocked() {
	var total int64
	for _, e := range s.entries {
		total += e.PdfSize
	}
	for total > pptxPreviewCacheLimitBytes && len(s.entries) > 0 {
		oldestKey := ""
		var oldest pptxPreviewEntry
		for k, e := range s.entries {
			if oldestKey == "" || e.LastAccess < oldest.LastAccess {
				oldestKey, oldest = k, e
			}
		}
		if oldestKey == "" {
			return
		}
		if err := os.Remove(filepath.Join(s.cacheDir, oldest.PdfName)); err != nil && !os.IsNotExist(err) {
			Logger().Warn("删除淘汰的预览缓存 PDF 失败", "path", oldest.PdfName, "err", err)
		}
		delete(s.entries, oldestKey)
		total -= oldest.PdfSize
	}
}

// loadLocked 懒加载索引（文件不存在 = 首次使用；损坏降级空索引 + 告警，不阻塞
// 调用链路，下一次写入即以空索引重建文件）。须持 s.mu。
func (s *PptxPreviewService) loadLocked() error {
	if s.loaded {
		return nil
	}
	if !util.FileExists(s.indexPath) {
		s.entries = map[string]pptxPreviewEntry{}
		s.loaded = true
		return nil
	}
	var data pptxPreviewIndexJSON
	if err := util.LoadJSON(s.indexPath, &data); err != nil {
		Logger().Warn("预览缓存索引读取失败，降级为空索引", "path", s.indexPath, "err", err)
		s.entries = map[string]pptxPreviewEntry{}
		s.loaded = true
		return nil
	}
	if data.Entries == nil {
		data.Entries = map[string]pptxPreviewEntry{}
	}
	s.entries = data.Entries
	s.loaded = true
	return nil
}

// saveLocked 原子落盘索引（util.SaveJSON：temp → fsync → rename，禁
// os.WriteFile 直写）。须持 s.mu。
func (s *PptxPreviewService) saveLocked() error {
	return util.SaveJSON(s.indexPath, pptxPreviewIndexJSON{Entries: s.entries})
}

// pptxPreviewCacheKey 缓存键单点：<源路径哈希>-<size>-<mtime纳秒>（哈希取
// SHA-256 前 16 hex 字符，防文件名非法字符与超长路径；同路径不同 size/mtime
// 产生新键，源变更即失效）。
func pptxPreviewCacheKey(absPath string, size int64, mtimeNs int64) string {
	sum := sha256.Sum256([]byte(absPath))
	return fmt.Sprintf("%s-%d-%d", hex.EncodeToString(sum[:8]), size, mtimeNs)
}

// PptxPdfConverter 暴露 assets 缩略图 COM 队列导出器作为 PPTX→PDF 转换执行器。
// 复用同一 STA worker 单队列（缩略图批次/跳转/转换串行执行），避免新建第二个
// COM 队列造成进程快照 diff 互杀（双队列各自 diff 归属判定会把对方的自启实例
// 判为「用户实例」禁杀，超时强杀能力整体失效——docs/spec/assets-thumbs-com.md
// 线程模型契约）。非 Windows 平台 exporter 桩未实现转换方法，类型断言落到
// unavailablePptxConverter 兜底（Available 恒 false 驱动降级）。
func (s *AssetLibraryService) PptxPdfConverter() PptxPdfConverter {
	if c, ok := s.thumbQ.exporter.(PptxPdfConverter); ok {
		return c
	}
	return unavailablePptxConverter{}
}

// unavailablePptxConverter 转换能力兜底桩（exporter 未实现 PptxPdfConverter 时
// 的降级出口）。
type unavailablePptxConverter struct{}

func (unavailablePptxConverter) Available() bool { return false }

func (unavailablePptxConverter) ConvertToPdf(_ context.Context, _, _ string) error {
	return model.NewAppError(model.ErrCodePptxPreviewUnavailable,
		"未检测到 PowerPoint/WPS，无法内嵌预览 PPT 文件")
}
