package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/service"
)

// buildMinimalPptx 构造最小合法 pptx（单页），供 App 层委托测试走真实拆分核心。
// 核心拆分语义的 fixture 全链路在 service 包覆盖（程序化构造、禁依赖本机
// Office），此处只需接通 App 委托与结果透传。
func buildMinimalPptx(t *testing.T, dir, name string) string {
	t.Helper()
	const (
		xmlDecl = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
		relsNS  = "http://schemas.openxmlformats.org/package/2006/relationships"
	)
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", xmlDecl +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>` +
			`<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/>` +
			`</Types>`},
		{"_rels/.rels", xmlDecl +
			`<Relationships xmlns="` + relsNS + `">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="ppt/presentation.xml"/>` +
			`</Relationships>`},
		{"ppt/presentation.xml", xmlDecl +
			`<p:presentation xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
			`<p:sldIdLst><p:sldId id="256" r:id="rId2"/></p:sldIdLst>` +
			`<p:sldSz cx="12192000" cy="6858000"/>` +
			`</p:presentation>`},
		{"ppt/_rels/presentation.xml.rels", xmlDecl +
			`<Relationships xmlns="` + relsNS + `">` +
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/slide" Target="slides/slide1.xml"/>` +
			`</Relationships>`},
		{"ppt/slides/slide1.xml", xmlDecl +
			`<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"/>`},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		w, err := zw.Create(p.name)
		if err != nil {
			t.Fatalf("构造条目 %s: %v", p.name, err)
		}
		if _, err := w.Write([]byte(p.content)); err != nil {
			t.Fatalf("写入条目 %s: %v", p.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("收口 zip: %v", err)
	}
	src := filepath.Join(dir, name)
	if err := os.WriteFile(src, buf.Bytes(), 0644); err != nil {
		t.Fatalf("写入源文件: %v", err)
	}
	return src
}

// newPptxTestApp 构造 PPT 拆分域测试用 App：仅注入真实 PptxSplitService
// （生产构造 = 纯 Go 核心；nil ctx 时 wails 出口对事件发射静默跳过）。
func newPptxTestApp() *App {
	return &App{AppServices: &AppServices{pptxSplitSvc: service.NewPptxSplitService(nil)}}
}

// TestApp_SplitPptxPerPage_Delegates 委托链路：App 方法透传 service → 核心，
// 单页 pptx 拆出 1 个文件，输出目录/文件路径符合命名规则。
func TestApp_SplitPptxPerPage_Delegates(t *testing.T) {
	dir := t.TempDir()
	src := buildMinimalPptx(t, dir, "委托拆分.pptx")

	got, err := newPptxTestApp().SplitPptxPerPage(src)
	if err != nil {
		t.Fatalf("SplitPptxPerPage: %v", err)
	}
	wantOutDir := filepath.Join(dir, "委托拆分")
	if got.OutputDir != wantOutDir {
		t.Errorf("OutputDir = %q, want %q", got.OutputDir, wantOutDir)
	}
	if got.PageCount != 1 || len(got.Files) != 1 {
		t.Fatalf("应拆出 1 页，实际 PageCount=%d len(Files)=%d", got.PageCount, len(got.Files))
	}
	if got.Files[0] != filepath.Join(wantOutDir, "委托拆分-001.pptx") {
		t.Errorf("Files[0] = %q, want %q", got.Files[0], filepath.Join(wantOutDir, "委托拆分-001.pptx"))
	}
	if _, err := os.Stat(got.Files[0]); err != nil {
		t.Errorf("输出文件应存在: %v", err)
	}
}

// TestApp_SplitPptxPerPage_RejectsPpt 非 .pptx 输入错误透传（「仅支持 .pptx」
// 引导口径在核心层收敛，App 层零改写）。
func TestApp_SplitPptxPerPage_RejectsPpt(t *testing.T) {
	src := filepath.Join(t.TempDir(), "旧版.ppt")
	if err := os.WriteFile(src, []byte("irrelevant"), 0644); err != nil {
		t.Fatalf("写入源文件: %v", err)
	}
	_, err := newPptxTestApp().SplitPptxPerPage(src)
	if err == nil || !strings.Contains(err.Error(), "仅支持 .pptx") {
		t.Fatalf("应报「仅支持 .pptx」，实际：%v", err)
	}
}

// TestApp_CancelPptxSplit_NoInFlight 无在途拆分时取消幂等返回 false。
func TestApp_CancelPptxSplit_NoInFlight(t *testing.T) {
	if newPptxTestApp().CancelPptxSplit() {
		t.Error("无在途拆分应返回 false")
	}
}
