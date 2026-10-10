package main

import "workbench/model"

// ===== 进程管理（工具箱） =====
//
// 委托 ProcessService：进程枚举检索 + 停止/强杀编排（自我保护与错误码在 service 层收敛）。
// 跨层契约见 docs/spec/cross-layer-contracts.md：新增 App 方法与 model 导出 struct 须同步
// frontend/wailsjs/ 三处（App.js / App.d.ts / models.ts）。

// ListProcesses 检索本机进程。
// keyword 为空列出全部处于监听状态的进程；mode 取值 auto / port / name（空串按 auto）。
func (a *App) ListProcesses(keyword, mode string) ([]model.ProcessInfo, error) {
	return a.processSvc.ListProcesses(keyword, mode)
}

// StopProcess 停止进程：先发优雅停止请求，5 秒内未退出自动转强杀（全自动两级，中途不询问）。
// 优雅请求能否生效取决于目标进程是否响应、无法事前预判，故不做进程类型预判，统一按
// 轮询实测存活判定。返回的 Method 如实反映最终生效手段（graceful / forced），UI 据此提示。
func (a *App) StopProcess(pid int, killTree bool) (*model.ProcessStopResult, error) {
	return a.processSvc.StopProcess(pid, killTree)
}

// KillProcess 强制终止进程，跳过优雅阶段。killTree 为 true 时连带子进程（显式二级选项）。
func (a *App) KillProcess(pid int, killTree bool) error {
	return a.processSvc.KillProcess(pid, killTree)
}
