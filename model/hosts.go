// model/hosts.go
package model

// HostsEntry hosts 文件单行条目（工具箱·hosts 文件，纯只读）。
//
// Kind 用普通 string 而非 `type X string` 具名类型：wails generate 不为具名 string
// 类型生成 models.ts 类型别名，App.d.ts 引用后 npm run build 会报 MISSING_EXPORT
// （vitest 走 esbuild 不报类型错，极易漏，见 docs/spec/cross-layer-contracts.md）。
// 取值 entry / comment / blank / unknown（util 层常量，service 层映射时透传）。
type HostsEntry struct {
	Kind      string   `json:"kind"`      // 行类别：entry（主机映射）/ comment（纯注释）/ blank（空行）/ unknown（未识别）
	IP        string   `json:"ip"`        // entry：地址原样 token（含 IPv6）；其余类别空串
	Hostnames []string `json:"hostnames"` // entry：主机名列表（IP 单独成行时为空数组）；其余类别为空
	Comment   string   `json:"comment"`   // entry：行内注释（首个 # 之后文本）；其余类别空串
	Raw       string   `json:"raw"`       // 原始行文本（剥离行尾换行），复制单行取此值
}

// HostsView 本机 hosts 文件的解析视图（单组，只读）。
//
// 读取失败不作为整体异常返回：用户可读错误消息写入 Error 字段（对齐 EnvVarGroup.Error
// 的单来源失败形态），前端组级 warning 行就地提示；「显示注释」开关按 Kind 过滤属于
// 前端展示层职责，后端恒返回全部行条目。
type HostsView struct {
	Path    string       `json:"path"`    // 实际读取的 hosts 文件路径（Windows 与 Linux 系统路径不同）
	Entries []HostsEntry `json:"entries"` // 解析后的全部行条目（含注释/空行/未识别行）；读取失败时为空数组
	Error   string       `json:"error"`   // 读取失败的非致命错误消息（用户可读，不含错误码前缀）；成功为空串
}
