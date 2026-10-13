package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- fixture 程序化构造 ----------
//
// 最小合法 pptx（OPC zip 包）：不做 PowerPoint 级校验，但结构与真实包一致
// （页序经 sldIdLst → rId → rels 三跳映射、notesSlide 关系双向指回 slide、
// 节信息挂根级 extLst），足以驱动拆分核心全链路。禁依赖本机 Office。

const (
	pptxFixtureXMLDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
	testPackageRelsNS  = "http://schemas.openxmlformats.org/package/2006/relationships"
	testOfficeRelNS    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

// pptxFixtureMediaBytes 伪 PNG 字节（魔数 + 区分度 payload），二进制 part 逐字节断言基准。
var pptxFixtureMediaBytes = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x01, 0x02, 0x03}

// pptxFixtureSlide fixture 中一页的定义。
type pptxFixtureSlide struct {
	fileName string   // ppt/slides/ 下的 part 文件名，可与展示页序无关（页序以 sldIdLst 为准）
	marker   string   // 幻灯片 XML 中的可识别文本（断言页内容映射）
	notes    bool     // 是否生成备注页（notesSlide 按展示页序编号）
	jumpsTo  []string // 内部跳转目标 slide 文件名（跨页超链接/动作按钮形态），可指向自身
	extLink  bool     // 是否带外部 URL 超链接关系（TargetMode=External，拆分必须保留）
}

// pptxFixture 拆分 fixture 的整体定义。
type pptxFixture struct {
	slides       []pptxFixtureSlide // 顺序即 sldIdLst 展示页序
	withSections bool               // 是否生成 custShowLst 与根级 extLst（p14:sectionLst）
	media        bool               // 是否生成 media 图片 part 并由第 1 页（展示序）引用
}

// pptxFixtureBuild 构造产物：src 源文件路径；parts 为全部 part 原始字节
// （保留 part 逐字节一致断言的基准）；order 为 zip 写入顺序。
type pptxFixtureBuild struct {
	src   string
	parts map[string][]byte
	order []string
}

const pptxFixtureRootRels = pptxFixtureXMLDecl +
	`<Relationships xmlns="` + testPackageRelsNS + `">` +
	`<Relationship Id="rId1" Type="` + testOfficeRelNS + `/officeDocument" Target="ppt/presentation.xml"/>` +
	`</Relationships>`

const pptxFixtureMasterXML = pptxFixtureXMLDecl +
	`<p:sldMaster xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>`

const pptxFixtureMasterRels = pptxFixtureXMLDecl +
	`<Relationships xmlns="` + testPackageRelsNS + `">` +
	`<Relationship Id="rId1" Type="` + testOfficeRelNS + `/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>` +
	`</Relationships>`

const pptxFixtureLayoutXML = pptxFixtureXMLDecl +
	`<p:sldLayout xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>`

const pptxFixtureLayoutRels = pptxFixtureXMLDecl +
	`<Relationships xmlns="` + testPackageRelsNS + `">` +
	`<Relationship Id="rId1" Type="` + testOfficeRelNS + `/slideMaster" Target="../slideMasters/slideMaster1.xml"/>` +
	`</Relationships>`

const pptxFixtureThemeXML = pptxFixtureXMLDecl +
	`<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="主题 1"/>`

const pptxFixtureNotesSlideXML = pptxFixtureXMLDecl +
	`<p:notes xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>`

func pptxFixtureSlideXML(marker string) string {
	return pptxFixtureXMLDecl +
		`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:cSld><p:spTree><a:t>` + marker + `</a:t></p:spTree></p:cSld></p:sld>`
}

func pptxFixtureNotesSlideRels(slideFile string) string {
	return pptxFixtureXMLDecl +
		`<Relationships xmlns="` + testPackageRelsNS + `">` +
		`<Relationship Id="rId1" Type="` + testOfficeRelNS + `/slide" Target="../slides/` + slideFile + `"/>` +
		`</Relationships>`
}

// pptxFixtureContentTypes 包内容类型声明：Default 按扩展名、Override 按 part 声明。
func pptxFixtureContentTypes(slideFiles, notesFiles []string, withMedia bool) string {
	var b strings.Builder
	b.WriteString(pptxFixtureXMLDecl)
	b.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	if withMedia {
		b.WriteString(`<Default Extension="png" ContentType="image/png"/>`)
	}
	b.WriteString(`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>`)
	b.WriteString(`<Override PartName="/ppt/slideMasters/slideMaster1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideMaster+xml"/>`)
	b.WriteString(`<Override PartName="/ppt/slideLayouts/slideLayout1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slideLayout+xml"/>`)
	b.WriteString(`<Override PartName="/ppt/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>`)
	for _, f := range slideFiles {
		b.WriteString(`<Override PartName="/ppt/slides/` + f + `" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>`)
	}
	for _, n := range notesFiles {
		b.WriteString(`<Override PartName="/ppt/notesSlides/` + n + `" ContentType="application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml"/>`)
	}
	b.WriteString(`</Types>`)
	return b.String()
}

// pptxFixturePresentationXML presentation.xml：sldIdLst 页条目（页序权威来源），
// withSections 时附 custShowLst（按页 rId 引用）与根级 extLst（p14:sectionLst 按数字 id 引用页）。
func pptxFixturePresentationXML(rids []string, withSections bool) string {
	var b strings.Builder
	b.WriteString(pptxFixtureXMLDecl)
	b.WriteString(`<p:presentation xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:p14="http://schemas.microsoft.com/office/powerpoint/2010/main">`)
	b.WriteString(`<p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>`)
	b.WriteString(`<p:sldIdLst>`)
	for i, rid := range rids {
		fmt.Fprintf(&b, `<p:sldId id="%d" r:id="%s"/>`, 256+i, rid)
	}
	b.WriteString(`</p:sldIdLst>`)
	b.WriteString(`<p:sldSz cx="12192000" cy="6858000"/>`)
	if withSections {
		b.WriteString(`<p:custShowLst><p:custShow name="放映 1" id="0"><p:sldLst>`)
		for _, rid := range rids {
			fmt.Fprintf(&b, `<p:sldId r:id="%s"/>`, rid)
		}
		b.WriteString(`</p:sldLst></p:custShow></p:custShowLst>`)
		b.WriteString(`<p:extLst><p:ext uri="{521415D9-36F7-43E2-AB2F-B90AF26B5E84}"><p14:sectionLst>`)
		b.WriteString(`<p14:section name="第 1 节" id="{10000000-0000-0000-0000-000000000001}"><p14:sldIdLst>`)
		for i := range rids {
			fmt.Fprintf(&b, `<p14:sldId id="%d"/>`, 256+i)
		}
		b.WriteString(`</p14:sldIdLst></p14:section></p14:sectionLst></p:ext></p:extLst>`)
	}
	b.WriteString(`</p:presentation>`)
	return b.String()
}

// pptxFixturePresentationRels presentation.xml.rels：rId1 母版，rId2.. 按展示页序对应各 slide 文件。
func pptxFixturePresentationRels(slideFiles []string) string {
	var b strings.Builder
	b.WriteString(pptxFixtureXMLDecl)
	b.WriteString(`<Relationships xmlns="` + testPackageRelsNS + `">`)
	b.WriteString(`<Relationship Id="rId1" Type="` + testOfficeRelNS + `/slideMaster" Target="slideMasters/slideMaster1.xml"/>`)
	for i, f := range slideFiles {
		fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="%s/slide" Target="slides/%s"/>`, i+2, testOfficeRelNS, f)
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

// pptxFixtureSlideRels 单页 rels：版式必带，备注页/内部跳转/外部链接/媒体按定义追加。
func pptxFixtureSlideRels(notesFile string, jumpsTo []string, extLink, media bool) string {
	var b strings.Builder
	b.WriteString(pptxFixtureXMLDecl)
	b.WriteString(`<Relationships xmlns="` + testPackageRelsNS + `">`)
	fmt.Fprintf(&b, `<Relationship Id="rId1" Type="%s/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>`, testOfficeRelNS)
	next := 2
	add := func(relType, target, mode string) {
		if mode != "" {
			fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="%s%s" Target="%s" TargetMode="%s"/>`, next, testOfficeRelNS, relType, target, mode)
		} else {
			fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="%s%s" Target="%s"/>`, next, testOfficeRelNS, relType, target)
		}
		next++
	}
	if notesFile != "" {
		add("/notesSlide", "../notesSlides/"+notesFile, "")
	}
	for _, j := range jumpsTo {
		add("/slide", j, "")
	}
	if extLink {
		add("/hyperlink", "https://example.com/", "External")
	}
	if media {
		add("/image", "../media/image1.png", "")
	}
	b.WriteString(`</Relationships>`)
	return b.String()
}

// buildPptxFixture 按定义程序化构造 pptx 并写入 dir/name，返回构造产物。
func buildPptxFixture(t *testing.T, dir, name string, fx pptxFixture) *pptxFixtureBuild {
	t.Helper()
	slideFiles := make([]string, len(fx.slides))
	notesFileByIndex := make(map[int]string)
	var notesFiles []string
	for i, s := range fx.slides {
		slideFiles[i] = s.fileName
		if s.notes {
			n := fmt.Sprintf("notesSlide%d.xml", i+1)
			notesFileByIndex[i] = n
			notesFiles = append(notesFiles, n)
		}
	}
	rids := make([]string, len(fx.slides))
	for i := range fx.slides {
		rids[i] = fmt.Sprintf("rId%d", i+2)
	}

	built := &pptxFixtureBuild{parts: make(map[string][]byte)}
	add := func(part, content string) {
		built.parts[part] = []byte(content)
		built.order = append(built.order, part)
	}

	add(pptxContentTypesPart, pptxFixtureContentTypes(slideFiles, notesFiles, fx.media))
	add("_rels/.rels", pptxFixtureRootRels)
	add(pptxPresentationPart, pptxFixturePresentationXML(rids, fx.withSections))
	add(pptxPresentationRelsPart, pptxFixturePresentationRels(slideFiles))
	add("ppt/slideMasters/slideMaster1.xml", pptxFixtureMasterXML)
	add("ppt/slideMasters/_rels/slideMaster1.xml.rels", pptxFixtureMasterRels)
	add("ppt/slideLayouts/slideLayout1.xml", pptxFixtureLayoutXML)
	add("ppt/slideLayouts/_rels/slideLayout1.xml.rels", pptxFixtureLayoutRels)
	add("ppt/theme/theme1.xml", pptxFixtureThemeXML)
	if fx.media {
		add("ppt/media/image1.png", string(pptxFixtureMediaBytes))
	}
	for i, s := range fx.slides {
		slidePart := "ppt/slides/" + s.fileName
		add(slidePart, pptxFixtureSlideXML(s.marker))
		add("ppt/slides/_rels/"+s.fileName+".rels",
			pptxFixtureSlideRels(notesFileByIndex[i], s.jumpsTo, s.extLink, fx.media && i == 0))
		if n, ok := notesFileByIndex[i]; ok {
			add("ppt/notesSlides/"+n, pptxFixtureNotesSlideXML)
			add("ppt/notesSlides/_rels/"+n+".rels", pptxFixtureNotesSlideRels(s.fileName))
		}
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, part := range built.order {
		w, err := zw.Create(part)
		if err != nil {
			t.Fatalf("fixture 构造条目 %s: %v", part, err)
		}
		if _, err := w.Write(built.parts[part]); err != nil {
			t.Fatalf("fixture 写入条目 %s: %v", part, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("fixture 收口 zip: %v", err)
	}
	built.src = filepath.Join(dir, name)
	if err := os.WriteFile(built.src, buf.Bytes(), 0644); err != nil {
		t.Fatalf("fixture 写入源文件: %v", err)
	}
	return built
}

// ---------- 断言辅助 ----------

// mustZipPart 读取 zip 内指定 part 内容，缺失即 Fatal。
func mustZipPart(t *testing.T, r *zip.Reader, name string) []byte {
	t.Helper()
	for _, f := range r.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("打开 %s: %v", name, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("读取 %s: %v", name, err)
			}
			return data
		}
	}
	t.Fatalf("zip 内缺少 part：%s", name)
	return nil
}

// relsSourcePart .rels 文件路径反推其关系源 part（包根 .rels 对应空路径）。
func relsSourcePart(relsName string) string {
	dir, base := path.Split(relsName)
	sourceDir := strings.TrimSuffix(dir, "_rels/")
	name := strings.TrimSuffix(base, ".rels")
	if name == "" {
		return sourceDir
	}
	return sourceDir + name
}

// assertSinglePageOutput 单页输出的通用断言面：
//   - sldIdLst 恰一条且 r:id 为期望值；presentation.xml 无其他页 r:id 引用，
//     无 custShowLst / sectionLst / extLst 残留，未编辑区域（sldSz）保留；
//   - 页 part 存在且含期望内容标记；
//   - 删除集 part 全部不存在；
//   - [Content_Types].xml 全部 Override 指向存在的 part（无悬空声明）；
//   - 全包 .rels 无悬空关系（除 TargetMode=External 外 Target 均可解析到存在的 part）；
//   - 保留 part 与构造基准逐字节一致。
func assertSinglePageOutput(t *testing.T, outPath, wantRid, wantPart, wantMarker string, deletedParts map[string]bool, keptBaselines map[string][]byte, otherRids []string) {
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

	for part := range deletedParts {
		if names[part] {
			t.Errorf("%s 不应包含已删 part %s", filepath.Base(outPath), part)
		}
	}

	presBytes := mustZipPart(t, &zr.Reader, pptxPresentationPart)
	var meta pptxPresentationMeta
	if err := xml.Unmarshal(presBytes, &meta); err != nil {
		t.Fatalf("%s presentation.xml 解析: %v", filepath.Base(outPath), err)
	}
	if len(meta.SldIdLst.SldIds) != 1 {
		t.Fatalf("%s sldIdLst 应恰 1 条，实际 %d 条", filepath.Base(outPath), len(meta.SldIdLst.SldIds))
	}
	if meta.SldIdLst.SldIds[0].RID != wantRid {
		t.Errorf("%s sldIdLst 应保留 %s，实际 %s", filepath.Base(outPath), wantRid, meta.SldIdLst.SldIds[0].RID)
	}
	for _, rid := range otherRids {
		if bytes.Contains(presBytes, []byte(`r:id="`+rid+`"`)) {
			t.Errorf("%s presentation.xml 不应残留已删页引用 %s", filepath.Base(outPath), rid)
		}
	}
	for _, banned := range []string{"custShowLst", "sectionLst", "extLst"} {
		if bytes.Contains(presBytes, []byte(banned)) {
			t.Errorf("%s presentation.xml 不应残留 %s", filepath.Base(outPath), banned)
		}
	}
	if !bytes.Contains(presBytes, []byte("sldSz")) {
		t.Errorf("%s presentation.xml 未编辑区域（sldSz）被改动", filepath.Base(outPath))
	}

	slideBytes := mustZipPart(t, &zr.Reader, wantPart)
	if !bytes.Contains(slideBytes, []byte(wantMarker)) {
		t.Errorf("%s 应包含页内容标记 %q", filepath.Base(outPath), wantMarker)
	}

	ctBytes := mustZipPart(t, &zr.Reader, pptxContentTypesPart)
	var contentTypes struct {
		Overrides []struct {
			PartName string `xml:"PartName,attr"`
		} `xml:"Override"`
	}
	if err := xml.Unmarshal(ctBytes, &contentTypes); err != nil {
		t.Fatalf("%s [Content_Types].xml 解析: %v", filepath.Base(outPath), err)
	}
	for _, ov := range contentTypes.Overrides {
		name := strings.TrimPrefix(ov.PartName, "/")
		if !names[name] {
			t.Errorf("%s Content_Types Override 悬空：%s", filepath.Base(outPath), ov.PartName)
		}
	}

	for _, f := range zr.Reader.File {
		if !strings.HasSuffix(f.Name, ".rels") {
			continue
		}
		var rels pptxRelationships
		if err := xml.Unmarshal(mustZipPart(t, &zr.Reader, f.Name), &rels); err != nil {
			t.Fatalf("%s 解析 %s: %v", filepath.Base(outPath), f.Name, err)
		}
		source := relsSourcePart(f.Name)
		for _, rel := range rels.Relationships {
			if rel.Target == "" || strings.EqualFold(rel.TargetMode, "External") {
				continue
			}
			target := resolveRelTarget(source, rel.Target)
			if !names[target] {
				t.Errorf("%s 关系悬空：%s → %s", f.Name, rel.ID, target)
			}
		}
	}

	for part, want := range keptBaselines {
		if !bytes.Equal(mustZipPart(t, &zr.Reader, part), want) {
			t.Errorf("%s 保留 part %s 内容被改动（期望逐字节一致）", filepath.Base(outPath), part)
		}
	}
}

// assertNoTempResidue 原子写收口断言：输出目录无临时文件残留。
func assertNoTempResidue(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取输出目录 %s: %v", dir, err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("输出目录存在临时文件残留：%s", e.Name())
		}
	}
}

// ---------- 拆分核心用例 ----------

// TestSplitPptxPerPage_MultiPageShuffledFiles 综合主链路：3 页、slide 文件名乱序
// （sldIdLst 页序 slide3 → slide1 → slide2）、含备注页、含外部链接与二进制媒体。
// 断言输出数量/命名/目录、页内容按三跳映射对应、已删 part 不存在、保留 part
// 逐字节一致、Content_Types 与关系表无悬空、无临时文件残留。
func TestSplitPptxPerPage_MultiPageShuffledFiles(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "季度汇报.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide3.xml", marker: "第三页内容", notes: true},
			{fileName: "slide1.xml", marker: "第一页内容", notes: true, extLink: true},
			{fileName: "slide2.xml", marker: "第二页内容"},
		},
		media: true,
	})

	result, err := SplitPptxPerPage(context.Background(), built.src)
	if err != nil {
		t.Fatalf("SplitPptxPerPage: %v", err)
	}

	wantOutDir := filepath.Join(dir, "季度汇报")
	if result.OutputDir != wantOutDir {
		t.Errorf("OutputDir = %q, want %q", result.OutputDir, wantOutDir)
	}
	if result.PageCount != 3 || len(result.Files) != 3 {
		t.Fatalf("应拆出 3 页，实际 PageCount=%d len(Files)=%d", result.PageCount, len(result.Files))
	}
	for i, wantName := range []string{"季度汇报-01.pptx", "季度汇报-02.pptx", "季度汇报-03.pptx"} {
		if result.Files[i] != filepath.Join(wantOutDir, wantName) {
			t.Errorf("Files[%d] = %q, want %q", i, result.Files[i], filepath.Join(wantOutDir, wantName))
		}
	}

	// 共享保留 part（三页输出中均应逐字节一致）
	sharedKept := map[string][]byte{
		"_rels/.rels":                                  built.parts["_rels/.rels"],
		"ppt/slideMasters/slideMaster1.xml":            built.parts["ppt/slideMasters/slideMaster1.xml"],
		"ppt/slideMasters/_rels/slideMaster1.xml.rels": built.parts["ppt/slideMasters/_rels/slideMaster1.xml.rels"],
		"ppt/slideLayouts/slideLayout1.xml":            built.parts["ppt/slideLayouts/slideLayout1.xml"],
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": built.parts["ppt/slideLayouts/_rels/slideLayout1.xml.rels"],
		"ppt/theme/theme1.xml":                         built.parts["ppt/theme/theme1.xml"],
		"ppt/media/image1.png":                         built.parts["ppt/media/image1.png"],
	}

	// 第 1 页：保留 slide3.xml（sldIdLst 首位），其余页与其备注页删除
	page1Deleted := map[string]bool{
		"ppt/slides/slide1.xml":                      true,
		"ppt/slides/_rels/slide1.xml.rels":           true,
		"ppt/notesSlides/notesSlide2.xml":            true,
		"ppt/notesSlides/_rels/notesSlide2.xml.rels": true,
		"ppt/slides/slide2.xml":                      true,
		"ppt/slides/_rels/slide2.xml.rels":           true,
	}
	page1Kept := map[string][]byte{
		"ppt/slides/slide3.xml":                      built.parts["ppt/slides/slide3.xml"],
		"ppt/slides/_rels/slide3.xml.rels":           built.parts["ppt/slides/_rels/slide3.xml.rels"],
		"ppt/notesSlides/notesSlide1.xml":            built.parts["ppt/notesSlides/notesSlide1.xml"],
		"ppt/notesSlides/_rels/notesSlide1.xml.rels": built.parts["ppt/notesSlides/_rels/notesSlide1.xml.rels"],
	}
	for k, v := range sharedKept {
		page1Kept[k] = v
	}
	assertSinglePageOutput(t, result.Files[0], "rId2", "ppt/slides/slide3.xml", "第三页内容", page1Deleted, page1Kept, []string{"rId3", "rId4"})

	// 第 2 页：保留 slide1.xml（外部链接所在页），其余页删除
	page2Deleted := map[string]bool{
		"ppt/slides/slide3.xml":                      true,
		"ppt/slides/_rels/slide3.xml.rels":           true,
		"ppt/notesSlides/notesSlide1.xml":            true,
		"ppt/notesSlides/_rels/notesSlide1.xml.rels": true,
		"ppt/slides/slide2.xml":                      true,
		"ppt/slides/_rels/slide2.xml.rels":           true,
	}
	page2Kept := map[string][]byte{
		"ppt/slides/slide1.xml":                      built.parts["ppt/slides/slide1.xml"],
		"ppt/slides/_rels/slide1.xml.rels":           built.parts["ppt/slides/_rels/slide1.xml.rels"],
		"ppt/notesSlides/notesSlide2.xml":            built.parts["ppt/notesSlides/notesSlide2.xml"],
		"ppt/notesSlides/_rels/notesSlide2.xml.rels": built.parts["ppt/notesSlides/_rels/notesSlide2.xml.rels"],
	}
	for k, v := range sharedKept {
		page2Kept[k] = v
	}
	assertSinglePageOutput(t, result.Files[1], "rId3", "ppt/slides/slide1.xml", "第一页内容", page2Deleted, page2Kept, []string{"rId2", "rId4"})

	// 第 3 页：保留 slide2.xml（无备注页形态），其余页与其备注页删除
	page3Deleted := map[string]bool{
		"ppt/slides/slide3.xml":                      true,
		"ppt/slides/_rels/slide3.xml.rels":           true,
		"ppt/notesSlides/notesSlide1.xml":            true,
		"ppt/notesSlides/_rels/notesSlide1.xml.rels": true,
		"ppt/slides/slide1.xml":                      true,
		"ppt/slides/_rels/slide1.xml.rels":           true,
		"ppt/notesSlides/notesSlide2.xml":            true,
		"ppt/notesSlides/_rels/notesSlide2.xml.rels": true,
	}
	page3Kept := map[string][]byte{
		"ppt/slides/slide2.xml":            built.parts["ppt/slides/slide2.xml"],
		"ppt/slides/_rels/slide2.xml.rels": built.parts["ppt/slides/_rels/slide2.xml.rels"],
	}
	for k, v := range sharedKept {
		page3Kept[k] = v
	}
	assertSinglePageOutput(t, result.Files[2], "rId4", "ppt/slides/slide2.xml", "第二页内容", page3Deleted, page3Kept, []string{"rId2", "rId3"})

	assertNoTempResidue(t, wantOutDir)
}

// TestSplitPptxPerPage_SectionsAndCustShowsStripped 含节与自定义放映的演示文稿：
// 输出 presentation.xml 的 custShowLst / extLst（内含 p14:sectionLst）整块剥离，
// 节内 p14:sldId（与真实页条目局部名相同）不被误编辑路径波及。
func TestSplitPptxPerPage_SectionsAndCustShowsStripped(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "带节演示.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "首页内容"},
			{fileName: "slide2.xml", marker: "次页内容"},
		},
		withSections: true,
	})

	result, err := SplitPptxPerPage(context.Background(), built.src)
	if err != nil {
		t.Fatalf("SplitPptxPerPage: %v", err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("应拆出 2 页，实际 %d", len(result.Files))
	}

	zr, err := zip.OpenReader(result.Files[0])
	if err != nil {
		t.Fatalf("打开输出: %v", err)
	}
	defer zr.Close()
	presBytes := mustZipPart(t, &zr.Reader, pptxPresentationPart)
	// p14 前缀的 xmlns 声明留在根元素上属合法保留（无害），只断言节/放映结构剥离
	for _, banned := range []string{"custShowLst", "sectionLst", "extLst"} {
		if bytes.Contains(presBytes, []byte(banned)) {
			t.Errorf("输出 presentation.xml 不应残留 %s", banned)
		}
	}
	if !bytes.Contains(presBytes, []byte("sldSz")) {
		t.Error("presentation.xml 未编辑区域（sldSz）被改动")
	}
}

// TestSplitPptxPerPage_NotesSlideFollowsItsSlide 备注页随所属页联动：删除页的
// notesSlide（part / rels / Content_Types Override）全部移除，保留页备注页逐字节一致。
func TestSplitPptxPerPage_NotesSlideFollowsItsSlide(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "带备注.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "首页内容", notes: true},
			{fileName: "slide2.xml", marker: "次页内容", notes: true},
		},
	})

	result, err := SplitPptxPerPage(context.Background(), built.src)
	if err != nil {
		t.Fatalf("SplitPptxPerPage: %v", err)
	}

	zr, err := zip.OpenReader(result.Files[0])
	if err != nil {
		t.Fatalf("打开输出: %v", err)
	}
	defer zr.Close()
	names := make(map[string]bool)
	for _, f := range zr.Reader.File {
		names[f.Name] = true
	}
	if !names["ppt/notesSlides/notesSlide1.xml"] {
		t.Error("保留页备注页 notesSlide1.xml 应存在")
	}
	for _, gone := range []string{"ppt/notesSlides/notesSlide2.xml", "ppt/notesSlides/_rels/notesSlide2.xml.rels"} {
		if names[gone] {
			t.Errorf("删除页备注页 %s 不应存在", gone)
		}
	}
	ctBytes := mustZipPart(t, &zr.Reader, pptxContentTypesPart)
	if bytes.Contains(ctBytes, []byte("/ppt/notesSlides/notesSlide2.xml")) {
		t.Error("Content_Types 不应残留已删备注页 Override")
	}
	if got, want := mustZipPart(t, &zr.Reader, "ppt/notesSlides/notesSlide1.xml"), built.parts["ppt/notesSlides/notesSlide1.xml"]; !bytes.Equal(got, want) {
		t.Error("保留页备注页内容应逐字节一致")
	}
}

// TestSplitPptxPerPage_CrossPageLinksCleaned 保留页 rels 的关系清理：指向已删页的
// 内部跳转移除，指向保留页（自身）的跳转与外部 URL 超链接保留。
func TestSplitPptxPerPage_CrossPageLinksCleaned(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "跨页链接.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide2.xml", marker: "首页内容", jumpsTo: []string{"slide3.xml", "slide2.xml"}, extLink: true},
			{fileName: "slide3.xml", marker: "次页内容"},
		},
	})

	result, err := SplitPptxPerPage(context.Background(), built.src)
	if err != nil {
		t.Fatalf("SplitPptxPerPage: %v", err)
	}

	zr, err := zip.OpenReader(result.Files[0])
	if err != nil {
		t.Fatalf("打开输出: %v", err)
	}
	defer zr.Close()
	relsBytes := mustZipPart(t, &zr.Reader, "ppt/slides/_rels/slide2.xml.rels")
	if bytes.Contains(relsBytes, []byte(`Target="slide3.xml"`)) {
		t.Error("指向已删页的内部跳转关系应被移除")
	}
	if !bytes.Contains(relsBytes, []byte(`Target="slide2.xml"`)) {
		t.Error("指向保留页（自身）的跳转关系应保留")
	}
	if !bytes.Contains(relsBytes, []byte("https://example.com/")) {
		t.Error("外部 URL 超链接关系应保留")
	}
}

// TestSplitPptxPerPage_SinglePage 单页演示文稿：拆出 1 个文件，无删除面时
// presentation.xml.rels 与 [Content_Types].xml 逐字节不变。
func TestSplitPptxPerPage_SinglePage(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "单页.pptx", pptxFixture{
		slides: []pptxFixtureSlide{{fileName: "slide1.xml", marker: "唯一一页"}},
	})

	result, err := SplitPptxPerPage(context.Background(), built.src)
	if err != nil {
		t.Fatalf("SplitPptxPerPage: %v", err)
	}
	if result.PageCount != 1 || len(result.Files) != 1 {
		t.Fatalf("应拆出 1 页，实际 PageCount=%d len(Files)=%d", result.PageCount, len(result.Files))
	}

	zr, err := zip.OpenReader(result.Files[0])
	if err != nil {
		t.Fatalf("打开输出: %v", err)
	}
	defer zr.Close()
	for _, part := range []string{pptxPresentationRelsPart, pptxContentTypesPart} {
		if got := mustZipPart(t, &zr.Reader, part); !bytes.Equal(got, built.parts[part]) {
			t.Errorf("单页无删除面时 %s 应逐字节不变", part)
		}
	}
}

// TestSplitPptxPerPage_OverwriteExistingOutput 输出文件夹已存在时覆盖重生成：
// 同名单文件被替换为有效 zip。
func TestSplitPptxPerPage_OverwriteExistingOutput(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "覆盖重生成.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "首页内容"},
			{fileName: "slide2.xml", marker: "次页内容"},
		},
	})

	first, err := SplitPptxPerPage(context.Background(), built.src)
	if err != nil {
		t.Fatalf("首次拆分: %v", err)
	}
	// 破坏既有输出，模拟上次运行残留的半截/损坏文件
	if err := os.WriteFile(first.Files[0], []byte("corrupted leftover"), 0644); err != nil {
		t.Fatalf("写入损坏输出: %v", err)
	}

	second, err := SplitPptxPerPage(context.Background(), built.src)
	if err != nil {
		t.Fatalf("重复拆分: %v", err)
	}
	zr, err := zip.OpenReader(second.Files[0])
	if err != nil {
		t.Fatalf("覆盖后输出应为有效 zip: %v", err)
	}
	defer zr.Close()
	var meta pptxPresentationMeta
	if err := xml.Unmarshal(mustZipPart(t, &zr.Reader, pptxPresentationPart), &meta); err != nil {
		t.Fatalf("覆盖后 presentation.xml 解析: %v", err)
	}
	if len(meta.SldIdLst.SldIds) != 1 {
		t.Errorf("覆盖后 sldIdLst 应恰 1 条，实际 %d", len(meta.SldIdLst.SldIds))
	}
}

// TestSplitPptxPerPage_RejectsNonPptx 非 .pptx 输入明确拒绝；.PPTX 大写扩展名接受。
func TestSplitPptxPerPage_RejectsNonPptx(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"旧版.ppt", "无扩展名"} {
		src := filepath.Join(dir, name)
		if err := os.WriteFile(src, []byte("irrelevant"), 0644); err != nil {
			t.Fatalf("写入 %s: %v", name, err)
		}
		_, err := SplitPptxPerPage(context.Background(), src)
		if err == nil {
			t.Fatalf("%s 应被拒绝", name)
		}
		if !strings.Contains(err.Error(), "仅支持 .pptx") {
			t.Errorf("%s 错误提示应含「仅支持 .pptx」，实际：%v", name, err)
		}
	}

	built := buildPptxFixture(t, dir, "大写扩展.PPTX", pptxFixture{
		slides: []pptxFixtureSlide{{fileName: "slide1.xml", marker: "唯一一页"}},
	})
	result, err := SplitPptxPerPage(context.Background(), built.src)
	if err != nil {
		t.Fatalf(".PPTX 大写扩展名应被接受: %v", err)
	}
	if result.PageCount != 1 {
		t.Errorf(".PPTX 应拆出 1 页，实际 %d", result.PageCount)
	}
}

// TestSplitPptxPerPage_CorruptZip 损坏（非 zip）文件报可读错误，且不产生输出目录。
func TestSplitPptxPerPage_CorruptZip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "损坏.pptx")
	if err := os.WriteFile(src, []byte("this is not a zip archive"), 0644); err != nil {
		t.Fatalf("写入损坏文件: %v", err)
	}
	_, err := SplitPptxPerPage(context.Background(), src)
	if err == nil {
		t.Fatal("损坏文件应报错")
	}
	if !strings.Contains(err.Error(), "损坏") {
		t.Errorf("错误提示应引导损坏/加密口径，实际：%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "损坏")); !os.IsNotExist(err) {
		t.Error("失败路径不应创建输出目录")
	}
}

// TestSplitPptxPerPage_MissingPresentationPart 缺 presentation.xml 的 zip 报「不是有效演示文稿」。
func TestSplitPptxPerPage_MissingPresentationPart(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "缺核心部件.pptx")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("docProps/core.xml")
	if err != nil {
		t.Fatalf("构造条目: %v", err)
	}
	if _, err := w.Write([]byte(`<?xml version="1.0"?><cp:coreProperties/>`)); err != nil {
		t.Fatalf("写入条目: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("收口 zip: %v", err)
	}
	if err := os.WriteFile(src, buf.Bytes(), 0644); err != nil {
		t.Fatalf("写入源文件: %v", err)
	}

	_, err = SplitPptxPerPage(context.Background(), src)
	if err == nil {
		t.Fatal("缺少 presentation.xml 应报错")
	}
	if !strings.Contains(err.Error(), "不是有效的 PowerPoint 演示文稿") {
		t.Errorf("错误提示口径不符，实际：%v", err)
	}
}

// TestSplitPptxPerPage_EmptySlideList sldIdLst 为空时明确拒绝。
func TestSplitPptxPerPage_EmptySlideList(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "空演示.pptx", pptxFixture{})
	_, err := SplitPptxPerPage(context.Background(), built.src)
	if err == nil {
		t.Fatal("空演示文稿应报错")
	}
	if !strings.Contains(err.Error(), "不包含任何幻灯片") {
		t.Errorf("错误提示口径不符，实际：%v", err)
	}
}

// TestSplitPptxPerPage_MissingSlideRelationship sldId 引用关系表不存在的 rId 时
// 明确报损坏（三跳映射第二跳断裂）。
func TestSplitPptxPerPage_MissingSlideRelationship(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "孤立引用.pptx")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("构造条目 %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("写入条目 %s: %v", name, err)
		}
	}
	add(pptxContentTypesPart, `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`)
	add("_rels/.rels", `<Relationships xmlns="`+testPackageRelsNS+`"/>`)
	add(pptxPresentationPart, `<?xml version="1.0"?><p:presentation xmlns:r="`+testOfficeRelNS+`" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:sldIdLst><p:sldId id="256" r:id="rId9"/></p:sldIdLst></p:presentation>`)
	add(pptxPresentationRelsPart, `<Relationships xmlns="`+testPackageRelsNS+`"/>`)
	if err := zw.Close(); err != nil {
		t.Fatalf("收口 zip: %v", err)
	}
	if err := os.WriteFile(src, buf.Bytes(), 0644); err != nil {
		t.Fatalf("写入源文件: %v", err)
	}

	_, err := SplitPptxPerPage(context.Background(), src)
	if err == nil {
		t.Fatal("孤立 sldId 引用应报错")
	}
	if !strings.Contains(err.Error(), "关系表缺少") {
		t.Errorf("错误提示口径不符，实际：%v", err)
	}
}

// TestSplitPptxPerPage_CanceledContext 预取消 ctx：立即返回取消错误且保留
// errors.Is(err, context.Canceled) 穿透判定。
func TestSplitPptxPerPage_CanceledContext(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "取消场景.pptx", pptxFixture{
		slides: []pptxFixtureSlide{{fileName: "slide1.xml", marker: "唯一一页"}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := SplitPptxPerPage(ctx, built.src)
	if err == nil {
		t.Fatal("已取消的 ctx 应报错")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("取消错误应保留 context.Canceled 穿透，实际：%v", err)
	}
}

// TestSplitPptxPerPageWithProgress_ProgressSequence 进度回调按页序推进：
// 每页写盘成功后回调一次（done 从 1 计、total 为总页数），全部页完成后返回
// 完整结果；nil 回调形态（SplitPptxPerPage 直通）不触发回调。
func TestSplitPptxPerPageWithProgress_ProgressSequence(t *testing.T) {
	dir := t.TempDir()
	built := buildPptxFixture(t, dir, "进度场景.pptx", pptxFixture{
		slides: []pptxFixtureSlide{
			{fileName: "slide1.xml", marker: "首页内容"},
			{fileName: "slide2.xml", marker: "次页内容"},
			{fileName: "slide3.xml", marker: "三页内容"},
		},
	})

	var calls []fakeProgressCall
	result, err := SplitPptxPerPageWithProgress(context.Background(), built.src, func(done, total int) {
		calls = append(calls, fakeProgressCall{done: done, total: total})
	})
	if err != nil {
		t.Fatalf("SplitPptxPerPageWithProgress: %v", err)
	}
	if len(calls) != 3 {
		t.Fatalf("应回调 3 次，实际 %d 次：%+v", len(calls), calls)
	}
	for i, want := range []fakeProgressCall{{done: 1, total: 3}, {done: 2, total: 3}, {done: 3, total: 3}} {
		if calls[i] != want {
			t.Errorf("第 %d 次回调 = %+v, want %+v", i, calls[i], want)
		}
	}
	if result.PageCount != 3 || len(result.Files) != 3 {
		t.Errorf("进度回调不应影响拆分结果：%+v", result)
	}
}

// ---------- 字节级编辑与路径工具用例 ----------

// TestDropXMLSpans_BytePreservation 字节外科编辑保真：除被删子树外，前缀写法、
// 属性顺序、注释、CDATA、实体、空白全部逐字节保留。
func TestDropXMLSpans_BytePreservation(t *testing.T) {
	data := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><!-- 注释保留 -->
<root xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <keep a="1">文本 &amp; 实体</keep>
  <p:drop id="1" r:id="rId2"/>
  <outer><inner/><keep2/></outer>
  <![CDATA[原始数据]]>
</root>`
	got, err := dropXMLSpans([]byte(data), func(depth int, ancestors []string, name xml.Name, attrs []xml.Attr) bool {
		return name.Local == "drop" || name.Local == "outer"
	})
	if err != nil {
		t.Fatalf("dropXMLSpans: %v", err)
	}
	// 被删元素前后的空白与换行在元素区间之外，均保留
	want := strings.ReplaceAll(data, `<p:drop id="1" r:id="rId2"/>`, "")
	want = strings.ReplaceAll(want, `<outer><inner/><keep2/></outer>`, "")
	if string(got) != want {
		t.Errorf("字节保真破坏：\ngot  %q\nwant %q", got, want)
	}
}

// TestDropXMLSpans_NoMatch 无命中时返回原字节（不产生新分配面）。
func TestDropXMLSpans_NoMatch(t *testing.T) {
	data := []byte(`<root><keep/></root>`)
	got, err := dropXMLSpans(data, func(depth int, ancestors []string, name xml.Name, attrs []xml.Attr) bool {
		return name.Local == "missing"
	})
	if err != nil {
		t.Fatalf("dropXMLSpans: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("无命中应原样返回，实际 %q", got)
	}
}

// TestDropXMLSpans_MalformedXML 非法 XML 返回错误而非半截输出。
func TestDropXMLSpans_MalformedXML(t *testing.T) {
	_, err := dropXMLSpans([]byte(`<root><unclosed>`), func(depth int, ancestors []string, name xml.Name, attrs []xml.Attr) bool {
		return false
	})
	if err == nil {
		t.Fatal("非法 XML 应报错")
	}
}

// TestResolveRelTarget 关系目标解析：包根绝对路径 / 相对源目录 / 上级目录清洗。
func TestResolveRelTarget(t *testing.T) {
	cases := []struct {
		source, target, want string
	}{
		{source: "ppt/presentation.xml", target: "slides/slide1.xml", want: "ppt/slides/slide1.xml"},
		{source: "ppt/presentation.xml", target: "/ppt/slides/slide1.xml", want: "ppt/slides/slide1.xml"},
		{source: "ppt/slides/slide1.xml", target: "../media/image1.png", want: "ppt/media/image1.png"},
		{source: "ppt/slides/slide1.xml", target: "slide2.xml", want: "ppt/slides/slide2.xml"},
		{source: "ppt/slides/slide1.xml", target: "../notesSlides/notesSlide1.xml", want: "ppt/notesSlides/notesSlide1.xml"},
	}
	for _, tc := range cases {
		if got := resolveRelTarget(tc.source, tc.target); got != tc.want {
			t.Errorf("resolveRelTarget(%q, %q) = %q, want %q", tc.source, tc.target, got, tc.want)
		}
	}
}

// TestRelsPartOfAndSourcePart rels 路径与源 part 互推（含包根 .rels 特例）。
func TestRelsPartOfAndSourcePart(t *testing.T) {
	if got := relsPartOf("ppt/slides/slide1.xml"); got != "ppt/slides/_rels/slide1.xml.rels" {
		t.Errorf("relsPartOf = %q", got)
	}
	if got := relsPartOf("ppt/notesSlides/notesSlide1.xml"); got != "ppt/notesSlides/_rels/notesSlide1.xml.rels" {
		t.Errorf("relsPartOf = %q", got)
	}
	if got := relsSourcePart("ppt/_rels/presentation.xml.rels"); got != "ppt/presentation.xml" {
		t.Errorf("relsSourcePart = %q", got)
	}
	if got := relsSourcePart("_rels/.rels"); got != "" {
		t.Errorf("relsSourcePart(包根) = %q, want 空串", got)
	}
}

// ---------- 服务编排用例 ----------

// fakePptxSplitProvider 拆分能力 fake：记录调用参数，返回预置结果/错误；
// 进度回调原样记录，progress 序列非空时在返回前重放（驱动编排层事件断言）。
type fakePptxSplitProvider struct {
	gotCtx      context.Context
	gotSrc      string
	gotProgress []fakeProgressCall
	progress    []fakeProgressCall // 预置：返回结果前依次回调（done/total）
	result      model.PptxSplitResult
	err         error
}

// fakeProgressCall 一次进度回调的实参快照。
type fakeProgressCall struct {
	done  int
	total int
}

func (f *fakePptxSplitProvider) Split(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error) {
	f.gotCtx = ctx
	f.gotSrc = srcPath
	for _, p := range f.progress {
		f.gotProgress = append(f.gotProgress, p)
		if onProgress != nil {
			onProgress(p.done, p.total)
		}
	}
	return f.result, f.err
}

// TestPptxSplitService_DelegatesToProvider 服务编排透传：ctx 与路径原样下发，
// 结果与错误原样上抛；进度回调逐次转译为 pptx-split:progress 事件。
func TestPptxSplitService_DelegatesToProvider(t *testing.T) {
	want := model.PptxSplitResult{OutputDir: "/tmp/演示", Files: []string{"/tmp/演示/演示-01.pptx"}, PageCount: 1}
	fake := &fakePptxSplitProvider{result: want}
	svc := &PptxSplitService{provider: fake}

	ctx := context.Background()
	got, err := svc.Split(ctx, "D:/docs/演示.pptx")
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if got.OutputDir != want.OutputDir || got.PageCount != want.PageCount || len(got.Files) != 1 {
		t.Errorf("结果透传不符：%+v", got)
	}
	if fake.gotCtx == nil || fake.gotCtx == ctx {
		t.Error("应向 provider 下发派生的可取消 ctx（非调用方 ctx 原样）")
	}
	if fake.gotSrc != "D:/docs/演示.pptx" {
		t.Errorf("srcPath 应原样下发，实际 %q", fake.gotSrc)
	}

	// 错误上抛 + 零值结果
	fake.err = errors.New("boom")
	got, err = svc.Split(ctx, "D:/docs/演示.pptx")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("provider 错误应上抛，实际：%v", err)
	}
	if got.OutputDir != "" || got.Files != nil || got.PageCount != 0 {
		t.Errorf("错误时应返回零值结果，实际：%+v", got)
	}
}

// TestPptxSplitService_DefaultProviderIsCore 生产构造默认接纯 Go 核心：
// 非 .pptx 输入走到核心校验（报「仅支持 .pptx」）即证明链路接通。
func TestPptxSplitService_DefaultProviderIsCore(t *testing.T) {
	svc := NewPptxSplitService(context.Background())
	_, err := svc.Split(context.Background(), filepath.Join(t.TempDir(), "旧版.ppt"))
	if err == nil || !strings.Contains(err.Error(), "仅支持 .pptx") {
		t.Fatalf("默认 provider 应为真实核心（扩展名校验生效），实际：%v", err)
	}
}
