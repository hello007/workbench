# Git 并发操作控制

## Goal

为 WorkBench 的 Git 变更类操作引入并发控制，按仓库路径串行化，防止用户对同一仓库并发触发互相冲突的 Git 命令（如 pull 进行中又点 checkout、commit 未完又 discard、rebase 中途又 merge）导致工作区状态错乱或 `.git/index.lock` 竞争。桌面单用户场景，核心矛盾是同一仓库的写操作串行化，跨仓库天然安全可并行。

## Requirements

- 同一仓库路径的变更类 Git 操作串行化：通过 `sync.Mutex` TryLock 实现，锁已被占用立即返回错误「该仓库有 Git 操作进行中，请稍后重试」，不等待
- 不同仓库的操作互不阻塞（按仓库路径为键的独立锁）
- 只读操作不抢锁，可与变更操作并发
- 锁放 service 层统一拦截（所有变更操作经 `a.gitSvc.XXX` → service，单一收敛点）
- 前端对「操作进行中」冲突错误统一拦截，弹 `ElMessage.warning` 提示
- `BatchPull` 并发 worker 与手动单仓 PullRepo 抢同一仓锁，失败方被拒，符合预期
- 锁超时受现有 `Execute` 30s timeout 约束，锁持有 ≤ 30s，无需额外等待超时

## Acceptance Criteria

- [ ] 同仓库并发触发两个变更操作，第二个立即返回错误，不产生 git lock 竞争或工作区错乱
- [ ] 不同仓库的变更操作可并行执行
- [ ] 只读操作与变更操作可并发执行
- [ ] BatchPull 跨仓并行不受锁影响，同仓抢锁失败被拒
- [ ] 后端单元测试覆盖：同仓互斥拒绝、跨仓并行、只读不阻塞、锁正常释放（无泄漏）
- [ ] 前端对冲突错误有 `ElMessage.warning` 用户可见反馈
- [ ] `go test ./...` + `cd frontend && npm test` 全绿
- [ ] service 覆盖率 ≥76%、前端 ≥70%（exclude wailsjs）

## Definition of Done

- 后端测试覆盖率达门禁基线（service ≥76%）
- 前端测试覆盖率达门禁基线（≥70%，exclude wailsjs）
- `go test ./...` + `npm test` 全绿
- wails 绑定同步（如签名变更）
- README / 路线图勾选更新（Git 操作优化 → 并发操作控制）

## Technical Approach

**锁设计**：
- `service/git.go` GitService 新增 `opMu sync.Mutex` 保护 `opLocks map[string]*sync.Mutex`（仓库路径 → 锁）
- 变更操作入口调用 `s.tryLockRepo(dirPath) (release func(), err error)`：取/建该仓锁，`TryLock` 失败返 `ErrOperationInProgress`
- 成功返 release 闭包，`defer release()` 保证释放（含 panic 路径，无泄漏）
- 只读操作不加锁

**变更操作清单（抢锁）**：Clone/Pull/Push/Commit/DiscardChanges/StageFiles/UnstageFiles/CheckoutBranch/CreateBranch/DeleteBranch/RenameBranch/Merge/Rebase/CherryPick/ResolveConflict/ContinueMerge/ContinueRebase/ContinueCherryPick/AbortMerge/AbortRebase/AbortCherryPick/SkipRebase/CreateTag/DeleteTag/PushTag/AddRemote/RemoveRemote/FetchRepo/SetBranchUpstream/BatchPull 内部单仓 Pull

**只读操作清单（不抢锁）**：GetInfo/ExtractRepoName/GetRemote/HasRemote/GetBranches/GetLocalChanges/GetDiff/GetCommitFileDiff/GetRangeDiff/HasUpstream/GetTags/GetRemotes/GetConflictState/ScanGitRepos

**前端**：变更操作调用统一错误拦截，匹配 `ErrOperationInProgress` 文案 → `ElMessage.warning('该仓库有 Git 操作进行中，请稍后重试')`

**扩展点**：锁结构预留，未来升级排队（B）或取消+进度（C）只需替换 tryLockRepo 实现。

## Decision (ADR-lite)

**Context**: 路线图「Git 操作优化 → 并发操作控制」需防止同仓并发写操作冲突。现有 `Execute` 每次独立 30s timeout、GitService 无锁、前端同步绑定无进度事件。

**Decision**: 方案 A — service 层按仓库路径 `sync.Mutex` TryLock 互斥拒绝。锁粒度 A1 纯仓库锁（git index lock 本身全仓互斥，类别分锁收益低）。前端 F1 被动收错误 + warning 提示。锁等待 T1 立即拒绝不等待。

**Consequences**:
- 优点：复用现有 mutex 范式（filetree_cache/commit_history_cache），改动集中在 service 层 + 前端错误拦截，低风险
- 缺点：用户遇冲突需手动重试，无排队与进度
- 风险：锁 map 自身需 mutex 保护防并发读写；release 须 defer 保证无泄漏；BatchPull 与手动 PullRepo 同仓并发时失败方被拒需测试覆盖

## Out of Scope

- 操作排队等待（方案 B）
- 长操作取消 + context.Context 贯穿（方案 C）
- 进度事件 EventsEmit + 前端进度条
- 前端按钮主动禁用（GetOngoingOperations 轮询/事件，方案 F2）
- 操作类型细分锁（A2）

## Technical Notes

- `util/git.go` GitCommand.Execute：`context.WithTimeout(context.Background(), 30s)`，无 caller 取消传播
- `service/git.go` GitService 结构：`gitCmd *util.GitCommand` + `scanCache *ScanCacheManager`
- `app_git.go` 变更方法全经 `a.gitSvc.XXX`，锁放 service 层单一收敛
- `BatchPull`（service/git.go:700）：并发 5 跨仓 worker，每仓调 `s.Pull(repo)`，抢各自仓锁无跨仓死锁
- `ScanAndPullRepos`（app_git.go:66）：`go func` 异步跑 BatchPull，立即返回 summary
- 范式参考：`service/filetree_cache.go`、`service/commit_history_cache.go`（sync.Mutex + map + 深拷贝）
- 测试覆盖门禁见 [docs/spec/test-coverage-gate.md](docs/spec/test-coverage-gate.md)
