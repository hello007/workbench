package util

import "net"

// LocalIPv4Addresses 枚举本机全部启用的非回环接口的 IPv4 地址。
//
// 消费方：浏览器访问通道候选访问地址计算（webAccessUrls）与自签 TLS 证书
// SAN 生成（server.EnsureSelfSignedCert）——两处必须同源，保证「提示可达的
// 地址」与「证书覆盖的地址」一致。
//
// 枚举失败返回错误（调用方降级为仅回环地址/回环 SAN）；无可用地址返回空切片。
// 链路本地地址（169.254.x.x）不可作为稳定访问入口，一并排除。
func LocalIPv4Addresses() ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var ips []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				ips = append(ips, ip.String())
			}
		}
	}
	return ips, nil
}
