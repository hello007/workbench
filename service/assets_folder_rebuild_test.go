package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// 子目录级重建索引（RebuildAssetFolder，PRD「子目录右键重建」）用例：夹具 =
// 临时目录 + 伪 pptx + parseFn fake（对齐 assets_folders_ops_test.go 纪律，禁
// 依赖本机 Office 与真实文件系统时序）。核心契约：范围收敛（只清+只扫目标子树
// 范围外零触碰）、标注 path 重绑恢复、消失记录直清不转 lost、守卫拒绝面
// （根/越界/排除命中/不存在目录）与扫描互斥（E_ASSETS_SCAN_IN_PROGRESS）。

// TestRebuildAssetFolder_SubtreeScopeAndRebind 范围收敛 + 标注重绑端到端：
// 两目录各含文件并打标注；子目录改动（删一文件 + 新增一文件）后重建该子目录
// ——范围外目录记录/标注零触碰，范围内消失记录直清（不转 lost）、新增入库、
// 保留文件标注经 defer 重绑按 path 恢复。
func TestRebuildAssetFolder_SubtreeScopeAndRebind(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "子目录重建库", root)
	mustWritePptxFile(t, filepath.Join(root, "方案A", "a1.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "方案A", "a2.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "方案B", "b1.pptx"))
	mustScan(t, svc, lib.ID)

	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	idsByPath := make(map[string]int64, len(files))
	for _, f := range files {
		idsByPath[f.Path] = f.ID
	}
	if err := svc.UpdateFileMeta(lib.ID, idsByPath["方案A/a1.pptx"], model.AssetFileMetaUpdate{Tags: []string{"文标A"}}); err != nil {
		t.Fatalf("a1 文件打标签: %v", err)
	}
	if err := svc.UpdateFileMeta(lib.ID, idsByPath["方案B/b1.pptx"], model.AssetFileMetaUpdate{Tags: []string{"文标B"}}); err != nil {
		t.Fatalf("b1 文件打标签: %v", err)
	}
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["方案A/a1.pptx"], 1, model.AssetSlideMetaUpdate{
		Rating: 4, Note: "a1 备注", TagNames: []string{"页标A"},
	})
	mustUpdateSlideMeta(t, svc, lib.ID, idsByPath["方案B/b1.pptx"], 1, model.AssetSlideMetaUpdate{
		Rating: 2, Note: "b1 备注", TagNames: []string{"页标B"},
	})

	// 子目录改动：删 a2（重建后其记录应直清不转 lost）、增 a3（重扫入库）
	if err := os.Remove(filepath.Join(root, "方案A", "a2.pptx")); err != nil {
		t.Fatalf("Remove a2: %v", err)
	}
	mustWritePptxFile(t, filepath.Join(root, "方案A", "a3.pptx"))

	result, err := svc.RebuildAssetFolder(context.Background(), lib.ID, "方案A")
	if err != nil {
		t.Fatalf("RebuildAssetFolder: %v", err)
	}
	if result.Total != 2 || result.New != 2 {
		t.Fatalf("重建结果应 total=2 new=2（a1/a3 全量新增），实际：%+v", result)
	}

	files, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("重建后 ListFiles: %v", err)
	}
	after := make(map[string]model.AssetFileEntry, len(files))
	for _, f := range files {
		after[f.Path] = f
	}
	// 范围内：a2 消失记录直清（无 lost 残留）、a1/a3 在
	if _, ok := after["方案A/a2.pptx"]; ok {
		t.Errorf("消失文件 a2 的记录应随子目录重建直清（不转 lost），实际仍在索引")
	}
	a1, ok := after["方案A/a1.pptx"]
	if !ok {
		t.Fatalf("a1 应在索引，实际：%+v", files)
	}
	if _, ok := after["方案A/a3.pptx"]; !ok {
		t.Errorf("新增文件 a3 应在索引")
	}
	// 范围外：b1 记录与标注零触碰
	b1, ok := after["方案B/b1.pptx"]
	if !ok {
		t.Fatalf("范围外 b1 应在索引，实际：%+v", files)
	}
	if len(b1.Tags) != 1 || b1.Tags[0].Name != "文标B" {
		t.Errorf("范围外 b1 文件标签应原样保留，实际：%+v", b1.Tags)
	}
	keptB := slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "方案B/b1.pptx", 1)
	if keptB.Rating != 2 || keptB.Note != "b1 备注" || len(keptB.OwnTags) != 1 || keptB.OwnTags[0].Name != "页标B" {
		t.Errorf("范围外 b1 页标注应原样保留，实际：%+v", keptB)
	}
	// 范围内：a1 标注经 defer 重绑按 path 恢复
	if len(a1.Tags) != 1 || a1.Tags[0].Name != "文标A" {
		t.Errorf("a1 文件标签应经重绑恢复，实际：%+v", a1.Tags)
	}
	keptA := slideEntryByNum(t, mustListSlides(t, svc, lib.ID), "方案A/a1.pptx", 1)
	if keptA.Rating != 4 || keptA.Note != "a1 备注" || len(keptA.OwnTags) != 1 || keptA.OwnTags[0].Name != "页标A" {
		t.Errorf("a1 页标注应经重绑恢复，实际：%+v", keptA)
	}
}

// TestRebuildAssetFolder_NestedFolder 深层子目录（多段路径）重建：前缀命中含
// 目录自身整棵子树，深层文件入库后库内路径正确（folderRel 前缀拼接）。
func TestRebuildAssetFolder_NestedFolder(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "深层重建库", root)
	mustWritePptxFile(t, filepath.Join(root, "归档", "2025", "旧.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "其它", "x.pptx"))
	mustScan(t, svc, lib.ID)

	if err := os.Remove(filepath.Join(root, "归档", "2025", "旧.pptx")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	mustWritePptxFile(t, filepath.Join(root, "归档", "2025", "新.pptx"))

	result, err := svc.RebuildAssetFolder(context.Background(), lib.ID, "归档/2025")
	if err != nil {
		t.Fatalf("RebuildAssetFolder(归档/2025): %v", err)
	}
	if result.Total != 1 || result.New != 1 {
		t.Fatalf("重建结果应 total=1 new=1，实际：%+v", result)
	}
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	paths := make(map[string]bool, len(files))
	for _, f := range files {
		paths[f.Path] = true
	}
	if !paths["归档/2025/新.pptx"] {
		t.Errorf("深层新文件应以库内相对路径入库，实际：%+v", files)
	}
	if paths["归档/2025/旧.pptx"] {
		t.Errorf("深层旧文件记录应已清除")
	}
	if !paths["其它/x.pptx"] {
		t.Errorf("范围外文件应零触碰仍在索引")
	}
}

// TestRebuildAssetFolder_Rejects 守卫拒绝面（错误在清库前返回，索引零影响）：
// 库根空串（全量重建走 RebuildScan）、越界路径、排除规则命中目录、不存在的
// 目录、指向文件（非目录）。
func TestRebuildAssetFolder_Rejects(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "拒绝面库", root)
	if err := os.MkdirAll(filepath.Join(root, "secret"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mustWritePptxFile(t, filepath.Join(root, "secret", "s.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "普通.pptx"))
	mustScan(t, svc, lib.ID)

	// 排除规则命中目录（写规则后未重扫不影响守卫判定——守卫按 matcher 直接判）
	if err := svc.setLibraryRules(lib.ID, []string{"secret"}, nil); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}

	cases := []struct {
		name      string
		folder    string
		wantSplit string // 期望错误消息含的判别片段
	}{
		{"库根空串", "", "全量重建"},
		{"越界路径", "../../outside", "越界|非法"},
		{"排除命中目录", "secret", "排除"},
		{"不存在的目录", "不存在目录", "不存在"},
	}
	for _, tc := range cases {
		_, err := svc.RebuildAssetFolder(context.Background(), lib.ID, tc.folder)
		if err == nil {
			t.Errorf("%s: 应拒绝，实际成功", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.wantSplit) && tc.wantSplit != "" {
			// 越界分支的错误文案含「越界」或「非法」任一即通过（resolveLibraryRelPath 单点）
			if tc.name != "越界路径" {
				t.Errorf("%s: 错误消息应含「%s」，实际：%v", tc.name, tc.wantSplit, err)
			} else if !strings.Contains(err.Error(), "越界") && !strings.Contains(err.Error(), "非法") {
				t.Errorf("越界路径: 错误消息应含「越界」或「非法」，实际：%v", err)
			}
		}
	}

	// 拒绝后索引零影响（清库未发生）
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("拒绝后索引应原样（2 个文件），实际：%+v", files)
	}
}

// TestRebuildAssetFolder_ScanInProgressRejected 扫描互斥：占扫描锁后调用返回
// E_ASSETS_SCAN_IN_PROGRESS（与 StartScan/RebuildScan 同锁口径）。
func TestRebuildAssetFolder_ScanInProgressRejected(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "互斥库", root)
	mustWritePptxFile(t, filepath.Join(root, "目录", "x.pptx"))
	mustScan(t, svc, lib.ID)

	release, err := svc.tryLockScan(lib.ID)
	if err != nil {
		t.Fatalf("tryLockScan: %v", err)
	}
	defer release()
	_, err = svc.RebuildAssetFolder(context.Background(), lib.ID, "目录")
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsScanInProgress {
		t.Fatalf("应返回 E_ASSETS_SCAN_IN_PROGRESS，实际：%v", err)
	}
}
