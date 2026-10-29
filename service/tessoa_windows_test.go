//go:build windows

package service

import (
	"os"
	"path/filepath"
	"testing"
)

// PATH 探测命中与配置失效回退用例：fake tessoa 需 Windows 可执行后缀（tessoa.exe），
// 与 POSIX（可执行位、无后缀）断言不同，按 linux-platform 约定拆双平台文件。

// TestResolveTessoa_PathLookupHits Windows 下 PATH 注入目录含 tessoa.exe 时探测命中。
func TestResolveTessoa_PathLookupHits(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tessoa.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir)

	if got := resolveTessoaExecutable(""); got != exe {
		t.Fatalf("resolveTessoaExecutable(\"\") = %q, want %q", got, exe)
	}
}

// TestResolveTessoa_ConfiguredMissingFallsBackPATH 配置路径失效时回退 PATH 探测命中。
func TestResolveTessoa_ConfiguredMissingFallsBackPATH(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tessoa.exe")
	if err := os.WriteFile(exe, []byte("stub"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir)

	if got := resolveTessoaExecutable(filepath.Join(dir, "missing.exe")); got != exe {
		t.Fatalf("配置路径失效后应回退 PATH 探测命中 stub，实际 = %q", got)
	}
}
