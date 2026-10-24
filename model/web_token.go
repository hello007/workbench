package model

// WebTokenRecord 浏览器访问通道的多 token 记录（GitHub PAT 式：落盘只存 SHA-256
// 哈希，明文仅在创建时一次性返回，不落盘）。存储于 data/web_tokens.json
// （util.SaveJSON 原子写，对齐 data-atomic-write 规范）。
//
// 字段全为基本类型（string/int64/bool）：无具名类型穿越 Wails 绑定签名，
// 无需手动补 models.ts type 别名（CLAUDE.md 跨层契约规则）。
type WebTokenRecord struct {
	// ID 短随机 id（8 位 hex，非 secret）：审计日志与前端列表操作的归因键。
	ID string `json:"id"`
	// Name 设备/用途名（如「手机」「macbook-safari」），创建时用户填写。
	Name string `json:"name"`
	// Hash SHA-256(明文) 的 hex 编码。明文不落盘。
	Hash string `json:"hash"`
	// CreatedAt 创建时间（unix 秒）。
	CreatedAt int64 `json:"createdAt"`
	// LastUsedAt 最近认证命中时间（unix 秒；0 = 从未使用）。内存更新 +
	// debounce 落盘（约 5s 延迟），重启丢弃最近数秒精度，可接受。
	LastUsedAt int64 `json:"lastUsedAt"`
	// ExpiresAt 过期时间（unix 秒；0 = 永不过期）。认证时惰性判定，过期记录
	// 不自动删除（保留审计与列表可见性，界面标记「已过期」）。
	ExpiresAt int64 `json:"expiresAt"`
	// Revoked 已撤销。撤销立即生效（后续认证全部拒绝）；记录保留不删除。
	Revoked bool `json:"revoked"`
}

// WebTokenList data/web_tokens.json 文件根结构。
type WebTokenList struct {
	Version int              `json:"version"`
	Tokens  []WebTokenRecord `json:"tokens"`
}

// WebTokenListVersion 当前 data/web_tokens.json 文件格式版本。
const WebTokenListVersion = 1

// WebTokenCreated 新建 token 的一次性返回：明文仅经本结构返回一次（不落盘），
// Record 为持久化形态（只含哈希）。
//
// Wails v2.16 绑定仅支持 1~2 个返回值（value, error），三返回值
// （plaintext, record, error）不可绑定，故以单结构体承载两个出参。
type WebTokenCreated struct {
	// PlainText 明文 token（64 位 hex）。仅创建时经本字段返回一次，服务端不落盘。
	PlainText string `json:"plainText"`
	// Record 新建的 token 记录（含 ID/Name/CreatedAt 等，Hash 非明文）。
	Record WebTokenRecord `json:"record"`
}

// WebTokenStatus 令牌记录 + 在线连接数（App.ListWebTokens 的返回包装形态，
// 连接管控任务新增）。在线数由 App 层连接明细缓存（hub 连接回调写入）按
// TokenID 聚合 join 而来，单调用列表+计数原子一致；**不参与落盘序列化**——
// data/web_tokens.json 仍以 WebTokenRecord 单独落盘（运行时字段不污染存储
// 形态），本结构仅存在于绑定返回值面。
type WebTokenStatus struct {
	// Record 令牌记录（含已撤销/已过期记录，与原 ListWebTokens 列表口径一致）。
	Record WebTokenRecord `json:"record"`
	// OnlineConns 该令牌当前在途 WS 连接数（0 = 无在线连接；令牌已撤销但连接
	// 未断时 Record.Revoked=true 且本值 > 0，与明细面板 TokenName 空串同款语义）。
	OnlineConns int `json:"onlineConns"`
	// 封禁状态三字段（连接管控四期，join 形态对齐 OnlineConns；数据源为
	// data/web_bans.json 封禁记录按 TokenID join，不参与任何落盘序列化）。
	// Banned=true 表示名册存在该令牌的封禁记录——含已过期记录（BannedUntil
	// 非 0 且不晚于当前时间为「已过期」态，拒连判定由服务层懒过期负责，此处
	// 原样透出供列表展示）；BannedUntil=0 表示永久封禁；BanReason 为封禁备注
	// （未填为空串）。
	Banned      bool   `json:"banned,omitempty"`
	BannedUntil int64  `json:"bannedUntil,omitempty"`
	BanReason   string `json:"banReason,omitempty"`
}

// WebBanRecord 单条令牌封禁记录（连接管控四期）：tokenID 维度临时拒连状态机
// 的持久化形态，存储于 data/web_bans.json（util.SaveJSON 原子写）。字段全为
// 基本类型，不单独穿越 Wails 绑定签名（封禁状态经 WebTokenStatus 扩字段 join
// 透出），无需手动补 models.ts type 别名。
type WebBanRecord struct {
	// TokenID 被封禁的令牌记录 ID（web_tokens.json 名册主键）。
	TokenID string `json:"tokenId"`
	// TokenName 封禁时的令牌名快照（审计与列表展示用；令牌改名后不回填）。
	TokenName string `json:"tokenName,omitempty"`
	// BannedAt 封禁时间（unix 秒；重复封禁刷新为本次时间）。
	BannedAt int64 `json:"bannedAt"`
	// ExpiresAt 封禁到期时间（unix 秒；0 = 永久封禁，须手动解封）。到期判定
	// 为懒式（认证点组合判定时比较当前时间），过期记录不自动删除（列表保留
	// 「已过期」态可见性），与 WebTokenRecord.ExpiresAt 同款约定。
	ExpiresAt int64 `json:"expiresAt"`
	// Reason 封禁备注（可空；审计行与前端徽标 tooltip 展示）。
	Reason string `json:"reason,omitempty"`
	// ActorSource 封禁操作者来源（desktop | rpc，对齐审计 actorSource 口径）。
	ActorSource string `json:"actorSource,omitempty"`
	// ActorTokenID 封禁操作者令牌 ID（RPC 调用者；桌面为空）。
	ActorTokenID string `json:"actorTokenId,omitempty"`
}

// WebBanList data/web_bans.json 文件根结构。
type WebBanList struct {
	Version int            `json:"version"`
	Bans    []WebBanRecord `json:"bans"`
}

// WebBanListVersion 当前 data/web_bans.json 文件格式版本。
const WebBanListVersion = 1
