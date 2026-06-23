//go:build windows

package service

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

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
