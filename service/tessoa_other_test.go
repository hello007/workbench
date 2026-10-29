//go:build !windows

package service

import (
	"os"
	"path/filepath"
	"testing"
)

// PATH 探测命中与配置失效回退用例：POSIX 下 fake tessoa 需可执行位且无 Windows
// 后缀语义，与 Windows（tessoa.exe 后缀解析）断言不同，按 linux-platform 约定拆双平台文件。

// TestResolveTessoa_PathLookupHits PATH 注入目录含可执行 tessoa 时探测命中。
func TestResolveTessoa_PathLookupHits(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tessoa")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0755); err != nil {
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
	exe := filepath.Join(dir, "tessoa")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	t.Setenv("PATH", dir)

	if got := resolveTessoaExecutable(filepath.Join(dir, "missing")); got != exe {
		t.Fatalf("配置路径失效后应回退 PATH 探测命中 stub，实际 = %q", got)
	}
}
