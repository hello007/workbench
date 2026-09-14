// Package testutil 提供 WorkBench 后端测试跨包共用的 git 仓库与文件 fixture 辅助函数。
//
// 收敛来源（PR1 测试重构）：service / util / 主包三处重复的 runGit / initTempRepo /
// writeFile / setupFFRepo / setupConflictRepo 等辅助合并为单一导出版本，与前端
// src/test/wails-mock-defaults.js 单一数据源模式对齐。
//
// 仅 _test.go 与 benchmark 调用，不进生产二进制（全部导入方均为 _test.go）。
// 所有函数均调 t.Helper()，失败定位指向调用点。
//
// 参数类型用 testing.TB（接口）而非 *testing.T：*testing.T（普通测试）与 *testing.B
// （benchmark）均实现该接口，使 benchmark 可直接复用本组 fixture 构造函数，避免在
// perf_bench_test.go 重复造 git init/config 与批量写文件的轮子。原有 *testing.T
// 调用点零改动（自动满足 testing.TB）。
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// RunGit 在 dir 目录执行 git 命令，失败即终止测试或 benchmark。
//
// 输出经 CombinedOutput 捕获并附在失败消息中，便于定位 git 报错根因。
// 收敛 service.runGit / util.runGitSimple / main.runGitIn 三处同义实现：
// 三者语义一致（指定目录执行 git，失败终止），仅失败消息详略不同，统一为含 stderr 版本。
func RunGit(t testing.TB, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, out)
	}
}

// WriteFile 写入文件内容，自动创建父目录。
//
// MkdirAll 为 service.writeFile 原有行为；对原不建父目录的 svcMergeWriteFile /
// writeDiffFile / gitMergeTestWriteFile 调用方为无副作用超集（父目录已存在时 MkdirAll 为 no-op）。
// 收敛上述四处同义实现。
func WriteFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// InitTempRepo 初始化一个临时 git 仓库并配置身份（test@test.com / test），返回仓库根目录。
//
// 收敛 service.initTempRepo 与各包内联的 git init + config user.email/user.name 模式。
// 不设置初始分支（沿用 git 默认）；如需显式 master 分支请配合 SetupMasterBranch。
func InitTempRepo(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	RunGit(t, dir, "init")
	RunGit(t, dir, "config", "user.email", "test@test.com")
	RunGit(t, dir, "config", "user.name", "test")
	return dir
}

// SetupMasterBranch 显式以 master 作为初始分支，规避 git 新版默认 main 致分支名歧义。
//
// 收敛 service.svcSetupMasterBranch 与 util.setupMasterBranch 两处同义实现。
// 须在首次提交前调用（修改未出生的 HEAD symbolic ref）。
func SetupMasterBranch(t testing.TB, dir string) {
	t.Helper()
	RunGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/master")
}

// SetupFFRepo 构造可快进合并仓库：master 基线提交，feature 领先一个提交，切回 master。
//
// merge feature 进 master 为 fast-forward，无冲突。返回仓库根目录。
// 收敛 service.svcSetupFFRepo 与 util.setupFFRepo 两处同义 fixture。
func SetupFFRepo(t testing.TB) string {
	t.Helper()
	dir := InitTempRepo(t)
	SetupMasterBranch(t, dir)
	WriteFile(t, filepath.Join(dir, "a.txt"), "base\n")
	RunGit(t, dir, "add", "a.txt")
	RunGit(t, dir, "commit", "-m", "base")
	RunGit(t, dir, "checkout", "-b", "feature")
	WriteFile(t, filepath.Join(dir, "a.txt"), "feature\n")
	RunGit(t, dir, "add", "a.txt")
	RunGit(t, dir, "commit", "-m", "feature")
	RunGit(t, dir, "checkout", "master")
	return dir
}

// SetupConflictRepo 构造冲突仓库：master 与 feature 各改 a.txt 同一行 line2 并提交，
// merge feature 进 master 必然产生冲突。返回仓库根目录。
//
// 收敛 service.svcSetupConflictRepo 与 util.setupConflictRepo 两处同义 fixture。
func SetupConflictRepo(t testing.TB) string {
	t.Helper()
	dir := InitTempRepo(t)
	SetupMasterBranch(t, dir)
	WriteFile(t, filepath.Join(dir, "a.txt"), "line1\nline2\nline3\n")
	RunGit(t, dir, "add", "a.txt")
	RunGit(t, dir, "commit", "-m", "base")
	RunGit(t, dir, "checkout", "-b", "feature")
	WriteFile(t, filepath.Join(dir, "a.txt"), "line1\nfeature-line2\nline3\n")
	RunGit(t, dir, "add", "a.txt")
	RunGit(t, dir, "commit", "-m", "feature change line2")
	RunGit(t, dir, "checkout", "master")
	WriteFile(t, filepath.Join(dir, "a.txt"), "line1\nmaster-line2\nline3\n")
	RunGit(t, dir, "add", "a.txt")
	RunGit(t, dir, "commit", "-m", "master change line2")
	return dir
}
