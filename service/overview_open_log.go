package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// OverviewOpenEntry 是 data/overview_opens.log 的单行结构（JSONL）。
// Ts 为 Unix 毫秒时间戳，Date 为本地时区 YYYY-MM-DD；两者由同一次 time.Now()
// 生成，保证跨自然日边界时归段口径一致（T4 判定按 Date 聚合 W1/W2 两段）。
// Source 只取三个显式入口，见 validOverviewOpenSource。
type OverviewOpenEntry struct {
	Ts     int64  `json:"ts"`
	Date   string `json:"date"`
	Source string `json:"source"`
}

// OverviewOpenLog 是 T4 Go/No-Go 门槛取证的极轻量埋点日志器：
// 独立 O_APPEND 单行追加，不复用 AuditLogger / AuditRecord 闭集（避免污染
// QueryAuditLog 读侧）。只记录「概览面板进入」，不上主看板卡片、不扩到其他面板。
//
// 判 Go 后 T5 须直接复用本日志器扩展，不得另起一套计数定义。
type OverviewOpenLog struct {
	mu   sync.Mutex
	path string
}

// NewOverviewOpenLog 构造埋点日志器，path 由调用方注入（生产为
// data/overview_opens.log，单测为 t.TempDir() 下的临时路径）。构造纯 new，
// 不启动 goroutine、不预建文件：首次 Record 时才创建父目录与文件。
func NewOverviewOpenLog(path string) *OverviewOpenLog {
	return &OverviewOpenLog{path: path}
}

// validOverviewOpenSource 是 G1 计数口径的唯一守卫：仅点击图标（click）、
// 快捷键（shortcut）、Alt+数字（alt）三个显式入口计入。会话恢复、切档、
// 手动刷新、TTL 命中、下钻切走均不传源，故不落盘。
func validOverviewOpenSource(source string) bool {
	switch source {
	case "click", "shortcut", "alt":
		return true
	default:
		return false
	}
}

// Record 追加一条概览面板主动打开记录。非法 source 直接丢弃；写失败仅 Warn
// 降级、不返回错误——埋点是旁路观测面，任何情况下不得阻断概览面板加载。
func (l *OverviewOpenLog) Record(source string) {
	if !validOverviewOpenSource(source) {
		return
	}

	now := time.Now()
	line, err := json.Marshal(OverviewOpenEntry{
		Ts:     now.UnixMilli(),
		Date:   now.Format("2006-01-02"),
		Source: source,
	})
	if err != nil {
		// 全为基本类型字段不会失败，防御保留
		Logger().Warn("marshal overview open record failed", "err", err)
		return
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	// 父目录不存在时自动创建；裸目录创建失败由下方 open 兜底告警
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		Logger().Warn("create overview open log dir failed", "path", l.path, "err", err)
		return
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		Logger().Warn("open overview open log failed", "path", l.path, "err", err)
		return
	}
	if _, err := f.Write(line); err != nil {
		Logger().Warn("write overview open log failed", "path", l.path, "err", err)
	}
	_ = f.Close()
}

// ReadAll 只读解析全部记录，供判定日按 Date 聚合（或 T5 复用）使用。
// 文件不存在返回空切片与 nil error；坏行跳过、空行跳过，与审计日志读侧容错一致。
func (l *OverviewOpenLog) ReadAll() ([]OverviewOpenEntry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.Open(l.path)
	if os.IsNotExist(err) {
		return []OverviewOpenEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	entries := make([]OverviewOpenEntry, 0)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e OverviewOpenEntry
		if json.Unmarshal(line, &e) != nil || e.Date == "" || e.Source == "" {
			continue
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}
