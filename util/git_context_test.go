package util

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"workbench/model"
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

// TestExecuteWithContext_TimeoutReturnsAppError 超时路径收口：返回 E_GIT_TIMEOUT
// 引导级 AppError，%w 包装 DeadlineExceeded 保留 errors.Is 穿透（供上层判定超时），
// 不误判为取消。（2026-09-29 收口：既有裸错误格式 `git [args] failed: <stderr>`
// 有意推翻——超时曾被退出码白名单/锁 parse 吞为冲突假态或误导性空摘要，格式变更
// 已在 docs/spec/logging-and-errors.md 明示。）
func TestExecuteWithContext_TimeoutReturnsAppError(t *testing.T) {
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
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("超时错误应保留 DeadlineExceeded 穿透供上层判定: %v", err)
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("超时应返回 AppError，实际: %v", err)
	}
	if appErr.Code != model.ErrCodeGitTimeout {
		t.Errorf("错误码应为 E_GIT_TIMEOUT，实际: %s", appErr.Code)
	}
}

// TestExecuteWithCodesWithContext_TimeoutNotSwallowedByWhitelist 核心收口验收：
// 超时被杀 exit 1 恰命中 conflictExitCodes={1} 时，不得被白名单吞为
// (stdout, nil)「冲突正常返回」（曾致冲突面板假态），须返回 E_GIT_TIMEOUT 错误。
func TestExecuteWithCodesWithContext_TimeoutNotSwallowedByWhitelist(t *testing.T) {
	dir := t.TempDir()
	testutil.RunGit(t, dir, "init")

	g := NewGitCommandWithTimeout(1 * time.Nanosecond) // 立即超时
	output, err := g.ExecuteWithCodesWithContext(context.Background(), dir, conflictExitCodes, "status")
	if err == nil {
		t.Fatalf("超时不得被白名单吞为正常返回（output %q）", output)
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeGitTimeout {
		t.Fatalf("超时应返回 E_GIT_TIMEOUT AppError，实际: %v", err)
	}
}

// TestCloneWithContext_TimeoutReturnsAppError 克隆超时与 run 收口对称：返回
// E_GIT_TIMEOUT 引导级 AppError，不落入锁 parse 误判、不返回裸错误。
func TestCloneWithContext_TimeoutReturnsAppError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	g := NewGitCommand()
	_, err := g.CloneWithContext(ctx, "https://example.com/repo.git", t.TempDir())
	if err == nil {
		t.Fatal("克隆超时应返回错误")
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeGitTimeout {
		t.Fatalf("克隆超时应返回 E_GIT_TIMEOUT AppError，实际: %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("超时不应误判为取消: %v", err)
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

// TestConflictOpsWithContext_Canceled 冲突类七方法 ctx 变体贯穿取消信号（预取消模式，
// 无时序依赖）：Merge/Rebase/CherryPick 及 Continue/Skip 系在 ctx 已取消时均不执行命令，
// 直接返回包装的 context.Canceled。
func TestConflictOpsWithContext_Canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 预先取消：稳定触发，无真实慢命令依赖

	g := NewGitCommand()
	cases := []struct {
		name string
		run  func() error
	}{
		{"MergeWithContext", func() error { _, err := g.MergeWithContext(ctx, t.TempDir(), "b", "no-ff"); return err }},
		{"RebaseWithContext", func() error { _, err := g.RebaseWithContext(ctx, t.TempDir(), "b"); return err }},
		{"CherryPickWithContext", func() error { _, err := g.CherryPickWithContext(ctx, t.TempDir(), "HEAD"); return err }},
		{"MergeContinueWithContext", func() error { _, err := g.MergeContinueWithContext(ctx, t.TempDir()); return err }},
		{"RebaseContinueWithContext", func() error { _, err := g.RebaseContinueWithContext(ctx, t.TempDir()); return err }},
		{"CherryPickContinueWithContext", func() error { _, err := g.CherryPickContinueWithContext(ctx, t.TempDir()); return err }},
		{"RebaseSkipWithContext", func() error { _, err := g.RebaseSkipWithContext(ctx, t.TempDir()); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("ctx 已取消应返回 context.Canceled，实际: %v", err)
			}
		})
	}
}

// TestMergeWithContext_FFSuccess 正常路径冒烟：ctx 未取消时可快进合并成功，验证
// mergeArgs 参数构造（ff 模式不附加策略 flag）与冲突码白名单接受语义在 ctx 变体下不变。
func TestMergeWithContext_FFSuccess(t *testing.T) {
	dir := testutil.SetupFFRepo(t)

	g := NewGitCommand()
	output, err := g.MergeWithContext(context.Background(), dir, "feature", "ff")
	if err != nil {
		t.Fatalf("可快进合并应成功: %v (output %q)", err, output)
	}
	if g.IsMergeInProgress(dir) {
		t.Error("ff 合并成功后不应处于冲突态")
	}
}

// TestMergeWithContext_ConflictAccepted 冲突路径冒烟：ctx 变体下冲突 exit 1 仍被
// conflictExitCodes 接受为正常返回（验收项：冲突接受语义保留）。
func TestMergeWithContext_ConflictAccepted(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)

	g := NewGitCommand()
	if _, err := g.MergeWithContext(context.Background(), dir, "feature", "ff"); err != nil {
		t.Fatalf("冲突 exit 1 应被接受为正常返回: %v", err)
	}
	if !g.IsMergeInProgress(dir) {
		t.Error("冲突合并后应处于合并冲突态")
	}
}
