package model

// IP 封禁来源取值（连接管控七期手动封禁新增）：manual 为管理员经桌面/浏览器
// RPC 主动下发（持久化 data/ip_bans.json）；auto 为认证失败达阈值自动触发
// （五期形态，纯内存态不落盘、重启清零）。
const (
	IPBanSourceManual = "manual"
	IPBanSourceAuto   = "auto"
)

// IPBanStatus 单条来源 IP 封禁状态（连接管控五期：认证失败自动封禁；七期扩展
// 为手动 + 自动共存的统一条目形态），service.IPBanService.List 的返回条目形态。
// 仅内存态运行时面（封禁状态本身不落盘；手动封禁经 IPBanRecord 独立持久化），
// 不参与任何持久化序列化。
//
// 字段全为基本类型（string/int64/bool）：暂未穿越 Wails 绑定签名（后续接入
// App 绑定时若作为返回值面暴露，因无具名类型也无需手动补 models.ts 别名，
// 对齐 WebTokenRecord 既有结论）。
type IPBanStatus struct {
	// IP 被封禁的来源 IP（RemoteAddr 剥端口归一后的形态）。
	IP string `json:"ip"`
	// BannedAt 最近一次封禁时刻（unix 秒；自动重复触发或手动重复封禁均刷新为
	// 本次时间）。
	BannedAt int64 `json:"bannedAt"`
	// ExpiresAt 封禁到期时间（unix 秒；0 = 永久封禁，须手动解封）。到期判定为
	// 懒式（拒绝点比较当前时间），过期记录不自动删除（列表保留「已过期」态
	// 可见性，禁后台扫描），与 WebBanRecord.ExpiresAt 同款约定。
	ExpiresAt int64 `json:"expiresAt"`
	// FailCount 触发本次封禁时滑动窗口内的认证失败计数（自动封禁为触发时窗口
	// 计数，封禁期间失败不累计，该值冻结至下次触发刷新；手动封禁无自动触发
	// 语境，恒为 0）。
	FailCount int `json:"failCount"`
	// Strike 累计触发自动封禁的次数（连接管控六期递增封禁：第 N 次触发的封禁
	// 时长按指数翻倍递增至封顶；解封/轮换联动清零/重启清零后重新从 1 起算）。
	// 手动封禁不改写该值（管理员决策非攻击信号，不参与递增）；条目曾有自动史
	// 时手动封禁后仍保留自动史档位。
	Strike int `json:"strike"`
	// Expired 构造时按当前时间算好的过期标记（true = 已到期，仅剩列表可见
	// 性；拒连判定以服务层懒过期为准）。永久封禁恒为 false。
	Expired bool `json:"expired"`
	// Source 封禁来源：IPBanSourceManual（管理员手动下发，七期）或
	// IPBanSourceAuto（认证失败自动触发）。恒透传非 omitempty（对齐既有字段
	// 风格，前端徽标按来源分流展示）。
	Source string `json:"source"`
	// Kind 条目形态（连接管控八期）：IPBanKindCidr（网段封禁，IP 字段为 CIDR
	// 串）或 IPBanKindIP（单 IP 精确封禁）。恒透传非 omitempty（前端按形态分流
	// 徽标渲染；七期旧客户端新增字段缺省零值不影响既有消费）。
	Kind string `json:"kind"`
	// CoveredBy 被更宏观封禁覆盖的标注（连接管控八期，后端单点计算透传）：
	// 单 IP 条目命中某网段条目时填该网段串；网段条目被更宽网段（掩码更短且
	// 包含）覆盖时填覆盖者网段串；多命中取掩码最短（最宏观）者。无覆盖留空
	// （omitempty 序列化消失），前端只渲染徽标不复刻判定。
	CoveredBy string `json:"coveredBy,omitempty"`
}

// IP 封禁条目形态取值（连接管控八期）：Kind 字段的两个合法取值。
const (
	// IPBanKindIP 单 IP 精确封禁（entries map 条目，IP 字段为归一 IP）。
	IPBanKindIP = "ip"
	// IPBanKindCidr 网段封禁（cidrBans 列表条目，IP 字段为规范化 CIDR 串）。
	IPBanKindCidr = "cidr"
)

// IPBanRecord 单条手动封禁记录（连接管控七期）：IPBanService 手动封禁状态机的
// 持久化形态，存储于 data/ip_bans.json（util.SaveJSON 原子写，对齐 web_bans.json
// 四期先例）。仅手动封禁/解封触发落盘（自动封禁重启清零语义不变）；已过期条目
// 载入时丢弃（文件有界自洁，「已过期」态仅运行期内可见）。字段全为基本类型，
// 不穿越 Wails 绑定签名，无需手动补 models.ts 别名。
type IPBanRecord struct {
	// IP 被封禁的来源 IP（已归一形态，条目主键）。Kind=IPBanKindCidr 时该字段
	// 存规范化 CIDR 串（如 "203.0.113.0/24"）。
	IP string `json:"ip"`
	// Kind 条目形态（连接管控八期）：IPBanKindCidr = 网段封禁；空串 = 单 IP
	// 精确封禁。omitempty 使 v1 文件（无 kind 字段）反序列化后 Kind 恒为空串
	// ——空串即单 IP 语义，v1/v2 双版载入零特判兼容（服务层仅判 Kind ==
	// IPBanKindCidr 分流，其余形态一律走既有单 IP 载入路径）。
	Kind string `json:"kind,omitempty"`
	// BannedAt 封禁时间（unix 秒；重复封禁刷新为本次时间）。
	BannedAt int64 `json:"bannedAt"`
	// ExpiresAt 封禁到期时间（unix 秒；0 = 永久封禁，须手动解封）。懒过期判定
	// 与 WebBanRecord.ExpiresAt 同款约定。
	ExpiresAt int64 `json:"expiresAt"`
	// Reason 封禁备注（可空；输入为空时服务层生成缺省文案「管理员手动封禁
	// （永久）」/「管理员手动封禁，至 <时刻>」后落盘，重启载入后文案不丢）。
	Reason string `json:"reason,omitempty"`
	// ActorSource 封禁操作者来源（desktop | rpc，对齐审计 actorSource 口径；
	// 手动封禁恒非空）。
	ActorSource string `json:"actorSource,omitempty"`
	// ActorTokenID 封禁操作者令牌 ID（RPC 调用者；桌面为空）。
	ActorTokenID string `json:"actorTokenId,omitempty"`
}

// IPBanList data/ip_bans.json 文件根结构。
type IPBanList struct {
	Version int           `json:"version"`
	Bans    []IPBanRecord `json:"bans"`
}

// IPBanListVersion 当前 data/ip_bans.json 文件格式版本。2 = 连接管控八期引入
// Kind 字段（网段封禁条目）；载入侧不做版本硬校验（v1 文件无 kind 字段天然
// 解析为单 IP 条目，见 IPBanRecord.Kind 注释），版本号仅作格式标记与未来迁移
// 判定位。
const IPBanListVersion = 2
