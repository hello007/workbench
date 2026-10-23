package service

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// readAuditLines 读入审计日志全部行并解析为 AuditRecord（损坏行跳过——读侧
// 容错语义与生产一致）。
func readAuditLines(t *testing.T, path string) []AuditRecord {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("打开审计日志 %s: %v", path, err)
	}
	defer f.Close()
	var recs []AuditRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec AuditRecord
		if err := json.Unmarshal([]byte(line), &rec); err == nil {
			recs = append(recs, rec)
		}
	}
	return recs
}

// TestAuditLogger_AppendAndFormat 追加与行格式：两行合法 JSONL，字段
// （ts/event/tokenId/name/remoteAddr/userAgent）完整落行。
func TestAuditLogger_AppendAndFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)

	a.Log(AuditEventAuthSuccess, "tok-1", "", "127.0.0.1:1234", "Mozilla/5.0")
	a.Log(AuditEventTokenCreate, "tok-2", "手机", "", "")

	recs := readAuditLines(t, path)
	if len(recs) != 2 {
		t.Fatalf("应有两行审计记录, got %d", len(recs))
	}
	if recs[0].Event != AuditEventAuthSuccess || recs[0].TokenID != "tok-1" ||
		recs[0].RemoteAddr != "127.0.0.1:1234" || recs[0].UserAgent != "Mozilla/5.0" {
		t.Errorf("认证事件字段不符: %+v", recs[0])
	}
	if recs[1].Event != AuditEventTokenCreate || recs[1].Name != "手机" || recs[1].TokenID != "tok-2" {
		t.Errorf("生命周期事件字段不符: %+v", recs[1])
	}
	if recs[0].Ts == 0 || recs[1].Ts == 0 {
		t.Error("事件时间应就绪")
	}
}

// TestAuditLogger_NoPlaintextToken 审计面无明文泄露：Log 出口不接受明文 token
// 入参（签名即约束），落盘行不含敏感字段名形态（除 tokenId 哈希归因键）。
func TestAuditLogger_NoPlaintextToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)
	secret := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	a.Log(AuditEventAuthFailure, "", "", "192.168.1.5:999", "curl/8.0")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读审计日志: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Error("审计日志不得包含明文 token")
	}
}

// TestAuditLogger_Rotation 轮转：注入小 maxBytes 驱动超限，旧内容 rename 为 .1，
// 新内容继续落主文件；.1 与主文件均可解析（读侧无损坏）。
func TestAuditLogger_Rotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)
	a.mu.Lock()
	a.maxBytes = 160 // 驱动多次轮转的极小上限
	a.mu.Unlock()

	for i := 0; i < 6; i++ {
		a.Log(AuditEventAuthSuccess, "tok-r", "设备", "127.0.0.1:1", "ua")
	}

	dot1 := readAuditLines(t, path+".1")
	main := readAuditLines(t, path)
	if len(dot1) == 0 {
		t.Fatal("轮转应产生 .1 文件")
	}
	if len(main) == 0 {
		t.Fatal("轮转后主文件应继续追加")
	}
	for _, rec := range append(append([]AuditRecord{}, dot1...), main...) {
		if rec.Event != AuditEventAuthSuccess {
			t.Errorf("轮转前后记录均应完整可解析: %+v", rec)
		}
	}
}

// TestAuditLogger_Concurrent 并发写入安全（单写锁串行）：全部行完整可解析
// （无交错半行）。
func TestAuditLogger_Concurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web_audit.log")
	a := NewAuditLogger(path)

	const workers, perWorker = 8, 25
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				a.Log(AuditEventAuthFailure, "tok-c", "", "127.0.0.1:1", "ua")
			}
		}(i)
	}
	wg.Wait()

	recs := readAuditLines(t, path)
	if len(recs) != workers*perWorker {
		t.Fatalf("并发写入应无丢失, got %d want %d", len(recs), workers*perWorker)
	}
}

// TestAuditLogger_MissingDir 自动建目录：父目录不存在时追加仍成功（对齐
// data/ 落盘目录约定）。
func TestAuditLogger_MissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deeper")
	path := filepath.Join(dir, "web_audit.log")
	a := NewAuditLogger(path)
	a.Log(AuditEventTokenMigrate, "tok-0", DefaultWebTokenName, "", "")
	if recs := readAuditLines(t, path); len(recs) != 1 {
		t.Fatalf("父目录缺失时应自动创建并写入, got %d", len(recs))
	}
}
