package model

import (
	"encoding/json"
	"strings"
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
			Visible:     true,
			Height:      240,
			Tabs:        []TerminalTabSnapshot{{WorkDir: "D:/workspace/repo", ShellType: "powershell"}, {WorkDir: "D:/workspace/lib", ShellType: "cmd"}},
			ActiveIndex: 1,
			Fullscreen:  true,
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
	if len(restored.Terminal.Tabs) != 2 {
		t.Fatalf("Terminal.Tabs: got %d tabs, want 2", len(restored.Terminal.Tabs))
	}
	if restored.Terminal.Tabs[0].WorkDir != "D:/workspace/repo" || restored.Terminal.Tabs[0].ShellType != "powershell" {
		t.Errorf("Terminal.Tabs[0]: got %+v", restored.Terminal.Tabs[0])
	}
	if restored.Terminal.Tabs[1].WorkDir != "D:/workspace/lib" || restored.Terminal.Tabs[1].ShellType != "cmd" {
		t.Errorf("Terminal.Tabs[1]: got %+v", restored.Terminal.Tabs[1])
	}
	if restored.Terminal.ActiveIndex != 1 {
		t.Errorf("Terminal.ActiveIndex: got %d, want 1", restored.Terminal.ActiveIndex)
	}
	if !restored.Terminal.Fullscreen {
		t.Error("Terminal.Fullscreen: got false, want true")
	}
	if restored.Version != SessionStateVersion {
		t.Errorf("Version: got %q, want %s", restored.Version, SessionStateVersion)
	}
	if restored.SavedAt != 1757400000 {
		t.Errorf("SavedAt: got %d, want 1757400000", restored.SavedAt)
	}
}

// TestTerminalSnapshot_ActiveIndexZeroOmitLossless 验证 ActiveIndex=0 时 omitempty 省略字段，
// 反序列化仍得 0（零值无损，恢复第 0 个 tab 不受影响）。
func TestTerminalSnapshot_ActiveIndexZeroOmitLossless(t *testing.T) {
	original := &SessionState{
		Terminal: &TerminalSnapshot{
			Visible:     true,
			Height:      200,
			Tabs:        []TerminalTabSnapshot{{WorkDir: "D:/repo"}},
			ActiveIndex: 0,
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) == "" || !json.Valid(data) {
		t.Fatalf("marshal produced invalid json: %s", data)
	}

	var restored SessionState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if restored.Terminal == nil || restored.Terminal.ActiveIndex != 0 {
		t.Errorf("ActiveIndex zero should round-trip lossless, got %+v", restored.Terminal)
	}
}

// TestTerminalSnapshot_EmptyTabsSliceOmit 验证空 tabs 数组（前端无 tab 时恒发 tabs: []）
// 经 omitempty 序列化后字段整体省略（与 nil 同形态），反序列化还原为 nil，
// NormalizedTabs 不得据此降级出幽灵 tab。
func TestTerminalSnapshot_EmptyTabsSliceOmit(t *testing.T) {
	original := &SessionState{
		Terminal: &TerminalSnapshot{
			Visible: true,
			Height:  200,
			Tabs:    []TerminalTabSnapshot{},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"tabs"`) {
		t.Errorf("空 tabs 数组应被 omitempty 省略, got %s", data)
	}

	var restored SessionState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if restored.Terminal == nil {
		t.Fatal("Terminal should not be nil")
	}
	if restored.Terminal.Tabs != nil {
		t.Errorf("空 tabs 反序列化应为 nil, got %+v", restored.Terminal.Tabs)
	}
	if got := restored.Terminal.NormalizedTabs(); got != nil {
		t.Errorf("空 tabs 不得降级出幽灵 tab, got %+v", got)
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
// 升版 "2" 对应多终端 tab 快照结构，降级路径见 SessionStateVersion 注释。
func TestSessionState_VersionConstant(t *testing.T) {
	if SessionStateVersion != "2" {
		t.Errorf("SessionStateVersion: got %q, want 2", SessionStateVersion)
	}
}

// TestTerminalSnapshot_NormalizedTabs 归一化三分支：
// v2 快照（Tabs 非空）直接返回且忽略旧 WorkDir；v1 旧快照降级单 tab；两者皆空返回 nil。
func TestTerminalSnapshot_NormalizedTabs(t *testing.T) {
	t.Run("v2快照Tabs非空时优先且忽略WorkDir", func(t *testing.T) {
		snap := &TerminalSnapshot{
			WorkDir: "D:/legacy",
			Tabs:    []TerminalTabSnapshot{{WorkDir: "D:/a", ShellType: "cmd"}},
		}
		got := snap.NormalizedTabs()
		if len(got) != 1 || got[0].WorkDir != "D:/a" || got[0].ShellType != "cmd" {
			t.Errorf("Tabs 非空应以 Tabs 为准, got %+v", got)
		}
	})

	t.Run("v1旧快照降级为单tab且ShellType留空", func(t *testing.T) {
		snap := &TerminalSnapshot{Visible: true, Height: 240, WorkDir: "D:/workspace/repo"}
		got := snap.NormalizedTabs()
		if len(got) != 1 {
			t.Fatalf("旧快照应降级为 1 个 tab, got %d", len(got))
		}
		if got[0].WorkDir != "D:/workspace/repo" {
			t.Errorf("降级 tab WorkDir: got %q", got[0].WorkDir)
		}
		if got[0].ShellType != "" {
			t.Errorf("降级 tab ShellType 应留空由前端回退默认值, got %q", got[0].ShellType)
		}
	})

	t.Run("全空返回nil无可恢复终端", func(t *testing.T) {
		snap := &TerminalSnapshot{Visible: true, Height: 200}
		if got := snap.NormalizedTabs(); got != nil {
			t.Errorf("无可恢复终端应返回 nil, got %+v", got)
		}
	})

	t.Run("nil接收者安全返回nil", func(t *testing.T) {
		var snap *TerminalSnapshot
		if got := snap.NormalizedTabs(); got != nil {
			t.Errorf("nil 接收者应返回 nil, got %+v", got)
		}
	})

	t.Run("归一化幂等", func(t *testing.T) {
		snap := &TerminalSnapshot{WorkDir: "D:/legacy"}
		once := snap.NormalizedTabs()
		normalized := &TerminalSnapshot{Tabs: once}
		twice := normalized.NormalizedTabs()
		if len(twice) != 1 || twice[0].WorkDir != "D:/legacy" {
			t.Errorf("归一化应幂等, got %+v", twice)
		}
	})
}
