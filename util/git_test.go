package util

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"workbench/util/testutil"
)

// TestNewGitCommand_DefaultTimeout 默认超时 30s。
func TestNewGitCommand_DefaultTimeout(t *testing.T) {
	g := NewGitCommand()
	if g.timeout != 30*time.Second {
		t.Errorf("默认超时应为 30s, got %v", g.timeout)
	}
}

// TestNewGitCommandWithTimeout 自定义超时生效。
func TestNewGitCommandWithTimeout(t *testing.T) {
	g := NewGitCommandWithTimeout(5 * time.Second)
	if g.timeout != 5*time.Second {
		t.Errorf("超时应为 5s, got %v", g.timeout)
	}
}

// TestIsGitRepository_RealRepo 真实 git init 仓库返回 true。
func TestIsGitRepository_RealRepo(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")
	g := NewGitCommand()
	if !g.IsGitRepository(dir) {
		t.Error("真实仓库应识别为 true")
	}
}

// TestIsGitRepository_NonRepo 普通目录返回 false。
func TestIsGitRepository_NonRepo(t *testing.T) {
	g := NewGitCommand()
	if g.IsGitRepository(t.TempDir()) {
		t.Error("非仓库应返回 false")
	}
}

// TestFindGitRoot_RealRepo 仓库根目录向上查找返回自身。
func TestFindGitRoot_RealRepo(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")
	root, err := FindGitRoot(dir)
	if err != nil {
		t.Fatalf("FindGitRoot: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	if root != abs {
		t.Errorf("根应等于仓库目录, got %s want %s", root, abs)
	}
}

// TestFindGitRoot_Subdir 子目录向上查找定位到仓库根。
func TestFindGitRoot_Subdir(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	root, err := FindGitRoot(sub)
	if err != nil {
		t.Fatalf("FindGitRoot subdir: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	if root != abs {
		t.Errorf("子目录应向上找到根, got %s want %s", root, abs)
	}
}

// TestFindGitRoot_NonRepo 非仓库目录返回错误。
func TestFindGitRoot_NonRepo(t *testing.T) {
	_, err := FindGitRoot(t.TempDir())
	if err == nil {
		t.Error("非仓库应返回错误")
	}
}

// TestHasLocalChanges_NoChanges 无改动返回 false。
func TestHasLocalChanges_NoChanges(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")
	g := NewGitCommand()
	has, err := g.HasLocalChanges(dir)
	if err != nil {
		t.Fatalf("HasLocalChanges: %v", err)
	}
	if has {
		t.Error("无改动应返回 false")
	}
}

// TestHasLocalChanges_WithChanges 有未跟踪文件返回 true。
func TestHasLocalChanges_WithChanges(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	g := NewGitCommand()
	has, err := g.HasLocalChanges(dir)
	if err != nil {
		t.Fatalf("HasLocalChanges: %v", err)
	}
	if !has {
		t.Error("有改动应返回 true")
	}
}

// TestGetBranch_RealRepo 真实仓库获取分支不报错。
func TestGetBranch_RealRepo(t *testing.T) {
	dir := testutil.InitTempRepo(t)
	g := NewGitCommand()
	_, err := g.GetBranch(dir)
	if err != nil {
		t.Errorf("GetBranch 不应报错: %v", err)
	}
}

// TestExecute_InvalidArgs 非法 git 子命令返回错误。
func TestExecute_InvalidArgs(t *testing.T) {
	dir := t.TempDir()
	g := NewGitCommand()
	_, err := g.Execute(dir, "nonexistent-subcommand-xyz")
	if err == nil {
		t.Error("非法子命令应返回错误")
	}
}

// TestGetBranchesAll_RealRepo 真实仓库获取分支列表不报错。
func TestGetBranchesAll_RealRepo(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")
	g := NewGitCommand()
	_, err := g.GetBranchesAll(dir)
	if err != nil {
		t.Errorf("GetBranchesAll 不应报错: %v", err)
	}
}

// TestFindGitRoot_GitFile worktree/submodule 的 .git 是文件（内容形如 "gitdir: /path/..."），
// FindGitRoot 应识别该目录为仓库根，而非因 IsDir 漏判向上走到父目录。
func TestFindGitRoot_GitFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /some/path/.git/modules/x"), 0o644); err != nil {
		t.Fatalf("write .git file: %v", err)
	}
	root, err := FindGitRoot(dir)
	if err != nil {
		t.Fatalf("FindGitRoot gitfile: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	if root != abs {
		t.Errorf("gitfile 目录应识别为根, got %s want %s", root, abs)
	}
}

// TestFindGitRoot_GitFileInSubdir submodule 工作目录在父仓库之下，.git 为文件时，
// FindGitRoot 应停在该目录而非继续向上走到 superproject 根（修复后行为）。
func TestFindGitRoot_GitFileInSubdir(t *testing.T) {
	parent := t.TempDir()
	sub := filepath.Join(parent, "libs", "child")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	// submodule 工作目录的 .git 是文件
	if err := os.WriteFile(filepath.Join(sub, ".git"), []byte("gitdir: ../../.git/modules/libs/child"), 0o644); err != nil {
		t.Fatalf("write .git file: %v", err)
	}
	root, err := FindGitRoot(sub)
	if err != nil {
		t.Fatalf("FindGitRoot sub gitfile: %v", err)
	}
	abs, _ := filepath.Abs(sub)
	if root != abs {
		t.Errorf("submodule 目录应停在自身根, got %s want %s", root, abs)
	}
}
