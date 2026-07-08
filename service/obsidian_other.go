//go:build !windows

package service

// isObsidianProtocolRegistered 非 Windows 平台暂不支持，直接返回 false。
// 本特性当前仅面向 Windows（与现有 OpenIn*/资源管理器等一致）。
func isObsidianProtocolRegistered() bool {
	return false
}

// obsidianConfigPath 非 Windows 平台暂不支持，返回空串。
// 调用方（loadObsidianVaults）据此返回错误并降级为现状尽力打开。
func obsidianConfigPath() string {
	return ""
}

// isObsidianRunning 非 Windows 平台暂不支持进程检测，返回 false。
// 自动注册功能当前仅面向 Windows；非 Windows 调用方会在协议检测阶段返回 ErrObsidianNotInstalled。
func isObsidianRunning() bool {
	return false
}
