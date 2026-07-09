package util

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// IsPreviewable 判断文件是否可预览
func IsPreviewable(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))

	previewableExts := []string{
		".txt", ".md", ".markdown",
		".json", ".xml", ".yaml", ".yml",
		".js", ".ts", ".vue", ".go",
		".java", ".py", ".c", ".cpp",
		".html", ".css", ".sh", ".bat",
		".gitignore", ".env",
	}

	for _, pe := range previewableExts {
		if ext == pe {
			return true
		}
	}

	return false
}

// FormatFileSize 格式化文件大小
func FormatFileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// ReadFileSafe 安全读取文件（限制大小）
func ReadFileSafe(filePath string, maxSize int64) ([]byte, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	if info.Size() > maxSize {
		return nil, fmt.Errorf("file too large: %d bytes", info.Size())
	}

	return os.ReadFile(filePath)
}

// CreateDirectory 创建目录
func CreateDirectory(path string) error {
	return os.MkdirAll(path, 0755)
}

// CreateFile 创建文件
func CreateFile(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if content != "" {
		_, err = file.WriteString(content)
		if err != nil {
			return err
		}
	}

	return nil
}

// RenamePath 重命名
func RenamePath(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// RemovePath 删除
func RemovePath(path string) error {
	return os.RemoveAll(path)
}

// CopyFile 复制单个文件（保留权限）
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// CopyDir 递归复制目录
func CopyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// DetectTextEncoding 检测字节流的文本编码并尝试转码为 UTF-8。
// 返回 (encoding, content, ok)：
//   - 含 NUL 字节(0x00)（仅扫前 8KB，git heuristic）-> 二进制，ok=false
//   - 合法 UTF-8 -> encoding="utf-8", content=原串, ok=true
//   - 非合法 UTF-8 -> 尝试 GBK 解码：解码后 U+FFFD 替换字符占比 ≤5% -> encoding="gbk", content=解码结果, ok=true；否则 ok=false
//   - 空数据 -> ok=true, encoding="utf-8", content=""（空文本可显示）
//
// 设计取舍：GBK 是双字节编码，某些二进制数据恰好构成合法 GBK 序列会被误判为文本，
// 用 U+FFFD 占比阈值兜底（>5% 视为解码失败）。可接受少量误判，用户仍可"用默认程序打开"。
func DetectTextEncoding(data []byte) (encoding, content string, ok bool) {
	if len(data) == 0 {
		return "utf-8", "", true
	}

	// NUL 字节检测：仅扫前 8KB（性能好，与 git heuristic 一致），命中即判为二进制
	scanEnd := len(data)
	if scanEnd > 8192 {
		scanEnd = 8192
	}
	if bytes.IndexByte(data[:scanEnd], 0) != -1 {
		return "", "", false
	}

	// 合法 UTF-8 -> 直接返回原串
	if utf8.Valid(data) {
		return "utf-8", string(data), true
	}

	// 非合法 UTF-8 -> 尝试 GBK 解码（失败插入 U+FFFD 替换字符，不返回 err）
	decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(data)
	if err != nil {
		return "", "", false
	}
	// 解码后 U+FFFD 替换字符占比超阈值 -> 视为解码质量差（二进制/其他编码）
	if replacementCharRatio(decoded) > 0.05 {
		return "", "", false
	}
	return "gbk", string(decoded), true
}

// replacementCharRatio 计算 UTF-8 字节流中 U+FFFD 替换字符的 rune 占比。
// 用于 GBK 解码后判断解码质量：占比高说明大量字节无法解码 -> 视为解码失败。
func replacementCharRatio(b []byte) float64 {
	if len(b) == 0 {
		return 0
	}
	runeCount := utf8.RuneCount(b)
	if runeCount == 0 {
		return 0
	}
	replCount := bytes.Count(b, []byte("�"))
	return float64(replCount) / float64(runeCount)
}
