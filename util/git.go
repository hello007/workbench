package util

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitCommand Git命令执行器
type GitCommand struct {
	timeout time.Duration
}

// NewGitCommand 创建Git命令执行器
func NewGitCommand() *GitCommand {
	return &GitCommand{
		timeout: 30 * time.Second,
	}
}

// NewGitCommandWithTimeout 创建指定超时时间的 Git 命令执行器
func NewGitCommandWithTimeout(timeout time.Duration) *GitCommand {
	return &GitCommand{
		timeout: timeout,
	}
}

// Execute 执行Git命令
func (g *GitCommand) Execute(workDir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir
	HideCommandWindow(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %v failed: %s", args, stderr.String())
	}

	return stdout.String(), nil
}

// ExecuteWithCodes 执行 Git 命令，允许指定退出码视为正常（如 git diff --no-index 有差异时退出码 1）。
// 若退出码在 acceptedExitCodes 中，返回 stdout 与 nil；否则按 Execute 语义返回错误。
func (g *GitCommand) ExecuteWithCodes(workDir string, acceptedExitCodes map[int]bool, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir
	HideCommandWindow(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && acceptedExitCodes[exitErr.ExitCode()] {
			return stdout.String(), nil
		}
		return "", fmt.Errorf("git %v failed: %s", args, stderr.String())
	}

	return stdout.String(), nil
}

// IsGitRepository 检查目录是否是Git仓库
func (g *GitCommand) IsGitRepository(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	HideCommandWindow(cmd)
	return cmd.Run() == nil
}

// IsGitRepositoryFast 基于 .git 条目存在性快速判定目录是否为 Git 仓库。
// 仅判 os.Stat 无错，不要求 .git 是目录，从而覆盖 worktree/submodule 的 .git 文件场景
// （这两类的 .git 是文件，内容形如 "gitdir: /path/..."，若用 IsDir() 判定会漏判）。
// 与现有 IsGitRepository（fork git rev-parse）相比，单次 os.Stat 约 0.1ms，快 200~1000 倍。
//
// 已知边界：bare repo（无 .git 条目）会漏判，桌面工作目录场景可忽略；
// 损坏/残留 .git 会误判为仓库，后续真实 git 操作时会报错暴露，不产生静默数据错误。
//
// 注意：现有 FindGitRoot（本文件下方）使用 info.IsDir() 判定，对 worktree/submodule 漏判，
// 属已存在的潜在 bug，本函数避免重蹈覆辙。
func IsGitRepositoryFast(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// GetBranch 获取当前分支名
func (g *GitCommand) GetBranch(dir string) (string, error) {
	return g.Execute(dir, "branch", "--show-current")
}

// GetRemote 获取远程仓库URL
func (g *GitCommand) GetRemote(dir string) (string, string, error) {
	lines, err := g.ExecuteWithOutput(dir, "remote", "-v")
	if err != nil {
		return "", "", err
	}

	if len(lines) == 0 {
		return "", "", fmt.Errorf("no remote configured")
	}

	parts := strings.Fields(lines[0])
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid remote format")
	}

	return parts[0], strings.TrimSuffix(parts[1], " (fetch)"), nil
}

// ExecuteWithOutput 执行并返回行分割输出
func (g *GitCommand) ExecuteWithOutput(workDir string, args ...string) ([]string, error) {
	output, err := g.Execute(workDir, args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	return lines, nil
}

// Clone 克隆仓库
func (g *GitCommand) Clone(url, targetPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "clone", url, targetPath)
	HideCommandWindow(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git clone failed: %s", stderr.String())
	}

	return stdout.String(), nil
}

// FindGitRoot 从给定路径向上查找 Git 仓库根目录
func FindGitRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	for {
		gitDir := filepath.Join(abs, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("not a git repository: %s", path)
		}
		abs = parent
	}
}

// Pull 拉取更新
func (g *GitCommand) Pull(dir string) (string, error) {
	return g.Execute(dir, "pull")
}

// GetBranchesAll 获取所有分支（本地+远程）
func (g *GitCommand) GetBranchesAll(dir string) (string, error) {
	return g.Execute(dir, "branch", "-a")
}

// HasLocalChanges 检查是否有未提交的变更
func (g *GitCommand) HasLocalChanges(dir string) (bool, error) {
	output, err := g.Execute(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// CheckoutLocal 切换到本地分支
func (g *GitCommand) CheckoutLocal(dir, branch string) (string, error) {
	return g.Execute(dir, "checkout", branch)
}

// CheckoutRemote 从远程分支创建本地分支并跟踪
func (g *GitCommand) CheckoutRemote(dir, remoteBranch, localBranch string) (string, error) {
	return g.Execute(dir, "checkout", "-b", localBranch, remoteBranch)
}

// ===== 合并 / 变基 / 拣选 =====
//
// 以下操作均可能因冲突以 exit 1 正常结束，统一走 ExecuteWithCodes 接受退出码 1。
// 调用方据返回的 stdout/stderr 与 IsXxxInProgress 判定是否进入冲突态。
// workDir 须为仓库根目录（由 service 层 FindGitRoot 保证），冲突态检测依赖 .git 目录。

// conflictExitCodes 冲突类操作可接受的退出码集合：exit 1 表示存在冲突，视为正常返回。
var conflictExitCodes = map[int]bool{1: true}

// Merge 合并指定分支到当前分支。mode 取 ff/no-ff/squash：
// ff 走 git 默认行为（可快进时快进），no-ff 强制合并提交，squash 压缩为单个暂存变更。
// mode 以 string 传入而非 model.MergeMode，避免 util 层反向依赖 model，保持底层纯净。
func (g *GitCommand) Merge(workDir, branch, mode string) (string, error) {
	args := []string{"merge"}
	switch mode {
	case "no-ff":
		args = append(args, "--no-ff")
	case "squash":
		args = append(args, "--squash")
	default:
		// ff：git 默认即可快进时快进，不附加 flag
	}
	args = append(args, branch)
	return g.ExecuteWithCodes(workDir, conflictExitCodes, args...)
}

// Rebase 将当前分支变基到指定分支之上（git rebase <branch>）。
func (g *GitCommand) Rebase(workDir, branch string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "rebase", branch)
}

// CherryPick 将指定提交拣选到当前分支（git cherry-pick <sha>）。
func (g *GitCommand) CherryPick(workDir, sha string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "cherry-pick", sha)
}

// PullRebase 拉取远程更新并以变基方式重放本地提交（git pull --rebase）。
func (g *GitCommand) PullRebase(workDir string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "pull", "--rebase")
}

// MergeAbort 中止进行中的合并，回滚到合并前状态（git merge --abort）。
func (g *GitCommand) MergeAbort(workDir string) (string, error) {
	return g.Execute(workDir, "merge", "--abort")
}

// MergeContinue 合并冲突解决后提交合并。走 git commit --no-edit 复用预生成的 MERGE_MSG，
// 避免弹出编辑器。仍可能因遗留冲突以 exit 1 失败，走 ExecuteWithCodes 接受。
func (g *GitCommand) MergeContinue(workDir string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "commit", "--no-edit")
}

// RebaseAbort 中止进行中的变基，回滚到变基前分支位置（git rebase --abort）。
func (g *GitCommand) RebaseAbort(workDir string) (string, error) {
	return g.Execute(workDir, "rebase", "--abort")
}

// RebaseContinue 解决冲突后继续变基。可能再次冲突，走 ExecuteWithCodes 接受 exit 1。
func (g *GitCommand) RebaseContinue(workDir string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "rebase", "--continue")
}

// RebaseSkip 跳过当前冲突提交继续变基（git rebase --skip）。
func (g *GitCommand) RebaseSkip(workDir string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "rebase", "--skip")
}

// CherryPickAbort 中止进行中的拣选（git cherry-pick --abort）。
func (g *GitCommand) CherryPickAbort(workDir string) (string, error) {
	return g.Execute(workDir, "cherry-pick", "--abort")
}

// CherryPickContinue 解决冲突后继续拣选。可能再次冲突，走 ExecuteWithCodes 接受 exit 1。
func (g *GitCommand) CherryPickContinue(workDir string) (string, error) {
	return g.ExecuteWithCodes(workDir, conflictExitCodes, "cherry-pick", "--continue")
}

// ListConflictFiles 列出未解决冲突文件（git diff --name-only --diff-filter=U）。
// 返回相对仓库根的文件路径列表，无冲突时返回空切片。
func (g *GitCommand) ListConflictFiles(workDir string) ([]string, error) {
	output, err := g.Execute(workDir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return []string{}, nil
	}
	return strings.Split(output, "\n"), nil
}

// IsMergeInProgress 检测是否处于合并冲突态：.git/MERGE_HEAD 存在即合并进行中。
func (g *GitCommand) IsMergeInProgress(workDir string) bool {
	return fileExists(filepath.Join(workDir, ".git", "MERGE_HEAD"))
}

// IsRebaseInProgress 检测是否处于变基态：.git/rebase-merge/ 或 .git/rebase-apply/ 存在。
func (g *GitCommand) IsRebaseInProgress(workDir string) bool {
	return fileExists(filepath.Join(workDir, ".git", "rebase-merge")) ||
		fileExists(filepath.Join(workDir, ".git", "rebase-apply"))
}

// IsCherryPickInProgress 检测是否处于拣选冲突态：.git/CHERRY_PICK_HEAD 存在即拣选进行中。
func (g *GitCommand) IsCherryPickInProgress(workDir string) bool {
	return fileExists(filepath.Join(workDir, ".git", "CHERRY_PICK_HEAD"))
}

// fileExists 判定路径存在性（文件或目录均可），供冲突态检测复用。
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
