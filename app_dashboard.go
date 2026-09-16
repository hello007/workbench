package main

import "workbench/model"

// ===== 全局状态看板 =====
//
// 委托 DashboardService：pin 仓库列表增删查 + 批量状态计算。
// 跨层契约见 docs/spec/cross-layer-contracts.md：model 新增导出字段须同步 frontend/wailsjs/ 三处。

// GetDashboardPinned 获取 pin 仓库路径列表（只读，加载失败降级空列表不阻塞）。
func (a *App) GetDashboardPinned() []string {
	return a.dashboardSvc.LoadPinned()
}

// AddDashboardPin 将仓库路径加入看板 pin 列表（去重，路径内部规范化）。已存在幂等返回。
func (a *App) AddDashboardPin(path string) error {
	return a.dashboardSvc.AddPin(path)
}

// RemoveDashboardPin 从看板 pin 列表移除仓库路径（路径内部规范化）。不存在幂等返回。
func (a *App) RemoveDashboardPin(path string) error {
	return a.dashboardSvc.RemovePin(path)
}

// IsDashboardPinned 判断路径是否已在看板 pin 列表（路径内部规范化）。
// 供右键菜单「加入状态看板/取消关注」切换按钮态。
func (a *App) IsDashboardPinned(path string) bool {
	return a.dashboardSvc.IsPinned(path)
}

// GetDashboardStatuses 批量计算全部 pin 仓库的状态快照（并发，只读不走仓级锁）。
// 返回顺序与 pin 列表一致；失效路径标 Missing 不阻塞。
func (a *App) GetDashboardStatuses() []model.RepoStatus {
	return a.dashboardSvc.GetStatuses()
}

// RefreshDashboardStatuses 刷新全部 pin 仓库状态（当前与 GetDashboardStatuses 语义一致，
// 独立方法预留：后续接入 fetch 增强或事件驱动增量时在此扩展，前端调用方不变）。
func (a *App) RefreshDashboardStatuses() []model.RepoStatus {
	return a.dashboardSvc.GetStatuses()
}
