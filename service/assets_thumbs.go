package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
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

// thumbJumpEnqueueTimeout 跳转入队提交超时：与单页导出最坏耗时对齐（HD 档复杂
// 页可超 5s，批量在途时页边界 preempt 是唯一消费点——提交窗过短会在发送方退队
// 后被 preempt 摘到 stale 请求静默执行，check P2）；结果等待仍由
// thumbJumpResultTimeout(90s) 兜底。
const thumbJumpEnqueueTimeout = 60 * time.Second

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
	sinkFn   func() EventSink  // 事件出口（复用宿主 AssetLibraryService 的 sinkHolder，防双出口漂移）
	ledger   *thumbCacheLedger // 记账 ledger（生成成功后记账 + 页数缩减对账，assets_thumbs_ledger.go）

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
// 取宿主 AssetLibraryService.eventSink，ledger 为记账出口）。
func newThumbQueue(exp ThumbExporter, sinkFn func() EventSink, ledger *thumbCacheLedger) *thumbQueue {
	return &thumbQueue{
		exporter: exp,
		sinkFn:   sinkFn,
		ledger:   ledger,
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

// run worker 主循环：每轮先非阻塞摘跳转（严格优先），再阻塞等批次/跳转/退出。
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
		case jr := <-q.jumpCh:
			// 停泊分支与 first select 同集消费 jumpCh：批次完成后 worker 停泊于
			// 本 select 的空闲窗口内，新跳转在此被消费——漏掉则空闲期跳转无人
			// 接收，JumpToSlide 撞入队超时报 BUSY（队列空闲却报忙的假态），且
			// 停泊期挂起的 stale 请求会被后续批量唤醒时误执行（check P0）。
			// first select 保留非阻塞摘取：批量在途时提前一轮摘走维持「跳转
			// 优先于批次」，空闲期跳转经任一分支到达均零额外延迟。
			q.handleJump(jr)
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
			// 记账 + 页数缩减对账（仅成功路径——失败文件旧图仍有效，内容失效
			// 已由扫描失效联动处理）。记账粒度：逐档逐页 Stat 成功才记账（PNG
			// 缺失的页留给下次补缺链路重导出）。
			if q.ledger != nil {
				for _, tier := range thumbTiers {
					for page := 1; page <= n; page++ {
						if info, statErr := os.Stat(thumbPNGPath(b.ThumbsDir, tier.Key, f.FileID, page)); statErr == nil {
							q.ledger.record(b.LibraryID, f.FileID, tier.Key, page, info.Size())
						}
					}
					// 页数缩减对账（20 页 → 15 页清越界记账与旧图；文件级导出
					// 全量覆盖写幂等，越界页不覆盖须显式清）
					q.ledger.pruneBeyond(b.LibraryID, b.ThumbsDir, f.FileID, tier.Key, n)
				}
			}
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
		// 队列满（异常形态：8 库并发扫描）：放弃入队并清理注册项，补发 error
		// 事件（载荷 {libraryId, message}，沿用 assets-scan:error 契约——前端不弹
		// 提示仅复位，防与调用方错误处理双弹）
		cancel()
		q.mu.Lock()
		if cur, ok := q.batches[libraryID]; ok && cur == b {
			delete(q.batches, libraryID)
		}
		q.mu.Unlock()
		Logger().Warn("缩略图批次入队被拒（队列满）", "libraryId", libraryID, "files", len(files))
		emitEvent(s.eventSink(), "assets-thumbs:error", map[string]any{
			"libraryId": libraryID, "message": "缩略图生成队列已满，本次批次未入队",
		})
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
	case <-time.After(thumbJumpEnqueueTimeout):
		// 排队超时 = worker 忙且页间插队窗口未及消费（瞬态可重试），按 busy
		// warning 分流——不可复用 E_ASSETS_THUMBS_UNAVAILABLE（语义为环境无
		// PowerPoint，前端会误触降级 UI）
		return model.NewAppError(model.ErrCodeAssetsThumbsBusy, "跳转请求排队超时，请稍后重试")
	}
	select {
	case err := <-jr.Result:
		// 出口分流（check P2）：COM 原始错误（文件不存在/损坏/Invoke 失败）包装
		// 引导级 AppError——裸 HRESULT 文案前端不可读；nil 与既有 AppError
		// （排队/结果超时、不可用）语义已定，原样返回。
		if err == nil {
			return nil
		}
		var appErr *model.AppError
		if errors.As(err, &appErr) {
			return err
		}
		return model.WrapAppError(model.ErrCodeAssetsThumbsJumpFailed,
			"打开 PPT 或跳转失败，请确认文件存在且未被破坏", err)
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
// 待处理/在途批次，等 worker 退出，再收口 exporter（杀自启 powerpnt 残留）与
// 记账 ledger（同步 flush 全库攒批——此刻库连接仍开着，CloseAll 在其后）。
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
	// 随 quit 丢弃；等待方永挂风险由 JumpToSlide 双超时兜底（thumbJumpEnqueueTimeout
	// 入队提交 + thumbJumpResultTimeout 结果等待）。
	close(q.quit)
	q.wg.Wait()
	q.exporter.Shutdown()
	// 记账 ledger 关停：flush 全部有 dirty 的库后停 flusher（幂等，重复调用
	// 直接返回）。必须在库连接 CloseAll 之前——flush 需要活连接。
	s.thumbLedger.Shutdown()
}

// assetThumbsDir 库缩略图根目录单点（data/assets/<library-id>/thumbs，双档子目录
// small/、hd/）。所有 thumbsDir 来源统一走本方法：队列任务、ledger dirFn、失效
// 联动删文件、读取 API，防路径口径漂移。
func (s *AssetLibraryService) assetThumbsDir(libraryID string) string {
	return filepath.Join(s.assetsDir, libraryID, "thumbs")
}

// GetAssetThumb 读取单页缩略图 PNG（base64 标准编码裸串）。
//
// tierIndex：0=small（画廊卡片）1=hd（详情大图），非法值返回错误；未生成/已淘汰
// 返回 ("", nil)——非错误，前端回退占位卡。命中即 ledger.record 记账 touch（LRU
// 新鲜度，攒批落盘零同步写）。
//
// PR3 前端消费契约：返回值直接拼 data URL（`data:image/png;base64,<返回串>`），
// 禁 decodeBase64Utf8（文本专用，PNG 二进制会被双重编码破坏）禁裸 atob。
//
// 量级口径（PR3 接入约束）：hd 档 1600px 单张约 1-3MB、base64 再 +33%，经 Wails
// JSON 桥单次传输数 MB 级——仅详情大图单张按需调用；画廊卡片一律 small 档
// （320px 约 30-150KB），分页 50 张懒加载单屏峰值约 5MB，禁画廊直拉 hd 档。
func (s *AssetLibraryService) GetAssetThumb(libraryID string, fileID int64, slideNum, tierIndex int) (string, error) {
	if _, err := s.getLibrary(libraryID); err != nil {
		return "", err
	}
	if tierIndex < 0 || tierIndex >= len(thumbTiers) {
		return "", fmt.Errorf("缩略图档位非法（tierIndex=%d，取值 0-%d）", tierIndex, len(thumbTiers)-1)
	}
	if slideNum < 1 {
		return "", fmt.Errorf("页序非法（slideNum=%d，从 1 计）", slideNum)
	}
	if fileID < 1 {
		return "", fmt.Errorf("文件 ID 非法（fileId=%d）", fileID)
	}
	path := thumbPNGPath(s.assetThumbsDir(libraryID), thumbTiers[tierIndex].Key, fileID, slideNum)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // 未生成/已淘汰：前端回退占位卡
		}
		return "", fmt.Errorf("读取缩略图失败: %w", err)
	}
	s.thumbLedger.record(libraryID, fileID, thumbTiers[tierIndex].Key, slideNum, int64(len(data)))
	return base64.StdEncoding.EncodeToString(data), nil
}
