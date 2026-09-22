//go:build !windows

package util

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Linux 剪贴板实现：文本用 xclip（X11）/ wl-copy（Wayland）；
// 文件列表写 x-special/gnome-copied-files 目标（格式见 clipboard_payload.go）。
// 会话类型判定：WAYLAND_DISPLAY 非空视为 Wayland 会话，否则（X11 或未设置）默认 xclip。

// clipboardTools 返回当前会话类型对应的剪贴板写入/读取工具名。
func clipboardTools() (writeTool, readTool string) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wl-copy", "wl-paste"
	}
	return "xclip", "xclip"
}

// wrapClipboardToolError 将工具执行错误翻译为用户可读信息；
// 工具未安装（PATH 中找不到）返回明确的中文安装提示。
func wrapClipboardToolError(tool string, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("剪贴板工具 %s 未安装，请先安装（X11: xclip，Wayland: wl-clipboard）", tool)
	}
	return fmt.Errorf("剪贴板工具 %s 执行失败: %w", tool, err)
}

// WriteClipboardText 将文本写入系统剪贴板
// （X11: xclip -selection clipboard；Wayland: wl-copy），文本经 stdin 传入。
func WriteClipboardText(text string) error {
	writeTool, _ := clipboardTools()
	var cmd *exec.Cmd
	if writeTool == "wl-copy" {
		cmd = exec.Command("wl-copy")
	} else {
		cmd = exec.Command("xclip", "-selection", "clipboard")
	}
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return wrapClipboardToolError(writeTool, err)
	}
	return nil
}

// WriteClipboardFiles 将文件路径列表写入系统剪贴板
// （x-special/gnome-copied-files 目标，携带 copy/cut 动作），载荷经 stdin 传入。
func WriteClipboardFiles(paths []string, isCut bool) error {
	if len(paths) == 0 {
		return errors.New("no paths provided")
	}
	writeTool, _ := clipboardTools()
	payload := buildClipboardFilesPayload(paths, isCut)

	var cmd *exec.Cmd
	if writeTool == "wl-copy" {
		cmd = exec.Command("wl-copy", "-t", clipboardFilesTarget)
	} else {
		cmd = exec.Command("xclip", "-selection", "clipboard", "-t", clipboardFilesTarget, "-i")
	}
	stdin := strings.NewReader(payload)
	cmd.Stdin = stdin
	if err := cmd.Run(); err != nil {
		return wrapClipboardToolError(writeTool, err)
	}
	return nil
}

// ReadClipboardFiles 读取系统剪贴板中的文件路径列表。
// 剪贴板无 x-special/gnome-copied-files 内容（目标不可读或载荷无效）时
// 返回 (nil, false, nil)，与 Windows 版「无文件」语义对齐；
// 剪贴板工具未安装属环境缺失，返回明确错误。
func ReadClipboardFiles() ([]string, bool, error) {
	_, readTool := clipboardTools()
	var cmd *exec.Cmd
	if readTool == "wl-paste" {
		cmd = exec.Command("wl-paste", "-t", clipboardFilesTarget)
	} else {
		cmd = exec.Command("xclip", "-selection", "clipboard", "-t", clipboardFilesTarget, "-o")
	}
	out, err := cmd.Output()
	if err != nil {
		// 工具未安装属环境缺失，需提示用户；其余（目标不可读等）按「无文件」降级
		if errors.Is(err, exec.ErrNotFound) {
			return nil, false, wrapClipboardToolError(readTool, err)
		}
		return nil, false, nil
	}
	paths, isCut, ok := parseClipboardFilesPayload(string(out))
	if !ok {
		return nil, false, nil
	}
	return paths, isCut, nil
}
