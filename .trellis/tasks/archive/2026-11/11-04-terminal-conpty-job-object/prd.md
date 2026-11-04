# 终端关闭回收 ConPTY 子进程

## Goal

关闭终端 tab / 退出应用时，回收该会话 PTY 下的整棵进程树，避免交互式 shell 及其派生的长时间任务（`ping -t`、`npm run dev`、`sleep`）在后台残留。

## What I already know

- 当前 Windows PTY 使用 `UserExistsError/conpty v0.1.4`（`util/pty_windows.go`），`Close()` 调用 `ClosePseudoConsole` 后关闭句柄。
- 项目注释已实测记录：`ClosePseudoConsole` 只断 ConPTY 句柄，交互式 shell 可在句柄断开后驻留（`service/terminal.go` 的 `watchProcess` 注释）；子进程更不会随 shell 句柄关闭而必然退出。
- `conpty.ConPty` 暴露 `Pid()`，未暴露进程句柄；可用 `windows.OpenProcess` 按 PID 取得句柄后加入 Job Object。
- Windows Job Object 设 `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` 后，最后一个 Job 句柄关闭即终止 Job 内全部进程；应用崩溃时 OS 关闭句柄同样生效。
- Unix 侧 `creack/pty` 的 `StartWithAttrs` 设置 `Setsid`，shell 自成进程组；当前 `pty_other.go` 的 `Close` 只向 shell PID 发 `SIGTERM`，未杀进程组。
- 目标平台为 Windows 10 1809+（ConPTY 前置条件），支持嵌套 Job；WorkBench 自身是否在 Job 内不影响子进程新建 Job 后加入。
- 日志规范：`log/slog`，util 层可直接 `slog.Warn`。

## Assumptions (temporary)

- Job 绑定失败不应阻断终端创建，降级为「关闭时可能残留子进程」并记录 Warn。
- 分配 Job 发生在 `conpty.Start` 返回后，存在极小的「首个子进程先于绑定启动」竞态，可接受。
- 关闭顺序：先 `ClosePseudoConsole`（让 shell 感知关闭），再关闭 Job 句柄（强杀整树）。

## Open Questions

- 无。范围已定为仅 Windows（见 Decision）。

## Requirements (evolving)

- Windows：每个终端会话创建时绑定独立 Job Object，设置 `KILL_ON_JOB_CLOSE`。
- Windows：`PtyProcess.Close()` 关闭 Job 句柄，终止 shell 及其全部后代。
- 绑定失败：记录 Warn，终端仍可创建；不得 panic、不得阻断会话。
- `Close()` 幂等：重复调用不重复关闭句柄、不返回二次关闭错误。
- 不改变 `PtyProcess` 现有方法语义与调用方。

## Acceptance Criteria (evolving)

- [x] 终端内启动 `ping -t 127.0.0.1` 后关闭 tab，ping 进程在 2 秒内消失（实测 5 秒轮询窗口内通过）。
- [x] 关闭 tab 后 shell 及其子进程均无残留（按 PID 与父进程链断言）。
- [x] Job 绑定失败时终端仍可正常创建与使用，日志含 pid 与错误原因（降级单点 `bindJobOrWarn` 单测覆盖，创建流程无错误返回路径）。
- [x] `Close()` 连续调用两次不 panic、第二次不产生句柄错误。
- [x] 现有 `util` / `service` 终端测试全部通过；新增 Windows 专项测试。
- [x] Unix 行为不变（本次仅 Windows，`pty_other.go` 零改动）。

## Definition of Done (team quality bar)

- Windows 专项测试覆盖：Job 创建成功、绑定后关闭杀树、重复 Close、绑定失败降级（可注入）。
- `go test ./util/ ./service/` 通过；`go vet`、`gofmt` 干净。
- 规范沉淀：`docs/spec/terminal-multi-session.md` 或 `process-management.md` 补进程树回收契约。
- 无新增第三方依赖（仅 `golang.org/x/sys/windows`）。

## Decision (ADR-lite)

**Context**：Windows 下 `ClosePseudoConsole` 不能保证回收 shell 派生的子进程；Unix 侧 `creack/pty` 已通过 `Setsid` 建立独立会话，风险与改造面另计。

**Decision**：本次仅改 Windows 实现，为每个会话绑定 `KILL_ON_JOB_CLOSE` 的 Job Object；Unix 进程组回收不纳入本任务，后续按需另立项。

**Consequences**：Windows 关闭 tab 与异常退出均可整树回收；Unix 仍保持现状（SIGTERM shell PID），跨平台语义暂不完全一致，需在规范中注明。

## Out of Scope (explicit)

- 不改退出检测轮询（P2 独立任务）。
- 不改 ConPTY 库或引入 go-pty 迁移。
- 不做跨会话全局进程管理/工具箱联动。
- 不处理应用被强杀（Task Manager 结束进程）之外的异常退出场景；Job 句柄随进程终止由 OS 回收。

## Technical Notes

- 关键文件：`util/pty_windows.go`、`util/pty_windows_test.go`、`util/pty_other.go`（若含 Unix 范围）。
- 参考 API：`windows.CreateJobObject`、`windows.SetInformationJobObject`、`windows.AssignProcessToJobObject`、`windows.OpenProcess(PROCESS_SET_QUOTA|PROCESS_TERMINATE)`。
- 现有 `PtyProcess` 结构仅含 `conPty` 与 `pid`，需新增 `job windows.Handle` 字段。
- 测试可用 `cmd.exe /c ping -t 127.0.0.1` 或 `powershell -NoProfile -Command "Start-Sleep 60"` 作为长任务，关闭后按 PID 轮询存活。
- 关联调研：`docs/research/2026-11-04-终端内核开源方案调研与借鉴清单.md` 第 3.2 节。
