package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"

	"workbench/util"
)

// 自签 TLS 证书持久化文件名（位于 data/ 目录下）。独立文件不与其他 data/ 产品
// 数据耦合，对齐 web_token 单文件先例：证书 0644（公钥信息可公开）、私钥 0600
// （仅属主可读写，限制本机其他用户读取；Windows 上 NTFS 权限位不生效，由用户
// 目录 ACL 兜底，与 token.go 0600 先例同口径）。
const (
	DefaultTLSCertFile = "tls_cert.pem"
	DefaultTLSKeyFile  = "tls_key.pem"
)

// selfSignedCertValidity 自签证书有效期（10 年）。
//
// 决策记录：自签引导证书服务于「本机/内网临时访问」场景，浏览器信任靠「继续
// 前往」例外（按证书指纹记忆）或手动导入。有效期越长，证书过期导致的「信任
// 例外全体失效 + 警告复发」运维噪声越少；自签证书不受公共 CA 的 398 天上限
// 约束，10 年对个人工具为合理取值（对齐 research/web-tls-compat.md §3.2）。
const selfSignedCertValidity = 10 * 365 * 24 * time.Hour

// EnsureSelfSignedCert 确保自签 TLS 证书对存在并可用，返回证书与私钥文件路径。
//
// 复用语义：证书与私钥文件均已存在且可解析（tls.LoadX509KeyPair 成功 = 证书
// 解析、私钥解析、二者配对全部通过）且 NotAfter 未过期时直接复用，不做任何
// 写入——自签证书的浏览器信任例外与手动导入均绑定证书本体，重新生成即无声
// 作废用户已建立的信任。
//
// fail-fast 语义（刻意不静默重生成，research 清单明确项）：文件损坏、配对失败
// 或已过期时返回错误，由调用方提示用户手动删除文件后重试。静默重生成会让浏览
// 器证书例外失效且用户无从排查（警告复发却找不到原因）。
//
// 生成规格：ECDSA P-256 自签（兼容面最稳的默认，全部现代浏览器支持）、
// NotBefore now-24h（防本机时钟偏移导致「尚未生效」拒绝）、NotAfter 10 年、
// SAN 必填且覆盖 127.0.0.1 / ::1 / localhost + 本机全部非回环 IPv4（现代浏览
// 器对服务器证书的域名匹配只看 SAN，CN 自 Chrome 58 起不参与匹配，仅作展示）
// + 主机名、KeyUsage DigitalSignature|KeyEncipherment、ExtKeyUsage ServerAuth、
// IsCA=true（自签叶证书既是根也是叶，带 CA 位便于用户手动导入信任存储）。
func EnsureSelfSignedCert(dataDir string) (certPath, keyPath string, err error) {
	certPath = filepath.Join(dataDir, DefaultTLSCertFile)
	keyPath = filepath.Join(dataDir, DefaultTLSKeyFile)

	_, certStatErr := os.Stat(certPath)
	_, keyStatErr := os.Stat(keyPath)
	switch {
	case certStatErr == nil && keyStatErr == nil:
		return certPath, keyPath, validateExistingPair(certPath, keyPath)
	case certStatErr != nil && keyStatErr != nil:
		// 双缺失：首次启用 TLS 的正常路径，走生成
	default:
		return "", "", fmt.Errorf(
			"TLS 证书与私钥文件不完整（仅存在其一: %s / %s），不自动重新生成以免浏览器信任失效；请删除残留文件后重试",
			certPath, keyPath)
	}

	return certPath, keyPath, generateSelfSignedCert(certPath, keyPath)
}

// validateExistingPair 校验既有证书对可复用：解析 + 配对成功且未过期。
// 任何失败均返回错误（fail-fast 不重生成），错误消息带人工处置引导。
func validateExistingPair(certPath, keyPath string) error {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return fmt.Errorf(
			"TLS 证书文件损坏或证书私钥不配对（%s / %s），不自动重新生成以免浏览器信任失效；请删除两个文件后重试: %w",
			certPath, keyPath, err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf(
			"TLS 证书解析失败（%s），不自动重新生成；请删除 %s 与 %s 后重试: %w",
			certPath, certPath, keyPath, err)
	}
	if time.Now().After(leaf.NotAfter) {
		return fmt.Errorf(
			"TLS 自签证书已过期（NotAfter=%s），不自动重新生成；请删除 %s 与 %s 后重试（重新启用 TLS 时将生成新证书）",
			leaf.NotAfter.Format(time.RFC3339), certPath, keyPath)
	}
	return nil
}

// generateSelfSignedCert 生成自签证书对并落盘（私钥 0600、证书 0644）。
// 非原子写（对齐 SaveToken 先例：文件只在首次生成时写一次，后续只读，截断写
// 的崩溃窗口可忽略）；极端中断可能残留单边文件，下次启动命中「文件不完整」
// fail-fast，由用户删除后重试，不会静默换证书。
func generateSelfSignedCert(certPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("生成 TLS 私钥失败: %w", err)
	}
	// 128 位随机正序列号（RFC 5280 建议非确定序列号）
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("生成证书序列号失败: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "WorkBench 自签证书", Organization: []string{"WorkBench"}},
		NotBefore:             now.Add(-24 * time.Hour),
		NotAfter:              now.Add(selfSignedCertValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	// SAN 覆盖本机全部非回环 IPv4（同网段设备经 IP 直连访问必须命中证书）；
	// 枚举失败降级为仅回环 SAN（本机访问不受影响），不阻塞 TLS 启用
	if ips, err := util.LocalIPv4Addresses(); err == nil {
		seen := map[string]bool{"127.0.0.1": true, "::1": true}
		for _, s := range ips {
			ip := net.ParseIP(s)
			if ip == nil || seen[ip.String()] {
				continue
			}
			seen[ip.String()] = true
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		}
	}
	// 主机名一并覆盖（本机经 hostname 访问的形态）
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		tmpl.DNSNames = append(tmpl.DNSNames, hostname)
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("生成自签证书失败: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("序列化 TLS 私钥失败: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return fmt.Errorf("创建 TLS 证书目录失败: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return fmt.Errorf("持久化 TLS 私钥失败（%s）: %w", keyPath, err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("持久化 TLS 证书失败（%s）: %w", certPath, err)
	}
	return nil
}
