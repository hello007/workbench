package util

import (
	"os/exec"
	"os"
	"path/filepath"
	"testing"
)

// TestIsGitRepositoryFast_StandardRepo 标准仓库（.git 为目录）应被识别。
func TestIsGitRepositoryFast_StandardRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if !IsGitRepositoryFast(dir) {
		t.Error("expected standard repo (.git dir) to be detected")
	}
}

// TestIsGitRepositoryFast_GitFile worktree/submodule 的 .git 是文件，应被识别（不要求 IsDir）。
// 这是本预筛相对现有 FindGitRoot（用 IsDir 会漏判）的关键修复点。
func TestIsGitRepositoryFast_GitFile(t *testing.T) {
	dir := t.TempDir()
	// 模拟 worktree/submodule 的 .git 文件（内容形如 "gitdir: /path/..."）
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /some/path/.git/worktrees/x"), 0644); err != nil {
		t.Fatalf("write .git file: %v", err)
	}
	if !IsGitRepositoryFast(dir) {
		t.Error("expected worktree/submodule (.git file) to be detected, IsDir() would miss it")
	}
}

// TestIsGitRepositoryFast_NotRepo 无 .git 条目的普通目录应返回 false。
func TestIsGitRepositoryFast_NotRepo(t *testing.T) {
	dir := t.TempDir()
	if IsGitRepositoryFast(dir) {
		t.Error("expected non-repo dir to return false")
	}
}

// TestIsGitRepositoryFast_EmptyPath 空路径应返回 false。
func TestIsGitRepositoryFast_EmptyPath(t *testing.T) {
	if IsGitRepositoryFast("") {
		t.Error("expected empty path to return false")
	}
}

// TestIsGitRepositoryFast_NonexistentPath 不存在的路径应返回 false（os.Stat 失败）。
func TestIsGitRepositoryFast_NonexistentPath(t *testing.T) {
	if IsGitRepositoryFast(filepath.Join(t.TempDir(), "does-not-exist")) {
		t.Error("expected nonexistent path to return false")
	}
}

// TestIsGitRepositoryFast_RealGitRepo 用真实 git init 创建的仓库应被识别（端到端验证）。
func TestIsGitRepositoryFast_RealGitRepo(t *testing.T) {
	dir := t.TempDir()
	runGitSimple(t, dir, "init")
	if !IsGitRepositoryFast(dir) {
		t.Error("expected real git init repo to be detected")
	}
}

// runGitSimple 在指定目录执行 git 命令，失败即终止测试。
func runGitSimple(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v in %s failed: %v", args, dir, err)
	}
}
