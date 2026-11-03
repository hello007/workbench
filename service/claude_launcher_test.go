package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		files      map[string]string // shim 目录内预置文件（内容任意，仅测存在性）
		wantExeExt string            // 期望可执行文件扩展名（"" = 回退 claude）
		wantPrefix int               // 期望前缀参数个数（js 形态 = 1）
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
			name:     "js 入口 shim + 同目录 node.exe",
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

// TestResolveChatCommand 表驱动：AI 对话启动命令按 launcher 配置的四分支解析
// （空 = 默认 claude 透传 / 纯命令名 LookPath+shim 解析 / 绝对路径 shim 解析或
// 直用 / 未命中中文报错含配置值）。shim 分支仅 Windows 生效。
func TestResolveChatCommand(t *testing.T) {
	cases := []struct {
		name       string
		launcher   string
		shimBody   string            // 非 Windows 跳过的 shim 分支预置内容
		files      map[string]string // shim 目录内预置文件
		wantName   string            // 期望可执行（"" = 断言报错）
		wantPrefix int               // 期望前缀参数个数（js 形态 = 1）
		wantErrHas string            // 错误信息须包含的片段（含 launcher 值）
	}{
		{
			name:       "空配置透传默认 claude 解析",
			launcher:   "",
			wantName:   "stubbed-default-claude",
			wantErrHas: "",
		},
		{
			name:     "纯命令名 PATH 命中经 shim 解析出真实 exe",
			launcher: "mychatcli",
			shimBody: "@ECHO off\r\n\"%dp0%\\node_modules\\some-pkg\\bin\\mychatcli.exe\"   %*\r\n",
			files:    map[string]string{`node_modules\some-pkg\bin\mychatcli.exe`: "PE"},
			wantName: "mychatcli.exe",
		},
		{
			name:     "绝对路径 .cmd 同样经 shim 解析",
			launcher: `SHIMDIR\mychatcli.cmd`,
			shimBody: "@ECHO off\r\n\"%~dp0\\node_modules\\some-pkg\\bin\\mychatcli.exe\"   %*\r\n",
			files:    map[string]string{`node_modules\some-pkg\bin\mychatcli.exe`: "PE"},
			wantName: "mychatcli.exe",
		},
		{
			name:     "绝对路径非 .cmd 直用",
			launcher: `SHIMDIR\plain.exe`,
			files:    map[string]string{"plain.exe": "PE"},
			wantName: "plain.exe",
		},
		{
			name:       "绝对路径不存在报中文错误含配置值",
			launcher:   `SHIMDIR\ghost-path-tool.exe`,
			wantErrHas: `ghost-path-tool.exe`,
		},
		{
			name:       "纯命令名未命中 PATH 报中文错误含配置值",
			launcher:   "no-such-ai-cli-xyz",
			wantErrHas: "no-such-ai-cli-xyz",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 空 launcher 分支透传默认解析器：stub 断言透传与错误隔离
			orig := claudeCommandResolver
			claudeCommandResolver = func() (string, []string) { return "stubbed-default-claude", nil }
			t.Cleanup(func() { claudeCommandResolver = orig })

			dir := t.TempDir()
			launcher := tc.launcher
			if tc.shimBody != "" || strings.Contains(launcher, "SHIMDIR") {
				if runtime.GOOS != "windows" {
					t.Skip("shim/绝对路径分支仅 Windows 生效")
				}
			}
			if tc.shimBody != "" {
				shim := filepath.Join(dir, "mychatcli.cmd")
				if err := os.WriteFile(shim, []byte(tc.shimBody), 0o644); err != nil {
					t.Fatalf("写 shim: %v", err)
				}
				// LookPath 依赖 PATH，临时目录塞最前
				t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
			launcher = strings.ReplaceAll(launcher, "SHIMDIR", dir)

			name, prefix, err := ResolveChatCommand(launcher)
			if tc.wantErrHas != "" {
				if err == nil {
					t.Fatalf("应报错, got name=%q prefix=%v", name, prefix)
				}
				if !strings.Contains(err.Error(), tc.wantErrHas) {
					t.Errorf("错误应含配置值 %q: %v", tc.wantErrHas, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错: %v", err)
			}
			if !strings.HasSuffix(name, tc.wantName) {
				t.Errorf("可执行名: got %q want 后缀 %q", name, tc.wantName)
			}
			if len(prefix) != tc.wantPrefix {
				t.Errorf("前缀参数个数: got %d want %d", len(prefix), tc.wantPrefix)
			}
		})
	}
}

// TestResolveChatCommand_JS 命令名 shim 的 js 入口形态：node + js 路径前缀参数。
func TestResolveChatCommand_JS(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("shim 解析仅 Windows 生效")
	}
	orig := claudeCommandResolver
	claudeCommandResolver = func() (string, []string) { return "stubbed-default-claude", nil }
	t.Cleanup(func() { claudeCommandResolver = orig })

	dir := t.TempDir()
	shimBody := "@ECHO off\r\n\"%~dp0\\node_modules\\some-pkg\\cli.js\" %*\r\n"
	if err := os.WriteFile(filepath.Join(dir, "jsthim.cmd"), []byte(shimBody), 0o644); err != nil {
		t.Fatalf("写 shim: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "some-pkg"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "node_modules", "some-pkg", "cli.js"), []byte("// js"), 0o644); err != nil {
		t.Fatalf("写 js: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	name, prefix, err := ResolveChatCommand("jsthim")
	if err != nil {
		t.Fatalf("ResolveChatCommand: %v", err)
	}
	if filepath.Ext(name) != ".exe" || len(prefix) != 1 || !strings.HasSuffix(prefix[0], "cli.js") {
		t.Errorf("js 形态应为 node+js 前缀: name=%q prefix=%v", name, prefix)
	}
}
