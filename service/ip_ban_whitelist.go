package service

import (
	"net"
	"strings"
)

// 连接管控七期：可信网段白名单豁免（CIDR）。回环之外的唯一豁免口径单点——
// RecordFailure（白名单失败完全不计数不进队列）、RejectMessage（生效中封禁
// 追溯放行）、Ban（封禁白名单/回环 IP 拒绝）三处消费共用 isWhitelistedIP，
// 禁再散落第二套判定（口径漂移 = 静默漏豁免或误豁免，无报错面）。

// isWhitelistedIP 白名单豁免判定单点（nets 为 ApplyConfig 锁内预解析的网段集，
// 调用方须持 IPBanService.mu）：回环地址（127.0.0.1/::1，原有豁免口径收编）
// 或命中任一可信网段即豁免。nets 为 nil/空（未配置）时仅回环豁免（既有语义
// 零回归）。非法 IP 字符串（net.ParseIP 为 nil）不视为豁免——解析失败的地址
// 按普通来源参与计数，对齐 isLoopbackIP 既有防御。
func isWhitelistedIP(ip string, nets []*net.IPNet) bool {
	if isLoopbackIP(ip) {
		return true
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// parseIPBanWhitelistEntry 解析单条白名单条目为 CIDR 网段：CIDR 串直接解析；
// 裸 IP 自动补 /32（IPv4）或 /128（IPv6）——用户输入单个可信出口 IP 是高频
// 形态，不强制手写掩码。前后空白容忍（model.NormalizeIPBanSettings 已剔除，
// 此处防御保留）。非法返回 false——条目在 normalize 层已被剔除，此处再拦一手
// 手改 settings.json 绕过 normalize 的直灌形态（不入网段集，判定面不放大）。
func parseIPBanWhitelistEntry(item string) (*net.IPNet, bool) {
	item = strings.TrimSpace(item)
	if item == "" {
		return nil, false
	}
	if _, ipnet, err := net.ParseCIDR(item); err == nil {
		return ipnet, true
	}
	if ip := net.ParseIP(item); ip != nil {
		suffix := "/32"
		if ip.To4() == nil {
			suffix = "/128"
		}
		if _, ipnet, err := net.ParseCIDR(item + suffix); err == nil {
			return ipnet, true
		}
	}
	return nil, false
}
