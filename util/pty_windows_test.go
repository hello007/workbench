//go:build windows

package util

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestQuoteArg_PlainArg(t *testing.T) {
	result := quoteArg("powershell.exe")
	if result != "powershell.exe" {
		t.Errorf("无空格参数不应加引号, 实际=%s", result)
	}
}

func TestQuoteArg_ArgWithSpaces(t *testing.T) {
	result := quoteArg(`C:\Program Files\Git\bin\bash.exe`)
	if result != `"C:\Program Files\Git\bin\bash.exe"` {
		t.Errorf("含空格参数应加引号, 实际=%s", result)
	}
}

func TestQuoteArg_EmptyArg(t *testing.T) {
	result := quoteArg("")
	if result != `""` {
		t.Errorf("空参数应加引号, 实际=%s", result)
	}
}

func TestQuoteArg_ArgWithQuotes(t *testing.T) {
	result := quoteArg(`say "hello"`)
	if result != `"say \"hello\""` {
		t.Errorf("含双引号参数应转义, 实际=%s", result)
	}
}

func TestBuildCommandLine_NoArgs(t *testing.T) {
	result := buildCommandLine("cmd.exe", nil)
	if result != "cmd.exe" {
		t.Errorf("无参数: 期望 cmd.exe, 实际=%s", result)
	}
}

func TestBuildCommandLine_WithArgs(t *testing.T) {
	result := buildCommandLine("bash.exe", []string{"-i", "-l"})
	if result != `bash.exe -i -l` {
		t.Errorf("带参数: 期望 bash.exe -i -l, 实际=%s", result)
	}
}

func TestBuildCommandLine_PathWithSpaces(t *testing.T) {
	result := buildCommandLine(`C:\Program Files\Git\bin\bash.exe`, []string{"-i"})
	if result != `"C:\Program Files\Git\bin\bash.exe" -i` {
		t.Errorf("路径含空格+参数: 实际=%s", result)
	}
}

func TestNewPtyProcess_InvalidCommand(t *testing.T) {
	_, err := NewPtyProcess("nonexistent_command_xyz.exe", nil, "", 80, 24)
	if err == nil {
		t.Error("无效命令应返回错误")
	}
}

func TestPtyProcess_Close_Nil(t *testing.T) {
	p := &PtyProcess{}
	err := p.Close()
	if err != nil {
		t.Errorf("关闭 nil ConPTY 不应返回错误, 实际=%v", err)
	}
	if p.job != 0 {
		t.Errorf("零值 PtyProcess 关闭后 job 应保持 0, 实际=%v", p.job)
	}
}

func TestPtyProcess_IsProcessRunning_ZeroPid(t *testing.T) {
	p := &PtyProcess{pid: 0}
	if p.IsProcessRunning() {
		t.Error("pid=0 不应判定为运行中")
	}
}

func TestPtyProcess_GetCmd_AlwaysNil(t *testing.T) {
	p := &PtyProcess{}
	if p.GetCmd() != nil {
		t.Error("Windows 实现的 GetCmd 应始终返回 nil")
	}
}

// TestPtyProcess_Close_KillsProcessTree 端到端回归：会话内派生的长任务
// （ping -t）必须在 Close 后随 Job 句柄关闭被回收。
//
// 判定链路：powershell 子进程启动 ping 并把 PID 写入临时文件 → 测试读到 PID
// 并确认存活 → Close → 轮询 ping 与 shell 自身均退出。轮询 + 超时，不用固定 sleep。
// 本用例覆盖真实关闭链路（ClosePseudoConsole → 关 Job 句柄），属验收用例；
// 杀树归因是否真的来自 Job 由 TestPtyProcess_JobHandleClose_KillsProcessTree 单独锁定。
func TestPtyProcess_Close_KillsProcessTree(t *testing.T) {
	proc, childPid := spawnPingUnderPty(t)

	if err := proc.Close(); err != nil {
		t.Fatalf("关闭 PTY 进程失败: %v", err)
	}

	if !waitPIDExit(childPid, 5*time.Second) {
		t.Errorf("关闭终端后派生子进程 %d 未在 5 秒内退出", childPid)
	}
	if !waitPIDExit(proc.pid, 5*time.Second) {
		t.Errorf("关闭终端后 shell 进程 %d 未在 5 秒内退出", proc.pid)
	}
}

// TestPtyProcess_JobHandleClose_KillsProcessTree 杀树归因用例：只关闭 Job 句柄、
// 保持伪控制台打开，排除 ClosePseudoConsole 投递控制台关闭事件这一干扰源。
// 若本用例通过，说明进程树是被 Job 内核回收，而非控制台关闭后的自行退出。
func TestPtyProcess_JobHandleClose_KillsProcessTree(t *testing.T) {
	proc, childPid := spawnPingUnderPty(t)

	job := proc.job
	if job == 0 {
		t.Fatal("Job 对象未绑定（job=0），无法验证杀树归因")
	}
	// 摘出句柄并置零：Cleanup 中的 Close 不得重复关闭同一句柄
	proc.job = 0
	if err := windows.CloseHandle(job); err != nil {
		t.Fatalf("关闭 Job 句柄失败: %v", err)
	}

	if !waitPIDExit(childPid, 5*time.Second) {
		t.Errorf("仅关闭 Job 句柄后派生子进程 %d 未在 5 秒内退出", childPid)
	}
	if !waitPIDExit(proc.pid, 5*time.Second) {
		t.Errorf("仅关闭 Job 句柄后 shell 进程 %d 未在 5 秒内退出", proc.pid)
	}
}

// spawnPingUnderPty 创建 powershell 会话，会话内以 `ping -t` 拉起长任务并回写
// 子进程 PID。返回已绑定 Job 的 PTY 进程与子进程 PID；关闭与残留清理均已注册
// 到 t.Cleanup（含 Job 未绑定的失败路径兜底，不污染后续用例）。
func spawnPingUnderPty(t *testing.T) (*PtyProcess, int) {
	t.Helper()
	requirePowershell(t)
	if _, err := exec.LookPath("ping.exe"); err != nil {
		t.Skip("未找到 ping.exe，跳过 Windows 进程树回收测试")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "spawn-child.ps1")
	pidFile := filepath.Join(dir, "child.pid")

	// 脚本只做三件事：拉起 ping -t、回写子进程 PID、等待其退出（保持父链存活）。
	// -NoNewWindow 保证 ping 是 powershell 的直接子进程且与会话共用控制台，
	// 避免 ShellExecute 另起窗口导致归属不确定。
	scriptBody := "param([string]$PidFile)\n" +
		"$p = Start-Process ping.exe -ArgumentList '-t','127.0.0.1' -NoNewWindow -PassThru\n" +
		"Set-Content -Path $PidFile -Value $p.Id -Encoding Ascii\n" +
		"$p.WaitForExit()\n"
	if err := os.WriteFile(script, []byte(scriptBody), 0o600); err != nil {
		t.Fatalf("写入临时脚本失败: %v", err)
	}

	proc, err := NewPtyProcess(
		"powershell.exe",
		[]string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-PidFile", pidFile},
		"", 80, 24,
	)
	if err != nil {
		t.Fatalf("创建 PTY 进程失败: %v", err)
	}
	t.Cleanup(func() {
		_ = proc.Close()
		// Job 未绑定或异常路径下 Close 无法回收子进程，按 PID 文件兜底清理
		killPIDFromFile(pidFile)
	})

	if proc.job == 0 {
		t.Fatal("Job 对象未绑定（job=0），进程树回收不可用")
	}

	childPid, err := waitChildPID(pidFile, 30*time.Second)
	if err != nil {
		t.Fatalf("等待子进程 PID 写入失败: %v", err)
	}
	t.Cleanup(func() { forceKillPID(childPid) })

	if !isPIDRunning(childPid) {
		t.Fatalf("子进程 %d 在 Close 前已退出，无法验证回收", childPid)
	}
	return proc, childPid
}

// TestPtyProcess_Close_Idempotent 重复 Close 不得 panic、不得重复关闭句柄，
// 且两次返回同一结果。
func TestPtyProcess_Close_Idempotent(t *testing.T) {
	requirePowershell(t)

	proc, err := NewPtyProcess(
		"powershell.exe",
		[]string{"-NoProfile", "-Command", "Start-Sleep -Seconds 30"},
		"", 80, 24,
	)
	if err != nil {
		t.Fatalf("创建 PTY 进程失败: %v", err)
	}
	t.Cleanup(func() { _ = proc.Close() })

	shellPid := proc.pid

	if err := proc.Close(); err != nil {
		t.Fatalf("首次 Close 失败: %v", err)
	}
	if err := proc.Close(); err != nil {
		t.Errorf("重复 Close 应幂等返回首次结果 nil, 实际=%v", err)
	}
	if proc.job != 0 {
		t.Errorf("Close 后 Job 句柄应置零, 实际=%v", proc.job)
	}
	if !waitPIDExit(shellPid, 5*time.Second) {
		t.Errorf("Close 后 shell 进程 %d 未在 5 秒内退出", shellPid)
	}
}

// TestCreateKillOnCloseJob_InvalidPID 覆盖 Job 绑定失败的入口守卫：
// 非法 PID 直接报错、不泄漏句柄。
func TestCreateKillOnCloseJob_InvalidPID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		job, err := createKillOnCloseJob(pid)
		if err == nil {
			if job != 0 {
				windows.CloseHandle(job)
			}
			t.Errorf("pid=%d 应返回错误", pid)
			continue
		}
		if job != 0 {
			t.Errorf("pid=%d 失败时应返回 0 句柄, 实际=%v", pid, job)
		}
	}
}

// TestBindJobOrWarn_FailureDegrades 降级路径：Job 创建失败时不得 panic、
// 不得阻断会话创建，proc.job 保持 0（关闭时不再保证回收子进程）。
// 注入 create 参数覆盖失败分支，不依赖真实进程或真实 API 失败。
func TestBindJobOrWarn_FailureDegrades(t *testing.T) {
	proc := &PtyProcess{pid: 4321}
	createErr := fmt.Errorf("模拟 Job 创建失败")

	var gotPid int
	bindJobOrWarn(proc, func(pid int) (windows.Handle, error) {
		gotPid = pid
		return 0, createErr
	})

	if gotPid != proc.pid {
		t.Errorf("create 应收到会话 pid=%d, 实际=%d", proc.pid, gotPid)
	}
	if proc.job != 0 {
		t.Errorf("绑定失败时 job 应保持 0, 实际=%v", proc.job)
	}
}

// TestBindJobOrWarn_SuccessAssignsJob 成功路径：create 返回的句柄写入 proc.job，
// 随后由 PtyProcess.Close 关闭（测试内自行关闭句柄避免泄漏）。
// fake 句柄用真实 CreateJobObject 产出，不对任意数值做 CloseHandle（可能误关真实资源）。
func TestBindJobOrWarn_SuccessAssignsJob(t *testing.T) {
	want, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatalf("创建测试用 Job 对象失败: %v", err)
	}

	proc := &PtyProcess{pid: 4321}
	bindJobOrWarn(proc, func(int) (windows.Handle, error) {
		return want, nil
	})

	if proc.job != want {
		windows.CloseHandle(want)
		t.Fatalf("绑定成功后 job 应为 %v, 实际=%v", want, proc.job)
	}
	// 句柄归 proc 所有：置零后自行关闭，避免重复关闭
	proc.job = 0
	if err := windows.CloseHandle(want); err != nil {
		t.Errorf("关闭测试用 Job 句柄失败: %v", err)
	}
}

// requirePowershell 真机回归的前置依赖，缺失时跳过而非失败。
func requirePowershell(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		t.Skip("未找到 powershell.exe，跳过 Windows 进程树回收测试")
	}
}

// killPIDFromFile 读取脚本回写的 PID 文件并强杀对应进程，用于 Job 未绑定等
// 异常路径的残留兜底；文件不存在或内容非法时静默跳过。
func killPIDFromFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil {
		forceKillPID(pid)
	}
}

// isPIDRunning 按 PID 判断进程是否存活（259 = STILL_ACTIVE）
func isPIDRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return false
	}
	return exitCode == 259
}

// waitPIDExit 轮询等待进程退出，超时返回 false
func waitPIDExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !isPIDRunning(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitChildPID 轮询等待脚本回写子进程 PID，超时返回错误
func waitChildPID(path string, timeout time.Duration) (int, error) {
	deadline := time.Now().Add(timeout)
	for {
		if data, err := os.ReadFile(path); err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil && pid > 0 {
				return pid, nil
			}
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("等待子进程 PID 文件写入超时: %s", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// forceKillPID 清理用例失败时可能残留的进程，避免污染后续测试
func forceKillPID(pid int) {
	if pid <= 0 {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_ = windows.TerminateProcess(h, 1)
}
