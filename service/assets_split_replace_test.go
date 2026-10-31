package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"workbench/model"
)

// ---------- 就地替换落点（B 方案 PR1） ----------
//
// 与 assets_split_test.go 同属拆分链路测试；本文件聚焦 replace 落点的删源、
// 标注迁移与回滚。fixture 走 buildAssetsPptx（禁本机 Office），回收站经注入点
// fake 隔离真实系统调用。

// fakeReplaceCore 就地替换核心 fake：按真实核心命名 <outDir>/<base>-NNN.pptx
// 落盘指定页数（写 payload 字节，解析由 parseFn fake 接管），返回结果。
// dropFile=true 时把最后一个返回路径替换为不存在文件，驱动「rename 失败 → 回滚」
// 分支；ctxErr=true 时落盘后按 ctx 状态返回取消错误，驱动取消回滚分支。
type fakeReplaceCore struct {
	pages    int
	payload  []byte
	dropFile bool
	ctxErr   bool

	mu    sync.Mutex
	calls int
}

func (f *fakeReplaceCore) split(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	base := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
	outDir := filepath.Join(filepath.Dir(srcPath), base)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return model.PptxSplitResult{}, err
	}
	files := make([]string, 0, f.pages)
	for i := 1; i <= f.pages; i++ {
		p := filepath.Join(outDir, fmt.Sprintf("%s-%03d.pptx", base, i))
		if err := os.WriteFile(p, f.payload, 0644); err != nil {
			return model.PptxSplitResult{}, err
		}
		files = append(files, p)
		if onProgress != nil {
			onProgress(i, f.pages)
		}
	}
	if f.ctxErr {
		if err := ctx.Err(); err != nil {
			return model.PptxSplitResult{}, fmt.Errorf("拆分已取消: %w", err)
		}
	}
	if f.dropFile && len(files) > 0 {
		files[len(files)-1] = filepath.Join(outDir, base+"-999.pptx")
	}
	return model.PptxSplitResult{OutputDir: outDir, Files: files, PageCount: f.pages}, nil
}

// oneSlideParse 单页解析 fake：产物内容不参与断言（本链路只关心索引行与页数），
// 统一按 1 页成功返回，避免依赖产物字节是否为合法 pptx。
func oneSlideParse() assetsParseFn {
	return func(srcPath, baseName string) ([]parsedSlide, error) {
		return []parsedSlide{{Num: 1, Title: baseName, Text: "单页", Notes: ""}}, nil
	}
}

// fakeRecycleBin 注入回收站 fake：记录调用并把文件移到 bin 目录（模拟真实
// 「源文件从源目录消失」），返回调用记录读取函数。
func fakeRecycleBin(t *testing.T) (move func(string) error, calls func() []string) {
	t.Helper()
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("建回收站目录: %v", err)
	}
	var mu sync.Mutex
	var recorded []string
	move = func(p string) error {
		mu.Lock()
		recorded = append(recorded, p)
		mu.Unlock()
		return os.Rename(p, filepath.Join(binDir, filepath.Base(p)))
	}
	calls = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), recorded...)
	}
	return move, calls
}

// TestAssetSplitReplace_Success 就地替换成功主链路：源消失（经注入回收站）、
// 产物按页序前缀命名落 <源目录>/<源名主干>/、每页成为独立素材（files 三行 ok
// 且各 1 页）、源行删除、split_records 标记 target=replace/source_removed=1。
func TestAssetSplitReplace_Success(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "替换库", dir)
	src := buildAssetsPptx(t, dir, "方案.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("page-bytes")}).split
	svc.parseFn = oneSlideParse()
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "replace")
	if err != nil {
		t.Fatalf("SplitAssetFile(replace): %v", err)
	}
	wantDir := filepath.Join(dir, "方案")
	if res.OutputDir != wantDir || res.PageCount != 3 || res.Skipped {
		t.Fatalf("替换结果不符：%+v", res)
	}
	if res.Target != model.AssetSplitTargetReplace || !res.SourceRemoved {
		t.Errorf("结果应回显 replace 落点且已删源：%+v", res)
	}
	for i, name := range []string{"001-方案.pptx", "002-方案.pptx", "003-方案.pptx"} {
		want := filepath.Join(wantDir, name)
		if res.Files[i] != want {
			t.Errorf("产物命名应为 {页序}-{源名主干}.pptx：%q, want %q", res.Files[i], want)
		}
		if info, statErr := os.Stat(want); statErr != nil || info.Size() == 0 {
			t.Errorf("产物应落盘且非空（%s）: %v", want, statErr)
		}
	}
	// 源文件已被移入回收站（fake 记录 + 磁盘消失）
	if _, statErr := os.Stat(src); !os.IsNotExist(statErr) {
		t.Errorf("replace 成功后源文件应消失，stat err=%v", statErr)
	}
	if got := recycleCalls(); len(got) != 1 || got[0] != src {
		t.Errorf("回收站应收到源文件绝对路径一次，实际 %v", got)
	}

	store := svc.openStoreForTest(t, lib.ID)
	// 三条 ok 行、各 1 页
	rows, err := store.db.Query(
		"SELECT f.path, f.status, COUNT(s.id) FROM files f LEFT JOIN slides s ON s.file_id = f.id GROUP BY f.id ORDER BY f.path")
	if err != nil {
		t.Fatalf("查询产物索引: %v", err)
	}
	type fileRow struct {
		path   string
		status string
		pages  int
	}
	var got []fileRow
	for rows.Next() {
		var r fileRow
		if err := rows.Scan(&r.path, &r.status, &r.pages); err != nil {
			_ = rows.Close()
			t.Fatalf("扫描产物索引: %v", err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatalf("遍历产物索引: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("关闭产物索引结果集: %v", err)
	}
	wantPaths := []string{"方案/001-方案.pptx", "方案/002-方案.pptx", "方案/003-方案.pptx"}
	if len(got) != len(wantPaths) {
		t.Fatalf("应有 %d 条产物索引行，实际 %d：%+v", len(wantPaths), len(got), got)
	}
	for i, r := range got {
		if r.path != wantPaths[i] || r.status != model.AssetFileStatusOK || r.pages != 1 {
			t.Errorf("第 %d 条产物索引不符（应 ok 且 1 页）：%+v", i+1, r)
		}
	}
	// 源行已删除
	if rec, exists, err := store.fileRecordByPath("方案.pptx"); err != nil || exists {
		t.Errorf("replace 成功后源索引行应删除，实际 rec=%+v exists=%v err=%v", rec, exists, err)
	}
	// 拆分记录：target=replace、source_removed=1
	record, err := store.getSplitRecord("方案.pptx")
	if err != nil || record == nil {
		t.Fatalf("拆分记录应存在：%v", err)
	}
	if record.target != model.AssetSplitTargetReplace || !record.sourceRemoved ||
		record.pageCount != 3 || record.outputDir != wantDir {
		t.Errorf("拆分记录不符：%+v", record)
	}
}

// TestAssetSplitReplace_MigratesAnnotations 标注迁移：源文件级星级/备注/标签
// 复制到每个产物；源第 2 页的标签/备注/星级迁到第 2 个产物第 1 页；产物文件
// FTS 行含迁移后的备注（备注参与关键词检索）。
func TestAssetSplitReplace_MigratesAnnotations(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "标注迁移库", dir)
	buildAssetsPptx(t, dir, "笔记.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)

	store := svc.openStoreForTest(t, lib.ID)
	var srcID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "笔记.pptx").Scan(&srcID); err != nil {
		t.Fatalf("读源文件 ID: %v", err)
	}
	inserted, err := store.db.Exec("INSERT INTO tags(name, color) VALUES('重点', '#FF0000')")
	if err != nil {
		t.Fatalf("插标签: %v", err)
	}
	tagID, err := inserted.LastInsertId()
	if err != nil {
		t.Fatalf("取标签 ID: %v", err)
	}
	for _, stmt := range []struct {
		sql string
		arg []any
	}{
		{"UPDATE files SET rating = 4, note = '源文件备注' WHERE id = ?", []any{srcID}},
		{"INSERT INTO file_tags(file_id, tag_id) VALUES(?, ?)", []any{srcID, tagID}},
		{"INSERT INTO slide_meta(file_id, slide_num, note, rating) VALUES(?, 2, '第二页备注', 5)", []any{srcID}},
		{"INSERT INTO slide_tags(file_id, slide_num, tag_id) VALUES(?, 2, ?)", []any{srcID, tagID}},
	} {
		if _, err := store.db.Exec(stmt.sql, stmt.arg...); err != nil {
			t.Fatalf("预置源标注（%s）: %v", stmt.sql, err)
		}
	}

	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("page-bytes")}).split
	svc.parseFn = oneSlideParse()
	move, _ := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "笔记.pptx", "replace"); err != nil {
		t.Fatalf("SplitAssetFile(replace): %v", err)
	}

	// 每个产物：文件级星级/备注/标签与源一致
	rows, err := store.db.Query("SELECT id, path, rating, note FROM files WHERE path LIKE '笔记/%' ORDER BY path")
	if err != nil {
		t.Fatalf("查询产物行: %v", err)
	}
	type prodRow struct {
		id     int64
		path   string
		rating int
		note   string
	}
	var prods []prodRow
	for rows.Next() {
		var r prodRow
		if err := rows.Scan(&r.id, &r.path, &r.rating, &r.note); err != nil {
			_ = rows.Close()
			t.Fatalf("扫描产物行: %v", err)
		}
		prods = append(prods, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatalf("遍历产物行: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("关闭产物结果集: %v", err)
	}
	if len(prods) != 3 {
		t.Fatalf("应有 3 个产物，实际 %d", len(prods))
	}
	wantNoteTokens := splitFtsText("源文件备注")
	for _, p := range prods {
		if p.rating != 4 || p.note != "源文件备注" {
			t.Errorf("产物 %s 应继承源文件星级/备注：%+v", p.path, p)
		}
		var tagCount int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM file_tags WHERE file_id = ?", p.id).Scan(&tagCount); err != nil {
			t.Fatalf("统计产物文件标签: %v", err)
		}
		if tagCount != 1 {
			t.Errorf("产物 %s 应继承源文件标签 1 个，实际 %d", p.path, tagCount)
		}
		var ftsNote string
		if err := store.db.QueryRow("SELECT file_note FROM search_fts WHERE rowid = ?", p.id).Scan(&ftsNote); err != nil {
			t.Fatalf("读产物 FTS 行: %v", err)
		}
		if ftsNote != wantNoteTokens {
			t.Errorf("产物 %s FTS 备注应与迁移后备注同源切分：%q, want %q", p.path, ftsNote, wantNoteTokens)
		}
	}

	// 第 2 页产物的第 1 页带源第 2 页标注；其余产物页级标注为空
	var page2ID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "笔记/002-笔记.pptx").Scan(&page2ID); err != nil {
		t.Fatalf("读第 2 页产物 ID: %v", err)
	}
	var note string
	var rating int
	if err := store.db.QueryRow("SELECT note, rating FROM slide_meta WHERE file_id = ? AND slide_num = 1", page2ID).
		Scan(&note, &rating); err != nil {
		t.Fatalf("第 2 页产物应有迁移后的页级标注：%v", err)
	}
	if note != "第二页备注" || rating != 5 {
		t.Errorf("第 2 页产物页级标注不符：note=%q rating=%d", note, rating)
	}
	var pageTagCount int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM slide_tags WHERE file_id = ? AND slide_num = 1", page2ID).Scan(&pageTagCount); err != nil {
		t.Fatalf("统计第 2 页产物页标签: %v", err)
	}
	if pageTagCount != 1 {
		t.Errorf("第 2 页产物应带源第 2 页标签 1 个，实际 %d", pageTagCount)
	}
	var otherMeta int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM slide_meta WHERE file_id <> ?", page2ID).Scan(&otherMeta); err != nil {
		t.Fatalf("统计其余产物页级标注: %v", err)
	}
	if otherMeta != 0 {
		t.Errorf("源第 1/3 页无标注，其余产物不应有页级标注，实际 %d 行", otherMeta)
	}
}

// TestAssetSplitReplace_RenameFailureRollsBack 产物 rename 失败回滚：产物全部
// 删除（含已改名与未改名两种形态）、源文件与源行/源标注保留、outcome 标记
// rolledBack（经批量明细断言）。
func TestAssetSplitReplace_RenameFailureRollsBack(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "回滚库", dir)
	src := buildAssetsPptx(t, dir, "失败.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET rating = 3 WHERE path = ?", "失败.pptx"); err != nil {
		t.Fatalf("预置源星级: %v", err)
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("page-bytes"), dropFile: true}).split
	svc.parseFn = oneSlideParse()
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	batch, err := svc.SplitAssetLibrary(context.Background(), lib.ID, "replace")
	if err != nil {
		t.Fatalf("批量不应整体失败（单文件隔离）：%v", err)
	}
	if batch.Failed != 1 || batch.Split != 0 || len(batch.Items) != 1 {
		t.Fatalf("批量汇总应为单文件失败：%+v", batch)
	}
	item := batch.Items[0]
	if item.Status != model.AssetSplitStatusFailed || !item.RolledBack ||
		item.SourceRemoved || item.Code != "" || !strings.Contains(item.Message, "重命名失败") {
		t.Errorf("失败明细应标记已回滚且源未删：%+v", item)
	}
	// 产物全部删除（目录不存在或为空）
	outDir := filepath.Join(dir, "失败")
	if entries, err := os.ReadDir(outDir); err == nil && len(entries) > 0 {
		t.Errorf("回滚后产物目录应为空，实际 %d 项", len(entries))
	}
	// 源文件与源行/源标注保留
	if _, err := os.Stat(src); err != nil {
		t.Errorf("失败应保留源文件：%v", err)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("失败路径不得调用回收站，实际 %v", got)
	}
	if _, exists, err := store.fileRecordByPath("失败.pptx"); err != nil || !exists {
		t.Errorf("失败应保留源索引行，exists=%v err=%v", exists, err)
	}
	var rating int
	if err := store.db.QueryRow("SELECT rating FROM files WHERE path = ?", "失败.pptx").Scan(&rating); err != nil || rating != 3 {
		t.Errorf("失败应保留源标注，rating=%d err=%v", rating, err)
	}
	var prodRows int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM files WHERE path LIKE '失败/%'").Scan(&prodRows); err != nil || prodRows != 0 {
		t.Errorf("回滚后不应残留产物索引行，实际 %d 行（err=%v）", prodRows, err)
	}
	if record, _ := store.getSplitRecord("失败.pptx"); record != nil {
		t.Errorf("失败不应写拆分记录：%+v", record)
	}
}

// TestAssetSplitReplace_CancelRollsBack 取消回滚：核心按 ctx 返回取消错误 →
// 已产出页全部删除、源与源行保留、返回错误 errors.Is(context.Canceled)。
// 直接驱动内部链路（入口层会把取消映射为 E_ASSETS_CANCELLED AppError，见
// TestAssetSplit_CancelPreCancelled；此处验证回滚与原始错误链）。
func TestAssetSplitReplace_CancelRollsBack(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "取消替换库", dir)
	src := buildAssetsPptx(t, dir, "取消.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	hash, _, err := store.fileHashStatusByPath("取消.pptx")
	if err != nil {
		t.Fatalf("读源哈希: %v", err)
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("page-bytes"), ctxErr: true}).split

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, err := svc.splitOneFileWithHash(ctx, store, lib, "取消.pptx", src, hash, "replace", 1, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消错误应保留 context.Canceled 链，实际：%v", err)
	}
	if !outcome.rolledBack {
		t.Errorf("取消应标记产物已回滚：%+v", outcome)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("取消应保留源文件：%v", err)
	}
	if _, exists, err := store.fileRecordByPath("取消.pptx"); err != nil || !exists {
		t.Errorf("取消应保留源索引行，exists=%v err=%v", exists, err)
	}
	var prodRows int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM files WHERE path LIKE '取消/%'").Scan(&prodRows); err != nil || prodRows != 0 {
		t.Errorf("取消后不应残留产物索引行，实际 %d 行（err=%v）", prodRows, err)
	}
}

// TestAssetSplitReplace_CancelAfterCoreRollsBack 取消落在「核心已返回」窗口：
// 核心忽略 ctx 成功返回后，链路在改名/建索引前与删源前的检查点必须命中取消，
// 回滚产物且保留源与源行（防「取消仍把源移入回收站」破坏 PRD 验收）。
func TestAssetSplitReplace_CancelAfterCoreRollsBack(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "取消窗口库", dir)
	src := buildAssetsPptx(t, dir, "窗口.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	hash, _, err := store.fileHashStatusByPath("窗口.pptx")
	if err != nil {
		t.Fatalf("读源哈希: %v", err)
	}
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	// 核心忽略 ctx 直接成功（模拟取消信号在核心返回后才被链路观察到）
	svc.splitCoreFn = func(_ context.Context, srcPath string, _ func(done, total int)) (model.PptxSplitResult, error) {
		base := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
		outDir := filepath.Join(filepath.Dir(srcPath), base)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return model.PptxSplitResult{}, err
		}
		files := make([]string, 0, 2)
		for i := 1; i <= 2; i++ {
			p := filepath.Join(outDir, fmt.Sprintf("%s-%03d.pptx", base, i))
			if err := os.WriteFile(p, []byte("page-bytes"), 0644); err != nil {
				return model.PptxSplitResult{}, err
			}
			files = append(files, p)
		}
		return model.PptxSplitResult{OutputDir: outDir, Files: files, PageCount: 2}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome, err := svc.splitOneFileWithHash(ctx, store, lib, "窗口.pptx", src, hash, "replace", 1, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("核心后取消应保留 context.Canceled 链，实际：%v", err)
	}
	if !outcome.rolledBack {
		t.Errorf("取消应标记产物已回滚：%+v", outcome)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("取消不得把源移入回收站，实际 %v", got)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("取消应保留源文件：%v", statErr)
	}
	if _, exists, err := store.fileRecordByPath("窗口.pptx"); err != nil || !exists {
		t.Errorf("取消应保留源索引行，exists=%v err=%v", exists, err)
	}
	if entries, readErr := os.ReadDir(filepath.Join(dir, "窗口")); readErr == nil && len(entries) > 0 {
		t.Errorf("取消后产物目录应为空，实际 %d 项", len(entries))
	}
	var prodRows int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM files WHERE path LIKE '窗口/%'").Scan(&prodRows); err != nil || prodRows != 0 {
		t.Errorf("取消后不应残留产物索引行，实际 %d 行（err=%v）", prodRows, err)
	}
}

// TestAssetSplitReplace_RecycleFailureKeepsArtifacts 回收站失败口径（PRD D4）：
// 全页成功但源无法移入回收站 → 返回 E_ASSETS_SPLIT_SOURCE_DELETE_FAILED，源文件
// 与源行保留，产物、索引行与标注副本全部保留（可重试）。
func TestAssetSplitReplace_RecycleFailureKeepsArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "删源失败库", dir)
	src := buildAssetsPptx(t, dir, "占用.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET rating = 5, note = '保留备注' WHERE path = ?", "占用.pptx"); err != nil {
		t.Fatalf("预置源标注: %v", err)
	}
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("page-bytes")}).split
	svc.parseFn = oneSlideParse()
	svc.moveToRecycleBinFn = func(string) error { return errors.New("文件被占用") }

	_, err := svc.SplitAssetFile(context.Background(), lib.ID, "占用.pptx", "replace")
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsSplitSourceDeleteFailed {
		t.Fatalf("删源失败应返回 E_ASSETS_SPLIT_SOURCE_DELETE_FAILED，实际：%v", err)
	}
	if !strings.Contains(appErr.Message, "产物已保留") {
		t.Errorf("错误消息应说明产物已保留，实际：%s", appErr.Message)
	}
	// 源文件与源行保留
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("删源失败应保留源文件：%v", statErr)
	}
	if _, exists, err := store.fileRecordByPath("占用.pptx"); err != nil || !exists {
		t.Errorf("删源失败应保留源索引行，exists=%v err=%v", exists, err)
	}
	// 产物、索引行与标注副本保留
	outDir := filepath.Join(dir, "占用")
	for _, name := range []string{"001-占用.pptx", "002-占用.pptx"} {
		if _, statErr := os.Stat(filepath.Join(outDir, name)); statErr != nil {
			t.Errorf("删源失败应保留产物 %s：%v", name, statErr)
		}
	}
	rows, err := store.db.Query("SELECT id, rating, note FROM files WHERE path LIKE '占用/%' ORDER BY path")
	if err != nil {
		t.Fatalf("查询产物索引: %v", err)
	}
	count := 0
	for rows.Next() {
		var id int64
		var rating int
		var note string
		if err := rows.Scan(&id, &rating, &note); err != nil {
			_ = rows.Close()
			t.Fatalf("扫描产物索引: %v", err)
		}
		count++
		if rating != 5 || note != "保留备注" {
			t.Errorf("产物应保留迁移后的标注：rating=%d note=%q", rating, note)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatalf("遍历产物索引: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("关闭产物结果集: %v", err)
	}
	if count != 2 {
		t.Errorf("删源失败应保留 2 条产物索引行，实际 %d", count)
	}
	if record, _ := store.getSplitRecord("占用.pptx"); record != nil {
		t.Errorf("删源失败不应写成功拆分记录（重拆走完整判定）：%+v", record)
	}
}

// TestAssetSplitReplace_NumericBaseNameAvoidsDerivativeExclusion 数值主干命名
// 规避：源名主干为「≥3 位纯数字」时追加 -p，避免「父目录名-页序」形态命中
// isPptxSplitDerivative 被扫描静默排除（页序 <1000 补零三位、≥1000 不补零，
// 故 3 位与 4 位以上数字主干都存在与页序段等长的碰撞面）。
func TestAssetSplitReplace_NumericBaseNameAvoidsDerivativeExclusion(t *testing.T) {
	// 碰撞面单点断言：页 123 × 主干 123
	if got := replaceArtifactName(123, "123"); got != "123-123-p.pptx" {
		t.Errorf("3 位纯数字主干产物应加 -p：%q", got)
	}
	if isPptxSplitDerivative("123/123-123-p.pptx") {
		t.Error("加 -p 后不应命中拆分派生排除规则")
	}
	if !isPptxSplitDerivative("123/123-123.pptx") {
		t.Error("对照：未加 -p 的「父目录名-页序」形态应命中排除规则（证明碰撞真实存在）")
	}
	// 碰撞面单点断言：页 1234 × 主干 1234（≥1000 页不补零，页序段 4 位）
	if got := replaceArtifactName(1234, "1234"); got != "1234-1234-p.pptx" {
		t.Errorf("4 位纯数字主干产物应加 -p：%q", got)
	}
	if !isPptxSplitDerivative("1234/1234-1234.pptx") {
		t.Error("对照：4 位纯数字主干未加 -p 时同样命中排除规则（证明碰撞面覆盖 ≥3 位）")
	}
	if got := replaceArtifactName(12, "12"); got != "012-12.pptx" {
		t.Errorf("1~2 位数字主干与 3 位页序段不可能等长，无需 -p：%q", got)
	}

	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "数值主干库", dir)
	buildAssetsPptx(t, dir, "123.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("page-bytes")}).split
	svc.parseFn = oneSlideParse()
	move, _ := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "123.pptx", "replace")
	if err != nil {
		t.Fatalf("SplitAssetFile(replace): %v", err)
	}
	if len(res.Files) != 3 {
		t.Fatalf("应产出 3 页：%+v", res)
	}
	for i, p := range res.Files {
		name := filepath.Base(p)
		want := fmt.Sprintf("%03d-123-p.pptx", i+1)
		if name != want {
			t.Errorf("第 %d 页产物命名应为 %s，实际 %s", i+1, want, name)
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			t.Fatalf("计算产物相对路径: %v", err)
		}
		if isPptxSplitDerivative(filepath.ToSlash(rel)) {
			t.Errorf("产物 %s 不得命中拆分派生排除规则", name)
		}
	}
}

// TestAssetsStore_MigrateV8SplitRecordDefaults v7 → v8 迁移：存量 split_records
// 行补 target 列默认 'cache'、source_removed 默认 0（旧落点从不删源，语义正确）。
func TestAssetsStore_MigrateV8SplitRecordDefaults(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v7.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	if err != nil {
		t.Fatalf("打开原始库: %v", err)
	}
	for v := 1; v <= 7; v++ {
		if _, err := db.Exec(assetsMigrations[v]); err != nil {
			t.Fatalf("手工执行 v%d 迁移: %v", v, err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 7"); err != nil {
		t.Fatalf("钉版本: %v", err)
	}
	if _, err := db.Exec(
		"INSERT INTO split_records(source_rel_path, source_hash, output_dir, page_count, split_at) VALUES('存量.pptx', 'h', 'd', 2, '2026-01-01 00:00:00')",
	); err != nil {
		t.Fatalf("插入存量拆分记录: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("关闭原始库: %v", err)
	}

	store, err := openAssetsStore(dbPath)
	if err != nil {
		t.Fatalf("升级打开: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	rec, err := store.getSplitRecord("存量.pptx")
	if err != nil || rec == nil {
		t.Fatalf("存量记录应可读回：%v", err)
	}
	if rec.target != model.AssetSplitTargetCache || rec.sourceRemoved {
		t.Errorf("存量记录应默认 target=cache / source_removed=0：%+v", rec)
	}
	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != assetsSchemaVersion {
		t.Errorf("版本应钉到 v%d，实际 v%d（err=%v）", assetsSchemaVersion, version, err)
	}
}
