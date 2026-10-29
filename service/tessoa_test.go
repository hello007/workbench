package service

import (
	"os"
	"path/filepath"
	"testing"
)

// 以下用例将 PATH 置空以隔离本机是否真实安装 tessoa 的环境差异
// （test-stability：测试禁隐式依赖环境外部状态）。PATH 探测命中分支的
// 平台差异（Windows 可执行后缀解析 vs POSIX 可执行位）见双平台测试文件。

// TestResolveTessoa_ConfiguredFileExists 设置配置路径存在时直接采用，不依赖 PATH。
func TestResolveTessoa_ConfiguredFileExists(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tessoa-custom.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", "")

	if got := resolveTessoaExecutable(exe); got != exe {
		t.Fatalf("resolveTessoaExecutable(%q) = %q, want %q", exe, got, exe)
	}
}

// TestResolveTessoa_ConfiguredDirRejected 配置路径指向目录时无效，
// PATH 置空后无可探测目标，返回空串交由调用方降级系统管理器。
func TestResolveTessoa_ConfiguredDirRejected(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "")

	if got := resolveTessoaExecutable(dir); got != "" {
		t.Fatalf("resolveTessoaExecutable(dir) = %q, want empty", got)
	}
}

// TestResolveTessoa_NotConfiguredPathEmpty 未配置且 PATH 置空（本机无 tessoa），
// 返回空串由调用方降级系统文件管理器。
func TestResolveTessoa_NotConfiguredPathEmpty(t *testing.T) {
	t.Setenv("PATH", "")

	if got := resolveTessoaExecutable(""); got != "" {
		t.Fatalf("resolveTessoaExecutable(\"\") = %q, want empty", got)
	}
}

// TestOpenInExplorer_TessoaLaunchFallsBack tessoa 配置路径有效但启动失败时
// 降级系统文件管理器，不向调用方报错。会真实启动外部 GUI 进程，默认跳过；
// 如需手动验证请删除此行。
func TestOpenInExplorer_TessoaLaunchFallsBack(t *testing.T) {
	t.Skip("会启动外部 GUI 进程（资源管理器/tessoa），默认跳过；如需手动验证请删除此行")
	dir := t.TempDir()
	svc := NewFileOperationService()

	// 配置路径指向存在的普通文件：探测命中但 exec.Start 必失败（.bin 非 Windows
	// PATHEXT 可执行后缀、POSIX 无可执行位），确定走降级分支而非 PATH 回退
	stub := filepath.Join(dir, "not-executable.bin")
	if err := os.WriteFile(stub, []byte("stub"), 0644); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	err := svc.OpenInExplorer(dir, stub)
	if err != nil {
		t.Fatalf("tessoa 启动失败应降级而非报错: %v", err)
	}
}
