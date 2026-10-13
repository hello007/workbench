//go:build windows

package util

// HostsFilePath 本机 hosts 文件路径（Windows）。
//
// 与 Linux 路径分叉于 hosts_other.go（docs/spec/linux-platform.md §2：API 值平台独有
// 走 build tag 双文件）；读文件与解析逻辑在无 tag 的 hosts.go 两平台共享同测。
// 变更须同步 README / docs/功能说明.md 的路径描述。
const HostsFilePath = `C:\Windows\System32\drivers\etc\hosts`
