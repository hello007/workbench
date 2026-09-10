package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/util"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		// 相等
		{"相同版本", "1.0.0", "1.0.0", 0},
		{"相同版本带v前缀", "v1.0.0", "1.0.0", 0},
		{"都是v前缀", "v1.0.0", "v1.0.0", 0},

		// 大于
		{"主版本号大", "2.0.0", "1.0.0", 1},
		{"次版本号大", "1.1.0", "1.0.0", 1},
		{"修订号大", "1.0.9", "1.0.8", 1},
		{"实际更新场景", "1.0.9", "1.0.8", 1},

		// 小于
		{"主版本号小", "1.0.0", "2.0.0", -1},
		{"次版本号小", "1.0.0", "1.1.0", -1},
		{"修订号小", "1.0.8", "1.0.9", -1},
		{"实际当前版本旧", "1.0.7", "1.0.8", -1},

		// 不同位数
		{"两位vs三位", "1.0", "1.0.0", 0},
		{"一位vs三位", "1", "1.0.0", 0},
		{"短版本小于", "1.0", "1.0.1", -1},
		{"短版本大于", "1.1", "1.0.9", 1},

		// dev 版本
		{"dev版本", "dev", "1.0.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, expected %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		bytesPerSec float64
		unit        string
	}{
		{500, "B/s"},
		{1500, "KB/s"},
		{1500000, "MB/s"},
	}

	for _, tt := range tests {
		result := formatSpeed(tt.bytesPerSec)
		if !strContains(result, tt.unit) {
			t.Errorf("formatSpeed(%v) = %q, expected to contain %q", tt.bytesPerSec, result, tt.unit)
		}
	}
}

func strContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestBuildUpdateBat_ContainsKeyParts 生成的批处理脚本含 PID 等待、exe 替换、清理、启动各关键片段。
func TestBuildUpdateBat_ContainsKeyParts(t *testing.T) {
	bat := buildUpdateBat(1234, `C:\new.exe`, `C:\cur.exe`, `C:\pending.json`, `C:\upd`)
	wants := []string{
		"@echo off",
		"PID=1234",
		`move /Y "C:\new.exe" "C:\cur.exe"`,
		`del /Q "C:\pending.json"`,
		`rd /S /Q "C:\upd"`,
		`start "" "C:\cur.exe"`,
		"wait_loop",
	}
	for _, w := range wants {
		if !strings.Contains(bat, w) {
			t.Errorf("buildUpdateBat 缺少 %q\n输出:\n%s", w, bat)
		}
	}
}

// TestBuildApplyBat_ContainsKeyParts 生成的应用脚本含 exe 替换、清理、启动片段。
func TestBuildApplyBat_ContainsKeyParts(t *testing.T) {
	bat := buildApplyBat(`C:\new.exe`, `C:\cur.exe`, `C:\pending.json`, `C:\upd`)
	wants := []string{
		"@echo off",
		`move /Y "C:\new.exe" "C:\cur.exe"`,
		`del /Q "C:\pending.json"`,
		`rd /S /Q "C:\upd"`,
		`start "" "C:\cur.exe"`,
	}
	for _, w := range wants {
		if !strings.Contains(bat, w) {
			t.Errorf("buildApplyBat 缺少 %q\n输出:\n%s", w, bat)
		}
	}
}

// TestNewUpdateService_Construct 构造后 httpClient 已初始化。
func TestNewUpdateService_Construct(t *testing.T) {
	svc := NewUpdateService()
	if svc == nil {
		t.Fatal("NewUpdateService 返回 nil")
	}
	if svc.httpClient == nil {
		t.Error("httpClient 应已初始化")
	}
}

// TestUpdateService_SetContext SetContext 后 ctx 生效。
func TestUpdateService_SetContext(t *testing.T) {
	svc := NewUpdateService()
	ctx := context.Background()
	svc.SetContext(ctx)
	if svc.ctx != ctx {
		t.Error("SetContext 未生效")
	}
}

// TestHideWindow 返回隐藏窗口的 SysProcAttr。
func TestHideWindow(t *testing.T) {
	attr := hideWindow()
	if attr == nil {
		t.Fatal("hideWindow 应返回非 nil")
	}
	if !attr.HideWindow {
		t.Error("HideWindow 应为 true")
	}
	if attr.CreationFlags != 0x08000000 {
		t.Errorf("CreationFlags 期望 0x08000000, got %#x", attr.CreationFlags)
	}
}

// mockTransport 拦截 HTTP 请求返回固定响应，用于 CheckForUpdate 单测（绕过真实 GitHub API）。
type mockTransport struct {
	body       string
	statusCode int
	err        error
}

func (m *mockTransport) RoundTrip(*http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(strings.NewReader(m.body)),
		Header:     make(http.Header),
	}, nil
}

func newUpdateSvcWithMock(status int, body string) *UpdateService {
	svc := NewUpdateService()
	svc.httpClient = &http.Client{Transport: &mockTransport{statusCode: status, body: body}}
	return svc
}

// TestCheckForUpdate_Success 有效响应返回 UpdateInfo，版本比较正确。
func TestCheckForUpdate_Success(t *testing.T) {
	body := `{"tag_name":"v1.0.5","body":"release notes","published_at":"2026-01-01","assets":[{"name":"workbench.exe","browser_download_url":"http://example.com/wb.exe","size":12345}]}`
	svc := newUpdateSvcWithMock(200, body)
	info, err := svc.CheckForUpdate("1.0.0")
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if info == nil {
		t.Fatal("info 不应为 nil")
	}
	if info.LatestVer != "1.0.5" {
		t.Errorf("LatestVer: got %s, want 1.0.5", info.LatestVer)
	}
	if info.DownloadURL != "http://example.com/wb.exe" {
		t.Errorf("DownloadURL: got %s", info.DownloadURL)
	}
	if info.FileSize != 12345 {
		t.Errorf("FileSize: got %d", info.FileSize)
	}
	if !info.HasUpdate {
		t.Error("HasUpdate 应为 true（1.0.5 > 1.0.0）")
	}
}

// TestCheckForUpdate_NoAsset 无 workbench.exe 资产返回错误。
func TestCheckForUpdate_NoAsset(t *testing.T) {
	body := `{"tag_name":"v1.0.5","assets":[{"name":"other.exe","browser_download_url":"http://x","size":1}]}`
	svc := newUpdateSvcWithMock(200, body)
	if _, err := svc.CheckForUpdate("1.0.0"); err == nil {
		t.Error("无 workbench.exe 资产应返回错误")
	}
}

// TestCheckForUpdate_Non200 非 200 状态返回错误。
func TestCheckForUpdate_Non200(t *testing.T) {
	svc := newUpdateSvcWithMock(404, "")
	if _, err := svc.CheckForUpdate("1.0.0"); err == nil {
		t.Error("非 200 应返回错误")
	}
}

// TestCheckForUpdate_InvalidJSON 非法 JSON 返回错误。
func TestCheckForUpdate_InvalidJSON(t *testing.T) {
	svc := newUpdateSvcWithMock(200, "{not json")
	if _, err := svc.CheckForUpdate("1.0.0"); err == nil {
		t.Error("非法 JSON 应返回错误")
	}
}

// TestCheckForUpdate_HttpError 请求失败返回错误。
func TestCheckForUpdate_HttpError(t *testing.T) {
	svc := NewUpdateService()
	svc.httpClient = &http.Client{Transport: &mockTransport{err: io.ErrUnexpectedEOF}}
	if _, err := svc.CheckForUpdate("1.0.0"); err == nil {
		t.Error("HTTP 错误应返回错误")
	}
}

// TestCheckForUpdate_NoUpdate 当前版本已最新时 HasUpdate=false。
func TestCheckForUpdate_NoUpdate(t *testing.T) {
	body := `{"tag_name":"v1.0.0","assets":[{"name":"workbench.exe","browser_download_url":"http://x","size":1}]}`
	svc := newUpdateSvcWithMock(200, body)
	info, err := svc.CheckForUpdate("1.0.0")
	if err != nil {
		t.Fatalf("CheckForUpdate: %v", err)
	}
	if info.HasUpdate {
		t.Error("同版本 HasUpdate 应为 false")
	}
}

// cleanupUpdateDir 清理 DownloadUpdate 写入全局临时目录的产物，避免污染其他测试。
func cleanupUpdateDir(t *testing.T) {
	t.Helper()
	_ = os.RemoveAll(filepath.Join(os.TempDir(), UpdateTempDir))
}

// TestDownloadUpdate_Success 下载成功写入 workbench.exe 与 pending 标记。
func TestDownloadUpdate_Success(t *testing.T) {
	cleanupUpdateDir(t)
	defer cleanupUpdateDir(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake exe content"))
	}))
	defer srv.Close()

	svc := NewUpdateService() // ctx=nil，跳过进度 emit
	if err := svc.DownloadUpdate(srv.URL); err != nil {
		t.Fatalf("DownloadUpdate: %v", err)
	}
	target := filepath.Join(os.TempDir(), UpdateTempDir, "workbench.exe")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("读下载文件: %v", err)
	}
	if string(data) != "fake exe content" {
		t.Errorf("下载内容不符: %q", data)
	}
	// pending 标记应已生成
	pending := filepath.Join(os.TempDir(), UpdateTempDir, PendingUpdateFile)
	if !util.FileExists(pending) {
		t.Error("pending 标记应已生成")
	}
}

// TestDownloadUpdate_Non200 非 200 返回错误。
func TestDownloadUpdate_Non200(t *testing.T) {
	cleanupUpdateDir(t)
	defer cleanupUpdateDir(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	svc := NewUpdateService()
	if err := svc.DownloadUpdate(srv.URL); err == nil {
		t.Error("500 应返回错误")
	}
}

// TestDownloadUpdate_HttpError 连接失败返回错误。
func TestDownloadUpdate_HttpError(t *testing.T) {
	cleanupUpdateDir(t)
	defer cleanupUpdateDir(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 立即关闭，连接必失败

	svc := NewUpdateService()
	if err := svc.DownloadUpdate(srv.URL); err == nil {
		t.Error("连接失败应返回错误")
	}
}

// TestDownloadUpdate_InvalidURL 非法 URL 返回错误。
func TestDownloadUpdate_InvalidURL(t *testing.T) {
	cleanupUpdateDir(t)
	defer cleanupUpdateDir(t)

	svc := NewUpdateService()
	// 含空格的 URL 不是合法 http URL，NewRequestWithContext 失败
	if err := svc.DownloadUpdate("http://invalid host with space"); err == nil {
		t.Error("非法 URL 应返回错误")
	}
}

// TestCancelDownload_NoActiveDownload 无活跃下载时不 panic。
func TestCancelDownload_NoActiveDownload(t *testing.T) {
	svc := NewUpdateService()
	svc.CancelDownload()
}

// TestApplyUpdate_NotExists 更新文件不存在时返回错误（不启动 bat）。
func TestApplyUpdate_NotExists(t *testing.T) {
	cleanupUpdateDir(t)
	defer cleanupUpdateDir(t)
	svc := NewUpdateService()
	if err := svc.ApplyUpdate(); err == nil {
		t.Error("更新文件不存在应返回错误")
	}
}

// TestCheckPendingUpdate_NoPending 无 pending 标记返回 false。
func TestCheckPendingUpdate_NoPending(t *testing.T) {
	cleanupUpdateDir(t)
	defer cleanupUpdateDir(t)
	svc := NewUpdateService()
	has, err := svc.CheckPendingUpdate()
	if err != nil {
		t.Fatalf("CheckPendingUpdate: %v", err)
	}
	if has {
		t.Error("无 pending 应返回 false")
	}
}

// TestCheckPendingUpdate_PendingButNoExe pending 在但 exe 不存在时清理并返回 false。
func TestCheckPendingUpdate_PendingButNoExe(t *testing.T) {
	cleanupUpdateDir(t)
	defer cleanupUpdateDir(t)
	updateDir := filepath.Join(os.TempDir(), UpdateTempDir)
	if err := os.MkdirAll(updateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(updateDir, PendingUpdateFile), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write pending: %v", err)
	}
	// 不创建 workbench.exe

	svc := NewUpdateService()
	has, err := svc.CheckPendingUpdate()
	if err != nil {
		t.Fatalf("CheckPendingUpdate: %v", err)
	}
	if has {
		t.Error("pending 但无 exe 应返回 false 并清理")
	}
}
