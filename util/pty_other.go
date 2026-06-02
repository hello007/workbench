//go:build !windows

package util

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

// PtyProcess PTY 进程封装（Unix PTY 实现）
type PtyProcess struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

// NewPtyProcess 创建新的 PTY 进程
// cmdStr: 可执行文件路径, args: 命令参数, dir: 工作目录, cols/rows: 终端尺寸
func NewPtyProcess(cmdStr string, args []string, dir string, cols, rows uint16) (*PtyProcess, error) {
	cmd := exec.Command(cmdStr, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 PTY 失败: %w", err)
	}

	return &PtyProcess{ptmx: ptmx, cmd: cmd}, nil
}

// Read 从 PTY 读取输出
func (p *PtyProcess) Read(buf []byte) (int, error) {
	return p.ptmx.Read(buf)
}

// Write 向 PTY 写入输入
func (p *PtyProcess) Write(data []byte) (int, error) {
	return p.ptmx.Write(data)
}

// Resize 调整 PTY 窗口大小
func (p *PtyProcess) Resize(cols, rows uint16) error {
	return pty.Setsize(p.ptmx, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
}

// Close 关闭 PTY 进程
func (p *PtyProcess) Close() error {
	if p.ptmx != nil {
		p.ptmx.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Signal(syscall.SIGTERM)
	}
	return nil
}

// GetCmd 获取底层 exec.Cmd
func (p *PtyProcess) GetCmd() *exec.Cmd {
	return p.cmd
}

// IsProcessRunning 检查进程是否仍在运行
func (p *PtyProcess) IsProcessRunning() bool {
	if p.cmd == nil || p.cmd.Process == nil {
		return false
	}
	err := p.cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}
