// model/env.go
package model

// EnvVarInfo 环境变量查看器单行（工具箱·环境变量，纯只读）。
//
// Value 恒为真实值——后端只按敏感命名模式打 Sensitive 标记，不做打码：默认打码
// 显示、点击临时显值是前端展示层职责，复制单值直接取 Value（真实值）。
type EnvVarInfo struct {
	Name      string `json:"name"`      // 变量名
	Value     string `json:"value"`     // 变量值（真实值，脱敏打码由前端展示层处理）
	Sensitive bool   `json:"sensitive"` // 命中敏感命名模式（TOKEN/SECRET/PASSWORD/PASSWD/KEY/CREDENTIAL 大小写不敏感子串包含）：前端默认打码 + 点击临时显值
}

// EnvVarGroup 单个来源的环境变量分组。
//
// Source 用普通 string 而非 `type X string` 具名类型：wails generate 不为具名
// string 类型生成 models.ts 类型别名，App.d.ts 引用后 npm run build 会报
// MISSING_EXPORT（vitest 走 esbuild 不报类型错，极易漏，见
// docs/spec/cross-layer-contracts.md）。取值 system / user / process（service 层常量）。
type EnvVarGroup struct {
	Source string       `json:"source"` // 来源标识：system（系统级）/ user（用户级）/ process（进程级）
	Name   string       `json:"name"`   // 展示名：系统环境变量 / 用户环境变量 / 进程环境变量
	Vars   []EnvVarInfo `json:"vars"`   // 该来源的变量列表（组内按名称大小写不敏感排序）；枚举失败时为空
	Error  string       `json:"error"`  // 该来源枚举失败的非致命错误消息（用户可读，不含错误码前缀）；成功为空串。单来源失败不拖垮整体，仅全部来源失败才整体报错
}
