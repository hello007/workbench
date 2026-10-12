package main

import "workbench/model"

// ===== 环境变量查看（工具箱） =====
//
// 委托 EnvService：多来源只读枚举 + 脱敏标记 + 组内排序（敏感判定与来源编排收敛在
// service 层）。跨层契约见 docs/spec/cross-layer-contracts.md：新增 App 方法与 model
// 导出 struct 须同步 frontend/wailsjs/ 三处（App.js / App.d.ts / models.ts）。

// ListEnvVars 枚举环境变量并按来源分组（Windows 三级：系统/用户/进程；Linux 仅进程级）。
// 单来源失败不拖垮整体，错误消息随分组的 Error 字段下发；Value 恒为真实值——敏感项
// 默认打码与点击临时显值是前端展示层职责，复制单值取真实值。纯只读，无编辑能力。
func (a *App) ListEnvVars() ([]model.EnvVarGroup, error) {
	return a.envSvc.ListEnvVars()
}
