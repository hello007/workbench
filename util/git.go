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

// IsGitRepository 检查目录是否是Git仓库
func (g *GitCommand) IsGitRepository(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	HideCommandWindow(cmd)
	return cmd.Run() == nil
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
