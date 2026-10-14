package service

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- 解析 fixture 程序化构造 ----------
//
// 与拆分 fixture（pptx_split_test.go）同源的 OPC 最小包构造，但 slide 含真实
// 形状结构（标题占位符/正文形状/字段页码）与备注页文本，驱动解析器全链路。
// 禁依赖本机 Office（docs/spec/pptx-split.md）。

// assetsSlideSpec 解析 fixture 中一页的定义。
type assetsSlideSpec struct {
	titlePh   string   // 标题占位符文本（"" = 无标题占位符形状）
	body      []string // 正文形状段落（每项一个 a:p；nil = 无正文形状）
	slideNum  bool     // 是否含页码字段形状（a:fld，提取时必须排除）
	notes     string   // 备注页文本（"" = 无备注页）
	notesFld  string   // 备注页混入的页码字段文本（验证 fld 排除；仅 notes != "" 时生效）
	shortBody bool     // 首段为 ≤3 字短文本（验证回退链跳过短行）
}

// assetsSlideXML 构造单页 slide XML：标题占位符形状 + 正文形状 + 字段形状。
func assetsSlideXML(spec assetsSlideSpec) string {
	var b strings.Builder
	b.WriteString(pptxFixtureXMLDecl)
	b.WriteString(`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree>`)
	if spec.titlePh != "" {
		b.WriteString(`<p:sp><p:nvSpPr><p:cNvPr id="2" name="标题 1"/><p:nvPr><p:ph type="ctrTitle"/></p:nvPr></p:nvSpPr>` +
			`<p:txBody><a:bodyPr/><a:p><a:r><a:rPr lang="zh-CN"/><a:t>` + spec.titlePh + `</a:t></a:r></a:p></p:txBody></p:sp>`)
	}
	if len(spec.body) > 0 {
		b.WriteString(`<p:sp><p:nvSpPr><p:cNvPr id="3" name="内容占位符 2"/><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr><p:txBody><a:bodyPr/>`)
		for i, line := range spec.body {
			if spec.shortBody && i == 0 {
				fmt.Fprintf(&b, `<a:p><a:r><a:t>%s</a:t></a:r></a:p>`, line)
				continue
			}
			// 段内拆两个 run：验证段内 run 直接拼接（无分隔符）
			r := []rune(line)
			mid := len(r) / 2
			fmt.Fprintf(&b, `<a:p><a:r><a:t>%s</a:t></a:r><a:r><a:t>%s</a:t></a:r></a:p>`, string(r[:mid]), string(r[mid:]))
		}
		b.WriteString(`</p:txBody></p:sp>`)
	}
	if spec.slideNum {
		b.WriteString(`<p:sp><p:nvSpPr><p:cNvPr id="4" name="页码占位符 3"/><p:nvPr><p:ph type="sldNum"/></p:nvPr></p:nvSpPr>` +
			`<p:txBody><a:bodyPr/><a:p><a:fld id="{GUID}" type="slidenum"><a:r><a:rPr lang="zh-CN"/><a:t>3</a:t></a:r></a:fld></a:p></p:txBody></p:sp>`)
	}
	b.WriteString(`</p:spTree></p:cSld></p:sld>`)
	return b.String()
}

// assetsNotesSlideXML 构造备注页 XML：正文占位符 + 可选页码字段（验证 fld 排除）。
func assetsNotesSlideXML(notes, fldText string) string {
	var b strings.Builder
	b.WriteString(pptxFixtureXMLDecl)
	b.WriteString(`<p:notes xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree>`)
	b.WriteString(`<p:sp><p:nvSpPr><p:cNvPr id="2" name="备注占位符 1"/><p:nvPr><p:ph type="body"/></p:nvPr></p:nvSpPr>` +
		`<p:txBody><a:bodyPr/><a:p><a:r><a:t>` + notes + `</a:t></a:r></a:p></p:txBody></p:sp>`)
	if fldText != "" {
		b.WriteString(`<p:sp><p:nvSpPr><p:cNvPr id="3" name="幻灯片编号占位符 2"/><p:nvPr><p:ph type="sldNum"/></p:nvPr></p:nvSpPr>` +
			`<p:txBody><a:bodyPr/><a:p><a:fld id="{GUID}" type="slidenum"><a:r><a:t>` + fldText + `</a:t></a:r></a:fld></a:p></p:txBody></p:sp>`)
	}
	b.WriteString(`</p:spTree></p:cSld></p:notes>`)
	return b.String()
}

// buildAssetsPptx 程序化构造含富文本形状的 pptx 并写入 dir/name，返回源文件路径。
// 页序即 specs 顺序（sldIdLst 顺位）；slide 文件名与页序刻意错开（第 i 页对应
// slideN.xml，N = 总页数-i），验证解析按三跳映射而非文件名序。
func buildAssetsPptx(t *testing.T, dir, name string, specs []assetsSlideSpec) string {
	t.Helper()
	slideFiles := make([]string, len(specs))
	for i := range specs {
		slideFiles[i] = fmt.Sprintf("slide%d.xml", len(specs)-i)
	}

	parts := make(map[string]string)
	order := make([]string, 0)
	add := func(part, content string) {
		parts[part] = content
		order = append(order, part)
	}

	notesFiles := make([]string, 0)
	for i, spec := range specs {
		if spec.notes != "" {
			notesFiles = append(notesFiles, fmt.Sprintf("notesSlide%d.xml", i+1))
		}
	}
	add(pptxContentTypesPart, pptxFixtureContentTypes(slideFiles, notesFiles, defaultPptxMasters(), false))
	add("_rels/.rels", pptxFixtureRootRels)

	// presentation.xml：sldIdLst 页条目（页序权威来源）
	var presB strings.Builder
	presB.WriteString(pptxFixtureXMLDecl)
	presB.WriteString(`<p:presentation xmlns:r="` + testOfficeRelNS + `" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">`)
	presB.WriteString(`<p:sldIdLst>`)
	for i := range specs {
		fmt.Fprintf(&presB, `<p:sldId id="%d" r:id="rId%d"/>`, 256+i, i+2)
	}
	presB.WriteString(`</p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`)
	add(pptxPresentationPart, presB.String())

	// presentation.xml.rels：rId1 母版，rId2.. 按展示页序对应各 slide 文件
	var relsB strings.Builder
	relsB.WriteString(pptxFixtureXMLDecl)
	relsB.WriteString(`<Relationships xmlns="` + testPackageRelsNS + `">`)
	relsB.WriteString(`<Relationship Id="rId1" Type="` + testOfficeRelNS + `/slideMaster" Target="slideMasters/slideMaster1.xml"/>`)
	for i, f := range slideFiles {
		fmt.Fprintf(&relsB, `<Relationship Id="rId%d" Type="%s/slide" Target="slides/%s"/>`, i+2, testOfficeRelNS, f)
	}
	relsB.WriteString(`</Relationships>`)
	add(pptxPresentationRelsPart, relsB.String())

	add("ppt/slideMasters/slideMaster1.xml", pptxFixtureMasterXML(defaultPptxMasters()[0].layouts))
	add("ppt/slideMasters/_rels/slideMaster1.xml.rels", pptxFixtureMasterRels(defaultPptxMasters()[0]))
	add("ppt/slideLayouts/slideLayout1.xml", pptxFixtureLayoutXML)
	add("ppt/slideLayouts/_rels/slideLayout1.xml.rels", pptxFixtureLayoutRels("slideMaster1.xml", ""))
	add("ppt/theme/theme1.xml", pptxFixtureThemeXML)

	for i, spec := range specs {
		slidePart := "ppt/slides/" + slideFiles[i]
		add(slidePart, assetsSlideXML(spec))
		// slide rels：版式必带 + 备注页关系（有备注时，Target 相对 slide 所在目录）
		var srB strings.Builder
		srB.WriteString(pptxFixtureXMLDecl)
		srB.WriteString(`<Relationships xmlns="` + testPackageRelsNS + `">`)
		fmt.Fprintf(&srB, `<Relationship Id="rId1" Type="%s/slideLayout" Target="../slideLayouts/slideLayout1.xml"/>`, testOfficeRelNS)
		if spec.notes != "" {
			fmt.Fprintf(&srB, `<Relationship Id="rId2" Type="%s/notesSlide" Target="../notesSlides/notesSlide%d.xml"/>`, testOfficeRelNS, i+1)
		}
		srB.WriteString(`</Relationships>`)
		add("ppt/slides/_rels/"+slideFiles[i]+".rels", srB.String())

		if spec.notes != "" {
			add(fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", i+1), assetsNotesSlideXML(spec.notes, spec.notesFld))
		}
	}

	// zip 组装
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, part := range order {
		w, err := zw.Create(part)
		if err != nil {
			t.Fatalf("fixture 构造条目 %s: %v", part, err)
		}
		if _, err := w.Write([]byte(parts[part])); err != nil {
			t.Fatalf("fixture 写入条目 %s: %v", part, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("fixture 收口 zip: %v", err)
	}
	src := filepath.Join(dir, name)
	if err := os.WriteFile(src, buf.Bytes(), 0644); err != nil {
		t.Fatalf("fixture 写入源文件: %v", err)
	}
	return src
}

// buildOLEEncryptedPptx 构造加密 pptx 样本：OLE2 复合文档魔数开头 + 区分度字节。
func buildOLEEncryptedPptx(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	data := append([]byte{}, oleCompoundFileMagic...)
	data = append(data, 0x00, 0x01, 0x02, 0x03, 0xFE, 0xFF) // EncryptedPackage 流模拟载荷
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatalf("写入加密样本: %v", err)
	}
	return p
}

// ---------- 解析核心用例 ----------

// TestParsePptxSlides_Extraction 综合解析：标题占位符、段内 run 拼接、多段落、
// 页码字段排除、备注页文本（含 fld 排除）、页序三跳映射（slide 文件名乱序）。
func TestParsePptxSlides_Extraction(t *testing.T) {
	dir := t.TempDir()
	src := buildAssetsPptx(t, dir, "季度汇报.pptx", []assetsSlideSpec{
		{
			titlePh:  "城商行获客策略",
			body:     []string{"全面服务城商行客户", "数字化转型实践"},
			slideNum: true,
			notes:    "本页讲解获客整体思路",
			notesFld: "1",
		},
		{
			// 无标题占位符：首行 ≤3 字跳过，第二行（>3 字）作标题
			titlePh:   "",
			body:      []string{"待客", "代发工资业务介绍"},
			shortBody: true,
		},
		{
			// 全空页：标题回退「文件名-页码」
			titlePh: "",
			body:    nil,
		},
	})

	slides, err := parsePptxSlides(src, "季度汇报")
	if err != nil {
		t.Fatalf("parsePptxSlides: %v", err)
	}
	if len(slides) != 3 {
		t.Fatalf("应解析出 3 页，实际 %d", len(slides))
	}

	// 第 1 页：标题占位符生效；正文两段（段落换行、段内 run 拼接）；
	// 页码字段「3」排除；备注文本保留且页码字段「1」排除
	s1 := slides[0]
	if s1.Num != 1 || s1.Title != "城商行获客策略" {
		t.Errorf("第 1 页标题 = %q (num=%d), want 城商行获客策略", s1.Title, s1.Num)
	}
	wantText := "全面服务城商行客户\n数字化转型实践"
	if s1.Text != wantText {
		t.Errorf("第 1 页正文 = %q, want %q", s1.Text, wantText)
	}
	if strings.Contains(s1.Text, "3") {
		t.Errorf("第 1 页正文不应包含页码字段文本，实际 %q", s1.Text)
	}
	if s1.Notes != "本页讲解获客整体思路" {
		t.Errorf("第 1 页备注 = %q, want 备注文本且不含页码字段", s1.Notes)
	}

	// 第 2 页：无标题占位符 → 首行「待客」（≤3 字）跳过 → 第二行作标题；
	// 页序按 sldIdLst（slide 文件名乱序不影响）
	s2 := slides[1]
	if s2.Num != 2 {
		t.Errorf("第 2 页页序 = %d, want 2（sldIdLst 顺序）", s2.Num)
	}
	if s2.Title != "代发工资业务介绍" {
		t.Errorf("第 2 页标题应回退首个 >3 字行，实际 %q", s2.Title)
	}
	if s2.Notes != "" {
		t.Errorf("第 2 页应无备注，实际 %q", s2.Notes)
	}

	// 第 3 页：全空页 → 标题回退「文件名-页码」
	s3 := slides[2]
	if s3.Title != "季度汇报-03" {
		t.Errorf("第 3 页标题应回退文件名-页码，实际 %q", s3.Title)
	}
	if s3.Text != "" || s3.Notes != "" {
		t.Errorf("第 3 页应无正文与备注，实际 text=%q notes=%q", s3.Text, s3.Notes)
	}
}

// TestParsePptxSlides_TitleFallbackChain 标题回退链：占位符优先于正文行；
// 正文纯标点行跳过后取首个有效行。
func TestParsePptxSlides_TitleFallbackChain(t *testing.T) {
	dir := t.TempDir()
	src := buildAssetsPptx(t, dir, "回退链.pptx", []assetsSlideSpec{
		{titlePh: "占位符标题优先", body: []string{"正文长行会输给占位符"}},
		{titlePh: "", body: []string{"，，，", "纯标点行跳过后取这行有效标题"}},
	})
	slides, err := parsePptxSlides(src, "回退链")
	if err != nil {
		t.Fatalf("parsePptxSlides: %v", err)
	}
	if slides[0].Title != "占位符标题优先" {
		t.Errorf("占位符应优先于正文行，实际 %q", slides[0].Title)
	}
	if slides[1].Title != "纯标点行跳过后取这行有效标题" {
		t.Errorf("纯标点正文行应跳过，实际 %q", slides[1].Title)
	}
}

// TestParsePptxSlides_ShortLineSkipped 全部正文行 ≤3 字时回退文件名-页码。
func TestParsePptxSlides_ShortLineSkipped(t *testing.T) {
	dir := t.TempDir()
	src := buildAssetsPptx(t, dir, "短行.pptx", []assetsSlideSpec{
		{titlePh: "", body: []string{"ab", "等待"}, shortBody: true},
	})
	slides, err := parsePptxSlides(src, "短行")
	if err != nil {
		t.Fatalf("parsePptxSlides: %v", err)
	}
	if slides[0].Title != "短行-01" {
		t.Errorf("全短行标题应回退文件名-页码，实际 %q", slides[0].Title)
	}
}

// TestParsePptxSlides_Encrypted OLE 魔数样本识别为加密分类。
func TestParsePptxSlides_Encrypted(t *testing.T) {
	dir := t.TempDir()
	src := buildOLEEncryptedPptx(t, dir, "加密.pptx")
	_, err := parsePptxSlides(src, "加密")
	var pe *assetParseError
	if !errors.As(err, &pe) {
		t.Fatalf("应返回 assetParseError，实际：%v", err)
	}
	if pe.kind != assetParseEncrypted {
		t.Errorf("OLE 魔数应分类为加密，实际 %q（%v）", pe.kind, err)
	}
}

// TestParsePptxSlides_CorruptedZip 非 zip 内容与缺核心部件分别归类损坏。
func TestParsePptxSlides_CorruptedZip(t *testing.T) {
	dir := t.TempDir()
	// 非 zip 内容
	src := filepath.Join(dir, "损坏.pptx")
	if err := os.WriteFile(src, []byte("this is not a zip archive"), 0644); err != nil {
		t.Fatalf("写入损坏样本: %v", err)
	}
	_, err := parsePptxSlides(src, "损坏")
	var pe *assetParseError
	if !errors.As(err, &pe) || pe.kind != assetParseCorrupted {
		t.Fatalf("非 zip 内容应分类为损坏，实际：%v", err)
	}

	// zip 但缺 presentation.xml（复用拆分用例的构造方式）
	src2 := filepath.Join(dir, "缺部件.pptx")
	parts := map[string]string{"docProps/core.xml": `<?xml version="1.0"?><cp:coreProperties/>`}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("docProps/core.xml")
	if err != nil {
		t.Fatalf("构造条目: %v", err)
	}
	if _, err := w.Write([]byte(parts["docProps/core.xml"])); err != nil {
		t.Fatalf("写入条目: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("收口 zip: %v", err)
	}
	if err := os.WriteFile(src2, buf.Bytes(), 0644); err != nil {
		t.Fatalf("写入缺部件样本: %v", err)
	}
	_, err = parsePptxSlides(src2, "缺部件")
	if !errors.As(err, &pe) || pe.kind != assetParseCorrupted {
		t.Fatalf("缺核心部件应分类为损坏，实际：%v", err)
	}
}

// TestOpenErrorToParseError 打开错误分类：权限 → denied，其余 → corrupted；
// 根因链经 Unwrap 保留 errors.Is 穿透判定。
func TestOpenErrorToParseError(t *testing.T) {
	denied := openErrorToParseError(fmt.Errorf("open: %w", os.ErrPermission))
	if denied.kind != assetParseDenied {
		t.Errorf("os.ErrPermission 应分类 denied，实际 %q", denied.kind)
	}
	if !errors.Is(denied, os.ErrPermission) {
		t.Error("denied 错误应保留根因穿透（errors.Is 经 Unwrap）")
	}
	other := openErrorToParseError(errors.New("no such file"))
	if other.kind != assetParseCorrupted {
		t.Errorf("其他打开错误应分类 corrupted，实际 %q", other.kind)
	}
}

// TestIsMeaningfulTitleLine 首行短文本有效性边界（>3 字 + 非纯标点）。
func TestIsMeaningfulTitleLine(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"城商行获客策略", true},
		{"abcd", true},   // 4 字符 ASCII（>3）
		{"待客之道", true},   // 恰 4 字
		{"待客", false},    // ≤3 字
		{"abc", false},   // 恰 3 字符（不满足 >3）
		{"  ", false},    // 纯空白
		{"。。。。", false},  // 纯标点（4 字符但无文字/数字）
		{"1.1 概述", true}, // 数字开头
	}
	for _, tc := range cases {
		if got := isMeaningfulTitleLine(tc.line); got != tc.want {
			t.Errorf("isMeaningfulTitleLine(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}

// TestAssetTagColorFor 默认标签色确定性分配（同名跨库同色）。
func TestAssetTagColorFor(t *testing.T) {
	c1 := assetTagColorFor("重点客户")
	c2 := assetTagColorFor("重点客户")
	if c1 != c2 {
		t.Errorf("同名标签默认色应稳定：%q vs %q", c1, c2)
	}
	found := false
	for _, c := range assetTagPalette {
		if c == c1 {
			found = true
		}
	}
	if !found {
		t.Errorf("默认色应出自色板，实际 %q", c1)
	}
}

// ---------- zip 解压上限（防 zip 炸弹 OOM）----------

// buildHighlyCompressibleZip 程序化构造高压缩比 zip（单条目解压后 expanded 字节，
// 压缩后极小），写入 dir/name 返回源路径——zip 炸弹形态的最小模拟。
func buildHighlyCompressibleZip(t *testing.T, dir, name string, entries map[string]int) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for entry, size := range entries {
		w, err := zw.Create(entry)
		if err != nil {
			t.Fatalf("构造条目 %s: %v", entry, err)
		}
		if _, err := w.Write(bytes.Repeat([]byte{0x00}, size)); err != nil {
			t.Fatalf("写入条目 %s: %v", entry, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("收口 zip: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buf.Bytes(), 0644); err != nil {
		t.Fatalf("写入样本: %v", err)
	}
	return p
}

// TestReadZipPackageWithLimits 解压字节上限：单条目超限与全包累计超限均报错且
// 不再解压剩余条目；上限内正常读取；不设限（readZipPackage 透传）行为不变——
// 拆分链路共用函数语义逐字不变的守护。
func TestReadZipPackageWithLimits(t *testing.T) {
	dir := t.TempDir()

	// 单条目超限：1MB 上限 vs 2MB 解压体积
	single := buildHighlyCompressibleZip(t, dir, "single.zip", map[string]int{"big.bin": 2 << 20})
	zr, err := zip.OpenReader(single)
	if err != nil {
		t.Fatalf("打开样本: %v", err)
	}
	defer zr.Close()
	if _, err := readZipPackageWithLimits(&zr.Reader, 1<<20, 1<<30); err == nil {
		t.Error("单条目解压超上限应报错")
	} else if !strings.Contains(err.Error(), "单条目上限") {
		t.Errorf("单条目超限错误应含「单条目上限」，实际：%v", err)
	}
	// 恰好等于上限：不报错（多读 1 字节探测不误伤边界）
	if _, err := readZipPackageWithLimits(&zr.Reader, 2<<20, 1<<30); err != nil {
		t.Errorf("恰好等于上限不应报错: %v", err)
	}

	// 全包累计超限：两个 600KB 条目 vs 1MB 上限
	multi := buildHighlyCompressibleZip(t, dir, "multi.zip", map[string]int{
		"a.bin": 600 << 10,
		"b.bin": 600 << 10,
	})
	zr2, err := zip.OpenReader(multi)
	if err != nil {
		t.Fatalf("打开样本: %v", err)
	}
	defer zr2.Close()
	if _, err := readZipPackageWithLimits(&zr2.Reader, 1<<20, 1<<20); err == nil {
		t.Error("全包解压累计超上限应报错")
	} else if !strings.Contains(err.Error(), "累计超过上限") {
		t.Errorf("累计超限错误应含「累计超过上限」，实际：%v", err)
	}

	// 不设限透传：readZipPackage 对超限样本正常读取（拆分链路行为不变）
	if _, err := zip.OpenReader(single); err != nil {
		t.Fatalf("重开样本: %v", err)
	} else {
		pkg, err := readZipPackage(&zr.Reader)
		if err != nil {
			t.Errorf("不设限应正常解压: %v", err)
		} else if len(pkg.entries) != 1 || len(pkg.entries[0].data) != 2<<20 {
			t.Errorf("不设限解压结果不符：%d 条目", len(pkg.entries))
		}
	}
}

// TestParsePptxSlides_ZipLimitCorrupted 超限样本经解析链路按「文件损坏」分类
// 降级（不中断批量的既定语义），错误链保留上限根因。
func TestParsePptxSlides_ZipLimitCorrupted(t *testing.T) {
	dir := t.TempDir()
	src := buildHighlyCompressibleZip(t, dir, "炸弹.pptx", map[string]int{"huge.bin": 300 << 20})
	_, err := parsePptxSlides(src, "炸弹")
	var pe *assetParseError
	if !errors.As(err, &pe) {
		t.Fatalf("应返回带分类的解析错误，实际：%v", err)
	}
	if pe.kind != assetParseCorrupted {
		t.Errorf("超限应按文件损坏分类，实际 kind=%q", pe.kind)
	}
}
