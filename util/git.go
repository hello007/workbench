package util

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
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

// Execute 执行Git命令
func (g *GitCommand) Execute(workDir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workDir

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

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git clone failed: %s", stderr.String())
	}

	return stdout.String(), nil
}

// Pull 拉取更新
func (g *GitCommand) Pull(dir string) (string, error) {
	return g.Execute(dir, "pull")
}
