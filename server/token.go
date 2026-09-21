package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultTokenFile serve 模式访问令牌默认持久化文件名（位于 data/ 目录下）。
// 独立文件不与其他 data/*.json 产品数据耦合，避免被各服务的整体覆写逻辑碰触。
const DefaultTokenFile = "web_token"

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
