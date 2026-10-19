package service

import (
	"context"
	"sync"

	"workbench/model"
)

// PptxMergeProvider 多源页合并导出能力抽象（素材购物栏「合并导出」）。
//
// 生产实现为纯 Go OOXML 基底注入式合并核心（MergePptxPages，标准库
// archive/zip + encoding/xml，不依赖本机 Office）；单测注入 fake 验证服务编排
// 透传行为，合并语义本身由核心的 fixture 全链路测试覆盖（程序化构造 pptx，
// docs/spec/test-stability.md 禁环境外部状态依赖）。
//
// onProgress 为逐源进度回调（每个源文件处理完成后调用，done 为已就绪页数），
// 可为 nil；服务编排层借它把核心进度转译为 EventSink 事件，fake 捕获回调即可
// 驱动事件断言。
type PptxMergeProvider interface {
	Merge(ctx context.Context, items []model.PptxMergeItem, outPath string, onProgress func(done, total int)) (model.PptxMergeResult, error)
}

// corePptxMerger 生产实现，转发到纯 Go OOXML 合并核心。
type corePptxMerger struct{}

func (corePptxMerger) Merge(ctx context.Context, items []model.PptxMergeItem, outPath string, onProgress func(done, total int)) (model.PptxMergeResult, error) {
	return MergePptxPages(ctx, items, outPath, onProgress)
}

// PptxMergeService 多源页合并导出服务（素材购物栏「合并导出」）：基底注入式
// 合并编排入口。
//
// 合并为长操作（docs/spec/logging-and-errors.md 取消链路契约）：Merge 内部派生
// 可取消 ctx 并登记取消注册表（键 = normalizeRepoKey(outPath)，取消命中在途
// 导出），CancelPptxMerge 从另一调用线取消全部在途导出；取消错误经
// mapCancelError 统一映射 ErrOperationCancelled（E_GIT_CANCELLED，前端 warning
// 中性提示）。进度/终态事件经 sinkHolder 出口推送（assets-cart:* 前缀，素材
// 购物栏域独立命名）。
type PptxMergeService struct {
	provider   PptxMergeProvider
	sinkHolder // 事件出口持有器（assets-cart:* 事件推送），构造注入 + serve 模式经 SetEventSink 切换

	opMu    sync.Mutex              // 保护 cancels 的并发读写（职责对齐 PptxSplitService.opMu）
	cancels map[string]*cancelEntry // 输出路径规范化 -> 在途导出注册条目（复用 git.go 的 cancelEntry，同包共享）
}

// NewPptxMergeService 创建合并导出服务（生产路径：纯 Go OOXML 合并核心）。
// ctx 仅用于构造注入 Wails 桌面事件出口（PptxSplitService 同款构造注入模式；
// 允许 nil/Background——wailsEventSink 对非 Wails 上下文集中防护静默跳过）。
func NewPptxMergeService(ctx context.Context) *PptxMergeService {
	s := &PptxMergeService{provider: corePptxMerger{}}
	s.SetEventSink(NewWailsEventSink(ctx))
	return s
}

// registerCancel 登记在途导出的可取消入口（PptxSplitService.registerCancel 同款
// 单点语义：键经 normalizeRepoKey 规范化、同键重复注册覆盖写、注销经条目指针
// 比对不误删新注册项）。
func (s *PptxMergeService) registerCancel(outPath string, cancel context.CancelFunc) *cancelEntry {
	key := normalizeRepoKey(outPath)
	entry := &cancelEntry{cancel: cancel}
	s.opMu.Lock()
	if s.cancels == nil {
		s.cancels = make(map[string]*cancelEntry)
	}
	s.cancels[key] = entry
	s.opMu.Unlock()
	return entry
}

// unregisterCancel 导出结束（成功/失败/取消/panic 经 defer）注销注册表，防
// CancelFunc 残留（PptxSplitService.unregisterCancel 同款指针比对语义）。
func (s *PptxMergeService) unregisterCancel(outPath string, entry *cancelEntry) {
	if entry == nil {
		return
	}
	key := normalizeRepoKey(outPath)
	s.opMu.Lock()
	if s.cancels[key] == entry {
		delete(s.cancels, key)
	}
	s.opMu.Unlock()
}

// CancelPptxMerge 取消全部在途合并导出（App 层取消入口无路径参数，语义为全量
// 取消：对齐 CancelPptxSplit 的「删全部注册项 + 逐个 cancel」收口）。返回是否
// 取消了至少一个在途操作；无在途操作幂等返回 false。
func (s *PptxMergeService) CancelPptxMerge() bool {
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

// Merge 把有序页清单合并导出为单一 .pptx（outPath 由调用方经 SaveFileDialog
// 选定）。
//
// 可取消：内部派生可取消 ctx（父 ctx 取消同样生效）并登记取消注册表，
// CancelPptxMerge 命中；取消错误经 mapCancelError 统一映射 ErrOperationCancelled
// （E_GIT_CANCELLED）。defer LIFO：unregisterCancel 先于 cancel 执行，注册表与
// ctx 资源双收口。
// 事件：逐源 assets-cart:export-progress + 终态 assets-cart:export-done /
// assets-cart:export-error（未注入出口时静默跳过）。错误时返回零值结果。
func (s *PptxMergeService) Merge(ctx context.Context, items []model.PptxMergeItem, outPath string) (model.PptxMergeResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	entry := s.registerCancel(outPath, cancel)
	defer s.unregisterCancel(outPath, entry)

	result, err := s.provider.Merge(ctx, items, outPath, func(done, total int) {
		s.emitCurrent("assets-cart:export-progress", map[string]any{"outPath": outPath, "done": done, "total": total})
	})
	if err != nil {
		// 日志记原始错误（保留根因链），返回值与事件载荷用映射/提取后的用户可读形态
		Logger().Error("pptx merge export failed", "outPath", outPath, "items", len(items), "err", err)
		err = mapCancelError(err)
		s.emitCurrent("assets-cart:export-error", map[string]any{"outPath": outPath, "message": appErrMessage(err)})
		return model.PptxMergeResult{}, err
	}
	Logger().Info("pptx merge export done", "outPath", outPath, "pages", result.PageCount, "skipped", len(result.SkippedItems))
	s.emitCurrent("assets-cart:export-done", map[string]any{"outPath": outPath, "result": result})
	return result, nil
}
