package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestResolveClaudeCommand_NonCmdFallback 非 .cmd 场景一律回退 "claude"：
// shim 不存在（PATH 无 claude）时也不报错——解析失败回退旧行为是硬性约定。
func TestResolveClaudeCommand_NonCmdFallback(t *testing.T) {
	orig := claudeCommandResolver
	defer func() { claudeCommandResolver = orig }()
	claudeCommandResolver = resolveClaudeCommand

	if runtime.GOOS != "windows" {
		t.Skip("仅 Windows 分支行为")
	}
	name, prefix := resolveClaudeCommand()
	if name == "" {
		t.Fatal("解析结果不得为空")
	}
	if len(prefix) != 0 && filepath.Ext(name) != ".exe" {
		// node+js 形态合法；其余形态不应带前缀参数
		t.Errorf("异常前缀参数: name=%q prefix=%v", name, prefix)
	}
}

// TestResolveClaudeCommand_ShimParse 表驱动：npm shim 内容解析出真实启动目标。
func TestResolveClaudeCommand_ShimParse(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("shim 解析仅 Windows 生效")
	}
	cases := []struct {
		name       string
		shimBody   string
		files      map[string]string   // shim 目录内预置文件（内容任意，仅测存在性）
		wantExeExt string              // 期望可执行文件扩展名（"" = 回退 claude）
		wantPrefix int                 // 期望前缀参数个数（js 形态 = 1）
	}{
		{
			name: "原生 exe shim（claude-code 当前形态）",
			shimBody: "@ECHO off\r\nGOTO start\r\n:find_dp0\r\nSET dp0=%~dp0\r\nEXIT /b\r\n:start\r\nSETLOCAL\r\nCALL :find_dp0\r\n" +
				"\"%dp0%\\node_modules\\@anthropic-ai\\claude-code\\bin\\claude.exe\"   %*\r\n",
			files:      map[string]string{`node_modules\@anthropic-ai\claude-code\bin\claude.exe`: "PE"},
			wantExeExt: ".exe",
			wantPrefix: 0,
		},
		{
			name: "js 入口 shim + 同目录 node.exe",
			shimBody: "@ECHO off\r\n\"%~dp0\\node_modules\\some-pkg\\cli.js\" %*\r\n",
			files: map[string]string{
				`node_modules\some-pkg\cli.js`: "// js",
				"node.exe":                     "PE",
			},
			wantExeExt: ".exe", // node.exe
			wantPrefix: 1,
		},
		{
			name:       "目标文件不存在回退 claude",
			shimBody:   "@ECHO off\r\n\"%dp0%\\node_modules\\ghost\\bin\\claude.exe\" %*\r\n",
			wantExeExt: "",
			wantPrefix: 0,
		},
		{
			name:       "shim 无目标行回退 claude",
			shimBody:   "@ECHO off\r\nREM nothing here\r\n",
			wantExeExt: "",
			wantPrefix: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			shim := filepath.Join(dir, "claude.cmd")
			if err := os.WriteFile(shim, []byte(tc.shimBody), 0o644); err != nil {
				t.Fatalf("写 shim: %v", err)
			}
			for rel, content := range tc.files {
				full := filepath.Join(dir, rel)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
					t.Fatalf("写 %s: %v", rel, err)
				}
			}
			// LookPath 依赖 PATH，临时目录塞最前（TempDir 无扩展名问题，Windows 会补 PATHEXT 探测）
			origPath := os.Getenv("PATH")
			t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

			name, prefix := resolveClaudeCommand()
			if tc.wantExeExt == "" {
				if name != "claude" || len(prefix) != 0 {
					t.Errorf("应回退 claude, got name=%q prefix=%v", name, prefix)
				}
				return
			}
			if filepath.Ext(name) != tc.wantExeExt {
				t.Errorf("可执行扩展名: got %q want %q", name, tc.wantExeExt)
			}
			if len(prefix) != tc.wantPrefix {
				t.Errorf("前缀参数个数: got %d want %d (prefix=%v)", len(prefix), tc.wantPrefix, prefix)
			}
		})
	}
}
