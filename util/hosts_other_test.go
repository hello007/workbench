//go:build !windows

package util

import "testing"

// TestHostsFilePath_Other 非 Windows 平台 hosts 路径特征断言：锁定平台分叉契约
// （与 hosts_windows.go 的 C:\Windows\... 互为唯一编译侧），变更须同步文档路径描述。
func TestHostsFilePath_Other(t *testing.T) {
	if HostsFilePath != "/etc/hosts" {
		t.Errorf("非 Windows hosts 路径不符: got %q", HostsFilePath)
	}
}
