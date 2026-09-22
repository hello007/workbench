//go:build !windows

package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"workbench/util"
)

// isObsidianProtocolRegistered 检查 obsidian:// 协议是否已注册：
// xdg-mime 查询该 scheme 的默认处理程序，输出非空（存在关联的 .desktop 项）即视为已注册。
// xdg-mime 未安装/查询失败一律视为未注册（调用方返回 ErrObsidianNotInstalled，
// 与「未安装 Obsidian」的用户提示一致）。
func isObsidianProtocolRegistered() bool {
	out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/obsidian").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// obsidianConfigPath 返回 Obsidian vault 注册表路径：
// 优先 XDG_CONFIG_HOME，缺省 $HOME/.config/obsidian/obsidian.json（Electron 应用的 userData 根）。
// HOME 缺失时返回空串，调用方（loadObsidianVaults）据此返回错误并降级为现状尽力打开。
func obsidianConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "obsidian", "obsidian.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "obsidian", "obsidian.json")
}

// isObsidianRunning 检测 Obsidian 进程是否运行（pgrep -i 忽略大小写匹配进程名）。
// 对齐 Windows 版 tasklist 输出 ToLower 子串过滤的大小写不敏感语义：
// Electron AppImage/deb 等安装形态进程名可能为大写开头（Obsidian），小写模式会漏检，
// 漏检即误判未运行并继续写入，运行中的 Obsidian 回写会覆盖修改导致注册静默丢失。
// pgrep 默认只匹配进程名（comm），不会误命中其他命令；-i 为 procps-ng 选项
// （PRD 锁定的 Ubuntu 22.04+ 基线均支持）。
// pgrep 退出码 1 = 无匹配进程；其他错误（pgrep 不可用等）保守视为运行中（返回 true + 记日志）：
// 无法确认是否运行时若误判为未运行并继续写入，Obsidian 运行时回写会覆盖手动修改导致注册静默丢失，
// 保守阻塞（引导用户先关闭 Obsidian）虽有误杀但无数据丢失风险（与 Windows 版决策一致）。
func isObsidianRunning() bool {
	cmd := exec.Command("pgrep", "-i", "obsidian")
	util.HideCommandWindow(cmd)
	err := cmd.Run()
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false
	}
	Logger().Warn("pgrep detect failed, assume obsidian running", "err", err)
	return true
}
