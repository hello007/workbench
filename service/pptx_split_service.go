package service

import (
	"context"
	"errors"
	"sync"

	"workbench/model"
)

// PptxSplitProvider PPT 按页拆分能力抽象（工具箱·PPT 拆分）。
//
// 生产实现为纯 Go OOXML 拆分核心（SplitPptxPerPageWithProgress，标准库
// archive/zip + encoding/xml，不依赖本机 Office）；单测注入 fake 验证服务编排
// 透传行为，拆分语义本身由核心的 fixture 全链路测试覆盖（程序化构造 pptx，
// docs/spec/test-stability.md 禁环境外部状态依赖）。
//
// onProgress 为逐页进度回调（每页写盘成功后调用，done 从 1 计），可为 nil；
// 服务编排层借它把核心进度转译为 EventSink 事件，fake 捕获回调即可驱动
// 事件断言，无需真实拆分。
type PptxSplitProvider interface {
	Split(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error)
}

// corePptxSplitter 生产实现，转发到纯 Go OOXML 拆分核心。
type corePptxSplitter struct{}

func (corePptxSplitter) Split(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error) {
	return SplitPptxPerPageWithProgress(ctx, srcPath, onProgress)
}

// PptxSplitService PPT 按页拆分服务（工具箱·PPT 拆分）：按页拆分编排入口。
//
// 拆分为长操作（docs/spec/logging-and-errors.md 取消链路契约）：Split 内部派生
// 可取消 ctx 并登记取消注册表（键 = normalizeRepoKey(srcPath)，与 GitService
// 取消注册表同键规范化语义），CancelPptxSplit 从另一调用线命中在途操作；取消
// 错误经 mapCancelError 统一映射 ErrOperationCancelled（E_GIT_CANCELLED）。
// 进度/终态事件经 sinkHolder 出口推送（pptx-split:* 前缀）。
type PptxSplitService struct {
	provider   PptxSplitProvider
	sinkHolder // 事件出口持有器（pptx-split:* 事件推送），构造注入 + serve 模式经 SetEventSink 切换

	opMu    sync.Mutex              // 保护 cancels 的并发读写（职责对齐 GitService.opMu）
	cancels map[string]*cancelEntry // 源文件规范化路径 -> 在途拆分注册条目（复用 git.go 的 cancelEntry，同包共享）
}

// NewPptxSplitService 创建 PPT 拆分服务（生产路径：纯 Go OOXML 拆分核心）。
// ctx 仅用于构造注入 Wails 桌面事件出口（ChatService 同款构造注入模式；
// 允许 nil/Background——wailsEventSink 对非 Wails 上下文集中防护静默跳过）。
func NewPptxSplitService(ctx context.Context) *PptxSplitService {
	s := &PptxSplitService{provider: corePptxSplitter{}}
	s.SetEventSink(NewWailsEventSink(ctx))
	return s
}

// registerCancel 登记在途拆分的可取消入口。键经 normalizeRepoKey 规范化
// （filepath.Abs 归一，相对/绝对路径形式命中同键），对齐 GitService.registerCancel
// 单点语义：同键重复注册为覆盖写（后注册者在途，取消命中最新操作），注销经
// 条目指针比对不误删新注册项。
func (s *PptxSplitService) registerCancel(srcPath string, cancel context.CancelFunc) *cancelEntry {
	key := normalizeRepoKey(srcPath)
	entry := &cancelEntry{cancel: cancel}
	s.opMu.Lock()
	if s.cancels == nil {
		s.cancels = make(map[string]*cancelEntry)
	}
	s.cancels[key] = entry
	s.opMu.Unlock()
	return entry
}

// unregisterCancel 拆分结束（成功/失败/取消/panic 经 defer）注销注册表，防
// CancelFunc 残留。删前校验条目指针同一：同键并发双拆分单槽覆盖后，先结束
// 操作的注销不得误删后注册操作的在途注册项。
func (s *PptxSplitService) unregisterCancel(srcPath string, entry *cancelEntry) {
	if entry == nil {
		return
	}
	key := normalizeRepoKey(srcPath)
	s.opMu.Lock()
	if s.cancels[key] == entry {
		delete(s.cancels, key)
	}
	s.opMu.Unlock()
}

// CancelPptxSplit 取消全部在途拆分（App 层取消入口无路径参数，语义为全量取消：
// 对齐 GitService.CancelAllGitOperations 的「删全部注册项 + 逐个 cancel」收口）。
// 返回是否取消了至少一个在途操作；无在途操作幂等返回 false（CancelFunc 幂等，
// 取消与操作方注销的竞态无危害：注销按条目指针比对，已删除键不误删新注册项）。
func (s *PptxSplitService) CancelPptxSplit() bool {
	s.opMu.Lock()
	entries := make([]*cancelEntry, 0, len(s.cancels))
	for key, entry := range s.cancels {
		entries = append(entries, entry)
		delete(s.cancels, key)
	}
	s.opMu.Unlock()

	for _, entry := range entries {
		entry.cancel()
	}
	return len(entries) > 0
}

// Split 拆分指定 PPT：每页生成一个独立 .pptx，输出到源文件同级同名文件夹
// （`<原文件名>-<两位页序号>.pptx`），返回输出目录与生成文件列表。
//
// 可取消：内部派生可取消 ctx（父 ctx 取消同样生效）并登记取消注册表，
// CancelPptxSplit 命中；取消错误经 mapCancelError 统一映射 ErrOperationCancelled
// （E_GIT_CANCELLED，前端 warning 中性提示——PPT 拆分复用既有取消错误码而非
// 新增，该码语义为「操作被用户取消」，前端 error.js WARNING_CODES 已覆盖）。
// defer LIFO：unregisterCancel 先于 cancel 执行，注册表与 ctx 资源双收口。
// 事件：逐页 pptx-split:progress + 终态 pptx-split:done / pptx-split:error
// （未注入出口时静默跳过）。错误时返回零值结果；取消前已写盘的完整单页文件
// 保留在输出文件夹内。
func (s *PptxSplitService) Split(ctx context.Context, srcPath string) (model.PptxSplitResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	entry := s.registerCancel(srcPath, cancel)
	defer s.unregisterCancel(srcPath, entry)

	result, err := s.provider.Split(ctx, srcPath, func(done, total int) {
		s.emitCurrent("pptx-split:progress", map[string]any{"srcPath": srcPath, "done": done, "total": total})
	})
	if err != nil {
		// 日志记原始错误（保留根因链），返回值与事件载荷用映射/提取后的用户可读形态
		Logger().Error("pptx split failed", "srcPath", srcPath, "err", err)
		err = mapCancelError(err)
		s.emitCurrent("pptx-split:error", map[string]any{"srcPath": srcPath, "message": appErrMessage(err)})
		return model.PptxSplitResult{}, err
	}
	Logger().Info("pptx split done", "srcPath", srcPath, "outputDir", result.OutputDir, "pages", result.PageCount)
	s.emitCurrent("pptx-split:done", map[string]any{"srcPath": srcPath, "result": result})
	return result, nil
}

// appErrMessage 取错误的用户可读消息：AppError 取 Message（不含 [CODE] 前缀），
// 其余取 Error() 原文。供事件载荷下发（前端展示口径与 handleError 分流一致）。
func appErrMessage(err error) string {
	var appErr *model.AppError
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return err.Error()
}
