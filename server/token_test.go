package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateToken_Format 生成的令牌为 64 位 hex 且两次生成不同。
func TestGenerateToken_Format(t *testing.T) {
	token1, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if len(token1) != 64 {
		t.Errorf("令牌应为 64 字符 hex, got %d", len(token1))
	}
	if strings.ToLower(token1) != token1 {
		t.Errorf("hex 编码应为小写: %q", token1)
	}
	for _, c := range token1 {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("令牌含非 hex 字符: %q", c)
		}
	}

	token2, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken 第二次: %v", err)
	}
	if token1 == token2 {
		t.Error("两次生成的令牌不应相同")
	}
}

// TestLoadOrCreateToken_FirstRun 首次加载生成令牌并持久化。
func TestLoadOrCreateToken_FirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", DefaultTokenFile)

	token, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if token == "" {
		t.Fatal("生成的令牌不应为空")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("令牌文件未落盘: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != token {
		t.Errorf("落盘内容与返回不符: %q vs %q", got, token)
	}
}

// TestLoadOrCreateToken_ReuseExisting 已有令牌文件时读取并复用（含尾部换行容忍）。
func TestLoadOrCreateToken_ReuseExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultTokenFile)
	if err := os.WriteFile(path, []byte("existing-token\n"), 0o600); err != nil {
		t.Fatalf("准备令牌文件: %v", err)
	}

	token, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if token != "existing-token" {
		t.Errorf("应复用已有令牌, got %q", token)
	}
}

// TestLoadOrCreateToken_EmptyFileRegenerated 空令牌文件视为损坏，重新生成并覆写。
func TestLoadOrCreateToken_EmptyFileRegenerated(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultTokenFile)
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("准备空令牌文件: %v", err)
	}

	token, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if len(token) != 64 {
		t.Errorf("损坏文件应触发重新生成 64 位令牌, got %q", token)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回令牌文件: %v", err)
	}
	if strings.TrimSpace(string(data)) != token {
		t.Error("新令牌应已覆写落盘")
	}
}

// TestLoadOrCreateToken_DirIsFile 令牌路径的父级是文件（目录创建失败）时返回错误。
func TestLoadOrCreateToken_DirIsFile(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("准备阻塞文件: %v", err)
	}

	// blocker 是普通文件，以其为父目录创建必然失败
	if _, err := LoadOrCreateToken(filepath.Join(blocker, DefaultTokenFile)); err == nil {
		t.Error("目录创建失败应返回错误")
	}
}

// TestLoadOrCreateToken_ReadErrorSurfaced 读取出现非「不存在」类错误时透传错误，
// 不静默重建（path 指向目录，ReadFile 必然失败）。
func TestLoadOrCreateToken_ReadErrorSurfaced(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadOrCreateToken(dir); err == nil {
		t.Error("读取目录路径应返回错误而非静默重建")
	}
}

// TestSaveToken_RoundTrip SaveToken 持久化后 LoadOrCreateToken 读回一致（含
// 尾部换行写入形态），父目录不存在时自动创建。
func TestSaveToken_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "nested", DefaultTokenFile)

	if err := SaveToken(path, "rotated-token"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	got, err := LoadOrCreateToken(path)
	if err != nil {
		t.Fatalf("LoadOrCreateToken: %v", err)
	}
	if got != "rotated-token" {
		t.Errorf("读回令牌不符: got %q", got)
	}
}

// TestSaveToken_OverwriteExisting SaveToken 覆写已有文件（轮换语义：旧令牌被替换）。
func TestSaveToken_OverwriteExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultTokenFile)
	if err := SaveToken(path, "old-token"); err != nil {
		t.Fatalf("SaveToken 旧令牌: %v", err)
	}
	if err := SaveToken(path, "new-token"); err != nil {
		t.Fatalf("SaveToken 新令牌: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读回令牌文件: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "new-token" {
		t.Errorf("旧令牌应被覆写, got %q", got)
	}
}
