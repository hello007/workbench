package service

import (
	"net"
	"sort"

	"workbench/model"
)

// 连接管控九期：攻击源网段聚合建议（「自动观测 → 手动处置」建议闭环的数据面）。
// 七/八期覆盖单 IP 与网段的手动封禁，但同网段多攻击源需管理员自行发现——分散的
// 单 IP 自动封禁条目不产生网段级洞察。本文件把 entries 中同聚合网段的自动封禁
// 条目聚合为网段封禁建议，供处置面提示与一键预填；闭环止于建议，处置仍须管理员
// 确认提交（不自动封禁）。聚合口径与阈值判定的唯一实现点在此，禁散落第二套。

// IPBanAggregateSuggestThresholdDefault 网段聚合建议默认阈值（连接管控九期）：
// 同聚合网段内参与聚合的自动封禁条目数达到该值即产出网段封禁建议。构造即用本
// 默认值起跑；运行期值由设置段 aggregateSuggestThreshold（默认同值，clamp 2-50）
// 经 ApplyConfig 注入，本常量保留作默认值语义（对齐五期参数常量在六期参数化后
// 的角色——导出供测试引用与文档锚点）。
const IPBanAggregateSuggestThresholdDefault = 3

// aggregateIPsToSuggestions 将参与聚合的来源 IP 列表按固定粒度分桶计数并产出
// 网段封禁建议（纯函数，无锁无状态——服务方法收集参与条目后委托本函数，聚合
// 口径可独立单测）：
//   - 聚合粒度写死 IPv4 /24、IPv6 /64：网段级攻击源在同粒度内的 IP 漂移不再
//     绕过建议，粒度不随白名单/封禁条目掩码推导（可预期的固定洞察口径）；
//   - ParseIP 失败的条目跳过（防御：调用方传入的 IP 经 normalizeIP 归一后理论
//     可解析，脏数据不放大为 panic 也不计入任何桶）；
//   - 桶内计数 ≥ threshold（严格不小于，N-1 不产出）才产出建议；
//   - CIDR 串为规范化形态（网络地址 + 掩码的 ipnet.String()，如
//     "203.0.113.0/24"），可直接作为 BanCIDR 入参复用八期解析口径；
//   - 排序：计数降序（影响面大优先），同计数按网段串字典序升序（确定性输出，
//     前端渲染顺序稳定）。
//
// threshold ≤ 0 时全部桶产出（纯计数语义不在此收敛——设置段的 clamp 归一是
// 唯一收敛点，见 model.NormalizeIPBanSettings）。返回为新建切片，空态返回空
// 切片（非 nil）。
func aggregateIPsToSuggestions(ips []string, threshold int) []model.IPBanAggregateSuggestion {
	counts := make(map[string]int)
	for _, raw := range ips {
		ip := net.ParseIP(raw)
		if ip == nil {
			continue
		}
		ones, bits := 64, 128
		if ip.To4() != nil {
			ones, bits = 24, 32
		}
		mask := net.CIDRMask(ones, bits)
		ipnet := &net.IPNet{IP: ip.Mask(mask), Mask: mask}
		counts[ipnet.String()]++
	}
	out := make([]model.IPBanAggregateSuggestion, 0, len(counts))
	for cidr, count := range counts {
		if count < threshold {
			continue
		}
		out = append(out, model.IPBanAggregateSuggestion{CIDR: cidr, Count: count})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].CIDR < out[j].CIDR
	})
	return out
}

// AggregateSuggestions 返回网段封禁聚合建议（连接管控九期；处置动作由管理员
// 在网段封禁弹窗确认提交，本方法为纯只读查询面，不做任何自动封禁）。
//
// 参与口径（写死，禁分叉）：
//   - 仅内存 entries 中 source=auto 且 bannedAt 非零的条目参与——手动条目是
//     管理员已处置面不再建议；纯失败计数条目（未触发封禁，bannedAt 零值）
//     无封禁事实不参与；**含已过期条目**——与 List 可见条目同口径（所见即所
//     计，懒过期记录保留到重启，建议随之存续到重启）；
//   - 被生效中网段封禁覆盖（cidrCoveredBy 命中，与 List 的覆盖标注同源判定）
//     的条目剔除——网段已处置，对同网段再出建议即重复；ParseIP 失败条目同样
//     剔除（解析失败的地址无法归入聚合桶）；
//   - 白名单命中不特判：RecordFailure 对白名单来源早退不建条目，entries 中
//     理论上不含白名单 IP。
//
// 数据源刻意不用审计行：审计含已解封历史（解封来源不再是攻击面）且受 5MB
// 轮转边界漂移影响，聚合口径糊；内存条目与 List 所见一致，重启清零与自动封禁
// 「短时限、被动防线」定位自洽。
//
// 阈值取 suggestThreshold（构造默认值起跑，运行期经 ApplyConfig 注入设置段
// 归一值）。返回为新建切片，调用方修改不影响服务状态；空态返回空切片（非
// nil）。
func (s *IPBanService) AggregateSuggestions() []model.IPBanAggregateSuggestion {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.nowFn()
	ips := make([]string, 0, len(s.entries))
	for _, e := range s.entries {
		if e.source != model.IPBanSourceAuto || e.bannedAt.IsZero() {
			continue
		}
		parsed := net.ParseIP(e.ip)
		if parsed == nil {
			continue
		}
		if s.cidrCoveredBy(parsed, nil, now) != "" {
			continue
		}
		ips = append(ips, e.ip)
	}
	return aggregateIPsToSuggestions(ips, s.suggestThreshold)
}
