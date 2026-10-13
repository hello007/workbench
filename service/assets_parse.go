package service

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

// PPT 解析层（素材管理·单页索引）：基于既有拆分核心的 OOXML 读取链路
// （readZipPackage / resolveSlidePages / resolveNotesSlides，见 pptx_split.go）
// 提取每页文本框文本、页标题与演讲者备注。零 COM、零第三方依赖。
//
// 页序以 sldIdLst 三跳映射为准（slide 文件名 ≠ 页序）；备注页经 slide rels 的
// /notesSlide 关系定位，OPC 关系 Target 相对关系源 part 所在目录解析（该坑已由
// 拆分核心的 resolveRelTarget 统一处理）。

// 解析失败分类（*assetParseError.kind），扫描器据此映射文件状态：
// corrupted/其他 → failed，encrypted → encrypted（跳过），denied → denied（跳过）。
const (
	assetParseEncrypted = "encrypted"
	assetParseCorrupted = "corrupted"
	assetParseDenied    = "denied"
)

// assetParseTitleFallback 无任何文本来源时的标题兜底文案。
const assetParseTitleFallback = "[无标题]"

// 素材解析的 zip 解压字节上限（扫描批量解析外来文件，防 zip 炸弹类畸形包无上限
// 解压拖垮进程内存）：真实演示文稿解压量级为数十 MB（媒体为主），单条目 256MB /
// 全包 1GB 给足裕量，仅拦截异常形态；超限按「文件损坏」分类降级（不中断批量）。
// 拆分链路（pptx_split.go readZipPackage）不设限，行为逐字不变。
const (
	assetZipMaxEntryBytes = 256 << 20 // 单条目解压上限 256MB
	assetZipMaxTotalBytes = 1 << 30   // 全包解压累计上限 1GB
)

// oleCompoundFileMagic OLE2 复合文档魔数：PowerPoint 密码加密后的文件是 OLE
// 容器（内含 EncryptedPackage 流）而非 zip 包，读头 8 字节即可与普通损坏区分。
var oleCompoundFileMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

// assetParseError 带分类的解析错误。kind 供扫描器状态机分流，err 保留根因链
// （errors.Is/As 穿透），Error() 文案用户可读（日志口径）。
type assetParseError struct {
	kind string
	err  error
}

func (e *assetParseError) Error() string { return e.err.Error() }
func (e *assetParseError) Unwrap() error { return e.err }

// parsedSlide 单页解析产物（Num 为展示页序，从 1 计）。
type parsedSlide struct {
	Num   int
	Title string
	Text  string
	Notes string
}

// assetsParseFn 扫描器的解析注入点：生产实现为 parsePptxSlides，单测注入 fake
// 驱动状态机分流（加密/权限形态在 Windows 上无法用真实文件稳定构造，经注入
// 覆盖，见 docs/spec/test-stability.md 的环境外部状态约束）。
type assetsParseFn = func(srcPath, baseName string) ([]parsedSlide, error)

// parsePptxSlides 解析 .pptx：返回按展示页序排列的单页产物。
// baseName 为源文件名（去扩展名），供标题回退链第三级「文件名+页码」使用。
func parsePptxSlides(srcPath, baseName string) ([]parsedSlide, error) {
	if err := classifyEncryptedOrUnreadable(srcPath); err != nil {
		return nil, err
	}
	zr, err := zip.OpenReader(srcPath)
	if err != nil {
		return nil, &assetParseError{kind: assetParseCorrupted,
			err: fmt.Errorf("文件已损坏（不是有效的演示文稿包）: %w", err)}
	}
	defer zr.Close()

	pkg, err := readZipPackageWithLimits(&zr.Reader, assetZipMaxEntryBytes, assetZipMaxTotalBytes)
	if err != nil {
		return nil, &assetParseError{kind: assetParseCorrupted, err: err}
	}
	pages, err := resolveSlidePages(pkg)
	if err != nil {
		return nil, &assetParseError{kind: assetParseCorrupted, err: err}
	}
	notesOf, err := resolveNotesSlides(pkg, pages)
	if err != nil {
		return nil, &assetParseError{kind: assetParseCorrupted, err: err}
	}

	slides := make([]parsedSlide, 0, len(pages))
	for i, page := range pages {
		entry, ok := pkg.byName[page.part]
		if !ok {
			return nil, &assetParseError{kind: assetParseCorrupted,
				err: fmt.Errorf("页 part %s 缺失，文件可能已损坏", page.part)}
		}
		content, err := parseSlideContent(entry.data)
		if err != nil {
			return nil, &assetParseError{kind: assetParseCorrupted, err: err}
		}
		notes := ""
		if notesPart, ok := notesOf[page.part]; ok {
			if notesEntry, ok := pkg.byName[notesPart]; ok {
				notes = notesEntry.textContent()
			}
		}
		slides = append(slides, parsedSlide{
			Num:   i + 1,
			Title: resolveSlideTitle(content, baseName, i+1),
			Text:  strings.Join(content.bodyLines, "\n"),
			Notes: notes,
		})
	}
	return slides, nil
}

// classifyEncryptedOrUnreadable 打包文件读取前置分类：OLE 魔数 → 加密；权限
// 不足 → denied；其余读取失败 → corrupted。文件可读且非 OLE 时返回 nil。
func classifyEncryptedOrUnreadable(srcPath string) error {
	if err := checkOLEEncrypted(srcPath); err != nil {
		return err
	}
	// zip.OpenReader 内含 os.Open：权限不足在此暴露。预检仅读头 8 字节，
	// 权限判定统一收口到本函数，避免两处 os.Open 错误分流漂移。
	f, err := os.Open(srcPath)
	if err != nil {
		return openErrorToParseError(err)
	}
	return f.Close()
}

// checkOLEEncrypted 读文件头 8 字节比对 OLE 复合文档魔数；命中返回加密分类错误。
func checkOLEEncrypted(srcPath string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return openErrorToParseError(err)
	}
	defer f.Close()
	head := make([]byte, len(oleCompoundFileMagic))
	if _, err := io.ReadFull(f, head); err != nil {
		// 不足 8 字节的残缺文件：非 OLE（zip 校验留给后续环节给可读错误）
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return nil
		}
		return openErrorToParseError(err)
	}
	if bytes.Equal(head, oleCompoundFileMagic) {
		return &assetParseError{kind: assetParseEncrypted,
			err: errors.New("文件已加密（PowerPoint 密码保护），暂不支持索引")}
	}
	return nil
}

// openErrorToParseError 文件打开错误分类：权限不足 → denied（跳过），其余 →
// corrupted（含文件消失等，扫描日志可查）。
func openErrorToParseError(err error) *assetParseError {
	if errors.Is(err, os.ErrPermission) {
		return &assetParseError{kind: assetParseDenied,
			err: fmt.Errorf("文件无读取权限: %w", err)}
	}
	return &assetParseError{kind: assetParseCorrupted,
		err: fmt.Errorf("无法读取文件: %w", err)}
}

// slideContent 单页文本内容的中间结构：标题占位符文本 + 非标题文本行。
type slideContent struct {
	titleText string   // 标题占位符（type=title/ctrTitle）形状的文本，可空
	bodyLines []string // 非标题形状的段落文本行（形状内段落以行为单位）
}

// DrawingML 与 PresentationML 命名空间 URI（属性/元素匹配按解码后的完整 URI，
// 不依赖生产方前缀写法，与拆分核心的命名空间口径一致）。
const (
	drawingmlNS     = "http://schemas.openxmlformats.org/drawingml/2006/main"
	presentationNS  = "http://schemas.openxmlformats.org/presentationml/2006/main"
	placeholderName = "ph"
)

// parseSlideContent 流式解析单页 slide XML：收集各形状（p:sp）的文本段落，
// 按标题占位符分流。流式而非全量 Unmarshal：slide 结构开放（任意扩展元素），
// 全量结构映射脆弱且内存翻倍。
//
// 提取口径：
//   - 文本 = a:t 文本运行，段内多个 run 直接拼接，段落（a:p）以行为单位；
//   - a:fld 字段文本（页码/日期占位）排除——字段是渲染期占位符而非内容；
//   - 标题占位符形状（p:ph type=title/ctrTitle）文本单列入 titleText，不进正文；
//   - 形状外的散置文本（表格 a:tc 等 graphicFrame 内容）并入正文行。
func parseSlideContent(data []byte) (*slideContent, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	content := &slideContent{}

	var (
		depth        int  // 当前元素嵌套深度（根元素为 1）
		shapeDepth   = -1 // 当前 p:sp 形状元素深度；-1 = 不在形状内
		shapeIsTitle bool // 当前形状是否标题占位符
		titleLines   []string
		paraDepth    = -1 // 当前 a:p 段落深度；-1 = 不在段落内
		paraText     strings.Builder
		textDepth    = -1 // 当前 a:t 文本运行深度；-1 = 不在文本运行内
		fldDepth     = -1 // 当前 a:fld 字段深度（其内 a:t 一并排除）
	)

	// flushParagraph 段落闭合：标题占位符形状的段落进 titleLines（形状闭合时
	// 合并为 titleText），其余段落进正文行。空段落（纯空白）不入列。
	flushParagraph := func() {
		if line := paraText.String(); strings.TrimSpace(line) != "" {
			if shapeDepth >= 0 && shapeIsTitle {
				titleLines = append(titleLines, line)
			} else {
				content.bodyLines = append(content.bodyLines, line)
			}
		}
		paraText.Reset()
	}

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("解析幻灯片 XML 失败: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch {
			case t.Name.Space == presentationNS && t.Name.Local == "sp":
				shapeDepth = depth
				shapeIsTitle = false
			case shapeDepth >= 0 && t.Name.Space == presentationNS && t.Name.Local == placeholderName:
				// 占位符类型声明（p:ph）出现在形状属性区（txBody 之前），
				// 先于段落文本到达；type 缺省为正文/对象占位符
				if typ := attrValue(t.Attr, "", "type"); typ == "title" || typ == "ctrTitle" {
					shapeIsTitle = true
				}
			case fldDepth < 0 && t.Name.Space == drawingmlNS && t.Name.Local == "fld":
				fldDepth = depth
			case fldDepth < 0 && t.Name.Space == drawingmlNS && t.Name.Local == "p":
				paraDepth = depth
				paraText.Reset()
			case fldDepth < 0 && t.Name.Space == drawingmlNS && t.Name.Local == "t":
				textDepth = depth
			}
		case xml.CharData:
			if textDepth >= 0 {
				paraText.Write(t)
			}
		case xml.EndElement:
			switch {
			case fldDepth == depth && t.Name.Space == drawingmlNS && t.Name.Local == "fld":
				fldDepth = -1
			case textDepth == depth && t.Name.Space == drawingmlNS && t.Name.Local == "t":
				textDepth = -1
			case paraDepth == depth && t.Name.Space == drawingmlNS && t.Name.Local == "p":
				flushParagraph()
				paraDepth = -1
			case shapeDepth == depth && t.Name.Space == presentationNS && t.Name.Local == "sp":
				if shapeIsTitle && content.titleText == "" {
					content.titleText = strings.Join(titleLines, "\n")
				}
				titleLines = nil
				shapeDepth = -1
				shapeIsTitle = false
			}
			depth--
		}
	}
	// 收尾：形状外的散置段落（表格单元格等）已随段落闭合进入 bodyLines
	return content, nil
}

// resolveSlideTitle 页标题回退链（PRD）：标题占位符文本 → 首个有效短文本行
// （>3 可见字符且非纯标点）→「文件名-页码」；全部落空兜底「[无标题]」。
func resolveSlideTitle(content *slideContent, baseName string, num int) string {
	if title := strings.TrimSpace(content.titleText); title != "" {
		return title
	}
	for _, line := range content.bodyLines {
		if isMeaningfulTitleLine(line) {
			return strings.TrimSpace(line)
		}
	}
	if baseName != "" {
		return fmt.Sprintf("%s-%02d", baseName, num)
	}
	return assetParseTitleFallback
}

// isMeaningfulTitleLine 首行短文本有效性判定：去首尾空白后可见字符 >3，且至少
// 含一个文字/数字字符（纯标点、纯符号串不作标题）。
func isMeaningfulTitleLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	runes := []rune(trimmed)
	if len(runes) <= 3 {
		return false
	}
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// textContent 提取备注页等简单 XML 的全部文本（a:t 拼接，段落换行，字段排除）。
// 备注页只需整体文本，不区分标题/正文占位符。
func (e *zipEntry) textContent() string {
	dec := xml.NewDecoder(bytes.NewReader(e.data))
	var (
		lines     []string
		paraText  strings.Builder
		textDepth = -1
		fldDepth  = -1
		paraDepth = -1
		depth     int
	)
	flush := func() {
		if line := paraText.String(); strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
		paraText.Reset()
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "" // 备注页解析失败不阻断索引：备注按空处理（页正文仍可用）
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch {
			case fldDepth < 0 && t.Name.Space == drawingmlNS && t.Name.Local == "fld":
				fldDepth = depth
			case fldDepth < 0 && t.Name.Space == drawingmlNS && t.Name.Local == "p":
				paraDepth = depth
				paraText.Reset()
			case fldDepth < 0 && t.Name.Space == drawingmlNS && t.Name.Local == "t":
				textDepth = depth
			}
		case xml.CharData:
			if textDepth >= 0 {
				paraText.Write(t)
			}
		case xml.EndElement:
			switch {
			case textDepth == depth && t.Name.Space == drawingmlNS && t.Name.Local == "t":
				textDepth = -1
			case paraDepth == depth && t.Name.Space == drawingmlNS && t.Name.Local == "p":
				flush()
				paraDepth = -1
			case fldDepth == depth && t.Name.Space == drawingmlNS && t.Name.Local == "fld":
				fldDepth = -1
			}
			depth--
		}
	}
	flush()
	return strings.Join(lines, "\n")
}
