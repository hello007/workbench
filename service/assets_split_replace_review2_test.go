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

// ---------- 就地替换审核修复第二轮（10-31-b-review-fixes-2） ----------
//
// 覆盖：旧产物延迟清理与备份还原、目标同名拒绝、源哈希重算、空主干拒绝、
// 上次 replace 产物剔除/拒拆、目录守卫已解除（记录随事务迁移）、删源前取消标志复查。

// assertNoReplaceBackup 断言目录内无回滚备份残留（.replace-bak 形态）。
func assertNoReplaceBackup(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), replaceArtifactBackupSuffix) {
			t.Errorf("不应残留历史产物备份：%s", e.Name())
		}
	}
}

// resplitFixture 已成功 replace 过一次、且源文件已还原并重扫的夹具：返回源绝对
// 路径与原始字节，供「改源后再拆」类用例复用。
func resplitFixture(t *testing.T, svc *AssetLibraryService, lib model.AssetLibrary, dir, name string, pages int) (string, []byte) {
	t.Helper()
	specs := make([]assetsSlideSpec, 0, pages)
	for i := 1; i <= pages; i++ {
		specs = append(specs, assetsSlideSpec{titlePh: "页", body: []string{"内容"}})
	}
	src := buildAssetsPptx(t, dir, name, specs)
	original, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("读取源文件: %v", err)
	}
	mustScanLibrary(t, svc, lib.ID)
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: pages, payload: []byte("old-bytes")}).split
	move, _ := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, name, "replace"); err != nil {
		t.Fatalf("首次替换: %v", err)
	}
	// 模拟用户从回收站还原源文件并重新扫描（源行缺失，重扫后重新入库）
	if err := os.WriteFile(src, original, 0644); err != nil {
		t.Fatalf("还原源文件: %v", err)
	}
	mustScanLibrary(t, svc, lib.ID)
	return src, original
}

// TestAssetSplitReplace_RollbackRestoresOldArtifacts 失败回滚时旧产物原样保留：
// 重拆覆盖过程中第 2 页 rename 失败，被覆盖的第 1 页必须从备份还原（内容仍为
// 上一次拆分的字节），旧索引行仍指向可用文件，源文件不删（审核修复核心面）。
func TestAssetSplitReplace_RollbackRestoresOldArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "旧产物保留库", dir)
	resplitFixture(t, svc, lib, dir, "方案.pptx", 2)
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	// 第二次拆分：核心落盘后把最后一页路径替换为不存在文件，驱动 rename 失败
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes"), dropFile: true}).split

	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "replace"); err == nil {
		t.Fatal("rename 失败应返回错误")
	}
	outDir := filepath.Join(dir, "方案")
	for _, name := range []string{"001-方案.pptx", "002-方案.pptx"} {
		p := filepath.Join(outDir, name)
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("旧产物 %s 应保留：%v", name, err)
		}
		if string(got) != "old-bytes" {
			t.Errorf("旧产物 %s 应原样保留（old-bytes），实际 %q", name, got)
		}
	}
	assertNoReplaceBackup(t, outDir)
	store := svc.openStoreForTest(t, lib.ID)
	var prodRows int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM files WHERE path LIKE '方案/%'").Scan(&prodRows); err != nil || prodRows != 2 {
		t.Errorf("回滚后旧产物索引行应完整保留，实际 %d 行（err=%v）", prodRows, err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "方案.pptx")); statErr != nil {
		t.Errorf("回滚应保留源文件：%v", statErr)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("回滚路径不得移入回收站，实际 %v", got)
	}
}

// TestAssetSplitReplace_ResplitDropsStalePages 重拆成功且页数减少：新页之外的
// 旧页文件与索引行在删源前收敛（延迟清理仍在成功路径生效），无备份残留。
func TestAssetSplitReplace_ResplitDropsStalePages(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "旧页收敛库", dir)
	src, _ := resplitFixture(t, svc, lib, dir, "缩减.pptx", 3)

	// 源改为 2 页并同步库存哈希（确定性驱动重拆，不依赖 mtime 时序）
	buildAssetsPptx(t, dir, "缩减.pptx", []assetsSlideSpec{
		{titlePh: "新一", body: []string{"内容"}},
		{titlePh: "新二", body: []string{"内容"}},
	})
	newHash, err := hashFileSHA256(src)
	if err != nil {
		t.Fatalf("算新哈希: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if _, err := store.db.Exec("UPDATE files SET hash = ? WHERE path = ?", newHash, "缩减.pptx"); err != nil {
		t.Fatalf("更新库存哈希: %v", err)
	}
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("new-bytes")}).split

	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "缩减.pptx", "replace")
	if err != nil {
		t.Fatalf("重拆: %v", err)
	}
	if res.Skipped || res.PageCount != 2 || !res.SourceRemoved {
		t.Fatalf("重拆应成功并删源：%+v", res)
	}
	outDir := filepath.Join(dir, "缩减")
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("读产物目录: %v", err)
	}
	got := make(map[string]bool, len(entries))
	for _, e := range entries {
		got[e.Name()] = true
	}
	want := map[string]bool{"001-缩减.pptx": true, "002-缩减.pptx": true}
	for name := range got {
		if !want[name] {
			t.Errorf("页数减少后旧页文件应删除：%q", name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("新产物缺失：%q", name)
		}
	}
	assertNoReplaceBackup(t, outDir)
	var rows int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM files WHERE path LIKE '缩减/%'").Scan(&rows); err != nil || rows != 2 {
		t.Errorf("旧页索引行应收敛为 2 条，实际 %d（err=%v）", rows, err)
	}
	if _, exists, err := store.fileRecordByPath("缩减/003-缩减.pptx"); err != nil || exists {
		t.Errorf("越界旧页索引行应删除，实际 exists=%v err=%v", exists, err)
	}
	if got := recycleCalls(); len(got) != 1 {
		t.Errorf("重拆成功应移入回收站一次，实际 %v", got)
	}
}

// TestAssetSplitReplace_StrayArtifactBlocksSplit 无本目录 replace 记录但目录已
// 有同形文件：直接可见失败，不进核心、不动用户文件（延迟清理的前置校验）。
func TestAssetSplitReplace_StrayArtifactBlocksSplit(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "同形文件库", dir)
	buildAssetsPptx(t, dir, "方案.pptx", []assetsSlideSpec{
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
	coreCalled := false
	svc.splitCoreFn = func(context.Context, string, func(done, total int)) (model.PptxSplitResult, error) {
		coreCalled = true
		return model.PptxSplitResult{}, errors.New("不应调用核心")
	}
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	_, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "replace")
	if err == nil || !strings.Contains(err.Error(), "已存在与拆分产物同名的文件") {
		t.Fatalf("应拒绝并说明同形文件冲突，实际：%v", err)
	}
	if coreCalled {
		t.Error("同形文件冲突不得进入拆分核心")
	}
	if got, readErr := os.ReadFile(userFile); readErr != nil || string(got) != "user-created" {
		t.Errorf("用户文件应原样保留（err=%v, content=%q）", readErr, got)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "方案.pptx")); statErr != nil {
		t.Errorf("源文件应保留：%v", statErr)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("不得移入回收站，实际 %v", got)
	}
}

// TestAssetSplitReplace_ExistingTargetDirRejectedWithRollback rename 前目标名已
// 存在且不属于历史产物（目录形态，快照按文件收集故不在 oldArtifacts）时中止
// 并回滚本次新增，既有目标保持不动。
func TestAssetSplitReplace_ExistingTargetDirRejectedWithRollback(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "目标占用库", dir)
	src := buildAssetsPptx(t, dir, "占用.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	outDir := filepath.Join(dir, "占用")
	blocker := filepath.Join(outDir, "001-占用.pptx")
	if err := os.MkdirAll(blocker, 0755); err != nil {
		t.Fatalf("占位目录: %v", err)
	}
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 1, payload: []byte("page-bytes")}).split
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	_, err := svc.SplitAssetFile(context.Background(), lib.ID, "占用.pptx", "replace")
	if err == nil || !strings.Contains(err.Error(), "已存在且不属于历史拆分产物") {
		t.Fatalf("目标被非历史产物占用应中止，实际：%v", err)
	}
	if info, statErr := os.Stat(blocker); statErr != nil || !info.IsDir() {
		t.Errorf("既有目标目录应保持不动，err=%v", statErr)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("中止应保留源文件：%v", statErr)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("不得移入回收站，实际 %v", got)
	}
	if entries, readErr := os.ReadDir(outDir); readErr == nil && len(entries) != 1 {
		t.Errorf("回滚后仅应剩既有目标目录，实际 %d 项", len(entries))
	}
}

// TestAssetSplitReplace_SourceHashMismatchRequiresRescan 源内容与扫描快照不一致
// （改文件未重扫）：拒绝拆分并引导重扫，不产生产物、不动源文件。
func TestAssetSplitReplace_SourceHashMismatchRequiresRescan(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "哈希不一致库", dir)
	src := buildAssetsPptx(t, dir, "变更.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	// 改内容但不重扫：files.hash 仍是旧快照
	if err := os.WriteFile(src, []byte("changed-without-rescan"), 0644); err != nil {
		t.Fatalf("改源文件: %v", err)
	}
	coreCalled := false
	svc.splitCoreFn = func(context.Context, string, func(done, total int)) (model.PptxSplitResult, error) {
		coreCalled = true
		return model.PptxSplitResult{}, nil
	}
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	_, err := svc.SplitAssetFile(context.Background(), lib.ID, "变更.pptx", "replace")
	if err == nil || !strings.Contains(err.Error(), "请先重新扫描素材库后再拆分") {
		t.Fatalf("哈希不一致应要求重扫，实际：%v", err)
	}
	if coreCalled {
		t.Error("哈希不一致不得进入拆分核心")
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("源文件应保留：%v", statErr)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("不得移入回收站，实际 %v", got)
	}
}

// TestAssetSplitReplace_EmptyBaseNameRejected 文件名恰为 .pptx（主干为空）时
// 落点会退化为源目录，必须明确拒绝。
func TestAssetSplitReplace_EmptyBaseNameRejected(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "空主干库", dir)
	buildAssetsPptx(t, dir, ".pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	_, err := svc.SplitAssetFile(context.Background(), lib.ID, ".pptx", "replace")
	if err == nil || !strings.Contains(err.Error(), "源文件名主干为空") {
		t.Fatalf("空主干应明确拒绝，实际：%v", err)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("不得移入回收站，实际 %v", got)
	}
}

// TestAssetSplitReplace_BatchSkipsReplaceArtifacts 批量清单剔除上次 replace 生成的
// 单页产物（任意落点）：二次批量不产生「页的页」；单文件入口对产物二次 replace
// 拆分同样拒绝。
func TestAssetSplitReplace_BatchSkipsReplaceArtifacts(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "产物剔除库", dir)
	buildAssetsPptx(t, dir, "方案.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
		{titlePh: "三", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 3, payload: []byte("page-bytes")}).split
	move, _ := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案.pptx", "replace"); err != nil {
		t.Fatalf("首次替换: %v", err)
	}
	// 批量：全部 ok 文件只剩 3 个单页产物，应被剔除
	svc.splitCoreFn = func(context.Context, string, func(done, total int)) (model.PptxSplitResult, error) {
		t.Error("上次 replace 产物不得再次批量拆分")
		return model.PptxSplitResult{}, errors.New("不应调用核心")
	}
	for _, target := range []string{"replace", "cache"} {
		batch, err := svc.SplitAssetLibrary(context.Background(), lib.ID, target)
		if err != nil {
			t.Fatalf("批量（%s）不应失败: %v", target, err)
		}
		if batch.Total != 0 || batch.Split != 0 || len(batch.Items) != 0 {
			t.Errorf("批量（%s）应剔除全部 replace 产物，实际 %+v", target, batch)
		}
	}
	// 单文件入口兜底：产物二次 replace 拆分拒绝
	_, err := svc.SplitAssetFile(context.Background(), lib.ID, "方案/001-方案.pptx", "replace")
	if err == nil || !strings.Contains(err.Error(), "单页拆分产物") {
		t.Fatalf("产物二次 replace 拆分应拒绝，实际：%v", err)
	}
}

// TestSplitRecordMigrate_RemovesDirectoryGuard 目录守卫已解除（B 方案 PR2）：
// source_removed=0 的记录同样不再阻断目录重命名，且随同一事务迁移到新路径；
// target=cache 记录的 output_dir 保持原绝对路径（derived 物理目录不随迁移）。
func TestSplitRecordMigrate_RemovesDirectoryGuard(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "目录守卫库", dir)
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	buildAssetsPptx(t, filepath.Join(dir, "media"), "a.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	// 源已删的 replace 记录：不阻断且 output_dir 按库根重算
	if err := store.upsertSplitRecord("media/a.pptx", "hash", filepath.Join(dir, "media", "a"),
		2, "2026-10-20 10:00:00", model.AssetSplitTargetReplace, "",
		true); err != nil {
		t.Fatalf("写拆分记录: %v", err)
	}
	if _, err := svc.RenameAssetFolder(lib.ID, "media", "media2"); err != nil {
		t.Fatalf("source_removed=1 的记录不应阻断目录重命名：%v", err)
	}
	rec, err := store.getSplitRecord("media2/a.pptx")
	if err != nil || rec == nil {
		t.Fatalf("拆分记录应随重命名迁移到新路径：%v", err)
	}
	if want := filepath.Join(dir, "media2", "a"); rec.outputDir != want {
		t.Errorf("replace 记录 output_dir 应按库根重算为 %q，实际 %q", want, rec.outputDir)
	}
	// 源仍存在的 cache 记录（source_removed=0）同样不阻断，output_dir 保持原值
	if err := store.upsertSplitRecord("media2/a.pptx", "hash", filepath.Join(dir, "media2", "a"),
		2, "2026-10-20 10:00:00", model.AssetSplitTargetCache, "",
		false); err != nil {
		t.Fatalf("写拆分记录: %v", err)
	}
	if _, err := svc.RenameAssetFolder(lib.ID, "media2", "media3"); err != nil {
		t.Fatalf("source_removed=0 的记录不应再阻断目录重命名：%v", err)
	}
	rec, err = store.getSplitRecord("media3/a.pptx")
	if err != nil || rec == nil {
		t.Fatalf("cache 拆分记录应随重命名迁移到新路径：%v", err)
	}
	if want := filepath.Join(dir, "media2", "a"); rec.outputDir != want {
		t.Errorf("cache 记录 output_dir 应保持原绝对路径 %q，实际 %q", want, rec.outputDir)
	}
}

// TestAssetSplitReplace_SourceDeleteFailureRecordsOwnershipAndRetries 删源失败
// （8b）补写 target=replace/source_removed=0 记录：产物保有归属，重试不被「同形
// 文件」拒绝、批量不会把产物当新源二次拆分；占用解除后重试完整重拆并成功删源。
func TestAssetSplitReplace_SourceDeleteFailureRecordsOwnershipAndRetries(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "删源失败库", dir)
	src := buildAssetsPptx(t, dir, "删源.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	svc.parseFn = oneSlideParse()
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("page-bytes")}).split
	// 第一次：源被占用，移入回收站失败
	svc.moveToRecycleBinFn = func(string) error { return errors.New("占用中") }
	_, err := svc.SplitAssetFile(context.Background(), lib.ID, "删源.pptx", "replace")
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsSplitSourceDeleteFailed {
		t.Fatalf("删源失败应返回 E_ASSETS_SPLIT_SOURCE_DELETE_FAILED，实际：%v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	rec, recErr := store.getSplitRecord("删源.pptx")
	if recErr != nil || rec == nil || rec.target != model.AssetSplitTargetReplace || rec.sourceRemoved {
		t.Fatalf("删源失败应补写 target=replace/source_removed=0 记录，实际 rec=%+v err=%v", rec, recErr)
	}
	if rec.artifactStem != replaceArtifactBase("删源") {
		t.Errorf("删源失败补写记录同样须持久化产物主干 %q，实际 %q",
			replaceArtifactBase("删源"), rec.artifactStem)
	}
	for _, name := range []string{"001-删源.pptx", "002-删源.pptx"} {
		if _, statErr := os.Stat(filepath.Join(dir, "删源", name)); statErr != nil {
			t.Fatalf("删源失败产物应保留：%s（%v）", name, statErr)
		}
	}

	// 第二次：占用解除，重试须走完整重拆（记录归属命中，旧产物备份让位）
	move, calls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move
	res, err := svc.SplitAssetFile(context.Background(), lib.ID, "删源.pptx", "replace")
	if err != nil || res.Skipped || !res.SourceRemoved || res.PageCount != 2 {
		t.Fatalf("删源失败后重试应完整重拆并成功删源，实际 res=%+v err=%v", res, err)
	}
	if got := calls(); len(got) != 1 || got[0] != src {
		t.Errorf("重试应把源移入回收站一次，实际 %v", got)
	}
	rec, recErr = store.getSplitRecord("删源.pptx")
	if recErr != nil || rec == nil || !rec.sourceRemoved {
		t.Errorf("重试成功后记录应更新 source_removed=1，实际 rec=%+v err=%v", rec, recErr)
	}
	assertNoReplaceBackup(t, filepath.Join(dir, "删源"))
}

// TestSplitCancelledAfterCheckpoint_RegistryWindow 删源前取消复查：CancelAssetSplit
// 先摘除注册条目再调用 cancel，复查须在 ctx.Err() 尚为 nil 时由「条目已消失」
// 命中；入口链路命中后回滚产物且源保留。
func TestSplitCancelledAfterCheckpoint_RegistryWindow(t *testing.T) {
	svc := newTestAssetService(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const libID = "registry-window"
	entry := svc.registerSplitCancel(libID, cancel)
	if svc.splitCancelledAfterCheckpoint(ctx, libID, entry) {
		t.Fatal("在途且未取消时复查应为 false")
	}
	// 模拟 CancelAssetSplit 的第一步：摘除条目（尚未调用 cancel）
	svc.opMu.Lock()
	delete(svc.splitCancels, normalizeRepoKey(libID))
	svc.opMu.Unlock()
	if !svc.splitCancelledAfterCheckpoint(ctx, libID, entry) {
		t.Fatal("条目已摘除（取消已发起、ctx 尚未取消）时应为 true")
	}
	if svc.splitCancelledAfterCheckpoint(context.Background(), libID, nil) {
		t.Fatal("entry 为 nil 且 ctx 未取消时应为 false")
	}
	cancel()
	if !svc.splitCancelledAfterCheckpoint(ctx, libID, nil) {
		t.Fatal("ctx 已取消时应为 true")
	}
	svc.unregisterSplitCancel(libID, entry)
}

// TestAssetSplitReplace_CancelFlagRecheckRollsBack 入口链路：索引阶段模拟
// CancelAssetSplit 摘除注册条目（ctx 未取消），删源前复查命中 → 回滚产物、
// 源保留、返回 E_ASSETS_CANCELLED。
func TestAssetSplitReplace_CancelFlagRecheckRollsBack(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "取消复查库", dir)
	src := buildAssetsPptx(t, dir, "复查.pptx", []assetsSlideSpec{
		{titlePh: "一", body: []string{"内容"}},
		{titlePh: "二", body: []string{"内容"}},
	})
	mustScanLibrary(t, svc, lib.ID)
	svc.splitCoreFn = (&fakeReplaceCore{pages: 2, payload: []byte("page-bytes")}).split
	// 解析阶段摘除取消注册条目（等价于 CancelAssetSplit 的「先摘除」半步）
	svc.parseFn = func(srcPath, baseName string) ([]parsedSlide, error) {
		svc.opMu.Lock()
		delete(svc.splitCancels, normalizeRepoKey(lib.ID))
		svc.opMu.Unlock()
		return oneSlideParse()(srcPath, baseName)
	}
	move, recycleCalls := fakeRecycleBin(t)
	svc.moveToRecycleBinFn = move

	_, err := svc.SplitAssetFile(context.Background(), lib.ID, "复查.pptx", "replace")
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsCancelled {
		t.Fatalf("取消复查命中应映射 E_ASSETS_CANCELLED，实际：%v", err)
	}
	if got := recycleCalls(); len(got) != 0 {
		t.Errorf("取消复查命中不得移入回收站，实际 %v", got)
	}
	if _, statErr := os.Stat(src); statErr != nil {
		t.Errorf("取消应保留源文件：%v", statErr)
	}
	if entries, readErr := os.ReadDir(filepath.Join(dir, "复查")); readErr == nil && len(entries) > 0 {
		t.Errorf("取消应回滚全部产物，实际 %d 项", len(entries))
	}
	store := svc.openStoreForTest(t, lib.ID)
	var prodRows int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM files WHERE path LIKE '复查/%'").Scan(&prodRows); err != nil || prodRows != 0 {
		t.Errorf("取消后不应残留产物索引行，实际 %d（err=%v）", prodRows, err)
	}
	if _, exists, err := store.fileRecordByPath("复查.pptx"); err != nil || !exists {
		t.Errorf("取消应保留源索引行，exists=%v err=%v", exists, err)
	}
}
