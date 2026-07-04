# 工作目录 git 标识缓存与异步刷新优化启动速度

## Goal

修复启动 exe 后工作目录树列表显示延迟的问题——根因是上轮（workdir-git-detail）在 `GetDirectories` 里同步遍历所有工作目录跑 `git rev-parse --git-dir` 检测 `IsGitRepo`，N 个目录 N 次子进程阻塞 UI。优化为：**添加/修改时计算并持久化 `IsGitRepo` 到 `directories.json`，启动时直接读缓存秒回，启动后异步刷新一次覆盖"后来才纳管为 git"的情况**。

## What I already know（已探明的事实）

### 当前链路与根因

- `app.go:GetDirectories`（第 90-93 行）：`directorySvc.Load()` 后**同步**对每个 `dir` 跑 `util.NewGitCommand().IsGitRepository(d.Path)` 填充 `IsGitRepo` 再返回。前端 `Home.onMounted → loadDirectories → GetDirectories` 必须等这个同步检测完成才能渲染列表 → 启动延迟。
- `DirectoryService.Load/Save` 走 `data/directories.json`；`Create/Update/Delete/SetDefault/Reorder` 均在改动后 `Save`，但 `IsGitRepo` 从未在持久化时写入有效值（Create 不检测）。
- `model.Directory.IsGitRepo` json tag `isGitRepo`；旧配置无该字段时零值 false（上轮 `TestDirectory_OldConfigBackwardCompat` 已证）。
- `applyGitRepoFlag` 单条检测辅助：用于 `AddDirectory/UpdateDirectory/GetDefaultDirectory`。
- `IsGitRepository` 在路径不存在 / git 未安装 / 检测异常时返回 false，不报错。

### 用户方案（已明确）

1. 添加工作目录时自动计算 `IsGitRepo` 并**保存到 directories.json**。
2. 启动时 `GetDirectories` **不再同步检测**，直接返回缓存的 `IsGitRepo`（秒回，UI 立即渲染）。
3. 启动后**异步刷新一次**：重新检测所有目录，更新 `IsGitRepo`（含"刚开始非 git、后续纳管到 git"的情况），并**回写 directories.json**。

## Assumptions（待验证）

- 工作目录数量通常 <10，刷新总耗时 1-3 秒内可接受；UI 先用缓存渲染，刷新期间不阻塞操作。
- 异步刷新结果对前端可见（左栏 git 标记可能延迟刷新）。
- 检测失败仍按 false 处理，不报错。

## Open Questions（仅阻塞/偏好类）

1. **[已定]** 刷新回传机制 = **同步 API `RefreshDirectoriesGitFlag()` + 前端 `await`**（UI 先渲染缓存，await 期间不阻塞操作，无需事件注册）。✅
2. **[已定]** 刷新范围 = **全量刷新所有目录**（简单、保证新鲜；兼顾"git 变非 git"的逆向变化）。✅

## Decision (ADR-lite)

**Context**：上轮在 `GetDirectories` 同步检测导致启动延迟；需把检测移出启动关键路径，同时保持"工作目录是否 git 仓库"的标记新鲜。

**Decision**：
1. `IsGitRepo` 改为**持久化字段**（写入 `directories.json`），在 `Create`/`Update(path 变)` 时检测并保存。
2. `GetDirectories` **去掉同步检测**，直接返回 `Load()` 结果（启动秒回）。
3. 新增 `RefreshDirectoriesGitFlag()` 同步 API：全量检测所有目录 `IsGitRepo` → 基于**最新 Load** 合并（只更新 IsGitRepo，保留其他字段最新值，规避与用户并发 AddDirectory 的竞态）→ `Save` 回写 → 返回新列表。
4. 前端 `Home.onMounted`：先 `loadDirectories()`（缓存秒显）→ 再 `await RefreshDirectoriesGitFlag()` 用返回值替换 `directories`。

**Consequences**：
- 优点：启动快（GetDirectories 零子进程）；标记新鲜（启动后刷新覆盖纳管变化）；持久化保证离线/二次启动仍秒显；无事件机制复杂度。
- 代价：刷新期间（1-3 秒）标记可能短暂显示缓存值（用户可接受）；前端调用栈 await（但 UI 不阻塞）。
- 风险：并发竞态（刷新 Save vs 用户 AddDirectory）已用"Save 前基于最新 Load 合并"规避；检测失败按 false（不报错、不写脏数据，下次刷新自愈）。

## Requirements（演进中）

- `GetDirectories` 去掉同步检测循环，直接返回 `Load()` 结果（启动快）。
- `AddDirectory`（Create）与 `UpdateDirectory`（Update，path 变化时）检测 `IsGitRepo` 并**持久化**到 `directories.json`。
- 新增 `RefreshDirectoriesGitFlag` 能力：启动后异步检测所有目录、更新 `IsGitRepo`、回写 `directories.json`、结果回前端（替换前端列表）。
- 旧 `directories.json`（无 `isGitRepo`）兼容：启动时读 false，异步刷新后补正并持久化。
- 左栏 git 标记、右栏 git 详情直显行为不回归（数据源仍是 `Directory.IsGitRepo`）。

## Acceptance Criteria（演进ing）

- [ ] 启动后工作目录列表**立即显示**（无同步检测延迟），`IsGitRepo` 取自 `directories.json` 缓存。
- [ ] 新增工作目录 → `directories.json` 立即写入正确的 `isGitRepo`。
- [ ] 启动后异步刷新完成 → 左栏 git 标记按最新状态显示，`directories.json` 被回写。
- [ ] "旧非 git 目录后纳管为 git"场景：刷新后标记从无变有，且持久化。
- [ ] 上轮的"选中 git 工作目录右栏直显详情"行为不回归。
- [ ] `go test ./...` 与 `npm test` 全绿。

## Definition of Done

- 后端单测：Create/Update 持久化 IsGitRepo、Refresh 检测+回写、Load 不再触发检测、旧配置兼容。
- 前端：启动流程改为先 `loadDirectories`（缓存）→ 异步刷新；左栏标记不回归。
- `npm test` / `npm run build` / `go test ./...` 全绿。
- 文档：若行为变化（启动流程）需确认更新 `docs/功能说明.md` / `README.md`。

## Out of Scope（明确排除）

- 文件系统监听（自动感知目录变 git 仓库）——刷新仍由启动触发。
- 检测其他属性（分支名、未提交数、远端等）。
- 手动"立即刷新"按钮（除非用户要求）。
- 多仓库子目录递归扫描。

## Technical Notes

- 关键文件：
  - 后端：`service/directory.go`（Create/Update/Save 持久化 IsGitRepo；Load 不检测）、`app.go`（GetDirectories 去检测、新增 RefreshDirectoriesGitFlag、Create/Update 持久化）、`util/git.go`（IsGitRepository，复用）。
  - 前端：`frontend/src/views/Home.vue`（onMounted 流程：loadDirectories → 异步刷新 + 事件监听）、`components/DirectoryTree.vue`（标记数据源不变）。
- Wails 事件机制：后端 `runtime.EventsEmit(ctx, name, data)`；前端 `EventsOn/EventsOff`（ContentPanel 已有 pull-progress 事件模式可参考）。
- 并发：异步刷新与用户同时 AddDirectory 可能竞态（刷新 goroutine 用旧快照 Save 覆盖新目录）；需要 Save 时基于最新 Load（或检测期间加锁/重试）。MVP 可先按"基于最新 Load 合并"处理。
