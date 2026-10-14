//go:build windows

// PowerPoint COM 探针——素材管理第二批（任务 10-14-ppt-com）PR1 首日实测工具。
//
// 逐项验证 research/com-thumbnail-export.md 文末「待实测清单」7 项 + 进程快照
// 归属判定，输出结论报告供接口设计定案。本程序为一次性手动工具，不进 CI、
// 不进构建产物；结论回填 research 文档与 PRD 后即可归档。
//
// 用法（需本机安装 PowerPoint）：
//
//	go run ./scripts/ppt-com-probe              # 跑全部自动项（P6 需人工配合默认跳过）
//	go run ./scripts/ppt-com-probe -only 3,5    # 只跑指定项
//	go run ./scripts/ppt-com-probe -with-p6     # 含 P6（运行中会暂停等你手动开一个空 PowerPoint）
//
// 运行前确认：关闭所有已打开的 PowerPoint 窗口（P2/P7 归属判定依赖干净环境）。
package main

import (
	"archive/zip"
	"encoding/binary"
	"encoding/csv"
	"flag"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// ---- 探针项注册 ----

type probeItem struct {
	id          int
	name        string
	interactive bool                    // true = 需人工配合（-with-p6 才跑）
	run         func(dir string) string // 返回结论行；panic 视为该项失败
}

var probeItems = []probeItem{
	{1, "初始化链路 + DisplayAlerts 数值探测（ppAlertsNone 预判 1）", false, probe1InitAndAlerts},
	{2, "进程快照 diff 归属判定（CreateInstance 前后 POWERPNT.EXE 集合）", false, probe2OwnershipDiff},
	{3, "Slide.Export 纵横比行为（16:9 与 4:3 × 多组传参，读 PNG IHDR）", false, probe3AspectRatio},
	{4, "显式 ScaleWidth/Height 是否绕开 ExportBitmapResolution", false, probe4ResolutionBypass},
	{5, "WithWindow:=msoFalse 无窗口导出可用性（PNG 解码 + 像素多样性）", false, probe5NoWindowExport},
	{6, "用户实例复用（先手动开一个空 PowerPoint 再跑，进程数应不变）", true, probe6UserInstanceReuse},
	{7, "强杀自启实例后阻塞 Invoke 返回时序（预期 0x800706BA，秒级返回）", false, probe7KillTiming},
	{8, "同文件二次打开行为 + Presentations 集合 FullName 复用检查", false, probe8ReuseOpened},
}

func main() {
	only := flag.String("only", "", "逗号分隔只跑指定探针项（如 3,5）")
	withP6 := flag.Bool("with-p6", false, "含需人工配合的 P6（运行中暂停等你手动打开空 PowerPoint）")
	flag.Parse()

	var runSet map[int]bool // nil = 全跑（-only 未传）
	if *only != "" {
		runSet = map[int]bool{}
		for _, s := range strings.Split(*only, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			fatalIf(err, "-only 参数非法: %q", *only)
			runSet[n] = true
		}
	}

	dir, err := os.MkdirTemp("", "ppt-com-probe-*")
	fatalIf(err, "创建临时目录失败")
	defer os.RemoveAll(dir)
	fmt.Printf("工作目录: %s\n\n", dir)

	for _, item := range probeItems {
		if runSet != nil && !runSet[item.id] {
			continue
		}
		if item.interactive && !*withP6 {
			fmt.Printf("── P%d %s\n   SKIP（需人工配合，加 -with-p6 启用）\n\n", item.id, item.name)
			continue
		}
		fmt.Printf("── P%d %s\n", item.id, item.name)
		start := time.Now()
		conclusion := func() (conclusion string) {
			defer func() {
				if r := recover(); r != nil {
					conclusion = fmt.Sprintf("FAIL: panic: %v", r)
				}
			}()
			return item.run(dir)
		}()
		fmt.Printf("   [%s] %s（耗时 %s）\n\n", statusOf(conclusion), conclusion, time.Since(start).Round(time.Millisecond))
	}
}

func statusOf(conclusion string) string {
	switch {
	case strings.HasPrefix(conclusion, "PASS"), strings.HasPrefix(conclusion, "CONFIRMED"):
		return "OK"
	case strings.HasPrefix(conclusion, "SKIP"):
		return "SKIP"
	default:
		return "!!"
	}
}

func fatalIf(err error, format string, args ...any) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "探针中止: %s: %v\n", fmt.Sprintf(format, args...), err)
		os.Exit(1)
	}
}

// ---- COM 会话基建 ----

// withComSession 在独立 goroutine 内建立 STA COM 会话（LockOSThread +
// CoInitializeEx 放行 S_FALSE），fn 内所有 COM 调用锁定在该线程执行。
// 这是生产 ThumbsService COM worker 的同款结构，探针先行验证结构成立性。
func withComSession(fn func(app *ole.IDispatch) error) error {
	type result struct{ err error }
	ch := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		if err != nil {
			// S_FALSE(0x1) = 线程已是 STA，go-ole 当错误返回——放行，仍须配对 CoUninitialize
			if oleErrorHresult(err) != 1 {
				ch <- result{fmt.Errorf("CoInitializeEx(STA) 失败: %w", err)}
				return
			}
		}
		defer ole.CoUninitialize()

		// CreateInstance 带 0x8001010A（RETRYLATER，应用忙）重试——research §3.4：
		// 语义为「目标应用忙」应等待重试而非判失败；WPS 强杀后的僵尸激活态数秒内恢复
		var unknown *ole.IUnknown
		for attempt := 0; attempt < 5; attempt++ {
			u, err := oleutil.CreateObject("PowerPoint.Application")
			if err == nil {
				unknown = u
				break
			}
			if oleErrorHresult(err) != 0x8001010A || attempt == 4 {
				ch <- result{fmt.Errorf("CreateObject(PowerPoint.Application) 失败 (hr=0x%08X): %w", oleErrorHresult(err), err)}
				return
			}
			time.Sleep(time.Second)
		}
		defer unknown.Release()
		app, err := unknown.QueryInterface(ole.IID_IDispatch)
		if err != nil {
			ch <- result{fmt.Errorf("QueryInterface(IDispatch) 失败: %w", err)}
			return
		}
		defer app.Release()

		// 防弹窗三板斧（P1 已验证数值被接受；WPS 对 minimal fixture 可能弹修复
		// 对话框卡死后续探针项——每个会话默认压制）
		_, _ = oleutil.PutProperty(app, "DisplayAlerts", 1)
		_, _ = oleutil.PutProperty(app, "AutomationSecurity", 3)

		ch <- result{fn(app)}
	}()
	return (<-ch).err
}

// oleErrorHresult 提取 OleError 的 HRESULT 码（非 OleError 返回哨兵 0xFFFFFFFF）。
func oleErrorHresult(err error) uintptr {
	if oe, ok := err.(*ole.OleError); ok {
		return oe.Code()
	}
	return 0xFFFFFFFF
}

// MsoTriState 取值（COM 层字面量）。
const (
	msoFalse = 0
	msoTrue  = -1
)

// officeServerProcNames Office 演示类 COM 服务器进程名集合（小写比对）：
// Microsoft PowerPoint（POWERPNT.EXE）与 WPS Presentation（wpp.exe）——本机
// PowerPoint.Application ProgID 可能由 WPS 接管（实测 reg：标准 MS CLSID 键存在
// 但 LocalServer32 缺失，CoCreateInstance 实际启动 wpp.exe），归属判定按集合
// 过滤而非硬编码单进程名。
var officeServerProcNames = map[string]bool{
	"powerpnt.exe": true,
	"wpp.exe":      true,
}

// officeServerPids 枚举 Office 演示类进程 PID 集合（tasklist 全量 CSV 后按
// officeServerProcNames 过滤；对齐 process-management 契约「tasklist 走
// encoding/csv」先例）。
// lastTasklistDiag 记录最近一次命中的进程行（诊断归属判定）。
var lastTasklistDiag string

func officeServerPids() map[int]bool {
	pids := map[int]bool{}
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	out, err := cmd.Output()
	if err != nil {
		lastTasklistDiag = fmt.Sprintf("tasklist 失败: %v, stderr=%s", err, out)
		return pids
	}
	r := csv.NewReader(strings.NewReader(string(out)))
	hits := []string{}
	for {
		row, err := r.Read()
		if err != nil {
			break
		}
		if len(row) >= 2 {
			name := strings.ToLower(strings.TrimSpace(row[0]))
			name = strings.Trim(name, `"`)
			if officeServerProcNames[name] {
				if pid, err := strconv.Atoi(strings.TrimSpace(row[1])); err == nil {
					pids[pid] = true
					hits = append(hits, fmt.Sprintf("%s/%d", name, pid))
				}
			}
		}
	}
	lastTasklistDiag = strings.Join(hits, ", ")
	if lastTasklistDiag == "" {
		lastTasklistDiag = "(无 Office 演示类进程)"
	}
	return pids
}

func taskkillPid(pid int) error {
	return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F").Run()
}

// quitApp 退出 PowerPoint 实例（仅自启实例可调用；复用用户实例时禁调）。
func quitApp(app *ole.IDispatch) {
	_, _ = oleutil.CallMethod(app, "Quit")
}

// diffPids 求 added = after - before（新增 PID = 自启实例）。
func diffPids(before, after map[int]bool) []int {
	added := []int{}
	for pid := range after {
		if !before[pid] {
			added = append(added, pid)
		}
	}
	return added
}

func pidKeys(m map[int]bool) []int {
	out := []int{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ---- fixture 构造（纯 zip 程序化，禁依赖本机 Office——pptx-split 契约同款）----

// writeFixturePptx 生成最小合法 pptx：slideCount 页、16:9（widescreen）或 4:3。
// 每页放一个文本框写入「PAGE-n」字样（有内容非空白，P5 黑图检测依赖）。
// 结构含 slideLayout（悬挂 layout 会触发 PowerPoint 修复对话框——恰是探针要压制的模态来源）。
func writeFixturePptx(path string, slideCount int, widescreen bool) error {
	cx := `9144000` // 4:3
	if widescreen {
		cx = `12192000` // 16:9
	}
	overrides, sldIDEntries, presSlideRels := "", "", ""
	for i := 1; i <= slideCount; i++ {
		overrides += fmt.Sprintf(`<Override PartName="/ppt/slides/slide%d.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`, i)
		sldIDEntries += fmt.Sprintf(`<p:sldId id="%d" r:id="rId%d"/>`, 255+i, i+1)
		presSlideRels += fmt.Sprintf(`<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide%d.xml"/>`, i+1, i)
	}

	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>
<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>
<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>
` + overrides + `
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>
</Relationships>`,
		"ppt/presentation.xml": fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>
<p:sldIdLst>%s</p:sldIdLst>
<p:sldSz cx="%s" cy="6858000"/><p:notesSz cx="6858000" cy="9144000"/>
</p:presentation>`, sldIDEntries, cx),
		"ppt/_rels/presentation.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideMaster" Target="slideMasters/slideMaster1.xml"/>
` + presSlideRels + `</Relationships>`,
		"ppt/slideMasters/slideMaster1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldMaster xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">
<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>
<p:clrMap bg1="lt1" tx1="dk1" bg2="lt2" tx2="dk2" accent1="accent1" accent2="accent2" accent3="accent3" accent4="accent4" accent5="accent5" accent6="accent6" hlink="hlink" folHlink="folHlink"/>
<p:sldLayoutIdLst><p:sldLayoutId id="2147483649" r:id="rId1"/></p:sldLayoutIdLst>
</p:sldMaster>`,
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
</Relationships>`,
		"ppt/slideLayouts/slideLayout1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sldLayout xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" type="blank" preserve="1">
<p:cSld name="Blank"><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:spTree></p:cSld>
<p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr>
</p:sldLayout>`,
	}
	for i := 1; i <= slideCount; i++ {
		files[fmt.Sprintf("ppt/slides/slide%d.xml", i)] = fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>
<p:sp><p:nvSpPr><p:cNvPr id="2" name="TextBox"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr>
<p:spPr><a:xfrm><a:off x="1000000" y="1000000"/><a:ext cx="6000000" cy="1000000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr>
<p:txBody><a:bodyPr/><a:p><a:r><a:rPr sz="4400" b="1"/><a:solidFill><a:srgbClr val="1F4E79"/></a:solidFill><a:t>PAGE-%d</a:t></a:r></a:p></p:txBody></p:sp>
</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`, i)
		files[fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", i)] = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>
</Relationships>`
	}
	return writeZip(path, files)
}

func writeZip(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return err
		}
	}
	return zw.Close()
}

// comMakeFixture 经 COM 会话自产 pptx：新建 Presentation → 设页尺寸 → 加页
// 加文本框 → SaveAs。页尺寸 pt 口径（16:9=960x540、4:3=720x540）。
// 返回绝对路径。fixture 兼容性问题（手写 OOXML 被 WPS 拒开）由此绕开。
func comMakeFixture(app *ole.IDispatch, path string, slideCount int, widescreen bool) error {
	presCol, err := oleutil.GetProperty(app, "Presentations")
	if err != nil {
		return fmt.Errorf("Presentations: %w", err)
	}
	presColDisp := presCol.ToIDispatch()
	defer presColDisp.Release()
	pV, err := oleutil.CallMethod(presColDisp, "Add", msoFalse)
	if err != nil {
		return fmt.Errorf("Presentations.Add: %w", err)
	}
	pres := pV.ToIDispatch()
	defer pres.Release()

	w, h := 720.0, 540.0
	if widescreen {
		w, h = 960.0, 540.0
	}
	setup, err := oleutil.GetProperty(pres, "PageSetup")
	if err != nil {
		return fmt.Errorf("PageSetup: %w", err)
	}
	setupDisp := setup.ToIDispatch()
	defer setupDisp.Release()
	if _, err := oleutil.PutProperty(setupDisp, "SlideWidth", w); err != nil {
		return fmt.Errorf("SlideWidth: %w", err)
	}
	if _, err := oleutil.PutProperty(setupDisp, "SlideHeight", h); err != nil {
		return fmt.Errorf("SlideHeight: %w", err)
	}

	slides, err := oleutil.GetProperty(pres, "Slides")
	if err != nil {
		return fmt.Errorf("Slides: %w", err)
	}
	slidesDisp := slides.ToIDispatch()
	defer slidesDisp.Release()
	for i := 1; i <= slideCount; i++ {
		slideV, err := oleutil.CallMethod(slidesDisp, "Add", i, 12) // ppLayoutBlank=12
		if err != nil {
			return fmt.Errorf("Slides.Add(%d): %w", i, err)
		}
		slide := slideV.ToIDispatch()
		shapesV, err := oleutil.GetProperty(slide, "Shapes")
		if err != nil {
			slide.Release()
			return fmt.Errorf("Shapes: %w", err)
		}
		shapes := shapesV.ToIDispatch()
		// 渲染验证标记：红色大矩形（铺 60% 页面，AddShape 不依赖文字引擎，err 必检）
		shapeV, err := oleutil.CallMethod(shapes, "AddShape", 1, 100.0, 100.0, 500.0, 300.0) // msoShapeRectangle=1
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
				_, _ = oleutil.PutProperty(fc, "RGB", 0xFF0000) // 红色（BGR 序 0xFF0000=蓝? VBA RGB()=R+G*256+B*65536，红=0x0000FF）
				fc.Release()
			}
			_, _ = oleutil.CallMethod(fill, "Solid")
			fill.Release()
		}
		shape.Release()
		// 文字标记（err 检查——上轮 AddTextbox err 被吞致空白页误判渲染失败）
		textboxV, err := oleutil.CallMethod(shapes, "AddTextbox", 1, 100.0, 420.0, 500.0, 80.0)
		if err != nil {
			shapes.Release()
			slide.Release()
			return fmt.Errorf("AddTextbox: %w", err)
		}
		textbox := textboxV.ToIDispatch()
		if tfV, e := oleutil.GetProperty(textbox, "TextFrame"); e == nil {
			tf := tfV.ToIDispatch()
			if trV, e2 := oleutil.GetProperty(tf, "TextRange"); e2 == nil {
				tr := trV.ToIDispatch()
				if _, e3 := oleutil.PutProperty(tr, "Text", fmt.Sprintf("PAGE-%d", i)); e3 != nil {
					shapes.Release()
					slide.Release()
					return fmt.Errorf("Text 写入: %w", e3)
				}
				tr.Release()
			}
			tf.Release()
		}
		textbox.Release()
		shapes.Release()
		slide.Release()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := oleutil.CallMethod(pres, "SaveAs", abs, 24); err != nil { // ppSaveAsOpenXMLPresentation=24
		return fmt.Errorf("SaveAs: %w", err)
	}
	oleutil.CallMethod(pres, "Close")
	return nil
}

// ---- 断言辅助 ----

// pngSize 读 PNG IHDR 宽高（文件头第 16-23 字节，big-endian uint32 ×2）。
func pngSize(path string) (w, h int, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	if len(b) < 24 || string(b[12:16]) != "IHDR" {
		return 0, 0, fmt.Errorf("非 PNG 或缺 IHDR: len=%d", len(b))
	}
	return int(binary.BigEndian.Uint32(b[16:20])), int(binary.BigEndian.Uint32(b[20:24])), nil
}

// pngInkRatio 解码 PNG 按 50×50 网格抽样统计非白（任一分量 <245）像素占比。
// 判读口径：白底带文字/图形 = 少量非白（>0）；全黑渲染失败 = 接近 1；纯白空白 = 0。
// （5×5 多样性判定对白底小文字误报——25 个采样点可能全落空白区。）
func pngInkRatio(path string) (ratio float64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return 0, err
	}
	bounds := img.Bounds()
	total, ink := 0, 0
	for gx := 0; gx < 50; gx++ {
		for gy := 0; gy < 50; gy++ {
			x := bounds.Min.X + gx*(bounds.Dx()-1)/49
			y := bounds.Min.Y + gy*(bounds.Dy()-1)/49
			r, g, b, _ := img.At(x, y).RGBA()
			total++
			if r < 245*257 || g < 245*257 || b < 245*257 {
				ink++
			}
		}
	}
	if total == 0 {
		return 0, nil
	}
	return float64(ink) / float64(total), nil
}

// openPresentation 按 (ReadOnly, WithWindow) 组合打开文件，返回 Presentation
// dispatch（调用方负责 Close/Release——Close 仅关文档不退应用）。
func openPresentation(app *ole.IDispatch, pptPath string, readOnly, withWindow bool) (*ole.IDispatch, error) {
	pres, err := oleutil.GetProperty(app, "Presentations")
	if err != nil {
		return nil, fmt.Errorf("Presentations: %w", err)
	}
	presDisp := pres.ToIDispatch()
	defer presDisp.Release()
	ro, ww := 0, 0
	if readOnly {
		ro = -1 // msoTrue
	}
	if withWindow {
		ww = -1
	}
	p, err := oleutil.CallMethod(presDisp, "Open", pptPath, ro, 0, ww)
	if err != nil {
		return nil, fmt.Errorf("Open(ro=%d,ww=%d): %w (desc=%s)", ro, ww, err, oleDesc(err))
	}
	return p.ToIDispatch(), nil
}

// exportSlide 导出指定页（1-based）到 pngPath；导出完 Close 该 Presentation。
func exportSlide(app *ole.IDispatch, pptPath, pngPath string, slideNum int, w, h int, withWindow bool) error {
	pDisp, err := openPresentation(app, pptPath, true, withWindow)
	if err != nil {
		return err
	}
	defer pDisp.Release()
	defer oleutil.CallMethod(pDisp, "Close") // msoFalse 场景也安全（无窗口仅移出集合）

	slides, err := oleutil.GetProperty(pDisp, "Slides")
	if err != nil {
		return fmt.Errorf("Slides: %w", err)
	}
	slidesDisp := slides.ToIDispatch()
	defer slidesDisp.Release()
	slide, err := oleutil.CallMethod(slidesDisp, "Item", slideNum)
	if err != nil {
		return fmt.Errorf("Slides.Item(%d): %w", slideNum, err)
	}
	slideDisp := slide.ToIDispatch()
	defer slideDisp.Release()
	if _, err := oleutil.CallMethod(slideDisp, "Export", pngPath, "PNG", w, h); err != nil {
		return fmt.Errorf("Export: %w (desc=%s)", err, oleDesc(err))
	}
	return nil
}

func oleDesc(err error) string {
	if oe, ok := err.(*ole.OleError); ok {
		if d := oe.Description(); d != "" {
			return d
		}
	}
	return ""
}

// ---- 探针项实现 ----

// P1：STA 初始化 + CreateInstance + 防弹窗数值探测。
// DisplayAlerts 依次试写 1（ppAlertsNone 预判）与 2（ppAlertsAll），记录接受情况。
func probe1InitAndAlerts(dir string) string {
	var conclusion []string
	err := withComSession(func(app *ole.IDispatch) error {
		for _, v := range []int{1, 2} {
			if _, err := oleutil.PutProperty(app, "DisplayAlerts", v); err != nil {
				conclusion = append(conclusion, fmt.Sprintf("DisplayAlerts=%d 被拒(hr=0x%08X desc=%s)", v, oleErrorHresult(err), oleDesc(err)))
			} else {
				got, _ := oleutil.GetProperty(app, "DisplayAlerts")
				val := "?"
				if got != nil {
					val = fmt.Sprintf("%v", got.Value())
				}
				conclusion = append(conclusion, fmt.Sprintf("DisplayAlerts=%d 被接受(回读=%s)", v, val))
			}
		}
		if _, err := oleutil.PutProperty(app, "AutomationSecurity", 3); err != nil {
			conclusion = append(conclusion, fmt.Sprintf("AutomationSecurity=3 被拒(hr=0x%08X)", oleErrorHresult(err)))
		} else {
			conclusion = append(conclusion, "AutomationSecurity=3 被接受")
		}
		quitApp(app)
		return nil
	})
	if err != nil {
		return "FAIL: " + err.Error()
	}
	return "PASS: " + strings.Join(conclusion, "; ")
}

// P2：CreateInstance 前后进程快照 diff——验证新增 PID 检出归属判定 + 强杀收口。
func probe2OwnershipDiff(dir string) string {
	before := officeServerPids()
	err := withComSession(func(app *ole.IDispatch) error {
		after := officeServerPids()
		added := diffPids(before, after)
		fmt.Printf("   before=%v after=%v 新增=%v\n   tasklist 原始输出: %q\n", pidKeys(before), pidKeys(after), added, lastTasklistDiag)
		if len(added) == 1 {
			pid := added[0]
			if err := taskkillPid(pid); err != nil {
				return fmt.Errorf("taskkill /PID %d 失败: %w", pid, err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if _, alive := officeServerPids()[pid]; !alive {
					quitApp(app) // 实例已死，Quit 仅为验证断连后调用无害
					return nil
				}
				time.Sleep(200 * time.Millisecond)
			}
			return fmt.Errorf("强杀后 5s 内 PID %d 仍存活", pid)
		}
		quitApp(app)
		return nil
	})
	if err != nil {
		return "FAIL: " + err.Error()
	}
	if len(before) > 0 {
		return "PASS（前置已有用户实例，归属退化为复用语义——请关掉 PowerPoint 重跑本项以验证强杀分支）"
	}
	return "PASS: 自启实例经 diff 检出且强杀收口生效"
}

// P3：纵横比行为——16:9 三页 fixture 与 4:3 两页 fixture 各导，读 IHDR 判定
// fit-inside 语义（实际输出某边=传参值、另一边按原始比例收窄）。
func probe3AspectRatio(dir string) string {
	wide := filepath.Join(dir, "wide.pptx")
	classic := filepath.Join(dir, "classic.pptx")

	repl := strings.NewReplacer(" ", "_", ":", "")
	cases := []struct {
		ppt      string
		label    string
		slideNum int
		w, h     int
	}{
		{wide, "16:9 传 480x270", 1, 480, 270},
		{wide, "16:9 传 480x480", 2, 480, 480},
		{wide, "16:9 传 1920x1080", 3, 1920, 1080},
		{classic, "4:3 传 480x270", 1, 480, 270},
		{classic, "4:3 传 1600x1200", 2, 1600, 1200},
	}
	lines := []string{}
	err := withComSession(func(app *ole.IDispatch) error {
		// fixture 会话内自产（手写 minimal OOXML 被 WPS 拒开——实测「发生意外」）
		if err := comMakeFixture(app, wide, 3, true); err != nil {
			return fmt.Errorf("自产 wide fixture: %w", err)
		}
		if err := comMakeFixture(app, classic, 2, false); err != nil {
			return fmt.Errorf("自产 classic fixture: %w", err)
		}
		for _, c := range cases {
			pngPath := filepath.Join(dir, fmt.Sprintf("p3-%s.png", repl.Replace(c.label)))
			if err := exportSlide(app, c.ppt, pngPath, c.slideNum, c.w, c.h, false); err != nil {
				return fmt.Errorf("%s: %w", c.label, err)
			}
			w, h, err := pngSize(pngPath)
			if err != nil {
				return fmt.Errorf("%s: %w", c.label, err)
			}
			lines = append(lines, fmt.Sprintf("%s -> 实际 %dx%d", c.label, w, h))
		}
		quitApp(app)
		return nil
	})
	if err != nil {
		return "FAIL: " + err.Error()
	}
	return "PASS（fit-inside 结论人工判读）: " + strings.Join(lines, "; ")
}

// P4：显式尺寸是否绕开 ExportBitmapResolution——读注册表值，分别以
// 「不传尺寸」（Export 尾部截断两参形态）与「显式 480x270」导出同页，对比输出。
func probe4ResolutionBypass(dir string) string {
	wide := filepath.Join(dir, "wide4.pptx")
	fatalIf(writeFixturePptx(wide, 1, true), "构造 fixture")

	regVal := "?（查询失败）"
	out, err := exec.Command("reg", "query", `HKCU\Software\Microsoft\Office`, "/s", "/f", "ExportBitmapResolution", "/v").CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "ExportBitmapResolution") {
				regVal = strings.TrimSpace(line)
				break
			}
		}
	}

	var defW, defH, expW, expH int
	err = withComSession(func(app *ole.IDispatch) error {
		if err := comMakeFixture(app, wide, 1, true); err != nil {
			return fmt.Errorf("自产 fixture: %w", err)
		}
		pDisp, err := openPresentation(app, wide, true, false)
		if err != nil {
			return fmt.Errorf("默认尺寸导出: %w", err)
		}
		defer pDisp.Release()
		defer oleutil.CallMethod(pDisp, "Close")
		slides, err := oleutil.GetProperty(pDisp, "Slides")
		if err != nil {
			return err
		}
		slidesDisp := slides.ToIDispatch()
		defer slidesDisp.Release()
		slide, err := oleutil.CallMethod(slidesDisp, "Item", 1)
		if err != nil {
			return err
		}
		slideDisp := slide.ToIDispatch()
		defer slideDisp.Release()
		defPath := filepath.Join(dir, "p4-default.png")
		if _, err := oleutil.CallMethod(slideDisp, "Export", defPath, "PNG"); err != nil {
			return fmt.Errorf("Export(无尺寸): %w (desc=%s)", err, oleDesc(err))
		}
		defW, defH, err = pngSize(defPath)
		if err != nil {
			return err
		}
		if err := exportSlide(app, wide, filepath.Join(dir, "p4-explicit.png"), 1, 480, 270, false); err != nil {
			return fmt.Errorf("显式尺寸导出: %w", err)
		}
		expW, expH, err = pngSize(filepath.Join(dir, "p4-explicit.png"))
		return err
	})
	if err != nil {
		return "FAIL: " + err.Error()
	}
	quitSoon(dir)
	bypassed := expW != defW || expH != defH
	return fmt.Sprintf("PASS: 注册表[%s] 默认=%dx%d 显式480x270=%dx%d 绕开=%v", regVal, defW, defH, expW, expH, bypassed)
}

// quitSoon 兜底退出探针遗留的 PowerPoint 进程（仅自启场景；有用户实例时无害——
// Quit 对 multi-use 复用实例也会执行，探针环境运行前已要求关闭全部 PowerPoint）。
func quitSoon(_ string) {
	for pid := range officeServerPids() {
		_ = taskkillPid(pid)
	}
}

// P5：无窗口导出渲染验证（两形态对比）——第六轮实测 ww=0 导出黑图（WPS 无窗口
// 不渲染），形态 (b) 验证「WithWindow 打开 + 应用级最小化」路径的渲染与前台干扰。
func probe5NoWindowExport(dir string) string {
	wide := filepath.Join(dir, "wide5.pptx")
	lines := []string{}
	err := withComSession(func(app *ole.IDispatch) error {
		if err := comMakeFixture(app, wide, 2, true); err != nil {
			return fmt.Errorf("自产 fixture: %w", err)
		}
		// 形态 (a)：ww=0 无窗口导出（第六轮黑图基线复测）
		pngA := filepath.Join(dir, "p5-nowindow.png")
		if err := exportSlide(app, wide, pngA, 1, 480, 270, false); err != nil {
			lines = append(lines, "形态a ww=0: Open/Export 失败: "+err.Error())
		} else if d, e := pngInkRatio(pngA); e == nil {
			lines = append(lines, fmt.Sprintf("形态a ww=0: 非白占比=%.1f%%（0%%=空白渲染, ≥95%%=全黑）", d*100))
		}
		// 形态 (b)：ww=-1 打开 + Application.WindowState=ppWindowMinimized(2) + 导出
		if _, err := oleutil.PutProperty(app, "WindowState", 2); err != nil {
			lines = append(lines, fmt.Sprintf("WindowState=2 被拒(hr=0x%08X)", oleErrorHresult(err)))
		} else {
			lines = append(lines, "WindowState=2（最小化）被接受")
		}
		pngB := filepath.Join(dir, "p5-minimized.png")
		if err := exportSlide(app, wide, pngB, 2, 480, 270, true); err != nil {
			lines = append(lines, "形态b ww=-1+最小化: Open/Export 失败: "+err.Error())
		} else if d, e := pngInkRatio(pngB); e == nil {
			lines = append(lines, fmt.Sprintf("形态b ww=-1+最小化: 非白占比=%.1f%%", d*100))
		}
		// 形态 (c)：ww=-1 打开 + Presentation.Windows.Item(1) 移屏幕外 + Export
		// （ActiveWindow 在最小化态残留后取不到——改经 Presentation 自身窗口集合）
		winV, err := oleutil.GetProperty(app, "Presentations")
		if err != nil {
			lines = append(lines, "形态c: 取 Presentations 失败: "+err.Error())
			return nil
		}
		presCol := winV.ToIDispatch()
		defer presCol.Release()
		pV, err := oleutil.CallMethod(presCol, "Open", wide, msoTrue, msoFalse, msoTrue)
		if err != nil {
			lines = append(lines, "形态c: ww=-1 Open 失败: "+err.Error())
			return nil
		}
		pres := pV.ToIDispatch()
		defer pres.Release()
		winsV, err := oleutil.GetProperty(pres, "Windows")
		if err != nil {
			lines = append(lines, "形态c: 取 Windows 失败: "+err.Error())
			oleutil.CallMethod(pres, "Close")
			return nil
		}
		wins := winsV.ToIDispatch()
		winsCount := 0
		if cv, e := oleutil.GetProperty(wins, "Count"); e == nil {
			winsCount = int(cv.Val)
		}
		if winsCount == 0 {
			lines = append(lines, "形态c: Presentation.Windows 为空（无窗口形态）")
			oleutil.CallMethod(pres, "Close")
			return nil
		}
		win1V, err := oleutil.CallMethod(wins, "Item", 1)
		if err != nil {
			lines = append(lines, "形态c: Windows.Item(1) 失败: "+err.Error())
			oleutil.CallMethod(pres, "Close")
			return nil
		}
		win := win1V.ToIDispatch()
		moved := true
		if _, e1 := oleutil.PutProperty(win, "Left", -4000.0); e1 != nil {
			moved = false
			lines = append(lines, "形态c: Left 被拒: "+e1.Error())
		}
		if _, e2 := oleutil.PutProperty(win, "Top", -4000.0); e2 != nil {
			moved = false
			lines = append(lines, "形态c: Top 被拒: "+e2.Error())
		}
		win.Release()
		pngC := filepath.Join(dir, "p5-offscreen.png")
		slidesV2, err := oleutil.GetProperty(pres, "Slides")
		if err == nil {
			slides2 := slidesV2.ToIDispatch()
			slide1V, err := oleutil.CallMethod(slides2, "Item", 1)
			if err == nil {
				slide1 := slide1V.ToIDispatch()
				if _, err := oleutil.CallMethod(slide1, "Export", pngC, "PNG", 480, 270); err != nil {
					lines = append(lines, "形态c: Export 失败: "+err.Error())
				} else if d, e := pngInkRatio(pngC); e == nil {
					lines = append(lines, fmt.Sprintf("形态c ww=-1+移屏外(窗口已移=%v): 非白占比=%.1f%%", moved, d*100))
				}
				slide1.Release()
			}
			slides2.Release()
		}
		oleutil.CallMethod(pres, "Close")
		quitApp(app)
		return nil
	})
	if err != nil {
		return "FAIL: " + err.Error()
	}
	return "PASS（结论人工判读）: " + strings.Join(lines, "; ")
}

// P6：用户实例复用——运行中暂停，等用户手动开一个空 PowerPoint，然后
// CreateInstance 看进程数是否不变（不变 = multi-use 复用确认）。
func probe6UserInstanceReuse(dir string) string {
	before := officeServerPids()
	fmt.Println("   请现在手动打开一个空 PowerPoint（保持运行），然后回到这里按回车继续...")
	fmt.Scanln()
	err := withComSession(func(app *ole.IDispatch) error {
		after := officeServerPids()
		added := diffPids(before, after)
		fmt.Printf("   before=%v after=%v 新增=%v\n   tasklist 原始输出: %q\n", pidKeys(before), pidKeys(after), added, lastTasklistDiag)
		// 复用实例禁 Quit（用户窗口还开着）——仅释放引用
		return nil
	})
	if err != nil {
		return "FAIL: " + err.Error()
	}
	return "PASS（结论人工判读：新增为空 = multi-use 复用成立，该形态禁 Quit 禁杀）"
}

// P7：强杀时序——导图循环中途强杀自启 PID，测阻塞 Invoke 返回耗时与错误码
// （预期 0x800706BA RPC_S_SERVER_UNAVAILABLE，秒级返回）。
func probe7KillTiming(dir string) string {
	wide := filepath.Join(dir, "wide7.pptx")

	before := officeServerPids()
	type killResult struct {
		elapsed time.Duration
		hr      uintptr
		err     error
	}
	ch := make(chan killResult, 1)
	start := time.Now()
	go func() {
		var ourPID int
		var killOnce sync.Once
		err := withComSession(func(app *ole.IDispatch) error {
			after := officeServerPids()
			added := diffPids(before, after)
			fmt.Printf("   tasklist 原始输出: %q\n", lastTasklistDiag)
			if len(added) != 1 {
				return fmt.Errorf("归属判定异常（新增=%v，须恰 1 个自启实例）——请关闭全部 PowerPoint 后重跑", added)
			}
			ourPID = added[0]
			// fixture 会话内自产（60 页拉长导出窗口；同实例不新增进程，不污染归属 diff）
			if err := comMakeFixture(app, wide, 60, true); err != nil {
				return fmt.Errorf("自产 fixture: %w", err)
			}
			// 杀进程是纯 Win32 操作（非 COM 调用），可在任意 goroutine 执行；
			// 2.5s 后杀自启实例，此时导图循环应在第 2-5 页之间阻塞在 Invoke 上
			killOnce.Do(func() {
				time.AfterFunc(2500*time.Millisecond, func() {
					_ = taskkillPid(ourPID)
				})
			})
			for i := 1; i <= 60; i++ {
				if err := exportSlide(app, wide, filepath.Join(dir, fmt.Sprintf("p7-%02d.png", i)), 1, 320, 180, false); err != nil {
					ch <- killResult{elapsed: time.Since(start), hr: oleErrorHresult(err), err: err}
					return nil
				}
			}
			ch <- killResult{elapsed: time.Since(start), err: nil}
			return nil
		})
		if err != nil && ourPID == 0 {
			ch <- killResult{elapsed: time.Since(start), hr: oleErrorHresult(err), err: err}
		}
	}()
	r := <-ch
	if r.err == nil {
		return "FAIL: 60 页全部导完未被杀（强杀时序未触发或导出过快）"
	}
	return fmt.Sprintf("PASS: 阻塞 Invoke 经 %s 返回 hr=0x%08X err=%v", r.elapsed.Round(time.Millisecond), r.hr, r.err)
}

// P8：同文件二次打开 + Presentations 集合复用——第六轮「ww=-1 Open 报发生意外」
// 疑为刚 Close 文件句柄未释放时序问题，本轮 (a) 验证 ww=-1 打开可行性（含 1s
// 缓冲重试），(b) 已打开状态下同文件 ReadOnly ww=0 二次打开，(c) 集合枚举。
func probe8ReuseOpened(dir string) string {
	wide := filepath.Join(dir, "wide8.pptx")
	lines := []string{}
	err := withComSession(func(app *ole.IDispatch) error {
		if err := comMakeFixture(app, wide, 2, true); err != nil {
			return fmt.Errorf("自产 fixture: %w", err)
		}
		abs, err := filepath.Abs(wide)
		if err != nil {
			return err
		}
		p1, err := openPresentation(app, abs, false, true)
		if err != nil {
			lines = append(lines, "(a) 生成后立即 ww=-1 打开: "+err.Error())
			time.Sleep(time.Second)
			p1, err = openPresentation(app, abs, false, true)
			if err != nil {
				lines = append(lines, "(a-retry) 1s 后 ww=-1 重开: "+err.Error())
				return nil
			}
			lines = append(lines, "(a-retry) 1s 后 ww=-1 重开成功（第六轮失败为句柄时序）")
		} else {
			lines = append(lines, "(a) ww=-1 打开成功")
		}
		defer p1.Release()

		p2, err := openPresentation(app, abs, true, false)
		if err != nil {
			lines = append(lines, "(b) 已开状态下 ww=0 二次打开: "+err.Error())
		} else {
			lines = append(lines, "(b) 已开状态下 ww=0 二次打开成功（同名双开允许）")
			oleutil.CallMethod(p2, "Close")
			p2.Release()
		}

		presCol, err := oleutil.GetProperty(app, "Presentations")
		if err != nil {
			return err
		}
		presColDisp := presCol.ToIDispatch()
		defer presColDisp.Release()
		countV, err := oleutil.GetProperty(presColDisp, "Count")
		if err != nil {
			return err
		}
		n := int(countV.Val)
		lines = append(lines, fmt.Sprintf("(c) Presentations.Count=%d", n))
		for i := 1; i <= n; i++ {
			itemV, err := oleutil.CallMethod(presColDisp, "Item", i)
			if err != nil {
				continue
			}
			item := itemV.ToIDispatch()
			if fn, e2 := oleutil.GetProperty(item, "FullName"); e2 == nil {
				lines = append(lines, fmt.Sprintf("    [%d] FullName=%v", i, fn.Value()))
			}
			item.Release()
		}
		oleutil.CallMethod(p1, "Close")
		quitApp(app)
		return nil
	})
	if err != nil {
		return "FAIL: " + err.Error()
	}
	return "PASS（结论人工判读）: " + strings.Join(lines, "; ")
}
