package service

import (
	"fmt"
	"strings"
	"time"

	"workbench/model"
	"workbench/util"
)

// 连接管控七期：IP 手动封禁（管理员主动处置面，补五期「只能等自动触发」缺口）
// 与手动条目持久化。手动与自动共用同一 entries map 与判定谓词（banActive 单点），
// server 两认证点 IPBanCheck 闭包零改动自动覆盖（写入即生效，无推送链路）；
// 自动封禁语义（五期状态机/六期参数化递增）不因本文件新增而分叉。

// ipBansFileName 手动封禁持久化文件名（与 web_bans.json 命名对齐；路径由
// SetBansPath 注入的目录推导）。
const ipBansFileName = "ip_bans.json"

// ipBanRollback Ban 落盘失败回滚的字段级快照：只覆盖 Ban 写过的四个封禁字段，
// 不动 failures/strikes（并发 RecordFailure 在落盘窗口内的计数变更不被回滚
// 覆盖——递增记忆与失败计数非本次动作的写入面）。
type ipBanRollback struct {
	exists        bool
	source        string
	bannedAt      time.Time
	bannedUntil   time.Time
	lastFailCount int
	reason        string
	actorSource   string
	actorTokenID  string
}

// SetBansPath 注入手动封禁持久化文件路径（data/ip_bans.json；App 装配层构造后
// 注入一次，须在首个 Ban/UnbanIP 之前完成）并即时载入存量手动条目（仅未过期
// 条目，见 loadManualBansLocked）。未注入（bansPath 空）为纯内存模式：持久化
// no-op，语义与五期一致，既有构造路径 NewIPBanService(nil) 零行为变化。
func (s *IPBanService) SetBansPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bansPath = path
	s.loadManualBansLocked()
}

// Ban 手动封禁指定来源 IP（连接管控七期，对齐四期 WebTokenService.Ban 形态）：
// expiresAt 为到期 unix 秒（0 = 永久封禁须手动解封；非 0 须晚于当前时间），
// reason 备注可选（空串生成缺省文案「管理员手动封禁（永久）」/「管理员手动
// 封禁，至 <时刻>」），actorSource/actorTokenID 为操作者归因（桌面 desktop/空，
// 浏览器 RPC 经 rpcAuditTarget 装饰捕获后 rpc/调用者令牌记录 ID）。
//
// 即时生效：写入与自动封禁同一 entries map，两认证点 IPBanCheck 判定闭包零改动
// 自动覆盖。覆盖语义（对齐四期「重复封禁 = 刷新参数」）：封禁生效中的同名条目
// （自动或手动）整体覆盖 bannedAt/bannedUntil/reason/归因并置 source=manual
// （改时长/改永久均经此路径，记录恒唯一）；failures/strikes 刻意不动——管理员
// 决策非攻击信号不参与递增（strikes 保留 = 曾有自动史的条目解封后仍按档位
// 递增），失败计数与封禁状态正交（自动遇手动在途：RecordFailure 封禁中早退
// 天然不累计，剪枝 cutoff 通用无分叉）。
//
// 白名单/回环命中拒绝（E_IP_BAN_WHITELISTED，豁免面与封禁动作互斥）；IP 空串
// 拒绝（E_IP_BAN_IP_REQUIRED）；过期时刻拒绝（E_TOKEN_EXPIRY_INVALID，对齐
// WebTokenService.Ban 校验形态）。落盘成功（含写内存）后才写 ip_ban 审计行
// （FailCount/Strike 零值不落键，reason 为空时生成缺省文案）；落盘失败回滚
// 内存（字段级恢复）返回错误不写审计；不触发 onBanned（自动封禁专属通知面，
// 前端对手动封禁经 Promise 返回后自行刷新）。
func (s *IPBanService) Ban(ip string, expiresAt int64, reason, actorSource, actorTokenID string) error {
	ip = strings.TrimSpace(ip) // 手动入参用户可输，前后空白防御收敛（RemoteAddr 自动路径不受影响）
	ip = normalizeIP(ip)
	if ip == "" {
		return model.NewAppError(model.ErrCodeIPBanIPRequired, "请填写要封禁的来源 IP")
	}
	reason = strings.TrimSpace(reason)

	s.mu.Lock()
	if isWhitelistedIP(ip, s.whitelistNets) {
		s.mu.Unlock()
		return model.NewAppError(model.ErrCodeIPBanWhitelisted,
			"该 IP 在可信网段白名单中（含本机回环地址），禁止封禁；如确需封禁请先在设置中移除对应白名单条目")
	}
	now := s.nowFn()
	if expiresAt != 0 && expiresAt <= now.Unix() {
		s.mu.Unlock()
		return model.NewAppError(model.ErrCodeTokenExpiryInvalid, "封禁到期时间必须晚于当前时间")
	}
	entry, ok := s.entries[ip]
	if !ok {
		entry = &ipBanEntry{ip: ip}
		s.entries[ip] = entry
	}
	rollback := ipBanRollback{
		exists:        ok,
		source:        entry.source,
		bannedAt:      entry.bannedAt,
		bannedUntil:   entry.bannedUntil,
		lastFailCount: entry.lastFailCount,
		reason:        entry.reason,
		actorSource:   entry.actorSource,
		actorTokenID:  entry.actorTokenID,
	}
	if reason == "" {
		reason = manualBanDefaultReason(expiresAt)
	}
	entry.source = model.IPBanSourceManual
	entry.bannedAt = now
	if expiresAt == 0 {
		entry.bannedUntil = time.Time{} // 永久封禁：零值 = 无到期时刻（banActive 恒生效）
	} else {
		entry.bannedUntil = time.Unix(expiresAt, 0)
	}
	entry.lastFailCount = 0 // 手动封禁无自动触发语境，冻结计数清零（审计行 omitempty 不落键）
	entry.reason = reason
	entry.actorSource = actorSource
	entry.actorTokenID = actorTokenID
	s.manualVersion++
	audit := s.audit
	s.mu.Unlock()

	if err := s.persistManualBans(); err != nil {
		// 落盘失败回滚：字段级恢复 Ban 写过的封禁字段（条目被并发 UnbanIP 删除
		// 时不重建——删除态优先于本次未生效的封禁）
		s.mu.Lock()
		if cur, exists := s.entries[ip]; exists {
			cur.source = rollback.source
			cur.bannedAt = rollback.bannedAt
			cur.bannedUntil = rollback.bannedUntil
			cur.lastFailCount = rollback.lastFailCount
			cur.reason = rollback.reason
			cur.actorSource = rollback.actorSource
			cur.actorTokenID = rollback.actorTokenID
		}
		s.mu.Unlock()
		return fmt.Errorf("持久化手动封禁失败: %w", err)
	}
	if audit != nil {
		audit.LogIPBan(IPBanAudit{
			IP:           ip,
			ExpiresAt:    expiresAt,
			Reason:       reason,
			ActorSource:  actorSource,
			ActorTokenID: actorTokenID,
		})
	}
	return nil
}

// manualBanDefaultReason 手动封禁备注缺省文案（输入为空时生成）：永久与定时
// 分流（定时含解封时刻，复用 banExpiryMessageFormat 分钟精度格式，与拒绝消息
// 中的时刻呈现一致）。
func manualBanDefaultReason(expiresAt int64) string {
	if expiresAt == 0 {
		return "管理员手动封禁（永久）"
	}
	return "管理员手动封禁，至 " + time.Unix(expiresAt, 0).Format(banExpiryMessageFormat)
}

// loadManualBansLocked 载入手动封禁记录（调用方须持 mu；SetBansPath 专属启动
// 路径，生产时序在构造后立即执行、entries 尚无在途状态）。文件缺失（尚未使用
// 过手动封禁）为常态跳过；损坏降级空列表不阻塞（对齐 web_bans.json 四期先例
// ——封禁为管控增强面，宁放行不可阻断启动）。仅载入未过期条目：已过期条目
// 载入时丢弃（文件有界自洁，「已过期」态仅运行期内可见，懒过期语义不变）。
// upsert 语义：覆盖同名条目的封禁字段并整体重置 failures/strikes（重启即显式
// 重置面，对齐六期「重启清零」递增记忆纪律）；仅 manual 条目进文件，载入恒置
// source=manual。
//
// 条目形态分流（八期）：Kind=IPBanKindCidr 走网段条目载入（IP 字段存 CIDR 串，
// parseIPBanWhitelistEntry 解析失败 Warn 跳过不入列）；Kind 空/其余值一律走
// 既有单 IP 路径——v1 文件无 kind 字段反序列化后 Kind 恒为空串，天然落单 IP
// 分支，v1/v2 双版零特判兼容。
func (s *IPBanService) loadManualBansLocked() {
	if s.bansPath == "" {
		return
	}
	list := model.IPBanList{Version: model.IPBanListVersion}
	if util.FileExists(s.bansPath) {
		if err := util.LoadJSON(s.bansPath, &list); err != nil {
			Logger().Warn("ip bans file corrupt, starting from empty list",
				"path", s.bansPath, "err", err)
			list = model.IPBanList{Version: model.IPBanListVersion}
		}
	}
	now := s.nowFn()
	for _, rec := range list.Bans {
		if rec.IP == "" {
			continue
		}
		if rec.ExpiresAt != 0 && !time.Unix(rec.ExpiresAt, 0).After(now) {
			continue // 已过期：载入时丢弃（懒过期放行，文件有界自洁）
		}
		if rec.Kind == model.IPBanKindCidr {
			parsed, ok := parseIPBanWhitelistEntry(rec.IP)
			if !ok {
				Logger().Warn("ip bans file has invalid cidr record, skipping",
					"path", s.bansPath, "cidr", rec.IP)
				continue
			}
			s.cidrBans = append(s.cidrBans, &ipBanCidrEntry{
				net:          parsed,
				cidr:         parsed.String(),
				bannedAt:     time.Unix(rec.BannedAt, 0),
				reason:       rec.Reason,
				actorSource:  rec.ActorSource,
				actorTokenID: rec.ActorTokenID,
			})
			// bannedUntil 恒最后赋值（永久条目保持零值语义）
			if rec.ExpiresAt != 0 {
				s.cidrBans[len(s.cidrBans)-1].bannedUntil = time.Unix(rec.ExpiresAt, 0)
			}
			continue
		}
		entry, ok := s.entries[rec.IP]
		if !ok {
			entry = &ipBanEntry{ip: rec.IP}
			s.entries[rec.IP] = entry
		}
		entry.source = model.IPBanSourceManual
		entry.bannedAt = time.Unix(rec.BannedAt, 0)
		if rec.ExpiresAt == 0 {
			entry.bannedUntil = time.Time{}
		} else {
			entry.bannedUntil = time.Unix(rec.ExpiresAt, 0)
		}
		entry.lastFailCount = 0
		entry.failures = nil
		entry.strikes = 0
		entry.reason = rec.Reason
		entry.actorSource = rec.ActorSource
		entry.actorTokenID = rec.ActorTokenID
	}
	s.manualVersion++
}

// snapshotManualBansLocked 收集手动条目持久化快照与当前代际（调用方须持 mu）。
// 仅 source=manual 且有封禁状态的条目进快照（自动封禁/失败计数纯内存，重启
// 清零语义不变）；永久封禁 ExpiresAt 落 0。八期扩网段条目（Kind=IPBanKindCidr，
// IP 字段存规范化 CIDR 串——网段条目恒 manual 语义全量进快照）。
func (s *IPBanService) snapshotManualBansLocked() (model.IPBanList, int) {
	list := model.IPBanList{Version: model.IPBanListVersion, Bans: []model.IPBanRecord{}}
	for _, e := range s.entries {
		if e.source != model.IPBanSourceManual || e.bannedAt.IsZero() {
			continue
		}
		rec := model.IPBanRecord{
			IP:           e.ip,
			BannedAt:     e.bannedAt.Unix(),
			Reason:       e.reason,
			ActorSource:  e.actorSource,
			ActorTokenID: e.actorTokenID,
		}
		if !e.bannedUntil.IsZero() {
			rec.ExpiresAt = e.bannedUntil.Unix()
		}
		list.Bans = append(list.Bans, rec)
	}
	for _, e := range s.cidrBans {
		rec := model.IPBanRecord{
			IP:           e.cidr,
			Kind:         model.IPBanKindCidr,
			BannedAt:     e.bannedAt.Unix(),
			Reason:       e.reason,
			ActorSource:  e.actorSource,
			ActorTokenID: e.actorTokenID,
		}
		if !e.bannedUntil.IsZero() {
			rec.ExpiresAt = e.bannedUntil.Unix()
		}
		list.Bans = append(list.Bans, rec)
	}
	return list, s.manualVersion
}

// persistManualBans 持久化手动封禁快照（Ban/UnbanIP 落盘单点；锁外 IO——认证
// 失败热路径与判定读路径共用的 mu 不被磁盘写延迟拖住，对齐「锁内收集快照、
// 锁外落盘」纪律）。
//
// 并发一致性方案（快照带版本复核，重写收敛）：快照与 manualVersion 在同一 mu
// 临界区收集，落盘完成后重新持锁复核版本——版本未变则文件即当前态的忠实快照；
// 版本已变（落盘窗口内有并发 Ban/UnbanIP/ClearAll，管理面低频操作）则以新快照
// 重写直至收敛（终止性：每轮至少完成一次完整 IO，并发变更为有限次，管理面
// 操作频率下重试轮数有界）。落盘失败（磁盘/权限类错误）向调用方上抛，由其
// 决定内存回滚（Ban 字段级恢复 / UnbanIP 重插条目）——文件保持旧态与内存回滚
// 后的状态一致。未注入 bansPath（纯内存模式）恒 no-op 返回 nil。
func (s *IPBanService) persistManualBans() error {
	if s.bansPath == "" {
		return nil
	}
	for {
		s.mu.Lock()
		list, version := s.snapshotManualBansLocked()
		s.mu.Unlock()
		if err := util.SaveJSON(s.bansPath, list); err != nil {
			return err
		}
		s.mu.Lock()
		unchanged := s.manualVersion == version
		s.mu.Unlock()
		if unchanged {
			return nil
		}
		// 落盘窗口内有并发变更：以新快照重写（下一轮）
	}
}
