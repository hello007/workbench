package service

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readZipEntries 读回 zip 全部条目内容，按名称索引，供诊断包内容完整性断言。
func readZipEntries(t *testing.T, zipPath string) map[string]string {
	t.Helper()
	f, err := os.Open(zipPath)
	if err != nil {
		t.Fatalf("打开 zip: %v", err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		t.Fatalf("stat zip: %v", err)
	}
	zr, err := zip.NewReader(f, stat.Size())
	if err != nil {
		t.Fatalf("解析 zip: %v", err)
	}
	out := map[string]string{}
	for _, zf := range zr.File {
		rc, err := zf.Open()
		if err != nil {
			t.Fatalf("打开条目 %s: %v", zf.Name, err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		out[zf.Name] = string(data)
	}
	return out
}

// TestBuildDiagnosticsZip_Full 诊断包内容完整性：diagnostics.txt 元信息 +
// 日志文件收纳 + session.json 收纳。
func TestBuildDiagnosticsZip_Full(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(logDir, "app.log"), []byte("log line 1\n"), 0o644)
	os.WriteFile(filepath.Join(logDir, "app-2026-09-24T00-00-00.000.zip"), []byte("rotated"), 0o644)
	sessionPath := filepath.Join(dir, "session.json")
	os.WriteFile(sessionPath, []byte(`{"version":"2"}`), 0o644)

	zipPath := filepath.Join(dir, "diag.zip")
	info := DiagnosticsInfo{Version: "1.2.3", BuildTime: "2026-09-24", GoVersion: "go1.26.6", OSInfo: "windows/amd64"}
	if err := BuildDiagnosticsZip(zipPath, info, logDir, sessionPath); err != nil {
		t.Fatalf("BuildDiagnosticsZip: %v", err)
	}

	entries := readZipEntries(t, zipPath)
	if len(entries) != 4 {
		t.Fatalf("应有 4 个条目: %v", keysOf(entries))
	}
	diag := entries["diagnostics.txt"]
	for _, want := range []string{"1.2.3", "2026-09-24", "go1.26.6", "windows/amd64", "logs/app.log", "session.json"} {
		if !strings.Contains(diag, want) {
			t.Errorf("diagnostics.txt 缺 %q: %s", want, diag)
		}
	}
	if entries["logs/app.log"] != "log line 1\n" {
		t.Errorf("日志内容异常: %q", entries["logs/app.log"])
	}
	if entries["session.json"] != `{"version":"2"}` {
		t.Errorf("session.json 内容异常: %q", entries["session.json"])
	}
}

// TestBuildDiagnosticsZip_MissingLogsAndSession 日志目录与 session.json 均缺失时
// 降级：仅 diagnostics.txt 且注明缺失说明。
func TestBuildDiagnosticsZip_MissingLogsAndSession(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "diag.zip")

	info := DiagnosticsInfo{Version: "dev"}
	if err := BuildDiagnosticsZip(zipPath, info, filepath.Join(dir, "no-logs"), filepath.Join(dir, "no-session.json")); err != nil {
		t.Fatalf("缺失场景应降级不报错: %v", err)
	}

	entries := readZipEntries(t, zipPath)
	if len(entries) != 1 {
		t.Fatalf("仅应有 diagnostics.txt: %v", keysOf(entries))
	}
	diag := entries["diagnostics.txt"]
	if !strings.Contains(diag, "[缺失] logs/") || !strings.Contains(diag, "[缺失] session.json") {
		t.Errorf("应注明缺失项: %s", diag)
	}
}

// TestBuildDiagnosticsZip_UnwritableTarget 目标路径不可写报错。
func TestBuildDiagnosticsZip_UnwritableTarget(t *testing.T) {
	info := DiagnosticsInfo{Version: "dev"}
	if err := BuildDiagnosticsZip(filepath.Join(t.TempDir(), "no-such-dir", "d.zip"), info, t.TempDir(), "x.json"); err == nil {
		t.Error("不可写路径应报错")
	}
}

// TestCrashFlagLifecycle 异常退出标记全周期：不存在→标记→检测→清除→不存在。
func TestCrashFlagLifecycle(t *testing.T) {
	flag := filepath.Join(t.TempDir(), "crash.flag")

	if DetectLastCrash(flag) {
		t.Error("初始不应有崩溃标记")
	}
	if err := MarkSessionStart(flag); err != nil {
		t.Fatalf("MarkSessionStart: %v", err)
	}
	if !DetectLastCrash(flag) {
		t.Error("标记后应检测到")
	}
	// 标记内容为时间戳文本（人工可读创建时机）
	data, _ := os.ReadFile(flag)
	if len(strings.TrimSpace(string(data))) == 0 {
		t.Error("标记文件应含时间戳内容")
	}
	if err := ClearCrashFlag(flag); err != nil {
		t.Fatalf("ClearCrashFlag: %v", err)
	}
	if DetectLastCrash(flag) {
		t.Error("清除后不应有崩溃标记")
	}
	// 清除不存在的标记静默成功（shutdown 幂等）
	if err := ClearCrashFlag(flag); err != nil {
		t.Errorf("重复清除应静默成功: %v", err)
	}
}

// keysOf 返回 map 的键列表（测试断言辅助）。
func keysOf(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
