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

// IPBanAggregateSuggestion 网段聚合封禁建议条目（连接管控九期）：同聚合网段内
// 自动封禁来源数达到阈值时产出，供处置面（桌面 IP 封禁区块「聚合建议」小节）
// 提示「该网段 N 个来源已被封禁，建议网段封禁」并一键预填网段封禁弹窗——闭环
// 止于预填，处置仍须管理员确认提交，不做自动封禁。
//
// 聚合粒度写死 IPv4 /24、IPv6 /64（网段级攻击源典型漂移范围，不随白名单/封禁
// 条目粒度推导）；数据源口径见 service.IPBanService.AggregateSuggestions（仅 auto 且
// 已触发封禁的内存条目，含已过期，与 List 可见条目同口径；被生效中网段封禁
// 覆盖的条目剔除防重复建议；不用审计行——含已解封历史与轮转边界漂移，口径糊）。
//
// 隐私红线：仅桌面出口（App.ListIPBanSuggestions 已进 server RPCHiddenMethods
// 屏蔽列表，浏览器通道与「方法不存在」同响应，整体不可达无降级 UI——对齐他人
// IP 明细仅走桌面出口的既有红线）；载荷仅网段串与计数，无具体 IP 明细。字段全
// 为基本类型，struct 形态经 wails generate 自动生成 models.ts class（无需手动
// 补别名，区别于 type X string 具名别名规则）。
type IPBanAggregateSuggestion struct {
	// CIDR 建议封禁的聚合网段（规范化形态：网络地址 + 掩码的 ipnet.String()，
	// 如 "203.0.113.0/24"、IPv6 如 "2001:db8:1:2::/64"），可直接作为网段封禁
	// 入参复用八期解析口径。
	CIDR string `json:"cidr"`
	// Count 该网段内参与聚合的自动封禁条目数（与阈值判定同源——产出建议的
	// 桶计数恒不小于建议阈值，前端文案「该网段 N 个来源已被封禁」直接消费）。
	Count int `json:"count"`
}

// IPBanTrendBucket 攻击趋势聚合的单桶条目（连接管控十期）：审计历史 ip_ban 行
// 按固定粒度桶聚合的天级时序视图条目。桶 key 为网段串（单 IP 掩入 IPv4 /24、
// IPv6 /64 固定粒度，与九期聚合建议同粒度）或手动网段封禁的规范化 CIDR 原串
// （ipnet.String()）；聚合口径详见 service.aggregateIPBanTrend doc comment。
//
// 隐私粒度封顶：载荷仅桶 key + 天级计数序列 + 计数 + 布尔，无单 IP 明细（单 IP
// 历史全量明细桌面已有审计弹窗入口，趋势面桶聚合正是差异化价值；对齐九期
// 「仅网段串+计数无 IP 明细」纪律）。App.QueryIPBanTrend 进 server
// RPCHiddenMethods 屏蔽列表（整体不进浏览器通道）。字段全为基本类型，struct
// 形态经 wails generate 自动生成 models.ts class（无需手动补别名）。
type IPBanTrendBucket struct {
	// Key 桶标识：单 IP 掩入的固定粒度网段串（"203.0.113.0/24"）或手动网段
	// 封禁的规范化 CIDR 原串。kind 从 key 形态天然推导（与桶由何种行产生一致），
	// 无额外字段。
	Key string `json:"key"`
	// Days 窗口内天级自动封禁计数序列，长度恒等于窗口天数；下标 0 = 窗口最旧日，
	// 末下标 = 今天（本地时区日界）。仅 auto（actorSource=system）行计入。
	Days []int `json:"days"`
	// AutoTotal 主趋势计数：窗口内该桶 auto（actorSource=system）ip_ban 行总数
	// （恒等于 sum(Days)），机器观测的攻击频度。
	AutoTotal int `json:"autoTotal"`
	// ManualCount 管理员处置计数：窗口内该桶 manual（actorSource=desktop/rpc）
	// ip_ban 行总数，与主趋势分列不混入（auto=攻击频度、manual=人类决策频度，
	// 混则口径失真）。
	ManualCount int `json:"manualCount"`
	// Rebounded 解封后再犯标记（后端计算透传，前端零判定）：桶内存在解封时间
	// 之后又有 auto ip_ban 行的攻击事实。
	Rebounded bool `json:"rebounded"`
	// LastSeen 桶内最新一条 ip_ban 行（auto/manual 均计）的 ts（unix 秒）。
	LastSeen int64 `json:"lastSeen"`
}

// IPBanTrendResult 攻击趋势聚合查询结果（连接管控十期，App.QueryIPBanTrend
// 返回形态）：基于审计行 ip_ban/ip_unban 的跨重启攻击源时序视图，补九期
// （内存条目、仅窗口内事实）之外的长期面。
type IPBanTrendResult struct {
	// WindowDays 本次查询生效的回看窗口天数（设置段 trendWindowDays 归一值，
	// 透传给前端对齐 Days 数组长度）。
	WindowDays int `json:"windowDays"`
	// EarliestTs 审计快照中最早一条 ip_ban 行的 ts（unix 秒；0 = 快照无任何
	// ip_ban 行）。含窗口外与桶解析失败行——语义为「数据自何时起」，前端据
	// earliestTs 是否早于窗口起点判定「留存是否覆盖整个窗口」，不覆盖时提示
	// 「数据自 M-D 起」（轮转丢历史诚实标注，禁静默降级）。
	EarliestTs int64 `json:"earliestTs"`
	// Buckets 聚合桶切片，恒非 nil（空态返回空切片）。排序：AutoTotal 降序
	// （影响面大优先），同计数按 Key 字典序升序（确定性输出，前端渲染顺序稳定，
	// 对齐九期建议排序纪律）。
	Buckets []IPBanTrendBucket `json:"buckets"`
}
