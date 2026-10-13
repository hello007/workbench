package main

import "workbench/model"

// ===== hosts 文件查看（工具箱） =====
//
// 委托 HostsService：本机 hosts 只读读取 + 行解析编排。跨层契约见
// docs/spec/cross-layer-contracts.md：新增 App 方法与 model 导出 struct 须同步
// frontend/wailsjs/ 三处（App.js / App.d.ts / models.ts）。

// GetHosts 读取并解析本机 hosts 文件（Windows C:\Windows\System32\drivers\etc\hosts /
// Linux /etc/hosts）。条目含注释/空行/未识别行，「显示注释」开关过滤由前端处理；
// 读取失败不作为整体异常返回，错误消息随 HostsView.Error 字段下发。纯只读，无编辑能力。
func (a *App) GetHosts() (*model.HostsView, error) {
	return a.hostsSvc.GetHosts()
}
