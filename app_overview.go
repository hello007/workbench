package main

import "workbench/model"

// GetOverviewSnapshot 个人看板「概览」面板快照（只读聚合，单次 IPC）。
//
// rangeKey 为所选时间档位（today/week/7d/30d/90d，空值/非法值回落 week），
// 只影响 AI 域（M4/M5）的 Selected 块；仓库与待办域为快照/全量语义不受其影响。
// force=true 跳过 5 分钟 TTL 强制重算（面板手动刷新）。
//
// 三个数据域各自带 Error 字段实现分区降级：单域失败不阻断其余域，也不会
// 把查询失败伪装成 0。error 仅在整体不可恢复时返回；当前三域均自带分区降级，
// 故恒返回 nil error，保留 error 位供后续不可恢复错误（如装配缺失）上报。
func (a *App) GetOverviewSnapshot(rangeKey string, force bool) (*model.OverviewSnapshot, error) {
	return a.overviewSvc.GetSnapshot(rangeKey, force), nil
}

// RecordOverviewOpen 记录一次「概览面板主动打开」，供 T4 Go/No-Go 门槛 G1 取证。
//
// source 仅取 click（点击图标）/ shortcut（快捷键）/ alt（Alt+数字）三个显式入口，
// 非法值由 service 层白名单丢弃；会话恢复、切档、手刷、TTL 命中、下钻切走均不传源。
// 埋点是旁路观测面：写失败只在 service 内 Warn 降级，此处恒返回 nil error，
// 任何情况下不阻断概览面板加载。
func (a *App) RecordOverviewOpen(source string) error {
	if a.overviewOpenLogSvc != nil {
		a.overviewOpenLogSvc.Record(source)
	}
	return nil
}
