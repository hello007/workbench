package util

import (
	"fmt"
	"io"
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
