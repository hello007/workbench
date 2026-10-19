package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// ---------- 扫描用例（产物排除 + 字典序 + 权限降级） ----------

// TestScanPptxFilesForSplit_DerivativeExcluded 递归扫描综合用例：
//   - 子目录深层递归、扩展名大小写不敏感均扫入；
//   - 拆分产物（同名文件夹布局，两位/三位页序通吃）排除不拆；
//   - 同名文件夹内主干不同的文件、其他目录下同名形态文件不误伤；
//   - 非 .pptx 忽略；顺序为 WalkDir 字典序稳定。
func TestScanPptxFilesForSplit_DerivativeExcluded(t *testing.T) {
	dir := t.TempDir()
	// 扫入面
	included := []string{
		"汇报.pptx",          // 根目录源文件
		"汇报/其他-01.pptx",     // 与产物同文件夹但主干不同：非产物
		"旧目录/汇报-01.pptx",    // 同名形态但父目录名不匹配：非产物
		"子目录/大写.PPTX",       // 深层递归 + 大小写不敏感扩展名
	}
	// 排除面（拆分产物）
	excluded := []string{
		"汇报/汇报-01.pptx",  // 两位页序（旧 %02d 产物）
		"汇报/汇报-001.pptx", // 三位页序（现 %03d 产物）
	}
	for _, rel := range append(append([]string{}, included...), excluded...) {
		testutil.WriteFile(t, filepath.Join(dir, filepath.FromSlash(rel)), "dummy")
	}
	testutil.WriteFile(t, filepath.Join(dir, "汇报/说明.txt"), "非 PPT 忽略")

	files, err := scanPptxFilesForSplit(dir)
	if err != nil {
		t.Fatalf("scanPptxFilesForSplit: %v", err)
	}

	got := make(map[string]bool, len(files))
	for _, f := range files {
		rel, err := filepath.Rel(dir, f)
		if err != nil {
			t.Fatalf("计算相对路径 %s: %v", f, err)
		}
		got[filepath.ToSlash(rel)] = true
	}
	for _, rel := range included {
		if !got[rel] {
			t.Errorf("应扫入 %s，实际：%v", rel, files)
		}
	}
	for _, rel := range excluded {
		if got[rel] {
			t.Errorf("拆分产物 %s 不应被扫入（重复批量拆分会被再拆）", rel)
		}
	}
	if len(files) != len(included) {
		t.Errorf("应扫入 %d 个文件，实际 %d 个：%v", len(included), len(files), files)
	}
	// 字典序稳定由 WalkDir 逐层按名排序保证，串行顺序断言见
	// TestSplitFolder_SerialOrderAndSummary（无前缀关系的命名面）
}

// TestScanPptxFilesForSplit_EmptyDir 空目录与非 PPT 目录返回空清单（SplitFolder
// 层负责转引导级错误）。
func TestScanPptxFilesForSplit_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := scanPptxFilesForSplit(dir)
	if err != nil {
		t.Fatalf("scanPptxFilesForSplit: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("空目录应返回空清单，实际：%v", files)
	}
}

// ---------- SplitFolder 编排用例 ----------

// fakeBatchPptxProvider 批量编排 fake：按源路径分流预置结果/错误与页级进度；
// cancelDuring 命中的文件在 Split 调用时先经 svc.CancelPptxSplit 触发整批取消
// （复用注册表链路），随后按预置结果/错误返回——分别驱动「取消中断当前文件」
// 与「取消时当前文件已完成」两类编排路径。调用序列全程记录。
type fakeBatchPptxProvider struct {
	svc          *PptxSplitService
	calls        []string
	results      map[string]model.PptxSplitResult
	errs         map[string]error
	progress     map[string][]fakeProgressCall
	cancelDuring map[string]bool
}

func (f *fakeBatchPptxProvider) Split(ctx context.Context, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error) {
	f.calls = append(f.calls, srcPath)
	if f.cancelDuring[srcPath] && f.svc != nil {
		f.svc.CancelPptxSplit()
	}
	for _, p := range f.progress[srcPath] {
		if onProgress != nil {
			onProgress(p.done, p.total)
		}
	}
	if err := f.errs[srcPath]; err != nil {
		return model.PptxSplitResult{}, err
	}
	return f.results[srcPath], nil
}

// newBatchTestService 构造带 fake 出口的批量测试服务（对齐现有
// &PptxSplitService{provider: fake} + SetEventSink 测试构造范式）。
func newBatchTestService(t *testing.T, fake *fakeBatchPptxProvider) (*PptxSplitService, *fakeEventSink) {
	t.Helper()
	svc := &PptxSplitService{provider: fake}
	sink := &fakeEventSink{}
	svc.SetEventSink(sink)
	fake.svc = svc
	return svc, sink
}

// batchFixtureDir 造批量 fixture 目录：files 中的 rel 路径写哑文件（ScanBytes
// 不做真实拆分，fake provider 直接返回结果），返回目录绝对路径。
func batchFixtureDir(t *testing.T, rels ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, rel := range rels {
		testutil.WriteFile(t, filepath.Join(dir, filepath.FromSlash(rel)), "dummy pptx bytes")
	}
	return dir
}

// TestSplitFolder_SerialOrderAndSummary 串行编排主链路：按扫描字典序逐文件调
// provider（顺序可预期），汇总含全部文件的成功明细；调用方 ctx 派生后下发。
func TestSplitFolder_SerialOrderAndSummary(t *testing.T) {
	dir := batchFixtureDir(t, "b.pptx", "a.pptx", "子/子.pptx")
	fake := &fakeBatchPptxProvider{results: map[string]model.PptxSplitResult{}}
	for _, name := range []string{"a.pptx", "b.pptx", filepath.Join("子", "子.pptx")} {
		p := filepath.Join(dir, name)
		fake.results[p] = model.PptxSplitResult{OutputDir: filepath.Join(dir, strings.TrimSuffix(name, ".pptx")), PageCount: 5}
	}
	svc, _ := newBatchTestService(t, fake)

	got, err := svc.SplitFolder(context.Background(), dir)
	if err != nil {
		t.Fatalf("SplitFolder: %v", err)
	}
	if got.DirPath != dir {
		t.Errorf("DirPath = %q, want %q", got.DirPath, dir)
	}
	if got.Total != 3 || len(got.Items) != 3 {
		t.Fatalf("Total/Items 应为 3，实际 Total=%d Items=%d", got.Total, len(got.Items))
	}
	// WalkDir 字典序：a.pptx → b.pptx → 子/子.pptx（目录排在同名文件之后逐层下沉）
	wantOrder := []string{
		filepath.Join(dir, "a.pptx"),
		filepath.Join(dir, "b.pptx"),
		filepath.Join(dir, "子", "子.pptx"),
	}
	for i, want := range wantOrder {
		if fake.calls[i] != want {
			t.Errorf("第 %d 个拆分调用 = %q, want %q（应按扫描字典序串行）", i, fake.calls[i], want)
		}
		if got.Items[i].SrcPath != want {
			t.Errorf("Items[%d].SrcPath = %q, want %q", i, got.Items[i].SrcPath, want)
		}
		if got.Items[i].Error != "" {
			t.Errorf("Items[%d] 成功文件不应有 Error，实际 %q", i, got.Items[i].Error)
		}
		if got.Items[i].PageCount != 5 || got.Items[i].OutputDir != fake.results[want].OutputDir {
			t.Errorf("Items[%d] 成功明细透传不符：%+v", i, got.Items[i])
		}
	}
}

// TestSplitFolder_SingleFailureContinues 单文件失败不中断：失败原因（用户可读
// 消息）记入该文件 Item.Error，其余文件继续拆分，整批不报错。
func TestSplitFolder_SingleFailureContinues(t *testing.T) {
	dir := batchFixtureDir(t, "a.pptx", "b.pptx", "c.pptx")
	fake := &fakeBatchPptxProvider{
		results: map[string]model.PptxSplitResult{
			filepath.Join(dir, "a.pptx"): {OutputDir: "out-a", PageCount: 2},
			filepath.Join(dir, "c.pptx"): {OutputDir: "out-c", PageCount: 4},
		},
		errs: map[string]error{
			filepath.Join(dir, "b.pptx"): fmt.Errorf("文件已损坏: %w", errors.New("zip: not a valid zip")),
		},
	}
	svc, _ := newBatchTestService(t, fake)

	got, err := svc.SplitFolder(context.Background(), dir)
	if err != nil {
		t.Fatalf("单文件失败不应中断整批，实际：%v", err)
	}
	if got.Total != 3 || len(got.Items) != 3 {
		t.Fatalf("3 个文件均应有结果项，实际 Total=%d Items=%d", got.Total, len(got.Items))
	}
	if len(fake.calls) != 3 {
		t.Errorf("失败后应继续拆分（3 次调用），实际 %d 次：%v", len(fake.calls), fake.calls)
	}
	if got.Items[0].Error != "" || got.Items[0].PageCount != 2 {
		t.Errorf("Items[0] 应成功：%+v", got.Items[0])
	}
	if got.Items[1].Error == "" || !strings.Contains(got.Items[1].Error, "文件已损坏") {
		t.Errorf("Items[1].Error 应记用户可读失败原因，实际 %q", got.Items[1].Error)
	}
	if got.Items[1].OutputDir != "" || got.Items[1].PageCount != 0 {
		t.Errorf("Items[1] 失败时输出明细应为零值：%+v", got.Items[1])
	}
	if got.Items[2].Error != "" || got.Items[2].PageCount != 4 {
		t.Errorf("Items[2] 应在失败后继续成功：%+v", got.Items[2])
	}
}

// TestSplitFolder_ValidationErrors 入参校验：空路径、目录不存在、路径为文件
// 均返回引导级错误且不触发任何拆分。
func TestSplitFolder_ValidationErrors(t *testing.T) {
	fake := &fakeBatchPptxProvider{}
	svc, _ := newBatchTestService(t, fake)

	if _, err := svc.SplitFolder(context.Background(), "  "); err == nil || !strings.Contains(err.Error(), "请先选择") {
		t.Errorf("空路径应引导选择文件夹，实际：%v", err)
	}
	if _, err := svc.SplitFolder(context.Background(), filepath.Join(t.TempDir(), "不存在")); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Errorf("目录不存在应报引导错误，实际：%v", err)
	}
	filePath := filepath.Join(t.TempDir(), "普通文件.pptx")
	testutil.WriteFile(t, filePath, "dummy")
	if _, err := svc.SplitFolder(context.Background(), filePath); err == nil || !strings.Contains(err.Error(), "不是文件夹") {
		t.Errorf("文件路径应报「不是文件夹」，实际：%v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("校验失败不应触发拆分，实际调用：%v", fake.calls)
	}
}

// TestSplitFolder_EmptyScan 递归扫描为空（无可拆 .pptx）返回引导级错误。
func TestSplitFolder_EmptyScan(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteFile(t, filepath.Join(dir, "说明.txt"), "非 PPT")

	fake := &fakeBatchPptxProvider{}
	svc, _ := newBatchTestService(t, fake)
	_, err := svc.SplitFolder(context.Background(), dir)
	if err == nil || !strings.Contains(err.Error(), "未找到 PPT 文件") {
		t.Fatalf("空扫描应报引导错误，实际：%v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("空扫描不应触发拆分，实际调用：%v", fake.calls)
	}
}

// TestSplitFolder_PreCanceledContext ctx 预先取消：不触发任何拆分，错误映射为
// E_GIT_CANCELLED AppError（对齐 git 取消测试的 ctx 预取消构造手法）。
func TestSplitFolder_PreCanceledContext(t *testing.T) {
	dir := batchFixtureDir(t, "a.pptx")
	fake := &fakeBatchPptxProvider{}
	svc, _ := newBatchTestService(t, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := svc.SplitFolder(ctx, dir)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeGitCancelled {
		t.Fatalf("取消错误应映射为 E_GIT_CANCELLED AppError，实际：%v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("已取消的批次不应开始任何文件拆分，实际调用：%v", fake.calls)
	}
	if got.Total != 1 || len(got.Items) != 0 {
		t.Errorf("未开始任何文件时 Items 应为空，实际：%+v", got)
	}
}

// TestSplitFolder_CancelMidBatch 批量取消（当前文件被中止）：取消错误不算单文件
// 业务失败，整批经 mapCancelError 映射 E_GIT_CANCELLED 上抛；已完成 Items 保留、
// 剩余文件不再开始；返回后注册表无残留。
func TestSplitFolder_CancelMidBatch(t *testing.T) {
	dir := batchFixtureDir(t, "a.pptx", "b.pptx", "c.pptx")
	fake := &fakeBatchPptxProvider{
		results: map[string]model.PptxSplitResult{
			filepath.Join(dir, "a.pptx"): {OutputDir: "out-a", PageCount: 3},
		},
		errs: map[string]error{
			// b 拆分中途被取消：核心取消路径的 %w 包装形态
			filepath.Join(dir, "b.pptx"): fmt.Errorf("拆分已取消: %w", context.Canceled),
		},
		cancelDuring: map[string]bool{filepath.Join(dir, "b.pptx"): true},
	}
	svc, _ := newBatchTestService(t, fake)

	got, err := svc.SplitFolder(context.Background(), dir)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeGitCancelled {
		t.Fatalf("批量取消应映射为 E_GIT_CANCELLED AppError，实际：%v", err)
	}
	if len(fake.calls) != 2 {
		t.Errorf("取消后剩余文件不应开始（应恰 2 次调用 a、b），实际：%v", fake.calls)
	}
	if got.Total != 3 || len(got.Items) != 1 {
		t.Fatalf("已完成文件应保留（1 项），实际 Total=%d Items=%d", got.Total, len(got.Items))
	}
	if got.Items[0].SrcPath != filepath.Join(dir, "a.pptx") || got.Items[0].PageCount != 3 {
		t.Errorf("已完成 Items[0] 应为 a.pptx 的成功明细：%+v", got.Items[0])
	}
	if svc.CancelPptxSplit() {
		t.Error("SplitFolder 返回后注册表应无残留（再次取消应返回 false）")
	}
}

// TestSplitFolder_CancelBetweenFiles 批量取消（当前文件完成后、下一文件开始前）：
// 循环顶部 ctx 检查兜底——已完成的当前文件计入 Items，剩余文件不再开始。
func TestSplitFolder_CancelBetweenFiles(t *testing.T) {
	dir := batchFixtureDir(t, "a.pptx", "b.pptx", "c.pptx")
	fake := &fakeBatchPptxProvider{
		results: map[string]model.PptxSplitResult{
			filepath.Join(dir, "a.pptx"): {OutputDir: "out-a", PageCount: 1},
			filepath.Join(dir, "b.pptx"): {OutputDir: "out-b", PageCount: 2},
		},
		cancelDuring: map[string]bool{filepath.Join(dir, "b.pptx"): true},
	}
	svc, _ := newBatchTestService(t, fake)

	got, err := svc.SplitFolder(context.Background(), dir)
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeGitCancelled {
		t.Fatalf("批量取消应映射为 E_GIT_CANCELLED AppError，实际：%v", err)
	}
	if len(fake.calls) != 2 {
		t.Errorf("取消后剩余文件不应开始（应恰 2 次调用 a、b），实际：%v", fake.calls)
	}
	if got.Total != 3 || len(got.Items) != 2 {
		t.Fatalf("已完成文件应保留（2 项），实际 Total=%d Items=%d", got.Total, len(got.Items))
	}
	if got.Items[1].Error != "" {
		t.Errorf("取消前已完成的 b.pptx 不应记业务失败：%+v", got.Items[1])
	}
	if svc.CancelPptxSplit() {
		t.Error("SplitFolder 返回后注册表应无残留")
	}
}

// TestSplitFolder_BatchProgressEvents 双层进度事件：每文件开始前推 1 条
// pptx-split:batch:progress（index 从 1 计），文件内页级沿用 pptx-split:progress
// 载荷不变；批量模式不逐文件发 done/error 事件（终态经返回值汇总）。
func TestSplitFolder_BatchProgressEvents(t *testing.T) {
	dir := batchFixtureDir(t, "a.pptx", "b.pptx")
	fake := &fakeBatchPptxProvider{
		results: map[string]model.PptxSplitResult{
			filepath.Join(dir, "a.pptx"): {OutputDir: "out-a", PageCount: 2},
			filepath.Join(dir, "b.pptx"): {OutputDir: "out-b", PageCount: 3},
		},
		progress: map[string][]fakeProgressCall{
			filepath.Join(dir, "a.pptx"): {{done: 1, total: 2}, {done: 2, total: 2}},
			filepath.Join(dir, "b.pptx"): {{done: 1, total: 3}, {done: 2, total: 3}, {done: 3, total: 3}},
		},
	}
	svc, sink := newBatchTestService(t, fake)

	if _, err := svc.SplitFolder(context.Background(), dir); err != nil {
		t.Fatalf("SplitFolder: %v", err)
	}

	// 事件序列：batch:progress(a) + 2 页 progress + batch:progress(b) + 3 页 progress = 7 条
	if len(sink.calls) != 7 {
		t.Fatalf("应收到 7 条事件（2 batch:progress + 5 progress），实际 %d 条：%#v", len(sink.calls), sink.calls)
	}
	type batchCall struct {
		index int
		src   string
	}
	wantBatches := []batchCall{{index: 1, src: filepath.Join(dir, "a.pptx")}, {index: 2, src: filepath.Join(dir, "b.pptx")}}
	for i, want := range wantBatches {
		call := sink.calls[i*3] // batch:progress 恒在每文件页级事件之前
		if call.name != "pptx-split:batch:progress" {
			t.Fatalf("第 %d 条事件应为 pptx-split:batch:progress，实际 %q", i*3, call.name)
		}
		payload, ok := call.data[0].(map[string]any)
		if !ok || payload["dirPath"] != dir || payload["index"] != want.index || payload["total"] != 2 || payload["currentFile"] != want.src {
			t.Errorf("batch:progress 载荷不符（期望 index=%d currentFile=%q）：%#v", want.index, want.src, call.data)
		}
	}
	for i, call := range sink.calls {
		if call.name == "pptx-split:done" || call.name == "pptx-split:error" {
			t.Errorf("批量模式不应发单文件终态事件，第 %d 条为 %q", i, call.name)
		}
	}
	// 页级事件载荷与单文件形态逐字一致（单文件消费方不受影响）
	pageCalls := []*fakeEmitCall{&sink.calls[1], &sink.calls[2], &sink.calls[4], &sink.calls[5], &sink.calls[6]}
	wantPages := []fakeProgressCall{{1, 2}, {2, 2}, {1, 3}, {2, 3}, {3, 3}}
	for i, call := range pageCalls {
		if call.name != "pptx-split:progress" {
			t.Errorf("第 %d 条页级事件名不符：%q", i, call.name)
		}
		payload, ok := call.data[0].(map[string]any)
		wantSrc := filepath.Join(dir, "a.pptx")
		if i >= 2 {
			wantSrc = filepath.Join(dir, "b.pptx")
		}
		if !ok || payload["srcPath"] != wantSrc || payload["done"] != wantPages[i].done || payload["total"] != wantPages[i].total {
			t.Errorf("页级 progress 载荷不符（期望 %v src=%q）：%#v", wantPages[i], wantSrc, call.data)
		}
	}
}
