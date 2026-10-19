package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"workbench/model"
)

// ---------- 购物栏存储（Add/List/Remove/Clear/Reorder/持久化/失效探测）用例 ----------

func cartItem(libID string, fileID int64, slideNum int) model.AssetCartItem {
	return model.AssetCartItem{
		LibraryID:   libID,
		FileID:      fileID,
		SlideNum:    slideNum,
		SrcPath:     filepath.Join("D:", "assets", libID, "deck.pptx"),
		FileName:    "deck.pptx",
		SlideTitle:  "第页标题",
		LibraryName: "库名" + libID,
	}
}

func newTestCartService(t *testing.T) (*AssetCartService, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "asset_cart.json")
	svc := NewAssetCartService(path, nil)
	return svc, path
}

func TestAssetCart_AddDedupIdempotent(t *testing.T) {
	svc, _ := newTestCartService(t)

	result, err := svc.Add([]model.AssetCartItem{cartItem("lib-1", 11, 1), cartItem("lib-1", 11, 2)})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if result.Added != 2 || result.Duplicates != 0 {
		t.Fatalf("首次加入计数不符: %+v", result)
	}

	// 重复加入同键：全部去重，幂等返回提示语义计数
	result, err = svc.Add([]model.AssetCartItem{cartItem("lib-1", 11, 1), cartItem("lib-2", 22, 1)})
	if err != nil {
		t.Fatalf("Add 重复: %v", err)
	}
	if result.Added != 1 || result.Duplicates != 1 {
		t.Fatalf("去重计数不符: %+v", result)
	}

	items, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("条目数应 3，实际 %d", len(items))
	}
	// 入参顺序追加语义 + 键形态
	wantKeys := []string{"lib-1|11|1", "lib-1|11|2", "lib-2|22|1"}
	for i, want := range wantKeys {
		if items[i].Key() != want {
			t.Fatalf("第 %d 条键应 %s，实际 %s", i, want, items[i].Key())
		}
	}
	if items[0].AddedAt == "" {
		t.Fatal("AddedAt 应由后端权威生成")
	}
}

func TestAssetCart_AddInvalidItemRejected(t *testing.T) {
	svc, _ := newTestCartService(t)
	if _, err := svc.Add([]model.AssetCartItem{{LibraryID: "", FileID: 1, SlideNum: 1}}); err == nil {
		t.Fatal("libraryId 空应拒绝")
	}
	if _, err := svc.Add([]model.AssetCartItem{{LibraryID: "lib", FileID: 0, SlideNum: 1}}); err == nil {
		t.Fatal("fileId 非法应拒绝")
	}
	if _, err := svc.Add([]model.AssetCartItem{{LibraryID: "lib", FileID: 1, SlideNum: 0}}); err == nil {
		t.Fatal("页码非法应拒绝")
	}
}

func TestAssetCart_PersistenceRoundTrip(t *testing.T) {
	svc, path := newTestCartService(t)
	if _, err := svc.Add([]model.AssetCartItem{cartItem("lib-1", 11, 2), cartItem("lib-2", 33, 5)}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// 新实例同路径懒加载：持久化含顺序
	svc2 := NewAssetCartService(path, nil)
	items, err := svc2.List()
	if err != nil {
		t.Fatalf("重开 List: %v", err)
	}
	if len(items) != 2 || items[0].Key() != "lib-1|11|2" || items[1].Key() != "lib-2|33|5" {
		t.Fatalf("重开读取顺序不符: %+v", items)
	}
	if items[0].LibraryName != "库名lib-1" {
		t.Fatalf("快照字段应持久化: %+v", items[0])
	}
}

func TestAssetCart_CorruptFileDegradesEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "asset_cart.json")
	if err := os.WriteFile(path, []byte("{半截 JSON"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := NewAssetCartService(path, nil)
	items, err := svc.List()
	if err != nil {
		t.Fatalf("损坏降级不应报错: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("降级应回退空购物栏，实际 %d 条", len(items))
	}
	// 降级后下一次写入即以空购物栏重建文件
	if _, err := svc.Add([]model.AssetCartItem{cartItem("lib-1", 11, 1)}); err != nil {
		t.Fatalf("降级后写入: %v", err)
	}
}

func TestAssetCart_Remove(t *testing.T) {
	svc, _ := newTestCartService(t)
	if _, err := svc.Add([]model.AssetCartItem{cartItem("lib-1", 11, 1), cartItem("lib-1", 11, 2)}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	if err := svc.Remove([]string{"lib-1|11|1", "不存在的键"}); err != nil {
		t.Fatalf("Remove 容忍未知键: %v", err)
	}
	items, _ := svc.List()
	if len(items) != 1 || items[0].Key() != "lib-1|11|2" {
		t.Fatalf("移除后剩余不符: %+v", items)
	}

	// 全部键未命中：报错提示刷新
	if err := svc.Remove([]string{"不存在的键"}); err == nil {
		t.Fatal("全未命中应报错")
	}
}

func TestAssetCart_ClearAndReorder(t *testing.T) {
	svc, _ := newTestCartService(t)
	if _, err := svc.Add([]model.AssetCartItem{cartItem("lib-1", 11, 1), cartItem("lib-1", 11, 2), cartItem("lib-2", 22, 1)}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// 拖拽排序：条目键完整排列回写
	if err := svc.Reorder([]string{"lib-2|22|1", "lib-1|11|1", "lib-1|11|2"}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	items, _ := svc.List()
	if items[0].Key() != "lib-2|22|1" || items[2].Key() != "lib-1|11|2" {
		t.Fatalf("重排后顺序不符: %+v", items)
	}

	// 长度不符 / 未知键拒绝（防静默丢条目）
	if err := svc.Reorder([]string{"lib-1|11|1"}); err == nil {
		t.Fatal("长度不符应拒绝")
	}
	if err := svc.Reorder([]string{"lib-2|22|1", "lib-1|11|1", "未知键"}); err == nil {
		t.Fatal("未知键应拒绝")
	}
	if err := svc.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	items, _ = svc.List()
	if len(items) != 0 {
		t.Fatalf("清空后应为空，实际 %d 条", len(items))
	}
}

func TestAssetCart_ListStaleDetection(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "存在.pptx")
	if err := os.WriteFile(existing, []byte("zip"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := NewAssetCartService(filepath.Join(dir, "asset_cart.json"), nil)
	if _, err := svc.Add([]model.AssetCartItem{
		{LibraryID: "lib", FileID: 1, SlideNum: 1, SrcPath: existing},
		{LibraryID: "lib", FileID: 2, SlideNum: 1, SrcPath: filepath.Join(dir, "已删除.pptx")},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	items, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if items[0].Stale {
		t.Fatal("存在的源文件不应标 stale")
	}
	if !items[1].Stale {
		t.Fatal("源文件不存在的条目应标 stale")
	}
	// stale 为探测态非持久化：Add 落盘的条目本体不带该标志
	raw, err := os.ReadFile(filepath.Join(dir, "asset_cart.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"stale"`) {
		t.Fatal("stale 探测态不应持久化进文件")
	}
}

// ---------- 合并导出编排（fake provider 注入）用例 ----------

// fakeCartMergeProvider 可控合并 provider：阻塞/错误注入驱动导出编排各分支
// （合并语义本身由核心 fixture 测试覆盖，此处只验证编排透传与互斥/取消收口）。
type fakeCartMergeProvider struct {
	mu               sync.Mutex
	closeStartedOnce sync.Once
	started          chan struct{} // Merge 首次被调用时关闭（驱动方等待导出确已启动；复调用幂等）
	release          chan struct{} // 阻塞型用例：关闭放行返回（nil = 不阻塞）
	gotItems         []model.PptxMergeItem
	gotPath          string
	result           model.PptxMergeResult
	err              error
}

func (p *fakeCartMergeProvider) Merge(ctx context.Context, items []model.PptxMergeItem, outPath string, _ func(done, total int)) (model.PptxMergeResult, error) {
	p.mu.Lock()
	p.gotItems = append([]model.PptxMergeItem(nil), items...)
	p.gotPath = outPath
	p.mu.Unlock()
	p.closeStartedOnce.Do(func() { close(p.started) })
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return model.PptxMergeResult{}, fmtWrapCancel(ctx.Err())
		}
	}
	if err := ctx.Err(); err != nil {
		return model.PptxMergeResult{}, fmtWrapCancel(err)
	}
	return p.result, p.err
}

// fmtWrapCancel 复刻核心取消错误形态（%w 包装 context.Canceled，保留 errors.Is
// 穿透供 mapCancelError 判定）。
func fmtWrapCancel(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("合并导出已取消: %w", err)
}

func newFakeMergeSvc(block bool) (*PptxMergeService, *fakeCartMergeProvider) {
	svc := NewPptxMergeService(context.Background())
	p := &fakeCartMergeProvider{started: make(chan struct{})}
	if block {
		p.release = make(chan struct{})
	}
	svc.provider = p
	return svc, p
}

func newTestCartWithMerge(t *testing.T, block bool) (*AssetCartService, *fakeCartMergeProvider) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "asset_cart.json")
	mergeSvc, fake := newFakeMergeSvc(block)
	svc := NewAssetCartService(path, mergeSvc)
	if _, err := svc.Add([]model.AssetCartItem{
		{LibraryID: "lib", FileID: 11, SlideNum: 2, SrcPath: "D:/a.pptx"},
		{LibraryID: "lib", FileID: 22, SlideNum: 1, SrcPath: "D:/b.pptx"},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	return svc, fake
}

func TestAssetCart_MergePassesCartOrderToProvider(t *testing.T) {
	svc, fake := newTestCartWithMerge(t, false)
	fake.result = model.PptxMergeResult{OutputPath: "D:/out.pptx", PageCount: 2}

	result, err := svc.MergeCartToPptx(context.Background(), "D:/out.pptx")
	if err != nil {
		t.Fatalf("MergeCartToPptx: %v", err)
	}
	if result.PageCount != 2 {
		t.Fatalf("结果透传不符: %+v", result)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.gotPath != "D:/out.pptx" {
		t.Fatalf("输出路径不符: %s", fake.gotPath)
	}
	// 清单 = 购物栏顺序（(SrcPath, PageNum) 逐项对齐）
	if len(fake.gotItems) != 2 ||
		fake.gotItems[0].SrcPath != "D:/a.pptx" || fake.gotItems[0].PageNum != 2 ||
		fake.gotItems[1].SrcPath != "D:/b.pptx" || fake.gotItems[1].PageNum != 1 {
		t.Fatalf("合并清单与购物栏顺序不符: %+v", fake.gotItems)
	}
}

func TestAssetCart_MergeEmptyCartRejected(t *testing.T) {
	svc, _ := newTestCartService(t)
	if _, err := svc.MergeCartToPptx(context.Background(), "D:/out.pptx"); err == nil {
		t.Fatal("空购物栏应报引导错误")
	}
}

func TestAssetCart_MergeReentrantRejectedAndReset(t *testing.T) {
	mergeSvc, fake := newFakeMergeSvc(true)
	path := filepath.Join(t.TempDir(), "asset_cart.json")
	svc := NewAssetCartService(path, mergeSvc)
	if _, err := svc.Add([]model.AssetCartItem{{LibraryID: "lib", FileID: 11, SlideNum: 1, SrcPath: "D:/a.pptx"}}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	done := make(chan struct{})
	var firstErr error
	go func() {
		defer close(done)
		_, firstErr = svc.MergeCartToPptx(context.Background(), "D:/out.pptx")
	}()
	select {
	case <-fake.started:
	case <-time.After(2 * time.Second):
		t.Fatal("导出未启动")
	}

	// 在途导出期间二次导出：E_CART_EXPORT_IN_PROGRESS（warning 引导级）
	_, err := svc.MergeCartToPptx(context.Background(), "D:/out2.pptx")
	var appErr *model.AppError
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeCartExportInProgress {
		t.Fatalf("二次导出应报 E_CART_EXPORT_IN_PROGRESS，实际: %v", err)
	}

	// List/Add 导出期间仍可用（导出执行在锁外，快照语义）
	if _, err := svc.List(); err != nil {
		t.Fatalf("导出期间 List 应可用: %v", err)
	}

	close(fake.release)
	<-done
	if firstErr != nil {
		t.Fatalf("首次导出应成功: %v", firstErr)
	}
	// 导出结束互斥位复位：可再次导出
	if _, err := svc.MergeCartToPptx(context.Background(), "D:/out3.pptx"); err != nil {
		t.Fatalf("导出结束后再导出应放行: %v", err)
	}
}

func TestAssetCart_CancelExport(t *testing.T) {
	mergeSvc, fake := newFakeMergeSvc(true)
	path := filepath.Join(t.TempDir(), "asset_cart.json")
	svc := NewAssetCartService(path, mergeSvc)
	if _, err := svc.Add([]model.AssetCartItem{{LibraryID: "lib", FileID: 11, SlideNum: 1, SrcPath: "D:/a.pptx"}}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	done := make(chan struct{})
	var mergeErr error
	go func() {
		defer close(done)
		_, mergeErr = svc.MergeCartToPptx(context.Background(), "D:/out.pptx")
	}()
	select {
	case <-fake.started:
	case <-time.After(2 * time.Second):
		t.Fatal("导出未启动")
	}

	if !svc.CancelAssetCartExport() {
		t.Fatal("在途导出取消应返回 true")
	}
	<-done
	// 取消错误经 mapCancelError 统一映射 E_GIT_CANCELLED（warning 中性提示）
	var appErr *model.AppError
	if !errors.As(mergeErr, &appErr) || appErr.Code != model.ErrCodeGitCancelled {
		t.Fatalf("取消应映射 E_GIT_CANCELLED，实际: %v", mergeErr)
	}
	// 无在途操作幂等 false
	if svc.CancelAssetCartExport() {
		t.Fatal("无在途导出取消应返回 false")
	}
}
