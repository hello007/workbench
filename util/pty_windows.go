//go:build windows

package util

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"unsafe"

	"github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
)

// PtyProcess PTY 进程封装（Windows ConPTY 实现）
//
// job 为会话级 Job Object 句柄（由 createKillOnCloseJob 创建）。之所以需要它：
// ClosePseudoConsole 只是断开 ConPTY 管道并把「控制台关闭」事件投递给 shell，
// 既不保证 shell 本身退出（交互式 shell 可在句柄断开后继续驻留），更不会回收
// shell 派生的子进程（`ping -t`、`npm run dev`、`sleep` 这类长任务会孤儿化）。
// 因此每个会话额外绑定一个 KILL_ON_JOB_CLOSE 的 Job Object：关闭该句柄时由内核
// 终止 Job 内的整棵进程树；应用崩溃或被结束进程时 OS 关闭句柄同样生效。
type PtyProcess struct {
	conPty *conpty.ConPty
	pid    int
	job    windows.Handle // 0 表示 Job 绑定失败（降级：关闭时不再保证回收子进程）

	// Close 幂等：关闭动作（断 ConPTY + 关 Job 句柄）只执行一次，重复调用返回
	// 首次结果，避免二次关闭句柄触发 ERROR_INVALID_HANDLE 或重复投递关闭事件
	closeOnce sync.Once
	closeErr  error
}

// NewPtyProcess 创建新的 PTY 进程
// cmd: 可执行文件路径, args: 命令参数, dir: 工作目录, cols/rows: 终端尺寸
func NewPtyProcess(cmd string, args []string, dir string, cols, rows uint16) (*PtyProcess, error) {
	// 拼接命令行：Windows ConPTY 需要完整的命令行字符串
	commandLine := buildCommandLine(cmd, args)

	// 构建选项
	opts := []conpty.ConPtyOption{
		conpty.ConPtyDimensions(int(cols), int(rows)),
	}
	if dir != "" {
		opts = append(opts, conpty.ConPtyWorkDir(dir))
	}

	// 检查 ConPTY 是否可用
	if !conpty.IsConPtyAvailable() {
		return nil, fmt.Errorf("当前 Windows 版本不支持 ConPTY（需要 Windows 10 1809+）")
	}

	// 启动 ConPTY
	cpty, err := conpty.Start(commandLine, opts...)
	if err != nil {
		return nil, fmt.Errorf("创建 ConPTY 失败: %w", err)
	}

	proc := &PtyProcess{
		conPty: cpty,
		pid:    cpty.Pid(),
	}

	// 绑定 Job Object：紧随 conpty.Start 之后同步执行，尽量缩小「shell 已派生出
	// 子进程但尚未加入 Job」的窗口（该窗口下已存在的后代不会随 Job 关闭而回收，
	// 属已知可接受取舍）
	bindJobOrWarn(proc, createKillOnCloseJob)

	return proc, nil
}

// bindJobOrWarn 为会话绑定 Job Object，失败时降级为「不绑定」并记录 Warn。
//
// create 为 Job 创建与绑定动作（生产传入 createKillOnCloseJob，测试注入 fake），
// 以参数而非包级变量传入，既保证单测可覆盖降级路径，又不引入全局可变状态。
// 成功时把 Job 句柄写入 proc.job（由 PtyProcess.Close 负责关闭）；失败时保持
// proc.job 为 0，终端创建流程不受影响。
func bindJobOrWarn(proc *PtyProcess, create func(int) (windows.Handle, error)) {
	job, err := create(proc.pid)
	if err != nil {
		// 降级：绑定失败不阻断终端创建，只是关闭时不再保证回收派生子进程
		slog.Warn("终端会话 Job 绑定失败，关闭终端时派生子进程可能残留",
			"pid", proc.pid, "err", err)
		return
	}
	proc.job = job
}

// createKillOnCloseJob 为指定进程创建并绑定 KILL_ON_JOB_CLOSE 的 Job Object。
// 成功后返回 Job 句柄（调用方负责在会话关闭时 CloseHandle）；任何一步失败都
// 关闭已创建句柄并返回 0 与错误，调用方据此降级为「不绑定」。
func createKillOnCloseJob(pid int) (windows.Handle, error) {
	if pid <= 0 {
		return 0, fmt.Errorf("无效的进程 PID: %d", pid)
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("创建 Job 对象失败: %w", err)
	}

	// 设置 KILL_ON_JOB_CLOSE：Job 的最后一个句柄关闭时，内核终止 Job 内全部进程
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("设置 Job 对象 KILL_ON_JOB_CLOSE 失败: %w", err)
	}

	// 按 PID 取得进程句柄：AssignProcessToJobObject 要求 PROCESS_SET_QUOTA +
	// PROCESS_TERMINATE 权限（ConPTY 封装不对外暴露已持有的进程句柄）
	hProc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("打开进程 %d 句柄失败: %w", pid, err)
	}
	// 分配完成即关闭进程句柄：Job 只记录进程归属关系，不依赖该句柄存活
	defer windows.CloseHandle(hProc)

	if err := windows.AssignProcessToJobObject(job, hProc); err != nil {
		windows.CloseHandle(job)
		return 0, fmt.Errorf("将进程 %d 加入 Job 对象失败: %w", pid, err)
	}

	return job, nil
}

// buildCommandLine 将命令和参数拼接为 Windows 命令行字符串
// 遵循 Windows 命令行参数转义规则
func buildCommandLine(cmd string, args []string) string {
	parts := []string{quoteArg(cmd)}
	for _, arg := range args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.Join(parts, " ")
}

// quoteArg 对命令行参数进行引号包裹
// 仅当参数包含空格、制表符或双引号时才包裹
func quoteArg(arg string) string {
	if arg == "" || strings.ContainsAny(arg, " \t\"") {
		return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
	}
	return arg
}

// Read 从 PTY 读取输出
func (p *PtyProcess) Read(buf []byte) (int, error) {
	return p.conPty.Read(buf)
}

// Write 向 PTY 写入输入
func (p *PtyProcess) Write(data []byte) (int, error) {
	return p.conPty.Write(data)
}

// Resize 调整 PTY 窗口大小
func (p *PtyProcess) Resize(cols, rows uint16) error {
	return p.conPty.Resize(int(cols), int(rows))
}

// Close 关闭 PTY 进程
//
// 顺序固定：先 ClosePseudoConsole（让 shell 收到控制台关闭事件，尽量自行退出、
// 走正常清理流程），再关闭 Job 句柄（内核立即终止 Job 内仍存活的全部进程，
// 包括不响应关闭事件的 shell 与它派生的长任务）。
// 幂等：重复调用只返回首次结果，不重复关闭句柄。
func (p *PtyProcess) Close() error {
	p.closeOnce.Do(func() {
		p.closeErr = p.close()
	})
	return p.closeErr
}

// close 执行实际的关闭动作，返回首个错误（后续清理照常进行）
func (p *PtyProcess) close() error {
	var firstErr error

	if p.conPty != nil {
		if err := p.conPty.Close(); err != nil {
			firstErr = err
		}
	}

	if p.job != 0 {
		if err := windows.CloseHandle(p.job); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("关闭终端 Job 对象失败: %w", err)
		}
		// 置零仅为状态自省（重复调用已被 closeOnce 拦住），不参与关闭判定
		p.job = 0
	}

	return firstErr
}

// GetCmd 获取底层命令（Windows ConPTY 不直接暴露 exec.Cmd）
func (p *PtyProcess) GetCmd() *exec.Cmd {
	return nil
}

// IsProcessRunning 检查进程是否仍在运行
// 通过 Windows API 查询进程退出码判断
func (p *PtyProcess) IsProcessRunning() bool {
	if p.pid == 0 {
		return false
	}

	// 尝试打开进程句柄
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	// 查询退出码，259 = STILL_ACTIVE
	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == 259
}
