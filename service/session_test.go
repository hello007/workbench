package service

import (
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

// TestSessionLoad_Valid 合法快照正确反序列化。
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
	if !got.Terminal.Visible || got.Terminal.Height != 240 || got.Terminal.WorkDir != "D:/repo" {
		t.Errorf("Terminal 未正确还原: %+v", got.Terminal)
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

// TestSessionSave_RoundTrip 保存后新建服务重新加载，内容一致。
func TestSessionSave_RoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "session.json")
	svc1 := NewSessionService(p)
	want := &model.SessionState{
		SelectedDirectoryID: "dir-2",
		ActivePanel:         "stats",
		Terminal:            &model.TerminalSnapshot{Visible: false, Height: 200, WorkDir: ""},
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
