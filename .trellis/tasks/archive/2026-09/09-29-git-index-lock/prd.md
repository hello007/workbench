# Git 取消链路 index.lock 残留探测与引导

## Goal

Git 取消链路（Pull/Clone/BatchPull + 冲突类 forward 七方法）以 TerminateProcess 终止 git 子进程，commit 写窗口被杀后 `.git/index.lock` 残留，该仓后续全部写操作报 `index.lock exists`，UI 无探测无引导。本任务收口：后端探测残留（可观测）+ 锁错误分流引导（用户可操作）。

## 勘察结论（已完成）

### 取消链路现状
- util 层取消收口：`util/git.go` `run()` 取消分支（util/git.go:57-62），`ctx.Err() == context.Canceled` 时丢弃 stderr 返回 `%w` 包装 `context.Canceled`。
- service 层 9 处 `registerCancel`/`unregisterCancel`（git.go:244/271/1082/1566/1596/1626/1723/1750/1777）：Pull/Clone/BatchPull + Merge/Rebase/CherryPick/Continue 三类 + SkipRebase。
- 取消映射：`mapCancelError`（service/git.go:48）→ `ErrOperationCancelled`（E_GIT_CANCELLED），语义不变约束。

### 真实锁错误形态（本机实测，git exit 128）
```
fatal: Unable to create '<repo>/.git/index.lock': File exists.

Another git process seems to be running in this repository, e.g.
an editor opened by 'git commit'. Please make sure all processes
are terminated then try again. ...
remove the file manually to continue.
```
- 稳定匹配片段：`Unable to create` + `index.lock` + `File exists`。
- 现状：裸 `fmt.Errorf("git %v failed: %s", args, stderr)` 透传 → `formatAppError`（main.go:185）无 code → 前端 error 分流，无引导。

### 分流链路（复用，禁另起）
- `formatAppError`（main.go:185-196）：`errors.As(*model.AppError)` → `{code,message}`，单点收口。
- 前端 `frontend/src/utils/error.js`：`ErrorCode` 表 + `WARNING_CODES` Set + `handleError` 按 code 分流。
- 错误码三处同步约束：model/app_error.go + error.js + 文档。

### 探测时机原子性
- Windows `TerminateProcess` 返回后内核同步关闭全部句柄；git 以 O_CREAT|O_EXCL 创建 index.lock，创建即刻目录可见，无回滚。
- `cmd.Run()` 返回 = 进程已死，此后 `os.Stat` 探测无时序窗口。

### 依赖方向
- util 零业务依赖（不 import model）；model 仅依赖 fmt。保持惯例：util 定义自有锁错误类型，`formatAppError` 识别转码。

### 前端取消提示现状
- GitMerge.vue:214/309/353 `ElMessage.info('操作已取消')`、ContentPanel.vue:686 `'拉取已取消'`——均中性提示，无锁引导（本次不改取消提示本身，改后续失败分流）。

## What I already know

- 留笔：`.trellis/tasks/archive/2026-09/09-29-git-merge-rebase-cherry-pick/research/known-risks.md`，含「不自动删」决策理由（stale 判定不可靠，同仓可能有用户手动 git 进程合法持锁）。
- workDir 多数场景 = gitroot；子目录场景用 `FindGitRoot`（util/git.go 既有）向上解析；worktree 的 .git 是文件（gitdir: 指针），锁在真实 gitdir 下，边界场景静默跳过不误报。

## Decision (ADR-lite)

**Context**：方向 A（后端探测 Warn）/ 方向 B（错误分流引导）择一或组合；任务验收条款同时含探测单测（A）与错误码映射 + 前端分流用例（B），用户未另行选择。
**Decision**：A+B 组合。A = `run()` 取消分支探测残留 + slog.Warn；B = `run()` 识别锁 stderr → `util.GitLockError` → `formatAppError` 映射 `E_GIT_LOCKED` → 前端 WARNING_CODES warning 分流引导（文案带锁路径）。两方向共享 stderr 解析辅助。
**Consequences**：util 保持零业务依赖（不 import model，`formatAppError` 识别 `*util.GitLockError`）；`run()` 单点识别使锁错误引导覆盖所有 git 命令（超出取消链路面，零漏）；util→model 依赖未引入，分层不变。

## Requirements（evolving）

- 禁自动删 index.lock（硬约束）。
- 复用 AppError/ErrorFormatter/handleError 链路（硬约束）。
- 错误码三处同步（硬约束）。
- 不改 E_GIT_CANCELLED 既有语义（硬约束）。
- 探测失败静默降级，不阻塞取消主流程（硬约束）。

## Acceptance Criteria（evolving）

- [ ] 锁残留错误识别单测（真实 stderr 文案 → 错误码映射）
- [ ] 探测逻辑单测（构造/清除 index.lock 文件驱动，无时序依赖）
- [ ] 前端分流用例（warning 引导非 error）
- [ ] 非锁残留错误行为逐字不变
- [ ] go test ./... + race + soak + vitest 全绿，覆盖率不低于各包门禁

## Out of Scope（evolving）

- 自动删除 index.lock（stale 判定不可靠，误删合法锁）。
- Abort 三类（无 ctx 变体，秒级完成，无此面）。
- 取消提示文案本身改动（GitMerge.vue/ContentPanel.vue 的 info 提示保持）。
- 其他锁类型（refs 锁、shallow 锁等）。

## Technical Notes

- 勘察时间：2026-09-29；勘察人：主线程（codegraph + 本机 git 实测）。
