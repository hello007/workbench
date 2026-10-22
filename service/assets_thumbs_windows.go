//go:build windows

// 素材缩略图链路 Windows COM 实现（go-ole，PowerPoint.Application late binding）。
//
// 技术依据：.trellis/tasks/10-14-ppt-com/research/com-thumbnail-export.md +
// 探针实测（scripts/ppt-com-probe，probe-report.txt）：
//   - STA 会话：专用常驻 goroutine（LockOSThread + CoInitializeEx STA，放行
//     S_FALSE(1)）——全部 COM 调用锁在该线程，主侧经任务通道提交；
//   - Presentations/Slides 等集合经 GetProperty 取（DISPATCH_PROPERTYGET；
//     CallMethod 只走 DISPATCH_METHOD 报「找不到成员」——探针 P3 实测踩坑）；
//   - 导出 Open(ReadOnly:=msoTrue(-1), Untitled:=msoFalse(0), WithWindow:=msoFalse(0))
//     + Slide.Export(path,"PNG",w,h)；跳转 Open WithWindow:=msoTrue(-1) +
//     ActiveWindow.View.GotoSlide(n) + Activate——两场景 Open 参数禁共用；
//     WPS 兼容层对 WithWindow 打开不顺带显示窗口（PowerPoint 语义）：COM 激活
//     实例应用级默认不可见，跳转链路须显式 PutProperty Visible（jumpInSession，
//     真机实测全新实例 Open/GotoSlide/Activate 全成功但零可见窗口）；
//   - 防弹窗三板斧先于一切 Open：DisplayAlerts=1（ppAlertsNone，探针 P1 实测
//     接受）+ AutomationSecurity=3（禁宏）+ 恒 ReadOnly；
//   - 归属判定：CreateInstance 前后 PowerPoint 进程快照 diff（util.ListProcesses
//     过滤 powerpnt.exe/wpp.exe 双进程名），新增 PID = 自启实例（可 Quit/可强杀）；
//     diff 为空 = 复用已存活实例（禁 Quit 禁杀，静默只读消费）。为区分「用户已开」
//     与「上会话崩溃残留」（归属纯内存态随崩溃丢失——真机 D7 定性：跳转在幽灵
//     后台实例内「成功」但窗口不可见），自启 PID 同步落盘 com_owned_pids.json，
//     下会话按「存活进程 ∩ 盘上残留集」识别自有残留（进程名白名单 + 24h mtime
//     窗口双重过滤防 PID 复用误杀）并精确强杀后重建；禁 taskkill /IM 全杀
//     （误杀用户 PPT）；
//   - 超时/取消：阻塞 Invoke 不可中断——主侧 ctx 超时杀自启 PID 让 Invoke 以
//     0x800706BA 返回，worker 走清理路径（research §3.2 推荐形态）；
//   - 占用规避走预防：恒 ReadOnly + 打开前 Presentations 集合按 FullName 复用。

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows/registry"

	"workbench/util"
)

// msoTriState MsoTriState 取值（COM 层字面量，注释语义——VBA 常量不可引用）。
const (
	msoFalse = 0
	msoTrue  = -1
)

// ppAlertsNone / msoAutomationSecurityForceDisable 防弹窗数值（探针 P1 实测接受）。
const (
	ppAlertsNone                      = 1
	msoAutomationSecurityForceDisable = 3
)

// officeServerProcNames 归属判定进程名集合（大小写不敏感比对）：PowerPoint 为
// powerpnt.exe；WPS 接管 PowerPoint.Application ProgID 的环境（探针实测本机）
// COM 启动的是 wpp.exe——若硬编码 powerpnt，WPS 环境下快照 diff 恒空 → 误判
// 「复用用户实例」→ 禁 Quit 禁杀，超时路径失去强杀能力（探针实测结论，
// research/com-thumbnail-export.md「实测新增结论 2」）。
var officeServerProcNames = []string{"powerpnt.exe", "wpp.exe"}

// jumpTimeout 打开跳转硬超时（交互操作秒级预期；防 PowerPoint 卡死挂死调用方）。
const jumpTimeout = 60 * time.Second

// CreateInstance 忙态重试（0x8001010A）：5 次 × 1s，research §3.4 推荐量级。
const (
	comCreateRetries    = 5
	comCreateRetryDelay = time.Second
)

// dispEException DISP_E_EXCEPTION（0x80020009）：COM 服务器内部异常。空 ExcepInfo
// 时 go-ole 把 null bstrDescription 渲染为字面量 "<nil>"（idispatch_windows.go
// Invoke 失败分支 + ole.go renderStrings 实测源码）——真机日志「发生意外。 (<nil>)」
// 即 hr=0x80020009 + bstrDescription 为 null 的形态，「发生意外。」来自 FormatMessage
// 对该 scode 的中文本地化文案，非 ExcepInfo 自带描述。
const dispEException = 0x80020009

// 页级 Export 瞬态重试与 owned 退出等待参数（包级 var 注入点：单测注入极小间隔
// 快速跑——test-stability 契约禁 sleep 等待竞态，禁依赖真实退避时长）。
var (
	thumbExportMaxAttempts = 3               // 页导出总尝试次数（含首次；即至多重试 2 次）
	thumbExportRetryDelay  = 1 * time.Second // 退避基准：第 n 次重试前等待 delay*n（1s/2s）
)

var (
	thumbOwnedExitPollInterval = 200 * time.Millisecond // owned 退出轮询间隔
	thumbOwnedExitWait         = 3 * time.Second        // owned 退出等待上限
)

// thumbOwnedExitProbe owned 实例存活探针（生产 = officeServerPIDSet 全局枚举；
// 单测注入 fake PID 集驱动「退出/恒存」分支——勿在测试路径直调全局枚举）。
var thumbOwnedExitProbe = officeServerPIDSet

// goOleThumbExporter ThumbExporter 的 Windows COM 实现。
//
// 线程模型：主侧方法（ExportPresentation/JumpOnly/…）只做任务提交与结果等待；
// 全部 COM 调用在 workerLoop 常驻 STA goroutine 内执行（comWorkerState 持有
// Application 会话）。主侧超时/取消以「杀自启 powerpnt」驱动 worker 内阻塞
// Invoke 返回（Go 无法中断 syscall，research §3.2）。
type goOleThumbExporter struct {
	taskCh     chan func(*comWorkerState) // STA worker 任务通道（无缓冲：提交即等待交接）——缩略图批量链路
	urgentCh   chan func(*comWorkerState) // 紧急任务通道（无缓冲，与 taskCh 同构）——用户可感知交互类任务（PPT 预览 PDF 转换）严格优先消费
	quit       chan struct{}
	workerDone chan struct{}

	available    atomic.Bool // 可用性缓存（注册表预检 + CreateInstance 兜底修正）
	availableSet atomic.Bool // 缓存是否已初始化（零值 false 不等于「不可用」）
	dirty        atomic.Bool // 会话脏标记（主侧 Discard 置位 / worker ensureApp 检查并清位——跨线程经实例 atomic，不用包级变量防多实例互染）
	startOnce    sync.Once
	shutdownOnce sync.Once // Shutdown 幂等（close(e.quit) 双重 close 会 panic——并发/重复调用收敛到首次执行，check P2）

	ownedMu     sync.Mutex   // 保护 ownedMirror（worker 线程写、主侧杀进程路径读）；owned 持久化与盘上残留集缓存同锁复用（D7）
	ownedMirror map[int]bool // 自启 PID 镜像：超时强杀路径在 worker 阻塞于 Invoke 时无法经任务通道取快照（通道请求永不被消费），镜像让主侧无需 worker 配合即可定位自启 PID。发布时整体替换新 map，读侧持锁取引用后可无锁遍历

	ownedPersistPath string       // 自启 PID 持久化文件路径（data/assets/com_owned_pids.json，D7 残留归属自证；空 = 不持久化——非 Windows 桩/单测构造天然跳过）
	staleOwnedSet    bool         // 盘上残留集是否已加载（load-once 缓存标记，区分「未加载」与「加载后为空」）
	staleOwnedPIDs   map[int]bool // 盘上残留 PID 集（loadStaleOwnedPIDs 懒加载一次，先于首个 CreateInstance 读保证读到上会话崩溃时的内容）
}

// comWorkerState STA worker 线程内会话状态（app/ownedPIDs 仅 worker goroutine
// 访问，无锁；owner 反向引用供 publishOwned 发布镜像）。
type comWorkerState struct {
	app       *ole.IDispatch
	ownedPIDs map[int]bool        // 自启实例（CreateInstance 快照 diff 检出；空 = 复用已存活实例）。发布镜像并同步落盘 com_owned_pids.json——上会话崩溃（Shutdown 未跑）后下会话据此识别幽灵残留（D7）
	owner     *goOleThumbExporter // 反向引用：发布 owned PID 镜像（nil 时 publishOwned 跳过）
}

// newThumbExporter 平台分叉构造点（无 tag 侧调用）。ownedPersistPath 为自启
// PID 持久化文件路径（D7 残留归属自证；空 = 不持久化，单测构造传空跳过落盘面）。
func newThumbExporter(ownedPersistPath string) ThumbExporter {
	e := &goOleThumbExporter{
		taskCh:           make(chan func(*comWorkerState)),
		urgentCh:         make(chan func(*comWorkerState)),
		quit:             make(chan struct{}),
		workerDone:       make(chan struct{}),
		ownedPersistPath: ownedPersistPath,
	}
	return e
}

// startWorker 惰性启动 STA worker（首次任务提交时触发）。
func (e *goOleThumbExporter) startWorker() {
	e.startOnce.Do(func() {
		go e.workerLoop()
	})
}

// workerLoop STA 常驻线程：CoInitializeEx → 循环消费任务 → quit 时收尾。
func (e *goOleThumbExporter) workerLoop() {
	defer close(e.workerDone)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		// S_FALSE(0x1) = 线程已是 STA：go-ole 当错误返回，放行（仍须配对 CoUninitialize）
		if oleErrorHresult(err) != 1 {
			Logger().Error("COM STA 初始化失败，缩略图 worker 退出", "err", err)
			// 排空任务通道（含 urgent 通道）：主侧 select 等待会超时收口
			for {
				t := e.receiveNextTask()
				if t == nil {
					return
				}
				// 任务无法执行（无 STA）：调用方经 resCh 超时或任务自行报错——
				// 约定任务闭包内首查 w.app，此处直接以空状态执行让任务报错
				// （owner 必填：任务内 ensureApp 失败路径经 publishOwned 发布镜像）
				t(&comWorkerState{owner: e})
			}
		}
	}
	defer ole.CoUninitialize()

	w := &comWorkerState{ownedPIDs: map[int]bool{}, owner: e}
	for {
		t := e.receiveNextTask()
		if t == nil {
			w.cleanup()
			return
		}
		t(w)
	}
}

// receiveNextTask 单轮消费决策（worker 主循环与 STA 初始化失败排空路径共用）：
// urgent 通道严格优先——先非阻塞探测 urgentCh，命中即取；未命中降级为
// urgentCh/taskCh/quit 三路阻塞选择。优先级只作用于「下一个被消费任务」的
// 选取：正在执行的 COM 任务不可打断（阻塞 Invoke 契约），紧急任务最早在当前
// 任务返回后的下一轮被消费。返回 nil = quit 收口（提交侧任务闭包恒非 nil，
// nil 不会与真实任务混淆）。
func (e *goOleThumbExporter) receiveNextTask() func(*comWorkerState) {
	select {
	case t := <-e.urgentCh:
		return t
	default:
	}
	select {
	case t := <-e.urgentCh:
		return t
	case t := <-e.taskCh:
		return t
	case <-e.quit:
		return nil
	}
}

// submit 提交任务到 STA worker 普通通道（FIFO 排队语义；缩略图批量/跳转链路）。
// 阻塞至交接成功，不等执行完成——结果经任务写入的 resCh 由 waitFor 等待。
// ctx 仅约束提交等待；COM 调用不可中断——ctx 触发时杀自启实例促使 worker 内
// Invoke 返回（用户实例复用场景不可杀，仅放弃等待并告警）。
func (e *goOleThumbExporter) submit(ctx context.Context, task func(*comWorkerState)) error {
	return e.submitTo(ctx, e.taskCh, task)
}

// submitUrgent 提交紧急任务到 STA worker urgent 通道（严格优先）：worker 每轮
// 消费决策先探测 urgentCh（receiveNextTask），批量缩略图导图在途时用户可感知
// 的交互类任务——当前即 PPT 预览 PDF 转换（ConvertToPdf）——经此通道插队。
// 正在执行的 COM 任务不可打断，紧急任务最早在当前任务返回后的下一轮被消费。
// 其余语义与 submit 完全一致（单 worker 单 STA、阻塞至交接、quit/ctx 分支同构）。
func (e *goOleThumbExporter) submitUrgent(ctx context.Context, task func(*comWorkerState)) error {
	return e.submitTo(ctx, e.urgentCh, task)
}

// submitTo 提交等待共用实现（submit/submitUrgent 单点）：无缓冲通道阻塞至
// worker 交接成功；ctx 仅约束提交等待；quit 已收口返回 errThumbsUnavailable。
func (e *goOleThumbExporter) submitTo(ctx context.Context, ch chan func(*comWorkerState), task func(*comWorkerState)) error {
	e.startWorker()
	select {
	case ch <- task:
	case <-ctx.Done():
		return ctx.Err()
	case <-e.quit:
		return errThumbsUnavailable
	}
	return nil
}

// waitFor 等待任务结果：ctx 触发（超时/取消）走杀自启进程收口路径。
func (e *goOleThumbExporter) waitFor(ctx context.Context, resCh chan error) error {
	select {
	case err := <-resCh:
		return err
	case <-ctx.Done():
		e.killOwnedPowerpnt()
		select {
		case err := <-resCh: // 杀进程后 Invoke 秒级返回，worker 走清理路径
			return err
		case <-time.After(10 * time.Second):
			Logger().Warn("COM worker 未随进程终止按预期收尾（观测性告警，不阻塞）")
			return ctx.Err()
		}
	}
}

// killPids 按 PID 精确强杀集合（禁 taskkill /IM 按映像名全杀——误杀用户 PPT
// 红线契约不变）。超时/取消收口（killOwnedPowerpnt）与上会话残留回收
// （reclaimResidualInstance，D7）共用单点，reason 供日志对号场景。
func killPids(pids map[int]bool, reason string) {
	for pid := range pids {
		Logger().Warn(reason, "pid", pid)
		_ = util.KillProcess(pid, true)
	}
}

// killOwnedPowerpnt 强杀自启实例（精确 PID；禁按映像名全杀）。
// 归属信息经互斥镜像直读，不取道任务通道——调用时机恰是「worker 阻塞在
// Invoke 上」（超时/取消场景前提），无缓冲 taskCh 的快照请求永不被消费，
// 经通道等待会永久死锁。用户实例复用场景（镜像空）不可杀——记 Warn 后返回，
// waitFor 的 10s 收尾宽限兜底。
func (e *goOleThumbExporter) killOwnedPowerpnt() {
	e.ownedMu.Lock()
	pids := e.ownedMirror // 发布时整体替换新 map，取引用后无锁遍历安全
	e.ownedMu.Unlock()
	if len(pids) == 0 {
		Logger().Warn("超时/取消：无自启 PowerPoint 实例可杀（复用用户实例或会话未建立），等待路径兜底")
		return
	}
	killPids(pids, "超时/取消：强杀自启 PowerPoint 进程")
}

// publishOwned 把 worker 侧权威 ownedPIDs 发布为主侧镜像（整体替换，不原地
// 改旧 map——主侧读引用后无锁遍历）+ 同步落盘持久化文件（D7：归属内存态随
// 崩溃丢失，盘上镜像供下会话识别幽灵残留）。owner 为 nil（排空路径构造的空
// 状态且未触达 ensureApp 成功分支）时跳过。清空发布（releaseApp 路径）同样
// 落盘 {"pids":[]}——本会话已知自身无 owned 实例，旧残留集据此失效。
func (w *comWorkerState) publishOwned() {
	if w.owner == nil {
		return
	}
	w.owner.ownedMu.Lock()
	if len(w.ownedPIDs) == 0 {
		w.owner.ownedMirror = nil
	} else {
		m := make(map[int]bool, len(w.ownedPIDs))
		for pid := range w.ownedPIDs {
			m[pid] = true
		}
		w.owner.ownedMirror = m
	}
	w.owner.ownedMu.Unlock()
	w.owner.persistOwnedMirror() // 锁外落盘：util.SaveJSON 含 fsync，不占主侧强杀路径的读锁窗口
}

// ownedPIDsFilePayload 持久化文件结构（{"pids":[...]}，LoadJSON/SaveJSON 对称）。
type ownedPIDsFilePayload struct {
	PIDs []int `json:"pids"`
}

// ownedPIDsFileMaxAge 残留识别文件有效窗口：超期视同无残留——残留对应进程几乎
// 必然早已退出，且窗口收窄显著压低 PID 复用误判概率（D7 双保险之一，另一重是
// 存活交集按 officeServerProcNames 白名单过滤）。
const ownedPIDsFileMaxAge = 24 * time.Hour

// persistOwnedMirror 把当前 owned 镜像同步落盘（ownedPersistPath 空 = 不持久化，
// 非 Windows 桩/单测构造天然跳过）。util.SaveJSON 原子写（data-atomic-write
// 契约：CreateTemp 同目录 + fsync + rename，崩溃不产生半截文件误判残留，禁
// os.WriteFile 直写）。落盘失败仅 Warn 不致命：镜像内存态仍有效，持久化只是
// 跨会话残留识别的辅助手段，失败降级 = 上会话崩溃后无法识别残留（回归修复前
// 行为）。调用点 = publishOwned（worker 线程，调用天然串行，无并发序问题）。
func (e *goOleThumbExporter) persistOwnedMirror() {
	if e.ownedPersistPath == "" {
		return
	}
	e.ownedMu.Lock()
	pids := e.ownedMirror
	e.ownedMu.Unlock()
	payload := ownedPIDsFilePayload{PIDs: sortedPIDList(pids)}
	if err := util.SaveJSON(e.ownedPersistPath, payload); err != nil {
		Logger().Warn("owned PID 镜像落盘失败（跨会话残留识别降级，不影响本会话）",
			"path", e.ownedPersistPath, "err", err)
	}
}

// loadStaleOwnedPIDs 盘上残留集懒加载（load-once 缓存，ensureApp 建/重建会话
// 前调用——不重复读盘）。先于首个 CreateInstance 是时序要害：本会话一旦成功
// 发布 owned 镜像即覆盖写盘，后加载会读到本会话内容而非上会话崩溃残留（WPS
// 多实例形态下首个 CreateInstance 可能直接新建进程、跳过 diff 空分支，届时
// 不再有加载机会）。锁复用 ownedMu：写入路径 worker 线程独占，主侧仅在超时
// 强杀路径短暂读 ownedMirror，首载的一次性文件读不构成实质争用。
func (e *goOleThumbExporter) loadStaleOwnedPIDs() map[int]bool {
	e.ownedMu.Lock()
	defer e.ownedMu.Unlock()
	if !e.staleOwnedSet {
		e.staleOwnedPIDs = loadOwnedPIDsFile(e.ownedPersistPath)
		e.staleOwnedSet = true
	}
	return e.staleOwnedPIDs
}

// loadOwnedPIDsFile 从持久化文件读残留 PID 集（纯读函数，单测直测）：无文件 =
// 空集合（正常首启）；读失败/JSON 损坏 = 空集合 + Warn（降级为修复前「复用
// 用户实例」语义，不致命）；mtime 超 ownedPIDsFileMaxAge = 空集合（陈旧残留
// 视同无——测试经 os.Chtimes 注入陈旧时间驱动，禁依赖 mtime 写入时序，
// test-stability 契约）。
func loadOwnedPIDsFile(path string) map[int]bool {
	pids := map[int]bool{}
	if path == "" {
		return pids
	}
	info, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			Logger().Warn("读取 owned PID 残留文件失败（按无残留降级）", "path", path, "err", err)
		}
		return pids
	}
	if time.Since(info.ModTime()) > ownedPIDsFileMaxAge {
		return pids
	}
	var payload ownedPIDsFilePayload
	if err := util.LoadJSON(path, &payload); err != nil {
		Logger().Warn("解析 owned PID 残留文件失败（按无残留降级）", "path", path, "err", err)
		return pids
	}
	for _, pid := range payload.PIDs {
		pids[pid] = true
	}
	return pids
}

// removeOwnedPersistFile 删除持久化残留文件（Shutdown 正常收口后调用）：自启
// 实例已 Quit（或强杀兜底），残留识别依据失效，保留只会引入 PID 复用误判——
// 删除。失败仅 Warn（下会话仍有存活交集 + mtime 窗口双重过滤，误杀面已收窄）。
func (e *goOleThumbExporter) removeOwnedPersistFile() {
	if e.ownedPersistPath == "" {
		return
	}
	if err := os.Remove(e.ownedPersistPath); err != nil && !os.IsNotExist(err) {
		Logger().Warn("删除 owned PID 残留文件失败", "path", e.ownedPersistPath, "err", err)
	}
}

// sortedPIDList PID 集合转升序切片（persist 载荷与日志确定性输出，便于对号）。
func sortedPIDList(pids map[int]bool) []int {
	out := make([]int, 0, len(pids))
	for pid := range pids {
		out = append(out, pid)
	}
	sort.Ints(out)
	return out
}

// cleanup 退出自启实例（worker quit 路径）：Quit 温和收口 + 强杀残留兜底。
func (w *comWorkerState) cleanup() {
	if w.app != nil {
		if len(w.ownedPIDs) > 0 {
			_, _ = oleutil.CallMethod(w.app, "Quit")
		}
		w.app.Release()
		w.app = nil
	}
	// 残留兜底：Quit 后 3s 仍存活的自启实例强杀（温和收口失败的防御路径）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !anyPIDAlive(w.ownedPIDs) {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	for pid := range w.ownedPIDs {
		if util.IsProcessAlive(pid) {
			_ = util.KillProcess(pid, true)
		}
	}
}

// anyPIDAlive 任一 PID 存活判定。
func anyPIDAlive(pids map[int]bool) bool {
	if len(pids) == 0 {
		return false
	}
	for pid := range pids {
		if util.IsProcessAlive(pid) {
			return true
		}
	}
	return false
}

// ensureApp 取可用 Application 会话：脏态/未建则重建（CreateInstance + 归属判定
// + 防弹窗三板斧）。失败路径 w.app 已被 releaseApp 置 nil，下次任务自然重建
// （无需显式置脏）。
func (w *comWorkerState) ensureApp() (*ole.IDispatch, error) {
	if w.app != nil && (w.owner == nil || !w.owner.dirty.Load()) {
		return w.app, nil
	}
	// Discard 重建链（修复：Quit 异步——COM 单实例激活会重连尚未退出的旧进程，
	// 真机实测 Discard 后 277ms 内重建即连到垂死 wpp.exe、Export 秒败）：快照
	// owned PID 后 Quit，等待进程实际退出再 CreateInstance。复用用户实例路径
	// （ownedPIDs 空）不等待——禁杀语义下 Quit 本就未调用。
	staleOwned := make(map[int]bool, len(w.ownedPIDs))
	for pid := range w.ownedPIDs {
		staleOwned[pid] = true
	}
	w.releaseApp()
	stillAliveOwned := waitForOwnedProcessesExit(staleOwned)

	// 盘上残留集先于首个 CreateInstance 懒加载（load-once 缓存）：时序要害见
	// loadStaleOwnedPIDs 注释——本会话发布 owned 镜像即覆盖写盘，必须在此之前
	// 读到上会话崩溃时的内容。无 owner（排空路径构造的空状态）跳过——残留回收
	// 本就需要 owner 侧缓存。
	if w.owner != nil {
		_ = w.owner.loadStaleOwnedPIDs()
	}

	// CreateInstance + 归属判定：进程快照 diff 检出新增 PID = 自启实例。diff 空
	// （复用已存活实例）时按盘上残留集识别「上会话自启残留」并回收重建（D7）。
	before := officeServerPIDSet()
	app, err := w.createInstanceRetry()
	if err != nil {
		return nil, err
	}
	after := officeServerPIDSet()
	w.ownedPIDs = diffPIDs(before, after)
	// 垂死实例困局并回（修复 2）：Quit 失败超时未退的 stale owned PID 在 before
	// 快照已在集，diffPIDs 只检新增进程故天然不进 diff——releaseApp 的清空发布
	// 又先于退出确认（内存镜像与盘上证据均已被清空覆盖），不并回则这些本会话
	// 自启实例被误归「复用用户实例」禁杀，垂死进程永不可回收。归属仍属本会话
	// 自证（PID 为本会话 CreateInstance 启动），并回在禁杀语义内；镜像与落盘
	// 证据由函数尾 publishOwned 统一恢复。并回须在 diff 赋值之后，否则被覆盖。
	for pid := range stillAliveOwned {
		w.ownedPIDs[pid] = true
	}
	if len(w.ownedPIDs) == 0 && w.owner != nil {
		// diff 空：单凭快照无法区分「用户已开实例」与「上会话崩溃残留的自启
		// 实例」——后者归属纯内存态随崩溃丢失。盘上残留集 ∩ 存活进程非空 =
		// 归属自证为自有残留，精确强杀后重建一次（防误杀双保险：after 由
		// officeServerPIDSet 按进程名白名单过滤、盘上残留集加载时已过滤 mtime
		// 24h 窗口）。单次回收禁循环重试：重建仍 diff 空（罕见：WPS 单实例激活
		// 落到另一已开实例）按真用户实例复用语义兜底放行，禁杀语义不变。
		if residual := classifyReusedInstance(after, w.owner.loadStaleOwnedPIDs()); len(residual) > 0 {
			// 覆盖写前必须 Release 首个 createInstanceRetry 返回的 IDispatch：残留
			// 回收已把其指向的进程强杀，该指针现为垂死代理，直接被
			// reclaimResidualInstance 的返回值覆盖 = 代理引用泄漏（对齐
			// releaseApp/cleanup 对垂死 proxy 的 Release 先例——Release 只递减本侧
			// 引用计数、不向死进程发新调用）。Release 在覆盖写前执行，回收失败
			// 返回 err 的分支同样不泄漏。
			app.Release()
			app, err = w.reclaimResidualInstance(residual)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(w.ownedPIDs) == 0 {
		Logger().Info("PowerPoint COM 复用用户已开实例（禁 Quit 禁杀，静默只读消费）")
	}
	w.publishOwned() // 发布归属镜像：超时强杀路径（主侧）依赖此处更新

	// 防弹窗三板斧（探针 P1 实测接受；失败不致命——超时兜底）
	_, _ = oleutil.PutProperty(app, "DisplayAlerts", ppAlertsNone)
	_, _ = oleutil.PutProperty(app, "AutomationSecurity", msoAutomationSecurityForceDisable)

	w.app = app
	if w.owner != nil {
		w.owner.dirty.Store(false) // 会话已重建，清脏标记
	}
	return app, nil
}

// createInstanceRetry CreateObject + QueryInterface(IDispatch) 单点（ensureApp
// 首建与残留回收重建共用），带 0x8001010A（RPC_E_SERVERCALL_RETRYLATER，应用忙）
// 重试：语义为「目标应用忙」应等待重试而非判实例死亡（research §3.4）；强杀后
// 的 COM 激活态残留数秒内恢复。重试仍失败按原错误返回（调用方走失败隔离）。
func (w *comWorkerState) createInstanceRetry() (*ole.IDispatch, error) {
	var unknown *ole.IUnknown
	var err error
	for attempt := 0; attempt < comCreateRetries; attempt++ {
		unknown, err = oleutil.CreateObject("PowerPoint.Application")
		if err == nil {
			break
		}
		if oleErrorHresult(err) != 0x8001010A || attempt == comCreateRetries-1 {
			return nil, fmt.Errorf("创建 PowerPoint COM 实例失败 (hr=0x%08X): %w", oleErrorHresult(err), err)
		}
		time.Sleep(comCreateRetryDelay)
	}
	defer unknown.Release()
	app, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, fmt.Errorf("QueryInterface(IDispatch) 失败: %w", err)
	}
	return app, nil
}

// diffPIDs 快照差集 after - before（新出现的 PID = 本次 CreateInstance 自启）。
func diffPIDs(before, after map[int]bool) map[int]bool {
	owned := map[int]bool{}
	for pid := range after {
		if !before[pid] {
			owned[pid] = true
		}
	}
	return owned
}

// classifyReusedInstance 复用判定纯函数：CreateInstance 后存活 Office 进程集合
// 与盘上持久化残留集的交集 = 「上会话自启残留」（可杀，D7）。防误杀双保险在
// 两侧入口各自收口：aliveAfter 由 officeServerPIDSet 产出（进程名白名单已过滤）、
// staleOwned 由盘上文件加载（mtime 24h 窗口已过滤），本函数只做交集。返回残留
// 交集（空 = 无残留证据，按真用户实例复用语义兜底）。
func classifyReusedInstance(aliveAfter, staleOwned map[int]bool) map[int]bool {
	residual := map[int]bool{}
	for pid := range aliveAfter {
		if staleOwned[pid] {
			residual[pid] = true
		}
	}
	return residual
}

// reclaimResidualInstance 上会话残留回收（D7）：精确强杀残留 PID → 等待退出
// （waitForOwnedProcessesExit，200ms 轮询 / 3s 上界——超时未退继续重建走
// CreateInstance 重试链兜底，COM 单实例激活可能重连尚未退净的旧进程）→ 重建
// COM 实例并重算归属（before2 在杀后取，已死残留天然不在集内，新进程归属
// 判定不受干扰）。单次回收，禁循环重试——重建仍复用已开实例（diff 空）由
// 调用方按真用户实例语义兜底放行。归属写入 w.ownedPIDs。强杀失败超时未退的
// 残留 PID 并回归属（垂死实例困局同源）：未退 PID 在 before 快照已在集、天然
// 不进 diff，不并回则重建重连到该垂死进程时被误归「复用用户实例」禁杀（归属
// 证据仍在盘上残留集，归属自证成立），证据落盘由调用方 ensureApp 尾部
// publishOwned 统一完成。
func (w *comWorkerState) reclaimResidualInstance(residual map[int]bool) (*ole.IDispatch, error) {
	Logger().Info("回收上会话残留 Office 实例后重建（归属自证，非用户实例）", "pids", sortedPIDList(residual))
	killPids(residual, "强杀上会话残留 PowerPoint 进程")
	stillAlive := waitForOwnedProcessesExit(residual)
	before := officeServerPIDSet()
	app, err := w.createInstanceRetry()
	if err != nil {
		return nil, fmt.Errorf("回收残留后重建 PowerPoint COM 实例失败: %w", err)
	}
	after := officeServerPIDSet()
	w.ownedPIDs = diffPIDs(before, after)
	for pid := range stillAlive {
		w.ownedPIDs[pid] = true
	}
	return app, nil
}

// releaseApp 释放会话引用（owned 实例温和 Quit；复用实例不动）。
func (w *comWorkerState) releaseApp() {
	if w.app == nil {
		return
	}
	if len(w.ownedPIDs) > 0 {
		_, _ = oleutil.CallMethod(w.app, "Quit")
	}
	w.app.Release()
	w.app = nil
	w.ownedPIDs = map[int]bool{}
	w.publishOwned() // 归属清空同步镜像（防主侧误杀上一会话遗留 PID）
}

// officeServerPIDSet 枚举 PowerPoint 系进程（powerpnt/wpp）PID 集合
// （util.ListProcesses 过滤，跨平台实现复用）。
func officeServerPIDSet() map[int]bool {
	pids := map[int]bool{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	all, err := util.ListProcesses(ctx)
	if err != nil {
		return pids // 枚举失败按空集合：归属判定退化为「无自启实例」（禁杀安全侧）
	}
	for _, p := range all {
		for _, name := range officeServerProcNames {
			if strings.EqualFold(p.Name, name) {
				pids[p.Pid] = true
				break
			}
		}
	}
	return pids
}

// oleErrorHresult 提取 OleError 的 HRESULT（非 OleError 返回哨兵）。
func oleErrorHresult(err error) uintptr {
	if oe, ok := err.(*ole.OleError); ok {
		return oe.Code()
	}
	return 0xFFFFFFFF
}

// isTransientExportError 页导出失败瞬态判定（重试准入口径，输入必须是 go-ole
// 返回的原始错误——调用方勿先 %w 包装再判，直接类型断言会失效）：
//   - scode == DISP_E_EXCEPTION 且 ExcepInfo 为空（description 为空串或 go-ole
//     对 null bstrDescription 的字面量渲染 "<nil>"）——WPS 冷启动、大文件页内容
//     未就绪的实测形态，仅 scode 有信息量；
//   - EXCEPINFO 描述含「发生意外」（WPS/Office 通用异常文案，bstrDescription
//     非空的另一形态）。
//
// ctx 取消/超时错误绝不瞬态（errors.Is 判定前置保证零重试）；其余错误（文件
// 损坏类 OleError、非 COM 包装错误）非瞬态，调用方立即返回不重试。
func isTransientExportError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	oe, ok := err.(*ole.OleError)
	if !ok {
		return false
	}
	desc := oe.Description()
	if strings.Contains(desc, "发生意外") {
		return true
	}
	return oleErrorHresult(err) == dispEException && (desc == "" || desc == "<nil>")
}

// retryTransientSlideExport 页级 Export 瞬态重试：WPS 冷启动、大文件页内容未
// 就绪时 Export 以 DISP_E_EXCEPTION 空 ExcepInfo 瞬态失败（真机实测：41MB/116 页
// 首批「发生意外。」7s 内秒败、本地 COM 探针延后执行成功佐证瞬态），同页同参
// 退避重试至 thumbExportMaxAttempts 次（总尝试含首次，退避 delay*n 线性增长）。
// ctx 取消/超时绝不重试（errors.Is 判定前置 + 退避等待可被 ctx 打断——超时强杀
// 收口路径不被重试拖延）；非瞬态（文件损坏类）立即返回，单次失败错误格式与
// 既有逐字一致。重试耗尽返回最后一次错误并追加「（已重试 N 次）」便于真机
// 定位；每次重试 slog.Warn 一行（page/tier/attempt/err）不刷屏。重试发生在
// Export 调用内部不跨页边界，preempt/jr.Result 契约不受影响。attempt 由调用方
// 闭包注入（单测注入 fake，COM 真调用不进单测）。
func retryTransientSlideExport(ctx context.Context, page int, tierKey string, attempt func() error) error {
	fail := func(err error) error {
		return fmt.Errorf("第 %d 页导出 %s 档失败: %w", page, tierKey, err)
	}
	var lastErr error
	for n := 1; ; n++ {
		if n > 1 && ctx.Err() != nil {
			// 重试准入前查 ctx：取消/超时后零重试，杀进程收口路径的错误即刻上抛
			return fail(lastErr)
		}
		err := attempt()
		if err == nil {
			return nil
		}
		lastErr = err
		if !isTransientExportError(err) {
			return fail(err) // 非瞬态：立即返回，错误格式与既有逐字一致
		}
		if n >= thumbExportMaxAttempts {
			if n > 1 {
				return fail(fmt.Errorf("%w（已重试 %d 次）", err, n-1))
			}
			return fail(err)
		}
		Logger().Warn("页导出瞬态失败，退避重试", "page", page, "tier", tierKey, "attempt", n, "err", err)
		select {
		case <-time.After(thumbExportRetryDelay * time.Duration(n)):
		case <-ctx.Done():
			return fail(lastErr)
		}
	}
}

// waitForOwnedProcessesExit 等待刚 Quit 的 owned 实例进程实际退出（Discard 重建
// 链，ensureApp 在 releaseApp 后调用）：COM 单实例激活会重连尚未退出的垂死进程
// （真机实测 Quit 异步、277ms 内重建即连到垂死 wpp.exe 致 Export 秒败），等待
// 落定是重建语义的一部分。轮询 thumbOwnedExitProbe，间隔 thumbOwnedExitPollInterval、
// 上限 thumbOwnedExitWait；返回超时后仍存活的 PID 子集（全部退出 = 空集）——调用
// 方必须把非空返回值并回 w.ownedPIDs（垂死实例困局：releaseApp 的清空发布先于
// 退出确认，diffPIDs 只检新增进程，不并回则 Quit 失败的残留 owned 实例被误归
// 「复用用户实例」禁杀，垂死实例永不可回收）。超时未退继续重建（CreateInstance
// 0x8001010A 重试链兜底），slog.Warn 记录观测。pids 为空（复用用户实例路径 /
// 无历史会话）不等待不探针——禁杀语义下 Quit 本就未调用。
func waitForOwnedProcessesExit(pids map[int]bool) map[int]bool {
	stillAlive := map[int]bool{}
	if len(pids) == 0 {
		return stillAlive
	}
	for pid := range pids {
		stillAlive[pid] = true
	}
	deadline := time.Now().Add(thumbOwnedExitWait)
	for {
		cur := thumbOwnedExitProbe()
		for pid := range stillAlive {
			if !cur[pid] {
				delete(stillAlive, pid)
			}
		}
		if len(stillAlive) == 0 {
			return stillAlive
		}
		if !time.Now().Before(deadline) {
			Logger().Warn("owned PowerPoint 实例 Quit 后未退出（超等待上限，继续重建走 CreateInstance 重试链兜底）", "pids", sortedPIDList(stillAlive))
			return stillAlive
		}
		time.Sleep(thumbOwnedExitPollInterval)
	}
}

// ---- ThumbExporter 接口实现（主侧） ----

// Available 两阶段探测：注册表预检（ProgID 键存在，不启动进程）→ 缓存；
// 首次真实任务失败（0x800401F3/0x80040154）时回写不可用修正注册表残留误报。
func (e *goOleThumbExporter) Available() bool {
	if e.availableSet.Load() {
		return e.available.Load()
	}
	ok := powerPointRegistered()
	e.available.Store(ok)
	e.availableSet.Store(true)
	return ok
}

// powerPointRegistered 注册表预检：HKCR\PowerPoint.Application 存在即视为
// 已安装（探针实测本机该键存在；卸载残留误报由首次任务失败回写修正）。
func powerPointRegistered() bool {
	k, err := registry.OpenKey(registry.CLASSES_ROOT, `PowerPoint.Application`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	_ = k.Close()
	return true
}

// markUnavailable 首次真实任务报「类未注册/ProgID 无效」时回写缓存（修正残留误报）。
func (e *goOleThumbExporter) markUnavailable() {
	e.available.Store(false)
	e.availableSet.Store(true)
}

// ExportPresentation 打开 pptx（ReadOnly 无窗口）逐页导出双档 PNG。
// 实现细节见 ThumbExporter 接口注释（assets_thumbs.go）。
func (e *goOleThumbExporter) ExportPresentation(ctx context.Context, spec thumbFileSpec, thumbsDir string, tiers []thumbTier, preempt func() *thumbJumpRequest) (int, error) {
	resCh := make(chan error, 1)
	pagesCh := make(chan int, 1)
	task := func(w *comWorkerState) {
		pages, err := w.exportSession(ctx, spec, thumbsDir, tiers, preempt)
		pagesCh <- pages
		resCh <- err
	}
	if err := e.submit(ctx, task); err != nil {
		return 0, err
	}
	err := e.waitFor(ctx, resCh)
	if err != nil {
		if isComClassError(err) {
			e.markUnavailable()
		}
		return 0, err
	}
	return <-pagesCh, nil
}

// JumpOnly 独立跳转（无批量在途路径）。结果写回 jr.Result（含错误）。
func (e *goOleThumbExporter) JumpOnly(jr *thumbJumpRequest) error {
	resCh := make(chan error, 1)
	task := func(w *comWorkerState) {
		err := w.jumpSession(jr)
		jr.Result <- err
		resCh <- err
	}
	ctx, cancel := context.WithTimeout(context.Background(), jumpTimeout)
	defer cancel()
	if err := e.submit(ctx, task); err != nil {
		jr.Result <- err
		return err
	}
	err := e.waitFor(ctx, resCh)
	if isComClassError(err) {
		e.markUnavailable()
	}
	return err
}

// Discard 失败隔离：置脏标记，worker 下个任务前重建 COM 会话（旧 owned 实例
// 随重建 Quit——会话已断连/损坏，温和 Quit 多半失败，无害）。主侧置位、worker
// ensureApp 检查并清位，跨线程经 exporter 实例 atomic 字段（实例级状态，无
// 包级变量多实例互染隐患）。
func (e *goOleThumbExporter) Discard() {
	e.dirty.Store(true)
}

// Shutdown 关停收口：停 worker（内部 Quit 自启实例）+ 残留强杀兜底。
// sync.Once 全包裹：并发/重复调用幂等——「select default 判已收口 → close」
// 非原子，双重 close(e.quit) 会 panic（check P2）；等待与兜底语义由首次调用
// 承担，后续调用等首次完成后原样返回（Do 阻塞至首次 f 返回）。
func (e *goOleThumbExporter) Shutdown() {
	e.shutdownOnce.Do(func() {
		e.startWorker() // 未启动过则启动——quit close 前确保通道语义一致
		close(e.quit)
		select {
		case <-e.workerDone:
		case <-time.After(5 * time.Second):
			// worker 卡死（如复用用户实例被模态弹窗阻塞）：按归属镜像强杀自启
			// 残留兜底（镜像空 = 复用用户实例，不可杀——不误伤）
			Logger().Warn("COM worker 关停超时（按归属镜像强杀自启残留兜底）")
			e.killOwnedPowerpnt()
		}
		// worker 正常退出路径由 cleanup 收口（Quit + 3s 强杀兜底），此处不重复
		// 枚举进程（避免误杀用户实例）。
		// 残留识别文件清理（D7）：正常收口后自启实例已 Quit，盘上残留集失效，
		// 保留只会引入下会话 PID 复用误判——删除（崩溃路径不执行本步，文件
		// 恰为下会话识别幽灵残留的依据）。
		e.removeOwnedPersistFile()
	})
}

// isComClassError 判定「PowerPoint 未安装」类错误（首次任务修正探测缓存）。
func isComClassError(err error) bool {
	switch oleErrorHresult(err) {
	case 0x800401F3, 0x80040154: // CO_E_CLASSSTRING / REGDB_E_CLASSNOTREG
		return true
	}
	return false
}

// ---- COM 任务实现（worker STA 线程内执行） ----

// exportSession 单文件导出会话：复用检查 → ReadOnly 无窗口打开 → 逐页双档
// Export（页级瞬态重试 + 页间 preempt 跳转插队）→ Close（复用的不 Close）。
// ctx 供页级重试的准入判定与退避等待消费（取消/超时零重试）。返回导出页数。
func (w *comWorkerState) exportSession(ctx context.Context, spec thumbFileSpec, thumbsDir string, tiers []thumbTier, preempt func() *thumbJumpRequest) (int, error) {
	app, err := w.ensureApp()
	if err != nil {
		return 0, err
	}
	// 档位目录预建（幂等）：Slide.Export 不自建父目录，目标目录缺失时 E_FAIL
	// （「发生意外」无定位信息——真机冒烟实测抓出，探针预建目录曾掩蔽）
	for _, tier := range tiers {
		if err := os.MkdirAll(filepath.Join(thumbsDir, tier.Key), 0o755); err != nil {
			return 0, fmt.Errorf("创建缩略图目录失败（%s）: %w", tier.Key, err)
		}
	}
	pres, reused, err := w.findOrOpen(app, spec.AbsPath, msoTrue, msoFalse)
	if err != nil {
		return 0, err
	}
	// Release 恒做（复用与否）：findOrOpen 返回的接口指针是本侧自有引用——
	// 复用路径只是不 Close 用户的 Presentation（Close 属归属操作），Release
	// 释放的是本侧 COM 代理引用，与归属无关，不释放 = 代理引用泄漏累积
	// （check P2）。defer LIFO：先注册 Release 后注册 Close → 执行序 Close
	// （让 PowerPoint 关闭演示文稿）先于 Release（释放本侧 COM 引用）——顺序
	// 颠倒会对已 Release 的指针调 Close，违反 COM 引用规则（代理断连时表现
	// 为 RPC_E_DISCONNECTED 等未定义行为）。
	defer pres.Release()
	if !reused {
		defer oleutil.CallMethod(pres, "Close")
	}

	slidesV, err := oleutil.GetProperty(pres, "Slides")
	if err != nil {
		return 0, fmt.Errorf("取 Slides 集合失败: %w", err)
	}
	slides := slidesV.ToIDispatch()
	defer slides.Release()
	countV, err := oleutil.GetProperty(slides, "Count")
	if err != nil {
		return 0, fmt.Errorf("取页数失败: %w", err)
	}
	count := int(countV.Val)

	exported := 0
	for i := 1; i <= count; i++ {
		slideV, err := oleutil.CallMethod(slides, "Item", i)
		if err != nil {
			return exported, fmt.Errorf("取第 %d 页失败: %w", i, err)
		}
		slide := slideV.ToIDispatch()
		for _, tier := range tiers {
			pngPath := thumbPNGPath(thumbsDir, tier.Key, spec.FileID, i)
			// 页级瞬态重试（WPS 冷启动/大文件页内容未就绪 DISP_E_EXCEPTION 瞬态，
			// 真机 41MB/116 页首批导出「发生意外。」秒败、探针延后执行成功佐证）。
			// 重试在 Export 调用内部不跨页边界，preempt/jr.Result 契约不受影响。
			if err := retryTransientSlideExport(ctx, i, tier.Key, func() error {
				_, err := oleutil.CallMethod(slide, "Export", pngPath, "PNG", tier.Width, tier.Height)
				return err
			}); err != nil {
				slide.Release()
				return exported, err
			}
		}
		slide.Release()
		exported = i

		// 页间跳转插队（PRD D3）：批量在途时用户跳转请求在页边界执行，
		// 用户窗口与本会话导图并存。跳转失败不中断导出（记日志继续）；
		// jr.Result 无论成败必回写——JumpToSlide 单读者阻塞等待，漏写即永挂。
		if preempt != nil {
			if jr := preempt(); jr != nil {
				jerr := w.jumpInSession(app, jr)
				jr.Result <- jerr
				if jerr != nil {
					Logger().Warn("页间插队跳转失败", "path", jr.AbsPath, "slide", jr.SlideNum, "err", jerr)
				}
			}
		}
	}
	return exported, nil
}

// jumpSession 独立跳转会话：复用检查 → WithWindow 打开 → GotoSlide → Activate。
// Presentation 不关闭（留给用户浏览）。
func (w *comWorkerState) jumpSession(jr *thumbJumpRequest) error {
	app, err := w.ensureApp()
	if err != nil {
		return err
	}
	return w.jumpInSession(app, jr)
}

// jumpInSession 跳转核心（jumpSession 与 exportSession preempt 共用）。
func (w *comWorkerState) jumpInSession(app *ole.IDispatch, jr *thumbJumpRequest) error {
	// 跳转场景 WithWindow:=msoTrue（无窗口无 View 无法跳页——research §2）
	pres, reused, err := w.findOrOpen(app, jr.AbsPath, msoTrue, msoTrue)
	if err != nil {
		return err
	}
	defer pres.Release() // 仅释放引用；Presentation 留给用户浏览

	// COM 激活的 WPS 实例应用级默认不可见：PowerPoint 的 Open(WithWindow:=msoTrue)
	// 顺带显示窗口，WPS 兼容层无此行为（真机实测全新实例 Open/GotoSlide 全成功但
	// 零可见窗口）——显式置应用可见；PowerPoint 已可见时幂等无副作用，失败不致命
	// （Warn 后继续窗口跳页，部分 WPS 版本可能在窗口级仍不可见）。exportSession
	// 的 preempt 插队跳转共用本函数同获兜底；ReadOnly 无窗口导出路径不经此处，
	// 不受影响。
	if _, err := oleutil.PutProperty(app, "Visible", msoTrue); err != nil {
		Logger().Warn("置应用可见失败（跳转窗口可能不可见）", "err", err)
	}

	// ActiveWindow.View.GotoSlide(n)（编辑窗口跳页；SlideShowView 是放映接口勿用）
	winV, err := oleutil.GetProperty(app, "ActiveWindow")
	if err != nil {
		return fmt.Errorf("取 ActiveWindow 失败（WithWindow 打开应有窗口）: %w", err)
	}
	win := winV.ToIDispatch()
	defer win.Release()
	viewV, err := oleutil.GetProperty(win, "View")
	if err != nil {
		return fmt.Errorf("取窗口 View 失败: %w", err)
	}
	view := viewV.ToIDispatch()
	defer view.Release()
	if _, err := oleutil.CallMethod(view, "GotoSlide", jr.SlideNum); err != nil {
		return fmt.Errorf("跳转到第 %d 页失败: %w", jr.SlideNum, err)
	}
	if _, err := oleutil.CallMethod(win, "Activate"); err != nil {
		Logger().Warn("窗口置前失败（前台锁压制，可忽略）", "err", err)
	}
	// 成功路径日志（D7）：此前成功无任何日志，跳转在幽灵后台实例内「成功」时
	// 零可观测痕迹，排障无从下手；失败路径已有错误返回链路，不在此重复。reused
	// 字段（D7 决策 3「复用与否」）：true = 命中已开实例复用，false = 新开窗口
	// ——真机区分「跳转落在用户已开实例」与「自启实例打开」的关键定位线索。
	Logger().Info("缩略图跳转完成", "file", filepath.Base(jr.AbsPath), "slide", jr.SlideNum, "reused", reused)
	return nil
}

// findOrOpen 打开前复用检查：Presentations 集合按 FullName 匹配（用户实例已开
// 同名文件时 Presentations.Open 二次打开报错——探针 P8 场景），命中直接复用
// （只读消费，不 Close 不 Quit 别人的）。reused=true 时调用方禁 Close。
func (w *comWorkerState) findOrOpen(app *ole.IDispatch, absPath string, readOnly, withWindow int) (*ole.IDispatch, bool, error) {
	presColV, err := oleutil.GetProperty(app, "Presentations")
	if err != nil {
		return nil, false, fmt.Errorf("取 Presentations 集合失败: %w", err)
	}
	presCol := presColV.ToIDispatch()
	defer presCol.Release()

	countV, err := oleutil.GetProperty(presCol, "Count")
	if err == nil {
		for i := 1; i <= int(countV.Val); i++ {
			itemV, err := oleutil.CallMethod(presCol, "Item", i)
			if err != nil {
				continue
			}
			item := itemV.ToIDispatch()
			fnV, err := oleutil.GetProperty(item, "FullName")
			if err != nil {
				item.Release()
				continue
			}
			fullName, _ := fnV.Value().(string)
			_ = fnV.Clear() // 释放 VT_BSTR（先取值后 Clear，漏 Clear = BSTR 泄漏累积——check P2）
			if strings.EqualFold(fullName, absPath) {
				return item, true, nil // 复用（FullName 匹配大小写不敏感）
			}
			item.Release()
		}
	}

	pV, err := oleutil.CallMethod(presCol, "Open", absPath, readOnly, msoFalse, withWindow)
	if err != nil {
		return nil, false, fmt.Errorf("打开 %s 失败 (hr=0x%08X): %w", filepath.Base(absPath), oleErrorHresult(err), err)
	}
	return pV.ToIDispatch(), false, nil
}
