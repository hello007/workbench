package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultTokenFile serve 模式访问令牌默认持久化文件名（位于 data/ 目录下）。
// 独立文件不与其他 data/*.json 产品数据耦合，避免被各服务的整体覆写逻辑碰触。
// 多 token 语义下（2026-10 三期 PR1）本文件退化为「明文兼容显示文件」：初始
// 引导与轮换编排会把当前「默认」令牌明文同步写该文件，保住设置页「查看令牌」
// 与 serve 控制台提示链路；多 token 记录本体（只含哈希）在 data/web_tokens.json。
const DefaultTokenFile = "web_token"

// TokenVerifier 令牌校验函数签名：入参为请求携带的明文凭据候选（header/
// cookie/查询参数/子协议通道提取），返回命中的 token 记录 ID（审计归因键）
// 与是否有效。生产实现为 service.WebTokenService.Verify（闭包持服务引用，
// 新建/撤销/过期即时生效，无需热轮换通知）。
type TokenVerifier func(candidate string) (tokenID string, ok bool)

// StaticVerifier 单 token 校验器：恒定时间比较 candidate 与固定 token。
// 供测试与冒烟用例注入（生产认证链走 WebTokenService）；token 为空串时恒
// 拒绝（fail closed，对齐原「空令牌未配置」语义）。命中返回伪 ID "static"
// （单 token 形态无记录 ID，审计归因字段留此占位）。
func StaticVerifier(token string) TokenVerifier {
	return func(candidate string) (string, bool) {
		if token == "" {
			return "", false
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) == 1 {
			return "static", true
		}
		return "", false
	}
}

// GenerateToken 生成 32 字节 crypto/rand 随机令牌（hex 编码，64 字符）。
func GenerateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成随机令牌失败: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// LoadOrCreateToken 读取持久化的访问令牌；文件不存在或内容为空时首次生成并落盘。
//
// path 为令牌文件完整路径（如 data/web_token）。写入权限 0600（仅属主可读写，
// 限制本机其他用户读取；Windows 上由用户目录 ACL 兜底）。父目录不存在时自动创建。
//
// 读取出现非「文件不存在」类错误（如权限不足）时直接返回错误，不静默重建，
// 避免掩盖部署环境问题。
func LoadOrCreateToken(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		token := strings.TrimSpace(string(data))
		if token != "" {
			return token, nil
		}
		// 空文件视为损坏，走重新生成
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("读取令牌文件失败: %w", err)
	}

	token, err := GenerateToken()
	if err != nil {
		return "", err
	}
	if err := SaveToken(path, token); err != nil {
		return "", err
	}
	return token, nil
}

// SaveToken 持久化访问令牌（0600，父目录自动创建）。
// 供「重新生成令牌」轮换写回，与 LoadOrCreateToken 的写入语义一致。
func SaveToken(path, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建令牌目录失败: %w", err)
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return fmt.Errorf("持久化令牌失败: %w", err)
	}
	return nil
}
