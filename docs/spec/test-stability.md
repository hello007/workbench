# 测试稳定性规范（Flaky 规避）

> 本文档规定 WorkBench 测试中依赖文件系统时序、环境端口、覆盖率插桩开销等外部状态的 flaky 规避方案与正反例。
> 最后更新：2026-11-20 · 来源任务：09-11-filetree-cache-mtime-flaky、09-23-testapp-startwebserve-startupmatrix、11-20-filehistorydialog

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

### 6.1 留底备份文件名同样禁含时间戳

「损坏文件留底备份恰好 N 份」类断言，若生产代码按 `time.Now().Format("20060102-150405")` 命名备份，同一份损坏内容跨秒重复读取就产生多份，属同一类时间依赖 flaky（`TestChatService_TemplatesCorruptFallback` 曾因此偶发失败）。

口径：留底文件名按内容摘要生成（`util.BackupCorruptFile` → `<路径>.bak.<SHA-256 前 12 位>`，见 [data-atomic-write.md「损坏留底备份命名」](data-atomic-write.md)），断言可确定性地写「恰好 1 份」；**禁**以 `time.Sleep` 等跨秒、也禁放宽为 `>= 1` 掩盖「备份随读取次数无上限增涨」的实现缺陷。

## 7. 端口 / 环境依赖

测试隐式依赖「本机某固定端口空闲」同样属于环境敏感 flaky：开发者本机常驻
运行中的 workbench.exe 会监听默认浏览器访问端口 36115（web serve 默认开启），
此时测试若经 `startWebServe` 走默认绑定地址 `127.0.0.1:36115`
（`model.DefaultWebServeBindAddress`），监听失败落入告警降级路径
（`Running()=false`），断言「应启动」即失败。表现为「审核时偶发、开发机
常驻应用时稳定挂」，与代码提交无关。

规避：经 `startWebServe` 的启动测试注入随机端口——优先在 settings 的
`WebServe.BindAddress` 写 `"127.0.0.1:0"`（保住 settings 选址分支覆盖），
或设 `webServeListenOverride = "127.0.0.1:0"`；直调 `webServeManager.Start`
的用例直接传 `"127.0.0.1:0"`（override 字段对其无效）；端口占用降级用例
显式自占随机端口。
正例：`web_serve_test.go` `TestApp_StartWebServe_StartupMatrix/设置开启默认启动`
（settings 注入范式）；`TestApp_GetWebServeConfig` 的 `Start("127.0.0.1:0")` 先例。

### 7.1 freePort 探测的 TOCTOU 窗口（定性接受）

`web_serve_test.go` 的 `freePort` 辅助先 `net.Listen("127.0.0.1:0")` 探测再
Close 返回端口号，Close 到后续 `Start` 真实绑定之间存在 TOCTOU 窗口（端口可能
被同机其他进程抢占）。定性**接受**，理由：

- **结构性不可避免**：地址切换、改址重启类用例的被测语义依赖「具体地址字符串
  变化」触发真实重启路径（`Start` 幂等判断走配置地址字符串比较，`"127.0.0.1:0"`
  同串命中 no-op），必须预知具体端口，无法统一改绑 `:0`。
- **窗口极小**：内核 `:0` 分配本身跳过已监听端口，静态常驻服务（如 36115 的
  workbench.exe）不构成冲突源；冲突仅来自探测后瞬间恰好有其他进程绑同一随机端口。
- **危害有界**：被抢时 `Start` 返回错误（不静默降级），表现为该用例误报一次，
  重跑即过，不产生假通过。

规则：能直接绑 `:0` 的用例一律直传 `"127.0.0.1:0"`，禁新增非必要 `freePort`
调用；确需具体端口的用例在 freePort 注释处留痕说明。

## 8. 前端覆盖率插桩放大单用例耗时

### 8.1 适用范围

前端单测中**单次遍历/渲染量大**的用例：逐页扫描链（多次串行 `await`）、大列表挂载（数百行以上）、大 diff 渲染等。此类用例在 `npm test` 下稳定通过，但在 `npm run test:coverage`（v8 插桩 + 全量多 worker 并行）下耗时被放大数倍，逼近或越过 vitest 默认 `testTimeout: 5000`。

### 8.2 根因

v8 插桩为每行/每分支增加计数开销，配合全量套件多 worker 的 CPU 争用，放大倍数随用例的计算与 DOM 操作量上升。实测 `FileHistoryDialog` 的 25 页扫描用例（累计渲染 6500 行）：

| 运行形态 | 耗时 |
|---|---|
| 单文件、无覆盖率 | 1356 ms |
| 单文件、v8 覆盖率 | 2699 ms |
| 全量并行、v8 覆盖率 | 4793 ms（距 5000 ms 上限 207 ms） |

全量并行连测 3 次该用例分别为 6453 / 5494 / 6274 ms，均超过默认 5000 ms——即此用例在覆盖率门禁下**必然**偶发失败，而非极端个例。失败与代码提交无关，纯环境敏感 flaky；同文件只扫 2 页的相邻用例覆盖率下仅 53 ms，不受影响。

### 8.3 方案对照

| 方案 | 做法 | 评价 |
|---|---|---|
| A. 用例级超时 + 显式轮询参数 | `it('...', { timeout: 20000 }, fn)`，`vi.waitFor(fn, { timeout: 15000, interval: 50 })` | **推荐**：影响面精确到该用例，其他用例保持默认守卫强度 |
| B. 文件级 `vi.setConfig({ testTimeout })` | 整个 spec 文件放宽 | 文件内其他用例失守默认超时，仅在整文件均为重链路时考虑 |
| C. 全局 `testTimeout` | 改 `vitest.config.js` | **禁止**：会把其他用例的真实变慢一并掩盖 |
| D. 缩短扫描/渲染量 | 改被测逻辑或减少断言 | **禁止**：等于放松断言与业务语义 |

### 8.4 推荐写法

`frontend/src/components/__tests__/FileHistoryDialog.spec.js`「initialSha 超定位上限未命中」范式：

```js
// 满 25 页扫描在 v8 插桩 + 全量并行下实测最坏 6.5s，逼近默认 5s 上限
it('initialSha 超定位上限未命中：info 提示并不再无限扫描', { timeout: 20000 }, async () => {
  // ...
  // 轮询上限显式抬到 15s：默认 1s 在插桩/并行下会先于扫描链完成而超时，
  // 用例级 20s 超时作为外层兜底
  await vi.waitFor(
    () => {
      expect(ElMessage.info).toHaveBeenCalledWith('该提交不在最近 500 条提交中')
    },
    { timeout: 15000, interval: 50 }
  )
  expect(GetFileCommitHistory).toHaveBeenCalledTimes(25)
})
```

先例：`BranchCompareDiffViewer.spec.js`「大 diff 截断保护」同样以 `{ timeout: 20000 }` 承载 5000 行渲染。

### 8.5 判定要点

- 仅当该用例**在无覆盖率单跑下耗时已达数百毫秒以上**、且失败形态是「testTimeout 超时」而非断言失败时才适用；断言类失败一律排查真实缺陷，禁以加超时掩盖。
- 用例级超时须与**显式轮询参数**配套：`vi.waitFor` 默认 `timeout: 1000`，只抬用例超时仍可能先死在轮询上限。
- 超时值按「全量并行实测最坏值」留足余量并在注释中写明依据：用例级超时约 ×3（6.5s → 20s）、轮询上限约 ×2（6.5s → 15s）；禁写成无依据的大数（如 60000）。
- 仍然**禁 `sleep` / 固定等待**：等待语义一律由 `vi.waitFor` 轮询表达。

### 8.6 反例（错误做法）

```js
// 反例 1：只加用例超时，轮询仍用默认 1s —— 先死在 vi.waitFor 上，超时值形同虚设
it('...', { timeout: 20000 }, async () => {
  await vi.waitFor(() => { expect(ElMessage.info).toHaveBeenCalledWith('...') })
})

// 反例 2：全局放宽 vitest.config.js —— 其他用例的真实变慢被一并掩盖
test: { testTimeout: 20000 }

// 反例 3：用固定等待消化扫描链 —— 既不确定又拖慢全部运行
await new Promise((r) => setTimeout(r, 3000))
expect(GetFileCommitHistory).toHaveBeenCalledTimes(25)
```

正例见 §8.4：双层超时显式落在该用例，轮询语义不变、断言原样保留。

### 8.7 断言要求

超时调整**只允许**新增参数，以下断言必须原样保留——它们是该用例的唯一价值所在：

- 扫描终点文案全等（`toHaveBeenCalledWith('该提交不在最近 500 条提交中')`），证明「500 条上限」这一上限常量未被改坏；
- 调用次数精确断言（`toHaveBeenCalledTimes(25)`），是「未无限扫描」的唯一信号，**禁**改为 `toBeGreaterThan` / `toHaveBeenCalled()` 等等价性更弱的写法；
- 目标 SHA 不出现在提示文案的负断言，防止提示泄露内部标识。

## 9. 相关文档

- [测试覆盖率分层门禁](test-coverage-gate.md) — service ≥76% 基线，本任务改动后覆盖率 77.7%
- `service/filetree_cache_test.go:286` `TestInvalidateCache_BypassesStaleCache` — 同款注入陈旧缓存范式（验证 clearPath 语义）
