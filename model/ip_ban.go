package model

// IPBanStatus 单条来源 IP 自动封禁状态（连接管控五期：认证失败自动封禁），
// service.IPBanService.List 的返回条目形态。仅内存态运行时面（IP 计数与封禁
// 均不落盘，重启清零），不参与任何持久化序列化。
//
// 字段全为基本类型（string/int64/bool）：暂未穿越 Wails 绑定签名（后续接入
// App 绑定时若作为返回值面暴露，因无具名类型也无需手动补 models.ts 别名，
// 对齐 WebTokenRecord 既有结论）。
type IPBanStatus struct {
	// IP 被封禁的来源 IP（RemoteAddr 剥端口归一后的形态）。
	IP string `json:"ip"`
	// BannedAt 最近一次自动封禁时刻（unix 秒；重复触发刷新为本次时间）。
	BannedAt int64 `json:"bannedAt"`
	// ExpiresAt 封禁到期时间（unix 秒）。到期判定为懒式（拒绝点比较当前
	// 时间），过期记录不自动删除（列表保留「已过期」态可见性，禁后台扫描），
	// 与 WebBanRecord.ExpiresAt 同款约定。
	ExpiresAt int64 `json:"expiresAt"`
	// FailCount 触发本次封禁时滑动窗口内的认证失败计数（当前恒为
	// IPBanThreshold；封禁期间失败不累计，该值冻结至下次触发刷新）。
	FailCount int `json:"failCount"`
	// Expired 构造时按当前时间算好的过期标记（true = 已到期，仅剩列表可见
	// 性；拒连判定以服务层懒过期为准）。
	Expired bool `json:"expired"`
}
