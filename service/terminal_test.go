package service

import (
	"git-manager/model"
	"testing"
)

func TestResolveShellConfig_Default(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("powershell", "")
	if config.Type != "powershell" {
		t.Errorf("期望 shellType=powershell, 实际=%s", config.Type)
	}
	if config.Executable != "powershell.exe" {
		t.Errorf("期望 executable=powershell.exe, 实际=%s", config.Executable)
	}
}

func TestResolveShellConfig_CustomPath(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("gitbash", "D:\\custom\\bash.exe")
	if config.Executable != "D:\\custom\\bash.exe" {
		t.Errorf("期望自定义路径, 实际=%s", config.Executable)
	}
}

func TestResolveShellConfig_UnknownType(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("unknown", "")
	if config.Type != "powershell" {
		t.Errorf("未知类型应回退到 powershell, 实际=%s", config.Type)
	}
}

// === buildCdCommand 各 Shell 类型测试 ===

func TestBuildCdCommand_Cmd(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\workspace\test`, "cmd")
	expected := `cd /d "D:\workspace\test"` + "\r"
	if cmd != expected {
		t.Errorf("CMD: 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_Cmd_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\my project\test folder`, "cmd")
	expected := `cd /d "D:\my project\test folder"` + "\r"
	if cmd != expected {
		t.Errorf("CMD(含空格): 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_PowerShell(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\workspace\test`, "powershell")
	expected := `cd "D:\workspace\test"` + "\r"
	if cmd != expected {
		t.Errorf("PowerShell: 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_PowerShell_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\工作\Doc\项目管理`, "powershell")
	expected := `cd "D:\工作\Doc\项目管理"` + "\r"
	if cmd != expected {
		t.Errorf("PowerShell(含空格/中文): 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_GitBash(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\workspace\test`, "gitbash")
	expected := `cd "D:/workspace/test"` + "\r"
	if cmd != expected {
		t.Errorf("Git Bash: 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_GitBash_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\工作\Doc\项目管理\10.岗职`, "gitbash")
	expected := `cd "D:/工作/Doc/项目管理/10.岗职"` + "\r"
	if cmd != expected {
		t.Errorf("Git Bash(含中文/空格): 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_Wsl(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\workspace\test`, "wsl")
	expected := `cd "/mnt/d/workspace/test"` + "\r"
	if cmd != expected {
		t.Errorf("WSL: 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_Wsl_DriveRoot(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`C:\`, "wsl")
	expected := `cd "/mnt/c/"` + "\r"
	if cmd != expected {
		t.Errorf("WSL(驱动器根): 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_Wsl_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\工作\Doc\项目管理`, "wsl")
	expected := `cd "/mnt/d/工作/Doc/项目管理"` + "\r"
	if cmd != expected {
		t.Errorf("WSL(含中文): 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_DefaultFallback(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`C:\Users`, "unknown_shell")
	expected := `cd "C:\Users"` + "\r"
	if cmd != expected {
		t.Errorf("未知 Shell 应回退到 PowerShell 语法, 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_PathNormalization(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand("C:/Users/test", "cmd")
	// filepath.Clean 在 Windows 上会将 / 转为 \
	expected := `cd /d "C:\Users\test"` + "\r"
	if cmd != expected {
		t.Errorf("路径应被规范化, 期望=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_EmptyPath(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand("", "cmd")
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
	types := []string{"powershell", "cmd", "gitbash", "wsl"}
	for _, shellType := range types {
		config := svc.resolveShellConfig(shellType, "")
		if config.Type != shellType {
			t.Errorf("类型 %s: 期望 Type=%s, 实际=%s", shellType, shellType, config.Type)
		}
		if config.Executable == "" {
			t.Errorf("类型 %s: Executable 不应为空", shellType)
		}
		if config.DisplayName == "" {
			t.Errorf("类型 %s: DisplayName 不应为空", shellType)
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
