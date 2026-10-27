package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"workbench/model"
	"workbench/util"
)

// 连接管控八期测试：CIDR 网段手动封禁（BanCIDR/UnbanCIDR 状态机、判定链优先级、
// 持久化双版兼容、CoveredBy 覆盖标注与 List 排序、自动封禁零回归）。时钟全部经
// newTestIPBanService 注入时间源 + advanceIPBanClock 推进（禁 sleep）；持久化
// 用例走 t.TempDir。

// TestIPBanService_CidrBanImmediateEffect 网段封禁即时生效：网段内任一 IP（含
// 首末地址）拒连，临时档消息含网段串与自动恢复时刻；List 条目 Kind=cidr、
// Source=manual、IP 为规范化 CIDR 串、FailCount/Strike 恒零。
func TestIPBanService_CidrBanImmediateEffect(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	expiresAt := now().Add(30 * time.Minute).Unix()

	if svc.RejectMessage("203.0.113.10") != "" {
		t.Fatal("封禁前应放行")
	}
	if err := svc.BanCIDR("203.0.113.0/24", expiresAt, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR: %v", err)
	}

	wantMsg := "该来源 IP 已被网段临时封禁，至 " + time.Unix(expiresAt, 0).Format(banExpiryMessageFormat) + " 自动恢复"
	for _, ip := range []string{"203.0.113.0", "203.0.113.10", "203.0.113.255"} {
		if got := svc.RejectMessage(ip); got != wantMsg {
			t.Errorf("网段内 IP %s 应拒绝且消息为 %q, got %q", ip, wantMsg, got)
		}
	}
	if got := svc.RejectMessage("203.0.114.10"); got != "" {
		t.Errorf("相邻网段 IP 不应命中, got %q", got)
	}
	bans := svc.List()
	if len(bans) != 1 {
		t.Fatalf("应恰有 1 条网段记录, got %+v", bans)
	}
	ban := bans[0]
	if ban.Kind != model.IPBanKindCidr || ban.IP != "203.0.113.0/24" {
		t.Errorf("网段条目应为 cidr/%s, got %s/%s", model.IPBanKindCidr, ban.Kind, ban.IP)
	}
	if ban.Source != model.IPBanSourceManual {
		t.Errorf("网段条目 source 应为 manual, got %q", ban.Source)
	}
	if ban.FailCount != 0 || ban.Strike != 0 {
		t.Errorf("网段条目无自动触发语境, FailCount/Strike 应为 0, got %d/%d", ban.FailCount, ban.Strike)
	}
	if ban.ExpiresAt != expiresAt || ban.Expired {
		t.Errorf("ExpiresAt 应为 %d 且未过期, got %d/%v", expiresAt, ban.ExpiresAt, ban.Expired)
	}
}

// TestIPBanService_CidrBanPermanent 永久网段封禁（expiresAt=0）：消息含网段串与
// 「永久」标识；时间大幅推进后仍拒绝（仅 UnbanCIDR 可解除）。
func TestIPBanService_CidrBanPermanent(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	if err := svc.BanCIDR("203.0.113.0/24", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR: %v", err)
	}
	msg := svc.RejectMessage("203.0.113.42")
	if !strings.Contains(msg, "203.0.113.0/24") || !strings.Contains(msg, "永久") {
		t.Errorf("永久网段封禁消息应含网段串与「永久」, got %q", msg)
	}
	advanceIPBanClock(t, svc, newAdvancedClockFn(now().AddDate(10, 0, 0)), 0)
	if svc.RejectMessage("203.0.113.42") == "" {
		t.Error("永久网段封禁不应随时间过期")
	}
}

// TestIPBanService_CidrRejectMessagePrecisionFirst 判定链优先级：精确条目与网段
// 条目并存时消息取精确条目（更具体优先）；白名单豁免恒最先（网段封禁内白名单
// IP 放行）；网段封禁内回环 IP 放行（回环豁免收编单点）。
func TestIPBanService_CidrRejectMessagePrecisionFirst(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"203.0.113.7"},
	}))
	expiresAt := now().Add(time.Hour).Unix()
	if err := svc.BanCIDR("203.0.113.0/24", expiresAt, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR: %v", err)
	}
	if err := svc.Ban("203.0.113.5", expiresAt, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban 精确条目: %v", err)
	}

	// 并存时精确条目优先：消息为单 IP 文案（非网段文案）
	wantExact := "该来源 IP 已被临时封禁，至 " + time.Unix(expiresAt, 0).Format(banExpiryMessageFormat) + " 自动恢复"
	if got := svc.RejectMessage("203.0.113.5"); got != wantExact {
		t.Errorf("精确条目与网段并存时消息应取精确条目 %q, got %q", wantExact, got)
	}
	// 仅网段命中：网段文案（临时档对齐七期格式带自动恢复时刻，网段串仅永久档携带）
	got := svc.RejectMessage("203.0.113.6")
	if !strings.Contains(got, "网段") || !strings.Contains(got, "自动恢复") {
		t.Errorf("仅网段命中应出网段临时档文案, got %q", got)
	}
	// 白名单豁免恒最先：网段封禁内白名单 IP 放行
	if got := svc.RejectMessage("203.0.113.7"); got != "" {
		t.Errorf("白名单 IP 不受网段封禁影响, got %q", got)
	}
	// 回环豁免：0.0.0.0/0 应急锁形态下回环仍放行
	if err := svc.BanCIDR("0.0.0.0/0", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("0.0.0.0/0 应急锁应允许: %v", err)
	}
	if got := svc.RejectMessage("127.0.0.1"); got != "" {
		t.Errorf("回环地址应恒豁免, got %q", got)
	}
}

// TestIPBanService_CidrBanValidation BanCIDR 校验面：非法网段拒绝
// （E_IP_BAN_CIDR_INVALID）、裸 IPv4 自动补 /32、裸 IPv6 自动补 /128、到期时刻
// 不晚于当前拒绝（E_TOKEN_EXPIRY_INVALID）；校验拒绝不产生任何条目。
func TestIPBanService_CidrBanValidation(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)

	requireAppErr(t, svc.BanCIDR("not-a-cidr", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanCidrInvalid)
	requireAppErr(t, svc.BanCIDR("203.0.113.0/99", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanCidrInvalid)
	requireAppErr(t, svc.BanCIDR("", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanCidrInvalid)
	requireAppErr(t, svc.BanCIDR("   ", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanCidrInvalid)
	requireAppErr(t, svc.BanCIDR("203.0.113.80", now().Unix()-1, "", AuditActorDesktop, ""), model.ErrCodeTokenExpiryInvalid)
	requireAppErr(t, svc.BanCIDR("203.0.113.80", now().Unix(), "", AuditActorDesktop, ""), model.ErrCodeTokenExpiryInvalid)
	if bans := svc.List(); len(bans) != 0 {
		t.Fatalf("校验拒绝不应产生条目, got %+v", bans)
	}

	// 裸 IPv4 补 /32：封禁生效且条目键为规范化 /32 形态
	if err := svc.BanCIDR("203.0.113.80", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("裸 IPv4 封禁: %v", err)
	}
	if svc.RejectMessage("203.0.113.80") == "" {
		t.Error("裸 IPv4 封禁应生效于该 IP")
	}
	if svc.RejectMessage("203.0.113.81") != "" {
		t.Error("裸 IPv4 补 /32 不应波及相邻 IP")
	}
	if ban, ok := banStatusByIP(svc.List(), "203.0.113.80/32"); !ok || ban.Kind != model.IPBanKindCidr {
		t.Errorf("裸 IPv4 应存为 /32 网段条目, got %+v", ban)
	}

	// 裸 IPv6 补 /128
	if err := svc.BanCIDR("2001:db8::9", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("裸 IPv6 封禁: %v", err)
	}
	if svc.RejectMessage("2001:db8::9") == "" {
		t.Error("裸 IPv6 封禁应生效于该 IP")
	}
	if svc.RejectMessage("2001:db8::a") != "" {
		t.Error("裸 IPv6 补 /128 不应波及相邻 IP")
	}
	if ban, ok := banStatusByIP(svc.List(), "2001:db8::9/128"); !ok {
		t.Errorf("裸 IPv6 应存为 /128 网段条目, got %+v", ban)
	}
}

// TestIPBanService_CidrBanWhitelistInteraction 白名单覆盖判定矩阵：白名单掩码
// 不长于目标网段且包含其网络地址 = 完全覆盖拒绝（E_IP_BAN_CIDR_WHITELISTED，
// 同掩码等范围、更宽白名单、裸 IP 白名单覆盖单 IP 网段）；相交不构成完全覆盖
// （白名单掩码更长、多条白名单拼片覆盖、跨地址族）放行；白名单移除后可封
// （豁免态可逆）。
func TestIPBanService_CidrBanWhitelistInteraction(t *testing.T) {
	svc, _, _ := newTestIPBanServiceWithBans(t)

	// 白名单 203.0.113.0/24：同掩码目标完全覆盖拒绝；相交（/25 白名单更长）放行
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"203.0.113.0/24"},
	}))
	requireAppErr(t, svc.BanCIDR("203.0.113.0/24", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanCidrWhitelisted)
	// 更宽白名单覆盖：/16 白名单覆盖 /24 目标
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"203.0.0.0/16"},
	}))
	requireAppErr(t, svc.BanCIDR("203.0.113.0/24", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanCidrWhitelisted)
	// 裸 IP 白名单覆盖单 IP 网段（/32 vs 裸 IP 补 /32）
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"203.0.113.77"},
	}))
	requireAppErr(t, svc.BanCIDR("203.0.113.77", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanCidrWhitelisted)
	// 相交放行：白名单掩码更长（/25 只覆盖目标前半），后半仍有封禁效果
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"203.0.113.0/25"},
	}))
	if err := svc.BanCIDR("203.0.113.0/24", 0, "", AuditActorDesktop, ""); err != nil {
		t.Errorf("白名单相交（掩码更长）应放行, got %v", err)
	}
	// 前半段（白名单内）豁免放行；后半段（非白名单）照常拒连——豁免优先级保证
	if got := svc.RejectMessage("203.0.113.1"); got != "" {
		t.Errorf("白名单相交态前半段应豁免放行, got %q", got)
	}
	if svc.RejectMessage("203.0.113.200") == "" {
		t.Error("白名单相交态后半段（非白名单）应被网段封禁拒绝")
	}
	// 多条白名单拼片覆盖不构成完全覆盖（单条覆盖判定：任一条目掩码均长于目标）
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"198.51.100.0/25", "198.51.100.128/25"},
	}))
	if err := svc.BanCIDR("198.51.100.0/24", 0, "", AuditActorDesktop, ""); err != nil {
		t.Errorf("拼片覆盖应放行（两半合计覆盖，单条不构成完全覆盖）, got %v", err)
	}
	// 跨地址族不构成覆盖：IPv6 白名单与 IPv4 目标互不相干
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"2001:db8::/32"},
	}))
	if err := svc.BanCIDR("203.0.113.0/24", 0, "", AuditActorDesktop, ""); err != nil {
		t.Errorf("跨地址族不应构成覆盖, got %v", err)
	}
	// IPv6 完全覆盖与相交
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"2001:db8::/16"},
	}))
	requireAppErr(t, svc.BanCIDR("2001:db8::/32", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanCidrWhitelisted)
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"2001:db8::/48"},
	}))
	if err := svc.BanCIDR("2001:db8::/32", 0, "", AuditActorDesktop, ""); err != nil {
		t.Errorf("IPv6 白名单相交应放行, got %v", err)
	}

	// 白名单清空后同网段可封（豁免态可逆）；拒绝形态不产生网段条目
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
	}))
	if err := svc.BanCIDR("203.0.113.0/24", 0, "", AuditActorDesktop, ""); err != nil {
		t.Errorf("白名单移除后应可封禁, got %v", err)
	}
}

// TestIPBanService_CidrUnban 网段解封：规范化后精确匹配摘除（裸 IP 形态可解除
// 其 /32 条目）；无匹配（未封禁网段/非法串/不同网段）no-op 返回 false；解封后
// 网段内 IP 可达；重复解封幂等。
func TestIPBanService_CidrUnban(t *testing.T) {
	svc, _, _ := newTestIPBanServiceWithBans(t)
	if err := svc.BanCIDR("203.0.113.0/24", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR: %v", err)
	}

	if !svc.UnbanCIDR("203.0.113.0/24", AuditActorDesktop, "") {
		t.Fatal("规范化同串解封应成功")
	}
	if got := svc.RejectMessage("203.0.113.50"); got != "" {
		t.Errorf("解封后网段内 IP 应可达, got %q", got)
	}
	if svc.UnbanCIDR("203.0.113.0/24", AuditActorDesktop, "") {
		t.Error("重复解封应幂等返回 false")
	}

	// 裸 IP 形态解除 /32 条目（归一单点）
	if err := svc.BanCIDR("198.51.100.9", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR 裸 IP: %v", err)
	}
	if !svc.UnbanCIDR("198.51.100.9", AuditActorDesktop, "") {
		t.Error("裸 IP 解封应匹配其 /32 条目")
	}
	if svc.RejectMessage("198.51.100.9") != "" {
		t.Error("解封后应可达")
	}

	// miss 形态：未封禁网段 / 非法串 / 不同网段
	if svc.UnbanCIDR("192.0.2.0/24", AuditActorDesktop, "") {
		t.Error("未封禁网段解封应返回 false")
	}
	if svc.UnbanCIDR("not-a-cidr", AuditActorDesktop, "") {
		t.Error("非法串解封应返回 false")
	}
	if err := svc.BanCIDR("10.0.0.0/8", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR: %v", err)
	}
	if svc.UnbanCIDR("10.1.0.0/16", AuditActorDesktop, "") {
		t.Error("不同网段解封应返回 false")
	}
	if svc.RejectMessage("10.2.3.4") == "" {
		t.Error("未匹配解封不应影响在途网段封禁")
	}
}

// TestIPBanService_CidrBanAuditAndDefaultReason 网段封禁审计行形态（ip_ban
// 单事件复用）：IP 字段存规范化 CIDR 串、reason 空入参生成网段缺省文案、actor
// 归因透传；不触发 onBanned；解封审计行 IP 字段存 CIDR 串。
func TestIPBanService_CidrBanAuditAndDefaultReason(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	audit := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	svc.SetAudit(audit)
	notifyCh := make(chan model.IPBanStatus, 1)
	svc.SetOnBanned(func(status model.IPBanStatus) { notifyCh <- status })

	timedExpiry := now().Add(time.Hour).Unix()
	if err := svc.BanCIDR("203.0.113.0/24", timedExpiry, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("桌面定时网段封禁: %v", err)
	}
	if err := svc.BanCIDR("198.51.100.0/24", 0, "机房处置", AuditActorRPC, "tok-8"); err != nil {
		t.Fatalf("RPC 永久网段封禁: %v", err)
	}
	if err := svc.BanCIDR("not-a-cidr", 0, "", AuditActorDesktop, ""); err == nil {
		t.Fatal("非法网段应拒绝")
	}
	if !svc.UnbanCIDR("198.51.100.0/24", AuditActorRPC, "tok-8") {
		t.Fatal("解封应成功")
	}

	rows := readAuditLines(t, audit.path)
	var banRows, unbanRows []AuditRecord
	for i := range rows {
		switch rows[i].Event {
		case AuditEventIPBan:
			banRows = append(banRows, rows[i])
		case AuditEventIPUnban:
			unbanRows = append(unbanRows, rows[i])
		}
	}
	if len(banRows) != 2 {
		t.Fatalf("应恰有 2 条 ip_ban 审计行, got %d", len(banRows))
	}
	desktop, rpc := banRows[0], banRows[1]
	if desktop.IP != "203.0.113.0/24" {
		t.Errorf("审计行 IP 字段应存规范化 CIDR 串, got %q", desktop.IP)
	}
	if desktop.Reason != "管理员手动封禁网段，至 "+time.Unix(timedExpiry, 0).Format(banExpiryMessageFormat) {
		t.Errorf("定时封禁空备注应生成网段缺省文案, got %q", desktop.Reason)
	}
	if rpc.Reason != "机房处置" || rpc.ActorSource != AuditActorRPC || rpc.ActorTokenID != "tok-8" {
		t.Errorf("非空备注应保留原文且归因透传, got %+v", rpc)
	}
	if len(unbanRows) != 1 || unbanRows[0].IP != "198.51.100.0/24" || unbanRows[0].ActorSource != AuditActorRPC {
		t.Errorf("解封审计行应存 CIDR 串且归因 rpc, got %+v", unbanRows)
	}
	select {
	case status := <-notifyCh:
		t.Errorf("网段封禁不应触发 onBanned 自动通知, got %+v", status)
	default:
	}
}

// TestIPBanService_CidrBanOverwriteRefresh 同网段重复封禁 = 刷新参数：条目唯一、
// 时间戳/备注/归因整体覆盖；不同网段条目并存不受影响。
func TestIPBanService_CidrBanOverwriteRefresh(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	if err := svc.BanCIDR("203.0.113.0/24", now().Add(time.Hour).Unix(), "第一次", AuditActorDesktop, ""); err != nil {
		t.Fatalf("第一次 BanCIDR: %v", err)
	}
	secondExpiry := now().Add(2 * time.Hour).Unix()
	if err := svc.BanCIDR("203.0.113.0/24", secondExpiry, "第二次", AuditActorRPC, "tok-2"); err != nil {
		t.Fatalf("第二次 BanCIDR: %v", err)
	}
	if err := svc.BanCIDR("198.51.100.0/24", 0, "并存网段", AuditActorDesktop, ""); err != nil {
		t.Fatalf("另一网段 BanCIDR: %v", err)
	}
	bans := svc.List()
	if len(bans) != 2 {
		t.Fatalf("应恰有 2 条网段记录, got %+v", bans)
	}
	ban, ok := banStatusByIP(bans, "203.0.113.0/24")
	if !ok {
		t.Fatal("同网段重复封禁记录应唯一")
	}
	if ban.ExpiresAt != secondExpiry {
		t.Errorf("重复封禁应刷新到期时刻为 %d, got %d", secondExpiry, ban.ExpiresAt)
	}
	svc.mu.Lock()
	var stored *ipBanCidrEntry
	for _, e := range svc.cidrBans {
		if e.cidr == "203.0.113.0/24" {
			stored = e
		}
	}
	svc.mu.Unlock()
	if stored == nil || stored.reason != "第二次" || stored.actorSource != AuditActorRPC || stored.actorTokenID != "tok-2" {
		t.Errorf("重复封禁应整体覆盖备注与归因, got %+v", stored)
	}
}

// TestIPBanService_CidrPersistRoundTrip 网段封禁持久化往返：落盘后新服务同路径
// 载入——网段封禁复现（消息含网段串）、单 IP 封禁复现、自动封禁不跨重启复活。
func TestIPBanService_CidrPersistRoundTrip(t *testing.T) {
	svc, now, path := newTestIPBanServiceWithBans(t)
	timedExpiry := now().Add(2 * time.Hour).Unix()
	if err := svc.BanCIDR("203.0.113.0/24", timedExpiry, "网段处置", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR 定时: %v", err)
	}
	if err := svc.BanCIDR("198.51.100.0/24", 0, "", AuditActorRPC, "tok-4"); err != nil {
		t.Fatalf("BanCIDR 永久: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读持久化文件: %v", err)
	}
	var stored model.IPBanList
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("文件应为合法 IPBanList JSON: %v", err)
	}
	if stored.Version != model.IPBanListVersion {
		t.Errorf("文件版本应为 %d, got %d", model.IPBanListVersion, stored.Version)
	}
	cidrCount := 0
	for _, rec := range stored.Bans {
		if rec.Kind == model.IPBanKindCidr {
			cidrCount++
			if rec.IP != "203.0.113.0/24" && rec.IP != "198.51.100.0/24" {
				t.Errorf("网段记录 IP 字段应存 CIDR 串, got %q", rec.IP)
			}
		}
	}
	if cidrCount != 2 {
		t.Fatalf("文件应含 2 条 kind=cidr 记录, got %d (%s)", cidrCount, raw)
	}

	reloaded := NewIPBanService(func() time.Time { return now() })
	reloaded.SetBansPath(path)
	msg := reloaded.RejectMessage("203.0.113.66")
	if !strings.Contains(msg, "网段") || !strings.Contains(msg, "自动恢复") {
		t.Errorf("载入后网段封禁应生效（临时档文案）, got %q", msg)
	}
	if msg := reloaded.RejectMessage("198.51.100.5"); !strings.Contains(msg, "永久") {
		t.Errorf("载入后永久网段封禁应生效, got %q", msg)
	}
	ban, ok := banStatusByIP(reloaded.List(), "198.51.100.0/24")
	if !ok || ban.Kind != model.IPBanKindCidr || ban.Source != model.IPBanSourceManual {
		t.Fatalf("载入网段条目应为 cidr/manual, got %+v", ban)
	}
}

// TestIPBanService_CidrLoadLegacyAndInvalid v1 文件兼容与坏条目防御：无 kind
// 字段的历史记录（v1 形态）载入为单 IP 条目照常拒连；kind=cidr 但 CIDR 串非法
// 的条目 Warn 跳过不入列；已过期网段条目载入时丢弃。
func TestIPBanService_CidrLoadLegacyAndInvalid(t *testing.T) {
	now := time.Unix(1_700_000_100, 0)
	path := filepath.Join(t.TempDir(), "ip_bans.json")
	list := model.IPBanList{
		Version: 1, // v1 形态：无 kind 字段
		Bans: []model.IPBanRecord{
			{IP: "192.0.2.88", BannedAt: now.Add(-time.Hour).Unix(), ExpiresAt: 0, Reason: "v1 单 IP 条目"},
			{IP: "203.0.113.0/24", Kind: model.IPBanKindCidr, BannedAt: now.Add(-time.Hour).Unix(), ExpiresAt: now.Add(time.Hour).Unix()},
			{IP: "203.0.114.0/24", Kind: model.IPBanKindCidr, BannedAt: now.Add(-2 * time.Hour).Unix(), ExpiresAt: now.Add(-time.Hour).Unix()}, // 已过期：丢弃
			{IP: "not-a-cidr", Kind: model.IPBanKindCidr, BannedAt: now.Add(-time.Hour).Unix(), ExpiresAt: 0},                                // 非法串：跳过
		},
	}
	if err := util.SaveJSON(path, list); err != nil {
		t.Fatalf("写 fixture: %v", err)
	}
	svc := NewIPBanService(func() time.Time { return now })
	svc.SetBansPath(path)

	if msg := svc.RejectMessage("192.0.2.88"); !strings.Contains(msg, "永久") {
		t.Errorf("v1 无 kind 记录应按单 IP 载入并拒连, got %q", msg)
	}
	legacy, ok := banStatusByIP(svc.List(), "192.0.2.88")
	if !ok || legacy.Kind != model.IPBanKindIP {
		t.Errorf("v1 记录条目形态应为 ip, got %+v", legacy)
	}
	if svc.RejectMessage("203.0.113.5") == "" {
		t.Error("生效中网段条目载入后应拒连")
	}
	if svc.RejectMessage("203.0.114.5") != "" {
		t.Error("已过期网段条目载入时应丢弃（放行）")
	}
	bans := svc.List()
	if len(bans) != 2 {
		t.Errorf("过期与非法条目不应进列表, got %+v", bans)
	}
}

// TestIPBanService_CidrBanPersistFailureRollback 落盘失败回滚：BanCIDR 报错、
// 内存摘除（封禁未生效）、不写审计行；UnbanCIDR 落盘失败重插条目返 false（解封
// 未生效）。坏路径构造对齐七期先例（bansPath 父路径为已存在文件）。
func TestIPBanService_CidrBanPersistFailureRollback(t *testing.T) {
	svc, now := newTestIPBanService(t)
	audit := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	svc.SetAudit(audit)
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备阻塞文件: %v", err)
	}
	svc.mu.Lock()
	svc.bansPath = filepath.Join(blocker, "ip_bans.json")
	svc.mu.Unlock()

	if err := svc.BanCIDR("203.0.113.0/24", now().Add(time.Hour).Unix(), "", AuditActorDesktop, ""); err == nil {
		t.Fatal("落盘失败应报错")
	}
	if got := svc.RejectMessage("203.0.113.5"); got != "" {
		t.Errorf("落盘失败应回滚内存（封禁未生效）, got %q", got)
	}
	if _, err := os.Stat(audit.path); err == nil {
		for _, line := range readAuditLines(t, audit.path) {
			if line.Event == AuditEventIPBan {
				t.Fatalf("落盘失败不写 ip_ban 审计, got %+v", line)
			}
		}
	}

	// 可写路径先生效一次封禁，再切坏路径验证 UnbanCIDR 落盘失败回滚
	goodPath := filepath.Join(t.TempDir(), "ip_bans.json")
	svc.mu.Lock()
	svc.bansPath = goodPath
	svc.mu.Unlock()
	if err := svc.BanCIDR("203.0.113.0/24", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR: %v", err)
	}
	svc.mu.Lock()
	svc.bansPath = filepath.Join(blocker, "ip_bans.json")
	svc.mu.Unlock()
	if svc.UnbanCIDR("203.0.113.0/24", AuditActorDesktop, "") {
		t.Fatal("UnbanCIDR 落盘失败应返 false（解封未生效）")
	}
	if svc.RejectMessage("203.0.113.5") == "" {
		t.Error("UnbanCIDR 落盘失败应回滚内存（封禁仍生效）")
	}

	// 替换态回滚：同网段重复封禁落盘失败应恢复原条目参数（非摘除）
	svc.mu.Lock()
	svc.bansPath = goodPath
	svc.mu.Unlock()
	newExpiry := now().Add(3 * time.Hour).Unix()
	if err := svc.BanCIDR("203.0.113.0/24", newExpiry, "刷新", AuditActorDesktop, ""); err != nil {
		t.Fatalf("刷新 BanCIDR: %v", err)
	}
	svc.mu.Lock()
	svc.bansPath = filepath.Join(blocker, "ip_bans.json")
	svc.mu.Unlock()
	if err := svc.BanCIDR("203.0.113.0/24", now().Add(4*time.Hour).Unix(), "将失败", AuditActorDesktop, ""); err == nil {
		t.Fatal("替换态落盘失败应报错")
	}
	svc.mu.Lock()
	svc.bansPath = goodPath
	svc.mu.Unlock()
	ban, ok := banStatusByIP(svc.List(), "203.0.113.0/24")
	if !ok || ban.ExpiresAt != newExpiry {
		t.Errorf("替换态回滚应恢复原条目到期时刻 %d, got %+v", newExpiry, ban)
	}
}

// TestIPBanService_CidrCoveredByAndOrdering CoveredBy 三态与 List 排序：单 IP
// 被网段覆盖填网段串、网段被更宽网段覆盖填覆盖者串、无覆盖为空串；多命中取
// 掩码最短（最宏观）者；排序为封禁中在前已过期在后、组内网段条目置前、再按
// bannedAt 降序。
func TestIPBanService_CidrCoveredByAndOrdering(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	// 网段条目：10.0.0.0/8（宽）与 10.1.1.0/24（窄，被宽者覆盖），均永久档；
	// 另加 10.1.0.0/16（中）构造多命中——窄网段与网段内单 IP 同时被宽+中两级
	// 覆盖，钉死「多命中取掩码最短（最宏观）者」
	if err := svc.BanCIDR("10.0.0.0/8", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR 宽: %v", err)
	}
	if err := svc.BanCIDR("10.1.1.0/24", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR 窄: %v", err)
	}
	if err := svc.BanCIDR("10.1.0.0/16", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR 中: %v", err)
	}
	// 单 IP 条目：命中 10.0.0.0/8（覆盖）与 192.0.2.9（无覆盖），永久档
	if err := svc.Ban("10.1.2.3", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban 单 IP: %v", err)
	}
	if err := svc.Ban("192.0.2.9", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban 单 IP 无覆盖: %v", err)
	}
	// 已过期单 IP 条目（排序「已过期在后」素材）：临时短档写入后推进时钟使其
	// 过期（永久档不受影响；已过期条目载入路径会丢弃，故经时钟推进构造）
	if err := svc.Ban("192.0.2.10", now().Add(30*time.Minute).Unix(), "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban 临时条目: %v", err)
	}
	advanceIPBanClock(t, svc, newAdvancedClockFn(now().Add(time.Hour)), 0)

	bans := svc.List()
	if len(bans) != 6 {
		t.Fatalf("应恰有 6 条记录（5 生效 + 1 已过期）, got %+v", bans)
	}
	// 排序：生效组在前（网段条目置前，按 bannedAt 降序），已过期组在后
	if bans[0].Kind != model.IPBanKindCidr || bans[1].Kind != model.IPBanKindCidr || bans[2].Kind != model.IPBanKindCidr {
		t.Errorf("生效组网段条目应置前, got %+v", bans)
	}
	for i := 1; i < len(bans); i++ {
		if bans[i-1].Expired && !bans[i].Expired {
			t.Errorf("封禁中应排在已过期之前, got %+v", bans)
		}
	}
	last := bans[len(bans)-1]
	if !last.Expired || last.IP != "192.0.2.10" {
		t.Errorf("已过期条目应排尾部, got %+v", last)
	}

	// CoveredBy 三态（含多命中取最宏观者钉死）
	ipCovered, ok := banStatusByIP(bans, "10.1.2.3")
	if !ok || ipCovered.CoveredBy != "10.0.0.0/8" {
		t.Errorf("单 IP 命中网段应填最宏观覆盖者（/8 与 /16 双命中取 /8）, got %+v", ipCovered)
	}
	ipNoCover, ok := banStatusByIP(bans, "192.0.2.9")
	if !ok || ipNoCover.CoveredBy != "" {
		t.Errorf("无覆盖单 IP 的 CoveredBy 应为空串, got %+v", ipNoCover)
	}
	netCovered, ok := banStatusByIP(bans, "10.1.1.0/24")
	if !ok || netCovered.CoveredBy != "10.0.0.0/8" {
		t.Errorf("窄网段被宽+中两级覆盖应取最宏观覆盖者 /8, got %+v", netCovered)
	}
	netMid, ok := banStatusByIP(bans, "10.1.0.0/16")
	if !ok || netMid.CoveredBy != "10.0.0.0/8" {
		t.Errorf("中间网段被宽网段覆盖应填覆盖者 /8, got %+v", netMid)
	}
	netTop, ok := banStatusByIP(bans, "10.0.0.0/8")
	if !ok || netTop.CoveredBy != "" {
		t.Errorf("最宽网段无覆盖者, got %+v", netTop)
	}
}

// TestIPBanService_CidrBanAutoBanZeroRegression 自动封禁零回归：网段封禁内 IP
// 认证失败照常计数、照常触发单 IP 自动封禁（单 IP 条目与网段条目并存）；白名单
// 语义不受网段封禁影响（网段封禁非豁免面）。
func TestIPBanService_CidrBanAutoBanZeroRegression(t *testing.T) {
	svc, _, _ := newTestIPBanServiceWithBans(t)
	if err := svc.BanCIDR("203.0.113.0/24", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("BanCIDR: %v", err)
	}

	fillFailures(svc, "203.0.113.90:5678", IPBanThreshold)
	bans := svc.List()
	cidrEntry, ok := banStatusByIP(bans, "203.0.113.0/24")
	if !ok || cidrEntry.Kind != model.IPBanKindCidr {
		t.Fatalf("网段条目应存续, got %+v", bans)
	}
	autoEntry, ok := banStatusByIP(bans, "203.0.113.90")
	if !ok || autoEntry.Kind != model.IPBanKindIP || autoEntry.Source != model.IPBanSourceAuto || autoEntry.Strike != 1 {
		t.Fatalf("网段封禁内 IP 应照常触发单 IP 自动封禁, got %+v", autoEntry)
	}
	// 精确条目存在：消息取精确条目（自动临时档文案），非网段文案
	msg := svc.RejectMessage("203.0.113.90")
	if strings.Contains(msg, "网段") {
		t.Errorf("自动条目并存时消息应取精确条目, got %q", msg)
	}
	// 解除自动条目后回落网段文案（判定链逐级回落）
	if !svc.UnbanIP("203.0.113.90", AuditActorDesktop, "") {
		t.Fatal("自动条目解封应成功")
	}
	msg = svc.RejectMessage("203.0.113.90")
	if !strings.Contains(msg, "网段") {
		t.Errorf("精确条目解除后应回落网段文案, got %q", msg)
	}
}

// TestIPBanService_CidrBanRaceWithRecordFailure 并发安全（-race）：网段封禁/
// 解封（含落盘快照路径）与自动计数、拒绝判定、列表读取交错，无数据竞态。
func TestIPBanService_CidrBanRaceWithRecordFailure(t *testing.T) {
	svc, _, _ := newTestIPBanServiceWithBans(t)
	cidrs := []string{"203.0.113.0/24", "198.51.100.0/24", "192.0.2.0/24"}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				cidr := cidrs[(g+j)%len(cidrs)]
				switch (g + j) % 6 {
				case 0:
					_ = svc.BanCIDR(cidr, 0, "并发网段封禁", AuditActorDesktop, "")
				case 1:
					svc.RecordFailure("203.0.113.70:1234")
				case 2:
					_ = svc.RejectMessage("203.0.113.70")
				case 3:
					svc.UnbanCIDR(cidr, AuditActorDesktop, "")
				case 4:
					_ = svc.List()
				case 5:
					svc.RecordSuccess("203.0.113.70")
				}
			}
		}(g)
	}
	wg.Wait()
	bans := svc.List()
	for i := 1; i < len(bans); i++ {
		if bans[i-1].Expired && !bans[i].Expired {
			t.Errorf("List 应封禁中在前: %+v", bans)
			break
		}
	}
}
