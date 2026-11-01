package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// 拆分记录随目录迁移（B 方案 PR2）用例：目录重命名/移动/前缀替换在同一事务内
// 同步 split_records 的 source_rel_path 与 output_dir，并解除「源仍存在的记录
// 拒绝目录操作」的旧守卫。

const splitMigrateNow = "2026-11-01 10:00:00"

// TestSplitRecordMigrate_RenameFolderEndToEnd 目录重命名（中文目录名）后 replace
// 记录的路径与落点均迁移：批量源清单剔除产物、落点快照返回新 output_dir、单文件
// 入口对产物二次拆分拒绝。
func TestSplitRecordMigrate_RenameFolderEndToEnd(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "方案A", "城商行方案.pptx"))
	artifactOldRel := "方案A/城商行方案/001-城商行方案.pptx"
	mustWritePptxFile(t, filepath.Join(root, filepath.FromSlash(artifactOldRel)))
	lib := mustCreateLibrary(t, svc, "迁移库", root)
	mustScan(t, svc, lib.ID)

	store := svc.openStoreForTest(t, lib.ID)
	oldOut := filepath.Join(root, "方案A", "城商行方案")
	if err := store.upsertSplitRecord("方案A/城商行方案.pptx", "hash-1", oldOut, 2,
		splitMigrateNow, model.AssetSplitTargetReplace, true); err != nil {
		t.Fatalf("写拆分记录: %v", err)
	}

	if _, err := svc.RenameAssetFolder(lib.ID, "方案A", "方案B"); err != nil {
		t.Fatalf("RenameAssetFolder: %v", err)
	}

	// source_rel_path 迁移：新键可查、旧键不残留
	rec, err := store.getSplitRecord("方案B/城商行方案.pptx")
	if err != nil || rec == nil {
		t.Fatalf("拆分记录应迁移到新路径：rec=%v err=%v", rec, err)
	}
	if old, _ := store.getSplitRecord("方案A/城商行方案.pptx"); old != nil {
		t.Errorf("旧路径拆分记录不应残留：%+v", old)
	}
	newOut := filepath.Join(root, "方案B", "城商行方案")
	if rec.outputDir != newOut {
		t.Errorf("replace 记录 output_dir 应重算为 %q，实际 %q", newOut, rec.outputDir)
	}

	// listReplaceSplitOutputs 返回新落点
	outputs, err := store.listReplaceSplitOutputs()
	if err != nil {
		t.Fatalf("listReplaceSplitOutputs: %v", err)
	}
	foundOut := false
	for _, o := range outputs {
		if o.sourceRel == "方案B/城商行方案.pptx" && o.outputDir == newOut {
			foundOut = true
		}
	}
	if !foundOut {
		t.Errorf("落点快照应返回新 output_dir，实际 %+v", outputs)
	}

	// filterReplaceArtifactSources 在新目录下剔除产物（未迁移时会漏剔）
	newArtifactRel := "方案B/城商行方案/001-城商行方案.pptx"
	sources, err := store.listOKFileSplitSources()
	if err != nil {
		t.Fatalf("listOKFileSplitSources: %v", err)
	}
	foundRaw := false
	for _, s := range sources {
		if s.rel == newArtifactRel {
			foundRaw = true
		}
	}
	if !foundRaw {
		t.Fatalf("夹具产物应已进索引（剔除断言前置）：%+v", sources)
	}
	filtered := filterReplaceArtifactSources(lib, sources, outputs)
	for _, s := range filtered {
		if s.rel == newArtifactRel {
			t.Errorf("迁移后批量源清单应剔除 replace 产物")
		}
	}

	// 单文件入口拒绝把产物再次 replace 拆分（迁移后落点归属仍成立）
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, newArtifactRel, model.AssetSplitTargetReplace); err == nil ||
		!strings.Contains(err.Error(), "不能再次拆分") {
		t.Errorf("产物二次 replace 拆分应被拒绝，实际：%v", err)
	}
}

// TestSplitRecordMigrate_MoveFolderThreeTargets 目录移动后 cache/source/replace
// 三类记录 source_rel_path 全部迁移；cache output_dir 保持原绝对路径，
// source/replace 按库根重算。
func TestSplitRecordMigrate_MoveFolderThreeTargets(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		mustWritePptxFile(t, filepath.Join(root, "src", name+".pptx"))
	}
	if err := os.MkdirAll(filepath.Join(root, "dst"), 0o755); err != nil {
		t.Fatalf("MkdirAll(dst): %v", err)
	}
	lib := mustCreateLibrary(t, svc, "三类迁移库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)

	cacheOut := filepath.Join(root, "assets", "lib", "derived", "src", "a")
	if err := store.upsertSplitRecord("src/a.pptx", "ha", cacheOut, 2, splitMigrateNow, model.AssetSplitTargetCache, false); err != nil {
		t.Fatalf("upsert cache: %v", err)
	}
	if err := store.upsertSplitRecord("src/b.pptx", "hb", filepath.Join(root, "src", "b"), 2, splitMigrateNow, model.AssetSplitTargetSource, false); err != nil {
		t.Fatalf("upsert source: %v", err)
	}
	if err := store.upsertSplitRecord("src/c.pptx", "hc", filepath.Join(root, "src", "c"), 2, splitMigrateNow, model.AssetSplitTargetReplace, true); err != nil {
		t.Fatalf("upsert replace: %v", err)
	}

	if _, err := svc.MoveAssetFolder(lib.ID, "src", "dst"); err != nil {
		t.Fatalf("MoveAssetFolder: %v", err)
	}

	cases := []struct {
		rel     string
		wantOut string
	}{
		{"dst/src/a.pptx", cacheOut},                            // cache：物理目录不随迁移
		{"dst/src/b.pptx", filepath.Join(root, "dst/src", "b")}, // source：库根重算
		{"dst/src/c.pptx", filepath.Join(root, "dst/src", "c")}, // replace：库根重算
	}
	for _, tc := range cases {
		rec, err := store.getSplitRecord(tc.rel)
		if err != nil || rec == nil {
			t.Errorf("记录应迁移到 %q：rec=%v err=%v", tc.rel, rec, err)
			continue
		}
		if rec.outputDir != tc.wantOut {
			t.Errorf("%q 的 output_dir 应为 %q，实际 %q", tc.rel, tc.wantOut, rec.outputDir)
		}
	}
	for _, old := range []string{"src/a.pptx", "src/b.pptx", "src/c.pptx"} {
		if rec, _ := store.getSplitRecord(old); rec != nil {
			t.Errorf("旧路径记录不应残留：%q → %+v", old, rec)
		}
	}
}

// TestSplitRecordMigrate_EmptyFolderOnlyRecord 空目录（无 files 行）仅有拆分
// 记录时迁移照常执行（replaceFilePaths 对空 renames 仍跑拆分记录迁移）。
func TestSplitRecordMigrate_EmptyFolderOnlyRecord(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "空目录"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "空目录迁移库", root)
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord("空目录/孤.pptx", "h", filepath.Join(root, "空目录", "孤"), 2,
		splitMigrateNow, model.AssetSplitTargetSource, false); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}

	if _, err := svc.RenameAssetFolder(lib.ID, "空目录", "空目录2"); err != nil {
		t.Fatalf("空目录重命名: %v", err)
	}
	rec, err := store.getSplitRecord("空目录2/孤.pptx")
	if err != nil || rec == nil {
		t.Fatalf("空目录的拆分记录应迁移：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "空目录2", "孤"); rec.outputDir != want {
		t.Errorf("source 记录 output_dir 应为 %q，实际 %q", want, rec.outputDir)
	}
}

// TestSplitRecordMigrate_Idempotent 重复迁移零变更：改写后前缀不再命中；大小写
// 变体命中时按改写结果与原值相等跳过。
func TestSplitRecordMigrate_Idempotent(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "幂等迁移库", root)
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord("旧/a.pptx", "h", "out-a", 2, splitMigrateNow, model.AssetSplitTargetCache, false); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	mig := splitRecordMigration{libraryRoot: root, oldPrefix: "旧", newPrefix: "新"}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	n1, err := store.migrateSplitRecordsTx(tx, mig)
	if err != nil {
		t.Fatalf("首次迁移: %v", err)
	}
	n2, err := store.migrateSplitRecordsTx(tx, mig)
	if err != nil {
		t.Fatalf("二次迁移: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if n1 != 1 || n2 != 0 {
		t.Errorf("首次迁移 1 行、重复迁移 0 行，实际 %d / %d", n1, n2)
	}
	if rec, _ := store.getSplitRecord("新/a.pptx"); rec == nil {
		t.Errorf("记录应已迁移到新前缀")
	}

	// 大小写变体：路径已是目标形态（ABC/b.pptx），oldPrefix=abc 命中但改写结果相等 → 跳过
	if err := store.upsertSplitRecord("ABC/b.pptx", "h", "out-b", 2, splitMigrateNow, model.AssetSplitTargetCache, false); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	tx2, err := store.db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	n3, err := store.migrateSplitRecordsTx(tx2, splitRecordMigration{libraryRoot: root, oldPrefix: "abc", newPrefix: "ABC"})
	if err != nil {
		t.Fatalf("大小写变体迁移: %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if n3 != 0 {
		t.Errorf("大小写变体命中且改写后相同应零变更，实际 %d 行", n3)
	}
	if rec, _ := store.getSplitRecord("ABC/b.pptx"); rec == nil {
		t.Errorf("原记录应保持不动")
	}
}

// TestSplitRecordMigrate_NestedPrefixChain 嵌套前缀链式腾位（新前缀位于旧前缀
// 子树内，如「旧目录 → 旧目录/归档」）：某条较短记录的新键恰是另一条更长记录的
// 旧键，两条记录都必须存活并各自落到链式腾位后的新路径，不得被误判为目标键冲突
// 而删除（对齐 assetPathRenamesForPrefix 的按旧路径长度降序安全序）。
func TestSplitRecordMigrate_NestedPrefixChain(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "嵌套链迁移库", root)
	store := svc.openStoreForTest(t, lib.ID)
	// 短记录在前插入（rowid 序即查询序），若未按长度降序处理会先落位撞上长记录旧键
	if err := store.upsertSplitRecord("d/a.pptx", "h-short", filepath.Join(root, "d", "a"), 2,
		splitMigrateNow, model.AssetSplitTargetSource, false); err != nil {
		t.Fatalf("upsert short: %v", err)
	}
	if err := store.upsertSplitRecord("d/sub/a.pptx", "h-long", filepath.Join(root, "d", "sub", "a"), 2,
		splitMigrateNow, model.AssetSplitTargetSource, false); err != nil {
		t.Fatalf("upsert long: %v", err)
	}

	if err := store.replaceFilePaths(nil, splitMigrateNow, "", "",
		splitRecordMigration{libraryRoot: root, oldPrefix: "d", newPrefix: "d/sub"}); err != nil {
		t.Fatalf("嵌套前缀迁移: %v", err)
	}

	// d/a.pptx → d/sub/a.pptx（短记录落位），d/sub/a.pptx → d/sub/sub/a.pptx（长记录腾位）
	for _, want := range []struct {
		rel, hash, out string
	}{
		{"d/sub/a.pptx", "h-short", filepath.Join(root, "d", "sub", "a")},
		{"d/sub/sub/a.pptx", "h-long", filepath.Join(root, "d", "sub", "sub", "a")},
	} {
		rec, err := store.getSplitRecord(want.rel)
		if err != nil || rec == nil {
			t.Errorf("链式腾位后记录 %q 应存活：rec=%v err=%v", want.rel, rec, err)
			continue
		}
		if rec.sourceHash != want.hash {
			t.Errorf("%q 应来自 %q 记录，实际 %q", want.rel, want.hash, rec.sourceHash)
		}
		if rec.outputDir != want.out {
			t.Errorf("%q 的 output_dir 应为 %q，实际 %q", want.rel, want.out, rec.outputDir)
		}
	}
	for _, old := range []string{"d/a.pptx"} {
		if rec, _ := store.getSplitRecord(old); rec != nil {
			t.Errorf("旧短键 %q 应已腾空，实际 %+v", old, rec)
		}
	}
}

// TestSplitRecordMigrate_ConflictKeepsRemovedTrace 迁移后目标键冲突：保留
// source_removed=1 的一条（两条同类保留既有行），落败行删除且迁移不中断。
func TestSplitRecordMigrate_ConflictKeepsRemovedTrace(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "冲突迁移库", root)
	store := svc.openStoreForTest(t, lib.ID)
	// a：既有行是留痕（removed=1），待迁移行源仍在 → 既有胜出
	// b：待迁移行是留痕，既有行源仍在 → 留痕胜出
	// c：两条同类（均源仍在）→ 既有胜出
	seed := []struct {
		rel, hash, out string
		removed        bool
	}{
		{"old/a.pptx", "mig-a", "migOutA", false},
		{"new/a.pptx", "exist-a", "existingDirA", true},
		{"old/b.pptx", "mig-b", "migOutB", true},
		{"new/b.pptx", "exist-b", "existingDirB", false},
		{"old/c.pptx", "mig-c", "migOutC", false},
		{"new/c.pptx", "exist-c", "existingDirC", false},
	}
	for _, s := range seed {
		if err := store.upsertSplitRecord(s.rel, s.hash, s.out, 2, splitMigrateNow, model.AssetSplitTargetCache, s.removed); err != nil {
			t.Fatalf("upsert %s: %v", s.rel, err)
		}
	}

	if err := store.replaceFilePaths(nil, splitMigrateNow, "", "",
		splitRecordMigration{libraryRoot: root, oldPrefix: "old", newPrefix: "new"}); err != nil {
		t.Fatalf("迁移遇冲突不应中断: %v", err)
	}

	// a：既有留痕胜出
	if rec, _ := store.getSplitRecord("old/a.pptx"); rec != nil {
		t.Errorf("落败行 old/a.pptx 应删除")
	}
	if rec, _ := store.getSplitRecord("new/a.pptx"); rec == nil || rec.sourceHash != "exist-a" || !rec.sourceRemoved {
		t.Errorf("应保留既有留痕记录，实际 %+v", rec)
	}
	// b：迁移的留痕胜出
	if rec, _ := store.getSplitRecord("old/b.pptx"); rec != nil {
		t.Errorf("落败行 old/b.pptx 应删除")
	}
	if rec, _ := store.getSplitRecord("new/b.pptx"); rec == nil || rec.sourceHash != "mig-b" || !rec.sourceRemoved {
		t.Errorf("留痕记录应胜出并落位新键，实际 %+v", rec)
	}
	// c：同类保留既有行
	if rec, _ := store.getSplitRecord("new/c.pptx"); rec == nil || rec.sourceHash != "exist-c" {
		t.Errorf("两条同类应保留既有行，实际 %+v", rec)
	}
}

// TestSplitRecordMigrate_PrefixCaseDifference ReplaceAssetPathPrefix 在旧前缀与
// 实际路径存在 ASCII 大小写差异时正确迁移，不产生重复路径段。
func TestSplitRecordMigrate_PrefixCaseDifference(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "Alpha", "a.pptx"))
	lib := mustCreateLibrary(t, svc, "大小写迁移库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord("Alpha/a.pptx", "h", filepath.Join(root, "Alpha", "a"), 2,
		splitMigrateNow, model.AssetSplitTargetSource, false); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}

	result, err := svc.ReplaceAssetPathPrefix(lib.ID, "alpha", "Beta")
	if err != nil {
		t.Fatalf("ReplaceAssetPathPrefix: %v", err)
	}
	if result.Replaced != 1 {
		t.Errorf("大小写差异前缀应命中 1 条，实际 %d", result.Replaced)
	}
	if rec, _ := store.getSplitRecord("Beta/Alpha/a.pptx"); rec != nil {
		t.Errorf("前缀剥离不得把整条旧路径当后缀（重复路径段）：%+v", rec)
	}
	rec, err := store.getSplitRecord("Beta/a.pptx")
	if err != nil || rec == nil {
		t.Fatalf("记录应迁移为 Beta/a.pptx：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "Beta", "a"); rec.outputDir != want {
		t.Errorf("output_dir 应重算为 %q，实际 %q", want, rec.outputDir)
	}
}

// TestSplitRecordMigrate_RollbackOnFailure 迁移中途失败（触发器注入）时
// split_records、files.path、folder_meta 全部回滚。
func TestSplitRecordMigrate_RollbackOnFailure(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "旧", "a.pptx"))
	lib := mustCreateLibrary(t, svc, "回滚库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)

	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", "旧/a.pptx").Scan(&fileID); err != nil {
		t.Fatalf("查文件记录: %v", err)
	}
	if err := store.updateFolderMeta("旧", "目录备注", nil, splitMigrateNow); err != nil {
		t.Fatalf("写文件夹标注: %v", err)
	}
	if err := store.upsertSplitRecord("旧/a.pptx", "h", "oldOut", 2, splitMigrateNow, model.AssetSplitTargetCache, false); err != nil {
		t.Fatalf("写拆分记录: %v", err)
	}
	if _, err := store.db.Exec(
		"CREATE TRIGGER block_split_mig BEFORE UPDATE ON split_records BEGIN SELECT RAISE(ABORT, 'blocked'); END",
	); err != nil {
		t.Fatalf("建注入触发器: %v", err)
	}

	err := store.replaceFilePaths([]assetPathRename{
		{fileID: fileID, oldPath: "旧/a.pptx", newRel: "新/a.pptx"},
	}, splitMigrateNow, "旧", "新", splitRecordMigration{libraryRoot: root, oldPrefix: "旧", newPrefix: "新"})
	if err == nil {
		t.Fatalf("注入触发器后迁移应失败")
	}

	// files.path 回滚
	var path string
	if err := store.db.QueryRow("SELECT path FROM files WHERE id = ?", fileID).Scan(&path); err != nil || path != "旧/a.pptx" {
		t.Errorf("files.path 应回滚为旧路径，实际 %q（err=%v）", path, err)
	}
	// folder_meta 回滚
	if note, _, _, _ := store.getFolderMeta("旧"); note != "目录备注" {
		t.Errorf("folder_meta 旧键应保留，实际 %q", note)
	}
	if note, _, _, _ := store.getFolderMeta("新"); note != "" {
		t.Errorf("folder_meta 新键不应出现，实际 %q", note)
	}
	// split_records 回滚
	rec, err := store.getSplitRecord("旧/a.pptx")
	if err != nil || rec == nil || rec.outputDir != "oldOut" {
		t.Errorf("split_records 应回滚到旧键与原值，实际 %+v（err=%v）", rec, err)
	}
}

// TestSplitRecordMigrate_FilterBehaviorDifference 迁移前后行为差异：未迁移记录
// （仍指旧 output_dir）无法剔除新路径下的产物，迁移后即剔除。
func TestSplitRecordMigrate_FilterBehaviorDifference(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "行为差异库", root)
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord("old/a.pptx", "h", filepath.Join(root, "old", "a"), 2,
		splitMigrateNow, model.AssetSplitTargetReplace, true); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}
	sources := []assetSplitSource{{rel: "new/a/001-a.pptx"}}

	outputs, err := store.listReplaceSplitOutputs()
	if err != nil {
		t.Fatalf("listReplaceSplitOutputs: %v", err)
	}
	if got := filterReplaceArtifactSources(lib, sources, outputs); len(got) != 1 {
		t.Errorf("未迁移时落点失配，产物不应被剔除，实际 %+v", got)
	}

	if err := store.replaceFilePaths(nil, splitMigrateNow, "", "",
		splitRecordMigration{libraryRoot: root, oldPrefix: "old", newPrefix: "new"}); err != nil {
		t.Fatalf("迁移: %v", err)
	}
	outputs, err = store.listReplaceSplitOutputs()
	if err != nil {
		t.Fatalf("listReplaceSplitOutputs: %v", err)
	}
	if len(outputs) != 1 || outputs[0].outputDir != filepath.Join(root, "new", "a") {
		t.Errorf("落点快照应重算为新 output_dir，实际 %+v", outputs)
	}
	if got := filterReplaceArtifactSources(lib, sources, outputs); len(got) != 0 {
		t.Errorf("迁移后应剔除 replace 产物，实际 %+v", got)
	}
}
