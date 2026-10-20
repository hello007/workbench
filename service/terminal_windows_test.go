//go:build windows

package service

import (
	"testing"
	"time"
)

// 以下断言依赖 Windows 平台的 Shell 类型（powershell/cmd/gitbash）与
// Windows 路径分隔符语义（filepath.Clean 的 / → \ 归一），
// Linux 侧对应断言见 terminal_test.go 的跨平台用例。

// TestResolveShellConfig_Default Windows 默认 Shell 为 PowerShell。
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

// TestResolveShellConfig_CustomPath gitbash 类型解析 + 自定义路径覆盖。
func TestResolveShellConfig_CustomPath(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("gitbash", "D:\\custom\\bash.exe")
	if config.Executable != "D:\\custom\\bash.exe" {
		t.Errorf("期望自定义路径, 实际=%s", config.Executable)
	}
}

// TestResolveShellConfig_UnknownType 未知类型回退到 PowerShell（Windows 默认）。
func TestResolveShellConfig_UnknownType(t *testing.T) {
	svc := NewTerminalService(nil)
	config := svc.resolveShellConfig("unknown", "")
	if config.Type != "powershell" {
		t.Errorf("未知类型应回退到 powershell, 实际=%s", config.Type)
	}
}

// TestBuildCdCommand_PathNormalization cmd 分支依赖 Windows 分隔符语义：
// filepath.Clean 在 Windows 上会将 / 转为 \（Linux 上 \ 为合法文件名字符不参与归一）。
func TestBuildCdCommand_PathNormalization(t *testing.T) {
	svc := NewTerminalService(nil)
	cmd, err := svc.buildCdCommand("C:/Users/test", "cmd")
	if err != nil {
		t.Fatalf("buildCdCommand: %v", err)
	}
	expected := `cd /d "C:\Users\test"` + "\r"
	if cmd != expected {
		t.Errorf("路径应被规范化, 期望=%q, 实际=%q", expected, cmd)
	}
}

// TestTerminalService_MultiSessionClose_WatchExitPerSession 多会话关闭回归（P0 守护）：
// 服务级共享 watch 计数形态下，CloseTerminal(其一) 的收口等待会被另一在册会话的
// watch 计数永久阻塞（双 tab 关其一即死锁）。per-session 退出确认契约：关闭入口
// 有界返回且等待的仅是本会话 watch；他会话的会话表与 watch 存活均不受影响；双会话
// 逐个关闭后 CloseAll 空表幂等绿。真实 conpty fork（形态对齐
// TestTerminalService_CreateTerminal_InvalidShellFallback），powershell 不可用
// 环境 skip；等待全部走 chan 守护，禁 sleep 时序。
func TestTerminalService_MultiSessionClose_WatchExitPerSession(t *testing.T) {
	svc := NewTerminalService(nil)
	idA, err := svc.CreateTerminal("C:\\", "powershell", "", 80, 24)
	if err != nil {
		t.Skipf("powershell 终端创建失败（环境缺 Shell）: %v", err)
	}
	idB, err := svc.CreateTerminal("C:\\", "powershell", "", 80, 24)
	if err != nil {
		svc.CloseTerminal(idA)
		t.Skipf("第二个终端创建失败（环境缺 Shell）: %v", err)
	}
	exitA := watchExitChanOf(t, svc, idA)
	exitB := watchExitChanOf(t, svc, idB)

	// 关 A：CloseTerminal 有界返回——共享计数形态下此处被 B 的 watch 卡死，
	// 守护超时即 P0 回归暴露
	closeTerminalBounded(t, svc, idA)
	assertWatchExitClosed(t, exitA, "A")
	// B 不受影响：会话仍在册、watch 未被波及收口（退出确认 chan 未关闭）
	assertSessionRegistered(t, svc, idB)
	assertWatchExitOpen(t, exitB, "B")

	// 关 B：同样有界返回 + watch 收口确认
	closeTerminalBounded(t, svc, idB)
	assertWatchExitClosed(t, exitB, "B")

	// 空表 CloseAll 幂等绿
	svc.CloseAll()
}

// watchExitChanOf 快照会话 watch 退出确认 chan（mu 内读取；未注册即 Fatal）。
func watchExitChanOf(t *testing.T, svc *TerminalService, sessionID string) chan struct{} {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	ch, ok := svc.watchExits[sessionID]
	if !ok {
		t.Fatalf("会话 %s 的 watch 退出确认 chan 应已注册", sessionID)
	}
	return ch
}

// closeTerminalBounded 有界等待 CloseTerminal 返回（5s 守护 chan，禁 sleep 时序）。
func closeTerminalBounded(t *testing.T, svc *TerminalService, sessionID string) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- svc.CloseTerminal(sessionID) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CloseTerminal(%s) 应成功: %v", sessionID, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("CloseTerminal(%s) 5s 未返回（watch 收口等待被他会话阻塞——P0 回归）", sessionID)
	}
}

// assertWatchExitClosed 断言 watch 退出确认 chan 已关闭（关闭返回 ⇒ watch 已收口）。
func assertWatchExitClosed(t *testing.T, ch chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("会话 %s 的 watch 未退出（退出确认 chan 未关闭）", label)
	}
}

// assertWatchExitOpen 断言 watch 退出确认 chan 未被他会话关闭波及。
func assertWatchExitOpen(t *testing.T, ch chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("会话 %s 的 watch 被他会话关闭误收口（退出确认 chan 提前关闭）", label)
	default:
	}
}

// assertSessionRegistered 断言会话仍在册（他会话关闭不得波及会话表）。
func assertSessionRegistered(t *testing.T, svc *TerminalService, sessionID string) {
	t.Helper()
	svc.mu.Lock()
	_, ok := svc.sessions[sessionID]
	svc.mu.Unlock()
	if !ok {
		t.Fatalf("会话 %s 应仍在册（他会话关闭不得波及会话表）", sessionID)
	}
}
