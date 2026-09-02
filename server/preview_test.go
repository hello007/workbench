package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newPreviewTestFile 在临时目录创建文件并返回绝对路径（正斜杠形式，与 URL path 一致）。
func newPreviewTestFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	abs := filepath.Join(dir, name)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file failed: %v", err)
	}
	return filepath.ToSlash(abs)
}

// TestPreviewHandler_PDFRegression /preview-pdf 既有行为回归：pdf 放行、其他扩展拒绝。
func TestPreviewHandler_PDFRegression(t *testing.T) {
	pdfPath := newPreviewTestFile(t, "doc.pdf", "%PDF-1.4 fake")
	txtPath := newPreviewTestFile(t, "note.txt", "hello")

	handler := PreviewHandler()

	// .pdf 放行
	req := httptest.NewRequest(http.MethodGet, "/preview-pdf?path="+pdfPath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("pdf: expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// 非 pdf 扩展拒绝（query 式路由仍仅限 pdf）
	req = httptest.NewRequest(http.MethodGet, "/preview-pdf?path="+txtPath, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("txt via preview-pdf: expected 400, got %d", rec.Code)
	}
}

// TestPreviewRaw_Served 白名单内资源（css/html/字体）正常返回，Content-Type 正确。
func TestPreviewRaw_Served(t *testing.T) {
	cssPath := newPreviewTestFile(t, "style.css", "body{color:red}")
	htmlPath := newPreviewTestFile(t, "index.html", "<html></html>")
	woff2Path := newPreviewTestFile(t, "font.woff2", "binary-ish")

	handler := PreviewHandler()
	cases := []struct {
		path        string
		wantCT      string
		wantContent string
	}{
		{cssPath, "text/css", "body{color:red}"},
		{htmlPath, "text/html", "<html></html>"},
		{woff2Path, "", "binary-ish"}, // 字体 Content-Type 由 mime 表决定，仅校验 200
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, previewRawPrefix+c.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d, body=%s", c.path, rec.Code, rec.Body.String())
			continue
		}
		if c.wantCT != "" {
			gotCT := rec.Header().Get("Content-Type")
			if len(gotCT) == 0 || gotCT[:len(c.wantCT)] != c.wantCT {
				t.Errorf("%s: Content-Type expected prefix %s, got %s", c.path, c.wantCT, gotCT)
			}
		}
		if rec.Body.String() != c.wantContent {
			t.Errorf("%s: body mismatch: %q", c.path, rec.Body.String())
		}
	}
}

// TestPreviewRaw_WhitelistRejected 白名单外扩展（.exe/.pdf/.go/无扩展）一律 400。
func TestPreviewRaw_WhitelistRejected(t *testing.T) {
	exePath := newPreviewTestFile(t, "evil.exe", "MZ")
	pdfPath := newPreviewTestFile(t, "doc.pdf", "%PDF")
	goPath := newPreviewTestFile(t, "main.go", "package main")
	noExtPath := newPreviewTestFile(t, "README", "x")

	handler := PreviewHandler()
	for _, p := range []string{exePath, pdfPath, goPath, noExtPath} {
		req := httptest.NewRequest(http.MethodGet, previewRawPrefix+p, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", p, rec.Code)
		}
	}
}

// TestPreviewRaw_PathTraversalRejected 相对路径 / 穿越形态拒绝。
func TestPreviewRaw_PathTraversalRejected(t *testing.T) {
	handler := PreviewHandler()
	badPaths := []string{
		previewRawPrefix,                     // 空路径
		previewRawPrefix + "style.css",       // 相对路径（无盘符）
		previewRawPrefix + "../style.css",    // 穿越
		previewRawPrefix + "..%2Fstyle.css",  // 编码穿越（Go 解码到 URL.Path）
		previewRawPrefix + "D%3A/style.css", // 解码后 D:/style.css（通常不存在，404 或 400 均非 200）
	}
	for _, p := range badPaths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("%s: expected non-200, got 200", p)
		}
	}
}

// TestPreviewRaw_DirectoryRejected 目录请求拒绝（列目录风险）。
func TestPreviewRaw_DirectoryRejected(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	handler := PreviewHandler()
	req := httptest.NewRequest(http.MethodGet, previewRawPrefix+dir, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("directory: expected 400, got %d", rec.Code)
	}
}

// TestPreviewRaw_NotFound 存在目录但文件不存在 → 404。
func TestPreviewRaw_NotFound(t *testing.T) {
	dir := filepath.ToSlash(t.TempDir())
	handler := PreviewHandler()
	req := httptest.NewRequest(http.MethodGet, previewRawPrefix+dir+"/missing.css", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing file: expected 404, got %d", rec.Code)
	}
}

// TestPreviewRaw_MethodRejected 非 GET/HEAD 拒绝。
func TestPreviewRaw_MethodRejected(t *testing.T) {
	cssPath := newPreviewTestFile(t, "style.css", "body{}")
	handler := PreviewHandler()
	req := httptest.NewRequest(http.MethodPost, previewRawPrefix+cssPath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: expected 405, got %d", rec.Code)
	}
}

// TestPreviewRaw_ChinesePath 中文路径/文件名正常返回。
func TestPreviewRaw_ChinesePath(t *testing.T) {
	cssPath := newPreviewTestFile(t, "样式表.css", "body{color:blue}")
	handler := PreviewHandler()
	req := httptest.NewRequest(http.MethodGet, previewRawPrefix+cssPath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("chinese path: expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
