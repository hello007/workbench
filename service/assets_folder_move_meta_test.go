package service

import (
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// 目录级移动（MoveAssetFolder，PRD R5/D，PR4 统一核心）与文件夹级标注
// （GetAssetFolderMeta/UpdateAssetFolderMeta，PRD R8）用例。

func TestAssetFolderMove_SuccessAndKeys(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "目录移动库", root)
	mustScan(t, svc, lib.ID)

	// 预置源目录标注（folder_meta + folder_tags），验证键同事务级联迁移
	if err := svc.openStoreForTest(t, lib.ID).updateFolderMeta("media", "素材目录备注", []string{"重点"}, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta: %v", err)
	}

	result, err := svc.MoveAssetFolder(lib.ID, "media", "归档")
	if err != nil {
		t.Fatalf("MoveAssetFolder: %v", err)
	}
	if result.OldPath != "media" || result.NewPath != "归档/media" {
		t.Fatalf("移动结果路径口径应 old=media new=归档/media，实际 %s→%s", result.OldPath, result.NewPath)
	}

	// 磁盘面：目录落位（自身名称保持）
	if _, err := os.Stat(filepath.Join(root, "归档", "media", "a.pptx")); err != nil {
		t.Fatalf("目标目录应含移动后的子目录与文件")
	}
	// 索引面：path 前缀更新
	entries, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	statusByPath := map[string]string{}
	for _, e := range entries {
		statusByPath[e.Path] = e.Status
	}
	if statusByPath["归档/media/a.pptx"] != model.AssetFileStatusOK {
		t.Fatalf("移动后索引路径应更新且保持 ok：%v", statusByPath)
	}

	// 标注键同事务级联迁移：旧键清空、新键可查且内容原样
	note, _, tags, err := svc.openStoreForTest(t, lib.ID).getFolderMeta("归档/media")
	if err != nil {
		t.Fatalf("getFolderMeta(新键): %v", err)
	}
	if note != "素材目录备注" || len(tags) != 1 || tags[0].Name != "重点" {
		t.Errorf("标注应随键迁移且内容原样，实际 note=%q tags=%+v", note, tags)
	}
	if n, _, _, _ := svc.openStoreForTest(t, lib.ID).getFolderMeta("media"); n != "" {
		t.Errorf("旧键标注不应残留")
	}
}

func TestAssetFolderMove_Refusals(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "stem", "stem-001.pptx")) // 派生容器
	if err := os.MkdirAll(filepath.Join(root, "secret", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "目录移动拒绝库", root)
	if err := svc.setLibraryRules(lib.ID, []string{"secret"}, nil); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}

	// 目标位于源目录自身子树内拒绝（rename 进后代）
	if _, err := svc.MoveAssetFolder(lib.ID, "media", "media/sub"); err == nil {
		t.Errorf("移入自身子树应拒绝")
	}
	// 源目标相同拒绝（防误触发）
	if _, err := svc.MoveAssetFolder(lib.ID, "media", ""); err == nil {
		t.Errorf("移入当前位置应拒绝")
	}
	// 目标为排除命中目录拒绝
	if _, err := svc.MoveAssetFolder(lib.ID, "media", "secret"); err == nil {
		t.Errorf("目标为排除命中目录应拒绝")
	}
	// 目标为派生容器拒绝
	if _, err := svc.MoveAssetFolder(lib.ID, "media", "stem"); err == nil {
		t.Errorf("目标为派生容器应拒绝")
	}
	// 源为派生容器拒绝
	if _, err := svc.MoveAssetFolder(lib.ID, "stem", "media"); err == nil {
		t.Errorf("源为派生容器应拒绝")
	}
	// 源含拆分记录拒绝（与 PR2 重命名同口径）
	if err := svc.openStoreForTest(t, lib.ID).upsertSplitRecord("media/a.pptx", "hash", "D:/derived/media/a", 2, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}
	if _, err := svc.MoveAssetFolder(lib.ID, "media", ""); err == nil {
		t.Errorf("源含拆分记录应拒绝")
	}
	// 库根拒绝
	if _, err := svc.MoveAssetFolder(lib.ID, "", "media"); err == nil {
		t.Errorf("库根移动应拒绝")
	}
}

func TestAssetFolderMeta_CRUD(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "secret"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "文件夹标注库", root)

	// 未标注目录：零值形态（Tags 恒非 nil 空切片）
	meta, err := svc.GetAssetFolderMeta(lib.ID, "media")
	if err != nil {
		t.Fatalf("GetAssetFolderMeta: %v", err)
	}
	if meta.Note != "" || len(meta.Tags) != 0 || meta.Tags == nil {
		t.Errorf("未标注目录应返回零值形态（Tags 非 nil 空切片），实际 %+v", meta)
	}

	// 保存：不存在的标签自动创建（ID 口径与文件/页标签同源）
	if err := svc.UpdateAssetFolderMeta(lib.ID, "media", model.AssetFolderMetaUpdate{
		Note:     "渠道素材目录",
		TagNames: []string{"重点", "渠道"},
	}); err != nil {
		t.Fatalf("UpdateAssetFolderMeta: %v", err)
	}
	meta, err = svc.GetAssetFolderMeta(lib.ID, "media")
	if err != nil {
		t.Fatalf("GetAssetFolderMeta: %v", err)
	}
	if meta.Note != "渠道素材目录" || len(meta.Tags) != 2 {
		t.Fatalf("标注应保存回读一致，实际 %+v", meta)
	}
	// 标签本体落 tags 表（与文件标签同表）
	assetTags, err := svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	names := map[string]bool{}
	for _, tag := range assetTags {
		names[tag.Name] = true
	}
	if !names["重点"] || !names["渠道"] {
		t.Errorf("文件夹标签应复用 tags 表，实际 %v", names)
	}

	// 整体替换：标签集以本次提交为准（未提及即移除）
	if err := svc.UpdateAssetFolderMeta(lib.ID, "media", model.AssetFolderMetaUpdate{Note: "更新备注", TagNames: []string{"重点"}}); err != nil {
		t.Fatalf("UpdateAssetFolderMeta(替换): %v", err)
	}
	meta, err = svc.GetAssetFolderMeta(lib.ID, "media")
	if err != nil {
		t.Fatalf("GetAssetFolderMeta: %v", err)
	}
	if meta.Note != "更新备注" || len(meta.Tags) != 1 || meta.Tags[0].Name != "重点" {
		t.Errorf("整体替换语义应生效，实际 %+v", meta)
	}

	// 禁标注守卫：库根 / 排除命中目录 / 派生容器
	if err := svc.UpdateAssetFolderMeta(lib.ID, "", model.AssetFolderMetaUpdate{Note: "根"}); err == nil {
		t.Errorf("库根标注应拒绝")
	}
	if err := svc.setLibraryRules(lib.ID, []string{"secret"}, nil); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}
	if _, err := svc.GetAssetFolderMeta(lib.ID, "secret"); err == nil {
		t.Errorf("排除命中目录读取标注应拒绝")
	}
	if err := svc.UpdateAssetFolderMeta(lib.ID, "secret", model.AssetFolderMetaUpdate{Note: "x"}); err == nil {
		t.Errorf("排除命中目录标注应拒绝")
	}
}

func TestAssetFolderDelete_CascadesFolderMeta(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "media", "sub"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "删除级联标注库", root)
	// 父子目录均预置标注
	if err := svc.openStoreForTest(t, lib.ID).updateFolderMeta("media", "父目录备注", nil, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta: %v", err)
	}
	if err := svc.openStoreForTest(t, lib.ID).updateFolderMeta("media/sub", "子目录备注", nil, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta: %v", err)
	}

	if _, err := svc.DeleteAssetFolder(lib.ID, "media", true); err != nil {
		t.Fatalf("DeleteAssetFolder: %v", err)
	}
	// 父子键一并级联清理（前缀边界语义，孤儿键不残留）
	if n, _, _, _ := svc.openStoreForTest(t, lib.ID).getFolderMeta("media"); n != "" {
		t.Errorf("父目录标注应级联删除")
	}
	if n, _, _, _ := svc.openStoreForTest(t, lib.ID).getFolderMeta("media/sub"); n != "" {
		t.Errorf("子目录标注应级联删除（前缀语义）")
	}
	// 标签本体不受影响（删除目录不删标签，SRS「删除标签不删素材」反向对称）
	if _, err := svc.ListTags(lib.ID); err != nil {
		t.Fatalf("ListTags: %v", err)
	}
}

// 回归（check 阶段修复）：replaceFilePaths 文件夹键级联的 substr 偏移曾用 Go
// len()（字节数），SQLite substr 对 TEXT 按字符计位——旧目录名含非 ASCII（中文，
// 字节长 > 字符长）时偏移错位致新键截断（「素材/x」→「backup/」丢失后缀）。
// 用中文旧目录名守护字符口径。
func TestAssetFolderMove_NonASCIIKeyCascade(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "素材", "a.pptx"))
	lib := mustCreateLibrary(t, svc, "中文目录键迁移库", root)
	if err := svc.openStoreForTest(t, lib.ID).updateFolderMeta("素材", "中文目录备注", []string{"重点"}, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta: %v", err)
	}

	if _, err := svc.RenameAssetFolder(lib.ID, "素材", "backup"); err != nil {
		t.Fatalf("RenameAssetFolder: %v", err)
	}
	note, _, tags, err := svc.openStoreForTest(t, lib.ID).getFolderMeta("backup")
	if err != nil {
		t.Fatalf("getFolderMeta(新键): %v", err)
	}
	if note != "中文目录备注" || len(tags) != 1 || tags[0].Name != "重点" {
		t.Errorf("中文旧目录名的标注键应按字符口径迁移且内容原样，实际 note=%q tags=%+v", note, tags)
	}
	if n, _, _, _ := svc.openStoreForTest(t, lib.ID).getFolderMeta("素材"); n != "" {
		t.Errorf("旧键标注不应残留")
	}
}

// 回归（check 阶段修复）：moveAssetFolderCore 曾在 records 为空（无 pptx 索引面）
// 时提前 return，跳过 folder_meta/folder_tags 键级联——空目录/仅非 PPT 文件的目录
// 同样可标注，重命名后标注成旧路径孤儿键（静默丢失）。
func TestAssetFolderMove_EmptyFolderKeyCascade(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "空目录"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "空目录键迁移库", root)
	if err := svc.openStoreForTest(t, lib.ID).updateFolderMeta("空目录", "空目录备注", nil, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("updateFolderMeta: %v", err)
	}

	if _, err := svc.RenameAssetFolder(lib.ID, "空目录", "已归档"); err != nil {
		t.Fatalf("RenameAssetFolder: %v", err)
	}
	note, _, _, err := svc.openStoreForTest(t, lib.ID).getFolderMeta("已归档")
	if err != nil {
		t.Fatalf("getFolderMeta(新键): %v", err)
	}
	if note != "空目录备注" {
		t.Errorf("空目录的标注键应随重命名迁移，实际新键 note=%q", note)
	}
	if n, _, _, _ := svc.openStoreForTest(t, lib.ID).getFolderMeta("空目录"); n != "" {
		t.Errorf("旧键标注不应残留")
	}
}
