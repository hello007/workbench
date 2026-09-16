package service

import (
	"os"
	"path/filepath"
	"testing"

	"workbench/util/testutil"
)

// newTestDashboardService 创建用临时配置路径的 DashboardService。
func newTestDashboardService(t *testing.T) *DashboardService {
	t.Helper()
	return NewDashboardService(filepath.Join(t.TempDir(), "dashboard_pinned.json"))
}

func TestDashboardService_AddPin_PersistsAndNormalizes(t *testing.T) {
	svc := newTestDashboardService(t)
	dir := testutil.InitTempRepo(t) // 提供一个真实路径，确保 Abs 可规范

	if err := svc.AddPin(dir); err != nil {
		t.Fatalf("AddPin failed: %v", err)
	}
	// 路径规范化为绝对路径后持久化
	pinned := svc.LoadPinned()
	if len(pinned) != 1 {
		t.Fatalf("expected 1 pinned, got %d", len(pinned))
	}
	abs, _ := filepath.Abs(dir)
	if pinned[0] != abs {
		t.Errorf("expected normalized %q, got %q", abs, pinned[0])
	}
}

func TestDashboardService_AddPin_Idempotent(t *testing.T) {
	svc := newTestDashboardService(t)
	dir := testutil.InitTempRepo(t)

	if err := svc.AddPin(dir); err != nil {
		t.Fatalf("first AddPin failed: %v", err)
	}
	// 同路径加相对路径变体（应规范化后去重）
	rel, _ := filepath.Rel(t.TempDir(), dir)
	if rel != "" {
		// rel 基于另一个 TempDir，可能无意义；改为直接重复加同路径验证幂等
	}
	if err := svc.AddPin(dir); err != nil {
		t.Fatalf("second AddPin failed: %v", err)
	}
	if got := len(svc.LoadPinned()); got != 1 {
		t.Errorf("expected 1 after duplicate add, got %d", got)
	}
}

func TestDashboardService_RemovePin(t *testing.T) {
	svc := newTestDashboardService(t)
	dir := testutil.InitTempRepo(t)

	svc.AddPin(dir)
	svc.AddPin(testutil.InitTempRepo(t)) // 第二个仓

	if err := svc.RemovePin(dir); err != nil {
		t.Fatalf("RemovePin failed: %v", err)
	}
	pinned := svc.LoadPinned()
	if len(pinned) != 1 {
		t.Fatalf("expected 1 after remove, got %d", len(pinned))
	}
	abs, _ := filepath.Abs(dir)
	for _, p := range pinned {
		if p == abs {
			t.Errorf("removed path still present: %q", abs)
		}
	}
}

func TestDashboardService_RemovePin_Idempotent(t *testing.T) {
	svc := newTestDashboardService(t)
	// 移除不存在的路径应幂等返回 nil
	if err := svc.RemovePin("/nonexistent/path"); err != nil {
		t.Errorf("RemovePin nonexistent should be idempotent, got %v", err)
	}
}

func TestDashboardService_IsPinned(t *testing.T) {
	svc := newTestDashboardService(t)
	dir := testutil.InitTempRepo(t)
	svc.AddPin(dir)

	if !svc.IsPinned(dir) {
		t.Errorf("IsPinned should be true after AddPin")
	}
	if svc.IsPinned("/some/other/path") {
		t.Errorf("IsPinned should be false for unpinned path")
	}
}

func TestDashboardService_LoadPinned_MissingFileReturnsEmpty(t *testing.T) {
	// 配置文件不存在时应返回空列表，不报错
	svc := NewDashboardService(filepath.Join(t.TempDir(), "absent.json"))
	if got := svc.LoadPinned(); len(got) != 0 {
		t.Errorf("expected empty for missing file, got %v", got)
	}
}

func TestDashboardService_LoadPinned_CorruptFileDegradesGracefully(t *testing.T) {
	// 损坏 JSON 应降级空列表 + 不 panic（RepoMetaService 范式）
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "dashboard_pinned.json")
	if err := os.WriteFile(cfgPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	svc := NewDashboardService(cfgPath)
	if got := svc.LoadPinned(); len(got) != 0 {
		t.Errorf("expected empty for corrupt file, got %v", got)
	}
}

func TestDashboardService_GetStatuses_EmptyPinned(t *testing.T) {
	svc := newTestDashboardService(t)
	got := svc.GetStatuses()
	if len(got) != 0 {
		t.Errorf("expected empty statuses for empty pin list, got %d", len(got))
	}
}

func TestDashboardService_GetStatuses_MissingPathMarksMissing(t *testing.T) {
	svc := newTestDashboardService(t)
	svc.AddPin(filepath.Join(t.TempDir(), "deleted-repo"))
	got := svc.GetStatuses()
	if len(got) != 1 {
		t.Fatalf("expected 1 status, got %d", len(got))
	}
	if !got[0].Missing {
		t.Errorf("expected Missing=true for nonexistent path, got %+v", got[0])
	}
	if got[0].IsRepo {
		t.Errorf("expected IsRepo=false for nonexistent path")
	}
}
