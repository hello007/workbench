package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
