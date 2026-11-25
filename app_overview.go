package main

import "workbench/model"

// GetOverviewSnapshot 个人看板「概览」面板快照（只读聚合，单次 IPC）。
//
// rangeKey 为时间范围回显（today/week），T1 固定同时返回今日与本周块；
// force=true 跳过 5 分钟 TTL 强制重算（面板手动刷新）。
//
// 三个数据域各自带 Error 字段实现分区降级：单域失败不阻断其余域，也不会
// 把查询失败伪装成 0。error 仅在整体不可恢复时返回；当前三域均自带分区降级，
// 故恒返回 nil error，保留 error 位供后续不可恢复错误（如装配缺失）上报。
func (a *App) GetOverviewSnapshot(rangeKey string, force bool) (*model.OverviewSnapshot, error) {
	return a.overviewSvc.GetSnapshot(rangeKey, force), nil
}
