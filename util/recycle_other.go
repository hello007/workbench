//go:build !windows

// util/recycle_other.go
package util

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// moveToRecycleBinPlatform freedesktop Trash 规范实现（Linux 等非 Windows 平台）：
// 把文件移到 $XDG_DATA_HOME/Trash/files/<名字>，并在 Trash/info/<名字>.trashinfo
// 写入原始位置与删除时间（桌面环境据此展示「还原」入口）。
//
// 与 Windows 实现的语义对齐：失败时源文件保持原位（info 先写、移动失败即清理
// info；跨设备复制路线在删除源失败时回滚目标副本），保证调用方可以安全地按
// 「失败 = 什么都没发生」处置。
func moveToRecycleBinPlatform(path string) error {
	trashRoot, err := trashDataDir()
	if err != nil {
		return err
	}
	filesDir := filepath.Join(trashRoot, "files")
	infoDir := filepath.Join(trashRoot, "info")
	if err := os.MkdirAll(filesDir, 0700); err != nil {
		return fmt.Errorf("无法移入回收站：创建回收站目录失败: %w", err)
	}
	if err := os.MkdirAll(infoDir, 0700); err != nil {
		return fmt.Errorf("无法移入回收站：创建回收站目录失败: %w", err)
	}

	// 条目名冲突时追加 .<n>（files 与 info 两侧同时判重，避免 info 与文件脱钩）
	base := filepath.Base(path)
	name := base
	for n := 1; ; n++ {
		if !pathExists(filepath.Join(filesDir, name)) && !pathExists(filepath.Join(infoDir, name+".trashinfo")) {
			break
		}
		name = base + "." + strconv.Itoa(n)
	}

	// info 先行：rename 成功后 info 写入失败会让回收站里出现无主条目（无法从
	// 桌面还原），先写 info 则失败可在移动前清理，源文件零风险
	infoPath := filepath.Join(infoDir, name+".trashinfo")
	if err := os.WriteFile(infoPath, []byte(trashInfoContent(path)), 0644); err != nil {
		return fmt.Errorf("无法移入回收站：写入回收站元数据失败: %w", err)
	}

	dst := filepath.Join(filesDir, name)
	if err := os.Rename(path, dst); err != nil {
		if !errors.Is(err, syscall.EXDEV) {
			_ = os.Remove(infoPath)
			return fmt.Errorf("无法移入回收站（%s）：%w", path, err)
		}
		// 跨文件系统（$XDG_DATA_HOME 与源目录分属不同挂载点）：退化为「复制 +
		// 删源」。复制失败不动源文件；删除源失败则回滚目标副本，保持源完好。
		if copyErr := copyPathForTrash(path, dst); copyErr != nil {
			_ = os.RemoveAll(dst)
			_ = os.Remove(infoPath)
			return fmt.Errorf("无法移入回收站（%s）：跨设备复制失败: %w", path, copyErr)
		}
		if rmErr := os.RemoveAll(path); rmErr != nil {
			_ = os.RemoveAll(dst)
			_ = os.Remove(infoPath)
			return fmt.Errorf("无法移入回收站（%s）：删除源文件失败: %w", path, rmErr)
		}
	}
	return nil
}

// trashDataDir 回收站根目录：$XDG_DATA_HOME/Trash，缺省 ~/.local/share/Trash。
func trashDataDir() (string, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("无法移入回收站：无法定位用户主目录: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "Trash"), nil
}

// trashInfoContent 生成 .trashinfo 内容（freedesktop 规范格式）：
//
//	[Trash Info]
//	Path=<URL 编码的绝对路径>
//	DeletionDate=YYYY-MM-DDTHH:MM:SS
//
// Path 用 URL 百分号编码（保留「/」分隔符），与桌面环境解析口径一致。
func trashInfoContent(path string) string {
	encoded := (&url.URL{Path: path}).EscapedPath()
	return "[Trash Info]\nPath=" + encoded + "\nDeletionDate=" +
		time.Now().Format("2006-01-02T15:04:05") + "\n"
}

// copyPathForTrash 跨设备移动的复制段：文件流式复制，目录递归（回收站语义
// 允许整目录移入，当前调用方只用文件，但实现保持通用避免后续误用）。
func copyPathForTrash(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPathForTrash(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// pathExists 存在性判定（回收站条目名判重用；Stat 错误一律按不存在处理）
func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
