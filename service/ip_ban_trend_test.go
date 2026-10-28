package service

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"workbench/model"
)

// 连接管控十期趋势面测试：纯函数 aggregateIPBanTrend 的聚合口径（双 source 分列/
// 双 kind 落桶/双败跳过/::ffff: To4 归 /24/天桶本地时区日界/窗口裁剪/earliestTs
// 语义/rebounded 三分支/排序纪律）与服务查询面 QueryAuditTrend 的委托口径
// （audit 未注入空结果、真实审计快照链路、窗口参数经 ApplyConfig 热更生效）。
// 时间面全部经纯函数 now 参数注入（禁 sleep，对齐 test-stability 纪律）。

// trendTestBase 测试基准时刻（本地时区；日桶断言全部相对该值计算，不依赖环境
// 时区——窗口起点与天桶下标用同一 Location 基准推导）。
var trendTestBase = time.Date(2026, 10, 28, 15, 4, 5, 0, time.Local)

// trendTs 构造基准时刻加偏移后的 unix 秒。
func trendTs(d time.Duration) int64 {
	return trendTestBase.Add(d).Unix()
}

// trendBanRec 构造一条 ip_ban 审计记录（actorSource 空串按调用方意图填充）。
func trendBanRec(ts int64, ip, actorSource string) AuditRecord {
	return AuditRecord{Ts: ts, Event: AuditEventIPBan, IP: ip, ActorSource: actorSource}
}

// trendUnbanRec 构造一条 ip_unban 审计记录。
func trendUnbanRec(ts int64, ip string) AuditRecord {
	return AuditRecord{Ts: ts, Event: AuditEventIPUnban, IP: ip}
}

// bucketByKey 从结果桶列表按 key 取桶（miss 返回 false）。
func bucketByKey(result model.IPBanTrendResult, key string) (model.IPBanTrendBucket, bool) {
	for _, b := range result.Buckets {
		if b.Key == key {
			return b, true
		}
	}
	return model.IPBanTrendBucket{}, false
}

// TestAggregateIPBanTrend_AutoManualSeparated 双 source 分列：auto（system）行计
// 入 Days/AutoTotal（主趋势），manual（desktop/rpc）行分列 ManualCount 不混入；
// actorSource 为空的脏数据行按 manual 分列（审计写入单点保证 system/manual 二值，
// 宽松兜底无第三形态）。
func TestAggregateIPBanTrend_AutoManualSeparated(t *testing.T) {
	records := []AuditRecord{
		trendBanRec(trendTs(0), "203.0.113.7", AuditActorSystem),
		trendBanRec(trendTs(time.Minute), "203.0.113.7", AuditActorSystem),
		trendBanRec(trendTs(2*time.Minute), "203.0.113.7", AuditActorDesktop),
		trendBanRec(trendTs(3*time.Minute), "203.0.113.7", AuditActorRPC),
		trendBanRec(trendTs(4*time.Minute), "203.0.113.7", ""), // 脏数据兜底
	}
	got := aggregateIPBanTrend(records, 3, trendTestBase)
	b, ok := bucketByKey(got, "203.0.113.0/24")
	if !ok {
		t.Fatalf("同网段五行应聚合为一条桶, got %+v", got.Buckets)
	}
	if b.AutoTotal != 2 {
		t.Errorf("auto 行应计入主趋势 2, got %d", b.AutoTotal)
	}
	if b.ManualCount != 3 {
		t.Errorf("manual 行（desktop/rpc/空）应分列 3 不混入主趋势, got %d", b.ManualCount)
	}
	if sum := b.Days[0] + b.Days[1] + b.Days[2]; sum != 2 {
		t.Errorf("Days 仅累计 auto 行, want 总和 2, got %v", b.Days)
	}
	if b.LastSeen != trendTs(4*time.Minute) {
		t.Errorf("LastSeen 应为桶内最新 ip_ban 行 ts（manual 行也计）, got %d", b.LastSeen)
	}
}

// TestAggregateIPBanTrend_BucketKinds 双 kind 落桶：单 IP 掩入固定粒度 /24；八期
// 网段封禁审计行（IP 字段为规范化 CIDR 串）按原串独立成桶（不并入固定粒度掩桶
// ——管理员封禁范围保真）；ParseIP/ParseCIDR 双败行跳过（不 panic、不进桶，
// 仍计入 EarliestTs）。
func TestAggregateIPBanTrend_BucketKinds(t *testing.T) {
	records := []AuditRecord{
		trendBanRec(trendTs(0), "203.0.113.7", AuditActorSystem),                 // 单 IP → /24 桶
		trendBanRec(trendTs(time.Minute), "203.0.113.0/24", AuditActorDesktop),   // 同网段 CIDR → 与单 IP 桶同 key 自然聚合
		trendBanRec(trendTs(2*time.Minute), "203.0.113.0/25", AuditActorDesktop), // 异掩码 CIDR → 独立桶（不并入 /24）
		trendBanRec(trendTs(3*time.Minute), "192.0.2.5/32", AuditActorDesktop),   // 裸 IP 网段封禁的规范化形态 → /32 独立桶
		trendBanRec(trendTs(4*time.Minute), "not-an-ip", AuditActorSystem),       // 双败 → 跳过
	}
	got := aggregateIPBanTrend(records, 3, trendTestBase)
	if len(got.Buckets) != 3 {
		t.Fatalf("应产出 3 条桶（/24 聚合桶 + /25 独立桶 + /32 独立桶）, got %+v", got.Buckets)
	}
	main, ok := bucketByKey(got, "203.0.113.0/24")
	if !ok || main.AutoTotal != 1 || main.ManualCount != 1 {
		t.Errorf("单 IP 与同网段 /24 CIDR 应同桶聚合（auto 1 + manual 1）, got %+v", main)
	}
	if b, ok := bucketByKey(got, "203.0.113.0/25"); !ok || b.AutoTotal != 0 || b.ManualCount != 1 {
		t.Errorf("异掩码 /25 CIDR 应独立成桶（manual 1）, got %+v", b)
	}
	if b, ok := bucketByKey(got, "192.0.2.5/32"); !ok || b.ManualCount != 1 {
		t.Errorf("裸 IP 网段封禁规范化形态应独立成桶, got %+v", b)
	}
}

// TestAggregateIPBanTrend_IPv4MappedToSlash24 IPv4-mapped IPv6（::ffff:a.b.c.d）
// 经 To4() 判定落 IPv4 /24 桶——与等值裸 IPv4 同桶聚合，不误入 IPv6 /64 桶
// （九期专项用例先例，趋势面同接）。
func TestAggregateIPBanTrend_IPv4MappedToSlash24(t *testing.T) {
	records := []AuditRecord{
		trendBanRec(trendTs(0), "::ffff:203.0.113.7", AuditActorSystem),
		trendBanRec(trendTs(time.Minute), "203.0.113.9", AuditActorSystem),
		trendBanRec(trendTs(2*time.Minute), "2001:db8:1:2::1", AuditActorSystem),
	}
	got := aggregateIPBanTrend(records, 3, trendTestBase)
	v4, ok := bucketByKey(got, "203.0.113.0/24")
	if !ok || v4.AutoTotal != 2 {
		t.Fatalf("IPv4-mapped IPv6 应与裸 IPv4 同 /24 桶聚合, got %+v", got.Buckets)
	}
	if _, ok := bucketByKey(got, "2001:db8:1:2::/64"); !ok {
		t.Errorf("真实 IPv6 应落 /64 桶, got %+v", got.Buckets)
	}
}

// TestAggregateIPBanTrend_DayBucketsLocalMidnight 天桶口径：本地时区日界（与 now
// 同基准），Days 下标 0 = 窗口最旧日、末下标 = 今天，跨日记录落相邻下标。
func TestAggregateIPBanTrend_DayBucketsLocalMidnight(t *testing.T) {
	// 基准 2026-10-28 15:04（本地），窗口 3 天 → 起点 10-26 00:00 本地
	records := []AuditRecord{
		trendBanRec(trendTs(-58*time.Hour), "203.0.113.7", AuditActorSystem), // 10-26 05:04 → 下标 0
		trendBanRec(trendTs(-34*time.Hour), "203.0.113.7", AuditActorSystem), // 10-27 05:04 → 下标 1
		trendBanRec(trendTs(-10*time.Hour), "203.0.113.7", AuditActorSystem), // 10-28 05:04 → 下标 2（今天）
		trendBanRec(trendTs(8*time.Hour), "203.0.113.7", AuditActorSystem),   // 10-28 23:04 → 仍今天（当日界前）下标 2
	}
	got := aggregateIPBanTrend(records, 3, trendTestBase)
	b, ok := bucketByKey(got, "203.0.113.0/24")
	if !ok {
		t.Fatalf("应产出趋势桶, got %+v", got.Buckets)
	}
	want := []int{1, 1, 2}
	if !reflect.DeepEqual(b.Days, want) {
		t.Errorf("天桶应为 %v（下标 0=最旧日，末下标=今天）, got %v", want, b.Days)
	}
	if b.AutoTotal != 4 {
		t.Errorf("AutoTotal 应为全窗口 auto 总数 4, got %d", b.AutoTotal)
	}
}

// TestAggregateIPBanTrend_WindowPruning 窗口裁剪：窗口起点之前的 ip_ban 行裁掉
// 不进桶（Days/AutoTotal/LastSeen 均不含），EarliestTs 语义不受裁剪影响（含窗口
// 外行——前端据此判定「留存是否覆盖整个窗口」）。
func TestAggregateIPBanTrend_WindowPruning(t *testing.T) {
	outsideTs := trendTs(-96 * time.Hour) // 10-24 15:04，早于 3 天窗口起点 10-26 00:00
	records := []AuditRecord{
		trendBanRec(outsideTs, "203.0.113.7", AuditActorSystem),
		trendBanRec(trendTs(0), "203.0.113.7", AuditActorSystem),
	}
	got := aggregateIPBanTrend(records, 3, trendTestBase)
	b, ok := bucketByKey(got, "203.0.113.0/24")
	if !ok {
		t.Fatalf("窗口内行应产出桶, got %+v", got.Buckets)
	}
	if b.AutoTotal != 1 || b.LastSeen != trendTs(0) {
		t.Errorf("窗口外行不应计入桶（AutoTotal=1、LastSeen=窗口内行）, got %+v", b)
	}
	if got.EarliestTs != outsideTs {
		t.Errorf("EarliestTs 应含窗口外 ip_ban 行, want %d, got %d", outsideTs, got.EarliestTs)
	}
}

// TestAggregateIPBanTrend_EarliestTsSemantics earliestTs 语义三分支：空快照
// （无 ip_ban 行）为 0；非 ip_ban 事件不计；解析失败行也计入（该行确为 ip_ban
// 事件，是「数据自何时起」的证据）。
func TestAggregateIPBanTrend_EarliestTsSemantics(t *testing.T) {
	if got := aggregateIPBanTrend(nil, 3, trendTestBase); got.EarliestTs != 0 || got.Buckets == nil || len(got.Buckets) != 0 {
		t.Fatalf("空快照应 EarliestTs=0 且空桶切片（非 nil）, got %+v", got)
	}
	onlyOther := []AuditRecord{{Ts: trendTs(0), Event: AuditEventAuthFailure, IP: "203.0.113.7"}}
	if got := aggregateIPBanTrend(onlyOther, 3, trendTestBase); got.EarliestTs != 0 || len(got.Buckets) != 0 {
		t.Fatalf("非 ip_ban 事件不应计入 earliestTs 或桶, got %+v", got)
	}
	brokenTs := trendTs(-time.Hour)
	mixed := []AuditRecord{
		trendBanRec(brokenTs, "garbage", AuditActorSystem), // 双败行：不进桶但计入 earliestTs
		trendBanRec(trendTs(0), "203.0.113.7", AuditActorSystem),
	}
	got := aggregateIPBanTrend(mixed, 3, trendTestBase)
	if got.EarliestTs != brokenTs {
		t.Errorf("解析失败 ip_ban 行也应计入 earliestTs, want %d, got %d", brokenTs, got.EarliestTs)
	}
	if len(got.Buckets) != 1 {
		t.Errorf("解析失败行不应产出桶, got %+v", got.Buckets)
	}
}

// TestAggregateIPBanTrend_Rebounded 解封后再犯判定三分支：unban 后同桶再 ban =
// true；unban 后无再 ban = false；多 IP 桶任一 IP 再犯 = true（桶级观测口径）。
// 另覆盖：窗口外的 unban 被裁，窗口内事实无解封证据恒 false。
func TestAggregateIPBanTrend_Rebounded(t *testing.T) {
	t.Run("unban后同桶再ban为true", func(t *testing.T) {
		records := []AuditRecord{
			trendBanRec(trendTs(-72*time.Hour), "203.0.113.7", AuditActorSystem), // ban1
			trendUnbanRec(trendTs(-48*time.Hour), "203.0.113.7"),                 // 解封
			trendBanRec(trendTs(-24*time.Hour), "203.0.113.7", AuditActorSystem), // ban2（再犯）
		}
		got := aggregateIPBanTrend(records, 5, trendTestBase)
		b, ok := bucketByKey(got, "203.0.113.0/24")
		if !ok || !b.Rebounded {
			t.Fatalf("解封后再封禁应置 rebounded, got %+v", b)
		}
	})
	t.Run("unban后无再ban为false", func(t *testing.T) {
		records := []AuditRecord{
			trendBanRec(trendTs(-72*time.Hour), "203.0.113.7", AuditActorSystem),
			trendUnbanRec(trendTs(-48*time.Hour), "203.0.113.7"),
		}
		got := aggregateIPBanTrend(records, 5, trendTestBase)
		b, ok := bucketByKey(got, "203.0.113.0/24")
		if !ok || b.Rebounded {
			t.Fatalf("解封后无再封禁不应置 rebounded, got %+v", b)
		}
	})
	t.Run("多IP桶任一再犯为true", func(t *testing.T) {
		records := []AuditRecord{
			trendBanRec(trendTs(-72*time.Hour), "203.0.113.7", AuditActorSystem), // IP1 ban
			trendUnbanRec(trendTs(-48*time.Hour), "203.0.113.7"),                 // IP1 解封（此后 IP1 无再犯）
			trendBanRec(trendTs(-24*time.Hour), "203.0.113.9", AuditActorSystem), // IP2 在 IP1 解封后触发（同桶）
		}
		got := aggregateIPBanTrend(records, 5, trendTestBase)
		b, ok := bucketByKey(got, "203.0.113.0/24")
		if !ok || !b.Rebounded {
			t.Fatalf("多 IP 桶任一再犯应置 rebounded（桶级观测）, got %+v", b)
		}
	})
	t.Run("窗口外unban不构成窗口内再犯证据", func(t *testing.T) {
		records := []AuditRecord{
			trendUnbanRec(trendTs(-120*time.Hour), "203.0.113.7"), // 10-23 15:04，早于 5 天窗口起点 10-24 00:00
			trendBanRec(trendTs(0), "203.0.113.7", AuditActorSystem),
		}
		got := aggregateIPBanTrend(records, 5, trendTestBase)
		b, ok := bucketByKey(got, "203.0.113.0/24")
		if !ok || b.Rebounded {
			t.Fatalf("窗口外解封被裁, 窗口内无解封证据不应置 rebounded, got %+v", b)
		}
	})
	t.Run("同IP多轮攻防取桶级最早解封", func(t *testing.T) {
		// ban1 → unban1 → ban2（unban1 后再犯）→ unban2（最新解封，此后无 ban）。
		// per-IP 最新解封（unban2）会把 ban2（合法解封周期内的封禁）漏判为非
		// 再犯；桶级口径取桶内全部解封的最早时刻（unban1），ban2 > unban1 判定
		// 再犯成立——「该桶有解封后再犯」的观测事实（见 doc comment 理由）。
		records := []AuditRecord{
			trendBanRec(trendTs(-96*time.Hour), "203.0.113.7", AuditActorSystem),
			trendUnbanRec(trendTs(-72*time.Hour), "203.0.113.7"),
			trendBanRec(trendTs(-48*time.Hour), "203.0.113.7", AuditActorSystem),
			trendUnbanRec(trendTs(-24*time.Hour), "203.0.113.7"),
		}
		got := aggregateIPBanTrend(records, 7, trendTestBase)
		b, ok := bucketByKey(got, "203.0.113.0/24")
		if !ok || !b.Rebounded {
			t.Fatalf("两轮攻防的桶级再犯事实应置 rebounded, got %+v", b)
		}
	})
}

// TestAggregateIPBanTrend_SortOrder 排序两级：AutoTotal 降序在前，同计数按 Key
// 字典序升序（确定性输出，对齐九期排序纪律）。
func TestAggregateIPBanTrend_SortOrder(t *testing.T) {
	records := []AuditRecord{
		// 198.51.100.0/24 计数 2、192.0.2.0/24 计数 2、203.0.113.0/24 计数 3
		trendBanRec(trendTs(0), "198.51.100.5", AuditActorSystem),
		trendBanRec(trendTs(0), "198.51.100.6", AuditActorSystem),
		trendBanRec(trendTs(0), "192.0.2.5", AuditActorSystem),
		trendBanRec(trendTs(0), "192.0.2.6", AuditActorSystem),
		trendBanRec(trendTs(0), "203.0.113.7", AuditActorSystem),
		trendBanRec(trendTs(0), "203.0.113.9", AuditActorSystem),
		trendBanRec(trendTs(0), "203.0.113.11", AuditActorSystem),
	}
	got := aggregateIPBanTrend(records, 3, trendTestBase)
	want := []string{"203.0.113.0/24", "192.0.2.0/24", "198.51.100.0/24"}
	if len(got.Buckets) != len(want) {
		t.Fatalf("应产出 3 条桶, got %+v", got.Buckets)
	}
	for i, w := range want {
		if got.Buckets[i].Key != w {
			t.Errorf("第 %d 条应为 %q（计数降序 + 同计数字典序）, got %q", i, w, got.Buckets[i].Key)
		}
	}
}

// TestAggregateIPBanTrend_ManualOnlyBucket 桶由 manual 行创建的形态：Days 全零、
// AutoTotal 0、ManualCount 计数、rebounded 恒 false（再犯判定只看 auto 行）。
func TestAggregateIPBanTrend_ManualOnlyBucket(t *testing.T) {
	records := []AuditRecord{
		trendBanRec(trendTs(0), "203.0.113.0/24", AuditActorDesktop), // 网段手动封禁
	}
	got := aggregateIPBanTrend(records, 3, trendTestBase)
	b, ok := bucketByKey(got, "203.0.113.0/24")
	if !ok {
		t.Fatalf("manual 行也应创建桶, got %+v", got.Buckets)
	}
	if b.AutoTotal != 0 || b.ManualCount != 1 || b.Rebounded {
		t.Errorf("manual 桶应 AutoTotal=0/ManualCount=1/rebounded=false, got %+v", b)
	}
	for i, d := range b.Days {
		if d != 0 {
			t.Errorf("Days 仅累计 auto 行, 下标 %d 应为 0, got %d", i, d)
		}
	}
}

// TestIPBanService_QueryAuditTrend_EmptyAudit 服务查询面空态：audit 未注入（旁路
// 构造）返回空结果不报错（WindowDays 照常透出、Buckets 空切片、EarliestTs 0）。
func TestIPBanService_QueryAuditTrend_EmptyAudit(t *testing.T) {
	svc, _ := newTestIPBanService(t)
	got, err := svc.QueryAuditTrend()
	if err != nil {
		t.Fatalf("audit 未注入应空结果不报错: %v", err)
	}
	if got.WindowDays != IPBanTrendWindowDaysDefault {
		t.Errorf("WindowDays 应透出构造默认值 %d, got %d", IPBanTrendWindowDaysDefault, got.WindowDays)
	}
	if got.Buckets == nil || len(got.Buckets) != 0 || got.EarliestTs != 0 {
		t.Errorf("空态应为空桶切片 + EarliestTs 0, got %+v", got)
	}
}

// TestIPBanService_QueryAuditTrend_SnapshotDelegation 服务查询面真实链路：读侧
// 懒扫审计快照（readSnapshot 单点），ip_ban/ip_unban 行落桶（窗口参数取
// trendWindowDays），写侧动作生效后落盘的审计行天然进入聚合（对齐审计写入时序
// 纪律——聚合读侧只见已生效事实）。时钟口径：审计行 Ts 与 RecordFailure 均取
// time.Now（生产默认构造），断言只依赖两时钟同源的相对面（计数/长度/非零），
// 天桶精确分布由纯函数注入时钟用例覆盖（规避测试运行跨本地日界的零点漂移）。
func TestIPBanService_QueryAuditTrend_SnapshotDelegation(t *testing.T) {
	svc := NewIPBanService(nil)
	audit := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	svc.SetAudit(audit)

	// 经真实状态机产生审计行：同一 /24 两个来源各触发一次自动封禁（auto 行），
	// 并手动封禁同网段一个新来源（manual 行）+ 写入一条无关事件
	for _, ip := range []string{"203.0.113.7", "203.0.113.9"} {
		for i := 0; i < IPBanThreshold; i++ {
			svc.RecordFailure(ip)
		}
	}
	if err := svc.Ban("203.0.113.200", 0, "", AuditActorDesktop, ""); err != nil {
		t.Fatalf("手动封禁: %v", err)
	}
	audit.Log(AuditEventAuthFailure, "", "", "198.51.100.1:1234", "ua")

	got, err := svc.QueryAuditTrend()
	if err != nil {
		t.Fatalf("QueryAuditTrend: %v", err)
	}
	b, ok := bucketByKey(got, "203.0.113.0/24")
	if !ok {
		t.Fatalf("审计快照应落桶, got %+v", got.Buckets)
	}
	if b.AutoTotal != 2 || b.ManualCount != 1 {
		t.Errorf("桶计数应为 auto 2 + manual 1, got %+v", b)
	}
	if len(b.Days) != IPBanTrendWindowDaysDefault {
		t.Errorf("Days 长度应等于窗口天数, want %d, got %d", IPBanTrendWindowDaysDefault, len(b.Days))
	}
	if b.LastSeen == 0 {
		t.Error("LastSeen 应为桶内最新 ip_ban 行 ts（非零）")
	}
	if got.EarliestTs == 0 {
		t.Error("有 ip_ban 行时 EarliestTs 应非零")
	}
}

// TestIPBanService_QueryAuditTrend_WindowHotApply 趋势窗口经 ApplyConfig 热更
// （十期接设置链，对齐九期阈值热更先例）：构造默认起跑 → ApplyConfig 注入窗口 2
// → 下次查询 WindowDays/Days 长度即 2 且行计数不丢（无追溯重算面，新窗口作用于
// 后续聚合；天桶精确分布由纯函数用例覆盖，此处不断言具体下标防零点漂移）。
func TestIPBanService_QueryAuditTrend_WindowHotApply(t *testing.T) {
	svc := NewIPBanService(nil)
	if svc.trendWindowDays != IPBanTrendWindowDaysDefault {
		t.Fatalf("前置：构造应以默认窗口起跑, got %d", svc.trendWindowDays)
	}
	audit := NewAuditLogger(filepath.Join(t.TempDir(), "web_audit.log"))
	svc.SetAudit(audit)
	for i := 0; i < IPBanThreshold; i++ {
		svc.RecordFailure("203.0.113.7")
	}

	svc.ApplyConfig(model.IPBanSettings{
		Threshold:                 IPBanThreshold,
		WindowMinutes:             int(IPBanWindow / time.Minute),
		DurationMinutes:           int(IPBanDuration / time.Minute),
		MaxBanDurationMinutes:     int(IPBanMaxBanDuration / time.Minute),
		Whitelist:                 []string{},
		AggregateSuggestThreshold: IPBanAggregateSuggestThresholdDefault,
		TrendWindowDays:           2,
	})
	got, err := svc.QueryAuditTrend()
	if err != nil {
		t.Fatalf("QueryAuditTrend: %v", err)
	}
	if got.WindowDays != 2 {
		t.Errorf("热更后窗口应为 2, got %d", got.WindowDays)
	}
	b, ok := bucketByKey(got, "203.0.113.0/24")
	if !ok || len(b.Days) != 2 {
		t.Fatalf("热更后 Days 长度应为 2, got %+v", b)
	}
	if sum := b.Days[0] + b.Days[1]; sum != 1 {
		t.Errorf("窗口 2 下今天的封禁行应计入天桶, want 总和 1, got %v", b.Days)
	}
	if b.AutoTotal != 1 {
		t.Errorf("AutoTotal 应为 1, got %d", b.AutoTotal)
	}
}

// TestIPBanTrendBucketKey_Boundaries 桶归属解析边界：IPv6 /64 规范化、裸 IPv4 补
// /24 掩码、裸 IPv6 补 /64、双败 false（单测补齐 ipBanTrendBucketKey 纯函数面）。
func TestIPBanTrendBucketKey_Boundaries(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"203.0.113.7", "203.0.113.0/24", true},
		{"::ffff:203.0.113.7", "203.0.113.0/24", true},
		{"2001:db8:1:2::1", "2001:db8:1:2::/64", true},
		{"203.0.113.0/24", "203.0.113.0/24", true},
		{"203.0.113.5/24", "203.0.113.0/24", true}, // CIDR 主机位归零
		{"192.0.2.5/32", "192.0.2.5/32", true},
		{"", "", false},
		{"not-an-ip", "", false},
		{"300.1.2.3", "", false},
	}
	for _, c := range cases {
		got, ok := ipBanTrendBucketKey(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("ipBanTrendBucketKey(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestIPBanService_QueryAuditTrend_ReadSnapshotFileReuse 审计文件复用形态：主+
// 轮转 .1 双文件行均进入聚合（readSnapshot 单点复用，零第二份 JSONL 解析逻辑）
// ——写满轮转触发 .1 后查询，两文件内的 ip_ban 行同桶累计。
func TestIPBanService_QueryAuditTrend_ReadSnapshotFileReuse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "web_audit.log")
	audit := NewAuditLogger(path)
	audit.maxBytes = 1 // 极小上限：下一行即轮转（生产 webAuditMaxBytes，测试注入）
	svc, _ := newTestIPBanService(t)
	svc.SetAudit(audit)

	for i := 0; i < IPBanThreshold; i++ {
		svc.RecordFailure("203.0.113.7") // 达阈值触发封禁 → ip_ban 行 1
	}
	for i := 0; i < IPBanThreshold; i++ {
		svc.RecordFailure("203.0.113.9") // 第二来源达阈值 → ip_ban 行 2（写入前触发轮转）
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("前置：应已轮转出 .1 文件: %v", err)
	}

	got, err := svc.QueryAuditTrend()
	if err != nil {
		t.Fatalf("QueryAuditTrend: %v", err)
	}
	b, ok := bucketByKey(got, "203.0.113.0/24")
	if !ok || b.AutoTotal != 2 {
		t.Fatalf("主+.1 双文件的 ip_ban 行应同桶累计 auto 2, got %+v", got.Buckets)
	}
}
