package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// 文件批量移动（MoveAssetFiles，PRD R4/R6/R9，PR3 拖拽移动后端入口）用例。
// 夹具同文件夹管理用例：临时目录 + 伪 pptx + parseFn 注入 fake（禁环境外部状态）。

func TestAssetMoveFiles_Success(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "media", "b.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动成功库", root)
	mustScan(t, svc, lib.ID)
	entries, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	idByPath := map[string]int64{}
	for _, e := range entries {
		idByPath[e.Path] = e.ID
	}

	result, err := svc.MoveAssetFiles(lib.ID, []int64{idByPath["media/a.pptx"], idByPath["media/b.pptx"]}, "归档")
	if err != nil {
		t.Fatalf("MoveAssetFiles: %v", err)
	}
	if result.Succeeded != 2 || result.Failed != 0 || result.Total != 2 {
		t.Fatalf("移动应全部成功（total=2 succeeded=2 failed=0），实际 %+v", result)
	}

	// 磁盘面：文件落位新目录
	if _, err := os.Stat(filepath.Join(root, "归档", "a.pptx")); err != nil {
		t.Fatalf("目标目录应含移动后的文件")
	}
	if _, err := os.Stat(filepath.Join(root, "media", "a.pptx")); !os.IsNotExist(err) {
		t.Fatalf("旧位置文件应已移走")
	}
	// 索引面：path 更新 + 状态保持 ok
	entries, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	statusByPath := map[string]string{}
	for _, e := range entries {
		statusByPath[e.Path] = e.Status
	}
	if statusByPath["归档/a.pptx"] != model.AssetFileStatusOK || statusByPath["归档/b.pptx"] != model.AssetFileStatusOK {
		t.Fatalf("移动后索引路径应更新且保持 ok：%v", statusByPath)
	}

	// FTS 命中新路径语境（检索列不含路径，rowid 稳定——文件 title 基名不变应仍命中）
	page, err := svc.SearchAssets(lib.ID, "a", model.AssetSearchOptions{Flatten: true, Limit: 50})
	if err != nil {
		t.Fatalf("SearchAssets: %v", err)
	}
	if page.Total == 0 {
		t.Errorf("移动后关键词检索应仍命中（FTS 行不丢）")
	}

	// 库根移动（targetFolder 空串 = 库根，根「全部素材」drop 语义）
	result, err = svc.MoveAssetFiles(lib.ID, []int64{idByPath["media/b.pptx"]}, "")
	if err != nil {
		t.Fatalf("MoveAssetFiles(库根): %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("库根移动应成功，实际 %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "b.pptx")); err != nil {
		t.Fatalf("库根应含移动后的文件")
	}
}

func TestAssetMoveFiles_Refusals(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "dup", "a.pptx"))         // 目标内重名夹具（同名文件）
	mustWritePptxFile(t, filepath.Join(root, "stem", "stem-001.pptx")) // 派生容器夹具（拆分派生布局）
	if err := os.MkdirAll(filepath.Join(root, "secret"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动拒绝库", root)
	mustScan(t, svc, lib.ID)
	entries, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	idByPath := map[string]int64{}
	for _, e := range entries {
		idByPath[e.Path] = e.ID
	}

	// 目标内重名拒绝（dup/a.pptx 已在磁盘，移入同名文件拒绝）
	result, err := svc.MoveAssetFiles(lib.ID, []int64{idByPath["media/a.pptx"]}, "dup")
	if err != nil {
		t.Fatalf("MoveAssetFiles（目标整体校验通过，条目级重名）: %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("目标内重名应记 failed 明细，实际 %+v", result)
	}
	if result.Items[0].Message == "" {
		t.Errorf("失败明细应带可读原因")
	}

	// 目标为排除命中目录整体拒绝
	if err := svc.setLibraryRules(lib.ID, []string{"secret"}, nil); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}
	if _, err := svc.MoveAssetFiles(lib.ID, []int64{idByPath["media/a.pptx"]}, "secret"); err == nil {
		t.Errorf("目标为排除命中目录应整体拒绝")
	}

	// 目标为拆分派生容器整体拒绝
	if _, err := svc.MoveAssetFiles(lib.ID, []int64{idByPath["media/a.pptx"]}, "stem"); err == nil {
		t.Errorf("目标为派生容器应拒绝")
	}
	// 目标不存在整体拒绝
	if _, err := svc.MoveAssetFiles(lib.ID, []int64{idByPath["media/a.pptx"]}, "不存在的目录"); err == nil {
		t.Errorf("目标不存在应整体拒绝")
	}
}

func TestAssetMoveFiles_ExclusionFlips(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "归档", "a.pptx"))
	mustWritePptxFile(t, filepath.Join(root, "media", "b.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "公开区"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动排除翻转库", root)
	// 先入库（规则未配置 → 两文件 ok 且有解析产物），再配置目录规则命中「归档」
	// 并重扫 → 归档/a.pptx 转 excluded
	mustScan(t, svc, lib.ID)
	if err := svc.setLibraryRules(lib.ID, []string{"归档"}, nil); err != nil {
		t.Fatalf("setLibraryRules: %v", err)
	}
	mustScan(t, svc, lib.ID)
	entries, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	idByPath := map[string]int64{}
	for _, e := range entries {
		idByPath[e.Path] = e.ID
	}
	if idByPath["归档/a.pptx"] == 0 || idByPath["media/b.pptx"] == 0 {
		t.Fatalf("夹具文件应已入库：%v", idByPath)
	}

	// 反向翻转（excluded 目录的唯一出口）：移出排除命中目录 → 原排除解除经
	// fileHasSlides 门槛 + reviveLostFile 复活 ok（FTS 同事务重建）
	result, err := svc.MoveAssetFiles(lib.ID, []int64{idByPath["归档/a.pptx"]}, "公开区")
	if err != nil {
		t.Fatalf("MoveAssetFiles(归档→公开区): %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("移出排除目录应成功，实际 %+v", result)
	}
	entries, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	statusByPath := map[string]string{}
	for _, e := range entries {
		statusByPath[e.Path] = e.Status
	}
	if statusByPath["公开区/a.pptx"] != model.AssetFileStatusOK {
		t.Errorf("移出排除目录且产物在的文件应复活 ok，实际 %s", statusByPath["公开区/a.pptx"])
	}
	page, err := svc.SearchAssets(lib.ID, "a", model.AssetSearchOptions{Flatten: true, Limit: 50})
	if err != nil {
		t.Fatalf("SearchAssets: %v", err)
	}
	if page.Total == 0 {
		t.Errorf("复活后关键词检索应命中（FTS 同事务重建）")
	}

	// 正向翻转触发面（勘察结论）：目标目录含排除段在入口整体拒绝，目录段命中
	// 不可达；新路径命中仅经后缀规则触发（ExcludeExts 含 .pptx 的真实配置形态）
	// ——以 a.pptx 移入普通目录后命中 .pptx 后缀规则构造
	if err := svc.setLibraryRules(lib.ID, []string{}, []string{".pptx"}); err != nil {
		t.Fatalf("setLibraryRules(ext): %v", err)
	}
	result, err = svc.MoveAssetFiles(lib.ID, []int64{idByPath["media/b.pptx"]}, "公开区")
	if err != nil {
		t.Fatalf("MoveAssetFiles(b→公开区): %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("后缀规则形态移动应成功，实际 %+v", result)
	}
	entries, err = svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	statusByPath = map[string]string{}
	for _, e := range entries {
		statusByPath[e.Path] = e.Status
	}
	if statusByPath["公开区/b.pptx"] != model.AssetFileStatusExcluded {
		t.Errorf("新路径命中后缀规则的文件应翻 excluded，实际 %s", statusByPath["公开区/b.pptx"])
	}
}

func TestAssetMoveFiles_SplitRecordRewritten(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动拆分记录库", root)
	mustScan(t, svc, lib.ID)
	entries, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	var fileID int64
	for _, e := range entries {
		if e.Path == "media/a.pptx" {
			fileID = e.ID
		}
	}
	// 预置拆分记录（源在旧路径）
	if err := svc.openStoreForTest(t, lib.ID).upsertSplitRecord("media/a.pptx", "hash-a", "D:/derived/media/a", 3, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("upsertSplitRecord: %v", err)
	}

	if _, err := svc.MoveAssetFiles(lib.ID, []int64{fileID}, "归档"); err != nil {
		t.Fatalf("MoveAssetFiles: %v", err)
	}
	// 拆分记录源路径同事务重写（单表主键重写；output_dir 绝对路径不受影响，
	// 文件基名不变 → 「打开拆分单页」溯源保持有效）
	rec, err := svc.openStoreForTest(t, lib.ID).getSplitRecord("归档/a.pptx")
	if err != nil {
		t.Fatalf("getSplitRecord: %v", err)
	}
	if rec == nil {
		t.Fatalf("移动后拆分记录应按新源路径可查")
	}
	if rec.outputDir != "D:/derived/media/a" || rec.pageCount != 3 {
		t.Errorf("拆分记录其余字段应原样保留，实际 %+v", rec)
	}
	if stale, _ := svc.openStoreForTest(t, lib.ID).getSplitRecord("media/a.pptx"); stale != nil {
		t.Errorf("旧源路径的拆分记录不应残留")
	}
}

// 自动扫描失败降级（P2 收口）：移动/重定位已提交，StartScan 失败（含
// E_ASSETS_SCAN_IN_PROGRESS 被并发扫描抢占）经 autoScanFn 注入驱动，断言整体
// 调用照常返回成功结果（错误不上抛）——最终一致由在途扫描 done 事件兜底。
func TestAssetMoveAndRelocate_ScanFailureDegraded(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	svc.autoScanFn = func(ctx context.Context, libraryID string) (model.AssetScanResult, error) {
		return model.AssetScanResult{}, model.NewAppError(model.ErrCodeAssetsScanInProgress,
			"该素材库正在扫描中，请等待当前扫描结束后再试")
	}
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "扫描降级库", root)
	mustScan(t, svc, lib.ID)
	entries, _ := svc.ListFiles(lib.ID)
	var fileID int64
	for _, e := range entries {
		fileID = e.ID
	}

	// 文件批量移动：扫描失败降级，调用成功返回且移动结果完整
	result, err := svc.MoveAssetFiles(lib.ID, []int64{fileID}, "归档")
	if err != nil {
		t.Fatalf("扫描失败应降级不阻断（文件移动），实际 err：%v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("移动应成功返回，实际 %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "归档", "a.pptx")); err != nil {
		t.Fatalf("移动应已落盘")
	}

	// 目录移动：同口径降级（runFolderRelocate 统一编排面，注入复用）
	result2, err := svc.RenameAssetFolder(lib.ID, "归档", "归档2")
	if err != nil {
		t.Fatalf("扫描失败应降级不阻断（目录重命名），实际 err：%v", err)
	}
	if result2.Replaced != 1 {
		t.Fatalf("重命名索引同步应 1 条，实际 %+v", result2)
	}
	// 目录移动入口同核（MoveAssetFolder → runFolderRelocate）：移入另一目录
	if err := os.MkdirAll(filepath.Join(root, "目标区"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if _, err := svc.MoveAssetFolder(lib.ID, "归档2", "目标区"); err != nil {
		t.Fatalf("扫描失败应降级不阻断（目录移动），实际 err：%v", err)
	}
}

// no-op 文件移动（P2 收口）：移入文件当前所在目录单独识别，文案与目标内重名区分，
// 计入 failed 明细正常返回（非错误码新增），磁盘与索引零操作。
func TestAssetMoveFiles_NoOpSameDirectory(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	lib := mustCreateLibrary(t, svc, "no-op 移动库", root)
	mustScan(t, svc, lib.ID)
	entries, _ := svc.ListFiles(lib.ID)
	var fileID int64
	var oldPath string
	for _, e := range entries {
		fileID = e.ID
		oldPath = e.Path
	}

	result, err := svc.MoveAssetFiles(lib.ID, []int64{fileID}, "media")
	if err != nil {
		t.Fatalf("no-op 移动应正常返回，实际 err：%v", err)
	}
	if result.Failed != 1 || len(result.Items) != 1 {
		t.Fatalf("no-op 应记 failed 明细，实际 %+v", result)
	}
	if !strings.Contains(result.Items[0].Message, "文件已在该目录中") {
		t.Errorf("no-op 文案应明确「文件已在该目录中，无需移动」，实际 %q", result.Items[0].Message)
	}
	if result.Items[0].Label != oldPath {
		t.Errorf("no-op 明细应带 Label（当前路径），实际 %q", result.Items[0].Label)
	}
	// 零操作：磁盘与索引原样
	if _, err := os.Stat(filepath.Join(root, "media", "a.pptx")); err != nil {
		t.Errorf("no-op 移动不应触碰磁盘文件")
	}
	if entries, _ := svc.ListFiles(lib.ID); entries[0].Path != oldPath || entries[0].Status != model.AssetFileStatusOK {
		t.Errorf("no-op 移动不应变更索引，实际 %+v", entries[0])
	}
}

func TestAssetMoveFiles_PartialFailureDetail(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	if err := os.MkdirAll(filepath.Join(root, "归档"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lib := mustCreateLibrary(t, svc, "移动部分失败库", root)
	mustScan(t, svc, lib.ID)
	entries, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	var fileID int64
	for _, e := range entries {
		fileID = e.ID
	}

	// 混合目标：有效 fileID + 不存在 fileID（并发删除窗口形态）→ 部分失败明细，
	// 成功条目不回滚
	result, err := svc.MoveAssetFiles(lib.ID, []int64{fileID, 99999}, "归档")
	if err != nil {
		t.Fatalf("MoveAssetFiles: %v", err)
	}
	if result.Total != 2 || result.Succeeded != 1 || result.Failed != 1 {
		t.Fatalf("应 1 成 1 败（total=2），实际 %+v", result)
	}
	if len(result.Items) != 2 {
		t.Fatalf("逐条目明细应 2 行，实际 %d", len(result.Items))
	}
	okCount, failedCount := 0, 0
	for _, item := range result.Items {
		switch item.Status {
		case model.AssetBatchTagStatusOK:
			okCount++
			if item.Label == "" {
				t.Errorf("成功明细应带 Label（移动前路径）")
			}
		case model.AssetBatchTagStatusFailed:
			failedCount++
			if item.Message == "" {
				t.Errorf("失败明细应带可读原因")
			}
		}
	}
	if okCount != 1 || failedCount != 1 {
		t.Errorf("明细状态分布应 1 ok / 1 failed")
	}

	// fileID 重复去重（保序）：同一 ID 提交两次只移动一次
	result, err = svc.MoveAssetFiles(lib.ID, []int64{fileID, fileID}, "")
	if err != nil {
		t.Fatalf("MoveAssetFiles(去重): %v", err)
	}
	if result.Total != 1 {
		t.Errorf("重复 fileID 应去重为单条目，实际 total=%d", result.Total)
	}
}

func TestAssetMoveFiles_TargetGuards(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	lib := mustCreateLibrary(t, svc, "移动目标守卫库", root)
	mustScan(t, svc, lib.ID)
	entries, _ := svc.ListFiles(lib.ID)
	var fileID int64
	for _, e := range entries {
		fileID = e.ID
	}

	// processing 残留行拒绝（自愈不变式）
	if err := svc.openStoreForTest(t, lib.ID).markProcessing(fileID, "2026-10-20 10:00:00"); err != nil {
		t.Fatalf("markProcessing: %v", err)
	}
	result, err := svc.MoveAssetFiles(lib.ID, []int64{fileID}, "media")
	if err != nil {
		t.Fatalf("MoveAssetFiles: %v", err)
	}
	if result.Failed != 1 || result.Items[0].Message == "" {
		t.Errorf("processing 行应记 failed 明细带原因，实际 %+v", result.Items)
	}
	// 恢复状态（后续用例语境干净）
	if _, err := svc.openStoreForTest(t, lib.ID).markFileLost(fileID, "2026-10-20 10:00:01"); err != nil {
		t.Fatalf("markFileLost: %v", err)
	}
}

// 跨盘拒绝（PRD R6）：VolumeName 比对为纯字符串判定，直接以另一卷盘符构造目标
// 绝对路径驱动 moveOneAssetFile 的跨盘分支（Windows 目录 junction 可使目标解析到
// 另一卷，入口越界遏制对 junction 不可见——该分支为防御面，单测直调单条目函数
// 覆盖；盘符动态选取与库根当前卷不同，CI 环境无外部状态依赖）。
func TestAssetMoveFiles_CrossVolumeRefused(t *testing.T) {
	svc := newTestAssetService(t)
	svc.parseFn = okParseForAll
	root := t.TempDir()
	mustWritePptxFile(t, filepath.Join(root, "media", "a.pptx"))
	lib := mustCreateLibrary(t, svc, "跨盘拒绝库", root)
	mustScan(t, svc, lib.ID)
	entries, _ := svc.ListFiles(lib.ID)
	var fileID int64
	var oldPath string
	for _, e := range entries {
		fileID = e.ID
		oldPath = e.Path
	}
	store := svc.openStoreForTest(t, lib.ID)
	matcher := newAssetExcludeMatcher(nil, nil)

	// 选取与库根不同卷的盘符（不可得时跳过——单卷环境无法构造跨盘差异）
	rootVolume := filepath.VolumeName(root)
	otherVolume := ""
	for _, c := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		v := string(c) + `:\`
		if !strings.EqualFold(v, rootVolume) {
			otherVolume = v
			break
		}
	}
	if otherVolume == "" {
		t.Skip("无法构造与库根不同卷的目标路径")
	}
	targetAbs := filepath.Join(otherVolume, "外部目标")
	item := svc.moveOneAssetFile(store, lib, matcher, fileID, "", targetAbs, "2026-10-20 10:00:00")
	if item.Status != model.AssetBatchTagStatusFailed {
		t.Fatalf("跨盘移动应记 failed 明细，实际 %+v", item)
	}
	if item.Label != oldPath || item.Message == "" {
		t.Errorf("失败明细应带 Label 与可读原因，实际 %+v", item)
	}
	// 磁盘与索引零变更（跨盘在 rename 前拒绝，零半态）
	if _, err := os.Stat(filepath.Join(root, "media", "a.pptx")); err != nil {
		t.Errorf("跨盘拒绝不应移动磁盘文件")
	}
}
