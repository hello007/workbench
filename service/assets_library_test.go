package service

import (
	"context"
	"path/filepath"
	"testing"
)

// TestNewAssetLibraryService_RelativeAssetsDirAbsolutized 相对入参须在构造时
// 绝对化：thumb PNG 路径（assetThumbsDir 产出）最终经 COM 服务器进程
// （wpp.exe/powerpnt.exe）的 Slide.Export 解析，COM 服务器按自身 cwd 解析
// 相对路径非法（真机「发生意外」确定性失败根因）。Abs 一处收口保护全部
// 消费面（db 路径/注册表/thumbs/未来跨进程消费者）。
func TestNewAssetLibraryService_RelativeAssetsDirAbsolutized(t *testing.T) {
	svc := NewAssetLibraryService(context.Background(), "data")
	t.Cleanup(svc.CloseAll)

	if !filepath.IsAbs(svc.assetsDir) {
		t.Fatalf("相对入参构造后 assetsDir 应为绝对路径，实际：%s", svc.assetsDir)
	}
	wantBase, err := filepath.Abs("data")
	if err != nil {
		t.Fatalf("filepath.Abs(data): %v", err)
	}
	if svc.assetsDir != wantBase {
		t.Errorf("assetsDir 应等于按 cwd 绝对化的 data 目录，实际：%s，期望：%s", svc.assetsDir, wantBase)
	}

	const libID = "lib-relative-test"
	dbPath := svc.libraryDBPath(libID)
	if !filepath.IsAbs(dbPath) {
		t.Errorf("libraryDBPath 应为绝对路径，实际：%s", dbPath)
	}
	if want := filepath.Join(wantBase, libID+".db"); dbPath != want {
		t.Errorf("libraryDBPath 应以绝对化 data 目录为基准拼接，实际：%s，期望：%s", dbPath, want)
	}

	thumbsDir := svc.assetThumbsDir(libID)
	if !filepath.IsAbs(thumbsDir) {
		t.Errorf("assetThumbsDir 应为绝对路径，实际：%s", thumbsDir)
	}
	if want := filepath.Join(wantBase, libID, "thumbs"); thumbsDir != want {
		t.Errorf("assetThumbsDir 应以绝对化 data 目录为基准拼接，实际：%s，期望：%s", thumbsDir, want)
	}
}

// TestNewAssetLibraryService_AbsoluteAssetsDirPreserved 绝对入参原样保持
// （filepath.Abs 幂等）：既有测试与生产 serve 模式传 temp dir 绝对参不受影响。
func TestNewAssetLibraryService_AbsoluteAssetsDirPreserved(t *testing.T) {
	absIn := t.TempDir() // 绝对且 clean 形式的路径
	svc := NewAssetLibraryService(context.Background(), absIn)
	t.Cleanup(svc.CloseAll)

	if svc.assetsDir != absIn {
		t.Errorf("绝对入参应原样保持（Abs 幂等），实际：%s，期望：%s", svc.assetsDir, absIn)
	}
}
