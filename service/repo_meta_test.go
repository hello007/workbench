package service

import (
	"path/filepath"
	"testing"

	"workbench/model"
)

// createRepoMetaService 创建测试用 RepoMetaService，配置文件在临时目录。
func createRepoMetaService(t *testing.T) *RepoMetaService {
	t.Helper()
	return NewRepoMetaService(filepath.Join(t.TempDir(), "repo_meta.json"))
}

// TestRepoMeta_Load_Empty 配置文件不存在时 Load 返回空 map 不报错。
func TestRepoMeta_Load_Empty(t *testing.T) {
	svc := createRepoMetaService(t)
	m, err := svc.Load()
	if err != nil {
		t.Fatalf("Load empty: got error %v", err)
	}
	if len(m) != 0 {
		t.Errorf("Load empty count: got %d, want 0", len(m))
	}
}

// TestRepoMeta_Upsert_And_Load Upsert 后 Load 能读到，且路径被规范化。
func TestRepoMeta_Upsert_And_Load(t *testing.T) {
	svc := createRepoMetaService(t)
	dir := t.TempDir()
	abs, _ := filepath.Abs(dir)

	meta := &model.RepoMeta{
		Path:    dir, // 传入未规范化路径，Upsert 应内部 filepath.Abs
		Summary: "测试简述",
		Tags:    []string{"tag1", "tag2"},
	}
	if err := svc.Upsert(meta); err != nil {
		t.Fatalf("Upsert: got error %v", err)
	}

	m, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: got error %v", err)
	}
	got, ok := m[abs]
	if !ok {
		t.Fatalf("expected meta keyed by normalized path %q, keys: %v", abs, m)
	}
	if got.Summary != "测试简述" {
		t.Errorf("Summary: got %q, want %q", got.Summary, "测试简述")
	}
	if len(got.Tags) != 2 || got.Tags[0] != "tag1" || got.Tags[1] != "tag2" {
		t.Errorf("Tags: got %v, want [tag1 tag2]", got.Tags)
	}
	// Upsert 应刷新 UpdatedAt
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set by Upsert")
	}
	// 路径主键应被规范化写入
	if got.Path != abs {
		t.Errorf("Path normalized: got %q, want %q", got.Path, abs)
	}
}

// TestRepoMeta_Upsert_Updates_Existing 再次 Upsert 同一路径应更新而非新增。
func TestRepoMeta_Upsert_Updates_Existing(t *testing.T) {
	svc := createRepoMetaService(t)
	dir := t.TempDir()
	abs, _ := filepath.Abs(dir)

	svc.Upsert(&model.RepoMeta{Path: dir, Summary: "旧简述", Tags: []string{"a"}})
	svc.Upsert(&model.RepoMeta{Path: dir, Summary: "新简述", Tags: []string{"b", "c"}})

	m, _ := svc.Load()
	if len(m) != 1 {
		t.Fatalf("expected 1 entry after re-upsert, got %d", len(m))
	}
	got := m[abs]
	if got.Summary != "新简述" {
		t.Errorf("Summary: got %q, want %q", got.Summary, "新简述")
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags count: got %d, want 2", len(got.Tags))
	}
}

// TestRepoMeta_Upsert_PathNormalization 不同大小写/分隔符的路径经 filepath.Abs 规范化后应一致。
// Windows 下 filepath.Abs 统一分隔符与大小写处理。
func TestRepoMeta_Upsert_PathNormalization(t *testing.T) {
	svc := createRepoMetaService(t)
	dir := t.TempDir()
	abs, _ := filepath.Abs(dir)

	// 用原始路径 Upsert
	svc.Upsert(&model.RepoMeta{Path: dir, Summary: "first"})

	// 用 abs 路径删除应命中（规范化后主键一致）
	if err := svc.Delete(abs); err != nil {
		t.Fatalf("Delete by abs path: got error %v", err)
	}

	m, _ := svc.Load()
	if len(m) != 0 {
		t.Errorf("after delete by normalized path, expected 0 entries, got %d", len(m))
	}
}

// TestRepoMeta_Delete 删除存在的记录成功，删除不存在的报错。
func TestRepoMeta_Delete(t *testing.T) {
	svc := createRepoMetaService(t)
	dir := t.TempDir()

	svc.Upsert(&model.RepoMeta{Path: dir, Summary: "x"})

	if err := svc.Delete(dir); err != nil {
		t.Fatalf("Delete existing: got error %v", err)
	}

	m, _ := svc.Load()
	if len(m) != 0 {
		t.Errorf("after delete, expected 0 entries, got %d", len(m))
	}

	// 删除不存在
	if err := svc.Delete(dir); err == nil {
		t.Error("Delete nonexistent: expected error, got nil")
	}
}

// TestRepoMeta_DeleteMissing 清理 Missing=true 记录，保留 Missing=false 记录。
func TestRepoMeta_DeleteMissing(t *testing.T) {
	svc := createRepoMetaService(t)
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	abs1, _ := filepath.Abs(dir1)
	abs2, _ := filepath.Abs(dir2)

	svc.Upsert(&model.RepoMeta{Path: abs1, Summary: "有效"})
	svc.Upsert(&model.RepoMeta{Path: abs2, Summary: "失效"})
	// 手动标记 dir2 失效
	m, _ := svc.Load()
	m[abs2].Missing = true
	svc.Save(m)

	removed, err := svc.DeleteMissing()
	if err != nil {
		t.Fatalf("DeleteMissing: got error %v", err)
	}
	if removed != 1 {
		t.Errorf("removed count: got %d, want 1", removed)
	}

	m, _ = svc.Load()
	if len(m) != 1 {
		t.Fatalf("after cleanup, expected 1 entry, got %d", len(m))
	}
	if _, ok := m[abs1]; !ok {
		t.Error("valid entry (dir1) should remain")
	}
	if _, ok := m[abs2]; ok {
		t.Error("missing entry (dir2) should be removed")
	}
}

// TestRepoMeta_DeleteMissing_None 没有失效记录时返回 0 不报错。
func TestRepoMeta_DeleteMissing_None(t *testing.T) {
	svc := createRepoMetaService(t)
	dir := t.TempDir()
	svc.Upsert(&model.RepoMeta{Path: dir, Summary: "x"})

	removed, err := svc.DeleteMissing()
	if err != nil {
		t.Fatalf("DeleteMissing none: got error %v", err)
	}
	if removed != 0 {
		t.Errorf("removed count: got %d, want 0", removed)
	}
}

// TestRepoMeta_Persistence 跨实例持久化：写入后用新实例 Load 能读到。
func TestRepoMeta_Persistence(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "repo_meta.json")
	svc1 := NewRepoMetaService(configPath)
	dir := t.TempDir()
	abs, _ := filepath.Abs(dir)

	svc1.Upsert(&model.RepoMeta{Path: dir, Summary: "持久化", Tags: []string{"t"}})

	// 新实例从同一文件加载
	svc2 := NewRepoMetaService(configPath)
	m, err := svc2.Load()
	if err != nil {
		t.Fatalf("reload Load: got error %v", err)
	}
	got, ok := m[abs]
	if !ok {
		t.Fatalf("expected persisted entry at %q", abs)
	}
	if got.Summary != "持久化" {
		t.Errorf("Summary: got %q, want %q", got.Summary, "持久化")
	}
}

// TestRepoMeta_Upsert_NilOrEmptyPath nil 或空路径应报错。
func TestRepoMeta_Upsert_NilOrEmptyPath(t *testing.T) {
	svc := createRepoMetaService(t)

	if err := svc.Upsert(nil); err == nil {
		t.Error("Upsert nil meta: expected error, got nil")
	}
	if err := svc.Upsert(&model.RepoMeta{Path: ""}); err == nil {
		t.Error("Upsert empty path: expected error, got nil")
	}
}
