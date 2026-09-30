# PRD：Git log 流式超时收口（streamCommitLog → E_GIT_TIMEOUT 引导级）

## 摘要

前序 d9aec46 已将 util 层 `run()`/`clone()` 的 `DeadlineExceeded` 分支收口为 `E_GIT_TIMEOUT` 引导级 AppError，但主包 `commit_history_cli.go` 的 `streamCommitLog` 独立构造 git log 子进程（`const commitLogTimeout = 30s`），超时未收口：读循环/Wait 错误路径不判 `ctx.Err()`，超时杀进程后表现为裸错误（「读取 git log 输出失败: read |0: file already closed」或「git log failed: exit status ...」），前端提交历史报错无引导，与 util 层收口语义不一致。本任务补齐三处超时判定，统一返回 `E_GIT_TIMEOUT`。

## 背景（勘察结论）

- `streamCommitLog`（commit_history_cli.go:300）：`exec.CommandContext(ctx, ...)` + 30s 超时；错误出口四处——管道创建失败、`Start` 失败、读循环 `readErr`、`cmd.Wait` 失败。超时杀进程后管道断，`readErr`/`Wait` 报裸错误。
- **关键时序**：`finish()` 内先 `cancel()` 再 `Wait`——读循环分支的超时 err 在 `finish()` 之前捕获（防御性：标准库首个 cause 胜出，超时后再 `cancel()` 为 no-op 不覆写 `DeadlineExceeded`，但正确性不悬在该内部语义上，审核修正 2026-09-30）。
- **Start 分支必然命中（1ns 注入）**：util 先例（util/git_context_test.go:16 注释）证实「exec.CommandContext 对已取消 ctx 在 Start 前即返回 ctx.Err()」。`commitLogTimeout` 注入 1ns 时 Start 直接失败——该分支若不判定超时，1ns 单测无法确定性命中。真实世界 30s 超时几乎不会打在 Start，但语义一致（超时导致的启动失败仍是超时），一并收口。
- 调用方消费方式（两处，均无需改动）：
  - `fetchCommitHistoryFromGit`（app_git.go:504）→ `%w` 包裹「无法获取提交历史: %w」→ `formatAppError`（main.go:185）用 `errors.As` 穿透 → 前端拿到 `{code: E_GIT_TIMEOUT}`，三处同步（ErrorFormatter / 前端 error.js）d9aec46 已就绪，零改动。
  - `incrementalCommits`（app_git.go:365）→ 吞错落 `slog.Warn` 回退全量扫；全量走 `loadCommitLogBatch` → `util.Execute`，超时已由 d9aec46 收口，链路自洽。
- service 层无对该 error 的字符串匹配。
- 前端提交历史面板报错形态：`handleError` 按 code 分流，`E_GIT_TIMEOUT` 为 warning 中性提示（已有）。

## 范围

### 做

1. `commitLogTimeout`：`const` → 包级 `var`（测试注入点，注释说明测试临时改写 defer 还原）。
2. `streamCommitLog` 三处超时判定，命中返回 `model.WrapAppError(model.ErrCodeGitTimeout, "Git 操作超时已终止，请重试", ctx.Err())`（与 util 层逐字一致）：
   - `cmd.Start()` 失败分支：判定 `ctx.Err() == context.DeadlineExceeded`（1ns 注入确定性依赖）；
   - 读循环 `readErr` 非 EOF 分支：判定置于 `finish()` **之前**（cancel 覆写问题）；
   - `cmd.Wait()` 失败分支：EOF 路径 cancel 未触发，直接判定。
   - 不加锁残留探测：git log 只读不写 index，无 `index.lock` 残留风险（与 util run() 分叉点，注释说明）。
3. 测试（commit_history_cli_test.go）：
   - 超时单测：注入 1ns（defer 还原），断言 `errors.As` AppError + `Code == E_GIT_TIMEOUT` + `errors.Is DeadlineExceeded` + 非 `Canceled`；同步断言非「读取 git log 输出失败」类裸错误形态。
   - 非超时错误格式逐字回归：非 git 目录执行，断言错误串保留既有前缀（`启动 git log 失败` / `git log failed`）。
4. 文档同步：
   - `docs/spec/logging-and-errors.md` 收口段补句：E_GIT_TIMEOUT 覆盖 run/clone/streamCommitLog 三处。
   - CLAUDE.md 关键规则超时句追加 streamCommitLog。

### 不做（留笔）

- **提前终止控制流不动**：`handle` 返 false 关管道返 nil 属正常控制流。
- **解析错误路径与非超时错误格式逐字不变**：「git log 记录头部字段不完整」等解析错误、「git log failed: %w, stderr: %s」「启动 git log 失败」「读取 git log 输出失败」「创建 git log 管道失败」非超时格式全保留。
- **不涉及 E_GIT_CANCELLED**：streamCommitLog 无外部取消入口，ctx 仅内部超时驱动。
- **service/content_search.go 留笔（非 git 域，不入本任务）**：`app_search.go:70-76` 构造 60s(searchAll)/10s(单目录) ctx；`searchWithRipgrep`（service/content_search.go:166-172）rg 超时被杀（Windows TerminateProcess exit 1）恰命中 `ExitCode() == 1`「无匹配」分支，**静默返 `nil, nil`**——超时表现为「搜索无结果」，比裸错误更隐蔽。后续若收口，需区分 exit 1 无匹配与被杀（`ctx.Err()` 判定），错误码建议新立非 git 域码（如 E_SEARCH_TIMEOUT），不强行复用 E_GIT_TIMEOUT。

## 验收

- 超时单测绿（1ns 注入断言 E_GIT_TIMEOUT）。
- 非超时 git log 错误格式逐字回归绿；提前终止（TestStreamCommitLog_StopEarly）与解析错误既有用例全绿。
- `go test ./...` + `-race` + 前端 vitest 全绿。
- 文档同步完成（logging-and-errors.md + CLAUDE.md 关键规则）。
- trellis-check + 子 agent 审核通过。

## 技术方案备注

超时判定样板（与 util/git.go:67-69 同构）：

```go
if ctx.Err() == context.DeadlineExceeded {
    return model.WrapAppError(model.ErrCodeGitTimeout, "Git 操作超时已终止，请重试", ctx.Err())
}
```

读循环分支顺序约束：

```go
if readErr != nil {
    if readErr == io.EOF {
        break
    }
    // 超时 err 在 finish() 前捕获：finish 先 cancel 后 Wait，判定不依赖 cancel 时序
    err := ctx.Err()
    finish()
    if err == context.DeadlineExceeded {
        return model.WrapAppError(...)
    }
    return fmt.Errorf("读取 git log 输出失败: %w", readErr) // 非超时格式逐字不变
}
```
