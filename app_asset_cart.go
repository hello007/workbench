package main

import (
	"context"

	"workbench/model"
)

// ===== 素材购物栏（素材管理「购物栏」：跨库页级收集 + 按序合并导出） =====
//
// 委托 AssetCartService：购物栏 CRUD/排序持久化（data/asset_cart.json）+ 合并导出
// 编排（复用 PptxMergeService，进度/终态经 assets-cart:export-progress/done/error
// 事件推送，取消经其注册表收口）。跨层契约见 docs/spec/cross-layer-contracts.md：
// 新增 App 方法与 model 导出 struct 须同步 frontend/wailsjs/ 三处（App.js /
// App.d.ts / models.ts）。

// ListAssetCartItems 购物栏全部条目（按购物栏顺序）。逐条按源路径快照做存在性
// 探测，源文件不存在的条目 stale=true（前端置灰可移除，导出跳过并在结果中报告）。
func (a *App) ListAssetCartItems() ([]model.AssetCartItem, error) {
	return a.assetCartSvc.List()
}

// AddAssetCartItems 批量加入购物栏（幂等去重）：键 (libraryId, fileId, slideNum)
// 已在栏的条目跳过计入 duplicates，新增条目按入参顺序追加。重复加号操作幂等
// （added=0 且 duplicates>0 时前端提示「已在购物栏」）。
func (a *App) AddAssetCartItems(items []model.AssetCartItem) (model.AssetCartAddResult, error) {
	return a.assetCartSvc.Add(items)
}

// RemoveAssetCartItems 按条目键批量移除（键形态 `<libraryId>|<fileId>|<slideNum>`，
// model.AssetCartItem.Key 同构）。不存在的键容忍跳过，全部未命中报错。
func (a *App) RemoveAssetCartItems(keys []string) error {
	return a.assetCartSvc.Remove(keys)
}

// ClearAssetCart 清空购物栏（空购物栏幂等）。
func (a *App) ClearAssetCart() error {
	return a.assetCartSvc.Clear()
}

// ReorderAssetCart 按键列表重排购物栏顺序（拖拽排序回写）；orderedKeys 须为当前
// 条目键的完整排列，不符拒绝（防静默丢条目）。
func (a *App) ReorderAssetCart(orderedKeys []string) error {
	return a.assetCartSvc.Reorder(orderedKeys)
}

// MergeAssetCartToPptx 把购物栏内全部条目按购物栏顺序合并导出为单一 .pptx
// （outPath 由前端经 SaveFileDialog 选定）。合并导出为可取消长操作：进度经
// assets-cart:export-progress 事件推送，终态经 assets-cart:export-done /
// assets-cart:export-error；重复导出报 E_CART_EXPORT_IN_PROGRESS（warning 引导级），
// 取消属预期拒绝（E_GIT_CANCELLED，前端按 warning 中性提示）。跳过项（源缺失/
// 页越界/重复项）在结果 SkippedItems 中报告。
func (a *App) MergeAssetCartToPptx(outPath string) (model.PptxMergeResult, error) {
	return a.assetCartSvc.MergeCartToPptx(context.Background(), outPath)
}

// CancelAssetCartExport 取消全部在途购物栏合并导出。返回是否取消了至少一个在途
// 操作；无在途操作时幂等返回 false。
func (a *App) CancelAssetCartExport() bool {
	return a.assetCartSvc.CancelAssetCartExport()
}
