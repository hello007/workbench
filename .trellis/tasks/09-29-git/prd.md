# PRD：Git 超时+冲突码白名单命中语义收口

## 1. 背景与问题

前序任务 8abb17d（index.lock 残留收口）审核发现：`util/git.go` `run()` 分支序中，
超时（`context.DeadlineExceeded`）探测分支已置于白名单前（保锁残留告警），但**不返回**，
继续落入退出码白名单分支。超时被杀的 `pull --rebase`/`merge` 等子进程在 Windows
`TerminateProcess` 下产生 exit code 1，恰命中 `conflictExitCodes={1}`，白名单分支
返回 `(stdout, nil)`：

- service 层冲突类方法（PullRebase/Merge/Rebase/CherryPick 及 Continue 三类）把
  超时被杀误判为「冲突正常返回」，进入冲突流程；
- `GetConflictState` 探不到冲突则前端冲突面板卡空白假态；
- 超时被静默吞掉，无任何报错。

同文件 `clone()` 无白名单、无此误判，但超时同样落入 `parseGitLockError` + 裸错误路径：
被杀子进程 stderr 半截内容不可信，存在误判为 `E_GIT_LOCKED` 的理论风险，且超时以
`git clone failed: <空 stderr>` 裸错误呈现，无引导。

## 2. 目标（brainstorm 定案：方向 A + B 组合）

1. **方向 A（语义收口，必做）**：`run()` 中 `DeadlineExceeded` 分支在锁残留告警后
   **补返回错误**，不再落入白名单与锁 parse——超时不被吞、不被误判为冲突/锁。
2. **方向 B（错误码化）**：新增 `E_GIT_TIMEOUT` 专用错误码，超时错误从裸 error 升
   `*model.AppError` 引导级，前端 warning 分流「操作超时已终止」。

### 2.1 行为变更定义

| 维度 | 变更前 | 变更后 |
|---|---|---|
| run() 超时 + exit 1 命中白名单 | `(stdout, nil)` 冲突假态 | `*AppError{E_GIT_TIMEOUT}` |
| run() 超时未命中白名单 | 裸错误 `git [args] failed: <stderr>` | `*AppError{E_GIT_TIMEOUT}`（格式推翻，文档明示） |
| run() 超时 + stderr 半截疑似锁文本 | 可能误判 `E_GIT_LOCKED` | 直接超时分支返回，跳过锁 parse |
| clone() 超时 | 裸错误 `git clone failed: <stderr>` | `*AppError{E_GIT_TIMEOUT}`（同 run 收口） |
| 非超时冲突 exit 1 | `(stdout, nil)` | **不变** |
| 非超时锁占用 | `E_GIT_LOCKED` | **不变** |
| 取消链路 | `E_GIT_CANCELLED`（%w 包装 Canceled） | **不变** |
| 锁残留告警（超时/取消后） | slog.Warn 探测 | **不变**（告警仍在返回前执行） |

### 2.2 AppError 构造

- 常量：`ErrCodeGitTimeout = "E_GIT_TIMEOUT"`，注释「Git 操作超时被终止（warning，用户可重试或放大超时）」。
- 构造：`model.WrapAppError(model.ErrCodeGitTimeout, "Git 操作超时已终止，请重试", ctx.Err())`
  —— wrap `context.DeadlineExceeded` 保留 `errors.Is(err, context.DeadlineExceeded)` 穿透，
  与取消链 `%w` 包装模式对称。
- 覆盖点：`run()` 与 `clone()` 两处 `DeadlineExceeded` 分支统一收口（超时为同质失败模式，
  clone 大仓库超时同样常见，不一致收口留裸错误无意义）。

## 3. 三处同步（错误码同步规则）

1. `model/app_error.go` 常量表加 `ErrCodeGitTimeout`；
2. `frontend/src/utils/error.js` `ErrorCode` 加 `GitTimeout` + `WARNING_CODES` 加该码；
3. `docs/spec/logging-and-errors.md` 错误码表补行 + 超时格式推翻说明。

另：`CLAUDE.md` 关键规则「Git 操作取消链路」行追加超时收口句。

## 4. 测试（无时序依赖）

1. **超时+白名单命中**（核心验收）：`NewGitCommandWithTimeout(1ns)` +
   `ExecuteWithCodesWithContext(ctx, dir, conflictExitCodes, "status")`（临时仓 fixture），
   断言返回 err 非 nil、`errors.Is(err, context.DeadlineExceeded)`、code 为 `E_GIT_TIMEOUT`，
   断言非 `(stdout, nil)`。ctx 已过期时 `exec.CommandContext` Start 前即返回 ctx.Err()，
   无真杀进程、无 sleep，时序稳定。
2. **clone 超时**：同法 `CloneWithContext`（1ns），断言 `E_GIT_TIMEOUT`。
3. **非超时冲突回归**：既有 `util/git_context_test.go` exit 1 接受语义用例全绿。
4. **取消/锁识别既有用例全绿**：`go test ./...` 全量。

## 5. 非目标（审核确认的已知边界）

- `commit_history_cli.go` `streamCommitLog`（主包，30s 超时独立构造 git log 子进程）：
  无退出码白名单、无吞错 bug，超时仍返回裸错误 `git log failed: context deadline exceeded`。
  未纳入本次收口（超时极少触发、错误可读），后续若做「超时引导级全覆盖」再统一。

## 6. 约束

- 不改取消链路语义（E_GIT_CANCELLED）、不改锁识别链路（E_GIT_LOCKED 非超时路径）。
- 超时错误格式推翻须文档明示（logging-and-errors.md + CLAUDE.md）。
- 测试禁 sleep/固定端口/mtime 时序（1ns timeout 构造）。
- service 层零改动：`mapCancelError` 非 Canceled 原样透传 AppError，
  Wails ErrorFormatter 与 serve `rpcFailureFromCall` 双通道自动透传（8abb17d 先例）。

## 7. 验收

- [ ] 超时+白名单命中单测返回错误（非 (stdout, nil)）
- [ ] 非超时冲突 exit 1 接受语义回归不变
- [ ] 取消/锁识别既有用例全绿
- [ ] `go test ./...` + `-race` + vitest 全绿 + 覆盖率门禁
- [ ] 三处同步 + CLAUDE.md 关键规则行追加
- [ ] trellis-check + 子 agent 审核
