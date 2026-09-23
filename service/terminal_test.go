package service

import (
	"runtime"
	"strings"
	"testing"

	"workbench/model"
)

// TestResolveShellConfig_KnownType 解析当前平台默认 Shell（列表首项），
// Windows 断言见 terminal_windows_test.go（powershell/gitbash 专属用例）。
func TestResolveShellConfig_KnownType(t *testing.T) {
	svc := NewTerminalService(nil)
	first := model.GetShellConfigs()[0]
	config := svc.resolveShellConfig(first.Type, "")
	if config.Type != first.Type {
		t.Errorf("期望 shellType=%s, 实际=%s", first.Type, config.Type)
	}
	if config.Executable != first.Executable {
		t.Errorf("期望 executable=%s, 实际=%s", first.Executable, config.Executable)
	}
}

// === buildCdCommand 各 Shell 类型测试 ===

// mustBuildCdCommand 测试辅助：buildCdCommand 自 G1 起返回 (string, error)，
// 各 Shell 类型正常用例错误即 Fatal，拒绝分支（换行路径）单独建用例断言错误。
func mustBuildCdCommand(t *testing.T, svc *TerminalService, dir, shellType string) string {
	t.Helper()
	cmd, err := svc.buildCdCommand(dir, shellType)
	if err != nil {
		t.Fatalf("buildCdCommand(%q, %q) 不应报错: %v", dir, shellType, err)
	}
	return cmd
}

func TestBuildCdCommand_Cmd(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, `D:\workspace\test`, "cmd")
	expected := `cd /d "D:\workspace\test"` + "\r"
	if cmd != expected {
		t.Errorf("CMD: 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_Cmd_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, `D:\my project\test folder`, "cmd")
	expected := `cd /d "D:\my project\test folder"` + "\r"
	if cmd != expected {
		t.Errorf("CMD(含空格): 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_PowerShell(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, `D:\workspace\test`, "powershell")
	expected := `cd "D:\workspace\test"` + "\r"
	if cmd != expected {
		t.Errorf("PowerShell: 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_PowerShell_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, `D:\工作\Doc\项目管理`, "powershell")
	expected := `cd "D:\工作\Doc\项目管理"` + "\r"
	if cmd != expected {
		t.Errorf("PowerShell(含空格/中文): 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_GitBash(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, `D:\workspace\test`, "gitbash")
	expected := `cd "D:/workspace/test"` + "\r"
	if cmd != expected {
		t.Errorf("Git Bash: 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_GitBash_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, `D:\工作\Doc\项目管理\10.岗职`, "gitbash")
	expected := `cd "D:/工作/Doc/项目管理/10.岗职"` + "\r"
	if cmd != expected {
		t.Errorf("Git Bash(含中文/空格): 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_Wsl(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, `D:\workspace\test`, "wsl")
	expected := `cd "/mnt/d/workspace/test"` + "\r"
	if cmd != expected {
		t.Errorf("WSL: 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_Wsl_DriveRoot(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, `C:\`, "wsl")
	expected := `cd "/mnt/c/"` + "\r"
	if cmd != expected {
		t.Errorf("WSL(驱动器根): 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_Wsl_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, `D:\工作\Doc\项目管理`, "wsl")
	expected := `cd "/mnt/d/工作/Doc/项目管理"` + "\r"
	if cmd != expected {
		t.Errorf("WSL(含中文): 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_DefaultFallback(t *testing.T) {
	svc := NewTerminalService(nil)
	if runtime.GOOS == "windows" {
		// Windows 上未知 Shell 回退 PowerShell 语法
		cmd := mustBuildCdCommand(t, svc, `C:\Users`, "unknown_shell")
		expected := `cd "C:\Users"` + "\r"
		if cmd != expected {
			t.Errorf("未知 Shell 应回退到 PowerShell 语法, 期望=%q, 实际=%q", expected, cmd)
		}
		return
	}
	// 非 Windows 平台未知 Shell 兜底按 POSIX 语法输出，避免向 Unix shell 发出 Windows 语法
	cmd := mustBuildCdCommand(t, svc, "/tmp/work", "unknown_shell")
	expected := "cd -- '/tmp/work'" + "\r"
	if cmd != expected {
		t.Errorf("非 Windows 平台未知 Shell 应回退到 POSIX 语法, 期望=%q, 实际=%q", expected, cmd)
	}
}

// TestBuildCdCommand_PosixBash POSIX shell（bash）输出 cd -- '<path>' 语法（跨平台纯函数断言）。
func TestBuildCdCommand_PosixBash(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, "/tmp/work dir", "bash")
	expected := "cd -- '/tmp/work dir'" + "\r"
	if cmd != expected {
		t.Errorf("POSIX(bash): 期望=%q, 实际=%q", expected, cmd)
	}
}

// TestBuildCdCommand_PosixQuoteEscape 路径内单引号按 POSIX 规则转义为 '\''。
func TestBuildCdCommand_PosixQuoteEscape(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, `/tmp/it's`, "zsh")
	expected := "cd -- '/tmp/it'\\''s'" + "\r"
	if cmd != expected {
		t.Errorf("POSIX(单引号转义): 期望=%q, 实际=%q", expected, cmd)
	}
}

// TestBuildPosixCdCommand_TrailingCR POSIX 与 Windows 分支一致保留 \r 结尾（PTY 回车执行语义）。
func TestBuildPosixCdCommand_TrailingCR(t *testing.T) {
	cmd, err := buildPosixCdCommand("/tmp/work", "bash")
	if err != nil {
		t.Fatalf("buildPosixCdCommand: %v", err)
	}
	if !strings.HasSuffix(cmd, "\r") {
		t.Errorf("POSIX cd 命令应以 \\r 结尾, 实际=%q", cmd)
	}
}

// TestBuildPosixCdCommand_RejectsNewline 路径含 \n/\r 时拒绝生成 cd 命令（G1）：
// Linux 文件名合法含换行，单引号包裹无法阻止 PTY 按行分割，cd 截断后剩余片段会被
// shell 当作独立命令执行（注入面），故返回错误由 ChangeDir 透传前端提示。
func TestBuildPosixCdCommand_RejectsNewline(t *testing.T) {
	for _, dir := range []string{"/tmp/a\nb", "/tmp/a\rb", "/tmp/a\r\nb", "\n/tmp"} {
		if _, err := buildPosixCdCommand(dir, "bash"); err == nil {
			t.Errorf("路径含换行应拒绝生成命令, dir=%q", dir)
		}
	}
	// 正常路径不误伤
	if _, err := buildPosixCdCommand("/tmp/work", "bash"); err != nil {
		t.Errorf("正常路径不应报错: %v", err)
	}
}

// TestBuildCdCommand_PosixNewlineRejected buildCdCommand 链路层面拒绝（G1）：
// POSIX 分支透传换行错误；Windows 分支无对应注入面（Windows 文件名不允许换行）不受影响。
func TestBuildCdCommand_PosixNewlineRejected(t *testing.T) {
	svc := NewTerminalService(nil)
	if _, err := svc.buildCdCommand("/tmp/a\nb", "zsh"); err == nil {
		t.Error("POSIX 分支路径含换行应返回错误")
	}
	if _, err := svc.buildCdCommand("/tmp/a\nb", "fish"); err == nil {
		t.Error("fish 分支路径含换行同样应返回错误")
	}
}

// TestBuildPosixCdCommand_FishOmitsDashDash fish 防御省略 `--`（G2，查证结论见
// buildPosixCdCommand 注释），bash/zsh 等其余 POSIX shell 保持 cd -- '<path>'。
func TestBuildPosixCdCommand_FishOmitsDashDash(t *testing.T) {
	fish, err := buildPosixCdCommand("/tmp/work dir", "fish")
	if err != nil {
		t.Fatalf("buildPosixCdCommand(fish): %v", err)
	}
	if fish != "cd '/tmp/work dir'\r" {
		t.Errorf("fish 应输出 cd '<path>'（无 --）, 实际=%q", fish)
	}
	bash, err := buildPosixCdCommand("/tmp/work dir", "bash")
	if err != nil {
		t.Fatalf("buildPosixCdCommand(bash): %v", err)
	}
	if bash != "cd -- '/tmp/work dir'\r" {
		t.Errorf("bash 应保持 cd -- '<path>', 实际=%q", bash)
	}
}

func TestBuildCdCommand_EmptyPath(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := mustBuildCdCommand(t, svc, "", "cmd")
	if cmd == "" {
		t.Error("空路径不应产生空命令")
	}
}

// === toWslPath 辅助函数测试 ===

func TestToWslPath_DrivePath(t *testing.T) {
	result := toWslPath(`D:\workspace\test`)
	expected := "/mnt/d/workspace/test"
	if result != expected {
		t.Errorf("期望=%q, 实际=%q", expected, result)
	}
}

func TestToWslPath_DriveRoot(t *testing.T) {
	result := toWslPath(`C:\`)
	expected := "/mnt/c/"
	if result != expected {
		t.Errorf("期望=%q, 实际=%q", expected, result)
	}
}

func TestToWslPath_LowercaseDrive(t *testing.T) {
	result := toWslPath(`D:\path`)
	expected := "/mnt/d/path"
	if result != expected {
		t.Errorf("驱动器号应转小写, 期望=%q, 实际=%q", expected, result)
	}
}

func TestToWslPath_NoDrive(t *testing.T) {
	result := toWslPath(`relative\path`)
	expected := "relative/path"
	if result != expected {
		t.Errorf("无驱动器号应只转斜杠, 期望=%q, 实际=%q", expected, result)
	}
}

// === 以下为服务层原有测试 ===

func TestNewTerminalService(t *testing.T) {
	svc := NewTerminalService(nil)
	if svc == nil {
		t.Fatal("NewTerminalService 返回 nil")
	}
	if svc.sessions == nil {
		t.Error("sessions map 未初始化")
	}
}

func TestTerminalService_CloseTerminal_NotExists(t *testing.T) {
	svc := NewTerminalService(nil)
	err := svc.CloseTerminal("nonexistent-id")
	if err == nil {
		t.Error("关闭不存在的会话应返回错误")
	}
}

func TestTerminalService_WriteInput_NotExists(t *testing.T) {
	svc := NewTerminalService(nil)
	err := svc.WriteInput("nonexistent-id", "test")
	if err == nil {
		t.Error("向不存在的会话写入应返回错误")
	}
}

func TestTerminalService_ChangeDir_NotExists(t *testing.T) {
	svc := NewTerminalService(nil)
	err := svc.ChangeDir("nonexistent-id", "C:\\")
	if err == nil {
		t.Error("切换不存在的会话目录应返回错误")
	}
}

func TestTerminalService_Resize_NotExists(t *testing.T) {
	svc := NewTerminalService(nil)
	err := svc.Resize("nonexistent-id", 80, 24)
	if err == nil {
		t.Error("调整不存在的会话大小应返回错误")
	}
}

func TestTerminalService_CloseAll_Empty(t *testing.T) {
	svc := NewTerminalService(nil)
	svc.CloseAll() // 不应 panic
}

func TestTerminalService_CreateTerminal_InvalidShellFallback(t *testing.T) {
	svc := NewTerminalService(nil)
	// 未知 Shell 类型会回退到 powershell，在 Windows 上 powershell.exe 可用
	// 因此终端创建应成功，而不是报错
	sessionID, err := svc.CreateTerminal("C:\\", "nonexistent_shell", "", 80, 24)
	if err != nil {
		t.Logf("未知 Shell 类型创建终端失败（可接受）: %v", err)
		return
	}
	// 创建成功则验证会话有效并清理
	if sessionID == "" {
		t.Error("创建成功但 sessionID 为空")
	}
	svc.CloseTerminal(sessionID)
}

func TestResolveShellConfig_AllTypes(t *testing.T) {
	svc := NewTerminalService(nil)
	// 遍历当前平台 Shell 列表：每个类型都应解析回自身且字段完整
	for _, shell := range model.GetShellConfigs() {
		config := svc.resolveShellConfig(shell.Type, "")
		if config.Type != shell.Type {
			t.Errorf("类型 %s: 期望 Type=%s, 实际=%s", shell.Type, shell.Type, config.Type)
		}
		if config.Executable == "" {
			t.Errorf("类型 %s: Executable 不应为空", shell.Type)
		}
		if config.DisplayName == "" {
			t.Errorf("类型 %s: DisplayName 不应为空", shell.Type)
		}
	}
}

func TestPtyStore_StoreAndGet(t *testing.T) {
	session := &model.TerminalSession{ID: "test-store-session"}
	// 未存储的会话应返回 nil
	proc := getPtyProcess(session)
	if proc != nil {
		t.Error("未存储的会话应返回 nil PtyProcess")
	}
}

func TestPtyStore_GetAfterStore(t *testing.T) {
	session := &model.TerminalSession{ID: "test-store-get-session"}
	// 存储一个非 PtyProcess 类型（测试类型断言失败场景）
	ptyStore.Store(session.ID, "not-a-pty-process")
	proc := getPtyProcess(session)
	if proc != nil {
		t.Error("存储非 PtyProcess 类型应返回 nil")
	}
	// 清理
	ptyStore.Delete(session.ID)
}
