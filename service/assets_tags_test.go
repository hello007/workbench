package service

import (
	"errors"
	"strings"
	"testing"

	"workbench/model"
)

// ---------- 标签树与批量打标签（素材管理第六批，SRS §2.4） ----------
//
// 覆盖面：标签 CRUD（重名/父存在/防环/级联删除）、批量文件/页标签（相对语义
// 追加移除混合、自动建根标签、部分失败明细、参数校验）、ListTags 计数口径、
// 检索子树级联展开（FTS 关键词路径 + 纯标签浏览路径）、平铺老数据行为不变。
//
// fixture 走真实扫描链路（buildAssetsPptx + StartScan）保证 files/slides/FTS 行
// 完整；标签绑定经 UpdateFileMeta/UpdateSlideMeta（整体替换语义）或批量 API
// 写入。无 sleep/mtime 依赖（docs/spec/test-stability.md）。

// assetFileIDByPath 按相对路径查文件记录 ID（ListFiles 数据源，未命中 Fatal）。
func assetFileIDByPath(t *testing.T, svc *AssetLibraryService, libraryID, relPath string) int64 {
	t.Helper()
	files, err := svc.ListFiles(libraryID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if f.Path == relPath {
			return f.ID
		}
	}
	t.Fatalf("未找到文件 %s", relPath)
	return 0
}

// tagItemByName 按名取标签清单条目（ListTags 数据源，未命中 Fatal）。
func tagItemByName(t *testing.T, svc *AssetLibraryService, libraryID, name string) model.AssetTagListItem {
	t.Helper()
	items, err := svc.ListTags(libraryID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("ListTags 中未找到标签 %s", name)
	return model.AssetTagListItem{}
}

// mustCreateTag 建标签断言辅助。
func mustCreateTag(t *testing.T, svc *AssetLibraryService, libraryID, name string, parentID int64, color string) model.AssetTag {
	t.Helper()
	tag, err := svc.CreateTag(libraryID, name, parentID, color)
	if err != nil {
		t.Fatalf("CreateTag(%s): %v", name, err)
	}
	return tag
}

// mustBindFileTags 文件标签绑定断言辅助（整体替换语义，初始态设置用）。
func mustBindFileTags(t *testing.T, svc *AssetLibraryService, libraryID string, fileID int64, tagNames []string) {
	t.Helper()
	if err := svc.UpdateFileMeta(libraryID, fileID, model.AssetFileMetaUpdate{Tags: tagNames}); err != nil {
		t.Fatalf("UpdateFileMeta(%d): %v", fileID, err)
	}
}

// mustBindSlideTags 页自有标签绑定断言辅助（初始态设置用）。
func mustBindSlideTags(t *testing.T, svc *AssetLibraryService, libraryID string, fileID int64, slideNum int, tagNames []string) {
	t.Helper()
	if err := svc.UpdateSlideMeta(libraryID, fileID, slideNum, model.AssetSlideMetaUpdate{TagNames: tagNames}); err != nil {
		t.Fatalf("UpdateSlideMeta(%d, %d): %v", fileID, slideNum, err)
	}
}

// ---------- 标签 CRUD 与 ListTags ----------

// TestAssetTagCRUD_CreateAndList 新建与清单：指定色保留、空色走默认色板（同名
// 确定性取色）、重名拒绝、父存在性校验、ListTags 按「父级分组 + 组内 sort_order」
// 序返回且计数 = 文件绑定 + 页自有绑定之和、未使用标签计数 0、新标签追加组尾。
func TestAssetTagCRUD_CreateAndList(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "标签清单库", dir)
	buildAssetsPptx(t, dir, "文件甲.pptx", []assetsSlideSpec{
		{titlePh: "标题", body: []string{"内容"}},
	})
	mustScanIndex(t, svc, lib)

	// 指定色保留；空色按标签名确定性分配默认色板
	colored := mustCreateTag(t, svc, lib.ID, "红色系", 0, "#ff0000")
	if colored.Color != "#ff0000" {
		t.Errorf("指定色应保留，实际 %q", colored.Color)
	}
	def := mustCreateTag(t, svc, lib.ID, "默认色系", 0, "")
	if want := assetTagColorFor("默认色系"); def.Color != want {
		t.Errorf("空色应分配默认色板 %q，实际 %q", want, def.Color)
	}
	if colored.ID <= 0 {
		t.Error("新标签应携带自增 ID")
	}

	// 重名拒绝（库内唯一）
	if _, err := svc.CreateTag(lib.ID, "红色系", 0, ""); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Errorf("重名应拒绝且提示已存在，实际 err=%v", err)
	}
	// 父标签存在性校验
	if _, err := svc.CreateTag(lib.ID, "孤儿", 99999, ""); err == nil || !strings.Contains(err.Error(), "父标签不存在") {
		t.Errorf("父不存在应拒绝，实际 err=%v", err)
	}
	// 空名拒绝
	if _, err := svc.CreateTag(lib.ID, "   ", 0, ""); err == nil {
		t.Error("空名应拒绝")
	}
	// 子标签：ParentID 落库
	child := mustCreateTag(t, svc, lib.ID, "子标签", colored.ID, "")
	if child.ParentID != colored.ID {
		t.Errorf("子标签 ParentID 应为 %d，实际 %d", colored.ID, child.ParentID)
	}
	// 追加创建的根标签排根组尾（兄弟序号连续：先建者序号小）
	late := mustCreateTag(t, svc, lib.ID, "后建标签", 0, "")
	if late.ID <= 0 {
		t.Error("后建标签应携带自增 ID")
	}

	// ListTags 计数口径：文件绑定 + 页自有绑定之和；未使用标签计数 0
	fileID := assetFileIDByPath(t, svc, lib.ID, "文件甲.pptx")
	mustBindFileTags(t, svc, lib.ID, fileID, []string{"红色系"})
	mustBindSlideTags(t, svc, lib.ID, fileID, 1, []string{"红色系", "子标签"})

	items, err := svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("应有 4 个标签，实际 %d", len(items))
	}
	// 兄弟序契约：根组在前按 sort_order 序（红色系 → 默认色系 → 后建标签），
	// 子组随后（子标签）；组内序号 0 起连续
	wantOrder := []string{"红色系", "默认色系", "后建标签", "子标签"}
	for i, want := range wantOrder {
		if items[i].Name != want {
			t.Errorf("平铺序[%d] 应为 %s，实际 %s（全序：%v）", i, want, items[i].Name, assetTagListNames(items))
		}
	}
	wantSort := []int{0, 1, 2, 0}
	for i, want := range wantSort {
		if items[i].SortOrder != want {
			t.Errorf("「%s」SortOrder 应为 %d，实际 %d", items[i].Name, want, items[i].SortOrder)
		}
	}
	byName := map[string]model.AssetTagListItem{}
	for _, item := range items {
		byName[item.Name] = item
	}
	if got := byName["红色系"].Count; got != 2 { // 文件绑定 1 + 页自有绑定 1
		t.Errorf("「红色系」直接绑定计数应为 2（文件 1 + 页 1），实际 %d", got)
	}
	if got := byName["子标签"].Count; got != 1 { // 仅页自有绑定 1
		t.Errorf("「子标签」直接绑定计数应为 1，实际 %d", got)
	}
	if got := byName["默认色系"].Count; got != 0 {
		t.Errorf("未使用标签计数应为 0，实际 %d", got)
	}
}

// assetTagListNames 清单名称序投影（断言失败信息可读性辅助；与 git_tag_test.go
// 的 tagNames（GitTag 版）分名防同包冲突）。
func assetTagListNames(items []model.AssetTagListItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
}

// TestAssetTagCRUD_UpdateCycleGuard 标签编辑防环：父不能选自身（直接环）、不选
// 直接子代与孙代（间接环）；合法移动生效；重名/父不存在/目标不存在拒绝。
func TestAssetTagCRUD_UpdateCycleGuard(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "防环库", dir)
	// 三层链：行业 → 城商行 → 对公（对公.parent=城商行，城商行.parent=行业）
	root := mustCreateTag(t, svc, lib.ID, "行业", 0, "")
	child := mustCreateTag(t, svc, lib.ID, "城商行", root.ID, "")
	grand := mustCreateTag(t, svc, lib.ID, "对公", child.ID, "")

	// 直接环：父选自身
	if err := svc.UpdateTag(lib.ID, root.ID, "行业", root.ID, ""); err == nil {
		t.Error("父选自身应拒绝")
	}
	// 间接环：父选直接子代 / 孙代（均为自身后代）
	if err := svc.UpdateTag(lib.ID, root.ID, "行业", child.ID, ""); err == nil || !strings.Contains(err.Error(), "循环") {
		t.Errorf("父选子代（间接环）应拒绝，实际 err=%v", err)
	}
	if err := svc.UpdateTag(lib.ID, root.ID, "行业", grand.ID, ""); err == nil || !strings.Contains(err.Error(), "循环") {
		t.Errorf("父选孙代（间接环）应拒绝，实际 err=%v", err)
	}
	// 父不存在拒绝
	if err := svc.UpdateTag(lib.ID, child.ID, "城商行", 99999, ""); err == nil || !strings.Contains(err.Error(), "父标签不存在") {
		t.Errorf("父不存在应拒绝，实际 err=%v", err)
	}
	// 重名拒绝（不同 ID 同名）
	if err := svc.UpdateTag(lib.ID, child.ID, "行业", 0, ""); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Errorf("改名撞已有名应拒绝，实际 err=%v", err)
	}
	// 目标标签不存在拒绝
	if err := svc.UpdateTag(lib.ID, 99999, "任意", 0, ""); err == nil || !strings.Contains(err.Error(), "标签不存在") {
		t.Errorf("目标不存在应拒绝，实际 err=%v", err)
	}

	// 合法移动生效：子标签挂到新根下；改名与改色同次提交
	fresh := mustCreateTag(t, svc, lib.ID, "新分组", 0, "")
	if err := svc.UpdateTag(lib.ID, child.ID, "城商行专题", fresh.ID, "#00ff00"); err != nil {
		t.Fatalf("合法移动应成功: %v", err)
	}
	item := tagItemByName(t, svc, lib.ID, "城商行专题")
	if item.ParentID != fresh.ID || item.Color != "#00ff00" {
		t.Errorf("移动/改色未生效：%+v", item)
	}
	// 移回根（ParentID=0 写 NULL 归一 0）
	if err := svc.UpdateTag(lib.ID, child.ID, "城商行专题", 0, "#00ff00"); err != nil {
		t.Fatalf("移回根应成功: %v", err)
	}
	if got := tagItemByName(t, svc, lib.ID, "城商行专题").ParentID; got != 0 {
		t.Errorf("移回根后 ParentID 应为 0，实际 %d", got)
	}
}

// TestAssetTagCRUD_DeleteCascades 删除级联：删父标签连带删全部后代标签本体，
// 解除全部文件绑定与页自有绑定（删除标签不删素材），返回绑定行数口径的受影响
// 计数；单事务无半态。
func TestAssetTagCRUD_DeleteCascades(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "级联删除库", dir)
	buildAssetsPptx(t, dir, "文件甲.pptx", []assetsSlideSpec{
		{titlePh: "标题", body: []string{"内容"}},
		{titlePh: "次页", body: []string{"内容"}},
	})
	buildAssetsPptx(t, dir, "文件乙.pptx", []assetsSlideSpec{
		{titlePh: "标题", body: []string{"内容"}},
	})
	mustScanIndex(t, svc, lib)

	root := mustCreateTag(t, svc, lib.ID, "行业", 0, "")
	mustCreateTag(t, svc, lib.ID, "城商行", root.ID, "") // 子标签本体随删父级联删除（不单独断言 ID）
	mustCreateTag(t, svc, lib.ID, "无关标签", 0, "")

	fileA := assetFileIDByPath(t, svc, lib.ID, "文件甲.pptx")
	fileB := assetFileIDByPath(t, svc, lib.ID, "文件乙.pptx")
	// 绑定面：文件甲绑 [行业, 城商行]（2 行）+ 文件乙绑 [城商行]（1 行）+ 文件乙页 1 绑 [城商行]（1 行）→ 删「行业」子树共解除 4 行绑定
	mustBindFileTags(t, svc, lib.ID, fileA, []string{"行业", "城商行"})
	mustBindFileTags(t, svc, lib.ID, fileB, []string{"城商行"})
	mustBindSlideTags(t, svc, lib.ID, fileB, 1, []string{"城商行"})

	affected, err := svc.DeleteTag(lib.ID, root.ID)
	if err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if affected != 4 {
		t.Errorf("解除关联绑定行数应为 4，实际 %d", affected)
	}

	// 标签本体：子树两行均删，无关标签保留
	items, err := svc.ListTags(lib.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(items) != 1 || items[0].Name != "无关标签" {
		t.Errorf("删父后应仅剩无关标签，实际：%v", items)
	}
	// 绑定全部解除（素材本体不动）
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	for _, f := range files {
		if len(f.Tags) != 0 {
			t.Errorf("文件 %s 标签应已解除，实际 %v", f.Path, f.Tags)
		}
	}
	slides := mustListSlides(t, svc, lib.ID)
	for _, s := range slides {
		if len(s.OwnTags) != 0 {
			t.Errorf("页 %s#%d 自有标签应已解除，实际 %v", s.FilePath, s.SlideNum, s.OwnTags)
		}
	}
	if len(slides) != 3 {
		t.Errorf("素材页记录不应被删除，实际 %d 页", len(slides))
	}

	// 不存在标签拒绝（幂等边界由调用方前置校验）
	if _, err := svc.DeleteTag(lib.ID, root.ID); err == nil || !strings.Contains(err.Error(), "标签不存在") {
		t.Errorf("重复删除应拒绝，实际 err=%v", err)
	}
}

// ---------- 批量打标签 ----------

// TestAssetBatchTag_FileTagsMixed 文件批量：相对语义追加/移除混合（非整体替换）、
// 不存在的添加名自动创建为根标签（默认色板）、批内同名去重、已绑定条目幂等。
func TestAssetBatchTag_FileTagsMixed(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "批量文件库", dir)
	buildAssetsPptx(t, dir, "文件甲.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"内容"}}})
	buildAssetsPptx(t, dir, "文件乙.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"内容"}}})
	mustScanIndex(t, svc, lib)

	fileA := assetFileIDByPath(t, svc, lib.ID, "文件甲.pptx")
	fileB := assetFileIDByPath(t, svc, lib.ID, "文件乙.pptx")
	mustBindFileTags(t, svc, lib.ID, fileA, []string{"旧标签"})
	// 预置「保留标签」，验证 remove 不误伤 add 集之外标签
	mustCreateTag(t, svc, lib.ID, "保留标签", 0, "")

	result, err := svc.BatchUpdateFileTags(lib.ID,
		[]int64{fileA, fileB, fileA}, // 含重复 fileID：按提交序逐条目处理（幂等）
		model.AssetBatchTagOp{
			AddTags:    []string{"新标签", "  保留标签 ", "新标签"}, // 带空白与批内重名
			RemoveTags: []string{"旧标签"},
		})
	if err != nil {
		t.Fatalf("BatchUpdateFileTags: %v", err)
	}
	if result.Total != 3 || result.Succeeded != 3 || result.Failed != 0 {
		t.Errorf("汇总不符：%+v", result)
	}
	if len(result.Items) != 3 {
		t.Fatalf("应有 3 条明细，实际 %d", len(result.Items))
	}
	for i, item := range result.Items {
		if item.Status != model.AssetBatchTagStatusOK || item.Message != "" {
			t.Errorf("明细 %d 应成功：%+v", i, item)
		}
		if item.SlideNum != 0 {
			t.Errorf("文件条目 SlideNum 应为 0（omitempty 省略），实际 %d", item.SlideNum)
		}
	}

	// 绑定结果：文件甲 [保留标签, 新标签]（旧标签移除，按名称序）；文件乙同
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	byPath := map[string][]string{}
	for _, f := range files {
		names := make([]string, 0, len(f.Tags))
		for _, tag := range f.Tags {
			names = append(names, tag.Name)
		}
		byPath[f.Path] = names
	}
	if got := byPath["文件甲.pptx"]; len(got) != 2 || got[0] != "保留标签" || got[1] != "新标签" {
		t.Errorf("文件甲应为 [保留标签 新标签]（旧标签移除），实际 %v", got)
	}
	if got := byPath["文件乙.pptx"]; len(got) != 2 {
		t.Errorf("文件乙应为 [保留标签 新标签]，实际 %v", got)
	}

	// 自动创建的新标签为根标签 + 默认色板
	item := tagItemByName(t, svc, lib.ID, "新标签")
	if item.ParentID != 0 {
		t.Errorf("自动创建标签应为根（ParentID=0），实际 %d", item.ParentID)
	}
	if want := assetTagColorFor("新标签"); item.Color != want {
		t.Errorf("自动创建标签色应为默认色板 %q，实际 %q", want, item.Color)
	}
	if item.Count != 2 { // 两个文件各 1 行（重复 fileID 幂等，不产生第三行）
		t.Errorf("「新标签」绑定计数应为 2，实际 %d", item.Count)
	}
}

// TestAssetBatchTag_FileParamsRejected 批量参数校验：add/remove 交集非空、两组
// 全空、条目为空、库不存在分别拒绝（库不存在走 E_ASSETS_LIB_NOT_FOUND 语义）。
func TestAssetBatchTag_FileParamsRejected(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "批量参数库", dir)

	if _, err := svc.BatchUpdateFileTags(lib.ID, []int64{1},
		model.AssetBatchTagOp{AddTags: []string{"同名"}, RemoveTags: []string{"同名"}}); err == nil || !strings.Contains(err.Error(), "同时添加与移除") {
		t.Errorf("交集非空应拒绝，实际 err=%v", err)
	}
	if _, err := svc.BatchUpdateFileTags(lib.ID, []int64{1}, model.AssetBatchTagOp{}); err == nil || !strings.Contains(err.Error(), "至少选择") {
		t.Errorf("双全空应拒绝，实际 err=%v", err)
	}
	if _, err := svc.BatchUpdateFileTags(lib.ID, []int64{1},
		model.AssetBatchTagOp{AddTags: []string{"   "}}); err == nil || !strings.Contains(err.Error(), "至少选择") {
		t.Errorf("仅空白名等价双空应拒绝，实际 err=%v", err)
	}
	if _, err := svc.BatchUpdateFileTags(lib.ID, nil,
		model.AssetBatchTagOp{AddTags: []string{"任意"}}); err == nil || !strings.Contains(err.Error(), "先选择") {
		t.Errorf("条目为空应拒绝，实际 err=%v", err)
	}
	if _, err := svc.BatchUpdateFileTags("不存在的库", []int64{1},
		model.AssetBatchTagOp{AddTags: []string{"任意"}}); !errors.Is(err, errAssetLibraryNotFound) {
		t.Errorf("库不存在应返回 E_ASSETS_LIB_NOT_FOUND 语义，实际 err=%v", err)
	}
}

// TestAssetBatchTag_FilePartialFailure 部分失败隔离：不存在 fileID 记 failed
// 明细继续，成功条目已写入不回滚。
func TestAssetBatchTag_FilePartialFailure(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "批量隔离库", dir)
	buildAssetsPptx(t, dir, "文件甲.pptx", []assetsSlideSpec{{titlePh: "标题", body: []string{"内容"}}})
	mustScanIndex(t, svc, lib)

	fileA := assetFileIDByPath(t, svc, lib.ID, "文件甲.pptx")
	result, err := svc.BatchUpdateFileTags(lib.ID, []int64{99999, fileA},
		model.AssetBatchTagOp{AddTags: []string{"存活标签"}})
	if err != nil {
		t.Fatalf("部分失败不应整体报错: %v", err)
	}
	if result.Total != 2 || result.Succeeded != 1 || result.Failed != 1 {
		t.Fatalf("汇总不符：%+v", result)
	}
	if result.Items[0].Status != model.AssetBatchTagStatusFailed ||
		!strings.Contains(result.Items[0].Message, "文件不存在") {
		t.Errorf("首条应为「文件不存在」failed 明细：%+v", result.Items[0])
	}
	if result.Items[1].Status != model.AssetBatchTagStatusOK {
		t.Errorf("次条应成功：%+v", result.Items[1])
	}
	// 成功条目已写入（不回滚）
	files, err := svc.ListFiles(lib.ID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files[0].Tags) != 1 || files[0].Tags[0].Name != "存活标签" {
		t.Errorf("成功条目标签应已写入，实际 %v", files[0].Tags)
	}
}

// TestAssetBatchTag_SlideTags 页自有标签批量：(file_id, slide_num) 键正确落
// slide_tags（页自有与文件标签互不写入）、不存在页记 failed 明细、slideNum < 1
// 整体参数拒绝。
func TestAssetBatchTag_SlideTags(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "批量页标签库", dir)
	buildAssetsPptx(t, dir, "文件甲.pptx", []assetsSlideSpec{
		{titlePh: "首页标题", body: []string{"首页正文"}},
		{titlePh: "次页标题", body: []string{"次页正文"}},
	})
	mustScanIndex(t, svc, lib)
	fileA := assetFileIDByPath(t, svc, lib.ID, "文件甲.pptx")
	mustBindFileTags(t, svc, lib.ID, fileA, []string{"文件级标签"})

	// slideNum < 1：调用方构造错误，整体参数拒绝（区别于数据态的逐条目明细）
	if _, err := svc.BatchUpdateSlideTags(lib.ID,
		[]model.AssetSlideTarget{{FileID: fileA, SlideNum: 0}},
		model.AssetBatchTagOp{AddTags: []string{"页标签"}}); err == nil || !strings.Contains(err.Error(), "从 1 计") {
		t.Errorf("slideNum<1 应参数拒绝，实际 err=%v", err)
	}
	if _, err := svc.BatchUpdateSlideTags(lib.ID, nil,
		model.AssetBatchTagOp{AddTags: []string{"页标签"}}); err == nil || !strings.Contains(err.Error(), "先选择") {
		t.Errorf("目标为空应拒绝，实际 err=%v", err)
	}

	result, err := svc.BatchUpdateSlideTags(lib.ID,
		[]model.AssetSlideTarget{
			{FileID: fileA, SlideNum: 1},
			{FileID: fileA, SlideNum: 2},
			{FileID: fileA, SlideNum: 9}, // 不存在页
		},
		model.AssetBatchTagOp{
			AddTags:    []string{"页标签"},
			RemoveTags: []string{"不存在的标签"}, // 移除不存在名 = 无操作，不报错
		})
	if err != nil {
		t.Fatalf("BatchUpdateSlideTags: %v", err)
	}
	if result.Total != 3 || result.Succeeded != 2 || result.Failed != 1 {
		t.Fatalf("汇总不符：%+v", result)
	}
	if result.Items[2].Status != model.AssetBatchTagStatusFailed ||
		!strings.Contains(result.Items[2].Message, "页不存在") || result.Items[2].SlideNum != 9 {
		t.Errorf("第 3 条应为「页不存在」failed 明细：%+v", result.Items[2])
	}

	// 页自有标签落 slide_tags（OwnTags），文件标签（Tags）不受影响
	slides := mustListSlides(t, svc, lib.ID)
	for _, s := range slides {
		if len(s.OwnTags) != 1 || s.OwnTags[0].Name != "页标签" {
			t.Errorf("页 %s#%d 自有标签应为 [页标签]，实际 %v", s.FilePath, s.SlideNum, s.OwnTags)
		}
		if len(s.Tags) != 1 || s.Tags[0].Name != "文件级标签" {
			t.Errorf("页 %s#%d 继承文件标签不应受影响，实际 %v", s.FilePath, s.SlideNum, s.Tags)
		}
	}
	// 文件标签绑定行未被批量写入
	item := tagItemByName(t, svc, lib.ID, "页标签")
	if item.Count != 2 {
		t.Errorf("「页标签」绑定计数应为 2（仅页自有），实际 %d", item.Count)
	}
}

// ---------- 检索子树级联展开 ----------

// TestAssetTagSubtreeExpansion_Search 级联命中（PRD D2）：选父标签命中全部后代
// 绑定条目（FTS 关键词路径与纯标签浏览路径同口径）；选子标签不命中父自身绑定；
// OnlyOwn 开关仅收窄页自有分支，文件条目不受影响。fixture：行业(根) → 城商行
// (子)；文件甲（文件名含关键词，FTS 文件行命中载体）绑子标签、文件乙绑父标签
// （其页 1 另绑子标签）、文件丙绑无关标签；三文件页文本同含关键词，使标签筛选
// 成为唯一命中变量。
func TestAssetTagSubtreeExpansion_Search(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "级联检索库", dir)
	buildAssetsPptx(t, dir, "城商行专题.pptx", []assetsSlideSpec{{titlePh: "甲标题", body: []string{"城商行专题内容"}}})
	buildAssetsPptx(t, dir, "文件乙.pptx", []assetsSlideSpec{
		{titlePh: "乙首页", body: []string{"城商行合作方案"}},
		{titlePh: "乙次页", body: []string{"城商行落地细节"}},
	})
	buildAssetsPptx(t, dir, "文件丙.pptx", []assetsSlideSpec{{titlePh: "丙标题", body: []string{"城商行无关引用"}}})
	mustScanIndex(t, svc, lib)

	parent := mustCreateTag(t, svc, lib.ID, "行业", 0, "")
	child := mustCreateTag(t, svc, lib.ID, "城商行", parent.ID, "")
	mustCreateTag(t, svc, lib.ID, "其他", 0, "") // 无关标签（文件丙绑定，验证不串扰）

	fileA := assetFileIDByPath(t, svc, lib.ID, "城商行专题.pptx")
	fileB := assetFileIDByPath(t, svc, lib.ID, "文件乙.pptx")
	fileC := assetFileIDByPath(t, svc, lib.ID, "文件丙.pptx")
	mustBindFileTags(t, svc, lib.ID, fileA, []string{"城商行"}) // 子标签绑定
	mustBindFileTags(t, svc, lib.ID, fileB, []string{"行业"})  // 父标签绑定
	mustBindSlideTags(t, svc, lib.ID, fileB, 1, []string{"城商行"})
	mustBindFileTags(t, svc, lib.ID, fileC, []string{"其他"})

	// 纯标签浏览：选父 = 父 ∪ 后代（[行业] 展开为 [行业, 城商行]）
	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{TagIDs: []int64{parent.ID}, Flatten: true})
	if err != nil {
		t.Fatalf("纯标签浏览选父: %v", err)
	}
	got := hitKeySet(page)
	wantSet := map[hitKey]bool{
		{refType: model.AssetRefTypeFile, path: "城商行专题.pptx"}:          true, // 经子标签级联
		{refType: model.AssetRefTypeFile, path: "文件乙.pptx"}:            true, // 父自身绑定
		{refType: model.AssetRefTypeSlide, path: "城商行专题.pptx", num: 1}: true,
		{refType: model.AssetRefTypeSlide, path: "文件乙.pptx", num: 1}:   true, // 页自身 + 文件继承双通道
		{refType: model.AssetRefTypeSlide, path: "文件乙.pptx", num: 2}:   true, // 经文件继承
	}
	if page.Total != len(wantSet) || len(got) != len(wantSet) {
		t.Fatalf("选父纯标签浏览应命中 %d 条，实际 total=%d：%v", len(wantSet), page.Total, got)
	}
	for k := range wantSet {
		if !got[k] {
			t.Errorf("选父应命中 %v", k)
		}
	}
	if got[hitKey{refType: model.AssetRefTypeFile, path: "文件丙.pptx"}] {
		t.Error("无关标签绑定文件不应命中")
	}

	// 选子不命中父自身绑定（[城商行] 展开为 [城商行]，文件乙的父绑定不在集内；
	// 文件乙页 1 经页自身子标签绑定仍命中——默认合并语义）
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{TagIDs: []int64{child.ID}, Flatten: true})
	if err != nil {
		t.Fatalf("纯标签浏览选子: %v", err)
	}
	got = hitKeySet(page)
	wantSet = map[hitKey]bool{
		{refType: model.AssetRefTypeFile, path: "城商行专题.pptx"}:          true,
		{refType: model.AssetRefTypeSlide, path: "城商行专题.pptx", num: 1}: true,
		{refType: model.AssetRefTypeSlide, path: "文件乙.pptx", num: 1}:   true, // 仅页自身绑定命中
	}
	if page.Total != len(wantSet) || len(got) != len(wantSet) {
		t.Fatalf("选子纯标签浏览应命中 %d 条，实际 total=%d：%v", len(wantSet), page.Total, got)
	}
	for k := range wantSet {
		if !got[k] {
			t.Errorf("选子应命中 %v", k)
		}
	}
	if got[hitKey{refType: model.AssetRefTypeFile, path: "文件乙.pptx"}] ||
		got[hitKey{refType: model.AssetRefTypeSlide, path: "文件乙.pptx", num: 2}] {
		t.Error("选子不应命中父标签自身绑定的文件条目及其无自有标签页")
	}

	// FTS 关键词路径：keyword + 标签筛选组合，同口径级联（选父）
	page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{TagIDs: []int64{parent.ID}, Flatten: true})
	if err != nil {
		t.Fatalf("关键词路径选父: %v", err)
	}
	got = hitKeySet(page)
	// FTS 命中：文件甲文件行（文件名含词，fileFilter 含甲 → 命中）+ 三文件页行；
	// 行级判定出局：文件丙 file 行/页行（无标签通道）
	wantSet = map[hitKey]bool{
		{refType: model.AssetRefTypeFile, path: "城商行专题.pptx"}:          true,
		{refType: model.AssetRefTypeSlide, path: "城商行专题.pptx", num: 1}: true,
		{refType: model.AssetRefTypeSlide, path: "文件乙.pptx", num: 1}:   true,
		{refType: model.AssetRefTypeSlide, path: "文件乙.pptx", num: 2}:   true,
	}
	if page.Total != len(wantSet) || len(got) != len(wantSet) {
		t.Fatalf("关键词路径选父应命中 %d 条，实际 total=%d：%v", len(wantSet), page.Total, got)
	}
	for k := range wantSet {
		if !got[k] {
			t.Errorf("关键词路径选父应命中 %v", k)
		}
	}

	// OnlyOwn 开关：仅页自身绑定命中（文件乙页 1）；文件条目不受 OnlyOwn 影响
	// （文件甲文件行命中）；文件继承通道的页条目（文件乙页 2、文件甲页 1）收窄
	page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{
		TagIDs: []int64{child.ID}, SlideTagOnlyOwn: true, Flatten: true,
	})
	if err != nil {
		t.Fatalf("关键词路径 OnlyOwn: %v", err)
	}
	got = hitKeySet(page)
	wantSet = map[hitKey]bool{
		{refType: model.AssetRefTypeFile, path: "城商行专题.pptx"}:        true, // 文件条目不受 OnlyOwn 影响
		{refType: model.AssetRefTypeSlide, path: "文件乙.pptx", num: 1}: true, // 仅页自身绑定
	}
	if page.Total != len(wantSet) || len(got) != len(wantSet) {
		t.Fatalf("OnlyOwn 应命中 %d 条，实际 total=%d：%v", len(wantSet), page.Total, got)
	}
	for k := range wantSet {
		if !got[k] {
			t.Errorf("OnlyOwn 应命中 %v", k)
		}
	}
	if got[hitKey{refType: model.AssetRefTypeSlide, path: "城商行专题.pptx", num: 1}] ||
		got[hitKey{refType: model.AssetRefTypeSlide, path: "文件乙.pptx", num: 2}] {
		t.Error("OnlyOwn 下文件继承通道的页条目不应命中")
	}
}

// TestAssetTagSubtreeExpansion_FlatDataUnchanged 平铺老数据（全 parent_id=0）
// 行为不变：展开 = 自身集合，筛选只命中直接绑定——多级标签引入前既有语义回归守护。
func TestAssetTagSubtreeExpansion_FlatDataUnchanged(t *testing.T) {
	svc := newTestAssetService(t)
	dir := t.TempDir()
	lib := mustCreateLibrary(t, svc, "平铺回归库", dir)
	buildAssetsPptx(t, dir, "文件甲.pptx", []assetsSlideSpec{{titlePh: "甲标题", body: []string{"城商行内容甲"}}})
	buildAssetsPptx(t, dir, "文件乙.pptx", []assetsSlideSpec{{titlePh: "乙标题", body: []string{"城商行内容乙"}}})
	mustScanIndex(t, svc, lib)

	tagA := mustCreateTag(t, svc, lib.ID, "标签甲", 0, "")
	tagB := mustCreateTag(t, svc, lib.ID, "标签乙", 0, "")
	fileA := assetFileIDByPath(t, svc, lib.ID, "文件甲.pptx")
	fileB := assetFileIDByPath(t, svc, lib.ID, "文件乙.pptx")
	mustBindFileTags(t, svc, lib.ID, fileA, []string{"标签甲"})
	mustBindFileTags(t, svc, lib.ID, fileB, []string{"标签乙"})

	// 纯标签浏览：筛选互不串扰
	page, err := svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{TagIDs: []int64{tagA.ID}, Flatten: true})
	if err != nil {
		t.Fatalf("平铺筛选: %v", err)
	}
	got := hitKeySet(page)
	if page.Total != 2 || len(got) != 2 ||
		!got[hitKey{refType: model.AssetRefTypeFile, path: "文件甲.pptx"}] ||
		!got[hitKey{refType: model.AssetRefTypeSlide, path: "文件甲.pptx", num: 1}] {
		t.Errorf("平铺数据选标签甲应仅命中文件甲两条，实际 total=%d：%v", page.Total, got)
	}

	// FTS 关键词路径同口径
	page, err = svc.SearchAssets(lib.ID, "城商行", model.AssetSearchOptions{TagIDs: []int64{tagB.ID}, Flatten: true})
	if err != nil {
		t.Fatalf("平铺关键词检索: %v", err)
	}
	if page.Total != 1 || !hitKeySet(page)[hitKey{refType: model.AssetRefTypeSlide, path: "文件乙.pptx", num: 1}] {
		t.Errorf("平铺数据关键词 + 标签乙应仅命中文件乙页 1，实际 total=%d：%v", page.Total, hitKeySet(page))
	}

	// 不存在标签 ID：展开集含未知 ID，无绑定命中（空结果不报错）
	page, err = svc.SearchAssets(lib.ID, "", model.AssetSearchOptions{TagIDs: []int64{99999}, Flatten: true})
	if err != nil {
		t.Fatalf("未知标签 ID 筛选不应报错: %v", err)
	}
	if page.Total != 0 {
		t.Errorf("未知标签 ID 应零命中，实际 total=%d", page.Total)
	}
}
