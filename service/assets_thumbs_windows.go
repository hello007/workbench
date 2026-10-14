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
//   - 防弹窗三板斧先于一切 Open：DisplayAlerts=1（ppAlertsNone，探针 P1 实测
//     接受）+ AutomationSecurity=3（禁宏）+ 恒 ReadOnly；
//   - 归属判定：CreateInstance 前后 PowerPoint 进程快照 diff（util.ListProcesses
//     过滤 powerpnt.exe/wpp.exe 双进程名），新增 PID = 自启实例（可 Quit/可强杀）；
//     diff 为空 = 复用用户实例（禁 Quit 禁杀，静默只读消费）；禁 taskkill /IM
//     全杀（误杀用户 PPT）；
//   - 超时/取消：阻塞 Invoke 不可中断——主侧 ctx 超时杀自启 PID 让 Invoke 以
//     0x800706BA 返回，worker 走清理路径（research §3.2 推荐形态）；
//   - 占用规避走预防：恒 ReadOnly + 打开前 Presentations 集合按 FullName 复用。

package service

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
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

// goOleThumbExporter ThumbExporter 的 Windows COM 实现。
//
// 线程模型：主侧方法（ExportPresentation/JumpOnly/…）只做任务提交与结果等待；
// 全部 COM 调用在 workerLoop 常驻 STA goroutine 内执行（comWorkerState 持有
// Application 会话）。主侧超时/取消以「杀自启 powerpnt」驱动 worker 内阻塞
// Invoke 返回（Go 无法中断 syscall，research §3.2）。
type goOleThumbExporter struct {
	taskCh     chan func(*comWorkerState) // STA worker 任务通道（无缓冲：提交即等待交接）
	quit       chan struct{}
	workerDone chan struct{}

	available    atomic.Bool // 可用性缓存（注册表预检 + CreateInstance 兜底修正）
	availableSet atomic.Bool // 缓存是否已初始化（零值 false 不等于「不可用」）
	dirty        atomic.Bool // 会话脏标记（主侧 Discard 置位 / worker ensureApp 检查并清位——跨线程经实例 atomic，不用包级变量防多实例互染）
	startOnce    sync.Once

	ownedMu     sync.Mutex   // 保护 ownedMirror（worker 线程写、主侧杀进程路径读）
	ownedMirror map[int]bool // 自启 PID 镜像：超时强杀路径在 worker 阻塞于 Invoke 时无法经任务通道取快照（通道请求永不被消费），镜像让主侧无需 worker 配合即可定位自启 PID。发布时整体替换新 map，读侧持锁取引用后可无锁遍历
}

// comWorkerState STA worker 线程内会话状态（app/ownedPIDs 仅 worker goroutine
// 访问，无锁；owner 反向引用供 publishOwned 发布镜像）。
type comWorkerState struct {
	app       *ole.IDispatch
	ownedPIDs map[int]bool        // 自启实例（CreateInstance 快照 diff 检出；空 = 复用用户实例）
	owner     *goOleThumbExporter // 反向引用：发布 owned PID 镜像（nil 时 publishOwned 跳过）
}

// newThumbExporter 平台分叉构造点（无 tag 侧调用）。
func newThumbExporter() ThumbExporter {
	e := &goOleThumbExporter{
		taskCh:     make(chan func(*comWorkerState)),
		quit:       make(chan struct{}),
		workerDone: make(chan struct{}),
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
			// 排空任务通道：主侧 select 等待会超时收口
			for {
				select {
				case t := <-e.taskCh:
					// 任务无法执行（无 STA）：调用方经 resCh 超时或任务自行报错——
					// 约定任务闭包内首查 w.app，此处直接以空状态执行让任务报错
					// （owner 必填：任务内 ensureApp 失败路径经 publishOwned 发布镜像）
					t(&comWorkerState{owner: e})
				case <-e.quit:
					return
				}
			}
		}
	}
	defer ole.CoUninitialize()

	w := &comWorkerState{ownedPIDs: map[int]bool{}, owner: e}
	for {
		select {
		case t := <-e.taskCh:
			t(w)
		case <-e.quit:
			w.cleanup()
			return
		}
	}
}

// submit 提交任务到 STA worker（阻塞至交接成功，不等执行完成——结果经任务
// 写入的 resCh 由 waitFor 等待）。ctx 仅约束提交等待；COM 调用不可中断——
// ctx 触发时杀自启实例促使 worker 内 Invoke 返回（用户实例复用场景不可杀，
// 仅放弃等待并告警）。
func (e *goOleThumbExporter) submit(ctx context.Context, task func(*comWorkerState)) error {
	e.startWorker()
	select {
	case e.taskCh <- task:
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
	for pid := range pids {
		Logger().Warn("超时/取消：强杀自启 PowerPoint 进程", "pid", pid)
		_ = util.KillProcess(pid, true)
	}
}

// publishOwned 把 worker 侧权威 ownedPIDs 发布为主侧镜像（整体替换，不原地
// 改旧 map——主侧读引用后无锁遍历）。owner 为 nil（排空路径构造的空状态且
// 未触达 ensureApp 成功分支）时跳过。
func (w *comWorkerState) publishOwned() {
	if w.owner == nil {
		return
	}
	w.owner.ownedMu.Lock()
	defer w.owner.ownedMu.Unlock()
	if len(w.ownedPIDs) == 0 {
		w.owner.ownedMirror = nil
		return
	}
	m := make(map[int]bool, len(w.ownedPIDs))
	for pid := range w.ownedPIDs {
		m[pid] = true
	}
	w.owner.ownedMirror = m
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
	w.releaseApp()

	// CreateInstance 带 0x8001010A（RPC_E_SERVERCALL_RETRYLATER，应用忙）重试：
	// 语义为「目标应用忙」应等待重试而非判实例死亡（research §3.4）；强杀后的
	// COM 激活态残留数秒内恢复。重试仍失败按原错误返回（调用方走失败隔离）。
	before := officeServerPIDSet()
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
	after := officeServerPIDSet()
	w.ownedPIDs = map[int]bool{}
	for pid := range after {
		if !before[pid] {
			w.ownedPIDs[pid] = true
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
		pages, err := w.exportSession(spec, thumbsDir, tiers, preempt)
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
func (e *goOleThumbExporter) Shutdown() {
	select {
	case <-e.quit:
		return // 已收口
	default:
	}
	e.startWorker() // 未启动过则无需收口——quit close 前确保通道语义一致
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
// Export（页间 preempt 跳转插队）→ Close（复用的不 Close）。返回导出页数。
func (w *comWorkerState) exportSession(spec thumbFileSpec, thumbsDir string, tiers []thumbTier, preempt func() *thumbJumpRequest) (int, error) {
	app, err := w.ensureApp()
	if err != nil {
		return 0, err
	}
	pres, reused, err := w.findOrOpen(app, spec.AbsPath, msoTrue, msoFalse)
	if err != nil {
		return 0, err
	}
	releasable := !reused // 复用的 Presentation 是用户的，不 Close 不 Release 归属
	if releasable {
		// defer LIFO：先 Close（让 PowerPoint 关闭演示文稿）后 Release（释放本
		// 侧 COM 引用）——顺序颠倒会对已 Release 的指针调 Close，违反 COM 引用
		// 规则（代理断连时表现为 RPC_E_DISCONNECTED 等未定义行为）
		defer pres.Release()
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
			if _, err := oleutil.CallMethod(slide, "Export", pngPath, "PNG", tier.Width, tier.Height); err != nil {
				slide.Release()
				return exported, fmt.Errorf("第 %d 页导出 %s 档失败: %w", i, tier.Key, err)
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
	pres, _, err := w.findOrOpen(app, jr.AbsPath, msoTrue, msoTrue)
	if err != nil {
		return err
	}
	defer pres.Release() // 仅释放引用；Presentation 留给用户浏览

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
