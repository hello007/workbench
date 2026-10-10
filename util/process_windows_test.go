//go:build windows

package util

import (
	"context"
	"errors"
	"os"
	"testing"

	"workbench/model"
)

// fakeProcessRunner 构造注入用的命令执行器：按命令名返回预设输出，并记录调用参数。
// 真跑 netstat/tasklist/taskkill 会依赖本机常驻进程与端口（实测 36115 常被本应用
// 自身占用），断言随环境漂移必然 flaky（docs/spec/test-stability.md），故全部用 fake。
func fakeProcessRunner(t *testing.T, outputs map[string]string, failWith error) (processCommandRunner, *[]string) {
	t.Helper()
	var calls []string
	return func(ctx context.Context, name string, args ...string) (string, error) {
		calls = append(calls, name)
		if failWith != nil {
			return "", failWith
		}
		return outputs[name], nil
	}, &calls
}

func TestListProcessesWithRunner_FakeCommands(t *testing.T) {
	run, calls := fakeProcessRunner(t, map[string]string{
		"netstat":  netstatFixture,
		"tasklist": tasklistFixture,
	}, nil)
	infos, err := listProcessesWithRunner(context.Background(), run)
	if err != nil {
		t.Fatalf("枚举失败: %v", err)
	}
	if len(*calls) != 2 {
		t.Errorf("须依次调用 netstat 与 tasklist: got %v", *calls)
	}
	var target *model.ProcessInfo
	for i := range infos {
		if infos[i].Pid == 44336 {
			target = &infos[i]
		}
	}
	if target == nil {
		t.Fatalf("PID 44336 未出现在结果中: %+v", infos)
	}
	if target.Name != "node.exe" || target.MemoryKB != 105464 {
		t.Errorf("进程名与内存须来自 tasklist: got %+v", target)
	}
	if len(target.Ports) != 2 || target.Ports[0].Port != 36115 {
		t.Errorf("端口须来自 netstat: got %+v", target.Ports)
	}
}

// TestListProcessesWithRunner_TimeoutBeforeExitCode 超时必须优先于命令失败判定：
// Windows 下子进程被杀 exit 1，若先判退出码会把超时误判为普通命令失败（丢失重试引导）。
func TestListProcessesWithRunner_TimeoutBeforeExitCode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	<-ctx.Done() // 使 ctx.Err() 确定为 DeadlineExceeded
	run, _ := fakeProcessRunner(t, nil, errors.New("exit status 1"))

	_, err := listProcessesWithRunner(ctx, run)
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("须返回 AppError: got %v", err)
	}
	if appErr.Code != model.ErrCodeProcessEnumTimeout {
		t.Errorf("错误码: got %s, want %s", appErr.Code, model.ErrCodeProcessEnumTimeout)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("须 %w 包装 ctx 错误保留 errors.Is 穿透")
	}
}

func TestListProcessesWithRunner_CommandFailure(t *testing.T) {
	run, _ := fakeProcessRunner(t, nil, errors.New("exit status 1"))
	_, err := listProcessesWithRunner(context.Background(), run)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeProcessEnumFailed {
		t.Fatalf("命令失败须返回 E_PROCESS_ENUM_FAILED: got %v", err)
	}
}

func TestBuildTaskkillArgs(t *testing.T) {
	cases := []struct {
		name     string
		pid      int
		killTree bool
		force    bool
		want     []string
	}{
		{"单进程优雅", 1234, false, false, []string{"/PID", "1234"}},
		{"进程树优雅", 1234, true, false, []string{"/PID", "1234", "/T"}},
		{"单进程强杀", 1234, false, true, []string{"/PID", "1234", "/F"}},
		{"进程树强杀", 1234, true, true, []string{"/PID", "1234", "/T", "/F"}},
	}
	for _, c := range cases {
		got := buildTaskkillArgs(c.pid, c.killTree, c.force)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

func TestClassifyTaskkillResult(t *testing.T) {
	cases := []struct {
		name     string
		output   string
		runErr   error
		force    bool
		wantOK   bool
		wantCode string
	}{
		{name: "成功", output: "SUCCESS: ...", runErr: nil, force: false, wantOK: true},
		{name: "进程不存在", output: "ERROR: The process with PID 1234 not found.", runErr: errors.New("exit 1"), wantCode: model.ErrCodeProcessNotFound},
		{name: "进程不存在中文", output: "错误: 没有找到进程 \"1234\"。", runErr: errors.New("exit 1"), force: true, wantCode: model.ErrCodeProcessNotFound},
		{name: "权限不足", output: "ERROR: ... Reason: Access is denied.", runErr: errors.New("exit 1"), wantCode: model.ErrCodeProcessAccessDenied},
		{name: "权限不足中文", output: "错误: 无法终止 PID 1234 的进程。原因: 拒绝访问。", runErr: errors.New("exit 1"), wantCode: model.ErrCodeProcessAccessDenied},
		// 优雅请求被目标拒绝（实测 cmd /k 为此形态）：非错误，ok=false 交由调用方转强杀
		{name: "优雅请求被拒", output: "ERROR: ... Reason: This process can only be terminated forcefully (with /F option).", runErr: errors.New("exit 1")},
		{name: "优雅请求被拒中文", output: "错误: ... 原因: 只能强行终止这个进程(带 /F 选项)。", runErr: errors.New("exit 1")},
		{name: "强杀未知失败", output: "ERROR: something else", runErr: errors.New("exit 1"), force: true, wantCode: model.ErrCodeProcessKillFailed},
		{name: "优雅未知失败转强杀", output: "ERROR: something else", runErr: errors.New("exit 1")},
	}
	for _, c := range cases {
		ok, appErr := classifyTaskkillResult(c.output, c.runErr, 1234, c.force)
		if ok != c.wantOK {
			t.Errorf("%s: ok got %v, want %v", c.name, ok, c.wantOK)
		}
		if c.wantCode == "" {
			if appErr != nil {
				t.Errorf("%s: 不应返回错误: %v", c.name, appErr)
			}
			continue
		}
		if appErr == nil || appErr.Code != c.wantCode {
			t.Errorf("%s: 错误码 got %v, want %s", c.name, appErr, c.wantCode)
		}
	}
}

func TestRequestProcessStopWithRunner(t *testing.T) {
	var args []string
	run := func(ctx context.Context, name string, a ...string) (string, error) {
		args = a
		return "SUCCESS: Sent termination signal", nil
	}
	ok, err := requestProcessStopWithRunner(4321, true, run)
	if err != nil || !ok {
		t.Fatalf("优雅请求成功须返回 ok=true, err=nil: got %v, %v", ok, err)
	}
	if len(args) != 3 || args[1] != "4321" || args[2] != "/T" {
		t.Errorf("优雅请求参数: got %v, want [/PID 4321 /T]", args)
	}
}

func TestKillProcessWithRunner_Force(t *testing.T) {
	var args []string
	run := func(ctx context.Context, name string, a ...string) (string, error) {
		args = a
		return "SUCCESS: terminated", nil
	}
	if err := killProcessWithRunner(4321, false, run); err != nil {
		t.Fatalf("强杀成功须返回 nil: %v", err)
	}
	if len(args) != 3 || args[1] != "4321" || args[2] != "/F" {
		t.Errorf("强杀参数须带 /F 且不带 /T: got %v", args)
	}
}

func TestIsProcessAlive(t *testing.T) {
	if !IsProcessAlive(os.Getpid()) {
		t.Error("本进程须判定为存活（自身 PID 必然存在，非环境依赖）")
	}
	for _, pid := range []int{0, -1} {
		if IsProcessAlive(pid) {
			t.Errorf("pid %d 须判定为不存活（0 为 System Idle Process，语义不稳定）", pid)
		}
	}
}
