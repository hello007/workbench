# PRD：Git 操作取消功能（context 贯穿收口）

## 背景

路线图并发控制条目留白「操作取消功能（暂不实现，留 context 贯穿扩展点）」，本轮落地。现状：

- `util/git.go` 的 `Execute` / `ExecuteWithCodes` / `Clone` 均已用 `exec.CommandContext`，但 ctx 为内部自建 `context.WithTimeout(context.Background(), timeout)`，外部取消信号传不进去（杀进程机制现成，缺信号入口）。
- `service/git.go` 的 `tryLockRepo` 返回 release 闭包，按 repoPath 绝对路径规范化为键，天然可挂 per-repo 取消注册表。
- `service` 层方法签名均无 ctx 参数；App 层 `PullRepo` / `ScanAndPullRepos` / `CloneRepo` 为 Wails 绑定方法。

## 目标

批量拉取 / 单仓 Pull / Clone 进行中可取消：在途 git 进程终止，已完成结果保留，取消展示为中性提示（非 error 级误报），取消后可立即重试（注册表与锁无残留）。

## 需求明细

### 1. util 层（util/git.go）

- 新增 `ExecuteWithContext(ctx, workDir, args...)` 与 `CloneWithContext(ctx, url, targetPath)`。
- 内部保留单命令超时语义：`context.WithTimeout(外部ctx, g.timeout)`，外部取消经 parent 传播。
- 取消判定：`ctx.Err() == context.Canceled` 时错误以 `%w` wrap `ctx.Err()`，供 service 层 `errors.Is(err, context.Canceled)` 识别；超时（DeadlineExceeded）错误格式与现状完全一致（行为不变验收项）。
- 现有 `Execute` / `ExecuteWithCodes` 抽私有统一实现 `run(ctx, workDir, codes, args...)`，对外签名与行为不动（全部既有调用点零改动）。`HideCommandWindow` 在 ctx 路径下保持生效。

### 2. service 层（service/git.go）

- `GitService` 新增 per-repo 取消注册表 `cancels map[string]context.CancelFunc`，与 `opLocks` 同用 `opMu` 锁保护。键规范化复用 `filepath.Abs`（抽 `normalizeRepoKey` helper，与 tryLockRepo 键一致）。
- 新增取消范围方法：
  - `CancelGitOperation(repoPath) (bool, error)`：取消在途操作 + 清注册表；无在途操作返回 false（幂等无害）。
  - `CancelAllGitOperations() int`：遍历注册表全部取消（批量取消 = 对未完成仓库逐个调的服务端实现），返回取消数。
- 可取消链路贯穿（仅 Pull / Clone / BatchPull 三个变更类长操作，其余不动）：
  - 抽包内 `pullOne(repoPath) model.PullResult`（自建 cancel ctx、注册、tryLockRepo、执行、defer 注销+释放锁+cancel）。
  - `Pull` / `Clone`：tryLockRepo 后建 ctx 注册，git 调用走 `ExecuteWithContext` / `CloneWithContext`。
  - `BatchPull`：per-repo ctx，goroutine 开头（sem 排队前）注册，覆盖「排队中未启动仓库」取消；`ctx.Err()` 已取消时不抢锁不执行，产出 `Cancelled=true` 结果（未启动跳过）；在途取消经 ctx 传播杀 git 进程。
- 错误映射：`errors.Is(err, context.Canceled)` 映射 `*model.AppError{E_GIT_CANCELLED, "操作已取消"}`（warning 分流，不走 error 提示）。
- 注册表与锁在操作完成 / 取消 / panic 路径均不泄漏（defer LIFO：unregister 先于 release；goleak 门禁护航）。

### 3. model 层（model/）

- `PullResult` 增 `Cancelled bool \`json:"cancelled"\``（该仓被取消：在途中断或排队未启动）。
- `PullSummary` 增 `Cancelled int \`json:"cancelled"\``（取消仓库数标注）。
- 新错误码 `ErrCodeGitCancelled = "E_GIT_CANCELLED"`，三处同步：`model/app_error.go` + `docs/spec/logging-and-errors.md` + `frontend/src/utils/error.js`（WARNING_CODES，warning 级）。

### 4. App 层（app_git.go）+ Wails 绑定

- 新增绑定 `CancelGitOperation(repoPath string) (bool, error)`、`CancelAllGitOperations() int`（委托 gitSvc）。
- `PullRepo` / `CloneRepo`（string 返回协议）取消时返回 `"已取消"`（非 `"错误: "` 前缀），前端据此中性提示。
- 绑定三处同步：`frontend/wailsjs/go/main/App.js` / `App.d.ts` / `models.ts`（PullResult/PullSummary 字段增量 + 新方法）。

### 5. 前端（frontend/src）

- `ContentPanel.vue`：
  - 批量进度对话框 footer 增「取消未完成」按钮（进行中显示），调 `CancelAllGitOperations`，不关弹窗，等 pull-complete。
  - 表格状态列 / 结果列增 `row.cancelled` 分支：中性色（warning 黄 / 灰）「已取消」，非 error 红。
  - `pull-complete` 读 `summary.cancelled` 并展示；`startBatchPull` 重置 cancelled 计数。
  - 单仓 `pullRepo` / `cloneRepo` 操作中态（gitLoading / cloneLoading）给取消入口，调 `CancelGitOperation`；结果/异常含取消时 `ElMessage.info('已取消')` 分流，不走 handleGitError error。
- 设计决策（与原 prompt「行加取消按钮」的差异）：现表格行是完成时 append（pull-progress 每仓完成一条），进行中/排队仓无行宿主，行级取消按钮无挂载点；取消入口收敛为对话框级「取消未完成」单按钮（服务端遍历注册表逐个取消，语义等价「对未完成仓库逐个调」）。被取消仓库随后经 pull-progress 逐条产出 `Cancelled=true` 行并标「已取消」，进度行标注语义完整保留。
- 不新增事件通道：取消计数走既有 pull-complete payload 增量键，规避 EventsOn/Off 生命周期新面。

## 约束

- 复用现有 `tryLockRepo` 锁与 `HideCommandWindow` 机制，禁另起并发控制路径。
- 现有 `Execute` / `ExecuteWithCodes` / `Clone` / `Pull` / `BatchPull` / `ScanAndPullRepos` 对外签名全部不动。
- 浏览器 serve 通道事件经 EventSink（既有 emitCurrent 路径复用，禁直调 EventsEmit）。
- service 层改动过 soak 三档轮次与泄漏断言（goleak）。

## 验收

1. 批量拉取进行中点「取消未完成」：在途 git 进程终止，已完成仓库结果保留，PullSummary/pull-complete 标注取消数，前端无 error 级误报，未完成行标「已取消」。
2. 单仓 Pull / Clone 取消同链路：操作中态取消后「已取消」中性提示，gitLoading/cloneLoading 复位。
3. 同仓取消后立即重试操作可正常获锁执行（注册表清理无残留）。
4. 超时路径（非取消）行为不变（错误格式逐字一致）。
5. vitest + 后端 `go test ./...` 全绿；新增取消链路单测：util ctx 已取消快速返回 Canceled、注册表注册/取消/清理生命周期、pullOne ctx 已取消跳过语义（未启动跳过）、ctx.Canceled 到 AppError 映射、BatchPull 部分完成语义。注：在途进程真实终止的真实验证归手动/E2E 验收（真 git 慢命令在单测中不可稳定构造）。
6. 前端覆盖率 ≥70%；trellis-check + 子 agent 审核。
7. 文档同步：路线图勾选取消子项、API参考.md 新增绑定方法（153→155）、logging-and-errors.md 错误码、CLAUDE.md 关键规则如涉及。

## 范围外

- 冲突类操作（merge/rebase/cherry-pick）的取消（走 ExecuteWithCodes，本任务不加 codes ctx 变体，留扩展点）。
- ContentPanel 现有 `EventsOff` 全局注销模式的整改（关键规则已登记，本任务不加新事件不扩散，另行任务收口）。
- 表格行进行中状态预填（需改 ScanAndPullRepos 返回或加 batch-start 事件，另行评估）。
