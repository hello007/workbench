package service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- 审核缺陷修复（10-31-b-review-fixes） ----------

// TestAssetSplitReplace_SourceModifiedDuringSplitRollsBack 源文件在核心读取后
// 被修改：删源前 Stat 复核不一致 → 回滚本次产物与索引行，源保留、回收站不调用。
func TestAssetSplitReplace_SourceModifiedDuringSplitRollsBack(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "替换库", dir)
	src := buildAssetsPptx(t, dir, "方案.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = func(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error) {
		res, err := (&fakeReplaceCore{pages: 2, payload: []byte("page-bytes")}).split(ctx, srcPath, onProgress)
		if err != nil {
			return res, err
		}
		// 模拟拆分窗口内外部保存：内容与大小变化
		return res, os.WriteFile(srcPath, []byte(strings.Repeat("changed-after-core", 64)), 0644)
	}
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "replace"); err == nil {
		t.Fatal("源文件被修改应返回失败")
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("源文件应保留：%v", err)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("复核失败不得移动源文件，实际 %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "方案", "001-方案.pptx")); !os.IsNotExist(err) {
		t.Errorf("回滚后不应残留产物，stat err=%v", err)
	}
}

// TestAssetSplitReplace_RestoredSourceResplits 源文件从回收站还原并重新扫描后，
// 拆分记录虽标记 source_removed，仍应走完整重拆而不是「已最新」跳过。
func TestAssetSplitReplace_RestoredSourceResplits(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "替换库", dir)
	src := buildAssetsPptx(t, dir, "方案.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("读取源文件: %v", err)
	}
	mustScanLibrary(t, svc, lib.ID)
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("page-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "replace"); err != nil {
		t.Fatalf("首次替换: %v", err)
	}
	// 模拟用户从回收站还原源文件并重新扫描
	if err := os.WriteFile(src, original, 0644); err != nil {
		t.Fatalf("还原源文件: %v", err)
	}
	mustScanLibrary(t, svc, lib.ID)

	// 还原后页内容未变，但 source_removed 记录不得触发跳过
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "replace")
	if err != nil {
		t.Fatalf("还原后重拆: %v", err)
	}
	if res.Skipped {
		t.Fatal("源文件已还原，不应命中「已最新」跳过")
	}
	if !res.SourceRemoved {
		t.Error("重拆成功应再次移入回收站")
	}
	if got := recycleCalls(); len(got) != 2 {
		t.Errorf("回收站应被调用两次，实际 %d：%v", len(got), got)
	}
}

// TestAssetSplitReplace_RollbackKeepsPreexistingUserFile 回滚只删本次新增产物，
// 目录内拆分前已存在的同形用户文件保留。
func TestAssetSplitReplace_RollbackKeepsPreexistingUserFile(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "替换库", dir)
	src := buildAssetsPptx(t, dir, "方案.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	outDir := filepath.Join(dir, "方案")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("建产物目录: %v", err)
	}
	userFile := filepath.Join(outDir, "009-方案.pptx")
	if err := os.WriteFile(userFile, []byte("user-created"), 0644); err != nil {
		t.Fatalf("写用户文件: %v", err)
	}
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("page-bytes"), dropFile: true}).split
	move, _ := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "replace"); err == nil {
		t.Fatal("产物缺失应回滚并返回失败")
	}
	if _, err := os.Stat(userFile); err != nil {
		t.Errorf("拆分前已存在的用户文件不得被回滚删除：%v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "方案-001.pptx")); !os.IsNotExist(err) {
		t.Errorf("本次核心中间态产物应被回滚删除，stat err=%v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("源文件应保留：%v", err)
	}
}

// TestAssetSplitReplace_ExcludeRuleRejectsProduct replace 产物命中库排除规则时
// 拒绝并回滚，不写入随即会被扫描转 excluded 的索引行。
func TestAssetSplitReplace_ExcludeRuleRejectsProduct(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "替换库", dir)
	src := buildAssetsPptx(t, dir, "方案.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	if _, err := svc.UpdateLibraryRules(lib.ID, []string{"方案"}, nil); err != nil {
		t.Fatalf("设置排除规则: %v", err)
	}
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("page-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	_, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "replace")
	if err == nil || !strings.Contains(err.Error(), "排除规则") {
		t.Fatalf("命中排除规则应失败并说明原因，err=%v", err)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("源文件应保留：%v", statErr)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("不得移动源文件，实际 %v", got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "方案", "001-方案.pptx")); !os.IsNotExist(statErr) {
		t.Errorf("命中排除的回滚应删除产物，stat err=%v", statErr)
	}
}

// TestFinalizeReplaceSource_MigratesLateAnnotationsAndDeletesRow 删源收尾单事务：
// 晚到的源文件级评分/备注在删行前再次迁移到产物，源行同步删除。
func TestFinalizeReplaceSource_MigratesLateAnnotationsAndDeletesRow(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "替换库", dir)
	buildAssetsPptx(t, dir, "源.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	prodDir := filepath.Join(dir, "源")
	if err := os.MkdirAll(prodDir, 0755); err != nil {
		t.Fatalf("建产物目录: %v", err)
	}
	buildAssetsPptx(t, prodDir, "001-源.pptx", []assetsSlideSpec{{titlePh: "一", body: []string{"内容"}}})
	buildAssetsPptx(t, prodDir, "002-源.pptx", []assetsSlideSpec{{titlePh: "二", body: []string{"内容"}}})
	mustScanLibrary(t, svc, lib.ID)

	store := svc.openStoreForTest(t, lib.ID)
	srcRec, srcExists, err := store.fileRecordByPath("源.pptx")
	if err != nil || !srcExists {
		t.Fatalf("源索引记录: exists=%v err=%v", srcExists, err)
	}
	p1, ok1, err1 := store.fileRecordByPath("源/001-源.pptx")
	p2, ok2, err2 := store.fileRecordByPath("源/002-源.pptx")
	if err1 != nil || err2 != nil || !ok1 || !ok2 {
		t.Fatalf("产物索引记录: %v %v %v %v", ok1, ok2, err1, err2)
	}
	// 模拟标注在预迁移之后、删行之前到达
	if _, err := store.db.Exec("UPDATE files SET rating = 5, note = '晚到备注' WHERE id = ?", srcRec.id); err != nil {
		t.Fatalf("写入晚到标注: %v", err)
	}
	targets := []splitAnnotationTarget{{pageNum: 1, fileID: p1.id}, {pageNum: 2, fileID: p2.id}}
	if err := store.finalizeReplaceSource(srcRec.id, targets); err != nil {
		t.Fatalf("finalizeReplaceSource: %v", err)
	}
	for _, tc := range []struct {
		name string
		id   int64
	}{{"001", p1.id}, {"002", p2.id}} {
		var rating int
		var note string
		if err := store.db.QueryRow("SELECT rating, note FROM files WHERE id = ?", tc.id).Scan(&rating, &note); err != nil {
			t.Fatalf("读产物 %s 标注: %v", tc.name, err)
		}
		if rating != 5 || note != "晚到备注" {
			t.Errorf("产物 %s 应带晚到标注，实际 rating=%d note=%q", tc.name, rating, note)
		}
	}
	if rec, exists, err := store.fileRecordByPath("源.pptx"); err != nil || exists {
		t.Errorf("源行应删除，实际 rec=%+v exists=%v err=%v", rec, exists, err)
	}
}

// TestAssetsStore_MigrateV8PartialRecovers 半迁移库（target 列已加、版本仍为 7）
// 重开应跳过已存在列并补齐 source_removed，不再报 duplicate column name。
func TestAssetsStore_MigrateV8PartialRecovers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v7-partial.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("打开原始库: %v", err)
	}
	for v := 1; v <= 7; v++ {
		if _, err := db.Exec(assetsMigrations[v]); err != nil {
			t.Fatalf("手工执行 v%d 迁移: %v", v, err)
		}
	}
	// 模拟 v8 第一条 ALTER 成功后中断
	if _, err := db.Exec("ALTER TABLE split_records ADD COLUMN target TEXT NOT NULL DEFAULT 'cache'"); err != nil {
		t.Fatalf("模拟半迁移: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 7"); err != nil {
		t.Fatalf("钉版本: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("关闭原始库: %v", err)
	}

	store, err := openAssetsStore(dbPath)
	if err != nil {
		t.Fatalf("半迁移库应可升级打开: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, col := range []string{"target", "source_removed"} {
		var n int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('split_records') WHERE name = ?", col).Scan(&n); err != nil {
			t.Fatalf("查列 %s: %v", col, err)
		}
		if n != 1 {
			t.Errorf("列 %s 应存在且唯一，实际 %d", col, n)
		}
	}
	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != assetsSchemaVersion {
		t.Errorf("版本应钉到 v%d，实际 v%d（err=%v）", assetsSchemaVersion, version, err)
	}
}
