//go:build windows

// 素材缩略图 COM 实现可注入面单测（Windows 原生，零 COM 真调用——COM 真链路归
// 集成冒烟验证）：页级 Export 瞬态重试（修复 1）+ Discard 重建 owned 退出等待
// （修复 2）。退避/轮询间隔经包级 var 注入 0 或极小值，禁 sleep 等待竞态
// （test-stability 契约）；平台耦合断言独立 _windows_test.go 文件
// （linux-platform 契约）。

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-ole/go-ole"
)

// transientExportErr 按 go-ole 真实形态构造瞬态错误：DISP_E_EXCEPTION + 空
// ExcepInfo（go-ole Invoke 失败分支把 null bstrDescription 经 renderStrings 渲染
// 为字面量 "<nil>"——真机日志「发生意外。 (<nil>)」同源形态）。
func transientExportErr() *ole.OleError {
	return ole.NewErrorWithSubError(dispEException, "<nil>", ole.EXCEPINFO{})
}

// withThumbExportRetryParams 注入页级重试参数（保存/恢复包级 var，包内串行无竞态）。
func withThumbExportRetryParams(t *testing.T, maxAttempts int, delay time.Duration) {
	t.Helper()
	oldMax, oldDelay := thumbExportMaxAttempts, thumbExportRetryDelay
	thumbExportMaxAttempts, thumbExportRetryDelay = maxAttempts, delay
	t.Cleanup(func() { thumbExportMaxAttempts, thumbExportRetryDelay = oldMax, oldDelay })
}

// withThumbOwnedExitWaitParams 注入 owned 退出等待参数与探针（抽象注入点，
// 勿直调全局 officeServerPIDSet 枚举）。
func withThumbOwnedExitWaitParams(t *testing.T, interval, waitCap time.Duration, probe func() map[int]bool) {
	t.Helper()
	oldInterval, oldWait, oldProbe := thumbOwnedExitPollInterval, thumbOwnedExitWait, thumbOwnedExitProbe
	thumbOwnedExitPollInterval, thumbOwnedExitWait, thumbOwnedExitProbe = interval, waitCap, probe
	t.Cleanup(func() {
		thumbOwnedExitPollInterval, thumbOwnedExitWait, thumbOwnedExitProbe = oldInterval, oldWait, oldProbe
	})
}

// TestIsTransientExportError 瞬态判定口径表驱动：真机形态（0x80020009 + 空
// ExcepInfo「<nil>」/空串）与描述含「发生意外」均瞬态；实质业务描述、文件损坏
// 类、非 COM 包装错误、ctx 取消/超时（前置判定）均非瞬态。
func TestIsTransientExportError(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		transient bool
	}{
		{"真机形态：0x80020009+空ExcepInfo字面量nil", ole.NewErrorWithSubError(dispEException, "<nil>", ole.EXCEPINFO{}), true},
		{"0x80020009+空描述", ole.NewErrorWithDescription(dispEException, ""), true},
		{"描述含发生意外", ole.NewErrorWithDescription(dispEException, "发生意外。"), true},
		{"描述含发生意外但其他hr", ole.NewErrorWithDescription(0x8000FFFF, "发生意外。"), true},
		{"0x80020009但实质描述不赌重试", ole.NewErrorWithDescription(dispEException, "页序超出范围"), false},
		{"文件损坏类其他hr", ole.NewErrorWithDescription(0x8000FFFF, "文件已损坏"), false},
		{"非COM包装错误", errors.New("COM 打开失败"), false},
		{"ctx取消前置判定", fmt.Errorf("invoke: %w", context.Canceled), false},
		{"ctx超时前置判定", fmt.Errorf("invoke: %w", context.DeadlineExceeded), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientExportError(tc.err); got != tc.transient {
				t.Fatalf("isTransientExportError = %v, want %v（err=%v）", got, tc.transient, tc.err)
			}
		})
	}
}

// TestRetryTransientSlideExport_TransientThenSuccess 修复 1 主路径：前两次瞬态
// 第三次成功 → 重试至成功且返回 nil（导出继续）。
func TestRetryTransientSlideExport_TransientThenSuccess(t *testing.T) {
	withThumbExportRetryParams(t, 3, 0)
	calls := 0
	err := retryTransientSlideExport(context.Background(), 1, "small", func() error {
		calls++
		if calls < 3 {
			return transientExportErr()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("瞬态消除后应成功: %v", err)
	}
	if calls != 3 {
		t.Fatalf("应尝试 3 次后成功, got %d", calls)
	}
}

// TestRetryTransientSlideExport_AlwaysTransientExhausts 恒瞬态：3 次尽返回最后
// 一次错误，错误信息追加「已重试 2 次」定位标记，attempt 恰执行 3 次。
func TestRetryTransientSlideExport_AlwaysTransientExhausts(t *testing.T) {
	withThumbExportRetryParams(t, 3, 0)
	calls := 0
	err := retryTransientSlideExport(context.Background(), 2, "hd", func() error {
		calls++
		return transientExportErr()
	})
	if err == nil {
		t.Fatal("恒瞬态应返回错误")
	}
	if calls != 3 {
		t.Fatalf("应尝试 3 次, got %d", calls)
	}
	if !strings.Contains(err.Error(), "已重试 2 次") {
		t.Fatalf("错误应含「已重试」定位标记: %v", err)
	}
	if !strings.Contains(err.Error(), "第 2 页导出 hd 档失败") {
		t.Fatalf("错误应保留既有定位格式: %v", err)
	}
}

// TestRetryTransientSlideExport_CtxCanceledZeroRetry ctx 已取消：零重试立即返回
// （取消/超时错误绝不重试——判定前置）。
func TestRetryTransientSlideExport_CtxCanceledZeroRetry(t *testing.T) {
	withThumbExportRetryParams(t, 3, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	err := retryTransientSlideExport(ctx, 1, "small", func() error {
		calls++
		return transientExportErr()
	})
	if err == nil {
		t.Fatal("应返回错误")
	}
	if calls != 1 {
		t.Fatalf("ctx 取消后零重试, got %d 次尝试", calls)
	}
	if strings.Contains(err.Error(), "已重试") {
		t.Fatalf("未发生重试不应带「已重试」标记: %v", err)
	}
}

// TestRetryTransientSlideExport_CancelDuringBackoff 退避等待中取消：立即从退避
// 返回不再重试。大退避值下 select 必走 ctx.Done 分支，chan 交接替代 sleep
// （首次 attempt 通知后取消，不依赖时序竞态）。
func TestRetryTransientSlideExport_CancelDuringBackoff(t *testing.T) {
	withThumbExportRetryParams(t, 3, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstAttempt := make(chan struct{}, 8)
	errCh := make(chan error, 1)
	go func() {
		errCh <- retryTransientSlideExport(ctx, 1, "small", func() error {
			firstAttempt <- struct{}{} // 退避基准 1h：第二次 attempt 不会发生，恰一次发送
			return transientExportErr()
		})
	}()
	<-firstAttempt // 首次失败已发生（即将/已进入退避 select）
	cancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("取消后应返回错误")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("退避中取消未及时返回（select 未消费 ctx.Done）")
	}
}

// TestRetryTransientSlideExport_NonTransientNoRetry 非瞬态（文件损坏类 OleError、
// 实质业务描述、普通 error）：零重试立即返回，单次失败错误格式与既有逐字一致。
func TestRetryTransientSlideExport_NonTransientNoRetry(t *testing.T) {
	withThumbExportRetryParams(t, 3, 0)
	cases := []struct {
		name string
		err  error
	}{
		{"文件损坏类OleError", ole.NewErrorWithDescription(0x8000FFFF, "文件已损坏")},
		{"DISP_E_EXCEPTION但实质描述", ole.NewErrorWithDescription(dispEException, "页序超出范围")},
		{"普通error", errors.New("COM 打开失败")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			err := retryTransientSlideExport(context.Background(), 1, "small", func() error {
				calls++
				return tc.err
			})
			if err == nil {
				t.Fatal("应返回错误")
			}
			if calls != 1 {
				t.Fatalf("非瞬态零重试, got %d 次尝试", calls)
			}
			if !strings.Contains(err.Error(), "第 1 页导出 small 档失败") {
				t.Fatalf("错误格式应与既有逐字一致: %v", err)
			}
			if strings.Contains(err.Error(), "已重试") {
				t.Fatalf("零重试不应带「已重试」标记: %v", err)
			}
		})
	}
}

// TestWaitForOwnedProcessesExit_ReturnsWhenPIDGone 修复 2：owned PID 两轮探针后
// 消失 → 等待立即结束（重建放行），探针恰消费 3 轮（前两轮存活、第三轮消失）。
func TestWaitForOwnedProcessesExit_ReturnsWhenPIDGone(t *testing.T) {
	calls := 0
	withThumbOwnedExitWaitParams(t, time.Millisecond, 50*time.Millisecond, func() map[int]bool {
		calls++
		if calls < 3 {
			return map[int]bool{4242: true}
		}
		return map[int]bool{}
	})
	waitForOwnedProcessesExit(map[int]bool{4242: true})
	if calls != 3 {
		t.Fatalf("应轮询至 PID 消失（第 3 轮）, got %d 轮", calls)
	}
}

// TestWaitForOwnedProcessesExit_CapsAndProceedsOnTimeout 恒存活：等待上限到达后
// 返回继续重建（CreateInstance 0x8001010A 重试链兜底），不永挂。
func TestWaitForOwnedProcessesExit_CapsAndProceedsOnTimeout(t *testing.T) {
	withThumbOwnedExitWaitParams(t, time.Millisecond, 20*time.Millisecond, func() map[int]bool {
		return map[int]bool{4242: true} // 恒存活
	})
	done := make(chan struct{})
	go func() {
		waitForOwnedProcessesExit(map[int]bool{4242: true})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("等待上限后应返回继续重建（超时未退由 CreateInstance 重试链兜底）")
	}
}

// TestWaitForOwnedProcessesExit_EmptyOwnedSkipsProbe 复用用户实例路径（owned 空）：
// 不等待不探针——禁杀语义下 Quit 本就未调用。
func TestWaitForOwnedProcessesExit_EmptyOwnedSkipsProbe(t *testing.T) {
	probed := false
	withThumbOwnedExitWaitParams(t, time.Millisecond, 50*time.Millisecond, func() map[int]bool {
		probed = true
		return nil
	})
	waitForOwnedProcessesExit(nil)
	if probed {
		t.Fatal("owned 集为空不应触发探针（复用用户实例路径禁等待）")
	}
}
