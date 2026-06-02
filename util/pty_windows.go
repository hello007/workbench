//go:build windows

package util

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
)

// PtyProcess PTY 进程封装（Windows ConPTY 实现）
type PtyProcess struct {
	conPty *conpty.ConPty
	pid    int
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

	return &PtyProcess{
		conPty: cpty,
		pid:    cpty.Pid(),
	}, nil
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
func (p *PtyProcess) Close() error {
	if p.conPty != nil {
		return p.conPty.Close()
	}
	return nil
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
