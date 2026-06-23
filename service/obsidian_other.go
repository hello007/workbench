//go:build !windows

package service

// isObsidianProtocolRegistered 非 Windows 平台暂不支持，直接返回 false。
// 本特性当前仅面向 Windows（与现有 OpenIn*/资源管理器等一致）。
func isObsidianProtocolRegistered() bool {
	return false
}
