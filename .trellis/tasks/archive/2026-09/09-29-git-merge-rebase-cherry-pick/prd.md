# PRD：Git 冲突类操作取消贯穿（merge/rebase/cherry-pick 接入取消链路）

## 背景

前序任务 866b7a1 已收口 Pull/Clone/批量拉取的取消链路，并为冲突类操作明确留笔扩展点：

- util 层 `ExecuteWithContext`/`ExecuteWithCodesWithContext` 已就绪（conflictExitCodes 语义保留、取消检测先于白名单分支）；
- service 层取消注册表（`cancels` 与 `opLocks` 同 `opMu` 保护、`cancelEntry` 条目指针身份校验、`mapCancelError` 映射 `E_GIT_CANCELLED`）全部现成；
- 冲突类操作 merge/rebase/cherry-pick 未贯穿（明确留笔）。

现状勘察：

- `service/git.go` Merge/Rebase/CherryPick 走 `precheckMutation`（多次快命令检查，不值得取消）+ `gitCmd.Merge/Rebase/CherryPick`（ExecuteWithCodes conflictExitCodes，冲突 exit 1 正常返回——长挂起点在真正执行步）；
- ContinueMerge/ContinueRebase/ContinueCherryPick/SkipRebase 走 `requireInProgress` + 各自封装（冲突解决后继续执行也可能再冲突长时间挂起）；
- 前端操作入口在 GitMerge.vue / GitBranches.vue 等组件，操作中态（loading）取消入口需勘察挂载点。

## 需求范围

### 1. util 层：薄委托 ctx 变体（7 方法）

模式照抄 `PullWithContext`，内部走 `ExecuteWithCodesWithContext` + conflictExitCodes：

- `MergeWithContext`
- `RebaseWithContext`
- `CherryPickWithContext`
- `MergeContinueWithContext`
- `RebaseContinueWithContext`
- `CherryPickContinueWithContext`
- `RebaseSkipWithContext`

**Abort 三类（MergeAbort/RebaseAbort/CherryPickAbort）刻意不提供 ctx 变体**——abort 本身就是回滚终止手段（且秒级），被中途取消可能留半态。

### 2. service 层：forward 七方法接入取消注册表

Merge/Rebase/CherryPick/ContinueMerge/ContinueRebase/ContinueCherryPick/SkipRebase 按既有模式接入：

- `tryLockRepo` 后自建 `context.WithCancel` + `registerCancel`/`unregisterCancel` defer LIFO（先注销后释放锁）；
- 仅执行步走 ctx 变体；`precheckMutation`/`requireInProgress` 不贯穿（快命令，保持现状）；
- `mapCancelError` 映射 `ErrOperationCancelled`。

### 3. App 层 + 前端：取消入口

- 勘察操作中态挂载点，Merge/Rebase/CherryPick 进行中给取消入口（复用 `CancelGitOperation` 单仓取消 + activePullPath 快照模式防切节点打错仓）；
- 取消后「已取消」中性提示（info，非 error）；
- 冲突面板（GitMerge.vue）合并/变基/拣选执行按钮 loading 态配取消。

## 约束

- 复用既有取消注册表与 mapCancelError，禁另起路径；
- precheckMutation/requireInProgress/Abort 三类保持现状（明确范围外）；
- 现有 Merge/Rebase/CherryPick 等签名全部不动；
- 冲突 exit 1 接受语义保留（ExecuteWithCodesWithContext 已保证）;
- 浏览器 serve 通道无新事件（取消经既有返回值链路）；
- service 改动过 soak 与 goleak。

## 验收标准

1. 冲突类七方法取消链路单测（照抄 git_cancel_test.go ctx 预取消模式：注册表生命周期、ctx 已取消跳过执行、mapCancelError 映射、abort 无 ctx 变体的负向断言可选）；
2. 取消后重试可正常获锁（注册表清理）；
3. 非取消路径（真冲突/真失败）行为逐字不变；
4. go test ./... + race + soak + vitest 全绿 + 覆盖率 ≥70%；
5. trellis-check + 子 agent 审核；
6. 文档同步：路线图并发控制条目扩充覆盖范围、API参考如新增绑定、CLAUDE.md 关键规则取消行更新覆盖清单「现有覆盖 Pull/Clone/BatchPull + 冲突类 forward 七方法」。
