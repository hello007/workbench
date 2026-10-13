package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"workbench/model"
)

// ---------- 取消注册表用例（docs/spec/logging-and-errors.md 取消链路契约） ----------

// TestPptxSplitService_CancelNoInFlight 无在途拆分时取消幂等返回 false，不报错。
func TestPptxSplitService_CancelNoInFlight(t *testing.T) {
	svc := &PptxSplitService{}
	if svc.CancelPptxSplit() {
		t.Error("无在途拆分应返回 false")
	}
}

// blockingPptxSplitProvider 阻塞型 fake：Split 启动即通告，随后阻塞至 ctx 取消，
// 以 %w 包装 context.Canceled 返回（模拟核心取消路径的错误形态）。
type blockingPptxSplitProvider struct {
	started chan struct{}
}

func (f *blockingPptxSplitProvider) Split(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error) {
	close(f.started)
	<-ctx.Done()
	return model.PptxSplitResult{}, fmt.Errorf("拆分已取消: %w", ctx.Err())
}

// TestPptxSplitService_CancelInFlight 在途拆分取消全链路：CancelPptxSplit 触发
// 已派生 ctx 的取消（provider 侧感知），Split 返回值统一映射为 E_GIT_CANCELLED
// AppError（前端 warning 中性提示），事件出口收到用户可读消息的 error 事件；
// Split 返回后注册表无残留（再次取消幂等 false）。
func TestPptxSplitService_CancelInFlight(t *testing.T) {
	fake := &blockingPptxSplitProvider{started: make(chan struct{})}
	sink := &fakeEventSink{}
	svc := &PptxSplitService{provider: fake}
	svc.SetEventSink(sink)
	src := filepath.Join(t.TempDir(), "在途.pptx")

	done := make(chan error, 1)
	go func() {
		_, err := svc.Split(context.Background(), src)
		done <- err
	}()
	<-fake.started

	if !svc.CancelPptxSplit() {
		t.Fatal("在途拆分应可取消（返回 true）")
	}
	select {
	case err := <-done:
		var appErr *model.AppError
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeGitCancelled {
			t.Fatalf("取消错误应映射为 E_GIT_CANCELLED AppError，实际：%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("取消后 Split 未返回")
	}

	// Split 返回即代表 defer unregister 已执行：注册表无残留
	if svc.CancelPptxSplit() {
		t.Error("拆分结束后注册表应清空（再次取消应返回 false）")
	}
	// 事件：仅 1 条 error，载荷为用户可读消息（不含 [CODE] 前缀）
	if len(sink.calls) != 1 || sink.calls[0].name != "pptx-split:error" {
		t.Fatalf("应收到 1 条 pptx-split:error，实际：%#v", sink.calls)
	}
	payload, ok := sink.calls[0].data[0].(map[string]any)
	if !ok || payload["message"] != "操作已取消" || payload["srcPath"] != src {
		t.Errorf("error 事件载荷不符：%#v", sink.calls[0].data)
	}
}

// TestPptxSplitService_RegistryKeyNormalized 注册表键规范化（对齐
// TestCancelGitOperation_KeyNormalized）：注册与注销一侧带尾部点段形式、
// 一侧为原始路径，normalizeRepoKey 归一命中同键；同键重复注册为单槽覆盖，
// 先注册条目注销经指针比对不误删后注册项、不触发对方 CancelFunc。
func TestPptxSplitService_RegistryKeyNormalized(t *testing.T) {
	svc := &PptxSplitService{}
	src := filepath.Join(t.TempDir(), "键规范化.pptx")

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	// 尾部点段形式（filepath.Abs Clean 后与原始路径同键）
	entry1 := svc.registerCancel(src+string(filepath.Separator)+".", cancel1)

	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	entry2 := svc.registerCancel(src, cancel2) // 覆盖 entry1 的单槽

	// 先注册操作先结束：注销 entry1 不得删除 entry2 的在途注册项
	svc.unregisterCancel(src, entry1)
	svc.unregisterCancel(src, nil) // nil 条目注销不应 panic

	if !svc.CancelPptxSplit() {
		t.Fatal("entry1 注销后 entry2 仍应可取消")
	}
	select {
	case <-ctx2.Done():
	case <-time.After(time.Second):
		t.Fatal("entry2 的 CancelFunc 未被触发")
	}
	select {
	case <-ctx1.Done():
		t.Fatal("entry1 不应被误取消（其条目已不在注册表）")
	default:
	}
	// 取消即清注册表：再次取消幂等 false；旧条目重复注销不 panic（指针不匹配）
	if svc.CancelPptxSplit() {
		t.Error("取消后注册表应已清空")
	}
	svc.unregisterCancel(src, entry2)
}

// ---------- 进度/终态事件用例 ----------

// TestPptxSplitService_ProgressAndDoneEvents 事件链：逐页 progress（done/total
// 随页推进）+ 终态 done（载荷含源路径与完整结果）；未注入出口时静默跳过不 panic。
func TestPptxSplitService_ProgressAndDoneEvents(t *testing.T) {
	wantResult := model.PptxSplitResult{
		OutputDir: "/tmp/演示",
		Files:     []string{"/tmp/演示/演示-01.pptx", "/tmp/演示/演示-02.pptx", "/tmp/演示/演示-03.pptx"},
		PageCount: 3,
	}
	fake := &fakePptxSplitProvider{
		progress: []fakeProgressCall{{done: 1, total: 3}, {done: 2, total: 3}, {done: 3, total: 3}},
		result:   wantResult,
	}
	sink := &fakeEventSink{}
	svc := &PptxSplitService{provider: fake}
	svc.SetEventSink(sink)

	src := "D:/docs/演示.pptx"
	got, err := svc.Split(context.Background(), src)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if got.PageCount != 3 {
		t.Fatalf("结果透传不符：%+v", got)
	}

	// 事件序列：3 条 progress + 1 条 done
	if len(sink.calls) != 4 {
		t.Fatalf("应收到 4 条事件（3 progress + 1 done），实际 %d 条：%#v", len(sink.calls), sink.calls)
	}
	for i, want := range fake.progress {
		call := sink.calls[i]
		if call.name != "pptx-split:progress" {
			t.Fatalf("第 %d 条事件应为 pptx-split:progress，实际 %q", i, call.name)
		}
		payload, ok := call.data[0].(map[string]any)
		if !ok || payload["srcPath"] != src || payload["done"] != want.done || payload["total"] != want.total {
			t.Errorf("progress 事件载荷不符（期望 done=%d total=%d）：%#v", want.done, want.total, call.data)
		}
	}
	doneCall := sink.calls[3]
	if doneCall.name != "pptx-split:done" {
		t.Fatalf("末条事件应为 pptx-split:done，实际 %q", doneCall.name)
	}
	payload, ok := doneCall.data[0].(map[string]any)
	if !ok || payload["srcPath"] != src {
		t.Errorf("done 事件载荷 srcPath 不符：%#v", doneCall.data)
	}
	if result, ok := payload["result"].(model.PptxSplitResult); !ok || result.PageCount != 3 || len(result.Files) != 3 {
		t.Errorf("done 事件载荷 result 不符：%#v", doneCall.data)
	}

	// 未注入出口：progress 回照常发生，emitEvent nil 防护静默跳过不 panic
	bare := &PptxSplitService{provider: fake}
	if _, err := bare.Split(context.Background(), src); err != nil {
		t.Fatalf("无出口 Split 不应报错: %v", err)
	}
}

// TestPptxSplitService_ErrorEventMapping 错误事件与错误码映射：
//   - 普通 provider 错误：原样上抛（不误映射为取消），事件载荷带错误原文；
//   - ctx 取消形态错误：%w 包装 context.Canceled，映射为 E_GIT_CANCELLED
//     AppError，事件载荷为用户可读消息（不含 [CODE] 前缀）。
func TestPptxSplitService_ErrorEventMapping(t *testing.T) {
	// 普通错误
	sink := &fakeEventSink{}
	svc := &PptxSplitService{provider: &fakePptxSplitProvider{err: errors.New("磁盘写入失败")}}
	svc.SetEventSink(sink)
	_, err := svc.Split(context.Background(), "D:/docs/演示.pptx")
	if err == nil || err.Error() != "磁盘写入失败" {
		t.Fatalf("普通错误应原样上抛，实际：%v", err)
	}
	if len(sink.calls) != 1 || sink.calls[0].name != "pptx-split:error" {
		t.Fatalf("应收到 1 条 pptx-split:error，实际：%#v", sink.calls)
	}
	if payload, ok := sink.calls[0].data[0].(map[string]any); !ok || payload["message"] != "磁盘写入失败" {
		t.Errorf("error 事件载荷不符：%#v", sink.calls[0].data)
	}

	// 取消错误（核心取消路径的 %w 包装形态）
	sink2 := &fakeEventSink{}
	svc2 := &PptxSplitService{provider: &fakePptxSplitProvider{err: fmt.Errorf("拆分已取消: %w", context.Canceled)}}
	svc2.SetEventSink(sink2)
	_, err = svc2.Split(context.Background(), "D:/docs/演示.pptx")
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeGitCancelled {
		t.Fatalf("取消错误应映射为 E_GIT_CANCELLED AppError，实际：%v", err)
	}
	if len(sink2.calls) != 1 || sink2.calls[0].name != "pptx-split:error" {
		t.Fatalf("应收到 1 条 pptx-split:error，实际：%#v", sink2.calls)
	}
	if payload, ok := sink2.calls[0].data[0].(map[string]any); !ok || payload["message"] != "操作已取消" {
		t.Errorf("取消 error 事件载荷应为用户可读消息，实际：%#v", sink2.calls[0].data)
	}
}
