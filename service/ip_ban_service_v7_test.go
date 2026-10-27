package service

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"workbench/model"
	"workbench/util"
)

// 连接管控七期测试：IP 手动封禁（Ban 状态机 + data/ip_bans.json 持久化）与
// 可信网段白名单豁免（CIDR 单点判定）。时钟全部经 newTestIPBanService 注入
// 时间源 + advanceIPBanClock 推进（禁 sleep）；持久化用例走 t.TempDir。

// newTestIPBanServiceWithBans 构造带手动封禁持久化路径的测试服务（tempdir 下
// ip_bans.json，SetBansPath 即时载入；返回路径供落盘断言与二次载入）。
func newTestIPBanServiceWithBans(t *testing.T) (*IPBanService, func() time.Time, string) {
	t.Helper()
	svc, now := newTestIPBanService(t)
	path := filepath.Join(t.TempDir(), "ip_bans.json")
	svc.SetBansPath(path)
	return svc, now, path
}

// newAdvancedClockFn 以绝对时刻构造推进后的时钟闭包（advanceIPBanClock 的绝对
// 形态，语义对齐既有用例「闭包内不得调持锁方法」纪律——入参为预先取好的值）。
func newAdvancedClockFn(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// TestIPBanService_ManualBanImmediateEffect 手动封禁即时生效：定时档拒绝消息含
// 解封时刻；List 条目 source=manual、FailCount/Strike 为手动语义零值、ExpiresAt
// 为传入到期时刻；两认证点判定入口 RejectMessage 同步拒绝。
func TestIPBanService_ManualBanImmediateEffect(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	ip := "203.0.113.60"
	expiresAt := now().Add(30 * time.Minute).Unix()

	if svc.RejectMessage(ip) != "" {
		t.Fatal("封禁前应放行")
	}
	if err := svc.Ban(ip, expiresAt, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban: %v", err)
	}

	wantMsg := "该来源 IP 已被临时封禁，至 " + time.Unix(expiresAt, 0).Format(banExpiryMessageFormat) + " 自动恢复"
	if msg := svc.RejectMessage(ip); msg != wantMsg {
		t.Errorf("拒绝消息应为 %q, got %q", wantMsg, msg)
	}
	ban, ok := banStatusByIP(svc.List(), ip)
	if !ok {
		t.Fatal("封禁后应有列表记录")
	}
	if ban.Source != model.IPBanSourceManual {
		t.Errorf("手动封禁条目 source 应为 manual, got %q", ban.Source)
	}
	if ban.FailCount != 0 || ban.Strike != 0 {
		t.Errorf("手动封禁无自动触发语境, FailCount/Strike 应为 0, got %d/%d", ban.FailCount, ban.Strike)
	}
	if ban.ExpiresAt != expiresAt {
		t.Errorf("ExpiresAt 应为 %d, got %d", expiresAt, ban.ExpiresAt)
	}
	if ban.Expired {
		t.Error("生效中封禁不应标记已过期")
	}
}

// TestIPBanService_ManualBanPermanent 永久封禁（expiresAt=0）：拒绝消息含
// 「永久」标识；ExpiresAt 透出 0、Expired 恒 false；时间大幅推进后仍拒绝
// （无到期时刻，仅 UnbanIP 可解除）。
func TestIPBanService_ManualBanPermanent(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	ip := "203.0.113.61"

	if err := svc.Ban(ip, 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban: %v", err)
	}
	msg := svc.RejectMessage(ip)
	if !strings.Contains(msg, "永久") {
		t.Errorf("永久封禁消息应含「永久」, got %q", msg)
	}
	// 推进 10 年：永久封禁不受时间影响
	advanceIPBanClock(t, svc, newAdvancedClockFn(now().AddDate(10, 0, 0)), 0)
	if svc.RejectMessage(ip) == "" {
		t.Error("永久封禁不应随时间过期")
	}
	ban, ok := banStatusByIP(svc.List(), ip)
	if !ok {
		t.Fatal("永久封禁应有列表记录")
	}
	if ban.ExpiresAt != 0 {
		t.Errorf("永久封禁 ExpiresAt 应透出 0, got %d", ban.ExpiresAt)
	}
	if ban.Expired {
		t.Error("永久封禁不应标记已过期")
	}
}

// TestIPBanService_ManualBanOverwritesAutoBan 手动封禁覆盖在途自动封禁：
// bannedUntil 顺延、source 置 manual、strikes 不变（管理员决策非攻击信号不参与
// 递增）、failures 不动（白盒断言失败队列原样保留）。
func TestIPBanService_ManualBanOverwritesAutoBan(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	ip := "203.0.113.62:1234"

	fillFailures(svc, ip, IPBanThreshold)
	autoBan, ok := banStatusByIP(svc.List(), "203.0.113.62") // List 条目为归一后 IP（去端口）
	if !ok || autoBan.Strike != 1 {
		t.Fatalf("应已触发自动封禁且 Strike=1, got %+v", autoBan)
	}
	// 白盒：自动条目失败队列应有 10 个时间戳（手动封禁不得清空）
	svc.mu.Lock()
	autoFailures := len(svc.entries["203.0.113.62"].failures)
	svc.mu.Unlock()
	if autoFailures != IPBanThreshold {
		t.Fatalf("自动条目失败队列应为 %d, got %d", IPBanThreshold, autoFailures)
	}

	newExpiry := now().Add(2 * time.Hour).Unix()
	if err := svc.Ban(ip, newExpiry, "攻击源处置", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban: %v", err)
	}
	ban, ok := banStatusByIP(svc.List(), "203.0.113.62")
	if !ok {
		t.Fatal("手动封禁后应有列表记录")
	}
	if ban.Source != model.IPBanSourceManual {
		t.Errorf("覆盖后 source 应置 manual, got %q", ban.Source)
	}
	if ban.ExpiresAt != newExpiry {
		t.Errorf("覆盖后 bannedUntil 应顺延为 %d, got %d", newExpiry, ban.ExpiresAt)
	}
	if ban.Strike != 1 {
		t.Errorf("手动封禁不应改写 strikes, got %d", ban.Strike)
	}
	if ban.FailCount != 0 {
		t.Errorf("手动封禁 FailCount 应清零, got %d", ban.FailCount)
	}
	svc.mu.Lock()
	gotFailures := len(svc.entries["203.0.113.62"].failures)
	svc.mu.Unlock()
	if gotFailures != IPBanThreshold {
		t.Errorf("手动封禁不应动 failures, 应为 %d, got %d", IPBanThreshold, gotFailures)
	}
}

// TestIPBanService_ManualBanOverwriteRefresh 重复手动封禁 = 刷新参数（对齐四期
// WebTokenService.Ban 幂等扩展形态）：bannedAt/bannedUntil/reason/归因整体覆盖
// 为本次值，记录恒唯一。
func TestIPBanService_ManualBanOverwriteRefresh(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	ip := "203.0.113.63"

	if err := svc.Ban(ip, now().Add(time.Hour).Unix(), "第一次", AuditActorDesktop, ""); err != nil {
		t.Fatalf("第一次 Ban: %v", err)
	}
	secondExpiry := now().Add(2 * time.Hour).Unix()
	if err := svc.Ban(ip, secondExpiry, "第二次", AuditActorRPC, "tok-1"); err != nil {
		t.Fatalf("第二次 Ban: %v", err)
	}
	bans := svc.List()
	if len(bans) != 1 {
		t.Fatalf("重复封禁记录应恒唯一, got %d 条", len(bans))
	}
	ban := bans[0]
	if ban.ExpiresAt != secondExpiry || ban.BannedAt != now().Unix() {
		t.Errorf("重复封禁应刷新时间戳, got %+v", ban)
	}
	if ban.Source != model.IPBanSourceManual {
		t.Errorf("覆盖后 source 应为 manual, got %q", ban.Source)
	}
	svc.mu.Lock()
	entry := svc.entries[ip]
	storedReason, storedActor, storedToken := entry.reason, entry.actorSource, entry.actorTokenID
	svc.mu.Unlock()
	if storedReason != "第二次" || storedActor != AuditActorRPC || storedToken != "tok-1" {
		t.Errorf("重复封禁应整体覆盖备注与归因, got %q/%q/%q", storedReason, storedActor, storedToken)
	}
}

// TestIPBanService_ManualBanValidation 入参校验：IP 空串拒绝（E_IP_BAN_IP_REQUIRED）、
// 到期时刻不在未来拒绝（E_TOKEN_EXPIRY_INVALID，对齐 WebTokenService.Ban 校验
// 形态）；校验拒绝不产生任何条目。
func TestIPBanService_ManualBanValidation(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)

	requireAppErr(t, svc.Ban("", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanIPRequired)
	requireAppErr(t, svc.Ban("  ", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanIPRequired)
	requireAppErr(t, svc.Ban("203.0.113.64", now().Unix()-1, "", AuditActorDesktop, ""), model.ErrCodeTokenExpiryInvalid)
	if bans := svc.List(); len(bans) != 0 {
		t.Errorf("校验拒绝不应产生条目, got %+v", bans)
	}
	// 恰等于当前时刻同为「不在未来」拒绝（对齐 expiresAt <= now 判定）
	requireAppErr(t, svc.Ban("203.0.113.64", now().Unix(), "", AuditActorDesktop, ""), model.ErrCodeTokenExpiryInvalid)
}

// TestIPBanService_ManualBanRejectsWhitelistedAndLoopback 封禁白名单/回环 IP 被
// 拒（E_IP_BAN_WHITELISTED，豁免面与封禁动作互斥）；拒绝不产生条目；白名单
// 移除后同 IP 可正常封禁（豁免态可逆）。
func TestIPBanService_ManualBanRejectsWhitelistedAndLoopback(t *testing.T) {
	svc, _, _ := newTestIPBanServiceWithBans(t)
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"203.0.113.0/24"},
	}))

	requireAppErr(t, svc.Ban("203.0.113.70", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanWhitelisted)
	requireAppErr(t, svc.Ban("127.0.0.1", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanWhitelisted)
	requireAppErr(t, svc.Ban("[::1]:8080", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanWhitelisted)
	if bans := svc.List(); len(bans) != 0 {
		t.Errorf("拒绝封禁不应产生条目, got %+v", bans)
	}

	// 白名单移除后可正常封禁（豁免态可逆；回环仍恒拒绝——回环豁免收编进单点）
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
	}))
	if err := svc.Ban("203.0.113.70", 0, "", AuditActorDesktop, ""); err != nil {
		t.Errorf("白名单移除后应可封禁, got %v", err)
	}
	requireAppErr(t, svc.Ban("127.0.0.1", 0, "", AuditActorDesktop, ""), model.ErrCodeIPBanWhitelisted)
}

// TestIPBanService_ManualBanAuditTrail 手动封禁审计行形态（ip_ban 单事件复用）：
// actorSource=desktop/rpc + actorTokenId 归因、reason 空输入生成缺省文案（永久/
// 定时分流）、非空输入保留原文；FailCount/Strike 零值不落键（omitempty）；不触发
// onBanned（自动封禁专属通知面）。
func TestIPBanService_ManualBanAuditTrail(t *testing.T) {
	svc, now, _ := newTestIPBanServiceWithBans(t)
	audit := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	svc.SetAudit(audit)
	notifyCh := make(chan model.IPBanStatus, 1)
	svc.SetOnBanned(func(status model.IPBanStatus) { notifyCh <- status })

	timedExpiry := now().Add(time.Hour).Unix()
	if err := svc.Ban("192.0.2.1", timedExpiry, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("桌面定时封禁: %v", err)
	}
	if err := svc.Ban("192.0.2.2", 0, "自定义备注", AuditActorRPC, "tok-9"); err != nil {
		t.Fatalf("RPC 永久封禁: %v", err)
	}

	rows := readAuditLines(t, audit.path)
	var banRows []AuditRecord
	for i := range rows {
		if rows[i].Event == AuditEventIPBan {
			banRows = append(banRows, rows[i])
		}
	}
	if len(banRows) != 2 {
		t.Fatalf("应恰有 2 条 ip_ban 审计行, got %d", len(banRows))
	}
	desktop, rpc := banRows[0], banRows[1]
	if desktop.ActorSource != AuditActorDesktop || desktop.ActorTokenID != "" {
		t.Errorf("桌面行归因应为 desktop/空, got %q/%q", desktop.ActorSource, desktop.ActorTokenID)
	}
	if desktop.Reason != "管理员手动封禁，至 "+time.Unix(timedExpiry, 0).Format(banExpiryMessageFormat) {
		t.Errorf("定时封禁空备注应生成缺省文案, got %q", desktop.Reason)
	}
	if rpc.ActorSource != AuditActorRPC || rpc.ActorTokenID != "tok-9" {
		t.Errorf("RPC 行归因应为 rpc/tok-9, got %q/%q", rpc.ActorSource, rpc.ActorTokenID)
	}
	if rpc.Reason != "自定义备注" {
		t.Errorf("非空备注应保留原文, got %q", rpc.Reason)
	}
	if rpc.ExpiresAt != 0 {
		t.Errorf("永久封禁审计 ExpiresAt 应为 0, got %d", rpc.ExpiresAt)
	}
	// FailCount/Strike 零值不落键：原始行 JSON 不含字段名（AuditRecord 反序列化
	// 零值与缺字段不可区分，须看原始字节）
	raw, err := os.ReadFile(audit.path)
	if err != nil {
		t.Fatalf("读审计文件: %v", err)
	}
	if strings.Contains(string(raw), "failCount") || strings.Contains(string(raw), "strike") {
		t.Errorf("手动封禁审计行不应携带 failCount/strike 键: %s", raw)
	}
	select {
	case status := <-notifyCh:
		t.Errorf("手动封禁不应触发 onBanned 自动通知, got %+v", status)
	default:
	}
}

// TestIPBanService_ManualBansPersistRoundTrip 持久化往返：手动封禁（定时 + 永久）
// 落盘后以新服务同路径载入——封禁状态/来源/备注/归因复现，RejectMessage 立即
// 拒绝；自动封禁与失败计数不落盘（重启清零语义不变）；文件中已过期手动条目
// 载入时丢弃（文件有界自洁）。
func TestIPBanService_ManualBansPersistRoundTrip(t *testing.T) {
	svc, now, path := newTestIPBanServiceWithBans(t)
	timedExpiry := now().Add(2 * time.Hour).Unix()
	if err := svc.Ban("192.0.2.11", timedExpiry, "机房扫描", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban 定时: %v", err)
	}
	if err := svc.Ban("192.0.2.12", 0, "", AuditActorRPC, "tok-3"); err != nil {
		t.Fatalf("Ban 永久: %v", err)
	}
	// 自动封禁对照：不落盘
	fillFailures(svc, "192.0.2.13", IPBanThreshold)

	// 文件内容：仅 2 条手动记录（自动封禁 IP 不在文件中）
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读持久化文件: %v", err)
	}
	if strings.Contains(string(raw), "192.0.2.13") {
		t.Errorf("自动封禁不应落盘: %s", raw)
	}
	var stored model.IPBanList
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("文件应为合法 IPBanList JSON: %v", err)
	}
	if len(stored.Bans) != 2 {
		t.Fatalf("文件应恰有 2 条手动记录, got %d", len(stored.Bans))
	}

	// 新服务同路径载入：手动封禁复现，自动封禁消失
	reloaded := NewIPBanService(func() time.Time { return now() })
	reloaded.SetBansPath(path)
	if msg := reloaded.RejectMessage("192.0.2.11"); !strings.Contains(msg, "自动恢复") {
		t.Errorf("载入后定时封禁应生效, got %q", msg)
	}
	if msg := reloaded.RejectMessage("192.0.2.12"); !strings.Contains(msg, "永久") {
		t.Errorf("载入后永久封禁应生效, got %q", msg)
	}
	if msg := reloaded.RejectMessage("192.0.2.13"); msg != "" {
		t.Errorf("自动封禁不应跨重启复活, got %q", msg)
	}
	manual, ok := banStatusByIP(reloaded.List(), "192.0.2.11")
	if !ok || manual.Source != model.IPBanSourceManual {
		t.Fatalf("载入条目 source 应为 manual, got %+v", manual)
	}
	if manual.FailCount != 0 || manual.Strike != 0 {
		t.Errorf("载入条目 FailCount/Strike 应重置为 0, got %d/%d", manual.FailCount, manual.Strike)
	}
	svc.mu.Lock()
	reason := svc.entries["192.0.2.11"].reason
	svc.mu.Unlock()
	if reason != "机房扫描" {
		t.Errorf("载入条目备注应保真, got %q", reason)
	}
}

// TestIPBanService_ManualBansLoadDropsExpired 已过期手动条目载入时丢弃：直接
// 构造含生效中与已过期两条记录的文件，载入后仅生效条目拒绝（「已过期」态仅
// 运行期内可见，懒过期语义不变）。
func TestIPBanService_ManualBansLoadDropsExpired(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	path := filepath.Join(t.TempDir(), "ip_bans.json")
	list := model.IPBanList{
		Version: model.IPBanListVersion,
		Bans: []model.IPBanRecord{
			{IP: "192.0.2.21", BannedAt: now.Add(-time.Hour).Unix(), ExpiresAt: now.Add(time.Hour).Unix(), Reason: "生效中"},
			{IP: "192.0.2.22", BannedAt: now.Add(-2 * time.Hour).Unix(), ExpiresAt: now.Add(-time.Hour).Unix(), Reason: "已过期"},
			{IP: "192.0.2.23", BannedAt: now.Add(-time.Hour).Unix(), ExpiresAt: 0, Reason: "永久"},
			{IP: "", BannedAt: now.Unix(), ExpiresAt: 0}, // 空白记录防御性跳过
		},
	}
	if err := util.SaveJSON(path, list); err != nil {
		t.Fatalf("写 fixture: %v", err)
	}
	svc := NewIPBanService(func() time.Time { return now })
	svc.SetBansPath(path)

	if svc.RejectMessage("192.0.2.21") == "" {
		t.Error("生效中条目载入后应拒绝")
	}
	if svc.RejectMessage("192.0.2.22") != "" {
		t.Error("已过期条目载入时应丢弃（放行）")
	}
	if svc.RejectMessage("192.0.2.23") == "" {
		t.Error("永久条目载入后应拒绝")
	}
	bans := svc.List()
	if len(bans) != 2 {
		t.Errorf("已过期与空白条目不应进列表, got %+v", bans)
	}
}

// TestIPBanService_ManualBansCorruptDowngrade 持久化文件损坏降级：载入空列表
// 不阻塞（对齐 web_bans.json 四期先例），后续手动封禁正常写入与生效。
func TestIPBanService_ManualBansCorruptDowngrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ip_bans.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("写损坏 fixture: %v", err)
	}
	svc, _ := newTestIPBanService(t)
	svc.SetBansPath(path)

	if bans := svc.List(); len(bans) != 0 {
		t.Errorf("损坏文件应降级空列表, got %+v", bans)
	}
	if err := svc.Ban("192.0.2.31", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("降级后手动封禁应正常: %v", err)
	}
	if svc.RejectMessage("192.0.2.31") == "" {
		t.Error("降级后手动封禁应生效")
	}
}

// TestIPBanService_UnbanIPPersistsRemoval 解封触发落盘更新：手动封禁解封后文件
// 同步移除记录（新服务载入不再拒绝——防重启后手动封禁复活）；解封失败计数条目
// （无封禁状态）不构成落盘扰动。
func TestIPBanService_UnbanIPPersistsRemoval(t *testing.T) {
	svc, now, path := newTestIPBanServiceWithBans(t)
	if err := svc.Ban("192.0.2.41", now().Add(time.Hour).Unix(), "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban: %v", err)
	}
	if !svc.UnbanIP("192.0.2.41", AuditActorDesktop, "") {
		t.Fatal("解封应成功")
	}

	reloaded := NewIPBanService(func() time.Time { return now() })
	reloaded.SetBansPath(path)
	if msg := reloaded.RejectMessage("192.0.2.41"); msg != "" {
		t.Errorf("解封后新服务载入不应再拒绝（防重启复活）, got %q", msg)
	}
	if bans := reloaded.List(); len(bans) != 0 {
		t.Errorf("解封后文件不应残留手动记录, got %+v", bans)
	}
}

// TestIPBanService_ManualBanPersistFailureRollback 落盘失败回滚：Ban 报错、内存
// 字段级恢复（封禁未生效）、不写审计行；UnbanIP 落盘失败重插条目返 false（解封
// 未生效，防重启复活语义）。落盘失败构造对齐 web_token_ban_test 先例——bansPath
// 父路径为已存在文件（ENOTDIR 确定性失败）。
func TestIPBanService_ManualBanPersistFailureRollback(t *testing.T) {
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

	// Ban 落盘失败：报错 + 内存回滚 + 不写审计
	if err := svc.Ban("192.0.2.51", now().Add(time.Hour).Unix(), "", AuditActorDesktop, ""); err == nil {
		t.Fatal("落盘失败应报错")
	}
	if msg := svc.RejectMessage("192.0.2.51"); msg != "" {
		t.Errorf("落盘失败应回滚内存（封禁未生效）, got %q", msg)
	}
	if bans := svc.List(); len(bans) != 0 {
		t.Errorf("落盘失败后列表应为空, got %+v", bans)
	}
	if _, err := os.Stat(audit.path); err == nil {
		for _, line := range readAuditLines(t, audit.path) {
			if line.Event == AuditEventIPBan {
				t.Fatalf("落盘失败不写 ip_ban 审计, got %+v", line)
			}
		}
	}

	// 先在可写路径生效一次封禁，再切换到坏路径验证 UnbanIP 落盘失败回滚
	goodPath := filepath.Join(t.TempDir(), "ip_bans.json")
	svc.mu.Lock()
	svc.bansPath = goodPath
	svc.mu.Unlock()
	if err := svc.Ban("192.0.2.52", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("Ban: %v", err)
	}
	svc.mu.Lock()
	svc.bansPath = filepath.Join(blocker, "ip_bans.json")
	svc.mu.Unlock()
	if svc.UnbanIP("192.0.2.52", AuditActorDesktop, "") {
		t.Fatal("UnbanIP 落盘失败应返 false（解封未生效）")
	}
	if svc.RejectMessage("192.0.2.52") == "" {
		t.Error("UnbanIP 落盘失败应回滚内存（封禁仍生效）")
	}
}

// TestIPBanService_WhitelistRecordFailureNotCounted 白名单网段命中 IP 认证失败
// 完全不计数：计满阈值不触发封禁、列表无痕（条目不创建、failures 队列无记录）；
// 网段外对照 IP 正常计数触发（豁免逻辑非空转）。
func TestIPBanService_WhitelistRecordFailureNotCounted(t *testing.T) {
	svc, _, _ := newTestIPBanServiceWithBans(t)
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
		Whitelist: []string{"203.0.113.0/24"},
	}))

	for i := 0; i < IPBanThreshold+5; i++ {
		svc.RecordFailure("203.0.113.80:5678")
	}
	if svc.RejectMessage("203.0.113.80") != "" {
		t.Error("白名单网段命中 IP 不应触发封禁")
	}
	if bans := svc.List(); len(bans) != 0 {
		t.Errorf("白名单命中不应产生任何记录（failures 队列无痕）, got %+v", bans)
	}
	svc.mu.Lock()
	_, hasEntry := svc.entries["203.0.113.80"]
	svc.mu.Unlock()
	if hasEntry {
		t.Error("白名单命中不应创建条目")
	}

	// 网段外对照：正常计数触发
	fillFailures(svc, "198.51.100.1", IPBanThreshold)
	if svc.RejectMessage("198.51.100.1") == "" {
		t.Error("网段外对照应正常触发封禁")
	}
}

// TestIPBanService_WhitelistRejectMessageRetroactivePass 白名单追溯放行：已封禁
// IP 加入白名单即放行（在途记录不删，豁免为语义修正）；白名单移除且未过期即
// 恢复拒连。
func TestIPBanService_WhitelistRejectMessageRetroactivePass(t *testing.T) {
	svc, _, _ := newTestIPBanServiceWithBans(t)
	fillFailures(svc, "203.0.113.81", IPBanThreshold)
	if svc.RejectMessage("203.0.113.81") == "" {
		t.Fatal("封禁应先生效")
	}

	base := model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
	})
	base.Whitelist = []string{"203.0.113.0/24"}
	svc.ApplyConfig(base)
	if msg := svc.RejectMessage("203.0.113.81"); msg != "" {
		t.Errorf("加入白名单后应追溯放行, got %q", msg)
	}
	if _, ok := banStatusByIP(svc.List(), "203.0.113.81"); !ok {
		t.Error("追溯放行不应删除在途记录")
	}

	// 白名单移除（ApplyConfig 无白名单配置）且封禁未过期：恢复拒连
	noWhitelist := model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
	})
	svc.ApplyConfig(noWhitelist)
	if svc.RejectMessage("203.0.113.81") == "" {
		t.Error("白名单移除且未过期应恢复拒连")
	}
}

// TestIPBanService_WhitelistApplyConfigHotEffect 白名单热更即时生效：9 次失败后
// 加入白名单，第 10 次失败不计数（不触发）；移除白名单后 1 次失败即触发
// （证明白名单期间该次失败确实未进队列，窗口计数 9+1）。
func TestIPBanService_WhitelistApplyConfigHotEffect(t *testing.T) {
	svc, _, _ := newTestIPBanServiceWithBans(t)
	ip := "203.0.113.82"

	fillFailures(svc, ip, IPBanThreshold-1)
	if svc.RejectMessage(ip) != "" {
		t.Fatal("9 次失败不应触发")
	}

	whitelisted := model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
	})
	whitelisted.Whitelist = []string{ip}
	svc.ApplyConfig(whitelisted)
	svc.RecordFailure(ip)
	if svc.RejectMessage(ip) != "" {
		t.Error("白名单热更后失败不应计数触发")
	}

	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 5, MaxBanDurationMinutes: 60,
	}))
	svc.RecordFailure(ip)
	if svc.RejectMessage(ip) == "" {
		t.Error("移除白名单后 1 次失败应触发（9+1 计满）")
	}
}

// TestIsWhitelistedIP_CIDRBoundaries 白名单判定纯函数边界矩阵：/32 精确命中
// （相邻 IP 不命中）、/24 网段首尾命中相邻网段不命中、IPv6 /128 精确与 /32 段、
// 裸 IP 自动补码（v4 /32、v6 /128）、非法条目忽略、回环恒豁免、非法 IP 串不豁免。
func TestIsWhitelistedIP_CIDRBoundaries(t *testing.T) {
	mustNet := func(t *testing.T, item string) *net.IPNet {
		t.Helper()
		n, ok := parseIPBanWhitelistEntry(item)
		if !ok {
			t.Fatalf("条目 %q 应可解析", item)
		}
		return n
	}
	nets := []*net.IPNet{
		mustNet(t, "203.0.113.5/32"),
		mustNet(t, "198.51.100.0/24"),
		mustNet(t, "2001:db8::1/128"),
		mustNet(t, "2001:db8::/32"),
		// 非法条目不进网段集（ApplyConfig 解析失败剔除）
	}

	cases := []struct {
		ip   string
		want bool
	}{
		{"203.0.113.5", true},      // /32 精确命中
		{"203.0.113.6", false},     // 相邻 IP 不命中（/32 精确）
		{"198.51.100.0", true},     // /24 首地址
		{"198.51.100.255", true},   // /24 末地址
		{"198.51.101.0", false},    // 相邻网段不命中
		{"2001:db8::1", true},      // IPv6 /128 精确
		{"2001:db8:aaaa::1", true}, // IPv6 /32 段命中
		{"2001:db9::1", false},     // 段外不命中
		{"127.0.0.1", true},        // 回环恒豁免（收编进单点）
		{"::1", true},              // IPv6 回环
		{"not-an-ip", false},       // 非法 IP 串不豁免
		{"", false},                // 空串不豁免
	}
	for _, c := range cases {
		if got := isWhitelistedIP(c.ip, nets); got != c.want {
			t.Errorf("isWhitelistedIP(%q) = %v, want %v", c.ip, got, c.want)
		}
	}
	// IPv6 /128 精确性独立验证（混合网段集中 /32 段会吞掉段内全部地址，
	// /128 的「相邻不命中」须在仅含 /128 的网段集下断言）
	only128 := []*net.IPNet{mustNet(t, "2001:db8::1/128")}
	if !isWhitelistedIP("2001:db8::1", only128) {
		t.Error("/128 自身地址应命中")
	}
	if isWhitelistedIP("2001:db8::2", only128) {
		t.Error("/128 相邻地址不应命中")
	}
	// nets 为 nil（未配置）时仅回环豁免（既有语义零回归）
	if isWhitelistedIP("203.0.113.5", nil) {
		t.Error("未配置白名单时不应豁免普通 IP")
	}
	if !isWhitelistedIP("127.0.0.1", nil) {
		t.Error("未配置白名单时回环仍应豁免")
	}
}

// TestParseIPBanWhitelistEntry 条目解析：CIDR 直解析、裸 v4 补 /32、裸 v6 补
// /128、前后空白容忍、非法串拒绝。
func TestParseIPBanWhitelistEntry(t *testing.T) {
	cases := []struct {
		item  string
		want  bool
		check func(t *testing.T, n *net.IPNet)
	}{
		{item: "192.168.1.0/24", want: true, check: func(t *testing.T, n *net.IPNet) {
			if got := n.String(); got != "192.168.1.0/24" {
				t.Errorf("CIDR 应原样解析, got %q", got)
			}
		}},
		{item: "10.0.0.5", want: true, check: func(t *testing.T, n *net.IPNet) {
			if n.String() != "10.0.0.5/32" {
				t.Errorf("裸 IPv4 应自动补 /32, got %q", n.String())
			}
		}},
		{item: "2001:db8::9", want: true, check: func(t *testing.T, n *net.IPNet) {
			if n.String() != "2001:db8::9/128" {
				t.Errorf("裸 IPv6 应自动补 /128, got %q", n.String())
			}
		}},
		{item: "  10.0.0.5  ", want: true, check: func(t *testing.T, n *net.IPNet) {
			if n.String() != "10.0.0.5/32" {
				t.Errorf("前后空白应容忍, got %q", n.String())
			}
		}},
		{item: "not-an-ip", want: false},
		{item: "10.0.0.5/99", want: false},
		{item: "", want: false},
		{item: "   ", want: false},
	}
	for _, c := range cases {
		n, ok := parseIPBanWhitelistEntry(c.item)
		if ok != c.want {
			t.Errorf("parseIPBanWhitelistEntry(%q) ok = %v, want %v", c.item, ok, c.want)
			continue
		}
		if ok && c.check != nil {
			c.check(t, n)
		}
	}
}

// TestIPBanService_ManualBanRaceWithRecordFailure 并发安全（-race）：手动封禁/
// 解封（含落盘快照路径）与自动计数、拒绝判定、列表读取交错，无数据竞态；
// 终态不变式——List 按 BannedAt 降序、手动条目 source=manual。
func TestIPBanService_ManualBanRaceWithRecordFailure(t *testing.T) {
	svc, _, _ := newTestIPBanServiceWithBans(t)
	ips := []string{"192.0.2.61", "192.0.2.62", "192.0.2.63"}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				ip := ips[(g+j)%len(ips)]
				switch (g + j) % 6 {
				case 0:
					_ = svc.Ban(ip, 0, "并发封禁", AuditActorDesktop, "") // 永久档避免并发下过期漂移
				case 1:
					svc.RecordFailure(ip + ":1234")
				case 2:
					_ = svc.RejectMessage(ip)
				case 3:
					svc.UnbanIP(ip, AuditActorDesktop, "")
				case 4:
					_ = svc.List()
				case 5:
					svc.RecordSuccess(ip)
				}
			}
		}(g)
	}
	wg.Wait()

	bans := svc.List()
	for i := 1; i < len(bans); i++ {
		if bans[i-1].BannedAt < bans[i].BannedAt {
			t.Errorf("List 应按 BannedAt 降序: %+v", bans)
			break
		}
	}
	for _, b := range bans {
		// 并发混流下条目来源可为 manual（Ban 并发写入）或 auto（RecordFailure
		// 恰计满阈值触发），但恒非空——来源字段为七期起每个封禁条目的必填形态
		if b.Source != model.IPBanSourceManual && b.Source != model.IPBanSourceAuto {
			t.Errorf("封禁条目 source 应为 manual/auto 之一, got %q (%+v)", b.Source, b)
		}
	}
}
