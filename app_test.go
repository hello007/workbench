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

// TestGetDirectories_FillsIsGitRepo 验证 GetDirectories 对 git 仓库返回 true、对普通目录返回 false。
// 同时验证旧配置文件（不含 isGitRepo 字段）反序列化零值兼容。
func TestGetDirectories_FillsIsGitRepo(t *testing.T) {
	// 准备一个 git 仓库目录
	repoDir := t.TempDir()
	runGitIn(t, repoDir, "init")
	runGitIn(t, repoDir, "config", "user.email", "test@test.com")
	runGitIn(t, repoDir, "config", "user.name", "test")

	// 准备一个普通目录
	plainDir := t.TempDir()

	// 写入配置文件（刻意不含 isGitRepo 字段，验证旧配置反序列化零值兼容）
	configPath := filepath.Join(t.TempDir(), "directories.json")
	writeDirectoriesConfig(t, configPath, []*model.Directory{
		{ID: "d1", Name: "repo", Path: repoDir, IsDefault: true},
		{ID: "d2", Name: "plain", Path: plainDir, IsDefault: false},
	})

	app := &App{directorySvc: service.NewDirectoryService(configPath)}
	got := app.GetDirectories()
	if len(got) != 2 {
		t.Fatalf("expected 2 directories, got %d", len(got))
	}

	byID := make(map[string]*model.Directory, len(got))
	for _, d := range got {
		byID[d.ID] = d
	}
	if !byID["d1"].IsGitRepo {
		t.Errorf("expected d1 (%s) IsGitRepo=true", repoDir)
	}
	if byID["d2"].IsGitRepo {
		t.Errorf("expected d2 (%s) IsGitRepo=false", plainDir)
	}
}

// TestGetDirectories_MissingPathAndAbsentConfig 验证路径不存在时 IsGitRepo=false（不报错），
// 以及配置文件不存在时 GetDirectories 返回空切片。
func TestGetDirectories_MissingPathAndAbsentConfig(t *testing.T) {
	// 路径不存在 → 检测返回 false，不 panic
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

// TestApplyGitRepoFlag_NilSafe 验证 nil 入参不 panic。
func TestApplyGitRepoFlag_NilSafe(t *testing.T) {
	app := &App{}
	app.applyGitRepoFlag(nil) // 不应 panic
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
