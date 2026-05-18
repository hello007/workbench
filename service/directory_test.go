package service

import (
	"os"
	"path/filepath"
	"testing"
)

// createTestService 创建测试用 DirectoryService，配置文件在临时目录
func createTestService(t *testing.T) *DirectoryService {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "directories.json")
	return NewDirectoryService(configPath)
}

// --- Create 测试 ---

func TestCreate_Success(t *testing.T) {
	dir := t.TempDir()
	svc := createTestService(t)

	created, err := svc.Create("测试目录", dir, false)
	if err != nil {
		t.Fatalf("Create succeeded path: got error %v", err)
	}
	if created.Name != "测试目录" {
		t.Errorf("Create name: got %q, want %q", created.Name, "测试目录")
	}
	if created.Path != dir {
		t.Errorf("Create path: got %q, want %q", created.Path, dir)
	}
	if created.ID == "" {
		t.Error("Create ID: got empty string, want non-empty")
	}
}

func TestCreate_PathNotExists(t *testing.T) {
	svc := createTestService(t)

	_, err := svc.Create("不存在", "/nonexistent/path/that/does/not/exist", false)
	if err == nil {
		t.Fatal("Create nonexistent path: expected error, got nil")
	}
}

func TestCreate_DuplicatePath(t *testing.T) {
	dir := t.TempDir()
	svc := createTestService(t)

	_, err := svc.Create("目录1", dir, false)
	if err != nil {
		t.Fatalf("First create: got error %v", err)
	}

	_, err = svc.Create("目录2", dir, false)
	if err == nil {
		t.Fatal("Duplicate path create: expected error, got nil")
	}
}

func TestCreate_PathNormalization(t *testing.T) {
	dir := t.TempDir()
	svc := createTestService(t)

	// 使用 filepath.Abs 验证相对路径被规范化
	// 在临时目录下创建子目录
	subdir := filepath.Join(dir, "subdir")
	os.MkdirAll(subdir, 0755)

	created, err := svc.Create("子目录", subdir, false)
	if err != nil {
		t.Fatalf("Create with sub path: got error %v", err)
	}

	// 路径应该是绝对路径
	if !filepath.IsAbs(created.Path) {
		t.Errorf("Create path not absolute: got %q", created.Path)
	}
}

func TestCreate_AsDefault(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	svc := createTestService(t)

	_, err := svc.Create("目录1", dir1, false)
	if err != nil {
		t.Fatalf("First create: got error %v", err)
	}

	_, err = svc.Create("默认目录", dir2, true)
	if err != nil {
		t.Fatalf("Default create: got error %v", err)
	}

	dirs, _ := svc.Load()
	// 第二个应该是默认，第一个不应该是默认
	for _, d := range dirs {
		if d.Path == dir1 && d.IsDefault {
			t.Error("First dir should not be default")
		}
		if d.Path == dir2 && !d.IsDefault {
			t.Error("Second dir should be default")
		}
	}
}

func TestCreate_PersistedToFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config", "directories.json")
	svc := NewDirectoryService(configPath)

	workDir := t.TempDir()
	_, err := svc.Create("持久化测试", workDir, false)
	if err != nil {
		t.Fatalf("Create: got error %v", err)
	}

	// 重新创建 service 从同一配置文件加载，验证持久化
	svc2 := NewDirectoryService(configPath)
	dirs, err := svc2.Load()
	if err != nil {
		t.Fatalf("Load after create: got error %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("Persisted count: got %d, want 1", len(dirs))
	}
	if dirs[0].Name != "持久化测试" {
		t.Errorf("Persisted name: got %q, want %q", dirs[0].Name, "持久化测试")
	}
}

// --- Delete 测试 ---

func TestDelete_Success(t *testing.T) {
	dir := t.TempDir()
	svc := createTestService(t)

	created, _ := svc.Create("待删除", dir, false)

	err := svc.Delete(created.ID)
	if err != nil {
		t.Fatalf("Delete: got error %v", err)
	}

	dirs, _ := svc.Load()
	if len(dirs) != 0 {
		t.Errorf("After delete count: got %d, want 0", len(dirs))
	}
}

func TestDelete_NotExists(t *testing.T) {
	svc := createTestService(t)

	err := svc.Delete("nonexistent-id")
	if err == nil {
		t.Fatal("Delete nonexistent: expected error, got nil")
	}
}

func TestDelete_PersistedAfterDelete(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	svc := createTestService(t)

	created1, _ := svc.Create("保留", dir1, false)
	svc.Create("删除", dir2, false)

	svc.Delete(created1.ID)

	dirs, _ := svc.Load()
	if len(dirs) != 1 {
		t.Fatalf("After delete count: got %d, want 1", len(dirs))
	}
	// 按路径查找（goroutine 顺序不确定时按路径查找）
	found := false
	for _, d := range dirs {
		if d.Path == dir2 {
			found = true
		}
	}
	if !found {
		t.Error("Remaining directory not found after delete")
	}
}

// --- Load 测试 ---

func TestLoad_Empty(t *testing.T) {
	svc := createTestService(t)

	dirs, err := svc.Load()
	if err != nil {
		t.Fatalf("Load empty: got error %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("Load empty count: got %d, want 0", len(dirs))
	}
}

// --- SetDefault 测试 ---

func TestSetDefault_Success(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	svc := createTestService(t)

	created1, _ := svc.Create("目录1", dir1, true)
	svc.Create("目录2", dir2, false)

	err := svc.SetDefault(created1.ID)
	if err != nil {
		t.Fatalf("SetDefault: got error %v", err)
	}

	dirs, _ := svc.Load()
	for _, d := range dirs {
		if d.Path == dir1 && !d.IsDefault {
			t.Error("dir1 should be default after SetDefault")
		}
		if d.Path == dir2 && d.IsDefault {
			t.Error("dir2 should not be default after SetDefault")
		}
	}
}

func TestSetDefault_Toggle(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dir3 := t.TempDir()
	svc := createTestService(t)

	created1, _ := svc.Create("目录1", dir1, true)
	created2, _ := svc.Create("目录2", dir2, false)
	svc.Create("目录3", dir3, false)

	err := svc.SetDefault(created2.ID)
	if err != nil {
		t.Fatalf("SetDefault to dir2: got error %v", err)
	}

	dirs, _ := svc.Load()
	for _, d := range dirs {
		if d.Path == dir1 && d.IsDefault {
			t.Error("dir1 should not be default after toggle")
		}
		if d.Path == dir2 && !d.IsDefault {
			t.Error("dir2 should be default after toggle")
		}
		if d.Path == dir3 && d.IsDefault {
			t.Error("dir3 should not be default")
		}
	}

	err = svc.SetDefault(created1.ID)
	if err != nil {
		t.Fatalf("SetDefault back to dir1: got error %v", err)
	}

	dirs, _ = svc.Load()
	for _, d := range dirs {
		if d.Path == dir1 && !d.IsDefault {
			t.Error("dir1 should be default after toggle back")
		}
		if d.Path == dir2 && d.IsDefault {
			t.Error("dir2 should not be default after toggle back")
		}
		if d.Path == dir3 && d.IsDefault {
			t.Error("dir3 should not be default after toggle back")
		}
	}
}

func TestSetDefault_NotExists(t *testing.T) {
	dir1 := t.TempDir()
	svc := createTestService(t)
	svc.Create("目录1", dir1, false)

	err := svc.SetDefault("nonexistent-id")
	if err == nil {
		t.Fatal("SetDefault nonexistent: expected error, got nil")
	}
}

// --- GetDefault 测试 ---

func TestGetDefault_WithDefault(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	svc := createTestService(t)

	svc.Create("目录1", dir1, false)
	svc.Create("目录2", dir2, true)

	got, err := svc.GetDefault()
	if err != nil {
		t.Fatalf("GetDefault: got error %v", err)
	}
	if got.Path != dir2 {
		t.Errorf("GetDefault path: got %q, want %q", got.Path, dir2)
	}
}

func TestGetDefault_FallbackToFirst(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	svc := createTestService(t)

	svc.Create("目录1", dir1, false)
	svc.Create("目录2", dir2, false)

	got, err := svc.GetDefault()
	if err != nil {
		t.Fatalf("GetDefault fallback: got error %v", err)
	}
	if got.Path != dir1 {
		t.Errorf("GetDefault fallback path: got %q, want %q", got.Path, dir1)
	}
}

func TestGetDefault_EmptyList(t *testing.T) {
	svc := createTestService(t)

	_, err := svc.GetDefault()
	if err == nil {
		t.Fatal("GetDefault empty list: expected error, got nil")
	}
}

// --- 持久化一致性测试 ---

func TestPersistence_MultipleOperations(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "directories.json")
	svc := NewDirectoryService(configPath)

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	dir3 := t.TempDir()

	// 步骤1：添加目录1和目录2
	created1, err := svc.Create("目录1", dir1, false)
	if err != nil {
		t.Fatalf("Create dir1: got error %v", err)
	}
	created2, err := svc.Create("目录2", dir2, true)
	if err != nil {
		t.Fatalf("Create dir2: got error %v", err)
	}

	// 步骤2：设置目录1为默认
	err = svc.SetDefault(created1.ID)
	if err != nil {
		t.Fatalf("SetDefault dir1: got error %v", err)
	}

	// 步骤3：添加目录3
	svc.Create("目录3", dir3, false)

	// 步骤4：删除目录2
	err = svc.Delete(created2.ID)
	if err != nil {
		t.Fatalf("Delete dir2: got error %v", err)
	}

	// 步骤5：重新加载，验证状态一致
	svc2 := NewDirectoryService(configPath)
	dirs, err := svc2.Load()
	if err != nil {
		t.Fatalf("Reload: got error %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("After reload count: got %d, want 2", len(dirs))
	}

	// 验证目录1是默认
	gotDefault, err := svc2.GetDefault()
	if err != nil {
		t.Fatalf("GetDefault after reload: got error %v", err)
	}
	if gotDefault.Path != dir1 {
		t.Errorf("Default after reload: got %q, want %q", gotDefault.Path, dir1)
	}

	// 验证目录3存在
	found3 := false
	for _, d := range dirs {
		if d.Path == dir3 {
			found3 = true
		}
	}
	if !found3 {
		t.Error("dir3 should exist after reload")
	}
}
