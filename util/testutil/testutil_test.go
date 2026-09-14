package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireGit git 不在 PATH 时跳过自测（CI 保证有 git，本地异常环境降级为 skip 不 fail）。
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 不在 PATH，跳过 testutil 自测")
	}
}

// TestRunGit_ExecutesInDir RunGit 在指定目录执行 git 命令成功，.git 目录生成。
func TestRunGit_ExecutesInDir(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	RunGit(t, dir, "init")
	if !isGitRepo(dir) {
		t.Error("RunGit init 后目录应为 git 仓库")
	}
}

// TestWriteFile_CreatesParentDirs WriteFile 自动创建多层父目录并写入内容。
func TestWriteFile_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "file.txt")
	WriteFile(t, path, "hello")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content: got %q want hello", data)
	}
}

// TestWriteFile_OverwriteExisting 父目录已存在时 WriteFile 无副作用覆盖写入。
func TestWriteFile_OverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	WriteFile(t, path, "first")
	WriteFile(t, path, "second")
	data, _ := os.ReadFile(path)
	if string(data) != "second" {
		t.Errorf("overwrite: got %q want second", data)
	}
}

// TestInitTempRepo_ConfiguresIdentity InitTempRepo 初始化仓库并配置 user.name=test。
func TestInitTempRepo_ConfiguresIdentity(t *testing.T) {
	requireGit(t)
	dir := InitTempRepo(t)
	if !isGitRepo(dir) {
		t.Fatal("InitTempRepo 应初始化 git 仓库")
	}
	out, err := exec.Command("git", "-C", dir, "config", "user.name").Output()
	if err != nil {
		t.Fatalf("git config user.name: %v", err)
	}
	if strings.TrimSpace(string(out)) != "test" {
		t.Errorf("user.name: got %q want test", strings.TrimSpace(string(out)))
	}
}

// TestSetupMasterBranch_SetsMaster SetupMasterBranch 后 HEAD 指向 refs/heads/master。
func TestSetupMasterBranch_SetsMaster(t *testing.T) {
	requireGit(t)
	dir := InitTempRepo(t)
	SetupMasterBranch(t, dir)
	out, err := exec.Command("git", "-C", dir, "symbolic-ref", "HEAD").Output()
	if err != nil {
		t.Fatalf("symbolic-ref HEAD: %v", err)
	}
	if strings.TrimSpace(string(out)) != "refs/heads/master" {
		t.Errorf("HEAD symbolic-ref: got %q want refs/heads/master", strings.TrimSpace(string(out)))
	}
}

// TestSetupFFRepo_OnMasterWithBaseContent SetupFFRepo 后停在 master 分支，a.txt 为 base 版本。
func TestSetupFFRepo_OnMasterWithBaseContent(t *testing.T) {
	requireGit(t)
	dir := SetupFFRepo(t)
	branch := gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if branch != "master" {
		t.Errorf("SetupFFRepo 后应在 master 分支, got %q", branch)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	// Windows git autocrlf 默认将 LF 转 CRLF，统一换行后断言内容
	got := strings.ReplaceAll(string(data), "\r\n", "\n")
	if got != "base\n" {
		t.Errorf("a.txt: got %q want base\\n", got)
	}
}

// TestSetupConflictRepo_OnMasterWithMasterContent SetupConflictRepo 后停在 master 分支，
// a.txt 含 master-line2（master 侧改动）。
func TestSetupConflictRepo_OnMasterWithMasterContent(t *testing.T) {
	requireGit(t)
	dir := SetupConflictRepo(t)
	branch := gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if branch != "master" {
		t.Errorf("SetupConflictRepo 后应在 master 分支, got %q", branch)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "a.txt"))
	if !strings.Contains(string(data), "master-line2") {
		t.Errorf("a.txt 应含 master-line2, got %q", data)
	}
}

// isGitRepo 判定目录是否为 git 仓库（.git 目录或文件存在）。
func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// gitOut 执行 git 命令返回 trim 后输出（自测校验用）。
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v in %s: %v", args, dir, err)
	}
	return strings.TrimSpace(string(out))
}
