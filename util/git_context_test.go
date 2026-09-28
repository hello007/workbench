package util

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"workbench/util/testutil"
)

// TestExecuteWithContext_CanceledCtxReturnsCanceledError ctx 已取消时命令不执行，
// 错误以 %w 包装 context.Canceled，供上层 errors.Is 识别并映射「操作已取消」。
// exec.CommandContext 对已取消 ctx 在 Start 前即返回 ctx.Err()，无时序依赖。
func TestExecuteWithContext_CanceledCtxReturnsCanceledError(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 预先取消：稳定触发，无真实慢命令依赖

	g := NewGitCommand()
	_, err := g.ExecuteWithContext(ctx, dir, "status")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx 已取消应返回 context.Canceled，实际: %v", err)
	}
	if !strings.Contains(err.Error(), "git [status] failed") {
		t.Errorf("错误应保留命令前缀便于定位，实际: %v", err)
	}
}

// TestExecuteWithContext_TimeoutErrorFormatUnchanged 超时路径（非取消）行为不变：
// 错误保持既有 stderr 摘要格式，不包装 DeadlineExceeded（验收项：超时行为零漂移）。
func TestExecuteWithContext_TimeoutErrorFormatUnchanged(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")

	g := NewGitCommandWithTimeout(1 * time.Nanosecond) // 立即超时
	_, err := g.ExecuteWithContext(context.Background(), dir, "status")
	if err == nil {
		t.Fatal("超时应返回错误")
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("超时不应误判为取消: %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("超时错误保持既有格式，不包装 DeadlineExceeded: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "git [status] failed:") {
		t.Errorf("超时错误格式应与历史一致（git [...] failed: <stderr>），实际: %v", err)
	}
}

// TestExecuteWithCodesWithContext_Canceled 冲突码白名单变体同样贯穿取消信号，
// 取消错误不走白名单分支（exit code 不可得），直接包装 Canceled。
func TestExecuteWithCodesWithContext_Canceled(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := NewGitCommand()
	_, err := g.ExecuteWithCodesWithContext(ctx, dir, conflictExitCodes, "status")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("codes 变体 ctx 已取消应返回 context.Canceled，实际: %v", err)
	}
}

// TestCloneWithContext_Canceled 克隆 ctx 变体贯穿取消信号。
func TestCloneWithContext_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := NewGitCommand()
	_, err := g.CloneWithContext(ctx, "https://example.com/repo.git", t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CloneWithContext ctx 已取消应返回 context.Canceled，实际: %v", err)
	}
}

// TestPullWithContext_NoRemoteNormalError 冒烟：ctx 未取消时 pull 走普通错误路径
// （无远程配置），取消包装不误伤正常错误语义。
func TestPullWithContext_NoRemoteNormalError(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")

	g := NewGitCommand()
	_, err := g.PullWithContext(context.Background(), dir)
	if err == nil {
		t.Fatal("无远程仓库 pull 应报错")
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("普通失败不应误判为取消: %v", err)
	}
}
