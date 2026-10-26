package service

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	"workbench/model"
)

// 认证失败自动封禁参数（连接管控五期，导出供测试与后续装配层引用）：
//   - 阈值：滑动窗口内累计失败达到 IPBanThreshold 次即自动封禁；
//   - 窗口：IPBanWindow 内的失败才参与计数（窗口外时间戳按次剪枝）；
//   - 时长：封禁持续 IPBanDuration，到期懒恢复，期满后窗口重新计数。
//
// 一期硬编码不参数化（设置项为后续迭代）；持续攻击表现为「封禁 5 分钟 → 到期
// 重新计满 → 再封禁」的循环，等效于递进处置。
const (
	IPBanThreshold = 10
	IPBanWindow    = 10 * time.Minute
	IPBanDuration  = 5 * time.Minute
)

// ipBanEntry 单个来源 IP 的失败计数与封禁状态（map 值，仅 IPBanService.mu 锁内
// 可变）。failures 为滑动窗口内的失败时间戳队列（头部为最早，剪枝保序）；
// bannedAt 为最近一次自动封禁时刻（零值 = 从未触发封禁，仅失败计数的条目）；
// lastFailCount 冻结触发封禁时的窗口计数（封禁期间失败不累计，供审计与列表展示）。
type ipBanEntry struct {
	ip            string
	failures      []time.Time
	bannedAt      time.Time
	bannedUntil   time.Time
	lastFailCount int
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
	mu       sync.Mutex
	entries  map[string]*ipBanEntry
	nowFn    func() time.Time
	audit    *AuditLogger // 审计出口（可 nil：未注入时静默跳过，旁路观测面）
	onBanned func(model.IPBanStatus) // 自动封禁触发回调（可 nil：未注入不通知）
}

// NewIPBanService 构造 IP 自动封禁服务。nowFn 为时间源（nil 时默认 time.Now；
// 测试注入固定/可推进时钟驱动窗口滑动与到期，不依赖 sleep 与真实时钟）。
func NewIPBanService(nowFn func() time.Time) *IPBanService {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &IPBanService{
		entries: make(map[string]*ipBanEntry),
		nowFn:   nowFn,
	}
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
//   - 回环地址（127.0.0.1/::1，归一后判定）与空串直接忽略——本机访问不参与
//     自动封禁（桌面同开场景下本机误配不触发自锁）；
//   - 封禁生效中（未到期）不累计（封禁期间失败不再计数）；
//   - 剪枝失效时间戳后追加本次失败；窗口内失败数达到阈值即触发自动封禁
//     （刷新 bannedAt/bannedUntil/lastFailCount，写 ip_ban 审计行，
//     actorSource=system）。
//
// 剪枝规则统一承载「期满重新计数」语义：有封禁历史的条目，封禁到期时刻之前的
// 旧失败一并失效——否则封禁前记满窗口的 10 次失败在期满后仍在 10 分钟窗口内，
// 一次新失败即叠加触发立即再封，违背「期满重新起算」。封禁标记（bannedAt）
// 不因过期复位，保住 List 对「已过期」记录的可见性（懒过期，记录保留到重启）。
//
// ip 为调用方连接的 RemoteAddr 原型串（如 "1.2.3.4:5678"、"[::1]:8080"），
// 服务内部经 normalizeIP 归一。
func (s *IPBanService) RecordFailure(ip string) {
	ip = normalizeIP(ip)
	if ip == "" || isLoopbackIP(ip) {
		return
	}
	s.mu.Lock()
	now := s.nowFn()
	entry, ok := s.entries[ip]
	if !ok {
		entry = &ipBanEntry{ip: ip}
		s.entries[ip] = entry
	}
	if !entry.bannedAt.IsZero() && now.Before(entry.bannedUntil) {
		s.mu.Unlock()
		return
	}
	// 剪枝：窗口外时间戳出窗；封禁到期时刻之前的旧失败失效（从未封禁的条目
	// cutoff 为零值，仅走窗口剪枝）
	cutoff := entry.bannedUntil
	kept := entry.failures[:0]
	for _, ts := range entry.failures {
		if now.Sub(ts) < IPBanWindow && ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	entry.failures = append(kept, now)
	// 封禁触发的通知面（回调与载荷快照在锁内备妥，出锁后触发——回调实现方
	// 不得回调本服务方法，锁内直调会形成同 goroutine 重入死锁，对齐
	// notifyConnectionDetails 的出锁后触发口径）
	var notify func(model.IPBanStatus)
	var status model.IPBanStatus
	if len(entry.failures) >= IPBanThreshold {
		entry.bannedAt = now
		entry.bannedUntil = now.Add(IPBanDuration)
		entry.lastFailCount = len(entry.failures)
		s.auditIPBanLocked(entry)
		status = model.IPBanStatus{
			IP:        entry.ip,
			BannedAt:  entry.bannedAt.Unix(),
			ExpiresAt: entry.bannedUntil.Unix(),
			FailCount: entry.lastFailCount,
		}
		notify = s.onBanned
	}
	s.mu.Unlock()
	if notify != nil {
		notify(status)
	}
}

// RecordSuccess 登记一次来源 IP 的认证成功：清空该 IP 的失败计数（封禁生效中
// 也清计数，但封禁本身不解除——封禁到期判定独立，手动解除走 UnbanIP）。
// 纯计数条目（从未触发封禁）清零后即从 map 移除（无保留价值）；有封禁历史的
// 条目保留（List 可见性与期满重计语义依赖 bannedAt 判定）。
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
// IP 命中生效中的自动封禁时返回专用拒绝消息（含自动恢复时刻，与普通 401「请
// 携带访问令牌」可区分），未封禁/已过期（懒过期放行，记录不删）返回空串。
// ip 为空串直接放行。
func (s *IPBanService) RejectMessage(ip string) string {
	ip = normalizeIP(ip)
	if ip == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[ip]
	if !ok || entry.bannedAt.IsZero() || !s.nowFn().Before(entry.bannedUntil) {
		return ""
	}
	return "该来源 IP 已被临时封禁，至 " + entry.bannedUntil.Format(banExpiryMessageFormat) + " 自动恢复"
}

// UnbanIP 手动解封指定来源 IP：删除封禁记录并清失败计数，返回是否实际解封
// （true = 存在封禁记录并被删除）。无记录（含仅有失败计数无封禁的条目——顺手
// 清除计数但不构成解封）为 no-op 返回 false（幂等：重复解封/列表过期重试不报
// 错）。实际解除才写 ip_unban 审计行（对齐「动作生效后才写审计」纪律），操作者
// 归因透传 actorSource/actorTokenID（桌面路径 desktop/空，浏览器 RPC 路径经
// rpcAuditTarget 装饰捕获调用者令牌 ID 后 rpc/记录 ID，对齐 WebTokenService.Unban
// 先例）。
func (s *IPBanService) UnbanIP(ip, actorSource, actorTokenID string) bool {
	ip = normalizeIP(ip)
	if ip == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[ip]
	if !ok {
		return false
	}
	delete(s.entries, ip)
	if entry.bannedAt.IsZero() {
		return false
	}
	if s.audit != nil {
		s.audit.LogIPUnban(IPBanAudit{IP: ip, ActorSource: actorSource, ActorTokenID: actorTokenID})
	}
	return true
}

// List 返回全部 IP 封禁记录（含已到期未清理的——列表「已过期」态展示数据源，
// 拒连判定的懒过期语义以 RejectMessage 为准），按封禁时间降序（最新在前）。
// 仅含曾触发封禁的条目（纯失败计数不进列表）。返回切片为内部数据拷贝，调用方
// 修改不影响服务状态；空列表返回空切片（非 nil）。
func (s *IPBanService) List() []model.IPBanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowFn()
	out := make([]model.IPBanStatus, 0, len(s.entries))
	for _, e := range s.entries {
		if e.bannedAt.IsZero() {
			continue
		}
		out = append(out, model.IPBanStatus{
			IP:        e.ip,
			BannedAt:  e.bannedAt.Unix(),
			ExpiresAt: e.bannedUntil.Unix(),
			FailCount: e.lastFailCount,
			Expired:   !now.Before(e.bannedUntil),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].BannedAt > out[j].BannedAt })
	return out
}

// ClearAll 清空全部来源 IP 的失败计数与封禁记录（令牌轮换/撤销联动调用：轮换
// 产生的旧 cookie 失败潮防误伤叠加）。不写审计——联动清零为预防性动作非处置，
// 与撤销令牌联动清理封禁记录的既有口径一致。
func (s *IPBanService) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]*ipBanEntry)
}

// auditIPBanLocked 发 ip_ban 审计行（调用方须持 mu；audit 为 nil 时静默跳过）。
// actorSource 恒 system（自动触发无人类操作者），reason 文案携带触发时窗口内
// 失败计数（对齐四期「动作生效后才写审计」——封禁写入内存后调用）。
func (s *IPBanService) auditIPBanLocked(entry *ipBanEntry) {
	if s.audit == nil {
		return
	}
	s.audit.LogIPBan(IPBanAudit{
		IP:          entry.ip,
		FailCount:   entry.lastFailCount,
		ExpiresAt:   entry.bannedUntil.Unix(),
		Reason:      fmt.Sprintf("%d 分钟内认证失败 %d 次，自动临时封禁", int(IPBanWindow/time.Minute), entry.lastFailCount),
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
