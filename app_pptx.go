package main

import (
	"context"

	"workbench/model"
)

// ===== PPT 拆分（工具箱） =====
//
// 委托 PptxSplitService：.pptx 按页拆分编排 + 取消注册表（进度事件、取消错误码
// 映射在 service 层收敛）。跨层契约见 docs/spec/cross-layer-contracts.md：新增
// App 方法与 model 导出 struct 须同步 frontend/wailsjs/ 三处（App.js / App.d.ts /
// models.ts）。

// SplitPptxPerPage 按页拆分指定 PPT：每页生成一个独立 .pptx，输出到源文件同级
// 以源文件名命名的文件夹（`<原文件名>-<两位页序号>.pptx`），返回输出目录与生成
// 文件列表。拆分进度经 pptx-split:progress 事件推送（已完成页数/总页数），终态
// 经 pptx-split:done / pptx-split:error。拆分过程可经 CancelPptxSplit 取消，
// 取消属预期拒绝（前端按 warning 中性提示，非 error）。
func (a *App) SplitPptxPerPage(srcPath string) (model.PptxSplitResult, error) {
	return a.pptxSplitSvc.Split(context.Background(), srcPath)
}

// CancelPptxSplit 取消全部在途拆分（无路径参数：拆分为用户逐文件发起的低频
// 长操作，一次取消全部在途项）。返回是否取消了至少一个在途操作；无在途操作
// 时幂等返回 false。
func (a *App) CancelPptxSplit() bool {
	return a.pptxSplitSvc.CancelPptxSplit()
}
