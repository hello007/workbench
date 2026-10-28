package service

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"workbench/model"
)

// 认证失败自动封禁默认参数（连接管控五期定稿常量；六期参数化后降级为默认值
// 语义——服务字段初值与设置段 DefaultIPBanSettings 同源，运行期经 ApplyConfig
// 热更。导出保留供既有测试引用与文档锚点）：
//   - 阈值：滑动窗口内累计失败达到 IPBanThreshold 次即自动封禁；
//   - 窗口：IPBanWindow 内的失败才参与计数（窗口外时间戳按次剪枝）；
//   - 时长：首次封禁持续 IPBanDuration，到期懒恢复，期满后窗口重新计数；
//   - 封顶：递增封禁的时长上限（六期新增，反复触发封越久但不过 IPBanMaxBanDuration）。
//
// 参数运行期取值来自设置段（AppSettings.IPBan，clamp 归一后经 ApplyConfig 注入），
// 变更即时生效于后续判定；生效中封禁的到期时刻不追溯调整，在途失败计数不清零
// （六期定稿：调参仅面向未来，不产生赦免或延长）。
const (
	IPBanThreshold      = 10
	IPBanWindow         = 10 * time.Minute
	IPBanDuration       = 5 * time.Minute
	IPBanMaxBanDuration = 60 * time.Minute
)

// ipBanEntry 单个来源 IP 的失败计数与封禁状态（map 值，仅 IPBanService.mu 锁内
// 可变）。failures 为滑动窗口内的失败时间戳队列（头部为最早，剪枝保序）；
// bannedAt 为最近一次封禁时刻（零值 = 从未触发封禁，仅失败计数的条目）；
// lastFailCount 冻结触发封禁时的窗口计数（封禁期间失败不累计，供审计与列表
// 展示；手动封禁清零——无自动触发语境）；strikes 为累计触发自动封禁次数（六期
// 递增封禁：认证成功清失败计数不清此值，解封/ClearAll/重启归零——递增记忆只在
// 显式重置或进程生命周期内保留）。
//
// 七期手动封禁扩展四字段（自动路径恒零值不写）：source 标记条目来源（manual/
// auto，决定 List 徽标分流与持久化资格——仅 manual 落盘）；reason/actorSource/
// actorTokenID 为手动封禁的备注与操作者归因快照（持久化复现与重启载入保真，
// 自动封禁的同类信息仅随审计行落盘不驻留条目）。手动封禁覆盖条目时四字段整体
// 刷新（对齐四期「重复封禁 = 刷新参数」），failures/strikes 不动。
type ipBanEntry struct {
	ip            string
	failures      []time.Time
	bannedAt      time.Time
	bannedUntil   time.Time
	lastFailCount int
	strikes       int
	source        string
	reason        string
	actorSource   string
	actorTokenID  string
}

// banActive 封禁是否生效中（判定谓词单点，调用方须持 mu）：bannedAt 非零且
// 未到期。bannedUntil 零值 = 永久封禁（七期手动封禁 expiresAt=0 形态，无到期
// 时刻恒生效，仅 UnbanIP 可解除）；自动封禁恒有非零 bannedUntil，零值分支对
// 其不可达（既有语义零回归）。
func (e *ipBanEntry) banActive(now time.Time) bool {
	if e.bannedAt.IsZero() {
		return false
	}
	if e.bannedUntil.IsZero() {
		return true
	}
	return now.Before(e.bannedUntil)
}

// IPBanService 浏览器访问通道的来源 IP 自动封禁服务（连接管控五期）：认证失败
// 频次达标即自动临时封禁来源 IP（短时限），补「管理员不在线时的被动安全」档位。
// 拒绝点在两认证点 verify 之前（与四期 BanCheck 的 tokenID 维度判定并存两道
// 独立防线，注入装配属后续任务范围，本服务只维护状态机本身）。
//
// 纯内存态不落盘（重启清零，攻击间隙 ≤ 5 分钟可接受）：失败计数与封禁记录均
// 不持久化，对齐「封禁为管控增强面」定位；过期记录懒清理不删除（列表保留
// 「已过期」态可见性到重启，禁后台扫描，对齐 WebTokenService 封禁先例）。
//
// 时间计算：全部差值判定（窗口剪枝、封禁到期）均以 nowFn 返回值之间的
// Sub/Before 完成——生产 nowFn=time.Now() 时其返回值携带单调时钟读数，差值
// 优先取单调差（系统墙钟回拨不破坏窗口与封禁判定）；测试注入无单调读数的固定
// 时钟时退化为墙钟差（注入推进语义所需）。nowFn 产生的墙钟值仅用于对外暴露的
// unix 秒（List/审计行/拒绝消息中的解封时刻）。
type IPBanService struct {
	mu      sync.Mutex
	entries map[string]*ipBanEntry
	// 四参数（六期参数化）：初值取五期常量（与设置段默认值同源），运行期经
	// ApplyConfig 整体替换（mu 锁内，保存链单点热更）。变更即时生效于后续的
	// 窗口剪枝/阈值判定/封禁时长计算；不动 entries（在途封禁不追溯、计数不清）。
	threshold      int
	window         time.Duration
	duration       time.Duration
	maxBanDuration time.Duration
	// whitelistNets 可信网段白名单（七期）：ApplyConfig 锁内对设置段白名单逐条
	// 预解析为 *net.IPNet 整体替换——判定热路径（每次认证失败的 RecordFailure
	// 与 RejectMessage）零重复解析。豁免判定单点 isWhitelistedIP（回环收编），
	// 三处消费：RecordFailure 不计数、RejectMessage 追溯放行、Ban 拒绝封禁。
	whitelistNets []*net.IPNet
	// cidrBans 网段手动封禁列表（八期，service/ip_ban_cidr.go）：独立于 entries
	// 的刻意决策——entries 的 failures/strikes 是单 IP 自动封禁专属语义（五/六期
	// 状态机），网段条目恒 manual 语义无计数面，混存会污染遍历与快照。判定链
	// 白名单 → entries 精确查 → 本列表线性扫 Contains（条目量级 = 管理员手动
	// 操作数，无压力），见 matchCidrBan。
	cidrBans []*ipBanCidrEntry
	// suggestThreshold 网段聚合建议阈值（九期，service/ip_ban_aggregate.go）：
	// 同聚合网段（IPv4 /24、IPv6 /64）内参与聚合的自动封禁条目数达到该值时，
	// AggregateSuggestions 产出网段封禁建议。构造以默认值起跑，运行期经
	// ApplyConfig 注入设置段归一值（IPBanSettings.aggregateSuggestThreshold，
	// clamp 2-50——对齐六期四参数先例，「原字段换参不动 entries」纪律同款）。
	suggestThreshold int
	// trendWindowDays 攻击趋势回看窗口天数（十期，service/ip_ban_trend.go）：
	// QueryAuditTrend 审计历史聚合的天级时间桶数量。构造以默认值起跑，运行期经
	// ApplyConfig 注入设置段归一值（IPBanSettings.trendWindowDays，clamp 1-30
	// ——对齐九期聚合建议阈值先例）。
	trendWindowDays int
	// 手动封禁持久化（七期）：bansPath 为 data/ip_bans.json（SetBansPath 注入，
	// 未注入为纯内存模式——既有构造路径零行为变化）；manualVersion 为手动条目
	// 变更代际（Ban 写入/UnbanIP 删除/载入/ClearAll 均在 mu 锁内递增），落盘
	// 「快照-IO-复核」流程以此检测 IO 期间的并发变更（见 persistManualBans）。
	bansPath      string
	manualVersion int
	nowFn         func() time.Time
	audit         *AuditLogger            // 审计出口（可 nil：未注入时静默跳过，旁路观测面）
	onBanned      func(model.IPBanStatus) // 自动封禁触发回调（可 nil：未注入不通知）
}

// NewIPBanService 构造 IP 自动封禁服务。nowFn 为时间源（nil 时默认 time.Now；
// 测试注入固定/可推进时钟驱动窗口滑动与到期，不依赖 sleep 与真实时钟）。四参数
// 以五期常量初值起跑；装配层在构造后经 ApplyConfig 注入设置段归一值（热更入口
// 与启动初值同一路径，无第二初始化通道）。聚合建议阈值（九期）以默认值起跑，
// 运行期同经 ApplyConfig 注入设置段归一值。
func NewIPBanService(nowFn func() time.Time) *IPBanService {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &IPBanService{
		entries:          make(map[string]*ipBanEntry),
		threshold:        IPBanThreshold,
		window:           IPBanWindow,
		duration:         IPBanDuration,
		maxBanDuration:   IPBanMaxBanDuration,
		suggestThreshold: IPBanAggregateSuggestThresholdDefault,
		trendWindowDays:  IPBanTrendWindowDaysDefault,
		nowFn:            nowFn,
	}
}

// ApplyConfig 热更四参数与白名单（连接管控六期：设置保存链「保存+生效」单点的
// 生效步；七期扩白名单段，九期扩聚合建议阈值，十期扩趋势回看窗口天数）。入参为
// 归一后的设置段值（调用方经 NormalizeIPBanSettings 保证；本方法不重复 clamp
// ——双口径收敛点在 model 层单点）。mu 锁内整体替换，原子切换：并发判定要么看
// 到全套旧参数要么全套新参数，无半套中间态。白名单逐条预解析为 *net.IPNet
// （service/ip_ban_whitelist.go parseIPBanWhitelistEntry，裸 IP 自动补 /32 //128；
// 非法条目 model 层已剔除，此处再拦一手防御直灌形态），整体替换后判定热路径零
// 重复解析。生效语义（六期定稿）：仅面向未来——生效中封禁的 bannedUntil 不变
// （已执行动作不追溯，改小时长不提前释放、改大不延长），在途失败计数不清零
// （调参不意外赦免攻击者）；threshold 改小后存量计数自然满足，下次失败即触发，
// 无主动扫描；聚合建议阈值同口径（九期：改小后存量条目自然满足，下次
// AggregateSuggestions 即产出）；趋势窗口天数同口径（十期：下次 QueryAuditTrend
// 即按新窗口聚合，无追溯重算面）。白名单为唯一例外（七期定稿「豁免 = 语义
// 修正」）：命中生效中封禁的 IP 追溯放行（在途记录不删，白名单移除且未过期即
// 恢复拒连）。
func (s *IPBanService) ApplyConfig(cfg model.IPBanSettings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.threshold = cfg.Threshold
	s.window = time.Duration(cfg.WindowMinutes) * time.Minute
	s.duration = time.Duration(cfg.DurationMinutes) * time.Minute
	s.maxBanDuration = time.Duration(cfg.MaxBanDurationMinutes) * time.Minute
	s.suggestThreshold = cfg.AggregateSuggestThreshold
	s.trendWindowDays = cfg.TrendWindowDays
	nets := make([]*net.IPNet, 0, len(cfg.Whitelist))
	for _, item := range cfg.Whitelist {
		if n, ok := parseIPBanWhitelistEntry(item); ok {
			nets = append(nets, n)
		}
	}
	s.whitelistNets = nets
}

// SetAudit 注入审计出口（ip_ban/ip_unban 事件落盘）。
func (s *IPBanService) SetAudit(a *AuditLogger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit = a
}

// SetOnBanned 注入自动封禁触发回调（桌面 toast 通知数据源，main 装配层在服务
// 开始前注入；生产装配一次即定，无运行期热更路径——与 hub SetOnConnectionsChange
// 同款口径）。回调载荷为刚写入内存的封禁状态快照（IP/触发时刻/到期时刻/窗口
// 失败计数，对齐 model.IPBanStatus json tag 小写形态）。
//
// 调用时序：封禁写入内存（含 ip_ban 审计行）之后在 RecordFailure 返回前同步
// 触发；mu 已释放——回调实现方不得回调本服务任何方法（无重入死锁面），也不得
// 阻塞（认证失败热路径上同步执行）。
func (s *IPBanService) SetOnBanned(fn func(model.IPBanStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onBanned = fn
}

// RecordFailure 登记一次来源 IP 的认证失败：
//   - 空串直接忽略；回环地址与白名单网段命中（七期豁免判定单点 isWhitelistedIP
//     收编回环既有口径）完全忽略——不创建条目不进失败队列（NAT 共享出口等可信
//     来源不参与自动封禁，桌面同开场景下本机误配不触发自锁）；
//   - 封禁生效中（未到期；永久封禁恒生效，七期 banActive 单点判定）不累计
//     （封禁期间失败不再计数）；
//   - 剪枝失效时间戳后追加本次失败；窗口内失败数达到阈值即触发自动封禁
//     （strikes 递增 + 刷新 bannedAt/bannedUntil/lastFailCount，封禁时长按
//     递增档位计算，写 ip_ban 审计行 actorSource=system）。
//
// 剪枝规则统一承载「期满重新计数」语义：有封禁历史的条目，封禁到期时刻之前的
// 旧失败一并失效——否则封禁前记满窗口的失败在期满后仍在窗口内，一次新失败即
// 叠加触发立即再封，违背「期满重新起算」。封禁标记（bannedAt）不因过期复位，
// 保住 List 对「已过期」记录的可见性（懒过期，记录保留到重启）。strikes 不受
// 剪枝与成功清零影响（六期递增记忆：期满恢复后窗口从零重新计满，但下次触发
// 封禁时长按 strikes+1 递增——持续攻击成本递增，合法用户被解封后重置）。
//
// ip 为调用方连接的 RemoteAddr 原型串（如 "1.2.3.4:5678"、"[::1]:8080"），
// 服务内部经 normalizeIP 归一。
func (s *IPBanService) RecordFailure(ip string) {
	ip = normalizeIP(ip)
	if ip == "" {
		return
	}
	s.mu.Lock()
	// 白名单豁免单点（七期）：nets 随 mu 锁内读取，豁免来源不建条目不计数
	//（对齐回环既有「完全忽略」语义，CIDR 命中网段同口径）
	if isWhitelistedIP(ip, s.whitelistNets) {
		s.mu.Unlock()
		return
	}
	now := s.nowFn()
	entry, ok := s.entries[ip]
	if !ok {
		entry = &ipBanEntry{ip: ip}
		s.entries[ip] = entry
	}
	if entry.banActive(now) {
		s.mu.Unlock()
		return
	}
	// 剪枝：窗口外时间戳出窗（window 为运行期参数，热更后新窗口按新长度剪枝）；
	// 封禁到期时刻之前的旧失败失效（从未封禁的条目 cutoff 为零值，仅走窗口剪枝）
	cutoff := entry.bannedUntil
	kept := entry.failures[:0]
	for _, ts := range entry.failures {
		if now.Sub(ts) < s.window && ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	entry.failures = append(kept, now)
	// 封禁触发的通知面（回调与载荷快照在锁内备妥，出锁后触发——回调实现方
	// 不得回调本服务方法，锁内直调会形成同 goroutine 重入死锁，对齐
	// notifyConnectionDetails 的出锁后触发口径）
	var notify func(model.IPBanStatus)
	var status model.IPBanStatus
	if len(entry.failures) >= s.threshold {
		entry.strikes++
		entry.source = model.IPBanSourceAuto
		entry.bannedAt = now
		banDuration := escalateBanDuration(s.duration, s.maxBanDuration, entry.strikes)
		entry.bannedUntil = now.Add(banDuration)
		entry.lastFailCount = len(entry.failures)
		s.auditIPBanLocked(entry, banDuration)
		status = model.IPBanStatus{
			IP:        entry.ip,
			BannedAt:  entry.bannedAt.Unix(),
			ExpiresAt: entry.bannedUntil.Unix(),
			FailCount: entry.lastFailCount,
			Strike:    entry.strikes,
		}
		notify = s.onBanned
	}
	s.mu.Unlock()
	if notify != nil {
		notify(status)
	}
}

// escalateBanDuration 递增封禁时长（连接管控六期）：第 strikes 次触发的封禁
// 时长 = base × 2^(strikes-1) 封顶 max（1 次 = base，2 次 = 2×base，以此类推）。
// 循环翻倍 clamp 实现——禁 math.Pow 浮点，且 strikes 值任意大时循环在达 max 后
// 即止（有界），无整数溢出面（max ≤ 1440min，翻倍中途恒可安全 ×2）。
// base > max 的矛盾配置由 normalize 层收敛（MaxBanDuration ≥ Duration），此处
// 保留 d > max 兜底防御。
func escalateBanDuration(base, max time.Duration, strikes int) time.Duration {
	d := base
	for i := 1; i < strikes && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	return d
}

// RecordSuccess 登记一次来源 IP 的认证成功：清空该 IP 的失败计数（封禁生效中
// 也清计数，但封禁本身不解除——封禁到期判定独立，手动解除走 UnbanIP）。
// 纯计数条目（从未触发封禁）清零后即从 map 移除（无保留价值）；有封禁历史的
// 条目保留（List 可见性与期满重计语义依赖 bannedAt 判定）。strikes 刻意不清
// （六期递增记忆：拿到令牌的合法登录不代表攻击停止，递增档位仅在显式重置面
// ——UnbanIP/ClearAll/重启——归零）。
func (s *IPBanService) RecordSuccess(ip string) {
	ip = normalizeIP(ip)
	if ip == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[ip]
	if !ok {
		return
	}
	entry.failures = nil
	if entry.bannedAt.IsZero() {
		delete(s.entries, ip)
	}
}

// RejectMessage IP 封禁拒绝消息（后续装配层注入两认证点的生产实现原型）：来源
// IP 命中生效中的封禁时返回专用拒绝消息（含自动恢复时刻或「永久封禁」标识，
// 与普通 401「请携带访问令牌」可区分），未封禁/已过期（懒过期放行，记录不删）
// 返回空串。白名单命中（七期，豁免判定单点含回环）恒放行——豁免为语义修正，
// 追溯作用于生效中封禁（在途记录不删，白名单移除且未过期即恢复拒连）。ip 为
// 空串直接放行。
//
// 判定链（八期扩网段维度）：白名单豁免 → entries 精确查 → cidrBans 线性扫
// Contains。精确条目命中优先出单 IP 消息（更具体优先——精确条目与网段条目
// 并存时管理员处置对象以精确为准）；仅命中网段条目出网段专用消息（文案含
// 网段串与永久/到期标识）。
func (s *IPBanService) RejectMessage(ip string) string {
	ip = normalizeIP(ip)
	if ip == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if isWhitelistedIP(ip, s.whitelistNets) {
		return ""
	}
	now := s.nowFn()
	if entry, ok := s.entries[ip]; ok && entry.banActive(now) {
		if entry.bannedUntil.IsZero() {
			return "该来源 IP 已被封禁（永久封禁），请联系管理员解封"
		}
		return "该来源 IP 已被临时封禁，至 " + entry.bannedUntil.Format(banExpiryMessageFormat) + " 自动恢复"
	}
	if cidr := s.matchCidrBan(ip, now); cidr != nil {
		if cidr.bannedUntil.IsZero() {
			return "该来源 IP 已被网段封禁（" + cidr.cidr + "，永久），请联系管理员解封"
		}
		return "该来源 IP 已被网段临时封禁，至 " + cidr.bannedUntil.Format(banExpiryMessageFormat) + " 自动恢复"
	}
	return ""
}

// UnbanIP 手动解封指定来源 IP：删除封禁记录并清失败计数与递增档位（strikes
// 随条目删除一并归零——管理员显式介入即完全重置，下次触发从第 1 档重新起算，
// 连接管控六期），返回是否实际解封
// （true = 存在封禁记录并被删除）。无记录（含仅有失败计数无封禁的条目——顺手
// 清除计数但不构成解封）为 no-op 返回 false（幂等：重复解封/列表过期重试不报
// 错）。实际解除才写 ip_unban 审计行（对齐「动作生效后才写审计」纪律），操作者
// 归因透传 actorSource/actorTokenID（桌面路径 desktop/空，浏览器 RPC 路径经
// rpcAuditTarget 装饰捕获调用者令牌 ID 后 rpc/记录 ID，对齐 WebTokenService.Unban
// 先例）。
//
// 持久化联动（七期）：解封删除手动条目后触发落盘更新（data/ip_bans.json 移除
// 该记录，防重启后手动封禁复活）；落盘失败回滚内存（重插条目，解封未生效）返回
// false 且不写审计。自动条目解封同样触发落盘（快照只含手动条目，本次写入即为
// 重申当前手动集合——无手动条目时写空列表，无害幂等）。未注入 bansPath（纯内存
// 模式）时落盘 no-op，既有语义零变化。
func (s *IPBanService) UnbanIP(ip, actorSource, actorTokenID string) bool {
	ip = normalizeIP(ip)
	if ip == "" {
		return false
	}
	s.mu.Lock()
	entry, ok := s.entries[ip]
	if !ok {
		s.mu.Unlock()
		return false
	}
	delete(s.entries, ip)
	if entry.bannedAt.IsZero() {
		// 仅有失败计数的条目：清计数不构成解封，不落盘不审计
		s.mu.Unlock()
		return false
	}
	audit := s.audit
	s.manualVersion++
	s.mu.Unlock()

	if err := s.persistManualBans(); err != nil {
		// 落盘失败回滚：重插删除前的条目快照（仅当键仍空缺——并发 Ban 已重建
		// 同名条目时不覆盖新值）；解封未生效不写审计
		s.mu.Lock()
		if _, exists := s.entries[ip]; !exists {
			s.entries[ip] = entry
		}
		s.mu.Unlock()
		return false
	}
	if audit != nil {
		audit.LogIPUnban(IPBanAudit{IP: ip, ActorSource: actorSource, ActorTokenID: actorTokenID})
	}
	return true
}

// List 返回全部 IP 封禁记录（含已到期未清理的——列表「已过期」态展示数据源，
// 拒连判定的懒过期语义以 RejectMessage 为准）。仅含曾触发封禁的条目（纯失败
// 计数不进列表）；Source 字段透出条目来源（七期：manual 管理员手动下发 / auto
// 认证失败自动触发，前端徽标分流）。永久封禁（七期手动 expiresAt=0 形态）
// ExpiresAt 透出 0、Expired 恒 false。返回切片为内部数据拷贝，调用方修改不影
// 响服务状态；空列表返回空切片（非 nil）。
//
// 八期扩网段条目合并输出（Kind 分流单 IP/网段）与排序两级化：封禁中在前已过期
// 在后（处置优先级），组内网段条目置前（影响面大优先呈现）再按封禁时间降序。
// CoveredBy 为后端单点计算的覆盖标注（cidrCoveredBy，口径与 RejectMessage 同源
// 的 Contains 判定），前端只渲染不复刻判定。
func (s *IPBanService) List() []model.IPBanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowFn()
	out := make([]model.IPBanStatus, 0, len(s.entries)+len(s.cidrBans))
	for _, e := range s.entries {
		if e.bannedAt.IsZero() {
			continue
		}
		expiresAt := int64(0)
		expired := false
		if !e.bannedUntil.IsZero() {
			expiresAt = e.bannedUntil.Unix()
			expired = !now.Before(e.bannedUntil)
		}
		var coveredBy string
		if parsed := net.ParseIP(e.ip); parsed != nil {
			coveredBy = s.cidrCoveredBy(parsed, nil, now)
		}
		out = append(out, model.IPBanStatus{
			IP:        e.ip,
			BannedAt:  e.bannedAt.Unix(),
			ExpiresAt: expiresAt,
			FailCount: e.lastFailCount,
			Strike:    e.strikes,
			Source:    e.source,
			Expired:   expired,
			Kind:      model.IPBanKindIP,
			CoveredBy: coveredBy,
		})
	}
	for _, e := range s.cidrBans {
		expiresAt := int64(0)
		expired := false
		if !e.bannedUntil.IsZero() {
			expiresAt = e.bannedUntil.Unix()
			expired = !now.Before(e.bannedUntil)
		}
		out = append(out, model.IPBanStatus{
			IP:        e.cidr,
			BannedAt:  e.bannedAt.Unix(),
			ExpiresAt: expiresAt,
			Source:    model.IPBanSourceManual,
			Expired:   expired,
			Kind:      model.IPBanKindCidr,
			CoveredBy: s.cidrCoveredBy(nil, e.net, now),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Expired != out[j].Expired {
			return !out[i].Expired // 封禁中在前，已过期在后
		}
		iCidr, jCidr := out[i].Kind == model.IPBanKindCidr, out[j].Kind == model.IPBanKindCidr
		if iCidr != jCidr {
			return iCidr // 同组内网段条目置前
		}
		return out[i].BannedAt > out[j].BannedAt
	})
	return out
}

// cidrCoveredBy 计算条目被哪个生效中网段条目覆盖（List 的 CoveredBy 数据源，
// 调用方须持 IPBanService.mu）：subjectIP 非 nil 时按具体 IP 判定 Contains
// （单 IP 条目命中网段条目）；否则 subjectNet 非 nil 时按「更宽网段」判定
// （掩码严格更短且包含网段网络地址——同掩码为自身不构成覆盖）。多命中取掩码
// 最短（最宏观）者——覆盖标注面向管理员展示最大处置范围。仅统计生效中条目
// （已过期网段不再拒连，不构成覆盖）。未覆盖返回空串（model 层 omitempty
// 序列化消失）。与 matchCidrBan 共用 cidrBans Contains 判定面（口径同源），
// 取向刻意不同：拒绝对外文案取最具体（掩码最长），覆盖标注取最宏观（掩码
// 最短）。
func (s *IPBanService) cidrCoveredBy(subjectIP net.IP, subjectNet *net.IPNet, now time.Time) string {
	var best *ipBanCidrEntry
	bestOnes := 0
	for _, e := range s.cidrBans {
		if !e.active(now) {
			continue
		}
		ones, bits := e.net.Mask.Size()
		// 取最宏观者：已有候选且本条掩码不更短（范围不更大）时不可能更宏观，
		// 跳过 Contains 探测（同族同掩码非同网段不相交，等掩码不会双双命中）
		if best != nil && ones >= bestOnes {
			continue
		}
		if subjectIP != nil {
			if !e.net.Contains(subjectIP) {
				continue
			}
		} else {
			// 网段被更宽网段覆盖：同族（掩码位数一致）且掩码严格更短（范围
			// 更大——同掩码即自身，不构成覆盖）且包含目标网段的网络地址
			subOnes, subBits := subjectNet.Mask.Size()
			if bits != subBits || ones >= subOnes {
				continue
			}
			if !e.net.Contains(subjectNet.IP) {
				continue
			}
		}
		best = e
		bestOnes = ones
	}
	if best == nil {
		return ""
	}
	return best.cidr
}

// ClearAll 清空全部来源 IP 的失败计数与封禁记录（含递增档位 strikes，六期——
// 重建 map 天然全清；令牌轮换/撤销联动调用：轮换产生的旧 cookie 失败潮防误伤
// 叠加）。不写审计——联动清零为预防性动作非处置，与撤销令牌联动清理封禁记录
// 的既有口径一致。手动条目一并清除但不触发落盘（七期：仅手动封禁/解封触发
// 保存的定稿口径——落盘文件保留手动记录，重启后手动封禁恢复生效，与「ClearAll
// 为防误伤的内存面预防动作」定位一致；八期网段条目同口径一并清内存）；manualVersion
// 递增使在途落盘的快照复核失效（防「快照含已清条目」的旧快照覆盖文件——见
// persistManualBans）。
func (s *IPBanService) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]*ipBanEntry)
	s.cidrBans = nil
	s.manualVersion++
}

// auditIPBanLocked 发 ip_ban 审计行（调用方须持 mu；audit 为 nil 时静默跳过）。
// actorSource 恒 system（自动触发无人类操作者），reason 文案携带触发时窗口内
// 失败计数、递增档位与本次实际封禁时长（六期：档位时长随 strikes 递增，文案
// 与生效时长一致才有排查价值；对齐四期「动作生效后才写审计」——封禁写入内存
// 后调用）。
func (s *IPBanService) auditIPBanLocked(entry *ipBanEntry, banDuration time.Duration) {
	if s.audit == nil {
		return
	}
	s.audit.LogIPBan(IPBanAudit{
		IP:          entry.ip,
		FailCount:   entry.lastFailCount,
		Strike:      entry.strikes,
		ExpiresAt:   entry.bannedUntil.Unix(),
		Reason:      fmt.Sprintf("%d 分钟内认证失败 %d 次，第 %d 次触发，自动临时封禁 %d 分钟", int(s.window/time.Minute), entry.lastFailCount, entry.strikes, int(banDuration/time.Minute)),
		ActorSource: AuditActorSystem,
	})
}

// normalizeIP 归一连接地址为来源 IP：net.SplitHostPort 成功取 host（覆盖
// "1.2.3.4:5678" 与 "[::1]:8080" 两形态），失败（无端口裸 IP、裸 IPv6 多冒号、
// 空串等）原样返回。调用方传 RemoteAddr 原型串，服务内部统一归一；不做大小写
// 或 IPv6 展缩写规范化（保留原样语义，计数与封禁按归一后的字符串精确匹配）。
func normalizeIP(raw string) string {
	if raw == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return host
	}
	return raw
}

// isLoopbackIP 判断归一后的 IP 是否回环地址（127.0.0.1/::1 及其同义形态）。
// 非法 IP 字符串（net.ParseIP 为 nil）不视为回环——解析失败的地址按普通来源
// 参与计数（封禁判定不受影响）。
func isLoopbackIP(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.IsLoopback()
}
