//go:build !windows

// util/recycle_other_test.go
package util

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 非 Windows 平台（freedesktop Trash）实现的单测：把 XDG_DATA_HOME 指到临时
// 目录即完整隔离回收站根，不依赖 HOME 等外部状态（docs/spec/test-stability.md）。
// 这些用例在 Windows 开发机不参与编译（Go 跨平台测试由 CI 的 Linux job 覆盖）。

// setupTrashEnv 隔离 XDG_DATA_HOME 并返回回收站根目录。
func setupTrashEnv(t *testing.T) string {
	t.Helper()
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	return filepath.Join(dataHome, "Trash")
}

// writeTrashSource 构造待移入的源文件（独立临时目录，与回收站根分离）。
func writeTrashSource(t *testing.T, content string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "演示.pptx")
	if err := os.WriteFile(src, []byte(content), 0644); err != nil {
		t.Fatalf("写源文件: %v", err)
	}
	return src
}

// trashInfoPathOf 解析 .trashinfo 的 Path 行并做 URL 解码（freedesktop 规范：
// Path 为百分号编码的绝对路径）。
func trashInfoPathOf(t *testing.T, content string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		rest, ok := strings.CutPrefix(line, "Path=")
		if !ok {
			continue
		}
		p, err := url.PathUnescape(rest)
		if err != nil {
			t.Fatalf("Path 行 URL 解码失败（%q）: %v", rest, err)
		}
		return p
	}
	t.Fatalf("trashinfo 缺少 Path 行：%q", content)
	return ""
}

// TestMoveToRecycleBin_Other_Success 成功移入：源消失，Trash/files 与
// Trash/info 均落条目，trashinfo 的 Path 指向源绝对路径且带删除时间。
func TestMoveToRecycleBin_Other_Success(t *testing.T) {
	trash := setupTrashEnv(t)
	src := writeTrashSource(t, "pptx-bytes")

	if err := MoveToRecycleBin(src); err != nil {
		t.Fatalf("MoveToRecycleBin: %v", err)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Errorf("源文件应消失，stat err=%v", err)
	}
	dst := filepath.Join(trash, "files", "演示.pptx")
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "pptx-bytes" {
		t.Errorf("回收站应保存源内容（err=%v, content=%q）", err, got)
	}
	infoBytes, err := os.ReadFile(filepath.Join(trash, "info", "演示.pptx.trashinfo"))
	if err != nil {
		t.Fatalf("回收站元数据应存在: %v", err)
	}
	info := string(infoBytes)
	if !strings.HasPrefix(info, "[Trash Info]") {
		t.Errorf("trashinfo 应以 [Trash Info] 开头：%q", info)
	}
	if decoded := trashInfoPathOf(t, info); decoded != src {
		t.Errorf("trashinfo Path 应指向源绝对路径：%q, want %q", decoded, src)
	}
	if !strings.Contains(info, "DeletionDate=") {
		t.Errorf("trashinfo 应包含 DeletionDate：%q", info)
	}
}

// TestMoveToRecycleBin_Other_NameConflictAppendsSuffix 同名条目冲突：files 与
// info 两侧同时让位，新条目名追加 .1，既有条目内容不被覆盖。
func TestMoveToRecycleBin_Other_NameConflictAppendsSuffix(t *testing.T) {
	trash := setupTrashEnv(t)
	filesDir := filepath.Join(trash, "files")
	infoDir := filepath.Join(trash, "info")
	for _, dir := range []string{filesDir, infoDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("建回收站目录: %v", err)
		}
	}
	occupiedFile := filepath.Join(filesDir, "演示.pptx")
	occupiedInfo := filepath.Join(infoDir, "演示.pptx.trashinfo")
	if err := os.WriteFile(occupiedFile, []byte("occupied"), 0644); err != nil {
		t.Fatalf("占位回收站文件: %v", err)
	}
	if err := os.WriteFile(occupiedInfo, []byte("occupied-info"), 0644); err != nil {
		t.Fatalf("占位回收站元数据: %v", err)
	}
	src := writeTrashSource(t, "pptx-bytes")

	if err := MoveToRecycleBin(src); err != nil {
		t.Fatalf("MoveToRecycleBin: %v", err)
	}
	if got, err := os.ReadFile(occupiedFile); err != nil || string(got) != "occupied" {
		t.Errorf("既有条目不得被覆盖（err=%v, content=%q）", err, got)
	}
	if got, err := os.ReadFile(occupiedInfo); err != nil || string(got) != "occupied-info" {
		t.Errorf("既有元数据不得被覆盖（err=%v, content=%q）", err, got)
	}
	if got, err := os.ReadFile(filepath.Join(filesDir, "演示.pptx.1")); err != nil || string(got) != "pptx-bytes" {
		t.Errorf("新条目应追加 .1 后缀（err=%v, content=%q）", err, got)
	}
	if _, err := os.Lstat(filepath.Join(infoDir, "演示.pptx.1.trashinfo")); err != nil {
		t.Errorf("新条目元数据应为 .1.trashinfo：%v", err)
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Errorf("源文件应消失，stat err=%v", err)
	}
}

// TestMoveToRecycleBin_Other_InfoWriteFailureKeepsSource 元数据写入失败：移动
// 尚未执行，源文件原位保留、回收站无残留条目（info 先写后移动的次序保证）。
func TestMoveToRecycleBin_Other_InfoWriteFailureKeepsSource(t *testing.T) {
	trash := setupTrashEnv(t)
	src := writeTrashSource(t, "pptx-bytes")
	orig := trashInfoWrite
	trashInfoWrite = func(string, []byte, os.FileMode) error {
		return errors.New("磁盘已满")
	}
	t.Cleanup(func() { trashInfoWrite = orig })

	err := MoveToRecycleBin(src)
	if err == nil || !strings.Contains(err.Error(), "写入回收站元数据失败") {
		t.Fatalf("元数据写入失败应报错，实际：%v", err)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("失败时源文件应原位保留：%v", statErr)
	}
	if entries, readErr := os.ReadDir(filepath.Join(trash, "files")); readErr == nil && len(entries) != 0 {
		t.Errorf("失败时回收站不应有条目，实际 %d 项", len(entries))
	}
	if entries, readErr := os.ReadDir(filepath.Join(trash, "info")); readErr == nil && len(entries) != 0 {
		t.Errorf("失败时不应残留元数据，实际 %d 项", len(entries))
	}
}

// TestMoveToRecycleBin_Other_MkdirFailureKeepsSource 回收站根不可建（files
// 路径被普通文件占用）：报错且源文件原位保留，符合「失败 = 什么都没发生」口径。
func TestMoveToRecycleBin_Other_MkdirFailureKeepsSource(t *testing.T) {
	trash := setupTrashEnv(t)
	if err := os.MkdirAll(trash, 0700); err != nil {
		t.Fatalf("建回收站根: %v", err)
	}
	if err := os.WriteFile(filepath.Join(trash, "files"), []byte("block"), 0644); err != nil {
		t.Fatalf("占用 files 路径: %v", err)
	}
	src := writeTrashSource(t, "pptx-bytes")

	err := MoveToRecycleBin(src)
	if err == nil || !strings.Contains(err.Error(), "创建回收站目录失败") {
		t.Fatalf("目录不可建应报错，实际：%v", err)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("失败时源文件应原位保留：%v", statErr)
	}
}
