//go:build windows

package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
	"workbench/util"
)

// obsidianConfigPath 返回 Obsidian vault 注册表路径：%APPDATA%\obsidian\obsidian.json。
// Obsidian 在此文件中维护已注册 vault 列表（非官方公开内部机制，需防御性解析）。
// APPDATA 缺失时返回空串，调用方据此降级。
func obsidianConfigPath() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return ""
	}
	return filepath.Join(appdata, "obsidian", "obsidian.json")
}

// isObsidianProtocolRegistered 检查 obsidian:// 协议是否已注册（即 Obsidian 已安装并至少运行过一次）。
// 优先查 HKEY_CLASSES_ROOT（系统级），兜底查 HKEY_CURRENT_USER（用户级安装）。
// cmd /c start 在协议未注册时会弹系统"如何打开此链接"对话框而非返回错误码，
// 故必须预检以给出友好提示。
func isObsidianProtocolRegistered() bool {
	locations := []struct {
		root registry.Key
		sub  string
	}{
		{registry.CLASSES_ROOT, `obsidian\shell\open\command`},
		{registry.CURRENT_USER, `Software\Classes\obsidian\shell\open\command`},
	}
	for _, loc := range locations {
		key, err := registry.OpenKey(loc.root, loc.sub, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		val, _, _ := key.GetStringValue("")
		key.Close()
		if strings.Contains(strings.ToLower(val), "obsidian") {
			return true
		}
	}
	return false
}

// isObsidianRunning 检测 Obsidian 进程是否运行（tasklist 枚举，过滤 obsidian.exe）。
// 用 tasklist /FO CSV /NH 全量输出再过滤，比 /FI 过滤在 cmd 嵌套下更可靠（/FI 易 exit status 1）。
// 检测失败时保守视为运行中（返回 true + 记日志）：tasklist 失败时无法确认 Obsidian 是否运行，
// 若误判为未运行并继续写入，Obsidian 运行时回写会覆盖手动修改导致注册静默丢失；
// 保守阻塞（引导用户先关闭 Obsidian）虽有误杀但无数据丢失风险，更安全（见 research Caveat #8）。
// 调用方在写入前据此判断是否引导用户先关闭 Obsidian（规避运行时回写覆盖）。
func isObsidianRunning() bool {
	cmd := exec.Command("tasklist", "/FO", "CSV", "/NH")
	util.HideCommandWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		println("警告: tasklist 进程枚举失败，保守视为 Obsidian 运行中:", err.Error())
		return true
	}
	return strings.Contains(strings.ToLower(string(out)), "obsidian.exe")
}
