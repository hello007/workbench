package service

import (
	"testing"
	"time"

	"workbench/model"
)

// 连接管控九期聚合面测试：纯函数 aggregateIPsToSuggestions 的聚合口径（粒度/
// 阈值边界/排序/防御）与服务查询面 AggregateSuggestions 的条目筛选口径（仅
// auto、含已过期、CoveredBy 剔除、manual 剔除）。时间面全部经 newTestIPBanService
// 注入固定时钟与 advanceIPBanClock 推进（对齐五期先例），不依赖 sleep 与真实时钟。

// TestAggregateIPsToSuggestions_IPv4SameSlash24 同 /24 的两条 IPv4 聚合为一条
// 建议：CIDR 串为规范化网络地址形态（网络地址 + /24 掩码），计数为桶内条目数。
func TestAggregateIPsToSuggestions_IPv4SameSlash24(t *testing.T) {
	got := aggregateIPsToSuggestions([]string{"203.0.113.7", "203.0.113.9"}, 2)
	if len(got) != 1 {
		t.Fatalf("同 /24 两条应聚合为 1 条建议, got %d 条: %+v", len(got), got)
	}
	if got[0].CIDR != "203.0.113.0/24" {
		t.Errorf("CIDR 应为规范化网段串 203.0.113.0/24, got %q", got[0].CIDR)
	}
	if got[0].Count != 2 {
		t.Errorf("Count 应为桶内条目数 2, got %d", got[0].Count)
	}
}

// TestAggregateIPsToSuggestions_IPv6SameSlash64 同 /64 的两条 IPv6 聚合为一条
// 建议（IPv6 聚合粒度 /64，与 IPv4 /24 对应的固定粒度），规范化为 /64 网段串。
func TestAggregateIPsToSuggestions_IPv6SameSlash64(t *testing.T) {
	got := aggregateIPsToSuggestions([]string{"2001:db8:1:2::1", "2001:db8:1:2:ffff::ffff"}, 2)
	if len(got) != 1 {
		t.Fatalf("同 /64 两条应聚合为 1 条建议, got %d 条: %+v", len(got), got)
	}
	if got[0].CIDR != "2001:db8:1:2::/64" {
		t.Errorf("CIDR 应为规范化网段串 2001:db8:1:2::/64, got %q", got[0].CIDR)
	}
	if got[0].Count != 2 {
		t.Errorf("Count 应为 2, got %d", got[0].Count)
	}
}

// TestAggregateIPsToSuggestions_FamiliesDoNotMix IPv4 与 IPv6 分属不同桶（地址
// 族不互混）：各族单条互不叠加，阈值 2 下均不产出。
func TestAggregateIPsToSuggestions_FamiliesDoNotMix(t *testing.T) {
	got := aggregateIPsToSuggestions([]string{"203.0.113.7", "2001:db8:1:2::1"}, 2)
	if len(got) != 0 {
		t.Errorf("IPv4 与 IPv6 不应聚合到同一桶, got %+v", got)
	}
}

// TestAggregateIPsToSuggestions_IPv4MappedIPv6 IPv4-mapped IPv6 形态
// （::ffff:a.b.c.d）按 To4() 非 nil 判定走 IPv4 /24 分桶——与等值裸 IPv4 同桶
// 聚合（地址族内嵌形态不误入 IPv6 /64 桶），与真实 IPv6 仍不互混。
func TestAggregateIPsToSuggestions_IPv4MappedIPv6(t *testing.T) {
	got := aggregateIPsToSuggestions([]string{"::ffff:203.0.113.7", "203.0.113.9"}, 2)
	if len(got) != 1 || got[0].CIDR != "203.0.113.0/24" || got[0].Count != 2 {
		t.Fatalf("IPv4-mapped IPv6 应与裸 IPv4 同 /24 桶聚合, got %+v", got)
	}
	mixed := aggregateIPsToSuggestions([]string{"::ffff:203.0.113.7", "2001:db8:1:2::1"}, 2)
	if len(mixed) != 0 {
		t.Errorf("IPv4-mapped IPv6 不应与真实 IPv6 聚合同桶, got %+v", mixed)
	}
}

// TestAggregateIPsToSuggestions_ThresholdBoundary 阈值边界：桶内计数恰好等于
// 阈值产出（严格不小于），N-1 不产出。
func TestAggregateIPsToSuggestions_ThresholdBoundary(t *testing.T) {
	ips := []string{"203.0.113.7", "203.0.113.9"}
	if got := aggregateIPsToSuggestions(ips, 3); len(got) != 0 {
		t.Errorf("计数 2 低于阈值 3 不应产出, got %+v", got)
	}
	if got := aggregateIPsToSuggestions(ips, 2); len(got) != 1 {
		t.Errorf("计数 2 等于阈值 2 应产出, got %+v", got)
	}
}

// TestAggregateIPsToSuggestions_DefaultThreshold 默认阈值
// IPBanAggregateSuggestThresholdDefault（3）下的产出面：同 /24 三条产出一条、
// 两条不产出。
func TestAggregateIPsToSuggestions_DefaultThreshold(t *testing.T) {
	three := []string{"203.0.113.7", "203.0.113.9", "203.0.113.11"}
	if got := aggregateIPsToSuggestions(three, IPBanAggregateSuggestThresholdDefault); len(got) != 1 || got[0].Count != 3 {
		t.Errorf("默认阈值 3 下同 /24 三条应产出一条建议, got %+v", got)
	}
	if got := aggregateIPsToSuggestions(three[:2], IPBanAggregateSuggestThresholdDefault); len(got) != 0 {
		t.Errorf("默认阈值 3 下同 /24 两条不应产出, got %+v", got)
	}
}

// TestAggregateIPsToSuggestions_SortCountDescThenCIDR 排序两级：计数降序在前，
// 同计数按网段串字典序升序（确定性输出，前端渲染顺序稳定）。
func TestAggregateIPsToSuggestions_SortCountDescThenCIDR(t *testing.T) {
	ips := []string{
		"203.0.113.7", "203.0.113.9", "203.0.113.11", // 203.0.113.0/24 计数 3
		"198.51.100.5", "198.51.100.6", // 198.51.100.0/24 计数 2
		"192.0.2.5", "192.0.2.6", // 192.0.2.0/24 计数 2
	}
	got := aggregateIPsToSuggestions(ips, 2)
	want := []string{"203.0.113.0/24", "192.0.2.0/24", "198.51.100.0/24"}
	if len(got) != len(want) {
		t.Fatalf("应产出 3 条建议, got %d: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].CIDR != w {
			t.Errorf("第 %d 条应为 %q（计数降序 + 同计数字典序）, got %q", i, w, got[i].CIDR)
		}
	}
}

// TestAggregateIPsToSuggestions_ParseFailureSkipped 非法 IP 跳过（防御：不
// panic、不计入任何桶），合法条目照常聚合。
func TestAggregateIPsToSuggestions_ParseFailureSkipped(t *testing.T) {
	ips := []string{"203.0.113.7", "", "not-an-ip", "300.1.2.3", "203.0.113.9"}
	got := aggregateIPsToSuggestions(ips, 2)
	if len(got) != 1 || got[0].CIDR != "203.0.113.0/24" || got[0].Count != 2 {
		t.Errorf("非法条目应跳过、合法条目照常聚合, got %+v", got)
	}
}

// TestAggregateIPsToSuggestions_EmptyInput 空入参返回空切片（非 nil，对齐服务
// 查询面空态契约）。
func TestAggregateIPsToSuggestions_EmptyInput(t *testing.T) {
	got := aggregateIPsToSuggestions(nil, 3)
	if got == nil || len(got) != 0 {
		t.Errorf("空入参应返回空切片（非 nil）, got %#v", got)
	}
}

// TestIPBanService_AggregateSuggestions_SourceScope 服务查询面来源口径：auto
// 条目参与聚合；同网段追加 manual 条目不参与（桶计数不叠加）。
func TestIPBanService_AggregateSuggestions_SourceScope(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	// 直改字段即阈值注入面（同包测试直灌形态；设置链热更有 ApplyConfig 专项用例）
	svc.suggestThreshold = 3

	for _, ip := range []string{"203.0.113.7", "203.0.113.9", "203.0.113.11"} {
		for i := 0; i < IPBanThreshold; i++ {
			svc.RecordFailure(ip)
		}
	}
	got := svc.AggregateSuggestions()
	if len(got) != 1 || got[0].CIDR != "203.0.113.0/24" || got[0].Count != 3 {
		t.Fatalf("三个 auto 条目应聚合为一条建议, got %+v", got)
	}

	// 同网段手动封禁一个新来源：manual 条目不参与聚合，计数仍为 3
	if err := svc.Ban("203.0.113.200", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("手动封禁不应失败: %v", err)
	}
	got = svc.AggregateSuggestions()
	if len(got) != 1 || got[0].Count != 3 {
		t.Errorf("manual 条目不应参与聚合, got %+v", got)
	}
}

// TestIPBanService_AggregateSuggestions_CoveredByExcluded 被生效中网段封禁覆盖
// 的条目剔除（防重复建议）：网段封禁写入前同桶产出建议，按建议处置（封禁整个
// 网段）后整桶剔除为空。
func TestIPBanService_AggregateSuggestions_CoveredByExcluded(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	svc.suggestThreshold = 2

	for _, ip := range []string{"203.0.113.7", "203.0.113.9"} {
		for i := 0; i < IPBanThreshold; i++ {
			svc.RecordFailure(ip)
		}
	}
	if got := svc.AggregateSuggestions(); len(got) != 1 {
		t.Fatalf("网段封禁前应产出建议, got %+v", got)
	}

	if err := svc.BanCIDR("203.0.113.0/24", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("网段封禁不应失败: %v", err)
	}
	if got := svc.AggregateSuggestions(); len(got) != 0 {
		t.Errorf("网段封禁覆盖后不应再产出建议, got %+v", got)
	}
}

// TestIPBanService_AggregateSuggestions_ExpiredStillCounted 含已过期条目仍参与
// 聚合（与 List 可见条目同口径——所见即所计）：封禁到期后建议存续，直至条目
// 随重启清零。
func TestIPBanService_AggregateSuggestions_ExpiredStillCounted(t *testing.T) {
	svc, now := newTestIPBanService(t)
	svc.suggestThreshold = 3

	for _, ip := range []string{"203.0.113.7", "203.0.113.9", "203.0.113.11"} {
		for i := 0; i < IPBanThreshold; i++ {
			svc.RecordFailure(ip)
		}
	}
	// 推进跨封禁到期点（首档时长 IPBanDuration，推进 6 分钟）：List 全部标记已过期
	advanceIPBanClock(t, svc, now, IPBanDuration+time.Minute)
	for _, b := range svc.List() {
		if !b.Expired {
			t.Fatalf("推进后条目应已过期: %+v", b)
		}
	}
	if got := svc.AggregateSuggestions(); len(got) != 1 || got[0].Count != 3 {
		t.Errorf("已过期 auto 条目仍应参与聚合, got %+v", got)
	}
}

// TestIPBanService_AggregateSuggestions_EmptyState 空态：无条目返回空切片（非
// nil）；纯失败计数条目（多次失败未达封禁阈值，bannedAt 零值）不参与。
func TestIPBanService_AggregateSuggestions_EmptyState(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	if got := svc.AggregateSuggestions(); got == nil || len(got) != 0 {
		t.Fatalf("无条目应返回空切片（非 nil）, got %#v", got)
	}
	for i := 0; i < IPBanThreshold-1; i++ {
		svc.RecordFailure("203.0.113.7")
	}
	if got := svc.AggregateSuggestions(); len(got) != 0 {
		t.Errorf("未触发封禁的纯计数条目不应参与聚合, got %+v", got)
	}
}

// TestIPBanService_AggregateSuggestions_ApplyConfigThresholdHotEffect 聚合建议
// 阈值热更生效（连接管控九期接设置链，对齐六期 ApplyConfig 热更先例）：构造默认
// 阈值起跑 → 同网段两条不产出 → ApplyConfig 注入阈值 2 后同批存量条目下次查询
// 即产出（改小后存量条目自然满足，无主动扫描）→ 改回 3 再收敛。
func TestIPBanService_AggregateSuggestions_ApplyConfigThresholdHotEffect(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	if svc.suggestThreshold != IPBanAggregateSuggestThresholdDefault {
		t.Fatalf("前置：构造应以默认阈值起跑, got %d", svc.suggestThreshold)
	}

	for _, ip := range []string{"203.0.113.7", "203.0.113.9"} {
		for i := 0; i < IPBanThreshold; i++ {
			svc.RecordFailure(ip)
		}
	}
	if got := svc.AggregateSuggestions(); len(got) != 0 {
		t.Fatalf("默认阈值 3 下同网段两条不应产出, got %+v", got)
	}

	// 热更阈值 2：存量条目下次查询即满足（其余参数段同步透传，四参数零回归）
	svc.ApplyConfig(model.IPBanSettings{
		Threshold:                 IPBanThreshold,
		WindowMinutes:             int(IPBanWindow / time.Minute),
		DurationMinutes:           int(IPBanDuration / time.Minute),
		MaxBanDurationMinutes:     int(IPBanMaxBanDuration / time.Minute),
		Whitelist:                 []string{},
		AggregateSuggestThreshold: 2,
	})
	got := svc.AggregateSuggestions()
	if len(got) != 1 || got[0].CIDR != "203.0.113.0/24" || got[0].Count != 2 {
		t.Fatalf("热更阈值 2 后同网段两条应产出建议, got %+v", got)
	}

	// 热更回默认 3：同批条目收敛为空（仅面向未来语义，无追溯豁免矛盾面）
	svc.ApplyConfig(model.IPBanSettings{
		Threshold:                 IPBanThreshold,
		WindowMinutes:             int(IPBanWindow / time.Minute),
		DurationMinutes:           int(IPBanDuration / time.Minute),
		MaxBanDurationMinutes:     int(IPBanMaxBanDuration / time.Minute),
		Whitelist:                 []string{},
		AggregateSuggestThreshold: IPBanAggregateSuggestThresholdDefault,
	})
	if got := svc.AggregateSuggestions(); len(got) != 0 {
		t.Errorf("热更回默认阈值后应收敛为空, got %+v", got)
	}
}
