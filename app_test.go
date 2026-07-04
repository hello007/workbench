package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"workbench/model"
	"workbench/service"
	"workbench/util"
)

func TestGetAppVersion(t *testing.T) {
	app := NewApp()
	v := app.GetAppVersion()
	if v == "" {
		t.Error("GetAppVersion should return non-empty string")
	}
	t.Logf("App version: %s", v)
}

func TestGetGitRemoteURL_ValidRepo(t *testing.T) {
	// Create temporary test repository
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "test-repo")
	os.MkdirAll(repoPath, 0755)

	// Initialize Git repository
	err := exec.Command("git", "init", repoPath).Run()
	if err != nil {
		t.Skip("Cannot create test repository")
	}

	app := NewApp()
	info, err := app.GetGitRemoteURL(repoPath)
	if err != nil {
		t.Fatalf("GetGitRemoteURL failed: %v", err)
	}

	if info == nil {
		t.Fatal("Expected GitRemoteInfo, got nil")
	}
}

func TestGetGitRemoteURL_InvalidPath(t *testing.T) {
	app := NewApp()
	_, err := app.GetGitRemoteURL("/invalid/nonexistent/path")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestGetGitRemoteURL_CurrentRepo(t *testing.T) {
	// Test with the current repository (workbench)
	app := NewApp()
	info, err := app.GetGitRemoteURL(".")
	if err != nil {
		t.Fatalf("GetGitRemoteURL failed on current repo: %v", err)
	}

	if info == nil {
		t.Fatal("Expected GitRemoteInfo, got nil")
	}

	// The function should work even without origin remote
	// It will return empty strings in that case
	t.Logf("Repository Info - Branch: %s, RemoteURL: %s, IsDetached: %v",
		info.Branch, info.RemoteURL, info.IsDetached)

	// Verify the structure is valid (not nil)
	if info.RemoteURL == "" && info.Branch == "" && !info.IsDetached {
		t.Log("Repository has no origin remote (this is OK for the test)")
	}
}

func TestGetCommitHistory_Limit(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "test-repo")
	os.MkdirAll(repoPath, 0755)

	// 初始化 Git 仓库并创建测试提交
	exec.Command("git", "init", repoPath).Run()
	exec.Command("git", "-C", repoPath, "config", "user.name", "Test").Run()
	exec.Command("git", "-C", repoPath, "config", "user.email", "test@test.com").Run()

	// 创建多个测试提交
	for i := 1; i <= 5; i++ {
		filename := filepath.Join(repoPath, fmt.Sprintf("file%d.txt", i))
		os.WriteFile(filename, []byte(fmt.Sprintf("content %d", i)), 0644)
		exec.Command("git", "-C", repoPath, "add", ".").Run()
		exec.Command("git", "-C", repoPath, "commit", "-m", fmt.Sprintf("Commit %d", i)).Run()
	}

	app := NewApp()
	commits, err := app.GetCommitHistory(repoPath, 3, 0)
	if err != nil {
		t.Fatalf("GetCommitHistory failed: %v", err)
	}

	if len(commits) != 3 {
		t.Errorf("Expected 3 commits, got %d", len(commits))
	}

	// Git commit messages include trailing newline
	if commits[0].Message != "Commit 5\n" {
		t.Errorf("Expected 'Commit 5\\n', got %s", commits[0].Message)
	}
}

func TestGetCommitHistory_Offset(t *testing.T) {
	tempDir := t.TempDir()
	repoPath := filepath.Join(tempDir, "test-repo")
	os.MkdirAll(repoPath, 0755)

	exec.Command("git", "init", repoPath).Run()
	exec.Command("git", "-C", repoPath, "config", "user.name", "Test").Run()
	exec.Command("git", "-C", repoPath, "config", "user.email", "test@test.com").Run()

	for i := 1; i <= 5; i++ {
		filename := filepath.Join(repoPath, fmt.Sprintf("file%d.txt", i))
		os.WriteFile(filename, []byte(fmt.Sprintf("content %d", i)), 0644)
		exec.Command("git", "-C", repoPath, "add", ".").Run()
		exec.Command("git", "-C", repoPath, "commit", "-m", fmt.Sprintf("Commit %d", i)).Run()
	}

	app := NewApp()
	commits, err := app.GetCommitHistory(repoPath, 2, 2)
	if err != nil {
		t.Fatalf("GetCommitHistory failed: %v", err)
	}

	if len(commits) != 2 {
		t.Errorf("Expected 2 commits, got %d", len(commits))
	}

	// Git commit messages include trailing newline
	if commits[0].Message != "Commit 3\n" {
		t.Errorf("Expected 'Commit 3\\n', got %s", commits[0].Message)
	}
}

// runGitIn 在指定目录执行 git 命令，失败即终止测试。
func runGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, out)
	}
}

// writeDirectoriesConfig 写入临时目录配置文件（directories.json），返回其路径。
func writeDirectoriesConfig(t *testing.T, path string, dirs []*model.Directory) {
	t.Helper()
	cfg := struct {
		Directories []*model.Directory `json:"directories"`
	}{Directories: dirs}
	if err := util.SaveJSON(path, cfg); err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}
}

// TestGetDirectories_NoRuntimeDetection 验证 GetDirectories 不再触发运行时检测：
// 直接返回持久化的 IsGitRepo 值（启动零子进程）。
// 构造"持久化 IsGitRepo=true 但实际路径非 git"的配置，断言 GetDirectories 原样返回 true 而不重算为 false。
func TestGetDirectories_NoRuntimeDetection(t *testing.T) {
	// 准备一个普通目录（非 git 仓）
	plainDir := t.TempDir()

	// 持久化值刻意标 true（与实际不符），若 GetDirectories 仍运行时检测会被纠正为 false
	configPath := filepath.Join(t.TempDir(), "directories.json")
	writeDirectoriesConfig(t, configPath, []*model.Directory{
		{ID: "d1", Name: "stale", Path: plainDir, IsDefault: false, IsGitRepo: true},
	})

	app := &App{directorySvc: service.NewDirectoryService(configPath)}
	got := app.GetDirectories()
	if len(got) != 1 {
		t.Fatalf("expected 1 directory, got %d", len(got))
	}
	if !got[0].IsGitRepo {
		t.Errorf("expected persisted IsGitRepo=true to be returned as-is without runtime detection, got false")
	}
}

// TestGetDirectories_OldConfigBackwardCompat 验证旧配置（无 isGitRepo 字段）反序列化零值兼容：
// GetDirectories 返回 IsGitRepo=false，等待 RefreshDirectoriesGitFlag 异步刷新补正。
func TestGetDirectories_OldConfigBackwardCompat(t *testing.T) {
	repoDir := t.TempDir()
	runGitIn(t, repoDir, "init")

	// 刻意不含 isGitRepo 字段，模拟旧配置（用 model 序列化保证路径转义正确，仅不设置 IsGitRepo）
	configPath := filepath.Join(t.TempDir(), "directories.json")
	writeDirectoriesConfig(t, configPath, []*model.Directory{
		{ID: "d1", Name: "repo", Path: repoDir, IsDefault: false, IsGitRepo: false},
	})

	app := &App{directorySvc: service.NewDirectoryService(configPath)}
	got := app.GetDirectories()
	if len(got) != 1 {
		t.Fatalf("expected 1 directory, got %d", len(got))
	}
	if got[0].IsGitRepo {
		t.Errorf("expected IsGitRepo=false (zero value) for old config without isGitRepo field, got true")
	}
}

// TestGetDirectories_MissingPathAndAbsentConfig 验证路径不存在时持久化零值 IsGitRepo=false 原样返回，
// 以及配置文件不存在时 GetDirectories 返回空切片。
func TestGetDirectories_MissingPathAndAbsentConfig(t *testing.T) {
	// 路径不存在 → IsGitRepo 字段缺省零值 false，GetDirectories 原样返回（不触发检测）
	configPath := filepath.Join(t.TempDir(), "directories.json")
	writeDirectoriesConfig(t, configPath, []*model.Directory{
		{ID: "d1", Name: "ghost", Path: filepath.Join(t.TempDir(), "does-not-exist")},
	})

	app := &App{directorySvc: service.NewDirectoryService(configPath)}
	got := app.GetDirectories()
	if len(got) != 1 {
		t.Fatalf("expected 1 directory, got %d", len(got))
	}
	if got[0].IsGitRepo {
		t.Error("expected IsGitRepo=false for non-existent path")
	}

	// 配置文件不存在 → Load 返回空，GetDirectories 返回空切片
	app2 := &App{directorySvc: service.NewDirectoryService(filepath.Join(t.TempDir(), "absent.json"))}
	if got := app2.GetDirectories(); len(got) != 0 {
		t.Errorf("expected empty slice when config absent, got %d", len(got))
	}
}

// TestDirectory_OldConfigBackwardCompat 验证不含 isGitRepo 字段的旧 JSON
// 反序列化后 IsGitRepo 零值为 false（兼容性回归保护）。
func TestDirectory_OldConfigBackwardCompat(t *testing.T) {
	oldJSON := `{"id":"d1","name":"x","path":"/tmp/x","isDefault":false,"createTime":"2026-01-01T00:00:00Z"}`
	tmp := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(tmp, []byte(oldJSON), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	var d model.Directory
	if err := util.LoadJSON(tmp, &d); err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}
	if d.IsGitRepo {
		t.Error("expected IsGitRepo=false (zero value) for old config without isGitRepo field")
	}
}

// TestRefreshDirectoriesGitFlag_DetectsAndPersists 验证 RefreshDirectoriesGitFlag：
// 构造持久化 IsGitRepo 全 false 的旧配置（含一个真实 git 仓 + 一个普通目录），
// 调用后 IsGitRepo 被刷新（git=true、plain=false）且回写 directories.json。
func TestRefreshDirectoriesGitFlag_DetectsAndPersists(t *testing.T) {
	// 准备真实 git 仓 + 普通目录
	repoDir := t.TempDir()
	runGitIn(t, repoDir, "init")
	runGitIn(t, repoDir, "config", "user.email", "test@test.com")
	runGitIn(t, repoDir, "config", "user.name", "test")
	plainDir := t.TempDir()

	// 旧配置：isGitRepo 字段全缺省（零值 false）
	configPath := filepath.Join(t.TempDir(), "directories.json")
	writeDirectoriesConfig(t, configPath, []*model.Directory{
		{ID: "d1", Name: "repo", Path: repoDir, IsDefault: false},
		{ID: "d2", Name: "plain", Path: plainDir, IsDefault: false},
	})

	app := &App{directorySvc: service.NewDirectoryService(configPath)}
	got := app.RefreshDirectoriesGitFlag()
	if len(got) != 2 {
		t.Fatalf("expected 2 directories, got %d", len(got))
	}
	byID := make(map[string]*model.Directory, len(got))
	for _, d := range got {
		byID[d.ID] = d
	}
	if !byID["d1"].IsGitRepo {
		t.Errorf("expected d1 (%s) refreshed to IsGitRepo=true", repoDir)
	}
	if byID["d2"].IsGitRepo {
		t.Errorf("expected d2 (%s) refreshed to IsGitRepo=false", plainDir)
	}

	// 重新 Load（独立 service 实例）验证回写持久化
	svc2 := service.NewDirectoryService(configPath)
	persisted, err := svc2.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	pByID := make(map[string]*model.Directory, len(persisted))
	for _, d := range persisted {
		pByID[d.ID] = d
	}
	if !pByID["d1"].IsGitRepo {
		t.Error("expected d1 IsGitRepo=true persisted to directories.json")
	}
	if pByID["d2"].IsGitRepo {
		t.Error("expected d2 IsGitRepo=false persisted to directories.json")
	}
}

// TestRefreshDirectoriesGitFlag_PreservesOtherFields 验证 Refresh 基于"最新 Load 合并"语义：
// 只更新 IsGitRepo，保留其他字段（如 Name）的最新持久化值，
// 规避并发竞态（刷新期间用户改名，刷新不应覆盖）。
func TestRefreshDirectoriesGitFlag_PreservesOtherFields(t *testing.T) {
	repoDir := t.TempDir()
	runGitIn(t, repoDir, "init")

	configPath := filepath.Join(t.TempDir(), "directories.json")
	writeDirectoriesConfig(t, configPath, []*model.Directory{
		{ID: "d1", Name: "original", Path: repoDir, IsDefault: false, IsGitRepo: false},
	})

	app := &App{directorySvc: service.NewDirectoryService(configPath)}

	// 模拟并发：刷新前另一路径改了 Name（绕过 app，直接写最新值）
	// 这里通过先调用 service 层改名来模拟外部最新持久化
	svc := service.NewDirectoryService(configPath)
	dir, _ := svc.GetDefault()
	if dir == nil {
		// 没有默认则取第一个
		dirs, _ := svc.Load()
		dir = dirs[0]
	}
	dir.Name = "renamed-by-user"
	svc.Save([]*model.Directory{dir})

	got := app.RefreshDirectoriesGitFlag()
	if len(got) != 1 {
		t.Fatalf("expected 1 directory, got %d", len(got))
	}
	if got[0].Name != "renamed-by-user" {
		t.Errorf("expected Name preserved as 'renamed-by-user', got %q", got[0].Name)
	}
	if !got[0].IsGitRepo {
		t.Error("expected IsGitRepo refreshed to true")
	}
}

// TestAddDirectory_PersistsIsGitRepo 验证 AddDirectory（service.Create）持久化 IsGitRepo：
// git 仓 → true，普通目录 → false。
func TestAddDirectory_PersistsIsGitRepo(t *testing.T) {
	repoDir := t.TempDir()
	runGitIn(t, repoDir, "init")
	plainDir := t.TempDir()

	configPath := filepath.Join(t.TempDir(), "directories.json")
	app := &App{directorySvc: service.NewDirectoryService(configPath)}

	app.AddDirectory("repo", repoDir, false)
	app.AddDirectory("plain", plainDir, false)

	got := app.GetDirectories()
	if len(got) != 2 {
		t.Fatalf("expected 2 directories, got %d", len(got))
	}
	byName := make(map[string]*model.Directory, len(got))
	for _, d := range got {
		byName[d.Name] = d
	}
	if !byName["repo"].IsGitRepo {
		t.Errorf("expected repo (%s) IsGitRepo=true persisted", repoDir)
	}
	if byName["plain"].IsGitRepo {
		t.Errorf("expected plain (%s) IsGitRepo=false persisted", plainDir)
	}
}

// TestUpdateDirectory_RecalculatesIsGitRepo 验证 UpdateDirectory（service.Update）在 path 变化后重算 IsGitRepo：
// 普通目录 → git 仓，IsGitRepo 从 false 变 true 并持久化。
func TestUpdateDirectory_RecalculatesIsGitRepo(t *testing.T) {
	// 初始：普通目录
	plainDir := t.TempDir()
	// 目标：另一个 git 仓
	repoDir := t.TempDir()
	runGitIn(t, repoDir, "init")

	configPath := filepath.Join(t.TempDir(), "directories.json")
	app := &App{directorySvc: service.NewDirectoryService(configPath)}

	created := app.AddDirectory("d", plainDir, false)
	if created.IsGitRepo {
		t.Fatalf("expected initial IsGitRepo=false for plain dir, got true")
	}

	// 改 path 到 git 仓
	updated := app.UpdateDirectory(created.ID, "d", repoDir, false)
	if updated == nil {
		t.Fatal("UpdateDirectory returned nil")
	}
	if !updated.IsGitRepo {
		t.Errorf("expected IsGitRepo=true after updating path to git repo, got false")
	}

	// 重新 Load 验证持久化
	svc2 := service.NewDirectoryService(configPath)
	dirs, _ := svc2.Load()
	if len(dirs) != 1 {
		t.Fatalf("expected 1 directory after update, got %d", len(dirs))
	}
	if !dirs[0].IsGitRepo {
		t.Error("expected IsGitRepo=true persisted after update")
	}
}
