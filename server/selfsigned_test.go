package server

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"workbench/util"
)

// TestEnsureSelfSignedCert_FirstGenerate 首次生成：证书与私钥文件落盘、规格
// 符合（ECDSA P-256、自签、有效期 10 年、NotBefore 提前 24h 防时钟偏移、
// SAN 必填覆盖回环与本机候选 IPv4、KeyUsage/ExtKeyUsage、IsCA）。
func TestEnsureSelfSignedCert_FirstGenerate(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, err := EnsureSelfSignedCert(dir)
	if err != nil {
		t.Fatalf("EnsureSelfSignedCert: %v", err)
	}
	if certPath != filepath.Join(dir, DefaultTLSCertFile) || keyPath != filepath.Join(dir, DefaultTLSKeyFile) {
		t.Fatalf("返回路径应位于 dataDir 下: %s / %s", certPath, keyPath)
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("读证书文件: %v", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("读私钥文件: %v", err)
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("证书私钥应配对可用: %v", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("解析证书: %v", err)
	}

	if leaf.PublicKeyAlgorithm != x509.ECDSA {
		t.Errorf("应为 ECDSA 密钥, got %v", leaf.PublicKeyAlgorithm)
	}
	if pub, ok := leaf.PublicKey.(*ecdsa.PublicKey); !ok || pub.Curve != elliptic.P256() {
		t.Errorf("应为 ECDSA P-256, got %T", leaf.PublicKey)
	}
	if leaf.Issuer.CommonName != leaf.Subject.CommonName {
		t.Errorf("应自签（Issuer==Subject）: %q vs %q", leaf.Issuer.CommonName, leaf.Subject.CommonName)
	}
	if leaf.Subject.CommonName != "WorkBench 自签证书" {
		t.Errorf("CN 应为描述性名称, got %q", leaf.Subject.CommonName)
	}
	if validity := leaf.NotAfter.Sub(leaf.NotBefore); validity < 10*365*24*time.Hour-time.Hour {
		t.Errorf("有效期应约 10 年, got %v", validity)
	}
	if backdated := time.Since(leaf.NotBefore); backdated < 23*time.Hour {
		t.Errorf("NotBefore 应提前约 24h 防时钟偏移, got %v", backdated)
	}
	if !leaf.IsCA || !leaf.BasicConstraintsValid {
		t.Error("IsCA 应为 true（自签叶证书便于手动导入信任）")
	}
	if leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 || leaf.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		t.Errorf("KeyUsage 应含 DigitalSignature|KeyEncipherment, got %v", leaf.KeyUsage)
	}
	if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Errorf("ExtKeyUsage 应为 ServerAuth, got %v", leaf.ExtKeyUsage)
	}

	// SAN 必填：回环恒在（127.0.0.1 / ::1 / localhost）；本机枚举到的非回环
	// IPv4 必须全部命中（提示可达地址与证书覆盖地址同源）
	hasIP := func(ip string) bool {
		for _, v := range leaf.IPAddresses {
			if v.String() == ip {
				return true
			}
		}
		return false
	}
	if !hasIP("127.0.0.1") || !hasIP("::1") {
		t.Errorf("SAN 应覆盖 127.0.0.1 与 ::1, got %v", leaf.IPAddresses)
	}
	hasDNS := func(s string) bool {
		for _, v := range leaf.DNSNames {
			if v == s {
				return true
			}
		}
		return false
	}
	if !hasDNS("localhost") {
		t.Errorf("DNSNames 应含 localhost, got %v", leaf.DNSNames)
	}
	ips, err := util.LocalIPv4Addresses()
	if err == nil {
		for _, s := range ips {
			if !hasIP(s) {
				t.Errorf("SAN 应覆盖本机非回环 IPv4 %s, got %v", s, leaf.IPAddresses)
			}
		}
	}

	// PEM 形态合法（供用户手动查看/导入）
	if !bytes.Contains(certPEM, []byte("-----BEGIN CERTIFICATE-----")) {
		t.Error("证书文件应为 PEM CERTIFICATE 形态")
	}
	if !bytes.Contains(keyPEM, []byte("-----BEGIN EC PRIVATE KEY-----")) {
		t.Error("私钥文件应为 PEM EC PRIVATE KEY 形态")
	}
	// 落盘权限尽力断言（Windows 权限位不生效，跳过）
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(keyPath); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("私钥权限应为 0600, got %v", info.Mode().Perm())
		}
		if info, err := os.Stat(certPath); err != nil || info.Mode().Perm() != 0o644 {
			t.Errorf("证书权限应为 0644, got %v", info.Mode().Perm())
		}
	}
}

// TestEnsureSelfSignedCert_Reuse 已有可用证书对直接复用：文件内容不变（不重写、
// 不换证书——浏览器信任例外与手动导入均绑定证书本体）。
func TestEnsureSelfSignedCert_Reuse(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, err := EnsureSelfSignedCert(dir)
	if err != nil {
		t.Fatalf("首次生成: %v", err)
	}
	certBefore, _ := os.ReadFile(certPath)
	keyBefore, _ := os.ReadFile(keyPath)

	certPath2, keyPath2, err := EnsureSelfSignedCert(dir)
	if err != nil {
		t.Fatalf("二次调用: %v", err)
	}
	if certPath2 != certPath || keyPath2 != keyPath {
		t.Fatalf("复用应返回同一路径: %s/%s vs %s/%s", certPath, keyPath, certPath2, keyPath2)
	}
	certAfter, _ := os.ReadFile(certPath)
	keyAfter, _ := os.ReadFile(keyPath)
	if !bytes.Equal(certBefore, certAfter) || !bytes.Equal(keyBefore, keyAfter) {
		t.Error("已有可用证书对应原样复用，文件内容不得变化")
	}
}

// TestEnsureSelfSignedCert_CorruptedFailFast 证书文件损坏：fail-fast 返回错误
// 且不重生成（静默重生成会无声作废用户已建立的浏览器信任且难排查）。
func TestEnsureSelfSignedCert_CorruptedFailFast(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, err := EnsureSelfSignedCert(dir)
	if err != nil {
		t.Fatalf("首次生成: %v", err)
	}
	keyBefore, _ := os.ReadFile(keyPath)

	if err := os.WriteFile(certPath, []byte("not a pem file"), 0o644); err != nil {
		t.Fatalf("写坏证书文件: %v", err)
	}
	_, _, err = EnsureSelfSignedCert(dir)
	if err == nil {
		t.Fatal("证书文件损坏时应返回错误（fail-fast 不重生成）")
	}

	certAfter, _ := os.ReadFile(certPath)
	keyAfter, _ := os.ReadFile(keyPath)
	if !bytes.Equal(certAfter, []byte("not a pem file")) || !bytes.Equal(keyAfter, keyBefore) {
		t.Error("fail-fast 路径不得改写既有文件（重生成即作废用户信任）")
	}
}

// TestEnsureSelfSignedCert_ExpiredFailFast 证书已过期：fail-fast 返回错误且不
// 重生成（对齐「损坏不静默重建」语义，由用户删除文件显式触发换证）。
func TestEnsureSelfSignedCert_ExpiredFailFast(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, DefaultTLSCertFile)
	keyPath := filepath.Join(dir, DefaultTLSKeyFile)

	// 程序化构造已过期的自签证书对
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("生成密钥: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "expired"},
		NotBefore:    time.Now().Add(-48 * time.Hour),
		NotAfter:     time.Now().Add(-24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("构造过期证书: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("序列化私钥: %v", err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatalf("写过期证书: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("写过期私钥: %v", err)
	}

	_, _, err = EnsureSelfSignedCert(dir)
	if err == nil {
		t.Fatal("证书已过期时应返回错误（fail-fast 不重生成）")
	}
}

// TestEnsureSelfSignedCert_PartialPairFailFast 证书/私钥仅存在其一：fail-fast
// 返回错误（不完整状态不可静默补齐另一半——补出的新对同样作废既有信任）。
func TestEnsureSelfSignedCert_PartialPairFailFast(t *testing.T) {
	dir := t.TempDir()
	_, keyPath, err := EnsureSelfSignedCert(dir)
	if err != nil {
		t.Fatalf("首次生成: %v", err)
	}
	if err := os.Remove(keyPath); err != nil {
		t.Fatalf("删除私钥: %v", err)
	}
	if _, _, err := EnsureSelfSignedCert(dir); err == nil {
		t.Fatal("仅证书存在时应返回错误（文件对不完整）")
	}

	// 反向形态：仅私钥存在
	dir2 := t.TempDir()
	certPath2, _, err := EnsureSelfSignedCert(dir2)
	if err != nil {
		t.Fatalf("首次生成: %v", err)
	}
	if err := os.Remove(certPath2); err != nil {
		t.Fatalf("删除证书: %v", err)
	}
	if _, _, err := EnsureSelfSignedCert(dir2); err == nil {
		t.Fatal("仅私钥存在时应返回错误（文件对不完整）")
	}
}
