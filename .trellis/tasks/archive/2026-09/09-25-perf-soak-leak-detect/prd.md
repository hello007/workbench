# WorkBench 性能测试补全：长跑稳定性测试 + 内存泄漏自动检测

## Goal

收口路线图「性能测试」节剩余两项：长时间运行稳定性测试与内存泄漏自动检测。
在既有性能基线（docs/spec/perf-baseline.md v1.4 PR1：Go benchmark 四组 + MemStats 快照 + 前端 bundle）之上，
新增 service/主包层长跑测试（高频循环核心读路径 + 缓存命中/失效混合）与泄漏自动断言
（heap 快照对比 + goroutine 数对比），量化数据沉淀回 perf-baseline.md，路线图勾选。

## What I already know

### 用户给定
- 长跑路径：GetTree / GetCommitHistory / GetLocalChanges / ScanGitRepos，混合缓存命中与手动 Invalidate 失效两形态
- testing.Short 分档：短模式缩轮次保 `go test ./...` 速度（Short 增量 <30s）；长模式显式 flag 深跑
- 循环周期采样：goroutine 数、HeapAlloc、HeapSys、NumGC，输出趋势结论
- 泄漏检测：两次 runtime.GC() 后 heap 快照对比断言，增量阈值可配置（防 CI 噪声）
- goroutine 泄漏：循环前后数量对比；评估 uber-go/goleak 选型（引入 vs 手写，含依赖成本）
- 优先覆盖有状态路径：treeCache / commitHistoryCache 增长、事件订阅、终端会话（如可测）
- 阈值宁松勿紧：抓趋势级明显泄漏，不抓噪声；CI 偶发失败比漏检危害大
- 禁 time.Sleep / 固定端口（34115 常驻 workbench.exe）；mtime 判定用注入陈旧缓存驱动
- benchmark/_test 不入覆盖率门禁；优先不改生产代码，生产 instrument 单独列出征确认

### 仓库勘察（已核实）
- 代码根即本仓库 git-manager/（CLAUDE.md 中 workbench/ 路径为历史称呼）
- `service/perf_bench_test.go`：FileTree/ScanGitRepos benchmark（buildFileTreeFixture 可参考）；
  主包 `perf_bench_test.go`：NewAppServices benchmark + TestPerfMemStats（perfDataDir 处理 lumberjack 句柄残留）
- **GetCommitHistory 在主包** `app_git.go`（App 方法，依赖 a.commitHistoryCache）；service 层对应采集/缓存层
  在 `commit_history_cli.go` + `commit_history_cache.go`（NewCommitHistoryCache 导出，Get/Set/ClearAll 导出）
- `GitService.GetLocalChanges` / `ScanGitRepos` / `FileTreeService.GetTree` 均为 service 层导出方法
- `treeCache` 为 FileTreeService 未导出字段（`filetree_cache.go`，get/set 未导出）——注入陈旧缓存范式
  （test-stability.md 方案 C）须在 service 包内测试文件完成
- `util/testutil`：RunGit / WriteFile / InitTempRepo / SetupMasterBranch / SetupFFRepo / SetupConflictRepo，
  参数 testing.TB，fixture 必须复用
- 路线图 `docs/路线图.md:262-265`：性能测试节两项未勾选
- go.mod：无 goleak 依赖；go 1.26.0 / toolchain go1.26.6
- MemStats 噪声说明（perf-baseline.md §5）：HeapAlloc 受 GC 时序影响，HeapSys 更稳；
  精确分配以 -benchmem B/op 为准

## Assumptions (temporary)

- 长跑测试放主包 + service 包两个文件（GetCommitHistory 仅主包可测；treeCache 注入陈旧缓存仅 service 包可测）
- 泄漏断言用 relative 增量 + 宽松阈值（如 heap 增量 < 若干 MB 且 goroutine 增量 = 0~小常数）
- 终端会话（TerminalService）若涉及 pty/conpty 真实进程则不进自动长跑，文档说明

## Decision (ADR-lite)

### 决策 (a)：泄漏断言 CI 策略 —— 宽松断言入 CI（用户选定，2026-09-25）

**Context**：CI（GitHub Actions Linux runner）跑裸 `go test ./...` 不带 `-short`，默认档即 CI 档；验收要求 `go test ./...` 全绿。
**Decision**：默认档带宽松断言入 CI——heap 增量阈值 service 8MB / 主包 16MB（`-leak-heap-mb` 可调，宁松勿紧）+ goroutine 增量 ≤2；fork git 路径轮次压低控时长；断言只抓趋势级泄漏，偶发误报靠宽松阈值兜底。
**Consequences**：CI 有泄漏拦截能力；偶发误报风险由「阈值宁松 + goroutine Δ≤2 + 轮次确定性」缓解；若后续出现误报，优先调阈值/登记良性栈，不撤门禁。

### 决策 (b)：goleak 引入 —— 引入 go.uber.org/goleak v1.3.0（用户选定，2026-09-25）

**Context**：goroutine 泄漏检测手写 `NumGoroutine` 计数对比存在身份置换盲区（旧退新进计数抵消漏检）；goleak 按栈集合 diff 可识破，依赖实测增量 go.mod +1 行 / go.sum +2 条（无传递树），过滤逻辑上游维护；已选 (a) 断言入 CI，门禁能力需完整。
**Decision**：引入 `go.uber.org/goleak v1.3.0`。goroutine 泄漏门禁用包级 `goleak.VerifyTestMain`（挂 service 包，全仓首个 TestMain；`testing.Short()` 短路保 <30s）；循环内 `NumGoroutine` 手写计数仅留作趋势采样输出，不断言。版本钉死 v1.3.0，不追 main 分支未发版 API（`IgnoreCreatedBy` v1.3.0 无）。良性栈噪声用 `IgnoreTopFunction`（全限定名）逐个登记，首轮先 `goleak.Find()` 打日志观察跑绿再换 `VerifyTestMain` 门禁。
**Consequences**：全仓首个 TestMain 出现在 service 包；依赖升级后须重跑 `govulncheck ./...`；预期候选忽略项（pty 读泵 / CLI 流读取）出现时逐个登记，不放宽全包豁免；主包是否同挂 goleak 视首轮观察结果定（websocket Hub 常驻 goroutine 在主包，误报面大，倾向不挂）。

## Open Questions

- (c) 长跑轮次与时长档位（Short/Long 两档具体数值）

### 决策 (c)：长跑轮次与时长档位 —— 推荐档（用户选定，2026-09-25）

**Context**：泄漏断言须轮次驱动（工作负载确定 → Δheap 可比）；CI 跑裸 `go test ./...`，默认档即 CI 档须 <30s；fork git 单轮 20-60ms 为估计值，首轮实测后修正 Long 档预算。
**Decision**：默认档 GetTree 200 / ScanGitRepos 200 / GetLocalChanges 40 / GetCommitHistory 30 轮（估 ~6-10s）；Short 档 50/50/10/8；Long 档 `-soak-rounds=N` 显式 10×（2000/2000/400/300，本机手动 ~1-3 分钟）。flag 解析优先级：`-soak-rounds` > Short 缩减 > 默认档。每档 10 检查点采样 goroutine/HeapAlloc/HeapSys/NumGC，`t.Logf` 趋势表输出。
**Consequences**：CI 增量 ~6-10s 可接受；Long 档预算待首轮实测修正。

## Requirements (final)

- 新增长跑稳定性测试：四读路径高频循环，命中/失效混合（GetTree 失效走手动 `InvalidateCache` 驱动，不依赖 mtime 时序），三档分档，10 检查点采样输出趋势
- 新增泄漏检测测试：双 `runtime.GC()` 后 heap 快照对比断言（阈值 `-leak-heap-mb` 可调：service 8MB / 主包 16MB）+ goroutine Δ≤2；goroutine 门禁用 goleak `VerifyTestMain`（service 包，Short 短路）
- 有状态路径覆盖：treeCache（service 包内，未导出字段可触）+ commitHistoryCache（导出 NewCommitHistoryCache/Get/Set）增长断言
- 事件订阅（sinkHolder 单出口换引用非累积订阅）核实无泄漏面，perf-baseline.md 说明即可，不专项测试
- 终端会话（真实 pty/conpty 进程，Windows-only conpty + 进程 fork 成本）不进自动长跑，文档记录理由
- perf-baseline.md 增补「长跑稳定性」「泄漏检测」两章节（量化数据 + 结论）；路线图两项勾选；CLAUDE.md 关键规则按需沉淀
- 文件布局：`service/soak_test.go`（TestMain + 三路径 + treeCache）+ 主包 `soak_test.go`（GetCommitHistory，复用 perfDataDir）；flag 包级注册 `-soak-rounds` / `-leak-heap-mb`
- fixture 复用 util/testutil；goleak 依赖升级后重跑 govulncheck

## Acceptance Criteria (final)

- [ ] `go test ./...` 与 `go test -tags=integration ./...` 全绿（含新增 soak/泄漏测试）
- [ ] 默认档（CI 档）新增测试增量 <30s；Short 档更快
- [ ] 泄漏断言阈值经 `-leak-heap-mb` 可配置；goleak VerifyTestMain 挂 service 包且 Short 短路
- [ ] perf-baseline.md 有量化数据（轮次/时长/goroutine/heap 趋势结论）+ 两新章节
- [ ] 路线图「长时间运行稳定性测试」「内存泄漏检测」勾选
- [ ] 禁 time.Sleep / 固定端口；缓存失效由手动 Invalidate 驱动非 mtime 时序
- [ ] fixture 复用 util/testutil，不重复造轮子；零生产代码改动（goleak 为 test 依赖）
- [ ] govulncheck 重跑清零

## Definition of Done (team quality bar)

- 测试新增且全绿；lint/vet 干净
- 文档同步（perf-baseline.md + 路线图 + CLAUDE.md 视需）
- 不改生产代码（如确需 instrument 单独列出征确认）

## Out of Scope (explicit)

- GUI 冷启动测量（维度 6 占位，另行手动）
- PR4 之后的进一步性能优化
- 前端长跑/泄漏（本轮仅 Go 后端）

## Technical Notes

- 关键文件：service/perf_bench_test.go、perf_bench_test.go（主包）、service/filetree.go、
  service/filetree_cache.go、service/commit_history_cache.go、app_git.go、util/testutil/testutil.go
- 测试稳定性约定：docs/spec/test-stability.md 全文有效
- 参考文档：docs/spec/perf-baseline.md（§5 噪声、§10 约束）、docs/spec/test-coverage-gate.md

## Research References

- （待研究 agents 回填：goleak 选型、Go 长跑/泄漏测试业界模式）
