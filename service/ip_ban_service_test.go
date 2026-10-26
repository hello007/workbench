package service

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"workbench/model"
)

// newTestIPBanService 构造测试用 IPBanService：nowFn 固定时间源（窗口滑动与
// 封禁到期经注入推进，不依赖 sleep 与真实时钟）。
func newTestIPBanService(t *testing.T) (*IPBanService, func() time.Time) {
	t.Helper()
	now := time.Unix(1_700_000_000, 0)
	svc := NewIPBanService(func() time.Time { return now })
	return svc, func() time.Time { return now }
}

// advanceIPBanClock 推进 IPBanService 的时间源（对齐 web_token_ban_test 的
// advanceNowFn 用法：持锁替换 nowFn，惰性判定即时生效）。
func advanceIPBanClock(t *testing.T, svc *IPBanService, base func() time.Time, d time.Duration) {
	t.Helper()
	svc.mu.Lock()
	svc.nowFn = advanceNowFn(base, d)
	svc.mu.Unlock()
}

// banStatusByIP 从封禁列表按 IP 取记录（miss 返回 false）。
func banStatusByIP(bans []model.IPBanStatus, ip string) (model.IPBanStatus, bool) {
	for _, b := range bans {
		if b.IP == ip {
			return b, true
		}
	}
	return model.IPBanStatus{}, false
}

// TestIPBanService_NilClockDefaultsToTimeNow nowFn 为 nil 时默认 time.Now：
// 构造可用（防 nil 解引用）且回环豁免在真实时钟下同样生效。
func TestIPBanService_NilClockDefaultsToTimeNow(t *testing.T) {
	svc := NewIPBanService(nil)
	if svc.nowFn == nil {
		t.Fatal("nowFn 为 nil 时应默认 time.Now")
	}
	for i := 0; i < IPBanThreshold+3; i++ {
		svc.RecordFailure("127.0.0.1:1000")
	}
	if svc.RejectMessage("127.0.0.1") != "" {
		t.Error("真实时钟下回环豁免也应生效")
	}
	svc.RecordSuccess("127.0.0.1")
	if len(svc.List()) != 0 {
		t.Error("回环地址不应产生任何记录")
	}
}

// TestIPBanService_ThresholdTriggersBan 阈值触发：第 10 次失败触发自动封禁、
// 第 9 次不足不触发；拒绝消息含自动恢复时刻；List 含 FailCount=10 与
// BannedAt/ExpiresAt（unix 秒）。
func TestIPBanService_ThresholdTriggersBan(t *testing.T) {
	svc, now := newTestIPBanService(t)
	ip := "203.0.113.7:5678" // RemoteAddr 原型串，入口归一为 203.0.113.7

	for i := 1; i <= 9; i++ {
		svc.RecordFailure(ip)
		if svc.RejectMessage(ip) != "" {
			t.Fatalf("第 %d 次失败不应触发封禁", i)
		}
		if len(svc.List()) != 0 {
			t.Fatalf("第 %d 次失败不应产生封禁记录", i)
		}
	}
	svc.RecordFailure(ip)

	msg := svc.RejectMessage(ip)
	if msg == "" {
		t.Fatal("第 10 次失败应触发自动封禁")
	}
	wantExpiry := now().Add(IPBanDuration)
	if !strings.Contains(msg, "至 "+wantExpiry.Format(banExpiryMessageFormat)+" 自动恢复") {
		t.Errorf("拒绝消息应含自动恢复时刻 %q, got %q", wantExpiry.Format(banExpiryMessageFormat), msg)
	}

	bans := svc.List()
	if len(bans) != 1 {
		t.Fatalf("应恰有 1 条封禁记录, got %d", len(bans))
	}
	ban := bans[0]
	if ban.IP != "203.0.113.7" {
		t.Errorf("封禁记录应为归一后 IP, got %q", ban.IP)
	}
	if ban.FailCount != IPBanThreshold {
		t.Errorf("FailCount 应为阈值 %d, got %d", IPBanThreshold, ban.FailCount)
	}
	if ban.Expired {
		t.Error("刚触发的封禁不应标记已过期")
	}
	if ban.BannedAt != now().Unix() {
		t.Errorf("BannedAt 应为触发时刻 %d, got %d", now().Unix(), ban.BannedAt)
	}
	if ban.ExpiresAt != wantExpiry.Unix() {
		t.Errorf("ExpiresAt 应为 %d, got %d", wantExpiry.Unix(), ban.ExpiresAt)
	}
}

// TestIPBanService_SlidingWindowPrunesOldFailures 窗口滑动：9 次失败随时间推进
// 出窗被剪枝，后续新失败从零重新计数——出窗后 1 次不触发、重新计满 10 次才触发。
func TestIPBanService_SlidingWindowPrunesOldFailures(t *testing.T) {
	svc, now := newTestIPBanService(t)
	ip := "198.51.100.9"

	for i := 0; i < 9; i++ {
		svc.RecordFailure(ip)
	}
	// 推进跨出窗口（10 分钟窗口，推进 11 分钟）：全部旧失败出窗
	advanceIPBanClock(t, svc, now, 11*time.Minute)

	svc.RecordFailure(ip)
	if svc.RejectMessage(ip) != "" {
		t.Error("出窗后 1 次新失败不应触发封禁（旧计数应已剪枝）")
	}
	// 重新计满才触发：再补 9 次（出窗后共 10 次）
	for i := 0; i < 8; i++ {
		svc.RecordFailure(ip)
	}
	if svc.RejectMessage(ip) != "" {
		t.Error("出窗后第 9 次失败不应触发封禁")
	}
	svc.RecordFailure(ip)
	if svc.RejectMessage(ip) == "" {
		t.Error("出窗后重新计满 10 次应触发封禁")
	}
}

// TestIPBanService_LazyExpiry 到期懒恢复：到期前拒绝、到期后放行（消息空串，
// 懒过期不做后台扫描）；记录保留在 List（Expired=true 可见性）；期满后窗口
// 重新计数——单次失败不触发，重新计满才再封。
func TestIPBanService_LazyExpiry(t *testing.T) {
	svc, now := newTestIPBanService(t)
	ip := "192.0.2.50"
	for i := 0; i < IPBanThreshold; i++ {
		svc.RecordFailure(ip)
	}
	if svc.RejectMessage(ip) == "" {
		t.Fatal("封禁应生效")
	}

	// 推进至到期点之前 1 秒：仍拒绝
	advanceIPBanClock(t, svc, now, IPBanDuration-time.Second)
	if svc.RejectMessage(ip) == "" {
		t.Error("到期前封禁应仍生效")
	}
	// 推进跨到期点：放行（基于原始基准的累计偏移，now+6min > now+5min 到期点）
	advanceIPBanClock(t, svc, now, IPBanDuration+time.Minute)
	if msg := svc.RejectMessage(ip); msg != "" {
		t.Errorf("到期后应放行（空串）, got %q", msg)
	}
	bans := svc.List()
	if len(bans) != 1 {
		t.Fatalf("过期封禁记录应保留, got %d 条", len(bans))
	}
	if !bans[0].Expired {
		t.Error("过期封禁记录应标记 Expired=true")
	}

	// 期满重新计数：旧窗口作废，单次失败不触发
	svc.RecordFailure(ip)
	if svc.RejectMessage(ip) != "" {
		t.Error("期满后单次失败不应立即再封（应重新计数）")
	}
	for i := 0; i < IPBanThreshold-1; i++ {
		svc.RecordFailure(ip)
	}
	if svc.RejectMessage(ip) == "" {
		t.Error("期满后重新计满 10 次应再次封禁")
	}
	// 第二次封禁刷新时间戳（对齐重复封禁刷新参数语义）
	ban, ok := banStatusByIP(svc.List(), ip)
	if !ok || ban.Expired {
		t.Errorf("再次封禁后记录应为生效中, got %+v", ban)
	}
}

// TestIPBanService_NoCountDuringBan 封禁期间失败不累计：封禁中多次 RecordFailure
// 不改变 FailCount、不顺延到期时间。
func TestIPBanService_NoCountDuringBan(t *testing.T) {
	svc, now := newTestIPBanService(t)
	ip := "192.0.2.60"
	for i := 0; i < IPBanThreshold; i++ {
		svc.RecordFailure(ip)
	}
	ban, _ := banStatusByIP(svc.List(), ip)

	for i := 0; i < 5; i++ {
		svc.RecordFailure(ip)
	}
	after, ok := banStatusByIP(svc.List(), ip)
	if !ok {
		t.Fatal("封禁记录应在封禁期间保留")
	}
	if after.FailCount != ban.FailCount {
		t.Errorf("封禁期间失败不应累计: 触发时 %d, 期间失败 5 次后仍 %d", ban.FailCount, after.FailCount)
	}
	if after.ExpiresAt != ban.ExpiresAt || after.BannedAt != ban.BannedAt {
		t.Errorf("封禁期间不应顺延封禁时间窗: before=%+v after=%+v", ban, after)
	}
	// 推进至到期点前 1 秒再失败：仍不累计
	advanceIPBanClock(t, svc, now, IPBanDuration-time.Second)
	svc.RecordFailure(ip)
	if after, _ = banStatusByIP(svc.List(), ip); after.FailCount != IPBanThreshold {
		t.Errorf("临到期失败也不应累计, got FailCount=%d", after.FailCount)
	}
}

// TestIPBanService_RecordSuccessResetsCount 认证成功清零：9 次失败后 1 次成功，
// 再 9 次失败不触发（计数已清零）；第 10 次才触发；对无记录 IP 成功为安全 no-op。
func TestIPBanService_RecordSuccessResetsCount(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	ip := "192.0.2.70"

	svc.RecordSuccess("203.0.113.99") // 无记录 IP：no-op 不崩
	for i := 0; i < 9; i++ {
		svc.RecordFailure(ip)
	}
	svc.RecordSuccess(ip)
	for i := 0; i < 9; i++ {
		svc.RecordFailure(ip)
		if svc.RejectMessage(ip) != "" {
			t.Fatalf("成功清零后第 %d 次失败不应触发封禁", i+1)
		}
	}
	svc.RecordFailure(ip)
	if svc.RejectMessage(ip) == "" {
		t.Error("清零后重新计满 10 次应触发封禁")
	}
}

// TestIPBanService_LoopbackExempt 回环豁免：127.0.0.1 与 ::1 各形态（含带端口
// 归一）失败均不触发；非回环对照正常触发（豁免逻辑非空转）。
func TestIPBanService_LoopbackExempt(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	loopbacks := []string{
		"127.0.0.1",
		"127.0.0.1:36115",
		"::1",
		"[::1]:8080",
	}
	for _, ip := range loopbacks {
		for i := 0; i < IPBanThreshold+5; i++ {
			svc.RecordFailure(ip)
		}
		if msg := svc.RejectMessage(ip); msg != "" {
			t.Errorf("回环地址 %s 不应触发封禁, got %q", ip, msg)
		}
	}
	if len(svc.List()) != 0 {
		t.Errorf("回环地址不应产生任何封禁记录, got %+v", svc.List())
	}

	// 非回环对照：达阈值正常触发
	for i := 0; i < IPBanThreshold; i++ {
		svc.RecordFailure("203.0.113.1")
	}
	if svc.RejectMessage("203.0.113.1") == "" {
		t.Error("非回环对照应正常触发封禁")
	}
}

// TestNormalizeIP 地址归一：带端口 IPv4/IPv6 取 host，无端口与裸 IPv6 原样，
// 空串安全。
func TestNormalizeIP(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"1.2.3.4:5678", "1.2.3.4"},
		{"[::1]:8080", "::1"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"1.2.3.4", "1.2.3.4"},  // 无端口原样
		{"::1", "::1"},          // 裸 IPv6 多冒号，SplitHostPort 报错原样
		{"2001:db8::9", "2001:db8::9"}, // 同上
		{"", ""},                // 空串安全
		{"example.com:443", "example.com"}, // host 形态按 SplitHostPort 语义取 host
	}
	for _, c := range cases {
		if got := normalizeIP(c.raw); got != c.want {
			t.Errorf("normalizeIP(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

// TestIPBanService_UnbanIP 手动解封语义：有封禁记录解除返 true 并写 ip_unban
// 审计行（指定 actorSource）；仅有失败计数无封禁的条目返 false；无记录 no-op
// 返 false（幂等）且不写审计；解封后新连接可达、失败计数同步清除。
func TestIPBanService_UnbanIP(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	audit := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	svc.SetAudit(audit)
	ip := "192.0.2.80"

	// 仅失败计数无封禁：解封返 false（清计数但不构成解封）
	for i := 0; i < 3; i++ {
		svc.RecordFailure(ip)
	}
	if svc.UnbanIP(ip, AuditActorDesktop, "") {
		t.Error("仅有失败计数无封禁记录时解封应返 false")
	}
	// 无记录 no-op 幂等
	if svc.UnbanIP("198.51.100.1", AuditActorDesktop, "") {
		t.Error("无记录 IP 解封应返 false")
	}

	for i := 0; i < IPBanThreshold; i++ {
		svc.RecordFailure(ip)
	}
	if svc.RejectMessage(ip) == "" {
		t.Fatal("封禁应生效")
	}
	if !svc.UnbanIP(ip, AuditActorDesktop, "") {
		t.Fatal("有封禁记录的解封应返 true")
	}
	if msg := svc.RejectMessage(ip); msg != "" {
		t.Errorf("解封后新连接应可达, got %q", msg)
	}
	if _, ok := banStatusByIP(svc.List(), ip); ok {
		t.Error("解封应删除封禁记录")
	}
	// 重复解封幂等返 false
	if svc.UnbanIP(ip, AuditActorDesktop, "") {
		t.Error("重复解封应幂等返 false")
	}
	// 解封同步清除失败计数：解封后 1 次失败不触发（从零重新计数）
	svc.RecordFailure(ip)
	if svc.RejectMessage(ip) != "" {
		t.Error("解封后单次失败不应触发封禁")
	}

	// 审计行：仅实际解除写一条 ip_unban，其余路径不写
	var unbanRows []AuditRecord
	for _, line := range readAuditLines(t, audit.path) {
		if line.Event == AuditEventIPUnban {
			unbanRows = append(unbanRows, line)
		}
	}
	if len(unbanRows) != 1 {
		t.Fatalf("应恰有 1 条 ip_unban 审计行, got %d", len(unbanRows))
	}
	if unbanRows[0].IP != ip || unbanRows[0].ActorSource != AuditActorDesktop {
		t.Errorf("ip_unban 行字段不符: %+v", unbanRows[0])
	}
	if unbanRows[0].FailCount != 0 || unbanRows[0].ExpiresAt != 0 || unbanRows[0].Reason != "" {
		t.Errorf("ip_unban 行不应携带计数/到期/备注: %+v", unbanRows[0])
	}
}

// TestIPBanService_ClearAll 清空全部：封禁与计数一并清零（令牌轮换/撤销联动
// 语义），后续失败从零重新计数；清空为预防性动作不写审计。
func TestIPBanService_ClearAll(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	audit := NewAuditLogger(t.TempDir() + "/web_audit.log")
	svc.SetAudit(audit)
	ipA, ipB := "192.0.2.90", "192.0.2.91"

	for i := 0; i < IPBanThreshold; i++ {
		svc.RecordFailure(ipA)
	}
	for i := 0; i < 3; i++ {
		svc.RecordFailure(ipB)
	}
	// 清空为预防性动作不写审计：对比 ClearAll 前后审计行数不变
	before := len(readAuditLines(t, audit.path))
	svc.ClearAll()

	if msg := svc.RejectMessage(ipA); msg != "" {
		t.Errorf("ClearAll 后封禁应解除, got %q", msg)
	}
	if bans := svc.List(); len(bans) != 0 {
		t.Errorf("ClearAll 后列表应为空, got %+v", bans)
	}
	if after := len(readAuditLines(t, audit.path)); after != before {
		t.Errorf("ClearAll 不应写审计行: 前后行数 %d -> %d", before, after)
	}
	// 清空后从零计数：B 再 10 次失败才触发（未叠加 ClearAll 前 3 次）
	for i := 0; i < IPBanThreshold-1; i++ {
		svc.RecordFailure(ipB)
	}
	if svc.RejectMessage(ipB) != "" {
		t.Error("ClearAll 后计数应从零起，9 次失败不应触发")
	}
	svc.RecordFailure(ipB)
	if svc.RejectMessage(ipB) == "" {
		t.Error("ClearAll 后重新计满 10 次应触发")
	}
}

// TestIPBanService_AuditTrail ip_ban 审计行形态：system 归因、来源 IP、触发时
// 窗口失败计数、到期 unix 秒、含计数的 reason 文案；未达阈值不写行（审计文件
// 尚未创建即为无行的等价形态）。
func TestIPBanService_AuditTrail(t *testing.T) {
	svc, now := newTestIPBanService(t)
	audit := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	svc.SetAudit(audit)
	ip := "192.0.2.100"

	for i := 0; i < IPBanThreshold-1; i++ {
		svc.RecordFailure(ip)
	}
	if _, err := os.Stat(audit.path); err == nil {
		for _, line := range readAuditLines(t, audit.path) {
			if line.Event == AuditEventIPBan {
				t.Fatalf("未达阈值不应写 ip_ban 审计, got %+v", line)
			}
		}
	}

	svc.RecordFailure(ip)
	wantExpires := now().Add(IPBanDuration).Unix()
	rows := readAuditLines(t, audit.path)
	var banRow *AuditRecord
	for i := range rows {
		if rows[i].Event == AuditEventIPBan {
			banRow = &rows[i]
		}
	}
	if banRow == nil {
		t.Fatal("达到阈值应写 ip_ban 审计行")
	}
	if banRow.IP != ip {
		t.Errorf("ip_ban 行 IP 应为 %q, got %q", ip, banRow.IP)
	}
	if banRow.ActorSource != AuditActorSystem {
		t.Errorf("ip_ban 行归因应为 system, got %q", banRow.ActorSource)
	}
	if banRow.FailCount != IPBanThreshold {
		t.Errorf("ip_ban 行 FailCount 应为 %d, got %d", IPBanThreshold, banRow.FailCount)
	}
	if banRow.ExpiresAt != wantExpires {
		t.Errorf("ip_ban 行 ExpiresAt 应为 %d, got %d", wantExpires, banRow.ExpiresAt)
	}
	if !strings.Contains(banRow.Reason, "认证失败 10 次") {
		t.Errorf("ip_ban 行 reason 应含触发计数, got %q", banRow.Reason)
	}
	if banRow.TokenID != "" || banRow.ActorTokenID != "" {
		t.Errorf("ip_ban 行不应携带令牌字段: %+v", banRow)
	}
}

// TestIPBanService_MultiIPIndependent 多 IP 独立：A 达阈值封禁不影响 B 的计数
// 与可达性；解封 A 不影响 B 后续独立触发封禁。
func TestIPBanService_MultiIPIndependent(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	ipA, ipB := "192.0.2.110", "192.0.2.111"

	for i := 0; i < IPBanThreshold; i++ {
		svc.RecordFailure(ipA)
	}
	svc.RecordFailure(ipB)
	if svc.RejectMessage(ipB) != "" {
		t.Error("B 单次失败不应受 A 封禁影响")
	}
	if !svc.UnbanIP(ipA, AuditActorDesktop, "") {
		t.Fatal("解封 A 应成功")
	}
	for i := 0; i < IPBanThreshold-1; i++ {
		svc.RecordFailure(ipB)
	}
	if svc.RejectMessage(ipB) == "" {
		t.Error("B 计满 10 次应独立触发封禁")
	}
	if svc.RejectMessage(ipA) != "" {
		t.Error("A 解封后应保持可达")
	}
	bans := svc.List()
	if len(bans) != 1 || bans[0].IP != ipB {
		t.Errorf("仅 B 应有封禁记录, got %+v", bans)
	}
}

// TestIPBanService_Concurrent 并发安全（-race）：多 goroutine 对同 IP 与多 IP
// 并发失败/成功/查询/解封/列表交错，无数据竞态；终态不变式——List 全部条目
// FailCount 恒为阈值且按 BannedAt 降序。
func TestIPBanService_Concurrent(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	ips := []string{"192.0.2.120", "192.0.2.121", "192.0.2.122", "192.0.2.123"}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				ip := ips[(g+j)%len(ips)]
				switch (g + j) % 5 {
				case 0:
					svc.RecordFailure(ip + ":1234")
				case 1:
					svc.RecordSuccess(ip)
				case 2:
					_ = svc.RejectMessage(ip)
				case 3:
					_ = svc.UnbanIP(ip, AuditActorDesktop, "")
				case 4:
					_ = svc.List()
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
		if b.FailCount < IPBanThreshold {
			t.Errorf("封禁记录 FailCount 不应低于阈值: %+v", b)
		}
		if b.Expired {
			t.Errorf("固定时钟下封禁 5 分钟内不应过期: %+v", b)
		}
	}
}
