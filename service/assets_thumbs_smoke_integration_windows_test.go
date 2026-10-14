//go:build integration && windows

package service

// 真机冒烟（任务 10-14-ppt-com PR2，手动触发、不进 CI——需本机 PowerPoint/WPS COM）。
//
// 运行方式：
//
//	go test -tags=integration -run TestSmokeAssetsThumbsEndToEnd -v ./service/ -timeout 20m
//
// 运行前置（两条硬性要求，违反会误伤用户文档）：
//  1. 本机装有 PowerPoint 或 WPS（PowerPoint.Application ProgID 可用）；
//  2. 关闭全部已打开的 PowerPoint/WPS 窗口——本测试启动的实例即唯一实例，
//     结束时 Quit 收口（进程预检发现残留时 skip 拒跑，不冒误关风险）。
//
// 覆盖（PR2 冒烟范围）：
//  1. D3 前提实测：用户已开文档（WithWindow:=msoTrue 模拟）与 msoFalse 无窗口
//     后台导图并存——导出完成后用户文档仍可读（FullName 属性可达）；
//  2. 端到端：一库扫描 → 扫描 goroutine 内自动补缺入队（PR2 接线）→ 批量生成
//     → thumbPNGPath 产物落盘 → GetAssetThumb 读取 API 数据就绪；
//  3. 记账落盘：WaitThumbsDone（ledger Shutdown flush）后 thumb_cache 行数 =
//     页数 × 档位数；
//  4. 幂等重扫：全量命中时补缺清单为空，不重导出（PNG mtime 不变）；
//  5. 设置变更生效（PR3 D2）：小图分辨率 320→480 → 清该档 + 重入队真实重导出
//     （PNG IHDR 宽度直读 480 为端到端证据）+ 记账重记满。
//
// fixture 经 COM 自产（不依赖仓库内样例文件；手写 OOXML zip 被 WPS 拒开，见
// smokeProduceFixtures 注释）。

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"

	"workbench/model"
)

// smokeWaitThumbsTotal 批量产物就绪等待上限（单文件超时 300s × 2 文件 + 扫描
// 余量；一次性冒烟工具允许轮询等待，不适用单测的 sleep 禁令）。
const smokeWaitThumbsTotal = 10 * time.Minute

// TestSmokeAssetsThumbsEndToEnd 见文件头注释。
func TestSmokeAssetsThumbsEndToEnd(t *testing.T) {
	if pre := officeServerPIDSet(); len(pre) > 0 {
		pids := make([]int, 0, len(pre))
		for pid := range pre {
			pids = append(pids, pid)
		}
		t.Skipf("检测到已运行的 PowerPoint/WPS 进程 %v，请关闭全部窗口后重跑（防 Quit 误关用户文档）", pids)
	}

	root := t.TempDir()
	pathA := filepath.Join(root, "demo-a.pptx")
	pathB := filepath.Join(root, "demo-b.pptx")
	// fixture 经 COM 自产（新建→加页→SaveAs）：手写 OOXML zip fixture 实测被
	// WPS 拒开（E_FAIL，探针注释明示的兼容性坑），COM 自产合法性与真实文档同级
	if err := smokeProduceFixtures([]smokeFixtureDef{{pathA, 3}, {pathB, 2}}); err != nil {
		if smokeIsNoOfficeError(err) {
			t.Skipf("本机无 PowerPoint/WPS COM，跳过冒烟: %v", err)
		}
		t.Fatalf("COM 自产 fixture 失败: %v", err)
	}

	svc := NewAssetLibraryService(nil, filepath.Join(root, "data-assets"))
	defer svc.CloseAll()
	// 注入设置读取口（镜像 app_services.go 生产装配）：闭包共享可变 cfg，队列
	// tiersFn 在批次执行时解析到最新宽度——对齐生产「SaveSettings 先更新设置、
	// 后应用变更」的时序（漏注入则重导出恒读默认 320，设置链路断路）
	cfg := model.DefaultAssetThumbsSettings()
	svc.SetThumbsSettingsProvider(func() model.AssetThumbsSettings { return cfg })
	lib, err := svc.CreateLibrary("冒烟库", root)
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}

	// ---- 1. D3 前提：模拟用户先开文档（带窗口，读写模式） ----
	doc := newSmokeUserDoc()
	if err := doc.open(pathA); err != nil {
		if smokeIsNoOfficeError(err) {
			t.Skipf("本机无 PowerPoint/WPS COM，跳过冒烟: %v", err)
		}
		t.Fatalf("打开用户文档失败: %v", err)
	}
	defer doc.close()

	// ---- 2. 扫描（自动补缺入队在扫描 goroutine 内，PR2 接线） ----
	res, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if res.New != 2 {
		t.Fatalf("扫描新增数 = %d，期望 2（汇总 %+v）", res.New, res)
	}

	// ---- 3. 等批量产物就绪（5 页 × 2 档 = 10 张 PNG） ----
	thumbsDir := svc.assetThumbsDir(lib.ID)
	if err := smokeWaitPNGCount(t, thumbsDir, 10, smokeWaitThumbsTotal); err != nil {
		t.Fatalf("缩略图产物未就绪: %v", err)
	}

	// ---- 4. 读取 API 数据就绪 + 记账 touch ----
	fileIDs := smokeFileIDsByPath(t, svc, lib.ID)
	idA, ok := fileIDs["demo-a.pptx"]
	if !ok {
		t.Fatalf("索引中未找到 demo-a.pptx: %v", fileIDs)
	}
	for _, tc := range []struct {
		tier int
		page int
	}{{0, 1}, {1, 3}} {
		b64, err := svc.GetAssetThumb(lib.ID, idA, tc.page, tc.tier)
		if err != nil {
			t.Fatalf("GetAssetThumb(tier=%d, page=%d) 失败: %v", tc.tier, tc.page, err)
		}
		raw, decErr := base64.StdEncoding.DecodeString(b64)
		if decErr != nil || len(raw) < 8 || string(raw[:4]) != "\x89PNG" {
			t.Fatalf("GetAssetThumb(tier=%d, page=%d) 返回非 PNG 数据（解码错=%v，头=%q）",
				tc.tier, tc.page, decErr, string(raw[:min(4, len(raw))]))
		}
	}
	// 未生成页：空串 + nil（前端回退占位卡契约）
	if b64, err := svc.GetAssetThumb(lib.ID, idA, 999, 0); err != nil || b64 != "" {
		t.Fatalf("未生成页应返回空串+nil，得到 (%q, %v)", b64, err)
	}

	// ---- 5. 幂等重扫：全量命中不重导（PNG mtime 不变） ----
	before := smokePNGMtimes(t, thumbsDir)
	res2, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("重扫失败: %v", err)
	}
	if res2.Unchanged != 2 {
		t.Fatalf("重扫未变数 = %d，期望 2（汇总 %+v）", res2.Unchanged, res2)
	}
	after := smokePNGMtimes(t, thumbsDir)
	for p, mt := range before {
		if after[p] != mt {
			t.Errorf("重扫后 PNG 被重导出（幂等破坏）: %s", p)
		}
	}

	// ---- 6. 设置变更生效（D2）：小图分辨率变更 → 清该档 + 重入队真实重导出 ----
	// 清档前小图宽度 = 默认 320；变更 480 后重导出的 PNG 宽度应为 480（IHDR 头
	// 直读，显式尺寸精确生效的端到端证据）。同步直调 applyThumbsSettingsChangeSync
	// （生产入口 ApplyThumbsSettingsChange 为异步包装）。
	smallDir := filepath.Join(thumbsDir, "small")
	wBefore, _, err := smokeFirstPNGDims(smallDir)
	if err != nil {
		t.Fatalf("读取清档前小图尺寸失败: %v", err)
	}
	if wBefore != 320 {
		t.Errorf("清档前小图宽度 = %d，期望 320（默认档）", wBefore)
	}
	prevCfg := cfg
	cfg.SmallWidth = 480 // 先更新设置（生产时序：SaveSettings 落盘换缓存后才应用变更）
	curCfg := cfg
	svc.applyThumbsSettingsChangeSync(prevCfg, curCfg)
	if err := smokeWaitSmallPNGWidth(smallDir, 5, 480, smokeWaitThumbsTotal); err != nil {
		t.Fatalf("分辨率变更后小图重导出未就绪: %v", err)
	}

	// ---- 7. 关停收口（ledger flush）+ 记账落盘行数对账（含清档重导出的新记账） ----
	svc.WaitThumbsDone()
	svc.WaitScansDone()
	var rows int
	if err := svc.withStore(lib.ID, func(store *assetsStore) error {
		return store.db.QueryRow("SELECT COUNT(*) FROM thumb_cache").Scan(&rows)
	}); err != nil {
		t.Fatalf("查询记账表失败: %v", err)
	}
	if rows != 10 {
		t.Errorf("thumb_cache 行数 = %d，期望 10（5 页 × 2 档，清档重导出后重新记满）", rows)
	}

	// ---- 8. D3 核心断言：导图并存后用户文档仍可读 ----
	if err := doc.verifyAlive(); err != nil {
		t.Errorf("导出完成后用户文档不可读（并存破坏）: %v", err)
	}
}

// ---- 用户文档会话（独立 goroutine STA，COM 调用锁定该线程） ----

// smokeUserDoc 模拟用户打开的 PPT 文档（读写 + 带窗口）。全部 COM 调用锁定在
// 内部 goroutine（STA 公寓规则），测试线程经 ops 通道投递操作。
type smokeUserDoc struct {
	ops  chan func(app, pres *ole.IDispatch) error
	errC chan error
	quit chan struct{}
}

func newSmokeUserDoc() *smokeUserDoc {
	return &smokeUserDoc{
		ops:  make(chan func(app, pres *ole.IDispatch) error),
		errC: make(chan error, 1),
		quit: make(chan struct{}),
	}
}

// open 打开文档并等待就绪（60s 超时——含 WPS 冷启动）。
func (d *smokeUserDoc) open(absPath string) error {
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
			// S_FALSE(0x1) = 已是 STA：放行，仍须配对 CoUninitialize（PR1 契约）
			if !smokeHresultIs(err, 1) {
				ready <- fmt.Errorf("CoInitializeEx(STA) 失败: %w", err)
				return
			}
		}
		defer ole.CoUninitialize()

		var unknown *ole.IUnknown
		for attempt := 0; attempt < 5; attempt++ {
			u, err := oleutil.CreateObject("PowerPoint.Application")
			if err == nil {
				unknown = u
				break
			}
			if !smokeHresultIs(err, 0x8001010A) || attempt == 4 {
				ready <- fmt.Errorf("CreateObject(PowerPoint.Application) 失败 (hr=0x%08X): %w",
					smokeHresultOf(err), err)
				return
			}
			time.Sleep(time.Second)
		}
		defer unknown.Release()
		app, err := unknown.QueryInterface(ole.IID_IDispatch)
		if err != nil {
			ready <- fmt.Errorf("QueryInterface(IDispatch) 失败: %w", err)
			return
		}
		defer app.Release()
		_, _ = oleutil.PutProperty(app, "DisplayAlerts", 1)
		_, _ = oleutil.PutProperty(app, "AutomationSecurity", 3)

		presDisp, err := oleutil.GetProperty(app, "Presentations")
		if err != nil {
			ready <- fmt.Errorf("Presentations: %w", err)
			return
		}
		presCollection := presDisp.ToIDispatch()
		defer presCollection.Release()
		// ReadOnly=msoTrue + WithWindow=msoTrue：对齐生产 JumpOnly 跳转场景的
		// Open 参数（读写带窗口 Open 在 WPS COM 上 E_FAIL，实测冒烟发现）
		p, err := oleutil.CallMethod(presCollection, "Open", absPath, -1, 0, -1)
		if err != nil {
			ready <- fmt.Errorf("Open(带窗口): %w", err)
			return
		}
		pres := p.ToIDispatch()
		defer pres.Release()
		ready <- nil

		// 操作循环：测试线程投递闭包在该 STA 线程执行
		for {
			select {
			case fn := <-d.ops:
				d.errC <- fn(app, pres)
			case <-d.quit:
				_, _ = oleutil.CallMethod(app, "Quit") // 预检保证唯一实例：Quit 只关本冒烟文档
				return
			}
		}
	}()
	select {
	case err := <-ready:
		return err
	case <-time.After(60 * time.Second):
		return fmt.Errorf("用户文档打开超时（60s）")
	}
}

// verifyAlive 用户文档仍可读（FullName 属性可达 = 未被后台导图破坏/关闭）。
func (d *smokeUserDoc) verifyAlive() error {
	d.ops <- func(app, pres *ole.IDispatch) error {
		if _, err := oleutil.GetProperty(pres, "FullName"); err != nil {
			return fmt.Errorf("FullName: %w", err)
		}
		return nil
	}
	select {
	case err := <-d.errC:
		return err
	case <-time.After(30 * time.Second):
		return fmt.Errorf("verifyAlive 30s 无响应")
	}
}

// close 收口：Quit 唯一实例并等 goroutine 退出。
func (d *smokeUserDoc) close() {
	close(d.quit)
	time.Sleep(500 * time.Millisecond) // goroutine Quit 后退出；冒烟工具容忍宽限
}

// smokeFixtureDef COM 自产 fixture 定义（保存路径 + 页数）。
type smokeFixtureDef struct {
	path  string
	pages int
}

// smokeProduceFixtures 经 COM 自产 fixture（独立会话：新建 Presentation → 逐页
// Add（空白版式）→ SaveAs → Close，最后 Quit 收口自启实例）。手写 OOXML zip
// fixture 实测被 WPS 拒开（Open E_FAIL——scripts/ppt-com-probe 注释明示的
// 兼容性坑），COM 自产合法性与真实文档同级。
func smokeProduceFixtures(defs []smokeFixtureDef) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
			if !smokeHresultIs(err, 1) { // S_FALSE = 已是 STA：放行，仍配对 Uninitialize
				done <- fmt.Errorf("CoInitializeEx(STA) 失败: %w", err)
				return
			}
		}
		defer ole.CoUninitialize()

		var unknown *ole.IUnknown
		var err error
		for attempt := 0; attempt < 5; attempt++ {
			unknown, err = oleutil.CreateObject("PowerPoint.Application")
			if err == nil {
				break
			}
			if !smokeHresultIs(err, 0x8001010A) || attempt == 4 {
				done <- fmt.Errorf("CreateObject(PowerPoint.Application) 失败 (hr=0x%08X): %w",
					smokeHresultOf(err), err)
				return
			}
			time.Sleep(time.Second)
		}
		defer unknown.Release()
		app, err := unknown.QueryInterface(ole.IID_IDispatch)
		if err != nil {
			done <- fmt.Errorf("QueryInterface(IDispatch) 失败: %w", err)
			return
		}
		defer app.Release()
		_, _ = oleutil.PutProperty(app, "DisplayAlerts", 1)
		_, _ = oleutil.PutProperty(app, "AutomationSecurity", 3)

		presDisp, err := oleutil.GetProperty(app, "Presentations")
		if err != nil {
			done <- fmt.Errorf("Presentations: %w", err)
			return
		}
		presCollection := presDisp.ToIDispatch()
		defer presCollection.Release()

		for _, def := range defs {
			if err := smokeProduceOne(presCollection, def); err != nil {
				done <- err
				return
			}
		}
		_, _ = oleutil.CallMethod(app, "Quit") // 自启实例：Quit 收口
		done <- nil
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Minute):
		return fmt.Errorf("fixture 自产超时（2min）")
	}
}

// smokeProduceOne 自产单个 fixture（ppLayoutBlank=12、ppSaveAsOpenXMLPresentation=24）。
func smokeProduceOne(presCollection *ole.IDispatch, def smokeFixtureDef) error {
	newP, err := oleutil.CallMethod(presCollection, "Add")
	if err != nil {
		return fmt.Errorf("Presentations.Add: %w", err)
	}
	pd := newP.ToIDispatch()
	defer pd.Release()
	slidesDisp, err := oleutil.GetProperty(pd, "Slides")
	if err != nil {
		return fmt.Errorf("Slides: %w", err)
	}
	sc := slidesDisp.ToIDispatch()
	defer sc.Release()
	for i := 1; i <= def.pages; i++ {
		if _, err := oleutil.CallMethod(sc, "Add", i, 12); err != nil {
			return fmt.Errorf("Slides.Add(%d): %w", i, err)
		}
	}
	if _, err := oleutil.CallMethod(pd, "SaveAs", def.path, 24); err != nil {
		return fmt.Errorf("SaveAs(%s): %w", def.path, err)
	}
	if _, err := oleutil.CallMethod(pd, "Close"); err != nil {
		return fmt.Errorf("Close(%s): %w", def.path, err)
	}
	return nil
}

// smokeWaitPNGCount 轮询等待两档目录 PNG 总数达标（冒烟工具允许轮询等待）。
func smokeWaitPNGCount(t *testing.T, thumbsDir string, want int, deadline time.Duration) error {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		n := 0
		for _, tier := range thumbTiers {
			entries, err := os.ReadDir(filepath.Join(thumbsDir, tier.Key))
			if err != nil {
				continue
			}
			for _, e := range entries {
				if filepath.Ext(e.Name()) == ".png" {
					n++
				}
			}
		}
		if n >= want {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("%v 内 PNG 未达 %d 张", deadline, want)
}

// smokeFirstPNGDims 读目录下首个 PNG 的像素尺寸（IHDR 头：宽高各 4 字节大端，
// 偏移 16/20——显式尺寸精确生效的端到端证据源）。
func smokeFirstPNGDims(dir string) (w, h int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".png" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return 0, 0, err
		}
		if len(data) < 24 || string(data[:4]) != "\x89PNG" {
			return 0, 0, fmt.Errorf("%s 非 PNG 数据", e.Name())
		}
		return int(binary.BigEndian.Uint32(data[16:20])), int(binary.BigEndian.Uint32(data[20:24])), nil
	}
	return 0, 0, os.ErrNotExist
}

// smokeWaitSmallPNGWidth 轮询等待小图目录 PNG 数达标且首个 PNG 宽度等于期望值
//（分辨率变更重导出就绪判定；冒烟工具允许轮询等待）。
func smokeWaitSmallPNGWidth(dir string, wantCount, wantWidth int, deadline time.Duration) error {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		entries, err := os.ReadDir(dir)
		if err == nil {
			n := 0
			for _, e := range entries {
				if filepath.Ext(e.Name()) == ".png" {
					n++
				}
			}
			if n >= wantCount {
				w, _, derr := smokeFirstPNGDims(dir)
				if derr == nil && w == wantWidth {
					return nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("%v 内小图未达 %d 张或宽度未变为 %d", deadline, wantCount, wantWidth)
}

// smokeFileIDsByPath 库内文件 ID 映射（相对路径 → files.id）。
func smokeFileIDsByPath(t *testing.T, svc *AssetLibraryService, libraryID string) map[string]int64 {
	t.Helper()
	out := make(map[string]int64)
	err := svc.withStore(libraryID, func(store *assetsStore) error {
		recs, err := store.listFileRecords()
		if err != nil {
			return err
		}
		for _, r := range recs {
			out[r.path] = r.id
		}
		return nil
	})
	if err != nil {
		t.Fatalf("读取文件索引失败: %v", err)
	}
	return out
}

// smokePNGMtimes 全部 PNG 的 mtime 快照（相对 thumbsDir 路径 → UnixNano）。
func smokePNGMtimes(t *testing.T, thumbsDir string) map[string]int64 {
	t.Helper()
	out := make(map[string]int64)
	for _, tier := range thumbTiers {
		dir := filepath.Join(thumbsDir, tier.Key)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if filepath.Ext(e.Name()) != ".png" {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			out[tier.Key+"/"+e.Name()] = info.ModTime().UnixNano()
		}
	}
	return out
}

// smokeIsNoOfficeError 判定「PowerPoint 未安装」类错误（冒烟降级 skip 不 fail）。
func smokeIsNoOfficeError(err error) bool {
	return smokeHresultIs(err, 0x800401F3) || smokeHresultIs(err, 0x80040154)
}

// smokeHresultIs 判定 error 链中 OleError 的 HRESULT 码。
func smokeHresultIs(err error, hr uintptr) bool {
	return smokeHresultOf(err) == hr
}

// smokeHresultOf 提取 OleError 的 HRESULT（非 OleError 返回哨兵）。
func smokeHresultOf(err error) uintptr {
	if oe, ok := err.(*ole.OleError); ok {
		return oe.Code()
	}
	return 0xFFFFFFFF
}
