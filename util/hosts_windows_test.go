//go:build windows

package util

import "testing"

// TestHostsFilePath_Windows Windows hosts 路径特征断言：锁定平台分叉契约
// （与 hosts_other.go 的 /etc/hosts 互为唯一编译侧），变更须同步文档路径描述。
func TestHostsFilePath_Windows(t *testing.T) {
	if HostsFilePath != `C:\Windows\System32\drivers\etc\hosts` {
		t.Errorf("Windows hosts 路径不符: got %q", HostsFilePath)
	}
}
