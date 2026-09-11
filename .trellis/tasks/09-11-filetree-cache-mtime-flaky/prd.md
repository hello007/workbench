# 修复 filetree 缓存 mtime 测试 flaky

## Goal

修复 `TestGetChildren_CacheInvalidatedOnMtimeChange`(service/filetree_cache_test.go:241)间歇性失败。该测试验证"新增文件 → 父目录 mtime 变化 → 缓存失效重扫",但在 Windows NTFS 上 flaky:连续两次独立运行第一次 FAIL、第二次 PASS。

## 根因分析

`FileTreeCache.get`(service/filetree_cache.go:55)失效判定:`entry.modTime.Equal(curMtime)` —— 目录 mtime 未变即命中缓存。

测试流程(filetree_cache_test.go:241-260):
1. `mustWriteFile(a.txt)` → 目录 mtime = T1
2. `GetChildren(dir)` → 扫描回写缓存,`modTime = T1`,返回 1 节点
3. `mustWriteFile(b.txt)` → 期望目录 mtime 变为 T2 ≠ T1
4. `GetChildren(dir)` → 期望 mtime 变化触发失效重扫,返回 2 节点

**问题**:Windows NTFS 目录 mtime 分辨率约 1 秒(实测可能更粗)。步骤 1 与步骤 3 若落在同一 mtime tick 内,目录 mtime 未变 → `Equal` 命中 → 返回 1 节点 → 测试 FAIL。重跑若跨秒则 mtime 变化 → PASS。这就是 flaky 来源。

非生产 bug:生产场景用户两次展开目录间隔远超 1 秒,mtime 通常已变;且 TTL 5min + 手动刷新兜底。问题仅在测试的 mtime 假设过强。

## 候选方案(待 brainstorm 确认)

### A. 测试侧 sleep 等待 mtime 跨 tick

步骤 3 前加 `time.Sleep(1100 * time.Millisecond)` 确保 mtime 变化。
- 优点:最小改动,测试语义不变
- 缺点:拖慢测试 ~1s/用例;依赖 NTFS 分辨率假设(未来文件系统变化仍可能漏)

### B. 测试侧主动 touch 目录 mtime

步骤 3 后显式 `os.Chtimes(dir, time.Now(), time.Now())` 强制刷新目录 mtime。
- 优点:确定性高,不 sleep,不依赖分辨率
- 缺点:测试操作了被测路径的元数据,略不自然;`Chtimes` 精度仍受文件系统限制(但 `time.Now()` 通常足够新)

### C. 测试改用直接验证缓存失效语义,不依赖真实 mtime

注入陈旧缓存(同 `TestInvalidateCache_BypassesStaleCache` filetree_cache_test.go:286 范式),用 `svc.treeCache.set(abs, oldMtime, staleNodes)` 模拟 mtime 变化,断言 `GetChildren` miss 重扫。
- 优点:不依赖文件系统时序,确定性最高,与现有测试范式一致(已有注入陈旧缓存的先例)
- 缺点:不验证"真实文件新增触发 mtime 变化"这条端到端路径,语义偏移到单元层

### D. 生产侧降低 mtime 比较粒度或加内容指纹

改 `FileTreeCache` 失效判定,加目录条目数 / 内容哈希兜底。
- 优点:生产更健壮
- 缺点:生产无此 bug(用户场景间隔足),属过度设计;改生产代码风险高,违背"修测试不修生产"原则

## What I already know

- 测试位置:service/filetree_cache_test.go:241-260
- 被测逻辑:service/filetree_cache.go:55-73(`get` 的 `modTime.Equal` 判定)
- 现有注入缓存范式:filetree_cache_test.go:286 `TestInvalidateCache_BypassesStaleCache`(用 `svc.treeCache.set` 注入陈旧缓存)
- 测试门禁:service ≥76% 基线([test-coverage-gate.md](../../../docs/spec/test-coverage-gate.md))
- 属已归档任务 09-10-filetree-cache 的遗留,本任务父任务 09-10-wb-phase-iteration 收尾时发现

## Open Questions

- 选哪个方案?(推荐 C:确定性最高,与现有注入范式一致,不拖慢测试)

## Requirements (evolving)

- 消除 `TestGetChildren_CacheInvalidatedOnMtimeChange` 的 flaky,连续多次运行稳定 PASS
- 不降低 service 覆盖率(≥76%)
- 不改生产代码(除非方案 D 被选中,不推荐)

## Acceptance Criteria

- [ ] 连续 5 次 `go test ./service/ -run TestGetChildren_CacheInvalidatedOnMtimeChange -count=1` 全 PASS
- [ ] `go test ./...` 全绿
- [ ] service 覆盖率 ≥76%

## Out of Scope

- 生产侧 FileTreeCache 失效逻辑改动(除非确认生产也有漏扫风险)
- 其他 filetree 测试重构

## Technical Notes

- 参考文件:service/filetree_cache_test.go(241/286)、service/filetree_cache.go(55-73)
- 测试门禁:docs/spec/test-coverage-gate.md
