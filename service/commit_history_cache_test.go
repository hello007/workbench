package service

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"workbench/model"
)

// ===== CommitHistoryCache 单元测试 =====

// sampleCommits 构造 n 个提交快照，Files 字段非 nil 以验证深拷贝切片隔离。
func sampleCommits(n int) []model.Commit {
	out := make([]model.Commit, n)
	for i := 0; i < n; i++ {
		out[i] = model.Commit{
			SHA:       fmt.Sprintf("sha-%d", i),
			ShortSHA:  fmt.Sprintf("short-%d", i),
			Message:   fmt.Sprintf("msg-%d", i),
			Author:    fmt.Sprintf("author-%d", i),
			Email:     fmt.Sprintf("email-%d@x.com", i),
			Timestamp: int64(i),
			DateTime:  fmt.Sprintf("2026-09-12 00:0%d:00", i),
			Files:     []string{fmt.Sprintf("file-%d-a.go", i), fmt.Sprintf("file-%d-b.go", i)},
		}
	}
	return out
}

func TestCommitHistoryCache_HitAndMiss(t *testing.T) {
	cache := NewCommitHistoryCache()
	commits := sampleCommits(3)

	// 未写入前应未命中
	if _, _, ok := cache.Get("k"); ok {
		t.Error("未写入前不应命中")
	}

	cache.Set("k", "head-1", commits)

	// 命中应返回深拷贝与正确 headSHA
	got, headSHA, ok := cache.Get("k")
	if !ok {
		t.Error("写入后应命中")
	}
	if headSHA != "head-1" {
		t.Errorf("headSHA 异常: got %q want head-1", headSHA)
	}
	if len(got) != 3 || got[0].SHA != "sha-0" {
		t.Errorf("命中数据异常: %+v", got)
	}
}

// TestCommitHistoryCache_DifferentHeadSHAReturnsFoundForIncremental
// HEAD 前移场景：缓存 headSHA 与当前不同，get 仍返 found=true 供调用方走增量 prepend。
func TestCommitHistoryCache_DifferentHeadSHAReturnsFoundForIncremental(t *testing.T) {
	cache := NewCommitHistoryCache()
	cache.Set("k", "head-old", sampleCommits(2))

	got, headSHA, ok := cache.Get("k")
	if !ok {
		t.Error("headSHA 不同仍应 found=true 供调用方增量判定")
	}
	if headSHA != "head-old" {
		t.Errorf("应返回缓存 headSHA: got %q want head-old", headSHA)
	}
	if len(got) != 2 {
		t.Errorf("应返回缓存全量: got %d want 2", len(got))
	}
}

// TestCommitHistoryCache_TTLExpiry 注入陈旧 cachedAt 驱动 TTL 过期分支，
// 不依赖真实时间 sleep，确定性 PASS（规避 flaky，同 FileTreeCache 范式）。
func TestCommitHistoryCache_TTLExpiry(t *testing.T) {
	cache := NewCommitHistoryCache()
	cache.Set("k", "head-1", sampleCommits(2))

	// TTL 内应命中
	if _, _, ok := cache.Get("k"); !ok {
		t.Error("TTL 内应命中")
	}

	// 手动将 cachedAt 置为过期，模拟 TTL 超时
	cache.mu.Lock()
	e := cache.entries["k"]
	e.cachedAt = time.Now().Add(-commitHistoryCacheTTL - time.Second)
	cache.entries["k"] = e
	cache.mu.Unlock()

	// TTL 过期 -> 未命中
	if _, _, ok := cache.Get("k"); ok {
		t.Error("TTL 过期应未命中")
	}

	// TTL 过期不仅判 miss，还应驱逐条目（delete），避免内存与历史访问仓库数成正比无界增长
	cache.mu.Lock()
	_, stillExists := cache.entries["k"]
	cache.mu.Unlock()
	if stillExists {
		t.Error("TTL 过期后条目应从 map 驱逐")
	}
}

func TestCommitHistoryCache_ClearPath(t *testing.T) {
	cache := NewCommitHistoryCache()
	cache.Set("k1", "h1", sampleCommits(1))
	cache.Set("k2", "h2", sampleCommits(1))

	cache.ClearPath("k1")

	if _, _, ok := cache.Get("k1"); ok {
		t.Error("clearPath 后 k1 应未命中")
	}
	if _, _, ok := cache.Get("k2"); !ok {
		t.Error("clearPath(k1) 不应影响 k2")
	}
}

func TestCommitHistoryCache_ClearAll(t *testing.T) {
	cache := NewCommitHistoryCache()
	cache.Set("k1", "h1", sampleCommits(1))
	cache.Set("k2", "h2", sampleCommits(1))

	cache.ClearAll()

	if _, _, ok := cache.Get("k1"); ok {
		t.Error("clearAll 后 k1 应未命中")
	}
	if _, _, ok := cache.Get("k2"); ok {
		t.Error("clearAll 后 k2 应未命中")
	}
}

// TestCommitHistoryCache_ClearByGitRoot 按仓库根清除该仓库全部 ref 键（分支 + detached），
// 不影响其他仓库。模拟同仓库分支切换遗留多 ref 键 + 跨仓库隔离。
func TestCommitHistoryCache_ClearByGitRoot(t *testing.T) {
	cache := NewCommitHistoryCache()
	// 仓库 A：两个分支 ref + 一个 detached 键
	cache.Set("/repoA|refs/heads/master", "h-a1", sampleCommits(1))
	cache.Set("/repoA|refs/heads/dev", "h-a2", sampleCommits(1))
	cache.Set("/repoA|HEAD|abc123", "h-a3", sampleCommits(1))
	// 仓库 B：不应受影响
	cache.Set("/repoB|refs/heads/master", "h-b1", sampleCommits(1))

	cache.ClearByGitRoot("/repoA")

	if _, _, ok := cache.Get("/repoA|refs/heads/master"); ok {
		t.Error("clearByGitRoot 后仓库 A master 应清除")
	}
	if _, _, ok := cache.Get("/repoA|refs/heads/dev"); ok {
		t.Error("clearByGitRoot 后仓库 A dev 应清除")
	}
	if _, _, ok := cache.Get("/repoA|HEAD|abc123"); ok {
		t.Error("clearByGitRoot 后仓库 A detached 应清除")
	}
	if _, _, ok := cache.Get("/repoB|refs/heads/master"); !ok {
		t.Error("clearByGitRoot(/repoA) 不应影响仓库 B")
	}
}

// TestCommitHistoryCache_GetReturnsDeepCopy 验证 get 返回深拷贝，
// 调用方修改返回切片（含 Files 元素与 append）不污染缓存内部数据。
func TestCommitHistoryCache_GetReturnsDeepCopy(t *testing.T) {
	cache := NewCommitHistoryCache()
	cache.Set("k", "h1", sampleCommits(2))

	got, _, ok := cache.Get("k")
	if !ok {
		t.Fatal("应命中")
	}

	// 修改返回切片（模拟调用方过滤/分页篡改）
	got[0].SHA = "mutated"
	got[0].Message = "mutated-msg"
	got[0].Files[0] = "mutated-file"
	got = append(got, model.Commit{SHA: "extra"})

	// 再次取，缓存应未受污染
	got2, _, _ := cache.Get("k")
	if got2[0].SHA != "sha-0" {
		t.Errorf("缓存 SHA 被污染: got %q want sha-0", got2[0].SHA)
	}
	if got2[0].Message != "msg-0" {
		t.Errorf("缓存 Message 被污染: got %q want msg-0", got2[0].Message)
	}
	if got2[0].Files[0] != "file-0-a.go" {
		t.Errorf("缓存 Files 被污染: got %q want file-0-a.go", got2[0].Files[0])
	}
	if len(got2) != 2 {
		t.Errorf("缓存长度被 append 污染: got %d want 2", len(got2))
	}
}

// TestCommitHistoryCache_SetStoresDeepCopy 验证 set 存储深拷贝，
// 调用方修改原始切片（set 之后）不污染缓存。
func TestCommitHistoryCache_SetStoresDeepCopy(t *testing.T) {
	cache := NewCommitHistoryCache()
	original := sampleCommits(2)
	cache.Set("k", "h1", original)

	// 修改原始切片
	original[0].SHA = "mutated"
	original[0].Files[0] = "mutated-file"
	original = append(original, model.Commit{SHA: "extra"})

	got, _, _ := cache.Get("k")
	if got[0].SHA != "sha-0" {
		t.Errorf("缓存被原始切片污染: got %q want sha-0", got[0].SHA)
	}
	if got[0].Files[0] != "file-0-a.go" {
		t.Errorf("缓存 Files 被原始切片污染: got %q want file-0-a.go", got[0].Files[0])
	}
	if len(got) != 2 {
		t.Errorf("缓存长度被原始切片污染: got %d want 2", len(got))
	}
}

// TestCommitHistoryCache_MaxEntriesNotCached 超上限条数不写缓存，防爆内存。
// 调用方仍可返回数据（走原 go-git 路径），缓存层仅拒绝 set。
func TestCommitHistoryCache_MaxEntriesNotCached(t *testing.T) {
	cache := NewCommitHistoryCache()
	overLimit := sampleCommits(CommitHistoryMaxEntries + 1)

	cache.Set("k", "h1", overLimit)

	if _, _, ok := cache.Get("k"); ok {
		t.Error("超 commitHistoryMaxEntries 不应写入缓存")
	}

	// 恰好等于上限应写入（边界含端点）
	cache.Set("k2", "h2", sampleCommits(CommitHistoryMaxEntries))
	if _, _, ok := cache.Get("k2"); !ok {
		t.Error("等于上限应写入缓存")
	}
}

// TestCommitHistoryCache_Concurrent 并发 get/set/clearPath/clearAll 不触发 map 读写竞态。
// 需配合 `go test -race` 运行以检测数据竞争。
func TestCommitHistoryCache_Concurrent(t *testing.T) {
	cache := NewCommitHistoryCache()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", i%10)
			cache.Set(key, fmt.Sprintf("h-%d", i), sampleCommits(2))
			cache.Get(key)
			cache.ClearPath(key)
		}(i)
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.ClearAll()
		}()
	}
	wg.Wait()
}

// TestDeepCopyCommits_CoversAllFields 用反射遍历 model.Commit 全字段，比对 deepCopyCommits
// 输出与原切片的逐字段值。保障机制：未来 Commit 新增字段未在深拷贝处理时，拷贝值为该字段
// 零值（struct 值拷贝自动覆盖值类型字段；slice 字段需显式处理），反射逐字段断言失败——
// 为手写深拷贝提供编译期之外的同步保障。
//
// 构造原则：所有字段均设非零值（含 Files 非 nil 切片）。前置断言强制 original 全字段非零：
// 未来新增字段未在此构造设非零值即失败，迫使维护者同步补构造，从而使漏拷贝能被下方
// DeepEqual 捕获。reflect.DeepEqual 对 slice 字段递归比对值（非地址）。
func TestDeepCopyCommits_CoversAllFields(t *testing.T) {
	original := []model.Commit{{
		SHA:       "sha-1",
		ShortSHA:  "short-1",
		Message:   "msg-1",
		Author:    "author-1",
		Email:     "email-1@x.com",
		Timestamp: 1700000000,
		DateTime:  "2026-09-12 00:00:00",
		Files:     []string{"a.go", "b.go"},
	}}

	cp := deepCopyCommits(original)
	if cp == nil {
		t.Fatal("deepCopyCommits 返回 nil")
	}

	want := reflect.ValueOf(original[0])
	got := reflect.ValueOf(cp[0])
	typ := want.Type()

	// 前置断言：original 每个字段必须非零值。零值字段拷贝后仍为零值，DeepEqual 无法区分漏拷贝；
	// 此断言强制未来新增字段时测试构造同步设非零值，从而使漏拷贝能被下方 DeepEqual 捕获。
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		w := want.Field(i)
		if reflect.DeepEqual(w.Interface(), reflect.Zero(w.Type()).Interface()) {
			t.Errorf("构造缺陷：字段 %s 为零值，无法检测 deepCopyCommits 是否拷贝；请设非零值", field.Name)
		}
	}

	// 反射逐字段比对原提交与拷贝提交的值，漏拷贝字段零值 != 原非零值即失败
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		w := want.Field(i)
		g := got.Field(i)
		if !reflect.DeepEqual(w.Interface(), g.Interface()) {
			t.Errorf("字段 %s 未同步拷贝: 原=%v, 拷贝=%v（deepCopyCommits 漏拷贝该字段，请同步更新）",
				field.Name, w.Interface(), g.Interface())
		}
	}

	// Files 切片底层数组应独立：修改拷贝不影响原切片
	cp[0].Files[0] = "mutated.go"
	if original[0].Files[0] != "a.go" {
		t.Errorf("Files 底层数组共享，修改拷贝污染原切片: got %q want a.go", original[0].Files[0])
	}
}
