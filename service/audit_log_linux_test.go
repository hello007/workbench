//go:build linux

// 审计轮转链读写竞速的 Unix 分叉断言（连接管控十一期）。平台分叉依据：Unix
// rename 只操作目录项，不受读侧打开句柄阻碍（与 Windows sharing violation 停链
// 互斥）——轮转成功、被打开的旧档内容为轮转前一致快照（打开-读-关闭快照语义）。
// 平台耦合断言按 linux-platform 规则拆 _windows_test.go/_linux_test.go，
// 禁共享一份。
package service

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAuditLogger_RotateWhileOpenForRead_UnixRenameUnblocked 读写并发（Unix 形态）：
// 读侧持有 .1 档句柄期间写侧轮转成功（rename 不受阻），链完整就位；读侧已打开
// 句柄读到的是轮转前一致快照（旧 .1 内容原样可见）。
func TestAuditLogger_RotateWhileOpenForRead_UnixRenameUnblocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)
	a.mu.Lock()
	a.maxBytes = 160
	a.depth = 2
	a.mu.Unlock()

	a.Log(AuditEventAuthSuccess, "tok-a", "", "127.0.0.1:1", "ua")
	a.Log(AuditEventAuthSuccess, "tok-b", "", "127.0.0.1:1", "ua") // 轮转：.1=tok-a 主=tok-b

	f, err := os.Open(path + ".1")
	if err != nil {
		t.Fatalf("打开 .1: %v", err)
	}
	defer func() { _ = f.Close() }()

	a.Log(AuditEventAuthSuccess, "tok-c", "", "127.0.0.1:1", "ua") // 轮转成功：.1→.2、主→.1

	if got := readAuditLines(t, path+".2"); len(got) != 1 || got[0].TokenID != "tok-a" {
		t.Errorf("轮转不受打开句柄阻碍，.1 应顺移为 .2: %+v", got)
	}
	if got := readAuditLines(t, path+".1"); len(got) != 1 || got[0].TokenID != "tok-b" {
		t.Errorf("原主文件应轮转为 .1: %+v", got)
	}
	if got := readAuditLines(t, path); len(got) != 1 || got[0].TokenID != "tok-c" {
		t.Errorf("轮转后主文件应继续追加: %+v", got)
	}

	// 已打开句柄为轮转前一致快照：读 fd 仍见旧 .1 内容（tok-a）
	buf := make([]byte, 256)
	n, readErr := f.ReadAt(buf, 0)
	if readErr != nil && readErr != io.EOF {
		t.Fatalf("读已打开句柄: %v", readErr)
	}
	if got := string(buf[:n]); !strings.Contains(got, `"tok-a"`) {
		t.Errorf("已打开句柄应保持轮转前一致快照（tok-a）, got %q", got)
	}
}
