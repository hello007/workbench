package service

import (
	"net"
	"sort"
	"time"

	"workbench/model"
)

// 连接管控十期：攻击源趋势面（「审计历史 → 趋势洞察」的数据面）。九期聚合建议的
// 数据源限定 entries 内存态——重启清零、仅窗口内事实，无法回答「该网段攻击持续
// 几天了」「解封后是否再犯」。本文件基于审计行 ip_ban/ip_unban 做跨重启的天级
// 时序聚合，补全九期「窗口内事实」之外的长期面。聚合口径与 rebounded 判定的
// 唯一实现点在此，禁散落第二套。

// IPBanTrendWindowDaysDefault 攻击趋势回看窗口默认天数（连接管控十期）：审计
// 历史聚合的天级时间桶数量。构造即用本默认值起跑；运行期值由设置段
// trendWindowDays（默认同值，clamp 1-30）经 ApplyConfig 注入，本常量保留作默认
// 值语义（对齐九期 IPBanAggregateSuggestThresholdDefault 在参数化后的角色——
// 导出供测试引用与文档锚点）。
const IPBanTrendWindowDaysDefault = 7

// ipBanTrendBucketKey 趋势桶归属解析（纯函数）：单 IP 掩入九期固定聚合粒度
// （IPv4 /24、IPv6 /64——与 aggregateIPsToSuggestions 同粒度同源口径，攻击源
// 网段典型漂移范围，禁动态推导）；IPv4-mapped IPv6（::ffff:a.b.c.d）经 To4()
// 判定落 /24 桶（不误入 IPv6 /64）；非 IP 串按 CIDR 解析取规范化网络地址形态
// （ipnet.String()，八期网段封禁审计行的 IP 字段形态，管理员封禁范围保真、
// 独立成桶）。两者均失败返回 false（调用方跳过该行，脏数据不 panic 不计入任何
// 桶）。
func ipBanTrendBucketKey(raw string) (string, bool) {
	if ip := net.ParseIP(raw); ip != nil {
		ones, bits := 64, 128
		if ip.To4() != nil {
			ones, bits = 24, 32
		}
		mask := net.CIDRMask(ones, bits)
		return (&net.IPNet{IP: ip.Mask(mask), Mask: mask}).String(), true
	}
	if _, ipnet, err := net.ParseCIDR(raw); err == nil {
		return ipnet.String(), true
	}
	return "", false
}

// ipBanTrendState 单桶聚合中间态（aggregateIPBanTrend 内部装配面）。
type ipBanTrendState struct {
	days          []int
	autoTotal     int
	manualCount   int
	rebounded     bool
	lastSeen      int64
	lastAuto      int64 // 桶内最新一条 auto ip_ban 行 ts（rebounded 判定比较基准）
	earliestUnban int64 // 桶内各 IP 最新解封时刻的最小值（0 = 窗口内无相关解封）
}

// aggregateIPBanTrend 将审计快照记录聚合为攻击源趋势视图（纯函数，无锁无状态
// ——服务查询面读快照后委托本函数，聚合口径可独立单测；时间参数 now 注入便于
// 测试时钟驱动，不依赖真实时钟与 sleep）。聚合口径写死如下，禁漂移：
//
//   - 数据源：审计行的 ip_ban 与 ip_unban 两事件（其余事件忽略）；读侧懒扫
//     审计主+.1 文件（复用 AuditLogger.readSnapshot 单点），零第二数据源零口径
//     漂移，留存上限 = 审计留存（5MB×2 轮转），轮转丢历史由 EarliestTs 诚实
//     标注。
//
//   - 主趋势计数仅 event=ip_ban 且 actorSource=system（auto，机器观测的攻击
//     频度）；manual（actorSource=desktop/rpc，管理员处置；非 system 一律按
//     manual 分列——审计写入单点保证 system/manual 二值，宽松兜底无第三形态）
//     分列 ManualCount 不混入主趋势（auto=攻击频度、manual=人类决策频度，混则
//     口径失真）。
//
//   - 桶归属：单 IP 掩入固定粒度 IPv4 /24、IPv6 /64（::ffff: 经 To4() 落 /24，
//     与九期聚合建议同粒度）；八期网段封禁审计行（IP 字段为规范化 CIDR 串）按
//     规范化原串独立成桶（管理员封禁范围保真；CIDR 桶恒 manual——自动封禁面
//     无 CIDR，由审计行的 actorSource 天然保证，无额外特判）；ParseIP 与
//     ParseCIDR 双败的行跳过（脏数据不放大为 panic，仍计入 EarliestTs——该行
//     确为 ip_ban 事件，是「数据自何时起」的证据）。
//
//   - 时间桶恒为天、本地时区日界（与 now 同基准）：Days 长度恒等于 windowDays，
//     下标 0 = 窗口最旧日、末下标 = 今天；窗口起点之前的记录裁掉不进桶。
//     windowDays 由设置段 clamp（1-30）保证合法，纯函数不重复收敛（≤0 时窗口
//     起点随之后移、Days 为空切片，防御性可定义形态）。
//
//   - EarliestTs：快照中最早一条 event=ip_ban 行的 ts，含窗口外与解析失败行，
//     0 = 快照无 ip_ban 行（前端据此判定「留存是否覆盖整个窗口」，不覆盖时
//     提示「数据自 M-D 起」——轮转丢历史诚实标注，禁静默降级）。
//
//   - Rebounded 解封后再犯判定：桶内全部 ip_unban 行（IP 经 ipBanTrendBucketKey
//     掩同规则归属桶）的最早解封时刻，与桶内最新一条 auto ip_ban 行 ts 比较，
//     存在 auto 行 ts 严格大于该值即置 true（「解封时间后该桶又有 auto 封禁」）。
//     取桶级最早解封而非 per-IP 最新解封的理由：同 IP 两轮攻防（ban1→unban1→
//     ban2→unban2、unban2 后无 ban3）中 ban2 是解封后再次攻击的既成事实，per-IP
//     最新值会把合法周期内的封禁漏判为非再犯；桶级最早值保住「该桶有解封后再
//     犯」的观测事实。九期 CoveredBy 剔除刻意不复用：建议面是「你可能想封」故
//     剔除已被网段封禁覆盖的条目防重复建议，趋势面是观测面「谁在攻击」——被
//     覆盖条目仍是攻击证据，保留。窗口内无相关解封（earliestUnban=0）的桶恒
//     false（窗口外的 unban 被裁，rebounded 只反映窗口内事实）。
//
//   - 桶创建：仅由窗口内的 ip_ban 行创建（只有 unban 行无 ban 行的空桶不产出
//     ——「近 N 天总数 0」的桶无展示价值）；unban 行仅参与既有桶的 rebounded
//     判定（桶级解封时刻独立收集，unban 先于同桶任一 ban 行到达时序下同样生效）。
//
//   - 排序：AutoTotal 降序（影响面大优先），同计数按 Key 字典序升序（确定性
//     输出，前端渲染顺序稳定），sort.SliceStable 对齐九期先例。
//
// 返回为新建结果（Buckets 恒非 nil 空切片），调用方修改不影响输入。
func aggregateIPBanTrend(records []AuditRecord, windowDays int, now time.Time) model.IPBanTrendResult {
	result := model.IPBanTrendResult{WindowDays: windowDays, Buckets: []model.IPBanTrendBucket{}}

	// 天桶边界：本地时区日界（now 同基准）。窗口起点 = 窗口最旧日的本地 0 点，
	// 含今天在内共 windowDays 个自然日。
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	windowStart := today.AddDate(0, 0, -(windowDays - 1))
	windowStartTs := windowStart.Unix()

	buckets := map[string]*ipBanTrendState{}
	// earliestUnbanByBucket 桶级解封时刻收集面（独立于 buckets——unban 行可能
	// 先于同桶任一 ip_ban 行到达，聚合循环结束后并入桶态，顺序无关）。
	earliestUnbanByBucket := map[string]int64{}
	// 桶 key 保序收集面（map 遍历无序，收进 slice 供稳定排序）。
	var keys []string

	// dayIndexOf 计算记录 ts 的天桶下标（0 = 窗口最旧日）。两端均为本地 0 点，
	// Sub 差值恒为 24h 整数倍 ±1h（区间内 DST 切换至多引入 1 小时偏移），+0.5
	// 四舍五入消除偏移（窗口 ≤ 30 天不会跨过 0.5 边界）。
	dayIndexOf := func(ts int64) int {
		t := time.Unix(ts, 0).In(windowStart.Location())
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		return int(day.Sub(windowStart).Hours()/24 + 0.5)
	}

	for _, rec := range records {
		switch rec.Event {
		case AuditEventIPBan:
			// EarliestTs 语义含全部 ip_ban 行（窗口外/解析失败行也是「数据自
			// 何时起」的证据），先于任何跳过分支更新
			if earliest := &result.EarliestTs; *earliest == 0 || rec.Ts < *earliest {
				*earliest = rec.Ts
			}
			if rec.Ts < windowStartTs {
				continue // 窗口外裁掉，不进桶
			}
			key, ok := ipBanTrendBucketKey(rec.IP)
			if !ok {
				continue
			}
			state, exists := buckets[key]
			if !exists {
				state = &ipBanTrendState{days: make([]int, windowDays)}
				buckets[key] = state
				keys = append(keys, key)
			}
			if rec.ActorSource == AuditActorSystem {
				state.autoTotal++
				// Days 仅累计 auto 行（主趋势=机器观测攻击频度，manual 分列
				// 不混入——天级分布与 AutoTotal 同口径）
				if idx := dayIndexOf(rec.Ts); idx >= 0 && idx < windowDays {
					state.days[idx]++
				}
				if rec.Ts > state.lastAuto {
					state.lastAuto = rec.Ts
				}
			} else {
				// 非 system 一律按 manual 分列（desktop/rpc；审计写入单点保证
				// 二值，宽松兜底无第三形态）
				state.manualCount++
			}
			if rec.Ts > state.lastSeen {
				state.lastSeen = rec.Ts
			}
		case AuditEventIPUnban:
			if rec.Ts < windowStartTs {
				continue
			}
			key, ok := ipBanTrendBucketKey(rec.IP)
			if !ok {
				continue // 解析失败的 unban 行无法关联任何桶，跳过
			}
			if cur := earliestUnbanByBucket[key]; cur == 0 || rec.Ts < cur {
				earliestUnbanByBucket[key] = rec.Ts
			}
		}
	}

	// rebounded 判定：桶内最早解封时刻 < 桶内最新 auto 行 ts 即再犯（桶级观测，
	// 见 doc comment「取桶级最早解封」理由）。
	for key, state := range buckets {
		state.earliestUnban = earliestUnbanByBucket[key]
		if state.earliestUnban != 0 && state.autoTotal > 0 {
			state.rebounded = state.lastAuto > state.earliestUnban
		}
	}

	out := make([]model.IPBanTrendBucket, 0, len(buckets))
	for _, key := range keys {
		state := buckets[key]
		out = append(out, model.IPBanTrendBucket{
			Key:         key,
			Days:        state.days,
			AutoTotal:   state.autoTotal,
			ManualCount: state.manualCount,
			Rebounded:   state.rebounded,
			LastSeen:    state.lastSeen,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AutoTotal != out[j].AutoTotal {
			return out[i].AutoTotal > out[j].AutoTotal
		}
		return out[i].Key < out[j].Key
	})
	result.Buckets = out
	return result
}

// QueryAuditTrend 查询攻击源趋势（连接管控十期，App.QueryIPBanTrend 委托面）：
// 读侧懒扫审计主+.1 快照（AuditLogger.readSnapshot 单点，同包私有直达——禁
// 制第二份 JSONL 解析逻辑），窗口参数取自身 trendWindowDays（设置段经
// ApplyConfig 注入的归一值，与聚合建议阈值同款「单一来源」纪律），聚合口径见
// aggregateIPBanTrend。
//
// 纯只读查询面，无新增错误码（审计旁路面既有降级口径够用）：audit 未注入（旁路
// 构造）返回空结果（WindowDays 照常透出、Buckets 空切片、EarliestTs 0）不报错；
// 快照读失败（非「文件不存在」的打开错误）上抛交由既有 error 链处理，前端吞错
// 静默空态（对齐 useIPBanList 降级口径）。
func (s *IPBanService) QueryAuditTrend() (model.IPBanTrendResult, error) {
	s.mu.Lock()
	windowDays := s.trendWindowDays
	now := s.nowFn()
	audit := s.audit
	s.mu.Unlock()

	empty := model.IPBanTrendResult{WindowDays: windowDays, Buckets: []model.IPBanTrendBucket{}}
	if audit == nil {
		return empty, nil
	}
	records, err := audit.readSnapshot()
	if err != nil {
		return empty, err
	}
	return aggregateIPBanTrend(records, windowDays, now), nil
}
