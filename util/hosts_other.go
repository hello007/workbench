//go:build !windows

package util

// HostsFilePath 本机 hosts 文件路径（非 Windows 平台：Linux / darwin 等 POSIX 系）。
//
// 与 Windows 路径分叉于 hosts_windows.go；读文件与解析逻辑在无 tag 的 hosts.go
// 两平台共享同测。变更须同步 README / docs/功能说明.md 的路径描述。
const HostsFilePath = "/etc/hosts"
