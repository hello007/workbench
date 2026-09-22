package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// newSessionSvc 创建指向临时快照文件的 SessionService。
func newSessionSvc(t *testing.T) *SessionService {
	t.Helper()
	return NewSessionService(filepath.Join(t.TempDir(), "session.json"))
}

// TestSessionLoad_NotExists 快照文件不存在时返回空状态且不报错（冷启动降级）。
func TestSessionLoad_NotExists(t *testing.T) {
	svc := newSessionSvc(t)
	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load 不存在文件不应报错: %v", err)
	}
	if got == nil {
		t.Fatal("Load 不应返回 nil")
	}
	if got.SelectedDirectoryID != "" || got.ActivePanel != "" {
		t.Errorf("空快照字段应为零值, got %+v", got)
	}
}

// TestSessionLoad_Valid 合法快照正确反序列化，且终端快照归一化为多 tab 形态。
func TestSessionLoad_Valid(t *testing.T) {
	svc := newSessionSvc(t)
	want := &model.SessionState{
		SelectedDirectoryID: "dir-1",
		ActivePanel:         "ai",
		Terminal: &model.TerminalSnapshot{
			Visible: true,
			Height:  240,
			WorkDir: "D:/repo",
		},
	}
	if err := svc.Save(want); err != nil {
		t.Fatalf("Save 前置: %v", err)
	}

	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SelectedDirectoryID != "dir-1" {
		t.Errorf("SelectedDirectoryID: got %q, want dir-1", got.SelectedDirectoryID)
	}
	if got.ActivePanel != "ai" {
		t.Errorf("ActivePanel: got %q, want ai", got.ActivePanel)
	}
	if got.Terminal == nil {
		t.Fatal("Terminal should not be nil after round-trip")
	}
	if !got.Terminal.Visible || got.Terminal.Height != 240 {
		t.Errorf("Terminal 未正确还原: %+v", got.Terminal)
	}
	// Load 归一化：旧形态 workDir 单值写回为单元素 Tabs，前端只消费统一形态；
	// 遗留 WorkDir 消费后清除，Load→Save 落盘为纯 v2 形态
	if got.Terminal.WorkDir != "" {
		t.Errorf("归一化应消费清除遗留 WorkDir, got %q", got.Terminal.WorkDir)
	}
	if len(got.Terminal.Tabs) != 1 || got.Terminal.Tabs[0].WorkDir != "D:/repo" {
		t.Errorf("Load 应将旧形态归一化为单 tab, got %+v", got.Terminal.Tabs)
	}
}

// TestSessionLoad_LegacySnapshotDowngrade 旧版 v1 单终端快照（workDir 单值）加载时
// 降级为单元素 Tabs（ShellType 留空，前端回退设置页默认 shell），向后兼容不丢现场。
func TestSessionLoad_LegacySnapshotDowngrade(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session.json")
	legacy := `{"selectedDirectoryId":"dir-1","activePanel":"directory",` +
		`"terminal":{"visible":true,"height":240,"workDir":"D:/workspace/repo"},` +
		`"version":"1","savedAt":1757400000}`
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatalf("写入旧版快照前置: %v", err)
	}
	svc := NewSessionService(p)

	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != "1" {
		t.Errorf("旧版快照 Version 应保留原值 1, got %q", got.Version)
	}
	if got.Terminal == nil {
		t.Fatal("Terminal should not be nil")
	}
	if !got.Terminal.Visible || got.Terminal.Height != 240 {
		t.Errorf("终端面板状态未还原: %+v", got.Terminal)
	}
	if len(got.Terminal.Tabs) != 1 {
		t.Fatalf("旧快照应降级为 1 个 tab, got %d", len(got.Terminal.Tabs))
	}
	if got.Terminal.Tabs[0].WorkDir != "D:/workspace/repo" {
		t.Errorf("降级 tab WorkDir: got %q, want D:/workspace/repo", got.Terminal.Tabs[0].WorkDir)
	}
	if got.Terminal.Tabs[0].ShellType != "" {
		t.Errorf("降级 tab ShellType 应留空, got %q", got.Terminal.Tabs[0].ShellType)
	}
}

// TestSessionSave_LegacyDowngradeRoundTrip 旧版 v1 快照 Load 归一化后 Save 落盘 v2 形态，
// 再次 Load 幂等（tab 列表不变、不重复降级）：验证「Load 归一化写回，下次 Save 即 v2」链路。
func TestSessionSave_LegacyDowngradeRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session.json")
	legacy := `{"selectedDirectoryId":"dir-1","activePanel":"directory",` +
		`"terminal":{"visible":true,"height":240,"workDir":"D:/workspace/repo"},` +
		`"version":"1","savedAt":1757400000}`
	if err := os.WriteFile(p, []byte(legacy), 0o644); err != nil {
		t.Fatalf("写入旧版快照前置: %v", err)
	}
	svc := NewSessionService(p)

	// 第一次 Load：v1 降级为单 tab
	first, err := svc.Load()
	if err != nil {
		t.Fatalf("第一次 Load: %v", err)
	}
	if err := svc.Save(first); err != nil {
		t.Fatalf("归一化结果写回 Save: %v", err)
	}

	// 落盘 JSON 应为纯 v2 形态：tabs 数组承载目录，终端级遗留 workDir 不再写入
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读回落盘文件: %v", err)
	}
	var rawState map[string]any
	if err := json.Unmarshal(raw, &rawState); err != nil {
		t.Fatalf("落盘 JSON 解析: %v", err)
	}
	if rawState["version"] != model.SessionStateVersion {
		t.Errorf("落盘版本应为当前版本号, got %v", rawState["version"])
	}
	term, ok := rawState["terminal"].(map[string]any)
	if !ok {
		t.Fatalf("落盘 terminal 缺失: %v", rawState)
	}
	if _, exists := term["workDir"]; exists {
		t.Errorf("遗留 workDir 归一化后不应再落盘: %v", term)
	}
	tabs, ok := term["tabs"].([]any)
	if !ok || len(tabs) != 1 {
		t.Fatalf("落盘应为 v2 tabs 形态: %v", term)
	}

	// 第二次 Load：幂等，tab 列表与首次归一化结果一致
	second, err := svc.Load()
	if err != nil {
		t.Fatalf("第二次 Load: %v", err)
	}
	if second.Terminal == nil || len(second.Terminal.Tabs) != 1 ||
		second.Terminal.Tabs[0].WorkDir != "D:/workspace/repo" ||
		second.Terminal.Tabs[0].ShellType != "" {
		t.Errorf("二次 Load 应幂等还原单 tab, got %+v", second.Terminal)
	}
}

// TestSessionLoad_MultiTabSnapshot 多 tab 快照（tabs/activeIndex/fullscreen）正确还原。
func TestSessionLoad_MultiTabSnapshot(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session.json")
	v2 := `{"selectedDirectoryId":"dir-2",` +
		`"terminal":{"visible":true,"height":300,` +
		`"tabs":[{"workDir":"D:/a","shellType":"powershell"},{"workDir":"D:/b","shellType":"cmd"}],` +
		`"activeIndex":1,"fullscreen":true},` +
		`"version":"2","savedAt":1757400001}`
	if err := os.WriteFile(p, []byte(v2), 0o644); err != nil {
		t.Fatalf("写入 v2 快照前置: %v", err)
	}
	svc := NewSessionService(p)

	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Terminal == nil {
		t.Fatal("Terminal should not be nil")
	}
	if len(got.Terminal.Tabs) != 2 {
		t.Fatalf("应还原 2 个 tab, got %d", len(got.Terminal.Tabs))
	}
	if got.Terminal.Tabs[0] != (model.TerminalTabSnapshot{WorkDir: "D:/a", ShellType: "powershell"}) {
		t.Errorf("Tabs[0] 未正确还原: %+v", got.Terminal.Tabs[0])
	}
	if got.Terminal.Tabs[1] != (model.TerminalTabSnapshot{WorkDir: "D:/b", ShellType: "cmd"}) {
		t.Errorf("Tabs[1] 未正确还原: %+v", got.Terminal.Tabs[1])
	}
	if got.Terminal.ActiveIndex != 1 {
		t.Errorf("ActiveIndex: got %d, want 1", got.Terminal.ActiveIndex)
	}
	if !got.Terminal.Fullscreen {
		t.Error("Fullscreen: got false, want true")
	}
}

// TestSessionLoad_TabsPriorityOverLegacyWorkDir Tabs 非空时归一化以新结构为准，
// 遗留 workDir 单值被忽略（防御手工编辑或异常数据导致的字段并存）。
func TestSessionLoad_TabsPriorityOverLegacyWorkDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session.json")
	mixed := `{"terminal":{"visible":true,"height":200,"workDir":"D:/legacy",` +
		`"tabs":[{"workDir":"D:/new","shellType":"gitbash"}]}}`
	if err := os.WriteFile(p, []byte(mixed), 0o644); err != nil {
		t.Fatalf("写入混合快照前置: %v", err)
	}
	svc := NewSessionService(p)

	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Terminal == nil || len(got.Terminal.Tabs) != 1 || got.Terminal.Tabs[0].WorkDir != "D:/new" {
		t.Errorf("Tabs 非空应以 Tabs 为准, got %+v", got.Terminal)
	}
}

// TestSessionLoad_InvalidJSON 损坏 JSON 时吞错返回空状态（不向上抛，降级冷启动）。
func TestSessionLoad_InvalidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(p, []byte("{not json"), 0o644)
	svc := NewSessionService(p)

	got, err := svc.Load()
	if err != nil {
		t.Fatalf("损坏 JSON 应吞错返回 nil 错误, got %v", err)
	}
	if got == nil || got.SelectedDirectoryID != "" {
		t.Errorf("损坏 JSON 应回退空状态, got %+v", got)
	}
}

// TestSessionSave_RoundTrip 保存后新建服务重新加载，内容一致（含多 tab 终端快照）。
func TestSessionSave_RoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session.json")
	svc1 := NewSessionService(p)
	want := &model.SessionState{
		SelectedDirectoryID: "dir-2",
		ActivePanel:         "stats",
		Terminal: &model.TerminalSnapshot{
			Visible:     true,
			Height:      260,
			Tabs:        []model.TerminalTabSnapshot{{WorkDir: "D:/a", ShellType: "powershell"}, {WorkDir: "D:/b", ShellType: "wsl"}},
			ActiveIndex: 1,
			Fullscreen:  true,
		},
	}
	if err := svc1.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	svc2 := NewSessionService(p)
	got, err := svc2.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if got.SelectedDirectoryID != "dir-2" {
		t.Errorf("round-trip SelectedDirectoryID: got %q, want dir-2", got.SelectedDirectoryID)
	}
	if got.ActivePanel != "stats" {
		t.Errorf("round-trip ActivePanel: got %q, want stats", got.ActivePanel)
	}
	if got.Terminal == nil || len(got.Terminal.Tabs) != 2 {
		t.Fatalf("round-trip Terminal.Tabs 未正确还原: %+v", got.Terminal)
	}
	if got.Terminal.Tabs[1].WorkDir != "D:/b" || got.Terminal.Tabs[1].ShellType != "wsl" {
		t.Errorf("round-trip Tabs[1]: got %+v", got.Terminal.Tabs[1])
	}
	if got.Terminal.ActiveIndex != 1 || !got.Terminal.Fullscreen {
		t.Errorf("round-trip ActiveIndex/Fullscreen: got %d/%v", got.Terminal.ActiveIndex, got.Terminal.Fullscreen)
	}
}

// TestSessionSave_SetsVersionAndTimestamp 验证 Save 写入前补版本号与时间戳，
// 保证快照自描述（便于未来迁移识别旧版本）。
func TestSessionSave_SetsVersionAndTimestamp(t *testing.T) {
	svc := newSessionSvc(t)
	// 传入不含 version/savedAt 的快照
	if err := svc.Save(&model.SessionState{ActivePanel: "directory"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Version != model.SessionStateVersion {
		t.Errorf("Version: got %q, want %s", got.Version, model.SessionStateVersion)
	}
	if got.SavedAt <= 0 {
		t.Errorf("SavedAt should be positive timestamp, got %d", got.SavedAt)
	}
}

// TestSessionSave_NilStateNoPanic 验证 Save(nil) 不 panic，写入空快照。
func TestSessionSave_NilStateNoPanic(t *testing.T) {
	svc := newSessionSvc(t)
	if err := svc.Save(nil); err != nil {
		t.Fatalf("Save(nil) 不应报错: %v", err)
	}
	got, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SelectedDirectoryID != "" {
		t.Errorf("Save(nil) 应写入空快照, got %+v", got)
	}
	// 即使 nil，Save 也应补版本号
	if got.Version != model.SessionStateVersion {
		t.Errorf("Version: got %q, want %s", got.Version, model.SessionStateVersion)
	}
}
