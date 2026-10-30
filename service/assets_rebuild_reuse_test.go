package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// ---------- 重建保留 fileID / 缩略图产物（PRD R4/R5） ----------

// TestRebuildKeepsFileIDAndThumbs 内容未变的全量重建：files.id 保持、缩略图产物
// 全部保留（重建不再全库重导）。
func TestRebuildKeepsFileIDAndThumbs(t *testing.T) {
	svc, store, lib, fileID, thumbsDir := setupInvalidationFixture(t)

	if _, err := svc.RebuildScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("RebuildScan: %v", err)
	}
	afterID := fileIDByPath(t, store, "甲.pptx")
	if afterID != fileID {
		t.Fatalf("重建后 fileID 应复用（缩略图缓存键稳定）: got %d want %d", afterID, fileID)
	}
	for _, tier := range thumbTiers {
		for page := 1; page <= 3; page++ {
			if _, err := os.Stat(thumbPNGPath(thumbsDir, tier.Key, fileID, page)); err != nil {
				t.Fatalf("内容未变应保留 %s 档第 %d 页: %v", tier.Key, page, err)
			}
		}
	}
}

// TestRebuildChangedFileDropsThumbs 内容变化的全量重建：该文件旧缩略图与记账清除
// （交给补缺链路重导）。
func TestRebuildChangedFileDropsThumbs(t *testing.T) {
	svc, store, lib, fileID, thumbsDir := setupInvalidationFixture(t)
	buildAssetsPptx(t, lib.RootPath, "甲.pptx", []assetsSlideSpec{{titlePh: "改", body: []string{"新内容"}}})

	if _, err := svc.RebuildScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("RebuildScan: %v", err)
	}
	if afterID := fileIDByPath(t, store, "甲.pptx"); afterID != fileID {
		t.Fatalf("重建后 fileID 应复用（内容变化不清主键）: got %d want %d", afterID, fileID)
	}
	for _, tier := range thumbTiers {
		if _, err := os.Stat(thumbPNGPath(thumbsDir, tier.Key, fileID, 1)); !os.IsNotExist(err) {
			t.Fatalf("内容变化应清除 %s 档旧缩略图, stat err=%v", tier.Key, err)
		}
	}
}

// TestRebuildPrunesOrphanThumbs 重建收尾清理无主产物：fileID 不在索引中的 PNG 删除，
// 在索引中的保留。
func TestRebuildPrunesOrphanThumbs(t *testing.T) {
	svc, _, lib, fileID, thumbsDir := setupInvalidationFixture(t)
	orphan := writeThumbPNG(t, thumbsDir, "small", 99999, 1, "orphan")

	if _, err := svc.RebuildScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("RebuildScan: %v", err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("无主 PNG 应被清理, stat err=%v", err)
	}
	if _, err := os.Stat(thumbPNGPath(thumbsDir, "small", fileID, 1)); err != nil {
		t.Fatalf("在用 PNG 不应被清理: %v", err)
	}
}

// TestRebuildRemovedFileDropsRecord 重建时磁盘已消失的文件：真删除记录（重建语义
// 等同旧「清空重来」，不落 lost），其缩略图一并清理。
func TestRebuildRemovedFileDropsRecord(t *testing.T) {
	svc, store, lib, fileID, thumbsDir := setupInvalidationFixture(t)
	if err := os.Remove(filepath.Join(lib.RootPath, "甲.pptx")); err != nil {
		t.Fatalf("删除源文件: %v", err)
	}

	_, err := svc.RebuildScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("RebuildScan: %v", err)
	}
	// 扫描阶段先按常规语义标 lost（result.Lost 计入），重建收尾再真删除——最终
	// 不变式是「索引零残留」，不看中间计数。
	var n int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM files").Scan(&n); err != nil {
		t.Fatalf("查 files 行数: %v", err)
	}
	if n != 0 {
		t.Fatalf("消失文件记录应删除, got %d 行", n)
	}
	if _, err := os.Stat(thumbPNGPath(thumbsDir, "small", fileID, 1)); !os.IsNotExist(err) {
		t.Fatalf("消失文件的缩略图应清理, stat err=%v", err)
	}
}

// TestThumbFileIDFromName 缓存键文件名解析（孤儿判定前置）。
func TestThumbFileIDFromName(t *testing.T) {
	cases := []struct {
		name string
		want int64
		ok   bool
	}{
		{"1024-003.png", 1024, true},
		{"1-001.png", 1, true},
		{"nope.png", 0, false},
		{"-001.png", 0, false},
		{"abc-001.png", 0, false},
	}
	for _, c := range cases {
		got, ok := thumbFileIDFromName(c.name)
		if got != c.want || ok != c.ok {
			t.Fatalf("thumbFileIDFromName(%q) = (%d,%v), want (%d,%v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

// TestRestoreRebuildPlaceholdersKeepsIDAndSequence 占位恢复显式主键复用，且后续
// AUTOINCREMENT 不与旧 ID 冲突（序列不回退）。
func TestRestoreRebuildPlaceholdersKeepsIDAndSequence(t *testing.T) {
	svc := newTestAssetService(t)
	lib := mustCreateLibrary(t, svc, "序列库", t.TempDir())
	store := svc.openStoreForTest(t, lib.ID)

	if err := store.restoreRebuildPlaceholders([]rebuildPlaceholder{
		{id: 100, path: "旧.pptx", size: 1, mtimeNs: 2, hash: "h", title: "旧"},
	}); err != nil {
		t.Fatalf("restoreRebuildPlaceholders: %v", err)
	}
	var status string
	if err := store.db.QueryRow("SELECT status FROM files WHERE id = 100").Scan(&status); err != nil {
		t.Fatalf("占位行应存在: %v", err)
	}
	if status != model.AssetFileStatusProcessing {
		t.Fatalf("占位行状态应为 processing, got %q", status)
	}
	id, err := store.insertProcessingFile("新.pptx", "新", 1, 2, "2026-01-01 00:00:00")
	if err != nil {
		t.Fatalf("insertProcessingFile: %v", err)
	}
	if id <= 100 {
		t.Fatalf("新分配 ID 应大于历史最大值（序列不回退）, got %d", id)
	}
}

// TestRebuildPageCountDriftDropsThumbs 页数漂移（R5 页数维度）：旧索引页数 3、重建
// 解析出 1 页 → 越界旧图无人对账（记账已清、补缺只按现页遍历），必须由重建收尾
// 主动清图重导。
func TestRebuildPageCountDriftDropsThumbs(t *testing.T) {
	svc, store, lib, fileID, thumbsDir := setupInvalidationFixture(t)
	for _, page := range []int{2, 3} {
		if _, err := store.db.Exec(
			"INSERT INTO slides(file_id, slide_num) VALUES(?, ?)", fileID, page,
		); err != nil {
			t.Fatalf("注入额外页: %v", err)
		}
	}

	if _, err := svc.RebuildScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("RebuildScan: %v", err)
	}
	for _, tier := range thumbTiers {
		for page := 1; page <= 3; page++ {
			if _, err := os.Stat(thumbPNGPath(thumbsDir, tier.Key, fileID, page)); !os.IsNotExist(err) {
				t.Fatalf("页数漂移应清除 %s 档第 %d 页, stat err=%v", tier.Key, page, err)
			}
		}
	}
}
