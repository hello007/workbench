package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"workbench/model"
)

// 连接管控六期测试：参数化（ApplyConfig 热更）与递增封禁（strikes 档位）。
// 时钟全部经 newTestIPBanService 注入时间源 + advanceIPBanClock 推进（禁 sleep）。

// fillFailures 连续登记 n 次认证失败（计满阈值触发用辅助，入参为 RemoteAddr
// 原型串，归一在服务内完成）。
func fillFailures(svc *IPBanService, ip string, n int) {
	for i := 0; i < n; i++ {
		svc.RecordFailure(ip)
	}
}

// TestIPBanService_EscalatingStrikesProgress 递增档位推进：默认参数（base 5min、
// 封顶 60min）下连续三轮触发，封禁时长 5→10→20min、Strike 1→2→3；每轮期满恢复
// 后窗口重新计满（既有剪枝语义）但档位叠加。
func TestIPBanService_EscalatingStrikesProgress(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	ip := "203.0.113.50:1000"

	wantDurations := []int{5, 10, 20}
	for round, wantMin := range wantDurations {
		fillFailures(svc, ip, IPBanThreshold)
		bans := svc.List()
		if len(bans) != 1 {
			t.Fatalf("第 %d 轮应恰有 1 条封禁记录, got %d", round+1, len(bans))
		}
		b := bans[0]
		if b.Strike != round+1 {
			t.Errorf("第 %d 轮 Strike 应为 %d, got %d", round+1, round+1, b.Strike)
		}
		if got := int(b.ExpiresAt-b.BannedAt) / 60; got != wantMin {
			t.Errorf("第 %d 次触发封禁时长应为 %d 分钟, got %d", round+1, wantMin, got)
		}
		if svc.RejectMessage(ip) == "" {
			t.Errorf("第 %d 轮封禁应生效中", round+1)
		}
		// 推进过期满（时长逐轮变长，按当前档位 +2 分钟保障出窗）
		advanceIPBanClock(t, svc, func() time.Time { return time.Unix(int64(b.ExpiresAt)+2*60, 0) }, 0)
		if svc.RejectMessage(ip) != "" {
			t.Fatalf("第 %d 轮封禁到期应懒过期放行", round+1)
		}
	}
}

// TestIPBanService_EscalatingCapAtMax 递增封顶：顺序触发 6 轮（档位只随触发次数
// 递增，不可跳档——到期恢复重计满后 strikes+1），第 5 次起 5×2^4=80min 超过默认
// 封顶 60min，时长恒为 60min（封顶后不再增长），Strike 继续递增（计数不封顶，
// 时长封顶）。
func TestIPBanService_EscalatingCapAtMax(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	ip := "203.0.113.51"

	// 第 1..6 次触发的期望时长（分钟）：5/10/20/40/80→封顶60/60
	wantByStrike := map[int]int{1: 5, 2: 10, 3: 20, 4: 40, 5: 60, 6: 60}
	for strike := 1; strike <= 6; strike++ {
		if bans := svc.List(); len(bans) == 1 {
			// 推进到上一轮到期后 2 分钟（期满懒过期放行）再计满
			expiry := bans[0].ExpiresAt
			advanceIPBanClock(t, svc, func() time.Time { return time.Unix(expiry+2*60, 0) }, 0)
			if svc.RejectMessage(ip) != "" {
				t.Fatalf("第 %d 轮推进后期满应放行", strike)
			}
		}
		fillFailures(svc, ip, IPBanThreshold)
		bans := svc.List()
		if len(bans) != 1 {
			t.Fatalf("第 %d 次触发应恰有 1 条记录", strike)
		}
		b := bans[0]
		if b.Strike != strike {
			t.Fatalf("Strike 应为 %d, got %d", strike, b.Strike)
		}
		if got := int(b.ExpiresAt-b.BannedAt) / 60; got != wantByStrike[strike] {
			t.Errorf("第 %d 次触发封禁时长应为 %d 分钟, got %d", strike, wantByStrike[strike], got)
		}
	}
}

// TestIPBanService_ApplyConfigHotEffect 热更即时生效：threshold 10→3 后存量计数
// 自然满足，下次失败即触发（无主动扫描）；封禁时长按新 duration 参数计算。
func TestIPBanService_ApplyConfigHotEffect(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	ip := "203.0.113.52"

	fillFailures(svc, ip, 5)
	if len(svc.List()) != 0 {
		t.Fatal("默认阈值 10 下 5 次失败不应触发")
	}
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 3, WindowMinutes: 10, DurationMinutes: 7, MaxBanDurationMinutes: 60,
	}))
	svc.RecordFailure(ip) // 存量 5 次 ≥ 新阈值 3，本次失败即触发
	bans := svc.List()
	if len(bans) != 1 {
		t.Fatal("热更后下次失败应按新阈值触发")
	}
	if bans[0].Strike != 1 {
		t.Errorf("首触 Strike 应为 1, got %d", bans[0].Strike)
	}
	if got := int(bans[0].ExpiresAt-bans[0].BannedAt) / 60; got != 7 {
		t.Errorf("封禁时长应按新 duration=7 分钟, got %d", got)
	}
}

// TestIPBanService_ApplyConfigKeepsInFlightBan 参数变更不动在途封禁：生效中封禁
// 的到期时刻不追溯调整——duration 5→60 后当前封禁仍按原 5 分钟到期放行（改大不
// 延长）；对偶形态（改小不提前释放）由同一 bannedUntil 不可变保证。
func TestIPBanService_ApplyConfigKeepsInFlightBan(t *testing.T) {
	svc, now := newTestIPBanService(t)
	ip := "203.0.113.53"

	fillFailures(svc, ip, IPBanThreshold)
	before, ok := banStatusByIP(svc.List(), ip)
	if !ok {
		t.Fatal("应已触发封禁")
	}
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 10, DurationMinutes: 60, MaxBanDurationMinutes: 1440,
	}))
	// 推进到原封禁到期后 1 分钟：若被追溯延长则仍在封禁中
	advanceIPBanClock(t, svc, func() time.Time { return now().Add(IPBanDuration + time.Minute) }, 0)
	if msg := svc.RejectMessage(ip); msg != "" {
		t.Errorf("生效中封禁不应被参数变更延长, got %q", msg)
	}
	after, _ := banStatusByIP(svc.List(), ip)
	if after.ExpiresAt != before.ExpiresAt {
		t.Errorf("bannedUntil 应保持 %d 不变, got %d", before.ExpiresAt, after.ExpiresAt)
	}
}

// TestIPBanService_ApplyConfigNewWindowPruning 新窗口长度作用于剪枝：window
// 10min→1min 后，2 分钟前的旧失败出窗失效，计不满新阈值不触发。
func TestIPBanService_ApplyConfigNewWindowPruning(t *testing.T) {
	svc, now := newTestIPBanService(t)
	ip := "203.0.113.54"

	fillFailures(svc, ip, 9)
	svc.ApplyConfig(model.NormalizeIPBanSettings(&model.IPBanSettings{
		Threshold: 10, WindowMinutes: 1, DurationMinutes: 5, MaxBanDurationMinutes: 60,
	}))
	// 推进 2 分钟：旧 9 次失败全部出窗（新窗口 1 分钟）
	advanceIPBanClock(t, svc, func() time.Time { return now().Add(2 * time.Minute) }, 0)
	svc.RecordFailure(ip)
	if bans := svc.List(); len(bans) != 0 {
		t.Fatalf("出窗后仅 1 次有效失败不应触发, got %+v", bans)
	}
}

// TestIPBanService_StrikesSurviveRecordSuccess 递增记忆不受认证成功清零：封禁
// 期内认证成功清失败计数但不清 strikes，期满后再触发按 Strike=2 递增封更久。
func TestIPBanService_StrikesSurviveRecordSuccess(t *testing.T) {
	svc, now := newTestIPBanService(t)
	ip := "203.0.113.55"

	fillFailures(svc, ip, IPBanThreshold)
	svc.RecordSuccess(ip) // 拿到令牌的合法登录：清 failures 保 strikes
	advanceIPBanClock(t, svc, func() time.Time { return now().Add(IPBanDuration + time.Minute) }, 0)
	svc.RecordSuccess(ip) // 期满后成功（清当次窗口失败）

	fillFailures(svc, ip, IPBanThreshold)
	b, ok := banStatusByIP(svc.List(), ip)
	if !ok {
		t.Fatal("期满重计后应再次触发")
	}
	if b.Strike != 2 {
		t.Errorf("认证成功不应清递增记忆：Strike 应为 2, got %d", b.Strike)
	}
	if got := int(b.ExpiresAt-b.BannedAt) / 60; got != 10 {
		t.Errorf("第 2 次触发应封 10 分钟, got %d", got)
	}
}

// TestIPBanService_UnbanIPResetsStrikes 解封完全重置：UnbanIP 删除条目，下次
// 触发从 Strike=1、基础时长重新起算（管理员显式介入即重置递增记忆）。
func TestIPBanService_UnbanIPResetsStrikes(t *testing.T) {
	svc, now := newTestIPBanService(t)
	ip := "203.0.113.56"

	fillFailures(svc, ip, IPBanThreshold)
	if !svc.UnbanIP(ip, AuditActorDesktop, "") {
		t.Fatal("应实际解封")
	}
	advanceIPBanClock(t, svc, func() time.Time { return now().Add(time.Minute) }, 0)
	fillFailures(svc, ip, IPBanThreshold)
	b, ok := banStatusByIP(svc.List(), ip)
	if !ok {
		t.Fatal("解封后重计满应重新触发")
	}
	if b.Strike != 1 {
		t.Errorf("解封后 Strike 应重置为 1, got %d", b.Strike)
	}
	if got := int(b.ExpiresAt-b.BannedAt) / 60; got != 5 {
		t.Errorf("解封后首触应为基础时长 5 分钟, got %d", got)
	}
}

// TestIPBanService_ClearAllResetsStrikes 轮换/撤销联动清零同时重置递增档位
// （ClearAll 重建 map，strikes 随条目一并清）。
func TestIPBanService_ClearAllResetsStrikes(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	ip := "203.0.113.57"

	fillFailures(svc, ip, IPBanThreshold)
	svc.ClearAll()
	svc.ClearAll() // 幂等无副作用
	fillFailures(svc, ip, IPBanThreshold)
	b, ok := banStatusByIP(svc.List(), ip)
	if !ok {
		t.Fatal("ClearAll 后重计满应重新触发")
	}
	if b.Strike != 1 {
		t.Errorf("ClearAll 后 Strike 应为 1, got %d", b.Strike)
	}
}

// TestIPBanService_AuditStrikeField ip_ban 审计行递增档位：Strike 字段落键、
// reason 文案含档位与实际封禁分钟数（第 2 次触发 = 10 分钟）；ip_unban 行无
// Strike 语义（零值不落键）。
func TestIPBanService_AuditStrikeField(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	audit := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	svc.SetAudit(audit)
	ip := "192.0.2.200"

	fillFailures(svc, ip, IPBanThreshold)
	// 推进到期满，重计满第二次触发（expiresAt 先取值——闭包内不得调 svc.List()
	// 等持锁方法：RecordFailure 持 mu 期间经 nowFn() 调用闭包，闭包再取 mu 即
	// 同 goroutine 重入死锁）
	expiresAt := svc.List()[0].ExpiresAt
	advanceIPBanClock(t, svc, func() time.Time { return time.Unix(expiresAt+2*60, 0) }, 0)
	fillFailures(svc, ip, IPBanThreshold)

	rows := readAuditLines(t, audit.path)
	var banRows []AuditRecord
	for i := range rows {
		if rows[i].Event == AuditEventIPBan {
			banRows = append(banRows, rows[i])
		}
	}
	if len(banRows) != 2 {
		t.Fatalf("应有 2 条 ip_ban 行, got %d", len(banRows))
	}
	if banRows[0].Strike != 1 || banRows[1].Strike != 2 {
		t.Errorf("ip_ban 行 Strike 应为 1,2, got %d,%d", banRows[0].Strike, banRows[1].Strike)
	}
	if banRows[1].FailCount != IPBanThreshold {
		t.Errorf("FailCount 应为 %d, got %d", IPBanThreshold, banRows[1].FailCount)
	}
	wantReason := "10 分钟内认证失败 10 次，第 2 次触发，自动临时封禁 10 分钟"
	if banRows[1].Reason != wantReason {
		t.Errorf("reason 应为 %q, got %q", wantReason, banRows[1].Reason)
	}
}

// TestEscalateBanDuration 递增公式纯函数：base×2^(n-1) 封顶 max；base>max 的
// 矛盾配置兜底收敛 max；大 strikes 有界（不溢出不长循环）。
func TestEscalateBanDuration(t *testing.T) {
	base := 5 * time.Minute
	max := 60 * time.Minute
	cases := []struct {
		strikes int
		want    time.Duration
	}{
		{1, 5 * time.Minute},
		{2, 10 * time.Minute},
		{3, 20 * time.Minute},
		{4, 40 * time.Minute},
		{5, 60 * time.Minute},    // 80 封顶 60
		{6, 60 * time.Minute},    // 恒封顶
		{1000, 60 * time.Minute}, // 大 strikes 有界
	}
	for _, c := range cases {
		if got := escalateBanDuration(base, max, c.strikes); got != c.want {
			t.Errorf("escalate(base=5m, max=60m, strikes=%d) = %v, want %v", c.strikes, got, c.want)
		}
	}
	// base > max 矛盾配置：兜底收敛 max
	if got := escalateBanDuration(60*time.Minute, 5*time.Minute, 1); got != 5*time.Minute {
		t.Errorf("base>max 应回敛到 max, got %v", got)
	}
	// 自定义 base（参数化档位基数生效）
	if got := escalateBanDuration(2*time.Minute, 100*time.Minute, 3); got != 8*time.Minute {
		t.Errorf("自定义 base=2m 第 3 档应为 8m, got %v", got)
	}
}

// TestIPBanService_ApplyConfigUnknownIPNoInteraction 热更对无记录 IP 无副作用
// （entries 不被 ApplyConfig 触碰：无失败计数的 IP 在参数变更后仍无任何记录）。
func TestIPBanService_ApplyConfigUnknownIPNoInteraction(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	svc.ApplyConfig(model.NormalizeIPBanSettings(nil)) // nil 段 = 全默认，构造合法入参
	if bans := svc.List(); len(bans) != 0 {
		t.Errorf("热更不应产生任何记录, got %+v", bans)
	}
	if _, err := os.Stat(auditPathIfExist(svc)); err == nil {
		t.Error("热更不应写审计文件")
	}
}

// auditPathIfExist 取测试服务的审计文件路径（未注入 audit 时返回不可能存在的
// 路径，Stat 走 not-exist 分支）。
func auditPathIfExist(svc *IPBanService) string {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.audit == nil {
		return filepath.Join(os.TempDir(), "ipban-v6-nonexistent-audit.log")
	}
	return svc.audit.path
}
