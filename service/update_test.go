package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"workbench/model"
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

// TestBuildUpdateSh_ContainsKeyParts 生成的 shell 脚本含 kill -0 等待轮询、超时强杀
// （强杀前 cmdline 身份校验防 PID 复用误杀，G4）、二进制替换（单引号包裹 + 失败中止）、
// 清理、nohup 启动、脚本自删各关键片段，且为 LF 换行（CR 会导致 shebang 解析失败）。
func TestBuildUpdateSh_ContainsKeyParts(t *testing.T) {
	sh := buildUpdateSh(1234, "/tmp/upd/workbench", "/opt/workbench/workbench", "/tmp/upd/pending.json", "/tmp/upd")
	wants := []string{
		"#!/bin/sh",
		"PID=1234",
		`while kill -0 "$PID" 2>/dev/null; do`,
		// G4：kill -9 前校验 /proc cmdline 含 workbench（tr 转 NUL 分隔的 cmdline 后 grep），
		// /proc 不可读时保守跳过校验，非 workbench 进程复用 PID 时跳过强杀
		`if [ -r "/proc/$PID/cmdline" ] && ! tr '\0' ' ' < "/proc/$PID/cmdline" | grep -q workbench; then`,
		`echo '旧进程 PID 已被其他进程复用，跳过强杀' >&2`,
		`kill -9 "$PID" 2>/dev/null`,
		"sleep 1",
		// 路径单引号包裹（shell 注入防护）+ mv 失败非零退出（防假更新：失败不得清理/重启）
		`mv -f '/tmp/upd/workbench' '/opt/workbench/workbench' || { echo '替换二进制失败（检查安装目录写入权限）' >&2; exit 1; }`,
		`rm -f '/tmp/upd/pending.json'`,
		`rm -rf '/tmp/upd'`,
		`nohup '/opt/workbench/workbench' >/dev/null 2>&1 &`,
		`rm -f -- "$0"`,
	}
	for _, w := range wants {
		if !strings.Contains(sh, w) {
			t.Errorf("buildUpdateSh 缺少 %q\n输出:\n%s", w, sh)
		}
	}
	if strings.Contains(sh, "\r") {
		t.Error("shell 脚本应为 LF 换行，不应包含 CR")
	}
}

// TestBuildApplySh_ContainsKeyParts 生成的应用脚本含替换（单引号包裹）、成功分支清理与
// 启动、失败分支跳过更新并拉起旧版本、自删片段，且为 LF 换行。
func TestBuildApplySh_ContainsKeyParts(t *testing.T) {
	sh := buildApplySh("/tmp/upd/workbench", "/opt/workbench/workbench", "/tmp/upd/pending.json", "/tmp/upd")
	wants := []string{
		"#!/bin/sh",
		// 成功分支：替换 → 清理 → 启动新版本
		`if mv -f '/tmp/upd/workbench' '/opt/workbench/workbench'; then`,
		`rm -f '/tmp/upd/pending.json'`,
		`rm -rf '/tmp/upd'`,
		`nohup '/opt/workbench/workbench' >/dev/null 2>&1 &`,
		// 失败分支：清除 pending 防反复重试、拉起旧版本保证应用可用（app 已 os.Exit(0)）、非零退出
		"else",
		`echo '替换二进制失败（检查安装目录写入权限），本次更新已跳过' >&2`,
		`exit 1`,
		`rm -f -- "$0"`,
	}
	for _, w := range wants {
		if !strings.Contains(sh, w) {
			t.Errorf("buildApplySh 缺少 %q\n输出:\n%s", w, sh)
		}
	}
	if strings.Contains(sh, "\r") {
		t.Error("shell 脚本应为 LF 换行，不应包含 CR")
	}
}

// TestShellQuote 单引号包裹与转义：单引号转义为 '\''，其余 shell 元字符
// （双引号 / $() / 分号）在单引号内失去特殊含义。
func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/opt/a b/c", `'/opt/a b/c'`},
		{`/opt/it's`, `'/opt/it'\''s'`},
		{`a"b$(reboot)c;d`, `'a"b$(reboot)c;d'`},
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBuildShScripts_QuoteInjection 路径含 shell 元字符（双引号 / 命令替换 / 分号 /
// 单引号）时必须整体落在单引号内且单引号经 '\'' 转义；不得以双引号直插路径
// （双引号内 $() 会被执行）。产出合法性由 linux 侧 sh -n 静态校验双重兜底。
func TestBuildShScripts_QuoteInjection(t *testing.T) {
	evil := `/opt/ev"il/$(reboot)/x;rm -rf /;y'a z`
	sh := buildUpdateSh(1, evil, "/opt/workbench/workbench", "/tmp/p", "/tmp/upd")
	apply := buildApplySh(evil, "/opt/workbench/workbench", "/tmp/p", "/tmp/upd")
	for name, content := range map[string]string{"update.sh": sh, "apply-update.sh": apply} {
		// evil 中的单引号必须以 '\'' 转义形态出现（POSIX 单引号内嵌单引号惯用法）
		if !strings.Contains(content, `y'\''a z`) {
			t.Errorf("%s 未对路径内单引号做 '\\'' 转义\n输出:\n%s", name, content)
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

// TestUpdateService_SetContext SetContext 后事件出口已注入，且 emit 安全（nil/非 Wails ctx 不 fatal）。
func TestUpdateService_SetContext(t *testing.T) {
	svc := NewUpdateService()
	if svc.eventSink() != nil {
		t.Error("构造后 sink 应为 nil（未注入上下文前不推送事件）")
	}
	svc.SetContext(nil)
	if svc.eventSink() == nil {
		t.Error("SetContext 未注入事件出口")
	}
	// nil ctx 与非 Wails 上下文（无 events 键）下 Emit 均须静默跳过，不得触发 wails runtime fatal
	svc.eventSink().Emit("update:download-progress", model.DownloadProgress{})
	svc.SetContext(context.Background())
	svc.eventSink().Emit("update:download-progress", model.DownloadProgress{})
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

// TestUpdateAssetName_MatchesPlatform 资产名平台无关断言（G5）：期望值不与实现共用
// GOOS 分支取同一常量（分支同构恒真只验证常量绑定），改断言产物名特征——后缀/平台段/
// 架构段，锚定与 release.yml 打包产物命名约定的行为一致（G3 起资产名按 GOARCH 组装）。
func TestUpdateAssetName_MatchesPlatform(t *testing.T) {
	name := updateAssetName()
	if runtime.GOOS == "windows" {
		// Windows 资产即二进制本体，固定 .exe（GOARCH 无关，release.yml 单一 exe 产物）
		if !strings.HasSuffix(name, ".exe") {
			t.Errorf("Windows 资产名应以 .exe 结尾, got %q", name)
		}
		return
	}
	if !strings.HasSuffix(name, ".tar.gz") {
		t.Errorf("Linux 资产名应以 .tar.gz 结尾, got %q", name)
	}
	if !strings.Contains(name, "linux-") {
		t.Errorf("Linux 资产名应含平台段 linux-, got %q", name)
	}
	// G3：资产名按 GOARCH 组装（amd64 命中 release 产物 workbench-linux-amd64.tar.gz）
	if !strings.Contains(name, runtime.GOARCH) {
		t.Errorf("Linux 资产名应含架构段 %s, got %q", runtime.GOARCH, name)
	}
}

// TestUpdateBinaryPath_MatchesPlatform 更新目录内二进制路径平台无关断言（G5）：
// Windows 资产即二进制本体（.exe 落地）；Linux 断言 Base 名与 tar.gz 解包约定一致
// （extractUpdateTarGz 仅提取 Base 为 workbench 的成员，字面量锚定该约定而非实现常量）。
func TestUpdateBinaryPath_MatchesPlatform(t *testing.T) {
	got := updateBinaryPath("/tmp/upd")
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(filepath.Base(got), ".exe") {
			t.Errorf("Windows 更新二进制应为 .exe 本体, got %q", got)
		}
		return
	}
	if filepath.Base(got) != "workbench" {
		t.Errorf("Linux 更新二进制 Base 名应与 tar.gz 解包约定一致（workbench）, got %q", got)
	}
}

// TestNoAssetError 资产未命中错误按架构分流（G3）：现有产物覆盖面（Windows 任意架构 /
// linux amd64）维持原文案；无产物的架构（linux/arm64 等）明确提示暂不支持，参数化
// goos/goarch 使全部分支跨平台可测。
func TestNoAssetError(t *testing.T) {
	cases := []struct {
		goos      string
		goarch    string
		wantText  string
		notWanted string
	}{
		{"windows", "amd64", "未找到可下载的更新文件", "暂不支持"},
		{"windows", "arm64", "未找到可下载的更新文件", "暂不支持"},
		{"linux", "amd64", "未找到可下载的更新文件", "暂不支持"},
		{"linux", "arm64", "暂不支持 linux/arm64 架构的自动更新", "未找到可下载的更新文件"},
		{"darwin", "arm64", "暂不支持 darwin/arm64 架构的自动更新", "未找到可下载的更新文件"},
	}
	for _, c := range cases {
		err := noAssetError(c.goos, c.goarch)
		if err == nil {
			t.Errorf("noAssetError(%s/%s) 应返回错误", c.goos, c.goarch)
			continue
		}
		if !strings.Contains(err.Error(), c.wantText) {
			t.Errorf("noAssetError(%s/%s) = %q, 应含 %q", c.goos, c.goarch, err.Error(), c.wantText)
		}
		if strings.Contains(err.Error(), c.notWanted) {
			t.Errorf("noAssetError(%s/%s) = %q, 不应含 %q", c.goos, c.goarch, err.Error(), c.notWanted)
		}
	}
}

// buildTestTarGz 构造测试用最小 tar.gz（扁平布局多成员），返回字节流。
// 仅 Linux 下载分支消费（模拟 release.yml 打包产物结构），Windows 分支载荷为 exe 字节；
// 无条件编译隔离以简化引用（跨平台编译均可达，行为由运行时 GOOS 分支决定）。
func buildTestTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     name,
			Mode:     0o755,
			Size:     int64(len(content)),
		}); err != nil {
			t.Fatalf("写 tar 头 %s: %v", name, err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("写 tar 内容 %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("关闭 tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("关闭 gzip: %v", err)
	}
	return buf.Bytes()
}

// updateTestPayload 按当前平台资产形态构造测试下载载荷：
// Windows 为 exe 字节直接落地；Linux 为最小 tar.gz（覆盖下载后解包链路）。
func updateTestPayload(t *testing.T) []byte {
	t.Helper()
	if runtime.GOOS == "windows" {
		return []byte("fake exe content")
	}
	return buildTestTarGz(t, map[string][]byte{
		updateBinaryLinux: []byte("fake linux binary"),
		"README-linux.md": []byte("readme"),
	})
}

// TestCheckForUpdate_Success 有效响应返回 UpdateInfo，版本比较正确。
// 资产名按当前平台构造（Windows/Linux CI 均须命中各自资产）。
func TestCheckForUpdate_Success(t *testing.T) {
	body := fmt.Sprintf(`{"tag_name":"v1.0.5","body":"release notes","published_at":"2026-01-01","assets":[{"name":%q,"browser_download_url":"http://example.com/asset","size":12345}]}`, updateAssetName())
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
	if info.DownloadURL != "http://example.com/asset" {
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
	body := fmt.Sprintf(`{"tag_name":"v1.0.0","assets":[{"name":%q,"browser_download_url":"http://x","size":1}]}`, updateAssetName())
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

// TestDownloadUpdate_Success 下载成功写入平台资产并生成 pending 标记。
// Windows 资产为 exe 字节直接落地；Linux 资产为 tar.gz，断言解包出的二进制内容
// （解包细节由 update_extract_linux_test.go 覆盖）。
func TestDownloadUpdate_Success(t *testing.T) {
	cleanupUpdateDir(t)
	defer cleanupUpdateDir(t)

	payload := updateTestPayload(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	svc := NewUpdateService() // ctx=nil，跳过进度 emit
	if err := svc.DownloadUpdate(srv.URL); err != nil {
		t.Fatalf("DownloadUpdate: %v", err)
	}

	updateDir := filepath.Join(os.TempDir(), UpdateTempDir)
	if runtime.GOOS == "windows" {
		data, err := os.ReadFile(filepath.Join(updateDir, updateAssetWindows))
		if err != nil {
			t.Fatalf("读下载文件: %v", err)
		}
		if string(data) != "fake exe content" {
			t.Errorf("下载内容不符: %q", data)
		}
	} else {
		data, err := os.ReadFile(filepath.Join(updateDir, updateBinaryLinux))
		if err != nil {
			t.Fatalf("读解包二进制: %v", err)
		}
		if string(data) != "fake linux binary" {
			t.Errorf("解包内容不符: %q", data)
		}
	}

	// pending 标记应已生成
	pending := filepath.Join(updateDir, PendingUpdateFile)
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
