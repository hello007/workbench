# 终端进程退出检测去轮询

## Goal

Windows 终端会话改为事件驱动退出检测，进程退出后立即收到通知，不再依赖 500ms 轮询；同时消除输出泵 EOF 与进程等待双通道重复发送 `terminal-exit` 的问题。Unix 保留轮询兜底（平台语义差异另行处理）。

## What I already know

- `watchProcess` 每 500ms 轮询 `IsProcessRunning`，退出提示最坏延迟半秒（`service/terminal.go:325-356`）。
- 输出泵在 `Read` 出错时也会判定退出并发送 `terminal-exit`；与 `watchProcess` 存在重复发送窗口，两处均只判会话在册、不判 `Running`。
- Windows 侧已持有会话 PID，可 `OpenProcess(SYNCHRONIZE)` 获得独立等待句柄；`golang.org/x/sys/windows` 提供 `WaitForMultipleObjects`、`CreateEvent`、`SetEvent`。
- `conpty.ConPty.Wait(ctx)` 内部仍按 1 秒轮询，不能满足本次目标，不采用。
- `PtyProcess.Close()` 已幂等（`sync.Once`），当前先断 ConPTY 再关 Job；Job 回收保证关闭时进程会退出。
- Unix 侧 `cmd.Wait` 可事件化，但 Close 仅发 SIGTERM，进程可能不退出，贸然引入 Wait goroutine 会带来僵尸/泄漏风险，本次不做。

## Assumptions (temporary)

- 独立等待句柄创建失败时降级回 500ms 轮询，不影响会话创建。
- 停止事件先于/后于进程退出信号触发均安全（`WaitForMultipleObjects` 返回任一即结束）。

## Open Questions

- 无。范围定为 Windows 事件驱动 + Unix 轮询兜底（见 Decision）。

## Requirements (evolving)

- Windows：创建会话时打开独立 `SYNCHRONIZE` 进程句柄 + 手动重置停止事件，起等待 goroutine，`WaitForMultipleObjects(进程句柄, 停止事件)`。
- `PtyProcess` 暴露只读 `ExitCh() <-chan struct{}`；等待不可用时返回 nil，调用方回退轮询。
- `Close()`：断 ConPTY → 关 Job → `SetEvent` 停止等待 → 等 goroutine 退出 → 关闭句柄；全程幂等。
- `watchProcess`：`ExitCh() != nil` 时 `select` 等待退出或 `done`；nil 时保留现有 500ms 轮询。
- 退出事件去重：`TerminalSession` 新增原子 `MarkExited() bool`，输出泵与 watch 均只在状态跃迁成功时 emit `terminal-exit`。
- 等待链路失败只 `slog.Warn`，不影响终端创建。

## Acceptance Criteria (evolving)

- [x] Windows 进程自然退出后 `ExitCh()` 关闭（5 秒内，事件驱动，无 500ms 轮询参与）。
- [x] `Close()` 能唤醒等待中的 goroutine，`ExitCh()` 关闭，句柄全部释放。
- [x] `ExitCh()` 不可用时 `watchProcess` 走轮询兜底，不 panic、不提前退出。
- [x] 输出泵与 watch 同时判定退出时，`terminal-exit` 只发送一次。
- [x] 重复 `Close()` 不 panic，不重复关句柄。
- [x] 现有 `util` / `service` 终端测试通过，新增 Windows 专项测试与去重单测（全量 service 另存在既有 assets -race 竞争，与本任务无关）。

## Definition of Done (team quality bar)

- Windows 测试覆盖：自然退出通道、Close 唤醒、nil 回退；service 测试覆盖 `MarkExited` 双调用只 emit 一次。
- `go test ./util/ ./service/ -run "Terminal|Pty|Exit"`、`go vet`、`gofmt` 通过。
- 规范更新：`docs/spec/terminal-multi-session.md` 补退出检测契约（Windows 事件驱动 / Unix 轮询兜底 / 单次 emit）。
- 无新增依赖。

## Decision (ADR-lite)

**Context**：Windows 有可靠进程等待原语，可零延迟感知退出；Unix 的 `cmd.Wait` 在 Close 仅发 SIGTERM 的既有语义下可能永久阻塞，需配套 SIGKILL 策略，不宜与本次混做。

**Decision**：Windows 采用「独立 SYNCHRONIZE 句柄 + 停止事件 + WaitForMultipleObjects」事件驱动；等待链路不可用时回退现有 500ms 轮询；Unix 本次保留轮询。输出泵与 watch 统一经 `MarkExited` 去重。

**Consequences**：Windows 退出提示零轮询延迟，关闭路径不再依赖轮询周期；Unix 仍有最坏 500ms 延迟，跨平台语义不对称需在规范注明。新增每会话 2 个句柄 + 1 个等待 goroutine，Close 时全部释放。

## Out of Scope (explicit)

- Unix 事件化退出检测（`cmd.Wait` + SIGKILL 兜底），另立项。
- 输出聚合/背压优化。
- ConPTY 库替换或 go-pty 迁移。
- 前端退出提示样式调整。

## Technical Notes

- 关键文件：`util/pty_windows.go`、`util/pty_windows_test.go`、`service/terminal.go`、`model/terminal.go`、`service/terminal_test.go`。
- 等待句柄：`windows.OpenProcess(windows.SYNCHRONIZE, false, pid)`；停止事件：`windows.CreateEvent(nil, 1, 0, nil)`。
- goroutine：`WaitForMultipleObjects([]Handle{proc, stop}, false, windows.INFINITE)`，退出时 close(exitCh)；Close 侧 `SetEvent` 后等待 done，避免句柄关闭与等待并发。
- 去重单点：`MarkExited` 持 `TerminalSession.mu`，`Running` 已 false 返回 false。
- 关联调研：`docs/research/2026-11-04-终端内核开源方案调研与借鉴清单.md` 第 3.2 节。
