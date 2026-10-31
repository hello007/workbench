//go:build windows

// PowerPoint COM 探针——第二阶段（任务 10-30-powerpoint-com）：多实例并行导出
// 与整本导出可行性。
//
// 与 P1-P8 的分工：P1-P8 定的是「单实例能不能稳定导出」，本文件定的是
// 「能不能并发」「能不能一次导出整本」。两项都只作决策依据，不作生产契约。
//
// 前置：运行前关闭全部 PowerPoint / WPS 演示窗口。P9-P13 启动即检测残留进程，
// 命中直接 SKIP（禁无差别 taskkill /IM，旧 quitSoon 不在本文件使用）。
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// ---- 运行配置（main.go 解析 flag 后写入）----

var (
	probeSampleFile string // -file：真实业务 pptx；空 = 用 COM 自产 fixture
	probeReps       = 3    // -reps：每档重复次数
	probePageLimit  = 12   // -pages：参与计时的页数上限
	probeJSONPath   string // -json：结构化结果落盘路径
)

// 缩略图生产口径尺寸（与 model.AssetThumbsSettings 默认值一致）。
const (
	probeSmallW, probeSmallH = 320, 180
	probeHDW, probeHDH       = 1600, 900
)

// PowerPoint.Application 的 CLSID（P9 直调 CoCreateInstance 用）。
const powerPointAppCLSID = "{91493441-5A91-11CF-8700-00AA0060263B}"

// ---- 结构化结果收集 ----

type probeRecord struct {
	ID      int            `json:"id"`
	Name    string         `json:"name"`
	Verdict string         `json:"verdict"`
	Metrics map[string]any `json:"metrics,omitempty"`
	Error   string         `json:"error,omitempty"`
	At      string         `json:"at"`
}

var (
	probeRecsMu  sync.Mutex
	probeRecs    []probeRecord
	probeEnvInfo map[string]any
)

// recordProbe 记录结果并返回给 main 打印的结论行。
func recordProbe(id int, name, verdict string, metrics map[string]any, err error) string {
	rec := probeRecord{
		ID: id, Name: name, Verdict: verdict, Metrics: metrics,
		At: time.Now().Format(time.RFC3339),
	}
	if err != nil {
		rec.Error = err.Error()
	}
	probeRecsMu.Lock()
	probeRecs = append(probeRecs, rec)
	probeRecsMu.Unlock()
	if err != nil {
		return "FAIL: " + err.Error()
	}
	return verdict
}

// writeProbeJSON 落盘全部结果（含环境头）。
func writeProbeJSON(path string) error {
	probeRecsMu.Lock()
	payload := map[string]any{
		"env":     probeEnvInfo,
		"records": append([]probeRecord(nil), probeRecs...),
	}
	probeRecsMu.Unlock()
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	fmt.Printf("结构化结果已写入: %s\n", path)
	return nil
}

// ---- 环境探测 ----

// detectOfficeEnv 探测本机装的是 MS PowerPoint 还是 WPS，以及样例文件信息。
// 结论适用范围只对探测到的引擎成立。
func detectOfficeEnv() map[string]any {
	env := map[string]any{
		"os":   runtime.GOOS,
		"time": time.Now().Format(time.RFC3339),
	}
	cands := officeExeCandidates()
	env["exeCandidates"] = cands
	msOffice, wps := false, false
	for _, c := range cands {
		lower := strings.ToLower(c)
		if strings.HasSuffix(lower, "powerpnt.exe") {
			msOffice = true
		}
		if strings.HasSuffix(lower, "wpp.exe") {
			wps = true
		}
	}
	env["msOfficeInstalled"] = msOffice
	env["wpsInstalled"] = wps
	env["sampleFile"] = probeSampleFile
	if probeSampleFile != "" {
		if fi, err := os.Stat(probeSampleFile); err == nil {
			env["sampleSizeBytes"] = fi.Size()
		}
	}
	probeEnvInfo = env
	return env
}

// officeExeCandidates 枚举本机可能存在的演示类 COM 服务器可执行文件。
// 同时覆盖 MS Office 与 WPS 的常见安装布局。
func officeExeCandidates() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" {
			return
		}
		if _, err := os.Stat(p); err != nil {
			return
		}
		key := strings.ToLower(p)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, p)
	}
	roots := []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		os.Getenv("LOCALAPPDATA"),
	}
	patterns := []string{
		filepath.Join("Microsoft Office", "*", "POWERPNT.EXE"),
		filepath.Join("Microsoft Office", "root", "Office*", "POWERPNT.EXE"),
		filepath.Join("Kingsoft", "WPS Office", "*", "office6", "wpp.exe"),
	}
	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, pat := range patterns {
			matches, err := filepath.Glob(filepath.Join(root, pat))
			if err != nil {
				continue
			}
			for _, m := range matches {
				add(m)
			}
		}
	}
	sort.Strings(out)
	return out
}

// requireCleanOfficeEnv 前置检查：存在任何演示类 Office 进程即拒跑。
func requireCleanOfficeEnv() error {
	pids := officeServerPids()
	if len(pids) == 0 {
		return nil
	}
	return fmt.Errorf("检测到 %d 个 Office 演示进程在运行（PID %v），请先全部关闭后重跑", len(pids), pidKeys(pids))
}

// waitNoOfficeProcess 轮询等待演示类进程退出（不杀）。
// 用途：quitApp 后立刻新建会话会重连尚未退净的旧实例，表现为
// 「RPC 服务器不可用 / 远程过程调用失败」——须等进程真正退出。
func waitNoOfficeProcess(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(officeServerPids()) == 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// quitAppAndWait 退出实例并等进程退净；超时未退按 PID 强杀（前置干净环境下
// 这些进程全部是本探针自启）。
func quitAppAndWait(app *ole.IDispatch, timeout time.Duration) {
	quitApp(app)
	waitNoOfficeProcess(timeout)
	if left := pidKeys(officeServerPids()); len(left) > 0 {
		killPids(left)
		_ = waitPidsExit(left, 3*time.Second)
	}
}

// killPids 按 PID 精确强杀（本探针只杀自己 diff 出来的进程）。
func killPids(pids []int) {
	for _, pid := range pids {
		_ = taskkillPid(pid)
	}
}

// waitPidsExit 轮询等待 PID 退出，返回仍存活的子集。
func waitPidsExit(pids []int, timeout time.Duration) []int {
	deadline := time.Now().Add(timeout)
	for {
		alive := map[int]bool{}
		cur := officeServerPids()
		for _, pid := range pids {
			if cur[pid] {
				alive[pid] = true
			}
		}
		if len(alive) == 0 || time.Now().After(deadline) {
			return pidKeys(alive)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// createAppOnCurrentThread 在当前 STA 线程再建一个 Application 实例（不初始化
// COM，调用方须已在 withComSession 内）。
func createAppOnCurrentThread() (*ole.IDispatch, error) {
	for attempt := 0; attempt < 5; attempt++ {
		unknown, err := oleutil.CreateObject("PowerPoint.Application")
		if err == nil {
			app, qiErr := unknown.QueryInterface(ole.IID_IDispatch)
			unknown.Release()
			if qiErr != nil {
				return nil, fmt.Errorf("QueryInterface 失败: %w", qiErr)
			}
			_, _ = oleutil.PutProperty(app, "DisplayAlerts", 1)
			_, _ = oleutil.PutProperty(app, "AutomationSecurity", 3)
			return app, nil
		}
		if oleErrorHresult(err) != 0x8001010A || attempt == 4 {
			return nil, err
		}
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("CreateObject 重试耗尽")
}

// listPresentations 枚举实例内已打开的文档 FullName。
// 注意：WPS 的 Presentations.Count 不可靠（开着 1 个报 0），枚举结果为空
// 只能作「未观测到」而非「确定未共享」。
func listPresentations(app *ole.IDispatch) []string {
	out := []string{}
	colV, err := oleutil.GetProperty(app, "Presentations")
	if err != nil {
		return out
	}
	col := colV.ToIDispatch()
	defer col.Release()
	n := 0
	if cv, e := oleutil.GetProperty(col, "Count"); e == nil && cv != nil {
		n = int(cv.Val)
	}
	if n > 50 {
		n = 50
	}
	for i := 1; i <= n; i++ {
		v, e := oleutil.CallMethod(col, "Item", i)
		if e != nil {
			continue
		}
		d := v.ToIDispatch()
		if fn, e2 := oleutil.GetProperty(d, "FullName"); e2 == nil && fn != nil {
			out = append(out, fmt.Sprint(fn.Value()))
		}
		d.Release()
	}
	return out
}

// ---- 统计辅助 ----

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// statsMs 对毫秒采样求 n/min/median/p95/max。
func statsMs(vals []float64) map[string]any {
	if len(vals) == 0 {
		return map[string]any{"n": 0}
	}
	s := append([]float64(nil), vals...)
	sort.Float64s(s)
	return map[string]any{
		"n":      len(s),
		"min":    round2(s[0]),
		"median": round2(percentile(s, 0.5)),
		"p95":    round2(percentile(s, 0.95)),
		"max":    round2(s[len(s)-1]),
	}
}

func medianMs(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	s := append([]float64(nil), vals...)
	sort.Float64s(s)
	return percentile(s, 0.5)
}

// ---- 通用导出辅助 ----

// sampleFilePath 返回本次耗时的样例文件：优先 -file，否则 COM 自产 fixture。
// 合成 fixture 无图片/图表，耗时会偏乐观，调用方须在结论里标注。
func sampleFilePath(dir string) (path string, synthetic bool, err error) {
	if probeSampleFile != "" {
		if _, statErr := os.Stat(probeSampleFile); statErr == nil {
			abs, absErr := filepath.Abs(probeSampleFile)
			if absErr != nil {
				return "", false, absErr
			}
			return abs, false, nil
		}
		return "", false, fmt.Errorf("样例文件不存在: %s", probeSampleFile)
	}
	fx := filepath.Join(dir, "sample-synthetic.pptx")
	abs, _ := filepath.Abs(fx)
	err = withComSession(func(app *ole.IDispatch) error {
		if err := comMakeFixture(app, abs, 8, true); err != nil {
			return err
		}
		quitAppAndWait(app, 5*time.Second)
		return nil
	})
	if err != nil {
		return "", true, err
	}
	return abs, true, nil
}

// slideCountOf 经 COM 取真实页数（比 zip 计数更接近导出侧口径）。
func slideCountOf(path string) (int, error) {
	var n int
	err := withComSession(func(app *ole.IDispatch) error {
		pres, err := openPresentation(app, path, true, false)
		if err != nil {
			return err
		}
		defer pres.Release()
		defer oleutil.CallMethod(pres, "Close")
		slidesV, err := oleutil.GetProperty(pres, "Slides")
		if err != nil {
			return err
		}
		slides := slidesV.ToIDispatch()
		defer slides.Release()
		cv, err := oleutil.GetProperty(slides, "Count")
		if err != nil {
			return err
		}
		n = int(cv.Val)
		quitAppAndWait(app, 5*time.Second)
		return nil
	})
	return n, err
}

// slideCountLite 取页数：优先 zip 计数（零 COM 会话，避免为计数多建一次
// 实例后立刻重连垂死进程），失败再退回 COM。
func slideCountLite(path string) (int, error) {
	zr, err := zip.OpenReader(path)
	if err == nil {
		defer zr.Close()
		n := 0
		for _, f := range zr.File {
			name := filepath.ToSlash(f.Name)
			if strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml") {
				n++
			}
		}
		if n > 0 {
			return n, nil
		}
	}
	return slideCountOf(path)
}

// pickPages 抽取首/中/尾等间隔页（1-based），去重并钳制到 [1,total]。
func pickPages(total, want int) []int {
	if total <= 0 {
		return nil
	}
	if want > total {
		want = total
	}
	seen := map[int]bool{}
	out := []int{}
	for i := 0; i < want; i++ {
		p := 1
		if want > 1 {
			p = 1 + i*(total-1)/(want-1)
		}
		if p < 1 {
			p = 1
		}
		if p > total {
			p = total
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// exportSeries 在一次 Open 会话内导出指定页列表（small 档 PNG），返回墙钟
// 耗时、成功数、失败数与首个错误。用于 P10 的串行/并发对比。
func exportSeries(path string, pages []int, outDir string, quitAtEnd bool) (time.Duration, int, int, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, 0, 0, err
	}
	start := time.Now()
	okCount, failCount := 0, 0
	var firstErr error
	err := withComSession(func(app *ole.IDispatch) error {
		if quitAtEnd {
			defer quitAppAndWait(app, 5*time.Second)
		}
		pres, err := openPresentation(app, path, true, false)
		if err != nil {
			return err
		}
		defer pres.Release()
		defer oleutil.CallMethod(pres, "Close")
		slidesV, err := oleutil.GetProperty(pres, "Slides")
		if err != nil {
			return err
		}
		slides := slidesV.ToIDispatch()
		defer slides.Release()
		for _, page := range pages {
			sv, err := oleutil.CallMethod(slides, "Item", page)
			if err != nil {
				failCount++
				if firstErr == nil {
					firstErr = fmt.Errorf("Slides.Item(%d): %w", page, err)
				}
				continue
			}
			slide := sv.ToIDispatch()
			out := filepath.Join(outDir, fmt.Sprintf("%03d.png", page))
			if _, e := oleutil.CallMethod(slide, "Export", out, "PNG", probeSmallW, probeSmallH); e != nil {
				failCount++
				if firstErr == nil {
					firstErr = fmt.Errorf("Export(%d): %w (desc=%s)", page, e, oleDesc(e))
				}
			} else {
				okCount++
			}
			slide.Release()
		}
		return nil
	})
	if err != nil && firstErr == nil {
		firstErr = err
	}
	return time.Since(start), okCount, failCount, firstErr
}

// ---- P9：多实例 CreateObject 行为 ----

// probe9MultiInstance 判定并行前置：多次 CreateObject 是否落在独立进程，
// 以及是否存在强制新建独立实例的可用路径。
func probe9MultiInstance(dir string) string {
	if err := requireCleanOfficeEnv(); err != nil {
		return "SKIP: " + err.Error()
	}
	waitNoOfficeProcess(5 * time.Second)
	metrics := map[string]any{}
	before := officeServerPids()

	// (a) 同一 STA 线程连续三次 CreateObject：PID 是否随次数增加。
	seqPIDs := make([][]int, 0, 3)
	crossVisible := false
	var crossDetail []string
	var seqErr error
	err := withComSession(func(app1 *ole.IDispatch) error {
		seqPIDs = append(seqPIDs, diffPids(before, officeServerPids()))

		app2, err := createAppOnCurrentThread()
		if err != nil {
			return fmt.Errorf("第二次 CreateObject 失败: %w", err)
		}
		defer app2.Release()
		seqPIDs = append(seqPIDs, diffPids(before, officeServerPids()))

		app3, err := createAppOnCurrentThread()
		if err != nil {
			return fmt.Errorf("第三次 CreateObject 失败: %w", err)
		}
		defer app3.Release()
		seqPIDs = append(seqPIDs, diffPids(before, officeServerPids()))

		// 交叉可见性：app2 打开文档后 app1 是否能看到同一文档。
		fx := filepath.Join(dir, "p9-shared.pptx")
		if abs, e := filepath.Abs(fx); e == nil {
			fx = abs
		}
		if err := comMakeFixture(app2, fx, 2, true); err != nil {
			return fmt.Errorf("自产 fixture 失败: %w", err)
		}
		pres, err := openPresentation(app2, fx, true, false)
		if err != nil {
			return fmt.Errorf("app2 打开 fixture 失败: %w", err)
		}
		defer pres.Release()
		crossDetail = listPresentations(app1)
		crossVisible = len(crossDetail) > 0
		oleutil.CallMethod(pres, "Close")
		return nil
	})
	seqErr = err
	metrics["sequentialCreates"] = seqPIDs
	metrics["sameThreadSharedSession"] = crossVisible
	metrics["app1SeenPresentations"] = crossDetail
	if err != nil {
		metrics["sequentialError"] = err.Error()
	}

	// (b) 两个独立 STA goroutine 并发 CreateObject：是否得到两个进程。
	type concRes struct {
		pids []int
		err  string
	}
	readyCh := make(chan concRes, 2)
	releaseCh := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			er := withComSession(func(app *ole.IDispatch) error {
				pids := diffPids(before, officeServerPids())
				readyCh <- concRes{pids: pids}
				<-releaseCh
				return nil
			})
			if er != nil {
				readyCh <- concRes{err: er.Error()}
			}
		}()
	}
	r1 := <-readyCh
	r2 := <-readyCh
	close(releaseCh)
	wg.Wait()

	union := map[int]bool{}
	var concErrs []string
	for _, r := range []concRes{r1, r2} {
		if r.err != "" {
			concErrs = append(concErrs, r.err)
		}
		for _, pid := range r.pids {
			union[pid] = true
		}
	}
	metrics["concurrentPids"] = []any{r1.pids, r2.pids}
	metrics["concurrentDistinctProcesses"] = len(union)
	if len(concErrs) > 0 {
		metrics["concurrentErrors"] = concErrs
	}

	// (c) 直调 CoCreateInstance(CLSCTX_LOCAL_SERVER)。
	coCreate := map[string]any{}
	clsid := ole.NewGUID(powerPointAppCLSID)
	beforeCC := officeServerPids()
	var ccErr error
	_ = withComSession(func(_ *ole.IDispatch) error {
		unknown, e := coCreateInstanceLocalServer(clsid)
		if e != nil {
			ccErr = e
			return nil
		}
		app, qiErr := unknown.QueryInterface(ole.IID_IDispatch)
		unknown.Release()
		if qiErr == nil {
			_, _ = oleutil.GetProperty(app, "Name")
			app.Release()
		}
		return nil
	})
	coCreate["addedPids"] = diffPids(beforeCC, officeServerPids())
	if ccErr != nil {
		coCreate["error"] = fmt.Sprintf("hr=0x%08X %v", oleErrorHresult(ccErr), ccErr)
	}
	metrics["coCreateInstance"] = coCreate

	// (e) 决定性实验：首个实例存活期间，各路径能否再建第二个独立进程。
	// 前面的 (c)(d) 每次尝试都从干净环境起步，「新建」与「抢到已起进程」不可区分。
	secondWhileAlive := map[string]any{}
	_ = withComSession(func(app1 *ole.IDispatch) error {
		alive := diffPids(before, officeServerPids())
		secondWhileAlive["firstInstancePids"] = alive

		// e1：同线程再 CreateObject。
		beforeE1 := officeServerPids()
		if app2, e := createAppOnCurrentThread(); e == nil {
			secondWhileAlive["createObjectAdded"] = diffPids(beforeE1, officeServerPids())
			app2.Release()
		} else {
			secondWhileAlive["createObjectError"] = e.Error()
		}

		// e2：直调 CoCreateInstance。
		beforeE2 := officeServerPids()
		if u, e := coCreateInstanceLocalServer(clsid); e == nil {
			secondWhileAlive["coCreateAdded"] = diffPids(beforeE2, officeServerPids())
			u.Release()
		} else {
			secondWhileAlive["coCreateError"] = e.Error()
		}

		// e3：直起进程，看新进程能否与首实例并存，且 CreateObject 是否改绑到它。
		if exes := officeExeCandidates(); len(exes) > 0 {
			exe := exes[len(exes)-1]
			beforeE3 := officeServerPids()
			cmd := exec.Command(exe)
			cmd.Dir = filepath.Dir(exe)
			if e := cmd.Start(); e == nil {
				done := make(chan error, 1)
				go func() { done <- cmd.Wait() }()
				select {
				case err := <-done:
					secondWhileAlive["exeExitedEarly"] = fmt.Sprint(err)
				case <-time.After(3 * time.Second):
					secondWhileAlive["exeAliveAfter3s"] = true
				}
				secondWhileAlive["exeLaunchedAdded"] = diffPids(beforeE3, officeServerPids())

				beforeE3b := officeServerPids()
				if app3, e := createAppOnCurrentThread(); e == nil {
					secondWhileAlive["createAfterExeAdded"] = diffPids(beforeE3b, officeServerPids())
					app3.Release()
				}
			} else {
				secondWhileAlive["exeStartError"] = e.Error()
			}
		}
		return nil
	})
	metrics["secondInstanceWhileFirstAlive"] = secondWhileAlive

	// (d) 先起进程再 CreateObject：判断是否能「抢到」自己起的进程。
	launchResults := []map[string]any{}
	for _, exe := range officeExeCandidates() {
		for _, arg := range []string{"", "/automation", "-automation"} {
			launchResults = append(launchResults, launchAndAttachProbe(exe, arg))
		}
	}
	metrics["exeLaunchAttempts"] = launchResults

	// 清理本项自启进程（仅 diff 出来的，禁全杀）。
	leftover := diffPids(before, officeServerPids())
	if len(leftover) > 0 {
		killPids(leftover)
		metrics["cleanupStuckPids"] = waitPidsExit(leftover, 5*time.Second)
	}

	// 判定：同线程连续创建 / 并发创建 / 首实例存活时再创建，三路任一出现
	// 第二个独立 PID 即视为「多进程可建」。
	seqDistinct := 0
	if len(seqPIDs) > 0 {
		set := map[int]bool{}
		for _, pid := range seqPIDs[len(seqPIDs)-1] {
			set[pid] = true
		}
		seqDistinct = len(set)
	}
	secondAdded := 0
	if v, ok := secondWhileAlive["createObjectAdded"].([]int); ok {
		secondAdded = len(v)
	}
	metrics["sequentialDistinctProcesses"] = seqDistinct

	verdict := "CONFIRMED: 单实例复用（CreateObject 不新建进程，同线程共享会话池）"
	if seqDistinct >= 2 || len(union) >= 2 || secondAdded >= 1 {
		verdict = "CONFIRMED: 多进程可建（至少一条路径得到第二个独立进程）"
	}
	if seqErr != nil {
		verdict = "PASS（部分失败，见 metrics.sequentialError）"
	}
	return recordProbe(9, "多实例 CreateObject 行为", verdict, metrics, nil)
}

var (
	modOle32               = syscall.NewLazyDLL("ole32.dll")
	procCoCreateInstance32 = modOle32.NewProc("CoCreateInstance")
)

// coCreateInstanceLocalServer 直调 ole32!CoCreateInstance(CLSCTX_LOCAL_SERVER)。
// go-ole v1.3.0 未导出该函数，探针自接 syscall；失败只记 hr，不判死。
func coCreateInstanceLocalServer(clsid *ole.GUID) (*ole.IUnknown, error) {
	var out unsafe.Pointer
	hr, _, _ := procCoCreateInstance32.Call(
		uintptr(unsafe.Pointer(clsid)),
		0,
		uintptr(ole.CLSCTX_LOCAL_SERVER),
		uintptr(unsafe.Pointer(ole.IID_IDispatch)),
		uintptr(unsafe.Pointer(&out)),
	)
	if int32(hr) < 0 {
		return nil, fmt.Errorf("CoCreateInstance(CLSCTX_LOCAL_SERVER) hr=0x%08X", uint32(hr))
	}
	return (*ole.IUnknown)(out), nil
}

// launchAndAttachProbe 起一个裸进程并尝试让 CreateObject 附着到它。
// 结果只描述「本次环境实测」，不假设两引擎参数等价。
func launchAndAttachProbe(exe, arg string) map[string]any {
	res := map[string]any{"exe": exe, "arg": arg}
	before := officeServerPids()
	args := []string{}
	if arg != "" {
		args = append(args, arg)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		res["startError"] = err.Error()
		return res
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		res["exitedEarly"] = fmt.Sprint(err)
	case <-time.After(3 * time.Second):
		res["aliveAfter3s"] = true
	}
	launched := diffPids(before, officeServerPids())
	res["launchedPids"] = launched

	// 再 CreateObject：若没有新增 PID，说明附着到了已起进程。
	beforeAttach := officeServerPids()
	attachErr := ""
	_ = withComSession(func(app *ole.IDispatch) error {
		if add := diffPids(beforeAttach, officeServerPids()); len(add) > 0 {
			res["createAddedPids"] = add
		} else {
			res["attachedToExisting"] = true
		}
		quitAppAndWait(app, 5*time.Second)
		return nil
	})
	if attachErr != "" {
		res["createError"] = attachErr
	}

	addedSinceStart := diffPids(before, officeServerPids())
	killPids(addedSinceStart)
	res["cleanupStuckPids"] = waitPidsExit(addedSinceStart, 5*time.Second)
	return res
}

// ---- P10：两实例并发导出总耗时 ----

// probe10ConcurrentExport 对比串行与两实例并发的墙钟时间。
func probe10ConcurrentExport(dir string) string {
	if err := requireCleanOfficeEnv(); err != nil {
		return "SKIP: " + err.Error()
	}
	waitNoOfficeProcess(5 * time.Second)
	sample, synthetic, err := sampleFilePath(dir)
	if err != nil {
		return "FAIL: " + err.Error()
	}
	total, err := slideCountLite(sample)
	if err != nil {
		return "FAIL: 取页数失败: " + err.Error()
	}
	limit := probePageLimit
	if limit > total {
		limit = total
	}
	if limit < 2 {
		return "SKIP: 样例页数不足 2"
	}
	half := limit / 2
	pagesA := make([]int, 0, half)
	pagesB := make([]int, 0, limit-half)
	for i := 1; i <= limit; i++ {
		if i <= half {
			pagesA = append(pagesA, i)
		} else {
			pagesB = append(pagesB, i)
		}
	}
	allPages := append(append([]int{}, pagesA...), pagesB...)

	metrics := map[string]any{
		"sampleFile": sample,
		"synthetic":  synthetic,
		"pages":      limit,
	}

	// 场景 B 需要两个独立文件（同源复制），避免「文件已打开」限制干扰。
	copyPath := filepath.Join(dir, "p10-copy.pptx")
	if err := copyFile(sample, copyPath); err != nil {
		return "FAIL: 复制样例文件失败: " + err.Error()
	}

	var serialSamples, parallelSameFile, parallelDiffFiles []float64
	var serialErrSamples, parallelErrSamples int
	var serialErrMsg, parallelErrMsg string

	for rep := 0; rep < probeReps; rep++ {
		// 串行基线：单实例一次打开导完。
		sDir := filepath.Join(dir, fmt.Sprintf("p10-serial-%d", rep))
		d, _, fails, e := exportSeries(sample, allPages, sDir, true)
		serialSamples = append(serialSamples, float64(d.Milliseconds()))
		serialErrSamples += fails
		if e != nil && serialErrMsg == "" {
			serialErrMsg = e.Error()
		}
		waitAllOfficeExit(5 * time.Second)

		// 场景 A：同一文件，两实例各导一半（WPS 允许双开；MS 可能拒绝）。
		aDir := filepath.Join(dir, fmt.Sprintf("p10-same-%d", rep))
		start := time.Now()
		var wa sync.WaitGroup
		var failsMu sync.Mutex
		scenarioAErr := ""
		for idx, pages := range [][]int{pagesA, pagesB} {
			wa.Add(1)
			go func(i int, pg []int) {
				defer wa.Done()
				_, _, fails, e := exportSeries(sample, pg, filepath.Join(aDir, fmt.Sprintf("inst%d", i)), true)
				failsMu.Lock()
				parallelErrSamples += fails
				if e != nil && scenarioAErr == "" {
					scenarioAErr = e.Error()
				}
				failsMu.Unlock()
			}(idx, pages)
		}
		wa.Wait()
		parallelSameFile = append(parallelSameFile, float64(time.Since(start).Milliseconds()))
		if scenarioAErr != "" && parallelErrMsg == "" {
			parallelErrMsg = "场景A: " + scenarioAErr
		}
		waitAllOfficeExit(5 * time.Second)

		// 场景 B：两个不同文件，两实例并发整本导。
		bDir := filepath.Join(dir, fmt.Sprintf("p10-diff-%d", rep))
		start = time.Now()
		for idx, f := range []string{sample, copyPath} {
			wa.Add(1)
			go func(i int, path string) {
				defer wa.Done()
				_, _, fails, e := exportSeries(path, allPages, filepath.Join(bDir, fmt.Sprintf("inst%d", i)), true)
				failsMu.Lock()
				parallelErrSamples += fails
				if e != nil && parallelErrMsg == "" {
					parallelErrMsg = "场景B: " + e.Error()
				}
				failsMu.Unlock()
			}(idx, f)
		}
		wa.Wait()
		parallelDiffFiles = append(parallelDiffFiles, float64(time.Since(start).Milliseconds()))
		waitAllOfficeExit(5 * time.Second)
	}

	serialMedian := medianMs(serialSamples)
	sameMedian := medianMs(parallelSameFile)
	diffMedian := medianMs(parallelDiffFiles)
	speedupSame, speedupDiff := 0.0, 0.0
	if sameMedian > 0 {
		speedupSame = serialMedian / sameMedian
	}
	if diffMedian > 0 {
		speedupDiff = serialMedian / diffMedian
	}

	metrics["serial"] = statsMs(serialSamples)
	metrics["parallelSameFile"] = statsMs(parallelSameFile)
	metrics["parallelDifferentFiles"] = statsMs(parallelDiffFiles)
	metrics["speedupSameFile"] = round2(speedupSame)
	metrics["speedupDifferentFiles"] = round2(speedupDiff)
	metrics["serialErrors"] = serialErrSamples
	metrics["parallelErrors"] = parallelErrSamples
	if serialErrMsg != "" {
		metrics["serialFirstError"] = serialErrMsg
	}
	if parallelErrMsg != "" {
		metrics["parallelFirstError"] = parallelErrMsg
	}

	verdict := "CONFIRMED: 并行无收益（加速比 <1.2）"
	switch {
	case speedupDiff >= 1.4 || speedupSame >= 1.4:
		verdict = "CONFIRMED: 并行有收益（加速比 ≥1.4）"
	case speedupDiff >= 1.2 || speedupSame >= 1.2:
		verdict = "PASS: 并行收益有限（1.2-1.4，仅千页级库有意义）"
	}
	if parallelErrSamples > 0 {
		verdict += "；并发存在报错，需看 parallelFirstError"
	}
	return recordProbe(10, "两实例并发导出总耗时", verdict, metrics, nil)
}

// copyFile 复制样例文件（场景 B 用，避免同文件打开限制）。
func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// waitAllOfficeExit 等待全部演示类进程退出（只在本探针已确认为自启实例的场景调用）。
func waitAllOfficeExit(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(officeServerPids()) == 0 {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// ---- P11：单页导出耗时分档 ----

// probe11TierTiming 拆分 Open / 单页 Export / Close 三段，对比 small、hd、BMP
// 三档单页成本。
func probe11TierTiming(dir string) string {
	if err := requireCleanOfficeEnv(); err != nil {
		return "SKIP: " + err.Error()
	}
	waitNoOfficeProcess(5 * time.Second)
	sample, synthetic, err := sampleFilePath(dir)
	if err != nil {
		return "FAIL: " + err.Error()
	}
	total, err := slideCountLite(sample)
	if err != nil {
		return "FAIL: 取页数失败: " + err.Error()
	}
	pages := pickPages(total, 10)
	if len(pages) == 0 {
		return "SKIP: 无可导页"
	}

	type tier struct {
		key  string
		args []any
	}
	tiers := []tier{
		{"small", []any{probeSmallW, probeSmallH}},
		{"hd", []any{probeHDW, probeHDH}},
		{"bmp", nil},
	}
	durations := map[string][]float64{"small": {}, "hd": {}, "bmp": {}}
	sizes := map[string][]int64{"small": {}, "hd": {}, "bmp": {}}
	openSamples := []float64{}
	closeSamples := []float64{}
	var firstErr error

	// 每轮独立会话：WPS 关闭最后一个 Presentation 后会退出进程，复用同一
	// app 指针在第二轮必然撞 RPC 不可用（实测）。
	for rep := 0; rep < probeReps; rep++ {
		rep := rep
		err = withComSession(func(app *ole.IDispatch) error {
			defer quitAppAndWait(app, 5*time.Second)
			openStart := time.Now()
			pres, err := openPresentation(app, sample, true, false)
			if err != nil {
				return fmt.Errorf("Open 失败(rep=%d): %w", rep, err)
			}
			openSamples = append(openSamples, float64(time.Since(openStart).Microseconds())/1000.0)
			slidesV, err := oleutil.GetProperty(pres, "Slides")
			if err != nil {
				pres.Release()
				return fmt.Errorf("Slides 失败: %w", err)
			}
			slides := slidesV.ToIDispatch()
			for pi, page := range pages {
				sv, err := oleutil.CallMethod(slides, "Item", page)
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				slide := sv.ToIDispatch()
				for ti := 0; ti < len(tiers); ti++ {
					// 轮转档位顺序，抵消「首导含渲染」的顺序偏差。
					t := tiers[(ti+rep+pi)%len(tiers)]
					ext := "png"
					if t.key == "bmp" {
						ext = "bmp"
					}
					out := filepath.Join(dir, fmt.Sprintf("p11-%s-%03d.%s", t.key, page, ext))
					callArgs := []any{out, strings.ToUpper(ext)}
					callArgs = append(callArgs, t.args...)
					t0 := time.Now()
					_, e := oleutil.CallMethod(slide, "Export", callArgs...)
					elapsed := float64(time.Since(t0).Microseconds()) / 1000.0
					if e != nil {
						if firstErr == nil {
							firstErr = fmt.Errorf("Export(%s, page %d): %w", t.key, page, e)
						}
						continue
					}
					durations[t.key] = append(durations[t.key], elapsed)
					if fi, e2 := os.Stat(out); e2 == nil {
						sizes[t.key] = append(sizes[t.key], fi.Size())
					}
				}
				slide.Release()
			}
			slides.Release()
			closeStart := time.Now()
			oleutil.CallMethod(pres, "Close")
			pres.Release()
			closeSamples = append(closeSamples, float64(time.Since(closeStart).Microseconds())/1000.0)
			return nil
		})
		if err != nil {
			return "FAIL: " + err.Error()
		}
	}

	metrics := map[string]any{
		"sampleFile": sample,
		"synthetic":  synthetic,
		"pages":      pages,
		"open":       statsMs(openSamples),
		"close":      statsMs(closeSamples),
	}
	for key := range durations {
		metrics[key] = statsMs(durations[key])
		metrics[key+"Bytes"] = statsMs(int64ToFloat(sizes[key]))
	}
	smallMed := medianMs(durations["small"])
	hdMed := medianMs(durations["hd"])
	bmpMed := medianMs(durations["bmp"])
	ratio := 0.0
	if hdMed > 0 {
		ratio = smallMed / hdMed
	}
	bmpRatio := 0.0
	if hdMed > 0 {
		bmpRatio = bmpMed / hdMed
	}
	metrics["smallOverHd"] = round2(ratio)
	metrics["bmpOverHd"] = round2(bmpRatio)
	if firstErr != nil {
		metrics["firstError"] = firstErr.Error()
	}

	verdict := "PASS"
	switch {
	case ratio >= 0.6:
		verdict = "CONFIRMED: small 单页 ≥ hd 的 60%，COM 导两档不划算（倾向 hd 一次 + Go 派生）"
	case ratio <= 0.3:
		verdict = "CONFIRMED: small 单页 ≤ hd 的 30%，双档导出可接受"
	default:
		verdict = "PASS: small/hd 比值居中（0.3-0.6），按 Go 后处理实测再定"
	}
	if bmpRatio > 0 && bmpRatio <= 0.7 {
		verdict += "；BMP 明显快于 hd PNG，进候选"
	}
	return recordProbe(11, "单页导出耗时分档", verdict, metrics, nil)
}

func int64ToFloat(v []int64) []float64 {
	out := make([]float64, 0, len(v))
	for _, x := range v {
		out = append(out, float64(x))
	}
	return out
}

// ---- P12：Go 侧后处理耗时 ----

// probe12GoPostProcess 测 hd PNG 的 解码 / 缩放 / 编码 三段成本。
// 缩放用本文件手写最近邻（不引入 golang.org/x/image，避免动生产依赖）。
func probe12GoPostProcess(dir string) string {
	if err := requireCleanOfficeEnv(); err != nil {
		return "SKIP: " + err.Error()
	}
	waitNoOfficeProcess(5 * time.Second)
	sample, synthetic, err := sampleFilePath(dir)
	if err != nil {
		return "FAIL: " + err.Error()
	}
	total, err := slideCountLite(sample)
	if err != nil {
		return "FAIL: 取页数失败: " + err.Error()
	}
	pages := pickPages(total, 10)
	hdDir := filepath.Join(dir, "p12-hd")
	if err := os.MkdirAll(hdDir, 0o755); err != nil {
		return "FAIL: " + err.Error()
	}
	// 先导出 hd 图（P12 只关心后处理，导出耗时不重复计入）。
	_, okCount, failCount, expErr := exportHDImages(sample, pages, hdDir)
	if okCount == 0 {
		return "FAIL: 未导出任何 hd 图: " + fmt.Sprint(expErr)
	}

	var decodeMs, scaleMs, encodeMs []float64
	var peakHeap uint64
	images := 0
	for _, page := range pages {
		p := filepath.Join(hdDir, fmt.Sprintf("%03d.png", page))
		if _, err := os.Stat(p); err != nil {
			continue
		}
		images++
		for rep := 0; rep < probeReps; rep++ {
			t0 := time.Now()
			img, err := decodePNGFile(p)
			decodeMs = append(decodeMs, float64(time.Since(t0).Microseconds())/1000.0)
			if err != nil {
				continue
			}
			t1 := time.Now()
			small := scaleNearest(img, probeSmallW, probeSmallH)
			scaleMs = append(scaleMs, float64(time.Since(t1).Microseconds())/1000.0)

			var buf bytes.Buffer
			t2 := time.Now()
			if err := png.Encode(&buf, small); err != nil {
				continue
			}
			encodeMs = append(encodeMs, float64(time.Since(t2).Microseconds())/1000.0)
			if h := heapInUse(); h > peakHeap {
				peakHeap = h
			}
		}
	}

	metrics := map[string]any{
		"sampleFile": sample,
		"synthetic":  synthetic,
		"images":     images,
		"hdExported": okCount,
		"hdFailed":   failCount,
		"decode":     statsMs(decodeMs),
		"scaleNN":    statsMs(scaleMs),
		"encodePNG":  statsMs(encodeMs),
		"peakHeapMB": round2(float64(peakHeap) / (1024 * 1024)),
	}
	totalMed := medianMs(decodeMs) + medianMs(scaleMs) + medianMs(encodeMs)
	metrics["totalMedianMs"] = round2(totalMed)
	if expErr != nil {
		metrics["exportFirstError"] = expErr.Error()
	}

	verdict := "PASS"
	if totalMed > 0 && totalMed <= 30 {
		verdict = "CONFIRMED: Go 后处理 ≤30ms/页，可承担"
	} else if totalMed > 30 {
		verdict = "CONFIRMED: Go 后处理 >30ms/页，需与 COM small 导出成本对比后再定"
	}
	return recordProbe(12, "Go 侧 hd 解码+缩放+编码耗时", verdict, metrics, nil)
}

// exportHDImages 导出 hd 档 PNG 供后处理计时。
func exportHDImages(path string, pages []int, outDir string) (time.Duration, int, int, error) {
	start := time.Now()
	okCount, failCount := 0, 0
	var firstErr error
	err := withComSession(func(app *ole.IDispatch) error {
		defer quitAppAndWait(app, 5*time.Second)
		pres, err := openPresentation(app, path, true, false)
		if err != nil {
			return err
		}
		defer pres.Release()
		defer oleutil.CallMethod(pres, "Close")
		slidesV, err := oleutil.GetProperty(pres, "Slides")
		if err != nil {
			return err
		}
		slides := slidesV.ToIDispatch()
		defer slides.Release()
		for _, page := range pages {
			sv, err := oleutil.CallMethod(slides, "Item", page)
			if err != nil {
				failCount++
				continue
			}
			slide := sv.ToIDispatch()
			out := filepath.Join(outDir, fmt.Sprintf("%03d.png", page))
			if _, e := oleutil.CallMethod(slide, "Export", out, "PNG", probeHDW, probeHDH); e != nil {
				failCount++
				if firstErr == nil {
					firstErr = e
				}
			} else {
				okCount++
			}
			slide.Release()
		}
		return nil
	})
	if err != nil && firstErr == nil {
		firstErr = err
	}
	return time.Since(start), okCount, failCount, firstErr
}

func decodePNGFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// scaleNearest 最近邻缩放（量级对照用；生产若需更高质量得引入 x/image）。
func scaleNearest(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	dx, dy := b.Dx(), b.Dy()
	if dx <= 0 || dy <= 0 {
		return dst
	}
	for y := 0; y < h; y++ {
		sy := b.Min.Y + y*dy/h
		row := dst.Pix[y*dst.Stride : y*dst.Stride+w*4]
		for x := 0; x < w; x++ {
			sx := b.Min.X + x*dx/w
			r, g, bb, a := src.At(sx, sy).RGBA()
			row[x*4+0] = uint8(r >> 8)
			row[x*4+1] = uint8(g >> 8)
			row[x*4+2] = uint8(bb >> 8)
			row[x*4+3] = uint8(a >> 8)
		}
	}
	return dst
}

func heapInUse() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapInuse
}

// ---- P13：Presentation.Export 整本导出 ----

// probe13WholeDeckExport 验证整本导出的产物命名、页序映射、隐藏页行为与耗时。
func probe13WholeDeckExport(dir string) string {
	if err := requireCleanOfficeEnv(); err != nil {
		return "SKIP: " + err.Error()
	}
	waitNoOfficeProcess(5 * time.Second)
	metrics := map[string]any{}
	var firstErr error

	// 合成 fixture：每页铺满唯一颜色矩形（R 通道 = 10+20*页序），并隐藏 2 页。
	// 页数取 30——6 页时整本与逐页都落在 25ms 内，计时粒度不足以判定 0.8 倍阈值。
	fixturePages := 30
	hiddenPages := []int{2, 5}
	fx := filepath.Join(dir, "p13-color.pptx")
	if abs, e := filepath.Abs(fx); e == nil {
		fx = abs
	}
	wholeDir := filepath.Join(dir, "p13-whole")
	loopDir := filepath.Join(dir, "p13-loop")
	_ = os.MkdirAll(wholeDir, 0o755)
	_ = os.MkdirAll(loopDir, 0o755)

	var wholeMs, loopMs float64
	var fileNames []string
	var mapping []map[string]any
	var hiddenExported int

	err := withComSession(func(app *ole.IDispatch) error {
		defer quitAppAndWait(app, 5*time.Second)
		if err := comMakeColorFixture(app, fx, fixturePages, hiddenPages); err != nil {
			return fmt.Errorf("自产彩色 fixture 失败: %w", err)
		}
		pres, err := openPresentation(app, fx, true, false)
		if err != nil {
			return fmt.Errorf("打开 fixture 失败: %w", err)
		}
		defer pres.Release()
		defer oleutil.CallMethod(pres, "Close")

		// 整本导出（路径必须绝对——COM 服务器按自身 cwd 解析）。
		t0 := time.Now()
		if _, err := oleutil.CallMethod(pres, "Export", wholeDir, "PNG", probeSmallW, probeSmallH); err != nil {
			return fmt.Errorf("Presentation.Export 失败: %w (desc=%s)", err, oleDesc(err))
		}
		wholeMs = float64(time.Since(t0).Milliseconds())

		entries, err := os.ReadDir(wholeDir)
		if err != nil {
			return fmt.Errorf("读取整本导出目录失败: %w", err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				fileNames = append(fileNames, e.Name())
			}
		}
		sort.Strings(fileNames)

		// 逐页导出对照。
		slidesV, err := oleutil.GetProperty(pres, "Slides")
		if err != nil {
			return err
		}
		slides := slidesV.ToIDispatch()
		defer slides.Release()
		cv, _ := oleutil.GetProperty(slides, "Count")
		n := int(cv.Val)
		t1 := time.Now()
		for i := 1; i <= n; i++ {
			sv, err := oleutil.CallMethod(slides, "Item", i)
			if err != nil {
				continue
			}
			slide := sv.ToIDispatch()
			out := filepath.Join(loopDir, fmt.Sprintf("%03d.png", i))
			if _, e := oleutil.CallMethod(slide, "Export", out, "PNG", probeSmallW, probeSmallH); e != nil && firstErr == nil {
				firstErr = e
			}
			slide.Release()
		}
		loopMs = float64(time.Since(t1).Milliseconds())

		// 页序映射：解码每张产物中心像素，反解颜色编码的页序。
		for _, name := range fileNames {
			m := map[string]any{"file": name}
			p := filepath.Join(wholeDir, name)
			if w, h, err := pngSize(p); err == nil {
				m["width"], m["height"] = w, h
			}
			if page, ok := pageIndexFromColor(p); ok {
				m["contentPage"] = page
				for _, hp := range hiddenPages {
					if page == hp {
						hiddenExported++
						break
					}
				}
			}
			mapping = append(mapping, m)
		}
		return nil
	})
	if err != nil {
		return recordProbe(13, "整本导出可行性", "FAIL", metrics, err)
	}

	metrics["fixturePages"] = fixturePages
	metrics["hiddenPages"] = hiddenPages
	metrics["wholeExportFiles"] = len(fileNames)
	metrics["fileNames"] = fileNames
	metrics["pageMapping"] = mapping
	metrics["hiddenExported"] = hiddenExported
	metrics["wholeExportMs"] = round2(wholeMs)
	metrics["perSlideLoopMs"] = round2(loopMs)
	if loopMs > 0 {
		metrics["wholeOverLoop"] = round2(wholeMs / loopMs)
	}

	// 命名规则稳定性：另一个页数的 fixture 再导一次，确认「幻灯片N.PNG」规则不随页数变。
	fx2 := filepath.Join(dir, "p13-color-7.pptx")
	if abs, e := filepath.Abs(fx2); e == nil {
		fx2 = abs
	}
	wholeDir2 := filepath.Join(dir, "p13-whole-7")
	_ = os.MkdirAll(wholeDir2, 0o755)
	_ = withComSession(func(app *ole.IDispatch) error {
		defer quitAppAndWait(app, 5*time.Second)
		if err := comMakeColorFixture(app, fx2, 7, nil); err != nil {
			return err
		}
		pres, err := openPresentation(app, fx2, true, false)
		if err != nil {
			return err
		}
		defer pres.Release()
		defer oleutil.CallMethod(pres, "Close")
		_, err = oleutil.CallMethod(pres, "Export", wholeDir2, "PNG", probeSmallW, probeSmallH)
		return err
	})
	if entries, err := os.ReadDir(wholeDir2); err == nil {
		names := []string{}
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		metrics["fixture7Names"] = names
	}

	// 真实样例：整本 vs 逐页各一次，验证命名规则在真实文档上是否一致。
	if probeSampleFile != "" {
		realOut := filepath.Join(dir, "p13-real")
		realLoop := filepath.Join(dir, "p13-real-loop")
		_ = os.MkdirAll(realOut, 0o755)
		_ = os.MkdirAll(realLoop, 0o755)
		var realCount int
		var realMs, realLoopMs float64
		realErr := withComSession(func(app *ole.IDispatch) error {
			defer quitAppAndWait(app, 5*time.Second)
			pres, err := openPresentation(app, probeSampleFile, true, false)
			if err != nil {
				return err
			}
			defer pres.Release()
			defer oleutil.CallMethod(pres, "Close")
			t0 := time.Now()
			if _, err := oleutil.CallMethod(pres, "Export", realOut, "PNG", probeSmallW, probeSmallH); err != nil {
				return err
			}
			realMs = float64(time.Since(t0).Milliseconds())
			entries, err := os.ReadDir(realOut)
			if err != nil {
				return err
			}
			names := []string{}
			for _, e := range entries {
				if !e.IsDir() {
					names = append(names, e.Name())
				}
			}
			sort.Strings(names)
			realCount = len(names)
			metrics["realFileFirstNames"] = headStrings(names, 3)
			metrics["realFileLastNames"] = tailStrings(names, 3)

			// 逐页对照（同文件、同尺寸）。
			slidesV, err := oleutil.GetProperty(pres, "Slides")
			if err != nil {
				return err
			}
			slides := slidesV.ToIDispatch()
			defer slides.Release()
			cv, _ := oleutil.GetProperty(slides, "Count")
			n := int(cv.Val)
			t1 := time.Now()
			for i := 1; i <= n; i++ {
				sv, e := oleutil.CallMethod(slides, "Item", i)
				if e != nil {
					continue
				}
				slide := sv.ToIDispatch()
				_, _ = oleutil.CallMethod(slide, "Export", filepath.Join(realLoop, fmt.Sprintf("%03d.png", i)), "PNG", probeSmallW, probeSmallH)
				slide.Release()
			}
			realLoopMs = float64(time.Since(t1).Milliseconds())
			return nil
		})
		metrics["realFileExportMs"] = round2(realMs)
		metrics["realFileExportCount"] = realCount
		metrics["realFileLoopMs"] = round2(realLoopMs)
		if realLoopMs > 0 {
			metrics["realFileWholeOverLoop"] = round2(realMs / realLoopMs)
		}
		if realErr != nil {
			metrics["realFileError"] = realErr.Error()
		}
	}

	// 判定优先用真实文件口径（fixture 页简单，耗时不代表业务文档）。
	ratio, ratioFrom := 0.0, "fixture"
	if v, ok := metrics["realFileWholeOverLoop"].(float64); ok && v > 0 {
		ratio, ratioFrom = v, "realFile"
	} else if loopMs > 0 {
		ratio = wholeMs / loopMs
	}
	metrics["timingRatioSource"] = ratioFrom
	metrics["timingRatio"] = round2(ratio)

	verdict := "PASS: 整本导出可用，但未同时满足三条替代判据"
	if ratio > 0 && ratio <= 0.8 && len(fileNames) > 0 && hiddenExported == len(hiddenPages) {
		verdict = "CONFIRMED: 整本导出满足耗时 ×0.8 + 映射可逆 + 隐藏页一致"
	}
	if firstErr != nil {
		metrics["perSlideFirstError"] = firstErr.Error()
	}
	return recordProbe(13, "整本导出可行性", verdict, metrics, nil)
}

func headStrings(v []string, n int) []string {
	if len(v) < n {
		return v
	}
	return v[:n]
}

func tailStrings(v []string, n int) []string {
	if len(v) < n {
		return v
	}
	return v[len(v)-n:]
}

// comMakeColorFixture 自产彩色页序 fixture：每页铺满唯一 RGB 矩形
// （R = 10+20*页序），并可指定隐藏页。导出后读中心像素即可反解页序。
func comMakeColorFixture(app *ole.IDispatch, path string, slideCount int, hidden []int) error {
	colV, err := oleutil.GetProperty(app, "Presentations")
	if err != nil {
		return fmt.Errorf("Presentations: %w", err)
	}
	col := colV.ToIDispatch()
	defer col.Release()
	pV, err := oleutil.CallMethod(col, "Add", msoFalse)
	if err != nil {
		return fmt.Errorf("Presentations.Add: %w", err)
	}
	pres := pV.ToIDispatch()
	defer pres.Release()

	setupV, err := oleutil.GetProperty(pres, "PageSetup")
	if err != nil {
		return fmt.Errorf("PageSetup: %w", err)
	}
	setup := setupV.ToIDispatch()
	_, _ = oleutil.PutProperty(setup, "SlideWidth", 960.0)
	_, _ = oleutil.PutProperty(setup, "SlideHeight", 540.0)
	setup.Release()

	slidesV, err := oleutil.GetProperty(pres, "Slides")
	if err != nil {
		return fmt.Errorf("Slides: %w", err)
	}
	slides := slidesV.ToIDispatch()
	defer slides.Release()

	hiddenSet := map[int]bool{}
	for _, h := range hidden {
		hiddenSet[h] = true
	}

	for i := 1; i <= slideCount; i++ {
		sv, err := oleutil.CallMethod(slides, "Add", i, 12) // ppLayoutBlank
		if err != nil {
			return fmt.Errorf("Slides.Add(%d): %w", i, err)
		}
		slide := sv.ToIDispatch()
		shapesV, err := oleutil.GetProperty(slide, "Shapes")
		if err != nil {
			slide.Release()
			return fmt.Errorf("Shapes: %w", err)
		}
		shapes := shapesV.ToIDispatch()
		shapeV, err := oleutil.CallMethod(shapes, "AddShape", 1, 0.0, 0.0, 960.0, 540.0)
		if err != nil {
			shapes.Release()
			slide.Release()
			return fmt.Errorf("AddShape: %w", err)
		}
		shape := shapeV.ToIDispatch()
		if fillV, e := oleutil.GetProperty(shape, "Fill"); e == nil {
			fill := fillV.ToIDispatch()
			if fcV, e2 := oleutil.GetProperty(fill, "ForeColor"); e2 == nil {
				fc := fcV.ToIDispatch()
				// VBA RGB() 口径：R + G*256 + B*65536
				_, _ = oleutil.PutProperty(fc, "RGB", 8*i)
				fc.Release()
			}
			_, _ = oleutil.CallMethod(fill, "Solid")
			fill.Release()
		}
		shape.Release()
		shapes.Release()

		if hiddenSet[i] {
			if trV, e := oleutil.GetProperty(slide, "SlideShowTransition"); e == nil {
				tr := trV.ToIDispatch()
				_, _ = oleutil.PutProperty(tr, "Hidden", msoTrue)
				tr.Release()
			}
		}
		slide.Release()
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := oleutil.CallMethod(pres, "SaveAs", abs, 24); err != nil {
		return fmt.Errorf("SaveAs: %w", err)
	}
	oleutil.CallMethod(pres, "Close")
	return nil
}

// pageIndexFromColor 解码导出 PNG 中心像素，按 R=8*页序 反解页号。
// 编码上限 31 页（R ≤ 248）——超出该范围需换多像素编码。
func pageIndexFromColor(path string) (int, bool) {
	img, err := decodePNGFile(path)
	if err != nil {
		return 0, false
	}
	b := img.Bounds()
	cx, cy := b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2
	r, _, _, _ := img.At(cx, cy).RGBA()
	rv := int(r >> 8)
	if rv < 8 || rv%8 != 0 {
		return 0, false
	}
	return rv / 8, true
}
