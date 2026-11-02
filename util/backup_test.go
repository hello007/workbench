package util

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBackupCorruptFile_SameContentSingleBackup 同一份损坏内容重复留底只保留一份
// 备份：文件名由内容摘要决定，不依赖读取时刻（时间戳命名会随读取次数无上限增涨）。
func TestBackupCorruptFile_SameContentSingleBackup(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "templates.json")
	raw := []byte(`{not-valid-json`)
	if err := os.WriteFile(src, raw, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	first, err := BackupCorruptFile(src, raw)
	if err != nil {
		t.Fatalf("first BackupCorruptFile: %v", err)
	}
	second, err := BackupCorruptFile(src, raw)
	if err != nil {
		t.Fatalf("second BackupCorruptFile: %v", err)
	}
	if first != second {
		t.Errorf("同一内容两次留底应返回同一路径: %q vs %q", first, second)
	}

	matches, err := filepath.Glob(src + ".bak.*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("同一内容重复留底应只有 1 个备份文件, got %v", matches)
	}
	if !strings.HasPrefix(matches[0], src+".bak.") {
		t.Errorf("备份文件命名异常: %q", matches[0])
	}

	got, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("备份内容应与原文完全一致, got %q want %q", got, raw)
	}
}

// TestBackupCorruptFile_DifferentContentTwice 不同内容各留一份备份。
func TestBackupCorruptFile_DifferentContentTwice(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "sessions.json")

	rawA := []byte("{bad-a")
	rawB := []byte("{bad-b-different-length")
	pathA, err := BackupCorruptFile(src, rawA)
	if err != nil {
		t.Fatalf("backup A: %v", err)
	}
	pathB, err := BackupCorruptFile(src, rawB)
	if err != nil {
		t.Fatalf("backup B: %v", err)
	}
	if pathA == pathB {
		t.Errorf("不同内容应生成不同备份路径: %q", pathA)
	}

	matches, err := filepath.Glob(src + ".bak.*")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("不同内容应各留一份备份, got %v", matches)
	}
	for _, p := range matches {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %q: %v", p, err)
		}
		if !bytes.Equal(data, rawA) && !bytes.Equal(data, rawB) {
			t.Errorf("备份 %q 内容与任一原文都不一致: %q", p, data)
		}
	}
}

// TestBackupCorruptFile_EmptyRaw 空内容同样正常留底（得到空文件的固定摘要）。
func TestBackupCorruptFile_EmptyRaw(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "messages.json")

	p, err := BackupCorruptFile(src, nil)
	if err != nil {
		t.Fatalf("empty raw backup: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("空内容备份大小应为 0, got %d", info.Size())
	}
}

// TestBackupCorruptFile_WriteFailure 目标目录不存在时返回错误（调用方按
// best effort 处理，不阻断损坏降级主流程）。
func TestBackupCorruptFile_WriteFailure(t *testing.T) {
	missingDir := filepath.Join(t.TempDir(), "no-such-dir")
	_, err := BackupCorruptFile(filepath.Join(missingDir, "templates.json"), []byte("{bad"))
	if err == nil {
		t.Fatal("目标目录不存在应返回写入错误")
	}
}
