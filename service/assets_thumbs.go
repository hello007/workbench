package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// thumbTiers 双档分辨率默认值（PRD D2 设置链路落地后保留作默认值来源：队列
// tiersFn 未注入时的回退 + thumbTiersFromSettings 默认一致性校验基准）。
var thumbTiers = []thumbTier{
	{Key: "small", Width: 320, Height: 180},
	{Key: "hd", Width: 1600, Height: 900},
}

// comOwnedPIDsFileName 自启 Office 实例 PID 持久化文件名（data/assets/ 下）：
// 上会话崩溃（Shutdown 未跑）残留的幽灵实例跨会话归属自证依据（D7）——下会话
// 建 COM 会话时「存活进程 ∩ 盘上残留集非空」即判定自有残留并精确强杀后重建，
// 区分「用户已开 WPS/PPT」与「本应用上会话自启残留」（真机跳转零可见效果定性）。
const comOwnedPIDsFileName = "com_owned_pids.json"

// thumbTiersFromSettings 由设置构造双档分辨率（纯函数，可直测）：key 恒
// small/hd 固定顺序（GetAssetThumb 的 tierIndex 为序号非宽度，分辨率可配后
// 宽度漂移、序号稳定），宽度取设置值，高按 16:9 自算——320*9/16=180、
// 1600*9/16=900，与既有默认严格一致。
func thumbTiersFromSettings(cfg model.AssetThumbsSettings) []thumbTier {
	return []thumbTier{
		{Key: "small", Width: cfg.SmallWidth, Height: cfg.SmallWidth * 9 / 16},
		{Key: "hd", Width: cfg.HDWidth, Height: cfg.HDWidth * 9 / 16},
	}
}

// batchBacklogTiers 批量补缺判定使用的档位（单点，恒 small 档）。
//
// hd 档退出批量补缺（PRD R1）：hd 单张 1-3MB，全库体积远超默认配额（1024MB），
// LRU 淘汰后若参与补缺判定会形成「淘汰 → 整本重导 → 再淘汰」的永久循环。hd 改为
// 详情侧按需生成（RequestAssetThumb），批量链路只负责画廊 small 档。
// 返回 nil 表示档位集合异常（不含 small）——调用方据此跳过补缺，不误判。
func batchBacklogTiers(tiers []thumbTier) []thumbTier {
	for _, t := range tiers {
		if t.Key == "small" {
			return []thumbTier{t}
		}
	}
	return nil
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
//
// Tiers/Pages 为可选收窄。**Tiers 为 nil 的批量批次恒只导 small**（hd 已退出批量
// 链路，见 handleBatch 默认档位）——「全档全页」仅指 Pages 语义：
//   - Tiers 非 nil = 只导出指定档位（详情侧 hd 按需生成只传 hd）。
//   - Pages 非 nil = 只导出指定页（1-based），此时 handleBatch 跳过 pruneBeyond
//     ——未导出页的既有产物与记账必须保留，按返回页数裁剪会误删后续页。
//   - **Pages[0] 恒为主目标页**（按需批次契约）：其余项为预取邻页，导出失败只
//     记日志不影响主目标页结果；done 载荷的 SlideNum 也取 Pages[0]。
//
// OnDemand 标记详情侧单页按需批次：不发 progress 事件（前端批量进度条不闪），
// done 事件带同名字段供前端过滤。
type thumbBatchTask struct {
	LibraryID string
	Files     []thumbFileSpec
	ThumbsDir string // 缩略图根目录 data/assets/<lib-id>/thumbs
	Tiers     []thumbTier
	Pages     []int
	OnDemand  bool
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
	// ExportPresentation 打开 pptx（ReadOnly 无窗口）逐页导出 PNG 到
	// thumbsDir/<tier.Key>/<fileID>-<3位页>.png。pages 为 nil 表示全书所有页，
	// 非 nil 为页白名单（1-based；详情侧单页按需生成只导目标页）。返回成功
	// 导出的页数。
	// 每页（含双档）导完后调用 preempt：非 nil 返回 = 有跳转请求插队，
	// 实现须在同一 COM 会话内执行跳转（JumpInSession 语义）并写回 jr.Result，
	// 之后继续导出下一页（用户窗口与后台导图并存，PRD D3）。
	ExportPresentation(ctx context.Context, spec thumbFileSpec, thumbsDir string, tiers []thumbTier, pages []int, preempt func() *thumbJumpRequest) (int, error)
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
	sinkFn   func() EventSink   // 事件出口（复用宿主 AssetLibraryService 的 sinkHolder，防双出口漂移）
	ledger   *thumbCacheLedger  // 记账 ledger（生成成功后记账 + 页数缩减对账，assets_thumbs_ledger.go）
	tiersFn  func() []thumbTier // 当前生效档位读取口（设置注入，nil 回退包级 thumbTiers 默认）

	batchCh  chan *thumbBatchTask   // 容量 thumbBatchQueueCap，多库并发入队不阻塞
	demandCh chan *thumbBatchTask   // 详情侧单页按需生成（容量 thumbOnDemandQueueCap）：不进 batches 注册表、不与批量互相取消，消费优先级高于批量
	jumpCh   chan *thumbJumpRequest // 无缓冲：交接即被消费（worker 空闲直收 / 批量在途由 preempt 页间摘取），提交阻塞等消费方消除时序竞态
	quit     chan struct{}
	wg       sync.WaitGroup // worker 计数（WaitThumbsDone 关停收口）

	mu      sync.Mutex
	batches map[string]*thumbBatchTask // libraryID -> 最新批次（pending/active，取消入口）
	// demandPending 按需单页请求去重集（键 libraryID|fileID|slideNum|tierKey）：
	// 详情侧连续切换条目可能对同一页重复请求，PNG 未落盘前两次入队会重复导出。
	// 仅内存态（进程退出即失效，幂等重入无害）。
	demandPending map[string]bool
	// demandTasks 在途/待执行按需任务（取消面）：按需任务不进 batches 注册表
	// （不与批量互相覆盖取消），但关停与「删库/清档」必须能取消它们——漏了则
	// 关停要等满按需超时，清档 RemoveAll 后按需任务还会以旧分辨率重建目录写图。
	demandTasks map[*thumbBatchTask]bool
	started     bool
	closed      bool // WaitThumbsDone 后置位，入队直接拒绝
}

// thumbBatchQueueCap 并发库数量级上限（超出视为异常，入队拒绝）。
const thumbBatchQueueCap = 8

// thumbOnDemandQueueCap 按需生成队列容量：详情侧打开瞬时可能连点多个条目，
// 单页任务耗时秒级，积压 16 个即视为异常（超出直接拒绝并记日志，不阻塞前端）。
const thumbOnDemandQueueCap = 16

// newThumbQueue 构造队列（exporter 由 newThumbExporter 平台分叉注入，sinkFn
// 取宿主 AssetLibraryService.eventSink，ledger 为记账出口，tiersFn 为设置注入
// 的档位读取口——nil 回退包级 thumbTiers，单测与 serve 精简装配兼容）。
func newThumbQueue(exp ThumbExporter, sinkFn func() EventSink, ledger *thumbCacheLedger, tiersFn func() []thumbTier) *thumbQueue {
	return &thumbQueue{
		exporter: exp,
		sinkFn:   sinkFn,
		ledger:   ledger,
		tiersFn:  tiersFn,
		batchCh:  make(chan *thumbBatchTask, thumbBatchQueueCap),
		demandCh: make(chan *thumbBatchTask, thumbOnDemandQueueCap),
		jumpCh:   make(chan *thumbJumpRequest),
		quit:     make(chan struct{}),
		batches:  make(map[string]*thumbBatchTask),

		demandPending: make(map[string]bool),
		demandTasks:   make(map[*thumbBatchTask]bool),
	}
}

// tiers 当前生效档位（每批解析一次）：nil tiersFn 回退包级默认（单测构造未注入）。
func (q *thumbQueue) tiers() []thumbTier {
	if q.tiersFn == nil {
		return thumbTiers
	}
	return q.tiersFn()
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

// run worker 主循环：每轮先非阻塞摘跳转（严格优先），再非阻塞摘按需单页任务
// （用户可感知，优先于批量），最后阻塞等批次/跳转/退出。
// 批量在途期间的跳转由 handleBatch 内 preempt 回调承接（页间插队）；按需任务
// 在两批之间被消费（批量文件级粒度，最多等一个文件的导出时间）。
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
		case b := <-q.demandCh:
			q.handleBatch(b)
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
		case b := <-q.demandCh:
			q.handleBatch(b)
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
// 按需批次（OnDemand）不发 progress，只发终态 done（前端据此过滤批量进度条）。
func (q *thumbQueue) handleBatch(b *thumbBatchTask) {
	defer b.Cancel() // 释放按需任务的超时定时器（成功路径同样回收）
	defer func() {
		q.mu.Lock()
		// 仅清理仍指向本批次的注册项（期间同库新批次已覆盖时不误删）。按需批次
		// 不进注册表，此处恒不命中——与批量共用同一收尾逻辑无副作用。
		if cur, ok := q.batches[b.LibraryID]; ok && cur == b {
			delete(q.batches, b.LibraryID)
		}
		if b.OnDemand {
			q.clearDemandPendingLocked(b) // 去重集释放：后续 miss 可再次请求
			delete(q.demandTasks, b)      // 取消面注销
		}
		q.mu.Unlock()
	}()

	if !q.exporter.Available() {
		// 入队后 PowerPoint 变为不可用（卸载/COM 损坏）：整批跳过，事件终态
		emitEvent(q.sinkFn(), "assets-thumbs:done", model.AssetThumbsResult{
			LibraryID: b.LibraryID, Total: len(b.Files), Skipped: len(b.Files),
			OnDemand: b.OnDemand,
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

	// 当前生效档位每批解析一次（设置注入，分辨率变更经 ApplyThumbsSettingsChange
	// 清档后重入队，批内恒一致）。批量的默认档位恒为 small（batchBacklogTiers
	// 单点）：hd 已退出批量链路改详情侧按需生成，批量仍导 hd 会白耗 COM 时间并
	// 挤占 hd 配额（淘汰后又按需重生成）。显式档位任务（按需单页）用任务自带值。
	tiers := b.Tiers
	if tiers == nil {
		tiers = batchBacklogTiers(q.tiers())
		if tiers == nil {
			tiers = q.tiers() // 档位集合异常（不含 small）：回退全档，保持既有行为
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
		n, err := q.exporter.ExportPresentation(fctx, f, b.ThumbsDir, tiers, b.Pages, preempt)
		cancel()
		switch {
		case err == nil:
			generated++
			// 记账 + 页数缩减对账（仅成功路径——失败文件旧图仍有效，内容失效
			// 已由扫描失效联动处理）。记账粒度：逐档逐页 Stat 成功才记账（PNG
			// 缺失的页留给下次补缺链路重导出）。
			if q.ledger != nil {
				q.recordExportedPages(b, f, tiers, n)
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
		if !b.OnDemand {
			emitEvent(q.sinkFn(), "assets-thumbs:progress", model.AssetThumbsProgress{
				LibraryID: b.LibraryID, Done: i + 1, Total: len(b.Files),
				CurrentFile: f.Path, Failed: failed,
			})
			// 批量在途时的按需插队点（与 jump 的页间 preempt 同层）：不在此消费，
			// 按需任务只能等整批结束（千页级数分钟），叠加排队超时必然作废——详情
			// 大图永不出现。嵌套 handleBatch 为单片任务，内部不再插队（无递归放大）。
			q.drainDemand()
		}
		if canceled {
			break
		}
	}

	Logger().Info("缩略图批次完成", "libraryId", b.LibraryID, "total", len(b.Files),
		"generated", generated, "failed", failed, "skipped", skipped, "canceled", canceled,
		"onDemand", b.OnDemand)
	done := model.AssetThumbsResult{
		LibraryID: b.LibraryID, Total: len(b.Files),
		Generated: generated, Skipped: skipped, Failed: failed, Canceled: canceled,
		OnDemand: b.OnDemand,
	}
	// 按需批次回填目标条目：前端据此把失败/取消落到当前详情卡（显示重试入口），
	// 否则 onDemand 失败后 hd 永为占位且无任何可见反馈。SlideNum 恒取 Pages[0]
	// ——按需任务的白名单首位即主目标页，其余为预取邻页（thumbPrefetchPages）。
	if b.OnDemand && len(b.Files) == 1 {
		done.FileID = b.Files[0].FileID
		if len(b.Pages) > 0 {
			done.SlideNum = b.Pages[0]
		}
	}
	emitEvent(q.sinkFn(), "assets-thumbs:done", done)
}

// drainDemand 非阻塞排空按需通道（批量每个文件边界调用一次）：详情侧单页任务
// 优先于后续批量文件执行，避免整批等待把按需请求拖到超时。
func (q *thumbQueue) drainDemand() {
	for {
		select {
		case d := <-q.demandCh:
			q.handleBatch(d)
		default:
			return
		}
	}
}

// recordExportedPages 导出成功后的记账与页数缩减对账。
//
// 全档全页批次（b.Pages == nil）：按返回页数 1..n 逐档逐页记账，并 pruneBeyond
// 清越界记账与旧图（页数缩减对账，20 页 → 15 页时越界页不覆盖须显式清）。
//
// 页白名单批次（b.Pages != nil，详情侧单页按需）：只对白名单页记账，且**跳过
// pruneBeyond**——未导出页的既有产物与记账必须保留，按「本次导出页数」裁剪会
// 把第 1 页之后的所有产物误删（按需单页导出返回 n=1 时将清空整本后续页）。
func (q *thumbQueue) recordExportedPages(b *thumbBatchTask, f thumbFileSpec, tiers []thumbTier, exported int) {
	if b.Pages == nil {
		for _, tier := range tiers {
			for page := 1; page <= exported; page++ {
				if info, statErr := os.Stat(thumbPNGPath(b.ThumbsDir, tier.Key, f.FileID, page)); statErr == nil {
					q.ledger.record(b.LibraryID, f.FileID, tier.Key, page, info.Size())
				}
			}
			q.ledger.pruneBeyond(b.LibraryID, b.ThumbsDir, f.FileID, tier.Key, exported)
		}
		return
	}
	// 白名单页：只有实际落盘的页入记账（导出失败页不被记账，留给补缺/按需重试）
	for _, tier := range tiers {
		for _, page := range b.Pages {
			if info, statErr := os.Stat(thumbPNGPath(b.ThumbsDir, tier.Key, f.FileID, page)); statErr == nil {
				q.ledger.record(b.LibraryID, f.FileID, tier.Key, page, info.Size())
			}
		}
	}
}

// thumbPNGPath 缩略图缓存路径单点（契约：thumbsDir/<tier>/<fileID>-<3位页>.png；
// slides.id 每轮重解析全变禁作缓存键——files.id 稳定）。队列与 COM 实现共用，
// PR2 记账表以同规则反查。
func thumbPNGPath(thumbsDir, tierKey string, fileID int64, slideNum int) string {
	return filepath.Join(thumbsDir, tierKey, fmt.Sprintf("%d-%03d.png", fileID, slideNum))
}

// pageRequested 页白名单判定：pages 为 nil（全书）恒真；非 nil 仅命中页导出。
// 白名单常见形态为单页（详情侧按需）或目标页+前后邻页（预取），线性查找足够。
func pageRequested(pages []int, slideNum int) bool {
	if pages == nil {
		return true
	}
	for _, p := range pages {
		if p == slideNum {
			return true
		}
	}
	return false
}

// thumbPrefetchPages 按需单页生成的实际导出页集合（纯函数，可直测）：目标页恒在
// 首位（Pages[0] = 主目标页契约——done 载荷回填与记账分流都依赖它），后随前后
// 邻页预取（详情侧连续翻页时下一页多半已就绪，摊薄单文件 Open 成本）。页序钳制
// 到 ≥1（首页无前邻页）、上限 3 页。单页文件的后邻页导出会失败——由 exportSession
// 的「非主目标页失败只告警」规则兜底，不影响主目标页结果。
func thumbPrefetchPages(slideNum int) []int {
	pages := []int{slideNum}
	for _, p := range []int{slideNum - 1, slideNum + 1} {
		if p < 1 || p == slideNum {
			continue
		}
		pages = append(pages, p)
	}
	return pages
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
//
// 取消面含在途/待执行的按需任务（detail 侧单页生成）：删库与清档链路依赖它，
// 漏取消会让按需任务在 RemoveAll 之后重建档目录并写入旧分辨率 PNG。
func (s *AssetLibraryService) CancelAssetThumbs(libraryID string) bool {
	q := s.thumbQ
	q.mu.Lock()
	b, ok := q.batches[libraryID]
	if ok {
		delete(q.batches, libraryID)
	}
	demands := make([]*thumbBatchTask, 0, 2)
	for t := range q.demandTasks {
		if t.LibraryID == libraryID {
			demands = append(demands, t)
			delete(q.demandTasks, t)
		}
	}
	q.mu.Unlock()
	if ok {
		b.Cancel()
	}
	for _, t := range demands {
		t.Cancel()
	}
	return ok || len(demands) > 0
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
	// 按需任务同样取消（不进注册表，须单独收口）：否则关停要在 q.wg.Wait 前
	// 等满按需超时（最长 120s），与「取消全部批次后 worker 退出」契约不符。
	for t := range q.demandTasks {
		batches = append(batches, t)
		delete(q.demandTasks, t)
	}
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
	tiers := s.resolvedThumbTiers()
	if tierIndex < 0 || tierIndex >= len(tiers) {
		return "", fmt.Errorf("缩略图档位非法（tierIndex=%d，取值 0-%d）", tierIndex, len(tiers)-1)
	}
	if slideNum < 1 {
		return "", fmt.Errorf("页序非法（slideNum=%d，从 1 计）", slideNum)
	}
	if fileID < 1 {
		return "", fmt.Errorf("文件 ID 非法（fileId=%d）", fileID)
	}
	path := thumbPNGPath(s.assetThumbsDir(libraryID), tiers[tierIndex].Key, fileID, slideNum)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // 未生成/已淘汰：前端回退占位卡
		}
		return "", fmt.Errorf("读取缩略图失败: %w", err)
	}
	s.thumbLedger.record(libraryID, fileID, tiers[tierIndex].Key, slideNum, int64(len(data)))
	return base64.StdEncoding.EncodeToString(data), nil
}

// ---------- 设置注入（PRD D2 设置链路，PR3） ----------

// SetThumbsSettingsProvider 注入缩略图设置读取口（app_services.go 装配期调用；
// nil = 未注入走默认值——单测与 serve 精简装配兼容）。
func (s *AssetLibraryService) SetThumbsSettingsProvider(fn func() model.AssetThumbsSettings) {
	s.thumbsCfgFn = fn
}

// thumbsSettings 当前生效设置（nil provider → 默认；返回值已 normalize）。
func (s *AssetLibraryService) thumbsSettings() model.AssetThumbsSettings {
	cfg := model.DefaultAssetThumbsSettings()
	if s.thumbsCfgFn != nil {
		cfg = s.thumbsCfgFn()
	}
	return model.NormalizeAssetThumbsSettings(&cfg)
}

// resolvedThumbTiers 当前生效双档分辨率（key 恒 small/hd 固定顺序，宽度来自
// 设置，高按 16:9 自算——320*9/16=180、1600*9/16=900 与既有默认严格一致）。
func (s *AssetLibraryService) resolvedThumbTiers() []thumbTier {
	return thumbTiersFromSettings(s.thumbsSettings())
}

// thumbQuotaBytes 档位配额字节（small/hd 分别取设置 MB << 20；未知档位按 small
// 口径防御返回——档位集合由 resolvedThumbTiers 单点构造，恒为 small/hd）。
func (s *AssetLibraryService) thumbQuotaBytes(tier string) int64 {
	cfg := s.thumbsSettings()
	if tier == "hd" {
		return int64(cfg.HDQuotaMB) << 20
	}
	return int64(cfg.SmallQuotaMB) << 20
}

// RetryAssetThumb 单文件缩略图重试（D4：详情侧边栏「重试」按钮入口；复用队列
// 链路，同库旧批次经 EnqueueAssetThumbs 既有取消语义覆盖）。
// 校验顺序：库不存在（E_ASSETS_LIB_NOT_FOUND 透传）→ 无 PowerPoint
// （E_ASSETS_THUMBS_UNAVAILABLE）→ 参数非法 → 源文件存在性。
func (s *AssetLibraryService) RetryAssetThumb(libraryID string, fileID int64, relPath string) error {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return err
	}
	if !s.thumbQ.exporter.Available() {
		return errThumbsUnavailable
	}
	if fileID < 1 {
		return fmt.Errorf("文件 ID 非法（fileId=%d）", fileID)
	}
	absPath, err := resolveAssetRelAbsPath(lib, relPath)
	if err != nil {
		return err
	}
	s.EnqueueAssetThumbs(libraryID, []thumbFileSpec{{
		FileID:  fileID,
		Path:    relPath,
		AbsPath: absPath,
	}}, s.assetThumbsDir(libraryID))
	return nil
}

// resolveAssetRelAbsPath 相对路径 → 库内绝对路径单点校验（RetryAssetThumb 与
// RequestAssetThumb 共用）。
//
// 越界遏制：relPath 来自前端（浏览器通道可达本方法），`..` 或绝对路径可逃逸
// 库根——让 PowerPoint 渲染库外任意可打开文件并把 PNG 落入本库缩略图目录，
// 经 GetAssetThumb 回读内容。校验 Join 后仍在库根内（Rel 无 `..` 前缀）才放行；
// 同时校验源文件存在（缩略图生成前提，缺失直接报错不浪费 COM 打开）。
func resolveAssetRelAbsPath(lib model.AssetLibrary, relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("文件相对路径不能为空")
	}
	rel := filepath.FromSlash(relPath)
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("文件路径非法（须为素材根目录内的相对路径）")
	}
	absPath := filepath.Join(lib.RootPath, rel)
	if back, relErr := filepath.Rel(lib.RootPath, absPath); relErr != nil ||
		back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("文件路径越界（须位于素材根目录内）")
	}
	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("源文件不存在或无法访问: %w", err)
	}
	return absPath, nil
}

// thumbOnDemandTimeout 按需单页生成的整任务超时（排队 + Open + 单页导出）。
// 队列繁忙时可能排在批量文件之后，故留足余量；超时由 exporter 侧杀自启实例
// 收口，与批量单文件超时同构。
const thumbOnDemandTimeout = 120 * time.Second

// thumbDemandKey 按需请求去重键（堆内内存态，见 thumbQueue.demandPending）。
func thumbDemandKey(libraryID string, fileID int64, slideNum int, tierKey string) string {
	return fmt.Sprintf("%s|%d|%d|%s", libraryID, fileID, slideNum, tierKey)
}

// dropDemandTask 按需任务提交失败回滚（去重键 + 取消面登记 + 超时定时器）。
func (q *thumbQueue) dropDemandTask(b *thumbBatchTask, key string) {
	b.Cancel()
	q.mu.Lock()
	delete(q.demandPending, key)
	delete(q.demandTasks, b)
	q.mu.Unlock()
}

// clearDemandPendingLocked 释放按需去重键（调用方持 q.mu）。按需任务恒为
// 单文件单档，页白名单首位为主目标页（其余为预取邻页，thumbPrefetchPages），
// 去重键按 Pages[0] 构造；非该形态的按需任务不产生键。
func (q *thumbQueue) clearDemandPendingLocked(b *thumbBatchTask) {
	if len(b.Files) != 1 || len(b.Pages) == 0 || len(b.Tiers) != 1 {
		return
	}
	delete(q.demandPending, thumbDemandKey(b.LibraryID, b.Files[0].FileID, b.Pages[0], b.Tiers[0].Key))
}

// RequestAssetThumb 详情侧单页单档按需生成（hd 档退离批量补缺后的唯一补齐入口，
// PRD R2）：目标 PNG 已存在直接返回（幂等）；在途同页请求去重；入队走独立按需
// 通道（不进库批次注册表，不与批量互相取消）。异步入队，前端等待该库
// assets-thumbs:done（载荷 onDemand=true）后重拉。
//
// 邻页预取（提效）：实际导出页为「主目标页 + 前后邻页」（thumbPrefetchPages），
// 目标页恒在首位；邻页失败不影响主目标页结果，翻页时命中既有 PNG 免再打开。
//
// 校验顺序：库存在（透传）→ 无 PowerPoint（E_ASSETS_THUMBS_UNAVAILABLE）→
// 档位/页序/文件 ID 非法 → 源文件存在性（越界遏制单点 resolveAssetRelAbsPath）。
func (s *AssetLibraryService) RequestAssetThumb(libraryID string, fileID int64, relPath string, slideNum, tierIndex int) error {
	lib, err := s.getLibrary(libraryID)
	if err != nil {
		return err
	}
	q := s.thumbQ
	if !q.exporter.Available() {
		return errThumbsUnavailable
	}
	tiers := s.resolvedThumbTiers()
	if tierIndex < 0 || tierIndex >= len(tiers) {
		return fmt.Errorf("缩略图档位非法（tierIndex=%d，取值 0-%d）", tierIndex, len(tiers)-1)
	}
	if slideNum < 1 {
		return fmt.Errorf("页序非法（slideNum=%d，从 1 计）", slideNum)
	}
	if fileID < 1 {
		return fmt.Errorf("文件 ID 非法（fileId=%d）", fileID)
	}
	absPath, err := resolveAssetRelAbsPath(lib, relPath)
	if err != nil {
		return err
	}
	// fileID 与路径一致性校验：两侧均来自前端（浏览器通道可达），不校验则调用方
	// 可把 A 文件的页面写进 B 的缓存键（之后 GetAssetThumb(B) 返回 A 的内容，且
	// 孤儿清扫会按索引 ID 判定行为不确定）。查无记录（已删除/未索引）同样拒绝。
	store, err := s.storeForThumbs(libraryID)
	if err != nil {
		return err
	}
	ownerID, err := store.fileIDForPath(relPath)
	if err != nil {
		return fmt.Errorf("查询文件记录失败: %w", err)
	}
	if ownerID != fileID {
		return fmt.Errorf("文件记录不匹配（fileId=%d 与该路径的索引记录不一致）", fileID)
	}
	thumbsDir := s.assetThumbsDir(libraryID)
	tier := tiers[tierIndex]
	if _, err := os.Stat(thumbPNGPath(thumbsDir, tier.Key, fileID, slideNum)); err == nil {
		return nil // 已存在：幂等返回，不重复入队
	}

	key := thumbDemandKey(libraryID, fileID, slideNum, tier.Key)
	ctx, cancel := context.WithTimeout(context.Background(), thumbOnDemandTimeout)
	b := &thumbBatchTask{
		LibraryID: libraryID,
		Files:     []thumbFileSpec{{FileID: fileID, Path: relPath, AbsPath: absPath}},
		ThumbsDir: thumbsDir,
		Tiers:     []thumbTier{tier},
		// 页白名单 = 主目标页 + 前后邻页预取（Pages[0] 恒为主目标页）。单文件会话
		// 已被 COM worker 缓存（assets_thumbs_windows.go），邻页在同一会话内导出，
		// 翻页时大概率命中既有 PNG——Open 成本被摊薄而非重复支付。
		Pages:    thumbPrefetchPages(slideNum),
		OnDemand: true,
		Ctx:      ctx,
		Cancel:   cancel,
	}
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		cancel()
		return nil // 关停期静默：无队列可消费，前端按 miss 占位兜底
	}
	if q.demandPending[key] {
		q.mu.Unlock()
		cancel()
		return nil // 同页请求在途，去重
	}
	q.demandPending[key] = true
	q.demandTasks[b] = true // 取消面登记（CancelAssetThumbs / WaitThumbsDone 可中断）
	q.mu.Unlock()

	q.start()
	select {
	case q.demandCh <- b:
		return nil
	case <-q.quit:
		q.dropDemandTask(b, key)
		return nil
	default:
		q.dropDemandTask(b, key)
		Logger().Warn("缩略图按需生成队列已满，请求被拒", "libraryId", libraryID,
			"fileId", fileID, "slide", slideNum, "tier", tier.Key)
		return model.NewAppError(model.ErrCodeAssetsThumbsBusy, "缩略图生成繁忙，请稍后重试")
	}
}

// ApplyThumbsSettingsChange 缩略图设置变更应用（App.SaveSettings 保存成功后调用）：
// 分辨率变更 → 清该档（全部库：取消在途批次 + 删记账该档行 + 清 dirty 该档 +
// 删该档目录 + 重入队补缺，重入队受 Auto 开关与 Available 短路）；上限变更 →
// 触发一次惰性淘汰检查。异步执行不阻塞保存 RPC；prev/cur 传值（App 层先
// Normalize）。错误全部仅告警（清档幂等可自愈，不回滚保存）。
//
// 关停竞态口径（结构性接受，与 DeleteLibrary 取消异步同风格）：本 goroutine 不
// 感知 App.shutdown——WaitThumbsDone 置位 closed 后入队被拒绝（重入队面收口）；
// 窗口内已入队批次滞留 batchCh 无 worker 消费、CloseAll 后 storeForThumbs 可能
// 惰性重开连接做一次清档/淘汰——两者仅发生在进程退出收尾期，无 COM 调用、无
// 可观察危害，不为收尾竞态加关停等待。
func (s *AssetLibraryService) ApplyThumbsSettingsChange(prev, cur model.AssetThumbsSettings) {
	go s.applyThumbsSettingsChangeSync(prev, cur)
}

// applyThumbsSettingsChangeSync 设置变更应用同步实现（公开方法异步包装本方法，
// 测试直调以 chan 通知驱动替代 sleep 等待竞态——test-stability 契约）。
func (s *AssetLibraryService) applyThumbsSettingsChangeSync(prev, cur model.AssetThumbsSettings) {
	cleared, requota := diffThumbsSettings(prev, cur)
	if len(cleared) == 0 && len(requota) == 0 {
		return
	}
	libs, err := s.ListLibraries()
	if err != nil {
		Logger().Warn("缩略图设置变更应用跳过（库列表读取失败）", "err", err)
		return
	}
	for _, lib := range libs {
		for _, tier := range cleared {
			s.resetThumbTier(lib, tier)
		}
		// 全部 cleared 档清完后重入队补缺（一次一批覆盖双档——文件级粒度本就
		// 整文件重导出）：受 Auto 开关与 COM 可用性短路
		if len(cleared) > 0 && cur.Auto && s.thumbQ.exporter.Available() {
			s.reenqueueThumbsAfterTierReset(lib)
		}
		for _, tier := range requota {
			s.thumbLedger.checkQuota(lib.ID, tier)
		}
	}
}

// diffThumbsSettings 比较新旧设置得出受影响档位：宽度变 → 需清档重建（分辨率
// 变更旧 PNG 尺寸不符须全量重导出）；上限 MB 变 → 需一次性惰性淘汰检查。
// key 恒 small/hd 固定顺序。
func diffThumbsSettings(prev, cur model.AssetThumbsSettings) (cleared, requota []string) {
	if prev.SmallWidth != cur.SmallWidth {
		cleared = append(cleared, "small")
	}
	if prev.HDWidth != cur.HDWidth {
		cleared = append(cleared, "hd")
	}
	if prev.SmallQuotaMB != cur.SmallQuotaMB {
		requota = append(requota, "small")
	}
	if prev.HDQuotaMB != cur.HDQuotaMB {
		requota = append(requota, "hd")
	}
	return cleared, requota
}

// resetThumbTier 清除单库单档缩略图产物（分辨率变更清档联动）：取消在途批次 →
// 删记账该档行（db）→ 清 dirty 该档（防旧分辨率记账复活）→ 删该档 PNG 目录。
// 各步失败仅告警不中断。目录删除重试一次（CancelAssetThumbs 协作式取消不等待
// 在途页导出收口，Windows 上在途 PNG 句柄可致首删部分失败）；仍失败的残留旧
// 分辨率 PNG 无链路再清（记账行已删、Auto=false 时不重导出，补缺判定的 PNG
// 存在性权威会放行旧图），照常展示直至重建索引或手动清理——已登记口径。
func (s *AssetLibraryService) resetThumbTier(lib model.AssetLibrary, tier string) {
	s.CancelAssetThumbs(lib.ID)
	store, err := s.storeForThumbs(lib.ID)
	if err != nil {
		Logger().Warn("缩略图清档跳过（库不可用）", "libraryId", lib.ID, "tier", tier, "err", err)
		return
	}
	if err := store.deleteThumbCacheTier(tier); err != nil {
		Logger().Warn("删除缩略图记账档位失败", "libraryId", lib.ID, "tier", tier, "err", err)
	}
	s.thumbLedger.dropTier(lib.ID, tier)
	dir := filepath.Join(s.assetThumbsDir(lib.ID), tier)
	if err := os.RemoveAll(dir); err != nil {
		if err := os.RemoveAll(dir); err != nil {
			Logger().Warn("删除缩略图档位目录失败（重试后仍失败，残留旧分辨率 PNG 将照常展示）",
				"libraryId", lib.ID, "tier", tier, "err", err)
		}
	}
}

// reenqueueThumbsAfterTierReset 清档后重入队补缺（可用性/Auto 判断由调用方
// applyThumbsSettingsChangeSync 完成）。补缺收集失败仅告警，下次扫描重试。
func (s *AssetLibraryService) reenqueueThumbsAfterTierReset(lib model.AssetLibrary) {
	store, err := s.storeForThumbs(lib.ID)
	if err != nil {
		Logger().Warn("缩略图清档重入队跳过（库不可用）", "libraryId", lib.ID, "err", err)
		return
	}
	thumbsDir := s.assetThumbsDir(lib.ID)
	backlog, err := s.collectThumbsBacklog(store, lib, thumbsDir)
	if err != nil {
		Logger().Warn("缩略图清档重入队收集失败（下次扫描重试）", "libraryId", lib.ID, "err", err)
		return
	}
	if len(backlog) == 0 {
		return
	}
	Logger().Info("缩略图清档重入队", "libraryId", lib.ID, "files", len(backlog))
	s.EnqueueAssetThumbs(lib.ID, backlog, thumbsDir)
}
