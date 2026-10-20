package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
)

// 文件夹管理操作（CreateAssetFolder / DeleteAssetFolder / RenameAssetFolder，
// PRD「素材管理：文件夹树与拖拽整理」PR2）用例。夹具 = 临时目录 + 伪 pptx 文件
// （解析经 parseFn 注入 fake，禁依赖本机 Office 与真实文件系统时序，
// docs/spec/test-stability.md）；写索引的删除/重命名用例经真实扫描链路（注入
// fake 解析）驱动状态收敛。

// okParseForAll 全部解析成功的 fake（单页标题 = 文件名主干）：驱动文件入库 ok
// 且有解析产物（fileHasSlides 资格满足，复活链路可达）。
func okParseForAll(srcPath, baseName string) ([]parsedSlide, error) {
	return []parsedSlide{{Num: 1, Title: baseName, Text: baseName + " 正文"}}, nil
}

// mustScan 驱动一次全量扫描并断言成功（夹具收敛入口）。
func mustScan(t *testing.T, svc *AssetLibraryService, libraryID string) model.AssetScanResult {
	t.Helper()
	result, err := svc.StartScan(context.Background(), libraryID)
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	return result
}

// mustWritePptx 落盘伪 pptx 文件（内容无关紧要——解析经 fake 注入）。
func mustWritePptxFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("pptx"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

// ---------- 新建子目录（CreateAssetFolder） ----------

func TestAssetFolderCreate(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "方案A"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "新建目录库", root)

	// 根目录下新建
	node, err := svc.CreateAssetFolder(lib.ID, "", "新目录")
	if err != nil {
		t.Fatalf("CreateAssetFolder(root): %v", err)
	}
	if node.Path != "新目录" || node.Name != "新目录" {
		t.Errorf("返回节点 path/name 应为新目录，实际 %s/%s", node.Path, node.Name)
	}
	if info, err := os.Stat(filepath.Join(root, "新目录")); err != nil || !info.IsDir() {
		t.Fatalf("磁盘目录应已创建")
	}

	// 子目录下新建（父路径多段）
	if _, err := svc.CreateAssetFolder(lib.ID, "方案A", "子级"); err != nil {
		t.Fatalf("CreateAssetFolder(方案A): %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "方案A", "子级")); err != nil || !info.IsDir() {
		t.Fatalf("子级目录应已创建")
	}

	// 重名拒绝（os.ErrExist 口径明确报错）
	if _, err := svc.CreateAssetFolder(lib.ID, "", "新目录"); err == nil {
		t.Errorf("重名应拒绝")
	}

	// 父目录不存在拒绝
	if _, err := svc.CreateAssetFolder(lib.ID, "不存在的父", "子"); err == nil {
		t.Errorf("父目录不存在应拒绝")
	}

	// 父为排除命中目录拒绝
	if err := svc.setLibraryRules(lib.ID, []string{"secret"}, nil); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "secret"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if _, err := svc.CreateAssetFolder(lib.ID, "secret", "子"); err == nil {
		t.Errorf("父为排除命中目录应拒绝")
	}

	// 名称非法（含路径分隔符 / 点形态）拒绝
	for _, bad := range []string{"a/b", `a\b`, "..", "."} {
		if _, err := svc.CreateAssetFolder(lib.ID, "", bad); err == nil {
			t.Errorf("非法名 %q 应拒绝", bad)
		}
	}
}

// ---------- 删除目录（DeleteAssetFolder） ----------

func TestAssetFolderDelete_ForceAndIndexCollapse(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "media", "sub", "b.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "empty", "占位.tmp")) // 非空目录（仅非 pptx 条目）
	if err := os.Mkdir(filepath.Join(root, "真空目录"), 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "删除目录库", root)
	mustScan(t, svc, lib.ID)

	// 空目录 force=false 直接删除（PRD R3：空目录删除无需确认）
	lost, err := svc.DeleteAssetFolder(lib.ID, "真空目录", false)
	if err != nil {
		t.Fatalf("DeleteAssetFolder(空目录): %v", err)
	}
	if lost != 0 {
		t.Errorf("空目录删除 lost 应 0，实际 %d", lost)
	}

	// 非空目录 force=false 拒绝（E_ASSETS_FOLDER_NOT_EMPTY 确认型，删除未发生）
	_, err = svc.DeleteAssetFolder(lib.ID, "empty", false)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeAssetsFolderNotEmpty {
		t.Fatalf("非空目录 force=false 应返回 E_ASSETS_FOLDER_NOT_EMPTY，实际：%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "empty")); err != nil {
		t.Fatalf("确认型拒绝不应删除磁盘目录")
	}
	if _, err := svc.DeleteAssetFolder(lib.ID, "empty", true); err != nil {
		t.Fatalf("DeleteAssetFolder(force=true): %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "empty")); !os.IsNotExist(err) {
		t.Fatalf("force=true 应删除磁盘目录")
	}

	// media 递归删除 + lost 收口
	lost, err = svc.DeleteAssetFolder(lib.ID, "media", true)
	if err != nil {
		t.Fatalf("DeleteAssetFolder(media): %v", err)
	}
	if lost != 2 {
		t.Errorf("media 删除应标记 2 条 lost，实际 %d", lost)
	}
	if _, err := os.Stat(filepath.Join(root, "media")); !os.IsNotExist(err) {
		t.Fatalf("media 应已删除")
	}

	// 索引收口：media 前缀下文件行全部翻 lost（含子目录），标注语义保留
	entries, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, e := range entries {
		if e.Path == "media/a.pptx" || e.Path == "media/sub/b.pptx" {
			if e.Status != model.AssetFileStatusLost {
				t.Errorf("删除目录内文件 %s 应标 lost，实际 %s", e.Path, e.Status)
			}
		}
	}
}

func TestAssetFolderDelete_Refusals(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	// 拆分派生容器：media/media-001.pptx（同名文件夹布局）
	mustWritePptxFile(t, filepath.Join(root, "media", "media-001.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "secret"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "删除拒绝库", root)
	if err := svc.setLibraryRules(lib.ID, []string{"secret"}, nil); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}

	// 拆分派生容器拒绝（派生产物由拆分链路管理）
	if _, err := svc.DeleteAssetFolder(lib.ID, "media", true); err == nil {
		t.Errorf("拆分派生容器删除应拒绝")
	}
	// 排除命中目录拒绝
	if _, err := svc.DeleteAssetFolder(lib.ID, "secret", true); err == nil {
		t.Errorf("排除命中目录删除应拒绝")
	}
	// 库根拒绝
	if _, err := svc.DeleteAssetFolder(lib.ID, "", true); err == nil {
		t.Errorf("库根删除应拒绝")
	}
}

func TestAssetFolderDelete_SplitRecordsCollapsed(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "media", "b.pptx"))
	lib := mustCreateLibrary(t, svc, "拆分记录清理库", root)
	mustScan(t, svc, lib.ID)

	// 直写拆分记录（media/a.pptx 命中前缀、other/c.pptx 不命中）
	if err := svc.openStoreForTest(t, lib.ID).upsertSplitRecord("media/a.pptx", "hash-a", "D:/derived/a", 3, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}
	if err := svc.openStoreForTest(t, lib.ID).upsertSplitRecord("other/c.pptx", "hash-c", "D:/derived/c", 2, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}

	if _, err := svc.DeleteAssetFolder(lib.ID, "media", true); err != nil {
		t.Fatalf("DeleteAssetFolder: %v", err)
	}
	store := svc.openStoreForTest(t, lib.ID)
	if rec, _ := store.getSplitRecord("media/a.pptx"); rec != nil {
		t.Errorf("删除目录内源的拆分记录应清理")
	}
	if rec, _ := store.getSplitRecord("other/c.pptx"); rec == nil {
		t.Errorf("前缀外的拆分记录不应被波及")
	}
}

// ---------- 重命名目录（RenameAssetFolder） ----------

func TestAssetFolderRename_IndexSyncAndFts(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "media", "sub", "b.pptx"))
	lib := mustCreateLibrary(t, svc, "重命名库", root)
	mustScan(t, svc, lib.ID)

	// 重命名前关键词命中（FTS 文件行 = 标题 baseName）
	page, err := svc.SearchAssets(lib.ID, "a", model.AssetSearchOptions{Flatten: true, Limit: 50})
	if err != nil {
		t.Fatalf("SearchAssets: %v", err)
	}
	if page.Total == 0 {
		t.Fatalf("重命名前应能检索命中")
	}

	result, err := svc.RenameAssetFolder(lib.ID, "media", "素材")
	if err != nil {
		t.Fatalf("RenameAssetFolder: %v", err)
	}
	if result.Replaced != 2 {
		t.Errorf("索引路径同步应 2 条（含子目录文件），实际 %d", result.Replaced)
	}
	if result.OldPath != "media" || result.NewPath != "素材" {
		t.Errorf("结果路径口径应 old=media new=素材，实际 %s→%s", result.OldPath, result.NewPath)
	}

	// 磁盘面：目录与新路径就位
	if _, err := os.Stat(filepath.Join(root, "media")); !os.IsNotExist(err) {
		t.Fatalf("旧目录应已不存在")
	}
	if _, err := os.Stat(filepath.Join(root, "素材", "a.pptx")); err != nil {
		t.Fatalf("新目录应含原文件")
	}

	// 索引面：path 前缀更新 + 状态保持 ok
	entries, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	paths := map[string]string{}
	for _, e := range entries {
		paths[e.Path] = e.Status
	}
	if paths["素材/a.pptx"] != model.AssetFileStatusOK || paths["素材/sub/b.pptx"] != model.AssetFileStatusOK {
		t.Fatalf("重命名后索引路径应更新且保持 ok：%v", paths)
	}

	// FTS 命中不丢（rowid = files.id 稳定 + FTS 检索列不含路径，纯路径重命名零重建）
	page, err = svc.SearchAssets(lib.ID, "a", model.AssetSearchOptions{Flatten: true, Limit: 50})
	if err != nil {
		t.Fatalf("SearchAssets(重命名后): %v", err)
	}
	if page.Total == 0 {
		t.Errorf("重命名后关键词检索应仍然命中（FTS 行不丢）")
	}
}

func TestAssetFolderRename_ExclusionFlips(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	// 单目录双文件夹具：目标排除名「归档」在磁盘上不存在（rename 目标重名预检会
	// 拒绝已存在的磁盘目录，排除翻转用例需目标名空闲）
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "media", "b.pptx"))
	lib := mustCreateLibrary(t, svc, "排除翻转库", root)
	mustScan(t, svc, lib.ID)
	// 规则后置：先把文件入库 ok（有解析产物），再配置规则命中「归档」
	if err := svc.setLibraryRules(lib.ID, []string{"归档"}, nil); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}

	// 正向：重命名进排除名 → 翻 excluded
	if _, err := svc.RenameAssetFolder(lib.ID, "media", "归档"); err != nil {
		t.Fatalf("RenameAssetFolder(media→归档): %v", err)
	}
	entries, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	statusByPath := map[string]string{}
	for _, e := range entries {
		statusByPath[e.Path] = e.Status
	}
	if statusByPath["归档/a.pptx"] != model.AssetFileStatusExcluded || statusByPath["归档/b.pptx"] != model.AssetFileStatusExcluded {
		t.Errorf("重命名进排除名后文件应翻 excluded：%v", statusByPath)
	}

	// 反向：改宽（重命名出排除名）→ excluded 经 reviveLostFile 复活 ok（FTS 同事务重建）
	if _, err := svc.RenameAssetFolder(lib.ID, "归档", "公开"); err != nil {
		t.Fatalf("RenameAssetFolder(归档→公开): %v", err)
	}
	entries, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	statusByPath = map[string]string{}
	for _, e := range entries {
		statusByPath[e.Path] = e.Status
	}
	if statusByPath["公开/a.pptx"] != model.AssetFileStatusOK || statusByPath["公开/b.pptx"] != model.AssetFileStatusOK {
		t.Errorf("解除排除且产物在的文件应复活 ok：%v", statusByPath)
	}
	// 复活后关键词检索可命中（FTS 重建生效）
	page, err := svc.SearchAssets(lib.ID, "b", model.AssetSearchOptions{Flatten: true, Limit: 50})
	if err != nil {
		t.Fatalf("SearchAssets: %v", err)
	}
	if page.Total == 0 {
		t.Errorf("复活后关键词检索应命中（FTS 同事务重建）")
	}
}

func TestAssetFolderRename_Refusals(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "media-001.pptx")) // 派生容器
	mustWritePptxFile(t, filepath.Join(root, "withsplit", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "占用"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "重命名拒绝库", root)

	// 拆分记录守卫（宁严勿错）：子树内存在拆分记录拒绝重命名
	if err := svc.openStoreForTest(t, lib.ID).upsertSplitRecord("withsplit/a.pptx", "hash", "D:/derived/withsplit/a", 2, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}
	if _, err := svc.RenameAssetFolder(lib.ID, "withsplit", "改名"); err == nil {
		t.Errorf("子树含拆分记录的重命名应拒绝")
	}

	// 拆分派生容器拒绝
	if _, err := svc.RenameAssetFolder(lib.ID, "media", "改名"); err == nil {
		t.Errorf("拆分派生容器重命名应拒绝")
	}

	// 磁盘重名拒绝（os.ErrExist 口径）
	if err := os.MkdirAll(filepath.Join(root, "普通"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if _, err := svc.RenameAssetFolder(lib.ID, "普通", "占用"); err == nil {
		t.Errorf("目标重名应拒绝")
	}

	// 新名非法拒绝
	for _, bad := range []string{"a/b", ".."} {
		if _, err := svc.RenameAssetFolder(lib.ID, "普通", bad); err == nil {
			t.Errorf("非法名 %q 应拒绝", bad)
		}
	}

	// 同名 no-op 拒绝
	if _, err := svc.RenameAssetFolder(lib.ID, "普通", "普通"); err == nil {
		t.Errorf("同名重命名应拒绝")
	}
}

// ---------- 树标记联动（PR2：Derived 标记） ----------

func TestAssetListFolders_DerivedContainerMarked(t *testing.T) {
	svc := newTestAssetService(t)
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "media-001.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "normal", "a.pptx"))
	lib := mustCreateLibrary(t, svc, "派生标记库", root)

	nodes, err := svc.ListFolders(lib.ID)
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	media := mustFindFolder(nodes, "media")
	if media == nil || !media.Derived {
		t.Errorf("派生容器应带 Derived 标记：%+v", media)
	}
	if media.FileCount != 0 {
		t.Errorf("派生副本不应计入 FileCount，实际 %d", media.FileCount)
	}
	normal := mustFindFolder(nodes, "normal")
	if normal == nil || normal.Derived {
		t.Errorf("普通目录不应带 Derived 标记")
	}
	if normal.FileCount != 1 {
		t.Errorf("normal FileCount 应 1，实际 %d", normal.FileCount)
	}
}
