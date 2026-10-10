//go:build windows

package util

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"workbench/model"
)

// stillActiveExitCode GetExitCodeProcess 对存活进程返回的占位退出码（Win32 STILL_ACTIVE）。
const stillActiveExitCode = 259

// processTerminateCommandTimeout 单条 taskkill 命令的执行上限。
//
// taskkill 不带 /F 对弹「是否保存」对话框的进程可能长时间不返回，无限等待会让 Wails
// 绑定调用永不返回（前端卡在操作中）；超时视为未送达优雅请求，由 service 转强杀。
const processTerminateCommandTimeout = 3 * time.Second

// processCommandRunner 外部命令执行器签名（netstat / tasklist / taskkill 共用）。
//
// 抽出的目的是让单测注入 fake 输出：真实枚举结果随本机常驻进程变化（实测 36115
// 常被 workbench 自身占用），依赖真实命令输出的断言属环境外部状态、必然 flaky
// （docs/spec/test-stability.md）。
type processCommandRunner func(ctx context.Context, name string, args ...string) (string, error)

// ListProcesses 枚举本机进程及其占用端口（Windows：netstat -ano + tasklist /FO CSV /NH）。
//
// ctx 由调用方给定超时（service 层 10s）；命令失败出口先判 ctx.Err() 再判退出码，
// 超时须给可重试的 warning 引导而非「无进程」假态（对齐 service/content_search.go 收口范式）。
func ListProcesses(ctx context.Context) ([]model.ProcessInfo, error) {
	return listProcessesWithRunner(ctx, runProcessCommand)
}

// listProcessesWithRunner 枚举编排（fake runner 注入点），合并两命令输出为进程列表。
func listProcessesWithRunner(ctx context.Context, run processCommandRunner) ([]model.ProcessInfo, error) {
	netstatOut, err := run(ctx, "netstat", "-ano")
	if err != nil {
		return nil, wrapProcessEnumError(ctx, err, "netstat")
	}
	tasklistOut, err := run(ctx, "tasklist", "/FO", "CSV", "/NH")
	if err != nil {
		return nil, wrapProcessEnumError(ctx, err, "tasklist")
	}
	return mergeWindowsProcessInfos(
		parseNetstatOutput(netstatOut),
		parseTasklistCSV(tasklistOut),
		selfProcessPIDs(),
	), nil
}

// wrapProcessEnumError 枚举命令失败的统一出口：先判 ctx.Err() 再判命令错误。
// 超时/取消与真失败的错误码不同（前者可重试 warning，后者 error），前端分流有别。
func wrapProcessEnumError(ctx context.Context, err error, command string) error {
	if ctxErr := ctx.Err(); errors.Is(ctxErr, context.DeadlineExceeded) {
		return model.WrapAppError(model.ErrCodeProcessEnumTimeout, "进程枚举超时，请重试", ctxErr)
	}
	if ctxErr := ctx.Err(); errors.Is(ctxErr, context.Canceled) {
		return model.WrapAppError(model.ErrCodeProcessEnumTimeout, "进程枚举已取消，请重试", ctxErr)
	}
	return model.WrapAppError(model.ErrCodeProcessEnumFailed,
		fmt.Sprintf("进程枚举命令 %s 执行失败，请重试", command), err)
}

// runProcessCommand 真实命令执行：exec.CommandContext（ctx 超时经 TerminateProcess 杀子进程）
// + HideCommandWindow（避免 GUI 场景下闪黑窗）。
func runProcessCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	HideCommandWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		// taskkill 的失败原因文案（"ERROR: ... not found." / "Access is denied."）可能
		// 输出到 stderr，而 cmd.Output 默认丢弃 stderr：不补齐则 classifyTaskkillResult
		// 抓不到特征，会把「已不存在」「权限不足」误判为无特征失败（PRD 边缘态提示失真）。
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			out = append(out, exitErr.Stderr...)
		}
	}
	return string(out), err
}

// selfProcessPIDs 返回需硬禁终止的 PID 集合：本应用自身 + 父进程链。
//
// 判据只用 PID，禁止用进程名 "workbench.exe"：E2E 会把 exe 复制改名 workbench-e2e.exe
// 实现实例隔离（docs/spec/e2e-testing.md），按名称保护会漏；反之也会误伤用户自建同名 exe。
//
// Windows 侧父链覆盖到直接父进程（os.Getppid 可得）：更上层祖先需 NtQueryInformationProcess
// 或已弃用的 wmic，不值得为低频场景引入（Linux 侧可读 /proc 拿全链）。
func selfProcessPIDs() map[int]bool {
	pids := map[int]bool{os.Getpid(): true}
	if ppid := os.Getppid(); ppid > 0 {
		pids[ppid] = true
	}
	return pids
}

// IsSelfProcess 判定 pid 是否属本应用自身或父进程链。
// service 层终止入口的后端最终防线——前端按钮禁用可被绕过，后端必须再判一次。
func IsSelfProcess(pid int) bool {
	return selfProcessPIDs()[pid]
}

// RequestProcessStop 发起优雅停止请求（Windows：taskkill 不带 /F，对控制台进程投递控制
// 信号、对窗口进程投递关闭消息）。
//
// 请求能否生效**取决于目标进程是否响应，无法事前预判**：本机实测 `ping -n 300` 响应并退出
// （SUCCESS 且 1 秒后已退出），而 `cmd /k` 为保持交互会话主动忽略、报「只能强制终止」且仍存活。
// 故本函数只负责送达请求，是否真的退出由调用方轮询存活实测判定。
//
// 返回值 ok=true 表示请求已送达（进程可能弹「是否保存」对话框，需调用方轮询存活——SUCCESS
// 只代表信号送达，不代表进程已退出，刷盘等延迟退出的进程须靠轮询确认）；
// ok=false 且 err=nil 表示请求被目标拒绝，调用方应直接转强杀（PRD 已定：如实反映最终生效手段）。
func RequestProcessStop(pid int, killTree bool) (bool, error) {
	return requestProcessStopWithRunner(pid, killTree, runProcessCommand)
}

// requestProcessStopWithRunner 优雅停止（fake runner 注入点）。
func requestProcessStopWithRunner(pid int, killTree bool, run processCommandRunner) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), processTerminateCommandTimeout)
	defer cancel()
	out, err := run(ctx, "taskkill", buildTaskkillArgs(pid, killTree, false)...)
	ok, appErr := classifyTaskkillResult(out, err, pid, false)
	if appErr != nil {
		return false, appErr
	}
	return ok, nil
}

// KillProcess 强制终止单个进程（Windows：taskkill /F）。
//
// 默认只终止目标进程本身（不带 /T）；killTree 为 true 时连带子进程（/T），作为显式
// 二级选项由上层控制（对齐 Windows 任务管理器「结束进程树」的独立项语义）。
// 既有 KillProcessTree 保留给 AI 任务取消等原调用方，两者互不改动。
func KillProcess(pid int, killTree bool) error {
	return killProcessWithRunner(pid, killTree, runProcessCommand)
}

// killProcessWithRunner 强杀（fake runner 注入点）。
func killProcessWithRunner(pid int, killTree bool, run processCommandRunner) error {
	ctx, cancel := context.WithTimeout(context.Background(), processTerminateCommandTimeout)
	defer cancel()
	out, err := run(ctx, "taskkill", buildTaskkillArgs(pid, killTree, true)...)
	_, appErr := classifyTaskkillResult(out, err, pid, true)
	if appErr != nil {
		return appErr
	}
	return nil
}

// buildTaskkillArgs 组装 taskkill 参数：force 为 true 加 /F（强杀），killTree 为 true 加 /T（进程树）。
func buildTaskkillArgs(pid int, killTree bool, force bool) []string {
	args := []string{"/PID", strconv.Itoa(pid)}
	if killTree {
		args = append(args, "/T")
	}
	if force {
		args = append(args, "/F")
	}
	return args
}

// classifyTaskkillResult 收敛 taskkill 输出与退出码为统一结果，**形态识别的唯一出口**
// （对齐 util/git_lock.go 的 parseGitLockError 单点范式，避免各调用点各自匹配中文子串）。
//
// 判定顺序（互斥文案）：进程不存在 → 权限不足 → 优雅请求被拒 → 兜底。
// 返回 ok 表示优雅请求是否送达（仅优雅路径有意义），appErr 为 nil 时非强制路径即
// 「请求被目标拒绝」，由调用方转强杀——注意 SUCCESS 只代表信号送达，不代表进程已退出。
func classifyTaskkillResult(output string, runErr error, pid int, force bool) (ok bool, appErr *model.AppError) {
	if runErr == nil {
		return true, nil
	}
	switch {
	case outputContainsAny(output, "not found", "没有找到进程"):
		return false, model.NewAppError(model.ErrCodeProcessNotFound,
			fmt.Sprintf("进程 %d 已不存在，列表可能已过期", pid))
	case outputContainsAny(output, "Access is denied", "拒绝访问"):
		return false, model.NewAppError(model.ErrCodeProcessAccessDenied,
			"权限不足，无法结束该进程（可能是受保护的系统进程或其他用户的进程），请尝试以管理员身份运行 WorkBench")
	case outputContainsAny(output, "only be terminated forcefully", "只能强行终止", "只能强制终止"):
		// 优雅请求被目标拒绝（实测 cmd /k 为保持交互会话主动忽略即为此形态）：
		// 非错误，交由调用方转强杀，UI 不得谎称「已正常退出」
		return false, nil
	}
	if force {
		return false, model.WrapAppError(model.ErrCodeProcessKillFailed,
			fmt.Sprintf("强制结束进程 %d 失败", pid), errors.New(strings.TrimSpace(output)))
	}
	// 优雅路径未识别特征：视为请求被拒转强杀，真失败会在强杀路径暴露为具体错误
	return false, nil
}

// outputContainsAny 判定输出是否含任一特征子串（中英文双形态覆盖：本机为英文 UI，
// 中文系统文案形态一并匹配）。
func outputContainsAny(output string, patterns ...string) bool {
	for _, pattern := range patterns {
		if strings.Contains(output, pattern) {
			return true
		}
	}
	return false
}

// IsProcessAlive 探测进程是否存活（Windows：OpenProcess + GetExitCodeProcess，只读不发信号），
// 供 service 层「停止」的 5s 轮询判定进程是否已退出。
//
// pid <= 0 直接判否（0 为 System Idle Process，OpenProcess 语义不稳定）。
// 拒绝访问（受保护进程 / 更高完整性级别）时保守判存活——判否会让调用方误以为优雅
// 停止已生效而谎报成功。
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(handle)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return true // 句柄有效但读退出码失败：无法判定，保守判存活
	}
	return exitCode == stillActiveExitCode
}
