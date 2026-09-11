# 测试稳定性规范（Flaky 规避）

> 本文档规定 WorkBench 测试中依赖文件系统时序的 flaky 规避方案与正反例。
> 最后更新：2026-09-11 · 来源任务：09-11-filetree-cache-mtime-flaky

## 1. 适用范围

- 依赖文件 / 目录 mtime 变化触发缓存失效、重扫、同步判定的测试
- 跨 Windows NTFS 与 Unix 文件系统需确定性 PASS 的测试
- 任何隐式假设「写文件 → 父目录 mtime 必然变化」的测试

## 2. 根因

Windows NTFS 目录 mtime 分辨率约 1 秒（实测可能更粗）。连续两次写文件操作若落在同一 mtime tick 内，父目录 mtime **不变**。测试若用「写文件后读目录 mtime 比对」判定状态变化，会间歇性失败：

- 同 tick 运行 → mtime 未变 → 命中缓存或判定未变化 → FAIL
- 跨秒重跑 → mtime 变化 → PASS

非生产 bug：生产场景用户两次操作间隔远超 1 秒，mtime 通常已变，且有 TTL / 手动刷新兜底。问题仅在测试的 mtime 假设过强。

## 3. 方案对照

| 方案 | 做法 | 评价 |
|---|---|---|
| A. sleep 等待跨 tick | 写文件后 `time.Sleep(1100ms)` | 拖慢测试；仍依赖 FS 分辨率假设，不推荐 |
| B. 显式 touch 目录 mtime | `os.Chtimes(dir, time.Now(), time.Now())` | 确定性较高，但 `Chtimes` 精度仍受 FS 限制；操作被测路径元数据略不自然 |
| C. 注入陈旧缓存验证失效语义 | 用 `cache.set(abs, staleMtime, staleNodes)` 注入 modTime 明确早于当前的陈旧缓存，直接驱动失效分支 | **推荐**：不依赖 FS 时序，确定性最高，与现有注入范式一致 |

## 4. 推荐写法（方案 C）

核心：不依赖真实文件系统 mtime 时序触发失效，改用注入一个 modTime 明确落后于当前 mtime 的陈旧缓存，直接驱动被测代码的 `modTime.Equal` 判定走失效分支。

`service/filetree_cache_test.go` `TestGetChildren_CacheInvalidatedOnMtimeChange` 范式：

```go
func TestGetChildren_CacheInvalidatedOnMtimeChange(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("a"))
	svc := NewFileTreeService()

	// 1. 首次扫描回写真实缓存，确认基线
	nodes1, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("first GetChildren: %v", err)
	}
	if len(nodes1) != 1 {
		t.Fatalf("first call: %d nodes", len(nodes1))
	}

	// 2. 新增文件（不依赖其是否更新目录 mtime）
	mustWriteFile(t, filepath.Join(dir, "b.txt"), []byte("b"))

	abs, _ := filepath.Abs(dir)
	info, _ := os.Stat(dir)
	curMtime := info.ModTime()

	// 3. 注入陈旧缓存：modTime 明确早于当前，节点名与实际不符
	staleMtime := curMtime.Add(-time.Hour)
	staleNodes := []*model.FileTreeNode{model.NewFileTreeNode("stale.txt", filepath.Join(dir, "stale.txt"), "file")}
	svc.treeCache.set(abs, staleMtime, staleNodes)

	// 4. get 判 staleMtime != curMtime → 失效 → miss 重扫返回真实数据
	nodes2, err := svc.GetChildren(dir)
	if err != nil {
		t.Fatalf("second GetChildren: %v", err)
	}
	if len(nodes2) != 2 {
		t.Errorf("mtime 变化应使缓存失效重扫: got %d nodes, want 2", len(nodes2))
	}
	// 5. 反污染断言：返回真实数据而非陈旧缓存
	for _, n := range nodes2 {
		if n.Name == "stale.txt" {
			t.Error("缓存未失效，返回了陈旧缓存的 stale.txt")
		}
	}
}
```

## 5. 反例（flaky 写法）

```go
// 反例：依赖 mustWriteFile 触发目录 mtime 变化来失效缓存
mustWriteFile(t, filepath.Join(dir, "b.txt"), []byte("b"))
nodes2, _ := svc.GetChildren(dir)
if len(nodes2) != 2 {  // NTFS 同 tick 时 mtime 未变 → 命中缓存返回 1 → FAIL
	t.Errorf("want 2")
}
```

问题：隐式假设「写文件 → 目录 mtime 必变」，NTFS 分辨率不足时间歇性失败。

## 6. 判定要点

写测试前问：**该断言是否依赖文件系统 mtime / 时间戳的精度与时序？**

- 是 → 改用注入明确不等的时间值（`curMtime.Add(-time.Hour)`）直接驱动判定分支，不依赖真实 FS 时序
- 必须端到端验证 mtime 触发路径时 → 用 `os.Chtimes` 显式设置，不用 `time.Sleep` 等待

## 7. 相关文档

- [测试覆盖率分层门禁](test-coverage-gate.md) — service ≥76% 基线，本任务改动后覆盖率 77.7%
- `service/filetree_cache_test.go:286` `TestInvalidateCache_BypassesStaleCache` — 同款注入陈旧缓存范式（验证 clearPath 语义）
