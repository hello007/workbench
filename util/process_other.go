//go:build !windows

package util

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"workbench/model"
)

// defaultProcDir Linux 进程信息根路径。
const defaultProcDir = "/proc"

// maxParentChainDepth 父进程链上溯深度上限（防 /proc 数据异常成环）。
const maxParentChainDepth = 16

// ListProcesses 枚举本机进程及其占用端口（Linux：解析 /proc/net/tcp{,6} + /proc/<pid>/fd）。
//
// ctx 由调用方给定超时（service 层 10s）；枚举为纯文件读取，遍历过程中检测 ctx.Err()，
// 超时须给可重试的 warning 引导而非「无进程」假态（对齐 service/content_search.go 收口范式）。
func ListProcesses(ctx context.Context) ([]model.ProcessInfo, error) {
	return listLinuxProcesses(ctx, defaultProcDir)
}

// listLinuxProcesses 枚举编排，procDir 参数供单测注入 fixture 目录——真实 /proc 内容
// 随本机进程变化，依赖真实 /proc 的断言属环境外部状态、必然 flaky（docs/spec/test-stability.md）。
func listLinuxProcesses(ctx context.Context, procDir string) ([]model.ProcessInfo, error) {
	inodePorts, err := readProcNetPorts(procDir)
	if err != nil {
		return nil, model.WrapAppError(model.ErrCodeProcessEnumFailed, "读取网络连接信息失败，请重试", err)
	}
	portsByPid, err := mapSocketPortsToPIDs(ctx, procDir, inodePorts)
	if err != nil {
		return nil, err
	}
	metaByPid, err := readProcMeta(ctx, procDir)
	if err != nil {
		return nil, err
	}
	return mergeLinuxProcessInfos(portsByPid, metaByPid, selfProcessPIDs()), nil
}

// readProcNetPorts 读 /proc/net/{tcp,tcp6,udp,udp6}，返回 socket inode → 端口条目映射。
// tcp 读取失败即报错（连接信息主通道）；tcp6 / udp / udp6 缺失（未启用 IPv6 或 UDP
// 的内核）静默忽略，仅影响协议列是否出现对应协议，不影响主通道。
func readProcNetPorts(procDir string) (map[string]model.ProcessPort, error) {
	tcpContent, err := os.ReadFile(filepath.Join(procDir, "net", "tcp"))
	if err != nil {
		return nil, err
	}
	entries := parseProcNetTCP(string(tcpContent), false)
	if tcp6Content, err6 := os.ReadFile(filepath.Join(procDir, "net", "tcp6")); err6 == nil {
		entries = append(entries, parseProcNetTCP(string(tcp6Content), true)...)
	}
	if udpContent, errU := os.ReadFile(filepath.Join(procDir, "net", "udp")); errU == nil {
		entries = append(entries, parseProcNetUDP(string(udpContent), false)...)
	}
	if udp6Content, errU6 := os.ReadFile(filepath.Join(procDir, "net", "udp6")); errU6 == nil {
		entries = append(entries, parseProcNetUDP(string(udp6Content), true)...)
	}
	ports := make(map[string]model.ProcessPort, len(entries))
	for _, entry := range entries {
		ports[entry.Inode] = model.ProcessPort{
			Port:      entry.Port,
			Protocol:  entry.Protocol,
			State:     entry.State,
			LocalAddr: entry.LocalAddr,
		}
	}
	return ports, nil
}

// mapSocketPortsToPIDs 经 /proc/<pid>/fd/<n> 的 socket:[inode] 链接把端口反查到 PID。
// 仅在有目标 inode 时才遍历（端口检索场景不白扫）；进程已退出或无权读 fd 目录时跳过。
func mapSocketPortsToPIDs(ctx context.Context, procDir string, inodePorts map[string]model.ProcessPort) (map[int][]model.ProcessPort, error) {
	result := make(map[int][]model.ProcessPort)
	if len(inodePorts) == 0 {
		return result, nil
	}
	pids, err := listProcPIDs(procDir)
	if err != nil {
		return nil, err
	}
	for _, pid := range pids {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, wrapLinuxEnumContextError(ctxErr)
		}
		fdDir := filepath.Join(procDir, strconv.Itoa(pid), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // 进程已退出，或非 root 无权读他人进程 fd
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			inode, ok := parseSocketInode(link)
			if !ok {
				continue
			}
			port, ok := inodePorts[inode]
			if !ok {
				continue
			}
			result[pid] = append(result[pid], port)
		}
	}
	return result, nil
}

// readProcMeta 读取全部进程的元数据（进程名 / 内存 / 父进程 / 内核线程标志）。
func readProcMeta(ctx context.Context, procDir string) (map[int]linuxProcMeta, error) {
	pids, err := listProcPIDs(procDir)
	if err != nil {
		return nil, err
	}
	meta := make(map[int]linuxProcMeta, len(pids))
	for _, pid := range pids {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, wrapLinuxEnumContextError(ctxErr)
		}
		dir := filepath.Join(procDir, strconv.Itoa(pid))
		name := ""
		if data, err := os.ReadFile(filepath.Join(dir, "comm")); err == nil {
			name = strings.TrimSpace(string(data)) // comm 以换行结尾，须裁剪后展示
		}
		memory := int64(0)
		if data, err := os.ReadFile(filepath.Join(dir, "status")); err == nil {
			memory = parseProcStatusRSSKB(string(data))
		}
		ppid := 0
		if data, err := os.ReadFile(filepath.Join(dir, "stat")); err == nil {
			ppid = parseProcStatPPid(string(data))
		}
		meta[pid] = linuxProcMeta{
			Name:     name,
			MemoryKB: memory,
			PPid:     ppid,
			IsKernel: isKernelThread(filepath.Join(dir, "cmdline")),
		}
	}
	return meta, nil
}

// isKernelThread 判定是否为内核线程：/proc/<pid>/cmdline 为空（内核线程无命令行）。
// 读取失败（权限不足或进程已退出）保守判否——判为内核线程会误标 IsSystem 触发强确认。
func isKernelThread(cmdlinePath string) bool {
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}
	return len(data) == 0
}

// listProcPIDs 列出 procDir 下的进程目录数字名（升序）。
func listProcPIDs(procDir string) ([]int, error) {
	infos, err := os.ReadDir(procDir)
	if err != nil {
		return nil, model.WrapAppError(model.ErrCodeProcessEnumFailed, "读取进程列表失败，请重试", err)
	}
	pids := make([]int, 0, len(infos))
	for _, info := range infos {
		if !info.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(info.Name())
		if err != nil || pid <= 0 {
			continue // 非数字目录（如 self、net）不是进程目录
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// wrapLinuxEnumContextError 枚举遍历中检测到 ctx 结束的统一出口：
// 超时/取消都给可重试的 warning 引导（与 Windows 侧 wrapProcessEnumError 对称）。
func wrapLinuxEnumContextError(ctxErr error) error {
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		return model.WrapAppError(model.ErrCodeProcessEnumTimeout, "进程枚举超时，请重试", ctxErr)
	}
	return model.WrapAppError(model.ErrCodeProcessEnumTimeout, "进程枚举已取消，请重试", ctxErr)
}

// selfProcessPIDs 返回需硬禁终止的 PID 集合：本应用自身 + 父进程链。
//
// 判据只用 PID，禁止用进程名：E2E 会把 exe 改名做实例隔离，按名称保护会漏
// （docs/spec/e2e-testing.md）。Linux 侧经 /proc/<pid>/stat 可上溯完整父链
// （Windows 侧仅到直接父进程 os.Getppid，见 util/process_windows.go）。
func selfProcessPIDs() map[int]bool {
	pids := map[int]bool{os.Getpid(): true}
	current := os.Getppid()
	for depth := 0; depth < maxParentChainDepth; depth++ {
		if current <= 1 {
			break
		}
		pids[current] = true
		next := readParentPID(current)
		if next <= 0 || next == current {
			break
		}
		current = next
	}
	return pids
}

// readParentPID 读 /proc/<pid>/stat 取父进程 PID，失败返回 0（链条在此终止，不报错）。
func readParentPID(pid int) int {
	data, err := os.ReadFile(filepath.Join(defaultProcDir, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0
	}
	return parseProcStatPPid(string(data))
}

// IsSelfProcess 判定 pid 是否属本应用自身或父进程链。
// service 层终止入口的后端最终防线——前端按钮禁用可被绕过，后端必须再判一次。
func IsSelfProcess(pid int) bool {
	return selfProcessPIDs()[pid]
}

// RequestProcessStop 发起优雅停止请求（Linux：SIGTERM，进程可自行清理后退出）。
// 信号发送成功即返回 ok=true，进程是否在 5s 内真正退出由调用方轮询存活判定。
func RequestProcessStop(pid int, killTree bool) (bool, error) {
	if err := signalProcess(pid, syscall.SIGTERM, killTree); err != nil {
		return false, classifySignalError(err, pid)
	}
	return true, nil
}

// KillProcess 强制终止进程（Linux：SIGKILL，进程无法忽略）。
// killTree 为 false 时只对目标 PID 发信号，不连带子进程（进程树为显式二级选项）。
func KillProcess(pid int, killTree bool) error {
	if err := signalProcess(pid, syscall.SIGKILL, killTree); err != nil {
		return classifySignalError(err, pid)
	}
	return nil
}

// signalProcess 向进程发信号。killTree 为 true 时先按进程组发（-pid），失败退化为
// 单进程：-pid 仅在目标是进程组组长时生效，与既有 util/kill_other.go 的语义一致。
func signalProcess(pid int, signal syscall.Signal, killTree bool) error {
	if killTree {
		if err := syscall.Kill(-pid, signal); err == nil {
			return nil
		}
	}
	return syscall.Kill(pid, signal)
}

// classifySignalError 收敛信号失败形态为 AppError，**形态识别的唯一出口**
// （对齐 util/git_lock.go 的 parseGitLockError 单点范式）。
func classifySignalError(err error, pid int) *model.AppError {
	if errors.Is(err, syscall.ESRCH) {
		return model.NewAppError(model.ErrCodeProcessNotFound,
			fmt.Sprintf("进程 %d 已不存在，列表可能已过期", pid))
	}
	if errors.Is(err, syscall.EPERM) {
		return model.NewAppError(model.ErrCodeProcessAccessDenied,
			"权限不足，无法结束该进程（可能是其他用户的进程），请尝试以更高权限运行 WorkBench")
	}
	return model.WrapAppError(model.ErrCodeProcessKillFailed,
		fmt.Sprintf("结束进程 %d 失败", pid), err)
}

// IsProcessAlive 探测进程是否存活（Linux：kill(pid, 0) 只做存在性与权限检查，不发实际信号），
// 供 service 层「停止」的 5s 轮询判定进程是否已退出。
//
// pid <= 0 直接判否（kill(0, 0) 是进程组语义，与单进程判定不符）。
// EPERM（进程存在但无权限）保守判存活——判否会让调用方误以为优雅停止已生效而谎报成功。
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
