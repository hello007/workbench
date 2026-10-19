package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- 多源页合并导出（MergePptxPages）用例 ----------
//
// 基底注入式合并（路线 A）：基底 = 清单第一项源文件（PR1 集合化 GC 保留其被勾选
// 页），其余源文件提取保留页闭包重命名注入。fixture 复用 pptx_split_test.go
// 程序化构造基建；断言面含 part→rels 方向 r:id 可解析校验（assertZipIntegrity）。

// fixtureMarkerRe 页内容标记提取（fixture 幻灯片 XML 的 <a:t> 文本）。
var fixtureMarkerRe = regexp.MustCompile(`<a:t>([^<]*)</a:t>`)

// readMergedPageMarkers 按输出包 sldIdLst 顺序提取各页内容标记：sldIdLst → rId →
// 输出 presentation.xml.rels → part 名 → 标记文本。任一跳断裂即 Fatal（页序与
// 源页对应关系是合并导出的核心契约）。
func readMergedPageMarkers(t *testing.T, outPath string) []string {
	t.Helper()
	zr, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("打开输出 %s: %v", filepath.Base(outPath), err)
	}
	defer zr.Close()
	var meta pptxPresentationMeta
	if err := xml.Unmarshal(mustZipPart(t, &zr.Reader, pptxPresentationPart), &meta); err != nil {
		t.Fatalf("presentation.xml 解析: %v", err)
	}
	var rels pptxRelationships
	if err := xml.Unmarshal(mustZipPart(t, &zr.Reader, pptxPresentationRelsPart), &rels); err != nil {
		t.Fatalf("presentation.xml.rels 解析: %v", err)
	}
	ridToTarget := make(map[string]string, len(rels.Relationships))
	for _, rel := range rels.Relationships {
		ridToTarget[rel.ID] = rel.Target
	}
	var markers []string
	for _, e := range meta.SldIdLst.SldIds {
		target, ok := ridToTarget[e.RID]
		if !ok {
			t.Fatalf("sldIdLst 引用 %s 在输出关系表中不存在（悬空）", e.RID)
		}
		part := resolveRelTarget(pptxPresentationPart, target)
		m := fixtureMarkerRe.FindSubmatch(mustZipPart(t, &zr.Reader, part))
		if m == nil {
			t.Fatalf("页 part %s 缺少内容标记", part)
		}
		markers = append(markers, string(m[1]))
	}
	return markers
}

// openMergedOutput 打开输出包并返回条目名集合（断言辅助，统一 Fatal 口径）。
func openMergedOutput(t *testing.T, outPath string) (*zip.ReadCloser, map[string]bool) {
	t.Helper()
	zr, err := zip.OpenReader(outPath)
	if err != nil {
		t.Fatalf("打开输出 %s: %v", filepath.Base(outPath), err)
	}
	names := make(map[string]bool)
	for _, f := range zr.Reader.File {
		names[f.Name] = true
	}
	return zr, names
}

// TestMergePptxPages_TwoSourcesDualMaster 双源合并主链路：基底甲（单母版）+ 注入
// 乙（独立母版/版式/主题，另带未用版式与独占媒体）。断言：输出页序与购物栏一致、
// 双母版注入（sldMasterIdLst 两条）、乙源 part 冲突重命名（slide3/slide4 起全局
// 找空位）且 part 本体逐字节一致、乙 rels Target 按映射重写、注入条目进
// presentation.xml.rels 与 Content_Types、包完整性（无悬空关系/Override/r:id）。
func TestMergePptxPages_TwoSourcesDualMaster(t *testing.T) {
	dir := t.TempDir()
	builtA := buildPptxFixture(t, dir, "甲.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "甲一"},
			{fileName: "slide2.xml", marker: "甲二"},
		},
	})
	builtB := buildPptxFixture(t, dir, "乙.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "乙一", layout: "slideLayout1.xml", extLink: true},
			{fileName: "slide2.xml", marker: "乙二", layout: "slideLayout1.xml"},
		},
		masters: []pptxFixtureMasterDef{{
			part:  "slideMaster1.xml",
			theme: "theme1.xml",
			layouts: []pptxFixtureLayoutDef{
				{part: "slideLayout1.xml"},
				{part: "slideLayout2.xml", media: "image5.png"},
			},
		}},
	})

	out := filepath.Join(dir, "合并.pptx")
	items := []model.PptxMergeItem{
		{SrcPath: builtA.src, PageNum: 1},
		{SrcPath: builtB.src, PageNum: 1},
		{SrcPath: builtA.src, PageNum: 2},
		{SrcPath: builtB.src, PageNum: 2},
	}
	result, err := MergePptxPages(context.Background(), items, out, nil)
	if err != nil {
		t.Fatalf("MergePptxPages: %v", err)
	}
	if result.OutputPath != out || result.PageCount != 4 || len(result.Pages) != 4 {
		t.Fatalf("结果不符：%+v", result)
	}
	if len(result.SkippedItems) != 0 || result.SldSzMixed {
		t.Errorf("不应有跳过项/尺寸混排：%+v", result)
	}
	for i, want := range []model.PptxMergedPage{
		{SrcPath: builtA.src, SrcPageNum: 1, OutPageNum: 1},
		{SrcPath: builtB.src, SrcPageNum: 1, OutPageNum: 2},
		{SrcPath: builtA.src, SrcPageNum: 2, OutPageNum: 3},
		{SrcPath: builtB.src, SrcPageNum: 2, OutPageNum: 4},
	} {
		if result.Pages[i] != want {
			t.Errorf("Pages[%d] = %+v, want %+v", i, result.Pages[i], want)
		}
	}

	// 页序 = 购物栏顺序（跨文件交错：甲一 → 乙一 → 甲二 → 乙二）
	got := readMergedPageMarkers(t, out)
	want := []string{"甲一", "乙一", "甲二", "乙二"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("输出页序[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	zr, names := openMergedOutput(t, out)
	defer zr.Close()

	// 乙源 part 冲突重命名：slide1/slide2 → slide3/slide4，母版 → slideMaster2，
	// 主题 → theme2，版式 1/2 → slideLayout2/slideLayout3（同名冲突按去尾数字
	// 递增找空位），独占媒体 image5.png 无冲突保留原名
	for _, part := range []string{
		"ppt/slides/slide3.xml", "ppt/slides/slide4.xml",
		"ppt/slides/_rels/slide3.xml.rels", "ppt/slides/_rels/slide4.xml.rels",
		"ppt/slideMasters/slideMaster2.xml", "ppt/slideMasters/_rels/slideMaster2.xml.rels",
		"ppt/slideLayouts/slideLayout2.xml", "ppt/slideLayouts/slideLayout3.xml",
		"ppt/theme/theme2.xml", "ppt/media/image5.png",
	} {
		if !names[part] {
			t.Errorf("注入 part %s 应存在", part)
		}
	}

	// 注入母版注册：sldMasterIdLst 两条（基底 rId1 + 注入 rId6）；presentation.xml
	// sldIdLst 四条按购物栏序（甲页沿用原 rId2/rId3，乙页分配 rId4/rId5）
	presBytes := mustZipPart(t, &zr.Reader, pptxPresentationPart)
	var meta pptxPresentationMeta
	if err := xml.Unmarshal(presBytes, &meta); err != nil {
		t.Fatalf("presentation.xml 解析: %v", err)
	}
	if len(meta.SldMasterIdLst.SldMasterIds) != 2 ||
		meta.SldMasterIdLst.SldMasterIds[0].RID != "rId1" ||
		meta.SldMasterIdLst.SldMasterIds[1].RID != "rId6" {
		t.Errorf("sldMasterIdLst 应为 [rId1 rId6]，实际 %+v", meta.SldMasterIdLst.SldMasterIds)
	}
	wantSldRids := []string{"rId2", "rId4", "rId3", "rId5"}
	if len(meta.SldIdLst.SldIds) != len(wantSldRids) {
		t.Fatalf("sldIdLst 应 %d 条，实际 %d", len(wantSldRids), len(meta.SldIdLst.SldIds))
	}
	for i, rid := range wantSldRids {
		if meta.SldIdLst.SldIds[i].RID != rid {
			t.Errorf("sldIdLst[%d].r:id = %s, want %s", i, meta.SldIdLst.SldIds[i].RID, rid)
		}
	}

	// 注入关系与内容类型：乙页/母版关系追加、乙 part Override 以新 part 名追加
	presRels := mustZipPart(t, &zr.Reader, pptxPresentationRelsPart)
	for _, frag := range []string{
		`Id="rId4"`, `Target="slides/slide3.xml"`,
		`Id="rId5"`, `Target="slides/slide4.xml"`,
		`Id="rId6"`, `Target="slideMasters/slideMaster2.xml"`,
	} {
		if !bytes.Contains(presRels, []byte(frag)) {
			t.Errorf("presentation.xml.rels 应包含 %s", frag)
		}
	}
	ctBytes := mustZipPart(t, &zr.Reader, pptxContentTypesPart)
	for _, frag := range []string{"/ppt/slides/slide3.xml", "/ppt/slides/slide4.xml", "/ppt/slideMasters/slideMaster2.xml", "/ppt/theme/theme2.xml"} {
		if !bytes.Contains(ctBytes, []byte(frag)) {
			t.Errorf("[Content_Types].xml 应包含注入 Override %s", frag)
		}
	}

	// 乙页 rels 重写：版式指向重命名后的 slideLayout2
	b1Rels := mustZipPart(t, &zr.Reader, "ppt/slides/_rels/slide3.xml.rels")
	if !bytes.Contains(b1Rels, []byte(`Target="../slideLayouts/slideLayout2.xml"`)) {
		t.Errorf("注入页 rels 应重写版式指向，实际：%s", b1Rels)
	}
	// 注入 rels 重写保留外部链接：Target 原样透传（不参与映射）且 TargetMode=External 不丢失
	if !bytes.Contains(b1Rels, []byte(`Target="https://example.com/"`)) ||
		!bytes.Contains(b1Rels, []byte(`TargetMode="External"`)) {
		t.Errorf("注入页 rels 应原样保留外部链接与 TargetMode，实际：%s", b1Rels)
	}
	// 乙母版 rels：版式/主题指向全部按映射重写
	bMasterRels := mustZipPart(t, &zr.Reader, "ppt/slideMasters/_rels/slideMaster2.xml.rels")
	for _, frag := range []string{"slideLayout2.xml", "slideLayout3.xml", "theme2.xml"} {
		if !bytes.Contains(bMasterRels, []byte(frag)) {
			t.Errorf("注入母版 rels 应包含重写后的 %s 关系", frag)
		}
	}

	// 包完整性：基底保留 part 与注入 part 本体均逐字节一致（新名对应源 part）
	kept := map[string][]byte{
		// 基底甲：未触碰 part 原样
		"ppt/slides/slide1.xml":             builtA.parts["ppt/slides/slide1.xml"],
		"ppt/slides/slide2.xml":             builtA.parts["ppt/slides/slide2.xml"],
		"ppt/slideMasters/slideMaster1.xml": builtA.parts["ppt/slideMasters/slideMaster1.xml"],
		"ppt/slideLayouts/slideLayout1.xml": builtA.parts["ppt/slideLayouts/slideLayout1.xml"],
		"ppt/theme/theme1.xml":              builtA.parts["ppt/theme/theme1.xml"],
		// 注入乙：part 本体逐字节拷贝（新名）
		"ppt/slides/slide3.xml":             builtB.parts["ppt/slides/slide1.xml"],
		"ppt/slides/slide4.xml":             builtB.parts["ppt/slides/slide2.xml"],
		"ppt/slideMasters/slideMaster2.xml": builtB.parts["ppt/slideMasters/slideMaster1.xml"],
		"ppt/slideLayouts/slideLayout2.xml": builtB.parts["ppt/slideLayouts/slideLayout1.xml"],
		"ppt/slideLayouts/slideLayout3.xml": builtB.parts["ppt/slideLayouts/slideLayout2.xml"],
		"ppt/theme/theme2.xml":              builtB.parts["ppt/theme/theme1.xml"],
		"ppt/media/image5.png":              builtB.parts["ppt/media/image5.png"],
	}
	assertZipIntegrity(t, out, &zr.Reader, names, kept)
	assertNoTempResidue(t, dir)
}

// TestMergePptxPages_ThreeSourcesCartOrder 三源合并：输出页序严格等于购物栏清单
// 顺序（含同基底页被其他源页隔开的交错形态），页明细回显源文件与源页号；尺寸
// 混排（丙源 sldSz 与基底不同）置 SldSzMixed 标志。
func TestMergePptxPages_ThreeSourcesCartOrder(t *testing.T) {
	dir := t.TempDir()
	builtA := buildPptxFixture(t, dir, "甲.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "甲一"},
			{fileName: "slide2.xml", marker: "甲二"},
		},
	})
	builtB := buildPptxFixture(t, dir, "乙.pptx", pptxFixture{
		slides: []pptxFixtureSlide{{fileName: "slide1.xml", marker: "乙一"}},
	})
	builtC := buildPptxFixture(t, dir, "丙.pptx", pptxFixture{
		slides:  []pptxFixtureSlide{{fileName: "slide1.xml", marker: "丙一"}},
		sldSzCx: "9144000",
	})

	out := filepath.Join(dir, "三源合并.pptx")
	items := []model.PptxMergeItem{
		{SrcPath: builtB.src, PageNum: 1},
		{SrcPath: builtA.src, PageNum: 2},
		{SrcPath: builtC.src, PageNum: 1},
		{SrcPath: builtA.src, PageNum: 1},
	}
	result, err := MergePptxPages(context.Background(), items, out, nil)
	if err != nil {
		t.Fatalf("MergePptxPages: %v", err)
	}
	if !result.SldSzMixed {
		t.Error("丙源尺寸与基底不同应置 SldSzMixed")
	}
	got := readMergedPageMarkers(t, out)
	want := []string{"乙一", "甲二", "丙一", "甲一"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("输出页序[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// 基底 = 清单第一项所在源文件（乙），甲为注入源：甲 part 冲突重命名
	zr, names := openMergedOutput(t, out)
	defer zr.Close()
	if !names["ppt/slides/slide2.xml"] || !names["ppt/slides/slide3.xml"] {
		t.Error("注入甲源两页应重命名为 slide2/slide3（乙占 slide1）")
	}
}

// TestMergePptxPages_IDCollisionResolution 三重命名空间冲突消解：基底 rId 最大
// 数字后缀已占至 rId100（extraPresRels 占位）、sldId 数字 id 起点抬高位——注入页
// rId 从基底 max+1 起分配（rId101），注入母版紧随（rId102）；sldId 数字 id 取
// 基底 max+1（2147483602），sldMasterId 数字 id 取基底 max+1（2147483649）。
// 三个命名空间互不相干、互不冲突。
func TestMergePptxPages_IDCollisionResolution(t *testing.T) {
	dir := t.TempDir()
	builtA := buildPptxFixture(t, dir, "高位基底.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "甲一"},
			{fileName: "slide2.xml", marker: "甲二"},
		},
		extraPresRels: []string{
			`<Relationship Id="rId100" Type="` + testOfficeRelNS + `/slide" Target="presentation.xml"/>`,
		},
		sldIdStart: 2147483600,
	})
	builtB := buildPptxFixture(t, dir, "低位注入.pptx", pptxFixture{
		slides: []pptxFixtureSlide{{fileName: "slide1.xml", marker: "乙一"}},
	})

	out := filepath.Join(dir, "冲突消解.pptx")
	items := []model.PptxMergeItem{
		{SrcPath: builtA.src, PageNum: 1},
		{SrcPath: builtB.src, PageNum: 1},
		{SrcPath: builtA.src, PageNum: 2},
	}
	if _, err := MergePptxPages(context.Background(), items, out, nil); err != nil {
		t.Fatalf("MergePptxPages: %v", err)
	}

	zr, names := openMergedOutput(t, out)
	defer zr.Close()
	// 输出条目数字 id 断言用命名空间精确解析（xml:"id,attr" 标签恒命中 r:id，
	// 见 parsePptxListIDs 实现说明）
	outSldIDs, outMasterIDs, err := parsePptxListIDs(mustZipPart(t, &zr.Reader, pptxPresentationPart))
	if err != nil {
		t.Fatalf("presentation.xml 解析: %v", err)
	}
	// sldIdLst：基底页沿用原 id（2147483600/2147483601），注入页取基底 max+1
	wantSld := []struct {
		id  int
		rid string
	}{{2147483600, "rId2"}, {2147483602, "rId101"}, {2147483601, "rId3"}}
	if len(outSldIDs) != len(wantSld) {
		t.Fatalf("sldIdLst 应 %d 条，实际 %d", len(wantSld), len(outSldIDs))
	}
	for i, w := range wantSld {
		if outSldIDs[i].NumID != w.id || outSldIDs[i].RID != w.rid {
			t.Errorf("sldIdLst[%d] = {id=%d rid=%s}, want {id=%d rid=%s}",
				i, outSldIDs[i].NumID, outSldIDs[i].RID, w.id, w.rid)
		}
	}
	// sldMasterIdLst：基底 2147483648/rId1 + 注入 2147483649/rId102
	wantMaster := []struct {
		id  int
		rid string
	}{{2147483648, "rId1"}, {2147483649, "rId102"}}
	if len(outMasterIDs) != len(wantMaster) {
		t.Fatalf("sldMasterIdLst 应 %d 条，实际 %d", len(wantMaster), len(outMasterIDs))
	}
	for i, w := range wantMaster {
		if outMasterIDs[i].NumID != w.id || outMasterIDs[i].RID != w.rid {
			t.Errorf("sldMasterIdLst[%d] = {id=%d rid=%s}, want {id=%d rid=%s}",
				i, outMasterIDs[i].NumID, outMasterIDs[i].RID, w.id, w.rid)
		}
	}
	// 基底占位关系 rId100 保留、注入页关系 rId101 追加
	presRels := mustZipPart(t, &zr.Reader, pptxPresentationRelsPart)
	if !bytes.Contains(presRels, []byte(`Id="rId100"`)) {
		t.Error("基底占位关系 rId100 应原样保留")
	}
	if !bytes.Contains(presRels, []byte(`Id="rId101"`)) || !bytes.Contains(presRels, []byte(`Target="slides/slide3.xml"`)) {
		t.Errorf("注入页关系应为 rId101 → 注入后 part 名 slide3.xml，实际：%s", presRels)
	}
	_ = names
}

// TestMergePptxPages_UnselectedJumpLinksDropped 未勾选页跨页跳转关系清理：注入页
// 指向未勾选页（屏障）的跳转关系删除、指向同源被勾选页的跳转重写到注入后 part
// 名（拆分「指向已删页关系删除」语义的注入版）。
func TestMergePptxPages_UnselectedJumpLinksDropped(t *testing.T) {
	dir := t.TempDir()
	builtA := buildPptxFixture(t, dir, "基底.pptx", pptxFixture{
		slides: []pptxFixtureSlide{{fileName: "slide1.xml", marker: "甲一"}},
	})
	builtB := buildPptxFixture(t, dir, "跳转.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "乙一", jumpsTo: []string{"slide2.xml", "slide3.xml"}},
			{fileName: "slide2.xml", marker: "乙二"},
			{fileName: "slide3.xml", marker: "乙三"},
		},
	})

	out := filepath.Join(dir, "跳转清理.pptx")
	// 勾选乙 1、3；乙 2 未勾选（屏障）
	items := []model.PptxMergeItem{
		{SrcPath: builtA.src, PageNum: 1},
		{SrcPath: builtB.src, PageNum: 1},
		{SrcPath: builtB.src, PageNum: 3},
	}
	if _, err := MergePptxPages(context.Background(), items, out, nil); err != nil {
		t.Fatalf("MergePptxPages: %v", err)
	}

	got := readMergedPageMarkers(t, out)
	want := []string{"甲一", "乙一", "乙三"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("输出页序[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	zr, names := openMergedOutput(t, out)
	defer zr.Close()
	// 乙 part 注入名：基底占 slide1 → 乙 slide1 冲突改名 slide2；乙 slide3 无冲突
	// 保留原名 slide3；乙 slide2 未勾选不注入（slide4 也不应出现）
	if !names["ppt/slides/slide1.xml"] || !names["ppt/slides/slide2.xml"] || !names["ppt/slides/slide3.xml"] {
		t.Fatalf("注入 part 命名不符（基底 slide1 + 乙 slide2/slide3）")
	}
	if names["ppt/slides/slide4.xml"] {
		t.Error("未勾选页（乙 2）不应注入")
	}
	// 乙 1 源 rels：rId1 版式 / rId2 跳转未勾选页（删）/ rId3 跳转被勾选页（重写保留）
	b1Rels := mustZipPart(t, &zr.Reader, "ppt/slides/_rels/slide2.xml.rels")
	if bytes.Contains(b1Rels, []byte(`Id="rId2"`)) {
		t.Error("指向未勾选页的跳转关系（rId2）应被删除")
	}
	if !bytes.Contains(b1Rels, []byte(`Id="rId3"`)) || !bytes.Contains(b1Rels, []byte(`Target="slide3.xml"`)) {
		t.Errorf("指向同源被勾选页的跳转应保留并重写到注入后 part 名 slide3.xml，实际：%s", b1Rels)
	}
	if !bytes.Contains(b1Rels, []byte(`Target="../slideLayouts/slideLayout2.xml"`)) {
		t.Errorf("版式指向应重写到注入后 part 名，实际：%s", b1Rels)
	}
}

// TestMergePptxPages_MediaDefaultBackfilled 媒体扩展 Default 补齐：基底仅 png，
// 注入源带 gif 媒体——输出 [Content_Types].xml 补 gif Default，gif part 逐字节
// 拷贝、rels 指向重写正确。
func TestMergePptxPages_MediaDefaultBackfilled(t *testing.T) {
	dir := t.TempDir()
	builtA := buildPptxFixture(t, dir, "基底媒体.pptx", pptxFixture{
		slides: []pptxFixtureSlide{{fileName: "slide1.xml", marker: "甲一"}},
		media:  true,
	})
	builtB := buildPptxFixture(t, dir, "gif源.pptx", pptxFixture{
		slides: []pptxFixtureSlide{{fileName: "slide1.xml", marker: "乙一", mediaFile: "image2.gif"}},
	})

	out := filepath.Join(dir, "媒体补齐.pptx")
	items := []model.PptxMergeItem{
		{SrcPath: builtA.src, PageNum: 1},
		{SrcPath: builtB.src, PageNum: 1},
	}
	if _, err := MergePptxPages(context.Background(), items, out, nil); err != nil {
		t.Fatalf("MergePptxPages: %v", err)
	}

	zr, names := openMergedOutput(t, out)
	defer zr.Close()
	ctBytes := mustZipPart(t, &zr.Reader, pptxContentTypesPart)
	if !bytes.Contains(ctBytes, []byte(`Extension="png"`)) {
		t.Error("基底 png Default 应保留")
	}
	if !bytes.Contains(ctBytes, []byte(`Extension="gif"`)) {
		t.Error("基底缺失的 gif 扩展名应补 Default")
	}
	if !names["ppt/media/image2.gif"] {
		t.Fatal("注入 gif 媒体 part 应存在（无命名冲突保留原名）")
	}
	if got := mustZipPart(t, &zr.Reader, "ppt/media/image2.gif"); !bytes.Equal(got, pptxFixtureMediaBytes) {
		t.Error("注入媒体 part 应逐字节一致")
	}
	b1Rels := mustZipPart(t, &zr.Reader, "ppt/slides/_rels/slide2.xml.rels")
	if !bytes.Contains(b1Rels, []byte(`Target="../media/image2.gif"`)) {
		t.Errorf("注入页 rels 媒体指向应保留（原名无冲突），实际：%s", b1Rels)
	}
}

// TestMergePptxPages_EmbeddedFontStripped 嵌入字体剥离（MVP 边界）：基底
// embeddedFontLst 与 fntdata part 整体移除，presentation.xml.rels 字体关系清理，
// presentation.xml 无残留 r:id 悬空引用（assertZipIntegrity 反向校验覆盖）；注入
// 源的嵌入字体结构性不注入（字体关系挂 presentation.xml 级，注入闭包根仅保留页，
// fntdata 不可达），输出同样无残留。
func TestMergePptxPages_EmbeddedFontStripped(t *testing.T) {
	dir := t.TempDir()
	builtA := buildPptxFixture(t, dir, "嵌入字体.pptx", pptxFixture{
		slides:       []pptxFixtureSlide{{fileName: "slide1.xml", marker: "甲一"}},
		embeddedFont: true,
	})
	builtB := buildPptxFixture(t, dir, "字体注入源.pptx", pptxFixture{
		slides:       []pptxFixtureSlide{{fileName: "slide1.xml", marker: "乙一"}},
		embeddedFont: true,
	})

	out := filepath.Join(dir, "字体剥离.pptx")
	items := []model.PptxMergeItem{
		{SrcPath: builtA.src, PageNum: 1},
		{SrcPath: builtB.src, PageNum: 1},
	}
	if _, err := MergePptxPages(context.Background(), items, out, nil); err != nil {
		t.Fatalf("MergePptxPages: %v", err)
	}

	zr, names := openMergedOutput(t, out)
	defer zr.Close()
	presBytes := mustZipPart(t, &zr.Reader, pptxPresentationPart)
	if bytes.Contains(presBytes, []byte("embeddedFontLst")) {
		t.Error("presentation.xml 不应残留 embeddedFontLst")
	}
	for name := range names {
		if strings.Contains(name, "ppt/fonts/") {
			t.Errorf("fntdata part %s 应随嵌入字体剥离移除（含注入源不可达形态）", name)
		}
	}
	presRels := mustZipPart(t, &zr.Reader, pptxPresentationRelsPart)
	if bytes.Contains(presRels, []byte("fntdata")) {
		t.Error("presentation.xml.rels 不应残留字体关系")
	}
	assertZipIntegrity(t, out, &zr.Reader, names, nil)
}

// TestMergePptxPages_SkippedItemsReported 跳过语义：非基底源缺失/页号越界/重复项
// 跳过并在结果中报告，其余页正常导出；基底缺失整体失败；全部无效报引导错误。
func TestMergePptxPages_SkippedItemsReported(t *testing.T) {
	dir := t.TempDir()
	builtA := buildPptxFixture(t, dir, "清单.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "甲一"},
			{fileName: "slide2.xml", marker: "甲二"},
		},
	})
	missing := filepath.Join(dir, "不存在.pptx")
	out := filepath.Join(dir, "跳过报告.pptx")

	result, err := MergePptxPages(context.Background(), []model.PptxMergeItem{
		{SrcPath: builtA.src, PageNum: 1},
		{SrcPath: missing, PageNum: 1},
		{SrcPath: builtA.src, PageNum: 5},
		{SrcPath: builtA.src, PageNum: 1}, // 重复项
	}, out, nil)
	if err != nil {
		t.Fatalf("MergePptxPages: %v", err)
	}
	if result.PageCount != 1 || len(result.Pages) != 1 || result.Pages[0].SrcPageNum != 1 {
		t.Errorf("仅基底第 1 页有效，实际：%+v", result.Pages)
	}
	if got := readMergedPageMarkers(t, out); len(got) != 1 || got[0] != "甲一" {
		t.Errorf("输出页不符：%v", got)
	}
	if len(result.SkippedItems) != 3 {
		t.Fatalf("应报告 3 个跳过项，实际 %d：%+v", len(result.SkippedItems), result.SkippedItems)
	}
	reasons := make([]string, 0, 3)
	for _, s := range result.SkippedItems {
		reasons = append(reasons, s.Reason)
	}
	joined := strings.Join(reasons, "\n")
	for _, frag := range []string{"无法访问", "超出范围", "重复"} {
		if !strings.Contains(joined, frag) {
			t.Errorf("跳过原因应包含 %q，实际：%s", frag, joined)
		}
	}
	assertNoTempResidue(t, dir)

	t.Run("基底文件缺失整体失败", func(t *testing.T) {
		_, err := MergePptxPages(context.Background(), []model.PptxMergeItem{
			{SrcPath: missing, PageNum: 1},
			{SrcPath: builtA.src, PageNum: 1},
		}, filepath.Join(dir, "整体失败.pptx"), nil)
		if err == nil || !strings.Contains(err.Error(), "无法访问") {
			t.Errorf("基底缺失应整体失败，实际：%v", err)
		}
	})
	t.Run("全部无效页报引导错误", func(t *testing.T) {
		_, err := MergePptxPages(context.Background(), []model.PptxMergeItem{
			{SrcPath: builtA.src, PageNum: 99},
		}, filepath.Join(dir, "全无效.pptx"), nil)
		if err == nil || !strings.Contains(err.Error(), "没有可导出的有效页") {
			t.Errorf("全部无效应报引导错误，实际：%v", err)
		}
	})
	t.Run("空清单拒绝", func(t *testing.T) {
		_, err := MergePptxPages(context.Background(), nil, filepath.Join(dir, "空.pptx"), nil)
		if err == nil || !strings.Contains(err.Error(), "请先选择要合并导出的页") {
			t.Errorf("空清单应拒绝，实际：%v", err)
		}
	})
	t.Run("导出路径与源文件相同拒绝", func(t *testing.T) {
		_, err := MergePptxPages(context.Background(), []model.PptxMergeItem{
			{SrcPath: builtA.src, PageNum: 1},
		}, builtA.src, nil)
		if err == nil || !strings.Contains(err.Error(), "导出路径不能与源文件相同") {
			t.Errorf("同路径导出应拒绝，实际：%v", err)
		}
	})
}

// TestMergePptxPages_CanceledContext 预取消 ctx：立即返回取消错误且保留
// errors.Is(err, context.Canceled) 穿透判定，不产生输出文件。
func TestMergePptxPages_CanceledContext(t *testing.T) {
	dir := t.TempDir()
	builtA := buildPptxFixture(t, dir, "取消场景.pptx", pptxFixture{
		slides: []pptxFixtureSlide{{fileName: "slide1.xml", marker: "唯一一页"}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out := filepath.Join(dir, "取消.pptx")

	_, err := MergePptxPages(ctx, []model.PptxMergeItem{{SrcPath: builtA.src, PageNum: 1}}, out, nil)
	if err == nil {
		t.Fatal("已取消的 ctx 应报错")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("取消错误应保留 context.Canceled 穿透，实际：%v", err)
	}
	if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
		t.Error("取消路径不应产生输出文件")
	}
}

// TestMergePptxPages_SingleSourceMatchesSubset 单源清单回归对照：单源部分页合并
// 与 PR1 子集导出语义一致——sldIdLst 页序/rId 一致、页内容一致；差异面仅合并
// MVP 特有的备注页剔除（子集导出保留勾选页备注，合并剔除）。
func TestMergePptxPages_SingleSourceMatchesSubset(t *testing.T) {
	dir := t.TempDir()
	builtA := buildPptxFixture(t, dir, "对照.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide3.xml", marker: "第三页内容", notes: true},
			{fileName: "slide1.xml", marker: "第一页内容", notes: true},
			{fileName: "slide2.xml", marker: "第二页内容"},
			{fileName: "slide4.xml", marker: "第四页内容"},
		},
		withSections: true,
	})

	subsetOut := filepath.Join(dir, "子集.pptx")
	subset, err := ExportPptxPageSubset(context.Background(), builtA.src, []int{1, 3}, subsetOut)
	if err != nil {
		t.Fatalf("ExportPptxPageSubset: %v", err)
	}
	mergeOut := filepath.Join(dir, "合并.pptx")
	merge, err := MergePptxPages(context.Background(), []model.PptxMergeItem{
		{SrcPath: builtA.src, PageNum: 1},
		{SrcPath: builtA.src, PageNum: 3},
	}, mergeOut, nil)
	if err != nil {
		t.Fatalf("MergePptxPages: %v", err)
	}
	if subset.PageCount != 2 || merge.PageCount != 2 {
		t.Fatalf("两侧均应导出 2 页：子集 %d，合并 %d", subset.PageCount, merge.PageCount)
	}

	subsetMarkers := readMergedPageMarkers(t, subsetOut)
	mergeMarkers := readMergedPageMarkers(t, mergeOut)
	if len(subsetMarkers) != 2 || len(mergeMarkers) != 2 ||
		subsetMarkers[0] != mergeMarkers[0] || subsetMarkers[1] != mergeMarkers[1] {
		t.Errorf("单源合并与子集导出页序应一致：子集 %v，合并 %v", subsetMarkers, mergeMarkers)
	}

	// 页 rId 序一致（单源合并沿用基底原 rId，与子集导出的保留口径相同）
	readSldRids := func(path string) []string {
		zr, err := zip.OpenReader(path)
		if err != nil {
			t.Fatalf("打开 %s: %v", filepath.Base(path), err)
		}
		defer zr.Close()
		var meta pptxPresentationMeta
		if err := xml.Unmarshal(mustZipPart(t, &zr.Reader, pptxPresentationPart), &meta); err != nil {
			t.Fatalf("presentation.xml 解析: %v", err)
		}
		rids := make([]string, 0, len(meta.SldIdLst.SldIds))
		for _, e := range meta.SldIdLst.SldIds {
			rids = append(rids, e.RID)
		}
		return rids
	}
	subsetRids, mergeRids := readSldRids(subsetOut), readSldRids(mergeOut)
	for i := range subsetRids {
		if subsetRids[i] != mergeRids[i] {
			t.Errorf("sldIdLst[%d] 应一致：子集 %s，合并 %s", i, subsetRids[i], mergeRids[i])
		}
	}

	// 合并 MVP 差异面：备注页剔除（子集导出保留勾选页备注）
	zr, names := openMergedOutput(t, mergeOut)
	defer zr.Close()
	if names["ppt/notesSlides/notesSlide1.xml"] || names["ppt/notesSlides/notesSlide3.xml"] {
		t.Error("合并输出不应携带备注页（MVP 剔除语义）")
	}
	assertZipIntegrity(t, mergeOut, &zr.Reader, names, nil)
}

// ---------- 服务编排用例 ----------

// fakePptxMergeProvider 合并能力 fake：记录调用参数，返回预置结果/错误；
// progress 序列在返回前重放（驱动编排层事件断言）；blockUntilCancel 时阻塞至
// ctx 取消（驱动取消注册表断言）。
type fakePptxMergeProvider struct {
	gotCtx           context.Context
	gotItems         []model.PptxMergeItem
	gotOutPath       string
	gotProgress      []fakeProgressCall
	progress         []fakeProgressCall
	result           model.PptxMergeResult
	err              error
	blockUntilCancel bool
	started          chan struct{}
}

func (f *fakePptxMergeProvider) Merge(ctx context.Context, items []model.PptxMergeItem, outPath string, onProgress func(done, total int)) (model.PptxMergeResult, error) {
	f.gotCtx = ctx
	f.gotItems = items
	f.gotOutPath = outPath
	if f.started != nil {
		close(f.started)
	}
	if f.blockUntilCancel {
		<-ctx.Done()
		return model.PptxMergeResult{}, ctx.Err()
	}
	for _, p := range f.progress {
		f.gotProgress = append(f.gotProgress, p)
		if onProgress != nil {
			onProgress(p.done, p.total)
		}
	}
	return f.result, f.err
}

// TestPptxMergeService_DelegatesAndEmits 服务编排透传：ctx/清单/输出路径原样下发，
// 结果与错误按口径上抛；进度逐次转译为 assets-cart:export-progress 事件，终态
// export-done / export-error 载荷齐全。
func TestPptxMergeService_DelegatesAndEmits(t *testing.T) {
	want := model.PptxMergeResult{
		OutputPath: "/tmp/合并.pptx",
		Pages:      []model.PptxMergedPage{{SrcPath: "/tmp/甲.pptx", SrcPageNum: 1, OutPageNum: 1}},
		PageCount:  1,
	}
	fake := &fakePptxMergeProvider{result: want, progress: []fakeProgressCall{{done: 1, total: 2}, {done: 2, total: 2}}}
	svc := &PptxMergeService{provider: fake}
	sink := &fakeEventSink{}
	svc.SetEventSink(sink)

	items := []model.PptxMergeItem{{SrcPath: "/tmp/甲.pptx", PageNum: 1}}
	ctx := context.Background()
	got, err := svc.Merge(ctx, items, "/tmp/合并.pptx")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got.OutputPath != want.OutputPath || got.PageCount != want.PageCount {
		t.Errorf("结果透传不符：%+v", got)
	}
	if fake.gotCtx == nil || fake.gotCtx == ctx {
		t.Error("应向 provider 下发派生的可取消 ctx（非调用方 ctx 原样）")
	}
	if fake.gotOutPath != "/tmp/合并.pptx" || len(fake.gotItems) != 1 {
		t.Errorf("清单/输出路径应原样下发：%+v", fake.gotItems)
	}

	calls := sinkSnapshot(sink)
	var progressCount, doneCount int
	for _, c := range calls {
		switch c.name {
		case "assets-cart:export-progress":
			progressCount++
			payload := c.data[0].(map[string]any)
			if payload["outPath"] != "/tmp/合并.pptx" {
				t.Errorf("进度事件载荷 outPath 不符：%+v", payload)
			}
		case "assets-cart:export-done":
			doneCount++
			payload := c.data[0].(map[string]any)
			if payload["outPath"] != "/tmp/合并.pptx" {
				t.Errorf("完成事件载荷 outPath 不符：%+v", payload)
			}
		}
	}
	if progressCount != 2 || doneCount != 1 {
		t.Errorf("事件序列不符：progress=%d done=%d", progressCount, doneCount)
	}

	// 错误路径：零值结果 + export-error 事件
	fake.err = errors.New("boom")
	got, err = svc.Merge(ctx, items, "/tmp/合并.pptx")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("provider 错误应上抛，实际：%v", err)
	}
	if got.OutputPath != "" || got.Pages != nil || got.PageCount != 0 {
		t.Errorf("错误时应返回零值结果，实际：%+v", got)
	}
	found := false
	for _, c := range sinkSnapshot(sink) {
		if c.name == "assets-cart:export-error" {
			found = true
			payload := c.data[0].(map[string]any)
			if payload["message"] != "boom" {
				t.Errorf("错误事件载荷 message 不符：%+v", payload)
			}
		}
	}
	if !found {
		t.Error("错误路径应发出 assets-cart:export-error 事件")
	}
}

// TestPptxMergeService_Cancel 取消注册表：CancelPptxMerge 命中在途导出（provider
// 收到取消 ctx），取消错误映射 ErrOperationCancelled（E_GIT_CANCELLED）；导出结束
// 后注册表无残留（再次取消幂等 false）。
func TestPptxMergeService_Cancel(t *testing.T) {
	fake := &fakePptxMergeProvider{blockUntilCancel: true, started: make(chan struct{})}
	svc := &PptxMergeService{provider: fake}

	type mergeOutcome struct {
		err error
	}
	done := make(chan mergeOutcome, 1)
	go func() {
		_, err := svc.Merge(context.Background(), []model.PptxMergeItem{{SrcPath: "/tmp/甲.pptx", PageNum: 1}}, "/tmp/合并.pptx")
		done <- mergeOutcome{err: err}
	}()
	<-fake.started
	if !svc.CancelPptxMerge() {
		t.Fatal("应命中至少一个在途导出")
	}
	outcome := <-done
	if outcome.err == nil {
		t.Fatal("取消后 Merge 应返回错误")
	}
	var appErr *model.AppError
	if !errors.As(outcome.err, &appErr) || appErr.Code != model.ErrCodeGitCancelled {
		t.Errorf("取消错误应映射 E_GIT_CANCELLED，实际：%v", outcome.err)
	}
	if svc.CancelPptxMerge() {
		t.Error("导出结束并注销后再次取消应幂等返回 false")
	}
}

// TestPptxMergeService_DefaultProviderIsCore 生产构造默认接纯 Go 核心：空清单走到
// 核心校验（报「请先选择要合并导出的页」）即证明链路接通。
func TestPptxMergeService_DefaultProviderIsCore(t *testing.T) {
	svc := NewPptxMergeService(context.Background())
	_, err := svc.Merge(context.Background(), nil, filepath.Join(t.TempDir(), "out.pptx"))
	if err == nil || !strings.Contains(err.Error(), "请先选择要合并导出的页") {
		t.Fatalf("默认 provider 应为真实核心（空清单校验生效），实际：%v", err)
	}
}

// sinkSnapshot 读取 fake 出口的调用快照（并发安全拷贝）。
func sinkSnapshot(sink *fakeEventSink) []fakeEmitCall {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]fakeEmitCall(nil), sink.calls...)
}

// 并发纪律说明：fakePptxMergeProvider 的 started 通道由 provider 首次 Merge 关闭
//（单 goroutine 调用保证恰好一次），测试中 Merge 仅从单个 goroutine 调用，无需
// 额外互斥；fakeEventSink 自带锁（复用 event_sink_test.go 既有实现）。
