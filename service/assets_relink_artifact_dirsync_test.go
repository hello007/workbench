package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// 重链时识别产物目录已随源搬迁（11-01-relink-artifact-dirsync，PRD 方案 A）单测：
//   - source / replace 落点的源文件与同名产物容器目录一起搬走后重链 → output_dir
//     改指新位置、打开单页可用、重复重链幂等；
//   - 仅搬走源文件、产物留原处 → output_dir 保持旧值（现有行为回归）；
//   - 新位置同名目录存在但产物形态不匹配 → 不误迁移；
//   - cache 落点即使新位置有同名产物也不迁移（产物在应用数据目录，不参与判定）；
//   - 拆分记录写失败时 files.path 与 output_dir 均不变（同一事务）。

const dirSyncSplitAt = "2026-11-01 10:00:00"

// dirSyncFixture 搬迁探测用例夹具：库根下 `旧目录/方案.pptx` 已扫描，一条落点由
// target 决定的拆分记录 + 两页产物；newRel / newAbs / newOut 为搬迁目标位。
type dirSyncFixture struct {
	svc    *AssetLibraryService
	lib    model.AssetLibrary
	root   string
	fileID int64
	oldRel string
	newRel string
	oldOut string
	newOut string
	newAbs string
}

// dirSyncArtifactName 产物文件名（命名形态随落点：replace 为 <页序>-<主干>，
// 其余为 <主干>-<页序>）。
func dirSyncArtifactName(target string, page int) string {
	if target == model.AssetSplitTargetReplace {
		return replaceArtifactName(page, "方案")
	}
	return fmt.Sprintf("方案-%03d.pptx", page)
}

// setupDirSyncFixture 建库 + 扫描 + 拆分记录 + 两页产物（产物写在旧落点目录）。
func setupDirSyncFixture(t *testing.T, target string) *dirSyncFixture {
	t.Helper()
	svc := newTestAssetService(t)
	root := t.TempDir()
	lib := mustCreateLibrary(t, svc, "重链落点库", root)
	oldDir := filepath.Join(root, "旧目录")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("建旧目录: %v", err)
	}
	buildAssetsPptx(t, oldDir, "方案.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"正文"}}})
	if _, err := svc.StartScan(context.Background(), lib.ID); err != nil {
		t.Fatalf("首次扫描: %v", err)
	}

	oldRel := "旧目录/方案.pptx"
	newDir := filepath.Join(root, "新目录")
	oldOut := filepath.Join(oldDir, "方案")
	// cache 落点在应用数据目录（库根外），source / replace 与源同目录同名
	if target == model.AssetSplitTargetCache {
		oldOut = filepath.Join(svc.assetDerivedDir(lib.ID), "旧目录", "方案")
	}

	store := svc.openStoreForTest(t, lib.ID)
	if err := store.upsertSplitRecord(oldRel, "h-dirsync", oldOut, 2, dirSyncSplitAt, target, false); err != nil {
		t.Fatalf("写入拆分记录: %v", err)
	}
	for page := 1; page <= 2; page++ {
		writeSplitArtifactForTest(t, filepath.Join(oldOut, dirSyncArtifactName(target, page)))
	}

	var fileID int64
	if err := store.db.QueryRow("SELECT id FROM files WHERE path = ?", oldRel).Scan(&fileID); err != nil {
		t.Fatalf("查文件记录: %v", err)
	}
	return &dirSyncFixture{
		svc: svc, lib: lib, root: root, fileID: fileID,
		oldRel: oldRel,
		newRel: "新目录/方案.pptx",
		oldOut: oldOut,
		newOut: filepath.Join(newDir, "方案"),
		newAbs: filepath.Join(newDir, "方案.pptx"),
	}
}

// moveSourceForDirSync 把源文件搬到新位置（新目录需预先存在）。
func moveSourceForDirSync(t *testing.T, f *dirSyncFixture) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.newAbs), 0o755); err != nil {
		t.Fatalf("建新目录: %v", err)
	}
	if err := os.Rename(filepath.Join(f.root, filepath.FromSlash(f.oldRel)), f.newAbs); err != nil {
		t.Fatalf("移动源文件: %v", err)
	}
}

// moveArtifactDirForDirSync 把产物容器目录整体搬到新位置（模拟用户连同源一起搬运）。
func moveArtifactDirForDirSync(t *testing.T, f *dirSyncFixture) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.newOut), 0o755); err != nil {
		t.Fatalf("建新目录父层: %v", err)
	}
	if err := os.Rename(f.oldOut, f.newOut); err != nil {
		t.Fatalf("移动产物目录: %v", err)
	}
}

// relocateForDirSync 执行重链并返回结果（失败即 Fatal）。
func relocateForDirSync(t *testing.T, f *dirSyncFixture) model.AssetRelocateResult {
	t.Helper()
	result, err := f.svc.RelocateAssetFile(f.lib.ID, f.fileID, f.newAbs, false)
	if err != nil {
		t.Fatalf("RelocateAssetFile: %v", err)
	}
	return result
}

// TestRelocateAssetFile_ArtifactDirMovedWithSource 源文件与产物容器目录一起搬走后
// 重链：source / replace 落点迁移 output_dir 到新位置，打开单页命中新位置产物，
// 重复重链幂等；cache 落点不参与判定（产物在应用数据目录，用户不会随源搬运）。
func TestRelocateAssetFile_ArtifactDirMovedWithSource(t *testing.T) {
	cases := []struct {
		name         string
		target       string
		wantMigrated bool
	}{
		{"源落点随源搬迁", model.AssetSplitTargetSource, true},
		{"替换落点随源搬迁", model.AssetSplitTargetReplace, true},
		{"缓存落点不参与判定", model.AssetSplitTargetCache, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setupDirSyncFixture(t, tc.target)
			var opened []string
			f.svc.openSplitPageFn = func(p string) error {
				opened = append(opened, p)
				return nil
			}
			if tc.wantMigrated {
				moveArtifactDirForDirSync(t, f)
			} else {
				// cache 落点：刻意在新位置放一份同形产物，验证判定面按落点类型排除；
				// 旧落点产物保留原位（打开单页仍命中旧目录）
				for page := 1; page <= 2; page++ {
					writeSplitArtifactForTest(t, filepath.Join(f.newOut, dirSyncArtifactName(tc.target, page)))
				}
			}
			moveSourceForDirSync(t, f)
			relocateForDirSync(t, f)

			wantOut := f.oldOut
			if tc.wantMigrated {
				wantOut = f.newOut
			}
			rec := splitRecordRowForTest(t, f.svc, f.lib.ID, f.newRel)
			if rec == nil {
				t.Fatal("重链后新键应命中拆分记录")
			}
			if !samePathFold(rec.outputDir, wantOut) {
				t.Errorf("output_dir 应为 %q，实际 %q", wantOut, rec.outputDir)
			}
			if rec.target != tc.target || rec.pageCount != 2 || rec.splitAt != dirSyncSplitAt {
				t.Errorf("记录其余字段应原样保留，实际 %+v", rec)
			}
			if splitRecordRowForTest(t, f.svc, f.lib.ID, f.oldRel) != nil {
				t.Error("旧键应消失（不得残留旧记录）")
			}

			// 打开单页：迁移后命中新落点产物；未迁移时仍按旧落点命中（现状回归）
			if err := f.svc.OpenAssetSplitPage(f.lib.ID, f.newRel, 1); err != nil {
				t.Fatalf("重链后打开拆分单页应成功: %v", err)
			}
			wantArtifact := filepath.Join(wantOut, dirSyncArtifactName(tc.target, 1))
			if len(opened) != 1 || !samePathFold(opened[0], wantArtifact) {
				t.Errorf("打开的产物路径应为 %q，实际 %v", wantArtifact, opened)
			}

			// 重复重链：候选落点与记录等价 → 零变更
			before := *splitRecordRowForTest(t, f.svc, f.lib.ID, f.newRel)
			relocateForDirSync(t, f)
			after := splitRecordRowForTest(t, f.svc, f.lib.ID, f.newRel)
			if after == nil || *after != before {
				t.Errorf("重复重链应幂等零变更，实际 %+v → %+v", before, after)
			}
			if n := countSplitRecordsForTest(t, f.svc, f.lib.ID); n != 1 {
				t.Errorf("记录条数应恒为 1，实际 %d", n)
			}
		})
	}
}

// TestRelocateAssetFile_ArtifactDirStaysOnSourceMoveOnly 仅搬走源文件、产物目录留在
// 原处：output_dir 保持旧值（现有行为不变），打开单页经受控回退反查旧落点仍可用。
func TestRelocateAssetFile_ArtifactDirStaysOnSourceMoveOnly(t *testing.T) {
	f := setupDirSyncFixture(t, model.AssetSplitTargetSource)
	var opened []string
	f.svc.openSplitPageFn = func(p string) error {
		opened = append(opened, p)
		return nil
	}
	moveSourceForDirSync(t, f)
	relocateForDirSync(t, f)

	rec := splitRecordRowForTest(t, f.svc, f.lib.ID, f.newRel)
	if rec == nil {
		t.Fatal("重链后新键应命中拆分记录")
	}
	if !samePathFold(rec.outputDir, f.oldOut) {
		t.Errorf("产物未搬迁时 output_dir 应保持 %q，实际 %q", f.oldOut, rec.outputDir)
	}
	if err := f.svc.OpenAssetSplitPage(f.lib.ID, f.newRel, 1); err != nil {
		t.Fatalf("产物留在旧落点时打开单页应仍成功: %v", err)
	}
	wantArtifact := filepath.Join(f.oldOut, dirSyncArtifactName(model.AssetSplitTargetSource, 1))
	if len(opened) != 1 || !samePathFold(opened[0], wantArtifact) {
		t.Errorf("打开的产物路径应为 %q，实际 %v", wantArtifact, opened)
	}
}

// TestRelocateAssetFile_ArtifactDirShapeMismatchNotMigrated 新位置存在同名目录但
// 不含合法产物形态（空目录 / 同名异构文件 / 随机文件）时不得误迁移。
func TestRelocateAssetFile_ArtifactDirShapeMismatchNotMigrated(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, f *dirSyncFixture)
	}{
		{"同名目录为空", func(t *testing.T, f *dirSyncFixture) {
			t.Helper()
			if err := os.MkdirAll(f.newOut, 0o755); err != nil {
				t.Fatalf("建空目录: %v", err)
			}
		}},
		{"仅同名异构文件与随机文件", func(t *testing.T, f *dirSyncFixture) {
			t.Helper()
			writeSplitArtifactForTest(t, filepath.Join(f.newOut, "方案-草稿.pptx"))
			writeSplitArtifactForTest(t, filepath.Join(f.newOut, "readme.txt"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := setupDirSyncFixture(t, model.AssetSplitTargetSource)
			tc.setup(t, f)
			moveSourceForDirSync(t, f)
			relocateForDirSync(t, f)

			rec := splitRecordRowForTest(t, f.svc, f.lib.ID, f.newRel)
			if rec == nil {
				t.Fatal("重链后新键应命中拆分记录")
			}
			if !samePathFold(rec.outputDir, f.oldOut) {
				t.Errorf("产物形态不匹配时不得迁移落点，应为 %q，实际 %q", f.oldOut, rec.outputDir)
			}
		})
	}
}

// TestResolveRelocatedSplitOutDir_EmptyStemRejected 空主干源名（文件名恰为 .pptx）
// 的候选目录会退化为源所在目录本身，目录内任何页序产物都可能被误认成容器目录——
// 必须直接拒绝，与拆分入口的空主干校验同口径。
func TestResolveRelocatedSplitOutDir_EmptyStemRejected(t *testing.T) {
	root := t.TempDir()
	// 源所在目录内放一个页序形状文件，验证候选退化时不被误判
	writeSplitArtifactForTest(t, filepath.Join(root, "其他-001.pptx"))
	rec := &assetSplitRecord{target: model.AssetSplitTargetSource, outputDir: filepath.Join(root, "别的落点")}
	if got := resolveRelocatedSplitOutDir(rec, filepath.Join(root, ".pptx"), ".pptx"); got != "" {
		t.Errorf("空主干源名不得迁移落点，实际返回 %q", got)
	}
}

// TestResolveRelocatedSplitOutDir_DegenerateInputs 搬迁探测纯函数边界：每类退化输入
// 都必须落在安全侧（返回空串 = 保持原落点），不得误迁移。
func TestResolveRelocatedSplitOutDir_DegenerateInputs(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T, root string) (*assetSplitRecord, string, string)
		wantHit bool
	}{
		{"无拆分记录", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			return nil, filepath.Join(root, "方案.pptx"), "方案.pptx"
		}, false},
		{"缓存落点不参与判定", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			writeSplitArtifactForTest(t, filepath.Join(root, "方案", "方案-001.pptx"))
			return &assetSplitRecord{target: model.AssetSplitTargetCache, outputDir: filepath.Join(root, "别的落点")},
				filepath.Join(root, "方案.pptx"), "方案.pptx"
		}, false},
		{"主干退化为点", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			return &assetSplitRecord{target: model.AssetSplitTargetSource, outputDir: filepath.Join(root, "别的落点")},
				filepath.Join(root, "..pptx"), "..pptx"
		}, false},
		{"主干退化为双点", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			return &assetSplitRecord{target: model.AssetSplitTargetSource, outputDir: filepath.Join(root, "别的落点")},
				filepath.Join(root, "...pptx"), "...pptx"
		}, false},
		{"候选不存在", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			return &assetSplitRecord{target: model.AssetSplitTargetSource, outputDir: filepath.Join(root, "别的落点")},
				filepath.Join(root, "方案.pptx"), "方案.pptx"
		}, false},
		{"候选是普通文件而非目录", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			if err := os.WriteFile(filepath.Join(root, "方案"), []byte("x"), 0o644); err != nil {
				t.Fatalf("写同名文件: %v", err)
			}
			return &assetSplitRecord{target: model.AssetSplitTargetSource, outputDir: filepath.Join(root, "别的落点")},
				filepath.Join(root, "方案.pptx"), "方案.pptx"
		}, false},
		{"候选与原落点等价", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			writeSplitArtifactForTest(t, filepath.Join(root, "方案", "方案-001.pptx"))
			return &assetSplitRecord{target: model.AssetSplitTargetSource, outputDir: filepath.Join(root, "方案")},
				filepath.Join(root, "方案.pptx"), "方案.pptx"
		}, false},
		{"首页产物缺失", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			writeSplitArtifactForTest(t, filepath.Join(root, "方案", "方案-002.pptx"))
			return &assetSplitRecord{target: model.AssetSplitTargetSource, outputDir: filepath.Join(root, "别的落点")},
				filepath.Join(root, "方案.pptx"), "方案.pptx"
		}, false},
		{"同页序多候选不可判定", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			writeSplitArtifactForTest(t, filepath.Join(root, "方案", "方案-001.pptx"))
			writeSplitArtifactForTest(t, filepath.Join(root, "方案", "其他-001.pptx"))
			return &assetSplitRecord{target: model.AssetSplitTargetSource, outputDir: filepath.Join(root, "别的落点")},
				filepath.Join(root, "方案.pptx"), "方案.pptx"
		}, false},
		{"源落点首页命中", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			writeSplitArtifactForTest(t, filepath.Join(root, "方案", "方案-001.pptx"))
			return &assetSplitRecord{target: model.AssetSplitTargetSource, outputDir: filepath.Join(root, "别的落点")},
				filepath.Join(root, "方案.pptx"), "方案.pptx"
		}, true},
		{"替换落点首页命中", func(t *testing.T, root string) (*assetSplitRecord, string, string) {
			writeSplitArtifactForTest(t, filepath.Join(root, "方案", replaceArtifactName(1, "方案")))
			return &assetSplitRecord{target: model.AssetSplitTargetReplace, outputDir: filepath.Join(root, "别的落点")},
				filepath.Join(root, "方案.pptx"), "方案.pptx"
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			rec, newAbs, newRel := tc.setup(t, root)
			got := resolveRelocatedSplitOutDir(rec, newAbs, newRel)
			if tc.wantHit {
				if !samePathFold(got, filepath.Join(root, "方案")) {
					t.Errorf("应命中候选 %q，实际返回 %q", filepath.Join(root, "方案"), got)
				}
				return
			}
			if got != "" {
				t.Errorf("退化输入应返回空串（保持原落点），实际返回 %q", got)
			}
		})
	}
}

// TestRelocateAssetFile_ArtifactDirMigrationTxRollback 拆分记录写失败时 files.path
// 与 output_dir 均保持原值（落点改写与路径改写同一事务，无半态）。
func TestRelocateAssetFile_ArtifactDirMigrationTxRollback(t *testing.T) {
	f := setupDirSyncFixture(t, model.AssetSplitTargetSource)
	moveArtifactDirForDirSync(t, f)
	moveSourceForDirSync(t, f)
	store := f.svc.openStoreForTest(t, f.lib.ID)
	// 触发器注入写失败：split_records 任一 UPDATE 中止事务
	if _, err := store.db.Exec(
		`CREATE TRIGGER block_split_dirsync BEFORE UPDATE ON split_records BEGIN SELECT RAISE(ABORT, 'blocked'); END;`,
	); err != nil {
		t.Fatalf("创建触发器: %v", err)
	}

	if _, err := f.svc.RelocateAssetFile(f.lib.ID, f.fileID, f.newAbs, false); err == nil {
		t.Fatal("拆分记录写失败时重链应报错")
	}

	var path string
	if err := store.db.QueryRow("SELECT path FROM files WHERE id = ?", f.fileID).Scan(&path); err != nil {
		t.Fatalf("读文件路径: %v", err)
	}
	if path != f.oldRel {
		t.Errorf("事务回滚后 files.path 应保持原值 %q，实际 %q", f.oldRel, path)
	}
	rec := splitRecordRowForTest(t, f.svc, f.lib.ID, f.oldRel)
	if rec == nil || !samePathFold(rec.outputDir, f.oldOut) {
		t.Errorf("事务回滚后记录应保持原键与原落点，实际 %+v", rec)
	}
	if splitRecordRowForTest(t, f.svc, f.lib.ID, f.newRel) != nil {
		t.Error("事务回滚后不得出现新键记录")
	}
}
