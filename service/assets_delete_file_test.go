package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbench/model"
)

// 单文件删除（DeleteAssetFile）：磁盘删除 + 索引标记丢失（与 DeleteAssetFolder
// 同构，标注/评分/缩略图记账保留）。

// TestDeleteAssetFile_MarksLostAndRemovesDisk 正常删除：磁盘文件消失、索引行转
// lost、解析产物（slides）保留待复活，列表仍可读到该条目（lost 态）。
func TestDeleteAssetFile_MarksLostAndRemovesDisk(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "删除文件库", dir)
	buildAssetsPptx(t, dir, "待删.pptx", []assetsSlideSpec{{titlePh: "页", body: []string{"文"}}})

	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("扫描: %v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("前置文件清单异常: %+v, err=%v", files, err)
	}
	rec := files[0]

	if err := svc.DeleteAssetFile(lib.ID, rec.ID); err != nil {
		t.Fatalf("DeleteAssetFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, rec.Path)); !os.IsNotExist(err) {
		t.Errorf("磁盘文件应已删除，Stat err = %v", err)
	}
	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("索引行应保留（lost 态），实际条目数 %d", len(files))
	}
	if files[0].Status != model.AssetFileStatusLost {
		t.Errorf("状态 = %q, want %q", files[0].Status, model.AssetFileStatusLost)
	}
	// 解析产物保留：文件回归后可零成本复活（fileHasSlides 资格门槛）
	store := svc.openStoreForTest(t, lib.ID)
	hasSlides, err := store.fileHasSlides(rec.ID)
	if err != nil {
		t.Fatalf("fileHasSlides: %v", err)
	}
	if !hasSlides {
		t.Error("删除后不应清除 slides（标注与解析产物保留）")
	}
}

// TestDeleteAssetFile_RejectsProcessing 解析中（processing 行，扫描写库在途）拒绝，
// 磁盘文件与索引状态均不变。
func TestDeleteAssetFile_RejectsProcessing(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "解析中库", dir)
	buildAssetsPptx(t, dir, "忙.pptx", []assetsSlideSpec{{titlePh: "页", body: []string{"文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("扫描: %v", err)
	}
	files, _ := svc.ListFiles(lib.ID)
	rec := files[0]
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.markProcessing(rec.ID, time.Now().Format("2006-01-02 15:04:05")); err != nil {
		t.Fatalf("置 processing: %v", err)
	}

	err := svc.DeleteAssetFile(lib.ID, rec.ID)
	if err == nil || !strings.Contains(err.Error(), "解析中") {
		t.Fatalf("应拒绝解析中文件，实际 err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, rec.Path)); statErr != nil {
		t.Errorf("拒绝后磁盘文件应保留: %v", statErr)
	}
}

// TestDeleteAssetFile_RejectsSplitDerivative 拆分派生产物拒绝（由拆分链路管理，
// 单独删会破坏拆分记录）。
func TestDeleteAssetFile_RejectsSplitDerivative(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "派生库", dir)
	// 造「同名目录/同名-001.pptx」形态的产物文件与索引记录
	if err := os.MkdirAll(filepath.Join(dir, "源"), 0o755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	artifact := filepath.Join(dir, "源", "源-001.pptx")
	if err := os.WriteFile(artifact, []byte("artifact"), 0o644); err != nil {
		t.Fatalf("写产物: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	id, err := store.insertProcessingFile("源/源-001.pptx", "源", 8, time.Now().UnixNano(), time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		t.Fatalf("插入产物记录: %v", err)
	}

	err = svc.DeleteAssetFile(lib.ID, id)
	if err == nil || !strings.Contains(err.Error(), "拆分派生产物") {
		t.Fatalf("应拒绝拆分派生产物，实际 err = %v", err)
	}
	if _, statErr := os.Stat(artifact); statErr != nil {
		t.Errorf("拒绝后产物文件应保留: %v", statErr)
	}
}

// TestDeleteAssetFile_RecordNotFound 记录不存在给出可读错误（前端并发清理窗口）。
func TestDeleteAssetFile_RecordNotFound(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "空库", t.TempDir())
	err := svc.DeleteAssetFile(lib.ID, 9999)
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("应报记录不存在，实际 err = %v", err)
	}
}
