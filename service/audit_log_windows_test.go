//go:build windows

// 审计轮转链读写竞速的 Windows 分叉断言（连接管控十一期）。平台分叉依据：
// Windows 下 Go os.Open 句柄不透出 FILE_SHARE_DELETE，写侧 rename 被打开的源档
// 报 sharing violation（轮转停链降级）；Unix rename 只操作目录项不受打开句柄
// 阻碍（轮转成功）——两平台行为互斥，平台耦合断言按 linux-platform 规则拆
// _windows_test.go/_linux_test.go，禁共享一份。
package service

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAuditLogger_RotateWhileOpenForRead 读写竞速（Windows 实测形态）：读侧持有
// 档句柄期间写侧轮转，Windows 对被打开源 rename 报 sharing violation——轮转停链
// 告警、主文件照常追加（不丢行），读侧句柄释放后下一行写入重试轮转成功（自愈）。
// 验证降级口径与自愈闭环，而非竞速无感通过。
func TestAuditLogger_RotateWhileOpenForRead(t *testing.T) {
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

	a.Log(AuditEventAuthSuccess, "tok-c", "", "127.0.0.1:1", "ua") // 轮转失败停链（.1 源被打开）

	if _, err := os.Stat(path + ".2"); !os.IsNotExist(err) {
		t.Fatal("停链后不应产生 .2")
	}
	if got := readAuditLines(t, path+".1"); len(got) != 1 || got[0].TokenID != "tok-a" {
		t.Errorf("停链后 .1 应保持原状: %+v", got)
	}
	if got := readAuditLines(t, path); len(got) != 2 || got[1].TokenID != "tok-c" {
		t.Errorf("停链后本行应照常追加主文件（不丢行，含轮转前 tok-b 共两行）: %+v", got)
	}

	// 句柄释放后下一行写入重试轮转：链恢复（自愈）
	if err := f.Close(); err != nil {
		t.Fatalf("关闭句柄: %v", err)
	}
	a.Log(AuditEventAuthSuccess, "tok-d", "", "127.0.0.1:1", "ua")
	if got := readAuditLines(t, path+".2"); len(got) != 1 || got[0].TokenID != "tok-a" {
		t.Errorf("自愈轮转后 .1 应顺移为 .2: %+v", got)
	}
	if got := readAuditLines(t, path+".1"); len(got) != 2 || got[1].TokenID != "tok-c" {
		t.Errorf("自愈轮转后主文件（两行）应轮转为 .1: %+v", got)
	}
	if got := readAuditLines(t, path); len(got) != 1 || got[0].TokenID != "tok-d" {
		t.Errorf("自愈轮转后主文件应继续追加: %+v", got)
	}
}
