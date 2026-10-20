package service

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// 文件丢失与重链（SRS §2.6.5，素材管理第九批 PR2）单测：lost 标记与元数据存续、
// lost 复活两分支、清理丢失（CASCADE + 缩略图收口）、手动重链（哈希相等/不等
// force/库外拒绝/空哈希跳过/processing 残留落重解析）、前缀替换（预览目录边界/
// 越界整体拒绝/三态收敛/自动扫描/混合长度逐记录配对/processing 残留落重解析）。
// 时序确定性：文件移动用 os.Rename（同卷保留 mtime，复活分支的
// mtime+size 匹配天然成立，无 sleep 无 Chtimes 时序依赖，对齐 test-stability）。

// markFileLostForTest 白盒注入 lost 状态（构造「已标记丢失」前置态，免走
// 移动文件 + 二次扫描的绕路）。
func markFileLostForTest(t *testing.T, svc *AssetLibraryService, libraryID, relPath string) int64 {
	t.Helper()
	store := svc.openStoreForTest(t, libraryID)
	var id int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", relPath).Scan(&id); err != nil {
		t.Fatalf("查文件记录: %v", err)
	}
	if _, err := store.db.Exec("UPDATE files SET status = ? WHERE id = ?", model.AssetFileStatusLost, id); err != nil {
		t.Fatalf("注入 lost 状态: %v", err)
	}
	return id
}

// fileStatusByPath 白盒读单文件状态。
func fileStatusByPath(t *testing.T, svc *AssetLibraryService, libraryID, relPath string) string {
	t.Helper()
	store := svc.openStoreForTest(t, libraryID)
	var status string
	if err := store.db.QueryRow("SELECT status FROM files WHERE path = ?", relPath).Scan(&status); err != nil {
		t.Fatalf("读文件状态（%s）: %v", relPath, err)
	}
	return status
}

// ---------- lost 标记与元数据存续 ----------

// TestAssetScan_LostPreservesAnnotations lost 标记的元数据存续断言（D1 核心）：
// 文件标签、页自有标签、页业务备注/星级、缩略图记账与 PNG 文件全保留（不再
// CASCADE 删除），FTS 行保留（关键词仍可命中 lost 文件辅助重链）。
func TestAssetScan_LostPreservesAnnotations(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "丢失存续库", dir)
	src := buildAssetsPptx(t, dir, "方案.pptx", []assetsSlideSpec{
		{titlePh: "标题", body: []string{"正文"}, notes: "备注"},
	})

	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	fileID := files[0].ID

	// 用户标注三面：文件标签 + 页自有标签 + 页备注/星级
	if err := svc.UpdateFileMeta(lib.ID, fileID, model.AssetFileMetaUpdate{
		Rating: 4, Note: "重点材料", Tags: []string{"城商行"},
	}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}
	if err := svc.UpdateSlideMeta(lib.ID, fileID, 1, model.AssetSlideMetaUpdate{
		Rating: 5, Note: "开场页", TagNames: []string{"首页"},
	}); err != nil {
		t.Fatalf("UpdateSlideMeta: %v", err)
	}
	// 缩略图记账 + PNG 文件（白盒注入：lost 语义要求两者全保留）
	store := svc.openStoreForTest(t, lib.ID)
	thumbsDir := svc.assetThumbsDir(lib.ID)
	pngPath := thumbPNGPath(thumbsDir, thumbTiers[0].Key, fileID, 1)
	if err := os.MkdirAll(filepath.Dir(pngPath), 0755); err != nil {
		t.Fatalf("建缩略图目录: %v", err)
	}
	if err := os.WriteFile(pngPath, []byte("png"), 0644); err != nil {
		t.Fatalf("写缩略图 PNG: %v", err)
	}
	if _, err := store.db.Exec(
		"INSERT INTO thumb_cache(file_id, tier, page, bytes, last_access_ns) VALUES(?, ?, 1, 3, 1)",
		fileID, thumbTiers[0].Key,
	); err != nil {
		t.Fatalf("注入缩略图记账: %v", err)
	}
	ftsBefore, err := store.countFtsRows()
	if err != nil {
		t.Fatalf("统计 FTS: %v", err)
	}
	if ftsBefore == 0 {
		t.Fatal("前置态应存在 FTS 行")
	}

	if err := os.Remove(src); err != nil {
		t.Fatalf("删除源文件: %v", err)
	}
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("二次扫描: %v", err)
	}

	if status := fileStatusByPath(t, svc, lib.ID, "方案.pptx"); status != model.AssetFileStatusLost {
		t.Fatalf("应标记 lost，实际 %q", status)
	}
	// 文件标签 + 星级/备注
	gotFiles, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if gotFiles[0].Rating != 4 || gotFiles[0].Note != "重点材料" || len(gotFiles[0].Tags) != 1 {
		t.Errorf("文件级标注应保留，实际：%+v", gotFiles[0])
	}
	// 页级标注
	page, err := svc.ListSlides(lib.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Rating != 5 || page.Items[0].Note != "开场页" || len(page.Items[0].OwnTags) != 1 {
		t.Errorf("页级标注应保留，实际：%+v", page.Items)
	}
	// 缩略图记账与 PNG
	var ledgerRows int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM thumb_cache WHERE file_id = ?", fileID).Scan(&ledgerRows); err != nil {
		t.Fatalf("查记账: %v", err)
	}
	if ledgerRows != 1 {
		t.Errorf("缩略图记账应保留 1 行，实际 %d", ledgerRows)
	}
	if _, err := os.Stat(pngPath); err != nil {
		t.Errorf("缩略图 PNG 应保留: %v", err)
	}
	// FTS 行保留（lost 文件仍可被关键词检索辅助重链）
	ftsAfter, err := store.countFtsRows()
	if err != nil {
		t.Fatalf("统计 FTS: %v", err)
	}
	if ftsAfter != ftsBefore {
		t.Errorf("FTS 行应保留（%d → %d）", ftsBefore, ftsAfter)
	}
}

// TestAssetScan_LostRevival lost 复活两分支：
//   - 未变复活：文件移出再移回（os.Rename 同卷保留 mtime+size）→ 直接复活 ok，
//     不重解析（解析计数不增，旧页记录原样保留）；
//   - 已变重解析：移回的是不同内容文件（mtime 变化）→ 走既有重解析路径。
func TestAssetScan_LostRevival(t *testing.T) {
	svc := newTestAssetService(t)
	parseCalls := 0
	baseParse := svc.parseFn
	svc.parseFn = func(src, base string) ([]parsedSlide, error) {
		parseCalls++
		if baseParse == nil {
			return parsePptxSlides(src, base)
		}
		return baseParse(src, base)
	}
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "复活库", dir)
	src := buildAssetsPptx(t, dir, "复活.pptx", []assetsSlideSpec{{titlePh: "原页", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	firstParseCalls := parseCalls

	// 移出 → 扫描标记 lost
	outside := filepath.Join(t.TempDir(), "复活.pptx")
	if err := os.Rename(src, outside); err != nil {
		t.Fatalf("移出文件: %v", err)
	}
	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("标记 lost 扫描: %v", err)
	}
	if result.Lost != 1 {
		t.Fatalf("应标记 1 条丢失，实际 %+v", result)
	}

	// 移回（同卷 rename 保留 mtime+size）→ 未变复活：不重解析
	if err := os.Rename(outside, src); err != nil {
		t.Fatalf("移回文件: %v", err)
	}
	result, err = svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("复活扫描: %v", err)
	}
	if result.Revived != 1 || result.Updated != 0 || result.Unchanged != 0 {
		t.Fatalf("未变复活应 revived=1（不与 unchanged 混计），实际 %+v", result)
	}
	if parseCalls != firstParseCalls {
		t.Errorf("未变复活不得重解析（解析调用 %d → %d）", firstParseCalls, parseCalls)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "复活.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("复活后状态应为 ok，实际 %q", status)
	}
	page, err := svc.ListSlides(lib.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "原页" {
		t.Errorf("复活后旧页记录应原样保留，实际：%+v", page.Items)
	}

	// 已变复活：替换为不同内容文件（mtime/size 变化）→ 走重解析
	if err := os.Remove(src); err != nil {
		t.Fatalf("移除旧文件: %v", err)
	}
	buildAssetsPptx(t, dir, "复活.pptx", []assetsSlideSpec{{titlePh: "新页", body: []string{"新内容"}}})
	result, err = svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("已变复活扫描: %v", err)
	}
	if result.Updated != 1 || result.Revived != 0 {
		t.Fatalf("已变复活应走重解析（updated=1 revived=0），实际 %+v", result)
	}
	if parseCalls != firstParseCalls+1 {
		t.Errorf("已变复活应重解析一次（解析调用 %d → %d）", firstParseCalls, parseCalls)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "复活.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("重解析后状态应为 ok，实际 %q", status)
	}
	page, err = svc.ListSlides(lib.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "新页" {
		t.Errorf("重解析后应为新页内容，实际：%+v", page.Items)
	}
}

// ---------- 清理丢失记录 ----------

// TestClearLostFiles 清理丢失记录（等效旧删除语义）：lost 记录与幻灯片/标签/
// 页级标注/缩略图记账全删 + FTS 行清除 + PNG 文件删除；无 lost 记录幂等返 0。
func TestClearLostFiles(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "清理库", dir)
	buildAssetsPptx(t, dir, "丢A.pptx", []assetsSlideSpec{{titlePh: "A页", body: []string{"正文"}}})
	buildAssetsPptx(t, dir, "留B.pptx", []assetsSlideSpec{{titlePh: "B页", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	var lostID, keepID int64
	for _, f := range files {
		if f.Path == "丢A.pptx" {
			lostID = f.ID
		} else {
			keepID = f.ID
		}
	}
	if err := svc.UpdateFileMeta(lib.ID, lostID, model.AssetFileMetaUpdate{
		Rating: 3, Note: "待清理", Tags: []string{"过期"},
	}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}
	if err := svc.UpdateSlideMeta(lib.ID, lostID, 1, model.AssetSlideMetaUpdate{
		Rating: 2, Note: "页备注", TagNames: []string{"页标签"},
	}); err != nil {
		t.Fatalf("UpdateSlideMeta: %v", err)
	}
	lostPNG := thumbPNGPath(svc.assetThumbsDir(lib.ID), thumbTiers[0].Key, lostID, 1)
	keepPNG := thumbPNGPath(svc.assetThumbsDir(lib.ID), thumbTiers[0].Key, keepID, 1)
	for _, p := range []string{lostPNG, keepPNG} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("建缩略图目录: %v", err)
		}
		if err := os.WriteFile(p, []byte("png"), 0644); err != nil {
			t.Fatalf("写缩略图 PNG: %v", err)
		}
	}
	markFileLostForTest(t, svc, lib.ID, "丢A.pptx")

	removed, err := svc.ClearLostAssetFiles(lib.ID)
	if err != nil {
		t.Fatalf("ClearLostAssetFiles: %v", err)
	}
	if removed != 1 {
		t.Fatalf("应清理 1 条，实际 %d", removed)
	}
	store := svc.openStoreForTest(t, lib.ID)
	// 记录与级联面
	var n int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM files WHERE id = ?", lostID).Scan(&n); err != nil || n != 0 {
		t.Errorf("lost 记录应删除，实际 n=%d err=%v", n, err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM slides WHERE file_id = ?", lostID).Scan(&n); err != nil || n != 0 {
		t.Errorf("slides 应随级联清除，实际 n=%d err=%v", n, err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM file_tags WHERE file_id = ?", lostID).Scan(&n); err != nil || n != 0 {
		t.Errorf("file_tags 应随级联清除，实际 n=%d err=%v", n, err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM slide_meta WHERE file_id = ?", lostID).Scan(&n); err != nil || n != 0 {
		t.Errorf("slide_meta 应随级联清除，实际 n=%d err=%v", n, err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM slide_tags WHERE file_id = ?", lostID).Scan(&n); err != nil || n != 0 {
		t.Errorf("slide_tags 应随级联清除，实际 n=%d err=%v", n, err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM thumb_cache WHERE file_id = ?", lostID).Scan(&n); err != nil || n != 0 {
		t.Errorf("thumb_cache 应显式清除，实际 n=%d err=%v", n, err)
	}
	ftsRows, err := store.countFtsRows()
	if err != nil {
		t.Fatalf("统计 FTS: %v", err)
	}
	// 仅剩 ok 文件的 FTS 行（1 文件行 + 1 页行）
	if ftsRows != 2 {
		t.Errorf("清理后 FTS 应仅剩留存文件的 2 行，实际 %d", ftsRows)
	}
	if _, err := os.Stat(lostPNG); !os.IsNotExist(err) {
		t.Errorf("lost 文件缩略图 PNG 应删除，err=%v", err)
	}
	if _, err := os.Stat(keepPNG); err != nil {
		t.Errorf("留存文件缩略图 PNG 不得误删: %v", err)
	}

	// 幂等：无 lost 记录再清返回 0
	removed, err = svc.ClearLostAssetFiles(lib.ID)
	if err != nil || removed != 0 {
		t.Errorf("无 lost 记录应幂等返回 0，实际 removed=%d err=%v", removed, err)
	}
}

// ---------- 手动重链 ----------

// TestRelocateAssetFile_HashMatch 哈希相等直接复活：改 path + 刷新 mtime/size +
// status=ok，不重解析（页记录/缩略图原样），文件级标注保留。
func TestRelocateAssetFile_HashMatch(t *testing.T) {
	svc := newTestAssetService(t)
	parseCalls := 0
	svc.parseFn = func(src, base string) ([]parsedSlide, error) {
		parseCalls++
		return parsePptxSlides(src, base)
	}
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重链库", dir)
	src := buildAssetsPptx(t, dir, "原位.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	fileID := files[0].ID
	if err := svc.UpdateFileMeta(lib.ID, fileID, model.AssetFileMetaUpdate{
		Rating: 4, Note: "重要", Tags: []string{"重点"},
	}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}
	parseCallsAtStart := parseCalls

	// 移动到库内子目录（os.Rename 保留 mtime+size，内容不变哈希相等）
	newDir := filepath.Join(dir, "新目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "原位.pptx")
	if err := os.Rename(src, newAbs); err != nil {
		t.Fatalf("移动文件: %v", err)
	}
	markFileLostForTest(t, svc, lib.ID, "原位.pptx")

	result, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false)
	if err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	if !result.HashMatched || result.SkippedHashCheck || result.Reparsed {
		t.Errorf("哈希匹配形态应为 HashMatched=true 且零重解析，实际：%+v", result)
	}
	if result.NewPath != "新目录/原位.pptx" {
		t.Errorf("新路径应为正斜杠相对路径，实际 %q", result.NewPath)
	}
	if parseCalls != parseCallsAtStart {
		t.Errorf("哈希匹配不得重解析（解析调用 %d → %d）", parseCallsAtStart, parseCalls)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "新目录/原位.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("重链后状态应为 ok，实际 %q", status)
	}
	// mtime/size 已刷新为新位置实值（下次扫描不误判变更）
	store := svc.openStoreForTest(t, lib.ID)
	info, err := os.Stat(newAbs)
	if err != nil {
		t.Fatalf("stat 新位置: %v", err)
	}
	var size, mtimeNs int64
	if err := store.db.QueryRow("SELECT size, mtime_ns FROM files WHERE path = ?", "新目录/原位.pptx").Scan(&size, &mtimeNs); err != nil {
		t.Fatalf("读记录: %v", err)
	}
	if size != info.Size() || mtimeNs != info.ModTime().UnixNano() {
		t.Errorf("mtime/size 应刷新为新位置实值，实际 size=%d mtimeNs=%d want %d/%d", size, mtimeNs, info.Size(), info.ModTime().UnixNano())
	}
	// 标注保留 + 页记录原样
	gotFiles, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if gotFiles[0].Rating != 4 || gotFiles[0].Note != "重要" || len(gotFiles[0].Tags) != 1 {
		t.Errorf("文件级标注应保留，实际：%+v", gotFiles[0])
	}
	page, err := svc.ListSlides(lib.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "标题" {
		t.Errorf("页记录应原样保留，实际：%+v", page.Items)
	}
	// 记录不存在
	_, err = svc.RelocateAssetFile(lib.ID, fileID+999, newAbs, false)
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Errorf("记录不存在应报可读错误，实际：%v", err)
	}
}

// TestRelocateAssetFile_MismatchForce 哈希不等两分支：force=false 返回
// E_ASSETS_RELINK_MISMATCH 确认型拒绝（记录不动）；force=true 强制重链重解析
// （status=ok、新页内容、标注保留）。目标文件在首次扫描后落盘（扫描后新增的
// 未入库文件——重链到已入库路径会撞 UNIQUE 被拒，见 TestRelocateAssetFile_TargetCollision）。
func TestRelocateAssetFile_MismatchForce(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "强制重链库", dir)
	buildAssetsPptx(t, dir, "原文件.pptx", []assetsSlideSpec{{titlePh: "原标题", body: []string{"原内容"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	// 扫描后落盘的不同内容文件（未入库，哈希必不等）
	otherAbs := buildAssetsPptx(t, dir, "另一份.pptx", []assetsSlideSpec{
		{titlePh: "完全不同的标题", body: []string{"完全不同的内容"}, notes: "别的备注"},
	})
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "原文件.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查记录: %v", err)
	}
	if err := svc.UpdateFileMeta(lib.ID, fileID, model.AssetFileMetaUpdate{Note: "旧备注"}); err != nil {
		t.Fatalf("UpdateFileMeta: %v", err)
	}
	markFileLostForTest(t, svc, lib.ID, "原文件.pptx")

	// force=false：确认型拒绝，记录不动
	result, err := svc.RelocateAssetFile(lib.ID, fileID, otherAbs, false)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsRelinkMismatch {
		t.Fatalf("哈希不等且未 force 应返回 E_ASSETS_RELINK_MISMATCH，实际：%v", err)
	}
	if result.HashMatched || result.Reparsed {
		t.Errorf("拒绝路径不应产生迁移，实际：%+v", result)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "原文件.pptx"); status != model.AssetFileStatusLost {
		t.Errorf("拒绝后记录应保持不动，实际 %q", status)
	}

	// force=true：重链 + 重解析，标注保留
	result, err = svc.RelocateAssetFile(lib.ID, fileID, otherAbs, true)
	if err != nil {
		t.Fatalf("force 重链: %v", err)
	}
	if result.HashMatched || !result.Reparsed {
		t.Errorf("force 形态应为 HashMatched=false Reparsed=true，实际：%+v", result)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "另一份.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("force 重链后状态应为 ok，实际 %q", status)
	}
	var note string
	if err := store.db.QueryRow("SELECT note FROM files WHERE path = ?", "另一份.pptx").Scan(&note); err != nil || note != "旧备注" {
		t.Errorf("文件备注应随重链保留，实际 note=%q err=%v", note, err)
	}
}

// TestRelocateAssetFile_TargetCollision 目标路径已有索引记录的冲突拒绝（可读
// 错误，不落 UNIQUE 原始报错）：重链语义是「改指到未入库的现存文件」，指向
// 已入库路径应改为清理冗余记录。
func TestRelocateAssetFile_TargetCollision(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "冲突库", dir)
	buildAssetsPptx(t, dir, "丢失件.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	buildAssetsPptx(t, dir, "在库件.pptx", []assetsSlideSpec{{titlePh: "另一标题", body: []string{"另一内容"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	var lostID, okID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "丢失件.pptx").Scan(&lostID); err != nil {
		t.Fatalf("查记录: %v", err)
	}
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "在库件.pptx").Scan(&okID); err != nil {
		t.Fatalf("查记录: %v", err)
	}
	markFileLostForTest(t, svc, lib.ID, "丢失件.pptx")

	otherAbs := filepath.Join(dir, "在库件.pptx")
	_, err := svc.RelocateAssetFile(lib.ID, lostID, otherAbs, false)
	if err == nil || !strings.Contains(err.Error(), "已有索引记录") {
		t.Fatalf("目标路径已入库应给出可读冲突错误，实际：%v", err)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "丢失件.pptx"); status != model.AssetFileStatusLost {
		t.Errorf("拒绝后记录不得变化，实际 %q", status)
	}
	// 自身路径重链（owner == 自身）：放行为 no-op 形态（哈希相等复活）
	selfAbs := filepath.Join(dir, "丢失件.pptx")
	result, err := svc.RelocateAssetFile(lib.ID, lostID, selfAbs, false)
	if err != nil {
		t.Fatalf("自身路径重链不应拒绝: %v", err)
	}
	if !result.HashMatched || result.NewPath != "丢失件.pptx" {
		t.Errorf("自身路径重链应为哈希匹配形态，实际：%+v", result)
	}
	_ = okID
}

// TestRelocateAssetFile_OutsideLibRejected 库外目标拒绝（E_ASSETS_RELINK_OUT_OF_LIB）：
// 绝对路径越出库根（`..` 逃逸形态）与库根外普通路径均拒，记录不动。
func TestRelocateAssetFile_OutsideLibRejected(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "越界库", dir)
	buildAssetsPptx(t, dir, "在库内.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "在库内.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查记录: %v", err)
	}
	outsideFile := filepath.Join(t.TempDir(), "库外.pptx")
	if err := os.WriteFile(outsideFile, []byte("x"), 0644); err != nil {
		t.Fatalf("写库外文件: %v", err)
	}
	for _, tc := range []struct {
		name    string
		newPath string
	}{
		{"库根外普通路径", outsideFile},
		{"相对路径形态", "在库内.pptx"},
	} {
		_, err := svc.RelocateAssetFile(lib.ID, fileID, tc.newPath, false)
		var appErr *model.AppError
		if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsRelinkOutsideLib {
			t.Errorf("%s 应返回 E_ASSETS_RELINK_OUT_OF_LIB，实际：%v", tc.name, err)
		}
	}
	if status := fileStatusByPath(t, svc, lib.ID, "在库内.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("拒绝后记录状态不得变化，实际 %q", status)
	}
}

// TestRelocateAssetFile_EmptyHashSkipsCheck 记录哈希为空（加密/权限跳过文件）
// 跳过校验直接重链重解析：即使目标内容与旧不同也放行（force 不要求），
// SkippedHashCheck=true。目标文件在首次扫描后落盘（未入库，避撞 UNIQUE）。
func TestRelocateAssetFile_EmptyHashSkipsCheck(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "空哈希库", dir)
	buildAssetsPptx(t, dir, "加密件.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	otherAbs := buildAssetsPptx(t, dir, "替换件.pptx", []assetsSlideSpec{
		{titlePh: "另一个标题", body: []string{"另一个内容"}},
	})
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "加密件.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查记录: %v", err)
	}
	// 注入空哈希（模拟加密/权限跳过文件无哈希）
	if _, err := store.db.Exec("UPDATE files SET hash = '' WHERE id = ?", fileID); err != nil {
		t.Fatalf("清空哈希: %v", err)
	}
	markFileLostForTest(t, svc, lib.ID, "加密件.pptx")

	result, err := svc.RelocateAssetFile(lib.ID, fileID, otherAbs, false)
	if err != nil {
		t.Fatalf("空哈希重链不应拒绝: %v", err)
	}
	if !result.SkippedHashCheck || result.HashMatched || !result.Reparsed {
		t.Errorf("空哈希形态应为 SkippedHashCheck=true Reparsed=true，实际：%+v", result)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "替换件.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("重链后状态应为 ok，实际 %q", status)
	}
}

// TestRelocateAssetFile_ProcessingRowReparses processing 残留行（崩溃半态）经
// 重链落重解析非直复活：哈希相等的直复活分支须排除 processing——上次解析中断
// 的行 slides/FTS 可能只有半份数据，直翻 ok 后增量扫描 unchanged 跳过永不纠正
// （「processing 必重处理」扫描自愈不变式）。
func TestRelocateAssetFile_ProcessingRowReparses(t *testing.T) {
	svc := newTestAssetService(t)
	parseCalls := 0
	svc.parseFn = func(src, base string) ([]parsedSlide, error) {
		parseCalls++
		return parsePptxSlides(src, base)
	}
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重链处理库", dir)
	src := buildAssetsPptx(t, dir, "中断件.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "中断件.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查记录: %v", err)
	}
	// 注入 processing 残留（行有解析产物、哈希非空——直复活分支三条件恰好全满足）
	if _, err := store.db.Exec("UPDATE files SET status = ? WHERE id = ?",
		model.AssetFileStatusProcessing, fileID); err != nil {
		t.Fatalf("注入 processing: %v", err)
	}
	parseCallsAtStart := parseCalls

	// 移动到库内子目录（os.Rename 保留 mtime+size，内容不变哈希相等）
	newDir := filepath.Join(dir, "新目录")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	newAbs := filepath.Join(newDir, "中断件.pptx")
	if err := os.Rename(src, newAbs); err != nil {
		t.Fatalf("移动文件: %v", err)
	}

	result, err := svc.RelocateAssetFile(lib.ID, fileID, newAbs, false)
	if err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	// HashMatched=true 是哈希事实陈述，Reparsed=true 才是 processing 行的必经
	// 自愈动作（直复活则 Reparsed=false 且解析调用不增）
	if !result.HashMatched || !result.Reparsed {
		t.Fatalf("processing 残留行哈希相等也应落重解析（HashMatched=true Reparsed=true），实际：%+v", result)
	}
	if parseCalls != parseCallsAtStart+1 {
		t.Errorf("processing 残留行应重解析一次（解析调用 %d → %d）", parseCallsAtStart, parseCalls)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "新目录/中断件.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("重解析后状态应为 ok，实际 %q", status)
	}
}

// ---------- 根目录前缀替换 ----------

// buildPrefixFixture 前缀替换共用夹具：库内 旧目录/甲.pptx 与 旧子/乙.pptx 两文件
// （旧目录命中、旧子目录边界外），返回 (lib, 甲abs, 乙abs)。
func buildPrefixFixture(t *testing.T, svc *AssetLibraryService, dir string) (model.AssetLibrary, string, string) {
	t.Helper()
	subA := filepath.Join(dir, "旧目录")
	subB := filepath.Join(dir, "旧子")
	for _, d := range []string{subA, subB} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("建目录: %v", err)
		}
	}
	absA := buildAssetsPptx(t, subA, "甲.pptx", []assetsSlideSpec{{titlePh: "甲标题", body: []string{"甲内容"}}})
	absB := buildAssetsPptx(t, subB, "乙.pptx", []assetsSlideSpec{{titlePh: "乙标题", body: []string{"乙内容"}}})
	lib := mustCreateLibrary(t, svc, "前缀库", dir)
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	return lib, absA, absB
}

// TestAssetPrefixReplace_Preview 命中数预览：目录边界语义（`旧目录` 命中该目录
// 全部文件，不误伤同头缀 `旧子`）；尾斜杠/反斜杠输入归一化；前缀非法/非法新前缀
// 返回参数错误。
func TestAssetPrefixReplace_Preview(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib, _, _ := buildPrefixFixture(t, svc, dir)

	n, err := svc.PreviewAssetPathPrefixReplace(lib.ID, "旧目录", "新目录")
	if err != nil || n != 1 {
		t.Errorf("旧目录应命中 1 条，实际 n=%d err=%v", n, err)
	}
	// 尾斜杠归一化
	n, err = svc.PreviewAssetPathPrefixReplace(lib.ID, "旧目录/", "新目录")
	if err != nil || n != 1 {
		t.Errorf("尾斜杠应归一化命中 1 条，实际 n=%d err=%v", n, err)
	}
	// 目录边界：`旧` 是 `旧目录` 与 `旧子` 的共同头缀，但均非 `旧/` 子级 → 零命中
	n, err = svc.PreviewAssetPathPrefixReplace(lib.ID, "旧", "新")
	if err != nil || n != 0 {
		t.Errorf("头缀不越目录边界应零命中，实际 n=%d err=%v", n, err)
	}
	// 非法前缀参数错误
	if _, err := svc.PreviewAssetPathPrefixReplace(lib.ID, "", "新"); err == nil {
		t.Error("空旧前缀应拒绝")
	}
	if _, err := svc.PreviewAssetPathPrefixReplace(lib.ID, "旧目录", ""); err == nil {
		t.Error("空新前缀应拒绝")
	}
	if _, err := svc.PreviewAssetPathPrefixReplace(lib.ID, "../逃逸", "新"); err == nil {
		t.Error("旧前缀相对逃逸应拒绝")
	}
}

// TestAssetPrefixReplace_Execute_MoveWithFiles 文件随目录移动的主链路：批量改
// 路径 → 新位置探测匹配 → lost 行复活 ok → 自动增量扫描收敛（total/unchanged
// 口径验证扫描已执行）。
func TestAssetPrefixReplace_Execute_MoveWithFiles(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib, absA, _ := buildPrefixFixture(t, svc, dir)
	markFileLostForTest(t, svc, lib.ID, "旧目录/甲.pptx")

	// 目录重组：旧目录 → 新目录（文件随之）
	newDir := filepath.Join(dir, "新目录")
	if err := os.Rename(filepath.Join(dir, "旧目录"), newDir); err != nil {
		t.Fatalf("目录重命名: %v", err)
	}

	result, err := svc.ReplaceAssetPathPrefix(lib.ID, "旧目录", "新目录")
	if err != nil {
		t.Fatalf("ReplaceAssetPathPrefix: %v", err)
	}
	if result.Replaced != 1 || result.Revived != 1 || result.Reparsed != 0 || result.StillLost != 0 {
		t.Fatalf("移动主链路应 replaced=1 revived=1，实际：%+v", result)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "新目录/甲.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("复活后状态应为 ok，实际 %q", status)
	}
	if _, err := os.Stat(filepath.Join(dir, "新目录", "甲.pptx")); err != nil {
		t.Errorf("新位置文件应存在: %v", err)
	}
	_ = absA
	// 自动增量扫描已执行：全库 2 文件全部收敛 unchanged（甲已复活 ok 且 mtime+size
	// 匹配；乙未波及）
	if result.Scan.Total != 2 || result.Scan.Unchanged != 2 {
		t.Errorf("自动扫描应收敛全部文件，实际：%+v", result.Scan)
	}
}

// TestAssetPrefixReplace_Execute_ThreeStates 三态收敛：新位置缺失保持 lost、
// 已变触发重解析、（匹配复活已在主链路覆盖）。
func TestAssetPrefixReplace_Execute_ThreeStates(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib, _, _ := buildPrefixFixture(t, svc, dir)
	markFileLostForTest(t, svc, lib.ID, "旧目录/甲.pptx")
	markFileLostForTest(t, svc, lib.ID, "旧子/乙.pptx")

	// 甲：随目录移动且内容已变（新位置重建为新内容）；乙：新位置不存在
	if err := os.Rename(filepath.Join(dir, "旧目录"), filepath.Join(dir, "新目录")); err != nil {
		t.Fatalf("目录重命名: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "新目录", "甲.pptx")); err != nil {
		t.Fatalf("移除移动后文件: %v", err)
	}
	buildAssetsPptx(t, filepath.Join(dir, "新目录"), "甲.pptx", []assetsSlideSpec{
		{titlePh: "改版标题", body: []string{"改版内容"}},
	})

	result, err := svc.ReplaceAssetPathPrefix(lib.ID, "旧目录", "新目录")
	if err != nil {
		t.Fatalf("ReplaceAssetPathPrefix: %v", err)
	}
	if result.Replaced != 1 || result.Reparsed != 1 {
		t.Fatalf("已变文件应重解析（replaced=1 reparsed=1），实际：%+v", result)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "新目录/甲.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("重解析后状态应为 ok，实际 %q", status)
	}
	page, err := svc.ListSlides(lib.ID, 0, 0)
	if err != nil {
		t.Fatalf("ListSlides: %v", err)
	}
	var gotNew bool
	for _, s := range page.Items {
		if s.Title == "改版标题" {
			gotNew = true
		}
	}
	if !gotNew {
		t.Errorf("重解析后应出现新页内容，实际：%+v", page.Items)
	}

	// 乙：新位置不存在 → 保持 lost（stillLost 计数在乙的替换事务内；单独跑一次
	// 两前缀替换验证 stillLost 口径）
	if _, err := svc.ReplaceAssetPathPrefix(lib.ID, "旧子", "不存在目录"); err != nil {
		t.Fatalf("乙前缀替换: %v", err)
	}
	result2, err := svc.ReplaceAssetPathPrefix(lib.ID, "不存在目录", "再换目录")
	if err != nil {
		t.Fatalf("乙二次替换: %v", err)
	}
	if result2.Replaced != 1 || result2.StillLost != 1 || result2.Revived != 0 {
		t.Fatalf("新位置缺失应 stillLost=1，实际：%+v", result2)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "再换目录/乙.pptx"); status != model.AssetFileStatusLost {
		t.Errorf("新位置缺失应保持 lost，实际 %q", status)
	}
}

// TestAssetPrefixReplace_Guards 防护面：零命中确认型拒绝、新前缀越界整体拒绝
// （记录零改动）、目标路径与现有记录冲突拒绝、新旧前缀相同拒绝。自带夹具——
// 「旧子」目录内置同名文件 甲.pptx 构造 UNIQUE 冲突形态（全部防护用例均不得
// 产生成功替换，状态断言 therefore 前后一致）。
func TestAssetPrefixReplace_Guards(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	for _, sub := range []string{"旧目录", "旧子"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatalf("建目录: %v", err)
		}
	}
	buildAssetsPptx(t, filepath.Join(dir, "旧目录"), "甲.pptx", []assetsSlideSpec{{titlePh: "甲标题", body: []string{"甲内容"}}})
	buildAssetsPptx(t, filepath.Join(dir, "旧子"), "甲.pptx", []assetsSlideSpec{{titlePh: "同名标题", body: []string{"同名内容"}}})
	lib := mustCreateLibrary(t, svc, "防护库", dir)
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	markFileLostForTest(t, svc, lib.ID, "旧目录/甲.pptx")

	// 零命中：确认型错误码
	_, err := svc.ReplaceAssetPathPrefix(lib.ID, "不存在前缀", "新目录")
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsPrefixNoMatch {
		t.Errorf("零命中应返回 E_ASSETS_PREFIX_REPLACE_NO_MATCH，实际：%v", err)
	}

	// 新前缀越界（中段 .. 清理后仍逃逸库根——「旧/../逃逸」会被路径清理消化为
	// 「逃逸」而不越界，须双 .. 才真逃出；前导 .. 已被前缀归一化先行拒绝）：
	// 整体拒绝，记录零改动
	_, err = svc.ReplaceAssetPathPrefix(lib.ID, "旧目录", "旧/../../逃逸")
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsRelinkOutsideLib {
		t.Errorf("越界应返回 E_ASSETS_RELINK_OUT_OF_LIB，实际：%v", err)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "旧目录/甲.pptx"); status != model.AssetFileStatusLost {
		t.Errorf("拒绝后记录不得变化，实际 %q", status)
	}

	// 新前缀相同（归一化后）
	if _, err := svc.ReplaceAssetPathPrefix(lib.ID, "旧目录", "旧目录/"); err == nil {
		t.Error("归一化后相同的新旧前缀应拒绝")
	}

	// 目标路径与现有记录冲突：把 旧目录 替换为 旧子，甲.pptx 与 旧子/甲.pptx 撞 UNIQUE
	_, err = svc.ReplaceAssetPathPrefix(lib.ID, "旧目录", "旧子")
	if err == nil || !strings.Contains(err.Error(), "冲突") {
		t.Errorf("目标路径冲突应拒绝并给出可读错误，实际：%v", err)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "旧目录/甲.pptx"); status != model.AssetFileStatusLost {
		t.Errorf("冲突拒绝后记录不得变化，实际 %q", status)
	}
}

// TestAssetScan_LostRevival_NonOkRowReclassifies 无解析产物行（encrypted，failed/
// denied 同理）丢失后文件原样回归：mtime+size 未变也不得直复活 ok——须走重解析
// 按实际内容重新分类（直复活会把坏文件谎报为 ok，且此后增量扫描 unchanged 跳过
// 永不纠正）。
func TestAssetScan_LostRevival_NonOkRowReclassifies(t *testing.T) {
	svc := newTestAssetService(t)
	parseCalls := 0
	svc.parseFn = func(src, base string) ([]parsedSlide, error) {
		parseCalls++
		return nil, &assetParseError{kind: assetParseEncrypted, err: errors.New("OLE 加密魔数")}
	}
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "加密复活库", dir)
	src := buildAssetsPptx(t, dir, "加密件.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "加密件.pptx"); status != model.AssetFileStatusEncrypted {
		t.Fatalf("前置态应为 encrypted，实际 %q", status)
	}
	firstParseCalls := parseCalls
	markFileLostForTest(t, svc, lib.ID, "加密件.pptx")

	// 移出再移回（os.Rename 同卷保留 mtime+size）→ 走重解析重新分类，不直复活
	outside := filepath.Join(t.TempDir(), "加密件.pptx")
	if err := os.Rename(src, outside); err != nil {
		t.Fatalf("移出文件: %v", err)
	}
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("标记 lost 扫描: %v", err)
	}
	if err := os.Rename(outside, src); err != nil {
		t.Fatalf("移回文件: %v", err)
	}
	result, err := svc.StartScan(context.Background(), lib.ID)
	if err != nil {
		t.Fatalf("回归扫描: %v", err)
	}
	if result.Revived != 0 || result.Encrypted != 1 {
		t.Fatalf("无产物行回归应重解析重新分类（revived=0 encrypted=1），实际 %+v", result)
	}
	if parseCalls != firstParseCalls+1 {
		t.Errorf("无产物行回归应重解析一次（解析调用 %d → %d）", firstParseCalls, parseCalls)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "加密件.pptx"); status != model.AssetFileStatusEncrypted {
		t.Errorf("重解析后应保持 encrypted（不得谎报 ok），实际 %q", status)
	}
}

// TestAssetPrefixReplace_NestedPrefixVacated 嵌套前缀（新前缀位于旧前缀子树内）
// 的链式腾位：本集记录的新路径撞另一条本集记录的旧路径时，对方让位后落位成功
// ——替换集自身路径不构成冲突（回归：冲突预检曾把替换集自身旧路径计入冲突表，
// 「旧目录 → 旧目录/归档」归档重组形态被误拒）。
func TestAssetPrefixReplace_NestedPrefixVacated(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "旧目录", "归档")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	buildAssetsPptx(t, filepath.Join(dir, "旧目录"), "甲.pptx", []assetsSlideSpec{{titlePh: "甲标题", body: []string{"甲内容"}}})
	buildAssetsPptx(t, sub, "甲.pptx", []assetsSlideSpec{{titlePh: "归档标题", body: []string{"归档内容"}}})
	lib := mustCreateLibrary(t, svc, "嵌套前缀库", dir)
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}

	// 旧目录 → 旧目录/归档：甲1（旧目录/甲.pptx）新路径 = 甲2（旧目录/归档/
	// 甲.pptx）的旧路径——甲2 同事务让位后甲1 落位，非真冲突
	result, err := svc.ReplaceAssetPathPrefix(lib.ID, "旧目录", "旧目录/归档")
	if err != nil {
		t.Fatalf("嵌套前缀腾位不应误拒: %v", err)
	}
	if result.Replaced != 2 {
		t.Fatalf("应替换 2 条，实际 %+v", result)
	}
	// 甲1 新位置是甲2 的同名文件（size/mtime 不匹配）→ 重解析；
	// 甲2 新位置（旧目录/归档/归档/甲.pptx）磁盘不存在 → 保持 lost
	if result.Reparsed != 1 || result.StillLost != 1 {
		t.Errorf("三态收敛应 reparsed=1 stillLost=1，实际：%+v", result)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "旧目录/归档/甲.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("腾位后甲1 应落位重解析为 ok，实际 %q", status)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "旧目录/归档/归档/甲.pptx"); status != model.AssetFileStatusLost {
		t.Errorf("腾位后甲2 应保持 lost，实际 %q", status)
	}
}

// TestAssetPrefixReplace_MixedLengthPairing 混合长度命中集的逐记录配对回归：
// 事务后探测须按 fileID 取记录快照配对自身新路径，禁 records[i] ↔ renames[i]
// 下标 zip——renames 已按旧路径长度降序重排（嵌套腾位安全序），与 records 原序
// （path 升序）不同序，下标配对在混合长度命中集下把记录 a 配到记录 b 的新路径
// （重解析/lost 标记写错行：现存文件误标 lost、他人新路径直复活）。既有
// NestedPrefixVacated 因「归档」(E5) 字节序小于「甲」(E7)、长路径恰排在升序
// 前面的同序巧合未暴露；本夹具用「乙」(E4) < 「子」(E5) 构造两序分化（path
// 升序短路径在前，长度降序长路径在前）。
func TestAssetPrefixReplace_MixedLengthPairing(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "旧目录", "子")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	// 两文件内容长度刻意不同：修复后乙1 探测自身新路径（其内是乙2 的文件）以
	// size 失配确定性走重解析，不依赖 mtime 时序（对齐 test-stability）
	buildAssetsPptx(t, filepath.Join(dir, "旧目录"), "乙.pptx", []assetsSlideSpec{{titlePh: "乙一标题", body: []string{"正文"}}})
	buildAssetsPptx(t, sub, "乙.pptx", []assetsSlideSpec{
		{titlePh: "乙二标题", body: []string{"乙二内容第一行", "乙二内容第二行", "乙二内容第三行"}},
	})
	lib := mustCreateLibrary(t, svc, "混合长度库", dir)
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	// 前置断言：records（path 升序）与 renames 排序键（oldPath 长度降序）确为
	// 不同序——否则本用例退化为同序巧合、失去回归力
	store := svc.openStoreForTest(t, lib.ID)
	records, err := store.listFilesByPathPrefix("旧目录")
	if err != nil {
		t.Fatalf("读命中记录: %v", err)
	}
	if len(records) != 2 || len(records[0].path) >= len(records[1].path) {
		t.Fatalf("夹具应保证 path 升序首条为短路径（两序分化），实际：%q %q",
			records[0].path, records[1].path)
	}

	// 旧目录 → 旧目录/子：乙1（旧目录/乙.pptx）新路径 = 乙2（旧目录/子/乙.pptx）
	// 的旧路径——对方同事务让位腾出，非真冲突
	result, err := svc.ReplaceAssetPathPrefix(lib.ID, "旧目录", "旧目录/子")
	if err != nil {
		t.Fatalf("混合长度嵌套腾位不应误拒: %v", err)
	}
	if result.Replaced != 2 {
		t.Fatalf("应替换 2 条，实际 %+v", result)
	}
	// 逐记录配对自身新路径：乙1 → 旧目录/子/乙.pptx（磁盘是乙2 的文件，size
	// 失配）落重解析；乙2 → 旧目录/子/子/乙.pptx（磁盘缺失）保持 lost。下标
	// zip 配对则相反（乙1 配到缺失路径误标 lost，乙2 配到装着自己旧文件的路径
	// 直复活，探测期 Reparsed=0）
	if result.Reparsed != 1 || result.StillLost != 1 || result.Revived != 0 {
		t.Fatalf("逐记录配对应 reparsed=1 stillLost=1，实际：%+v", result)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "旧目录/子/乙.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("乙1 应落位重解析为 ok，实际 %q", status)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "旧目录/子/子/乙.pptx"); status != model.AssetFileStatusLost {
		t.Errorf("乙2 应保持 lost，实际 %q", status)
	}
}

// TestAssetPrefixReplace_ProcessingRowReparses processing 残留行经前缀替换落
// 重解析非直复活：匹配收敛分支须排除 processing——文件随目录移动（os.Rename
// 同卷保留 mtime+size）且行有解析产物时三条件恰好全满足，不排除则崩溃半态行
// 直翻 ok 绕过「processing 必重处理」扫描自愈不变式。
func TestAssetPrefixReplace_ProcessingRowReparses(t *testing.T) {
	svc := newTestAssetService(t)
	parseCalls := 0
	svc.parseFn = func(src, base string) ([]parsedSlide, error) {
		parseCalls++
		return parsePptxSlides(src, base)
	}
	dir := t.TempDir()
	lib, _, _ := buildPrefixFixture(t, svc, dir)
	// 注入 processing 残留（首扫成功：行有解析产物——直复活分支前置全满足）
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET status = ? WHERE path = ?",
		model.AssetFileStatusProcessing, "旧目录/甲.pptx"); err != nil {
		t.Fatalf("注入 processing: %v", err)
	}
	parseCallsAtStart := parseCalls

	// 目录随文件移动（新位置 mtime+size 与记录一致——匹配分支前置成立）
	if err := os.Rename(filepath.Join(dir, "旧目录"), filepath.Join(dir, "新目录")); err != nil {
		t.Fatalf("目录重命名: %v", err)
	}

	result, err := svc.ReplaceAssetPathPrefix(lib.ID, "旧目录", "新目录")
	if err != nil {
		t.Fatalf("ReplaceAssetPathPrefix: %v", err)
	}
	if result.Replaced != 1 || result.Reparsed != 1 || result.Revived != 0 {
		t.Fatalf("processing 残留行应落重解析（reparsed=1 revived=0），实际：%+v", result)
	}
	if parseCalls != parseCallsAtStart+1 {
		t.Errorf("processing 残留行应重解析一次（解析调用 %d → %d）", parseCallsAtStart, parseCalls)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "新目录/甲.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("重解析后状态应为 ok，实际 %q", status)
	}
}

// ---------- 清理与替换/重链并发窗口的写面防御 ----------

// TestRelinkWrites_ZeroRowsAffectedOnClearedRecord 写面 UPDATE 零命中防御：行已
// 被「清理丢失记录」并发删除（ClearLostAssetFiles 不持扫描锁，与替换/重链存在
// 并发窗口）时，relocateFileOK / relocateFileProcessing / replaceFilePaths 的
// UPDATE 空命中不得静默零操作——静默会按名册虚报成功，且替换事务后探测段对幽灵
// 行 processScanFile 落 replaceFileSlides 会因 slides 外键违例上抛模糊错误。三写
// 面以 sql.ErrNoRows 上抛（服务层转「记录已被清理」可读错误），replaceFilePaths
// 整体回滚（先提交的正常行随事务一并回退，探测段自然不再进入）。真并发窗口在
// 服务层无确定性构造点（锁内链路全同步），白盒直调 store 层断言防御分支。
func TestRelinkWrites_ZeroRowsAffectedOnClearedRecord(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "并发清理库", dir)
	buildAssetsPptx(t, dir, "被清理.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	buildAssetsPptx(t, dir, "旁观.pptx", []assetsSlideSpec{{titlePh: "旁标题", body: []string{"旁内容"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	var ghostID, keepID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "被清理.pptx").Scan(&ghostID); err != nil {
		t.Fatalf("查被清理记录: %v", err)
	}
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "旁观.pptx").Scan(&keepID); err != nil {
		t.Fatalf("查旁观记录: %v", err)
	}
	// 模拟并发窗口终态：ghost 行已被「清理丢失记录」删除
	if _, err := store.db.Exec("DELETE FROM files WHERE id = ?", ghostID); err != nil {
		t.Fatalf("删除行: %v", err)
	}

	// relocateFileOK / relocateFileProcessing：零命中 → sql.ErrNoRows
	if err := store.relocateFileOK(ghostID, "新位置.pptx", 1, 1, "now"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("relocateFileOK 零命中应返回 sql.ErrNoRows，实际：%v", err)
	}
	if err := store.relocateFileProcessing(ghostID, "新位置.pptx", "now"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("relocateFileProcessing 零命中应返回 sql.ErrNoRows，实际：%v", err)
	}
	// replaceFilePaths：混入幽灵行 → sql.ErrNoRows 且事务整体回滚（旁观行不动）
	err := store.replaceFilePaths([]assetPathRename{
		{fileID: keepID, oldPath: "旁观.pptx", newRel: "新目录/旁观.pptx"},
		{fileID: ghostID, oldPath: "被清理.pptx", newRel: "新目录/被清理.pptx"},
	}, "now", "", "")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("replaceFilePaths 零命中应返回 sql.ErrNoRows，实际：%v", err)
	}
	if status := fileStatusByPath(t, svc, lib.ID, "旁观.pptx"); status != model.AssetFileStatusOK {
		t.Errorf("事务应整体回滚（旁观行不得变化），实际 %q", status)
	}
	var n int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM files WHERE path = ?", "新目录/旁观.pptx").Scan(&n); err != nil || n != 0 {
		t.Errorf("回滚后不得出现新路径行，实际 n=%d err=%v", n, err)
	}
}
