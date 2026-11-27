package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newOverviewOpenLog(t *testing.T) (*OverviewOpenLog, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "overview_opens.log")
	return NewOverviewOpenLog(path), path
}

func TestOverviewOpenLog_RecordAndReadAll(t *testing.T) {
	log, _ := newOverviewOpenLog(t)
	wantDate := time.Now().Format("2006-01-02")

	for _, source := range []string{"click", "shortcut", "alt"} {
		log.Record(source)
	}

	entries, err := log.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll 返回错误: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("记录条数 = %d，期望 3", len(entries))
	}
	for i, e := range entries {
		if e.Source != []string{"click", "shortcut", "alt"}[i] {
			t.Errorf("第 %d 条 source = %q", i, e.Source)
		}
		if e.Date != wantDate {
			t.Errorf("第 %d 条 date = %q，期望本地日期 %q", i, e.Date, wantDate)
		}
		if e.Ts <= 0 {
			t.Errorf("第 %d 条 ts = %d，期望正数 Unix 毫秒", i, e.Ts)
		}
		// Ts 与 Date 必须出自同一时刻，避免跨自然日边界归段漂移
		if got := time.UnixMilli(e.Ts).Format("2006-01-02"); got != e.Date {
			t.Errorf("第 %d 条 ts 推导日期 %q 与 date %q 不一致", i, got, e.Date)
		}
	}
}

func TestOverviewOpenLog_RestartDoesNotLoseRecords(t *testing.T) {
	_, path := newOverviewOpenLog(t)
	first := NewOverviewOpenLog(path)
	first.Record("click")
	first.Record("shortcut")

	// 重新构造实例等价于应用重启：既保留旧行，也能继续追加
	restarted := NewOverviewOpenLog(path)
	before, err := restarted.ReadAll()
	if err != nil {
		t.Fatalf("重启后 ReadAll 返回错误: %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("重启后记录条数 = %d，期望 2（重启不丢）", len(before))
	}
	restarted.Record("alt")

	after, err := NewOverviewOpenLog(path).ReadAll()
	if err != nil {
		t.Fatalf("再次重启 ReadAll 返回错误: %v", err)
	}
	if len(after) != 3 {
		t.Fatalf("追加后记录条数 = %d，期望 3", len(after))
	}
	if after[2].Source != "alt" {
		t.Errorf("追加行 source = %q，期望 alt", after[2].Source)
	}
}

func TestOverviewOpenLog_InvalidSourceNotRecorded(t *testing.T) {
	log, path := newOverviewOpenLog(t)

	// 会话恢复、切档、手刷、TTL 命中、下钻切走均传空源或非法源；大小写不符也不落盘
	for _, source := range []string{"", "restore", "refresh", "ttl", "drill", "CLICK"} {
		log.Record(source)
	}

	entries, err := log.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll 返回错误: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("非法 source 落盘了 %d 条记录", len(entries))
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("非法 source 不应创建日志文件，Stat 结果 err = %v", err)
	}
}

func TestOverviewOpenLog_ConcurrentAppend(t *testing.T) {
	log, path := newOverviewOpenLog(t)
	const n = 64

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			switch i % 3 {
			case 0:
				log.Record("click")
			case 1:
				log.Record("shortcut")
			default:
				log.Record("alt")
			}
		}(i)
	}
	wg.Wait()

	entries, err := log.ReadAll()
	if err != nil {
		t.Fatalf("并发追加后 ReadAll 返回错误: %v", err)
	}
	if len(entries) != n {
		t.Fatalf("并发追加后记录条数 = %d，期望 %d", len(entries), n)
	}
	for i, e := range entries {
		if !validOverviewOpenSource(e.Source) {
			t.Errorf("第 %d 条 source = %q 非法（并发写串行化失效）", i, e.Source)
		}
	}

	// 逐行校验物理文件：O_APPEND 单行写不得出现半行或交错行
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读物理文件失败: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != n {
		t.Fatalf("物理行数 = %d，期望 %d", len(lines), n)
	}
	for i, line := range lines {
		var e OverviewOpenEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("第 %d 行不是完整 JSON: %v（内容 %q）", i, err, line)
		}
	}
}

func TestOverviewOpenLog_ReadAllSkipsMalformedLines(t *testing.T) {
	_, path := newOverviewOpenLog(t)
	content := strings.Join([]string{
		`{"ts":1700000000000,"date":"2026-11-30","source":"click"}`,
		``,
		`{broken json`,
		`{"ts":1700000000001,"source":"alt"}`, // 缺 date
		`{"ts":1700000000002,"date":"2026-12-07"}`,
		`{"ts":1700000000003,"date":"2026-12-07","source":"shortcut"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入测试日志失败: %v", err)
	}

	entries, err := NewOverviewOpenLog(path).ReadAll()
	if err != nil {
		t.Fatalf("ReadAll 返回错误: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("有效记录条数 = %d，期望 2（坏行与缺字段行须跳过）", len(entries))
	}
	if entries[0].Date != "2026-11-30" || entries[0].Source != "click" {
		t.Errorf("首条 = %+v", entries[0])
	}
	if entries[1].Date != "2026-12-07" || entries[1].Source != "shortcut" {
		t.Errorf("末条 = %+v", entries[1])
	}
}

func TestOverviewOpenLog_ReadAllMissingFile(t *testing.T) {
	log, _ := newOverviewOpenLog(t)
	entries, err := log.ReadAll()
	if err != nil {
		t.Fatalf("文件不存在时 ReadAll 不应报错: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("文件不存在时应返回空切片，实际 %d 条", len(entries))
	}
}

func TestOverviewOpenLog_WriteFailureDegrades(t *testing.T) {
	// 父路径是普通文件而非目录：MkdirAll 失败，Record 只告警不得 panic
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("准备阻断文件失败: %v", err)
	}
	log := NewOverviewOpenLog(filepath.Join(blocker, "overview_opens.log"))
	log.Record("click")

	entries, err := log.ReadAll()
	if err != nil {
		t.Fatalf("写入失败后 ReadAll 不应报错: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("写入失败后不应有记录，实际 %d 条", len(entries))
	}
}
