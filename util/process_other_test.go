//go:build !windows

package util

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"workbench/model"
)

// writeProcFixture 构造 /proc 形态的目录树，返回根目录路径。
// 真实 /proc 内容随本机进程变化，依赖它的断言属环境外部状态、必然 flaky
// （docs/spec/test-stability.md），故枚举用例全部走 fixture 目录。
func writeProcFixture(t *testing.T, withSocket bool) string {
	t.Helper()
	root := t.TempDir()
	write := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	write(filepath.Join(root, "net", "tcp"), procNetTCPFixture)
	write(filepath.Join(root, "net", "udp"), procNetUDPFixture)
	write(filepath.Join(root, "1", "comm"), "systemd\n")
	write(filepath.Join(root, "1", "stat"), "1 (systemd) S 0 1 1 0 -1 0 0 0 0 0 0 0\n")
	write(filepath.Join(root, "1", "cmdline"), "/sbin/init\n")

	write(filepath.Join(root, "44336", "comm"), "node\n")
	write(filepath.Join(root, "44336", "status"), "Name:\tnode\nVmRSS:\t  105464 kB\n")
	write(filepath.Join(root, "44336", "stat"), "44336 (node) S 1234 44336 44336 0 -1 0 0 0 0 0 0 0\n")
	write(filepath.Join(root, "44336", "cmdline"), "node app.js\n")
	if withSocket {
		// fd/3 指向 socket:[22458]（/proc/net/tcp 首行 inode），用于反查端口→PID
		fdDir := filepath.Join(root, "44336", "fd")
		if err := os.MkdirAll(fdDir, 0o755); err != nil {
			t.Fatalf("mkdir fd: %v", err)
		}
		if err := os.Symlink("socket:[22458]", filepath.Join(fdDir, "3")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		if err := os.Symlink("/dev/null", filepath.Join(fdDir, "4")); err != nil {
			t.Fatalf("symlink: %v", err)
		}
	}
	// 非数字目录不得被当作进程目录
	write(filepath.Join(root, "self", "comm"), "self\n")
	return root
}

func TestListLinuxProcesses_Fixture(t *testing.T) {
	root := writeProcFixture(t, true)
	infos, err := listLinuxProcesses(context.Background(), root)
	if err != nil {
		t.Fatalf("枚举失败: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("条目数: got %d, want 2（self 等非数字目录须跳过）: %+v", len(infos), infos)
	}
	if infos[0].Pid != 1 || infos[1].Pid != 44336 {
		t.Fatalf("须按 PID 升序: got %d, %d", infos[0].Pid, infos[1].Pid)
	}
	if !infos[0].IsSystem {
		t.Error("PID 1 须标记 IsSystem")
	}
	node := infos[1]
	if node.Name != "node" || node.MemoryKB != 105464 {
		t.Errorf("进程名与内存须来自 comm/status: got %+v", node)
	}
	if len(node.Ports) != 1 || node.Ports[0].Port != 36082 || node.Ports[0].State != "LISTEN" {
		t.Errorf("端口须经 socket inode 反查挂接: got %+v", node.Ports)
	}
}

// TestListLinuxProcesses_UDPSocketMapped UDP 端口须与 TCP 一并解析：/proc/net/udp
// 与 tcp 列布局一致，经同一套 socket inode 反查挂到进程上，协议列据此显示 UDP。
func TestListLinuxProcesses_UDPSocketMapped(t *testing.T) {
	root := writeProcFixture(t, true)
	// fd/5 指向 /proc/net/udp 首行 inode（22470 → 端口 5353）
	if err := os.Symlink("socket:[22470]", filepath.Join(root, "44336", "fd", "5")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	infos, err := listLinuxProcesses(context.Background(), root)
	if err != nil {
		t.Fatalf("枚举失败: %v", err)
	}
	node := infos[1]
	if len(node.Ports) != 2 {
		t.Fatalf("TCP 与 UDP 端口须一并挂接: got %+v", node.Ports)
	}
	var udp model.ProcessPort
	found := false
	for _, p := range node.Ports {
		if p.Protocol == "UDP" {
			udp, found = p, true
		}
	}
	if !found || udp.Port != 5353 {
		t.Fatalf("须解析出 UDP 端口 5353: got %+v", node.Ports)
	}
	if udp.State != "" {
		t.Errorf("UDP 状态须留空（07/CLOSE 不具 TCP 语义）: got %q", udp.State)
	}
}

func TestListLinuxProcesses_NoSocketPorts(t *testing.T) {
	root := writeProcFixture(t, false)
	infos, err := listLinuxProcesses(context.Background(), root)
	if err != nil {
		t.Fatalf("枚举失败: %v", err)
	}
	if len(infos[1].Ports) != 0 {
		t.Errorf("无 socket fd 时端口须为空: got %+v", infos[1].Ports)
	}
}

// TestListLinuxProcesses_CanceledContext 取消必须优先于正常返回：
// 枚举为文件遍历，须在遍历中检测 ctx.Err() 给出可重试引导，不得返回半截结果。
func TestListLinuxProcesses_CanceledContext(t *testing.T) {
	root := writeProcFixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := listLinuxProcesses(ctx, root)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeProcessEnumTimeout {
		t.Fatalf("取消须返回 E_PROCESS_ENUM_TIMEOUT: got %v", err)
	}
}

func TestListLinuxProcesses_MissingNetTCP(t *testing.T) {
	root := t.TempDir()
	_, err := listLinuxProcesses(context.Background(), root)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeProcessEnumFailed {
		t.Fatalf("/proc/net/tcp 缺失须返回 E_PROCESS_ENUM_FAILED: got %v", err)
	}
}

func TestSelfProcessPIDs_ContainsSelf(t *testing.T) {
	pids := selfProcessPIDs()
	if !pids[os.Getpid()] {
		t.Error("自身 PID 必须纳入硬禁集合（判据只用 PID，不按进程名）")
	}
}

func TestClassifySignalError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"进程不存在", syscall.ESRCH, model.ErrCodeProcessNotFound},
		{"权限不足", syscall.EPERM, model.ErrCodeProcessAccessDenied},
		{"其它失败", errors.New("boom"), model.ErrCodeProcessKillFailed},
	}
	for _, c := range cases {
		appErr := classifySignalError(c.err, 1234)
		if appErr.Code != c.want {
			t.Errorf("%s: got %s, want %s", c.name, appErr.Code, c.want)
		}
	}
}

func TestIsProcessAlive(t *testing.T) {
	if !IsProcessAlive(os.Getpid()) {
		t.Error("本进程须判定为存活（自身 PID 必然存在，非环境依赖）")
	}
	for _, pid := range []int{0, -1} {
		if IsProcessAlive(pid) {
			t.Errorf("pid %d 须判定为不存活（kill(0,0) 是进程组语义，不可用于单进程判定）", pid)
		}
	}
}
