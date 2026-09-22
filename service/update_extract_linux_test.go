//go:build linux

package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// writeTestFile 落盘测试输入文件并返回路径。
func writeTestFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("写入 %s: %v", name, err)
	}
	return path
}

// TestExtractUpdateTarGz_Success 多成员 tar.gz 提取顶层 workbench：内容一致、可执行位就位、
// 其余成员（README / 子目录成员）不落盘。
func TestExtractUpdateTarGz_Success(t *testing.T) {
	dest := t.TempDir()
	payload := buildTestTarGz(t, map[string][]byte{
		updateBinaryLinux:        []byte("binary-bytes"),
		"README-linux.md":        []byte("readme-bytes"),
		"sub/nested-not-extract": []byte("nested-bytes"),
	})
	tarGz := writeTestFile(t, t.TempDir(), updateAssetLinux, payload)

	got, err := extractUpdateTarGz(tarGz, dest)
	if err != nil {
		t.Fatalf("extractUpdateTarGz: %v", err)
	}
	want := filepath.Join(dest, updateBinaryLinux)
	if got != want {
		t.Fatalf("返回路径 = %q, want %q", got, want)
	}

	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("读解包产物: %v", err)
	}
	if string(data) != "binary-bytes" {
		t.Errorf("解包内容不符: %q", data)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat 解包产物: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("解包产物应具备执行位, mode=%v", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(dest, "README-linux.md")); !os.IsNotExist(err) {
		t.Error("非二进制成员不应被提取")
	}
}

// TestExtractUpdateTarGz_MissingBinary 包内无 workbench 成员时返回明确错误且不产生残留文件。
func TestExtractUpdateTarGz_MissingBinary(t *testing.T) {
	dest := t.TempDir()
	payload := buildTestTarGz(t, map[string][]byte{"README-linux.md": []byte("readme")})
	tarGz := writeTestFile(t, t.TempDir(), updateAssetLinux, payload)

	if _, err := extractUpdateTarGz(tarGz, dest); err == nil {
		t.Fatal("无 workbench 成员应返回错误")
	} else if !bytes.Contains([]byte(err.Error()), []byte(updateBinaryLinux)) {
		t.Errorf("错误信息应包含二进制名 %q: %v", updateBinaryLinux, err)
	}
}

// TestExtractUpdateTarGz_PathTraversal 成员名带目录穿越成分时不落盘越界路径
// （Base 剥离后与二进制名不命中，整个包解析为「未找到」错误）。
func TestExtractUpdateTarGz_PathTraversal(t *testing.T) {
	dest := t.TempDir()
	outside := t.TempDir()
	evilPath := filepath.Join(outside, "evil")

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     filepath.Join("..", filepath.Base(evilPath)),
		Mode:     0o755,
		Size:     int64(len("evil")),
	}); err != nil {
		t.Fatalf("写 tar 头: %v", err)
	}
	if _, err := tw.Write([]byte("evil")); err != nil {
		t.Fatalf("写 tar 内容: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("关闭 tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("关闭 gzip: %v", err)
	}
	tarGz := writeTestFile(t, t.TempDir(), updateAssetLinux, buf.Bytes())

	if _, err := extractUpdateTarGz(tarGz, dest); err == nil {
		t.Fatal("穿越成员应导致「未找到二进制」错误")
	}
	if _, err := os.Stat(evilPath); !os.IsNotExist(err) {
		t.Error("穿越路径不应被写入")
	}
}

// TestExtractUpdateTarGz_CorruptArchive 非法 gzip 载荷返回错误而非 panic。
func TestExtractUpdateTarGz_CorruptArchive(t *testing.T) {
	dest := t.TempDir()
	tarGz := writeTestFile(t, t.TempDir(), updateAssetLinux, []byte("not a gzip"))

	if _, err := extractUpdateTarGz(tarGz, dest); err == nil {
		t.Fatal("非法 gzip 应返回错误")
	}
}
