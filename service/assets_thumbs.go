package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"workbench/model"
)

// 素材缩略图链路（素材管理第二批，SRS §2.2.2/§2.2.5）：PowerPoint COM 串行队列
// + 双档缩略图导出 + 打开跳页。本文件为平台无关的队列调度与事件编排；
// COM 实现见 assets_thumbs_windows.go（go-ole）/ assets_thumbs_other.go（桩）。
//
// 并发模型（SRS 硬约束：全局同一时刻最多 1 个 COM 任务）：
//   - 单 worker goroutine 消费 batchCh/jumpCh，缩略图批次与跳转请求串行执行；
//   - 跳转高优先级：worker 每轮先非阻塞摘 jumpCh（无批量在途时立即执行）；
//     批量在途时经 ExportPresentation 的 preempt 回调在页间摘取跳转请求
//     （同 COM 会话内执行，最多等一页导出时间——PRD D3）；
//   - 失败隔离：单文件导出失败（含超时强杀）经 exporter.Discard() 丢弃 COM
//     会话后继续后续文件，不中断批次（SRS「重启实例继续后续任务」）；
//   - 取消：CancelAssetThumbs(libraryID) 取消该库在途/待处理批次（ctx 传播，
//     超时强杀路径由 exporter 内部处理）；对齐 E_ASSETS_CANCELLED warning 语义。
//
// 事件（对齐 assets-scan:* 契约）：assets-thumbs:progress / done / error 载荷
// 带 libraryId，前端按库过滤；done 仅复位进度态不刷新数据（数据刷新由前端
// 在 done 后按需拉取缩略图），error 只复位不弹提示（防与 Promise 双弹）。

// thumbTier 缩略图分辨率档位（缓存子目录名 + 目标像素尺寸）。
// Key 为普通 string 字段非具名类型（models.ts 别名坑——本类型不跨层，防御性遵守）。
type thumbTier struct {
	Key    string // 缓存子目录名（small / hd）
	Width  int    // 目标宽度像素（fit-inside 上限，实际输出保持幻灯片纵横比）
	Height int    // 目标高度像素
}

// thumbTiers 双档分辨率默认值（PRD D2：可配置——设置项落地前恒定值，设置链路
// 在前端 PR3 接入时改为从设置读取注入）。
var thumbTiers = []thumbTier{
	{Key: "small", Width: 320, Height: 180},
	{Key: "hd", Width: 1600, Height: 900},
}

// thumbPerFileTimeout 单文件导出超时（SRS §2.2.2 推荐 300s）。超时路径由
// exporter 实现处理（杀自启 powerpnt 让阻塞 Invoke 返回），队列层 ctx 传播。
var thumbPerFileTimeout = 300 * time.Second

// thumbJumpResultTimeout 跳转结果等待上限（队列层兜底）：Windows 实现内部
// jumpTimeout(60s) + 杀进程收尾宽限(10s) 内正常返回，此上限仅兜 worker 永挂
// 场景（复用用户实例被模态弹窗阻塞且不可杀），防 JumpToSlide 前端 RPC 永挂。
const thumbJumpResultTimeout = 90 * time.Second

// thumbFileSpec 批量任务中的单文件规格。
type thumbFileSpec struct {
	FileID  int64  `json:"fileId"`  // files.id（稳定，缓存键组成部分——slides.id 禁用）
	Path    string `json:"path"`    // 相对库根目录路径（事件展示）
	AbsPath string `json:"absPath"` // 绝对路径（COM 打开用）
}

// thumbBatchTask 一次批量生成任务（粒度 = 库批次：扫描/重建完成后一批）。
// 文件级超时与失败隔离在批次内逐文件执行。
type thumbBatchTask struct {
	LibraryID string
	Files     []thumbFileSpec
	ThumbsDir string // 缩略图根目录 data/assets/<lib-id>/thumbs
	Ctx       context.Context
	Cancel    context.CancelFunc
}

// thumbJumpRequest 打开跳页请求（同步回传：result 由调用方等待，exporter 写入）。
type thumbJumpRequest struct {
	AbsPath  string     // PPT 绝对路径
	SlideNum int        // 目标页序（1-based）
	Result   chan error // 缓冲 1，exporter 完成后写回
}

// ThumbExporter PowerPoint COM 能力抽象（PRD PR1 接口定案）。生产实现：
// Windows = goOleThumbExporter（go-ole，assets_thumbs_windows.go）；非 Windows =
// 桩（assets_thumbs_other.go，Available 恒 false）；单测 = fake 注入
// （assets_thumbs_test.go，CI 零 PowerPoint 依赖）。
//
// 实现约束（契约，实现体必须遵守）：
//   - 全部 COM 调用须在专用 STA goroutine 内（LockOSThread + CoInitializeEx
//     STA 放行 S_FALSE），实现内部自管会话生命周期；
//   - ctx 取消/超时不可中断阻塞 Invoke——实现须以「杀自启 powerpnt 进程」
//     （进程快照 diff 归属判定，禁 taskkill /IM 全杀）让 Invoke 以
//     0x800706BA 返回后收尾；
//   - ExportPresentation 打开恒 ReadOnly + WithWindow:=msoFalse（导出场景）；
//     JumpOnly/JumpInSession 打开 WithWindow:=msoTrue + View.GotoSlide +
//     Activate（跳转场景）——两场景 Open 参数不同，禁共用封装。
type ThumbExporter interface {
	// Available 探测本机 PowerPoint COM 可用性（实现内部缓存探测结果；
	// 注册表预检 + CreateInstance 兜底两阶段）。
	Available() bool
	// ExportPresentation 打开 pptx（ReadOnly 无窗口）逐页导出双档 PNG 到
	// thumbsDir/<tier.Key>/<fileID>-<3位页>.png。返回成功导出的页数。
	// 每页（含双档）导完后调用 preempt：非 nil 返回 = 有跳转请求插队，
	// 实现须在同一 COM 会话内执行跳转（JumpInSession 语义）并写回 jr.Result，
	// 之后继续导出下一页（用户窗口与后台导图并存，PRD D3）。
	ExportPresentation(ctx context.Context, spec thumbFileSpec, thumbsDir string, tiers []thumbTier, preempt func() *thumbJumpRequest) (int, error)
	// JumpOnly 独立跳转（无批量在途时 worker 直调）：打开 + 跳页 + 置前，
	// 结果写回 jr.Result（成功与失败均恰一次——调用方 handleJump 不得二次写，
	// Result 缓冲 1 且仅 JumpToSlide 单读者，双写将永久阻塞队列 worker）。
	// Presentation 不关闭（留给用户浏览）。
	JumpOnly(jr *thumbJumpRequest) error
	// Discard 失败隔离：丢弃当前 COM 会话（断连/损坏态），下一次调用重建实例。
	// 仅会话级清理，不做进程强杀（强杀只在超时/取消路径由实现内部执行）。
	Discard()
	// Shutdown 关停收口：结束会话并清理自启 powerpnt 残留进程（App.shutdown
	// 调用，对齐 WaitScansDone 收口模式）。
	Shutdown()
}

// errThumbsUnavailable 本机无 PowerPoint COM（前端 D4 降级：提示条 + 入口隐藏；
// 后端防御返回，前端 error.js 同步 E_ASSETS_THUMBS_UNAVAILABLE 分流）。
var errThumbsUnavailable = model.NewAppError(model.ErrCodeAssetsThumbsUnavailable,
	"未检测到 PowerPoint，缩略图与打开跳转不可用（仅文本索引模式）")

// thumbQueue 缩略图串行队列（全局单实例，挂 AssetLibraryService 生命周期）。
type thumbQueue struct {
	exporter ThumbExporter
	sinkFn   func() EventSink // 事件出口（复用宿主 AssetLibraryService 的 sinkHolder，防双出口漂移）

	batchCh chan *thumbBatchTask   // 容量 thumbBatchQueueCap，多库并发入队不阻塞
	jumpCh  chan *thumbJumpRequest // 无缓冲：交接即被消费（worker 空闲直收 / 批量在途由 preempt 页间摘取），提交阻塞等消费方消除时序竞态
	quit    chan struct{}
	wg      sync.WaitGroup // worker 计数（WaitThumbsDone 关停收口）

	mu      sync.Mutex
	batches map[string]*thumbBatchTask // libraryID -> 最新批次（pending/active，取消入口）
	started bool
	closed  bool // WaitThumbsDone 后置位，入队直接拒绝
}

// thumbBatchQueueCap 并发库数量级上限（超出视为异常，入队拒绝）。
const thumbBatchQueueCap = 8

// newThumbQueue 构造队列（exporter 由 newThumbExporter 平台分叉注入，sinkFn
// 取宿主 AssetLibraryService.eventSink）。
func newThumbQueue(exp ThumbExporter, sinkFn func() EventSink) *thumbQueue {
	return &thumbQueue{
		exporter: exp,
		sinkFn:   sinkFn,
		batchCh:  make(chan *thumbBatchTask, thumbBatchQueueCap),
		jumpCh:   make(chan *thumbJumpRequest),
		quit:     make(chan struct{}),
		batches:  make(map[string]*thumbBatchTask),
	}
}

// start 惰性启动 worker（首次入队时触发，构造保持纯 new——app-services-assembly
// 契约「构造纯 new，生命周期副作用留 startup/shutdown」的折中：worker 常驻
// goroutine 为队列结构内在资源，惰性启动等价懒打开连接模式）。
func (q *thumbQueue) start() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.started || q.closed {
		return
	}
	q.started = true
	q.wg.Add(1)
	go q.run()
}

// run worker 主循环：每轮先非阻塞摘跳转（严格优先），再阻塞等批次/退出。
// 批量在途期间的跳转由 handleBatch 内 preempt 回调承接（页间插队）。
func (q *thumbQueue) run() {
	defer q.wg.Done()
	for {
		select {
		case jr := <-q.jumpCh:
			q.handleJump(jr)
			continue
		default:
		}
		select {
		case b := <-q.batchCh:
			q.handleBatch(b)
		case <-q.quit:
			return
		}
	}
}

// handleJump 执行独立跳转（无批量在途路径）。jr.Result 由 JumpOnly 契约写回
// （成功/失败均恰一次），本方法不得二次写——Result 缓冲 1 且仅 JumpToSlide
// 单读者，双写会让 worker 永久阻塞在此（整个队列停摆）。
func (q *thumbQueue) handleJump(jr *thumbJumpRequest) {
	if !q.exporter.Available() {
		jr.Result <- errThumbsUnavailable
		return
	}
	if err := q.exporter.JumpOnly(jr); err != nil {
		Logger().Warn("缩略图跳转失败", "path", jr.AbsPath, "slide", jr.SlideNum, "err", err)
	}
}

// handleBatch 执行一批缩略图生成：逐文件（单文件超时 + 失败隔离），逐文件发
// progress 事件，批次终态发 done 事件。取消路径 done.Canceled=true 且中断剩余。
func (q *thumbQueue) handleBatch(b *thumbBatchTask) {
	defer func() {
		q.mu.Lock()
		// 仅清理仍指向本批次的注册项（期间同库新批次已覆盖时不误删）
		if cur, ok := q.batches[b.LibraryID]; ok && cur == b {
			delete(q.batches, b.LibraryID)
		}
		q.mu.Unlock()
	}()

	if !q.exporter.Available() {
		// 入队后 PowerPoint 变为不可用（卸载/COM 损坏）：整批跳过，事件终态
		emitEvent(q.sinkFn(), "assets-thumbs:done", model.AssetThumbsResult{
			LibraryID: b.LibraryID, Total: len(b.Files), Skipped: len(b.Files),
		})
		return
	}

	preempt := func() *thumbJumpRequest {
		select {
		case jr := <-q.jumpCh:
			return jr
		default:
			return nil
		}
	}

	generated, failed, skipped := 0, 0, 0
	canceled := false
	for i, f := range b.Files {
		if b.Ctx.Err() != nil {
			canceled = true
			break
		}
		fctx, cancel := context.WithTimeout(b.Ctx, thumbPerFileTimeout)
		n, err := q.exporter.ExportPresentation(fctx, f, b.ThumbsDir, thumbTiers, preempt)
		cancel()
		switch {
		case err == nil:
			generated++
			_ = n // 页数供日志与 PR2 记账使用
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			// 批次取消（用户/关停）：中断剩余文件；单文件超时（DeadlineExceeded
			// 且批次未取消）不应走到这里——exporter 超时自兜底返回非 ctx 错误
			if b.Ctx.Err() != nil {
				canceled = true
			} else {
				failed++
				q.exporter.Discard()
				Logger().Warn("缩略图生成超时", "path", f.Path, "err", err)
			}
		default:
			failed++
			q.exporter.Discard() // 失败隔离：丢会话重启实例继续后续（SRS §2.2.2）
			Logger().Warn("缩略图生成失败", "path", f.Path, "slides", n, "err", err)
		}
		emitEvent(q.sinkFn(), "assets-thumbs:progress", model.AssetThumbsProgress{
			LibraryID: b.LibraryID, Done: i + 1, Total: len(b.Files),
			CurrentFile: f.Path, Failed: failed,
		})
		if canceled {
			break
		}
	}

	Logger().Info("缩略图批次完成", "libraryId", b.LibraryID, "total", len(b.Files),
		"generated", generated, "failed", failed, "skipped", skipped, "canceled", canceled)
	emitEvent(q.sinkFn(), "assets-thumbs:done", model.AssetThumbsResult{
		LibraryID: b.LibraryID, Total: len(b.Files),
		Generated: generated, Skipped: skipped, Failed: failed, Canceled: canceled,
	})
}

// thumbPNGPath 缩略图缓存路径单点（契约：thumbsDir/<tier>/<fileID>-<3位页>.png；
// slides.id 每轮重解析全变禁作缓存键——files.id 稳定）。队列与 COM 实现共用，
// PR2 记账表以同规则反查。
func thumbPNGPath(thumbsDir, tierKey string, fileID int64, slideNum int) string {
	return filepath.Join(thumbsDir, tierKey, fmt.Sprintf("%d-%03d.png", fileID, slideNum))
}

// EnqueueAssetThumbs 入队一批缩略图生成（扫描/重建完成后由扫描链路调用——PR2
// 接线；增量补缺过滤在调用方完成，本方法全量入队）。
// 无 PowerPoint 静默跳过（D1 降级）；同库重复入队取消旧批次（新批次覆盖）。
func (s *AssetLibraryService) EnqueueAssetThumbs(libraryID string, files []thumbFileSpec, thumbsDir string) {
	if len(files) == 0 {
		return
	}
	q := s.thumbQ
	if !q.exporter.Available() {
		return // 无 COM 环境：仅文本索引，静默跳过（D1/D4 降级）
	}
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	if old, ok := q.batches[libraryID]; ok {
		old.Cancel() // 旧批次（pending 或在途）作废
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &thumbBatchTask{LibraryID: libraryID, Files: files, ThumbsDir: thumbsDir, Ctx: ctx, Cancel: cancel}
	q.batches[libraryID] = b
	q.mu.Unlock()

	q.start()
	select {
	case q.batchCh <- b:
	default:
		// 队列满（异常形态：8 库并发扫描）：放弃入队并清理注册项
		cancel()
		q.mu.Lock()
		if cur, ok := q.batches[libraryID]; ok && cur == b {
			delete(q.batches, libraryID)
		}
		q.mu.Unlock()
		Logger().Warn("缩略图批次入队被拒（队列满）", "libraryId", libraryID, "files", len(files))
	}
}

// ThumbsAvailable 探测本机 PowerPoint COM 可用性（App 绑定暴露给前端作 D4
// 降级依据；实现内部缓存探测结果）。
func (s *AssetLibraryService) ThumbsAvailable() bool {
	return s.thumbQ.exporter.Available()
}

// JumpToSlide 打开 PPT 并跳到指定页（SRS §2.2.5）。同步语义：等待执行结果返回
// （批量在途时经 preempt 页间插队执行，最多等一页导出时间——PRD D3）。
func (s *AssetLibraryService) JumpToSlide(absPath string, slideNum int) error {
	if !s.thumbQ.exporter.Available() {
		return errThumbsUnavailable
	}
	jr := &thumbJumpRequest{AbsPath: absPath, SlideNum: slideNum, Result: make(chan error, 1)}
	q := s.thumbQ
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return errThumbsUnavailable
	}
	q.mu.Unlock()
	q.start()
	select {
	case q.jumpCh <- jr:
	case <-time.After(5 * time.Second):
		// 排队超时 = worker 忙且页间插队窗口未及消费（瞬态可重试），按 busy
		// warning 分流——不可复用 E_ASSETS_THUMBS_UNAVAILABLE（语义为环境无
		// PowerPoint，前端会误触降级 UI）
		return model.NewAppError(model.ErrCodeAssetsThumbsBusy, "跳转请求排队超时，请稍后重试")
	}
	select {
	case err := <-jr.Result:
		return err
	case <-time.After(thumbJumpResultTimeout):
		// exporter 侧 jumpTimeout + 杀进程收尾宽限内正常返回；此为 worker 永挂
		// 兜底（复用用户实例被模态弹窗阻塞且不可杀），防前端 RPC 与 goroutine
		// 永久挂起。jr.Result 由在途任务稍后写入（缓冲 1，无泄漏）。
		return model.NewAppError(model.ErrCodeAssetsThumbsBusy, "跳转执行超时，请稍后重试")
	}
}

// CancelAssetThumbs 取消指定库的缩略图批次（在途批次经 ctx 传播中断——COM 阻塞
// 调用由 exporter 杀自启进程收口）。返回是否存在可取消批次。
func (s *AssetLibraryService) CancelAssetThumbs(libraryID string) bool {
	q := s.thumbQ
	q.mu.Lock()
	b, ok := q.batches[libraryID]
	if ok {
		delete(q.batches, libraryID)
	}
	q.mu.Unlock()
	if !ok {
		return false
	}
	b.Cancel()
	return true
}

// WaitThumbsDone 关停收口（App.shutdown 内 WaitScansDone 之后调用）：取消全部
// 待处理/在途批次，等 worker 退出，再收口 exporter（杀自启 powerpnt 残留）。
// 关闭后队列拒绝新任务（closed 标志）。
func (s *AssetLibraryService) WaitThumbsDone() {
	q := s.thumbQ
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	batches := make([]*thumbBatchTask, 0, len(q.batches))
	for _, b := range q.batches {
		batches = append(batches, b)
	}
	q.batches = make(map[string]*thumbBatchTask)
	q.mu.Unlock()

	for _, b := range batches {
		b.Cancel()
	}
	// jumpCh 中已排队的请求无法逐个回复（无注册表）——worker 退出前会消费或
	// 随 quit 丢弃；等待方永挂风险由 JumpToSlide 双超时兜底（5s 入队排队 +
	// 90s 结果等待）。
	close(q.quit)
	q.wg.Wait()
	q.exporter.Shutdown()
}
