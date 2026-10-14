package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"workbench/model"
	"workbench/util/testutil"
)

// TestCancelGitOperation_NoInFlight 无在途操作时取消幂等返回 false，不报错。
func TestCancelGitOperation_NoInFlight(t *testing.T) {
	svc := NewGitService()

	cancelled, err := svc.CancelGitOperation(t.TempDir())
	if err != nil {
		t.Fatalf("无在途操作取消不应报错: %v", err)
	}
	if cancelled {
		t.Error("无在途操作应返回 false")
	}
}

// TestCancelRegistry_Lifecycle 注册表生命周期：注册后可取消（返回 true 且 CancelFunc
// 被调用、注册表清空），再次取消幂等返回 false。操作完成/取消路径注册表无残留
// （验收项：取消后立即重试可正常获锁执行的前提）。
func TestCancelRegistry_Lifecycle(t *testing.T) {
	svc := NewGitService()
	dir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entry := svc.registerCancel(dir, cancel)

	cancelled, err := svc.CancelGitOperation(dir)
	if err != nil || !cancelled {
		t.Fatalf("注册后取消应返回 true, got cancelled=%v err=%v", cancelled, err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("CancelGitOperation 应触发已注册的 CancelFunc")
	}

	// 取消即清注册表：再次取消幂等 false；旧条目注销不 panic（已被取走，指针不匹配）
	again, err := svc.CancelGitOperation(dir)
	if err != nil || again {
		t.Fatalf("取消后注册表应已清空, got cancelled=%v err=%v", again, err)
	}
	svc.unregisterCancel(dir, entry)
}

// TestUnregisterCancel_Idempotent 操作方注销幂等：nil 条目与重复注销均不 panic、
// 不误删，注册表收敛为空。
func TestUnregisterCancel_Idempotent(t *testing.T) {
	svc := NewGitService()
	dir := t.TempDir()

	svc.unregisterCancel(dir, nil) // nil 条目注销不应 panic

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	entry := svc.registerCancel(dir, cancel)
	svc.unregisterCancel(dir, entry)
	svc.unregisterCancel(dir, entry) // 重复注销不应 panic（键已删，指针不匹配不删他人）

	if got, err := svc.CancelGitOperation(dir); got || err != nil {
		t.Errorf("注销后应无可取消项, got cancelled=%v err=%v", got, err)
	}
}

// TestUnregisterCancel_StaleEntryNotDeletingNewRegistration 同仓并发双操作单槽覆盖
// 回归：BatchPull 在途 + 用户单仓 PullRepo 时后注册覆盖前注册，先结束操作的注销
// 不得误删后注册操作的在途注册项（条目指针身份校验），也不得触发对方 CancelFunc。
func TestUnregisterCancel_StaleEntryNotDeletingNewRegistration(t *testing.T) {
	svc := NewGitService()
	dir := t.TempDir()

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	entry1 := svc.registerCancel(dir, cancel1)

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	svc.registerCancel(dir, cancel2) // 覆盖 entry1 的单槽

	// 先注册操作先结束：注销 entry1 不得删除 entry2 的在途注册项
	svc.unregisterCancel(dir, entry1)

	got, err := svc.CancelGitOperation(dir)
	if err != nil || !got {
		t.Fatalf("entry1 注销后 entry2 仍应可取消, got %v err=%v", got, err)
	}
	select {
	case <-ctx2.Done():
	case <-time.After(time.Second):
		t.Fatal("entry2 的 CancelFunc 未被触发")
	}
	select {
	case <-ctx1.Done():
		t.Fatal("entry1 不应被误取消（其 CancelFunc 已不在注册表）")
	default:
	}
}

// TestCancelGitOperation_KeyNormalized 取消注册表与锁同键规范化：
// 一侧传原始路径一侧传尾部冗余分隔符/点段形式，normalizeRepoKey 经 filepath.Abs
// 归一命中同一注册表项，避免路径形式差异绕过取消。
func TestCancelGitOperation_KeyNormalized(t *testing.T) {
	svc := NewGitService()
	dir := t.TempDir()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.registerCancel(dir, cancel)

	// dir + "/." 经 Abs/Clean 归一为同一绝对路径
	cancelled, err := svc.CancelGitOperation(dir + "/.")
	if err != nil {
		t.Fatalf("取消不应报错: %v", err)
	}
	if !cancelled {
		t.Error("路径形式差异应归一命中同一注册表项")
	}
}

// TestCancelAllGitOperations_Batch 批量取消：全部注册项逐一取消并清空注册表，
// 返回取消数；已清空后再次调用返回 0（已完成仓库不在注册表自然保留的语义基础）。
func TestCancelAllGitOperations_Batch(t *testing.T) {
	svc := NewGitService()
	dirs := []string{t.TempDir(), t.TempDir(), t.TempDir()}

	ctxs := make([]context.Context, 0, len(dirs))
	for _, dir := range dirs {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		svc.registerCancel(dir, cancel)
		ctxs = append(ctxs, ctx)
	}

	if got := svc.CancelAllGitOperations(); got != len(dirs) {
		t.Fatalf("应取消 %d 项, got %d", len(dirs), got)
	}
	for i, ctx := range ctxs {
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatalf("第 %d 项 CancelFunc 未被触发", i)
		}
	}
	if got := svc.CancelAllGitOperations(); got != 0 {
		t.Fatalf("注册表应已清空, 二次取消 got %d", got)
	}
}

// TestPullOne_CtxCanceledSkipsExecution ctx 已取消（排队中未启动被打断）时
// pullOne 直接产出已取消结果，不执行仓库检查与 pull（pathspec：路径故意不存在，
// 若执行了真实检查会产出「不是 Git 仓库」错误而非 cancelled，断言可区分）。
func TestPullOne_CtxCanceledSkipsExecution(t *testing.T) {
	svc := NewGitService()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := svc.pullOne(ctx, t.TempDir()+"/nonexistent-repo")
	if !result.Cancelled {
		t.Fatalf("ctx 已取消应产出 Cancelled=true，实际: %+v", result)
	}
	if result.Success || result.Skipped || result.Error != "" {
		t.Errorf("未启动跳过不应有成功/跳过/错误标记，实际: %+v", result)
	}
}

// TestPullOne_NoRemoteSkippedNormalCtx 回归：ctx 正常时 pullOne 走既有语义
// （无远程仓 → Skipped），取消链路不误伤正常路径。
func TestPullOne_NoRemoteSkippedNormalCtx(t *testing.T) {
	svc := NewGitService()
	dir := testutil.InitTempRepo(t)

	result := svc.pullOne(context.Background(), dir)
	if result.Cancelled {
		t.Errorf("未取消不应标记 Cancelled: %+v", result)
	}
	if !result.Skipped {
		t.Errorf("无远程仓库应 Skipped（InitTempRepo 不配远程），实际: %+v", result)
	}
}

// TestMapCancelError 取消错误映射单点：util 层 %w 包装的 context.Canceled 映射为
// ErrOperationCancelled sentinel（前端 E_GIT_CANCELLED 中性分流的来源），
// 普通错误与 nil 原样透传。
func TestMapCancelError(t *testing.T) {
	wrapped := fmt.Errorf("git [pull] failed: %w", context.Canceled)
	got := mapCancelError(wrapped)
	var appErr *model.AppError
	if !errors.As(got, &appErr) || appErr.Code != model.ErrCodeGitCancelled {
		t.Fatalf("取消错误应映射为 E_GIT_CANCELLED AppError，实际: %v", got)
	}
	if !errors.Is(got, ErrOperationCancelled) {
		t.Error("映射结果应可 errors.Is ErrOperationCancelled")
	}

	normal := fmt.Errorf("git [pull] failed: some stderr")
	if mapCancelError(normal) != normal {
		t.Error("普通错误应原样透传")
	}
	if err := mapCancelError(nil); err != nil {
		t.Error("nil 应透传为 nil")
	}
}

// TestBatchPull_CompletePayloadHasCancelledKey pull-complete 汇总载荷含 cancelled 键
// （前端读 summary.cancelled，缺键会显示 undefined）。用无远程仓全跳过路径稳定驱动
// 完整批量流程，cancelled=0 仅验证键存在；非零取值语义由 pullOne 取消单测覆盖
// （真实在途取消时序在单测中不可稳定构造，归手动/E2E 验收）。
func TestBatchPull_CompletePayloadHasCancelledKey(t *testing.T) {
	svc := NewGitService()
	fake := &fakeEventSink{}
	svc.SetEventSink(fake)

	dirs := []string{testutil.InitTempRepo(t), testutil.InitTempRepo(t)}
	svc.BatchPull(dirs, 2)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	var complete *fakeEmitCall
	for i := range fake.calls {
		if fake.calls[i].name == "pull-complete" {
			complete = &fake.calls[i]
		}
	}
	if complete == nil {
		t.Fatal("应发出 pull-complete 事件")
	}
	payload, ok := complete.data[0].(map[string]int)
	if !ok {
		t.Fatalf("pull-complete 载荷应为 map[string]int，实际: %T", complete.data[0])
	}
	if _, has := payload["cancelled"]; !has {
		t.Errorf("pull-complete 载荷应含 cancelled 键，实际: %v", payload)
	}
}

// TestConflictOps_CancelRegistryClearedAfterRun 冲突类七方法取消注册表生命周期：
// 每方法真实执行一次（成功或冲突 exit 1 接受路径），断言操作穿透到执行层（无锁拒绝）、
// 返回后注册表已清理（CancelGitOperation 返回 false，defer unregisterCancel 无条件收口）、
// 立即重试可重新获锁执行（验收项：取消/完成后重试可正常获锁的前提）。
// 预先注册一个占位 ctx 验证 forward 方法自建注册覆盖与清理互不干扰。
func TestConflictOps_CancelRegistryClearedAfterRun(t *testing.T) {
	revParse := func(t *testing.T, dir, ref string) string {
		t.Helper()
		out, err := NewGitService().gitCmd.Execute(dir, "rev-parse", ref)
		if err != nil {
			t.Fatalf("rev-parse %s: %v", ref, err)
		}
		return strings.TrimSpace(out)
	}

	cases := []struct {
		name string
		// prepare 构造该方法的前置仓库状态（如 Continue 系需先进入冲突态），返回操作目录
		prepare func(t *testing.T) string
		run     func(svc *GitService, dir string) (string, error)
	}{
		{"Merge", func(t *testing.T) string { return testutil.SetupConflictRepo(t) },
			func(svc *GitService, dir string) (string, error) { return svc.Merge(dir, "feature", model.MergeModeFF) }},
		{"Rebase", func(t *testing.T) string { return testutil.SetupConflictRepo(t) },
			func(svc *GitService, dir string) (string, error) { return svc.Rebase(dir, "feature") }},
		{"CherryPick", func(t *testing.T) string { return testutil.SetupConflictRepo(t) },
			func(svc *GitService, dir string) (string, error) {
				return svc.CherryPick(dir, revParse(t, dir, "feature"))
			}},
		{"ContinueMerge", func(t *testing.T) string {
			dir := testutil.SetupConflictRepo(t)
			if _, err := NewGitService().Merge(dir, "feature", model.MergeModeFF); err != nil {
				t.Fatalf("prepare 进入合并冲突态失败: %v", err)
			}
			return dir
		}, func(svc *GitService, dir string) (string, error) { return svc.ContinueMerge(dir) }},
		{"ContinueRebase", func(t *testing.T) string {
			dir := testutil.SetupConflictRepo(t)
			if _, err := NewGitService().Rebase(dir, "feature"); err != nil {
				t.Fatalf("prepare 进入变基冲突态失败: %v", err)
			}
			return dir
		}, func(svc *GitService, dir string) (string, error) { return svc.ContinueRebase(dir) }},
		{"ContinueCherryPick", func(t *testing.T) string {
			dir := testutil.SetupConflictRepo(t)
			if _, err := NewGitService().CherryPick(dir, revParse(t, dir, "feature")); err != nil {
				t.Fatalf("prepare 进入拣选冲突态失败: %v", err)
			}
			return dir
		}, func(svc *GitService, dir string) (string, error) { return svc.ContinueCherryPick(dir) }},
		{"SkipRebase", func(t *testing.T) string {
			dir := testutil.SetupConflictRepo(t)
			if _, err := NewGitService().Rebase(dir, "feature"); err != nil {
				t.Fatalf("prepare 进入变基冲突态失败: %v", err)
			}
			return dir
		}, func(svc *GitService, dir string) (string, error) { return svc.SkipRebase(dir) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewGitService()
			dir := tc.prepare(t)

			// 占位注册：验证 forward 方法自建注册覆盖单槽后，其 defer 注销
			// 不会误删/误触发占位（单槽覆盖回归在注册表层已有专项测试）
			_, staleCancel := context.WithCancel(context.Background())
			defer staleCancel()
			svc.registerCancel(dir, staleCancel)

			output, err := tc.run(svc, dir)
			if err != nil && strings.Contains(err.Error(), "该仓库有 Git 操作进行中") {
				t.Fatalf("操作应穿透到执行层（锁拒绝不应发生）: %v", err)
			}
			_ = output // 冲突 exit 1 被接受为正常返回；结果内容由 git_merge_test.go 既有用例覆盖

			// 操作返回后注册表必须清空（占位条目已被执行方的单槽覆盖取走并清理）
			if cancelled, err := svc.CancelGitOperation(dir); cancelled || err != nil {
				t.Errorf("操作返回后注册表应已清空, got cancelled=%v err=%v", cancelled, err)
			}

			// 立即重试可重新获锁执行（锁拒绝不应发生；git 层结果不关心）
			if _, err := tc.run(svc, dir); err != nil && strings.Contains(err.Error(), "该仓库有 Git 操作进行中") {
				t.Errorf("重试应可正常获锁: %v", err)
			}
		})
	}
}

// TestConflictOps_CancelDuringPrecheckNoSideEffect 无在途注册时取消为幂等无害
// （本设计 TryLock 互斥拒绝不排队）：取消返回 false，不报错、不影响后续执行。
func TestConflictOps_CancelDuringPrecheckNoSideEffect(t *testing.T) {
	svc := NewGitService()
	dir := testutil.SetupConflictRepo(t)

	cancelled, err := svc.CancelGitOperation(dir)
	if err != nil || cancelled {
		t.Fatalf("无在途注册取消应幂等返回 false, got cancelled=%v err=%v", cancelled, err)
	}

	// 取消探测不干扰后续操作：merge 照常穿透执行
	if _, err := svc.Merge(dir, "feature", model.MergeModeFF); err != nil {
		t.Fatalf("取消探测后 merge 应正常执行: %v", err)
	}
}
