package service

import (
	"context"
	"errors"
	"fmt"
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
