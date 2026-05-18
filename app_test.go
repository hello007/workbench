package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
	// Test with the current repository (git-manager)
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
