package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- 多页子集导出（ExportPptxPageSubset）用例 ----------
//
// 保留页集合导出 = 购物栏合并导出的第一步能力（单文件内多页子集产出合法 pptx）。
// fixture 复用 pptx_split_test.go 程序化构造基建；断言面含 part→rels 方向
// r:id 可解析校验（assertZipIntegrity，GC 联动编辑缺位的结构性守护）。

// readPptxZipEntries 读取 zip 全部条目：返回条目名顺序（与源包写入顺序一致）与
// 名称→内容映射（整包逐字节对比断言用）。
func readPptxZipEntries(t *testing.T, path string) ([]string, map[string][]byte) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("打开 %s: %v", path, err)
	}
	defer zr.Close()
	order := make([]string, 0, len(zr.Reader.File))
	data := make(map[string][]byte, len(zr.Reader.File))
	for _, f := range zr.Reader.File {
		order = append(order, f.Name)
		data[f.Name] = mustZipPart(t, &zr.Reader, f.Name)
	}
	return order, data
}

// assertSubsetOutput 多页子集输出的通用断言面（assertSinglePageOutput 的集合版）：
//   - sldIdLst 恰为 wantRids 且顺序一致（源展示页序中保留页的相对顺序）；
//   - sldMasterIdLst 恰为 wantMasterRids（wantMasterRids 为 nil 跳过该断言）；
//   - presentation.xml 无 custShowLst / sectionLst / extLst 残留，未编辑区域
//     （sldSz）保留，bannedRefs 的 r:id 引用不残留；
//   - wantMarkers 中每个页 part 存在且含期望内容标记；
//   - deletedParts 全部不存在；
//   - 包完整性（assertZipIntegrity：Content_Types 无悬空 Override、.rels 无悬空
//     关系、part→rels 方向 r:id 可解析、keptBaselines 逐字节一致）。
func assertSubsetOutput(t *testing.T, outPath string, wantRids, wantMasterRids []string, wantMarkers map[string]string, deletedParts map[string]bool, keptBaselines map[string][]byte, bannedRefs []string) {
	t.Helper()
	zr, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("打开输出 %s: %v", filepath.Base(outPath), err)
	}
	defer zr.Close()
	names := make(map[string]bool)
	for _, f := range zr.Reader.File {
		names[f.Name] = true
	}
	base := filepath.Base(outPath)

	for part := range deletedParts {
		if names[part] {
			t.Errorf("%s 不应包含已删 part %s", base, part)
		}
	}

	presBytes := mustZipPart(t, &zr.Reader, pptxPresentationPart)
	var meta pptxPresentationMeta
	if err := xml.Unmarshal(presBytes, &meta); err != nil {
		t.Fatalf("%s presentation.xml 解析: %v", base, err)
	}
	if len(meta.SldIdLst.SldIds) != len(wantRids) {
		t.Fatalf("%s sldIdLst 应恰 %d 条，实际 %d 条", base, len(wantRids), len(meta.SldIdLst.SldIds))
	}
	for i, want := range wantRids {
		if meta.SldIdLst.SldIds[i].RID != want {
			t.Errorf("%s sldIdLst[%d] 应为 %s，实际 %s", base, i, want, meta.SldIdLst.SldIds[i].RID)
		}
	}
	if wantMasterRids != nil {
		if len(meta.SldMasterIdLst.SldMasterIds) != len(wantMasterRids) {
			t.Errorf("%s sldMasterIdLst 应恰 %d 条，实际 %d 条", base, len(wantMasterRids), len(meta.SldMasterIdLst.SldMasterIds))
		} else {
			for i, want := range wantMasterRids {
				if meta.SldMasterIdLst.SldMasterIds[i].RID != want {
					t.Errorf("%s sldMasterIdLst[%d] 应为 %s，实际 %s", base, i, want, meta.SldMasterIdLst.SldMasterIds[i].RID)
				}
			}
		}
	}
	for _, rid := range bannedRefs {
		if bytes.Contains(presBytes, []byte(`r:id="`+rid+`"`)) {
			t.Errorf("%s presentation.xml 不应残留已删页引用 %s", base, rid)
		}
	}
	for _, banned := range []string{"custShowLst", "sectionLst", "extLst"} {
		if bytes.Contains(presBytes, []byte(banned)) {
			t.Errorf("%s presentation.xml 不应残留 %s", base, banned)
		}
	}
	if !bytes.Contains(presBytes, []byte("sldSz")) {
		t.Errorf("%s presentation.xml 未编辑区域（sldSz）被改动", base)
	}

	for part, marker := range wantMarkers {
		if !bytes.Contains(mustZipPart(t, &zr.Reader, part), []byte(marker)) {
			t.Errorf("%s 页 part %s 应包含内容标记 %q", base, part, marker)
		}
	}

	assertZipIntegrity(t, outPath, &zr.Reader, names, keptBaselines)
}

// TestExportPptxPageSubset_MultiMasterMixedSelection 多母版页混选（GC 集合化主用例）：
// 双母版各带独立主题，页 1 属母版 1、页 2 属母版 2、页 3 属母版 2（同版式）；
// 母版 2 另有未用版式（独占媒体）。选中页 1、2：输出同时保留两个母版链路
// （rels 图可达性闭包自动多母版化），sldIdLst 按源页序留两条、sldMasterIdLst
// 两条全保留，未选中页 3 与母版 2 的未用版式/独占媒体剪除，母版 2 的 rels 与
// sldLayoutIdLst 联动剪除被剪版式（按保留母版分组的剪枝面）。
func TestExportPptxPageSubset_MultiMasterMixedSelection(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "多母版混选.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "母版一页", layout: "slideLayout1.xml"},
			{fileName: "slide2.xml", marker: "母版二页", layout: "slideLayout2.xml"},
			{fileName: "slide3.xml", marker: "母版二第三页", layout: "slideLayout2.xml"},
		},
		masters: []pptxFixtureMasterDef{
			{part: "slideMaster1.xml", theme: "theme1.xml", layouts: []pptxFixtureLayoutDef{{part: "slideLayout1.xml"}}},
			{part: "slideMaster2.xml", theme: "theme2.xml", layouts: []pptxFixtureLayoutDef{
				{part: "slideLayout2.xml"},
				{part: "slideLayout3.xml", media: "image7.png"},
			}},
		},
	})

	out := filepath.Join(dir, "子集.pptx")
	result, err := ExportPptxPageSubset(context.Background(), built.src, []int{1, 2}, out)
	if err != nil {
		t.Fatalf("ExportPptxPageSubset: %v", err)
	}
	if result.OutputPath != out || result.PageCount != 2 || len(result.Pages) != 2 || result.Pages[0] != 1 || result.Pages[1] != 2 {
		t.Errorf("结果不符：%+v", result)
	}

	deleted := map[string]bool{
		"ppt/slides/slide3.xml":                        true,
		"ppt/slides/_rels/slide3.xml.rels":             true,
		"ppt/slideLayouts/slideLayout3.xml":            true,
		"ppt/slideLayouts/_rels/slideLayout3.xml.rels": true,
		"ppt/media/image7.png":                         true,
	}
	kept := map[string][]byte{
		"ppt/slides/slide1.xml":                        built.parts["ppt/slides/slide1.xml"],
		"ppt/slides/_rels/slide1.xml.rels":             built.parts["ppt/slides/_rels/slide1.xml.rels"],
		"ppt/slides/slide2.xml":                        built.parts["ppt/slides/slide2.xml"],
		"ppt/slides/_rels/slide2.xml.rels":             built.parts["ppt/slides/_rels/slide2.xml.rels"],
		"ppt/slideMasters/slideMaster1.xml":            built.parts["ppt/slideMasters/slideMaster1.xml"],
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": built.parts["ppt/slideMasters/_rels/slideMaster1.xml.rels"],
		"ppt/slideLayouts/slideLayout1.xml":            built.parts["ppt/slideLayouts/slideLayout1.xml"],
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": built.parts["ppt/slideLayouts/_rels/slideLayout1.xml.rels"],
		"ppt/slideLayouts/slideLayout2.xml":            built.parts["ppt/slideLayouts/slideLayout2.xml"],
		"ppt/slideLayouts/_rels/slideLayout2.xml.rels": built.parts["ppt/slideLayouts/_rels/slideLayout2.xml.rels"],
		"ppt/theme/theme1.xml":                         built.parts["ppt/theme/theme1.xml"],
		"ppt/theme/theme2.xml":                         built.parts["ppt/theme/theme2.xml"],
	}
	// 页号 1 起展示页序：母版 rId1/rId2，页 rId3/rId4/rId5；选中页 1、2
	assertSubsetOutput(t, out, []string{"rId3", "rId4"}, []string{"rId1", "rId2"},
		map[string]string{"ppt/slides/slide1.xml": "母版一页", "ppt/slides/slide2.xml": "母版二页"},
		deleted, kept, []string{"rId5"})

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("打开输出: %v", err)
	}
	defer zr.Close()
	// 母版 2 rels：被剪版式关系移除，保留版式与主题关系原样
	master2Rels := mustZipPart(t, &zr.Reader, "ppt/slideMasters/_rels/slideMaster2.xml.rels")
	if bytes.Contains(master2Rels, []byte("slideLayout3.xml")) {
		t.Error("母版 2 rels 不应残留被剪版式 3 关系")
	}
	if !bytes.Contains(master2Rels, []byte("slideLayout2.xml")) || !bytes.Contains(master2Rels, []byte("theme2.xml")) {
		t.Error("母版 2 rels 应保留版式 2 与主题 2 关系")
	}
	// 母版 2 XML sldLayoutIdLst 联动剪除（字节外科）：版式 3 条目（r:id="rId2"）
	// 区间移除，其余逐字节一致
	master2XML := mustZipPart(t, &zr.Reader, "ppt/slideMasters/slideMaster2.xml")
	if bytes.Contains(master2XML, []byte(`r:id="rId2"`)) {
		t.Error("母版 2 XML 不应残留被剪版式 3 的 sldLayoutId 条目")
	}
	if !bytes.Contains(master2XML, []byte(`r:id="rId1"`)) {
		t.Error("母版 2 XML 应保留版式 2 的 sldLayoutId 条目")
	}
	wantMaster2XML := bytes.Replace(built.parts["ppt/slideMasters/slideMaster2.xml"],
		[]byte(`<p:sldLayoutId id="2147483649" r:id="rId2"/>`), nil, 1)
	if !bytes.Equal(master2XML, wantMaster2XML) {
		t.Error("母版 2 XML 编辑应为字节外科（仅少版式 3 条目区间，其余逐字节一致）")
	}
	assertNoTempResidue(t, dir)
}

// TestExportPptxPageSubset_SkipSelection 跳页选择：4 页选第 1、3 页（非连续），
// 输出 sldIdLst 按源页序留两条；未选页及其备注页删除；保留页 1 指向未选页 4 的
// 内部跳转关系清理、指向保留页 3 的跳转保留；节与自定义放映剥离语义与单页拆分
// 一致。
func TestExportPptxPageSubset_SkipSelection(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "跳页选择.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "第一页内容", jumpsTo: []string{"slide4.xml", "slide3.xml"}},
			{fileName: "slide2.xml", marker: "第二页内容", notes: true},
			{fileName: "slide3.xml", marker: "第三页内容"},
			{fileName: "slide4.xml", marker: "第四页内容", notes: true},
		},
		withSections: true,
	})

	out := filepath.Join(dir, "跳页.pptx")
	result, err := ExportPptxPageSubset(context.Background(), built.src, []int{1, 3}, out)
	if err != nil {
		t.Fatalf("ExportPptxPageSubset: %v", err)
	}
	if result.PageCount != 2 {
		t.Fatalf("应导出 2 页，实际 %d", result.PageCount)
	}

	deleted := map[string]bool{
		"ppt/slides/slide2.xml":                      true,
		"ppt/slides/_rels/slide2.xml.rels":           true,
		"ppt/notesSlides/notesSlide2.xml":            true,
		"ppt/notesSlides/_rels/notesSlide2.xml.rels": true,
		"ppt/slides/slide4.xml":                      true,
		"ppt/slides/_rels/slide4.xml.rels":           true,
		"ppt/notesSlides/notesSlide4.xml":            true,
		"ppt/notesSlides/_rels/notesSlide4.xml.rels": true,
	}
	kept := map[string][]byte{
		"ppt/slides/slide1.xml": built.parts["ppt/slides/slide1.xml"],
		"ppt/slides/slide3.xml": built.parts["ppt/slides/slide3.xml"],
	}
	// 单母版 rId1，页 rId2..rId5；选中页 1、3 → rId2、rId4，源页序相对顺序不变
	assertSubsetOutput(t, out, []string{"rId2", "rId4"}, []string{"rId1"},
		map[string]string{"ppt/slides/slide1.xml": "第一页内容", "ppt/slides/slide3.xml": "第三页内容"},
		deleted, kept, []string{"rId3", "rId5"})

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("打开输出: %v", err)
	}
	defer zr.Close()
	relsBytes := mustZipPart(t, &zr.Reader, "ppt/slides/_rels/slide1.xml.rels")
	if bytes.Contains(relsBytes, []byte(`Target="slide4.xml"`)) {
		t.Error("指向未选页（已删）的内部跳转关系应被移除")
	}
	if !bytes.Contains(relsBytes, []byte(`Target="slide3.xml"`)) {
		t.Error("指向保留页的内部跳转关系应保留")
	}
}

// TestExportPptxPageSubset_SelectAllByteIdentical 全选无剪除面：全部页选中且
// 无未用版式/孤儿媒体/节信息时，输出与源包逐条目逐字节一致（零编辑路径——
// 全部谓词无命中，编辑后字节与输入一致）。
func TestExportPptxPageSubset_SelectAllByteIdentical(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "全选.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "首页内容"},
			{fileName: "slide2.xml", marker: "次页内容"},
		},
		media: true,
	})

	out := filepath.Join(dir, "全选导出.pptx")
	// 顺序无关：乱序传入 [2,1]，归一化后按源页序导出
	result, err := ExportPptxPageSubset(context.Background(), built.src, []int{2, 1}, out)
	if err != nil {
		t.Fatalf("ExportPptxPageSubset: %v", err)
	}
	if result.PageCount != 2 || result.Pages[0] != 1 || result.Pages[1] != 2 {
		t.Errorf("页号应去重升序归一：%+v", result.Pages)
	}

	srcOrder, srcData := readPptxZipEntries(t, built.src)
	outOrder, outData := readPptxZipEntries(t, out)
	if len(srcOrder) != len(outOrder) {
		t.Fatalf("条目数应一致：源 %d，输出 %d", len(srcOrder), len(outOrder))
	}
	for i, name := range srcOrder {
		if outOrder[i] != name {
			t.Errorf("条目顺序应一致：源[%d]=%s，输出[%d]=%s", i, name, i, outOrder[i])
			continue
		}
		if !bytes.Equal(srcData[name], outData[name]) {
			t.Errorf("全选无剪除面时条目 %s 应逐字节一致", name)
		}
	}
}

// TestExportPptxPageSubset_SinglePageMatchesSplitPath 与单页拆分路径行为回归对照：
// 逐页对比「单页子集导出」与「SplitPptxPerPage」的输出——两者共用删除集计算与
// 联动编辑（deleteSetForPages 单页特例），输出应逐条目逐字节一致（集合化泛化
// 未改坏既有拆分路径的结构性守护）。
func TestExportPptxPageSubset_SinglePageMatchesSplitPath(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "回归对照.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide3.xml", marker: "第三页内容", notes: true, jumpsTo: []string{"slide1.xml"}},
			{fileName: "slide1.xml", marker: "第一页内容", notes: true, extLink: true},
			{fileName: "slide2.xml", marker: "第二页内容"},
		},
		media:        true,
		orphanMedia:  []string{"image9.png"},
		withSections: true,
	})

	splitResult, err := SplitPptxPerPage(context.Background(), built.src)
	if err != nil {
		t.Fatalf("SplitPptxPerPage: %v", err)
	}
	for page := 1; page <= 3; page++ {
		subsetOut := filepath.Join(dir, "回归对照子集.pptx")
		if _, err := ExportPptxPageSubset(context.Background(), built.src, []int{page}, subsetOut); err != nil {
			t.Fatalf("第 %d 页子集导出: %v", page, err)
		}
		splitOrder, splitData := readPptxZipEntries(t, splitResult.Files[page-1])
		subsetOrder, subsetData := readPptxZipEntries(t, subsetOut)
		if len(splitOrder) != len(subsetOrder) {
			t.Errorf("第 %d 页条目数不一致：拆分 %d，子集 %d", page, len(splitOrder), len(subsetOrder))
			continue
		}
		for i, name := range splitOrder {
			if subsetOrder[i] != name {
				t.Errorf("第 %d 页条目顺序不一致：拆分[%d]=%s，子集[%d]=%s", page, i, name, i, subsetOrder[i])
				continue
			}
			if !bytes.Equal(splitData[name], subsetData[name]) {
				t.Errorf("第 %d 页条目 %s 应与单页拆分输出逐字节一致", page, name)
			}
		}
	}
}

// TestExportPptxPageSubset_DegradedChainKeepsAllMasters 子集导出 GC 保守降级：
// 任一保留页链路不可解析（页 rels 缺版式关系）时整段 GC 关闭——全部母版/版式/
// 主题保留、sldMasterIdLst 原样、母版本体零编辑（与单页拆分降级语义一致，
// 集合版保守放大为整段降级）。
func TestExportPptxPageSubset_DegradedChainKeepsAllMasters(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "子集降级.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "首页内容", noLayout: true},
			{fileName: "slide2.xml", marker: "次页内容", noLayout: true},
		},
		masters: []pptxFixtureMasterDef{
			{part: "slideMaster1.xml", theme: "theme1.xml", layouts: []pptxFixtureLayoutDef{{part: "slideLayout1.xml"}}},
			{part: "slideMaster2.xml", theme: "theme2.xml", layouts: []pptxFixtureLayoutDef{{part: "slideLayout2.xml"}}},
		},
	})

	out := filepath.Join(dir, "降级子集.pptx")
	if _, err := ExportPptxPageSubset(context.Background(), built.src, []int{1, 2}, out); err != nil {
		t.Fatalf("ExportPptxPageSubset: %v", err)
	}

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("打开输出: %v", err)
	}
	defer zr.Close()
	names := make(map[string]bool)
	for _, f := range zr.Reader.File {
		names[f.Name] = true
	}
	for _, kept := range []string{
		"ppt/slideMasters/slideMaster1.xml", "ppt/slideMasters/slideMaster2.xml",
		"ppt/slideLayouts/slideLayout1.xml", "ppt/slideLayouts/slideLayout2.xml",
		"ppt/theme/theme1.xml", "ppt/theme/theme2.xml",
	} {
		if !names[kept] {
			t.Errorf("GC 降级形态应保留 %s", kept)
		}
	}
	// 零编辑面：母版 XML 与 rels 逐字节一致，sldMasterIdLst 两条原样
	for _, part := range []string{
		"ppt/slideMasters/slideMaster1.xml", "ppt/slideMasters/slideMaster2.xml",
		"ppt/slideMasters/_rels/slideMaster1.xml.rels", "ppt/slideMasters/_rels/slideMaster2.xml.rels",
	} {
		if got := mustZipPart(t, &zr.Reader, part); !bytes.Equal(got, built.parts[part]) {
			t.Errorf("GC 降级形态 %s 应零编辑（逐字节一致）", part)
		}
	}
	presBytes := mustZipPart(t, &zr.Reader, pptxPresentationPart)
	var meta pptxPresentationMeta
	if err := xml.Unmarshal(presBytes, &meta); err != nil {
		t.Fatalf("presentation.xml 解析: %v", err)
	}
	if len(meta.SldMasterIdLst.SldMasterIds) != 2 {
		t.Errorf("降级形态 sldMasterIdLst 应原样 2 条，实际 %d 条", len(meta.SldMasterIdLst.SldMasterIds))
	}
}

// TestExportPptxPageSubset_MixedChainDegradesWholeSet 集合版降级保守放大语义：
// 保留页中仅一条链路不可解析（页 1 链路完整、页 2 缺版式跳）时整段 GC 关闭——
// 链路正常的页 1 也随之放大为不剪除（母版/版式/主题全保留、sldMasterIdLst 原样、
// 母版本体与 rels 零编辑），而非仅对断链页降级（与 deleteSetForPages 注释差异
// 第 2 条「集合版保守放大为整段降级」对应）。
func TestExportPptxPageSubset_MixedChainDegradesWholeSet(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "混合降级.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "首页内容"},
			{fileName: "slide2.xml", marker: "次页内容", noLayout: true},
		},
		masters: []pptxFixtureMasterDef{
			{part: "slideMaster1.xml", theme: "theme1.xml", layouts: []pptxFixtureLayoutDef{{part: "slideLayout1.xml"}}},
			{part: "slideMaster2.xml", theme: "theme2.xml", layouts: []pptxFixtureLayoutDef{{part: "slideLayout2.xml"}}},
		},
	})

	out := filepath.Join(dir, "混合降级子集.pptx")
	if _, err := ExportPptxPageSubset(context.Background(), built.src, []int{1, 2}, out); err != nil {
		t.Fatalf("ExportPptxPageSubset: %v", err)
	}

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("打开输出: %v", err)
	}
	defer zr.Close()
	// 整段降级：被剪母版 2 的链路也必须原样保留（放大不剪）
	for _, kept := range []string{
		"ppt/slideMasters/slideMaster1.xml", "ppt/slideMasters/slideMaster2.xml",
		"ppt/slideLayouts/slideLayout1.xml", "ppt/slideLayouts/slideLayout2.xml",
		"ppt/theme/theme1.xml", "ppt/theme/theme2.xml",
	} {
		if got := mustZipPart(t, &zr.Reader, kept); !bytes.Equal(got, built.parts[kept]) {
			t.Errorf("混合链路降级形态 %s 应零编辑（逐字节一致）", kept)
		}
	}
	presBytes := mustZipPart(t, &zr.Reader, pptxPresentationPart)
	var meta pptxPresentationMeta
	if err := xml.Unmarshal(presBytes, &meta); err != nil {
		t.Fatalf("presentation.xml 解析: %v", err)
	}
	if len(meta.SldMasterIdLst.SldMasterIds) != 2 {
		t.Errorf("混合链路降级形态 sldMasterIdLst 应原样 2 条，实际 %d 条", len(meta.SldMasterIdLst.SldMasterIds))
	}
}

// TestExportPptxPageSubset_InputValidation 入参与路径校验矩阵。
func TestExportPptxPageSubset_InputValidation(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "校验.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "首页内容"},
			{fileName: "slide2.xml", marker: "次页内容"},
		},
	})
	ctx := context.Background()
	out := filepath.Join(dir, "out.pptx")

	t.Run("空选择拒绝", func(t *testing.T) {
		_, err := ExportPptxPageSubset(ctx, built.src, nil, out)
		if err == nil || !strings.Contains(err.Error(), "请先选择要导出的页") {
			t.Errorf("空选择应报引导错误，实际：%v", err)
		}
	})
	t.Run("全非法页号拒绝", func(t *testing.T) {
		_, err := ExportPptxPageSubset(ctx, built.src, []int{0, -3}, out)
		if err == nil || !strings.Contains(err.Error(), "请先选择要导出的页") {
			t.Errorf("全非法页号应报引导错误，实际：%v", err)
		}
	})
	t.Run("页号越界拒绝", func(t *testing.T) {
		_, err := ExportPptxPageSubset(ctx, built.src, []int{1, 99}, out)
		if err == nil || !strings.Contains(err.Error(), "超出范围") {
			t.Errorf("页号越界应报错，实际：%v", err)
		}
	})
	t.Run("重复页号去重", func(t *testing.T) {
		result, err := ExportPptxPageSubset(ctx, built.src, []int{2, 2, 1}, out)
		if err != nil {
			t.Fatalf("重复页号应去重成功: %v", err)
		}
		if result.PageCount != 2 || result.Pages[0] != 1 || result.Pages[1] != 2 {
			t.Errorf("重复页号应去重为 [1 2]，实际：%+v", result.Pages)
		}
	})
	t.Run("输出路径为空拒绝", func(t *testing.T) {
		_, err := ExportPptxPageSubset(ctx, built.src, []int{1}, "  ")
		if err == nil || !strings.Contains(err.Error(), "请先选择导出保存位置") {
			t.Errorf("空输出路径应报引导错误，实际：%v", err)
		}
	})
	t.Run("输出路径与源文件相同拒绝", func(t *testing.T) {
		_, err := ExportPptxPageSubset(ctx, built.src, []int{1}, built.src)
		if err == nil || !strings.Contains(err.Error(), "不能与源文件相同") {
			t.Errorf("同路径导出应拒绝，实际：%v", err)
		}
	})
	t.Run("非 pptx 扩展名拒绝", func(t *testing.T) {
		_, err := ExportPptxPageSubset(ctx, filepath.Join(dir, "旧版.ppt"), []int{1}, out)
		if err == nil || !strings.Contains(err.Error(), "仅支持 .pptx") {
			t.Errorf("非 .pptx 应拒绝，实际：%v", err)
		}
	})
	t.Run("输出目录自动创建", func(t *testing.T) {
		nested := filepath.Join(dir, "nested", "deep", "导出.pptx")
		result, err := ExportPptxPageSubset(ctx, built.src, []int{1}, nested)
		if err != nil {
			t.Fatalf("嵌套输出目录应自动创建: %v", err)
		}
		// readPptxZipEntries 解析失败即 Fatal，调用本身即输出有效性校验
		readPptxZipEntries(t, result.OutputPath)
	})
}

// TestExportPptxPageSubset_CanceledContext 预取消 ctx：立即返回取消错误且保留
// errors.Is(err, context.Canceled) 穿透判定（与拆分取消口径一致）。
func TestExportPptxPageSubset_CanceledContext(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "取消场景.pptx", pptxFixture{
		slides: []pptxFixtureSlide{{fileName: "slide1.xml", marker: "唯一一页"}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ExportPptxPageSubset(ctx, built.src, []int{1}, filepath.Join(dir, "out.pptx"))
	if err == nil {
		t.Fatal("已取消的 ctx 应报错")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("取消错误应保留 context.Canceled 穿透，实际：%v", err)
	}
}
