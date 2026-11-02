package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// replace 产物容器移动收口用例（2026-11-01 审核修复）：覆盖 output_dir 前缀迁移
//（容器目录重命名/移动/前缀替换）、单页产物禁止单独移动、目录边界与 LIKE 元字符、
// 嵌套前缀端到端与幂等。

// TestSplitRecordMigrate_ContainerOutputDirOnly 源键不在被移动目录下、仅产物
// 容器目录位于其下的 replace 记录：目录重命名后 output_dir 迁移（source_rel_path
// 保持），新路径产物被批量剔除、单文件入口拒拆；重复迁移零变更（幂等）。
func TestSplitRecordMigrate_ContainerOutputDirOnly(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "其他", "x.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "容器", "001-x.pptx"))
	lib := mustCreateLibrary(t, svc, "容器落点迁移库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)

	oldOut := filepath.Join(root, "容器")
	if err := store.upsertSplitRecord("其他/x.pptx", "h-container", oldOut, 2,
		splitMigrateNow, model.AssetSplitTargetReplace, "",
		true, nil); err != nil {
		t.Fatalf("写拆分记录: %v", err)
	}

	if _, err := svc.RenameAssetFolder(lib.ID, "容器", "新容器"); err != nil {
		t.Fatalf("RenameAssetFolder: %v", err)
	}

	// 源键不命中：记录键保持原值；落点迁移到新目录
	rec, err := store.getSplitRecord("其他/x.pptx")
	if err != nil || rec == nil {
		t.Fatalf("源键未命中的记录应原样保留：rec=%v err=%v", rec, err)
	}
	newOut := filepath.Join(root, "新容器")
	if rec.outputDir != newOut {
		t.Errorf("容器迁移后 output_dir 应为 %q，实际 %q", newOut, rec.outputDir)
	}

	// 幂等：同一迁移重复执行零变更
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	n, err := store.migrateSplitRecordsTx(tx, splitRecordMigration{
		libraryRoot: root, oldPrefix: "容器", newPrefix: "新容器",
	})
	if err != nil {
		t.Fatalf("重复迁移: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if n != 0 {
		t.Errorf("重复迁移应零变更，实际 %d 行", n)
	}
	if rec, _ := store.getSplitRecord("其他/x.pptx"); rec == nil || rec.outputDir != newOut {
		t.Errorf("重复迁移不得改写落点，实际 %+v", rec)
	}

	// 行为面：新路径产物被批量源清单剔除
	newArtifactRel := "新容器/001-x.pptx"
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
		t.Fatalf("迁移后产物应已进索引（剔除断言前置）：%+v", sources)
	}
	outputs, err := store.listReplaceSplitOutputs()
	if err != nil {
		t.Fatalf("listReplaceSplitOutputs: %v", err)
	}
	for _, s := range filterReplaceArtifactSources(lib, sources, outputs) {
		if s.rel == newArtifactRel {
			t.Errorf("output_dir 迁移后批量源清单应剔除产物 %q", newArtifactRel)
		}
	}

	// 单文件入口拒绝把移动后的产物再次 replace 拆分
	if _, err := svc.SplitAssetFile(context.Background(), lib.ID, newArtifactRel, model.AssetSplitTargetReplace); err == nil ||
		!strings.Contains(err.Error(), "不能再次拆分") {
		t.Errorf("产物二次 replace 拆分应被拒绝，实际：%v", err)
	}
}

// TestSplitRecordMigrate_NestedContainerMove 多级嵌套容器随上级目录整体移动：
// 源键命中的记录按新源路径重算落点，仅落点命中的记录按旧前缀剥离迁移，cache
// 记录落点保持原值。
func TestSplitRecordMigrate_NestedContainerMove(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "外部", "方案", "x.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "目标"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "嵌套容器移动库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)

	// 记录 1：源键命中（replace），落点随源路径重算
	if err := store.upsertSplitRecord("外部/方案/x.pptx", "h-in", filepath.Join(root, "外部", "方案", "x"),
		2, splitMigrateNow, model.AssetSplitTargetReplace, "",
		false, nil); err != nil {
		t.Fatalf("upsert 源键记录: %v", err)
	}
	// 记录 2：源键不命中、仅落点命中（source）
	if err := store.upsertSplitRecord("其他/y.pptx", "h-out", filepath.Join(root, "外部", "方案", "y"),
		2, splitMigrateNow, model.AssetSplitTargetSource, "",
		false, nil); err != nil {
		t.Fatalf("upsert 落点记录: %v", err)
	}
	// 记录 3：cache 落点即使在移动目录下也保持原值（target 显式分流）
	cacheOut := filepath.Join(root, "外部", "方案", "z")
	if err := store.upsertSplitRecord("其他/z.pptx", "h-cache", cacheOut,
		2, splitMigrateNow, model.AssetSplitTargetCache, "",
		false, nil); err != nil {
		t.Fatalf("upsert cache 记录: %v", err)
	}

	if _, err := svc.MoveAssetFolder(lib.ID, "外部", "目标"); err != nil {
		t.Fatalf("MoveAssetFolder: %v", err)
	}

	// 记录 1：键迁移到 目标/外部/方案/x.pptx，落点按新源路径重算
	rec, err := store.getSplitRecord("目标/外部/方案/x.pptx")
	if err != nil || rec == nil {
		t.Fatalf("源键应迁移到新路径：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "目标", "外部", "方案", "x"); rec.outputDir != want {
		t.Errorf("源键命中记录 output_dir 应为 %q，实际 %q", want, rec.outputDir)
	}
	// 记录 2：键不变，落点按目录前缀迁移
	rec, err = store.getSplitRecord("其他/y.pptx")
	if err != nil || rec == nil {
		t.Fatalf("仅落点命中的记录键应保持：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "目标", "外部", "方案", "y"); rec.outputDir != want {
		t.Errorf("仅落点命中记录 output_dir 应为 %q，实际 %q", want, rec.outputDir)
	}
	// 记录 3：cache 保持原绝对路径
	rec, err = store.getSplitRecord("其他/z.pptx")
	if err != nil || rec == nil {
		t.Fatalf("cache 记录应保留：rec=%v err=%v", rec, err)
	}
	if rec.outputDir != cacheOut {
		t.Errorf("cache 记录 output_dir 应保持 %q，实际 %q", cacheOut, rec.outputDir)
	}
}

// TestSplitRecordMigrate_PrefixReplaceContainerOutput ReplaceAssetPathPrefix 目录
// 前缀替换同样迁移 output_dir（源键不命中的容器记录），cache 记录不受影响。
func TestSplitRecordMigrate_PrefixReplaceContainerOutput(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "外部", "方案", "a.pptx"))
	lib := mustCreateLibrary(t, svc, "前缀替换落点库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)

	if err := store.upsertSplitRecord("其他/x.pptx", "h-prefix", filepath.Join(root, "外部", "方案", "x"),
		2, splitMigrateNow, model.AssetSplitTargetReplace, "",
		false, nil); err != nil {
		t.Fatalf("upsert replace: %v", err)
	}
	cacheOut := filepath.Join(root, "外部", "方案", "z")
	if err := store.upsertSplitRecord("其他/z.pptx", "h-prefix-cache", cacheOut,
		2, splitMigrateNow, model.AssetSplitTargetCache, "",
		false, nil); err != nil {
		t.Fatalf("upsert cache: %v", err)
	}

	result, err := svc.ReplaceAssetPathPrefix(lib.ID, "外部", "归档")
	if err != nil {
		t.Fatalf("ReplaceAssetPathPrefix: %v", err)
	}
	if result.Replaced < 1 {
		t.Fatalf("前缀替换应命中文件行，实际 %+v", result)
	}

	rec, err := store.getSplitRecord("其他/x.pptx")
	if err != nil || rec == nil {
		t.Fatalf("源键不命中的记录应保留：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "归档", "方案", "x"); rec.outputDir != want {
		t.Errorf("前缀替换后 replace 落点应为 %q，实际 %q", want, rec.outputDir)
	}
	rec, err = store.getSplitRecord("其他/z.pptx")
	if err != nil || rec == nil {
		t.Fatalf("cache 记录应保留：rec=%v err=%v", rec, err)
	}
	if rec.outputDir != cacheOut {
		t.Errorf("前缀替换不得改动 cache 落点，期望 %q，实际 %q", cacheOut, rec.outputDir)
	}
}

// TestSplitRecordMigrate_PrefixBoundary 目录边界负例：oldPrefix=a 只迁移「a/」
// 与恰等 a，不误伤同头缀「ab/」；仅落点命中的记录同口径；cache 落点保持。
func TestSplitRecordMigrate_PrefixBoundary(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "边界迁移库", root)
	store := svc.openStoreForTest(t, lib.ID)

	seed := []struct {
		rel, hash, out, target string
	}{
		{"a/x.pptx", "h-a", filepath.Join(root, "a", "x"), model.AssetSplitTargetSource},
		{"ab/x.pptx", "h-ab", filepath.Join(root, "ab", "x"), model.AssetSplitTargetReplace},
		{"其他/y.pptx", "h-y", filepath.Join(root, "a", "y"), model.AssetSplitTargetSource},
		{"其他/z.pptx", "h-z", filepath.Join(root, "a", "z"), model.AssetSplitTargetCache},
	}
	for _, s := range seed {
		if err := store.upsertSplitRecord(s.rel, s.hash, s.out, 2, splitMigrateNow, s.target, "", false, nil); err != nil {
			t.Fatalf("upsert %s: %v", s.rel, err)
		}
	}

	if err := store.replaceFilePaths(nil, splitMigrateNow, "", "",
		splitRecordMigration{libraryRoot: root, oldPrefix: "a", newPrefix: "b"}); err != nil {
		t.Fatalf("边界迁移: %v", err)
	}

	// a/x 迁移；ab/x 不迁移（同头缀不误伤）
	if rec, _ := store.getSplitRecord("a/x.pptx"); rec != nil {
		t.Errorf("旧键 a/x.pptx 应已迁移")
	}
	rec, err := store.getSplitRecord("b/x.pptx")
	if err != nil || rec == nil {
		t.Fatalf("a/x.pptx 应迁移到 b/x.pptx：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "b", "x"); rec.outputDir != want {
		t.Errorf("a/x.pptx 落点应为 %q，实际 %q", want, rec.outputDir)
	}
	rec, err = store.getSplitRecord("ab/x.pptx")
	if err != nil || rec == nil {
		t.Fatalf("同头缀目录 ab/ 的记录不应被迁移：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "ab", "x"); rec.outputDir != want {
		t.Errorf("ab/x.pptx 落点应保持 %q，实际 %q", want, rec.outputDir)
	}
	// 仅落点命中迁移；cache 保持
	rec, err = store.getSplitRecord("其他/y.pptx")
	if err != nil || rec == nil {
		t.Fatalf("其他/y.pptx 应保留：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "b", "y"); rec.outputDir != want {
		t.Errorf("仅落点命中的记录落点应为 %q，实际 %q", want, rec.outputDir)
	}
	rec, err = store.getSplitRecord("其他/z.pptx")
	if err != nil || rec == nil {
		t.Fatalf("其他/z.pptx 应保留：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "a", "z"); rec.outputDir != want {
		t.Errorf("cache 落点应保持 %q，实际 %q", want, rec.outputDir)
	}
}

// TestSplitRecordMigrate_LikeMetaDirRename LIKE 元字符目录名（50%off）重命名：
// 源键与落点均正确迁移（SQL LIKE 转义单点 + Go 侧目录边界剥离）。
func TestSplitRecordMigrate_LikeMetaDirRename(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "50%off", "x.pptx"))
	lib := mustCreateLibrary(t, svc, "元字符目录库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)

	if err := store.upsertSplitRecord("50%off/x.pptx", "h-like", filepath.Join(root, "50%off", "x"),
		2, splitMigrateNow, model.AssetSplitTargetReplace, "",
		false, nil); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// 仅落点命中同一元字符目录（输出目录前缀迁移面，修复前不会改写）
	if err := store.upsertSplitRecord("其他/y.pptx", "h-like-out", filepath.Join(root, "50%off", "y"),
		2, splitMigrateNow, model.AssetSplitTargetSource, "",
		false, nil); err != nil {
		t.Fatalf("upsert 落点记录: %v", err)
	}

	if _, err := svc.RenameAssetFolder(lib.ID, "50%off", "50%off2"); err != nil {
		t.Fatalf("RenameAssetFolder: %v", err)
	}

	if rec, _ := store.getSplitRecord("50%off/x.pptx"); rec != nil {
		t.Errorf("旧元字符目录键应已迁移，实际 %+v", rec)
	}
	rec, err := store.getSplitRecord("50%off2/x.pptx")
	if err != nil || rec == nil {
		t.Fatalf("记录应迁移到 50%%off2/x.pptx：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "50%off2", "x"); rec.outputDir != want {
		t.Errorf("落点应为 %q，实际 %q", want, rec.outputDir)
	}
	rec, err = store.getSplitRecord("其他/y.pptx")
	if err != nil || rec == nil {
		t.Fatalf("仅落点命中的记录应保留：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "50%off2", "y"); rec.outputDir != want {
		t.Errorf("仅落点命中记录的落点应为 %q，实际 %q", want, rec.outputDir)
	}
}

// TestSplitRecordMigrate_NestedPrefixEndToEnd 嵌套前缀（d → d/sub）经
// ReplaceAssetPathPrefix 端到端：源键与落点按链式腾位同时迁移，两记录均存活。
func TestSplitRecordMigrate_NestedPrefixEndToEnd(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "d", "a.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "d", "sub", "a.pptx"))
	lib := mustCreateLibrary(t, svc, "嵌套端到端库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)

	if err := store.upsertSplitRecord("d/a.pptx", "h-short", filepath.Join(root, "d", "a"),
		2, splitMigrateNow, model.AssetSplitTargetReplace, "",
		false, nil); err != nil {
		t.Fatalf("upsert short: %v", err)
	}
	if err := store.upsertSplitRecord("d/sub/a.pptx", "h-long", filepath.Join(root, "d", "sub", "a"),
		2, splitMigrateNow, model.AssetSplitTargetReplace, "",
		false, nil); err != nil {
		t.Fatalf("upsert long: %v", err)
	}
	// 仅落点命中嵌套旧前缀（输出目录前缀迁移面，修复前不会改写）
	if err := store.upsertSplitRecord("其他/n.pptx", "h-out-only", filepath.Join(root, "d", "sub", "n"),
		2, splitMigrateNow, model.AssetSplitTargetSource, "",
		false, nil); err != nil {
		t.Fatalf("upsert output-only: %v", err)
	}

	result, err := svc.ReplaceAssetPathPrefix(lib.ID, "d", "d/sub")
	if err != nil {
		t.Fatalf("ReplaceAssetPathPrefix: %v", err)
	}
	if result.Replaced != 2 {
		t.Fatalf("嵌套前缀应命中 2 条文件行，实际 %+v", result)
	}

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
	// 仅落点命中的记录：键不变，落点按嵌套新前缀迁移
	rec, err := store.getSplitRecord("其他/n.pptx")
	if err != nil || rec == nil {
		t.Fatalf("仅落点命中的记录应保留：rec=%v err=%v", rec, err)
	}
	if want := filepath.Join(root, "d", "sub", "sub", "n"); rec.outputDir != want {
		t.Errorf("嵌套前缀下仅落点命中的记录 output_dir 应为 %q，实际 %q", want, rec.outputDir)
	}
}

// TestAssetMoveFiles_RejectsReplaceArtifact replace 单页产物禁止被批量移动：
// 命中 failed 明细（文件与记录均不动），同批其余文件照常移动成功。
func TestAssetMoveFiles_RejectsReplaceArtifact(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "容器", "001-x.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "其他", "a.pptx"))
	lib := mustCreateLibrary(t, svc, "产物移动拒绝库", root)
	mustScan(t, svc, lib.ID)
	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord("其他/x.pptx", "h-move", filepath.Join(root, "容器"),
		2, splitMigrateNow, model.AssetSplitTargetReplace, "",
		true, nil); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}
	entries, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	idByPath := map[string]int64{}
	for _, e := range entries {
		idByPath[e.Path] = e.ID
	}
	artifactID, okArtifact := idByPath["容器/001-x.pptx"]
	otherID, okOther := idByPath["其他/a.pptx"]
	if !okArtifact || !okOther {
		t.Fatalf("夹具应已进索引：%v", idByPath)
	}

	result, err := svc.MoveAssetFiles(lib.ID, []int64{artifactID, otherID}, "")
	if err != nil {
		t.Fatalf("MoveAssetFiles: %v", err)
	}
	if result.Total != 2 || result.Failed != 1 || result.Succeeded != 1 {
		t.Fatalf("产物拒绝计入 failed、其余文件成功，实际 %+v", result)
	}
	var artifactItem *model.AssetBatchMoveItem
	for i := range result.Items {
		if result.Items[i].FileID == artifactID {
			artifactItem = &result.Items[i]
		}
	}
	if artifactItem == nil || artifactItem.Status != model.AssetBatchTagStatusFailed {
		t.Fatalf("产物条目应记 failed，实际 %+v", result.Items)
	}
	if !strings.Contains(artifactItem.Message, "整体移动其所在产物目录") {
		t.Errorf("失败明细应提示整体移动产物容器目录，实际 %q", artifactItem.Message)
	}

	// 产物磁盘位置与索引路径均未动
	if _, err := os.Stat(filepath.Join(root, "容器", "001-x.pptx")); err != nil {
		t.Errorf("产物磁盘文件不应被移动：%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "001-x.pptx")); !os.IsNotExist(err) {
		t.Errorf("产物不应落到库根：%v", err)
	}
	entries, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	paths := map[string]bool{}
	for _, e := range entries {
		paths[e.Path] = true
	}
	if !paths["容器/001-x.pptx"] {
		t.Errorf("产物索引路径不应变化：%v", paths)
	}
	if !paths["a.pptx"] {
		t.Errorf("同批其余文件应移动成功：%v", paths)
	}
	rec, err := store.getSplitRecord("其他/x.pptx")
	if err != nil || rec == nil || rec.outputDir != filepath.Join(root, "容器") {
		t.Errorf("拆分记录应保持原落点，实际 %+v（err=%v）", rec, err)
	}
}
