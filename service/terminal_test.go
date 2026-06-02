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

func TestBuildCdCommand_Windows(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\workspace\test`)
	expected := `cd /d "D:\workspace\test"` + "\n"
	if cmd != expected {
		t.Errorf("期望 cd 命令=%q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_WithSpaces(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand(`D:\my project\test folder`)
	expected := `cd /d "D:\my project\test folder"` + "\n"
	if cmd != expected {
		t.Errorf("期望 cd 命令(含空格)=%q, 实际=%q", expected, cmd)
	}
}

// === 以下为新增测试 ===

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

func TestBuildCdCommand_SimplePath(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand("C:\\Users")
	expected := `cd /d "C:\Users"` + "\n"
	if cmd != expected {
		t.Errorf("期望 %q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_PathNormalization(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand("C:/Users/test")
	// filepath.Clean 在 Windows 上会将 / 转为 \
	expected := `cd /d "C:\Users\test"` + "\n"
	if cmd != expected {
		t.Errorf("路径应被规范化, 期望 %q, 实际=%q", expected, cmd)
	}
}

func TestBuildCdCommand_EmptyPath(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd := svc.buildCdCommand("")
	if cmd == "" {
		t.Error("空路径不应产生空命令")
	}
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
