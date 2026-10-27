package service

import (
	"fmt"
	"net"
	"strings"
	"time"

	"workbench/model"
)

// 连接管控八期：CIDR 网段手动封禁。七期手动封禁按单 IP 精确匹配，攻击源切换
// 同网段 IP 即绕过；本文件补网段维度处置面——管理员封禁整个网段（如
// 203.0.113.0/24），与白名单豁免（放行网段）形成对称能力。
//
// 独立列表 cidrBans（不混 entries map）是定稿决策：entries 的 failures/strikes
// 是单 IP 自动封禁专属语义（五/六期状态机），网段条目恒 manual 语义无计数面，
// 混存会污染遍历与快照。判定链白名单豁免恒最先（七期 isWhitelistedIP 单点
// 不动）→ entries 精确查 → cidrBans 线性扫 Contains，精确条目优先于网段条目
// 出消息（更具体优先）。

// ipBanCidrEntry 单条网段封禁条目（仅 IPBanService.mu 锁内可变）。net 为预解析
// 网段（BanCIDR/载入时一次解析，判定热路径零重复解析）；cidr 为规范化串
// （net.IPNet.String() 形态，持久化 IP 字段与 UnbanCIDR 匹配键同源）。恒 manual
// 语义：无 failures/strikes 字段（单 IP 自动封禁专属，网段条目不参与计数状态机）。
type ipBanCidrEntry struct {
	net          *net.IPNet
	cidr         string
	bannedAt     time.Time
	bannedUntil  time.Time // 零值 = 永久封禁（与 ipBanEntry.banActive 同款约定）
	reason       string
	actorSource  string
	actorTokenID string
}

// active 网段封禁是否生效中（判定谓词，语义对齐 ipBanEntry.banActive）：bannedAt
// 非零且未到期；bannedUntil 零值 = 永久恒生效。
func (e *ipBanCidrEntry) active(now time.Time) bool {
	if e.bannedAt.IsZero() {
		return false
	}
	if e.bannedUntil.IsZero() {
		return true
	}
	return now.Before(e.bannedUntil)
}

// BanCIDR 手动封禁指定网段（连接管控八期，签名与语义对齐七期 Ban）：cidr 接受
// CIDR 串或裸 IP（裸 IP 自动补 /32 //128 双栈，复用 parseIPBanWhitelistEntry
// 解析单点——与白名单同一套口径，禁第二套解析）；expiresAt 为到期 unix 秒
// （0 = 永久封禁须手动解封，非 0 须晚于当前时间）；reason 备注可选（空串生成
// 网段缺省文案）；actorSource/actorTokenID 为操作者归因。
//
// 校验拒绝（AppError 错误码）：cidr 非法返回 E_IP_BAN_CIDR_INVALID；到期时刻不
// 晚于当前返回 E_TOKEN_EXPIRY_INVALID（对齐 Ban）；网段被白名单完全覆盖返回
// E_IP_BAN_CIDR_WHITELISTED——完全覆盖 = 网段内不存在任何可封禁 IP，封禁零效果
// 故拒绝。判定语义：存在某白名单条目其掩码不长于目标网段（范围更大或相等）且
// 包含目标网段的网络地址，即整段落入该白名单条目（单条覆盖判定；多条白名单
// 拼片覆盖——如 /24 被 /25+/25 覆盖——不构成完全覆盖，放行：两条各覆盖一半，
// 另一半仍有封禁效果）。白名单与网段仅相交（部分豁免）合法：豁免优先级
//（isWhitelistedIP 恒最先）已保证交集内 IP 不受影响，其余地址照常拒连。
//
// 即时生效：写入 cidrBans，RejectMessage 网段扫描自动覆盖（两认证点零改动）。
// 多网段条目刻意允许重叠（任一命中即拒，不做包含合并——管理员心智模型简单，
// Out of Scope）；同网段重复封禁 = 刷新参数（精确匹配规范化 cidr 串覆盖原
// 条目）。落盘成功后才写 ip_ban 审计行（IP 字段存 CIDR 串，reason 为空时生成
// 网段缺省文案）；落盘失败回滚内存（摘除本次条目）返回错误不写审计；不触发
// onBanned（自动封禁专属通知面）。
func (s *IPBanService) BanCIDR(cidr string, expiresAt int64, reason, actorSource, actorTokenID string) error {
	cidr = strings.TrimSpace(cidr) // 手动入参用户可输，前后空白防御收敛（对齐 Ban）
	parsed, ok := parseIPBanWhitelistEntry(cidr)
	if !ok {
		return model.NewAppError(model.ErrCodeIPBanCidrInvalid,
			"网段格式非法，请输入合法 CIDR（如 203.0.113.0/24）或 IP 地址")
	}
	key := parsed.String() // 规范化形态（网络地址 + 掩码），存储与匹配单点
	reason = strings.TrimSpace(reason)

	s.mu.Lock()
	if cidrFullyCoveredByWhitelist(parsed, s.whitelistNets) {
		s.mu.Unlock()
		return model.NewAppError(model.ErrCodeIPBanCidrWhitelisted,
			"该网段完全被可信网段白名单覆盖（网段内全部来源均在豁免中），封禁无效果；如确需封禁请先在设置中移除对应白名单条目")
	}
	now := s.nowFn()
	if expiresAt != 0 && expiresAt <= now.Unix() {
		s.mu.Unlock()
		return model.NewAppError(model.ErrCodeTokenExpiryInvalid, "封禁到期时间必须晚于当前时间")
	}
	if reason == "" {
		reason = manualCidrBanDefaultReason(expiresAt)
	}
	entry := &ipBanCidrEntry{
		net:          parsed,
		cidr:         key,
		bannedAt:     now,
		bannedUntil:  time.Time{},
		reason:       reason,
		actorSource:  actorSource,
		actorTokenID: actorTokenID,
	}
	if expiresAt != 0 {
		entry.bannedUntil = time.Unix(expiresAt, 0)
	}
	// 同网段重复封禁 = 刷新参数（精确匹配规范化串覆盖原条目，记录唯一；不同
	// 网段条目并存不受影响）
	replaced := false
	var prev *ipBanCidrEntry
	for i, cur := range s.cidrBans {
		if cur.cidr == key {
			prev = cur
			s.cidrBans[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		s.cidrBans = append(s.cidrBans, entry)
	}
	s.manualVersion++
	audit := s.audit
	s.mu.Unlock()

	if err := s.persistManualBans(); err != nil {
		// 落盘失败回滚：摘除本次写入（替换态恢复被覆盖的原条目，新增态摘除
		// ——并发 UnbanCIDR 已摘除同键时不重建，删除态优先于本次未生效的封禁）
		s.mu.Lock()
		for i, cur := range s.cidrBans {
			if cur.cidr == key {
				if replaced {
					s.cidrBans[i] = prev
				} else {
					s.cidrBans = append(s.cidrBans[:i], s.cidrBans[i+1:]...)
				}
				break
			}
		}
		s.mu.Unlock()
		return fmt.Errorf("持久化手动封禁失败: %w", err)
	}
	if audit != nil {
		audit.LogIPBan(IPBanAudit{
			IP:           key,
			ExpiresAt:    expiresAt,
			Reason:       reason,
			ActorSource:  actorSource,
			ActorTokenID: actorTokenID,
		})
	}
	return nil
}

// UnbanCIDR 手动解封指定网段：规范化后按 cidr 串精确匹配摘除单条目（输入与
// 存储同走 parseIPBanWhitelistEntry + String() 归一，裸 IP 形态可解除其 /32
// //128 条目），返回是否实际解封。无匹配（含非法串——非法串不可能存入）为
// no-op 返回 false（幂等，对齐 UnbanIP 口径）。落盘失败回滚重插条目返回 false
//（解封未生效）；实际解除才写 ip_unban 审计行（IP 字段存 CIDR 串）。
func (s *IPBanService) UnbanCIDR(cidr, actorSource, actorTokenID string) bool {
	cidr = strings.TrimSpace(cidr)
	parsed, ok := parseIPBanWhitelistEntry(cidr)
	if !ok {
		return false
	}
	key := parsed.String()

	s.mu.Lock()
	idx := -1
	var removed *ipBanCidrEntry
	for i, cur := range s.cidrBans {
		if cur.cidr == key {
			idx = i
			removed = cur
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return false
	}
	s.cidrBans = append(s.cidrBans[:idx], s.cidrBans[idx+1:]...)
	audit := s.audit
	s.manualVersion++
	s.mu.Unlock()

	if err := s.persistManualBans(); err != nil {
		// 落盘失败回滚：重插条目（仅当键仍空缺——并发 BanCIDR 已重建时不覆盖
		// 新值），解封未生效不写审计
		s.mu.Lock()
		exists := false
		for _, cur := range s.cidrBans {
			if cur.cidr == key {
				exists = true
				break
			}
		}
		if !exists {
			s.cidrBans = append(s.cidrBans, removed)
		}
		s.mu.Unlock()
		return false
	}
	if audit != nil {
		audit.LogIPUnban(IPBanAudit{IP: key, ActorSource: actorSource, ActorTokenID: actorTokenID})
	}
	return true
}

// matchCidrBan 网段封禁判定（调用方须持 IPBanService.mu）：归一后 IP 命中任一
// 生效中网段条目即返回该条目，未命中/已过期/非法 IP 串返回 nil。多网段命中取
// 掩码最长（最具体）者——拒绝消息面向管理员展示最贴近攻击源的处理对象。供
// RejectMessage 与 List 的 CoveredBy 计算共用（口径同源，禁第二套 Contains 判定）。
func (s *IPBanService) matchCidrBan(ip string, now time.Time) *ipBanCidrEntry {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return nil
	}
	var best *ipBanCidrEntry
	bestOnes := -1
	for _, e := range s.cidrBans {
		if !e.active(now) || !e.net.Contains(parsed) {
			continue
		}
		ones, _ := e.net.Mask.Size()
		if ones > bestOnes {
			best = e
			bestOnes = ones
		}
	}
	return best
}

// cidrFullyCoveredByWhitelist 网段是否被白名单完全覆盖（BanCIDR 拒绝判定）：
// 存在某白名单条目掩码不长于目标网段（ones ≤，范围更大或相等）且包含目标网段
// 的网络地址，即整段落入该条目——网段内不存在任何可封禁 IP。仅相交（白名单
// 掩码更长，覆盖部分网段）返回 false：剩余地址仍有封禁效果，放行（豁免优先级
// 保证交集内 IP 不受影响）。跨地址族（IPv4 网段 vs IPv6 白名单条目等）掩码
// 位数不同天然不构成覆盖；Contains 对跨族地址恒 false（net 包语义）兜底。
// cidr 与 nets 均为已解析形态（调用方经 parseIPBanWhitelistEntry/ApplyConfig
// 预解析），掩码恒规范，Mask.Size() 不会返回 (0,0) 退化形态。
func cidrFullyCoveredByWhitelist(cidr *net.IPNet, nets []*net.IPNet) bool {
	cidrOnes, cidrBits := cidr.Mask.Size()
	for _, w := range nets {
		wOnes, wBits := w.Mask.Size()
		if wBits != cidrBits || wOnes > cidrOnes {
			continue
		}
		if w.Contains(cidr.IP) {
			return true
		}
	}
	return false
}

// manualCidrBanDefaultReason 网段手动封禁备注缺省文案（输入为空时生成，对齐
// manualBanDefaultReason 永久/定时分流；加「网段」二字与单 IP 封禁备注可区分
// ——审计行仅靠 IP 字段形态区分不够直观）。
func manualCidrBanDefaultReason(expiresAt int64) string {
	if expiresAt == 0 {
		return "管理员手动封禁网段（永久）"
	}
	return "管理员手动封禁网段，至 " + time.Unix(expiresAt, 0).Format(banExpiryMessageFormat)
}
