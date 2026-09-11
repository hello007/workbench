package service

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"workbench/model"
)

// ===== FileTreeCache 单元测试 =====

func TestFileTreeCache_HitAndMiss(t *testing.T) {
	cache := NewFileTreeCache()
	mtime := time.Now()
	nodes := []*model.FileTreeNode{model.NewFileTreeNode("a", "/p/a", "file")}

	// 未写入前应未命中
	if _, ok := cache.get("/p", mtime); ok {
		t.Error("未写入前不应命中")
	}

	cache.set("/p", mtime, nodes)

	// mtime 相同应命中
	got, ok := cache.get("/p", mtime)
	if !ok {
		t.Error("写入后应命中")
	}
	if len(got) != 1 || got[0].Name != "a" {
		t.Errorf("命中数据异常: %+v", got)
	}
}

func TestFileTreeCache_MtimeChangeInvalidates(t *testing.T) {
	cache := NewFileTreeCache()
	mtime1 := time.Now()
	cache.set("/p", mtime1, []*model.FileTreeNode{model.NewFileTreeNode("a", "/p/a", "file")})

	// mtime 变化（目录条目增删）-> 缓存失效
	mtime2 := mtime1.Add(time.Second)
	if _, ok := cache.get("/p", mtime2); ok {
		t.Error("mtime 变化应使缓存失效")
	}
}

func TestFileTreeCache_TTLExpiry(t *testing.T) {
	cache := NewFileTreeCache()
	mtime := time.Now()
	cache.set("/p", mtime, []*model.FileTreeNode{model.NewFileTreeNode("a", "/p/a", "file")})

	// 命中
	if _, ok := cache.get("/p", mtime); !ok {
		t.Error("TTL 内应命中")
	}

	// 手动将 cachedAt 置为过期，模拟 TTL 超时
	cache.mu.Lock()
	e := cache.entries["/p"]
	e.cachedAt = time.Now().Add(-fileTreeCacheTTL - time.Second)
	cache.entries["/p"] = e
	cache.mu.Unlock()

	// TTL 过期 -> 未命中
	if _, ok := cache.get("/p", mtime); ok {
		t.Error("TTL 过期应未命中")
	}

	// #1: TTL 过期不仅判 miss，还应驱逐条目（delete），避免内存与历史访问目录数成正比无界增长
	cache.mu.Lock()
	_, stillExists := cache.entries["/p"]
	cache.mu.Unlock()
	if stillExists {
		t.Error("TTL 过期后条目应从 map 驱逐，不应常驻至 refreshAll 全清")
	}
}

func TestFileTreeCache_ClearPath(t *testing.T) {
	cache := NewFileTreeCache()
	mtime := time.Now()
	cache.set("/p1", mtime, []*model.FileTreeNode{model.NewFileTreeNode("a", "/p1/a", "file")})
	cache.set("/p2", mtime, []*model.FileTreeNode{model.NewFileTreeNode("b", "/p2/b", "file")})

	cache.clearPath("/p1")

	if _, ok := cache.get("/p1", mtime); ok {
		t.Error("clearPath 后 /p1 应未命中")
	}
	if _, ok := cache.get("/p2", mtime); !ok {
		t.Error("clearPath(/p1) 不应影响 /p2")
	}
}

func TestFileTreeCache_ClearAll(t *testing.T) {
	cache := NewFileTreeCache()
	mtime := time.Now()
	cache.set("/p1", mtime, []*model.FileTreeNode{model.NewFileTreeNode("a", "/p1/a", "file")})
	cache.set("/p2", mtime, []*model.FileTreeNode{model.NewFileTreeNode("b", "/p2/b", "file")})

	cache.clearAll()

	if _, ok := cache.get("/p1", mtime); ok {
		t.Error("clearAll 后 /p1 应未命中")
	}
	if _, ok := cache.get("/p2", mtime); ok {
		t.Error("clearAll 后 /p2 应未命中")
	}
}

// TestFileTreeCache_GetReturnsDeepCopy 验证 get 返回深拷贝，
// 调用方修改返回节点不污染缓存内部数据。
func TestFileTreeCache_GetReturnsDeepCopy(t *testing.T) {
	cache := NewFileTreeCache()
	mtime := time.Now()
	cache.set("/p", mtime, []*model.FileTreeNode{
		{
			Name:     "parent",
			Path:     "/p/parent",
			Type:     "directory",
			Children: []*model.FileTreeNode{model.NewFileTreeNode("child", "/p/parent/child", "file")},
		},
	})

	got, ok := cache.get("/p", mtime)
	if !ok {
		t.Fatal("应命中")
	}

	// 修改返回的节点（模拟 buildTree 填充 Children 或调用方篡改）
	got[0].Name = "mutated"
	got[0].Children[0].Name = "mutated-child"
	got = append(got, model.NewFileTreeNode("extra", "/p/extra", "file"))

	// 再次取，缓存应未受污染
	got2, _ := cache.get("/p", mtime)
	if got2[0].Name != "parent" {
		t.Errorf("缓存被污染: got %q want parent", got2[0].Name)
	}
	if got2[0].Children[0].Name != "child" {
		t.Errorf("缓存子节点被污染: got %q want child", got2[0].Children[0].Name)
	}
	if len(got2) != 1 {
		t.Errorf("缓存长度被污染: got %d want 1", len(got2))
	}
}

// TestFileTreeCache_SetStoresDeepCopy 验证 set 存储深拷贝，
// 调用方修改原始节点（set 之后）不污染缓存。
func TestFileTreeCache_SetStoresDeepCopy(t *testing.T) {
	cache := NewFileTreeCache()
	mtime := time.Now()
	original := []*model.FileTreeNode{model.NewFileTreeNode("orig", "/p/orig", "file")}
	cache.set("/p", mtime, original)

	// 修改原始节点
	original[0].Name = "mutated"
	original = append(original, model.NewFileTreeNode("extra", "/p/extra", "file"))

	got, _ := cache.get("/p", mtime)
	if got[0].Name != "orig" {
		t.Errorf("缓存被原始节点污染: got %q want orig", got[0].Name)
	}
	if len(got) != 1 {
		t.Errorf("缓存长度被原始节点污染: got %d want 1", len(got))
	}
}

// TestFileTreeCache_Concurrent 并发 get/set/clearPath/clearAll 不触发 map 读写竞态。
// 需配合 `go test -race` 运行以检测数据竞争。
func TestFileTreeCache_Concurrent(t *testing.T) {
	cache := NewFileTreeCache()
	mtime := time.Now()

	var wg sync.WaitGroup
	// 并发 set + get 同一组路径
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := fmt.Sprintf("/p/dir-%d", i%10)
			cache.set(path, mtime, []*model.FileTreeNode{model.NewFileTreeNode("n", path+"/n", "file")})
			cache.get(path, mtime)
			cache.clearPath(path)
		}(i)
	}
	// 并发 clearAll
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.clearAll()
		}()
	}
	wg.Wait()
}

// ===== FileTreeService 缓存集成测试 =====

// TestGetChildren_CachePopulatedAndHit 首次调用实扫并回写缓存，
// 第二次调用命中缓存返回等价数据（深拷贝，独立引用）。
func TestGetChildren_CachePopulatedAndHit(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("a"))
	svc := NewFileTreeService()

	nodes1, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("first GetChildren: %v", err)
	}
	if len(nodes1) != 1 || nodes1[0].Name != "a.txt" {
		t.Fatalf("first call unexpected: %+v", nodes1)
	}

	// 验证缓存已写入
	abs, _ := filepath.Abs(dir)
	info, _ := os.Stat(dir)
	if _, ok := svc.treeCache.get(abs, info.ModTime()); !ok {
		t.Error("缓存应已写入")
	}

	// 第二次调用：应命中缓存，返回等价数据
	nodes2, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("second GetChildren: %v", err)
	}
	if len(nodes2) != 1 || nodes2[0].Name != "a.txt" {
		t.Fatalf("second call unexpected: %+v", nodes2)
	}
	// 深拷贝：首次返回原始节点，二次命中返回深拷贝，引用应不同
	if nodes1[0] == nodes2[0] {
		t.Error("缓存命中应返回深拷贝，不应共享引用")
	}
}

// TestGetChildren_CacheInvalidatedOnMtimeChange 注入记录旧 mtime 的陈旧缓存，
// 验证 GetChildren 比对当前目录 mtime 不等时走失效重扫分支。
//
// 规避 NTFS mtime 时序 flaky：原写法依赖 mustWriteFile 触发目录 mtime 变化来失效
// 缓存，但 NTFS 目录 mtime 分辨率约 1 秒，连续写文件可能落在同一 tick 导致目录
// mtime 未变、缓存命中返回旧节点数而间歇性 FAIL（跨秒重跑又 PASS）。改为注入一个
// modTime 明确早于当前 mtime 的陈旧缓存，直接驱动 get 的 modTime.Equal 判定走
// 失效分支，不依赖文件系统 tick 时序，确定性 PASS。
func TestGetChildren_CacheInvalidatedOnMtimeChange(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("a"))
	svc := NewFileTreeService()

	// 首次扫描回写真实缓存（modTime = 目录当前 mtime），确认基线 1 节点
	nodes1, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("first GetChildren: %v", err)
	}
	if len(nodes1) != 1 {
		t.Fatalf("first call: %d nodes", len(nodes1))
	}

	// 新增文件（目录内容已变；不依赖其是否更新目录 mtime）
	mustWriteFile(t, filepath.Join(dir, "b.txt"), []byte("b"))

	abs, _ := filepath.Abs(dir)
	info, _ := os.Stat(dir)
	curMtime := info.ModTime()

	// 注入陈旧缓存：modTime 设为明显早于当前 mtime 的旧值，节点名与实际不符，
	// 模拟“缓存记录 mtime 落后于目录当前 mtime”的失效场景。
	staleMtime := curMtime.Add(-time.Hour)
	staleNodes := []*model.FileTreeNode{model.NewFileTreeNode("stale.txt", filepath.Join(dir, "stale.txt"), "file")}
	svc.treeCache.set(abs, staleMtime, staleNodes)

	// get 判 staleMtime != curMtime -> 缓存失效 -> miss 重扫返回真实 2 节点
	nodes2, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("second GetChildren: %v", err)
	}
	if len(nodes2) != 2 {
		t.Errorf("mtime 变化应使缓存失效重扫: got %d nodes, want 2", len(nodes2))
	}
	// 确认返回真实数据而非陈旧缓存
	for _, n := range nodes2 {
		if n.Name == "stale.txt" {
			t.Error("缓存未失效，返回了陈旧缓存的 stale.txt")
		}
	}
}

// TestGetChildren_ReturnsIndependentCopy 首次调用返回的原始节点被修改后，
// 缓存应未受污染（buildTree 填充 Children 的场景）。
func TestGetChildren_ReturnsIndependentCopy(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "folder"))
	svc := NewFileTreeService()

	nodes1, _ := svc.GetChildren(dir)
	// 模拟 buildTree 填充 Children 与篡改字段
	nodes1[0].Children = []*model.FileTreeNode{model.NewFileTreeNode("injected", "/x", "file")}
	nodes1[0].Name = "mutated"

	// 第二次调用：缓存应未受污染
	nodes2, _ := svc.GetChildren(dir)
	if nodes2[0].Name != "folder" {
		t.Errorf("缓存被污染: got %q want folder", nodes2[0].Name)
	}
	if len(nodes2[0].Children) != 0 {
		t.Errorf("缓存 Children 被污染: got %d want 0", len(nodes2[0].Children))
	}
}

// TestInvalidateCache_BypassesStaleCache 注入陈旧缓存（mtime 匹配会命中），
// InvalidateCache 应清除该路径缓存，后续 GetChildren miss 并实扫返回真实数据。
func TestInvalidateCache_BypassesStaleCache(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "real.txt"), []byte("real"))
	svc := NewFileTreeService()

	abs, _ := filepath.Abs(dir)
	info, _ := os.Stat(dir)

	// 注入陈旧缓存：节点名与实际不符，但 mtime 匹配（直接 GetChildren 会命中陈旧缓存）
	staleNodes := []*model.FileTreeNode{model.NewFileTreeNode("stale.txt", filepath.Join(dir, "stale.txt"), "file")}
	svc.treeCache.set(abs, info.ModTime(), staleNodes)

	// 前置验证：直接 GetChildren 命中陈旧缓存
	cached, _ := svc.GetChildren(dir)
	if len(cached) != 1 || cached[0].Name != "stale.txt" {
		t.Fatalf("前置验证失败，应命中陈旧缓存: %+v", cached)
	}

	// InvalidateCache 纯清缓存（不返回数据），后续 GetChildren 应 miss 并实扫
	svc.InvalidateCache(dir)
	fresh, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("GetChildren after invalidate: %v", err)
	}
	if len(fresh) != 1 || fresh[0].Name != "real.txt" {
		t.Errorf("InvalidateCache 后应返回真实数据: %+v", fresh)
	}
}

// TestDeepCopyNode_CoversAllFields 用反射遍历 model.FileTreeNode 全字段，比对 deepCopyNode
// 输出与原节点的字段值。保障机制：未来 FileTreeNode 新增字段未在 deepCopyNode 拷贝时，拷贝值为
// 该字段零值，原值为非零值，反射逐字段断言失败——为手写字段拷贝提供编译期之外的同步保障。
//
// 构造原则：所有字段均设非零值（bool=true / string=非空 / slice=非 nil），含 Children 递归一层。
// 前置断言强制 original 全字段非零：未来新增字段未在此构造设非零值即失败，迫使维护者同步补构造，
// 从而使漏拷贝能被下方 DeepEqual 捕获。reflect.DeepEqual 对 slice 字段递归比对值（非地址）。
func TestDeepCopyNode_CoversAllFields(t *testing.T) {
	original := &model.FileTreeNode{
		ID:          "id-1",
		Name:        "node-name",
		Path:        "/p/node",
		Type:        "directory",
		IsGitRepo:   true,
		HasRemote:   true,
		HasChildren: true,
		IsLeaf:      true,
		Children: []*model.FileTreeNode{
			{
				ID:          "child-id",
				Name:        "child-name",
				Path:        "/p/node/child",
				Type:        "file",
				IsGitRepo:   true,
				HasRemote:   true,
				HasChildren: true,
				IsLeaf:      true,
				// 非 nil 空 slice，检测 nil 与空 slice 的拷贝一致性
				Children: []*model.FileTreeNode{},
			},
		},
	}

	cp := deepCopyNode(original)
	if cp == nil {
		t.Fatal("deepCopyNode 返回 nil")
	}

	want := reflect.ValueOf(original).Elem()
	got := reflect.ValueOf(cp).Elem()
	typ := want.Type()

	// 前置断言：original 每个字段必须非零值。零值字段拷贝后仍为零值，DeepEqual 无法区分漏拷贝；
	// 此断言强制未来新增字段时测试构造同步设非零值，从而使漏拷贝能被下方 DeepEqual 捕获。
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		w := want.Field(i)
		if reflect.DeepEqual(w.Interface(), reflect.Zero(w.Type()).Interface()) {
			t.Errorf("构造缺陷：字段 %s 为零值，无法检测 deepCopyNode 是否拷贝；请设非零值", field.Name)
		}
	}

	// reflect 逐字段比对原节点与拷贝节点的值，漏拷贝字段零值 != 原非零值即失败
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		w := want.Field(i)
		g := got.Field(i)
		if !reflect.DeepEqual(w.Interface(), g.Interface()) {
			t.Errorf("字段 %s 未同步拷贝: 原=%v, 拷贝=%v（deepCopyNode 漏拷贝该字段，请同步更新）",
				field.Name, w.Interface(), g.Interface())
		}
	}
}

// TestClearAllCache_BypassesStaleCache 注入陈旧缓存，ClearAllCache 后
// GetChildren 应 miss 并返回真实数据。
func TestClearAllCache_BypassesStaleCache(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "real.txt"), []byte("real"))
	svc := NewFileTreeService()

	abs, _ := filepath.Abs(dir)
	info, _ := os.Stat(dir)

	// 注入陈旧缓存
	staleNodes := []*model.FileTreeNode{model.NewFileTreeNode("stale.txt", filepath.Join(dir, "stale.txt"), "file")}
	svc.treeCache.set(abs, info.ModTime(), staleNodes)

	// 清全部缓存
	svc.ClearAllCache()

	// GetChildren 应 miss 并返回真实数据
	fresh, _ := svc.GetChildren(dir)
	if len(fresh) != 1 || fresh[0].Name != "real.txt" {
		t.Errorf("ClearAllCache 后应返回真实数据: %+v", fresh)
	}
}

// TestGetChildren_NonExistentPathNotCached 目录不存在时 Stat 失败，
// 跳过缓存直接实扫并返回错误，不回写缓存。
func TestGetChildren_NonExistentPathNotCached(t *testing.T) {
	dir := t.TempDir()
	nonExistent := filepath.Join(dir, "does_not_exist")
	svc := NewFileTreeService()

	_, err := svc.GetChildren(nonExistent)
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}

	// 不应回写缓存（Stat 失败时 set 不执行）
	abs, _ := filepath.Abs(nonExistent)
	if _, ok := svc.treeCache.get(abs, time.Now()); ok {
		t.Error("不存在的路径不应被缓存")
	}
}
