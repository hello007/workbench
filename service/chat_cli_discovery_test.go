package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// withChatCliNames 临时替换已知命令名集合（ DiscoverChatClis 白名单口径），
// 测试结束恢复。fake 名用本用例专属命名，避免与本机 PATH 真实命令撞名。
func withChatCliNames(t *testing.T, names []string) {
	t.Helper()
	orig := chatCliKnownNames
	chatCliKnownNames = names
	t.Cleanup(func() { chatCliKnownNames = orig })
}

// writeFakeShim 在 dir 写一个内容可被 shim 解析的 .cmd（目标 exe 一并预置），
// 返回 shim 完整路径。
func writeFakeShim(t *testing.T, dir, shimName, targetRel string) string {
	t.Helper()
	shimBody := "@ECHO off\r\n\"%dp0%\\" + targetRel + "\"   %*\r\n"
	shim := filepath.Join(dir, shimName)
	if err := os.WriteFile(shim, []byte(shimBody), 0o644); err != nil {
		t.Fatalf("写 shim %s: %v", shimName, err)
	}
	full := filepath.Join(dir, targetRel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte("PE"), 0o644); err != nil {
		t.Fatalf("写目标: %v", err)
	}
	return shim
}

// TestDiscoverChatClis_HitAndDedup 表驱动：PATH 注入 fake 命令验证命中、
// 嵌套 shim 指向同一 exe 的按路径去重、PATH 无命中返回空集。
func TestDiscoverChatClis_HitAndDedup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("fake shim 依赖 .cmd + PATHEXT 语义，仅 Windows 生效")
	}
	cases := []struct {
		name      string
		cliNames  []string // 临时替换的已知命令名集合
		shims     []string // 预置 shim 文件名（均指向 fake-target.exe）
		wantNames []string // 期望命中的命令名（按集合声明序）
	}{
		{
			name:      "命中单个候选",
			cliNames:  []string{"fakeai-a"},
			shims:     []string{"fakeai-a.cmd"},
			wantNames: []string{"fakeai-a"},
		},
		{
			name:      "两个命名指向同一 exe 按路径去重保留首个",
			cliNames:  []string{"fakeai-b", "fakeai-c"},
			shims:     []string{"fakeai-b.cmd", "fakeai-c.cmd"},
			wantNames: []string{"fakeai-b"},
		},
		{
			name:      "部分命中：未安装命令跳过",
			cliNames:  []string{"fakeai-missing", "fakeai-d"},
			shims:     []string{"fakeai-d.cmd"},
			wantNames: []string{"fakeai-d"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, shim := range tc.shims {
				writeFakeShim(t, dir, shim, `node_modules\fake\fake-target.exe`)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			withChatCliNames(t, tc.cliNames)

			got := DiscoverChatClis()
			if len(got) != len(tc.wantNames) {
				t.Fatalf("候选数: got %d (%+v), want %d", len(got), got, len(tc.wantNames))
			}
			for i, want := range tc.wantNames {
				if got[i].Name != want {
					t.Errorf("候选[%d].Name: got %q want %q", i, got[i].Name, want)
				}
				if got[i].Path == "" {
					t.Errorf("候选[%d].Path 不应为空", i)
				}
			}
		})
	}
}

// TestDiscoverChatClis_Empty PATH 无任何已知命令命中时返回空列表不报错。
func TestDiscoverChatClis_Empty(t *testing.T) {
	// PATH 指向空目录：全部命令名未命中
	t.Setenv("PATH", t.TempDir())
	withChatCliNames(t, []string{"fakeai-none-1", "fakeai-none-2"})

	got := DiscoverChatClis()
	if len(got) != 0 {
		t.Errorf("空 PATH 应返回空列表, got %+v", got)
	}
}

// TestDiscoverChatClis_DedupKeyFallsBackToRawPath shim 内容无法解析（无目标行）
// 时去重键回退 LookPath 原路径：同名命中仅一条，异路径不去重。
func TestDiscoverChatClis_DedupKeyFallsBackToRawPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("fake shim 依赖 .cmd + PATHEXT 语义，仅 Windows 生效")
	}
	dir := t.TempDir()
	// 两个内容不可解析的 shim：路径不同 → 不互相去重
	for _, shim := range []string{"fakeai-e.cmd", "fakeai-f.cmd"} {
		if err := os.WriteFile(filepath.Join(dir, shim), []byte("@ECHO off\r\nREM no target\r\n"), 0o644); err != nil {
			t.Fatalf("写 shim: %v", err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	withChatCliNames(t, []string{"fakeai-e", "fakeai-f"})

	got := DiscoverChatClis()
	if len(got) != 2 {
		t.Errorf("异路径不可解析 shim 不应去重, got %+v", got)
	}
}

// writeFakeJsShim 在 dir 写一个 js 入口形态的 .cmd shim（node.exe 一并预置），
// shim 内容解析后目标为 node.exe + js 前缀参数（js 路径可各不相同）。
func writeFakeJsShim(t *testing.T, dir, shimName, jsRel string) {
	t.Helper()
	shimBody := "@ECHO off\r\n\"%~dp0\\" + jsRel + "\" %*\r\n"
	if err := os.WriteFile(filepath.Join(dir, shimName), []byte(shimBody), 0o644); err != nil {
		t.Fatalf("写 js shim %s: %v", shimName, err)
	}
	jsPath := filepath.Join(dir, jsRel)
	if err := os.MkdirAll(filepath.Dir(jsPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(jsPath, []byte("// js"), 0o644); err != nil {
		t.Fatalf("写 js: %v", err)
	}
}

// TestDiscoverChatClis_JsEntryDedup 同一 node.exe 启动不同 js 入口的 shim 不应
// 误去重（去重键须含 js 前缀参数）；同 js 入口视为同项去重保留首个。
func TestDiscoverChatClis_JsEntryDedup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("fake shim 依赖 .cmd + PATHEXT 语义，仅 Windows 生效")
	}
	dir := t.TempDir()
	// node.exe 预置于 shim 同目录（resolveFromPath js 形态优先取同目录 node）
	if err := os.WriteFile(filepath.Join(dir, "node.exe"), []byte("PE"), 0o644); err != nil {
		t.Fatalf("写 node.exe: %v", err)
	}
	// fakeai-g / fakeai-h / fakeai-i 均为 js 入口：g 与 h 指向不同 js（不去重），
	// i 与 g 指向同一 js（去重保留声明序靠前的 g）
	writeFakeJsShim(t, dir, "fakeai-g.cmd", `pkg-g\cli.js`)
	writeFakeJsShim(t, dir, "fakeai-h.cmd", `pkg-h\cli.js`)
	writeFakeJsShim(t, dir, "fakeai-i.cmd", `pkg-g\cli.js`)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	withChatCliNames(t, []string{"fakeai-i", "fakeai-g", "fakeai-h"})

	got := DiscoverChatClis()
	if len(got) != 2 {
		t.Fatalf("不同 js 入口不应互相去重、同 js 入口应去重: got %+v", got)
	}
	if got[0].Name != "fakeai-i" {
		// i 与 g 同键：声明序 i 在前保留 i
		t.Errorf("同 js 入口去重应保留声明序首个, got %+v", got)
	}
	if got[1].Name != "fakeai-h" {
		t.Errorf("异 js 入口应保留, got %+v", got)
	}
}
