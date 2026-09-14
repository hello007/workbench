package model

import (
	"encoding/json"
	"testing"
)

// TestSessionState_JSONRoundTrip 验证 SessionState 的 json tag 与 omitempty 行为，
// 保证前后端跨层契约（Go struct <-> wailsjs models.ts <-> 前端对象）字段名一致。
// 详见 docs/spec/cross-layer-contracts.md。
func TestSessionState_JSONRoundTrip(t *testing.T) {
	original := &SessionState{
		SelectedDirectoryID: "dir-1",
		ActivePanel:         "ai",
		Terminal: &TerminalSnapshot{
			Visible: true,
			Height:  240,
			WorkDir: "D:/workspace/repo",
		},
		Version: SessionStateVersion,
		SavedAt: 1757400000,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored SessionState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.SelectedDirectoryID != "dir-1" {
		t.Errorf("SelectedDirectoryID: got %q, want dir-1", restored.SelectedDirectoryID)
	}
	if restored.ActivePanel != "ai" {
		t.Errorf("ActivePanel: got %q, want ai", restored.ActivePanel)
	}
	if restored.Terminal == nil {
		t.Fatal("Terminal should not be nil after round-trip")
	}
	if !restored.Terminal.Visible {
		t.Error("Terminal.Visible: got false, want true")
	}
	if restored.Terminal.Height != 240 {
		t.Errorf("Terminal.Height: got %d, want 240", restored.Terminal.Height)
	}
	if restored.Terminal.WorkDir != "D:/workspace/repo" {
		t.Errorf("Terminal.WorkDir: got %q", restored.Terminal.WorkDir)
	}
	if restored.Version != SessionStateVersion {
		t.Errorf("Version: got %q, want %s", restored.Version, SessionStateVersion)
	}
	if restored.SavedAt != 1757400000 {
		t.Errorf("SavedAt: got %d, want 1757400000", restored.SavedAt)
	}
}

// TestSessionState_EmptyOmit 验证空快照经 omitempty 序列化后字段缺省，
// 前端据此判定为冷启动（无快照可恢复）。
func TestSessionState_EmptyOmit(t *testing.T) {
	empty := &SessionState{}
	data, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	// 空快照序列化为 "{}"，前端 restoreSession 判定无字段即走冷启动
	if string(data) != "{}" {
		t.Errorf("empty SessionState should marshal to {}, got %s", data)
	}
}

// TestSessionState_VersionConstant 验证版本常量值，防止误改破坏向后兼容识别。
func TestSessionState_VersionConstant(t *testing.T) {
	if SessionStateVersion != "1" {
		t.Errorf("SessionStateVersion: got %q, want 1", SessionStateVersion)
	}
}
